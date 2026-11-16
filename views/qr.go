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
	"fmt"
	"html/template"
	"strings"

	"github.com/jesarx/fimnme/internal/qr"
)

// QRSVG dibuja un código QR como SVG en línea, para el alta de la
// verificación en dos pasos.
//
// Siempre negro sobre blanco, con su margen de cuatro módulos, también en el
// tema oscuro: muchas cámaras no leen un QR invertido. Los colores van en
// atributos de presentación (fill), no en style="", que la CSP prohíbe.
// Devuelve "" si el texto no cabe en un QR: la página muestra entonces sólo
// la clave en texto.
func QRSVG(texto string) template.HTML {
	simbolo, err := qr.Codificar([]byte(texto))
	if err != nil {
		return ""
	}
	const margen = 4
	lado := simbolo.Tamano + 2*margen
	var trazo strings.Builder
	for y := range simbolo.Tamano {
		for x := range simbolo.Tamano {
			if simbolo.Oscuro(x, y) {
				fmt.Fprintf(&trazo, "M%d %dh1v1h-1z", x+margen, y+margen)
			}
		}
	}
	return template.HTML(fmt.Sprintf(
		`<svg class="qr" viewBox="0 0 %d %d" role="img" aria-hidden="true" focusable="false" shape-rendering="crispEdges">`+
			`<rect width="%d" height="%d" fill="#fff"/><path d="%s" fill="#000"/></svg>`,
		lado, lado, lado, lado, trazo.String()))
}
