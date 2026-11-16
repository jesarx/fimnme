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
	"slices"
	"testing"
)

// Pruebas de la traducción a MariaDB (ADR 0029). La carga y la verificación
// contra un MariaDB real las hace deploy/verify_and_migrate_mariadb.sh.

func TestChecksYCotejoDeLaDefinicion(t *testing.T) {
	definicion := `CREATE TABLE x (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE COLLATE NOCASE, -- comentario con CHECK (falso)
    estado TEXT NOT NULL CHECK (estado IN ('a','b (c)')),
    nota  TEXT DEFAULT 'texto con check(dentro) y ''comilla''',
    CHECK (estado = 'a' OR nota IS NOT NULL)
)`
	checks := ChecksDeDefinicion(definicion)
	if !slices.Equal(checks, []string{"estado IN ('a','b (c)')", "estado = 'a' OR nota IS NOT NULL"}) {
		t.Errorf("checks: %q", checks)
	}
	if sinMayusculas := ColumnasSinMayusculas(definicion); !sinMayusculas["email"] || sinMayusculas["estado"] || len(sinMayusculas) != 1 {
		t.Errorf("columnas NOCASE: %v", sinMayusculas)
	}
}

func TestValoresParaMariaDB(t *testing.T) {
	raro := "a\\b\tc\nd'e\x00f"
	if got := literalMariaDB(raro); got != `'a\\b`+"\t"+`c\nd\'e\0f'` {
		t.Errorf("literal: %q", got)
	}
	if got := campoDeLote(raro); got != `a\\b\tc`+`\nd'e\0f` {
		t.Errorf("campo de lote: %q", got)
	}
	if literalMariaDB(nil) != "NULL" || campoDeLote(nil) != "NULL" || literalMariaDB(int64(-7)) != "-7" || literalMariaDB([]byte{1, 255}) != "X'01ff'" {
		t.Error("NULL, enteros o binarios mal escritos")
	}
	columna := ColumnaParaMigrar{Nombre: "email", Tipo: "TEXT", SinMayusculas: true}
	if tipoMariaDB(columna, true) != "VARCHAR(255) COLLATE utf8mb4_unicode_ci" || tipoMariaDB(ColumnaParaMigrar{Tipo: "TEXT"}, false) != "LONGTEXT COLLATE utf8mb4_bin" ||
		tipoMariaDB(ColumnaParaMigrar{Tipo: "INTEGER"}, false) != "BIGINT" {
		t.Error("tipos mal traducidos")
	}
	if _, err := ordenPorDependencias([]TablaParaMigrar{
		{Nombre: "a", Foraneas: []LlaveForanea{{Tabla: "b"}}}, {Nombre: "b", Foraneas: []LlaveForanea{{Tabla: "a"}}}}); err == nil {
		t.Error("un ciclo de llaves foráneas debía rechazarse")
	}
}
