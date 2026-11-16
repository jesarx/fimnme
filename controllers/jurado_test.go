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
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de las pantallas del jurado (paso 12b): quién entra, la lista de
// trabajo, la ficha con su calificación y su excusa, y que ninguna de estas
// páginas deje ver a un jurado lo que escribió otro.

// idDeRutaAPI saca el id del final de una ruta de la API («/api/v1/postulaciones/7»).
func idDeRutaAPI(t *testing.T, ruta string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(ruta[strings.LastIndex(ruta, "/")+1:], "%d", &id); err != nil {
		t.Fatalf("ruta sin id: %s", ruta)
	}
	return id
}

func TestPantallasDelJurado(t *testing.T) {
	x := hastaEvaluacion(t)
	x.admin.estado(x.interpretes, "evaluacion", "")
	j1, j2 := x.juradosInt[0], x.juradosInt[1]
	luis, ana := idDeRutaAPI(t, x.rutaLuis), idDeRutaAPI(t, x.rutaAna)
	ficha := fmt.Sprintf("/evaluacion/postulaciones/%d", luis)
	lista := fmt.Sprintf("/evaluacion/convocatorias/%d", x.interpretes)

	// El menú ofrece la evaluación sólo a los jurados.
	esperar(t, j1.get("/postulaciones"), http.StatusOK, `href="/evaluacion"`)
	noAparece(t, x.revisora.get("/postulaciones"), `href="/evaluacion"`)
	for nombre, c := range map[string]*cliente{"revisora": x.revisora, "postulante": x.luis, "observador": x.observador} {
		if w := c.get("/evaluacion"); w.Code != http.StatusForbidden {
			t.Errorf("%s en /evaluacion: estado %d", nombre, w.Code)
		}
		if w := c.get(ficha); w.Code != http.StatusNotFound {
			t.Errorf("%s en la ficha: estado %d", nombre, w.Code)
		}
	}
	esperar(t, j1.get("/evaluacion"), http.StatusOK, "Convocatoria de Intérpretes 2028", "Has evaluado 0 de 1")

	// La lista: sólo la validada; la descartada no existe para el jurado.
	w := j1.get(lista)
	esperar(t, w, http.StatusOK, "FIM50-INT-0001", "Sin empezar", "Siguiente por evaluar", `href="`+ficha+`"`)
	if w := j1.get(fmt.Sprintf("/evaluacion/postulaciones/%d", ana)); w.Code != http.StatusNotFound {
		t.Errorf("la descartada en la ficha: estado %d", w.Code)
	}
	if w := x.juradosCom[0].get(ficha); w.Code != http.StatusNotFound {
		t.Errorf("un jurado de otra convocatoria en la ficha: estado %d", w.Code)
	}
	if w := x.juradosCom[0].get(lista); w.Code != http.StatusNotFound {
		t.Errorf("un jurado de otra convocatoria en la lista: estado %d", w.Code)
	}

	// La ficha: la postulación con sus documentos en el visor, y la escala.
	w = j1.get(ficha)
	esperar(t, w, http.StatusOK, "Ana López Ruiz", "Preludio a Colón", `class="visor__marco"`, `name="puntaje_realizacion" value="5"`,
		"Guardar borrador", "Enviar evaluación", "Excusarme de calificar esto", "1 de 1 en tu lista")
	// Nada de la revisión documental.
	noAparece(t, w, "Dictamen", "Observaciones internas", "luis@ejemplo.mx")

	// Borrador incompleto: se guarda. Enviar incompleto o fuera de escala: no.
	redirige(t, j1.post(ficha+"/evaluacion", url.Values{"accion": {"borrador"}, "puntaje_realizacion": {"4"}, "observaciones": {"Buen programa."}}),
		ficha+"?aviso=borrador#calificacion")
	esperar(t, j1.get(ficha+"?aviso=borrador"), http.StatusOK, "Borrador guardado", `value="4" checked`, "Buen programa.")
	w = j1.post(ficha+"/evaluacion", url.Values{"accion": {"enviar"}, "puntaje_realizacion": {"4"}})
	esperar(t, w, http.StatusUnprocessableEntity, "La evaluación no se guardó", `id="ce-artistico">`)
	esperar(t, j1.post(ficha+"/evaluacion", url.Values{"accion": {"enviar"}, "puntaje_realizacion": {"9"}, "puntaje_artistico": {"3"}}),
		http.StatusUnprocessableEntity, `criterio criterio--error`)
	evaluacion, err := x.e.repo.ParaJurado(x.idsInt[0]).Evaluacion(context.Background(), luis, 0)
	if err != nil || evaluacion.Enviada || evaluacion.Puntajes["realizacion"] != 4 {
		t.Fatalf("tras los rechazos debe quedar el borrador: %+v (%v)", evaluacion, err)
	}
	redirige(t, j1.post(ficha+"/evaluacion", url.Values{"accion": {"enviar"}, "puntaje_realizacion": {"4"}, "puntaje_artistico": {"5"}, "observaciones": {"SECRETO-J1"}}),
		ficha+"?aviso=enviada#calificacion")
	if evaluacion, _ = x.e.repo.ParaJurado(x.idsInt[0]).Evaluacion(context.Background(), luis, 0); !evaluacion.Enviada || evaluacion.Puntajes["artistico"] != 5 {
		t.Fatalf("tras enviar: %+v", evaluacion)
	}
	esperar(t, j1.get(lista), http.StatusOK, "Has evaluado 1 de 1", "Enviaste todas tus evaluaciones", "Enviada")
	esperar(t, j1.get(ficha), http.StatusOK, "Guardar cambios", "Puedes modificarla hasta que cierre la etapa")

	// La segunda jurado no ve nada de la primera, y escribe lo suyo.
	w = j2.get(ficha)
	noAparece(t, w, "SECRETO-J1", "checked")
	redirige(t, j2.post(ficha+"/evaluacion", url.Values{"accion": {"enviar"}, "puntaje_realizacion": {"2"}, "puntaje_artistico": {"2"}, "observaciones": {"SECRETO-J2"}}),
		ficha+"?aviso=enviada#calificacion")
	for nombre, caso := range map[string]struct {
		c             *cliente
		propia, ajena string
	}{"j1": {j1, "SECRETO-J1", "SECRETO-J2"}, "j2": {j2, "SECRETO-J2", "SECRETO-J1"}} {
		vistoLoPropio := false
		for _, ruta := range []string{"/evaluacion", lista, ficha, ficha + "?aviso=enviada", ficha + "?obra=0", ficha + "?jurado=1"} {
			w := caso.c.get(ruta)
			if strings.Contains(w.Body.String(), caso.ajena) {
				t.Errorf("FUGA: %s vio %q en %s", nombre, caso.ajena, ruta)
			}
			vistoLoPropio = vistoLoPropio || strings.Contains(w.Body.String(), caso.propia)
		}
		if !vistoLoPropio {
			t.Errorf("%s debía ver su propia observación: la prueba no tendría con qué fallar", nombre)
		}
	}

	// Al cerrar la etapa se consulta, pero no se modifica.
	x.admin.estado(x.interpretes, "cerrada", "")
	w = j1.get(ficha)
	esperar(t, w, http.StatusOK, "no está en etapa de evaluación", "SECRETO-J1", "disabled")
	noAparece(t, w, "Guardar cambios", "Excusarme")
	esperar(t, j1.post(ficha+"/evaluacion", url.Values{"accion": {"enviar"}, "puntaje_realizacion": {"1"}, "puntaje_artistico": {"1"}}), http.StatusUnprocessableEntity)
}

func TestFichaPorObraYExcusa(t *testing.T) {
	x := hastaEvaluacion(t)
	x.admin.estado(x.compositores, "evaluacion", "")
	jc := x.juradosCom[0]
	eva := idDeRutaAPI(t, x.rutaEva)
	obra1, obra2 := x.obrasDeEva[0], x.obrasDeEva[1]
	ficha1 := fmt.Sprintf("/evaluacion/postulaciones/%d?obra=%d", eva, obra1)

	// En Compositores cada obra es una unidad: la lista tiene dos, y la ficha
	// muestra sólo la obra que se califica.
	esperar(t, jc.get(fmt.Sprintf("/evaluacion/convocatorias/%d", x.compositores)), http.StatusOK, "Cuarteto n.º 1", "Cuarteto n.º 2", "Has evaluado 0 de 2")
	w := jc.get(ficha1)
	esperar(t, w, http.StatusOK, "Cuarteto n.º 1", "1 de 2 en tu lista", fmt.Sprintf(`name="obra" value="%d"`, obra1), "Particellas")
	noAparece(t, w, "Cuarteto n.º 2")
	if w := jc.get(fmt.Sprintf("/evaluacion/postulaciones/%d", eva)); w.Code != http.StatusNotFound {
		t.Errorf("sin obra no hay unidad en Compositores: estado %d", w.Code)
	}
	if w := jc.get(fmt.Sprintf("/evaluacion/postulaciones/%d?obra=99999", eva)); w.Code != http.StatusNotFound {
		t.Errorf("una obra que no es de la postulación: estado %d", w.Code)
	}

	// Excusa de una obra: exige motivo; no toca la otra; se retira.
	base := fmt.Sprintf("/evaluacion/postulaciones/%d", eva)
	esperar(t, jc.post(base+"/excusa", url.Values{"obra": {fmt.Sprint(obra1)}}), http.StatusUnprocessableEntity, `role="alert"`)
	redirige(t, jc.post(base+"/excusa", url.Values{"obra": {fmt.Sprint(obra1)}, "motivo": {"Fui maestro del compositor."}}), ficha1+"&aviso=excusa#excusa")
	w = jc.get(ficha1)
	esperar(t, w, http.StatusOK, "Te excusaste de calificar esto", "Fui maestro del compositor.", "Retirar la excusa")
	noAparece(t, w, `name="puntaje_realizacion"`)
	esperar(t, jc.get(fmt.Sprintf("/evaluacion/convocatorias/%d", x.compositores)), http.StatusOK, "Te excusaste de 1", "Te excusaste", "Sin empezar")
	redirige(t, jc.post(base+"/excusa/retirar", url.Values{"obra": {fmt.Sprint(obra1)}}), ficha1+"&aviso=retirada#excusa")
	esperar(t, jc.get(ficha1), http.StatusOK, `name="puntaje_realizacion"`)

	// Calificar la segunda obra por su ficha.
	ficha2 := fmt.Sprintf("%s?obra=%d", base, obra2)
	redirige(t, jc.post(base+"/evaluacion", url.Values{"obra": {fmt.Sprint(obra2)}, "accion": {"enviar"}, "puntaje_realizacion": {"3"}, "puntaje_artistico": {"4"}}),
		ficha2+"&aviso=enviada#calificacion")
	evaluacion, err := x.e.repo.ParaJurado(idDeJurado(t, x, jc)).Evaluacion(context.Background(), eva, obra2)
	if err != nil || !evaluacion.Enviada || evaluacion.Puntajes["artistico"] != 4 {
		t.Errorf("evaluación de la obra 2: %+v (%v)", evaluacion, err)
	}
	if e1, _ := x.e.repo.ParaJurado(idDeJurado(t, x, jc)).Evaluacion(context.Background(), eva, obra1); e1.Enviada {
		t.Error("calificar la obra 2 no debe tocar la obra 1")
	}
}

// idDeJurado busca el id de cuenta de un jurado de Compositores por su sesión.
func idDeJurado(t *testing.T, x *evaluacionDePrueba, c *cliente) int64 {
	t.Helper()
	s, err := x.e.repo.SesionPorID(context.Background(), models.ResumenDeSecreto(c.cookies[cookieSesion]))
	if err != nil {
		t.Fatalf("sesión del jurado: %v", err)
	}
	return s.Usuario.ID
}
