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
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// API de configuración (RF-CONV): ediciones, convocatorias, calendario y
// etapas, dotaciones, requisitos documentales, criterios, cuentas internas y
// asignaciones de jurados y revisores.
//
// Permisos (matriz de §2.2): escribe sólo admin_sistema; coordinacion_nacional
// puede leerlo todo. Los controladores comprueban el permiso y traducen
// errores; las reglas viven en models/rules.go y las aplica el repositorio
// dentro de la misma transacción que la escritura.

// --- Piezas compartidas ---------------------------------------------------------

// administra protege un manejador: sólo admin_sistema.
func (a *App) administra(manejador http.HandlerFunc) http.HandlerFunc {
	return a.conSesion(func(w http.ResponseWriter, r *http.Request) {
		if !models.PuedeAdministrar(sesionDe(r)) {
			a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
			return
		}
		manejador(w, r)
	})
}

// supervisa protege un manejador de sólo lectura: admin_sistema y
// coordinacion_nacional.
func (a *App) supervisa(manejador http.HandlerFunc) http.HandlerFunc {
	return a.conSesion(func(w http.ResponseWriter, r *http.Request) {
		if !models.PuedeSupervisar(sesionDe(r)) {
			a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
			return
		}
		manejador(w, r)
	})
}

// idDeRuta lee el identificador {id} de la ruta. Si no es un entero positivo
// responde 404 y devuelve false.
func (a *App) idDeRuta(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return 0, false
	}
	return id, true
}

// leerJSON decodifica el cuerpo de la petición en destino. Se rechazan los
// campos desconocidos: un nombre mal escrito debe notarse, no ignorarse en
// silencio. Si el cuerpo no sirve responde 400 y devuelve false.
func (a *App) leerJSON(w http.ResponseWriter, r *http.Request, destino any) bool {
	decodificador := json.NewDecoder(r.Body)
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(destino); err != nil || decodificador.More() {
		a.errorAPI(w, r, http.StatusBadRequest, "json_invalido")
		return false
	}
	return true
}

// bloqueadores traduce una lista de problemas al idioma de la petición.
func (a *App) bloqueadores(r *http.Request, errores []models.ErrorCampo) []views.Bloqueador {
	lista := make([]views.Bloqueador, 0, len(errores))
	for _, e := range errores {
		lista = append(lista, views.Bloqueador{Campo: e.Campo, Mensaje: a.vistas.T(idiomaDe(r), e.Clave)})
	}
	return lista
}

// falloDatos traduce un error del repositorio a su respuesta HTTP. Devuelve
// true si hubo error (y ya se respondió).
func (a *App) falloDatos(w http.ResponseWriter, r *http.Request, err error) bool {
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
		return false
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
	case errors.Is(err, models.ErrDuplicado), errors.Is(err, models.ErrEmailEnUso):
		a.errorAPI(w, r, http.StatusConflict, "duplicado")
	case errors.As(err, &regla):
		cuerpo := views.ErrorValidacionJSON(a.vistas.T(idiomaDe(r), "api.validacion"), a.bloqueadores(r, regla.Errores))
		if err := views.JSON(w, http.StatusUnprocessableEntity, cuerpo); err != nil {
			a.log.Error("no se pudo escribir la respuesta JSON", "error", err)
		}
	default:
		a.log.Error("fallo en la API de configuración", "ruta", r.URL.Path, "error", err)
		a.errorAPI(w, r, http.StatusInternalServerError, "error_interno")
	}
	return true
}

// responder escribe una respuesta JSON con el estado dado.
func (a *App) responder(w http.ResponseWriter, estado int, datos any) {
	if err := views.JSON(w, estado, datos); err != nil {
		a.log.Error("no se pudo escribir la respuesta JSON", "error", err)
	}
}

// responderConfiguracion responde con la configuración completa de una
// convocatoria, incluido lo que le falta para pasar a su siguiente etapa.
func (a *App) responderConfiguracion(w http.ResponseWriter, r *http.Request, estado int, convocatoriaID int64) {
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	siguiente, _ := models.EstadoSiguiente(cfg.Convocatoria.Estado)
	var pendientes []views.Bloqueador
	if siguiente != "" {
		pendientes = a.bloqueadores(r, models.ValidarCambioDeEstado(cfg, siguiente, "", a.ahora()))
	}
	a.responder(w, estado, views.ConfiguracionJSON(cfg, siguiente, pendientes))
}

// responderEdicion responde con una edición y el resumen de sus convocatorias.
func (a *App) responderEdicion(w http.ResponseWriter, r *http.Request, estado int, id int64) {
	edicion, err := a.datos.EdicionPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, estado, views.EdicionJSON(edicion, convocatorias))
}

// --- Ediciones (RF-CONV-01) -----------------------------------------------------

type entradaEdicion struct {
	Numero   int    `json:"numero"`
	Anio     int    `json:"anio"`
	NombreES string `json:"nombre_es"`
	NombreEN string `json:"nombre_en"`
}

func (e entradaEdicion) entidad(id int64) models.Edicion {
	return models.Edicion{ID: id, Numero: e.Numero, Anio: e.Anio,
		NombreES: strings.TrimSpace(e.NombreES), NombreEN: strings.TrimSpace(e.NombreEN)}
}

// GET /api/v1/ediciones
func (a *App) apiEdiciones(w http.ResponseWriter, r *http.Request) {
	ediciones, err := a.datos.ListarEdiciones(r.Context())
	if a.falloDatos(w, r, err) {
		return
	}
	lista := make([]map[string]any, 0, len(ediciones))
	for _, e := range ediciones {
		convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), e.ID)
		if a.falloDatos(w, r, err) {
			return
		}
		lista = append(lista, views.EdicionJSON(e, convocatorias))
	}
	a.responder(w, http.StatusOK, views.EdicionesJSON(lista))
}

// POST /api/v1/ediciones
func (a *App) apiCrearEdicion(w http.ResponseWriter, r *http.Request) {
	var entrada entradaEdicion
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	id, err := a.datos.CrearEdicion(r.Context(), a.actorDe(r), entrada.entidad(0))
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderEdicion(w, r, http.StatusCreated, id)
}

// GET /api/v1/ediciones/{id}
func (a *App) apiEdicion(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.responderEdicion(w, r, http.StatusOK, id)
	}
}

// PUT /api/v1/ediciones/{id}
func (a *App) apiActualizarEdicion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada entradaEdicion
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if a.falloDatos(w, r, a.datos.ActualizarEdicion(r.Context(), a.actorDe(r), entrada.entidad(id))) {
		return
	}
	a.responderEdicion(w, r, http.StatusOK, id)
}

// --- Convocatorias (RF-CONV-02, 03, 04, 05 y 08) --------------------------------

type entradaEtapa struct {
	Clave     string    `json:"clave"`
	IniciaAt  time.Time `json:"inicia_at"`
	TerminaAt time.Time `json:"termina_at"`
}

func etapasDeEntrada(entrada []entradaEtapa) []models.Etapa {
	etapas := make([]models.Etapa, 0, len(entrada))
	for _, e := range entrada {
		etapas = append(etapas, models.Etapa{Clave: models.ClaveEtapa(e.Clave), IniciaAt: e.IniciaAt, TerminaAt: e.TerminaAt})
	}
	return etapas
}

// entradaConvocatoria son los textos y límites de una convocatoria. Los
// límites omitidos toman el valor por omisión de su tipo.
type entradaConvocatoria struct {
	Tipo                   string         `json:"tipo"` // sólo al crear
	TituloES               string         `json:"titulo_es"`
	TituloEN               string         `json:"titulo_en"`
	BasesMDES              string         `json:"bases_md_es"`
	BasesMDEN              string         `json:"bases_md_en"`
	MaxObras               *int           `json:"max_obras"`
	DuracionMaxProgramaMin *int           `json:"duracion_max_programa_min"`
	MaxVideos              *int           `json:"max_videos"`
	Etapas                 []entradaEtapa `json:"etapas"` // sólo al crear
}

// entidad convierte la entrada en la entidad, aplicando los valores por
// omisión: en Compositores, 2 obras como máximo; en Intérpretes, sin límite;
// en ambas, hasta 3 ligas de video.
func (e entradaConvocatoria) entidad(tipo models.TipoConvocatoria) models.Convocatoria {
	c := models.Convocatoria{
		Tipo:     tipo,
		TituloES: strings.TrimSpace(e.TituloES), TituloEN: strings.TrimSpace(e.TituloEN),
		BasesMDES: e.BasesMDES, BasesMDEN: e.BasesMDEN,
		MaxVideos: models.MaxVideosPermitidos,
	}
	if tipo == models.TipoCompositores {
		c.MaxObras = models.MaxObrasCompositores
	}
	if e.MaxObras != nil {
		c.MaxObras = *e.MaxObras
	}
	if e.DuracionMaxProgramaMin != nil {
		c.DuracionMaxProgramaMin = *e.DuracionMaxProgramaMin
	}
	if e.MaxVideos != nil {
		c.MaxVideos = *e.MaxVideos
	}
	return c
}

// POST /api/v1/ediciones/{id}/convocatorias
//
// La convocatoria nace en BORRADOR, con los requisitos documentales y los
// criterios de evaluación iniciales de su tipo, listos para ajustarse.
func (a *App) apiCrearConvocatoria(w http.ResponseWriter, r *http.Request) {
	edicionID, ok := a.idDeRuta(w, r)
	var entrada entradaConvocatoria
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	tipo := models.TipoConvocatoria(entrada.Tipo)
	c := entrada.entidad(tipo)
	c.Edicion.ID, c.Estado, c.Etapas = edicionID, models.EstadoBorrador, etapasDeEntrada(entrada.Etapas)
	id, err := a.datos.CrearConvocatoriaConfigurada(r.Context(), a.actorDe(r), c,
		models.RequisitosIniciales(tipo), models.CriteriosIniciales())
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusCreated, id)
}

// GET /api/v1/convocatorias/{id}
func (a *App) apiConvocatoria(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.responderConfiguracion(w, r, http.StatusOK, id)
	}
}

// PUT /api/v1/convocatorias/{id}   — textos y límites
func (a *App) apiActualizarConvocatoria(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada entradaConvocatoria
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	// El tipo no cambia: se toma del registro para aplicar sus valores por omisión.
	actual, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	c := entrada.entidad(actual.Convocatoria.Tipo)
	c.ID = id
	if a.falloDatos(w, r, a.datos.ActualizarConvocatoria(r.Context(), a.actorDe(r), c)) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusOK, id)
}

// PUT /api/v1/convocatorias/{id}/etapas   — sustituye el calendario
func (a *App) apiDefinirEtapas(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Etapas []entradaEtapa `json:"etapas"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if a.falloDatos(w, r, a.datos.DefinirEtapas(r.Context(), a.actorDe(r), id, etapasDeEntrada(entrada.Etapas))) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusOK, id)
}

// POST /api/v1/convocatorias/{id}/estado   — publicar, avanzar, retroceder, archivar
//
// Es el cambio de etapa FORZADO por el administrador (§3). Sólo admite el
// estado inmediato anterior o siguiente; retroceder exige un motivo. Queda en
// la bitácora.
func (a *App) apiCambiarEstado(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Estado string `json:"estado"`
		Motivo string `json:"motivo"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	_, err := a.datos.CambiarEstadoConvocatoria(r.Context(), a.actorDe(r), id,
		models.EstadoConvocatoria(entrada.Estado), strings.TrimSpace(entrada.Motivo), false)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusOK, id)
}

// POST /api/v1/convocatorias/{id}/duplicar   — copia la configuración a otra edición
func (a *App) apiDuplicarConvocatoria(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		EdicionID int64 `json:"edicion_id"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	nuevoID, err := a.datos.DuplicarConvocatoria(r.Context(), a.actorDe(r), id, entrada.EdicionID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusCreated, nuevoID)
}

// --- Dotaciones (RF-CONV-06) ---------------------------------------------------

type entradaDotacion struct {
	NombreES      string `json:"nombre_es"`
	NombreEN      string `json:"nombre_en"`
	DescripcionES string `json:"descripcion_es"`
	DescripcionEN string `json:"descripcion_en"`
	Orden         int    `json:"orden"`
	Activa        *bool  `json:"activa"` // omitida = activa
}

// guardarDotacion crea (id 0, con la convocatoria de la ruta) o actualiza.
func (a *App) guardarDotacion(w http.ResponseWriter, r *http.Request, estado int, id, convocatoriaID int64) {
	var entrada entradaDotacion
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	d := models.Dotacion{
		ID: id, ConvocatoriaID: convocatoriaID,
		NombreES: strings.TrimSpace(entrada.NombreES), NombreEN: strings.TrimSpace(entrada.NombreEN),
		DescripcionES: entrada.DescripcionES, DescripcionEN: entrada.DescripcionEN,
		Orden: entrada.Orden, Activa: entrada.Activa == nil || *entrada.Activa,
	}
	_, convocatoriaID, err := a.datos.GuardarDotacion(r.Context(), a.actorDe(r), d)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, estado, convocatoriaID)
}

// POST /api/v1/convocatorias/{id}/dotaciones
func (a *App) apiCrearDotacion(w http.ResponseWriter, r *http.Request) {
	if convocatoriaID, ok := a.idDeRuta(w, r); ok {
		a.guardarDotacion(w, r, http.StatusCreated, 0, convocatoriaID)
	}
}

// PUT /api/v1/dotaciones/{id}   — para retirarla del catálogo: "activa": false
func (a *App) apiActualizarDotacion(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.guardarDotacion(w, r, http.StatusOK, id, 0)
	}
}

// --- Requisitos documentales (RF-CONV-07) ---------------------------------------

type entradaRequisito struct {
	DotacionID      int64    `json:"dotacion_id"` // 0 = regla general de la convocatoria
	Clave           string   `json:"clave"`
	EtiquetaES      string   `json:"etiqueta_es"`
	EtiquetaEN      string   `json:"etiqueta_en"`
	AyudaES         string   `json:"ayuda_es"`
	AyudaEN         string   `json:"ayuda_en"`
	Ambito          string   `json:"ambito"`
	Obligatorio     bool     `json:"obligatorio"`
	MimesPermitidos []string `json:"mimes_permitidos"`
	TamanoMaxBytes  int64    `json:"tamano_max_bytes"`
	MaxArchivos     *int     `json:"max_archivos"` // omitido = 1
	Orden           int      `json:"orden"`
}

func (a *App) guardarRequisito(w http.ResponseWriter, r *http.Request, estado int, id, convocatoriaID int64) {
	var entrada entradaRequisito
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	rq := models.RequisitoDocumental{
		ID: id, ConvocatoriaID: convocatoriaID, DotacionID: entrada.DotacionID, Clave: entrada.Clave,
		EtiquetaES: strings.TrimSpace(entrada.EtiquetaES), EtiquetaEN: strings.TrimSpace(entrada.EtiquetaEN),
		AyudaES: entrada.AyudaES, AyudaEN: entrada.AyudaEN, Ambito: models.AmbitoRequisito(entrada.Ambito),
		Obligatorio: entrada.Obligatorio, MimesPermitidos: entrada.MimesPermitidos,
		TamanoMaxBytes: entrada.TamanoMaxBytes, MaxArchivos: 1, Orden: entrada.Orden,
	}
	if entrada.MaxArchivos != nil {
		rq.MaxArchivos = *entrada.MaxArchivos
	}
	_, convocatoriaID, err := a.datos.GuardarRequisito(r.Context(), a.actorDe(r), rq)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, estado, convocatoriaID)
}

// POST /api/v1/convocatorias/{id}/requisitos
func (a *App) apiCrearRequisito(w http.ResponseWriter, r *http.Request) {
	if convocatoriaID, ok := a.idDeRuta(w, r); ok {
		a.guardarRequisito(w, r, http.StatusCreated, 0, convocatoriaID)
	}
}

// PUT /api/v1/requisitos/{id}
func (a *App) apiActualizarRequisito(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.guardarRequisito(w, r, http.StatusOK, id, 0)
	}
}

// DELETE /api/v1/requisitos/{id}   — sólo con la convocatoria en borrador
func (a *App) apiBorrarRequisito(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok && !a.falloDatos(w, r, a.datos.BorrarRequisito(r.Context(), a.actorDe(r), id)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Criterios de evaluación (RF-EVA-10) ---------------------------------------

type entradaCriterio struct {
	Clave         string `json:"clave"`
	EtiquetaES    string `json:"etiqueta_es"`
	EtiquetaEN    string `json:"etiqueta_en"`
	DescripcionES string `json:"descripcion_es"`
	DescripcionEN string `json:"descripcion_en"`
	EscalaMin     int    `json:"escala_min"`
	EscalaMax     int    `json:"escala_max"`
	Obligatorio   *bool  `json:"obligatorio"` // omitido = obligatorio
	Orden         int    `json:"orden"`
}

func (a *App) guardarCriterio(w http.ResponseWriter, r *http.Request, estado int, id, convocatoriaID int64) {
	var entrada entradaCriterio
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	c := models.CriterioEvaluacion{
		ID: id, ConvocatoriaID: convocatoriaID, Clave: entrada.Clave,
		EtiquetaES: strings.TrimSpace(entrada.EtiquetaES), EtiquetaEN: strings.TrimSpace(entrada.EtiquetaEN),
		DescripcionES: entrada.DescripcionES, DescripcionEN: entrada.DescripcionEN,
		EscalaMin: entrada.EscalaMin, EscalaMax: entrada.EscalaMax,
		Obligatorio: entrada.Obligatorio == nil || *entrada.Obligatorio, Orden: entrada.Orden,
	}
	_, convocatoriaID, err := a.datos.GuardarCriterio(r.Context(), a.actorDe(r), c)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, estado, convocatoriaID)
}

// POST /api/v1/convocatorias/{id}/criterios
func (a *App) apiCrearCriterio(w http.ResponseWriter, r *http.Request) {
	if convocatoriaID, ok := a.idDeRuta(w, r); ok {
		a.guardarCriterio(w, r, http.StatusCreated, 0, convocatoriaID)
	}
}

// PUT /api/v1/criterios/{id}
func (a *App) apiActualizarCriterio(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.guardarCriterio(w, r, http.StatusOK, id, 0)
	}
}

// DELETE /api/v1/criterios/{id}   — sólo con la convocatoria en borrador
func (a *App) apiBorrarCriterio(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok && !a.falloDatos(w, r, a.datos.BorrarCriterio(r.Context(), a.actorDe(r), id)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Asignaciones de jurados y revisores (RF-CONV-09) ---------------------------

// POST /api/v1/convocatorias/{id}/asignaciones
func (a *App) apiAsignar(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	var entrada struct {
		UsuarioID int64  `json:"usuario_id"`
		Perfil    string `json:"perfil"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	_, err := a.datos.Asignar(r.Context(), a.actorDe(r), convocatoriaID, entrada.UsuarioID, models.Perfil(entrada.Perfil))
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusCreated, convocatoriaID)
}

// DELETE /api/v1/asignaciones/{id}   — retira el perfil; no borra lo ya hecho
func (a *App) apiRetirarAsignacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	convocatoriaID, err := a.datos.RetirarAsignacion(r.Context(), a.actorDe(r), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusOK, convocatoriaID)
}

// --- Cuentas internas (RF-AUT-07, RF-NOT-08) ------------------------------------

// GET /api/v1/usuarios?q=&pagina=&tamano=
//
// Busca cuentas por correo, nombre o apellidos. Sirve para localizar la cuenta
// de alguien antes de asignarlo: si ya postuló otro año, ya tiene una.
func (a *App) apiUsuarios(w http.ResponseWriter, r *http.Request) {
	consulta := r.URL.Query()
	pagina, tamano := 1, tamanoPaginaPorOmision
	if texto := consulta.Get("pagina"); texto != "" {
		n, ok := enteroDeFiltro(texto)
		if !ok || n < 1 {
			a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", "pagina")
			return
		}
		pagina = int(n)
	}
	if texto := consulta.Get("tamano"); texto != "" {
		n, ok := enteroDeFiltro(texto)
		if !ok || n < 1 || n > tamanoPaginaMaximo {
			a.errorAPI(w, r, http.StatusBadRequest, "parametro_invalido", "tamano")
			return
		}
		tamano = int(n)
	}
	cuentas, total, err := a.datos.BuscarUsuarios(r.Context(), strings.TrimSpace(consulta.Get("q")), pagina, tamano)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.CuentasJSON(cuentas, total, pagina, tamano))
}

// correoDeActivacion genera un enlace de activación y el correo que lo lleva.
func (a *App) correoDeActivacion(r *http.Request, email, nombre, idioma string) (models.NuevoToken, models.CorreoSaliente, error) {
	claro, token, err := a.nuevoToken(models.TokenActivacion)
	if err != nil {
		return token, models.CorreoSaliente{}, err
	}
	correo, err := a.correo(r, models.PlantillaActivacion, idioma, email, map[string]string{
		"nombre": nombre, "liga": a.liga("/cuenta/activar", claro),
	})
	return token, correo, err
}

// POST /api/v1/usuarios
//
// Da de alta una cuenta interna: jurado, revisor u observador. Nace sin
// contraseña; la persona recibe por correo un enlace de activación para
// elegirla. "perfil_global" vacío crea una cuenta sin perfil global, que es lo
// que corresponde a un jurado o un revisor: sus perfiles se otorgan después,
// por convocatoria.
func (a *App) apiCrearUsuario(w http.ResponseWriter, r *http.Request) {
	var entrada struct {
		Email        string `json:"email"`
		Nombre       string `json:"nombre"`
		Apellidos    string `json:"apellidos"`
		Idioma       string `json:"idioma"`
		PerfilGlobal string `json:"perfil_global"`
	}
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	nuevo := models.NuevoUsuario{
		Email:  models.NormalizarEmail(entrada.Email),
		Nombre: strings.TrimSpace(entrada.Nombre), Apellidos: strings.TrimSpace(entrada.Apellidos),
		Idioma: entrada.Idioma, PasswordHash: models.PasswordSinDefinir, Perfil: models.Perfil(entrada.PerfilGlobal),
	}
	if nuevo.Idioma == "" {
		nuevo.Idioma = models.IdiomaES
	}
	if errores := models.ValidarCuentaInterna(nuevo); len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return
	}
	token, correo, err := a.correoDeActivacion(r, nuevo.Email, nuevo.Nombre, nuevo.Idioma)
	if a.falloDatos(w, r, err) {
		return
	}
	id, err := a.datos.CrearUsuario(r.Context(), a.actorDe(r), nuevo, &token, correo)
	if a.falloDatos(w, r, err) {
		return
	}
	cuenta, err := a.datos.UsuarioPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusCreated, views.CuentaJSON(cuenta))
}

// POST /api/v1/usuarios/{id}/activacion
//
// Envía de nuevo el enlace de activación a una cuenta que aún no elige su
// contraseña (el enlace anterior vence a los 7 días, y deja de valer al
// emitirse éste).
func (a *App) apiReenviarActivacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	cuenta, err := a.datos.UsuarioPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	if cuenta.Activada {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "id", Clave: "error.cuenta_activada"}}})
		return
	}
	token, correo, err := a.correoDeActivacion(r, cuenta.Usuario.Email, cuenta.Usuario.Nombre, cuenta.Usuario.Idioma)
	if a.falloDatos(w, r, err) {
		return
	}
	if a.falloDatos(w, r, a.datos.EmitirToken(r.Context(), a.actorDe(r), id, token, correo)) {
		return
	}
	a.responder(w, http.StatusOK, views.CuentaJSON(cuenta))
}
