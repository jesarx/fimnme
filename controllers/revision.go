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
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// API de la revisión documental (§5.5) y de la subsanación (§4.2).
//
// Permisos: revisan los revisores asignados a la convocatoria y el
// administrador; el observador sólo consulta. El permiso se comprueba en cada
// petición contra la convocatoria de la postulación.

// nombreDeElemento devuelve la función que nombra un elemento señalado —un
// campo o un documento— en el idioma dado. Los campos toman su etiqueta de
// las traducciones; los documentos, de la etiqueta que el administrador dio
// al requisito.
func (a *App) nombreDeElemento(idioma string, cfg models.ConfiguracionConvocatoria) views.NombreDeElemento {
	return func(clase models.ClaseProblema, campo string) string {
		if clase != models.ProblemaDocumento {
			return a.vistas.T(idioma, "campo."+campo)
		}
		for _, requisito := range cfg.Requisitos {
			if requisito.Clave == campo {
				if idioma == models.IdiomaEN && requisito.EtiquetaEN != "" {
					return requisito.EtiquetaEN
				}
				return requisito.EtiquetaES
			}
		}
		return campo
	}
}

// postulacionRevisable lee una postulación y comprueba que la sesión pueda
// revisarla. A quien no puede ni verla le responde 404; a quien la ve pero no
// la revisa (el observador, el titular), 403.
func (a *App) postulacionRevisable(w http.ResponseWriter, r *http.Request, id int64) (models.Postulacion, bool) {
	p, ok := a.postulacionVisible(w, r, id)
	if !ok {
		return p, false
	}
	if !models.PuedeRevisar(sesionDe(r), p.ConvocatoriaID) {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return p, false
	}
	return p, true
}

// GET /api/v1/convocatorias/{id}/revision   — bandeja del revisor
//
// Filtros: estado, subcategoria, toma (libre, mia u otra) y q (folio o nombre).
func (a *App) apiBandejaDeRevision(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	sesion := sesionDe(r)
	if !models.PuedeVerRevision(sesion, convocatoriaID) {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return
	}
	consulta := r.URL.Query()
	filtro := models.FiltroBandeja{
		Estado: models.EstadoDePostulacion(consulta.Get("estado")), Subcategoria: models.Subcategoria(consulta.Get("subcategoria")),
		Toma: consulta.Get("toma"), Texto: consulta.Get("q"),
	}
	if filtro.Toma != "" && filtro.Toma != "libre" && filtro.Toma != "mia" && filtro.Toma != "otra" {
		a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", "toma")
		return
	}
	// El tablero cuenta todas; la lista, sólo las que pasan el filtro.
	todas, err := a.datos.BandejaDeRevision(r.Context(), convocatoriaID, sesion.Usuario.ID, models.FiltroBandeja{})
	if a.falloDatos(w, r, err) {
		return
	}
	tablero := make(map[models.EstadoDePostulacion]int)
	for _, renglon := range todas {
		tablero[renglon.Estado]++
	}
	filtradas := todas
	if filtro != (models.FiltroBandeja{}) {
		if filtradas, err = a.datos.BandejaDeRevision(r.Context(), convocatoriaID, sesion.Usuario.ID, filtro); a.falloDatos(w, r, err) {
			return
		}
	}
	a.responder(w, http.StatusOK, views.BandejaJSON(filtradas, tablero, a.ahora()))
}

// POST /api/v1/postulaciones/{id}/toma   — «la reviso yo»
// DELETE /api/v1/postulaciones/{id}/toma — la libera
func (a *App) apiTomarPostulacion(tomar bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.idDeRuta(w, r)
		if !ok {
			return
		}
		if _, ok := a.postulacionRevisable(w, r, id); !ok {
			return
		}
		if a.falloDatos(w, r, a.datos.TomarPostulacion(r.Context(), a.actorDe(r), id, tomar)) {
			return
		}
		a.responderPostulacion(w, r, http.StatusOK, id)
	}
}

// PUT /api/v1/documentos/{id}/dictamen   — válido, inválido (con motivo) o pendiente
func (a *App) apiDictaminarDocumento(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Dictamen string `json:"dictamen"`
		Motivo   string `json:"motivo"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	documento, err := a.datos.DocumentoPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	if _, ok := a.postulacionRevisable(w, r, documento.PostulacionID); !ok {
		return
	}
	postulacionID, err := a.datos.DictaminarDocumento(r.Context(), a.actorDe(r), id, entrada.Dictamen, entrada.Motivo)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, postulacionID)
}

// POST /api/v1/postulaciones/{id}/observaciones   — nota interna del revisor
func (a *App) apiObservarPostulacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		ObraID int64  `json:"obra_id"`
		Texto  string `json:"texto"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if _, ok := a.postulacionRevisable(w, r, id); !ok {
		return
	}
	if a.falloDatos(w, r, a.datos.ObservarPostulacion(r.Context(), a.actorDe(r), id, entrada.ObraID, entrada.Texto)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusCreated, id)
}

// entradaElemento identifica en una petición un elemento de la postulación:
// un campo (por su ruta) o un documento (por la clave de su requisito).
type entradaElemento struct {
	Clase  string `json:"clase"` // "campo" o "documento"
	Campo  string `json:"campo"`
	ObraID int64  `json:"obra_id"`
}

// POST /api/v1/postulaciones/{id}/subsanaciones   — solicita una subsanación
//
// Cuerpo: {"problemas": [{"clase", "campo", "obra_id", "problema"}],
// "fecha_limite": "…", "nota": "…"}. Los documentos dictaminados como
// inválidos se añaden solos a la lista, con el motivo de su dictamen.
func (a *App) apiSolicitarSubsanacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Problemas []struct {
			entradaElemento
			Problema string `json:"problema"`
		} `json:"problemas"`
		FechaLimite time.Time `json:"fecha_limite"`
		Nota        string    `json:"nota"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	p, ok := a.postulacionRevisable(w, r, id)
	if !ok {
		return
	}
	nueva := models.NuevaSubsanacion{FechaLimite: entrada.FechaLimite, Nota: entrada.Nota}
	for _, problema := range entrada.Problemas {
		nueva.Problemas = append(nueva.Problemas, models.ProblemaSubsanacion{
			Clase: models.ClaseProblema(problema.Clase), Campo: problema.Campo, ObraID: problema.ObraID, Problema: problema.Problema,
		})
	}
	// El correo va en el idioma del postulante, no en el del revisor (RF-REV-06).
	titular, err := a.datos.UsuarioPorID(r.Context(), p.UsuarioID)
	if a.falloDatos(w, r, err) {
		return
	}
	plantilla, err := a.datos.PlantillaCorreo(r.Context(), models.PlantillaSubsanacion, titular.Usuario.Idioma)
	if a.falloDatos(w, r, err) {
		return
	}
	componer := func(p models.Postulacion, cfg models.ConfiguracionConvocatoria, titular models.Usuario, n models.NuevaSubsanacion) (models.CorreoSaliente, error) {
		idioma := titular.Idioma
		nombrar := a.nombreDeElemento(idioma, cfg)
		ordenDeObra := make(map[int64]int)
		for _, obra := range p.Obras {
			ordenDeObra[obra.ID] = obra.Orden
		}
		// Un renglón por problema: «• Partitura (obra 2): está incompleta».
		var lista strings.Builder
		for _, problema := range n.Problemas {
			elemento := nombrar(problema.Clase, problema.Campo)
			if problema.ObraID != 0 {
				elemento = a.vistas.T(idioma, "subsanacion.elemento_de_obra", elemento, ordenDeObra[problema.ObraID])
			}
			fmt.Fprintf(&lista, "• %s: %s\n", elemento, strings.TrimSpace(problema.Problema))
		}
		return views.RenderCorreo(plantilla, titular.Email, map[string]string{
			"nombre": titular.Nombre, "folio": p.Folio,
			"convocatoria": cfg.Convocatoria.Titulo(idioma), "edicion": cfg.Convocatoria.Edicion.Nombre(idioma),
			"lista_problemas": strings.TrimRight(lista.String(), "\n"),
			"fecha_limite":    a.vistas.FechaHora(idioma, n.FechaLimite),
			"nota":            n.Nota,
			"liga":            a.urlBase + "/postulaciones/" + p.Folio,
		}), nil
	}
	if a.falloDatos(w, r, a.datos.SolicitarSubsanacion(r.Context(), a.actorDe(r), id, nueva, componer)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusCreated, id)
}

// POST /api/v1/postulaciones/{id}/desbloqueos   — el postulante pide corregir algo no señalado
//
// Cuerpo: {"clase", "campo", "obra_id", "justificacion"}.
func (a *App) apiSolicitarDesbloqueo(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		entradaElemento
		Justificacion string `json:"justificacion"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	solicitud := models.SolicitudDesbloqueo{
		Clase: models.ClaseProblema(entrada.Clase), Campo: entrada.Campo, ObraID: entrada.ObraID, Justificacion: entrada.Justificacion,
	}
	if a.falloDatos(w, r, a.datos.SolicitarDesbloqueo(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, id, solicitud)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusCreated, id)
}

// PUT /api/v1/postulaciones/{id}/desbloqueos/{solicitud}   — el revisor concede o niega
//
// Cuerpo: {"concedida": true|false, "motivo": "…"}. Negar exige motivo.
func (a *App) apiResolverDesbloqueo(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	solicitudID, err := strconv.ParseInt(r.PathValue("solicitud"), 10, 64)
	var entrada struct {
		Concedida *bool  `json:"concedida"`
		Motivo    string `json:"motivo"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if err != nil || solicitudID < 1 {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	if entrada.Concedida == nil {
		a.errorAPI(w, r, http.StatusBadRequest, "json_invalido")
		return
	}
	if _, ok := a.postulacionRevisable(w, r, id); !ok {
		return
	}
	if a.falloDatos(w, r, a.datos.ResolverDesbloqueo(r.Context(), a.actorDe(r), id, solicitudID, *entrada.Concedida, entrada.Motivo)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// POST /api/v1/postulaciones/{id}/dictamen   — validada o descartada
//
// Cuerpo: {"dictamen": "validada"|"descartada", "motivo": "…"}. Descartar
// exige motivo. No se comunica nada al postulante.
func (a *App) apiDictaminarPostulacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Dictamen string `json:"dictamen"`
		Motivo   string `json:"motivo"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if _, ok := a.postulacionRevisable(w, r, id); !ok {
		return
	}
	err := a.datos.DictaminarPostulacion(r.Context(), a.actorDe(r), id, models.EstadoDePostulacion(entrada.Dictamen), entrada.Motivo)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}
