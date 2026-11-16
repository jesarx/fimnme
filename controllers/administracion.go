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
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pantallas de administración, primera parte (paso 13a): ediciones,
// convocatorias con toda su configuración, jurados y revisores, y cuentas
// internas.
//
// Como las demás pantallas, no tienen reglas propias: cada formulario llama a
// la misma operación del repositorio que la API (y que el modo interactivo
// de fimnme-cli), y lo que el modelo rechaza vuelve junto al formulario. Las
// ve quien supervisa; sólo el administrador ve formularios y puede enviarlos
// (§2.2).

// formatoLocal es el de un control datetime-local: fecha y hora, sin zona.
// Se escribe y se lee en hora de la Ciudad de México y se guarda en UTC.
const formatoLocal = "2006-01-02T15:04"

// aLocal escribe un instante para un control datetime-local.
func aLocal(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(views.ZonaMexico()).Format(formatoLocal)
}

// deLocal lee lo escrito en un control datetime-local; vacío o ilegible es cero.
func deLocal(texto string) time.Time {
	t, err := time.ParseInLocation(formatoLocal, strings.TrimSpace(texto), views.ZonaMexico())
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// enteroDeFormulario lee un número de un formulario; lo que no lo es vale
// -1, que ninguna regla admite: así el modelo lo rechaza con su motivo.
func enteroDeFormulario(texto string) int {
	if strings.TrimSpace(texto) == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(texto))
	if err != nil {
		return -1
	}
	return n
}

// --- Modelos de vista ----------------------------------------------------------

// vistaConvocatoriaAdmin es una convocatoria en una lista.
type vistaConvocatoriaAdmin struct {
	URL    string
	Titulo string
	Tipo   string
	Estado string
	Clase  string
}

// vistaEdicionAdmin es una edición con sus convocatorias.
type vistaEdicionAdmin struct {
	ID            int64
	URL           string
	Numero, Anio  int
	NombreES      string
	NombreEN      string
	Convocatorias []vistaConvocatoriaAdmin
	// Para el alta de una convocatoria: los tipos que aún no tiene, y las
	// convocatorias de otras ediciones de las que puede copiarse.
	TiposLibres []vistaOpcion
	Origenes    []vistaOpcion
}

// vistaPortadaAdmin es la portada de la administración.
type vistaPortadaAdmin struct {
	Ediciones []vistaEdicionAdmin
}

// vistaEtapaAdmin es un tramo del calendario, para su formulario.
type vistaEtapaAdmin struct {
	Clave, Nombre, Inicia, Termina string
}

// vistaDotacionAdmin es una dotación del catálogo, para su formulario.
type vistaDotacionAdmin struct {
	ID                                               int64
	NombreES, NombreEN, DescripcionES, DescripcionEN string
	Orden                                            int
	Activa                                           bool
}

// vistaRequisitoAdmin es un requisito documental, para su formulario.
type vistaRequisitoAdmin struct {
	ID                                              int64
	Clave, EtiquetaES, EtiquetaEN, AyudaES, AyudaEN string
	Ambito                                          string
	Dotaciones                                      []vistaOpcion // la elegida, marcada
	DotacionNombre                                  string
	Obligatorio                                     bool
	Mimes                                           []vistaOpcion // los admitidos, marcados los que pide
	TamanoMB, MaxArchivos, Orden                    int
}

// vistaCriterioAdmin es un criterio de evaluación, para su formulario.
type vistaCriterioAdmin struct {
	ID                                                          int64
	Clave, EtiquetaES, EtiquetaEN, DescripcionES, DescripcionEN string
	EscalaMin, EscalaMax, Orden                                 int
	Obligatorio                                                 bool
}

// vistaAsignacionAdmin es un jurado o revisor de la convocatoria.
type vistaAsignacionAdmin struct {
	ID        int64
	Perfil    string // clave de traducción
	Nombre    string
	Email     string
	Activada  bool
	UsuarioID int64
}

// vistaCuentaAdmin es una cuenta en una lista o en una búsqueda.
type vistaCuentaAdmin struct {
	ID       int64
	Nombre   string
	Email    string
	Perfiles []string // claves de traducción
	Activada bool
	Activa   bool
}

// vistaConvocatoriaConfig es la ficha de configuración de una convocatoria.
type vistaConvocatoriaConfig struct {
	ID               int64
	Base             string
	Edicion          string
	EdicionURL       string
	Publica          string // dirección pública de la convocatoria
	Tipo             string
	EsCompositores   bool
	Titulo           string
	Estado           string
	Clase            string
	PuedeAdministrar bool

	TituloES, TituloEN, BasesES, BasesEN string
	MaxObras, DuracionMax, MaxVideos     int
	BasesPDFES, BasesPDFEN, Imagen       string // direcciones públicas; vacías si no hay

	Etapas []vistaEtapaAdmin

	// Estados vecinos: clave de traducción y valor para el formulario; vacíos
	// si no hay.
	Siguiente, SiguienteValor string
	Anterior, AnteriorValor   string
	FaltaParaAvanzar          []string // lo que el modelo pide para pasar al siguiente

	Dotaciones         []vistaDotacionAdmin
	Requisitos         []vistaRequisitoAdmin
	NuevoRequisito     vistaRequisitoAdmin
	NuevaDotacion      vistaDotacionAdmin
	NuevoCriterio      vistaCriterioAdmin
	NuevaCuenta        map[string]string // alta de una cuenta para asignarla
	Criterios          []vistaCriterioAdmin
	Asignaciones       []vistaAsignacionAdmin
	Jurados, Revisores int
	CuotaJurados       int

	// Búsqueda de cuentas para asignar.
	Buscar      string
	Encontradas []vistaCuentaAdmin

	Error, ErrorEn string
}

// vistaUsuariosAdmin es la página de cuentas.
type vistaUsuariosAdmin struct {
	Buscar           string
	Cuentas          []vistaCuentaAdmin
	Total            int
	PuedeAdministrar bool
	Error            string
	Valores          map[string]string
}

// --- Portada y ediciones -------------------------------------------------------------

// exigirSupervision responde 403 si la sesión no supervisa.
func (a *App) exigirSupervision(w http.ResponseWriter, r *http.Request) bool {
	if !models.PuedeSupervisar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return false
	}
	return true
}

// edicionesAdmin arma las ediciones con sus convocatorias.
func (a *App) edicionesAdmin(r *http.Request) ([]vistaEdicionAdmin, error) {
	ediciones, err := a.datos.ListarEdiciones(r.Context())
	if err != nil {
		return nil, err
	}
	idioma := idiomaDe(r)
	var lista []vistaEdicionAdmin
	for _, e := range ediciones {
		convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), e.ID)
		if err != nil {
			return nil, err
		}
		v := vistaEdicionAdmin{ID: e.ID, URL: fmt.Sprintf("/administracion/ediciones/%d", e.ID), Numero: e.Numero, Anio: e.Anio,
			NombreES: e.NombreES, NombreEN: e.NombreEN}
		for _, c := range convocatorias {
			v.Convocatorias = append(v.Convocatorias, vistaConvocatoriaAdmin{URL: fmt.Sprintf("/administracion/convocatorias/%d", c.ID),
				Titulo: c.Titulo(idioma), Tipo: "tipo." + string(c.Tipo), Estado: claveDeEstadoAdmin(c.Estado), Clase: claseEstado(c.Estado)})
		}
		lista = append(lista, v)
	}
	return lista, nil
}

// GET /administracion   — ediciones y convocatorias
func (a *App) portadaDeAdministracion(w http.ResponseWriter, r *http.Request) {
	if !a.exigirSupervision(w, r) {
		return
	}
	ediciones, err := a.edicionesAdmin(r)
	if err != nil {
		a.errorInterno(w, r, "listar ediciones", err)
		return
	}
	p := a.pagina(r, "admin.titulo")
	p.Datos = vistaPortadaAdmin{Ediciones: ediciones}
	if pedido, hay := avisosDeAdmin[r.URL.Query().Get("aviso")]; hay {
		p.Aviso = &pedido
	}
	a.pintar(w, r, http.StatusOK, "admin_portada.html", p)
}

// avisosDeAdmin son los mensajes que una redirección pide mostrar.
var avisosDeAdmin = map[string]views.Aviso{
	"edicion":      {Tipo: "exito", Clave: "admin.aviso_edicion"},
	"convocatoria": {Tipo: "exito", Clave: "admin.aviso_convocatoria"},
	"guardado":     {Tipo: "exito", Clave: "admin.aviso_guardado"},
	"estado":       {Tipo: "exito", Clave: "admin.aviso_estado"},
	"borrado":      {Tipo: "exito", Clave: "admin.aviso_borrado"},
	"asignacion":   {Tipo: "exito", Clave: "admin.aviso_asignacion"},
	"retirada":     {Tipo: "exito", Clave: "admin.aviso_retirada"},
	"cuenta":       {Tipo: "exito", Clave: "admin.aviso_cuenta"},
	"activacion":   {Tipo: "exito", Clave: "admin.aviso_activacion"},
}

// mensajeDeRechazo traduce un error del repositorio a un mensaje para el
// formulario, o devuelve false si no es un rechazo del modelo.
func (a *App) mensajeDeRechazo(r *http.Request, err error) (string, bool) {
	var regla *models.ErrorValidacion
	switch {
	case errors.As(err, &regla) && len(regla.Errores) > 0:
		var mensajes []string
		for _, e := range regla.Errores {
			mensajes = append(mensajes, a.vistas.T(idiomaDe(r), e.Clave))
		}
		return strings.Join(slices.Compact(mensajes), " "), true
	case errors.Is(err, models.ErrDuplicado):
		return a.vistas.T(idiomaDe(r), "admin.duplicado"), true
	case errors.Is(err, models.ErrEmailEnUso):
		return a.vistas.T(idiomaDe(r), "admin.correo_en_uso"), true
	}
	return "", false
}

// edicionDelFormulario lee los datos de una edición.
func edicionDelFormulario(r *http.Request, id int64) models.Edicion {
	return models.Edicion{ID: id, Numero: enteroDeFormulario(r.PostFormValue("numero")), Anio: enteroDeFormulario(r.PostFormValue("anio")),
		NombreES: textoDeFormulario(r.PostFormValue("nombre_es")), NombreEN: textoDeFormulario(r.PostFormValue("nombre_en"))}
}

// POST /administracion/ediciones   — nueva edición
func (a *App) crearEdicionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := a.datos.CrearEdicion(r.Context(), a.actorDe(r), edicionDelFormulario(r, 0))
	if err == nil {
		http.Redirect(w, r, fmt.Sprintf("/administracion/ediciones/%d?aviso=edicion", id), http.StatusSeeOther)
		return
	}
	mensaje, ok := a.mensajeDeRechazo(r, err)
	if !ok {
		a.errorInterno(w, r, "crear edición", err)
		return
	}
	ediciones, err := a.edicionesAdmin(r)
	if err != nil {
		a.errorInterno(w, r, "listar ediciones", err)
		return
	}
	p := a.pagina(r, "admin.titulo")
	p.Datos = vistaPortadaAdmin{Ediciones: ediciones}
	p.Aviso = &views.Aviso{Tipo: "error", Clave: "admin.no_guardado"}
	p.Formulario = views.Formulario{Valores: map[string]string{"numero": r.PostFormValue("numero"), "anio": r.PostFormValue("anio"),
		"nombre_es": r.PostFormValue("nombre_es"), "nombre_en": r.PostFormValue("nombre_en"), "error": mensaje}}
	a.pintar(w, r, http.StatusUnprocessableEntity, "admin_portada.html", p)
}

// GET /administracion/ediciones/{id}   — una edición: sus datos y sus convocatorias
func (a *App) edicionDeAdmin(w http.ResponseWriter, r *http.Request) {
	a.pintarEdicion(w, r, http.StatusOK, "", "")
}

// pintarEdicion muestra la página de una edición, con un rechazo junto a su
// formulario si lo hay.
func (a *App) pintarEdicion(w http.ResponseWriter, r *http.Request, estado int, mensaje, en string) {
	if !a.exigirSupervision(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	ediciones, err := a.edicionesAdmin(r)
	if err != nil {
		a.errorInterno(w, r, "listar ediciones", err)
		return
	}
	posicion := slices.IndexFunc(ediciones, func(e vistaEdicionAdmin) bool { return e.ID == id })
	if posicion < 0 {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	v := ediciones[posicion]
	idioma := idiomaDe(r)
	tiene := make(map[string]bool)
	for _, c := range v.Convocatorias {
		tiene[c.Tipo] = true
	}
	for _, tipo := range []models.TipoConvocatoria{models.TipoInterpretes, models.TipoCompositores} {
		if !tiene["tipo."+string(tipo)] {
			v.TiposLibres = append(v.TiposLibres, vistaOpcion{Valor: string(tipo), Texto: a.vistas.T(idioma, "tipo."+string(tipo))})
		}
	}
	for _, otra := range ediciones {
		if otra.ID == id {
			continue
		}
		for _, c := range otra.Convocatorias {
			convocatoriaID := strings.TrimPrefix(c.URL, "/administracion/convocatorias/")
			v.Origenes = append(v.Origenes, vistaOpcion{Valor: convocatoriaID,
				Texto: fmt.Sprintf("%s · %s", otra.NombreES, a.vistas.T(idioma, c.Tipo))})
		}
	}
	p := a.pagina(r, "admin.titulo")
	p.Titulo = v.NombreES + " · " + a.vistas.T(idioma, "admin.titulo")
	p.Datos = v
	if pedido, hay := avisosDeAdmin[r.URL.Query().Get("aviso")]; hay {
		p.Aviso = &pedido
	}
	if mensaje != "" {
		// Lo escrito vuelve al formulario rechazado, para corregirlo.
		p.Formulario = views.Formulario{Valores: map[string]string{"error": mensaje, "error_en": en,
			"tipo": r.PostFormValue("tipo"), "titulo_es": r.PostFormValue("titulo_es"), "titulo_en": r.PostFormValue("titulo_en"),
			"origen": r.PostFormValue("origen")}}
		if en == "datos" {
			e := edicionDelFormulario(r, id)
			v.Numero, v.Anio, v.NombreES, v.NombreEN = e.Numero, e.Anio, e.NombreES, e.NombreEN
		}
		p.Datos = v
	}
	a.pintar(w, r, estado, "admin_edicion.html", p)
}

// accionDeEdicion es el esqueleto de toda acción sobre una edición.
func (a *App) accionDeEdicion(w http.ResponseWriter, r *http.Request, en string, operacion func(edicionID int64) (string, error)) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	destino, err := operacion(id)
	if err == nil {
		http.Redirect(w, r, destino, http.StatusSeeOther)
		return
	}
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	mensaje, ok := a.mensajeDeRechazo(r, err)
	if !ok {
		a.errorInterno(w, r, "acción sobre una edición", err)
		return
	}
	a.pintarEdicion(w, r, http.StatusUnprocessableEntity, mensaje, en)
}

// POST /administracion/ediciones/{id}   — actualiza los datos de la edición
func (a *App) actualizarEdicionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeEdicion(w, r, "datos", func(id int64) (string, error) {
		return fmt.Sprintf("/administracion/ediciones/%d?aviso=guardado", id), a.datos.ActualizarEdicion(r.Context(), a.actorDe(r), edicionDelFormulario(r, id))
	})
}

// POST /administracion/ediciones/{id}/convocatorias   — nueva convocatoria en blanco
//
// Nace en borrador con los requisitos y criterios iniciales de su tipo, como
// en la API.
func (a *App) crearConvocatoriaDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeEdicion(w, r, "convocatorias", func(edicionID int64) (string, error) {
		tipo := models.TipoConvocatoria(r.PostFormValue("tipo"))
		c := models.Convocatoria{Edicion: models.Edicion{ID: edicionID}, Tipo: tipo, Estado: models.EstadoBorrador,
			TituloES: textoDeFormulario(r.PostFormValue("titulo_es")), TituloEN: textoDeFormulario(r.PostFormValue("titulo_en")),
			MaxVideos: models.MaxVideosPermitidos}
		if tipo == models.TipoCompositores {
			c.MaxObras = models.MaxObrasCompositores
		}
		id, err := a.datos.CrearConvocatoriaConfigurada(r.Context(), a.actorDe(r), c, models.RequisitosIniciales(tipo), models.CriteriosIniciales())
		return fmt.Sprintf("/administracion/convocatorias/%d?aviso=convocatoria", id), err
	})
}

// POST /administracion/ediciones/{id}/duplicar   — nueva convocatoria copiando otra (origen)
//
// Copia textos, límites, dotaciones, requisitos y criterios; no el calendario
// ni las asignaciones, que son de cada año.
func (a *App) duplicarConvocatoriaDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeEdicion(w, r, "convocatorias", func(edicionID int64) (string, error) {
		origen, _ := strconv.ParseInt(r.PostFormValue("origen"), 10, 64)
		id, err := a.datos.DuplicarConvocatoria(r.Context(), a.actorDe(r), origen, edicionID)
		return fmt.Sprintf("/administracion/convocatorias/%d?aviso=convocatoria", id), err
	})
}

// --- Ficha de configuración de una convocatoria ----------------------------------------

// claveDeEstadoAdmin es la clave de traducción del estado de una
// convocatoria con el nombre del proceso (Publicada, Revisión documental…),
// no el que ve el público («Próxima apertura»), que además no tiene borrador.
func claveDeEstadoAdmin(estado models.EstadoConvocatoria) string {
	return "admin.estado." + string(estado)
}

// Lectores de formulario. Cada uno arma la entidad tal como la escribió el
// administrador; los usan tanto la acción (para guardarla) como el repintado
// tras un rechazo (para no perder lo escrito).

// dotacionDelFormulario lee una dotación. Una nueva nace activa; al editar,
// la casilla «activa» decide.
func dotacionDelFormulario(r *http.Request, nueva bool) models.Dotacion {
	return models.Dotacion{
		NombreES: textoDeFormulario(r.PostFormValue("nombre_es")), NombreEN: textoDeFormulario(r.PostFormValue("nombre_en")),
		DescripcionES: textoDeFormulario(r.PostFormValue("descripcion_es")), DescripcionEN: textoDeFormulario(r.PostFormValue("descripcion_en")),
		Orden: enteroDeFormulario(r.PostFormValue("orden")), Activa: nueva || r.PostFormValue("activa") != "",
	}
}

// requisitoDelFormulario lee un requisito documental. El tamaño se escribe
// en MB.
func requisitoDelFormulario(r *http.Request) models.RequisitoDocumental {
	rq := models.RequisitoDocumental{
		Clave:      strings.TrimSpace(r.PostFormValue("clave")),
		EtiquetaES: textoDeFormulario(r.PostFormValue("etiqueta_es")), EtiquetaEN: textoDeFormulario(r.PostFormValue("etiqueta_en")),
		AyudaES: textoDeFormulario(r.PostFormValue("ayuda_es")), AyudaEN: textoDeFormulario(r.PostFormValue("ayuda_en")),
		Ambito: models.AmbitoRequisito(r.PostFormValue("ambito")), Obligatorio: r.PostFormValue("obligatorio") != "",
		MimesPermitidos: r.PostForm["mimes"], TamanoMaxBytes: int64(enteroDeFormulario(r.PostFormValue("tamano_mb"))) << 20,
		MaxArchivos: enteroDeFormulario(r.PostFormValue("max_archivos")), Orden: enteroDeFormulario(r.PostFormValue("orden")),
	}
	rq.DotacionID, _ = strconv.ParseInt(r.PostFormValue("dotacion"), 10, 64)
	return rq
}

// criterioDelFormulario lee un criterio de evaluación.
func criterioDelFormulario(r *http.Request) models.CriterioEvaluacion {
	return models.CriterioEvaluacion{
		Clave:      strings.TrimSpace(r.PostFormValue("clave")),
		EtiquetaES: textoDeFormulario(r.PostFormValue("etiqueta_es")), EtiquetaEN: textoDeFormulario(r.PostFormValue("etiqueta_en")),
		DescripcionES: textoDeFormulario(r.PostFormValue("descripcion_es")), DescripcionEN: textoDeFormulario(r.PostFormValue("descripcion_en")),
		EscalaMin: enteroDeFormulario(r.PostFormValue("escala_min")), EscalaMax: enteroDeFormulario(r.PostFormValue("escala_max")),
		Obligatorio: r.PostFormValue("obligatorio") != "", Orden: enteroDeFormulario(r.PostFormValue("orden")),
	}
}

// dotacionDeVista convierte una dotación para su formulario.
func dotacionDeVista(d models.Dotacion) vistaDotacionAdmin {
	return vistaDotacionAdmin{ID: d.ID, NombreES: d.NombreES, NombreEN: d.NombreEN,
		DescripcionES: d.DescripcionES, DescripcionEN: d.DescripcionEN, Orden: d.Orden, Activa: d.Activa}
}

// criterioDeVista convierte un criterio para su formulario.
func criterioDeVista(c models.CriterioEvaluacion) vistaCriterioAdmin {
	return vistaCriterioAdmin{ID: c.ID, Clave: c.Clave, EtiquetaES: c.EtiquetaES, EtiquetaEN: c.EtiquetaEN,
		DescripcionES: c.DescripcionES, DescripcionEN: c.DescripcionEN, EscalaMin: c.EscalaMin, EscalaMax: c.EscalaMax,
		Orden: c.Orden, Obligatorio: c.Obligatorio}
}

// requisitoDeConfiguracion convierte un requisito para su formulario: con las
// dotaciones del catálogo para elegir, y los formatos admitidos por el
// sistema, marcados los que pide.
func (a *App) requisitoDeConfiguracion(idioma string, cfg models.ConfiguracionConvocatoria, rq models.RequisitoDocumental) vistaRequisitoAdmin {
	general := a.vistas.T(idioma, "admin.regla_general")
	vr := vistaRequisitoAdmin{ID: rq.ID, Clave: rq.Clave, EtiquetaES: rq.EtiquetaES, EtiquetaEN: rq.EtiquetaEN, AyudaES: rq.AyudaES,
		AyudaEN: rq.AyudaEN, Ambito: string(rq.Ambito), DotacionNombre: general, Obligatorio: rq.Obligatorio,
		TamanoMB: int((rq.TamanoMaxBytes + 1<<20 - 1) >> 20), MaxArchivos: rq.MaxArchivos, Orden: rq.Orden}
	vr.Dotaciones = append(vr.Dotaciones, vistaOpcion{Valor: "0", Texto: general, Elegida: rq.DotacionID == 0})
	for _, d := range cfg.Dotaciones {
		vr.Dotaciones = append(vr.Dotaciones, vistaOpcion{Valor: strconv.FormatInt(d.ID, 10), Texto: d.NombreES, Elegida: d.ID == rq.DotacionID})
		if d.ID == rq.DotacionID {
			vr.DotacionNombre = d.NombreES
		}
	}
	for _, mime := range models.MimesAdmitidos {
		vr.Mimes = append(vr.Mimes, vistaOpcion{Valor: mime, Texto: strings.ToUpper(strings.TrimPrefix(models.ExtensionDeMIME(mime), ".")),
			Elegida: slices.Contains(rq.MimesPermitidos, mime)})
	}
	return vr
}

// vistaDeConfiguracion arma la ficha de configuración de una convocatoria.
func (a *App) vistaDeConfiguracion(r *http.Request, cfg models.ConfiguracionConvocatoria) (*vistaConvocatoriaConfig, error) {
	idioma, sesion := idiomaDe(r), sesionDe(r)
	c := cfg.Convocatoria
	v := &vistaConvocatoriaConfig{
		ID: c.ID, Base: fmt.Sprintf("/administracion/convocatorias/%d", c.ID), Edicion: c.Edicion.Nombre(idioma),
		EdicionURL: fmt.Sprintf("/administracion/ediciones/%d", c.Edicion.ID), Publica: rutaConvocatoria(c),
		Tipo: "tipo." + string(c.Tipo), EsCompositores: c.Tipo == models.TipoCompositores, Titulo: c.Titulo(idioma),
		Estado: claveDeEstadoAdmin(c.Estado), Clase: claseEstado(c.Estado), PuedeAdministrar: models.PuedeAdministrar(sesion),
		TituloES: c.TituloES, TituloEN: c.TituloEN, BasesES: c.BasesMDES, BasesEN: c.BasesMDEN,
		MaxObras: c.MaxObras, DuracionMax: c.DuracionMaxProgramaMin, MaxVideos: c.MaxVideos,
		Jurados: cfg.Activos(models.PerfilJurado), Revisores: cfg.Activos(models.PerfilRevisorDocumental), CuotaJurados: models.CuotaJurados,
		NuevaDotacion: vistaDotacionAdmin{Activa: true, Orden: len(cfg.Dotaciones) + 1},
		NuevoCriterio: criterioDeVista(models.CriterioEvaluacion{EscalaMin: 1, EscalaMax: 5, Obligatorio: true, Orden: len(cfg.Criterios) + 1}),
		NuevaCuenta:   map[string]string{"idioma": models.IdiomaES, "perfil": string(models.PerfilJurado)},
	}
	archivos, err := a.datos.ArchivosDeConvocatoria(r.Context(), c.ID)
	if err != nil {
		return nil, err
	}
	if _, hay := archivos[models.ArchivoBasesES]; hay {
		v.BasesPDFES = v.Publica + "/bases.pdf?idioma=es"
	}
	if _, hay := archivos[models.ArchivoBasesEN]; hay {
		v.BasesPDFEN = v.Publica + "/bases.pdf?idioma=en"
	}
	if _, hay := archivos[models.ArchivoImagen]; hay {
		v.Imagen = v.Publica + "/imagen"
	}
	for _, clave := range etapasDelCalendarioAdmin {
		etapa, _ := c.Etapa(clave)
		v.Etapas = append(v.Etapas, vistaEtapaAdmin{Clave: string(clave), Nombre: "etapa." + string(clave), Inicia: aLocal(etapa.IniciaAt), Termina: aLocal(etapa.TerminaAt)})
	}
	if siguiente, hay := models.EstadoSiguiente(c.Estado); hay {
		v.Siguiente = claveDeEstadoAdmin(siguiente)
		v.SiguienteValor = string(siguiente)
		for _, e := range models.ValidarCambioDeEstado(cfg, siguiente, "", a.ahora()) {
			v.FaltaParaAvanzar = append(v.FaltaParaAvanzar, a.vistas.T(idioma, e.Clave))
		}
		v.FaltaParaAvanzar = slices.Compact(v.FaltaParaAvanzar)
	}
	if anterior, hay := models.EstadoAnterior(c.Estado); hay {
		v.Anterior, v.AnteriorValor = claveDeEstadoAdmin(anterior), string(anterior)
	}
	for _, d := range cfg.Dotaciones {
		v.Dotaciones = append(v.Dotaciones, dotacionDeVista(d))
	}
	for _, rq := range cfg.Requisitos {
		v.Requisitos = append(v.Requisitos, a.requisitoDeConfiguracion(idioma, cfg, rq))
	}
	v.NuevoRequisito = a.requisitoDeConfiguracion(idioma, cfg, models.RequisitoDocumental{Ambito: models.AmbitoObra, MimesPermitidos: []string{"application/pdf"},
		TamanoMaxBytes: 30 << 20, MaxArchivos: 1, Orden: len(cfg.Requisitos) + 1, Obligatorio: true})
	for _, cr := range cfg.Criterios {
		v.Criterios = append(v.Criterios, criterioDeVista(cr))
	}
	for _, asignacion := range cfg.Asignaciones {
		if asignacion.Activo {
			v.Asignaciones = append(v.Asignaciones, vistaAsignacionAdmin{ID: asignacion.ID, Perfil: "admin.perfil." + string(asignacion.Perfil),
				Nombre: asignacion.Usuario.Nombre + " " + asignacion.Usuario.Apellidos, Email: asignacion.Usuario.Email,
				Activada: asignacion.CuentaActivada, UsuarioID: asignacion.Usuario.ID})
		}
	}
	// Búsqueda de una cuenta para asignarla: la elige una persona, nunca el
	// sistema por coincidencia (§2.C).
	if v.Buscar = strings.TrimSpace(r.URL.Query().Get("buscar")); v.Buscar != "" && v.PuedeAdministrar {
		cuentas, _, err := a.datos.BuscarUsuarios(r.Context(), v.Buscar, 1, 15)
		if err != nil {
			return nil, err
		}
		v.Encontradas = cuentasDeVista(cuentas)
	}
	return v, nil
}

// superponerLoEnviado pone en la ficha lo que el administrador escribió en el
// formulario rechazado (en), para que lo corrija en vez de reescribirlo.
func (a *App) superponerLoEnviado(r *http.Request, cfg models.ConfiguracionConvocatoria, v *vistaConvocatoriaConfig, en string) {
	idioma := idiomaDe(r)
	// idDeRuta lee el id del elemento editado; 0 si es un alta.
	idDeRuta := func(nombre string) int64 {
		id, _ := strconv.ParseInt(r.PathValue(nombre), 10, 64)
		return id
	}
	switch en {
	case "datos":
		v.TituloES, v.TituloEN = textoDeFormulario(r.PostFormValue("titulo_es")), textoDeFormulario(r.PostFormValue("titulo_en"))
		v.MaxObras, v.DuracionMax = enteroDeFormulario(r.PostFormValue("max_obras")), enteroDeFormulario(r.PostFormValue("duracion_max"))
		v.MaxVideos = enteroDeFormulario(r.PostFormValue("max_videos"))
	case "bases":
		if r.PostForm.Has("bases_es") {
			v.BasesES, v.BasesEN = r.PostFormValue("bases_es"), r.PostFormValue("bases_en")
		}
	case "calendario":
		for i, etapa := range v.Etapas {
			v.Etapas[i].Inicia, v.Etapas[i].Termina = r.PostFormValue("inicia_"+etapa.Clave), r.PostFormValue("termina_"+etapa.Clave)
		}
	case "dotaciones":
		d := dotacionDelFormulario(r, idDeRuta("dotacion") == 0)
		if d.ID = idDeRuta("dotacion"); d.ID == 0 {
			v.NuevaDotacion = dotacionDeVista(d)
		}
		for i := range v.Dotaciones {
			if v.Dotaciones[i].ID == d.ID {
				v.Dotaciones[i] = dotacionDeVista(d)
			}
		}
	case "requisitos":
		if r.PostFormValue("accion") == "borrar" {
			return
		}
		rq := requisitoDelFormulario(r)
		if rq.ID = idDeRuta("requisito"); rq.ID == 0 {
			v.NuevoRequisito = a.requisitoDeConfiguracion(idioma, cfg, rq)
		}
		for i := range v.Requisitos {
			if v.Requisitos[i].ID == rq.ID {
				v.Requisitos[i] = a.requisitoDeConfiguracion(idioma, cfg, rq)
			}
		}
	case "criterios":
		if r.PostFormValue("accion") == "borrar" {
			return
		}
		c := criterioDelFormulario(r)
		if c.ID = idDeRuta("criterio"); c.ID == 0 {
			v.NuevoCriterio = criterioDeVista(c)
		}
		for i := range v.Criterios {
			if v.Criterios[i].ID == c.ID {
				v.Criterios[i] = criterioDeVista(c)
			}
		}
	case "cuenta":
		for _, campo := range []string{"email", "nombre", "apellidos", "idioma", "perfil"} {
			v.NuevaCuenta[campo] = r.PostFormValue(campo)
		}
	}
}

// etapasDelCalendarioAdmin son las tres etapas con fechas, en su orden.
var etapasDelCalendarioAdmin = []models.ClaveEtapa{models.EtapaPostulacion, models.EtapaRevisionDocumental, models.EtapaEvaluacion}

// cuentasDeVista convierte cuentas en renglones de una lista.
func cuentasDeVista(cuentas []models.CuentaAdmin) []vistaCuentaAdmin {
	var lista []vistaCuentaAdmin
	for _, c := range cuentas {
		vc := vistaCuentaAdmin{ID: c.Usuario.ID, Nombre: c.Usuario.Nombre + " " + c.Usuario.Apellidos, Email: c.Usuario.Email,
			Activada: c.Activada, Activa: c.Usuario.Activo}
		for _, perfil := range c.Perfiles {
			vc.Perfiles = append(vc.Perfiles, "admin.perfil."+string(perfil))
		}
		lista = append(lista, vc)
	}
	return lista
}

// GET /administracion/convocatorias/{id}   — ficha de configuración
func (a *App) convocatoriaDeAdmin(w http.ResponseWriter, r *http.Request) {
	if !a.exigirSupervision(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		a.errorInterno(w, r, "leer convocatoria", err)
		return
	}
	v, err := a.vistaDeConfiguracion(r, cfg)
	if err != nil {
		a.errorInterno(w, r, "armar la configuración", err)
		return
	}
	var aviso *views.Aviso
	if pedido, hay := avisosDeAdmin[r.URL.Query().Get("aviso")]; hay {
		aviso = &pedido
	}
	a.pintarConfiguracion(w, r, http.StatusOK, v, aviso)
}

// pintarConfiguracion envía la ficha de configuración.
func (a *App) pintarConfiguracion(w http.ResponseWriter, r *http.Request, estado int, v *vistaConvocatoriaConfig, aviso *views.Aviso) {
	p := a.pagina(r, "admin.titulo")
	p.Titulo = v.Titulo + " · " + a.vistas.T(p.Idioma, "admin.configuracion")
	p.Datos, p.Aviso = v, aviso
	a.pintar(w, r, estado, "admin_convocatoria.html", p)
}

// accionDeConvocatoria es el esqueleto de toda acción de configuración: sólo
// el administrador; carga la configuración; si sale bien, vuelve a la ficha
// con un aviso; si el modelo la rechaza, vuelve a pintarla con el motivo y
// con lo escrito junto al formulario (en).
func (a *App) accionDeConvocatoria(w http.ResponseWriter, r *http.Request, en, aviso string,
	operacion func(cfg models.ConfiguracionConvocatoria) error) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
	if err == nil {
		err = operacion(cfg)
	}
	if err == nil {
		http.Redirect(w, r, fmt.Sprintf("/administracion/convocatorias/%d?aviso=%s#%s", id, aviso, anclaDeSeccionAdmin(en)), http.StatusSeeOther)
		return
	}
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	mensaje, ok := a.mensajeDeRechazo(r, err)
	if !ok {
		a.errorInterno(w, r, "configurar la convocatoria", err)
		return
	}
	if cfg, err = a.datos.ConfiguracionDeConvocatoria(r.Context(), id); err != nil {
		a.errorInterno(w, r, "leer convocatoria", err)
		return
	}
	v, err := a.vistaDeConfiguracion(r, cfg)
	if err != nil {
		a.errorInterno(w, r, "armar la configuración", err)
		return
	}
	a.superponerLoEnviado(r, cfg, v, en)
	v.Error, v.ErrorEn = mensaje, en
	a.pintarConfiguracion(w, r, http.StatusUnprocessableEntity, v, nil)
}

// anclaDeSeccionAdmin es la sección de la ficha a la que vuelve una acción:
// el alta de una cuenta vive dentro de «asignaciones».
func anclaDeSeccionAdmin(en string) string {
	if en == "cuenta" {
		return "asignaciones"
	}
	return en
}

// POST /administracion/convocatorias/{id}/datos   — títulos y límites
func (a *App) datosDeConvocatoriaDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "datos", "guardado", func(cfg models.ConfiguracionConvocatoria) error {
		c := cfg.Convocatoria
		c.TituloES, c.TituloEN = textoDeFormulario(r.PostFormValue("titulo_es")), textoDeFormulario(r.PostFormValue("titulo_en"))
		c.MaxObras, c.DuracionMaxProgramaMin = enteroDeFormulario(r.PostFormValue("max_obras")), enteroDeFormulario(r.PostFormValue("duracion_max"))
		c.MaxVideos = enteroDeFormulario(r.PostFormValue("max_videos"))
		return a.datos.ActualizarConvocatoria(r.Context(), a.actorDe(r), c)
	})
}

// POST /administracion/convocatorias/{id}/bases   — las bases en Markdown, en los dos idiomas
func (a *App) basesDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "bases", "guardado", func(cfg models.ConfiguracionConvocatoria) error {
		c := cfg.Convocatoria
		c.BasesMDES = strings.ReplaceAll(r.PostFormValue("bases_es"), "\r\n", "\n")
		c.BasesMDEN = strings.ReplaceAll(r.PostFormValue("bases_en"), "\r\n", "\n")
		return a.datos.ActualizarConvocatoria(r.Context(), a.actorDe(r), c)
	})
}

// POST /administracion/convocatorias/{id}/archivos/{clase}   — sube las bases en PDF o la imagen
// POST /administracion/convocatorias/{id}/archivos/{clase}/quitar   — las retira
func (a *App) archivoDeConvocatoriaDesdeAdmin(quitar bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clase := models.ClaseArchivoConvocatoria(r.PathValue("clase"))
		a.accionDeConvocatoria(w, r, "archivos", "guardado", func(cfg models.ConfiguracionConvocatoria) error {
			if quitar {
				return a.datos.FijarArchivoDeConvocatoria(r.Context(), a.actorDe(r), cfg.Convocatoria.ID, clase, models.Blob{}, "", "")
			}
			tamanoMax := int64(models.TamanoMaxBasesPDF)
			if clase == models.ArchivoImagen {
				tamanoMax = models.TamanoMaxImagen
			}
			subidaInvalida := &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "documentos.subida_invalida"}}}
			s, err := a.abrirSubida(w, r, tamanoMax)
			if err != nil {
				return subidaInvalida
			}
			blob, err := a.guardarSubida(r, s, tamanoMax, func(tamano int64) []models.ErrorCampo {
				return models.ValidarArchivoDeConvocatoria(clase, s.nombre, s.mime, tamano)
			})
			if errors.Is(err, errSubidaInvalida) {
				return subidaInvalida
			}
			if err != nil {
				return err
			}
			return a.datos.FijarArchivoDeConvocatoria(r.Context(), a.actorDe(r), cfg.Convocatoria.ID, clase, blob, s.mime, s.nombre)
		})
	}
}

// POST /administracion/convocatorias/{id}/calendario   — las tres etapas (inicia_<clave>, termina_<clave>)
//
// Las fechas se escriben en hora de la Ciudad de México. Una etapa sin
// ninguna de sus dos fechas se omite: el calendario puede estar incompleto
// mientras la convocatoria es borrador. Extender un plazo es cambiar aquí su
// fecha.
func (a *App) calendarioDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "calendario", "guardado", func(cfg models.ConfiguracionConvocatoria) error {
		var etapas []models.Etapa
		for _, clave := range etapasDelCalendarioAdmin {
			inicia, termina := r.PostFormValue("inicia_"+string(clave)), r.PostFormValue("termina_"+string(clave))
			if strings.TrimSpace(inicia) == "" && strings.TrimSpace(termina) == "" {
				continue
			}
			etapas = append(etapas, models.Etapa{Clave: clave, IniciaAt: deLocal(inicia), TerminaAt: deLocal(termina)})
		}
		return a.datos.DefinirEtapas(r.Context(), a.actorDe(r), cfg.Convocatoria.ID, etapas)
	})
}

// POST /administracion/convocatorias/{id}/estado   — al estado siguiente o al anterior (destino, motivo)
//
// Es el cambio forzado del administrador: queda en la bitácora; retroceder
// exige motivo.
func (a *App) estadoDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "estado", "estado", func(cfg models.ConfiguracionConvocatoria) error {
		_, err := a.datos.CambiarEstadoConvocatoria(r.Context(), a.actorDe(r), cfg.Convocatoria.ID,
			models.EstadoConvocatoria(r.PostFormValue("destino")), textoDeFormulario(r.PostFormValue("motivo")), false)
		return err
	})
}

// elementoDeLaRuta lee el id de la ruta ({nombre}) y comprueba que esté entre
// los de esta convocatoria. Si no, la acción no procede: 404.
func elementoDeLaRuta(r *http.Request, nombre string, ids []int64) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(nombre), 10, 64)
	if err != nil || !slices.Contains(ids, id) {
		return 0, models.ErrNoEncontrado
	}
	return id, nil
}

// POST /administracion/convocatorias/{id}/dotaciones   — nueva dotación
// POST /administracion/convocatorias/{id}/dotaciones/{dotacion}   — la actualiza (o la retira: activa sin marcar)
func (a *App) dotacionDesdeAdmin(nueva bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.accionDeConvocatoria(w, r, "dotaciones", "guardado", func(cfg models.ConfiguracionConvocatoria) error {
			d := dotacionDelFormulario(r, nueva)
			d.ConvocatoriaID = cfg.Convocatoria.ID
			if !nueva {
				var ids []int64
				for _, existente := range cfg.Dotaciones {
					ids = append(ids, existente.ID)
				}
				id, err := elementoDeLaRuta(r, "dotacion", ids)
				if err != nil {
					return err
				}
				d.ID = id
			}
			_, _, err := a.datos.GuardarDotacion(r.Context(), a.actorDe(r), d)
			return err
		})
	}
}

// POST /administracion/convocatorias/{id}/requisitos   — nuevo requisito documental
// POST /administracion/convocatorias/{id}/requisitos/{requisito}   — lo actualiza, o lo borra (accion=borrar)
func (a *App) requisitoDesdeAdmin(nuevo bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aviso := "guardado"
		if r.PostFormValue("accion") == "borrar" {
			aviso = "borrado"
		}
		a.accionDeConvocatoria(w, r, "requisitos", aviso, func(cfg models.ConfiguracionConvocatoria) error {
			rq := requisitoDelFormulario(r)
			rq.ConvocatoriaID = cfg.Convocatoria.ID
			if !nuevo {
				var ids []int64
				for _, existente := range cfg.Requisitos {
					ids = append(ids, existente.ID)
				}
				id, err := elementoDeLaRuta(r, "requisito", ids)
				if err != nil {
					return err
				}
				if r.PostFormValue("accion") == "borrar" {
					return a.datos.BorrarRequisito(r.Context(), a.actorDe(r), id)
				}
				rq.ID = id
			}
			_, _, err := a.datos.GuardarRequisito(r.Context(), a.actorDe(r), rq)
			return err
		})
	}
}

// POST /administracion/convocatorias/{id}/criterios   — nuevo criterio de evaluación
// POST /administracion/convocatorias/{id}/criterios/{criterio}   — lo actualiza, o lo borra (accion=borrar)
func (a *App) criterioDesdeAdmin(nuevo bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aviso := "guardado"
		if r.PostFormValue("accion") == "borrar" {
			aviso = "borrado"
		}
		a.accionDeConvocatoria(w, r, "criterios", aviso, func(cfg models.ConfiguracionConvocatoria) error {
			c := criterioDelFormulario(r)
			c.ConvocatoriaID = cfg.Convocatoria.ID
			if !nuevo {
				var ids []int64
				for _, existente := range cfg.Criterios {
					ids = append(ids, existente.ID)
				}
				id, err := elementoDeLaRuta(r, "criterio", ids)
				if err != nil {
					return err
				}
				if r.PostFormValue("accion") == "borrar" {
					return a.datos.BorrarCriterio(r.Context(), a.actorDe(r), id)
				}
				c.ID = id
			}
			_, _, err := a.datos.GuardarCriterio(r.Context(), a.actorDe(r), c)
			return err
		})
	}
}

// POST /administracion/convocatorias/{id}/asignaciones   — asigna una cuenta existente (usuario, perfil)
func (a *App) asignarDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "asignaciones", "asignacion", func(cfg models.ConfiguracionConvocatoria) error {
		usuarioID, _ := strconv.ParseInt(r.PostFormValue("usuario"), 10, 64)
		_, err := a.datos.Asignar(r.Context(), a.actorDe(r), cfg.Convocatoria.ID, usuarioID, models.Perfil(r.PostFormValue("perfil")))
		return err
	})
}

// POST /administracion/convocatorias/{id}/asignaciones/{asignacion}/retirar   — retira el perfil
//
// No borra lo que la persona ya hizo: sólo le quita el acceso.
func (a *App) retirarAsignacionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "asignaciones", "retirada", func(cfg models.ConfiguracionConvocatoria) error {
		var ids []int64
		for _, asignacion := range cfg.Asignaciones {
			if asignacion.Activo {
				ids = append(ids, asignacion.ID)
			}
		}
		id, err := elementoDeLaRuta(r, "asignacion", ids)
		if err != nil {
			return err
		}
		_, err = a.datos.RetirarAsignacion(r.Context(), a.actorDe(r), id)
		return err
	})
}

// cuentaDelFormulario lee los datos de una cuenta interna nueva.
func cuentaDelFormulario(r *http.Request, perfilGlobal models.Perfil) models.NuevoUsuario {
	n := models.NuevoUsuario{Email: models.NormalizarEmail(r.PostFormValue("email")), Nombre: textoDeFormulario(r.PostFormValue("nombre")),
		Apellidos: textoDeFormulario(r.PostFormValue("apellidos")), Idioma: r.PostFormValue("idioma"),
		PasswordHash: models.PasswordSinDefinir, Perfil: perfilGlobal}
	if n.Idioma == "" {
		n.Idioma = models.IdiomaES
	}
	return n
}

// POST /administracion/convocatorias/{id}/cuentas   — crea una cuenta y la asigna (perfil, email, nombre…)
//
// La cuenta nace sin contraseña; la persona recibe por correo su enlace de
// activación. Antes de crearla se comprueba que la asignación procedería (la
// cuota de tres jurados, la etapa): una cuenta nueva está activa y no
// postula, así que si aquí pasa, después sólo puede fallar por una carrera
// con otra asignación simultánea; entonces la cuenta queda creada y el motivo
// se dice.
func (a *App) crearYAsignarDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeConvocatoria(w, r, "cuenta", "asignacion", func(cfg models.ConfiguracionConvocatoria) error {
		perfil := models.Perfil(r.PostFormValue("perfil"))
		if errores := models.ValidarAsignacion(cfg, models.Usuario{Activo: true}, perfil, false); len(errores) > 0 {
			return &models.ErrorValidacion{Errores: errores}
		}
		id, err := a.crearCuentaInterna(r, cuentaDelFormulario(r, ""))
		if err != nil {
			return err
		}
		_, err = a.datos.Asignar(r.Context(), a.actorDe(r), cfg.Convocatoria.ID, id, perfil)
		return err
	})
}

// --- Cuentas internas ---------------------------------------------------------------

// GET /administracion/usuarios?q=   — cuentas: búsqueda y alta
func (a *App) usuariosDeAdmin(w http.ResponseWriter, r *http.Request) {
	a.pintarUsuarios(w, r, http.StatusOK, "", nil)
}

// pintarUsuarios muestra la página de cuentas.
func (a *App) pintarUsuarios(w http.ResponseWriter, r *http.Request, estado int, mensaje string, valores map[string]string) {
	if !a.exigirSupervision(w, r) {
		return
	}
	buscar := strings.TrimSpace(r.URL.Query().Get("q"))
	cuentas, total, err := a.datos.BuscarUsuarios(r.Context(), buscar, 1, 100)
	if err != nil {
		a.errorInterno(w, r, "buscar cuentas", err)
		return
	}
	p := a.pagina(r, "admin.cuentas")
	p.Datos = vistaUsuariosAdmin{Buscar: buscar, Cuentas: cuentasDeVista(cuentas), Total: total, PuedeAdministrar: models.PuedeAdministrar(sesionDe(r)),
		Error: mensaje, Valores: valores}
	if pedido, hay := avisosDeAdmin[r.URL.Query().Get("aviso")]; hay {
		p.Aviso = &pedido
	}
	a.pintar(w, r, estado, "admin_usuarios.html", p)
}

// POST /administracion/usuarios   — nueva cuenta interna (perfil global opcional)
//
// Sin perfil global es la cuenta de un jurado o un revisor: su perfil se
// otorga después, por convocatoria.
func (a *App) crearCuentaDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	_, err := a.crearCuentaInterna(r, cuentaDelFormulario(r, models.Perfil(r.PostFormValue("perfil_global"))))
	if err == nil {
		http.Redirect(w, r, "/administracion/usuarios?aviso=cuenta&q="+url.QueryEscape(models.NormalizarEmail(r.PostFormValue("email"))), http.StatusSeeOther)
		return
	}
	mensaje, ok := a.mensajeDeRechazo(r, err)
	if !ok {
		a.errorInterno(w, r, "crear cuenta", err)
		return
	}
	valores := map[string]string{}
	for _, campo := range []string{"email", "nombre", "apellidos", "idioma", "perfil_global"} {
		valores[campo] = r.PostFormValue(campo)
	}
	a.pintarUsuarios(w, r, http.StatusUnprocessableEntity, mensaje, valores)
}

// POST /administracion/usuarios/{id}/activacion   — reenvía el enlace de activación
func (a *App) reenviarActivacionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	cuenta, err := a.datos.UsuarioPorID(r.Context(), id)
	if err == nil {
		err = a.reenviarActivacion(r, cuenta)
	}
	switch mensaje, rechazo := a.mensajeDeRechazo(r, err); {
	case err == nil:
		http.Redirect(w, r, "/administracion/usuarios?aviso=activacion&q="+url.QueryEscape(cuenta.Usuario.Email), http.StatusSeeOther)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	case rechazo:
		a.pintarUsuarios(w, r, http.StatusUnprocessableEntity, mensaje, nil)
	default:
		a.errorInterno(w, r, "reenviar activación", err)
	}
}
