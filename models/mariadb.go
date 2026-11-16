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
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Migración a MariaDB (§3.C, ADR 0029). Traduce el esquema VIVO de la base
// —el de las migraciones más las columnas que añadió «fimnme-cli schema»— y
// sus datos a un solo archivo .sql para MariaDB, y escribe un manifiesto con
// el número de filas y una suma SHA-256 del contenido de cada tabla, para que
// deploy/verify_and_migrate_mariadb.sh compruebe que lo cargado es idéntico.
//
// Aquí no se consulta la base: ExportarParaMigrar (repository.go) la lee y
// entrega las tablas y sus filas.

// largoIndexable es la longitud de una columna de texto que forma parte de una
// llave o de un índice: MariaDB no indexa LONGTEXT. 255 caracteres en utf8mb4
// son 1020 bytes; tres en un mismo índice caben en el máximo de InnoDB (3072).
const largoIndexable = 255

// sinComentarios quita los comentarios «-- …» de un texto SQL, sin tocar lo
// que esté entre comillas.
func sinComentarios(sql string) string {
	var salida strings.Builder
	enComillas := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case c == '\'':
			enComillas = !enComillas
		case !enComillas && c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			if i < len(sql) {
				salida.WriteByte('\n')
			}
			continue
		}
		salida.WriteByte(c)
	}
	return salida.String()
}

// parentesisDesde devuelve lo que hay entre el paréntesis que abre en la
// posición i y el que le corresponde, respetando comillas; y dónde termina.
func parentesisDesde(sql string, i int) (string, int) {
	profundidad, enComillas := 0, false
	for j := i; j < len(sql); j++ {
		switch c := sql[j]; {
		case c == '\'':
			enComillas = !enComillas
		case enComillas:
		case c == '(':
			profundidad++
		case c == ')':
			profundidad--
			if profundidad == 0 {
				return sql[i+1 : j], j
			}
		}
	}
	return "", len(sql)
}

// ChecksDeDefinicion extrae las expresiones de las restricciones CHECK de un
// CREATE TABLE, de columna o de tabla. SQLite no las expone por PRAGMA.
func ChecksDeDefinicion(definicion string) []string {
	sql := sinComentarios(definicion)
	mayusculas := strings.ToUpper(sql)
	var checks []string
	enComillas := false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' {
			enComillas = !enComillas
			continue
		}
		if enComillas || !strings.HasPrefix(mayusculas[i:], "CHECK") || (i > 0 && esDeIdentificador(sql[i-1])) ||
			(i+5 < len(sql) && esDeIdentificador(sql[i+5])) {
			continue
		}
		j := i + 5
		for j < len(sql) && unicode.IsSpace(rune(sql[j])) {
			j++
		}
		if j < len(sql) && sql[j] == '(' {
			expresion, fin := parentesisDesde(sql, j)
			checks = append(checks, strings.Join(strings.Fields(expresion), " "))
			i = fin
		}
	}
	return checks
}

// esDeIdentificador indica si un byte puede ser parte de un nombre SQL.
func esDeIdentificador(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ColumnasSinMayusculas devuelve las columnas declaradas COLLATE NOCASE en un
// CREATE TABLE (también las añadidas después con ALTER TABLE: SQLite las
// incorpora al texto).
func ColumnasSinMayusculas(definicion string) map[string]bool {
	sql := sinComentarios(definicion)
	inicio := strings.Index(sql, "(")
	columnas := map[string]bool{}
	if inicio < 0 {
		return columnas
	}
	cuerpo, _ := parentesisDesde(sql, inicio)
	// Partes separadas por comas de primer nivel: una por columna o restricción.
	profundidad, enComillas, desde := 0, false, 0
	partes := []string{}
	for i := 0; i < len(cuerpo); i++ {
		switch c := cuerpo[i]; {
		case c == '\'':
			enComillas = !enComillas
		case enComillas:
		case c == '(':
			profundidad++
		case c == ')':
			profundidad--
		case c == ',' && profundidad == 0:
			partes = append(partes, cuerpo[desde:i])
			desde = i + 1
		}
	}
	partes = append(partes, cuerpo[desde:])
	for _, parte := range partes {
		campos := strings.Fields(parte)
		if len(campos) > 0 && strings.Contains(strings.ToUpper(parte), "COLLATE NOCASE") {
			columnas[strings.Trim(campos[0], "\"`[]")] = true
		}
	}
	return columnas
}

// tipoMariaDB traduce el tipo declarado en SQLite según sus reglas de
// afinidad. indexada dice si la columna es parte de una llave o un índice.
func tipoMariaDB(c ColumnaParaMigrar, indexada bool) string {
	tipo := strings.ToUpper(c.Tipo)
	cotejo := "utf8mb4_bin" // la comparación exacta de SQLite (BINARY)
	if c.SinMayusculas {
		cotejo = "utf8mb4_unicode_ci"
	}
	switch {
	case strings.Contains(tipo, "INT"):
		return "BIGINT"
	case strings.Contains(tipo, "CHAR"), strings.Contains(tipo, "CLOB"), strings.Contains(tipo, "TEXT"):
		if indexada {
			return fmt.Sprintf("VARCHAR(%d) COLLATE %s", largoIndexable, cotejo)
		}
		return "LONGTEXT COLLATE " + cotejo
	case tipo == "", strings.Contains(tipo, "BLOB"):
		return "LONGBLOB"
	case strings.Contains(tipo, "REAL"), strings.Contains(tipo, "FLOA"), strings.Contains(tipo, "DOUB"):
		return "DOUBLE"
	default:
		return "DECIMAL(30,10)"
	}
}

// defectoLiteral devuelve el valor por omisión si es un literal simple (texto
// entre comillas o número); una expresión no se traslada.
func defectoLiteral(defecto string) (string, bool) {
	defecto = strings.TrimSpace(defecto)
	if len(defecto) >= 2 && defecto[0] == '\'' && defecto[len(defecto)-1] == '\'' {
		return defecto, true
	}
	if _, err := strconv.ParseFloat(defecto, 64); err == nil {
		return defecto, true
	}
	return "", false
}

// nombre entrecomilla un identificador para MariaDB.
func nombre(n string) string {
	return "`" + strings.ReplaceAll(n, "`", "``") + "`"
}

// nombres entrecomilla una lista de identificadores.
func nombres(lista []string) string {
	salida := make([]string, len(lista))
	for i, n := range lista {
		salida[i] = nombre(n)
	}
	return strings.Join(salida, ", ")
}

// literalMariaDB escribe un valor como literal SQL de MariaDB, con el modo de
// escapes por omisión (sin NO_BACKSLASH_ESCAPES).
func literalMariaDB(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case []byte:
		return "X'" + hex.EncodeToString(x) + "'"
	default:
		texto := fmt.Sprint(x)
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\x00", `\0`, "\n", `\n`, "\r", `\r`, "\x1a", `\Z`).Replace(texto) + "'"
	}
}

// campoDeLote escribe un valor como lo imprime «mariadb --batch»: NULL, y en
// el texto, la barra, el tabulador, el salto de línea y el carácter nulo
// escapados. Así una fila es un renglón comparable a ambos lados.
func campoDeLote(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []byte:
		return campoDeLote(string(x))
	default:
		return strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\x00", `\0`).Replace(fmt.Sprint(x))
	}
}

// ordenPorDependencias ordena las tablas para que cada una vaya después de
// las que referencia, y MariaDB pueda comprobar las llaves foráneas de cada
// fila al cargarla. Un ciclo entre tablas es un error: no hay orden posible.
func ordenPorDependencias(tablas []TablaParaMigrar) ([]TablaParaMigrar, error) {
	porNombre := map[string]TablaParaMigrar{}
	for _, t := range tablas {
		porNombre[t.Nombre] = t
	}
	var orden []TablaParaMigrar
	estado := map[string]int{} // 1 visitando, 2 lista
	var visitar func(string) error
	visitar = func(n string) error {
		switch estado[n] {
		case 1:
			return fmt.Errorf("ciclo de llaves foráneas en %s", n)
		case 2:
			return nil
		}
		estado[n] = 1
		for _, f := range porNombre[n].Foraneas {
			if _, existe := porNombre[f.Tabla]; existe && f.Tabla != n {
				if err := visitar(f.Tabla); err != nil {
					return err
				}
			}
		}
		estado[n] = 2
		orden = append(orden, porNombre[n])
		return nil
	}
	for _, t := range tablas {
		if err := visitar(t.Nombre); err != nil {
			return nil, err
		}
	}
	return orden, nil
}

// llavePrimaria devuelve las columnas de la llave primaria, en su orden.
func llavePrimaria(t TablaParaMigrar) []string {
	var llave []ColumnaParaMigrar
	for _, c := range t.Columnas {
		if c.PosicionLlave > 0 {
			llave = append(llave, c)
		}
	}
	slices.SortFunc(llave, func(a, b ColumnaParaMigrar) int { return a.PosicionLlave - b.PosicionLlave })
	columnas := make([]string, len(llave))
	for i, c := range llave {
		columnas[i] = c.Nombre
	}
	return columnas
}

// ddlMariaDB escribe el CREATE TABLE de una tabla para MariaDB.
func ddlMariaDB(t TablaParaMigrar, porNombre map[string]TablaParaMigrar) (string, map[string]bool, error) {
	llave := llavePrimaria(t)
	indexadas := map[string]bool{}
	for _, c := range llave {
		indexadas[c] = true
	}
	for _, ix := range t.Indices {
		for _, c := range ix.Columnas {
			indexadas[c] = true
		}
	}
	for _, f := range t.Foraneas {
		for _, c := range f.Columnas {
			indexadas[c] = true
		}
	}
	// Una columna referida por otra tabla también se indexa (con su tipo).
	for _, otra := range porNombre {
		for _, f := range otra.Foraneas {
			if f.Tabla == t.Nombre {
				for _, c := range f.Referidas {
					if c != "" {
						indexadas[c] = true
					}
				}
			}
		}
	}
	var lineas []string
	for _, c := range t.Columnas {
		linea := "  " + nombre(c.Nombre) + " " + tipoMariaDB(c, indexadas[c.Nombre])
		if c.NoNula || (len(llave) > 0 && slices.Contains(llave, c.Nombre)) {
			linea += " NOT NULL"
		}
		if len(llave) == 1 && llave[0] == c.Nombre && strings.Contains(strings.ToUpper(c.Tipo), "INT") {
			linea += " AUTO_INCREMENT" // INTEGER PRIMARY KEY: el rowid de SQLite
		} else if defecto, ok := defectoLiteral(c.Defecto); ok {
			linea += " DEFAULT " + defecto
		}
		lineas = append(lineas, linea)
	}
	if len(llave) > 0 {
		lineas = append(lineas, "  PRIMARY KEY ("+nombres(llave)+")")
	}
	for i, ix := range t.Indices {
		clase, etiqueta := "KEY", fmt.Sprintf("ix_%d", i+1)
		if ix.Unico {
			clase, etiqueta = "UNIQUE KEY", fmt.Sprintf("uq_%d", i+1)
		}
		if !strings.HasPrefix(ix.Nombre, "sqlite_autoindex") && len(ix.Nombre) <= 64 {
			etiqueta = ix.Nombre
		}
		lineas = append(lineas, fmt.Sprintf("  %s %s (%s)", clase, nombre(etiqueta), nombres(ix.Columnas)))
	}
	for i, f := range t.Foraneas {
		referidas := slices.Clone(f.Referidas)
		if slices.Contains(referidas, "") {
			referidas = llavePrimaria(porNombre[f.Tabla])
		}
		linea := fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)", nombre(fmt.Sprintf("fk_%s_%d", t.Nombre, i+1)),
			nombres(f.Columnas), nombre(f.Tabla), nombres(referidas))
		switch accion := strings.ToUpper(f.AlBorrar); accion {
		case "", "NO ACTION":
		case "CASCADE", "SET NULL", "RESTRICT":
			linea += " ON DELETE " + accion
		default:
			return "", nil, fmt.Errorf("%s: ON DELETE %s no existe en InnoDB", t.Nombre, accion)
		}
		lineas = append(lineas, linea)
	}
	for i, expresion := range t.Checks {
		lineas = append(lineas, fmt.Sprintf("  CONSTRAINT %s CHECK (%s)", nombre(fmt.Sprintf("ck_%s_%d", t.Nombre, i+1)), expresion))
	}
	return "CREATE TABLE " + nombre(t.Nombre) + " (\n" + strings.Join(lineas, ",\n") +
		"\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;\n", indexadas, nil
}

// triggersMariaDB son las protecciones de SQLite que no son restricciones:
// el puntaje dentro de la escala de su criterio y la bitácora de sólo
// inserción (material probatorio, §3.B). Se traducen a mano: RAISE(ABORT)
// de SQLite es SIGNAL en MariaDB. Los del índice de búsqueda no se
// trasladan: alimentan FTS5, que en MariaDB es un índice FULLTEXT.
const triggersMariaDB = `
DELIMITER //
CREATE TRIGGER evaluacion_puntaje_rango_ins BEFORE INSERT ON evaluacion_puntaje FOR EACH ROW
BEGIN
  IF NOT EXISTS (SELECT 1 FROM criterio_evaluacion c WHERE c.id = NEW.criterio_id AND NEW.puntaje BETWEEN c.escala_min AND c.escala_max) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'puntaje fuera de la escala del criterio';
  END IF;
END//
CREATE TRIGGER evaluacion_puntaje_rango_upd BEFORE UPDATE ON evaluacion_puntaje FOR EACH ROW
BEGIN
  IF NOT EXISTS (SELECT 1 FROM criterio_evaluacion c WHERE c.id = NEW.criterio_id AND NEW.puntaje BETWEEN c.escala_min AND c.escala_max) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'puntaje fuera de la escala del criterio';
  END IF;
END//
CREATE TRIGGER auditoria_no_update BEFORE UPDATE ON auditoria FOR EACH ROW
BEGIN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'auditoria es de solo insercion';
END//
CREATE TRIGGER auditoria_no_delete BEFORE DELETE ON auditoria FOR EACH ROW
BEGIN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'auditoria es de solo insercion';
END//
DELIMITER ;
`

// filasPorInsert es cuántas filas lleva cada INSERT: menos sentencias que
// una por fila, sin rebasar el tamaño máximo de un paquete.
const filasPorInsert = 200

// EscribirMigracionMariaDB escribe el .sql completo para MariaDB y el
// manifiesto de verificación. filas entrega las de cada tabla (lo da
// ExportarParaMigrar). Una columna de texto indexada con un valor de más de
// 255 caracteres es un error: MariaDB lo truncaría o lo rechazaría.
func EscribirMigracionMariaDB(destino, manifiesto io.Writer, tablas []TablaParaMigrar,
	filas func(t TablaParaMigrar, cada func([]any) error) error) error {
	ordenadas, err := ordenPorDependencias(tablas)
	if err != nil {
		return err
	}
	porNombre := map[string]TablaParaMigrar{}
	for _, t := range tablas {
		porNombre[t.Nombre] = t
	}
	salida := bufio.NewWriterSize(destino, 1<<20)
	fmt.Fprint(salida, "-- Migración de FIMNME de SQLite a MariaDB, generada por «fimnme-cli exportar-mariadb».\n"+
		"-- Se carga en una base VACÍA con deploy/verify_and_migrate_mariadb.sh, que además la verifica.\n"+
		"SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci;\n"+
		"SET SESSION sql_mode = 'STRICT_ALL_TABLES,NO_ENGINE_SUBSTITUTION,NO_AUTO_VALUE_ON_ZERO';\n"+
		"SET SESSION foreign_key_checks = 1;\n\n")
	for _, t := range ordenadas {
		ddl, _, err := ddlMariaDB(t, porNombre)
		if err != nil {
			return err
		}
		fmt.Fprintln(salida, ddl)
	}
	for _, t := range ordenadas {
		_, indexadas, _ := ddlMariaDB(t, porNombre)
		columnas := make([]string, len(t.Columnas))
		for i, c := range t.Columnas {
			columnas[i] = c.Nombre
		}
		cabecera := "INSERT INTO " + nombre(t.Nombre) + " (" + nombres(columnas) + ") VALUES\n"
		var lote []string
		var renglones []string
		vaciar := func() {
			if len(lote) > 0 {
				fmt.Fprint(salida, cabecera+strings.Join(lote, ",\n")+";\n")
				lote = lote[:0]
			}
		}
		err := filas(t, func(valores []any) error {
			literales := make([]string, len(valores))
			campos := make([]string, len(valores))
			for i, v := range valores {
				if texto, esTexto := v.(string); esTexto && indexadas[t.Columnas[i].Nombre] && len([]rune(texto)) > largoIndexable {
					return fmt.Errorf("%s.%s tiene un valor de %d caracteres: rebasa los %d de una columna indexada en MariaDB",
						t.Nombre, t.Columnas[i].Nombre, len([]rune(texto)), largoIndexable)
				}
				literales[i], campos[i] = literalMariaDB(v), campoDeLote(v)
			}
			lote = append(lote, "("+strings.Join(literales, ", ")+")")
			renglones = append(renglones, strings.Join(campos, "\t"))
			if len(lote) == filasPorInsert {
				vaciar()
			}
			return nil
		})
		if err != nil {
			return err
		}
		vaciar()
		slices.Sort(renglones) // byte a byte, como «LC_ALL=C sort»
		suma := sha256.New()
		for _, r := range renglones {
			io.WriteString(suma, r+"\n")
		}
		fmt.Fprintf(manifiesto, "%s\t%d\t%s\t%s\n", t.Nombre, len(renglones), hex.EncodeToString(suma.Sum(nil)), strings.Join(columnas, ","))
	}
	if _, existe := porNombre["indice_busqueda"]; existe {
		fmt.Fprint(salida, "\n-- El buscador: en SQLite es FTS5; en MariaDB, un índice FULLTEXT.\nALTER TABLE `indice_busqueda` ADD FULLTEXT KEY `ft_texto` (`texto`);\n")
	}
	_, conPuntajes := porNombre["evaluacion_puntaje"]
	_, conBitacora := porNombre["auditoria"]
	if conPuntajes && conBitacora {
		fmt.Fprint(salida, triggersMariaDB)
	}
	return salida.Flush()
}
