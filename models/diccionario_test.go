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

package models

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
)

// El diccionario de datos (docs/DICCIONARIO_DATOS_Y_ACL.md) describe el
// esquema VIGENTE: cada tabla tiene su sección y cada columna su renglón, y
// no describe tablas que ya no existen. Quien añade una migración actualiza
// el documento en el mismo cambio; si no, esta prueba lo dice.
func TestDiccionarioCubreElEsquema(t *testing.T) {
	documento, err := os.ReadFile("../docs/DICCIONARIO_DATOS_Y_ACL.md")
	if err != nil {
		t.Fatal(err)
	}
	texto := string(documento)
	r := repoDePrueba(t)
	vigentes := map[string]bool{}
	err = r.ExportarParaMigrar(context.Background(), func(tablas []TablaParaMigrar, _ func(TablaParaMigrar, func([]any) error) error) error {
		for _, tabla := range tablas {
			vigentes[tabla.Nombre] = true
			inicio := strings.Index(texto, "#### `"+tabla.Nombre+"`\n")
			if inicio < 0 {
				t.Errorf("falta la tabla %s en el diccionario", tabla.Nombre)
				continue
			}
			seccion := texto[inicio+1:]
			if fin := strings.Index(seccion, "\n#"); fin >= 0 {
				seccion = seccion[:fin]
			}
			for _, columna := range tabla.Columnas {
				if !strings.Contains(seccion, "| `"+columna.Nombre+"`") {
					t.Errorf("falta la columna %s.%s en el diccionario", tabla.Nombre, columna.Nombre)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("(?m)^#### `(\\w+)`$").FindAllStringSubmatch(texto, -1) {
		if !vigentes[m[1]] {
			t.Errorf("el diccionario describe la tabla %s, que no existe", m[1])
		}
	}
}
