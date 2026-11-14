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
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de extremo a extremo del paso 9a: archivo histórico con búsqueda y
// filtros, enlaces por folio, vistas guardadas y descargas masivas.

// archivoDePrueba es un entorno con cuatro postulaciones en estados distintos:
//
//	Luis  — Intérpretes, validada y ya en evaluación  (FIM50-INT-0001)
//	Ana   — Intérpretes, enviada y aún sin dictaminar (FIM50-INT-0002)
//	Eva   — Compositores, enviada                     (FIM50-COM-0001)
//	Bruno — Intérpretes, borrador
type archivoDePrueba struct {
	e                         *entornoPrueba
	admin, observador         *cliente
	revisora                  *cliente // de Intérpretes
	juradoInt, juradoCom      *cliente
	luis, ana, eva, bruno     *cliente
	rutaLuis, rutaAna         string
	interpretes, compositores int64
	cuarteto                  int64
	trabajador                *TrabajadorExportaciones
}

func nuevoArchivoDePrueba(t *testing.T) *archivoDePrueba {
	t.Helper()
	e := nuevoEntorno(t)
	admin, interpretes, compositores, cuarteto, _ := e.convocatoriasAbiertas()
	x := &archivoDePrueba{e: e, admin: admin, interpretes: interpretes, compositores: compositores, cuarteto: cuarteto}
	si := obj{"confirmo": true, "acepto_bases": true}

	x.luis, x.ana, x.eva = e.postulanteVerificado("luis@ejemplo.mx"), e.postulanteVerificado("ana@ejemplo.mx"), e.postulanteVerificado("eva@ejemplo.mx")
	x.rutaLuis, _ = e.solistaCompleto(x.luis, interpretes, "luis")
	guardar(t, x.luis, x.rutaLuis, obj{"datos.nombre_completo": "Luis Núñez Carrillo", "datos.nombre_artistico": "Luis N.", "datos.entidad_residencia": "NLE"})
	x.rutaAna, _ = e.solistaCompleto(x.ana, interpretes, "ana")
	rutaEva, _ := e.compositorCompleto(x.eva, compositores, cuarteto)
	guardar(t, x.eva, rutaEva, obj{"datos.nombre_completo": "Eva Brouwer", "datos.nombre_artistico": "Eva B."})
	x.bruno = e.postulante("bruno@ejemplo.mx")
	borrador := x.bruno.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "solista"})
	guardar(t, x.bruno, fmt.Sprintf("/api/v1/postulaciones/%d", id(borrador["id"])), obj{"datos.nombre_completo": "Bruno Díaz"})
	// En este orden, para que los folios sean los del comentario de arriba.
	x.luis.exige(http.StatusOK, "POST", x.rutaLuis+"/envio", si)
	x.ana.exige(http.StatusOK, "POST", x.rutaAna+"/envio", si)
	x.eva.exige(http.StatusOK, "POST", rutaEva+"/envio", si)

	x.revisora = e.revisor(admin, interpretes, "rita@ejemplo.mx")
	x.observador = e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	for i := 1; i <= 3; i++ {
		c, _ := e.asignado(admin, interpretes, fmt.Sprintf("jurado-int-%d@ejemplo.mx", i), "jurado")
		x.juradoInt = c
		x.juradoCom, _ = e.asignado(admin, compositores, fmt.Sprintf("jurado-com-%d@ejemplo.mx", i), "jurado")
	}
	admin.estado(interpretes, "revision_documental", "Inicio de la revisión, en la prueba")
	x.revisora.exige(http.StatusOK, "POST", x.rutaLuis+"/dictamen", obj{"dictamen": "validada"})
	admin.estado(interpretes, "evaluacion", "")

	x.trabajador = &TrabajadorExportaciones{Datos: e.repo, Almacen: e.almacen, Directorio: e.exportaciones,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Ahora: e.reloj.Ahora}
	return x
}

// nombresEn lista, en orden, los nombres de un listado del archivo.
func nombresEn(listado obj) string {
	var nombres []string
	for _, renglon := range lista(listado, "postulaciones") {
		nombres = append(nombres, strings.Fields(renglon["nombre"].(string))[0])
	}
	return strings.Join(nombres, " ")
}

// archivo consulta el listado con los parámetros dados y devuelve los nombres
// de pila que encontró, en orden.
func (x *archivoDePrueba) archivo(c *cliente, parametros string) string {
	x.e.t.Helper()
	return nombresEn(c.exige(http.StatusOK, "GET", "/api/v1/archivo?"+parametros, nil))
}

// Búsqueda de texto completo, filtros combinables, orden, paginación y CSV.
func TestArchivoBusquedaYFiltros(t *testing.T) {
	x := nuevoArchivoDePrueba(t)

	// Sin filtros el administrador ve todo, borradores incluidos, por folio.
	todo := x.admin.exige(http.StatusOK, "GET", "/api/v1/archivo", nil)
	if todo["total"] != float64(4) || todo["pagina"] != float64(1) || todo["tamano"] != float64(50) || todo["orden"] != "folio" {
		t.Fatalf("listado completo: %v", todo)
	}
	luis := lista(x.admin.exige(http.StatusOK, "GET", "/api/v1/archivo?q=luis", nil), "postulaciones")[0]
	if luis["folio"] != "FIM50-INT-0001" || luis["estado"] != "en_evaluacion" || luis["tipo"] != "interpretes" || luis["edicion"] != float64(50) ||
		luis["obras"] != float64(1) || luis["duracion_min"] != float64(12) || luis["subcategoria"] != "solista" || luis["enviada_at"] == nil {
		t.Errorf("renglón de Luis: %v", luis)
	}

	// Búsqueda: sin acentos ni mayúsculas, por inicio de palabra, por
	// fragmento, por folio, por título de obra, por compositor y por correo.
	for consulta, esperado := range map[string]string{
		"q=nunez":                "Luis",
		"q=N%C3%9A%C3%91EZ+carr": "Luis",     // «NÚÑEZ carr»
		"q=brouwer":              "Eva",      //
		"q=arteto":               "Eva",      // fragmento de «Cuarteto»
		"q=FIM50-COM":            "Eva",      // folio
		"q=FIM50-INT-0002":       "Ana",      //
		"q=preludio+colon":       "Luis Ana", // título de obra
		"q=juli%C3%A1n+carrillo": "Luis Ana", // compositor de la obra
		"q=bruno%40ejemplo.mx":   "Bruno",    // correo de la cuenta
		"q=diaz":                 "Bruno",    // los borradores también se encuentran
		"q=xilofono":             "",         // nada
		"q=nunez+brouwer":        "",         // todas las palabras deben estar
		"q=%22%27%3B+DROP+*+(":   "",         // la sintaxis del buscador no se interpreta
	} {
		if obtenido := x.archivo(x.admin, consulta); obtenido != esperado {
			t.Errorf("búsqueda %s: %q, se esperaba %q", consulta, obtenido, esperado)
		}
	}

	// Filtros, solos y combinados.
	for consulta, esperado := range map[string]string{
		"tipo=compositores":                            "Eva",
		"estado=borrador":                              "Bruno",
		"estado=en_evaluacion":                         "Luis",
		"subcategoria=solista&orden=nombre":            "Ana Bruno Luis",
		"entidad_residencia=NLE":                       "Luis",
		"entidad_concierto=CDMX&orden=nombre":          "Ana Luis",
		"genero=mujer&nacionalidad=MX&orden=nombre":    "Ana Eva Luis",
		fmt.Sprintf("dotacion=%d", x.cuarteto):         "Eva",
		"estreno=estreno_mundial":                      "Eva",
		"edicion=50&tipo=interpretes&estado=enviada":   "Ana",
		"edicion=49":                                   "",
		fmt.Sprintf("convocatoria=%d", x.compositores): "Eva",
		"enviada_desde=2026-11-25&orden=nombre":        "Ana Eva Luis",
		"enviada_hasta=2026-11-24":                     "",
		"documentos_pendientes=no&orden=nombre":        "Bruno",
		"con_subsanaciones=si":                         "",
		"ligas_con_problema=no&estado=enviada&q=ruiz":  "Ana",
		"q=lopez&tipo=compositores":                    "Eva", // el texto se combina con los filtros (Eva se registró como «López»)
	} {
		if obtenido := x.archivo(x.admin, consulta); obtenido != esperado {
			t.Errorf("filtro %s: %q, se esperaba %q", consulta, obtenido, esperado)
		}
	}

	// Selección a mano por id.
	idLuis, idAna := strings.TrimPrefix(x.rutaLuis, "/api/v1/postulaciones/"), strings.TrimPrefix(x.rutaAna, "/api/v1/postulaciones/")
	if obtenido := x.archivo(x.admin, "ids="+idLuis+","+idAna+"&orden=-nombre"); obtenido != "Luis Ana" {
		t.Errorf("selección por ids: %q", obtenido)
	}

	// Orden y paginación: el total es el de la búsqueda, no el de la página.
	pagina := x.admin.exige(http.StatusOK, "GET", "/api/v1/archivo?orden=-nombre&tamano=3", nil)
	if nombresEn(pagina) != "Luis Eva Bruno" || pagina["total"] != float64(4) || pagina["orden"] != "-nombre" {
		t.Errorf("primera página: %v", pagina)
	}
	if obtenido := x.archivo(x.admin, "orden=-nombre&tamano=3&pagina=2"); obtenido != "Ana" {
		t.Errorf("segunda página: %q", obtenido)
	}
	if obtenido := x.archivo(x.admin, "orden=duracion&estado=enviada"); obtenido != "Ana Eva" {
		t.Errorf("orden por duración: %q", obtenido)
	}

	// Valores que no son válidos se rechazan señalando el parámetro.
	for consulta, campo := range map[string]string{
		"orden=puntaje": "orden", "tamano=500": "tamano", "pagina=-1": "pagina", "con_subsanaciones=quizas": "con_subsanaciones",
		"tipo=CORO": "tipo", "edicion=x": "edicion", "enviada_desde=ayer": "enviada_desde", "ids=1,dos": "ids",
		"enviada_desde=2026-12-01&enviada_hasta=2026-11-01": "enviada_hasta",
	} {
		if campos := x.admin.rechaza("GET", "/api/v1/archivo?"+consulta, nil); !slices.Contains(campos, campo) {
			t.Errorf("%s: señaló %v, se esperaba %s", consulta, campos, campo)
		}
	}

	// CSV: el resultado completo de la búsqueda, sin paginar.
	csv := x.admin.bajar("/api/v1/archivo?tipo=interpretes&tamano=1&formato=csv")
	if csv.Code != http.StatusOK || !strings.HasPrefix(csv.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("CSV: %d %s", csv.Code, csv.Header().Get("Content-Type"))
	}
	renglones := strings.Split(strings.TrimSpace(csv.Body.String()), "\r\n")
	if len(renglones) != 4 || !strings.Contains(renglones[0], "folio,estado,edicion,convocatoria,subcategoria,nombre") ||
		!strings.HasPrefix(renglones[1], ",borrador,50,interpretes,solista,Bruno Díaz,") ||
		!strings.HasPrefix(renglones[2], "FIM50-INT-0001,en_evaluacion,50,interpretes,solista,Luis Núñez Carrillo,2026-11-25") {
		t.Errorf("CSV del listado:\n%s", csv.Body.String())
	}

	// El índice se rehace completo sin cambiar lo que se encuentra.
	if total, err := x.e.repo.ReconstruirIndiceDeBusqueda(context.Background()); err != nil || total != 4 {
		t.Fatalf("reconstruir índice: %d, %v", total, err)
	}
	if obtenido := x.archivo(x.admin, "q=nunez"); obtenido != "Luis" {
		t.Errorf("tras reconstruir el índice: %q", obtenido)
	}
}

// Lo que cada perfil ve en el archivo, que no caduca con la etapa.
func TestArchivoAlcancePorPerfil(t *testing.T) {
	x := nuevoArchivoDePrueba(t)
	x.admin.exige(http.StatusOK, "PUT", x.rutaLuis+"/dictamen-final", obj{"dictamen": "seleccionada"})

	for nombre, caso := range map[string]struct {
		c          *cliente
		parametros string
		esperado   string
	}{
		"el observador ve todo":                            {x.observador, "orden=nombre", "Ana Bruno Eva Luis"},
		"la revisora, su convocatoria sin borradores":      {x.revisora, "orden=nombre", "Ana Luis"},
		"la revisora no encuentra la otra convocatoria":    {x.revisora, "q=brouwer", ""},
		"la revisora no encuentra borradores":              {x.revisora, "q=bruno", ""},
		"la revisora no alcanza otras por id":              {x.revisora, "tipo=compositores", ""},
		"el jurado, sólo lo que pasó a evaluación":         {x.juradoInt, "", "Luis"},
		"el jurado no encuentra lo no validado":            {x.juradoInt, "q=FIM50-INT-0002", ""},
		"el jurado de la otra convocatoria, nada":          {x.juradoCom, "", ""},
		"la supervisión filtra por resultado":              {x.admin, "dictamen=seleccionada", "Luis"},
		"la supervisión filtra por resultado (ninguna)":    {x.admin, "dictamen=no_seleccionada", ""},
		"al jurado el filtro de resultado no le dice nada": {x.juradoInt, "dictamen=no_seleccionada", "Luis"},
		"a la revisora tampoco":                            {x.revisora, "dictamen=no_seleccionada&orden=nombre", "Ana Luis"},
		"al jurado tampoco los de la revisión":             {x.juradoInt, "documentos_pendientes=no", "Luis"},
	} {
		if obtenido := x.archivo(caso.c, caso.parametros); obtenido != caso.esperado {
			t.Errorf("%s: %q, se esperaba %q", nombre, obtenido, caso.esperado)
		}
	}
	// El postulante no consulta el archivo: tiene su propia lista.
	x.luis.exige(http.StatusForbidden, "GET", "/api/v1/archivo", nil)
	x.e.cliente().exige(http.StatusUnauthorized, "GET", "/api/v1/archivo", nil)

	// El resultado del proceso no viaja en el listado para nadie.
	if cuerpo := x.admin.bajar("/api/v1/archivo").Body.String(); strings.Contains(cuerpo, "seleccionada") {
		t.Errorf("el listado trae el resultado: %s", cuerpo)
	}

	// Enlace permanente por folio: mismos permisos que la consulta por id.
	idLuis := id(x.luis.exige(http.StatusOK, "GET", x.rutaLuis, nil)["id"])
	for nombre, c := range map[string]*cliente{"admin": x.admin, "observador": x.observador, "revisora": x.revisora, "jurado": x.juradoInt, "titular": x.luis} {
		if p := c.exige(http.StatusOK, "GET", "/api/v1/folios/FIM50-INT-0001", nil); id(p["id"]) != idLuis {
			t.Errorf("%s: el folio resolvió a %v", nombre, p["id"])
		}
	}
	for nombre, c := range map[string]*cliente{"otra postulante": x.ana, "jurado de otra convocatoria": x.juradoCom} {
		if estado, _ := c.api("GET", "/api/v1/folios/FIM50-INT-0001", nil); estado != http.StatusNotFound {
			t.Errorf("%s: estado %d al consultar un folio ajeno", nombre, estado)
		}
	}
	// El jurado no ve por folio lo que no pasó a evaluación.
	x.juradoInt.exige(http.StatusNotFound, "GET", "/api/v1/folios/FIM50-INT-0002", nil)
	x.admin.exige(http.StatusNotFound, "GET", "/api/v1/folios/FIM50-INT-9999", nil)
}

// Vistas guardadas: propias de cada cuenta y siempre dentro del sitio.
func TestVistasGuardadas(t *testing.T) {
	x := nuevoArchivoDePrueba(t)
	vistas := x.revisora.exige(http.StatusOK, "POST", "/api/v1/vistas", obj{"nombre": " Por validar ", "ruta": "/archivo?estado=enviada&orden=-enviada"})
	guardada := lista(vistas, "vistas")[0]
	if len(lista(vistas, "vistas")) != 1 || guardada["nombre"] != "Por validar" || guardada["ruta"] != "/archivo?estado=enviada&orden=-enviada" {
		t.Fatalf("vista guardada: %v", vistas)
	}
	x.revisora.exige(http.StatusConflict, "POST", "/api/v1/vistas", obj{"nombre": "Por validar", "ruta": "/archivo"})
	for ruta, campo := range map[string]string{"https://malo.example/archivo": "ruta", "//malo.example": "ruta", "archivo": "ruta"} {
		if campos := x.revisora.rechaza("POST", "/api/v1/vistas", obj{"nombre": "Otra", "ruta": ruta}); !slices.Contains(campos, campo) {
			t.Errorf("ruta %q: señaló %v", ruta, campos)
		}
	}
	if campos := x.revisora.rechaza("POST", "/api/v1/vistas", obj{"nombre": "  ", "ruta": "/archivo"}); !slices.Contains(campos, "nombre") {
		t.Errorf("nombre vacío: señaló %v", campos)
	}

	// Cada cuenta ve y borra sólo las suyas.
	if ajenas := lista(x.admin.exige(http.StatusOK, "GET", "/api/v1/vistas", nil), "vistas"); len(ajenas) != 0 {
		t.Errorf("el administrador ve vistas ajenas: %v", ajenas)
	}
	ruta := fmt.Sprintf("/api/v1/vistas/%d", id(guardada["id"]))
	x.admin.exige(http.StatusNotFound, "DELETE", ruta, nil)
	if quedan := lista(x.revisora.exige(http.StatusOK, "DELETE", ruta, nil), "vistas"); len(quedan) != 0 {
		t.Errorf("tras borrar quedan %v", quedan)
	}
	// También el postulante puede guardar las suyas.
	x.luis.exige(http.StatusOK, "POST", "/api/v1/vistas", obj{"nombre": "Mis postulaciones", "ruta": "/postulaciones"})
}

// zipDe baja una descarga masiva lista y devuelve sus archivos por nombre.
func zipDe(t *testing.T, c *cliente, tareaID int64) map[string][]byte {
	t.Helper()
	respuesta := c.bajar(fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tareaID))
	if respuesta.Code != http.StatusOK || respuesta.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(respuesta.Header().Get("Content-Disposition"), ".zip") {
		t.Fatalf("descarga del ZIP: %d %v", respuesta.Code, respuesta.Header())
	}
	lector, err := zip.NewReader(bytes.NewReader(respuesta.Body.Bytes()), int64(respuesta.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	archivos := make(map[string][]byte)
	for _, entrada := range lector.File {
		contenido, err := entrada.Open()
		if err != nil {
			t.Fatal(err)
		}
		datos, err := io.ReadAll(contenido)
		if err != nil {
			t.Fatal(err)
		}
		archivos[entrada.Name] = datos
	}
	return archivos
}

// pedirZIP pide una descarga masiva, la arma y devuelve su id y su estado final.
func (x *archivoDePrueba) pedirZIP(c *cliente, parametros string) (int64, obj) {
	x.e.t.Helper()
	tarea := c.exige(http.StatusAccepted, "POST", "/api/v1/exportaciones?"+parametros, nil)
	if tarea["estado"] != models.ExportacionEncolada {
		x.e.t.Fatalf("tarea recién pedida: %v", tarea)
	}
	tareaID := id(tarea["id"])
	if armadas := x.trabajador.Procesar(context.Background()); armadas != 1 {
		x.e.t.Fatalf("el trabajador armó %d descargas", armadas)
	}
	return tareaID, c.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/exportaciones/%d", tareaID), nil)
}

// Descarga masiva en ZIP: alcance, tipos, nomenclatura, manifiesto, permisos,
// registro y vencimiento.
func TestDescargaMasiva(t *testing.T) {
	x := nuevoArchivoDePrueba(t)
	nombres := func(archivos map[string][]byte) string {
		lista := make([]string, 0, len(archivos))
		for nombre := range archivos {
			lista = append(lista, nombre)
		}
		slices.Sort(lista)
		return strings.Join(lista, "\n")
	}

	// Sólo las partituras de Intérpretes: no hay que bajar todo para eso.
	pendiente := x.admin.exige(http.StatusAccepted, "POST", "/api/v1/exportaciones?tipo=interpretes&tipos=partitura", nil)
	tareaID := id(pendiente["id"])
	x.admin.exige(http.StatusConflict, "GET", fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tareaID), nil) // aún no está lista
	if armadas := x.trabajador.Procesar(context.Background()); armadas != 1 {
		t.Fatalf("el trabajador armó %d descargas", armadas)
	}
	tarea := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/exportaciones/%d", tareaID), nil)
	if tarea["estado"] != models.ExportacionLista || tarea["archivos"] != float64(2) || tarea["bytes"].(float64) <= 0 || tarea["expira_at"] == nil {
		t.Fatalf("tarea armada: %v", tarea)
	}
	archivos := zipDe(t, x.admin, tareaID)
	esperados := "FIM50-INT-0001_Luis_Nunez_Carrillo/obra-01_Preludio_a_Colon/partitura.pdf\n" +
		"FIM50-INT-0002_Ana_Lopez_Ruiz/obra-01_Preludio_a_Colon/partitura.pdf\nmanifiesto.csv"
	if nombres(archivos) != esperados {
		t.Fatalf("contenido del ZIP:\n%s", nombres(archivos))
	}
	// Los archivos son los originales, byte por byte, y el manifiesto lo dice.
	partitura := pdfDePrueba("partitura luis")
	if !bytes.Equal(archivos["FIM50-INT-0001_Luis_Nunez_Carrillo/obra-01_Preludio_a_Colon/partitura.pdf"], partitura) {
		t.Error("la partitura del ZIP no es la que se subió")
	}
	manifiesto := string(archivos["manifiesto.csv"])
	if !strings.Contains(manifiesto, "archivo,folio,postulante,obra_numero,obra,documento,version,bytes,sha256") ||
		!strings.Contains(manifiesto, fmt.Sprintf("FIM50-INT-0001_Luis_Nunez_Carrillo/obra-01_Preludio_a_Colon/partitura.pdf,FIM50-INT-0001,Luis Núñez Carrillo,1,Preludio a Colón,partitura,1,%d,%s",
			len(partitura), sumaDe(partitura))) {
		t.Errorf("manifiesto:\n%s", manifiesto)
	}
	// Bajarlo queda registrado, sin datos personales en la bitácora.
	if bajadas := x.e.asientos("exportacion.descargar"); len(bajadas) != 1 || bajadas[0].EntidadID != tareaID {
		t.Errorf("registro de la descarga: %v", bajadas)
	}
	if pedidas := x.e.asientos("exportacion.solicitar"); len(pedidas) != 1 || strings.Contains(fmt.Sprint(pedidas[0]), "Luis") {
		t.Errorf("registro de la solicitud: %v", pedidas)
	}

	// La revisora baja lo de su convocatoria —sin borradores— y nada más,
	// aunque no ponga ningún filtro o pida otra convocatoria.
	tareaRita, estado := x.pedirZIP(x.revisora, "")
	if estado["archivos"] != float64(6) {
		t.Errorf("descarga de la revisora: %v", estado)
	}
	for nombre := range zipDe(t, x.revisora, tareaRita) {
		if nombre != "manifiesto.csv" && !strings.HasPrefix(nombre, "FIM50-INT-") {
			t.Errorf("la revisora recibió %s", nombre)
		}
	}
	if _, estado := x.pedirZIP(x.revisora, "tipo=compositores"); estado["archivos"] != float64(0) {
		t.Errorf("la revisora bajó documentos de otra convocatoria: %v", estado)
	}
	// Una descarga es de quien la pidió.
	x.revisora.exige(http.StatusNotFound, "GET", fmt.Sprintf("/api/v1/exportaciones/%d", tareaID), nil)
	x.revisora.exige(http.StatusNotFound, "GET", fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tareaID), nil)

	// El jurado no descarga en masa; el postulante, sólo su postulación.
	idLuis, idAna := strings.TrimPrefix(x.rutaLuis, "/api/v1/postulaciones/"), strings.TrimPrefix(x.rutaAna, "/api/v1/postulaciones/")
	x.juradoInt.exige(http.StatusForbidden, "POST", "/api/v1/exportaciones", nil)
	x.juradoInt.exige(http.StatusNotFound, "POST", "/api/v1/exportaciones?postulacion="+idLuis, nil)
	x.luis.exige(http.StatusForbidden, "POST", "/api/v1/exportaciones", nil)
	x.luis.exige(http.StatusForbidden, "POST", "/api/v1/exportaciones?ids="+idLuis+","+idAna, nil)
	x.luis.exige(http.StatusNotFound, "POST", "/api/v1/exportaciones?postulacion="+idAna, nil)
	tareaLuis, estado := x.pedirZIP(x.luis, "postulacion="+idLuis)
	propios := zipDe(t, x.luis, tareaLuis)
	if estado["archivos"] != float64(3) || propios["FIM50-INT-0001_Luis_Nunez_Carrillo/fotografia.jpg"] == nil ||
		propios["FIM50-INT-0001_Luis_Nunez_Carrillo/obra-01_Preludio_a_Colon/autorizacion.pdf"] == nil {
		t.Errorf("descarga del postulante: %v\n%s", estado, nombres(propios))
	}

	// Una tarea que un apagado dejó a medias vuelve a la cola al arrancar.
	interrumpida := id(x.admin.exige(http.StatusAccepted, "POST", "/api/v1/exportaciones?tipos=fotografia", nil)["id"])
	if _, hay, err := x.e.repo.TomarExportacion(context.Background()); err != nil || !hay {
		t.Fatalf("tomar la tarea: %v %v", hay, err)
	}
	if x.trabajador.Procesar(context.Background()) != 0 {
		t.Error("una tarea «procesando» no debe tomarse dos veces")
	}
	if reencoladas, err := x.e.repo.ReencolarExportacionesInterrumpidas(context.Background()); err != nil || reencoladas != 1 {
		t.Fatalf("reencolar: %d %v", reencoladas, err)
	}
	if x.trabajador.Procesar(context.Background()) != 1 {
		t.Error("la tarea reencolada no se armó")
	}
	if fotos := zipDe(t, x.admin, interrumpida); len(fotos) != 4 { // tres fotografías y el manifiesto
		t.Errorf("fotografías: %s", nombres(fotos))
	}

	// A los siete días el enlace vence y el archivo se borra del disco.
	archivoEnDisco := filepath.Join(x.e.exportaciones, fmt.Sprintf("%d.zip", tareaID))
	if _, err := os.Stat(archivoEnDisco); err != nil {
		t.Fatalf("el ZIP no está en disco: %v", err)
	}
	x.e.reloj.Avanzar(models.VigenciaDeExportacion + time.Hour)
	x.admin.ingresar("admin@ejemplo.mx", passwordPrueba) // en una semana la sesión también venció
	x.admin.exige(http.StatusGone, "GET", fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tareaID), nil)
	x.trabajador.Procesar(context.Background())
	if _, err := os.Stat(archivoEnDisco); !os.IsNotExist(err) {
		t.Errorf("el ZIP vencido sigue en disco: %v", err)
	}
	if restos, err := os.ReadDir(x.e.exportaciones); err != nil || len(restos) != 0 {
		t.Errorf("quedan archivos tras vencer: %v %v", restos, err)
	}
}
