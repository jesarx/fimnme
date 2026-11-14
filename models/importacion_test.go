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
	"archive/zip"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// Pruebas del motor de importación asistida: lectura de hojas, forma de los
// renglones, análisis de conflictos y reglas de resolución; y del motor de
// extensión dinámica: la cerca de la lista blanca.

// xlsxDePrueba arma un .xlsx mínimo como los que escribe Excel: la hoja y,
// aparte, la tabla de textos compartidos. hoja es el XML de <sheetData>.
func xlsxDePrueba(t *testing.T, nombreDeHoja, hoja string, compartidos ...string) []byte {
	t.Helper()
	var salida bytes.Buffer
	escritor := zip.NewWriter(&salida)
	partes := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"xl/workbook.xml":     `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"/>`,
		nombreDeHoja: `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
			hoja + `</sheetData></worksheet>`,
	}
	if len(compartidos) > 0 {
		partes["xl/sharedStrings.xml"] = `<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			strings.Join(compartidos, "") + `</sst>`
	}
	for nombre, contenido := range partes {
		parte, err := escritor.Create(nombre)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parte.Write([]byte(contenido)); err != nil {
			t.Fatal(err)
		}
	}
	if err := escritor.Close(); err != nil {
		t.Fatal(err)
	}
	return salida.Bytes()
}

func TestLeerHoja(t *testing.T) {
	// CSV: con comas, con punto y coma, con marca inicial, con comillas.
	for nombre, caso := range map[string]struct {
		contenido string
		esperado  string
	}{
		"comas":                {"nombre_es,nombre_en\nCuarteto,Quartet\n", "[[nombre_es nombre_en] [Cuarteto Quartet]]"},
		"punto y coma":         {"nombre_es;nombre_en\r\nDúo, con piano;Duo\r\n", "[[nombre_es nombre_en] [Dúo, con piano Duo]]"},
		"marca de UTF-8":       {"\xEF\xBB\xBFnombre_es\nÓpera\n", "[[nombre_es] [Ópera]]"},
		"comillas":             {"nombre_es,descripcion_es\n\"Trío\",\"Violín, viola y \"\"chelo\"\"\"\n", `[[nombre_es descripcion_es] [Trío Violín, viola y "chelo"]]`},
		"renglones desiguales": {"a,b,c\n1\n", "[[a b c] [1]]"},
	} {
		hoja, err := LeerHoja([]byte(caso.contenido))
		if err != nil || fmt.Sprint(hoja) != caso.esperado {
			t.Errorf("CSV con %s: %v %v", nombre, hoja, err)
		}
	}
	if _, err := LeerHoja([]byte("nombre_es\nD\xFAo\n")); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("un CSV en Latin-1 debe rechazarse con una indicación: %v", err)
	}
	if _, err := LeerHoja(make([]byte, TamanoMaxImportacion+1)); err == nil {
		t.Error("una hoja desmesurada debe rechazarse")
	}

	// XLSX: textos compartidos (simples y con formato), texto en la celda,
	// números, celdas vacías que Excel omite y una fórmula con su valor.
	libro := xlsxDePrueba(t, "xl/worksheets/sheet1.xml",
		`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>idioma</t></is></c></row>`+
			`<row r="2"><c r="A2" t="s"><v>2</v></c><c r="C2" t="s"><v>3</v></c></row>`+
			`<row r="3"><c r="B3"><v>1996</v></c><c r="C3" t="str"><f>A1&amp;"x"</f><v>calculado</v></c></row>`,
		`<si><t>email</t></si>`, `<si><t xml:space="preserve">nombre </t></si>`,
		`<si><r><rPr><b/></rPr><t>ana@</t></r><r><t>ejemplo.mx</t></r></si>`, `<si><t>es</t></si>`)
	hoja, err := LeerHoja(libro)
	if err != nil || fmt.Sprintf("%q", hoja) != `[["email" "nombre " "idioma"] ["ana@ejemplo.mx" "" "es"] ["" "1996" "calculado"]]` {
		t.Errorf("lectura del .xlsx: %q %v", hoja, err)
	}
	// La hoja puede no llamarse sheet1; y si hay varias, se lee la primera.
	if hoja, err = LeerHoja(xlsxDePrueba(t, "xl/worksheets/hoja3.xml", `<row><c t="inlineStr"><is><t>nombre_es</t></is></c></row>`)); err != nil || fmt.Sprint(hoja) != "[[nombre_es]]" {
		t.Errorf("hoja con otro nombre: %v %v", hoja, err)
	}
	for nombre, malo := range map[string][]byte{
		"texto compartido que no existe": xlsxDePrueba(t, "xl/worksheets/sheet1.xml", `<row><c r="A1" t="s"><v>7</v></c></row>`, `<si><t>a</t></si>`),
		"sin hoja":                       xlsxDePrueba(t, "xl/otra/cosa.xml", ``),
		"referencia de celda inválida":   xlsxDePrueba(t, "xl/worksheets/sheet1.xml", `<row><c r="a1"><v>1</v></c></row>`),
		"demasiadas columnas":            xlsxDePrueba(t, "xl/worksheets/sheet1.xml", `<row><c r="ZZ1"><v>1</v></c></row>`),
		"XML roto":                       xlsxDePrueba(t, "xl/worksheets/sheet1.xml", `<row><c r="A1"><v>1</c></row>`),
		"ZIP que no lo es":               []byte("PK\x03\x04 esto no es un zip"),
	} {
		if _, err := LeerHoja(malo); err == nil {
			t.Errorf("un .xlsx con %s debe rechazarse", nombre)
		}
	}
}

func TestFilasDeImportacion(t *testing.T) {
	hoja := func(renglones ...string) HojaDeCalculo {
		var h HojaDeCalculo
		for _, renglon := range renglones {
			h = append(h, strings.Split(renglon, "|"))
		}
		return h
	}
	// Dotaciones: columnas en cualquier orden, renglones vacíos ignorados.
	filas, errores := FilasDeImportacion(ImportacionDotaciones, hoja(" Nombre_EN |nombre_es||", "Quartet| Cuarteto ", "||", "|Dúo|sobra"))
	if len(errores) != 0 || len(filas) != 2 || filas[0].Numero != 2 || filas[0].Valores["nombre_es"] != "Cuarteto" || filas[0].Valores["nombre_en"] != "Quartet" ||
		filas[1].Numero != 4 || filas[1].Valores["nombre_es"] != "Dúo" {
		t.Fatalf("renglones de dotaciones: %+v %v", filas, errores)
	}
	// Cuentas: el correo se recorta y el idioma tiene valor por omisión.
	filas, errores = FilasDeImportacion(ImportacionJurados, hoja("email|nombre|apellidos", " Nora.Paz@Ejemplo.MX |Nora|Paz"))
	if len(errores) != 0 || filas[0].Valores["email"] != "Nora.Paz@Ejemplo.MX" || filas[0].Valores["idioma"] != IdiomaES {
		t.Fatalf("renglones de cuentas: %+v %v", filas, errores)
	}
	for nombre, caso := range map[string]struct {
		tipo     string
		hoja     HojaDeCalculo
		esperado string
	}{
		"tipo desconocido":        {"postulaciones", hoja("folio", "FIM49-INT-0001"), "tipo:error.tipo_de_importacion"},
		"hoja vacía":              {ImportacionDotaciones, nil, "archivo:error.hoja_vacia"},
		"sólo encabezados":        {ImportacionDotaciones, hoja("nombre_es"), "archivo:error.hoja_vacia"},
		"falta una obligatoria":   {ImportacionJurados, hoja("email|nombre", "a@ejemplo.mx|Ana"), "fila 1:error.columnas_de_importacion"},
		"columna desconocida":     {ImportacionDotaciones, hoja("nombre_es|precio", "Dúo|3"), "fila 1:error.columnas_de_importacion"},
		"columna repetida":        {ImportacionDotaciones, hoja("nombre_es|nombre_es", "Dúo|Dúo"), "fila 1:error.columnas_de_importacion"},
		"sin nombre":              {ImportacionDotaciones, hoja("nombre_es|nombre_en", "|Duo"), "fila 2:error.fila_incompleta"},
		"nombre larguísimo":       {ImportacionDotaciones, hoja("nombre_es", strings.Repeat("a", 201)), "fila 2:error.texto_largo_campo"},
		"dotación repetida":       {ImportacionDotaciones, hoja("nombre_es", "Dúo", "Trío", " DUO "), "fila 4:error.fila_repetida"},
		"correo inválido":         {ImportacionJurados, hoja("email|nombre|apellidos", "no-es-correo|Ana|Paz"), "fila 2:error.fila_incompleta"},
		"sin apellidos":           {ImportacionRevisores, hoja("email|nombre|apellidos", "a@ejemplo.mx|Ana|"), "fila 2:error.fila_incompleta"},
		"idioma inventado":        {ImportacionJurados, hoja("email|nombre|apellidos|idioma", "a@ejemplo.mx|Ana|Paz|fr"), "fila 2:error.fila_incompleta"},
		"correo repetido":         {ImportacionJurados, hoja("email|nombre|apellidos", "a@ejemplo.mx|Ana|Paz", "A@Ejemplo.mx|Otra|Persona"), "fila 3:error.fila_repetida"},
		"varios errores a la vez": {ImportacionDotaciones, hoja("nombre_es|nombre_en", "|x", "Dúo|", "Dúo|"), "fila 2:error.fila_incompleta fila 4:error.fila_repetida"},
	} {
		filas, errores := FilasDeImportacion(caso.tipo, caso.hoja)
		var obtenido []string
		for _, e := range errores {
			obtenido = append(obtenido, e.Campo+":"+e.Clave)
		}
		if strings.Join(obtenido, " ") != caso.esperado || filas != nil {
			t.Errorf("%s: %v", nombre, obtenido)
		}
	}
	demasiadas := HojaDeCalculo{{"nombre_es"}}
	for i := 0; i <= maxFilasImportacion; i++ {
		demasiadas = append(demasiadas, []string{"Dotación " + strconv.Itoa(i)})
	}
	if _, errores := FilasDeImportacion(ImportacionDotaciones, demasiadas); len(errores) != 1 || errores[0].Clave != "error.demasiadas_filas" {
		t.Errorf("demasiados renglones: %v", errores)
	}
}

// Sólo un nombre idéntico es una correspondencia unívoca; y una cuenta nunca
// se da por la misma persona sin que alguien lo decida (RN-30).
func TestAnalisisDeImportacion(t *testing.T) {
	fila := func(numero int, pares ...string) FilaDeImportacion {
		f := FilaDeImportacion{Numero: numero, Valores: make(map[string]string)}
		for i := 0; i+1 < len(pares); i += 2 {
			f.Valores[pares[i]] = pares[i+1]
		}
		return f
	}
	resumen := func(conflictos []ConflictoDeImportacion) string {
		var lista []string
		for _, c := range conflictos {
			var candidatos []string
			for _, candidato := range c.Candidatos {
				candidatos = append(candidatos, fmt.Sprintf("%d:%s", candidato.ID, candidato.Motivo))
			}
			lista = append(lista, fmt.Sprintf("fila %d %v", c.Fila, candidatos))
		}
		return strings.Join(lista, " | ")
	}
	existentes := []Dotacion{{ID: 1, NombreES: "Cuarteto de cuerdas"}, {ID: 2, NombreES: "Obra electroacústica"}, {ID: 3, NombreES: "Dúo"}}
	conflictos := AnalizarDotaciones([]FilaDeImportacion{
		fila(2, "nombre_es", "Cuarteto de cuerdas"),  // idéntica: sin conflicto
		fila(3, "nombre_es", "cuarteto de CUERDAS"),  // la misma, escrita distinto: lo decide una persona
		fila(4, "nombre_es", "Cuarteto"),             // parecida
		fila(5, "nombre_es", "Obra electroacustica"), // sin acento
		fila(6, "nombre_es", "Quinteto de alientos"), // nueva
		fila(7, "nombre_es", "Duo"),                  // sin acento: mismo nombre, no idéntico
	}, existentes)
	if obtenido := resumen(conflictos); obtenido != "fila 3 [1:mismo_nombre] | fila 4 [1:nombre_parecido] | fila 5 [2:mismo_nombre] | fila 6 [] | fila 7 [3:mismo_nombre]" {
		t.Errorf("conflictos de dotaciones: %s", obtenido)
	}

	cuentas := []Usuario{{ID: 10, Email: "ana@ejemplo.mx", Nombre: "Ana", Apellidos: "Paz"}, {ID: 11, Email: "otra@ejemplo.mx", Nombre: "ANA", Apellidos: "páz"},
		{ID: 12, Email: "luis@ejemplo.mx", Nombre: "Luis", Apellidos: "Núñez"}}
	conflictos = AnalizarCuentas([]FilaDeImportacion{
		fila(2, "email", "ana@ejemplo.mx", "nombre", "Ana", "apellidos", "Paz"),      // mismo correo, y otra cuenta con el mismo nombre
		fila(3, "email", "nuevo@ejemplo.mx", "nombre", "Luis", "apellidos", "Nuñez"), // otro correo, mismo nombre
		fila(4, "email", "nora@ejemplo.mx", "nombre", "Nora", "apellidos", "Paz"),    // nadie
		fila(5, "email", "LUIS@Ejemplo.mx", "nombre", "Otro", "apellidos", "Nombre"), // mismo correo con otras mayúsculas, otro nombre
	}, cuentas)
	// TODO renglón es un conflicto, también el que coincide en el correo.
	if obtenido := resumen(conflictos); obtenido != "fila 2 [10:mismo_correo 11:mismo_nombre] | fila 3 [12:mismo_nombre] | fila 4 [] | fila 5 [12:mismo_correo]" {
		t.Errorf("conflictos de cuentas: %s", obtenido)
	}
	if conflictos[0].ValorOrigen != "Ana Paz <ana@ejemplo.mx>" || conflictos[0].Candidatos[0].Descripcion != "Ana Paz <ana@ejemplo.mx>" {
		t.Errorf("descripción del conflicto: %+v", conflictos[0])
	}

	// Reglas de resolución.
	conCorreo, conNombre, sinNadie := conflictos[0], conflictos[1], conflictos[2]
	for nombre, caso := range map[string]struct {
		tipo       string
		conflicto  ConflictoDeImportacion
		resolucion string
		entidadID  int64
		clave      string
	}{
		"omitir siempre se puede":                            {ImportacionJurados, conCorreo, ResolucionOmitir, 0, ""},
		"vincular al candidato":                              {ImportacionJurados, conCorreo, ResolucionVincular, 10, ""},
		"vincular a quien no es candidato":                   {ImportacionJurados, conCorreo, ResolucionVincular, 12, "error.candidato_requerido"},
		"vincular sin decir a quién":                         {ImportacionJurados, conNombre, ResolucionVincular, 0, "error.candidato_requerido"},
		"crear con el correo ya registrado":                  {ImportacionRevisores, conCorreo, ResolucionCrear, 0, "error.correo_ya_registrado"},
		"crear con el mismo nombre":                          {ImportacionJurados, conNombre, ResolucionCrear, 0, ""},
		"crear a quien no existe":                            {ImportacionJurados, sinNadie, ResolucionCrear, 0, ""},
		"vincular a nadie":                                   {ImportacionJurados, sinNadie, ResolucionVincular, 10, "error.candidato_requerido"},
		"resolución inventada":                               {ImportacionJurados, sinNadie, "fusionar", 0, "error.opcion_invalida"},
		"dotación: vincular a cualquiera de la convocatoria": {ImportacionDotaciones, ConflictoDeImportacion{}, ResolucionVincular, 3, ""},
		"dotación: vincular sin elegir":                      {ImportacionDotaciones, ConflictoDeImportacion{}, ResolucionVincular, 0, "error.candidato_requerido"},
	} {
		errores := ValidarResolucion(caso.tipo, caso.conflicto, caso.resolucion, caso.entidadID)
		if (caso.clave == "") != (len(errores) == 0) || (caso.clave != "" && errores[0].Clave != caso.clave) {
			t.Errorf("%s: %v", nombre, errores)
		}
	}

	// El Commit sólo se habilita con todo resuelto (RF-IMP-04).
	importacion := Importacion{Estado: ImportacionEsperando, FilasTotal: 5, Conflictos: []ConflictoDeImportacion{
		{Resolucion: ResolucionCrear}, {Resolucion: ResolucionVincular}, {Resolucion: ResolucionOmitir}, {}}}
	if efecto := importacion.Efecto(); fmt.Sprint(efecto) != "{1 1 1 1 1}" || importacion.PuedeConfirmarse() {
		t.Errorf("efecto con un conflicto pendiente: %+v", efecto)
	}
	importacion.Conflictos[3].Resolucion = ResolucionCrear
	if !importacion.PuedeConfirmarse() {
		t.Error("con todo resuelto debe poder confirmarse")
	}
	importacion.Estado = ImportacionConfirmada
	if importacion.PuedeConfirmarse() {
		t.Error("una importación confirmada no vuelve a confirmarse")
	}
	// Destino: dotaciones sólo en Compositores, y nada en una cerrada.
	for nombre, caso := range map[string]struct {
		tipo  string
		c     Convocatoria
		clave string
	}{
		"dotaciones en Compositores": {ImportacionDotaciones, Convocatoria{Tipo: TipoCompositores, Estado: EstadoPublicada}, ""},
		"dotaciones en Intérpretes":  {ImportacionDotaciones, Convocatoria{Tipo: TipoInterpretes, Estado: EstadoPublicada}, "error.dotacion_solo_compositores"},
		"jurados en Intérpretes":     {ImportacionJurados, Convocatoria{Tipo: TipoInterpretes, Estado: EstadoPostulacion}, ""},
		"convocatoria cerrada":       {ImportacionRevisores, Convocatoria{Tipo: TipoInterpretes, Estado: EstadoCerrada}, "error.convocatoria_cerrada"},
		"tipo que no existe":         {"postulaciones", Convocatoria{Tipo: TipoInterpretes, Estado: EstadoPublicada}, "error.tipo_de_importacion"},
	} {
		errores := ValidarNuevaImportacion(caso.tipo, caso.c)
		if (caso.clave == "") != (len(errores) == 0) || (caso.clave != "" && errores[0].Clave != caso.clave) {
			t.Errorf("%s: %v", nombre, errores)
		}
	}
}

// UNA POSTULACIÓN NUNCA SE IMPORTA (RF-IMP-07, RN-31). En todo el
// repositorio, sólo CrearPostulacion inserta en la tabla de postulaciones: el
// motor de importación no tiene cómo crear una.
func TestNadaImportaPostulaciones(t *testing.T) {
	archivos := token.NewFileSet()
	fuente, err := parser.ParseFile(archivos, "repository.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	encontradas := 0
	for _, declaracion := range fuente.Decls {
		funcion, ok := declaracion.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(funcion, func(nodo ast.Node) bool {
			literal, ok := nodo.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			texto, err := strconv.Unquote(literal.Value)
			if err != nil || !strings.Contains(strings.Join(strings.Fields(texto), " "), "INSERT INTO postulacion ") {
				return true
			}
			encontradas++
			if funcion.Name.Name != "CrearPostulacion" {
				t.Errorf("%s: %s inserta en la tabla de postulaciones; sólo CrearPostulacion puede", archivos.Position(literal.Pos()), funcion.Name.Name)
			}
			return true
		})
	}
	if encontradas != 1 {
		t.Errorf("se esperaba una sola inserción de postulaciones y hay %d: ¿cambió su forma?", encontradas)
	}
	if columnas, _ := ColumnasDeImportacion("postulaciones"); columnas != nil {
		t.Error("no debe existir un tipo de importación de postulaciones")
	}
}

// La cerca del motor de extensión: el DDL sólo se compone para tablas de la
// lista blanca y columnas «x_…».
func TestCercaDeLaExtensionDinamica(t *testing.T) {
	// Ninguna tabla está a la vez permitida y vedada, y las probatorias que
	// nombra la política están todas vedadas.
	for _, tabla := range NodosVedados {
		if NodoExtensible(tabla) {
			t.Errorf("%s está vedada y a la vez en la lista blanca", tabla)
		}
	}
	for _, probatoria := range []string{"evaluacion", "evaluacion_puntaje", "acuse", "descarga", "auditoria"} {
		if NodoExtensible(probatoria) {
			t.Errorf("la tabla probatoria %s admite DDL dinámico", probatoria)
		}
	}
	for nombre, caso := range map[string]struct {
		tabla, columna string
		tipo           TipoExtendido
		agregar        string // la sentencia, o vacío si no debe componerse
	}{
		"campo normal":            {"obra", "x_num_interpretes", ExtendidoEntero, "ALTER TABLE obra ADD COLUMN x_num_interpretes INTEGER"},
		"decimal":                 {"dotacion", "x_tarifa", ExtendidoDecimal, "ALTER TABLE dotacion ADD COLUMN x_tarifa REAL"},
		"tabla vedada":            {"evaluacion", "x_nota", ExtendidoTexto, ""},
		"tabla desconocida":       {"usuario", "x_nota", ExtendidoTexto, ""},
		"columna nativa":          {"postulacion", "estado", ExtendidoTexto, ""},
		"inyección en la columna": {"obra", "x_a TEXT; DROP TABLE acuse; --", ExtendidoTexto, ""},
		"inyección en la tabla":   {"obra; DROP TABLE acuse; --", "x_a", ExtendidoTexto, ""},
		"mayúsculas":              {"obra", "x_Nota", ExtendidoTexto, ""},
		"sólo el prefijo":         {"obra", "x_", ExtendidoTexto, ""},
		"tipo inventado":          {"obra", "x_nota", "BLOB", ""},
	} {
		sentencia, admisible := SentenciaParaAgregar(CampoExtendido{Tabla: caso.tabla, Columna: caso.columna, Tipo: caso.tipo})
		if sentencia != caso.agregar || admisible != (caso.agregar != "") {
			t.Errorf("%s: añadir compuso %q", nombre, sentencia)
		}
		eliminar, admisible := SentenciaParaEliminar(caso.tabla, caso.columna)
		if caso.agregar == "" && caso.tipo != "BLOB" && (admisible || eliminar != "") {
			t.Errorf("%s: eliminar compuso %q", nombre, eliminar)
		}
	}
	if sentencia, _ := SentenciaParaEliminar("obra", "x_num_interpretes"); sentencia != "ALTER TABLE obra DROP COLUMN x_num_interpretes" {
		t.Errorf("sentencia para eliminar: %q", sentencia)
	}
	if ColumnaExtendida(" nota ") != "x_nota" || ColumnaExtendida("x_nota") != "x_nota" {
		t.Error("el nombre físico lleva el prefijo una sola vez")
	}
	// Un campo extendido no se desbloquea en una subsanación (RF-CLI-26).
	if (PermisoDeEdicion{desbloqueados: map[string]bool{claveDeElemento(ProblemaCampo, "ext.obra.x_nota", 7): true, claveDeElemento(ProblemaCampo, CampoObras, 0): true}}).Campo("ext.obra.x_nota", 7) {
		t.Error("un campo extendido no debe quedar editable en una subsanación")
	}
	if !(PermisoDeEdicion{Total: true}).Campo("ext.obra.x_nota", 7) {
		t.Error("en un borrador, el campo extendido se captura")
	}
	// Quién ve un campo: la supervisión siempre; los demás, si el campo lo dice.
	campo := CampoExtendido{Tabla: "datos_solista", Columna: "x_nota", VisibleEn: []Perfil{PerfilPostulante, PerfilJurado}}
	p := Postulacion{UsuarioID: 100, ConvocatoriaID: 7}
	for nombre, caso := range map[string]struct {
		s  *Sesion
		ve bool
	}{
		"sin sesión":      {nil, false},
		"administrador":   {&Sesion{Perfiles: []Perfil{PerfilAdminSistema}}, true},
		"observador":      {&Sesion{Perfiles: []Perfil{PerfilCoordinacionNacional}}, true},
		"titular":         {&Sesion{Usuario: Usuario{ID: 100}}, true},
		"otro postulante": {&Sesion{Usuario: Usuario{ID: 101}}, false},
		"jurado":          {&Sesion{Asignaciones: []Asignacion{{ConvocatoriaID: 7, Perfil: PerfilJurado}}}, true},
		"jurado de otra":  {&Sesion{Asignaciones: []Asignacion{{ConvocatoriaID: 8, Perfil: PerfilJurado}}}, false},
		"revisor":         {&Sesion{Asignaciones: []Asignacion{{ConvocatoriaID: 7, Perfil: PerfilRevisorDocumental}}}, false},
	} {
		if VeExtendido(campo, caso.s, p) != caso.ve {
			t.Errorf("%s: ¿ve el campo? se esperaba %v", nombre, caso.ve)
		}
	}
}
