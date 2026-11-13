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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de extremo a extremo del paso 8: evaluación por jurado, matriz de
// resultados y, sobre todo, el aislamiento entre jurados.

// evaluacionDePrueba es un entorno llevado hasta la etapa de evaluación en
// sus dos convocatorias, con tres jurados en cada una.
type evaluacionDePrueba struct {
	e                         *entornoPrueba
	admin, observador         *cliente
	interpretes, compositores int64
	juradosInt, juradosCom    []*cliente // tres por convocatoria
	idsInt                    []int64    // ids de cuenta de los jurados de Intérpretes
	luis, ana, eva            *cliente   // postulantes
	rutaLuis, rutaAna         string     // Intérpretes: validada y descartada
	rutaEva                   string     // Compositores, con dos obras
	obrasDeEva                []int64
	revisora                  *cliente
}

// compositorCompleto crea una postulación de Compositores con dos obras para
// cuarteto y todo lo que el envío exige.
func (e *entornoPrueba) compositorCompleto(c *cliente, convocatoriaID, dotacionID int64) (ruta string, obras []int64) {
	e.t.Helper()
	p := c.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", convocatoriaID), obj{})
	ruta = fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	datos := datosDeSolista()
	delete(datos, "concierto.requerimientos_tecnicos")
	delete(datos, "concierto.entidad")
	guardar(e.t, c, ruta, datos)
	c.documento(ruta, "fotografia", 0, "retrato.png", contenidoPNG)
	for i := 1; i <= 2; i++ {
		p = c.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
		obraID := id(lista(p, "obras")[i-1]["id"])
		obras = append(obras, obraID)
		guardar(e.t, c, fmt.Sprintf("/api/v1/obras/%d", obraID), obj{"obra.titulo": fmt.Sprintf("Cuarteto n.º %d", i), "obra.duracion_min": 9,
			"obra.estreno": "estreno_mundial", "obra.dotacion_id": dotacionID, "obra.nota_obra": "Nota.", "obra.requerimientos_tecnicos": "Ninguno."})
		c.documento(ruta, "partitura", obraID, "partitura.pdf", pdfDePrueba(fmt.Sprintf("partitura %d", i)))
		c.documento(ruta, "particellas", obraID, "partes.pdf", pdfDePrueba(fmt.Sprintf("partes %d", i)))
	}
	return ruta, obras
}

// hastaEvaluacion prepara el entorno: postulaciones enviadas, revisión
// documental terminada (una validada y una descartada en Intérpretes; una
// validada en Compositores), jurados asignados y ambas convocatorias en
// etapa de evaluación.
func hastaEvaluacion(t *testing.T) *evaluacionDePrueba {
	t.Helper()
	e := nuevoEntorno(t)
	admin, interpretes, compositores, cuarteto, _ := e.convocatoriasAbiertas()
	x := &evaluacionDePrueba{e: e, admin: admin, interpretes: interpretes, compositores: compositores}
	si := obj{"confirmo": true, "acepto_bases": true}

	x.luis, x.ana, x.eva = e.postulanteVerificado("luis@ejemplo.mx"), e.postulanteVerificado("ana@ejemplo.mx"), e.postulanteVerificado("eva@ejemplo.mx")
	x.rutaLuis, _ = e.solistaCompleto(x.luis, interpretes, "luis")
	x.rutaAna, _ = e.solistaCompleto(x.ana, interpretes, "ana")
	x.rutaEva, x.obrasDeEva = e.compositorCompleto(x.eva, compositores, cuarteto)
	for c, ruta := range map[*cliente]string{x.luis: x.rutaLuis, x.ana: x.rutaAna, x.eva: x.rutaEva} {
		c.exige(http.StatusOK, "POST", ruta+"/envio", si)
	}

	x.revisora = e.revisor(admin, interpretes, "rita@ejemplo.mx")
	revisorCom := e.revisor(admin, compositores, "raul@ejemplo.mx")
	x.observador = e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	for i := 1; i <= 3; i++ {
		c, usuarioID := e.asignado(admin, interpretes, fmt.Sprintf("jurado-int-%d@ejemplo.mx", i), "jurado")
		x.juradosInt, x.idsInt = append(x.juradosInt, c), append(x.idsInt, usuarioID)
		c, _ = e.asignado(admin, compositores, fmt.Sprintf("jurado-com-%d@ejemplo.mx", i), "jurado")
		x.juradosCom = append(x.juradosCom, c)
	}
	admin.estado(interpretes, "revision_documental", "Inicio de la revisión, en la prueba")
	admin.estado(compositores, "revision_documental", "Inicio de la revisión, en la prueba")

	// Mientras quede una postulación sin dictaminar, no se pasa a evaluación.
	x.revisora.exige(http.StatusOK, "POST", x.rutaLuis+"/dictamen", obj{"dictamen": "validada"})
	if campos := admin.rechaza("POST", fmt.Sprintf("/api/v1/convocatorias/%d/estado", interpretes), obj{"estado": "evaluacion"}); fmt.Sprint(campos) != "[postulaciones]" {
		t.Fatalf("pasar a evaluación con una postulación sin dictaminar: %v", campos)
	}
	x.revisora.exige(http.StatusOK, "POST", x.rutaAna+"/dictamen", obj{"dictamen": "descartada", "motivo": "No cumple las bases."})
	revisorCom.exige(http.StatusOK, "POST", x.rutaEva+"/dictamen", obj{"dictamen": "validada"})
	return x
}

// evaluar guarda la evaluación de un jurado y exige que se acepte.
func evaluar(t *testing.T, jurado *cliente, ruta string, entrada obj) obj {
	t.Helper()
	return jurado.exige(http.StatusOK, "PUT", ruta+"/evaluacion", entrada)
}

func TestEvaluacionPorJurado(t *testing.T) {
	x := hastaEvaluacion(t)
	e, admin := x.e, x.admin
	j1, j2, j3 := x.juradosInt[0], x.juradosInt[1], x.juradosInt[2]
	lista1 := fmt.Sprintf("/api/v1/convocatorias/%d/evaluacion", x.interpretes)

	// Antes de la etapa de evaluación el jurado no tiene nada a la vista.
	antes := j1.exige(http.StatusOK, "GET", lista1, nil)
	if len(lista(antes, "unidades")) != 0 || antes["abierta"] != false {
		t.Errorf("antes de la evaluación no hay unidades: %v", antes)
	}
	if estado, _ := j1.api("GET", x.rutaLuis, nil); estado != http.StatusNotFound {
		t.Errorf("antes de la evaluación el jurado no ve la postulación: %d", estado)
	}
	admin.estado(x.interpretes, "evaluacion", "")

	// RN-20: sólo entra la validada. La descartada no llega al jurado.
	trabajo := j1.exige(http.StatusOK, "GET", lista1, nil)
	unidades := lista(trabajo, "unidades")
	if len(unidades) != 1 || unidades[0]["folio"] != "FIM50-INT-0001" || unidades[0]["obra_id"] != nil || unidades[0]["iniciada"] != false ||
		trabajo["abierta"] != true || fmt.Sprint(trabajo["progreso"]) != "map[enviadas:0 total:1]" || len(lista(trabajo, "criterios")) != 2 {
		t.Fatalf("lista de trabajo del jurado: %v", trabajo)
	}
	if estado, _ := j1.api("GET", x.rutaAna, nil); estado != http.StatusNotFound {
		t.Errorf("la postulación descartada no existe para el jurado: %d", estado)
	}
	// Las validadas pasaron a «en evaluación»; el postulante lo ve.
	if propia := x.luis.exige(http.StatusOK, "GET", x.rutaLuis, nil); propia["estado"] != "en_evaluacion" {
		t.Errorf("estado visible para el postulante: %v", propia["estado"])
	}

	// La evaluación NO es a ciegas: el jurado ve nombre, semblanza y documentos,
	// y puede descargarlos. Lo que no ve es la revisión ni el resultado.
	vista := j1.exige(http.StatusOK, "GET", x.rutaLuis, nil)
	campos := vista["campos"].(obj)
	if campos["datos.nombre_completo"] != "Ana López Ruiz" || campos["datos.semblanza"] != "Semblanza de Ana." || len(lista(vista, "documentos")) != 3 {
		t.Errorf("el jurado debe ver la postulación completa: %v", campos)
	}
	if _, hay := vista["revision"]; hay {
		t.Error("el jurado no recibe el bloque de revisión")
	}
	if _, hay := vista["dictamen_final"]; hay {
		t.Error("el jurado no recibe el dictamen final")
	}
	if d := j1.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", id(documentoDe(t, vista, "partitura")["id"]))); d.Code != http.StatusOK {
		t.Errorf("el jurado descarga los documentos: %d", d.Code)
	}

	// Quién puede evaluar: sólo los jurados de ESA convocatoria. El
	// administrador, no (§2.2).
	cuatro := obj{"puntajes": obj{"realizacion": 4, "artistico": 4}, "enviar": true}
	for nombre, caso := range map[string]struct {
		c      *cliente
		estado int
	}{"administrador": {admin, 403}, "observador": {x.observador, 403}, "revisora": {x.revisora, 403}, "titular": {x.luis, 403},
		"jurado de otra convocatoria": {x.juradosCom[0], 404}, "otro postulante": {x.ana, 404}, "sin sesión": {e.cliente(), 401}} {
		if estado, r := caso.c.api("PUT", x.rutaLuis+"/evaluacion", cuatro); estado != caso.estado {
			t.Errorf("evaluar como %s: %d, se esperaba %d (%v)", nombre, estado, caso.estado, r)
		}
		if estado, _ := caso.c.api("GET", x.rutaLuis+"/evaluacion", nil); estado != caso.estado {
			t.Errorf("leer la evaluación como %s: %d, se esperaba %d", nombre, estado, caso.estado)
		}
	}
	if estado, _ := admin.api("GET", lista1, nil); estado != http.StatusForbidden {
		t.Errorf("el administrador no tiene lista de trabajo de jurado: %d", estado)
	}

	// Borrador incompleto; reglas del envío (RF-EVA-03 y 06, RN-17).
	vacia := j1.exige(http.StatusOK, "GET", x.rutaLuis+"/evaluacion", nil)
	if vacia["iniciada"] != false || vacia["enviada"] != false || len(vacia["puntajes"].(obj)) != 0 {
		t.Errorf("evaluación sin empezar: %v", vacia)
	}
	borrador := evaluar(t, j1, x.rutaLuis, obj{"puntajes": obj{"realizacion": 4}, "observaciones": "  Pendiente de oír el video.  "})
	if borrador["iniciada"] != true || borrador["enviada"] != false || fmt.Sprint(borrador["puntajes"]) != "map[realizacion:4]" ||
		borrador["observaciones"] != "Pendiente de oír el video." {
		t.Errorf("borrador de evaluación: %v", borrador)
	}
	for nombre, caso := range map[string]struct {
		entrada obj
		campos  string
	}{
		"enviar sin un criterio":     {obj{"puntajes": obj{"realizacion": 4}, "enviar": true}, "[artistico]"},
		"puntaje fuera de la escala": {obj{"puntajes": obj{"realizacion": 6, "artistico": 0}}, "[artistico realizacion]"},
		"criterio que no existe":     {obj{"puntajes": obj{"originalidad": 3}}, "[originalidad]"},
		"una obra, en Intérpretes":   {obj{"obra_id": 1, "puntajes": obj{"realizacion": 4, "artistico": 4}}, "[obra_id]"},
	} {
		if campos := j1.rechaza("PUT", x.rutaLuis+"/evaluacion", caso.entrada); fmt.Sprint(campos) != caso.campos {
			t.Errorf("%s: %v, se esperaba %s", nombre, campos, caso.campos)
		}
	}
	if estado, _ := j1.api("PUT", x.rutaLuis+"/evaluacion", obj{"puntajes": obj{"realizacion": 4.5}}); estado != http.StatusBadRequest {
		t.Errorf("un puntaje con decimales no es válido: %d", estado)
	}
	if progreso := j1.exige(http.StatusOK, "GET", lista1, nil); fmt.Sprint(progreso["progreso"]) != "map[enviadas:0 total:1]" || lista(progreso, "unidades")[0]["iniciada"] != true {
		t.Errorf("avance con un borrador: %v", progreso["progreso"])
	}

	// Envío; después puede corregirse, pero no dejarse incompleta ni volver a borrador.
	enviada := evaluar(t, j1, x.rutaLuis, obj{"puntajes": obj{"realizacion": 4, "artistico": 5}, "observaciones": "OBS-J1", "enviar": true})
	if enviada["enviada"] != true || enviada["enviada_at"] == nil {
		t.Errorf("evaluación enviada: %v", enviada)
	}
	if campos := j1.rechaza("PUT", x.rutaLuis+"/evaluacion", obj{"puntajes": obj{"realizacion": 3}}); fmt.Sprint(campos) != "[artistico]" {
		t.Errorf("una evaluación enviada no puede quedar incompleta: %v", campos)
	}
	corregida := evaluar(t, j1, x.rutaLuis, obj{"puntajes": obj{"realizacion": 3, "artistico": 5}, "observaciones": "OBS-J1"})
	if corregida["enviada"] != true || corregida["enviada_at"] != enviada["enviada_at"] || fmt.Sprint(corregida["puntajes"]) != "map[artistico:5 realizacion:3]" {
		t.Errorf("corrección de una evaluación enviada: %v", corregida)
	}
	if progreso := j1.exige(http.StatusOK, "GET", lista1, nil); fmt.Sprint(progreso["progreso"]) != "map[enviadas:1 total:1]" {
		t.Errorf("avance tras enviar: %v", progreso["progreso"])
	}

	// RN-18: la postulación queda evaluada cuando los TRES han enviado.
	evaluar(t, j2, x.rutaLuis, obj{"puntajes": obj{"realizacion": 5, "artistico": 2}, "observaciones": "OBS-J2", "enviar": true})
	evaluar(t, j3, x.rutaLuis, obj{"puntajes": obj{"realizacion": 1, "artistico": 1}, "observaciones": "OBS-J3"}) // en borrador
	if p := admin.exige(http.StatusOK, "GET", x.rutaLuis, nil); p["estado"] != "en_evaluacion" {
		t.Errorf("con dos envíos y un borrador sigue en evaluación: %v", p["estado"])
	}

	// Matriz de resultados (RF-EVA-08): sólo cuentan las enviadas.
	resultados := fmt.Sprintf("/api/v1/convocatorias/%d/resultados", x.interpretes)
	matriz := admin.exige(http.StatusOK, "GET", resultados, nil)
	fila := lista(matriz, "filas")[0]
	if len(lista(matriz, "jurados")) != 3 || len(lista(matriz, "filas")) != 1 || id(fila["total"]) != 15 || id(fila["jurados_que_evaluaron"]) != 2 ||
		fila["promedio"].(float64) != 3.75 || id(fila["lugar"]) != 1 {
		t.Errorf("matriz con dos evaluaciones enviadas: %v", fila)
	}
	tercera := lista(fila, "evaluaciones")[2]
	if tercera["enviada"] != false || tercera["puntajes"] != nil || tercera["observaciones"] != nil {
		t.Errorf("un borrador no aparece en la matriz: %v", tercera)
	}
	evaluar(t, j3, x.rutaLuis, obj{"puntajes": obj{"realizacion": 2, "artistico": 3}, "observaciones": "OBS-J3", "enviar": true})
	if p := admin.exige(http.StatusOK, "GET", x.rutaLuis, nil); p["estado"] != "evaluada" {
		t.Errorf("con los tres envíos queda evaluada: %v", p["estado"])
	}
	matriz = x.observador.exige(http.StatusOK, "GET", resultados, nil)
	fila = lista(matriz, "filas")[0]
	if id(fila["total"]) != 20 || id(fila["jurados_que_evaluaron"]) != 3 || fila["estado"] != "evaluada" ||
		fmt.Sprint(lista(fila, "evaluaciones")[1]["puntajes"]) != "map[artistico:2 realizacion:5]" || lista(fila, "evaluaciones")[1]["observaciones"] != "OBS-J2" {
		t.Errorf("matriz completa: %v", fila)
	}
	// La matriz es sólo de supervisión, y cada consulta queda en la bitácora.
	for nombre, c := range map[string]*cliente{"jurado": j1, "revisora": x.revisora, "postulante": x.luis} {
		if estado, _ := c.api("GET", resultados, nil); estado != http.StatusForbidden {
			t.Errorf("matriz de resultados para %s: %d", nombre, estado)
		}
	}
	if n := len(e.asientos("evaluacion.consultar")); n != 2 {
		t.Errorf("consultas de la matriz en la bitácora: %d", n)
	}
	// La bitácora de evaluar no lleva puntajes ni observaciones.
	for _, asiento := range append(e.asientos("evaluacion.guardar"), e.asientos("evaluacion.enviar")...) {
		if texto := fmt.Sprint(asiento.Despues); strings.Contains(texto, "OBS-") || strings.Contains(texto, "puntaje") {
			t.Errorf("la bitácora no debe guardar el contenido de una evaluación: %s", texto)
		}
	}
	if n := len(e.asientos("evaluacion.enviar")); n != 3 {
		t.Errorf("envíos de evaluación en la bitácora: %d", n)
	}

	// Dictamen final: sólo el administrador; nadie más lo ve.
	rutaFinal := x.rutaLuis + "/dictamen-final"
	for nombre, c := range map[string]*cliente{"observador": x.observador, "jurado": j1, "revisora": x.revisora, "titular": x.luis} {
		if estado, _ := c.api("PUT", rutaFinal, obj{"dictamen": "seleccionada"}); estado != http.StatusForbidden {
			t.Errorf("dictamen final por %s: %d", nombre, estado)
		}
	}
	if campos := admin.rechaza("PUT", rutaFinal, obj{"dictamen": "ganadora"}); fmt.Sprint(campos) != "[dictamen]" {
		t.Errorf("dictamen final inventado: %v", campos)
	}
	if campos := admin.rechaza("PUT", x.rutaAna+"/dictamen-final", obj{"dictamen": "seleccionada"}); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("dictamen final de una postulación descartada: %v", campos)
	}
	if p := admin.exige(http.StatusOK, "PUT", rutaFinal, obj{"dictamen": "seleccionada"}); p["dictamen_final"] != "seleccionada" {
		t.Errorf("dictamen final: %v", p["dictamen_final"])
	}
	if lista(x.observador.exige(http.StatusOK, "GET", resultados, nil), "filas")[0]["dictamen_final"] != "seleccionada" {
		t.Error("la matriz debe traer el dictamen final")
	}
	for nombre, c := range map[string]*cliente{"titular": x.luis, "jurado": j1, "revisora": x.revisora} {
		if p := c.exige(http.StatusOK, "GET", x.rutaLuis, nil); strings.Contains(fmt.Sprint(p), "seleccionada") {
			t.Errorf("el dictamen final no debe llegar a %s", nombre)
		}
	}
	if propia := x.luis.exige(http.StatusOK, "GET", x.rutaLuis, nil); propia["estado"] != "evaluada" {
		t.Errorf("el postulante ve que fue evaluada: %v", propia["estado"])
	}
	// El descarte sigue oculto para su titular también en esta etapa.
	if propia := x.ana.exige(http.StatusOK, "GET", x.rutaAna, nil); propia["estado"] != "en_revision" {
		t.Errorf("la descartada sigue apareciendo en revisión a su titular: %v", propia["estado"])
	}

	// RN-19: cerrada la etapa, todo queda congelado, pero cada jurado sigue
	// viendo lo suyo.
	admin.estado(x.interpretes, "cerrada", "")
	if campos := j1.rechaza("PUT", x.rutaLuis+"/evaluacion", obj{"puntajes": obj{"realizacion": 5, "artistico": 5}}); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("evaluar con la etapa cerrada: %v", campos)
	}
	cerrada := j1.exige(http.StatusOK, "GET", lista1, nil)
	propia := j1.exige(http.StatusOK, "GET", x.rutaLuis+"/evaluacion", nil)
	if cerrada["abierta"] != false || fmt.Sprint(propia["puntajes"]) != "map[artistico:5 realizacion:3]" || propia["observaciones"] != "OBS-J1" {
		t.Errorf("tras el cierre el jurado conserva su evaluación, congelada: %v", propia)
	}
	if p := admin.exige(http.StatusOK, "PUT", rutaFinal, obj{"dictamen": "lista_espera"}); p["dictamen_final"] != "lista_espera" {
		t.Errorf("el dictamen final puede registrarse con la convocatoria cerrada: %v", p["dictamen_final"])
	}
}

// En Compositores se califica cada obra por separado (RN-16).
func TestEvaluacionPorObra(t *testing.T) {
	x := hastaEvaluacion(t)
	x.admin.estado(x.compositores, "evaluacion", "")
	j1 := x.juradosCom[0]
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d/evaluacion", x.compositores)
	trabajo := j1.exige(http.StatusOK, "GET", ruta, nil)
	unidades := lista(trabajo, "unidades")
	if len(unidades) != 2 || id(unidades[0]["obra_id"]) != x.obrasDeEva[0] || unidades[0]["obra"] != "Cuarteto n.º 1" || unidades[1]["obra"] != "Cuarteto n.º 2" ||
		unidades[0]["folio"] != "FIM50-COM-0001" || fmt.Sprint(trabajo["progreso"]) != "map[enviadas:0 total:2]" {
		t.Fatalf("unidades de Compositores: %v", unidades)
	}
	cinco := func(obraID int64) obj {
		return obj{"obra_id": obraID, "puntajes": obj{"realizacion": 5, "artistico": 5}, "enviar": true}
	}
	// Sin obra, o con una obra de otra postulación, no hay unidad.
	if campos := j1.rechaza("PUT", x.rutaEva+"/evaluacion", obj{"puntajes": obj{"realizacion": 5, "artistico": 5}}); fmt.Sprint(campos) != "[obra_id]" {
		t.Errorf("evaluar la postulación entera en Compositores: %v", campos)
	}
	if campos := j1.rechaza("PUT", x.rutaEva+"/evaluacion", cinco(9999)); fmt.Sprint(campos) != "[obra_id]" {
		t.Errorf("evaluar una obra ajena: %v", campos)
	}
	// Cada obra lleva su propia evaluación.
	evaluar(t, j1, x.rutaEva, cinco(x.obrasDeEva[0]))
	evaluar(t, j1, x.rutaEva, obj{"obra_id": x.obrasDeEva[1], "puntajes": obj{"realizacion": 2}})
	primera := j1.exige(http.StatusOK, "GET", fmt.Sprintf("%s/evaluacion?obra_id=%d", x.rutaEva, x.obrasDeEva[0]), nil)
	segunda := j1.exige(http.StatusOK, "GET", fmt.Sprintf("%s/evaluacion?obra_id=%d", x.rutaEva, x.obrasDeEva[1]), nil)
	if primera["enviada"] != true || segunda["enviada"] != false || fmt.Sprint(segunda["puntajes"]) != "map[realizacion:2]" {
		t.Errorf("evaluaciones por obra: %v / %v", primera, segunda)
	}
	if trabajo = j1.exige(http.StatusOK, "GET", ruta, nil); fmt.Sprint(trabajo["progreso"]) != "map[enviadas:1 total:2]" {
		t.Errorf("avance por obra: %v", trabajo["progreso"])
	}
	// La postulación queda evaluada cuando los tres han enviado LAS DOS obras.
	evaluar(t, j1, x.rutaEva, cinco(x.obrasDeEva[1]))
	for _, jurado := range x.juradosCom[1:] {
		evaluar(t, jurado, x.rutaEva, cinco(x.obrasDeEva[0]))
	}
	evaluar(t, x.juradosCom[1], x.rutaEva, cinco(x.obrasDeEva[1]))
	if p := x.admin.exige(http.StatusOK, "GET", x.rutaEva, nil); p["estado"] != "en_evaluacion" {
		t.Errorf("con cinco de seis evaluaciones sigue en evaluación: %v", p["estado"])
	}
	evaluar(t, x.juradosCom[2], x.rutaEva, obj{"obra_id": x.obrasDeEva[1], "puntajes": obj{"realizacion": 1, "artistico": 2}, "enviar": true})
	if p := x.admin.exige(http.StatusOK, "GET", x.rutaEva, nil); p["estado"] != "evaluada" {
		t.Errorf("con las seis evaluaciones queda evaluada: %v", p["estado"])
	}
	// La matriz trae una fila por obra, ordenada por total.
	filas := lista(x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/resultados", x.compositores), nil), "filas")
	if len(filas) != 2 || id(filas[0]["total"]) != 30 || filas[0]["obra"] != "Cuarteto n.º 1" || id(filas[1]["total"]) != 23 || id(filas[1]["lugar"]) != 2 {
		t.Errorf("matriz por obra: %v", filas)
	}
}

// TestAislamientoEntreJurados es la prueba obligatoria de §8.4: recorre TODAS
// las operaciones de la API —y las páginas— con la sesión de cada jurado,
// probando cada identificador, y falla si en alguna respuesta aparece algo de
// la evaluación de otro jurado.
//
// Cada jurado deja en sus observaciones una marca única. Si la marca de otro
// llega a cualquier respuesta, el aislamiento está roto.
func TestAislamientoEntreJurados(t *testing.T) {
	x := hastaEvaluacion(t)
	x.admin.estado(x.interpretes, "evaluacion", "")
	x.admin.estado(x.compositores, "evaluacion", "")
	marca := func(convocatoria string, n int) string { return fmt.Sprintf("SECRETO-%s-J%d", convocatoria, n) }
	for i, jurado := range x.juradosInt {
		evaluar(t, jurado, x.rutaLuis, obj{"puntajes": obj{"realizacion": i + 1, "artistico": 5 - i}, "observaciones": marca("INT", i+1), "enviar": i < 2})
	}
	for i, jurado := range x.juradosCom {
		for _, obraID := range x.obrasDeEva {
			evaluar(t, jurado, x.rutaEva, obj{"obra_id": obraID, "puntajes": obj{"realizacion": i + 2, "artistico": i + 3}, "observaciones": marca("COM", i+1), "enviar": true})
		}
	}
	// La supervisión sí ve todas las marcas: la prueba tiene con qué fallar.
	for convocatoria, convocatoriaID := range map[string]int64{"INT": x.interpretes, "COM": x.compositores} {
		matriz := fmt.Sprint(x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/resultados", convocatoriaID), nil))
		if !strings.Contains(matriz, marca(convocatoria, 1)) || !strings.Contains(matriz, marca(convocatoria, 2)) {
			t.Fatalf("la matriz de %s debe traer las observaciones de los jurados", convocatoria)
		}
	}

	// Todas las rutas del servidor, con cada variable sustituida por cada
	// identificador que puede existir en la base de prueba.
	sustituir := func(patron string) []string {
		rutas := []string{patron}
		for _, variable := range []struct {
			nombre  string
			valores []string
		}{{"{id}", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}}, {"{version}", []string{"1"}}, {"{solicitud}", []string{"1"}},
			{"{clase}", []string{"imagen"}}} {
			var expandidas []string
			for _, ruta := range rutas {
				if !strings.Contains(ruta, variable.nombre) {
					expandidas = append(expandidas, ruta)
					continue
				}
				for _, valor := range variable.valores {
					expandidas = append(expandidas, strings.Replace(ruta, variable.nombre, valor, 1))
				}
			}
			rutas = expandidas
		}
		return rutas
	}
	// Variantes con que alguien intentaría pedir lo de otro.
	trucos := []string{"", "?jurado_id=%d", "?obra_id=%d&jurado_id=%d", "?usuario_id=%d", "?formato=csv", "?todos=1"}

	barrer := func(nombre string, jurado *cliente, propia string, ajenas []string, otroID int64) {
		t.Helper()
		peticiones, conPropia := 0, false
		revisar := func(metodo, ruta, cuerpo string) {
			peticiones++
			if strings.Contains(cuerpo, propia) {
				conPropia = true
			}
			for _, ajena := range ajenas {
				if strings.Contains(cuerpo, ajena) {
					t.Errorf("FUGA: %s recibió %q en %s %s", nombre, ajena, metodo, ruta)
				}
			}
			// Tampoco la suma ni el promedio, que son de la supervisión. (La
			// descripción pública de la API nombra esos campos, pero no trae
			// ningún dato: se exceptúa.)
			esDescripcion := strings.HasPrefix(ruta, "/api/v1/openapi.json")
			if !esDescripcion && (strings.Contains(cuerpo, "jurados_que_evaluaron") || strings.Contains(cuerpo, `"promedio"`)) {
				t.Errorf("FUGA: %s recibió agregados en %s %s", nombre, metodo, ruta)
			}
		}
		for _, operacion := range e2eRutas(x.e) {
			for _, ruta := range sustituir(operacion.patron) {
				switch operacion.metodo {
				case "GET":
					for _, truco := range trucos {
						consulta := truco
						if strings.Count(truco, "%d") == 1 {
							consulta = fmt.Sprintf(truco, otroID)
						} else if strings.Count(truco, "%d") == 2 {
							consulta = fmt.Sprintf(truco, x.obrasDeEva[0], otroID)
						}
						r := httptest.NewRequest("GET", ruta+consulta, nil)
						revisar("GET", ruta+consulta, jurado.hacer(r).Body.String())
					}
				case "PUT":
					// Guardar la evaluación devuelve la propia; con un
					// jurado_id ajeno en el cuerpo, se rechaza o se ignora.
					if strings.HasSuffix(operacion.patron, "/evaluacion") {
						_, respuesta := jurado.api("PUT", ruta, obj{"jurado_id": otroID, "puntajes": obj{}})
						revisar("PUT", ruta, fmt.Sprint(respuesta))
					}
				}
			}
		}
		for _, pagina := range []string{"/", "/cuenta", "/convocatorias/50/interpretes", "/convocatorias/50/compositores"} {
			revisar("GET", pagina, jurado.get(pagina).Body.String())
		}
		if peticiones < 500 || !conPropia {
			t.Errorf("el barrido de %s fue insuficiente: %d peticiones, vio su propia evaluación: %v", nombre, peticiones, conPropia)
		}
	}
	for i, jurado := range x.juradosInt {
		var ajenas []string
		for n := 1; n <= 3; n++ {
			if n != i+1 {
				ajenas = append(ajenas, marca("INT", n))
			}
			ajenas = append(ajenas, marca("COM", n)) // ni nada de la otra convocatoria
		}
		barrer(fmt.Sprintf("el jurado %d de Intérpretes", i+1), jurado, marca("INT", i+1), ajenas, x.idsInt[(i+1)%3])
	}
	for i, jurado := range x.juradosCom {
		var ajenas []string
		for n := 1; n <= 3; n++ {
			if n != i+1 {
				ajenas = append(ajenas, marca("COM", n))
			}
			ajenas = append(ajenas, marca("INT", n))
		}
		barrer(fmt.Sprintf("el jurado %d de Compositores", i+1), jurado, marca("COM", i+1), ajenas, x.idsInt[0])
	}
	// Tampoco el revisor ni el postulante reciben evaluaciones.
	todas := []string{marca("INT", 1), marca("INT", 2), marca("INT", 3), marca("COM", 1)}
	for nombre, c := range map[string]*cliente{"la revisora": x.revisora, "el postulante": x.luis} {
		for _, ruta := range []string{x.rutaLuis, x.rutaLuis + "/evaluacion", fmt.Sprintf("/api/v1/convocatorias/%d/resultados", x.interpretes),
			fmt.Sprintf("/api/v1/convocatorias/%d/evaluacion", x.interpretes), "/api/v1/postulaciones"} {
			cuerpo := c.bajar(ruta).Body.String()
			for _, m := range todas {
				if strings.Contains(cuerpo, m) {
					t.Errorf("FUGA: %s recibió %q en %s", nombre, m, ruta)
				}
			}
		}
	}
}

// e2eRutas devuelve la tabla de operaciones de la API del servidor de prueba.
func e2eRutas(e *entornoPrueba) []rutaAPI { return e.rutas }

// TestSupervisionSoloDesdeResultados comprueba, leyendo el código de los
// controladores, que la matriz de resultados —lo único que cruza jurados— se
// pida en un solo lugar, y que ese lugar esté protegido por a.supervisa.
func TestSupervisionSoloDesdeResultados(t *testing.T) {
	archivos := token.NewFileSet()
	paquetes, err := parser.ParseDir(archivos, ".", func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	usos := 0
	for _, paquete := range paquetes {
		for _, archivo := range paquete.Files {
			for _, declaracion := range archivo.Decls {
				funcion, esFuncion := declaracion.(*ast.FuncDecl)
				if !esFuncion {
					continue
				}
				ast.Inspect(funcion, func(nodo ast.Node) bool {
					selector, ok := nodo.(*ast.SelectorExpr)
					if ok && selector.Sel.Name == "Supervision" {
						usos++
						if funcion.Name.Name != "apiResultados" {
							t.Errorf("%s: %s usa Supervision(); sólo apiResultados puede", archivos.Position(selector.Pos()), funcion.Name.Name)
						}
					}
					return true
				})
			}
		}
	}
	if usos != 1 {
		t.Errorf("Supervision() debe usarse exactamente una vez en los controladores: %d", usos)
	}
	fuente, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fuente), `"/api/v1/convocatorias/{id}/resultados", a.supervisa(a.apiResultados)`) || strings.Count(string(fuente), "apiResultados") != 1 {
		t.Error("la ruta de resultados debe ir envuelta en a.supervisa, y ser la única que llega a apiResultados")
	}
}
