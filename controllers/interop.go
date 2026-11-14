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
	"encoding/json"
	"net/http"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Interoperabilidad con el portal público (§5.16) y anonimización ARCO.
//
// El portal público es un sitio estático aparte, que no consulta la base ni
// comparte red con ella: se alimenta de la exportación JSON de aquí. Esa
// exportación sólo puede contener lo que cabe en los tipos models.Interop*,
// que son la lista blanca (RF-PUB-02). La nómina de seleccionados no se
// calcula al exportar: sale la última que un administrador PUBLICÓ, después
// del cierre y por una acción explícita (RF-PUB-04).

// GET /api/v1/interop/convocatoria.json   — exportación para el portal público
//
// Pública, sin sesión: todo lo que lleva es público por definición. Con
// «edicion» (el número), sólo esa edición.
func (a *App) apiInterop(w http.ResponseWriter, r *http.Request) {
	numero, err := enteroOpcional(r.URL.Query().Get("edicion"))
	if err != nil {
		a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", "edicion")
		return
	}
	portal, err := a.datos.PortalPublico(r.Context(), a.almacen, int(numero))
	if a.falloDatos(w, r, err) {
		return
	}
	// El generador del sitio la descarga al construir: puede guardarse unos
	// minutos, no más, para que una nómina retirada deje de servirse pronto.
	w.Header().Set("Cache-Control", "public, max-age=300")
	a.responder(w, http.StatusOK, portal)
}

// GET /api/v1/convocatorias/{id}/publicacion   — vista previa de la nómina
//
// Lo que se publicaría ahora mismo —la nómina tal como está hoy— y lo que ya
// se publicó. Sirve para ver exactamente qué sale antes de publicarlo
// (RF-PUB-07).
func (a *App) apiPublicacion(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	seleccionados, err := a.datos.SeleccionadosDe(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	publicaciones, _, err := a.datos.PublicacionesDe(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.PublicacionJSON(models.PuedePublicarNomina(cfg.Convocatoria), seleccionados, publicaciones))
}

// POST /api/v1/convocatorias/{id}/publicacion   — publica o retira la nómina
//
// Cuerpo: {"confirmo": true} publica la nómina de seleccionados tal como
// está en este momento; {"confirmo": true, "retirar": true} la retira del
// portal. Sólo el administrador y sólo con la convocatoria cerrada. Lo
// publicado queda congelado: un cambio posterior de dictamen no sale hasta
// que se vuelva a publicar. Queda registrado con su SHA-256.
func (a *App) apiPublicarNomina(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	var entrada struct {
		Confirmo bool `json:"confirmo"`
		Retirar  bool `json:"retirar"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if err == nil {
		// Se valida antes de escribir nada en el almacén; el repositorio lo
		// vuelve a validar dentro de la transacción.
		err = erroresDe(models.ValidarPublicacionDeNomina(cfg.Convocatoria, entrada.Confirmo))
	}
	if a.falloDatos(w, r, err) {
		return
	}
	nomina := models.InteropNomina{PublicadaAt: a.ahora().UTC().Format(time.RFC3339), Seleccionados: []models.InteropSeleccionado{}}
	if !entrada.Retirar {
		if nomina.Seleccionados, err = a.datos.SeleccionadosDe(r.Context(), convocatoriaID); a.falloDatos(w, r, err) {
			return
		}
	}
	contenido, err := json.Marshal(nomina)
	if err != nil {
		a.log.Error("no se pudo componer la nómina", "error", err)
		a.errorAPI(w, r, http.StatusInternalServerError, "error_interno")
		return
	}
	blob, err := a.almacen.Guardar(bytes.NewReader(contenido), int64(len(contenido)))
	if a.falloDatos(w, r, err) {
		return
	}
	if a.falloDatos(w, r, a.datos.PublicarNomina(r.Context(), a.actorDe(r), convocatoriaID, blob, len(nomina.Seleccionados), entrada.Confirmo)) {
		return
	}
	a.apiPublicacion(w, r)
}

// erroresDe convierte una lista de errores de validación en un error, o nil
// si la lista está vacía.
func erroresDe(errores []models.ErrorCampo) error {
	if len(errores) == 0 {
		return nil
	}
	return &models.ErrorValidacion{Errores: errores}
}

// POST /api/v1/postulaciones/{id}/anonimizacion   — anonimiza una postulación (ARCO)
//
// Cuerpo: {"folio", "fundamento", "confirmo", "con_cuenta"}. Borra los datos
// personales de la postulación conservando obra, dictamen y estadísticas. Es
// la única operación destructiva del sistema (RN-28): sólo el administrador,
// con confirmación doble —marcar «confirmo» y escribir de nuevo el folio— y
// con el fundamento de la solicitud, que queda en la bitácora. No se puede
// deshacer.
func (a *App) apiAnonimizar(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Folio      string `json:"folio"`
		Fundamento string `json:"fundamento"`
		Confirmo   bool   `json:"confirmo"`
		ConCuenta  bool   `json:"con_cuenta"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	resultado, err := a.datos.AnonimizarPostulacion(r.Context(), a.actorDe(r), id, models.SolicitudDeAnonimizacion{
		Folio: entrada.Folio, Fundamento: entrada.Fundamento, Confirmo: entrada.Confirmo, ConCuenta: entrada.ConCuenta})
	if a.falloDatos(w, r, err) {
		return
	}
	// Los archivos ya están marcados como purgados; ahora se borran del
	// disco. Si alguno no puede borrarse se dice: «fimnme-cli verify» lo
	// señalará y terminará la purga.
	pendientes := models.PurgarArchivos(a.almacen, resultado.ArchivosPorPurgar, func(archivo models.Archivo, err error) {
		a.log.Error("no se pudo borrar del disco un archivo purgado", "archivo", archivo.ID, "error", err)
	})
	a.responder(w, http.StatusOK, views.AnonimizacionJSON(resultado, pendientes))
}
