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
	"strconv"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// API REST (/api/v1). Responde JSON; los errores llevan un código estable y
// un mensaje en el idioma de la petición. La autenticación es la misma cookie
// de sesión que usan las páginas.

// Paginación de los listados de la API.
const (
	tamanoPaginaPorOmision = 50
	tamanoPaginaMaximo     = 200
	// filasMaximasCSV acota una exportación para que no agote la memoria.
	filasMaximasCSV = 50000
)

// apiOpenAPI entrega la descripción de la API en formato OpenAPI 3.1. Es
// pública: describe el contrato, no contiene datos.
//
//	GET /api/v1/openapi.json
func (a *App) apiOpenAPI(w http.ResponseWriter, r *http.Request) {
	documento, err := views.OpenAPI()
	if err != nil {
		a.log.Error("leer la descripción OpenAPI", "error", err)
		a.errorAPI(w, r, http.StatusInternalServerError, "error_interno")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(documento)
}

// responderJSON escribe una respuesta correcta de la API.
func (a *App) responderJSON(w http.ResponseWriter, datos any) {
	if err := views.JSON(w, http.StatusOK, datos); err != nil {
		a.log.Error("no se pudo escribir la respuesta JSON", "error", err)
	}
}

// apiSesion describe la sesión en curso: la cuenta, sus perfiles globales y
// sus asignaciones por convocatoria.
//
//	GET /api/v1/sesion
func (a *App) apiSesion(w http.ResponseWriter, r *http.Request) {
	a.responderJSON(w, views.SesionJSON(sesionDe(r)))
}

// fechaDeFiltro interpreta una fecha de la URL. Admite un instante completo
// (2027-03-14T18:05:00Z) o un día (2027-03-14), que se entiende como ese día
// en la Ciudad de México. Con finDelDia, un día se convierte en el inicio del
// siguiente, para usarlo como cota superior exclusiva.
func fechaDeFiltro(texto string, finDelDia bool) (time.Time, bool) {
	if texto == "" {
		return time.Time{}, true
	}
	if instante, err := time.Parse(time.RFC3339, texto); err == nil {
		return instante, true
	}
	dia, err := time.ParseInLocation("2006-01-02", texto, views.ZonaMexico())
	if err != nil {
		return time.Time{}, false
	}
	if finDelDia {
		dia = dia.AddDate(0, 0, 1)
	}
	return dia, true
}

// enteroDeFiltro interpreta un parámetro entero no negativo; vacío es 0.
func enteroDeFiltro(texto string) (int64, bool) {
	if texto == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(texto, 10, 64)
	return n, err == nil && n >= 0
}

// apiAuditoria consulta la bitácora (RF-AUD-03). Sólo para admin_sistema y
// coordinacion_nacional.
//
//	GET /api/v1/auditoria?accion=&entidad=&entidad_id=&actor_id=&desde=&hasta=&pagina=&tamano=
//	GET /api/v1/auditoria?…&formato=csv      descarga el resultado completo
func (a *App) apiAuditoria(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeConsultarAuditoria(sesionDe(r)) {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return
	}
	consulta := r.URL.Query()
	filtro := models.FiltroAuditoria{
		Accion:  consulta.Get("accion"),
		Entidad: consulta.Get("entidad"),
		Pagina:  1,
		Tamano:  tamanoPaginaPorOmision,
	}
	invalido := func(parametro string) {
		a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", parametro)
	}
	var ok bool
	if filtro.EntidadID, ok = enteroDeFiltro(consulta.Get("entidad_id")); !ok {
		invalido("entidad_id")
		return
	}
	if filtro.ActorID, ok = enteroDeFiltro(consulta.Get("actor_id")); !ok {
		invalido("actor_id")
		return
	}
	if filtro.Desde, ok = fechaDeFiltro(consulta.Get("desde"), false); !ok {
		invalido("desde")
		return
	}
	if filtro.Hasta, ok = fechaDeFiltro(consulta.Get("hasta"), true); !ok {
		invalido("hasta")
		return
	}
	if texto := consulta.Get("pagina"); texto != "" {
		n, ok := enteroDeFiltro(texto)
		if !ok || n < 1 {
			invalido("pagina")
			return
		}
		filtro.Pagina = int(n)
	}
	if texto := consulta.Get("tamano"); texto != "" {
		n, ok := enteroDeFiltro(texto)
		if !ok || n < 1 || n > tamanoPaginaMaximo {
			invalido("tamano")
			return
		}
		filtro.Tamano = int(n)
	}

	formato := consulta.Get("formato")
	switch formato {
	case "", "json":
	case "csv":
		// La exportación respeta el filtro e ignora la paginación.
		filtro.Pagina, filtro.Tamano = 1, filasMaximasCSV
	default:
		invalido("formato")
		return
	}

	asientos, total, err := a.datos.ConsultarAuditoria(r.Context(), filtro)
	if err != nil {
		a.log.Error("consultar bitácora", "error", err)
		a.errorAPI(w, r, http.StatusInternalServerError, "error_interno")
		return
	}
	if formato == "csv" {
		encabezados, filas := views.AuditoriaCSV(asientos)
		if err := views.CSV(w, "auditoria.csv", encabezados, filas); err != nil {
			a.log.Error("no se pudo escribir el CSV", "error", err)
		}
		return
	}
	a.responderJSON(w, views.AuditoriaJSON(asientos, total, filtro.Pagina, filtro.Tamano))
}
