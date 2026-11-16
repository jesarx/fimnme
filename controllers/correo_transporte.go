// Este archivo es parte de FIMNME, la plataforma de postulaciones del Foro
// Internacional de Música Nueva Manuel Enríquez (CNMO / INBAL).
//
// FIMNME es software libre: usted puede redistribuirlo y/o modificarlo bajo
// los términos de la Licencia Pública General de GNU, tal como la publica la
// Free Software Foundation, en la versión 3 de la Licencia.
//
// FIMNME se distribuye con la esperanza de que sea útil, pero SIN GARANTÍA
// ALGUNA; ni siquiera la garantía implícita de COMERCIABILIDAD o de APTITUD
// PARA UN PROPÓSITO PARTICULAR. Consulte la Licencia Pública General de GNU
// para más detalles.
//
// Debió recibir una copia de la Licencia Pública General de GNU junto con
// este programa, en el archivo LICENSE. Si no, consulte
// <https://www.gnu.org/licenses/>.
//
// SPDX-License-Identifier: GPL-3.0-only
package controllers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Transportes de correo reales (paso 14, §5.9). La bandeja de salida y su
// trabajador no cambian: sólo cambia qué hace Enviar con cada mensaje.
//
//   - MailerArchivo guarda cada mensaje completo como .eml en un directorio.
//     Sirve donde no puede enviarse correo (un VPS con el puerto 25
//     bloqueado): el mensaje es exactamente el que se habría enviado.
//   - MailerSMTP lo entrega por SMTP: al Postfix local (que firma con DKIM y
//     entrega) en la instalación final, o a un relevo externo por el puerto
//     587 (STARTTLS) o 465 (TLS directo).

// LectorDeArchivos es lo que los transportes necesitan para leer los
// adjuntos: el registro del archivo y su contenido en el acervo.
type LectorDeArchivos struct {
	Datos   models.Persistencia
	Almacen models.BlobStore
}

// adjuntos lee del acervo los adjuntos de un correo.
func (l LectorDeArchivos) adjuntos(ctx context.Context, c models.CorreoSaliente) ([]views.AdjuntoMIME, func(), error) {
	var abiertos []func() error
	cerrar := func() {
		for _, f := range abiertos {
			_ = f() // sólo lectura: un fallo al cerrar no cambia lo enviado
		}
	}
	var lista []views.AdjuntoMIME
	for _, a := range c.Adjuntos {
		archivo, err := l.Datos.ArchivoPorID(ctx, a.ArchivoID)
		if err != nil {
			cerrar()
			return nil, nil, fmt.Errorf("adjunto %d: %w", a.ArchivoID, err)
		}
		contenido, err := l.Almacen.Abrir(archivo.Clave)
		if err != nil {
			cerrar()
			return nil, nil, fmt.Errorf("adjunto %d: %w", a.ArchivoID, err)
		}
		abiertos = append(abiertos, contenido.Close)
		lista = append(lista, views.AdjuntoMIME{Nombre: a.Nombre, Tipo: archivo.MIME, Contenido: contenido})
	}
	return lista, cerrar, nil
}

// componer arma el mensaje MIME de un correo con sus adjuntos.
func (l LectorDeArchivos) componer(ctx context.Context, c models.CorreoSaliente, remitente string) ([]byte, string, error) {
	adjuntos, cerrar, err := l.adjuntos(ctx, c)
	if err != nil {
		return nil, "", err
	}
	defer cerrar()
	return views.MensajeMIME(c, remitente, dominioDe(remitente), time.Now(), adjuntos)
}

// dominioDe extrae el dominio de una dirección («FIMNME <foro@inba.gob.mx>»).
func dominioDe(direccion string) string {
	direccion = strings.TrimSuffix(strings.TrimSpace(direccion), ">")
	if i := strings.LastIndex(direccion, "@"); i >= 0 {
		return direccion[i+1:]
	}
	return "localhost"
}

// MailerArchivo guarda cada correo como un archivo .eml, con su número y su
// Message-ID en el nombre. No envía nada.
type MailerArchivo struct {
	LectorDeArchivos
	Directorio string
	Remitente  string
}

// Enviar escribe el mensaje en el directorio y lo da por entregado.
func (m MailerArchivo) Enviar(ctx context.Context, c models.CorreoSaliente) (string, error) {
	mensaje, messageID, err := m.componer(ctx, c, m.Remitente)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.Directorio, 0o750); err != nil {
		return "", err
	}
	nombre := fmt.Sprintf("%06d_%s.eml", c.ID, time.Now().UTC().Format("20060102T150405Z"))
	// Los enlaces de un solo uso son secretos: el archivo sólo lo lee el servicio.
	if err := os.WriteFile(filepath.Join(m.Directorio, nombre), mensaje, 0o600); err != nil {
		return "", err
	}
	return messageID, nil
}

// Modos de seguridad de la conexión SMTP.
const (
	SMTPStartTLS = "starttls" // 587: se cifra después del saludo (lo normal con un relevo)
	SMTPTLS      = "tls"      // 465: cifrada desde el principio
	SMTPSinTLS   = "ninguno"  // sólo al Postfix local en 127.0.0.1:25
)

// MailerSMTP entrega cada correo a un servidor SMTP.
type MailerSMTP struct {
	LectorDeArchivos
	Servidor  string // host:puerto
	Seguridad string // starttls, tls o ninguno
	Usuario   string // vacío: sin autenticación (Postfix local)
	Password  string
	Remitente string // «FIMNME <foro@dominio>»; su dirección es el remitente del sobre
	Plazo     time.Duration
}

// errSMTPSinCifrar impide mandar una contraseña o un correo por la red sin
// cifrar: «ninguno» sólo se admite hacia la propia máquina.
var errSMTPSinCifrar = errors.New("SMTP sin cifrar sólo se admite hacia 127.0.0.1 o localhost")

// Enviar entrega el mensaje y devuelve su Message-ID.
func (m MailerSMTP) Enviar(ctx context.Context, c models.CorreoSaliente) (string, error) {
	mensaje, messageID, err := m.componer(ctx, c, m.Remitente)
	if err != nil {
		return "", err
	}
	host, _, err := net.SplitHostPort(m.Servidor)
	if err != nil {
		return "", fmt.Errorf("servidor SMTP %q: %w", m.Servidor, err)
	}
	plazo := m.Plazo
	if plazo == 0 {
		plazo = 30 * time.Second
	}
	marcador := &net.Dialer{Timeout: plazo}
	var conexion net.Conn
	switch m.Seguridad {
	case SMTPTLS:
		conexion, err = tls.DialWithDialer(marcador, "tcp", m.Servidor, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	case SMTPStartTLS:
		conexion, err = marcador.DialContext(ctx, "tcp", m.Servidor)
	case SMTPSinTLS:
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return "", errSMTPSinCifrar
		}
		conexion, err = marcador.DialContext(ctx, "tcp", m.Servidor)
	default:
		return "", fmt.Errorf("modo de seguridad SMTP desconocido: %q", m.Seguridad)
	}
	if err != nil {
		return "", fmt.Errorf("conectar con %s: %w", m.Servidor, err)
	}
	if err := conexion.SetDeadline(time.Now().Add(plazo)); err != nil {
		conexion.Close()
		return "", err
	}
	cliente, err := smtp.NewClient(conexion, host)
	if err != nil {
		conexion.Close()
		return "", err
	}
	defer cliente.Close()
	if m.Seguridad == SMTPStartTLS {
		if err := cliente.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return "", fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if m.Usuario != "" {
		if err := cliente.Auth(smtp.PlainAuth("", m.Usuario, m.Password, host)); err != nil {
			return "", fmt.Errorf("autenticación SMTP: %w", err)
		}
	}
	if err := cliente.Mail(direccionDe(m.Remitente)); err != nil {
		return "", err
	}
	if err := cliente.Rcpt(c.Destinatario); err != nil {
		return "", err
	}
	escritor, err := cliente.Data()
	if err != nil {
		return "", err
	}
	if _, err := escritor.Write(mensaje); err != nil {
		return "", err
	}
	if err := escritor.Close(); err != nil {
		return "", err
	}
	return messageID, cliente.Quit()
}

// direccionDe extrae la dirección de «Nombre <dir@dominio>».
func direccionDe(remitente string) string {
	if i := strings.LastIndex(remitente, "<"); i >= 0 {
		return strings.TrimSuffix(strings.TrimSpace(remitente[i+1:]), ">")
	}
	return strings.TrimSpace(remitente)
}
