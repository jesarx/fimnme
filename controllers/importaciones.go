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
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Importación asistida con interacción humana (§5.15), para el administrador.
//
// Dos fases. Subir la hoja la ANALIZA: no escribe nada en el dominio y
// devuelve un conflicto por cada renglón que una persona debe resolver. Con
// todos resueltos se CONFIRMA, y entonces entra todo en una sola transacción
// o no entra nada. El motor es el mismo que usa fimnme-cli (RF-IMP-05).

// falloDeHoja responde cuando el archivo no pudo leerse como hoja de cálculo.
// Devuelve true si respondió.
func (a *App) falloDeHoja(w http.ResponseWriter, r *http.Request, err error) bool {
	if !errors.Is(err, models.ErrHojaIlegible) {
		return a.falloDatos(w, r, err)
	}
	a.log.Info("hoja de importación ilegible", "error", err)
	return a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "error.archivo_ilegible"}}})
}

// responderImportacion responde con el estado de una importación.
func (a *App) responderImportacion(w http.ResponseWriter, r *http.Request, estado int, id int64) {
	importacion, err := a.datos.ImportacionPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, estado, views.ImportacionJSON(importacion))
}

// POST /api/v1/importaciones   — fase Analyze
//
// Formulario multipart, con los campos antes del archivo: «tipo»
// (dotaciones, usuarios_jurado o usuarios_revisor), «convocatoria_id» y
// «archivo» (.csv o .xlsx, hasta 2 MB). Responde 201 con la importación y sus
// conflictos. No escribe nada en el dominio.
func (a *App) apiAnalizarImportacion(w http.ResponseWriter, r *http.Request) {
	id, err := a.analizarImportacion(w, r)
	if errors.Is(err, errSubidaInvalida) {
		a.falloDeSubida(w, r, err)
		return
	}
	if a.falloDeHoja(w, r, err) {
		return
	}
	a.responderImportacion(w, r, http.StatusCreated, id)
}

// analizarImportacion lee la hoja de una petición multipart (tipo,
// convocatoria_id, archivo) y la analiza. La comparten la API y la pantalla.
// Una hoja que no se puede leer devuelve models.ErrHojaIlegible; una subida
// cortada, errSubidaInvalida.
func (a *App) analizarImportacion(w http.ResponseWriter, r *http.Request) (int64, error) {
	subida, err := a.abrirSubida(w, r, models.TamanoMaxImportacion)
	if err != nil {
		return 0, err
	}
	convocatoriaID, err := strconv.ParseInt(subida.campos["convocatoria_id"], 10, 64)
	if err != nil || convocatoriaID < 1 {
		return 0, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "convocatoria_id", Clave: "error.convocatoria_requerida"}}}
	}
	contenido, err := io.ReadAll(io.LimitReader(subida.cuerpo, models.TamanoMaxImportacion+1))
	if err != nil {
		return 0, errSubidaInvalida
	}
	if len(contenido) > models.TamanoMaxImportacion {
		return 0, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "error.archivo_grande"}}}
	}
	return models.AnalizarImportacion(r.Context(), a.datos, a.almacen, a.actorDe(r), subida.campos["tipo"], convocatoriaID, subida.nombre, contenido)
}

// GET /api/v1/importaciones/{id}   — estado de una importación
//
// Sus conflictos, con los candidatos de cada uno y cómo se resolvió, y el
// efecto que tendrá confirmarla: cuántos renglones crean, cuántos vinculan y
// cuántos se omiten (RF-IMP-08).
func (a *App) apiImportacion(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.responderImportacion(w, r, http.StatusOK, id)
	}
}

// PUT /api/v1/importaciones/{id}/conflictos/{conflicto}   — resuelve un conflicto
//
// Cuerpo: {"resolucion": "vincular"|"crear_nuevo"|"omitir", "entidad_id"}.
// «entidad_id» es el registro elegido cuando se vincula. Queda registrado
// quién lo resolvió y cuándo. Puede cambiarse mientras no se confirme.
func (a *App) apiResolverConflicto(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	conflictoID, err := strconv.ParseInt(r.PathValue("conflicto"), 10, 64)
	var entrada struct {
		Resolucion string `json:"resolucion"`
		EntidadID  int64  `json:"entidad_id"`
	}
	if !ok {
		return
	}
	if err != nil || conflictoID < 1 {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	if a.falloDatos(w, r, a.datos.ResolverConflicto(r.Context(), a.actorDe(r), id, conflictoID, entrada.Resolucion, entrada.EntidadID)) {
		return
	}
	a.responderImportacion(w, r, http.StatusOK, id)
}

// POST /api/v1/importaciones/{id}/resoluciones   — resuelve los que no tienen candidatos
//
// Cuerpo: {"resolucion": "crear_nuevo"|"omitir"}. Aplica esa decisión a los
// conflictos pendientes que no se parecen a nada de lo que existe. Los que
// tienen candidatos se resuelven de uno en uno.
func (a *App) apiResolverSinCandidatos(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Resolucion string `json:"resolucion"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if _, err := a.datos.ResolverSinCandidatos(r.Context(), a.actorDe(r), id, entrada.Resolucion); a.falloDatos(w, r, err) {
		return
	}
	a.responderImportacion(w, r, http.StatusOK, id)
}

// POST /api/v1/importaciones/{id}/confirmacion   — fase Commit
//
// Cuerpo: {"confirmo": true}. Sólo procede con todos los conflictos
// resueltos. Aplica la importación en una sola transacción: si un renglón no
// puede entrar, no entra ninguno y la respuesta dice cuál. Las cuentas nuevas
// reciben por correo su enlace de activación.
func (a *App) apiConfirmarImportacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Confirmo bool `json:"confirmo"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if !entrada.Confirmo {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "confirmo", Clave: "error.confirmacion_requerida"}}})
		return
	}
	if a.falloDeHoja(w, r, a.confirmarImportacion(r, id)) {
		return
	}
	a.responderImportacion(w, r, http.StatusOK, id)
}

// confirmarImportacion aplica una importación (fase Commit). Cada cuenta
// nueva necesita su enlace de activación y el correo que lo lleva: se
// preparan aquí y se encolan dentro de la misma transacción. La comparten la
// API y la pantalla.
func (a *App) confirmarImportacion(r *http.Request, id int64) error {
	importacion, err := a.datos.ImportacionPorID(r.Context(), id)
	if err != nil {
		return err
	}
	filas, err := models.FilasDeLaImportacion(a.almacen, importacion)
	if err != nil {
		return err
	}
	activaciones := make(map[int]models.ActivacionDeCuenta)
	for _, fila := range models.CuentasPorCrear(importacion, filas) {
		token, correo, err := a.correoDeActivacion(r, fila.Valores["email"], fila.Valores["nombre"], fila.Valores["idioma"])
		if err != nil {
			return err
		}
		activaciones[fila.Numero] = models.ActivacionDeCuenta{Token: token, Correo: &correo}
	}
	_, err = a.datos.ConfirmarImportacion(r.Context(), a.actorDe(r), id, filas, activaciones)
	return err
}

// DELETE /api/v1/importaciones/{id}   — cancela una importación sin aplicarla
func (a *App) apiCancelarImportacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	if a.falloDatos(w, r, a.datos.CancelarImportacion(r.Context(), a.actorDe(r), id)) {
		return
	}
	a.responderImportacion(w, r, http.StatusOK, id)
}
