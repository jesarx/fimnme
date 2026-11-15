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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pantallas del jurado (§5.6, paso 12b): su lista de trabajo y la ficha de
// evaluación de cada unidad, con el visor de documentos.
//
// AISLAMIENTO ENTRE JURADOS. Como en la API, estas páginas sólo leen y
// escriben evaluaciones por a.datos.ParaJurado(id de la sesión), que no tiene
// modo de devolver nada de otro jurado; nunca usan Supervision(). Tampoco
// muestran nada de la revisión documental (dictámenes, observaciones,
// descartes). Una prueba barre todas estas páginas con la sesión de cada
// jurado buscando lo que escribieron los demás.

// --- Modelos de vista ----------------------------------------------------------

// vistaConvocatoriaDelJurado es una convocatoria en la portada de la evaluación.
type vistaConvocatoriaDelJurado struct {
	URL        string
	Titulo     string
	Edicion    string
	Estado     string
	Clase      string
	Enviadas   int
	Total      int
	Pendientes int
}

// vistaUnidadDelJurado es una unidad en la lista de trabajo.
type vistaUnidadDelJurado struct {
	URL    string
	Folio  string
	Nombre string
	Obra   string // título de la obra (Compositores)
	Estado string // clave de traducción: pendiente, borrador, enviada o excusada
	Clase  string
	Actual bool
}

// vistaListaDelJurado es la lista de trabajo de una convocatoria (RF-EVA-07).
type vistaListaDelJurado struct {
	Titulo, Edicion    string
	Abierta            bool // la convocatoria está en evaluación: puede calificar
	Unidades           []vistaUnidadDelJurado
	Enviadas           int
	Excusadas          int
	Total              int
	Pendientes         int
	SiguientePendiente string
}

// vistaCriterio es un criterio con su escala, listo para el formulario.
type vistaCriterio struct {
	Clave       string
	Etiqueta    string
	Descripcion string
	Obligatorio bool
	Valores     []int
	Elegido     int // 0 = sin calificar
	Error       string
}

// vistaFicha es la ficha de evaluación de una unidad.
type vistaFicha struct {
	PostulacionID                           int64
	Base                                    string // ruta de la ficha
	Lista                                   string // ruta de la lista de trabajo
	Titulo                                  string
	Edicion                                 string
	Folio                                   string
	Nombre                                  string
	Obra                                    string
	EsObra                                  bool  // Compositores: se califica cada obra
	ObraID                                  int64 // la obra calificada; 0 en Intérpretes
	Abierta                                 bool
	Secciones                               []vistaPasoResumido
	Documentos                              []vistaGrupoDocumentos
	Criterios                               []vistaCriterio
	Observaciones                           string
	Enviada                                 bool
	EnviadaAt                               time.Time
	Guardada                                time.Time
	Excusado                                bool
	MotivoExcusa                            string
	ErrorExcusa                             string
	Error                                   string
	Anterior, Siguiente, SiguientePendiente string
	Posicion, Total                         int
}

// --- Portada y lista de trabajo -------------------------------------------------

// estadoDeUnidad describe en qué va la evaluación propia de una unidad.
func estadoDeUnidad(u models.UnidadPropia) (clave, clase string) {
	switch {
	case u.Excusada:
		return "evaluacion.unidad_excusada", "cerrada"
	case u.Enviada:
		return "evaluacion.unidad_enviada", "abierta"
	case u.Iniciada:
		return "evaluacion.unidad_borrador", "proceso"
	}
	return "evaluacion.unidad_pendiente", "proxima"
}

// urlDeUnidad es la ficha de una unidad: la postulación y, en Compositores, la obra.
func urlDeUnidad(u models.UnidadDeEvaluacion) string {
	ruta := fmt.Sprintf("/evaluacion/postulaciones/%d", u.PostulacionID)
	if u.ObraID != 0 {
		ruta += fmt.Sprintf("?obra=%d", u.ObraID)
	}
	return ruta
}

// avanceDe cuenta, de la lista propia, lo enviado, lo excusado y lo que
// falta. Una unidad de la que se excusó no está pendiente.
func avanceDe(unidades []models.UnidadPropia) (enviadas, excusadas, pendientes int, siguiente string) {
	for _, u := range unidades {
		switch {
		case u.Excusada:
			excusadas++
		case u.Enviada:
			enviadas++
		default:
			pendientes++
			if siguiente == "" {
				siguiente = urlDeUnidad(u.UnidadDeEvaluacion)
			}
		}
	}
	return enviadas, excusadas, pendientes, siguiente
}

// GET /evaluacion   — las convocatorias en que la sesión es jurado
func (a *App) portadaDeEvaluacion(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	ids := models.EvaluacionesDe(sesion)
	if len(ids) == 0 {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	jurado := a.datos.ParaJurado(sesion.Usuario.ID)
	idioma := idiomaDe(r)
	var lista []vistaConvocatoriaDelJurado
	for _, id := range ids {
		cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
		if err != nil {
			a.errorInterno(w, r, "leer convocatoria", err)
			return
		}
		unidades, err := jurado.Unidades(r.Context(), id)
		if err != nil {
			a.errorInterno(w, r, "leer la lista de trabajo", err)
			return
		}
		enviadas, _, pendientes, _ := avanceDe(unidades)
		c := cfg.Convocatoria
		lista = append(lista, vistaConvocatoriaDelJurado{URL: fmt.Sprintf("/evaluacion/convocatorias/%d", id), Titulo: c.Titulo(idioma),
			Edicion: c.Edicion.Nombre(idioma), Estado: "estado.convocatoria." + string(c.Estado), Clase: claseEstado(c.Estado),
			Enviadas: enviadas, Total: len(unidades), Pendientes: pendientes})
	}
	p := a.pagina(r, "evaluacion.titulo")
	p.Datos = lista
	a.pintar(w, r, http.StatusOK, "evaluacion_portada.html", p)
}

// GET /evaluacion/convocatorias/{id}   — lista de trabajo del jurado
func (a *App) listaDelJurado(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	sesion := sesionDe(r)
	if err != nil || !models.PuedeEvaluar(sesion, convocatoriaID) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if err != nil {
		a.errorInterno(w, r, "leer convocatoria", err)
		return
	}
	unidades, err := a.datos.ParaJurado(sesion.Usuario.ID).Unidades(r.Context(), convocatoriaID)
	if err != nil {
		a.errorInterno(w, r, "leer la lista de trabajo", err)
		return
	}
	idioma := idiomaDe(r)
	v := vistaListaDelJurado{Titulo: cfg.Convocatoria.Titulo(idioma), Edicion: cfg.Convocatoria.Edicion.Nombre(idioma),
		Abierta: cfg.Convocatoria.Estado == models.EstadoEvaluacion, Total: len(unidades)}
	v.Enviadas, v.Excusadas, v.Pendientes, v.SiguientePendiente = avanceDe(unidades)
	for _, u := range unidades {
		estado, clase := estadoDeUnidad(u)
		v.Unidades = append(v.Unidades, vistaUnidadDelJurado{URL: urlDeUnidad(u.UnidadDeEvaluacion), Folio: u.Folio, Nombre: u.Nombre,
			Obra: u.Obra, Estado: estado, Clase: clase})
	}
	p := a.pagina(r, "evaluacion.titulo")
	p.Titulo = a.vistas.T(idioma, "evaluacion.lista") + " · " + v.Titulo
	p.Datos = v
	a.pintar(w, r, http.StatusOK, "evaluacion_lista.html", p)
}

// --- Ficha de evaluación -----------------------------------------------------------

// unidadDelJurado es una unidad abierta por su jurado: la postulación en
// sólo lectura, su lugar en la lista propia y SU evaluación.
type unidadDelJurado struct {
	ab         *postulacionAbierta
	jurado     models.JuradoRepo
	unidades   []models.UnidadPropia
	posicion   int // en la lista propia
	evaluacion models.Evaluacion
}

// abrirUnidad carga la unidad de la ruta: la postulación de {id} y, en
// Compositores, la obra de ?obra=. Si la sesión no es jurado de su
// convocatoria, o la unidad no está en su lista de trabajo, responde 404.
func (a *App) abrirUnidad(w http.ResponseWriter, r *http.Request) (*unidadDelJurado, bool) {
	sesion := sesionDe(r)
	id, errID := strconv.ParseInt(r.PathValue("id"), 10, 64)
	obraID, errObra := enteroOpcional(r.URL.Query().Get("obra"))
	if r.Method == http.MethodPost {
		obraID, errObra = enteroOpcional(r.PostFormValue("obra"))
	}
	if errID != nil || errObra != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return nil, false
	}
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if err == nil && (!models.PuedeVerPostulacion(sesion, p) || !models.PuedeEvaluar(sesion, p.ConvocatoriaID)) {
		err = models.ErrNoEncontrado
	}
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		a.errorInterno(w, r, "leer la postulación", err)
		return nil, false
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), p.ConvocatoriaID)
	if err != nil {
		a.errorInterno(w, r, "leer la convocatoria", err)
		return nil, false
	}
	jurado := a.datos.ParaJurado(sesion.Usuario.ID)
	unidades, err := jurado.Unidades(r.Context(), p.ConvocatoriaID)
	if err != nil {
		a.errorInterno(w, r, "leer la lista de trabajo", err)
		return nil, false
	}
	posicion := slices.IndexFunc(unidades, func(u models.UnidadPropia) bool { return u.PostulacionID == id && u.ObraID == obraID })
	if posicion < 0 {
		a.errorPagina(w, r, http.StatusNotFound)
		return nil, false
	}
	evaluacion, err := jurado.Evaluacion(r.Context(), id, obraID)
	if err != nil {
		a.errorInterno(w, r, "leer la evaluación propia", err)
		return nil, false
	}
	// La postulación, en sólo lectura y con los campos extendidos que el
	// jurado puede ver.
	ab := &postulacionAbierta{p: p, cfg: cfg, formulario: models.FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria),
		motivo: "error.postulacion_enviada", senalados: map[string]string{}}
	visibles := models.ExtendidosVisibles(&ab.p, cfg.Convocatoria.Tipo, cfg.Extendidos, sesion)
	ab.extendidos, ab.extendidosDeObra = models.ExtendidosDePostulacion(visibles, cfg.Convocatoria.Tipo, p.Subcategoria)
	return &unidadDelJurado{ab: ab, jurado: jurado, unidades: unidades, posicion: posicion, evaluacion: evaluacion}, true
}

// vistaDeFicha arma la ficha de evaluación de una unidad.
func (a *App) vistaDeFicha(r *http.Request, u *unidadDelJurado) *vistaFicha {
	idioma := idiomaDe(r)
	c := u.ab.cfg.Convocatoria
	propia := u.unidades[u.posicion]
	v := &vistaFicha{
		PostulacionID: propia.PostulacionID,
		Base:          urlDeUnidad(propia.UnidadDeEvaluacion), Lista: fmt.Sprintf("/evaluacion/convocatorias/%d", c.ID),
		Titulo: c.Titulo(idioma), Edicion: c.Edicion.Nombre(idioma), Folio: propia.Folio, Nombre: propia.Nombre, Obra: propia.Obra,
		EsObra: propia.ObraID != 0, ObraID: propia.ObraID, Abierta: c.Estado == models.EstadoEvaluacion,
		Observaciones: u.evaluacion.Observaciones, Enviada: u.evaluacion.Enviada, EnviadaAt: u.evaluacion.EnviadaAt,
		Guardada: u.evaluacion.ActualizadoAt, Excusado: u.evaluacion.Excusado, MotivoExcusa: u.evaluacion.MotivoExcusa,
		Posicion: u.posicion + 1, Total: len(u.unidades),
	}
	if u.posicion > 0 {
		v.Anterior = urlDeUnidad(u.unidades[u.posicion-1].UnidadDeEvaluacion)
	}
	if u.posicion+1 < len(u.unidades) {
		v.Siguiente = urlDeUnidad(u.unidades[u.posicion+1].UnidadDeEvaluacion)
	}
	for _, otra := range u.unidades {
		if !otra.Enviada && !otra.Excusada && otra.UnidadDeEvaluacion != propia.UnidadDeEvaluacion {
			v.SiguientePendiente = urlDeUnidad(otra.UnidadDeEvaluacion)
			break
		}
	}
	// La postulación entera, en sólo lectura. En Compositores, de las obras
	// sólo la que se califica: cada obra es una unidad aparte.
	for _, seccion := range models.SeccionesDe(c.Tipo) {
		if seccion == models.SeccionEnvio || seccion == models.SeccionDocumentos {
			continue
		}
		paso := a.vistaDe(r, u.ab, seccion)
		resumen := vistaPasoResumido{Seccion: seccion, Titulo: a.vistas.T(idioma, "seccion."+seccion), Grupos: paso.Grupos}
		for _, obra := range paso.Obras {
			if !v.EsObra || obra.ID == v.ObraID {
				resumen.Obras = append(resumen.Obras, obra)
			}
		}
		if paso.TieneIntegrantes {
			resumen.Integrantes = paso.Integrantes
		}
		if paso.TieneVideos {
			for _, video := range paso.Videos {
				if video.URL != "" {
					resumen.Videos = append(resumen.Videos, video)
				}
			}
		}
		v.Secciones = append(v.Secciones, resumen)
	}
	for _, grupo := range a.documentosDeVista(idioma, u.ab) {
		if v.EsObra && grupo.ID != "docs-postulacion" && grupo.ID != fmt.Sprintf("docs-obra-%d", v.ObraID) {
			continue
		}
		v.Documentos = append(v.Documentos, grupo)
	}
	// Los criterios, con lo que ya calificó.
	for _, criterio := range u.ab.cfg.Criterios {
		vc := vistaCriterio{Clave: criterio.Clave, Etiqueta: criterio.EtiquetaES, Descripcion: criterio.DescripcionES,
			Obligatorio: criterio.Obligatorio, Elegido: u.evaluacion.Puntajes[criterio.Clave]}
		if idioma == models.IdiomaEN {
			if criterio.EtiquetaEN != "" {
				vc.Etiqueta = criterio.EtiquetaEN
			}
			if criterio.DescripcionEN != "" {
				vc.Descripcion = criterio.DescripcionEN
			}
		}
		for valor := criterio.EscalaMin; valor <= criterio.EscalaMax; valor++ {
			vc.Valores = append(vc.Valores, valor)
		}
		v.Criterios = append(v.Criterios, vc)
	}
	return v
}

// avisosDeFicha son los mensajes que una redirección pide mostrar.
var avisosDeFicha = map[string]views.Aviso{
	"borrador": {Tipo: "exito", Clave: "evaluacion.aviso_borrador"},
	"enviada":  {Tipo: "exito", Clave: "evaluacion.aviso_enviada"},
	"excusa":   {Tipo: "exito", Clave: "evaluacion.aviso_excusa"},
	"retirada": {Tipo: "exito", Clave: "evaluacion.aviso_retirada"},
}

// pintarFicha envía la ficha de evaluación.
func (a *App) pintarFicha(w http.ResponseWriter, r *http.Request, estado int, v *vistaFicha, aviso *views.Aviso) {
	p := a.pagina(r, "evaluacion.titulo")
	p.Titulo = v.Folio + " · " + a.vistas.T(p.Idioma, "evaluacion.ficha")
	p.Datos, p.Aviso = v, aviso
	a.pintar(w, r, estado, "evaluacion_ficha.html", p)
}

// GET /evaluacion/postulaciones/{id}?obra=   — ficha de evaluación de una unidad
func (a *App) fichaDeEvaluacion(w http.ResponseWriter, r *http.Request) {
	u, ok := a.abrirUnidad(w, r)
	if !ok {
		return
	}
	var aviso *views.Aviso
	if pedido, hay := avisosDeFicha[r.URL.Query().Get("aviso")]; hay {
		aviso = &pedido
	}
	a.pintarFicha(w, r, http.StatusOK, a.vistaDeFicha(r, u), aviso)
}

// tras arma la dirección a la que vuelve una acción sobre una unidad.
func tras(u *unidadDelJurado, aviso, ancla string) string {
	ruta := urlDeUnidad(u.unidades[u.posicion].UnidadDeEvaluacion)
	separador := "?"
	if strings.Contains(ruta, "?") {
		separador = "&"
	}
	return ruta + separador + "aviso=" + aviso + "#" + ancla
}

// POST /evaluacion/postulaciones/{id}/evaluacion   — guarda la evaluación propia
//
// Campos: obra (en Compositores), puntaje_<criterio> por cada criterio,
// observaciones y accion («borrador» o «enviar»). En borrador puede quedar
// incompleta; enviarla exige todos los criterios obligatorios. Es el mismo
// guardado de la API, por el acceso de ESTE jurado.
func (a *App) guardarEvaluacionDesdeLaFicha(w http.ResponseWriter, r *http.Request) {
	u, ok := a.abrirUnidad(w, r)
	if !ok {
		return
	}
	entrada := models.EntradaEvaluacion{ObraID: u.unidades[u.posicion].ObraID, Puntajes: map[string]int{},
		Observaciones: textoDeFormulario(r.PostFormValue("observaciones")), Enviar: r.PostFormValue("accion") == "enviar"}
	for _, criterio := range u.ab.cfg.Criterios {
		if texto := r.PostFormValue("puntaje_" + criterio.Clave); texto != "" {
			puntaje, err := strconv.Atoi(texto)
			if err != nil {
				puntaje = -1 // fuera de toda escala: el modelo lo rechaza
			}
			entrada.Puntajes[criterio.Clave] = puntaje
		}
	}
	_, err := u.jurado.Guardar(r.Context(), a.actorDe(r), u.ab.p.ID, entrada)
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
		aviso := "borrador"
		if entrada.Enviar || u.evaluacion.Enviada {
			aviso = "enviada"
		}
		http.Redirect(w, r, tras(u, aviso, "calificacion"), http.StatusSeeOther)
	case errors.As(err, &regla):
		// Se vuelve a pintar con lo que escribió y el motivo de cada rechazo.
		v := a.vistaDeFicha(r, u)
		v.Observaciones = entrada.Observaciones
		mensajes := a.mensajesDe(r, regla.Errores)
		for i := range v.Criterios {
			v.Criterios[i].Elegido = entrada.Puntajes[v.Criterios[i].Clave]
			v.Criterios[i].Error = mensajes[v.Criterios[i].Clave]
		}
		var otros []string
		for _, e := range regla.Errores {
			if !slices.ContainsFunc(v.Criterios, func(c vistaCriterio) bool { return c.Clave == e.Campo }) {
				otros = append(otros, mensajes[e.Campo])
			}
		}
		v.Error = strings.Join(slices.Compact(otros), " ")
		a.pintarFicha(w, r, http.StatusUnprocessableEntity, v, &views.Aviso{Tipo: "error", Clave: "evaluacion.no_guardada"})
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	default:
		a.errorInterno(w, r, "guardar la evaluación", err)
	}
}

// POST /evaluacion/postulaciones/{id}/excusa   — se excusa por conflicto de interés (obra, motivo)
// POST /evaluacion/postulaciones/{id}/excusa/retirar   — retira la excusa (obra)
//
// La excusa es de la unidad: en Compositores, de la obra que se está viendo.
// No perjudica a la postulación (decisión de la CNMO).
func (a *App) excusaDesdeLaFicha(retirar bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.abrirUnidad(w, r)
		if !ok {
			return
		}
		obraID := u.unidades[u.posicion].ObraID
		var err error
		aviso := "excusa"
		if retirar {
			err, aviso = u.jurado.RetirarExcusa(r.Context(), a.actorDe(r), u.ab.p.ID, obraID), "retirada"
		} else {
			err = u.jurado.Excusarse(r.Context(), a.actorDe(r), u.ab.p.ID, obraID, textoDeFormulario(r.PostFormValue("motivo")))
		}
		var regla *models.ErrorValidacion
		switch {
		case err == nil:
			http.Redirect(w, r, tras(u, aviso, "excusa"), http.StatusSeeOther)
		case errors.As(err, &regla) && len(regla.Errores) > 0:
			v := a.vistaDeFicha(r, u)
			v.ErrorExcusa = a.vistas.T(idiomaDe(r), regla.Errores[0].Clave)
			a.pintarFicha(w, r, http.StatusUnprocessableEntity, v, nil)
		case errors.Is(err, models.ErrNoEncontrado):
			a.errorPagina(w, r, http.StatusNotFound)
		default:
			a.errorInterno(w, r, "excusa del jurado", err)
		}
	}
}
