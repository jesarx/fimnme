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
	"net/http"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// API de la evaluación por jurado (§5.6).
//
// AISLAMIENTO ENTRE JURADOS. Los manejadores de este archivo que atienden a
// un jurado sólo usan a.datos.ParaJurado(id de la sesión), que no tiene modo
// de devolver nada de otro jurado. La matriz, que sí cruza jurados, sólo la
// alcanza apiResultados, que va envuelto en a.supervisa. Ningún manejador
// usa las dos cosas.

// juradoDe comprueba que la sesión sea de un jurado de la convocatoria y
// devuelve su acceso a las evaluaciones: el suyo, y sólo el suyo.
func (a *App) juradoDe(w http.ResponseWriter, r *http.Request, convocatoriaID int64) (models.JuradoRepo, bool) {
	sesion := sesionDe(r)
	if !models.PuedeEvaluar(sesion, convocatoriaID) {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return models.JuradoRepo{}, false
	}
	return a.datos.ParaJurado(sesion.Usuario.ID), true
}

// GET /api/v1/convocatorias/{id}/evaluacion   — lista de trabajo del jurado
func (a *App) apiListaDeJurado(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	jurado, ok := a.juradoDe(w, r, convocatoriaID)
	if !ok {
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	unidades, err := jurado.Unidades(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.ListaDeJuradoJSON(unidades, cfg.Criterios, cfg.Convocatoria.Estado == models.EstadoEvaluacion))
}

// unidadDeLaPeticion lee la postulación de la ruta y comprueba que la sesión
// sea de un jurado de su convocatoria y que la postulación esté a su vista.
func (a *App) unidadDeLaPeticion(w http.ResponseWriter, r *http.Request) (models.JuradoRepo, int64, bool) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return models.JuradoRepo{}, 0, false
	}
	p, ok := a.postulacionVisible(w, r, id)
	if !ok {
		return models.JuradoRepo{}, 0, false
	}
	jurado, ok := a.juradoDe(w, r, p.ConvocatoriaID)
	return jurado, id, ok
}

// GET /api/v1/postulaciones/{id}/evaluacion   — la evaluación propia de una unidad
//
// En Compositores, ?obra_id= indica la obra.
func (a *App) apiEvaluacionPropia(w http.ResponseWriter, r *http.Request) {
	jurado, id, ok := a.unidadDeLaPeticion(w, r)
	if !ok {
		return
	}
	obraID, err := enteroOpcional(r.URL.Query().Get("obra_id"))
	if err != nil {
		a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", "obra_id")
		return
	}
	evaluacion, err := jurado.Evaluacion(r.Context(), id, obraID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.EvaluacionJSON(evaluacion))
}

// PUT /api/v1/postulaciones/{id}/evaluacion   — guarda la evaluación propia
//
// Cuerpo: {"obra_id", "puntajes": {"realizacion": 4, "artistico": 5},
// "observaciones", "enviar"}. Sin «enviar» queda en borrador y puede estar
// incompleta.
func (a *App) apiGuardarEvaluacion(w http.ResponseWriter, r *http.Request) {
	var entrada struct {
		ObraID        int64          `json:"obra_id"`
		Puntajes      map[string]int `json:"puntajes"`
		Observaciones string         `json:"observaciones"`
		Enviar        bool           `json:"enviar"`
	}
	// El cuerpo se lee antes de mirar la postulación: un JSON mal formado es
	// un 400 para cualquiera.
	if _, ok := a.idDeRuta(w, r); !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	jurado, id, ok := a.unidadDeLaPeticion(w, r)
	if !ok {
		return
	}
	evaluacion, err := jurado.Guardar(r.Context(), a.actorDe(r), id, models.EntradaEvaluacion{
		ObraID: entrada.ObraID, Puntajes: entrada.Puntajes, Observaciones: entrada.Observaciones, Enviar: entrada.Enviar,
	})
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.EvaluacionJSON(evaluacion))
}

// PUT /api/v1/postulaciones/{id}/excusa   — el jurado se excusa por conflicto de interés
//
// Cuerpo: {"obra_id", "motivo"}. En Compositores, sin obra_id se excusa de
// todas las obras de la postulación. La excusa no perjudica a la postulación:
// su puntaje se promedia entre los jurados que sí la calificaron.
func (a *App) apiExcusarse(w http.ResponseWriter, r *http.Request) {
	var entrada struct {
		ObraID int64  `json:"obra_id"`
		Motivo string `json:"motivo"`
	}
	if _, ok := a.idDeRuta(w, r); !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	jurado, id, ok := a.unidadDeLaPeticion(w, r)
	if !ok {
		return
	}
	if a.falloDatos(w, r, jurado.Excusarse(r.Context(), a.actorDe(r), id, entrada.ObraID, entrada.Motivo)) {
		return
	}
	a.responderEvaluacionPropia(w, r, jurado, id, entrada.ObraID)
}

// DELETE /api/v1/postulaciones/{id}/excusa   — retira la excusa
//
// En Compositores, ?obra_id= indica la obra; sin él, todas.
func (a *App) apiRetirarExcusa(w http.ResponseWriter, r *http.Request) {
	jurado, id, ok := a.unidadDeLaPeticion(w, r)
	if !ok {
		return
	}
	obraID, err := enteroOpcional(r.URL.Query().Get("obra_id"))
	if err != nil {
		a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", "obra_id")
		return
	}
	if a.falloDatos(w, r, jurado.RetirarExcusa(r.Context(), a.actorDe(r), id, obraID)) {
		return
	}
	a.responderEvaluacionPropia(w, r, jurado, id, obraID)
}

// responderEvaluacionPropia responde con la evaluación del jurado a una
// unidad. Si la excusa cubrió todas las obras de una postulación de
// Compositores (obraID 0), responde con la de la primera obra.
func (a *App) responderEvaluacionPropia(w http.ResponseWriter, r *http.Request, jurado models.JuradoRepo, postulacionID, obraID int64) {
	if obraID == 0 {
		if p, err := a.datos.PostulacionPorID(r.Context(), postulacionID); err == nil {
			if cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), p.ConvocatoriaID); err == nil {
				obraID = models.UnidadesDeEvaluacion(cfg.Convocatoria.Tipo, p)[0]
			}
		}
	}
	evaluacion, err := jurado.Evaluacion(r.Context(), postulacionID, obraID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.EvaluacionJSON(evaluacion))
}

// GET /api/v1/convocatorias/{id}/resultados   — matriz de resultados (supervisión)
//
// Puntaje por jurado y criterio, suma de los tres, promedio y orden por
// total. Sólo administrador y observador; cada consulta queda en la bitácora.
func (a *App) apiResultados(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	matriz, err := a.datos.Supervision().Matriz(r.Context(), a.actorDe(r), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.MatrizJSON(matriz))
}

// PUT /api/v1/postulaciones/{id}/dictamen-final   — resultado de la postulación
//
// Cuerpo: {"dictamen": "seleccionada"|"no_seleccionada"|"lista_espera"|""}.
// Sólo el administrador. No se comunica al postulante.
func (a *App) apiDictamenFinal(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Dictamen string `json:"dictamen"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if a.falloDatos(w, r, a.datos.RegistrarDictamenFinal(r.Context(), a.actorDe(r), id, entrada.Dictamen)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}
