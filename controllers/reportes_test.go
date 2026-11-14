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
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de extremo a extremo del paso 9b: reportes CSV, plantillas,
// expediente en PDF —sobre todo, lo que cada perfil recibe— y tableros.

// textoDePDF devuelve el texto de un PDF del paquete internal/pdf: los
// renglones de todas sus páginas, uno por línea, ya en UTF-8.
func textoDePDF(t *testing.T, archivo []byte) string {
	t.Helper()
	if !bytes.HasPrefix(archivo, []byte("%PDF-1.4")) {
		t.Fatalf("no es un PDF: %.60q", archivo)
	}
	var texto strings.Builder
	for _, flujo := range regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`).FindAllSubmatch(archivo, -1) {
		lector, err := zlib.NewReader(bytes.NewReader(flujo[1]))
		if err != nil {
			continue // una imagen, no una página
		}
		contenido, err := io.ReadAll(lector)
		if err != nil {
			t.Fatal(err)
		}
		for _, renglon := range regexp.MustCompile(`\(((?:\\.|[^\\)])*)\) Tj`).FindAllSubmatch(contenido, -1) {
			limpio := strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`).Replace(string(renglon[1]))
			// El PDF va en WinAnsi; para el español coincide con Latin-1.
			for _, b := range []byte(limpio) {
				texto.WriteRune(rune(b))
			}
			texto.WriteByte('\n')
		}
	}
	return texto.String()
}

// expedienteDe pide el expediente de una postulación y devuelve su texto.
func expedienteDe(t *testing.T, c *cliente, ruta, parametros string) string {
	t.Helper()
	respuesta := c.bajar(ruta + "/expediente?" + parametros)
	if respuesta.Code != http.StatusOK || respuesta.Header().Get("Content-Type") != "application/pdf" ||
		respuesta.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("GET %s/expediente: %d %v", ruta, respuesta.Code, respuesta.Header())
	}
	return textoDePDF(t, respuesta.Body.Bytes())
}

// contiene comprueba, de una vez, qué debe y qué no debe aparecer en un texto.
func contiene(t *testing.T, nombre, texto string, debe, noDebe []string) {
	t.Helper()
	for _, fragmento := range debe {
		if !strings.Contains(texto, fragmento) {
			t.Errorf("%s: falta «%s»", nombre, fragmento)
		}
	}
	for _, fragmento := range noDebe {
		if strings.Contains(texto, fragmento) {
			t.Errorf("%s: no debe aparecer «%s»", nombre, fragmento)
		}
	}
}

// evaluada lleva el entorno a la evaluación de Intérpretes con los tres
// jurados calificando a Luis, cada uno con una observación que lo delata, y
// con una observación de la revisora.
func evaluada(t *testing.T) *evaluacionDePrueba {
	t.Helper()
	x := hastaRevision(t)
	x.revisora.exige(http.StatusCreated, "POST", x.rutaLuis+"/observaciones", obj{"texto": "Nota interna de Rita."})
	x.dictaminar()
	x.admin.estado(x.interpretes, "evaluacion", "")
	for i, jurado := range x.juradosInt {
		evaluar(t, jurado, x.rutaLuis, obj{"puntajes": obj{"realizacion": 3 + i, "artistico": 5}, "enviar": true,
			"observaciones": fmt.Sprintf("Opinión del jurado %d.", i+1)})
	}
	return x
}

// Lo que cada perfil recibe en el expediente de una misma postulación
// (RF-ARCH-41, RN-26). Es la prueba de que el recorte se hace al generar.
func TestExpedientePorPerfil(t *testing.T) {
	x := evaluada(t)
	x.admin.exige(http.StatusOK, "PUT", x.rutaLuis+"/dictamen-final", obj{"dictamen": "seleccionada"})
	folio := x.admin.exige(http.StatusOK, "GET", x.rutaLuis, nil)["folio"].(string)

	comun := []string{"Expediente", folio, "Ana López Ruiz", "Preludio a Colón", "Julián Carrillo", "Jalisco", "Semblanza de Ana.",
		"CONFIDENCIAL", "Página 1 de", "La fotografía no pudo incluirse"}
	revision := []string{"Revisión documental", "Nota interna de Rita.", "Revisión: sin dictaminar"}
	todasLasOpiniones := []string{"Opinión del jurado 1.", "Opinión del jurado 2.", "Opinión del jurado 3."}
	agregados := []string{"Evaluación del jurado", "Suma", "Puntaje comparable", "Resultado del proceso", "Seleccionada"}

	// Supervisión: todo.
	for nombre, c := range map[string]*cliente{"administrador": x.admin, "observador": x.observador} {
		texto := expedienteDe(t, c, x.rutaLuis, "")
		contiene(t, nombre, texto, slices.Concat(comun, revision, todasLasOpiniones, agregados, []string{"27"}), []string{"Mi evaluación"})
	}
	// Revisora: la revisión, y nada de la evaluación.
	contiene(t, "revisora", expedienteDe(t, x.revisora, x.rutaLuis, ""), slices.Concat(comun, revision),
		slices.Concat(todasLasOpiniones, agregados, []string{"Mi evaluación"}))
	// Jurado: su evaluación y sólo la suya; ni la de los otros, ni la suma,
	// ni el resultado, ni la revisión.
	for i, jurado := range x.juradosInt {
		propia := fmt.Sprintf("Opinión del jurado %d.", i+1)
		ajenas := slices.DeleteFunc(slices.Clone(todasLasOpiniones), func(opinion string) bool { return opinion == propia })
		contiene(t, fmt.Sprintf("jurado %d", i+1), expedienteDe(t, jurado, x.rutaLuis, ""),
			slices.Concat(comun, []string{"Mi evaluación", propia, fmt.Sprintf("%d de 5", 3+i), "Enviada"}),
			slices.Concat(ajenas, agregados, revision))
	}
	// Titular: su postulación, sin nada interno.
	contiene(t, "titular", expedienteDe(t, x.luis, x.rutaLuis, ""), comun,
		slices.Concat(todasLasOpiniones, agregados, revision, []string{"Mi evaluación"}))
	// La titular de la postulación descartada no ve el descarte ni su motivo.
	contiene(t, "titular descartada", expedienteDe(t, x.ana, x.rutaAna, ""), []string{"En revisión"},
		[]string{"Descartada", "No cumple las bases.", "Revisión documental"})
	contiene(t, "revisora, descartada", expedienteDe(t, x.revisora, x.rutaAna, ""), []string{"Descartada", "No cumple las bases."}, nil)

	// Quien no puede ver la postulación no obtiene su expediente.
	for nombre, c := range map[string]*cliente{"otra postulante": x.ana, "jurado de otra convocatoria": x.juradosCom[0]} {
		if estado, _ := c.api("GET", x.rutaLuis+"/expediente", nil); estado != http.StatusNotFound {
			t.Errorf("%s: estado %d al pedir un expediente ajeno", nombre, estado)
		}
	}
	// El jurado no ve lo que no pasó a evaluación.
	x.juradosInt[0].exige(http.StatusNotFound, "GET", x.rutaAna+"/expediente", nil)
	x.e.cliente().exige(http.StatusUnauthorized, "GET", x.rutaLuis+"/expediente", nil)

	// Opciones: idioma, hoja de calificación y sin fotografía.
	contiene(t, "en inglés", expedienteDe(t, x.admin, x.rutaLuis, "idioma=en"),
		[]string{"Application file", "CONFIDENTIAL", "Jury evaluation", "Selected"}, []string{"Expediente", "CONFIDENCIAL"})
	contiene(t, "con hoja", expedienteDe(t, x.juradosInt[0], x.rutaLuis, "hoja=si"),
		[]string{"Hoja de calificación", "(de 1 a 5)", "Nombre y firma del jurado"}, nil)
	contiene(t, "sin fotografía", expedienteDe(t, x.admin, x.rutaLuis, "fotografia=no"), []string{folio}, []string{"La fotografía no pudo incluirse"})

	// Cada expediente generado queda registrado, sin datos personales.
	generados := x.e.asientos("expediente.generar")
	if len(generados) < 10 || strings.Contains(fmt.Sprint(generados), "López") {
		t.Errorf("registro de expedientes: %d asientos", len(generados))
	}
}

// Expediente por convocatoria: en segundo plano, con índice y marcadores, y
// con los permisos que la persona tenga AL GENERARSE.
func TestExpedientePorConvocatoria(t *testing.T) {
	x := evaluada(t)
	trabajador := x.e.trabajadorDeExportaciones()
	// pedir encola un expediente y devuelve el id de la tarea.
	pedir := func(c *cliente, parametros string) int64 {
		t.Helper()
		tarea := c.exige(http.StatusAccepted, "POST", "/api/v1/expedientes?"+parametros, nil)
		return id(tarea["id"])
	}
	// bajar devuelve el PDF de una tarea ya armada.
	bajar := func(c *cliente, tareaID int64) []byte {
		t.Helper()
		respuesta := c.bajar(fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tareaID))
		if respuesta.Code != http.StatusOK || respuesta.Header().Get("Content-Type") != "application/pdf" ||
			!strings.Contains(respuesta.Header().Get("Content-Disposition"), fmt.Sprintf("fimnme_expediente_%d.pdf", tareaID)) {
			t.Fatalf("descarga del expediente: %d %v", respuesta.Code, respuesta.Header())
		}
		return respuesta.Body.Bytes()
	}
	convocatoria := fmt.Sprintf("convocatoria=%d", x.interpretes)

	// Supervisión: las dos postulaciones enviadas de Intérpretes.
	tareaAdmin := pedir(x.admin, convocatoria)
	if armadas := trabajador.Procesar(context.Background()); armadas != 1 {
		t.Fatalf("el trabajador armó %d expedientes", armadas)
	}
	estado := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/exportaciones/%d", tareaAdmin), nil)
	if estado["estado"] != models.ExportacionLista || estado["archivos"] != float64(2) {
		t.Fatalf("expediente armado: %v", estado)
	}
	pdf := bajar(x.admin, tareaAdmin)
	texto := textoDePDF(t, pdf)
	contiene(t, "expediente de la convocatoria", texto, []string{"Índice", "Convocatoria de Intérpretes 2028", "FIM50-INT-0001", "FIM50-INT-0002",
		"Opinión del jurado 2.", "Descartada", "Documento de consulta interna"}, nil)
	// Marcadores de navegación: uno por postulación.
	if !bytes.Contains(pdf, []byte("/Type /Outlines")) || bytes.Count(pdf, []byte("/Parent ")) < 2 || !bytes.Contains(pdf, []byte("/PageMode /UseOutlines")) {
		t.Error("el expediente por convocatoria no trae marcadores")
	}
	// El índice da la página en que empieza cada postulación.
	paginas := regexp.MustCompile(`(?s)Índice\n.*?FIM50-INT-0001[^\n]*\n(\d+)\n.*?FIM50-INT-0002[^\n]*\n(\d+)\n`).FindStringSubmatch(texto)
	if paginas == nil || paginas[1] != "2" || paginas[2] <= paginas[1] {
		t.Errorf("índice del expediente: %v", paginas)
	}
	// Se avisa por correo a quien lo pidió.
	if avisos := x.e.correosDe(models.PlantillaExportacionLista); len(avisos) != 1 || avisos[0].Destinatario != "admin@ejemplo.mx" {
		t.Errorf("aviso del expediente: %v", avisos)
	}

	// Jurado: sólo lo que pasó a evaluación, con su evaluación y ninguna otra.
	tareaJurado := pedir(x.juradosInt[1], convocatoria+"&hoja=si")
	trabajador.Procesar(context.Background())
	contiene(t, "carpeta del jurado", textoDePDF(t, bajar(x.juradosInt[1], tareaJurado)),
		[]string{"Mi evaluación", "Opinión del jurado 2.", "Hoja de calificación"},
		[]string{"Opinión del jurado 1.", "Opinión del jurado 3.", "Suma", "Descartada", "Nota interna de Rita.", "No cumple las bases."})
	// Una tarea es de quien la pidió.
	x.juradosInt[0].exige(http.StatusNotFound, "GET", fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tareaAdmin), nil)

	// RN-26: cuentan los permisos al GENERAR. La revisora lo pide; antes de
	// que se arme le retiran la asignación; el expediente sale vacío.
	tareaRita := pedir(x.revisora, convocatoria)
	cfg := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", x.interpretes), nil)
	for _, asignacion := range lista(cfg, "asignaciones") {
		if asignacion["perfil"] == "revisor_documental" {
			x.admin.exige(http.StatusOK, "DELETE", fmt.Sprintf("/api/v1/asignaciones/%d", id(asignacion["id"])), nil)
		}
	}
	trabajador.Procesar(context.Background())
	tarea, err := x.e.repo.ExportacionPorID(context.Background(), tareaRita)
	if err != nil || tarea.Estado != models.ExportacionLista || tarea.Archivos != 0 {
		t.Errorf("expediente de quien perdió el permiso: %+v %v", tarea, err)
	}

	// Sin convocatoria no hay expediente; el postulante no lo pide.
	if campos := x.admin.rechaza("POST", "/api/v1/expedientes", nil); !slices.Contains(campos, "convocatoria") {
		t.Errorf("sin convocatoria: %v", campos)
	}
	x.luis.exige(http.StatusForbidden, "POST", "/api/v1/expedientes?"+convocatoria, nil)
}

// csvDe pide un CSV y lo devuelve por renglones, sin la marca inicial.
func csvDe(t *testing.T, c *cliente, ruta string) []string {
	t.Helper()
	respuesta := c.bajar(ruta)
	if respuesta.Code != http.StatusOK || !strings.HasPrefix(respuesta.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("GET %s: %d %s", ruta, respuesta.Code, respuesta.Body.String())
	}
	cuerpo, conMarca := strings.CutPrefix(respuesta.Body.String(), "\xEF\xBB\xBF")
	if !conMarca {
		t.Errorf("GET %s: el CSV no empieza con la marca de UTF-8", ruta)
	}
	return strings.Split(strings.TrimSuffix(cuerpo, "\r\n"), "\r\n")
}

// Generador de reportes: catálogo, campos, granularidad, filtros, puntaje,
// separador, plantillas y reporte de resultados.
func TestReportes(t *testing.T) {
	x := evaluada(t)
	x.admin.estado(x.compositores, "evaluacion", "")
	// Las dos obras de Eva empatan: 4 + 4 de cada uno de los tres jurados.
	for _, jurado := range x.juradosCom {
		for _, obraID := range x.obrasDeEva {
			evaluar(t, jurado, x.rutaEva, obj{"obra_id": obraID, "puntajes": obj{"realizacion": 4, "artistico": 4}, "enviar": true})
		}
	}
	reporteInt := fmt.Sprintf("/api/v1/convocatorias/%d/reporte", x.interpretes)
	reporteCom := fmt.Sprintf("/api/v1/convocatorias/%d/reporte", x.compositores)

	// Catálogo: campos del formulario, calculados y de la evaluación.
	catalogo := x.observador.exige(http.StatusOK, "GET", reporteInt+"/campos", nil)
	etiquetas := make(map[string]string)
	for _, campo := range lista(catalogo, "campos") {
		etiquetas[campo["clave"].(string)] = campo["etiqueta"].(string) + " [" + campo["nivel"].(string) + "]"
	}
	for clave, esperado := range map[string]string{
		"folio": "Folio [postulacion]", "datos.edad": "Edad al cierre [postulacion]", "datos.nombre_ensamble": "",
		"obra.compositor": "", "eval.total": "Suma [evaluacion]", "dictamen": "Resultado del proceso [postulacion]",
		"eval.j2.artistico": "Persona Interna · Puntaje artístico [evaluacion]",
	} {
		if obtenido, existe := etiquetas[clave]; !existe || (esperado != "" && obtenido != esperado) {
			t.Errorf("catálogo, %s: %q (existe: %v)", clave, obtenido, existe)
		}
	}
	if catalogo["granularidad_de_evaluacion"] != "postulacion" {
		t.Errorf("granularidad de evaluación en Intérpretes: %v", catalogo["granularidad_de_evaluacion"])
	}
	if _, hay := etiquetas["obra.dotacion_id"]; hay {
		t.Error("la dotación de la obra no es un campo de Intérpretes")
	}

	// Un reporte por postulación, con valores en claro.
	campos := "folio,estado,datos.nombre_completo,datos.genero,datos.entidad_residencia,datos.edad,obras.count,obras.titulos," +
		"revision.estado,revision.observaciones,eval.j1.realizacion,eval.j3.realizacion,eval.total,eval.lugar,dictamen"
	renglones := csvDe(t, x.admin, reporteInt+"?orden=folio&campos="+campos)
	if len(renglones) != 3 || renglones[0] != "Folio,Estado,Nombre completo,Género,Entidad de residencia,Edad al cierre,Número de obras,"+
		"Títulos de las obras,Resultado de la revisión,Observaciones de la revisión,Persona Interna · Puntaje de realización,"+
		"Persona Interna · Puntaje de realización,Suma,Lugar,Resultado del proceso" {
		t.Fatalf("encabezados del reporte:\n%s", strings.Join(renglones, "\n"))
	}
	luis, ana := renglones[1], renglones[2]
	if strings.HasSuffix(strings.Split(luis, ",")[1], "Descartada") {
		luis, ana = ana, luis // el orden de los folios depende de quién envió primero
	}
	if !strings.HasSuffix(luis, ",Evaluada,Ana López Ruiz,Mujer,Jalisco,36,1,Preludio a Colón,Validada,Nota interna de Rita.,3,5,27,1,") ||
		!strings.HasSuffix(ana, ",Descartada,Ana López Ruiz,Mujer,Jalisco,36,1,Preludio a Colón,Descartada,,,,,,") {
		t.Errorf("renglones del reporte:\n%s\n%s", luis, ana)
	}

	// Separador, idioma y filtros del archivo.
	ingles := csvDe(t, x.admin, reporteInt+"?campos=folio,estado,datos.genero&separador=punto_y_coma&idioma=en&estado=descartada")
	if len(ingles) != 2 || ingles[0] != "Reference number;Status;Gender" || !strings.HasSuffix(ingles[1], ";Discarded;Woman") {
		t.Errorf("reporte en inglés con punto y coma: %v", ingles)
	}
	// Rango de puntaje y orden por puntaje: sólo lo calificado.
	if conPuntaje := csvDe(t, x.admin, reporteInt+"?campos=folio,eval.puntaje&puntaje_min=20&orden=-puntaje"); len(conPuntaje) != 2 || !strings.HasSuffix(conPuntaje[1], ",27") {
		t.Errorf("reporte por rango de puntaje: %v", conPuntaje)
	}
	if fuera := csvDe(t, x.admin, reporteInt+"?campos=folio&puntaje_min=28"); len(fuera) != 1 {
		t.Errorf("nada alcanza 28 puntos: %v", fuera)
	}

	// Por obra, en Compositores: una fila por obra, con los datos de la
	// postulación repetidos y la evaluación de cada obra.
	porObra := csvDe(t, x.admin, reporteCom+"?granularidad=obra&campos=folio,datos.nombre_completo,obra.orden,obra.titulo,obra.dotacion_id,obra.estreno,eval.total,eval.lugar")
	if len(porObra) != 3 || porObra[1] != "FIM50-COM-0001,Ana López Ruiz,1,Cuarteto n.º 1,Cuarteto de cuerdas,Estreno mundial,24,1" ||
		porObra[2] != "FIM50-COM-0001,Ana López Ruiz,2,Cuarteto n.º 2,Cuarteto de cuerdas,Estreno mundial,24,1" {
		t.Errorf("reporte por obra:\n%s", strings.Join(porObra, "\n"))
	}

	// Lo que no cuadra se rechaza diciendo por qué.
	for consulta, campo := range map[string]string{
		reporteInt + "?campos=folio,inventado":                    "campos",
		reporteInt + "?campos=folio,folio":                        "campos",
		reporteInt:                                                "campos",
		reporteInt + "?campos=folio,obra.titulo":                  "granularidad", // campo de obra, reporte por postulación
		reporteCom + "?campos=folio,eval.total":                   "granularidad", // en Compositores se califica la obra
		reporteInt + "?campos=folio,eval.total&granularidad=obra": "granularidad", // en Intérpretes, la postulación
		reporteInt + "?campos=folio&separador=tabulador":          "separador",
		reporteInt + "?campos=folio&puntaje_min=alto":             "puntaje_min",
		reporteInt + "?campos=folio&puntaje_min=9&puntaje_max=3":  "puntaje_max",
		reporteInt + "?campos=folio&orden=inventado":              "orden",
	} {
		if señalados := x.admin.rechaza("GET", consulta, nil); !slices.Contains(señalados, campo) {
			t.Errorf("%s: señaló %v, se esperaba %s", consulta, señalados, campo)
		}
	}

	// Plantillas: se guardan con nombre y sirven en cualquier convocatoria
	// de su tipo.
	guardadas := x.observador.exige(http.StatusOK, "POST", "/api/v1/reportes/plantillas",
		obj{"nombre": "Descartadas", "tipo": "interpretes", "consulta": "campos=folio,estado&estado=descartada&separador=punto_y_coma"})
	plantillaID := id(lista(guardadas, "plantillas")[0]["id"])
	conPlantilla := csvDe(t, x.admin, fmt.Sprintf("%s?plantilla=%d", reporteInt, plantillaID))
	if len(conPlantilla) != 2 || conPlantilla[0] != "Folio;Estado" || !strings.HasSuffix(conPlantilla[1], ";Descartada") {
		t.Errorf("reporte desde plantilla: %v", conPlantilla)
	}
	// Los parámetros de la petición mandan sobre los de la plantilla.
	if sobrepuesta := csvDe(t, x.admin, fmt.Sprintf("%s?plantilla=%d&estado=evaluada&separador=coma", reporteInt, plantillaID)); len(sobrepuesta) != 2 ||
		!strings.HasSuffix(sobrepuesta[1], ",Evaluada") {
		t.Errorf("plantilla con parámetros propios: %v", sobrepuesta)
	}
	if señalados := x.admin.rechaza("GET", fmt.Sprintf("%s?plantilla=%d", reporteCom, plantillaID), nil); !slices.Contains(señalados, "plantilla") {
		t.Errorf("plantilla de otro tipo: %v", señalados)
	}
	x.admin.exige(http.StatusNotFound, "GET", reporteInt+"?plantilla=999", nil)
	x.admin.exige(http.StatusConflict, "POST", "/api/v1/reportes/plantillas", obj{"nombre": "Descartadas", "tipo": "interpretes", "consulta": "campos=folio"})
	for nombre, entrada := range map[string]obj{
		"consulta": {"nombre": "Con convocatoria", "tipo": "interpretes", "consulta": "campos=folio&convocatoria=3"},
		"tipo":     {"nombre": "Sin tipo", "tipo": "coro", "consulta": "campos=folio"},
		"nombre":   {"nombre": " ", "tipo": "interpretes", "consulta": "campos=folio"},
	} {
		if señalados := x.admin.rechaza("POST", "/api/v1/reportes/plantillas", entrada); !slices.Contains(señalados, nombre) {
			t.Errorf("plantilla inválida (%s): %v", nombre, señalados)
		}
	}
	// El observador borra las suyas; el administrador, cualquiera.
	delAdmin := x.admin.exige(http.StatusOK, "POST", "/api/v1/reportes/plantillas", obj{"nombre": "Del admin", "tipo": "compositores", "consulta": "campos=folio"})
	var idDelAdmin int64
	for _, plantilla := range lista(delAdmin, "plantillas") {
		if plantilla["nombre"] == "Del admin" {
			idDelAdmin = id(plantilla["id"])
		}
	}
	x.observador.exige(http.StatusNotFound, "DELETE", fmt.Sprintf("/api/v1/reportes/plantillas/%d", idDelAdmin), nil)
	if quedan := lista(x.admin.exige(http.StatusOK, "DELETE", fmt.Sprintf("/api/v1/reportes/plantillas/%d", plantillaID), nil), "plantillas"); len(quedan) != 1 {
		t.Errorf("tras borrar quedan %v", quedan)
	}

	// Reporte de resultados: la matriz en CSV, con los empates a la vista.
	resultados := csvDe(t, x.admin, fmt.Sprintf("/api/v1/convocatorias/%d/resultados?formato=csv", x.compositores))
	if len(resultados) != 3 || !strings.HasPrefix(resultados[0], "Lugar,Folio,Postulante,Obra,Estado,Puntaje,Suma,Promedio,Jurados que evaluaron,Persona Interna · Puntaje de realización") ||
		!strings.HasPrefix(resultados[1], "1,FIM50-COM-0001,Ana López Ruiz,Cuarteto n.º 1,Evaluada,24,24,4,3,4,4,4,4,4,4,") ||
		!strings.HasPrefix(resultados[2], "1,FIM50-COM-0001,Ana López Ruiz,Cuarteto n.º 2,Evaluada,24,24,4,3,") {
		t.Errorf("reporte de resultados:\n%s", strings.Join(resultados, "\n"))
	}
	matriz := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/resultados", x.compositores), nil)
	if filas := lista(matriz, "filas"); id(filas[0]["lugar"]) != 1 || id(filas[1]["lugar"]) != 1 {
		t.Errorf("los empates comparten lugar: %v %v", filas[0]["lugar"], filas[1]["lugar"])
	}

	// Sólo la supervisión genera reportes, ve tableros y usa plantillas.
	for nombre, c := range map[string]*cliente{"revisora": x.revisora, "jurado": x.juradosInt[0], "postulante": x.luis} {
		for _, ruta := range []string{reporteInt + "?campos=folio", reporteInt + "/campos", "/api/v1/reportes/plantillas",
			fmt.Sprintf("/api/v1/convocatorias/%d/tablero", x.interpretes), "/api/v1/ediciones/1/estadisticas",
			fmt.Sprintf("/api/v1/convocatorias/%d/resultados?formato=csv", x.interpretes)} {
			if estado, _ := c.api("GET", ruta, nil); estado != http.StatusForbidden {
				t.Errorf("%s en %s: estado %d, se esperaba 403", nombre, ruta, estado)
			}
		}
	}
	// Cada reporte queda en la bitácora: qué campos, cuántos renglones, y
	// ningún dato de nadie.
	generados := x.e.asientos("reporte.generar")
	if len(generados) < 5 || strings.Contains(fmt.Sprint(generados), "López") {
		t.Errorf("registro de reportes: %d asientos", len(generados))
	}
}

// Tablero de la convocatoria y estadísticas de la edición.
func TestTableroYEstadisticas(t *testing.T) {
	// Intérpretes llega a evaluación, con dos de sus tres jurados ya
	// calificando a Luis; Compositores sigue en revisión, con una
	// subsanación abierta para Eva que vence en dos días.
	x := hastaRevision(t)
	x.revisorCom.exige(http.StatusCreated, "POST", x.rutaEva+"/subsanaciones",
		obj{"problemas": []obj{{"clase": "campo", "campo": "datos.semblanza", "problema": "Muy breve."}}, "fecha_limite": "2026-11-27T18:00:00Z"})
	x.revisora.exige(http.StatusOK, "POST", x.rutaLuis+"/dictamen", obj{"dictamen": "validada"})
	x.revisora.exige(http.StatusOK, "POST", x.rutaAna+"/dictamen", obj{"dictamen": "descartada", "motivo": "No cumple las bases."})
	x.admin.estado(x.interpretes, "evaluacion", "")
	for _, jurado := range x.juradosInt[:2] {
		evaluar(t, jurado, x.rutaLuis, obj{"puntajes": obj{"realizacion": 4, "artistico": 5}, "observaciones": "Opinión reservada.", "enviar": true})
	}

	tablero := x.observador.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/tablero", x.interpretes), nil)
	porEstado := make(map[string]string)
	for _, cifra := range lista(tablero, "por_estado") {
		porEstado[cifra["clave"].(string)] = fmt.Sprintf("%v %v", cifra["cantidad"], cifra["filtro"])
	}
	filtro := fmt.Sprintf("convocatoria=%d&estado=", x.interpretes)
	if len(porEstado) != 2 || porEstado["en_evaluacion"] != "1 "+filtro+"en_evaluacion" || porEstado["descartada"] != "1 "+filtro+"descartada" {
		t.Errorf("postulaciones por estado: %v", porEstado)
	}
	// La cifra lleva al listado que la produce (RF-DASH-05).
	if listado := x.observador.exige(http.StatusOK, "GET", "/api/v1/archivo?"+filtro+"descartada", nil); listado["total"] != float64(1) {
		t.Errorf("el filtro de la cifra no da su listado: %v", listado["total"])
	}
	dias := lista(tablero, "envios_por_dia")
	if len(dias) != 1 || dias[0]["clave"] != "2026-11-25" || dias[0]["cantidad"] != float64(2) {
		t.Errorf("envíos por día: %v", dias)
	}
	if pendientes := tablero["documentos_pendientes"].(obj); pendientes["documentos"] != float64(6) || pendientes["postulaciones"].(obj)["cantidad"] != float64(2) {
		t.Errorf("documentos pendientes: %v", pendientes)
	}
	// Avance del jurado: cuántas lleva de cuántas, y ningún puntaje.
	avance := lista(tablero, "evaluacion")
	if len(avance) != 3 || fmt.Sprint(avance[0]["enviadas"], avance[0]["pendientes"], avance[0]["total"]) != "1 0 1" {
		t.Errorf("avance de los jurados: %v", avance)
	}
	if fmt.Sprint(avance[2]["enviadas"], avance[2]["pendientes"], avance[2]["total"]) != "0 1 1" {
		t.Errorf("avance del jurado que no ha calificado: %v", avance[2])
	}
	if cuerpo := fmt.Sprint(tablero); strings.Contains(cuerpo, "puntaje") || strings.Contains(cuerpo, "Opinión") {
		t.Errorf("el tablero no lleva puntajes ni observaciones: %s", cuerpo)
	}
	x.admin.exige(http.StatusNotFound, "GET", "/api/v1/convocatorias/999/tablero", nil)

	// Subsanaciones abiertas y por vencer, en Compositores.
	subsanaciones := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/tablero", x.compositores), nil)["subsanaciones"].(obj)
	if subsanaciones["abiertas"].(obj)["cantidad"] != float64(1) || subsanaciones["por_vencer"].(obj)["cantidad"] != float64(1) ||
		subsanaciones["vencidas"].(obj)["cantidad"] != float64(0) {
		t.Errorf("subsanaciones del tablero: %v", subsanaciones)
	}

	// Estadísticas de la edición: sólo lo enviado.
	edicion := id(x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", x.interpretes), nil)["edicion"].(obj)["id"])
	estadisticas := x.observador.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/ediciones/%d/estadisticas", edicion), nil)
	primera := func(clave string) string {
		cifras := lista(estadisticas, clave)
		if len(cifras) == 0 {
			return "(vacío)"
		}
		return fmt.Sprintf("%v=%v", cifras[0]["clave"], cifras[0]["cantidad"])
	}
	if estadisticas["postulaciones"] != float64(3) || estadisticas["obras"] != float64(4) || estadisticas["duracion_media_min"] != 10.5 ||
		primera("por_genero") != "mujer=3" || primera("por_entidad") != "JAL=3" || primera("por_pais") != "MX=3" || primera("por_edad") != "36_a_45=3" ||
		primera("por_estreno") != "estreno_mundial=2" || primera("por_convocatoria") != "interpretes=2" {
		t.Errorf("estadísticas de la edición: %v", estadisticas)
	}
	dotaciones := lista(estadisticas, "por_dotacion")
	if len(dotaciones) != 1 || dotaciones[0]["nombre"] != "Cuarteto de cuerdas" || dotaciones[0]["cantidad"] != float64(2) {
		t.Errorf("obras por dotación: %v", dotaciones)
	}
	// Con «convocatoria», sólo esa; y cada cifra trae su filtro.
	soloCom := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/ediciones/%d/estadisticas?convocatoria=%d", edicion, x.compositores), nil)
	if soloCom["postulaciones"] != float64(1) || !strings.Contains(lista(soloCom, "por_genero")[0]["filtro"].(string), "genero=mujer") {
		t.Errorf("estadísticas de una convocatoria: %v", soloCom)
	}
	x.admin.exige(http.StatusNotFound, "GET", "/api/v1/ediciones/999/estadisticas", nil)
}
