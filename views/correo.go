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
	"html/template"
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

// variableCorreo reconoce {{nombre}}: es la misma expresión con la que el
// modelo valida las plantillas.
var variableCorreo = models.VariableDeCorreo

// RenderCorreo arma el mensaje para un destinatario: asunto, versión de texto
// plano (la propia plantilla, que en Markdown es legible) y versión HTML.
// Una variable que la plantilla use y no se le pase queda visible tal cual,
// para que el error se note en la vista previa y no se envíe un hueco.
func RenderCorreo(p models.PlantillaCorreo, destinatario string, variables map[string]string) models.CorreoSaliente {
	asunto, cuerpo, texto := componerCorreo(p, variables)
	return models.CorreoSaliente{
		Destinatario:   destinatario,
		Idioma:         p.Idioma,
		Asunto:         asunto,
		Texto:          texto,
		PlantillaClave: p.Clave,
		HTML: `<!DOCTYPE html><html lang="` + html.EscapeString(p.Idioma) + `"><head><meta charset="utf-8"><title>` +
			html.EscapeString(asunto) + "</title></head><body>\n" + cuerpo + "</body></html>\n",
	}
}

// VistaPreviaDeCorreo arma un mensaje sin destinatario para mostrarlo en la
// pantalla de plantillas: el asunto, el cuerpo en HTML —el mismo que llevará
// el correo, ya escapado— y la versión de texto plano.
func VistaPreviaDeCorreo(p models.PlantillaCorreo, variables map[string]string) (asunto string, cuerpo template.HTML, texto string) {
	asunto, cuerpoHTML, texto := componerCorreo(p, variables)
	return asunto, template.HTML(cuerpoHTML), texto
}

// componerCorreo sustituye las variables en el asunto, el cuerpo HTML y el
// texto plano de una plantilla.
func componerCorreo(p models.PlantillaCorreo, variables map[string]string) (asunto, cuerpoHTML, texto string) {
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
		// Un valor de varios renglones (la lista de problemas de una
		// subsanación) conserva sus saltos de línea en la versión HTML.
		return strings.ReplaceAll(escapado, "\n", "<br>\n")
	}
	return sustituir(p.Asunto, unaLinea), sustituir(string(Markdown(p.CuerpoMD)), enHTML), sustituir(p.CuerpoMD, plano) + "\n"
}
