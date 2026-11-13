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
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Páginas públicas: portada, detalle de convocatoria, aviso de privacidad,
// cambio de idioma y páginas de error.

// vistaConvocatoria es el modelo de vista de una convocatoria: lo que la
// plantilla necesita, ya resuelto en el idioma de la petición. Las plantillas
// no deciden nada; sólo pintan estos campos.
type vistaConvocatoria struct {
	URL           string
	Titulo        string
	Edicion       string
	Estado        string // clave de traducción del estado
	EstadoClase   string // modificador CSS de la etiqueta de estado
	Abierta       bool   // admite postulaciones ahora
	Proxima       bool   // publicada, aún sin captura
	TieneFechas   bool   // la etapa de postulación tiene calendario
	Apertura      time.Time
	Cierre        time.Time
	Etapas        []vistaEtapa
	Bases         template.HTML
	BasesRespaldo bool // se muestran en español porque falta la versión inglesa
	// BasesPDF es la dirección de las bases en PDF, o "" si no se han cargado.
	// BasesPDFRespaldo indica que el PDF está en español aunque la visita sea
	// en inglés. Imagen es la dirección de la imagen de cabecera, o "".
	BasesPDF         string
	BasesPDFRespaldo bool
	Imagen           string
}

// vistaEtapa es un renglón del calendario de la convocatoria.
type vistaEtapa struct {
	Nombre  string // clave de traducción
	Inicia  time.Time
	Termina time.Time
	Actual  bool // es la etapa en curso
}

// etapaActual relaciona el estado de la convocatoria con el tramo del
// calendario que debe resaltarse.
var etapaActual = map[models.EstadoConvocatoria]models.ClaveEtapa{
	models.EstadoPostulacion:        models.EtapaPostulacion,
	models.EstadoRevisionDocumental: models.EtapaRevisionDocumental,
	models.EstadoEvaluacion:         models.EtapaEvaluacion,
}

// claseEstado asigna el estilo de la etiqueta según el estado.
func claseEstado(estado models.EstadoConvocatoria) string {
	switch estado {
	case models.EstadoPostulacion:
		return "abierta"
	case models.EstadoPublicada:
		return "proxima"
	case models.EstadoRevisionDocumental, models.EstadoEvaluacion:
		return "proceso"
	default:
		return "cerrada"
	}
}

// rutaConvocatoria es el enlace permanente de una convocatoria (RNF-17).
func rutaConvocatoria(c models.Convocatoria) string {
	return fmt.Sprintf("/convocatorias/%d/%s", c.Edicion.Numero, c.Tipo)
}

// aVista convierte la entidad en su modelo de vista. Con detalle=true incluye
// calendario completo y bases, que la portada no necesita.
func aVista(c models.Convocatoria, idioma string, detalle bool) vistaConvocatoria {
	v := vistaConvocatoria{
		URL:         rutaConvocatoria(c),
		Titulo:      c.Titulo(idioma),
		Edicion:     c.Edicion.Nombre(idioma),
		Estado:      "estado.convocatoria." + string(c.Estado),
		EstadoClase: claseEstado(c.Estado),
		Abierta:     models.AdmitePostulaciones(c),
		Proxima:     c.Estado == models.EstadoPublicada,
	}
	if etapa, ok := c.Etapa(models.EtapaPostulacion); ok {
		v.TieneFechas = true
		v.Apertura, v.Cierre = etapa.IniciaAt, etapa.TerminaAt
	}
	if !detalle {
		return v
	}
	for _, e := range c.Etapas {
		v.Etapas = append(v.Etapas, vistaEtapa{
			Nombre:  "etapa." + string(e.Clave),
			Inicia:  e.IniciaAt,
			Termina: e.TerminaAt,
			Actual:  etapaActual[c.Estado] == e.Clave,
		})
	}
	bases, respaldo := c.BasesMD(idioma)
	v.Bases = views.Markdown(bases)
	v.BasesRespaldo = respaldo
	return v
}

// pagina arma el dato raíz común a todas las plantillas.
func (a *App) pagina(r *http.Request, claveTitulo string) *views.Pagina {
	idioma := idiomaDe(r)
	return &views.Pagina{
		Idioma:  idioma,
		Ruta:    r.URL.Path,
		Titulo:  a.vistas.T(idioma, claveTitulo),
		Entorno: a.entorno,
		Sesion:  sesionDe(r),
	}
}

// pintar envía una página HTML. Si la plantilla falla se registra el error;
// el motor de vistas garantiza que no se haya enviado media página.
func (a *App) pintar(w http.ResponseWriter, r *http.Request, estado int, plantilla string, p *views.Pagina) {
	// El HTML nunca se guarda en cachés: cambia con el idioma y, pronto, con
	// la sesión.
	w.Header().Set("Cache-Control", "no-store")
	if err := a.vistas.Render(w, estado, plantilla, p); err != nil {
		a.log.Error("no se pudo pintar la página", "plantilla", plantilla, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// inicio muestra la portada con las convocatorias vigentes.
func (a *App) inicio(w http.ResponseWriter, r *http.Request) {
	convocatorias, err := a.datos.ConvocatoriasPublicas(r.Context())
	if err != nil {
		a.log.Error("no se pudieron leer las convocatorias", "error", err)
		a.errorPagina(w, r, http.StatusInternalServerError)
		return
	}
	idioma := idiomaDe(r)
	vistas := make([]vistaConvocatoria, 0, len(convocatorias))
	for _, c := range convocatorias {
		vistas = append(vistas, aVista(c, idioma, false))
	}
	p := a.pagina(r, "inicio.titulo")
	p.Datos = struct{ Convocatorias []vistaConvocatoria }{vistas}
	a.pintar(w, r, http.StatusOK, "inicio.html", p)
}

// convocatoria muestra las bases y el calendario de una convocatoria.
// Ruta permanente: /convocatorias/{edicion}/{tipo}, p. ej. /convocatorias/49/interpretes.
func (a *App) convocatoria(w http.ResponseWriter, r *http.Request) {
	numero, err := strconv.Atoi(r.PathValue("edicion"))
	tipo := models.TipoConvocatoria(r.PathValue("tipo"))
	if err != nil || numero <= 0 || !tipo.Valido() {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	c, err := a.datos.ConvocatoriaPublica(r.Context(), numero, tipo)
	switch {
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
		return
	case err != nil:
		a.log.Error("no se pudo leer la convocatoria", "error", err)
		a.errorPagina(w, r, http.StatusInternalServerError)
		return
	}
	archivos, err := a.datos.ArchivosDeConvocatoria(r.Context(), c.ID)
	if err != nil {
		a.log.Error("no se pudieron leer los archivos de la convocatoria", "error", err)
		a.errorPagina(w, r, http.StatusInternalServerError)
		return
	}
	p := a.pagina(r, "inicio.titulo")
	vista := aVista(c, p.Idioma, true)
	// Las bases en PDF se ofrecen en el idioma de la visita; si no hay
	// versión inglesa se ofrece la española, avisándolo.
	_, hayES := archivos[models.ArchivoBasesES]
	_, hayEN := archivos[models.ArchivoBasesEN]
	switch {
	case p.Idioma == models.IdiomaEN && hayEN:
		vista.BasesPDF = vista.URL + "/bases.pdf?idioma=en"
	case hayES:
		vista.BasesPDF, vista.BasesPDFRespaldo = vista.URL+"/bases.pdf?idioma=es", p.Idioma == models.IdiomaEN
	}
	if _, hay := archivos[models.ArchivoImagen]; hay {
		vista.Imagen = vista.URL + "/imagen"
	}
	p.Titulo = vista.Titulo + " · " + vista.Edicion
	p.Datos = vista
	a.pintar(w, r, http.StatusOK, "convocatoria.html", p)
}

// avisoPrivacidad muestra el aviso de privacidad (LFPDPPP).
func (a *App) avisoPrivacidad(w http.ResponseWriter, r *http.Request) {
	a.pintar(w, r, http.StatusOK, "aviso_privacidad.html", a.pagina(r, "aviso.titulo"))
}

// cambiarIdioma guarda el idioma elegido y regresa a la página desde la que
// se pidió el cambio. Con sesión, el idioma se guarda además en la cuenta,
// que es la preferencia que manda (RF-I18N-03) y la que rige sus correos.
func (a *App) cambiarIdioma(w http.ResponseWriter, r *http.Request) {
	if idioma := r.PostFormValue("idioma"); models.IdiomaValido(idioma) {
		if s := sesionDe(r); s != nil {
			u := s.Usuario
			if err := a.datos.ActualizarDatosPersonales(r.Context(), a.actorDe(r), u.ID, u.Nombre, u.Apellidos, idioma); err != nil {
				a.errorInterno(w, r, "guardar idioma de la cuenta", err)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieIdioma,
			Value:    idioma,
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			HttpOnly: true,
			Secure:   esHTTPS(r),
			SameSite: http.SameSiteLaxMode,
		})
	}
	http.Redirect(w, r, rutaLocal(r.PostFormValue("volver")), http.StatusSeeOther)
}

// rutaLocal valida un destino de redirección: sólo se aceptan rutas de este
// mismo sitio. Cualquier otra cosa ("//otro.sitio", "https://…", "/\…") se
// sustituye por la portada, para no servir de trampolín hacia sitios ajenos.
func rutaLocal(destino string) string {
	if !strings.HasPrefix(destino, "/") || strings.HasPrefix(destino, "//") ||
		strings.ContainsAny(destino, "\\\r\n\t") {
		return "/"
	}
	return destino
}

// noEncontrado atiende cualquier ruta sin manejador.
func (a *App) noEncontrado(w http.ResponseWriter, r *http.Request) {
	a.errorPagina(w, r, http.StatusNotFound)
}

// errorPagina responde un error según el estado HTTP: una página en el idioma
// del visitante o, en la API, un cuerpo JSON.
func (a *App) errorPagina(w http.ResponseWriter, r *http.Request, estado int) {
	codigo := estado
	// Sólo hay textos propios para estos estados; lo demás se presenta como 500.
	switch estado {
	case http.StatusNotFound, http.StatusForbidden, http.StatusTooManyRequests:
	default:
		codigo = http.StatusInternalServerError
	}
	if esAPI(r) {
		a.errorAPI(w, r, estado, map[int]string{
			http.StatusNotFound:            "no_encontrado",
			http.StatusForbidden:           "sin_permiso",
			http.StatusTooManyRequests:     "demasiadas",
			http.StatusInternalServerError: "error_interno",
		}[codigo])
		return
	}
	prefijo := "http." + strconv.Itoa(codigo)
	p := a.pagina(r, prefijo+"_titulo")
	p.Datos = struct {
		Codigo        int
		Titulo, Texto string
	}{codigo, prefijo + "_titulo", prefijo + "_texto"}
	a.pintar(w, r, estado, "error.html", p)
}
