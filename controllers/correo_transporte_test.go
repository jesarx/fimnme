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
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de los transportes de correo (paso 14): el mensaje MIME, el
// transporte a archivo y la entrega SMTP contra un servidor falso.

// archivosDePrueba es una persistencia que sólo sabe de un archivo.
type archivosDePrueba struct {
	models.Persistencia
	archivo models.Archivo
}

func (a archivosDePrueba) ArchivoPorID(_ context.Context, id int64) (models.Archivo, error) {
	if id != a.archivo.ID {
		return models.Archivo{}, models.ErrNoEncontrado
	}
	return a.archivo, nil
}

// correoConAcuse prepara un acervo con un PDF y un correo que lo adjunta.
func correoConAcuse(t *testing.T) (LectorDeArchivos, models.CorreoSaliente) {
	t.Helper()
	almacen, err := models.AbrirAlmacenEnDisco(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blob, err := almacen.Guardar(strings.NewReader("%PDF-1.7 acuse"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	lector := LectorDeArchivos{Datos: archivosDePrueba{archivo: models.Archivo{ID: 7, Clave: blob.Clave, MIME: "application/pdf"}}, Almacen: almacen}
	return lector, models.CorreoSaliente{ID: 42, Destinatario: "ana@ejemplo.mx", Idioma: "es",
		Asunto: "Acuse de tu postulación FIM49-INT-0137 · señal\r\nBcc: intruso@ejemplo.mx", Texto: "Hola, Ana.\n", HTML: "<p>Hola, Ana.</p>",
		Adjuntos: []models.Adjunto{{ArchivoID: 7, Nombre: "acuse_FIM49-INT-0137.pdf"}}}
}

// leerMensaje interpreta un mensaje MIME y devuelve el asunto decodificado,
// el texto plano y el nombre y contenido del adjunto.
func leerMensaje(t *testing.T, crudo []byte) (cabeceras mail.Header, texto, adjunto string, contenido []byte) {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(crudo))
	if err != nil {
		t.Fatalf("mensaje ilegible: %v\n%s", err, crudo)
	}
	_, parametros, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
	partes := multipart.NewReader(m.Body, parametros["boundary"])
	for {
		parte, err := partes.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		tipo, p, _ := mime.ParseMediaType(parte.Header.Get("Content-Type"))
		if tipo == "multipart/alternative" {
			alternativa, err := multipart.NewReader(parte, p["boundary"]).NextPart()
			if err != nil {
				t.Fatal(err)
			}
			datos, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, alternativa))
			texto = string(datos)
			continue
		}
		adjunto = parte.FileName()
		contenido, _ = io.ReadAll(base64.NewDecoder(base64.StdEncoding, parte))
	}
	return m.Header, texto, adjunto, contenido
}

func TestMensajeMIMEYTransporteAArchivo(t *testing.T) {
	lector, correo := correoConAcuse(t)
	directorio := filepath.Join(t.TempDir(), "correo")
	mailer := MailerArchivo{LectorDeArchivos: lector, Directorio: directorio, Remitente: "FIMNME <foro@ejemplo.mx>"}
	messageID, err := mailer.Enviar(context.Background(), correo)
	if err != nil || !strings.HasPrefix(messageID, "<42.") || !strings.HasSuffix(messageID, "@ejemplo.mx>") {
		t.Fatalf("envío a archivo: %q %v", messageID, err)
	}
	archivos, _ := filepath.Glob(filepath.Join(directorio, "000042_*.eml"))
	if len(archivos) != 1 {
		t.Fatalf("archivos: %v", archivos)
	}
	if info, _ := os.Stat(archivos[0]); info.Mode().Perm() != 0o600 {
		t.Errorf("permisos del .eml: %v", info.Mode().Perm())
	}
	crudo, _ := os.ReadFile(archivos[0])
	cabeceras, texto, adjunto, contenido := leerMensaje(t, crudo)
	asunto, _ := new(mime.WordDecoder).DecodeHeader(cabeceras.Get("Subject"))
	if !strings.HasPrefix(asunto, "Acuse de tu postulación FIM49-INT-0137 · señal") || cabeceras.Get("Bcc") != "" ||
		cabeceras.Get("Message-ID") != messageID || cabeceras.Get("To") != "ana@ejemplo.mx" {
		t.Errorf("cabeceras: %v (asunto %q)", cabeceras, asunto)
	}
	if texto != "Hola, Ana.\n" || adjunto != "acuse_FIM49-INT-0137.pdf" || string(contenido) != "%PDF-1.7 acuse" {
		t.Errorf("cuerpo %q, adjunto %q %q", texto, adjunto, contenido)
	}
}

// servidorSMTPFalso atiende una sola sesión SMTP sin cifrar y devuelve lo
// que recibió en DATA y el remitente y destinatario del sobre.
func servidorSMTPFalso(t *testing.T) (direccion string, recibido chan [3]string) {
	t.Helper()
	escucha, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { escucha.Close() })
	recibido = make(chan [3]string, 1)
	go func() {
		conexion, err := escucha.Accept()
		if err != nil {
			return
		}
		defer conexion.Close()
		lector := bufio.NewReader(conexion)
		responder := func(linea string) { io.WriteString(conexion, linea+"\r\n") }
		responder("220 falso ESMTP")
		var de, para string
		var datos strings.Builder
		for {
			linea, err := lector.ReadString('\n')
			if err != nil {
				return
			}
			orden := strings.ToUpper(strings.TrimSpace(linea))
			switch {
			case strings.HasPrefix(orden, "EHLO"), strings.HasPrefix(orden, "HELO"):
				responder("250 falso")
			case strings.HasPrefix(orden, "MAIL FROM:"):
				de = strings.TrimSpace(linea[10:])
				responder("250 ok")
			case strings.HasPrefix(orden, "RCPT TO:"):
				para = strings.TrimSpace(linea[8:])
				responder("250 ok")
			case orden == "DATA":
				responder("354 adelante")
				for {
					renglon, err := lector.ReadString('\n')
					if err != nil || renglon == ".\r\n" {
						break
					}
					datos.WriteString(strings.TrimPrefix(renglon, "."))
				}
				responder("250 encolado")
				recibido <- [3]string{de, para, datos.String()}
			case orden == "QUIT":
				responder("221 adiós")
				return
			default:
				responder("250 ok")
			}
		}
	}()
	return escucha.Addr().String(), recibido
}

func TestEntregaPorSMTP(t *testing.T) {
	lector, correo := correoConAcuse(t)
	direccion, recibido := servidorSMTPFalso(t)
	mailer := MailerSMTP{LectorDeArchivos: lector, Servidor: direccion, Seguridad: SMTPSinTLS, Remitente: "FIMNME <foro@ejemplo.mx>"}
	messageID, err := mailer.Enviar(context.Background(), correo)
	if err != nil {
		t.Fatalf("entrega SMTP: %v", err)
	}
	sesion := <-recibido
	if sesion[0] != "<foro@ejemplo.mx>" || sesion[1] != "<ana@ejemplo.mx>" {
		t.Errorf("sobre: %q → %q", sesion[0], sesion[1])
	}
	cabeceras, _, adjunto, _ := leerMensaje(t, []byte(sesion[2]))
	if cabeceras.Get("Message-ID") != messageID || adjunto != "acuse_FIM49-INT-0137.pdf" {
		t.Errorf("mensaje entregado: %v %q", cabeceras, adjunto)
	}

	// Sin cifrar, sólo hacia la propia máquina; y un modo desconocido se rechaza.
	remoto := MailerSMTP{LectorDeArchivos: lector, Servidor: "smtp.ejemplo.mx:25", Seguridad: SMTPSinTLS, Remitente: "x <a@b.mx>"}
	if _, err := remoto.Enviar(context.Background(), correo); !errors.Is(err, errSMTPSinCifrar) {
		t.Errorf("SMTP sin cifrar hacia afuera: %v", err)
	}
	if _, err := (MailerSMTP{LectorDeArchivos: lector, Servidor: direccion, Seguridad: "quizas"}).Enviar(context.Background(), correo); err == nil {
		t.Error("modo de seguridad desconocido aceptado")
	}
}
