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

// Pantallas del revisor documental (§5.5, paso 12a): la bandeja de cada
// convocatoria y la vista de revisión de una postulación, con el visor de
// documentos (RF-DOC-08).
//
// Como las del postulante, no tienen reglas propias: cada acción llama a la
// misma operación del repositorio que la API, y es el modelo quien decide si
// procede. Quien supervisa (administrador y observador) ve lo mismo; el
// observador, sin ningún formulario, porque no revisa (§2.2).

// --- Modelos de vista ----------------------------------------------------------

// vistaConvocatoriaEnRevision es una convocatoria en la portada de la revisión.
type vistaConvocatoriaEnRevision struct {
	URL     string
	Titulo  string
	Edicion string
	Estado  string
	Clase   string
	Total   int
}

// vistaCifraDeBandeja es una cifra del tablero de la bandeja: un estado, cuántas
// hay y el enlace que las filtra.
type vistaCifraDeBandeja struct {
	Estado string
	Clase  string
	Cuenta int
	URL    string
	Activa bool
}

// vistaRenglonDeBandeja es una postulación en la bandeja.
type vistaRenglonDeBandeja struct {
	URL                 string
	Folio               string
	Nombre              string
	Subcategoria        string
	Estado              string
	Clase               string
	EnviadaAt           time.Time
	Envios              int
	Toma                string // quién la tomó; vacío si está libre
	TomaPropia          bool
	Observaciones       int
	DocumentosInvalidos int
	FechaLimite         time.Time
	Vencida             bool
	Subsanada           bool
}

// vistaBandeja es la bandeja de una convocatoria (RF-REV-01).
type vistaBandeja struct {
	Base          string
	Titulo        string
	Edicion       string
	EsInterpretes bool
	Tablero       []vistaCifraDeBandeja
	Total         int
	Renglones     []vistaRenglonDeBandeja
	// Filtro en curso, para que el formulario lo conserve.
	Estado, Subcategoria, Toma, Texto string
	Estados                           []vistaOpcion
}

// vistaDocumentoEnRevision es un documento con su visor y su dictamen.
type vistaDocumentoEnRevision struct {
	ID       int64
	Ancla    string
	Nombre   string
	URL      string
	Tipo     string // pdf, audio o imagen: decide el visor
	Tamano   string
	Version  int
	Subido   time.Time
	Dictamen string // pendiente, valido o invalido
	Motivo   string
}

// vistaRequisitoEnRevision es lo que se pide y lo entregado, para el revisor.
type vistaRequisitoEnRevision struct {
	Etiqueta    string
	Obligatorio bool
	Documentos  []vistaDocumentoEnRevision
}

// vistaDocumentosEnRevision son los documentos de la postulación o de una obra.
type vistaDocumentosEnRevision struct {
	Titulo     string
	Requisitos []vistaRequisitoEnRevision
}

// vistaObservacion es una nota interna de la revisión.
type vistaObservacion struct {
	Texto   string
	Revisor string
	Fecha   time.Time
	Obra    string
}

// vistaCambio es una diferencia entre los dos últimos envíos.
type vistaCambio struct {
	Etiqueta, Antes, Despues string
}

// vistaSolicitudEnRevision es una solicitud de desbloqueo, con su id para
// resolverla.
type vistaSolicitudEnRevision struct {
	vistaSolicitud
	ID        int64
	Pendiente bool
}

// vistaSubsanacionEnRevision es una subsanación, abierta o pasada.
type vistaSubsanacionEnRevision struct {
	Estado      string // clave de traducción
	Clase       string
	EnviadaAt   time.Time
	FechaLimite time.Time
	Problemas   []vistaProblema
	Solicitudes []vistaSolicitudEnRevision
}

// vistaRevision es la vista de revisión de una postulación.
type vistaRevision struct {
	ID             int64
	Base           string // ruta de esta vista
	Bandeja        string
	Folio          string
	Titulo         string
	Edicion        string
	Subcategoria   string
	Estado         string
	Clase          string
	EnviadaAt      time.Time
	Envios         int
	MotivoDescarte string
	Titular        string
	CorreoTitular  string

	PuedeRevisar bool // revisor asignado o administrador; el observador, no
	Toma         string
	TomaPropia   bool

	Secciones     []vistaPasoResumido
	Documentos    []vistaDocumentosEnRevision
	Observaciones []vistaObservacion
	Obras         []vistaOpcion // para dirigir una observación a una obra
	Cambios       []vistaCambio
	Subsanaciones []vistaSubsanacionEnRevision
	Abierta       bool // hay una subsanación esperando al postulante

	// Formulario de subsanación.
	Elementos       []vistaOpcion
	FilasDeProblema []int
	FechaPropuesta  string // AAAA-MM-DDTHH:MM, hora de la Ciudad de México
	InvalidosAparte int    // documentos inválidos, que se añaden solos

	// Resultado de la última acción, junto al formulario que la hizo.
	Error   string
	ErrorEn string
}

// --- Bandeja ---------------------------------------------------------------------

// GET /revision   — las convocatorias que la sesión revisa o supervisa
func (a *App) portadaDeRevision(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	if !models.PuedeEntrarARevision(sesion) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	var ids []int64
	if models.PuedeSupervisar(sesion) {
		ediciones, err := a.datos.ListarEdiciones(r.Context())
		if err != nil {
			a.errorInterno(w, r, "listar ediciones", err)
			return
		}
		for _, e := range ediciones {
			convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), e.ID)
			if err != nil {
				a.errorInterno(w, r, "listar convocatorias", err)
				return
			}
			for _, c := range convocatorias {
				ids = append(ids, c.ID)
			}
		}
	} else {
		ids = models.RevisionesDe(sesion)
	}
	idioma := idiomaDe(r)
	var lista []vistaConvocatoriaEnRevision
	for _, id := range ids {
		cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
		if err != nil {
			a.errorInterno(w, r, "leer convocatoria", err)
			return
		}
		bandeja, err := a.datos.BandejaDeRevision(r.Context(), id, sesion.Usuario.ID, models.FiltroBandeja{})
		if err != nil {
			a.errorInterno(w, r, "leer bandeja", err)
			return
		}
		c := cfg.Convocatoria
		lista = append(lista, vistaConvocatoriaEnRevision{URL: fmt.Sprintf("/revision/convocatorias/%d", id), Titulo: c.Titulo(idioma),
			Edicion: c.Edicion.Nombre(idioma), Estado: "estado.convocatoria." + string(c.Estado), Clase: claseEstado(c.Estado), Total: len(bandeja)})
	}
	p := a.pagina(r, "revision.titulo")
	p.Datos = lista
	a.pintar(w, r, http.StatusOK, "revision_portada.html", p)
}

// clasesEnRevision asigna a cada estado, visto por el revisor, el estilo de
// su etiqueta. El revisor sí ve el descarte.
var clasesEnRevision = map[models.EstadoDePostulacion]string{
	models.PostulacionEnviada:       "proceso",
	models.PostulacionEnRevision:    "proceso",
	models.PostulacionEnSubsanacion: "proxima",
	models.PostulacionValidada:      "abierta",
	models.PostulacionDescartada:    "cerrada",
	models.PostulacionEnEvaluacion:  "abierta",
	models.PostulacionEvaluada:      "abierta",
}

// estadosEnRevision son los estados que puede tener una postulación en la
// bandeja, en el orden del proceso.
var estadosEnRevision = []models.EstadoDePostulacion{
	models.PostulacionEnviada, models.PostulacionEnRevision, models.PostulacionEnSubsanacion,
	models.PostulacionValidada, models.PostulacionDescartada, models.PostulacionEnEvaluacion, models.PostulacionEvaluada,
}

// GET /revision/convocatorias/{id}   — bandeja de una convocatoria
//
// Filtros en la URL, los mismos de la API: estado, subcategoria, toma (libre,
// mia, otra) y q (folio o nombre). Así la bandeja filtrada es un enlace.
func (a *App) bandejaDeRevision(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	sesion := sesionDe(r)
	if err != nil || !models.PuedeVerRevision(sesion, convocatoriaID) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		a.errorInterno(w, r, "leer convocatoria", err)
		return
	}
	consulta := r.URL.Query()
	filtro := models.FiltroBandeja{Estado: models.EstadoDePostulacion(consulta.Get("estado")),
		Subcategoria: models.Subcategoria(consulta.Get("subcategoria")), Toma: consulta.Get("toma"), Texto: strings.TrimSpace(consulta.Get("q"))}
	if !slices.Contains([]string{"", "libre", "mia", "otra"}, filtro.Toma) {
		filtro.Toma = ""
	}
	todas, err := a.datos.BandejaDeRevision(r.Context(), convocatoriaID, sesion.Usuario.ID, models.FiltroBandeja{})
	if err != nil {
		a.errorInterno(w, r, "leer bandeja", err)
		return
	}
	filtradas := todas
	if filtro != (models.FiltroBandeja{}) {
		if filtradas, err = a.datos.BandejaDeRevision(r.Context(), convocatoriaID, sesion.Usuario.ID, filtro); err != nil {
			a.errorInterno(w, r, "leer bandeja", err)
			return
		}
	}
	idioma, ahora := idiomaDe(r), a.ahora()
	base := fmt.Sprintf("/revision/convocatorias/%d", convocatoriaID)
	v := vistaBandeja{Base: base, Titulo: cfg.Convocatoria.Titulo(idioma), Edicion: cfg.Convocatoria.Edicion.Nombre(idioma),
		EsInterpretes: cfg.Convocatoria.Tipo == models.TipoInterpretes, Total: len(todas),
		Estado: string(filtro.Estado), Subcategoria: string(filtro.Subcategoria), Toma: filtro.Toma, Texto: filtro.Texto}
	cuentas := make(map[models.EstadoDePostulacion]int)
	for _, b := range todas {
		cuentas[b.Estado]++
	}
	for _, estado := range estadosEnRevision {
		v.Estados = append(v.Estados, vistaOpcion{Valor: string(estado), Texto: a.vistas.T(idioma, "estado.postulacion."+string(estado)), Elegida: estado == filtro.Estado})
		if cuentas[estado] > 0 {
			v.Tablero = append(v.Tablero, vistaCifraDeBandeja{Estado: "estado.postulacion." + string(estado), Clase: clasesEnRevision[estado],
				Cuenta: cuentas[estado], URL: base + "?estado=" + url.QueryEscape(string(estado)), Activa: estado == filtro.Estado})
		}
	}
	for _, b := range filtradas {
		renglon := vistaRenglonDeBandeja{
			URL: fmt.Sprintf("/revision/postulaciones/%d", b.ID), Folio: b.Folio, Nombre: b.Nombre, Subcategoria: string(b.Subcategoria),
			Estado: "estado.postulacion." + string(b.Estado), Clase: clasesEnRevision[b.Estado], EnviadaAt: b.EnviadaAt, Envios: b.Envios,
			Observaciones: b.Observaciones, DocumentosInvalidos: b.DocumentosInvalidos, FechaLimite: b.FechaLimite, Subsanada: b.Subsanada,
			Vencida: b.Estado == models.PostulacionEnSubsanacion && !b.FechaLimite.IsZero() && ahora.After(b.FechaLimite),
		}
		if b.Toma != nil {
			renglon.Toma, renglon.TomaPropia = b.Toma.Revisor, b.Toma.RevisorID == sesion.Usuario.ID
		}
		v.Renglones = append(v.Renglones, renglon)
	}
	p := a.pagina(r, "revision.titulo")
	p.Titulo = a.vistas.T(idioma, "revision.bandeja") + " · " + v.Titulo
	p.Datos = v
	a.pintar(w, r, http.StatusOK, "revision_bandeja.html", p)
}

// --- Vista de revisión -------------------------------------------------------------

// postulacionEnRevision es una postulación abierta por quien la revisa o la
// supervisa, con lo que la revisión ha dejado sobre ella.
type postulacionEnRevision struct {
	ab       *postulacionAbierta // en sólo lectura: el revisor no edita la postulación
	revision models.Revision
	titular  models.Usuario
}

// abrirParaRevision carga la postulación de la ruta para la vista de revisión.
// Quien no puede verla —un revisor de otra convocatoria, un borrador— recibe
// 404, igual que si no existiera.
func (a *App) abrirParaRevision(w http.ResponseWriter, r *http.Request, id int64) (*postulacionEnRevision, bool) {
	sesion := sesionDe(r)
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if err == nil && (!models.PuedeVerPostulacion(sesion, p) || !models.PuedeVerRevision(sesion, p.ConvocatoriaID) || p.Estado == models.PostulacionBorrador) {
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
	titular, err := a.datos.UsuarioPorID(r.Context(), p.UsuarioID)
	if err != nil {
		a.errorInterno(w, r, "leer el titular", err)
		return nil, false
	}
	revision, err := a.datos.RevisionDe(r.Context(), p.ID)
	if err != nil {
		a.errorInterno(w, r, "leer la revisión", err)
		return nil, false
	}
	// Sin ningún permiso de edición: todo se pinta en sólo lectura. Los
	// campos extendidos que se muestran son los que este perfil puede ver.
	ab := &postulacionAbierta{p: p, cfg: cfg, formulario: models.FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria),
		motivo: "error.postulacion_enviada", senalados: map[string]string{}}
	visibles := models.ExtendidosVisibles(&ab.p, cfg.Convocatoria.Tipo, cfg.Extendidos, sesion)
	ab.extendidos, ab.extendidosDeObra = models.ExtendidosDePostulacion(visibles, cfg.Convocatoria.Tipo, p.Subcategoria)
	ab.bloqueadores = models.ValidarParaEnvio(ab.p, cfg, titular.Usuario, a.ahora())
	return &postulacionEnRevision{ab: ab, revision: revision, titular: titular.Usuario}, true
}

// tipoDeVisor decide cómo se muestra un documento en el navegador.
func tipoDeVisor(mime string) string {
	switch {
	case mime == "application/pdf":
		return "pdf"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.HasPrefix(mime, "image/"):
		return "imagen"
	}
	return ""
}

// documentosEnRevision arma los documentos de la postulación y de cada obra,
// con su visor y su dictamen vigente.
func (a *App) documentosEnRevision(idioma string, pr *postulacionEnRevision) []vistaDocumentosEnRevision {
	dictamenes := make(map[int64]models.DictamenDocumento)
	for _, d := range pr.revision.Dictamenes {
		dictamenes[d.DocumentoID] = d
	}
	var grupos []vistaDocumentosEnRevision
	for _, g := range a.documentosDeVista(idioma, pr.ab) {
		grupo := vistaDocumentosEnRevision{Titulo: g.Titulo}
		for _, rq := range g.Requisitos {
			requisito := vistaRequisitoEnRevision{Etiqueta: rq.Etiqueta, Obligatorio: rq.Obligatorio}
			for _, d := range rq.Documentos {
				documento := vistaDocumentoEnRevision{ID: d.ID, Ancla: fmt.Sprintf("documento-%d", d.ID), Nombre: d.Nombre, URL: d.URL,
					Tamano: d.Tamano, Version: d.Version, Subido: d.Subido, Dictamen: "pendiente", Tipo: d.Tipo}
				if dictamen, hay := dictamenes[d.ID]; hay {
					documento.Dictamen, documento.Motivo = dictamen.Dictamen, dictamen.Motivo
				}
				requisito.Documentos = append(requisito.Documentos, documento)
			}
			grupo.Requisitos = append(grupo.Requisitos, requisito)
		}
		grupos = append(grupos, grupo)
	}
	return grupos
}

// elementosSenalables lista todo lo que un revisor puede señalar en una
// subsanación: campos, listas y documentos, por paso y por obra. Los campos
// extendidos no: no se desbloquean (RF-CLI-26).
func (a *App) elementosSenalables(idioma string, ab *postulacionAbierta) []vistaOpcion {
	var opciones []vistaOpcion
	agregar := func(clase models.ClaseProblema, campo string, obraID int64, prefijo string) {
		opciones = append(opciones, vistaOpcion{Valor: elementoDeDesbloqueo(clase, campo, obraID),
			Texto: prefijo + a.etiquetaDeElemento(idioma, ab, clase, campo, obraID)})
	}
	seccion := func(s string) string { return a.vistas.T(idioma, "seccion."+s) + " · " }
	for _, def := range ab.formulario.Campos {
		agregar(models.ProblemaCampo, def.Ruta, 0, seccion(models.SeccionDeElemento(models.ProblemaCampo, def.Ruta)))
	}
	if ab.formulario.TieneIntegrantes {
		agregar(models.ProblemaCampo, models.CampoIntegrantes, 0, seccion(models.SeccionDatos))
	}
	if ab.formulario.TieneVideos {
		agregar(models.ProblemaCampo, models.CampoVideos, 0, seccion(models.SeccionDatos))
	}
	agregar(models.ProblemaCampo, models.CampoObras, 0, "")
	for _, obra := range ab.p.Obras {
		for _, def := range ab.formulario.CamposObra {
			agregar(models.ProblemaCampo, def.Ruta, obra.ID, "")
		}
	}
	for _, grupo := range a.documentosDeVista(idioma, ab) {
		for _, rq := range grupo.Requisitos {
			agregar(models.ProblemaDocumento, rq.Clave, rq.ObraID, seccion(models.SeccionDocumentos))
		}
	}
	return opciones
}

// filasDeProblema es cuántos renglones vacíos ofrece el formulario de
// subsanación. Si hacen falta más, se pide otra subsanación después.
const filasDeProblema = 6

// vistaDeRevision arma la vista de revisión de una postulación.
func (a *App) vistaDeRevision(r *http.Request, pr *postulacionEnRevision) *vistaRevision {
	idioma, ahora, sesion := idiomaDe(r), a.ahora(), sesionDe(r)
	p, c := pr.ab.p, pr.ab.cfg.Convocatoria
	v := &vistaRevision{
		ID: p.ID, Base: fmt.Sprintf("/revision/postulaciones/%d", p.ID), Bandeja: fmt.Sprintf("/revision/convocatorias/%d", p.ConvocatoriaID),
		Folio: p.Folio, Titulo: c.Titulo(idioma), Edicion: c.Edicion.Nombre(idioma), Subcategoria: string(p.Subcategoria),
		Estado: "estado.postulacion." + string(p.Estado), Clase: clasesEnRevision[p.Estado], EnviadaAt: p.EnviadaAt, Envios: p.Envios,
		MotivoDescarte: p.MotivoDescarte, Titular: models.TitularParaMostrar(p, pr.titular).Nombre + " " + models.TitularParaMostrar(p, pr.titular).Apellidos,
		CorreoTitular: models.TitularParaMostrar(p, pr.titular).Email,
		PuedeRevisar:  models.PuedeRevisar(sesion, p.ConvocatoriaID),
	}
	if pr.revision.Toma != nil {
		v.Toma, v.TomaPropia = pr.revision.Toma.Revisor, pr.revision.Toma.RevisorID == sesion.Usuario.ID
	}

	// La postulación entera, en sólo lectura: la misma vista de cada paso.
	for _, seccion := range models.SeccionesDe(c.Tipo) {
		if seccion == models.SeccionEnvio || seccion == models.SeccionDocumentos {
			continue
		}
		paso := a.vistaDe(r, pr.ab, seccion)
		resumen := vistaPasoResumido{Seccion: seccion, Titulo: a.vistas.T(idioma, "seccion."+seccion), Grupos: paso.Grupos, Obras: paso.Obras}
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
	v.Documentos = a.documentosEnRevision(idioma, pr)

	// Observaciones internas.
	nombreDeObra := make(map[int64]string)
	for i, obra := range p.Obras {
		nombre := a.vistas.T(idioma, "postulacion.obra_n", i+1)
		if titulo := obra.Campos["obra.titulo"]; titulo != "" {
			nombre += " · " + titulo
		}
		nombreDeObra[obra.ID] = nombre
		v.Obras = append(v.Obras, vistaOpcion{Valor: strconv.FormatInt(obra.ID, 10), Texto: nombre})
	}
	for _, o := range pr.revision.Observaciones {
		v.Observaciones = append(v.Observaciones, vistaObservacion{Texto: o.Texto, Revisor: o.Revisor, Fecha: o.CreadoAt, Obra: nombreDeObra[o.ObraID]})
	}

	// Lo que cambió en el último reenvío (RF-REV-07).
	if cambios, err := a.datos.DiferenciasDelUltimoEnvio(r.Context(), p.ID); err == nil {
		for _, cambio := range cambios {
			v.Cambios = append(v.Cambios, vistaCambio{Etiqueta: a.etiquetaDeElemento(idioma, pr.ab, cambio.Clase, cambio.Campo, cambio.ObraID),
				Antes: cambio.Antes, Despues: cambio.Despues})
		}
	} else {
		a.log.Error("leer las diferencias del último envío", "postulacion", p.ID, "error", err)
	}

	// Subsanaciones, de la más reciente a la más antigua.
	for i := len(p.Subsanaciones) - 1; i >= 0; i-- {
		s := p.Subsanaciones[i]
		vs := vistaSubsanacionEnRevision{EnviadaAt: s.EnviadaAt, FechaLimite: s.FechaLimite, Estado: "revision.subsanacion_atendida", Clase: "abierta"}
		switch {
		case s.Pendiente() && ahora.After(s.FechaLimite):
			vs.Estado, vs.Clase = "revision.subsanacion_vencida", "cerrada"
		case s.Pendiente():
			vs.Estado, vs.Clase = "revision.subsanacion_pendiente", "proxima"
			v.Abierta = true
		case !s.CerradaAt.IsZero() && s.AtendidaAt.IsZero():
			vs.Estado, vs.Clase = "revision.subsanacion_cerrada", "cerrada"
		}
		base := fmt.Sprintf("/revision/postulaciones/%d", p.ID)
		for _, problema := range s.Problemas {
			vs.Problemas = append(vs.Problemas, vistaProblema{Etiqueta: a.etiquetaDeElemento(idioma, pr.ab, problema.Clase, problema.Campo, problema.ObraID),
				Problema: problema.Problema, URL: base + "#" + anclaDeElementoEnRevision(problema.Clase, problema.Campo, problema.ObraID, p)})
		}
		for _, solicitud := range s.Solicitudes {
			estado, clase := "subsanacion.solicitud_pendiente", "proxima"
			if solicitud.Resuelta && solicitud.Concedida {
				estado, clase = "subsanacion.solicitud_concedida", "abierta"
			} else if solicitud.Resuelta {
				estado, clase = "subsanacion.solicitud_negada", "cerrada"
			}
			vs.Solicitudes = append(vs.Solicitudes, vistaSolicitudEnRevision{ID: solicitud.ID, Pendiente: !solicitud.Resuelta,
				vistaSolicitud: vistaSolicitud{Etiqueta: a.etiquetaDeElemento(idioma, pr.ab, solicitud.Clase, solicitud.Campo, solicitud.ObraID),
					Justificacion: solicitud.Justificacion, Estado: estado, Clase: clase, Motivo: solicitud.MotivoRespuesta}})
		}
		v.Subsanaciones = append(v.Subsanaciones, vs)
	}

	// Formulario de subsanación: los elementos, renglones vacíos y una fecha
	// límite propuesta (diez días, a las 23:59 de la Ciudad de México).
	v.Elementos = a.elementosSenalables(idioma, pr.ab)
	v.FilasDeProblema = make([]int, filasDeProblema)
	for i := range v.FilasDeProblema {
		v.FilasDeProblema[i] = i + 1
	}
	propuesta := ahora.In(views.ZonaMexico()).AddDate(0, 0, 10)
	v.FechaPropuesta = aLocal(time.Date(propuesta.Year(), propuesta.Month(), propuesta.Day(), 23, 59, 0, 0, views.ZonaMexico()))
	for _, d := range pr.revision.Dictamenes {
		if d.Dictamen == "invalido" {
			v.InvalidosAparte++
		}
	}
	return v
}

// anclaDeElementoEnRevision lleva, en la vista de revisión, al elemento
// señalado: al documento vigente si es un documento; si no, a su paso.
func anclaDeElementoEnRevision(clase models.ClaseProblema, campo string, obraID int64, p models.Postulacion) string {
	if clase == models.ProblemaDocumento {
		for _, d := range p.Documentos {
			if d.Clave == campo && d.ObraID == obraID {
				return fmt.Sprintf("documento-%d", d.ID)
			}
		}
		return "documentos"
	}
	return "r-" + models.SeccionDeElemento(clase, campo)
}

// avisosDeRevision son los mensajes que una redirección pide mostrar.
var avisosDeRevision = map[string]views.Aviso{
	"tomada":      {Tipo: "exito", Clave: "revision.aviso_tomada"},
	"liberada":    {Tipo: "exito", Clave: "revision.aviso_liberada"},
	"dictamen":    {Tipo: "exito", Clave: "revision.aviso_dictamen_documento"},
	"observacion": {Tipo: "exito", Clave: "revision.aviso_observacion"},
	"subsanacion": {Tipo: "exito", Clave: "revision.aviso_subsanacion"},
	"desbloqueo":  {Tipo: "exito", Clave: "revision.aviso_desbloqueo"},
	"validada":    {Tipo: "exito", Clave: "revision.aviso_validada"},
	"descartada":  {Tipo: "exito", Clave: "revision.aviso_descartada"},
}

// pintarRevision envía la vista de revisión.
func (a *App) pintarRevision(w http.ResponseWriter, r *http.Request, estado int, v *vistaRevision, aviso *views.Aviso) {
	p := a.pagina(r, "revision.titulo")
	p.Titulo = v.Folio + " · " + a.vistas.T(p.Idioma, "revision.titulo")
	p.Datos, p.Aviso = v, aviso
	a.pintar(w, r, estado, "revision_postulacion.html", p)
}

// GET /revision/postulaciones/{id}   — vista de revisión
func (a *App) vistaDeRevisionHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	pr, ok := a.abrirParaRevision(w, r, id)
	if !ok {
		return
	}
	var aviso *views.Aviso
	if pedido, hay := avisosDeRevision[r.URL.Query().Get("aviso")]; hay {
		aviso = &pedido
	}
	a.pintarRevision(w, r, http.StatusOK, a.vistaDeRevision(r, pr), aviso)
}

// accionDeRevision es el esqueleto de toda acción del revisor: abre la
// postulación de la ruta, exige que la sesión pueda revisarla, ejecuta la
// operación y, si sale bien, vuelve a la vista con un aviso. Si el modelo la
// rechaza, vuelve a pintar la vista con el motivo junto al formulario (en).
func (a *App) accionDeRevision(w http.ResponseWriter, r *http.Request, en, aviso string,
	operacion func(pr *postulacionEnRevision) error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	pr, ok := a.abrirParaRevision(w, r, id)
	if !ok {
		return
	}
	if !models.PuedeRevisar(sesionDe(r), pr.ab.p.ConvocatoriaID) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	err = operacion(pr)
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
		http.Redirect(w, r, fmt.Sprintf("/revision/postulaciones/%d?aviso=%s#%s", id, aviso, en), http.StatusSeeOther)
		return
	case errors.As(err, &regla) && len(regla.Errores) > 0:
		// Se vuelve a leer: la vista sale de lo que quedó guardado.
		if pr, ok = a.abrirParaRevision(w, r, id); !ok {
			return
		}
		v := a.vistaDeRevision(r, pr)
		var mensajes []string
		for _, e := range regla.Errores {
			mensajes = append(mensajes, a.vistas.T(idiomaDe(r), e.Clave))
		}
		v.Error, v.ErrorEn = strings.Join(slices.Compact(mensajes), " "), en
		a.pintarRevision(w, r, http.StatusUnprocessableEntity, v, nil)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	default:
		a.errorInterno(w, r, "acción de revisión", err)
	}
}

// POST /revision/postulaciones/{id}/toma   — «la reviso yo» o la libera (accion=liberar)
func (a *App) tomarDesdeLaRevision(w http.ResponseWriter, r *http.Request) {
	tomar := r.PostFormValue("accion") != "liberar"
	aviso := "tomada"
	if !tomar {
		aviso = "liberada"
	}
	a.accionDeRevision(w, r, "toma", aviso, func(pr *postulacionEnRevision) error {
		return a.datos.TomarPostulacion(r.Context(), a.actorDe(r), pr.ab.p.ID, tomar)
	})
}

// POST /revision/postulaciones/{id}/documentos/{documento}/dictamen   — válido, inválido o pendiente
func (a *App) dictaminarDocumentoDesdeLaRevision(w http.ResponseWriter, r *http.Request) {
	documentoID, err := strconv.ParseInt(r.PathValue("documento"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	a.accionDeRevision(w, r, fmt.Sprintf("documento-%d", documentoID), "dictamen", func(pr *postulacionEnRevision) error {
		// El documento debe ser de ESTA postulación: la ruta no admite otro.
		if !slices.ContainsFunc(pr.ab.p.Documentos, func(d models.Documento) bool { return d.ID == documentoID }) {
			return models.ErrNoEncontrado
		}
		_, err := a.datos.DictaminarDocumento(r.Context(), a.actorDe(r), documentoID, r.PostFormValue("dictamen"), textoDeFormulario(r.PostFormValue("motivo")))
		return err
	})
}

// POST /revision/postulaciones/{id}/observaciones   — nota interna (texto, obra)
func (a *App) observarDesdeLaRevision(w http.ResponseWriter, r *http.Request) {
	a.accionDeRevision(w, r, "observaciones", "observacion", func(pr *postulacionEnRevision) error {
		obraID, _ := strconv.ParseInt(r.PostFormValue("obra"), 10, 64)
		return a.datos.ObservarPostulacion(r.Context(), a.actorDe(r), pr.ab.p.ID, obraID, textoDeFormulario(r.PostFormValue("texto")))
	})
}

// POST /revision/postulaciones/{id}/subsanacion   — solicita una subsanación
//
// Campos: elemento y problema (repetidos, uno por renglón; los renglones sin
// elemento no cuentan), fecha_limite (AAAA-MM-DDTHH:MM, hora de la Ciudad de
// México) y nota. Los documentos inválidos se añaden solos.
func (a *App) subsanacionDesdeLaRevision(w http.ResponseWriter, r *http.Request) {
	a.accionDeRevision(w, r, "subsanacion", "subsanacion", func(pr *postulacionEnRevision) error {
		if err := r.ParseForm(); err != nil {
			return err
		}
		nueva := models.NuevaSubsanacion{Nota: textoDeFormulario(r.PostFormValue("nota"))}
		problemas := r.PostForm["problema"]
		for i, elemento := range r.PostForm["elemento"] {
			partes := strings.SplitN(elemento, "|", 3)
			if len(partes) != 3 {
				continue // renglón sin elemento: no cuenta
			}
			obraID, _ := strconv.ParseInt(partes[2], 10, 64)
			problema := ""
			if i < len(problemas) {
				problema = textoDeFormulario(problemas[i])
			}
			nueva.Problemas = append(nueva.Problemas, models.ProblemaSubsanacion{Clase: models.ClaseProblema(partes[0]), Campo: partes[1], ObraID: obraID, Problema: problema})
		}
		// Una fecha escrita que no se entiende se rechaza: tomar otra en su
		// lugar le daría al postulante un plazo que nadie eligió.
		if texto := strings.TrimSpace(r.PostFormValue("fecha_limite")); texto != "" {
			fecha := deLocal(texto)
			if fecha.IsZero() {
				return &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "fecha_limite", Clave: "error.fecha_hora_invalida"}}}
			}
			nueva.FechaLimite = fecha
		}
		return a.solicitarSubsanacion(r, pr.ab.p, nueva)
	})
}

// POST /revision/postulaciones/{id}/desbloqueos/{solicitud}   — concede o niega (decision, motivo)
func (a *App) desbloqueoDesdeLaRevision(w http.ResponseWriter, r *http.Request) {
	solicitudID, err := strconv.ParseInt(r.PathValue("solicitud"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	a.accionDeRevision(w, r, "subsanaciones", "desbloqueo", func(pr *postulacionEnRevision) error {
		return a.datos.ResolverDesbloqueo(r.Context(), a.actorDe(r), pr.ab.p.ID, solicitudID,
			r.PostFormValue("decision") == "conceder", textoDeFormulario(r.PostFormValue("motivo")))
	})
}

// POST /revision/postulaciones/{id}/dictamen   — validada o descartada (dictamen, motivo)
//
// Nada se comunica al postulante: el descarte es interno (decisión de la CNMO).
func (a *App) dictamenDesdeLaRevision(w http.ResponseWriter, r *http.Request) {
	dictamen := models.EstadoDePostulacion(r.PostFormValue("dictamen"))
	aviso := "validada"
	if dictamen == models.PostulacionDescartada {
		aviso = "descartada"
	}
	a.accionDeRevision(w, r, "dictamen", aviso, func(pr *postulacionEnRevision) error {
		return a.datos.DictaminarPostulacion(r.Context(), a.actorDe(r), pr.ab.p.ID, dictamen, textoDeFormulario(r.PostFormValue("motivo")))
	})
}
