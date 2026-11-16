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
	"html/template"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Plantillas de correo (§5.8, paso 13d): el administrador edita el asunto y
// el cuerpo de cada correo del sistema, en español y en inglés, con vista
// previa y correo de prueba. Cada cambio es una versión nueva; ninguna se
// edita ni se borra. Los mensajes ya encolados no cambian.

// --- Lo común a la API y a las pantallas ---------------------------------------------

// plantillaDeLaRuta lee {clave} e {idioma} de la ruta y comprueba que la
// plantilla exista en el catálogo y que el idioma sea es o en.
func plantillaDeLaRuta(r *http.Request) (models.DefinicionDePlantilla, string, bool) {
	definicion, existe := models.DefinicionDePlantillaCorreo(r.PathValue("clave"))
	idioma := r.PathValue("idioma")
	return definicion, idioma, existe && (idioma == models.IdiomaES || idioma == models.IdiomaEN)
}

// ejemploDeVariables son valores de muestra para la vista previa y el correo
// de prueba: verosímiles, en el idioma de la plantilla, y con enlaces que no
// llevan a ninguna cuenta real.
func (a *App) ejemploDeVariables(idioma string, nombreDeQuienPrueba string) map[string]string {
	ahora := a.ahora()
	ejemplo := map[string]string{
		"nombre": nombreDeQuienPrueba, "folio": "FIM49-INT-0137", "email_nuevo": "nuevo@ejemplo.mx",
		"convocatoria": "Convocatoria de Intérpretes", "edicion": "Edición 49 · 2027",
		"liga":            a.urlBase + "/cuenta",
		"cierre":          a.vistas.FechaHora(idioma, ahora.Add(72*time.Hour)),
		"fecha_limite":    a.vistas.FechaHora(idioma, ahora.Add(5*24*time.Hour)),
		"vence":           a.vistas.FechaHora(idioma, ahora.Add(models.VigenciaDeExportacion)),
		"lista_problemas": "• Partitura de la obra 1: faltan las páginas 3 y 4.\n• Teléfono: falta la clave lada.",
		"nota":            "", "numero": "42", "libre": "8.5 GB", "umbral": "20 GB", "datos": "/var/lib/fimnme",
	}
	if idioma == models.IdiomaEN {
		ejemplo["convocatoria"], ejemplo["edicion"] = "Call for Performers", "49th Edition · 2027"
		ejemplo["lista_problemas"] = "• Score of work 1: pages 3 and 4 are missing.\n• Telephone: the area code is missing."
	}
	return ejemplo
}

// versionesConVigente lee las versiones de una plantilla y devuelve también
// la vigente: la más reciente en ese idioma o, si no hay ninguna, la que se
// usaría (la española).
func (a *App) versionesConVigente(r *http.Request, clave, idioma string) ([]models.VersionDePlantilla, models.PlantillaCorreo, error) {
	versiones, err := a.datos.VersionesDePlantilla(r.Context(), clave, idioma)
	if err != nil {
		return nil, models.PlantillaCorreo{}, err
	}
	if len(versiones) > 0 {
		return versiones, versiones[0].PlantillaCorreo, nil
	}
	vigente, err := a.datos.PlantillaCorreo(r.Context(), clave, idioma)
	if errors.Is(err, models.ErrNoEncontrado) {
		return versiones, models.PlantillaCorreo{Clave: clave, Idioma: idioma}, nil
	}
	return versiones, vigente, err
}

// correoDePrueba arma el mensaje de prueba de una plantilla con valores de
// muestra, dirigido a quien lo pide. El asunto lleva una marca para que nadie
// lo confunda con un correo real.
func (a *App) correoDePrueba(r *http.Request, p models.PlantillaCorreo) models.CorreoSaliente {
	sesion := sesionDe(r)
	correo := views.RenderCorreo(p, sesion.Usuario.Email, a.ejemploDeVariables(p.Idioma, sesion.Usuario.Nombre))
	correo.Asunto = a.vistas.T(p.Idioma, "correo.marca_prueba") + " " + correo.Asunto
	return correo
}

// --- API -------------------------------------------------------------------------

// versionJSON es una versión de una plantilla en una respuesta.
func versionJSON(v models.VersionDePlantilla) map[string]any {
	return map[string]any{"version": v.Version, "asunto": v.Asunto, "cuerpo_md": v.CuerpoMD, "creado_at": v.CreadoAt.UTC().Format(time.RFC3339)}
}

// plantillaJSON es una plantilla con su definición y sus versiones en un
// idioma.
func plantillaJSON(d models.DefinicionDePlantilla, idioma string, versiones []models.VersionDePlantilla) map[string]any {
	lista := []map[string]any{}
	for _, v := range versiones {
		lista = append(lista, versionJSON(v))
	}
	return map[string]any{"clave": d.Clave, "idioma": idioma, "variables": d.Variables, "obligatorias": d.Obligatorias, "versiones": lista}
}

// GET /api/v1/plantillas-correo   — catálogo de plantillas con su versión vigente en cada idioma
func (a *App) apiPlantillasDeCorreo(w http.ResponseWriter, r *http.Request) {
	plantillas := []map[string]any{}
	for _, d := range models.PlantillasDeCorreo {
		vigentes := map[string]any{}
		for _, idioma := range []string{models.IdiomaES, models.IdiomaEN} {
			versiones, err := a.datos.VersionesDePlantilla(r.Context(), d.Clave, idioma)
			if a.falloDatos(w, r, err) {
				return
			}
			vigentes[idioma] = nil
			if len(versiones) > 0 {
				vigentes[idioma] = versionJSON(versiones[0])
			}
		}
		plantillas = append(plantillas, map[string]any{"clave": d.Clave, "variables": d.Variables, "obligatorias": d.Obligatorias, "vigentes": vigentes})
	}
	a.responder(w, http.StatusOK, map[string]any{"plantillas": plantillas})
}

// GET /api/v1/plantillas-correo/{clave}/{idioma}   — historial de versiones de una plantilla
func (a *App) apiPlantillaDeCorreo(w http.ResponseWriter, r *http.Request) {
	a.responderPlantilla(w, r, http.StatusOK)
}

// responderPlantilla responde con una plantilla y sus versiones.
func (a *App) responderPlantilla(w http.ResponseWriter, r *http.Request, estado int) {
	definicion, idioma, ok := plantillaDeLaRuta(r)
	if !ok {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	versiones, err := a.datos.VersionesDePlantilla(r.Context(), definicion.Clave, idioma)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, estado, plantillaJSON(definicion, idioma, versiones))
}

// entradaDePlantilla es el cuerpo de las operaciones que reciben un texto.
type entradaDePlantilla struct {
	Asunto   string `json:"asunto"`
	CuerpoMD string `json:"cuerpo_md"`
}

// POST /api/v1/plantillas-correo/{clave}/{idioma}   — guarda una versión nueva
//
// Cuerpo: {"asunto", "cuerpo_md"}. Pasa a ser la vigente; se rechaza si usa
// una variable que la plantilla no recibe o le falta una indispensable.
func (a *App) apiGuardarPlantillaDeCorreo(w http.ResponseWriter, r *http.Request) {
	definicion, idioma, ok := plantillaDeLaRuta(r)
	var entrada entradaDePlantilla
	if !ok {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	_, err := a.datos.GuardarVersionDePlantilla(r.Context(), a.actorDe(r),
		models.PlantillaCorreo{Clave: definicion.Clave, Idioma: idioma, Asunto: entrada.Asunto, CuerpoMD: entrada.CuerpoMD})
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPlantilla(w, r, http.StatusCreated)
}

// POST /api/v1/plantillas-correo/{clave}/{idioma}/vista-previa   — arma el correo con valores de muestra
//
// Cuerpo: {"asunto", "cuerpo_md"}. No guarda nada. Dice además qué
// variables no reconoce y cuáles indispensables faltan.
func (a *App) apiVistaPreviaDeCorreo(w http.ResponseWriter, r *http.Request) {
	definicion, idioma, ok := plantillaDeLaRuta(r)
	var entrada entradaDePlantilla
	if !ok {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	p := models.PlantillaCorreo{Clave: definicion.Clave, Idioma: idioma, Asunto: entrada.Asunto, CuerpoMD: entrada.CuerpoMD}
	asunto, cuerpo, texto := views.VistaPreviaDeCorreo(p, a.ejemploDeVariables(idioma, sesionDe(r).Usuario.Nombre))
	todo := entrada.Asunto + "\n" + entrada.CuerpoMD
	a.responder(w, http.StatusOK, map[string]any{"asunto": asunto, "html": string(cuerpo), "texto": texto,
		"variables_desconocidas": noNulo(definicion.VariablesDesconocidas(todo)), "variables_faltantes": noNulo(definicion.VariablesFaltantes(todo))})
}

// noNulo convierte una lista vacía en [] para el JSON.
func noNulo(lista []string) []string {
	if lista == nil {
		return []string{}
	}
	return lista
}

// POST /api/v1/plantillas-correo/{clave}/{idioma}/prueba   — envía un correo de prueba a quien lo pide
//
// Cuerpo opcional: {"asunto", "cuerpo_md"} para probar un texto aún sin
// guardar; sin él, la versión vigente. Va a la bandeja de salida como
// cualquier correo, con una marca en el asunto.
func (a *App) apiPruebaDeCorreo(w http.ResponseWriter, r *http.Request) {
	definicion, idioma, ok := plantillaDeLaRuta(r)
	if !ok {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	var entrada entradaDePlantilla
	if r.ContentLength != 0 && !a.leerJSON(w, r, &entrada) {
		return
	}
	destinatario, err := a.enviarPrueba(r, definicion, idioma, entrada.Asunto, entrada.CuerpoMD)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusAccepted, map[string]any{"destinatario": destinatario})
}

// enviarPrueba encola el correo de prueba de un texto (o de la versión
// vigente, si el texto viene vacío) y devuelve a quién va. Un texto con
// variables que no existen no se envía.
func (a *App) enviarPrueba(r *http.Request, d models.DefinicionDePlantilla, idioma, asunto, cuerpo string) (string, error) {
	p := models.PlantillaCorreo{Clave: d.Clave, Idioma: idioma, Asunto: asunto, CuerpoMD: cuerpo}
	if strings.TrimSpace(asunto) == "" && strings.TrimSpace(cuerpo) == "" {
		_, vigente, err := a.versionesConVigente(r, d.Clave, idioma)
		if err != nil {
			return "", err
		}
		p = vigente
	}
	if errores := models.ValidarVersionDePlantilla(p); len(errores) > 0 {
		return "", &models.ErrorValidacion{Errores: errores}
	}
	correo := a.correoDePrueba(r, p)
	return correo.Destinatario, a.datos.EncolarAviso(r.Context(), correo)
}

// --- Pantallas -----------------------------------------------------------------

// vistaPlantillaEnLista es una plantilla en la lista de la administración.
type vistaPlantillaEnLista struct {
	Clave     string
	Nombre    string // clave de traducción
	Proposito string // clave de traducción
	PorIdioma []vistaIdiomaDePlantilla
}

// vistaIdiomaDePlantilla es la versión vigente de una plantilla en un idioma.
type vistaIdiomaDePlantilla struct {
	Idioma   string
	URL      string
	Version  int
	CreadoAt time.Time
	Falta    bool // no hay versión en este idioma: se usa la española
}

// vistaVariableDePlantilla es una variable que la plantilla recibe.
type vistaVariableDePlantilla struct {
	Nombre      string
	Marca       string // {{nombre}}
	Descripcion string // clave de traducción
	Obligatoria bool
}

// vistaEditorDePlantilla es el editor de una plantilla en un idioma.
type vistaEditorDePlantilla struct {
	Base                    string
	Nombre, Proposito       string
	Idioma                  string
	OtroIdioma              string
	Asunto, Cuerpo          string
	Cargada                 int // versión cargada en el editor, si no es la vigente
	Vigente                 int
	FaltaEnIdioma           bool
	Variables               []vistaVariableDePlantilla
	Versiones               []vistaVersionDePlantilla
	PrevAsunto              string
	PrevCuerpo              template.HTML
	PrevTexto               string
	Desconocidas, Faltantes []string
	Error                   string
	PuedeAdministrar        bool
}

// vistaVersionDePlantilla es una versión en el historial.
type vistaVersionDePlantilla struct {
	Version  int
	Asunto   string
	CreadoAt time.Time
	Cargar   string
	Vigente  bool
}

// GET /administracion/correos   — las plantillas de correo y su versión vigente
func (a *App) plantillasDeCorreoAdmin(w http.ResponseWriter, r *http.Request) {
	if !a.exigirSupervision(w, r) {
		return
	}
	var lista []vistaPlantillaEnLista
	for _, d := range models.PlantillasDeCorreo {
		v := vistaPlantillaEnLista{Clave: d.Clave, Nombre: "correo.plantilla." + d.Clave, Proposito: "correo.proposito." + d.Clave}
		for _, idioma := range []string{models.IdiomaES, models.IdiomaEN} {
			versiones, err := a.datos.VersionesDePlantilla(r.Context(), d.Clave, idioma)
			if err != nil {
				a.errorInterno(w, r, "versiones de plantilla", err)
				return
			}
			vi := vistaIdiomaDePlantilla{Idioma: idioma, URL: fmt.Sprintf("/administracion/correos/%s/%s", d.Clave, idioma), Falta: len(versiones) == 0}
			if len(versiones) > 0 {
				vi.Version, vi.CreadoAt = versiones[0].Version, versiones[0].CreadoAt
			}
			v.PorIdioma = append(v.PorIdioma, vi)
		}
		lista = append(lista, v)
	}
	p := a.pagina(r, "correo.titulo")
	p.Datos = lista
	a.pintar(w, r, http.StatusOK, "admin_correos.html", p)
}

// GET /administracion/correos/{clave}/{idioma}?version=   — editor de una plantilla
//
// Con «version», el editor se llena con esa versión anterior, para
// restaurarla guardándola de nuevo.
func (a *App) editorDePlantilla(w http.ResponseWriter, r *http.Request) {
	a.pintarEditorDePlantilla(w, r, http.StatusOK, nil, "")
}

// pintarEditorDePlantilla muestra el editor. enviado es lo que el
// administrador escribió (para la vista previa o tras un rechazo); nil, lo
// guardado.
func (a *App) pintarEditorDePlantilla(w http.ResponseWriter, r *http.Request, estado int, enviado *models.PlantillaCorreo, mensaje string) {
	if !a.exigirSupervision(w, r) {
		return
	}
	definicion, idioma, ok := plantillaDeLaRuta(r)
	if !ok {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	versiones, vigente, err := a.versionesConVigente(r, definicion.Clave, idioma)
	if err != nil {
		a.errorInterno(w, r, "versiones de plantilla", err)
		return
	}
	base := fmt.Sprintf("/administracion/correos/%s/%s", definicion.Clave, idioma)
	otro := models.IdiomaEN
	if idioma == models.IdiomaEN {
		otro = models.IdiomaES
	}
	v := vistaEditorDePlantilla{Base: base, Nombre: "correo.plantilla." + definicion.Clave, Proposito: "correo.proposito." + definicion.Clave,
		Idioma: idioma, OtroIdioma: fmt.Sprintf("/administracion/correos/%s/%s", definicion.Clave, otro),
		Asunto: vigente.Asunto, Cuerpo: vigente.CuerpoMD, FaltaEnIdioma: len(versiones) == 0, Error: mensaje,
		PuedeAdministrar: models.PuedeAdministrar(sesionDe(r))}
	if len(versiones) > 0 {
		v.Vigente = versiones[0].Version
	}
	pedida := r.URL.Query().Get("version")
	for _, version := range versiones {
		numero := fmt.Sprint(version.Version)
		v.Versiones = append(v.Versiones, vistaVersionDePlantilla{Version: version.Version, Asunto: version.Asunto, CreadoAt: version.CreadoAt,
			Cargar: base + "?version=" + numero + "#editor", Vigente: version.Version == v.Vigente})
		if pedida == numero && version.Version != v.Vigente {
			v.Asunto, v.Cuerpo, v.Cargada = version.Asunto, version.CuerpoMD, version.Version
		}
	}
	if enviado != nil {
		v.Asunto, v.Cuerpo = enviado.Asunto, enviado.CuerpoMD
	}
	for _, nombre := range definicion.Variables {
		v.Variables = append(v.Variables, vistaVariableDePlantilla{Nombre: nombre, Marca: "{{" + nombre + "}}",
			Descripcion: "correo.variable." + nombre, Obligatoria: slices.Contains(definicion.Obligatorias, nombre)})
	}
	todo := v.Asunto + "\n" + v.Cuerpo
	v.Desconocidas, v.Faltantes = definicion.VariablesDesconocidas(todo), definicion.VariablesFaltantes(todo)
	prev := models.PlantillaCorreo{Clave: definicion.Clave, Idioma: idioma, Asunto: v.Asunto, CuerpoMD: v.Cuerpo}
	v.PrevAsunto, v.PrevCuerpo, v.PrevTexto = views.VistaPreviaDeCorreo(prev, a.ejemploDeVariables(idioma, sesionDe(r).Usuario.Nombre))
	p := a.pagina(r, "correo.titulo")
	p.Titulo = a.vistas.T(p.Idioma, v.Nombre) + " · " + a.vistas.T(p.Idioma, "correo.titulo")
	p.Datos = v
	if aviso, hay := avisosDeCorreo[r.URL.Query().Get("aviso")]; hay {
		p.Aviso = &aviso
	}
	a.pintar(w, r, estado, "admin_correo.html", p)
}

// avisosDeCorreo son los mensajes que una redirección pide mostrar.
var avisosDeCorreo = map[string]views.Aviso{
	"guardada": {Tipo: "exito", Clave: "correo.aviso_guardada"},
	"prueba":   {Tipo: "exito", Clave: "correo.aviso_prueba"},
}

// POST /administracion/correos/{clave}/{idioma}   — vista previa, nueva versión o prueba (accion, asunto, cuerpo_md)
//
// Un solo formulario con tres botones: «accion» dice cuál se pulsó. La vista
// previa no guarda nada; guardar crea una versión nueva; la prueba manda el
// texto escrito a la cuenta de quien lo pide.
func (a *App) accionDePlantilla(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	definicion, idioma, ok := plantillaDeLaRuta(r)
	if !ok {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	enviado := models.PlantillaCorreo{Clave: definicion.Clave, Idioma: idioma,
		Asunto: r.PostFormValue("asunto"), CuerpoMD: strings.ReplaceAll(r.PostFormValue("cuerpo_md"), "\r\n", "\n")}
	base := fmt.Sprintf("/administracion/correos/%s/%s", definicion.Clave, idioma)
	var err error
	aviso := ""
	switch r.PostFormValue("accion") {
	case "guardar":
		_, err = a.datos.GuardarVersionDePlantilla(r.Context(), a.actorDe(r), enviado)
		aviso = "guardada"
	case "prueba":
		_, err = a.enviarPrueba(r, definicion, idioma, enviado.Asunto, enviado.CuerpoMD)
		aviso = "prueba"
	default:
		// Vista previa: se repinta con lo escrito, sin guardar.
		a.pintarEditorDePlantilla(w, r, http.StatusOK, &enviado, "")
		return
	}
	if err == nil {
		http.Redirect(w, r, base+"?aviso="+aviso+"#editor", http.StatusSeeOther)
		return
	}
	mensaje, rechazo := a.mensajeDeRechazo(r, err)
	if !rechazo {
		a.errorInterno(w, r, "plantilla de correo", err)
		return
	}
	a.pintarEditorDePlantilla(w, r, http.StatusUnprocessableEntity, &enviado, mensaje)
}
