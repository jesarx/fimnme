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
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de las pantallas de supervisión (paso 13b): quién entra, el
// tablero sin puntajes, la matriz con el dictamen final, el generador de
// reportes con sus plantillas, las estadísticas y la nómina.

// supervisionEvaluada prepara Intérpretes en evaluación con dos jurados que
// ya enviaron su calificación de Luis y una tercera excusada.
func supervisionEvaluada(t *testing.T) *evaluacionDePrueba {
	t.Helper()
	x := hastaEvaluacion(t)
	x.admin.estado(x.interpretes, "evaluacion", "")
	evaluar(t, x.juradosInt[0], x.rutaLuis, obj{"puntajes": obj{"realizacion": 4, "artistico": 5}, "observaciones": "OBS-J1", "enviar": true})
	evaluar(t, x.juradosInt[1], x.rutaLuis, obj{"puntajes": obj{"realizacion": 3, "artistico": 2}, "observaciones": "OBS-J2", "enviar": true})
	x.juradosInt[2].exige(http.StatusOK, "PUT", x.rutaLuis+"/excusa", obj{"motivo": "Fue mi alumna."})
	return x
}

func TestQuienEntraALaSupervision(t *testing.T) {
	x := supervisionEvaluada(t)
	base := fmt.Sprintf("/supervision/convocatorias/%d", x.interpretes)
	luis := idDeRutaAPI(t, x.rutaLuis)
	rutas := []string{"/supervision", base, base + "/resultados", base + "/reporte", base + "/reporte.csv?campos=folio",
		fmt.Sprintf("/supervision/ediciones/%d/estadisticas", 2)}

	// El menú la ofrece a quien supervisa; nadie más entra.
	esperar(t, x.admin.get("/postulaciones"), http.StatusOK, `href="/supervision"`)
	esperar(t, x.observador.get("/postulaciones"), http.StatusOK, `href="/supervision"`)
	for nombre, c := range map[string]*cliente{"jurado": x.juradosInt[0], "revisora": x.revisora, "postulante": x.luis} {
		noAparece(t, c.get("/postulaciones"), `href="/supervision"`)
		for _, ruta := range append(rutas, base+"/nomina") {
			if w := c.get(ruta); w.Code != http.StatusForbidden {
				t.Errorf("%s en %s: estado %d", nombre, ruta, w.Code)
			}
		}
		if w := c.post(fmt.Sprintf("/supervision/postulaciones/%d/dictamen-final", luis), url.Values{"dictamen": {"seleccionada"}}); w.Code != http.StatusForbidden {
			t.Errorf("%s al dictaminar: estado %d", nombre, w.Code)
		}
	}
	for _, ruta := range rutas {
		if w := x.observador.get(ruta); w.Code != http.StatusOK {
			t.Errorf("observador en %s: estado %d", ruta, w.Code)
		}
	}
	// La nómina y el dictamen final son del administrador.
	if w := x.observador.get(base + "/nomina"); w.Code != http.StatusForbidden {
		t.Errorf("observador en la nómina: %d", w.Code)
	}
	noAparece(t, x.observador.get(base), `href="`+base+`/nomina"`)
	for ruta, valores := range map[string]url.Values{
		fmt.Sprintf("/supervision/postulaciones/%d/dictamen-final", luis): {"dictamen": {"seleccionada"}},
		base + "/nomina": {"accion": {"publicar"}, "confirmo": {"1"}},
	} {
		if w := x.observador.post(ruta, valores); w.Code != http.StatusForbidden {
			t.Errorf("observador en %s: estado %d", ruta, w.Code)
		}
	}
	for _, ruta := range []string{"/supervision/convocatorias/999", "/supervision/convocatorias/999/resultados", "/supervision/ediciones/999/estadisticas",
		fmt.Sprintf("/supervision/ediciones/2/estadisticas?convocatoria=%d", 999)} {
		if w := x.admin.get(ruta); w.Code != http.StatusNotFound {
			t.Errorf("%s: estado %d", ruta, w.Code)
		}
	}
}

func TestTableroYResultadosDesdeLasPantallas(t *testing.T) {
	x := supervisionEvaluada(t)
	base := fmt.Sprintf("/supervision/convocatorias/%d", x.interpretes)
	luis := idDeRutaAPI(t, x.rutaLuis)

	// El tablero: cifras por estado (las de la revisión llevan a la bandeja)
	// y el avance de cada jurado, sin un solo puntaje ni observación.
	w := x.observador.get(base)
	esperar(t, w, http.StatusOK, "Postulaciones: 2", "Descartada", "Evaluada",
		fmt.Sprintf(`href="/revision/convocatorias/%d?estado=descartada"`, x.interpretes), "Avance de los jurados", "1 / 1", "Ritmo de envíos")
	noAparece(t, w, "OBS-J1", "OBS-J2", "Fue mi alumna", "Puntaje")

	// La matriz: lo de cada jurado, la excusa, la suma y el comparable.
	w = x.observador.get(base + "/resultados")
	esperar(t, w, http.StatusOK, "FIM50-INT-0001", "OBS-J1", "OBS-J2", "Excusa", "<b>21</b>", "2 jurados evaluaron", "Sin resultado",
		fmt.Sprintf("/api/v1/convocatorias/%d/resultados?formato=csv", x.interpretes))
	noAparece(t, w, `name="dictamen"`)
	// La descartada no está en la matriz.
	noAparece(t, w, "FIM50-INT-0002")

	// Dictamen final: el administrador lo captura; un valor inventado se rechaza junto a su fila.
	esperar(t, x.admin.get(base+"/resultados"), http.StatusOK, `name="dictamen"`, `<option value="seleccionada">Seleccionada</option>`)
	destino := fmt.Sprintf("/supervision/postulaciones/%d/dictamen-final", luis)
	esperar(t, x.admin.post(destino, url.Values{"dictamen": {"ganadora"}}), http.StatusUnprocessableEntity,
		`class="campo__error" role="alert">El valor no es una de las opciones disponibles.`)
	redirige(t, x.admin.post(destino, url.Values{"dictamen": {"seleccionada"}}), base+fmt.Sprintf("/resultados?aviso=dictamen#u-%d", luis))
	esperar(t, x.admin.get(base+"/resultados?aviso=dictamen"), http.StatusOK, "Resultado guardado", `<option value="seleccionada" selected>`)
	esperar(t, x.observador.get(base+"/resultados"), http.StatusOK, "Seleccionada")
	if w := x.admin.post("/supervision/postulaciones/999/dictamen-final", url.Values{"dictamen": {"seleccionada"}}); w.Code != http.StatusNotFound {
		t.Errorf("dictamen de una postulación inexistente: %d", w.Code)
	}
	// La descartada no recibe dictamen final.
	ana := idDeRutaAPI(t, x.rutaAna)
	esperar(t, x.admin.post(fmt.Sprintf("/supervision/postulaciones/%d/dictamen-final", ana), url.Values{"dictamen": {"seleccionada"}}),
		http.StatusUnprocessableEntity)
	if asientos := x.e.asientos("postulacion.dictamen_final"); len(asientos) != 1 {
		t.Errorf("el dictamen final debía quedar una vez en la bitácora: %d", len(asientos))
	}

	// En Compositores hay una fila por obra y un solo formulario por postulación.
	x.admin.estado(x.compositores, "evaluacion", "")
	w = x.admin.get(fmt.Sprintf("/supervision/convocatorias/%d/resultados", x.compositores))
	if formularios := strings.Count(w.Body.String(), `action="/supervision/postulaciones/`); formularios != 1 {
		t.Errorf("formularios de dictamen en Compositores: %d", formularios)
	}
}

func TestReportesYPlantillasDesdeLasPantallas(t *testing.T) {
	x := supervisionEvaluada(t)
	base := fmt.Sprintf("/supervision/convocatorias/%d", x.interpretes)

	// El generador ofrece el catálogo, también los campos de la evaluación.
	w := x.observador.get(base + "/reporte")
	esperar(t, w, http.StatusOK, `name="campos" value="folio" checked`, `value="datos.nombre_completo"`, `value="eval.total"`, "De la evaluación",
		`formaction="`+base+`/reporte/plantillas"`)

	// Descargar: las casillas llegan como varios «campos».
	w = x.observador.get(base + "/reporte.csv?campos=folio&campos=datos.nombre_completo&campos=eval.total&orden=-puntaje&estado=&separador=coma")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("reporte: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if cuerpo := w.Body.String(); !strings.Contains(cuerpo, "Folio") || !strings.Contains(cuerpo, "FIM50-INT-0001") || !strings.Contains(cuerpo, ",14") {
		t.Errorf("contenido del reporte: %s", cuerpo)
	}
	if len(x.e.asientos("reporte.generar")) == 0 {
		t.Error("el reporte no quedó en la bitácora")
	}
	// Sin campos: vuelve el generador con el motivo, no un JSON.
	esperar(t, x.observador.get(base+"/reporte.csv?granularidad=postulacion&puntaje_min=x"), http.StatusUnprocessableEntity,
		"No se pudo generar el reporte", "Elige entre 1 y 120 campos", "Puntaje mínimo: El valor de este filtro no es válido.")

	// Plantillas: se guardan desde el mismo formulario, se cargan, se usan y se borran.
	esperar(t, x.observador.post(base+"/reporte/plantillas", url.Values{"nombre": {""}, "campos": {"folio"}}), http.StatusUnprocessableEntity,
		`name="campos" value="folio" checked`)
	redirige(t, x.observador.post(base+"/reporte/plantillas", url.Values{"nombre": {"Acta del observador"}, "campos": {"folio", "eval.lugar"},
		"orden": {"-puntaje"}, "separador": {"punto_y_coma"}}), base+"/reporte?aviso=plantilla#plantillas")
	redirige(t, x.admin.post(base+"/reporte/plantillas", url.Values{"nombre": {"Acta del administrador"}, "campos": {"folio"}}),
		base+"/reporte?aviso=plantilla#plantillas")
	plantillas, err := x.e.repo.PlantillasDeReporte(t.Context())
	if err != nil || len(plantillas) != 2 {
		t.Fatalf("plantillas: %v (%v)", plantillas, err)
	}
	var delObservador, delAdmin models.PlantillaDeReporte
	for _, p := range plantillas {
		if p.Nombre == "Acta del observador" {
			delObservador = p
		} else {
			delAdmin = p
		}
	}
	if !strings.Contains(delObservador.Consulta, "campos=folio%2Ceval.lugar") || strings.Contains(delObservador.Consulta, "nombre") {
		t.Errorf("consulta guardada: %q", delObservador.Consulta)
	}
	w = x.observador.get(fmt.Sprintf("%s/reporte?plantilla=%d", base, delObservador.ID))
	esperar(t, w, http.StatusOK, `value="eval.lugar" checked`, `<option value="punto_y_coma" selected>`, "Acta del observador", "Acta del administrador")
	w = x.observador.get(fmt.Sprintf("%s/reporte.csv?plantilla=%d", base, delObservador.ID))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Lugar;Folio") && !strings.Contains(w.Body.String(), "Folio;Lugar") {
		t.Errorf("reporte por plantilla: %d %s", w.Code, w.Body.String())
	}
	// Una plantilla de Intérpretes no se carga en Compositores.
	if w := x.admin.get(fmt.Sprintf("/supervision/convocatorias/%d/reporte?plantilla=%d", x.compositores, delObservador.ID)); w.Code != http.StatusNotFound {
		t.Errorf("plantilla de otro tipo: %d", w.Code)
	}
	// El observador no borra la del administrador; el administrador borra cualquiera.
	if w := x.observador.post(fmt.Sprintf("%s/reporte/plantillas/%d/borrar", base, delAdmin.ID), nil); w.Code != http.StatusNotFound {
		t.Errorf("el observador borró la plantilla del administrador: %d", w.Code)
	}
	redirige(t, x.observador.post(fmt.Sprintf("%s/reporte/plantillas/%d/borrar", base, delObservador.ID), nil), base+"/reporte?aviso=borrada#plantillas")
	redirige(t, x.admin.post(fmt.Sprintf("%s/reporte/plantillas/%d/borrar", base, delAdmin.ID), nil), base+"/reporte?aviso=borrada#plantillas")
}

func TestEstadisticasYNominaDesdeLasPantallas(t *testing.T) {
	x := supervisionEvaluada(t)
	base := fmt.Sprintf("/supervision/convocatorias/%d", x.interpretes)
	cfg, err := x.e.repo.ConfiguracionDeConvocatoria(t.Context(), x.interpretes)
	if err != nil {
		t.Fatal(err)
	}
	estadisticas := fmt.Sprintf("/supervision/ediciones/%d/estadisticas", cfg.Convocatoria.Edicion.ID)

	// Estadísticas de la edición y de una convocatoria.
	esperar(t, x.observador.get(estadisticas), http.StatusOK, "Postulaciones enviadas", "<dd>3</dd>", "Por convocatoria", "Intérpretes",
		"Compositores", "Jalisco", "Mujer", `<meter class="barras__barra"`)
	esperar(t, x.observador.get(fmt.Sprintf("%s?convocatoria=%d", estadisticas, x.interpretes)), http.StatusOK, "<dd>2</dd>",
		fmt.Sprintf(`<option value="%d" selected>`, x.interpretes))

	// La nómina: lo que saldría, y sólo con la convocatoria cerrada.
	luis := idDeRutaAPI(t, x.rutaLuis)
	redirige(t, x.admin.post(fmt.Sprintf("/supervision/postulaciones/%d/dictamen-final", luis), url.Values{"dictamen": {"seleccionada"}}),
		base+fmt.Sprintf("/resultados?aviso=dictamen#u-%d", luis))
	w := x.admin.get(base + "/nomina")
	esperar(t, w, http.StatusOK, "Lo que saldría al portal", "Ana L.", "Semblanza de Ana.", "sólo puede publicarse con la convocatoria cerrada")
	noAparece(t, w, "ana@ejemplo.mx", "55 1234 5678", "OBS-J1", `name="confirmo"`)
	esperar(t, x.admin.post(base+"/nomina", url.Values{"accion": {"publicar"}, "confirmo": {"1"}}), http.StatusUnprocessableEntity)

	x.admin.estado(x.interpretes, "cerrada", "")
	esperar(t, x.admin.get(base+"/nomina"), http.StatusOK, "Confirmo que quiero publicar esta nómina de 1 seleccionados", "Aún no se ha publicado nada")
	esperar(t, x.admin.post(base+"/nomina", url.Values{"accion": {"publicar"}}), http.StatusUnprocessableEntity)
	if publicadas := x.e.asientos("interop.publicar"); len(publicadas) != 0 {
		t.Errorf("sin confirmar no se publica: %d", len(publicadas))
	}
	redirige(t, x.admin.post(base+"/nomina", url.Values{"accion": {"publicar"}, "confirmo": {"1"}}), base+"/nomina?aviso=publicada")
	esperar(t, x.admin.get(base+"/nomina?aviso=publicada"), http.StatusOK, "Nómina publicada en el portal", "1 seleccionados", "Retirar del portal")
	_, portal := x.admin.json("/api/v1/interop/convocatoria.json")
	if !strings.Contains(fmt.Sprint(portal), "Ana L.") {
		t.Errorf("la nómina no llegó al portal: %v", portal)
	}
	redirige(t, x.admin.post(base+"/nomina", url.Values{"accion": {"retirar"}, "confirmo": {"1"}}), base+"/nomina?aviso=retirada")
	esperar(t, x.admin.get(base+"/nomina"), http.StatusOK, "Retirada del portal")
	if _, portal = x.admin.json("/api/v1/interop/convocatoria.json"); strings.Contains(fmt.Sprint(portal), "Ana L.") {
		t.Errorf("la nómina retirada sigue en el portal")
	}
}
