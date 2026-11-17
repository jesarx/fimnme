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
	"io"
	"log/slog"
	"os"
	"regexp"
	"testing"

	"github.com/jesarx/fimnme/views"
)

// La matriz de control de acceso (docs/DICCIONARIO_DATOS_Y_ACL.md, §II.6)
// tiene un renglón por cada operación de la API, y ninguno de más.
func TestDiccionarioCubreLaAPI(t *testing.T) {
	documento, err := os.ReadFile("../docs/DICCIONARIO_DATOS_Y_ACL.md")
	if err != nil {
		t.Fatal(err)
	}
	enDocumento := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `((?:GET|POST|PUT|PATCH|DELETE) /api/[^`]+)` \\|").FindAllStringSubmatch(string(documento), -1) {
		enDocumento[m[1]] = true
	}
	vistas, err := views.Nuevo()
	if err != nil {
		t.Fatal(err)
	}
	app := Nueva(Config{Vistas: vistas, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	enCodigo := map[string]bool{}
	for _, ruta := range app.rutasAPI() {
		clave := ruta.metodo + " " + ruta.patron
		enCodigo[clave] = true
		if !enDocumento[clave] {
			t.Errorf("la operación %s no está en la matriz de control de acceso", clave)
		}
	}
	for clave := range enDocumento {
		if !enCodigo[clave] {
			t.Errorf("la matriz describe %s, que no existe", clave)
		}
	}
}
