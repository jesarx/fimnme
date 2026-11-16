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
package views

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
)

// AdjuntoMIME es un archivo que viaja con un correo: su nombre, su tipo y su
// contenido. Quien compone el mensaje ya lo leyó del acervo.
type AdjuntoMIME struct {
	Nombre    string
	Tipo      string
	Contenido io.Reader
}

// MensajeMIME compone un correo completo (RFC 5322 con MIME): cabeceras,
// versión de texto plano y HTML como alternativas, y los adjuntos. Es lo
// mismo que se entrega por SMTP o se guarda como .eml, de modo que lo que se
// revisa en un archivo es exactamente lo que se habría enviado.
//
// dominio es el del remitente, para el Message-ID. Devuelve el mensaje y su
// Message-ID.
func MensajeMIME(c models.CorreoSaliente, remitente, dominio string, fecha time.Time, adjuntos []AdjuntoMIME) ([]byte, string, error) {
	aleatorio := make([]byte, 12)
	if _, err := rand.Read(aleatorio); err != nil {
		return nil, "", err
	}
	messageID := fmt.Sprintf("<%d.%s@%s>", c.ID, hex.EncodeToString(aleatorio), dominio)
	var salida bytes.Buffer
	cabecera := func(nombre, valor string) {
		// Un salto de línea en un valor permitiría inyectar cabeceras.
		valor = strings.NewReplacer("\r", " ", "\n", " ").Replace(valor)
		fmt.Fprintf(&salida, "%s: %s\r\n", nombre, valor)
	}
	cabecera("From", remitente)
	cabecera("To", c.Destinatario)
	cabecera("Subject", mime.QEncoding.Encode("utf-8", c.Asunto))
	cabecera("Date", fecha.Format(time.RFC1123Z))
	cabecera("Message-ID", messageID)
	cabecera("MIME-Version", "1.0")
	cabecera("Content-Language", c.Idioma)
	cabecera("Auto-Submitted", "auto-generated")

	mezcla := multipart.NewWriter(&salida)
	cabecera("Content-Type", `multipart/mixed; boundary="`+mezcla.Boundary()+`"`)
	salida.WriteString("\r\n")

	// Texto y HTML, como alternativas de un mismo contenido.
	var alternativas bytes.Buffer
	alterna := multipart.NewWriter(&alternativas)
	for _, parte := range []struct{ tipo, cuerpo string }{{"text/plain", c.Texto}, {"text/html", c.HTML}} {
		escritor, err := alterna.CreatePart(textproto.MIMEHeader{
			"Content-Type": {parte.tipo + "; charset=utf-8"}, "Content-Transfer-Encoding": {"base64"}})
		if err != nil {
			return nil, "", err
		}
		if err := escribirBase64(escritor, strings.NewReader(parte.cuerpo)); err != nil {
			return nil, "", err
		}
	}
	if err := alterna.Close(); err != nil {
		return nil, "", err
	}
	escritor, err := mezcla.CreatePart(textproto.MIMEHeader{"Content-Type": {`multipart/alternative; boundary="` + alterna.Boundary() + `"`}})
	if err != nil {
		return nil, "", err
	}
	if _, err := escritor.Write(alternativas.Bytes()); err != nil {
		return nil, "", err
	}
	for _, adjunto := range adjuntos {
		escritor, err := mezcla.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {adjunto.Tipo},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": adjunto.Nombre})},
			"Content-Transfer-Encoding": {"base64"}})
		if err != nil {
			return nil, "", err
		}
		if err := escribirBase64(escritor, adjunto.Contenido); err != nil {
			return nil, "", err
		}
	}
	if err := mezcla.Close(); err != nil {
		return nil, "", err
	}
	return salida.Bytes(), messageID, nil
}

// escribirBase64 escribe un contenido en base64 con renglones de 76
// caracteres, como pide MIME.
func escribirBase64(destino io.Writer, origen io.Reader) error {
	contenido, err := io.ReadAll(origen)
	if err != nil {
		return err
	}
	codificado := base64.StdEncoding.EncodeToString(contenido)
	for len(codificado) > 76 {
		if _, err := io.WriteString(destino, codificado[:76]+"\r\n"); err != nil {
			return err
		}
		codificado = codificado[76:]
	}
	_, err = io.WriteString(destino, codificado+"\r\n")
	return err
}
