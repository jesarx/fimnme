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

// Pantallas del archivo (paso 13c): el buscador de todas las ediciones con
// sus filtros, las vistas guardadas, el CSV del resultado, la descarga masiva
// en ZIP, los expedientes en PDF y la lista de descargas de la cuenta.
//
// Usa el mismo listado que la API (ListarPostulaciones) con el mismo alcance
// por perfil, que no caduca: cada quien encuentra sólo lo que puede ver
// (RN-24 y 25). Lo entra el personal: supervisión, revisores y jurados.

// --- Modelos de vista ------------------------------------------------------------

// vistaRenglonArchivo es una postulación en el resultado del buscador.
type vistaRenglonArchivo struct {
	Folio        string
	URL          string // enlace permanente por folio; vacío en un borrador
	Nombre       string
	Convocatoria string
	Edicion      int
	Tipo         string // clave de traducción
	Estado       string // clave de traducción
	Clase        string
	Enviada      time.Time
	Obras        int
	Expediente   string
}

// vistaVistaGuardada es una vista guardada, lista para abrirse.
type vistaVistaGuardada struct {
	ID     int64
	Nombre string
	URL    string
	Actual bool
}

// vistaArchivo es la página del buscador.
type vistaArchivo struct {
	Consulta string // los filtros actuales, sin página, para CSV, vistas y descargas

	Texto, EnviadaDesde, EnviadaHasta string
	Ediciones, Convocatorias, Tipos, Subcategorias, Estados, Dictamenes,
	Entidades, Paises, Nacionalidades, Generos, Estrenos,
	Subsanaciones, Pendientes, Ligas, Ordenes []vistaOpcion
	FiltrosAvanzados bool // hay alguno de los filtros plegados en uso

	Renglones             []vistaRenglonArchivo
	Total                 int
	Pagina, Paginas       int
	Anterior              string
	Siguiente             string
	CSV                   string
	Errores               []string
	Rechazo               string // motivo de una acción rechazada (vista, descarga)
	Vistas                []vistaVistaGuardada
	PuedeZIP              bool
	TiposDeDocumento      []vistaOpcion
	PuedeExpediente       bool // con una convocatoria elegida
	ConvocatoriaDelFiltro string
}

// vistaDescarga es una descarga masiva o un expediente pedido por la cuenta.
type vistaDescarga struct {
	ID        int64
	Tipo      string // clave de traducción
	Estado    string // clave de traducción
	Clase     string
	Progreso  int
	Archivos  int
	Tamano    string
	Creada    time.Time
	Expira    time.Time
	Error     string
	Bajar     string // vacío si no está lista o ya venció
	Pendiente bool
}

// vistaDescargas es la página de las descargas de la cuenta.
type vistaDescargas struct {
	Descargas  []vistaDescarga
	Pendientes bool
}

// --- Buscador -----------------------------------------------------------------

// puedeEntrarAlArchivo responde 403 si la sesión no tiene nada que consultar
// en el archivo (un postulante tiene su propia lista).
func (a *App) puedeEntrarAlArchivo(w http.ResponseWriter, r *http.Request) (models.AlcanceDeConsulta, bool) {
	alcance := models.AlcanceDe(sesionDe(r))
	if alcance.Vacio() {
		a.errorPagina(w, r, http.StatusForbidden)
		return alcance, false
	}
	return alcance, true
}

// consultaSinPagina copia los parámetros del buscador sin la página ni lo
// vacío: es lo que identifica una búsqueda (para su CSV, su vista guardada y
// sus descargas).
func consultaSinPagina(consulta url.Values) url.Values {
	limpia := url.Values{}
	for nombre, valores := range consulta {
		if nombre == "pagina" || nombre == "aviso" || len(valores) == 0 || strings.TrimSpace(valores[0]) == "" {
			continue
		}
		limpia.Set(nombre, valores[0])
	}
	return limpia
}

// convocatoriasVisibles son las convocatorias que la sesión puede consultar
// en el archivo, con su configuración (para los tipos de documento).
func (a *App) convocatoriasVisibles(r *http.Request, alcance models.AlcanceDeConsulta) ([]models.ConfiguracionConvocatoria, error) {
	var ids []int64
	if alcance.Todo {
		ediciones, err := a.datos.ListarEdiciones(r.Context())
		if err != nil {
			return nil, err
		}
		for _, e := range ediciones {
			convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), e.ID)
			if err != nil {
				return nil, err
			}
			for _, c := range convocatorias {
				ids = append(ids, c.ID)
			}
		}
	} else {
		ids = append(slices.Clone(alcance.ComoRevisor), alcance.ComoJurado...)
		slices.Sort(ids)
		ids = slices.Compact(ids)
	}
	var visibles []models.ConfiguracionConvocatoria
	for _, id := range ids {
		cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
		if err != nil {
			return nil, err
		}
		visibles = append(visibles, cfg)
	}
	return visibles, nil
}

// opcionesConTodos antepone «Todos» a unas opciones y marca la elegida.
func opcionesConTodos(todos, elegida string, opciones []vistaOpcion) []vistaOpcion {
	return opcionesCon(elegida, append([]vistaOpcion{{Valor: "", Texto: todos}}, opciones...)...)
}

// GET /archivo   — buscador del archivo histórico
//
// Parámetros: los del listado de la API (q, edicion, convocatoria, tipo,
// estado…, orden, pagina). Con htmx sólo se reemplaza el resultado.
func (a *App) archivoDelPersonal(w http.ResponseWriter, r *http.Request) {
	a.pintarArchivo(w, r, r.URL.Query(), "")
}

// pintarArchivo muestra el buscador con una consulta y, si lo hay, el motivo
// por el que se rechazó una acción (guardar una vista, pedir una descarga).
func (a *App) pintarArchivo(w http.ResponseWriter, r *http.Request, consulta url.Values, rechazo string) {
	alcance, ok := a.puedeEntrarAlArchivo(w, r)
	if !ok {
		return
	}
	sesion, idioma := sesionDe(r), idiomaDe(r)
	t := func(clave string) string { return a.vistas.T(idioma, clave) }
	actual := consultaSinPagina(consulta)
	filtro, errores := filtroDeLaConsulta(consulta)
	if filtro.Tamano == 0 {
		filtro.Tamano = models.TamanoDePaginaPorOmision
	}
	visibles, err := a.convocatoriasVisibles(r, alcance)
	if err != nil {
		a.errorInterno(w, r, "convocatorias del archivo", err)
		return
	}
	v := vistaArchivo{Consulta: actual.Encode(), Rechazo: rechazo, Texto: consulta.Get("q"), EnviadaDesde: consulta.Get("enviada_desde"),
		EnviadaHasta: consulta.Get("enviada_hasta"), PuedeZIP: models.PuedeDescargarEnMasa(sesion),
		ConvocatoriaDelFiltro: consulta.Get("convocatoria")}

	// Opciones de los filtros.
	var ediciones, convocatorias []vistaOpcion
	vistas := map[int]bool{}
	tiposDeDocumento := map[string]string{}
	var ordenDeTipos []string
	for _, cfg := range visibles {
		c := cfg.Convocatoria
		if !vistas[c.Edicion.Numero] {
			vistas[c.Edicion.Numero] = true
			ediciones = append(ediciones, vistaOpcion{Valor: strconv.Itoa(c.Edicion.Numero), Texto: c.Edicion.Nombre(idioma)})
		}
		convocatorias = append(convocatorias, vistaOpcion{Valor: strconv.FormatInt(c.ID, 10), Texto: c.Edicion.Nombre(idioma) + " · " + c.Titulo(idioma)})
		if filtro.ConvocatoriaID != 0 && c.ID != filtro.ConvocatoriaID {
			continue
		}
		for _, rq := range cfg.Requisitos {
			if _, hay := tiposDeDocumento[rq.Clave]; !hay {
				tiposDeDocumento[rq.Clave] = enIdioma(idioma, rq.EtiquetaES, rq.EtiquetaEN)
				ordenDeTipos = append(ordenDeTipos, rq.Clave)
			}
		}
	}
	for _, clave := range ordenDeTipos {
		v.TiposDeDocumento = append(v.TiposDeDocumento, vistaOpcion{Valor: clave, Texto: tiposDeDocumento[clave]})
	}
	v.PuedeExpediente = filtro.ConvocatoriaID != 0 && slices.ContainsFunc(visibles, func(c models.ConfiguracionConvocatoria) bool {
		return c.Convocatoria.ID == filtro.ConvocatoriaID
	})
	todos, todas := t("revision.todos"), t("revision.todas")
	v.Ediciones = opcionesConTodos(todas, consulta.Get("edicion"), ediciones)
	v.Convocatorias = opcionesConTodos(todas, consulta.Get("convocatoria"), convocatorias)
	v.Tipos = opcionesConTodos(todos, consulta.Get("tipo"), []vistaOpcion{
		{Valor: string(models.TipoInterpretes), Texto: t("tipo.interpretes")}, {Valor: string(models.TipoCompositores), Texto: t("tipo.compositores")}})
	v.Subcategorias = opcionesConTodas(todas, consulta.Get("subcategoria"), []vistaOpcion{
		{Valor: string(models.SubcategoriaSolista), Texto: t("subcategoria.solista")}, {Valor: string(models.SubcategoriaEnsamble), Texto: t("subcategoria.ensamble")}})
	var estados []vistaOpcion
	if alcance.Todo {
		estados = append(estados, vistaOpcion{Valor: string(models.PostulacionBorrador), Texto: t("estado.postulacion.borrador")})
	}
	for _, estado := range estadosEnRevision {
		estados = append(estados, vistaOpcion{Valor: string(estado), Texto: t("estado.postulacion." + string(estado))})
	}
	v.Estados = opcionesConTodos(todos, consulta.Get("estado"), estados)
	if models.PuedeSupervisar(sesion) {
		var dictamenes []vistaOpcion
		for _, d := range dictamenesFinales {
			dictamenes = append(dictamenes, vistaOpcion{Valor: d, Texto: t("resultado." + d)})
		}
		v.Dictamenes = opcionesConTodos(todos, consulta.Get("dictamen"), dictamenes)
	}
	v.Entidades = opcionesConTodas(todas, consulta.Get("entidad_residencia"), entidades())
	v.Paises = opcionesConTodos(todos, consulta.Get("pais"), a.paises(idioma))
	v.Nacionalidades = opcionesConTodas(todas, consulta.Get("nacionalidad"), a.paises(idioma))
	var generos, estrenos []vistaOpcion
	for _, g := range []string{"mujer", "hombre", "no_binario", "otro", "prefiero_no_decir"} {
		generos = append(generos, vistaOpcion{Valor: g, Texto: t("opcion." + g)})
	}
	for _, e := range []string{"estreno_mundial", "estreno_mexico", "no_es_estreno"} {
		estrenos = append(estrenos, vistaOpcion{Valor: e, Texto: t("opcion." + e)})
	}
	v.Generos = opcionesConTodos(todos, consulta.Get("genero"), generos)
	v.Estrenos = opcionesConTodos(todos, consulta.Get("estreno"), estrenos)
	siNo := []vistaOpcion{{Valor: "si", Texto: t("archivo.si")}, {Valor: "no", Texto: t("archivo.no")}}
	v.Subsanaciones = opcionesConTodas(todas, consulta.Get("con_subsanaciones"), siNo)
	v.Pendientes = opcionesConTodas(todas, consulta.Get("documentos_pendientes"), siNo)
	v.Ligas = opcionesConTodas(todas, consulta.Get("ligas_con_problema"), siNo)
	ordenes := []vistaOpcion{{Valor: "folio", Texto: t("supervision.orden_folio")}, {Valor: "-enviada", Texto: t("archivo.orden_recientes")},
		{Valor: "enviada", Texto: t("archivo.orden_antiguas")}, {Valor: "nombre", Texto: t("supervision.orden_nombre")},
		{Valor: "estado", Texto: t("supervision.orden_estado")}, {Valor: "-obras", Texto: t("archivo.orden_obras")},
		{Valor: "-duracion", Texto: t("archivo.orden_duracion")}}
	v.Ordenes = opcionesCon(primeroNoVacio(consulta.Get("orden"), "folio"), ordenes...)
	for _, avanzado := range []string{"subcategoria", "dictamen", "entidad_residencia", "pais", "nacionalidad", "genero", "estreno",
		"enviada_desde", "enviada_hasta", "con_subsanaciones", "documentos_pendientes", "ligas_con_problema"} {
		v.FiltrosAvanzados = v.FiltrosAvanzados || actual.Has(avanzado)
	}

	// El resultado.
	if len(errores) > 0 {
		for _, e := range errores {
			v.Errores = append(v.Errores, t("archivo.filtro_"+e.Campo)+": "+t(e.Clave))
		}
		v.Errores = slices.Compact(v.Errores)
	} else {
		renglones, total, err := a.datos.ListarPostulaciones(r.Context(), alcance, filtro)
		if err != nil {
			a.errorInterno(w, r, "listar el archivo", err)
			return
		}
		v.Total, v.Pagina = total, filtro.Pagina
		v.Paginas = max(1, (total+filtro.Tamano-1)/filtro.Tamano)
		pagina := func(n int) string {
			copia := consultaSinPagina(consulta)
			if n > 1 {
				copia.Set("pagina", strconv.Itoa(n))
			}
			return "/archivo?" + copia.Encode()
		}
		if v.Pagina > 1 {
			v.Anterior = pagina(v.Pagina - 1)
		}
		if v.Pagina < v.Paginas {
			v.Siguiente = pagina(v.Pagina + 1)
		}
		paraCSV := consultaSinPagina(consulta)
		paraCSV.Set("formato", "csv")
		v.CSV = "/api/v1/archivo?" + paraCSV.Encode()
		for _, renglon := range renglones {
			fila := vistaRenglonArchivo{Folio: renglon.Folio, Nombre: renglon.Nombre, Convocatoria: renglon.Convocatoria,
				Edicion: renglon.Edicion, Tipo: "tipo." + string(renglon.Tipo), Estado: "estado.postulacion." + string(renglon.Estado),
				Clase: clasesEnRevision[renglon.Estado], Enviada: renglon.EnviadaAt, Obras: renglon.Obras,
				Expediente: fmt.Sprintf("/api/v1/postulaciones/%d/expediente", renglon.ID)}
			if renglon.Folio != "" {
				fila.URL = "/postulaciones/" + url.PathEscape(renglon.Folio)
			}
			v.Renglones = append(v.Renglones, fila)
		}
	}

	// Vistas guardadas de la cuenta (sólo las del archivo).
	guardadas, err := a.datos.VistasGuardadas(r.Context(), sesion.Usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "vistas guardadas", err)
		return
	}
	for _, vg := range guardadas {
		ruta := vg.Ruta
		if resto, deLaAPI := strings.CutPrefix(ruta, "/api/v1/archivo"); deLaAPI {
			ruta = "/archivo" + resto
		}
		if ruta != "/archivo" && !strings.HasPrefix(ruta, "/archivo?") {
			continue
		}
		v.Vistas = append(v.Vistas, vistaVistaGuardada{ID: vg.ID, Nombre: vg.Nombre, URL: ruta, Actual: ruta == "/archivo?"+v.Consulta})
	}

	p := a.pagina(r, "archivo.titulo")
	p.Datos = v
	if pedido, hay := avisosDelArchivo[consulta.Get("aviso")]; hay {
		p.Aviso = &pedido
	}
	estado := http.StatusOK
	if len(v.Errores) > 0 || rechazo != "" {
		estado = http.StatusUnprocessableEntity
	}
	a.pintar(w, r, estado, "archivo.html", p)
}

// opcionesConTodas es opcionesConTodos, con «Todas».
func opcionesConTodas(todas, elegida string, opciones []vistaOpcion) []vistaOpcion {
	return opcionesConTodos(todas, elegida, opciones)
}

// avisosDelArchivo son los mensajes que una redirección pide mostrar.
var avisosDelArchivo = map[string]views.Aviso{
	"vista":         {Tipo: "exito", Clave: "archivo.aviso_vista"},
	"vista_borrada": {Tipo: "exito", Clave: "archivo.aviso_vista_borrada"},
}

// --- Vistas guardadas -----------------------------------------------------------

// POST /archivo/vistas   — guarda la búsqueda actual con nombre (nombre, consulta)
//
// La ruta se arma aquí a partir de los filtros, siempre dentro de /archivo:
// una vista guardada no puede llevar a otra parte.
func (a *App) guardarVistaDelArchivo(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.puedeEntrarAlArchivo(w, r); !ok {
		return
	}
	consulta, err := url.ParseQuery(r.PostFormValue("consulta"))
	if err != nil {
		consulta = url.Values{}
	}
	limpia := consultaSinPagina(consulta)
	ruta := "/archivo"
	if len(limpia) > 0 {
		ruta += "?" + limpia.Encode()
	}
	_, err = a.datos.GuardarVista(r.Context(), sesionDe(r).Usuario.ID, r.PostFormValue("nombre"), ruta)
	if err == nil {
		destino := ruta + "&aviso=vista"
		if len(limpia) == 0 {
			destino = ruta + "?aviso=vista"
		}
		http.Redirect(w, r, destino, http.StatusSeeOther)
		return
	}
	a.rechazoEnElArchivo(w, r, limpia, err)
}

// rechazoEnElArchivo vuelve al buscador, con su búsqueda, y el motivo de
// un rechazo del modelo.
func (a *App) rechazoEnElArchivo(w http.ResponseWriter, r *http.Request, consulta url.Values, err error) {
	mensaje, rechazo := a.mensajeDeRechazo(r, err)
	if !rechazo {
		a.errorInterno(w, r, "acción en el archivo", err)
		return
	}
	if errors.Is(err, models.ErrDuplicado) {
		mensaje = a.vistas.T(idiomaDe(r), "archivo.vista_duplicada")
	}
	a.pintarArchivo(w, r, consulta, mensaje)
}

// POST /archivo/vistas/{vista}/borrar   — borra una vista guardada propia
func (a *App) borrarVistaDelArchivo(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.puedeEntrarAlArchivo(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("vista"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	switch err := a.datos.BorrarVista(r.Context(), sesionDe(r).Usuario.ID, id); {
	case err == nil:
		http.Redirect(w, r, "/archivo?aviso=vista_borrada", http.StatusSeeOther)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	default:
		a.errorInterno(w, r, "borrar vista", err)
	}
}

// --- Descargas masivas y expedientes -------------------------------------------------

// consultaDelFormulario lee los filtros que un formulario del archivo manda
// en «consulta» (los de la búsqueda en pantalla).
func consultaDelFormulario(r *http.Request) url.Values {
	consulta, err := url.ParseQuery(r.PostFormValue("consulta"))
	if err != nil {
		return url.Values{}
	}
	return consultaSinPagina(consulta)
}

// POST /archivo/descargas   — descarga masiva en ZIP de la búsqueda (consulta, tipos)
//
// La misma tarea que la API: se arma en segundo plano y se avisa por correo.
// Sólo la supervisión y los revisores; el jurado baja documento por documento.
func (a *App) pedirDescargaDesdeElArchivo(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	if !models.PuedeDescargarEnMasa(sesion) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.errorPagina(w, r, http.StatusBadRequest)
		return
	}
	consulta := consultaDelFormulario(r)
	filtro, errores := filtroDeLaConsulta(consulta)
	var err error
	if len(errores) > 0 {
		err = &models.ErrorValidacion{Errores: errores}
	} else {
		_, err = a.datos.EncolarExportacion(r.Context(), a.actorDe(r), models.TareaZIP,
			parametrosDeDescargaMasiva(sesion, filtro, r.PostForm["tipos"]))
	}
	if err != nil {
		a.rechazoEnElArchivo(w, r, consulta, err)
		return
	}
	http.Redirect(w, r, "/archivo/descargas?aviso=encolada", http.StatusSeeOther)
}

// POST /archivo/expedientes   — expediente en PDF de una convocatoria (consulta, idioma, fotografia, hoja)
//
// Recortado al generarse según los permisos que la persona tenga entonces.
func (a *App) pedirExpedienteDesdeElArchivo(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.puedeEntrarAlArchivo(w, r); !ok {
		return
	}
	consulta := consultaDelFormulario(r)
	filtro, errores := filtroDeLaConsulta(consulta)
	if filtro.ConvocatoriaID == 0 {
		errores = append(errores, models.ErrorCampo{Campo: "convocatoria", Clave: "error.convocatoria_requerida"})
	}
	idioma := r.PostFormValue("idioma")
	if idioma != models.IdiomaES && idioma != models.IdiomaEN {
		idioma = idiomaDe(r)
	}
	var err error
	if len(errores) > 0 {
		err = &models.ErrorValidacion{Errores: errores}
	} else {
		_, err = a.datos.EncolarExportacion(r.Context(), a.actorDe(r), models.TareaExpediente, models.ParametrosDeExportacion{
			Filtro: filtro, Idioma: idioma, SinFotografia: r.PostFormValue("fotografia") == "no", HojaDeCalificacion: r.PostFormValue("hoja") == "si"})
	}
	if err != nil {
		a.rechazoEnElArchivo(w, r, consulta, err)
		return
	}
	http.Redirect(w, r, "/archivo/descargas?aviso=encolada", http.StatusSeeOther)
}

// tamanoDeDescarga escribe un tamaño en bytes de forma legible.
func tamanoDeDescarga(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	return tamanoLegible(bytes)
}

// GET /archivo/descargas   — las descargas masivas y expedientes de la cuenta
//
// Mientras alguna se arma, la página se actualiza sola (htmx); sin
// JavaScript, con el enlace «Actualizar».
func (a *App) descargasDelArchivo(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.puedeEntrarAlArchivo(w, r); !ok {
		return
	}
	tareas, err := a.datos.ExportacionesDe(r.Context(), sesionDe(r).Usuario.ID, 30)
	if err != nil {
		a.errorInterno(w, r, "descargas de la cuenta", err)
		return
	}
	ahora := a.ahora()
	var v vistaDescargas
	for _, tarea := range tareas {
		d := vistaDescarga{ID: tarea.ID, Tipo: "archivo.tarea_" + tarea.Tipo, Estado: "archivo.estado_" + tarea.Estado, Progreso: tarea.Progreso,
			Archivos: tarea.Archivos, Tamano: tamanoDeDescarga(tarea.Bytes), Creada: tarea.CreadoAt, Expira: tarea.ExpiraAt, Error: tarea.MensajeError}
		switch tarea.Estado {
		case models.ExportacionEncolada, models.ExportacionProcesando:
			d.Clase, d.Pendiente, v.Pendientes = "proceso", true, true
		case models.ExportacionLista:
			if ahora.After(tarea.ExpiraAt) {
				d.Estado, d.Clase = "archivo.estado_vencida", "cerrada"
			} else {
				d.Clase, d.Bajar = "abierta", fmt.Sprintf("/api/v1/exportaciones/%d/archivo", tarea.ID)
			}
		default:
			d.Clase = "cerrada"
		}
		v.Descargas = append(v.Descargas, d)
	}
	p := a.pagina(r, "archivo.descargas")
	p.Datos = v
	if r.URL.Query().Get("aviso") == "encolada" {
		p.Aviso = &views.Aviso{Tipo: "exito", Clave: "archivo.aviso_encolada"}
	}
	a.pintar(w, r, http.StatusOK, "archivo_descargas.html", p)
}

// --- Enlace permanente por folio para el personal ------------------------------------

// destinoDelPersonal dice adónde lleva el enlace permanente de una
// postulación (/postulaciones/{folio}) a quien no es su titular: a la vista
// de revisión, si puede verla; a la ficha de evaluación, si es jurado y la
// postulación llegó a evaluación. Vacío si no puede verla: entonces 404, como
// si no existiera.
func destinoDelPersonal(sesion *models.Sesion, p models.Postulacion, tipo models.TipoConvocatoria) string {
	if !models.PuedeVerPostulacion(sesion, p) || p.Estado == models.PostulacionBorrador {
		return ""
	}
	if models.PuedeVerRevision(sesion, p.ConvocatoriaID) {
		return fmt.Sprintf("/revision/postulaciones/%d", p.ID)
	}
	if models.PuedeEvaluar(sesion, p.ConvocatoriaID) && models.EnEvaluacion(p.Estado) {
		destino := fmt.Sprintf("/evaluacion/postulaciones/%d", p.ID)
		// En Compositores se califica cada obra: se abre la primera.
		if tipo == models.TipoCompositores && len(p.Obras) > 0 {
			destino += fmt.Sprintf("?obra=%d", p.Obras[0].ID)
		}
		return destino
	}
	return ""
}
