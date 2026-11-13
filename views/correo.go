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
	"html"
	"regexp"
	"strings"

	"github.com/jesarx/fimnme/models"
)

// Correo saliente: convierte una plantilla editable en un mensaje concreto.
//
// Las plantillas las escribe un administrador en Markdown, con variables
// {{nombre}}, {{liga}}, etc. Las variables NO se interpretan como Markdown ni
// como HTML: primero se convierte la plantilla y después se insertan los
// valores, escapados. Así un nombre con asteriscos o con «<script>» llega al
// correo como texto literal.

var variableCorreo = regexp.MustCompile(`\{\{([a-z_]+)\}\}`)

// RenderCorreo arma el mensaje para un destinatario: asunto, versión de texto
// plano (la propia plantilla, que en Markdown es legible) y versión HTML.
// Una variable que la plantilla use y no se le pase queda visible tal cual,
// para que el error se note en la vista previa y no se envíe un hueco.
func RenderCorreo(p models.PlantillaCorreo, destinatario string, variables map[string]string) models.CorreoSaliente {
	sustituir := func(texto string, formato func(nombre, valor string) string) string {
		return variableCorreo.ReplaceAllStringFunc(texto, func(marca string) string {
			nombre := marca[2 : len(marca)-2]
			valor, ok := variables[nombre]
			if !ok {
				return marca
			}
			return formato(nombre, valor)
		})
	}
	plano := func(_, valor string) string { return valor }
	// En el asunto un salto de línea permitiría inyectar cabeceras.
	unaLinea := func(_, valor string) string {
		return strings.Join(strings.Fields(valor), " ")
	}
	enHTML := func(nombre, valor string) string {
		escapado := html.EscapeString(valor)
		if strings.HasPrefix(nombre, "liga") {
			return `<a href="` + escapado + `">` + escapado + `</a>`
		}
		return escapado
	}

	asunto := sustituir(p.Asunto, unaLinea)
	cuerpo := sustituir(string(Markdown(p.CuerpoMD)), enHTML)
	return models.CorreoSaliente{
		Destinatario:   destinatario,
		Idioma:         p.Idioma,
		Asunto:         asunto,
		Texto:          sustituir(p.CuerpoMD, plano) + "\n",
		PlantillaClave: p.Clave,
		HTML: `<!DOCTYPE html><html lang="` + html.EscapeString(p.Idioma) + `"><head><meta charset="utf-8"><title>` +
			html.EscapeString(asunto) + "</title></head><body>\n" + cuerpo + "</body></html>\n",
	}
}
