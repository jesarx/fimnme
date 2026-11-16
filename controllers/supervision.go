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

// Pantallas de supervisión (paso 13b): tablero de cada convocatoria, matriz
// de resultados con el dictamen final, generador de reportes con sus
// plantillas, estadísticas de la edición y publicación de la nómina.
//
// Las ven el administrador y el observador (§2.2). Lo que cruza a los
// jurados pasa SÓLO por matrizDeSupervision y avanceDeJurados, que vuelven
// a comprobar el perfil. Lo que cambia algo —el dictamen final, la nómina—
// es sólo del administrador; las plantillas de reporte las guardan los dos.

// --- Modelos de vista ------------------------------------------------------------

// vistaPestanasSupervision son las pestañas comunes a las páginas de una
// convocatoria en supervisión.
type vistaPestanasSupervision struct {
	Base        string // /supervision/convocatorias/{id}
	Actual      string // tablero, resultados, reporte, nomina
	VeNomina    bool   // la publicación es sólo del administrador
	Titulo      string
	Edicion     string
	Tipo        string
	Estado      string
	Clase       string
	Estadistica string // estadísticas de la edición, sólo de esta convocatoria
}

// vistaCifraSupervision es un número del tablero, con el enlace al listado
// que lo produce cuando lo hay.
type vistaCifraSupervision struct {
	Etiqueta string // clave de traducción, o texto ya escrito
	Cantidad int
	Clase    string
	URL      string
}

// vistaBarra es un renglón de una distribución: su etiqueta, su cantidad y
// el máximo de la serie, para dibujar la barra con <meter>.
type vistaBarra struct {
	Etiqueta string
	Cantidad int
	Maximo   int
}

// vistaAvanceJurado es el avance de un jurado, sin ningún puntaje.
type vistaAvanceJurado struct {
	Nombre                                 string
	Enviadas, Excusadas, Pendientes, Total int
}

// vistaTablero es el tablero de una convocatoria.
type vistaTablero struct {
	vistaPestanasSupervision
	Total                  int
	PorEstado              []vistaCifraSupervision
	DocumentosPendientes   int
	ConPendientes          vistaCifraSupervision
	SubsanacionesAbiertas  vistaCifraSupervision
	SubsanacionesPorVencer int
	SubsanacionesVencidas  int
	DiasPorVencer          int
	EnviosPorDia           []vistaBarra
	Jurados                []vistaAvanceJurado
}

// vistaCeldaJurado es lo que un jurado calificó a una unidad.
type vistaCeldaJurado struct {
	Puntajes []string // uno por criterio; vacío si no lo envió
	Excusado bool
}

// vistaObservacionJurado es la observación de un jurado a una unidad.
type vistaObservacionJurado struct {
	Jurado, Texto string
}

// vistaFilaMatriz es una unidad en la matriz de resultados.
type vistaFilaMatriz struct {
	Ancla         string
	PostulacionID int64
	Lugar         int
	Folio         string
	Nombre        string
	Obra          string
	Revision      string // enlace a la vista de revisión
	Estado        string // clave de traducción
	Celdas        []vistaCeldaJurado
	Evaluaron     int
	Total         int
	Comparable    string
	Promedio      string
	Observaciones []vistaObservacionJurado
	Dictamen      string // clave de traducción del dictamen final, o vacío
	DictamenValor string
	// ConFormulario: la primera fila de cada postulación lleva el
	// formulario del dictamen final (en Compositores hay una fila por obra y
	// el dictamen es de la postulación).
	ConFormulario bool
}

// vistaMatriz es la página de resultados.
type vistaMatriz struct {
	vistaPestanasSupervision
	Jurados          []string
	Criterios        []string
	Filas            []vistaFilaMatriz
	PorObra          bool
	PuedeDictaminar  bool // administrador, con la convocatoria en evaluación o cerrada
	DescargarCSV     string
	Dictamenes       []vistaOpcion
	Error, ErrorDeID string
}

// vistaPlantillaReporte es una plantilla guardada.
type vistaPlantillaReporte struct {
	ID         int64
	Nombre     string
	Descargar  string
	Cargar     string
	PuedeBorra bool
}

// vistaGrupoCampos son los campos de un nivel del catálogo de reportes.
type vistaGrupoCampos struct {
	Nivel  string // clave de traducción
	Campos []vistaOpcion
}

// vistaReporte es el generador de reportes.
type vistaReporte struct {
	vistaPestanasSupervision
	Accion       string // dirección del CSV
	Grupos       []vistaGrupoCampos
	Granularidad []vistaOpcion
	Separador    []vistaOpcion
	Idioma       []vistaOpcion
	Orden        []vistaOpcion
	Estados      []vistaOpcion
	Subcategoria []vistaOpcion
	Dotaciones   []vistaOpcion
	Dictamenes   []vistaOpcion
	PuntajeMin   string
	PuntajeMax   string
	ConPuntaje   bool // la convocatoria ya tiene calificaciones que filtrar
	Plantillas   []vistaPlantillaReporte
	Nombre       string // nombre propuesto para guardar como plantilla
	Errores      []string
}

// vistaEstadisticas son las estadísticas de una edición.
type vistaEstadisticas struct {
	Edicion       string
	Base          string
	Convocatorias []vistaOpcion
	Postulaciones int
	Obras         int
	DuracionMedia string
	Series        []vistaSerie
}

// vistaSerie es una distribución de las estadísticas.
type vistaSerie struct {
	Titulo string // clave de traducción
	Barras []vistaBarra
}

// vistaNomina es la publicación de la nómina.
type vistaNomina struct {
	vistaPestanasSupervision
	PuedePublicar bool
	Seleccionados []models.InteropSeleccionado
	Publicaciones []vistaPublicacion
	Error         string
}

// vistaPublicacion es una publicación registrada.
type vistaPublicacion struct {
	Momento       time.Time
	Autor         string
	Seleccionados int
	Retirada      bool
	SHA256        string
}

// --- Portada --------------------------------------------------------------------

// GET /supervision   — las ediciones con sus convocatorias
func (a *App) portadaDeSupervision(w http.ResponseWriter, r *http.Request) {
	if !a.exigirSupervision(w, r) {
		return
	}
	ediciones, err := a.edicionesAdmin(r)
	if err != nil {
		a.errorInterno(w, r, "listar ediciones", err)
		return
	}
	// Las mismas ediciones que la administración, con enlaces a supervisión.
	for i := range ediciones {
		ediciones[i].URL = fmt.Sprintf("/supervision/ediciones/%d/estadisticas", ediciones[i].ID)
		for j := range ediciones[i].Convocatorias {
			ediciones[i].Convocatorias[j].URL = strings.Replace(ediciones[i].Convocatorias[j].URL, "/administracion/", "/supervision/", 1)
		}
	}
	p := a.pagina(r, "supervision.titulo")
	p.Datos = vistaPortadaAdmin{Ediciones: ediciones}
	a.pintar(w, r, http.StatusOK, "supervision_portada.html", p)
}

// convocatoriaEnSupervision lee la configuración de la convocatoria de la
// ruta y arma sus pestañas. Responde 403 o 404 si no procede.
func (a *App) convocatoriaEnSupervision(w http.ResponseWriter, r *http.Request, actual string) (models.ConfiguracionConvocatoria, vistaPestanasSupervision, bool) {
	if !a.exigirSupervision(w, r) {
		return models.ConfiguracionConvocatoria{}, vistaPestanasSupervision{}, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return models.ConfiguracionConvocatoria{}, vistaPestanasSupervision{}, false
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return cfg, vistaPestanasSupervision{}, false
	}
	if err != nil {
		a.errorInterno(w, r, "leer convocatoria", err)
		return cfg, vistaPestanasSupervision{}, false
	}
	c, idioma := cfg.Convocatoria, idiomaDe(r)
	return cfg, vistaPestanasSupervision{
		Base: fmt.Sprintf("/supervision/convocatorias/%d", id), Actual: actual, VeNomina: models.PuedeAdministrar(sesionDe(r)),
		Titulo: c.Titulo(idioma), Edicion: c.Edicion.Nombre(idioma), Tipo: "tipo." + string(c.Tipo),
		Estado: claveDeEstadoAdmin(c.Estado), Clase: claseEstado(c.Estado),
		Estadistica: fmt.Sprintf("/supervision/ediciones/%d/estadisticas?convocatoria=%d", c.Edicion.ID, id),
	}, true
}

// pintarSupervision envía una página de una convocatoria en supervisión.
func (a *App) pintarSupervision(w http.ResponseWriter, r *http.Request, estado int, plantilla, titulo string, pestanas vistaPestanasSupervision, datos any) {
	p := a.pagina(r, titulo)
	p.Titulo = pestanas.Titulo + " · " + a.vistas.T(p.Idioma, titulo)
	p.Datos = datos
	if pedido, hay := avisosDeSupervision[r.URL.Query().Get("aviso")]; hay {
		p.Aviso = &pedido
	}
	a.pintar(w, r, estado, plantilla, p)
}

// avisosDeSupervision son los mensajes que una redirección pide mostrar.
var avisosDeSupervision = map[string]views.Aviso{
	"dictamen":  {Tipo: "exito", Clave: "supervision.aviso_dictamen"},
	"plantilla": {Tipo: "exito", Clave: "supervision.aviso_plantilla"},
	"borrada":   {Tipo: "exito", Clave: "supervision.aviso_plantilla_borrada"},
	"publicada": {Tipo: "exito", Clave: "supervision.aviso_publicada"},
	"retirada":  {Tipo: "exito", Clave: "supervision.aviso_retirada"},
}

// --- Tablero --------------------------------------------------------------------

// GET /supervision/convocatorias/{id}   — tablero de la convocatoria
//
// Postulaciones por estado, documentos por dictaminar, subsanaciones, ritmo
// de envíos y avance de cada jurado, sin puntajes (RF-DASH-01 y 02). Las
// cifras de la revisión llevan a la bandeja filtrada (RF-DASH-05).
func (a *App) tableroDeSupervision(w http.ResponseWriter, r *http.Request) {
	cfg, pestanas, ok := a.convocatoriaEnSupervision(w, r, "tablero")
	if !ok {
		return
	}
	id := cfg.Convocatoria.ID
	avance, err := avanceDeJurados(r.Context(), a.datos, sesionDe(r), id)
	if err != nil {
		a.errorInterno(w, r, "avance de jurados", err)
		return
	}
	t, err := a.datos.TableroDeConvocatoria(r.Context(), id)
	if err != nil {
		a.errorInterno(w, r, "tablero", err)
		return
	}
	bandeja := fmt.Sprintf("/revision/convocatorias/%d", id)
	v := vistaTablero{vistaPestanasSupervision: pestanas, DocumentosPendientes: t.DocumentosPendientes,
		ConPendientes:          vistaCifraSupervision{Etiqueta: "supervision.con_documentos_pendientes", Cantidad: t.PostulacionesConPendientes},
		SubsanacionesAbiertas:  vistaCifraSupervision{Etiqueta: "supervision.subsanaciones_abiertas", Cantidad: t.SubsanacionesAbiertas, URL: bandeja + "?estado=en_subsanacion"},
		SubsanacionesPorVencer: t.SubsanacionesPorVencer, SubsanacionesVencidas: t.SubsanacionesVencidas, DiasPorVencer: models.DiasPorVencer}
	for _, c := range t.PorEstado {
		estado := models.EstadoDePostulacion(c.Clave)
		cifra := vistaCifraSupervision{Etiqueta: "estado.postulacion." + c.Clave, Cantidad: c.Cantidad, Clase: clasesEnRevision[estado]}
		if slices.Contains(estadosEnRevision, estado) {
			cifra.URL = bandeja + "?estado=" + url.QueryEscape(c.Clave)
		}
		v.PorEstado = append(v.PorEstado, cifra)
		v.Total += c.Cantidad
	}
	dias := views.EnviosPorDia(t.PrimerosEnvios)
	maximo := 0
	for _, d := range dias {
		maximo = max(maximo, d.Cantidad)
	}
	for _, d := range dias {
		v.EnviosPorDia = append(v.EnviosPorDia, vistaBarra{Etiqueta: d.Clave, Cantidad: d.Cantidad, Maximo: maximo})
	}
	for _, j := range avance {
		v.Jurados = append(v.Jurados, vistaAvanceJurado{Nombre: j.Nombre, Enviadas: j.Enviadas, Excusadas: j.Excusadas,
			Pendientes: j.Total - j.Enviadas - j.Excusadas, Total: j.Total})
	}
	a.pintarSupervision(w, r, http.StatusOK, "supervision_tablero.html", "supervision.tablero", pestanas, v)
}

// --- Matriz de resultados y dictamen final ------------------------------------------

// decimal escribe un número con a lo más dos decimales.
func decimal(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// dictamenesFinales son las opciones del dictamen final, en su orden.
var dictamenesFinales = []string{models.DictamenSeleccionada, models.DictamenListaDeEspera, models.DictamenNoSeleccionada}

// GET /supervision/convocatorias/{id}/resultados   — matriz de resultados
//
// La calificación de cada jurado por criterio, la suma, el puntaje
// comparable por el que se ordena, el promedio, las observaciones y el
// dictamen final (RF-EVA-08 y 09). Sólo supervisión: es lo único que reúne
// a los tres jurados.
func (a *App) resultadosDeSupervision(w http.ResponseWriter, r *http.Request) {
	a.pintarResultados(w, r, http.StatusOK, "", 0)
}

// pintarResultados muestra la matriz, con el rechazo de un dictamen junto a
// su fila si lo hay.
func (a *App) pintarResultados(w http.ResponseWriter, r *http.Request, estado int, mensaje string, postulacionConError int64) {
	cfg, pestanas, ok := a.convocatoriaEnSupervision(w, r, "resultados")
	if !ok {
		return
	}
	matriz, err := matrizDeSupervision(r.Context(), a.datos, sesionDe(r), a.actorDe(r), cfg.Convocatoria.ID)
	if err != nil {
		a.errorInterno(w, r, "matriz de resultados", err)
		return
	}
	idioma := idiomaDe(r)
	c := cfg.Convocatoria
	v := vistaMatriz{vistaPestanasSupervision: pestanas, PorObra: c.Tipo == models.TipoCompositores,
		PuedeDictaminar: models.PuedeAdministrar(sesionDe(r)) && (c.Estado == models.EstadoEvaluacion || c.Estado == models.EstadoCerrada),
		DescargarCSV:    fmt.Sprintf("/api/v1/convocatorias/%d/resultados?formato=csv&idioma=%s", c.ID, idioma), Error: mensaje}
	if postulacionConError != 0 {
		v.ErrorDeID = fmt.Sprintf("u-%d", postulacionConError)
	}
	v.Dictamenes = append(v.Dictamenes, vistaOpcion{Valor: "", Texto: a.vistas.T(idioma, "supervision.sin_dictamen")})
	for _, d := range dictamenesFinales {
		v.Dictamenes = append(v.Dictamenes, vistaOpcion{Valor: d, Texto: a.vistas.T(idioma, "resultado."+d)})
	}
	for _, j := range matriz.Jurados {
		v.Jurados = append(v.Jurados, j.Nombre)
	}
	for _, cr := range matriz.Criterios {
		v.Criterios = append(v.Criterios, enIdioma(idioma, cr.EtiquetaES, cr.EtiquetaEN))
	}
	lugares := models.LugaresDeLaMatriz(matriz.Filas)
	vistas := make(map[int64]bool)
	for i, f := range matriz.Filas {
		fila := vistaFilaMatriz{Ancla: fmt.Sprintf("u-%d-%d", f.Unidad.PostulacionID, f.Unidad.ObraID), PostulacionID: f.Unidad.PostulacionID,
			Lugar: lugares[i], Folio: f.Unidad.Folio, Nombre: f.Unidad.Nombre, Obra: f.Unidad.Obra,
			Revision: fmt.Sprintf("/revision/postulaciones/%d", f.Unidad.PostulacionID), Estado: "estado.postulacion." + string(f.Estado),
			Evaluaron: f.JuradosQueEvaluaron, DictamenValor: f.DictamenFinal, ConFormulario: !vistas[f.Unidad.PostulacionID]}
		vistas[f.Unidad.PostulacionID] = true
		if fila.ConFormulario {
			fila.Ancla = fmt.Sprintf("u-%d", f.Unidad.PostulacionID)
		}
		if f.DictamenFinal != "" {
			fila.Dictamen = "resultado." + f.DictamenFinal
		}
		if f.JuradosQueEvaluaron > 0 {
			fila.Total, fila.Comparable, fila.Promedio = f.Total, decimal(f.Comparable), decimal(f.Promedio)
		}
		for _, j := range matriz.Jurados {
			evaluacion, enviada := f.PorJurado[j.ID]
			_, excusado := f.Excusas[j.ID]
			celda := vistaCeldaJurado{Excusado: excusado}
			for _, cr := range matriz.Criterios {
				puntaje := ""
				if valor, calificado := evaluacion.Puntajes[cr.Clave]; enviada && calificado {
					puntaje = strconv.Itoa(valor)
				}
				celda.Puntajes = append(celda.Puntajes, puntaje)
			}
			fila.Celdas = append(fila.Celdas, celda)
			if enviada && strings.TrimSpace(evaluacion.Observaciones) != "" {
				fila.Observaciones = append(fila.Observaciones, vistaObservacionJurado{Jurado: j.Nombre, Texto: evaluacion.Observaciones})
			}
		}
		v.Filas = append(v.Filas, fila)
	}
	a.pintarSupervision(w, r, estado, "supervision_resultados.html", "supervision.resultados", pestanas, v)
}

// POST /supervision/postulaciones/{id}/dictamen-final   — resultado del proceso (dictamen)
//
// Sólo el administrador. No se comunica al postulante: el resultado se da a
// conocer fuera del sistema.
func (a *App) dictamenFinalDesdeSupervision(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	if err == nil {
		err = a.datos.RegistrarDictamenFinal(r.Context(), a.actorDe(r), id, r.PostFormValue("dictamen"))
	}
	if err == nil {
		http.Redirect(w, r, fmt.Sprintf("/supervision/convocatorias/%d/resultados?aviso=dictamen#u-%d", p.ConvocatoriaID, id), http.StatusSeeOther)
		return
	}
	mensaje, rechazo := a.mensajeDeRechazo(r, err)
	if !rechazo {
		a.errorInterno(w, r, "dictamen final", err)
		return
	}
	// La matriz se vuelve a pintar como la de su convocatoria.
	r.SetPathValue("id", strconv.FormatInt(p.ConvocatoriaID, 10))
	a.pintarResultados(w, r, http.StatusUnprocessableEntity, mensaje, id)
}

// --- Generador de reportes ---------------------------------------------------------

// consultaDelGenerador convierte lo enviado por el formulario del generador
// en los parámetros del reporte: los campos marcados (una casilla por campo)
// van juntos, separados por comas, como los pide la API; lo vacío no se
// manda.
func consultaDelGenerador(enviado url.Values) url.Values {
	consulta := url.Values{}
	for nombre, valores := range enviado {
		switch nombre {
		case "campos":
			consulta.Set("campos", strings.Join(valores, ","))
		case "nombre":
			// El nombre de la plantilla no es parte del reporte.
		default:
			if valor := strings.TrimSpace(valores[0]); valor != "" {
				consulta.Set(nombre, valor)
			}
		}
	}
	return consulta
}

// opcionesCon marca, en una lista de opciones, la elegida.
func opcionesCon(elegida string, opciones ...vistaOpcion) []vistaOpcion {
	for i := range opciones {
		opciones[i].Elegida = opciones[i].Valor == elegida
	}
	return opciones
}

// vistaDelGenerador arma el generador de reportes con los valores de la
// consulta (la de la petición, o la de una plantilla que se carga).
func (a *App) vistaDelGenerador(r *http.Request, cfg models.ConfiguracionConvocatoria, pestanas vistaPestanasSupervision, consulta url.Values) (vistaReporte, error) {
	idioma, sesion := idiomaDe(r), sesionDe(r)
	c := cfg.Convocatoria
	t := func(clave string) string { return a.vistas.T(idioma, clave) }
	v := vistaReporte{vistaPestanasSupervision: pestanas, Accion: pestanas.Base + "/reporte.csv",
		PuntajeMin: consulta.Get("puntaje_min"), PuntajeMax: consulta.Get("puntaje_max"),
		ConPuntaje: c.Estado == models.EstadoEvaluacion || c.Estado == models.EstadoCerrada || c.Estado == models.EstadoArchivada}
	marcados := strings.Split(consulta.Get("campos"), ",")
	if consulta.Get("campos") == "" {
		marcados = []string{"folio", "estado"}
	}
	grupos := map[string]int{}
	for _, campo := range a.vistas.CamposDeReporte(idioma, cfg, models.JuradosDe(cfg)) {
		if campo.Nivel == models.NivelEvaluacion && !v.ConPuntaje {
			continue
		}
		i, hay := grupos[campo.Nivel]
		if !hay {
			i = len(v.Grupos)
			grupos[campo.Nivel] = i
			v.Grupos = append(v.Grupos, vistaGrupoCampos{Nivel: "supervision.nivel_" + campo.Nivel})
		}
		v.Grupos[i].Campos = append(v.Grupos[i].Campos, vistaOpcion{Valor: campo.Clave, Texto: campo.Etiqueta, Elegida: slices.Contains(marcados, campo.Clave)})
	}
	v.Granularidad = opcionesCon(consulta.Get("granularidad"),
		vistaOpcion{Valor: models.GranularidadPostulacion, Texto: t("supervision.por_postulacion")},
		vistaOpcion{Valor: models.GranularidadObra, Texto: t("supervision.por_obra")})
	if consulta.Get("granularidad") == "" {
		v.Granularidad[0].Elegida = true
	}
	v.Separador = opcionesCon(consulta.Get("separador"),
		vistaOpcion{Valor: "coma", Texto: t("supervision.separador_coma")},
		vistaOpcion{Valor: "punto_y_coma", Texto: t("supervision.separador_punto_y_coma")})
	if consulta.Get("separador") == "" {
		v.Separador[0].Elegida = true
	}
	v.Idioma = opcionesCon(primeroNoVacio(consulta.Get("idioma"), idioma),
		vistaOpcion{Valor: models.IdiomaES, Texto: t("cuenta.idioma_es")}, vistaOpcion{Valor: models.IdiomaEN, Texto: t("cuenta.idioma_en")})
	ordenes := []vistaOpcion{{Valor: "folio", Texto: t("supervision.orden_folio")}, {Valor: "nombre", Texto: t("supervision.orden_nombre")},
		{Valor: "enviada", Texto: t("supervision.orden_enviada")}, {Valor: "estado", Texto: t("supervision.orden_estado")}}
	if v.ConPuntaje {
		ordenes = append(ordenes, vistaOpcion{Valor: "-puntaje", Texto: t("supervision.orden_puntaje")})
	}
	v.Orden = opcionesCon(consulta.Get("orden"), ordenes...)
	v.Estados = []vistaOpcion{{Valor: "", Texto: t("revision.todos")}}
	for _, estado := range append([]models.EstadoDePostulacion{models.PostulacionBorrador}, estadosEnRevision...) {
		v.Estados = append(v.Estados, vistaOpcion{Valor: string(estado), Texto: t("estado.postulacion." + string(estado)), Elegida: consulta.Get("estado") == string(estado)})
	}
	if c.Tipo == models.TipoInterpretes {
		v.Subcategoria = opcionesCon(consulta.Get("subcategoria"), vistaOpcion{Valor: "", Texto: t("revision.todas")},
			vistaOpcion{Valor: string(models.SubcategoriaSolista), Texto: t("subcategoria.solista")},
			vistaOpcion{Valor: string(models.SubcategoriaEnsamble), Texto: t("subcategoria.ensamble")})
	} else {
		v.Dotaciones = []vistaOpcion{{Valor: "", Texto: t("revision.todas")}}
		for _, d := range cfg.Dotaciones {
			valor := strconv.FormatInt(d.ID, 10)
			v.Dotaciones = append(v.Dotaciones, vistaOpcion{Valor: valor, Texto: enIdioma(idioma, d.NombreES, d.NombreEN), Elegida: consulta.Get("dotacion") == valor})
		}
	}
	if v.ConPuntaje {
		v.Dictamenes = []vistaOpcion{{Valor: "", Texto: t("revision.todos")}}
		for _, d := range dictamenesFinales {
			v.Dictamenes = append(v.Dictamenes, vistaOpcion{Valor: d, Texto: t("resultado." + d), Elegida: consulta.Get("dictamen") == d})
		}
	}
	plantillas, err := a.datos.PlantillasDeReporte(r.Context())
	if err != nil {
		return v, err
	}
	for _, p := range plantillas {
		if p.Tipo != c.Tipo {
			continue
		}
		v.Plantillas = append(v.Plantillas, vistaPlantillaReporte{ID: p.ID, Nombre: p.Nombre,
			Descargar:  fmt.Sprintf("%s?plantilla=%d", v.Accion, p.ID),
			Cargar:     fmt.Sprintf("%s/reporte?plantilla=%d#generador", pestanas.Base, p.ID),
			PuedeBorra: models.PuedeAdministrar(sesion) || p.CreadoPor == sesion.Usuario.ID})
	}
	return v, nil
}

// enIdioma elige el texto en inglés cuando se pide y existe; si no, el
// español, que siempre está.
func enIdioma(idioma, es, en string) string {
	if idioma == models.IdiomaEN && en != "" {
		return en
	}
	return es
}

// primeroNoVacio devuelve el primer texto no vacío.
func primeroNoVacio(textos ...string) string {
	for _, texto := range textos {
		if texto != "" {
			return texto
		}
	}
	return ""
}

// GET /supervision/convocatorias/{id}/reporte?plantilla=   — generador de reportes
//
// Con «plantilla», el formulario se llena con lo que esa plantilla guarda.
func (a *App) generadorDeReportes(w http.ResponseWriter, r *http.Request) {
	cfg, pestanas, ok := a.convocatoriaEnSupervision(w, r, "reporte")
	if !ok {
		return
	}
	consulta := url.Values{}
	if texto := r.URL.Query().Get("plantilla"); texto != "" {
		id, _ := strconv.ParseInt(texto, 10, 64)
		plantilla, err := a.datos.PlantillaDeReportePorID(r.Context(), id)
		if errors.Is(err, models.ErrNoEncontrado) || (err == nil && plantilla.Tipo != cfg.Convocatoria.Tipo) {
			a.errorPagina(w, r, http.StatusNotFound)
			return
		}
		if err != nil {
			a.errorInterno(w, r, "leer plantilla", err)
			return
		}
		if consulta, err = url.ParseQuery(plantilla.Consulta); err != nil {
			consulta = url.Values{}
		}
	}
	v, err := a.vistaDelGenerador(r, cfg, pestanas, consulta)
	if err != nil {
		a.errorInterno(w, r, "armar el generador", err)
		return
	}
	a.pintarSupervision(w, r, http.StatusOK, "supervision_reporte.html", "supervision.reportes", pestanas, v)
}

// camposDelGenerador son los controles del generador cuyo nombre se
// antepone al motivo de un rechazo, para saber a qué se refiere.
var camposDelGenerador = map[string]bool{
	"campos": true, "granularidad": true, "separador": true, "orden": true, "estado": true, "subcategoria": true,
	"dotacion": true, "dictamen": true, "puntaje_min": true, "puntaje_max": true, "nombre": true,
}

// rechazoDelGenerador vuelve a pintar el generador con lo enviado y los
// motivos del rechazo.
func (a *App) rechazoDelGenerador(w http.ResponseWriter, r *http.Request, cfg models.ConfiguracionConvocatoria,
	pestanas vistaPestanasSupervision, consulta url.Values, err error) {
	var regla *models.ErrorValidacion
	if !errors.As(err, &regla) && !errors.Is(err, models.ErrDuplicado) {
		a.errorInterno(w, r, "generar reporte", err)
		return
	}
	v, errVista := a.vistaDelGenerador(r, cfg, pestanas, consulta)
	if errVista != nil {
		a.errorInterno(w, r, "armar el generador", errVista)
		return
	}
	v.Nombre = r.FormValue("nombre")
	if regla != nil {
		for _, e := range regla.Errores {
			mensaje := a.vistas.T(idiomaDe(r), e.Clave)
			if camposDelGenerador[e.Campo] {
				mensaje = a.vistas.T(idiomaDe(r), "supervision.campo_"+e.Campo) + ": " + mensaje
			}
			v.Errores = append(v.Errores, mensaje)
		}
	} else {
		v.Errores = append(v.Errores, a.vistas.T(idiomaDe(r), "supervision.plantilla_duplicada"))
	}
	v.Errores = slices.Compact(v.Errores)
	a.pintarSupervision(w, r, http.StatusUnprocessableEntity, "supervision_reporte.html", "supervision.reportes", pestanas, v)
}

// GET /supervision/convocatorias/{id}/reporte.csv   — descarga el reporte
//
// Pasa por GenerarReporte, como la API y fimnme-cli: mismas reglas, mismo
// alcance, misma bitácora. Si algo no procede, vuelve el generador con el
// motivo en vez de un JSON.
func (a *App) descargarReporte(w http.ResponseWriter, r *http.Request) {
	cfg, pestanas, ok := a.convocatoriaEnSupervision(w, r, "reporte")
	if !ok {
		return
	}
	consulta := consultaDelGenerador(r.URL.Query())
	reporte, err := GenerarReporte(r.Context(), a.datos, a.vistas, sesionDe(r), a.actorDe(r), cfg.Convocatoria.ID, consulta, primeroNoVacio(consulta.Get("idioma"), idiomaDe(r)))
	if err != nil {
		a.rechazoDelGenerador(w, r, cfg, pestanas, a.consultaConPlantilla(r, consulta), err)
		return
	}
	if err := views.CSVConSeparador(w, reporte.Nombre, reporte.Separador, reporte.Encabezados, reporte.Filas); err != nil {
		a.log.Error("no se pudo escribir el reporte", "error", err)
	}
}

// consultaConPlantilla completa una consulta con lo que guarda la plantilla
// que pide, como hace GenerarReporte: para volver a mostrar en el formulario
// lo que realmente se intentó generar.
func (a *App) consultaConPlantilla(r *http.Request, consulta url.Values) url.Values {
	id, err := strconv.ParseInt(consulta.Get("plantilla"), 10, 64)
	if err != nil {
		return consulta
	}
	plantilla, err := a.datos.PlantillaDeReportePorID(r.Context(), id)
	if err != nil {
		return consulta
	}
	completa, err := url.ParseQuery(plantilla.Consulta)
	if err != nil {
		return consulta
	}
	for nombre, valores := range consulta {
		completa[nombre] = valores
	}
	return completa
}

// POST /supervision/convocatorias/{id}/reporte/plantillas   — guarda lo elegido como plantilla (nombre)
//
// La plantilla sirve para cualquier convocatoria del mismo tipo, este año y
// los siguientes (RF-REP-04). Antes de guardarla se comprueba que el reporte
// proceda, para no guardar una definición que no sirve.
func (a *App) guardarPlantillaDesdeSupervision(w http.ResponseWriter, r *http.Request) {
	cfg, pestanas, ok := a.convocatoriaEnSupervision(w, r, "reporte")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.errorPagina(w, r, http.StatusBadRequest)
		return
	}
	consulta := consultaDelGenerador(r.PostForm)
	def := models.DefinicionDeReporte{Granularidad: consulta.Get("granularidad"), Separador: consulta.Get("separador"),
		Campos: strings.FieldsFunc(consulta.Get("campos"), func(r rune) bool { return r == ',' })}
	_, errores := models.ValidarReporte(&def, cfg)
	var err error
	if len(errores) > 0 {
		err = &models.ErrorValidacion{Errores: errores}
	} else {
		_, err = a.datos.GuardarPlantillaDeReporte(r.Context(), a.actorDe(r), models.PlantillaDeReporte{
			Nombre: textoDeFormulario(r.PostFormValue("nombre")), Tipo: cfg.Convocatoria.Tipo, Consulta: consulta.Encode()})
	}
	if err != nil {
		a.rechazoDelGenerador(w, r, cfg, pestanas, consulta, err)
		return
	}
	http.Redirect(w, r, pestanas.Base+"/reporte?aviso=plantilla#plantillas", http.StatusSeeOther)
}

// POST /supervision/convocatorias/{id}/reporte/plantillas/{plantilla}/borrar   — borra una plantilla
//
// El administrador borra cualquiera; el observador, las que él guardó.
func (a *App) borrarPlantillaDesdeSupervision(w http.ResponseWriter, r *http.Request) {
	_, pestanas, ok := a.convocatoriaEnSupervision(w, r, "reporte")
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("plantilla"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	sesion := sesionDe(r)
	soloDe := sesion.Usuario.ID
	if models.PuedeAdministrar(sesion) {
		soloDe = 0
	}
	switch err := a.datos.BorrarPlantillaDeReporte(r.Context(), a.actorDe(r), id, soloDe); {
	case err == nil:
		http.Redirect(w, r, pestanas.Base+"/reporte?aviso=borrada#plantillas", http.StatusSeeOther)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	default:
		a.errorInterno(w, r, "borrar plantilla", err)
	}
}

// --- Estadísticas ----------------------------------------------------------------

// GET /supervision/ediciones/{id}/estadisticas?convocatoria=   — estadísticas de la edición
//
// Distribuciones descriptivas de las postulaciones enviadas (RF-DASH-03).
func (a *App) estadisticasDeSupervision(w http.ResponseWriter, r *http.Request) {
	if !a.exigirSupervision(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	edicion, err := a.datos.EdicionPorID(r.Context(), id)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		a.errorInterno(w, r, "leer edición", err)
		return
	}
	convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), id)
	if err != nil {
		a.errorInterno(w, r, "listar convocatorias", err)
		return
	}
	idioma := idiomaDe(r)
	elegida := r.URL.Query().Get("convocatoria")
	v := vistaEstadisticas{Edicion: edicion.Nombre(idioma), Base: fmt.Sprintf("/supervision/ediciones/%d/estadisticas", id),
		Convocatorias: []vistaOpcion{{Valor: "", Texto: a.vistas.T(idioma, "supervision.toda_la_edicion")}}}
	var convocatoriaID int64
	var dotaciones []models.Dotacion
	for _, c := range convocatorias {
		valor := strconv.FormatInt(c.ID, 10)
		v.Convocatorias = append(v.Convocatorias, vistaOpcion{Valor: valor, Texto: c.Titulo(idioma), Elegida: valor == elegida})
		if valor == elegida {
			convocatoriaID = c.ID
		}
	}
	if elegida != "" && convocatoriaID == 0 {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	e, err := a.datos.EstadisticasDeEdicion(r.Context(), id, convocatoriaID)
	if err != nil {
		a.errorInterno(w, r, "estadísticas", err)
		return
	}
	for _, c := range convocatorias {
		if cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), c.ID); err == nil {
			dotaciones = append(dotaciones, cfg.Dotaciones...)
		}
	}
	v.Postulaciones, v.Obras = e.Postulaciones, e.Obras
	if e.DuracionMediaMin > 0 {
		v.DuracionMedia = strconv.FormatFloat(e.DuracionMediaMin, 'f', 1, 64)
	}
	// serie convierte una distribución; nombre traduce la clave de cada cifra.
	serie := func(titulo string, cifras []models.Cifra, nombre func(models.Cifra) string) {
		if len(cifras) == 0 {
			return
		}
		maximo := 0
		for _, c := range cifras {
			maximo = max(maximo, c.Cantidad)
		}
		s := vistaSerie{Titulo: titulo}
		for _, c := range cifras {
			etiqueta := a.vistas.T(idioma, "supervision.sin_dato")
			if c.Clave != "" {
				etiqueta = nombre(c)
			}
			s.Barras = append(s.Barras, vistaBarra{Etiqueta: etiqueta, Cantidad: c.Cantidad, Maximo: maximo})
		}
		v.Series = append(v.Series, s)
	}
	traducida := func(prefijo string) func(models.Cifra) string {
		return func(c models.Cifra) string { return a.vistas.T(idioma, prefijo+c.Clave) }
	}
	entidad := func(c models.Cifra) string {
		for _, e := range models.EntidadesFederativas() {
			if e.Clave == c.Clave {
				return e.Nombre
			}
		}
		return c.Clave
	}
	dotacion := func(c models.Cifra) string {
		for _, d := range dotaciones {
			if strconv.FormatInt(d.ID, 10) == c.Clave {
				return enIdioma(idioma, d.NombreES, d.NombreEN)
			}
		}
		return primeroNoVacio(c.Nombre, c.Clave)
	}
	serie("supervision.por_convocatoria", e.PorConvocatoria, traducida("tipo."))
	serie("supervision.por_entidad", e.PorEntidad, entidad)
	serie("supervision.por_pais", e.PorPais, traducida("pais."))
	serie("supervision.por_nacionalidad", e.PorNacionalidad, traducida("pais."))
	serie("supervision.por_genero", e.PorGenero, traducida("opcion."))
	serie("supervision.por_edad", e.PorEdad, traducida("supervision.edad_"))
	serie("supervision.por_dotacion", e.PorDotacion, dotacion)
	serie("supervision.por_estreno", e.PorEstreno, traducida("opcion."))
	p := a.pagina(r, "supervision.estadisticas")
	p.Titulo = v.Edicion + " · " + a.vistas.T(idioma, "supervision.estadisticas")
	p.Datos = v
	a.pintar(w, r, http.StatusOK, "supervision_estadisticas.html", p)
}

// --- Publicación de la nómina -------------------------------------------------------

// GET /supervision/convocatorias/{id}/nomina   — la nómina de seleccionados y su publicación
//
// Sólo el administrador: muestra exactamente lo que saldría al portal y las
// publicaciones anteriores (RF-PUB-04 y 05).
func (a *App) nominaDeSupervision(w http.ResponseWriter, r *http.Request) {
	a.pintarNomina(w, r, http.StatusOK, "")
}

// pintarNomina muestra la página de la nómina, con un rechazo si lo hay.
func (a *App) pintarNomina(w http.ResponseWriter, r *http.Request, estado int, mensaje string) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	cfg, pestanas, ok := a.convocatoriaEnSupervision(w, r, "nomina")
	if !ok {
		return
	}
	seleccionados, err := a.datos.SeleccionadosDe(r.Context(), cfg.Convocatoria.ID)
	if err != nil {
		a.errorInterno(w, r, "seleccionados", err)
		return
	}
	publicaciones, _, err := a.datos.PublicacionesDe(r.Context(), cfg.Convocatoria.ID)
	if err != nil {
		a.errorInterno(w, r, "publicaciones", err)
		return
	}
	v := vistaNomina{vistaPestanasSupervision: pestanas, PuedePublicar: models.PuedePublicarNomina(cfg.Convocatoria),
		Seleccionados: seleccionados, Error: mensaje}
	for _, pub := range publicaciones {
		v.Publicaciones = append(v.Publicaciones, vistaPublicacion{Momento: pub.CreadoAt, Autor: pub.AutorizadoPor,
			Seleccionados: pub.Seleccionados, Retirada: pub.Seleccionados == 0, SHA256: pub.SHA256})
	}
	a.pintarSupervision(w, r, estado, "supervision_nomina.html", "supervision.nomina", pestanas, v)
}

// POST /supervision/convocatorias/{id}/nomina   — publica (o retira, accion=retirar) la nómina (confirmo)
func (a *App) publicarNominaDesdeSupervision(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	retirar := r.PostFormValue("accion") == "retirar"
	err = a.publicarNomina(r, id, r.PostFormValue("confirmo") != "", retirar)
	switch mensaje, rechazo := a.mensajeDeRechazo(r, err); {
	case err == nil:
		aviso := "publicada"
		if retirar {
			aviso = "retirada"
		}
		http.Redirect(w, r, fmt.Sprintf("/supervision/convocatorias/%d/nomina?aviso=%s", id, aviso), http.StatusSeeOther)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	case rechazo:
		a.pintarNomina(w, r, http.StatusUnprocessableEntity, mensaje)
	default:
		a.errorInterno(w, r, "publicar la nómina", err)
	}
}
