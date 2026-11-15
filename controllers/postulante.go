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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Páginas del postulante (§5.3): el panel «Mis postulaciones» y el
// formulario por pasos, con guardado automático.
//
// Son la interfaz en HTML de las mismas operaciones que ofrece la API: cada
// manejador llama a la misma operación del repositorio, que es donde se
// decide qué puede modificarse y qué valor es aceptable. Aquí no hay reglas:
// el formulario se pinta a partir de la definición declarativa del modelo
// (models.FormularioDe), y lo que falta para enviar sale de
// models.ValidarParaEnvio, igual que en la API.
//
// Cada formulario funciona de dos maneras con un mismo manejador:
//
//   - con htmx, guarda solo mientras se escribe y recibe de vuelta los
//     fragmentos que cambiaron (la confirmación «Guardado a las…», el avance
//     de cada paso, lo que falta y el error de cada campo);
//   - sin JavaScript, se envía con su botón y la página se vuelve a pintar.
//
// Permisos: cada cuenta sólo sobre SUS postulaciones. Lo que no es suyo
// responde 404, igual que lo que no existe.

// seccionesConPagina son los pasos del formulario que ya tienen pantalla. Los
// de documentos y de revisión y envío llegan con la entrega 11b del plan.
var seccionesConPagina = []string{models.SeccionDatos, models.SeccionConcierto, models.SeccionObras}

// esHTMX indica si la petición la hizo htmx, que espera fragmentos.
func esHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// textoDeFormulario normaliza lo que llega de un control: los navegadores
// envían los saltos de línea como CR LF, y contados así un texto que cabía en
// su límite dejaría de caber.
func textoDeFormulario(valor string) string {
	return strings.TrimSpace(strings.ReplaceAll(valor, "\r\n", "\n"))
}

// idDeCampo convierte la ruta de un campo en un identificador de HTML: sin
// puntos, que en un selector significarían otra cosa. Los campos de una obra
// llevan delante su obra, porque en la página hay varias.
func idDeCampo(ruta string, obraID int64) string {
	id := strings.ReplaceAll(ruta, ".", "-")
	if obraID != 0 {
		id = "o" + strconv.FormatInt(obraID, 10) + "-" + id
	}
	return id
}

// --- Modelos de vista ----------------------------------------------------------

// vistaOpcion es una opción de un control de selección.
type vistaOpcion struct {
	Valor   string
	Texto   string
	Elegida bool
}

// vistaCampo es un campo del formulario listo para pintarse: su control, su
// valor y si quien lo mira puede modificarlo.
type vistaCampo struct {
	Ruta          string // nombre del control: «datos.telefono»
	ID            string // identificador de HTML: «datos-telefono»
	Etiqueta      string
	Ayuda         string
	Control       string // «texto», «area» o «seleccion»
	Tipo          string // atributo type del control de texto
	Numerico      bool   // teclado numérico en móvil
	Autocompletar string
	Valor         string
	Texto         string // el valor tal como se lee: el nombre de la opción elegida
	Obligatorio   bool
	Condicional   bool // obligatorio sólo si se cumple una condición que hoy no se cumple
	Longitud      int
	Opciones      []vistaOpcion
	Editable      bool
	Error         string
}

// vistaGrupo es un bloque de campos con su título.
type vistaGrupo struct {
	ID     string
	Titulo string // clave de traducción
	Nota   string // texto ya traducido
	Campos []vistaCampo
}

// vistaPaso es un paso del formulario en la barra de avance.
type vistaPaso struct {
	Numero     int
	Seccion    string
	Etiqueta   string
	URL        string // vacía si el paso aún no tiene pantalla
	Pendientes int
	Completo   bool
	Actual     bool
}

// vistaFaltante es algo que falta para poder enviar, con el enlace al control
// donde se captura.
type vistaFaltante struct {
	Mensaje string
	Ancla   string
}

// vistaIntegrante es un renglón de la lista de integrantes.
type vistaIntegrante struct {
	Nombre      string
	Instrumento string
	Nuevo       bool // recién añadido: recibe el foco
}

// vistaVideo es una liga a un video con el resultado de su comprobación.
type vistaVideo struct {
	Numero int
	URL    string
	Estado string // texto ya traducido; vacío si no hay liga
	Clase  string
}

// vistaObra es una obra del programa con sus campos.
type vistaObra struct {
	ID       int64
	Numero   int
	Titulo   string
	Campos   []vistaCampo
	Primera  bool
	Ultima   bool
	Editable bool // puede quitarse o moverse
	Foco     bool // acaba de añadirse o de moverse: recibe el foco
}

// vistaErrorDeCampo lleva a htmx el mensaje de error de un campo; vacío lo
// borra.
type vistaErrorDeCampo struct {
	ID      string
	Mensaje string
}

// vistaPostulacion es todo lo que necesita la página de un paso.
type vistaPostulacion struct {
	ID               int64
	Base             string // «/postulaciones/12»
	Folio            string
	Titulo           string // de la convocatoria
	Edicion          string
	EsInterpretes    bool
	Subcategoria     string
	OtraSubcategoria string // a la que puede cambiarse; vacía si no puede
	Estado           string // clave de traducción
	EstadoClase      string
	TieneCierre      bool
	Cierre           time.Time
	Editable         bool   // algo puede modificarse ahora
	Total            bool   // todo puede modificarse: es un borrador en plazo
	Motivo           string // clave de por qué no puede modificarse nada

	Paso      string
	Pasos     []vistaPaso
	Anterior  string // URL del paso anterior; vacía en el primero
	Siguiente string // sección a la que lleva «Guardar y continuar»
	Faltantes []vistaFaltante

	GuardadoAt    time.Time
	GuardadoAhora bool   // acaba de guardarse en esta petición
	Fallo         string // mensaje si el guardado se rechazó entero

	Grupos []vistaGrupo

	TieneIntegrantes    bool
	Integrantes         []vistaIntegrante
	IntegrantesEditable bool
	NotaIntegrantes     string
	TieneVideos         bool
	Videos              []vistaVideo
	VideosEditable      bool
	NotaVideos          string

	Obras            []vistaObra
	ObrasEditable    bool
	PuedeAgregarObra bool
	MaxObras         int
	DuracionTotal    int
	DuracionMaxima   int

	// Sólo en las respuestas a htmx.
	OOB      bool                // los bloques comunes se marcan «fuera de banda»
	Errores  []vistaErrorDeCampo // error de cada campo enviado
	Repintar []vistaCampo        // campos que cambian de forma
	Regrupar []vistaGrupo        // grupos que cambian de forma
}

// vistaResumen es una postulación en el panel.
type vistaResumen struct {
	URL          string
	Titulo       string
	Edicion      string
	Subcategoria string
	Folio        string
	Estado       string
	EstadoClase  string
	Actualizada  time.Time
	Accion       string // clave de traducción del enlace
	Vencida      bool   // borrador de una convocatoria que ya no recibe postulaciones
}

// vistaPanel es la página «Mis postulaciones».
type vistaPanel struct {
	Postulaciones []vistaResumen
	Abiertas      []vistaConvocatoriaAbierta
	PuedePostular bool
}

// vistaConvocatoriaAbierta es una convocatoria en la que la cuenta puede
// iniciar una postulación.
type vistaConvocatoriaAbierta struct {
	ID int64
	vistaConvocatoria
	EsInterpretes bool
}

// --- Carga de la postulación ---------------------------------------------------

// postulacionAbierta es una postulación de quien hace la petición, con lo que
// las páginas necesitan saber de ella.
type postulacionAbierta struct {
	p                models.Postulacion
	cfg              models.ConfiguracionConvocatoria
	formulario       models.DefinicionFormulario
	extendidos       []models.CampoExtendido // de la postulación, los que llena quien postula
	extendidosDeObra []models.CampoExtendido
	permiso          models.PermisoDeEdicion
	motivo           string // clave de por qué no puede modificarse nada; vacía si algo se puede
	bloqueadores     []models.Bloqueador
}

// abrirPostulacion carga la postulación de la ruta ({id}). Si no existe o no
// es de quien la pide, responde 404 y devuelve false.
func (a *App) abrirPostulacion(w http.ResponseWriter, r *http.Request) (*postulacionAbierta, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		a.errorPagina(w, r, http.StatusNotFound)
		return nil, false
	}
	return a.abrirPostulacionPorID(w, r, id)
}

// abrirPostulacionPorID carga una postulación de quien hace la petición.
func (a *App) abrirPostulacionPorID(w http.ResponseWriter, r *http.Request, id int64) (*postulacionAbierta, bool) {
	sesion := sesionDe(r)
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if err == nil && p.UsuarioID != sesion.Usuario.ID {
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
		a.errorInterno(w, r, "leer la convocatoria de la postulación", err)
		return nil, false
	}
	ahora := a.ahora()
	ab := &postulacionAbierta{p: p, cfg: cfg, formulario: models.FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria)}
	// Campos extendidos: del formulario sólo son los que llena quien postula.
	visibles := models.ExtendidosVisibles(&ab.p, cfg.Convocatoria.Tipo, cfg.Extendidos, sesion)
	dePostulacion, deObra := models.ExtendidosDePostulacion(visibles, cfg.Convocatoria.Tipo, p.Subcategoria)
	for _, c := range dePostulacion {
		if c.LoCapturaElPostulante() {
			ab.extendidos = append(ab.extendidos, c)
		}
	}
	for _, c := range deObra {
		if c.LoCapturaElPostulante() {
			ab.extendidosDeObra = append(ab.extendidosDeObra, c)
		}
	}
	var errores []models.ErrorCampo
	if ab.permiso, errores = models.PermisoDeEdicionDe(ab.p, cfg.Convocatoria, ahora); len(errores) > 0 {
		ab.motivo = errores[0].Clave
	}
	ab.bloqueadores = models.ValidarParaEnvio(ab.p, cfg, sesion.Usuario, ahora)
	return ab, true
}

// seccionDeExtendido dice en qué paso se captura un campo extendido de la
// postulación: los de la propuesta de concierto, en el suyo; los demás, con
// los datos generales.
func seccionDeExtendido(c models.CampoExtendido) string {
	if c.Tabla == "propuesta_concierto" {
		return models.SeccionConcierto
	}
	return models.SeccionDatos
}

// --- Del modelo a la vista -----------------------------------------------------

// paises devuelve el catálogo de países con su nombre en el idioma dado, en
// orden alfabético y con México al principio, que es de donde viene la
// mayoría de las postulaciones.
func (a *App) paises(idioma string) []vistaOpcion {
	codigos := models.CodigosDePais()
	opciones := make([]vistaOpcion, 0, len(codigos))
	for _, codigo := range codigos {
		if codigo != models.PaisMexico {
			opciones = append(opciones, vistaOpcion{Valor: codigo, Texto: a.vistas.T(idioma, "pais."+codigo)})
		}
	}
	sort.SliceStable(opciones, func(i, j int) bool {
		return views.ClaveDeOrden(opciones[i].Texto) < views.ClaveDeOrden(opciones[j].Texto)
	})
	mexico := vistaOpcion{Valor: models.PaisMexico, Texto: a.vistas.T(idioma, "pais."+models.PaisMexico)}
	return append([]vistaOpcion{mexico}, opciones...)
}

// entidades devuelve las 32 entidades federativas como opciones.
func entidades() []vistaOpcion {
	catalogo := models.EntidadesFederativas()
	opciones := make([]vistaOpcion, 0, len(catalogo))
	for _, e := range catalogo {
		opciones = append(opciones, vistaOpcion{Valor: e.Clave, Texto: e.Nombre})
	}
	return opciones
}

// campoDeVista convierte la definición de un campo en lo que la plantilla
// pinta. valores son los de la postulación o los de la obra; obraID es 0 para
// los campos de la postulación.
func (a *App) campoDeVista(idioma string, ab *postulacionAbierta, def models.DefinicionCampo, valores map[string]string, obraID int64) vistaCampo {
	c := vistaCampo{
		Ruta: def.Ruta, ID: idDeCampo(def.Ruta, obraID),
		Etiqueta: a.vistas.T(idioma, "campo."+def.Ruta), Ayuda: a.vistas.TextoOpcional(idioma, "ayuda."+def.Ruta),
		Control: "texto", Tipo: "text", Valor: valores[def.Ruta], Longitud: def.Longitud,
		Editable: ab.permiso.Campo(def.Ruta, obraID),
	}
	c.Obligatorio = def.Obligatorio || models.CondicionCumplida(def.Condicion, ab.p.Campos)
	c.Condicional = def.Condicion != "" && !c.Obligatorio
	c.Texto = c.Valor

	switch def.Tipo {
	case models.CampoTextoLargo:
		c.Control = "area"
		maximo := a.vistas.T(idioma, "form.maximo_caracteres", def.Longitud)
		c.Ayuda = strings.TrimSpace(c.Ayuda + " " + maximo)
	case models.CampoEntero, models.CampoAnio:
		c.Numerico = true
	case models.CampoDecimal:
		c.Longitud = 30
	case models.CampoFecha:
		c.Tipo = "date"
	case models.CampoFechaNacimiento:
		c.Tipo, c.Autocompletar = "date", "bday"
	case models.CampoHora:
		c.Tipo = "time"
	case models.CampoCorreo:
		c.Tipo, c.Autocompletar, c.Longitud = "email", "email", 254
	case models.CampoURL:
		c.Tipo, c.Longitud = "url", 2000
	case models.CampoOpcion:
		c.Control = "seleccion"
		for _, valor := range def.Opciones {
			c.Opciones = append(c.Opciones, vistaOpcion{Valor: valor, Texto: a.vistas.T(idioma, "opcion."+valor)})
		}
	case models.CampoPais:
		c.Control, c.Opciones = "seleccion", a.paises(idioma)
	case models.CampoEntidad:
		c.Control, c.Opciones = "seleccion", entidades()
	case models.CampoEntidadResidencia:
		// En México se elige del catálogo; en otro país se escribe.
		if ab.p.Campos["datos.pais_residencia"] == models.PaisMexico {
			c.Control, c.Opciones = "seleccion", entidades()
		}
	case models.CampoDotacion:
		c.Control = "seleccion"
		for _, d := range ab.cfg.Dotaciones {
			if !d.Activa {
				continue
			}
			nombre := d.NombreES
			if idioma == models.IdiomaEN && d.NombreEN != "" {
				nombre = d.NombreEN
			}
			c.Opciones = append(c.Opciones, vistaOpcion{Valor: strconv.FormatInt(d.ID, 10), Texto: nombre})
		}
	}
	if def.Ruta == "datos.telefono" {
		c.Tipo, c.Autocompletar = "tel", "tel"
	}
	for i := range c.Opciones {
		if c.Opciones[i].Valor == c.Valor {
			c.Opciones[i].Elegida, c.Texto = true, c.Opciones[i].Texto
		}
	}
	return c
}

// campoExtendidoDeVista hace lo mismo con un campo extendido, cuya etiqueta y
// ayuda no están en las traducciones sino en el diccionario.
func (a *App) campoExtendidoDeVista(idioma string, ab *postulacionAbierta, ext models.CampoExtendido, valores map[string]string, obraID int64) vistaCampo {
	c := a.campoDeVista(idioma, ab, ext.Definicion(), valores, obraID)
	c.Etiqueta, c.Ayuda = ext.Etiqueta(idioma), ext.AyudaES
	if idioma == models.IdiomaEN && ext.AyudaEN != "" {
		c.Ayuda = ext.AyudaEN
	}
	if ext.Tipo == models.ExtendidoTexto {
		c.Control, c.Longitud = "texto", 0
	}
	return c
}

// conErrores devuelve los campos con lo que se escribió y el motivo del
// rechazo en los que no se guardaron: lo escrito no se pierde al repintar.
func conErrores(campos []vistaCampo, escritos, errores map[string]string) []vistaCampo {
	for i := range campos {
		if mensaje, hay := errores[campos[i].Ruta]; hay {
			campos[i].Error, campos[i].Valor = mensaje, escritos[campos[i].Ruta]
		}
	}
	return campos
}

// gruposDe arma los campos de un paso de la postulación (no de las obras),
// agrupados como se muestran.
func (a *App) gruposDe(idioma string, ab *postulacionAbierta, seccion string) []vistaGrupo {
	var generales, sede []vistaCampo
	for _, def := range ab.formulario.Campos {
		if !strings.HasPrefix(def.Ruta, seccion+".") {
			continue
		}
		campo := a.campoDeVista(idioma, ab, def, ab.p.Campos, 0)
		if def.Condicion == models.CondicionFueraDeCDMX {
			sede = append(sede, campo)
		} else {
			generales = append(generales, campo)
		}
	}
	for _, ext := range ab.extendidos {
		if seccionDeExtendido(ext) == seccion {
			generales = append(generales, a.campoExtendidoDeVista(idioma, ab, ext, ab.p.Campos, 0))
		}
	}
	grupos := []vistaGrupo{{ID: "g-" + seccion, Titulo: "seccion." + seccion, Campos: generales}}
	if len(sede) > 0 {
		grupos = append(grupos, a.grupoDeSede(idioma, ab, sede))
	}
	return grupos
}

// grupoDeSede arma los cuatro datos de la sede del concierto, con la nota que
// dice si hoy son obligatorios: lo son fuera de la Ciudad de México (RN-06).
// La regla no se decide aquí: se le pregunta al modelo.
func (a *App) grupoDeSede(idioma string, ab *postulacionAbierta, campos []vistaCampo) vistaGrupo {
	nota := "postulacion.sede_sin_entidad"
	switch {
	case models.CondicionCumplida(models.CondicionFueraDeCDMX, ab.p.Campos):
		nota = "postulacion.sede_obligatoria"
	case ab.p.Campos["concierto.entidad"] != "":
		nota = "postulacion.sede_no_necesaria"
	}
	return vistaGrupo{ID: "g-sede", Titulo: "postulacion.sede", Nota: a.vistas.T(idioma, nota), Campos: campos}
}

// obraDeVista arma una obra con sus campos.
func (a *App) obraDeVista(idioma string, ab *postulacionAbierta, indice int) vistaObra {
	obra := ab.p.Obras[indice]
	v := vistaObra{ID: obra.ID, Numero: indice + 1, Titulo: obra.Campos["obra.titulo"],
		Primera: indice == 0, Ultima: indice == len(ab.p.Obras)-1, Editable: ab.permiso.Campo(models.CampoObras, 0)}
	for _, def := range ab.formulario.CamposObra {
		v.Campos = append(v.Campos, a.campoDeVista(idioma, ab, def, obra.Campos, obra.ID))
	}
	for _, ext := range ab.extendidosDeObra {
		v.Campos = append(v.Campos, a.campoExtendidoDeVista(idioma, ab, ext, obra.Campos, obra.ID))
	}
	return v
}

// motivosDeBloqueo traduce el motivo por el que el modelo no deja modificar
// una postulación al texto que se muestra arriba de la página.
var motivosDeBloqueo = map[string]string{
	"error.postulacion_enviada": "postulacion.motivo.enviada",
	"error.plazo_vencido":       "postulacion.motivo.plazo",
	"error.fuera_de_etapa":      "postulacion.motivo.plazo",
	"error.subsanacion_vencida": "postulacion.motivo.subsanacion",
}

// clasesDeEstado asigna a cada estado de una postulación el estilo de su
// etiqueta.
var clasesDeEstado = map[models.EstadoDePostulacion]string{
	models.PostulacionBorrador:      "proxima",
	models.PostulacionEnSubsanacion: "proxima",
	models.PostulacionEnviada:       "proceso",
	models.PostulacionEnRevision:    "proceso",
	models.PostulacionValidada:      "abierta",
	models.PostulacionEnEvaluacion:  "abierta",
	models.PostulacionEvaluada:      "abierta",
}

// vistaDe arma el modelo de vista de un paso. Sirve igual para la página
// completa que para los fragmentos que recibe htmx.
func (a *App) vistaDe(r *http.Request, ab *postulacionAbierta, seccion string) *vistaPostulacion {
	idioma := idiomaDe(r)
	c := ab.cfg.Convocatoria
	estado := models.EstadoParaTitular(ab.p.Estado)
	v := &vistaPostulacion{
		ID: ab.p.ID, Base: fmt.Sprintf("/postulaciones/%d", ab.p.ID), Folio: ab.p.Folio,
		Titulo: c.Titulo(idioma), Edicion: c.Edicion.Nombre(idioma),
		EsInterpretes: c.Tipo == models.TipoInterpretes, Subcategoria: string(ab.p.Subcategoria),
		Estado: "estado.postulacion." + string(estado), EstadoClase: clasesDeEstado[estado],
		Editable: ab.motivo == "", Total: ab.permiso.Total, Motivo: motivosDeBloqueo[ab.motivo],
		Paso: seccion, GuardadoAt: ab.p.ActualizadoAt,
		MaxObras: c.MaxObras, DuracionMaxima: c.DuracionMaxProgramaMin,
	}
	if etapa, ok := c.Etapa(models.EtapaPostulacion); ok {
		v.TieneCierre, v.Cierre = true, etapa.TerminaAt
	}
	// Cambiar de subcategoría sólo tiene sentido en un borrador de Intérpretes.
	if v.Total && v.EsInterpretes {
		v.OtraSubcategoria = string(models.SubcategoriaEnsamble)
		if ab.p.Subcategoria == models.SubcategoriaEnsamble {
			v.OtraSubcategoria = string(models.SubcategoriaSolista)
		}
	}

	// Barra de avance: lo que le falta a cada paso sale de la misma
	// comprobación que decide si la postulación puede enviarse.
	pendientes := make(map[string]int)
	for _, b := range ab.bloqueadores {
		pendientes[b.Seccion]++
	}
	secciones := models.SeccionesDe(c.Tipo)
	for i, s := range secciones {
		paso := vistaPaso{Numero: i + 1, Seccion: s, Etiqueta: a.vistas.T(idioma, "seccion."+s),
			Pendientes: pendientes[s], Completo: pendientes[s] == 0, Actual: s == seccion}
		if slices.Contains(seccionesConPagina, s) {
			paso.URL = v.Base + "/" + s
		}
		v.Pasos = append(v.Pasos, paso)
		if s == seccion {
			if i > 0 {
				v.Anterior = v.Pasos[i-1].URL
			}
			if i+1 < len(secciones) && slices.Contains(seccionesConPagina, secciones[i+1]) {
				v.Siguiente = secciones[i+1]
			}
		}
	}
	etiqueta := a.etiquetas(r)
	for _, b := range ab.bloqueadores {
		if b.Seccion != seccion {
			continue
		}
		mensaje := a.vistas.T(idioma, b.Clave)
		if strings.Contains(mensaje, "%s") {
			mensaje = fmt.Sprintf(mensaje, etiqueta(b.Campo))
		}
		if b.ObraID != 0 {
			for i, obra := range ab.p.Obras {
				if obra.ID == b.ObraID {
					mensaje = a.vistas.T(idioma, "postulacion.obra_n", i+1) + ": " + mensaje
				}
			}
		}
		ancla := idDeCampo(b.Campo, b.ObraID)
		if b.Campo == models.CampoObras {
			ancla = "obras"
		}
		v.Faltantes = append(v.Faltantes, vistaFaltante{Mensaje: mensaje, Ancla: ancla})
	}

	switch seccion {
	case models.SeccionDatos:
		v.Grupos = a.gruposDe(idioma, ab, seccion)
		if v.TieneIntegrantes = ab.formulario.TieneIntegrantes; v.TieneIntegrantes {
			v.IntegrantesEditable = ab.permiso.Campo(models.CampoIntegrantes, 0)
			for _, integrante := range ab.p.Integrantes {
				v.Integrantes = append(v.Integrantes, vistaIntegrante{Nombre: integrante.Nombre, Instrumento: integrante.Instrumento})
			}
			if len(v.Integrantes) == 0 && v.IntegrantesEditable {
				v.Integrantes = []vistaIntegrante{{}} // un renglón en blanco para empezar
			}
		}
		if v.TieneVideos = ab.formulario.TieneVideos; v.TieneVideos {
			v.VideosEditable = ab.permiso.Campo(models.CampoVideos, 0)
			v.Videos = a.videosDeVista(idioma, ab, nil)
		}
	case models.SeccionConcierto:
		v.Grupos = a.gruposDe(idioma, ab, seccion)
	case models.SeccionObras:
		v.ObrasEditable = ab.permiso.Campo(models.CampoObras, 0)
		v.PuedeAgregarObra = v.ObrasEditable && len(models.ValidarNuevaObra(c, len(ab.p.Obras))) == 0
		for i := range ab.p.Obras {
			v.Obras = append(v.Obras, a.obraDeVista(idioma, ab, i))
			minutos, _ := strconv.Atoi(ab.p.Obras[i].Campos["obra.duracion_min"])
			v.DuracionTotal += minutos
		}
	}
	return v
}

// videosDeVista arma los renglones de las ligas de video: siempre tantos como
// admite la convocatoria, llenos o vacíos. Con escritas se muestran esas en
// lugar de las guardadas (cuando el guardado se rechazó).
func (a *App) videosDeVista(idioma string, ab *postulacionAbierta, escritas []string) []vistaVideo {
	videos := make([]vistaVideo, ab.cfg.Convocatoria.MaxVideos)
	for i := range videos {
		videos[i].Numero = i + 1
		if i < len(ab.p.Videos) {
			guardado := ab.p.Videos[i]
			videos[i].URL = guardado.URL
			videos[i].Estado, videos[i].Clase = a.vistas.T(idioma, "enlace.estado."+string(guardado.Estado)), string(guardado.Estado)
			if guardado.Detalle != "" {
				videos[i].Estado += " " + a.vistas.T(idioma, "enlace.detalle."+guardado.Detalle)
			}
		}
		if escritas != nil {
			videos[i].URL, videos[i].Estado, videos[i].Clase = "", "", ""
			if i < len(escritas) {
				videos[i].URL = escritas[i]
			}
		}
	}
	return videos
}

// enfocarObra marca la obra que debe recibir el foco al repintarse la lista:
// la que se acaba de añadir o de mover. Quien navega con teclado no pierde
// su lugar.
func (v *vistaPostulacion) enfocarObra(obraID int64) {
	for i := range v.Obras {
		v.Obras[i].Foco = v.Obras[i].ID == obraID
	}
}

// --- Respuestas ----------------------------------------------------------------

// pintarPaso envía la página completa de un paso.
func (a *App) pintarPaso(w http.ResponseWriter, r *http.Request, estado int, v *vistaPostulacion, aviso *views.Aviso) {
	p := a.pagina(r, "postulacion.titulo")
	p.Titulo = a.vistas.T(p.Idioma, "seccion."+v.Paso) + " · " + v.Titulo
	p.Datos, p.Aviso = v, aviso
	a.pintar(w, r, estado, "postulacion.html", p)
}

// pintarFragmento envía a htmx un bloque de la página de postulación.
func (a *App) pintarFragmento(w http.ResponseWriter, r *http.Request, bloque string, v *vistaPostulacion) {
	v.OOB = true
	p := a.pagina(r, "postulacion.titulo")
	p.Datos = v
	w.Header().Set("Cache-Control", "no-store")
	if err := a.vistas.RenderFragmento(w, http.StatusOK, "postulacion.html", bloque, p); err != nil {
		a.log.Error("no se pudo pintar el fragmento", "bloque", bloque, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// mensajesDe traduce los errores de una regla, por campo.
func (a *App) mensajesDe(r *http.Request, errores []models.ErrorCampo) map[string]string {
	mensajes := make(map[string]string, len(errores))
	for _, e := range errores {
		mensajes[e.Campo] = a.vistas.T(idiomaDe(r), e.Clave)
	}
	return mensajes
}

// rechazo interpreta el error de una operación del repositorio en una página.
// Si es una regla que no se cumplió devuelve su primer mensaje, para
// mostrarlo; lo demás lo responde aquí (404 o 500) y devuelve respondido.
func (a *App) rechazo(w http.ResponseWriter, r *http.Request, operacion string, err error) (mensaje string, respondido bool) {
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
		return "", false
	case errors.As(err, &regla) && len(regla.Errores) > 0:
		return a.vistas.T(idiomaDe(r), regla.Errores[0].Clave), false
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	default:
		a.errorInterno(w, r, operacion, err)
	}
	return "", true
}

// --- Panel «Mis postulaciones» -------------------------------------------------

// GET /postulaciones   — las postulaciones de la cuenta y las convocatorias abiertas
func (a *App) misPostulaciones(w http.ResponseWriter, r *http.Request) {
	a.pintarPanel(w, r, http.StatusOK, nil)
}

// pintarPanel muestra el panel: el historial de la cuenta, de todas las
// ediciones (RF-POST-12), y las convocatorias en las que puede postular hoy
// (RF-POST-01).
func (a *App) pintarPanel(w http.ResponseWriter, r *http.Request, estado int, aviso *views.Aviso) {
	sesion := sesionDe(r)
	resumenes, err := a.datos.PostulacionesDeUsuario(r.Context(), sesion.Usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "leer las postulaciones de la cuenta", err)
		return
	}
	publicas, err := a.datos.ConvocatoriasPublicas(r.Context())
	if err != nil {
		a.errorInterno(w, r, "leer las convocatorias", err)
		return
	}
	idioma, ahora := idiomaDe(r), a.ahora()
	enPlazo := make(map[int64]bool)
	for _, c := range publicas {
		enPlazo[c.ID] = len(models.EnPlazoDePostulacion(c, ahora)) == 0
	}
	panel := vistaPanel{PuedePostular: models.PuedePostular(sesion)}
	yaPostulo := make(map[int64]bool)
	for _, rp := range resumenes {
		yaPostulo[rp.Convocatoria.ID] = true
		// Es la lista del propio postulante: el descarte no se le muestra.
		estadoVisible := models.EstadoParaTitular(rp.Estado)
		resumen := vistaResumen{
			URL: fmt.Sprintf("/postulaciones/%d", rp.ID), Titulo: rp.Convocatoria.Titulo(idioma), Edicion: rp.Convocatoria.Edicion.Nombre(idioma),
			Subcategoria: string(rp.Subcategoria), Folio: rp.Folio,
			Estado: "estado.postulacion." + string(estadoVisible), EstadoClase: clasesDeEstado[estadoVisible],
			Actualizada: rp.ActualizadoAt, Accion: "panel.ver",
		}
		switch {
		case rp.Estado == models.PostulacionBorrador && enPlazo[rp.Convocatoria.ID]:
			resumen.Accion = "panel.continuar"
		case rp.Estado == models.PostulacionBorrador:
			resumen.Vencida, resumen.EstadoClase = true, "cerrada"
		case rp.Estado == models.PostulacionEnSubsanacion:
			resumen.Accion = "panel.subsanar"
		}
		panel.Postulaciones = append(panel.Postulaciones, resumen)
	}
	for _, c := range publicas {
		if enPlazo[c.ID] && !yaPostulo[c.ID] {
			panel.Abiertas = append(panel.Abiertas, vistaConvocatoriaAbierta{ID: c.ID, vistaConvocatoria: aVista(c, idioma, false),
				EsInterpretes: c.Tipo == models.TipoInterpretes})
		}
	}
	p := a.pagina(r, "panel.titulo")
	p.Datos, p.Aviso = panel, aviso
	a.pintar(w, r, estado, "postulaciones.html", p)
}

// POST /postulaciones   — abre un borrador en una convocatoria
//
// Campos: convocatoria (su id) y, en Intérpretes, subcategoria.
func (a *App) iniciarPostulacion(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	convocatoriaID, err := strconv.ParseInt(r.PostFormValue("convocatoria"), 10, 64)
	if err != nil || !models.PuedePostular(sesion) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := a.datos.CrearPostulacion(r.Context(), a.actorDe(r), convocatoriaID, sesion.Usuario.ID,
		models.Subcategoria(r.PostFormValue("subcategoria")))
	var regla *models.ErrorValidacion
	switch {
	case errors.Is(err, models.ErrDuplicado):
		// Ya tiene una en esa convocatoria: se le lleva al panel, donde está.
		http.Redirect(w, r, "/postulaciones", http.StatusSeeOther)
		return
	case errors.As(err, &regla) && len(regla.Errores) > 0:
		a.pintarPanel(w, r, http.StatusUnprocessableEntity, &views.Aviso{Tipo: "error", Clave: regla.Errores[0].Clave})
		return
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
		return
	case err != nil:
		a.errorInterno(w, r, "crear la postulación", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/postulaciones/%d/%s", id, models.SeccionDatos), http.StatusSeeOther)
}

// GET /postulaciones/{ref}   — abre una postulación por su id o por su folio
//
// El enlace por folio es permanente (RNF-17). Lleva al primer paso; la
// pantalla de estado de una postulación ya enviada llega con la entrega 11b.
func (a *App) abrirPorReferencia(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		if id, err = a.datos.PostulacionPorFolio(r.Context(), ref); errors.Is(err, models.ErrNoEncontrado) {
			a.errorPagina(w, r, http.StatusNotFound)
			return
		}
		if err != nil {
			a.errorInterno(w, r, "buscar la postulación por folio", err)
			return
		}
	}
	if _, ok := a.abrirPostulacionPorID(w, r, id); ok {
		http.Redirect(w, r, fmt.Sprintf("/postulaciones/%d/%s", id, models.SeccionDatos), http.StatusSeeOther)
	}
}

// --- Pasos del formulario ------------------------------------------------------

// avisosDePaso son los mensajes que una redirección puede pedir que se
// muestren en el paso al que lleva (?aviso=…).
var avisosDePaso = map[string]views.Aviso{
	"subcategoria": {Tipo: "exito", Clave: "postulacion.subcategoria_cambiada"},
}

// GET /postulaciones/{id}/{seccion}   — un paso del formulario
func (a *App) pasoDePostulacion(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	seccion := r.PathValue("seccion")
	if !slices.Contains(models.SeccionesDe(ab.cfg.Convocatoria.Tipo), seccion) || !slices.Contains(seccionesConPagina, seccion) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	v := a.vistaDe(r, ab, seccion)
	// Tras «Añadir integrante» sin JavaScript se vuelve aquí con un renglón más.
	if r.URL.Query().Get("integrante") == "nuevo" && v.IntegrantesEditable {
		v.Integrantes = append(v.Integrantes, vistaIntegrante{Nuevo: true})
	}
	var aviso *views.Aviso
	if pedido, hay := avisosDePaso[r.URL.Query().Get("aviso")]; hay {
		aviso = &pedido
	}
	a.pintarPaso(w, r, http.StatusOK, v, aviso)
}

// destinoTras devuelve a dónde ir después de guardar un paso con su botón:
// al paso siguiente, al anterior o, si ya no hay más pantallas, al panel.
func destinoTras(v *vistaPostulacion, ir string) string {
	switch {
	case ir == "anterior" && v.Anterior != "":
		return v.Anterior
	case ir == "siguiente" && v.Siguiente != "":
		return v.Base + "/" + v.Siguiente
	case ir == "siguiente":
		return "/postulaciones"
	}
	return v.Base + "/" + v.Paso
}

// definicionesDe devuelve las definiciones de los campos de un paso de la
// postulación, nativos y extendidos: lo que el formulario de ese paso puede
// enviar.
func (ab *postulacionAbierta) definicionesDe(seccion string) []models.DefinicionCampo {
	var definiciones []models.DefinicionCampo
	for _, def := range ab.formulario.Campos {
		if strings.HasPrefix(def.Ruta, seccion+".") {
			definiciones = append(definiciones, def)
		}
	}
	for _, ext := range ab.extendidos {
		if seccionDeExtendido(ext) == seccion {
			definiciones = append(definiciones, ext.Definicion())
		}
	}
	return definiciones
}

// cambiosDe compara lo que llegó del formulario con lo guardado y devuelve
// lo escrito y, aparte, sólo lo que cambió. El formulario llega entero en
// cada guardado; escribir sólo lo que cambió evita, entre otras cosas, que
// una liga que nadie tocó vuelva a comprobarse en red.
func cambiosDe(r *http.Request, definiciones []models.DefinicionCampo, guardados map[string]string) (escritos, cambios map[string]string) {
	escritos, cambios = make(map[string]string), make(map[string]string)
	for _, def := range definiciones {
		valores, viene := r.PostForm[def.Ruta]
		if !viene || len(valores) == 0 {
			continue
		}
		valor := textoDeFormulario(valores[0])
		escritos[def.Ruta] = valor
		if valor != guardados[def.Ruta] {
			cambios[def.Ruta] = valor
		}
	}
	return escritos, cambios
}

// POST /postulaciones/{id}/campos   — guarda los campos de un paso
//
// Campos: seccion (el paso), los campos por su ruta («datos.telefono») e ir
// («siguiente», «anterior» o nada) cuando se envía con un botón.
func (a *App) guardarCamposDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	seccion := r.PostFormValue("seccion")
	if seccion != models.SeccionDatos && seccion != models.SeccionConcierto {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	definiciones := ab.definicionesDe(seccion)
	escritos, cambios := cambiosDe(r, definiciones, ab.p.Campos)

	var resultado models.ResultadoGuardado
	fallo := ""
	if len(cambios) > 0 {
		var err error
		resultado, err = a.datos.GuardarCamposDePostulacion(r.Context(), sesionDe(r).Usuario.ID, ab.p.ID, cambios)
		var respondido bool
		if fallo, respondido = a.rechazo(w, r, "guardar la postulación", err); respondido {
			return
		}
		// Se vuelve a leer: la vista sale de lo que quedó guardado.
		if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
			return
		}
	}
	errores := a.mensajesDe(r, resultado.Errores)
	v := a.vistaDe(r, ab, seccion)
	v.Fallo, v.GuardadoAhora = fallo, fallo == "" && len(resultado.Guardados) > 0
	for i := range v.Grupos {
		v.Grupos[i].Campos = conErrores(v.Grupos[i].Campos, escritos, errores)
	}

	if !esHTMX(r) {
		if fallo != "" || len(errores) > 0 {
			var aviso *views.Aviso
			if fallo == "" {
				aviso = &views.Aviso{Tipo: "error", Clave: "postulacion.hay_errores"}
			}
			a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, aviso)
			return
		}
		http.Redirect(w, r, destinoTras(v, r.PostFormValue("ir")), http.StatusSeeOther)
		return
	}

	// A htmx: el error (o su ausencia) de cada campo enviado y, si cambió
	// algo de lo que depende la forma de otro campo, ese campo de nuevo. Si
	// algo se rechazó, el aviso de guardado no dice «guardado»: lo dice.
	if fallo == "" && len(errores) > 0 {
		v.Fallo = a.vistas.T(idiomaDe(r), "postulacion.hay_errores")
	}
	for _, grupo := range v.Grupos {
		for _, campo := range grupo.Campos {
			if _, enviado := escritos[campo.Ruta]; enviado {
				v.Errores = append(v.Errores, vistaErrorDeCampo{ID: campo.ID, Mensaje: campo.Error})
			}
			if _, cambio := cambios["datos.pais_residencia"]; cambio && campo.Ruta == "datos.entidad_residencia" {
				v.Repintar = append(v.Repintar, campo)
			}
		}
		if _, cambio := cambios["concierto.entidad"]; cambio && grupo.ID == "g-sede" {
			v.Regrupar = append(v.Regrupar, grupo)
		}
	}
	a.pintarFragmento(w, r, "f_guardado", v)
}

// POST /postulaciones/{id}/subcategoria   — de solista a ensamble o al revés
//
// Borra los datos generales ya capturados, porque los formularios son
// distintos: por eso exige marcar la confirmación (RF-POST-02).
func (a *App) cambiarSubcategoriaDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	err := error(&models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "confirmar", Clave: "error.confirmar_cambio"}}})
	if r.PostFormValue("confirmar") != "" {
		err = a.datos.CambiarSubcategoria(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, ab.p.ID, models.Subcategoria(r.PostFormValue("subcategoria")))
	}
	var regla *models.ErrorValidacion
	if errors.As(err, &regla) && len(regla.Errores) > 0 {
		a.pintarPaso(w, r, http.StatusUnprocessableEntity, a.vistaDe(r, ab, models.SeccionDatos), &views.Aviso{Tipo: "error", Clave: regla.Errores[0].Clave})
		return
	}
	if _, respondido := a.rechazo(w, r, "cambiar la subcategoría", err); respondido {
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/postulaciones/%d/%s?aviso=subcategoria", ab.p.ID, models.SeccionDatos), http.StatusSeeOther)
}

// POST /postulaciones/{id}/integrantes   — guarda la lista de integrantes
//
// Campos: nombre e instrumento, repetidos, uno por renglón; y accion:
// «agregar» (un renglón más) o «quitar:N» (sin el renglón N).
//
// Se guardan los renglones completos. Uno a medio escribir —con nombre y
// aún sin instrumento— no se guarda ni hace rechazar a los demás: se avisa.
func (a *App) integrantesDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil || len(r.PostForm["nombre"]) != len(r.PostForm["instrumento"]) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	accion := r.PostFormValue("accion")
	var renglones []vistaIntegrante
	var completos []models.Integrante
	incompletos := 0
	for i, nombre := range r.PostForm["nombre"] {
		if accion == "quitar:"+strconv.Itoa(i) {
			continue
		}
		renglon := vistaIntegrante{Nombre: textoDeFormulario(nombre), Instrumento: textoDeFormulario(r.PostForm["instrumento"][i])}
		switch {
		case renglon.Nombre != "" && renglon.Instrumento != "":
			completos = append(completos, models.Integrante{Nombre: renglon.Nombre, Instrumento: renglon.Instrumento})
		case renglon.Nombre == "" && renglon.Instrumento == "":
			continue // un renglón en blanco no es nada
		default:
			incompletos++
		}
		renglones = append(renglones, renglon)
	}

	fallo := ""
	guardado := false
	if !slices.Equal(completos, ab.p.Integrantes) {
		err := a.datos.DefinirIntegrantes(r.Context(), sesionDe(r).Usuario.ID, ab.p.ID, completos)
		var respondido bool
		if fallo, respondido = a.rechazo(w, r, "guardar los integrantes", err); respondido {
			return
		}
		guardado = fallo == ""
		if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
			return
		}
	}

	if !esHTMX(r) {
		destino := fmt.Sprintf("/postulaciones/%d/%s", ab.p.ID, models.SeccionDatos)
		if accion == "agregar" {
			destino += "?integrante=nuevo"
		}
		if fallo != "" {
			v := a.vistaDe(r, ab, models.SeccionDatos)
			v.Integrantes, v.NotaIntegrantes = renglones, fallo
			a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, nil)
			return
		}
		http.Redirect(w, r, destino+"#integrantes", http.StatusSeeOther)
		return
	}

	v := a.vistaDe(r, ab, models.SeccionDatos)
	v.GuardadoAhora = guardado
	// Se devuelve lo que se escribió, no sólo lo guardado: un renglón a
	// medias sigue en pantalla.
	v.Integrantes = renglones
	switch {
	case fallo != "":
		v.NotaIntegrantes = fallo
	case incompletos > 0:
		v.NotaIntegrantes = a.vistas.T(idiomaDe(r), "postulacion.integrantes_incompletos")
	}
	if accion == "agregar" {
		v.Integrantes = append(v.Integrantes, vistaIntegrante{Nuevo: true})
	}
	if len(v.Integrantes) == 0 {
		v.Integrantes = []vistaIntegrante{{}}
	}
	if accion == "" {
		// Guardado mientras se escribe: los renglones no se tocan.
		a.pintarFragmento(w, r, "f_lista_guardada", v)
		return
	}
	a.pintarFragmento(w, r, "f_integrantes", v)
}

// POST /postulaciones/{id}/videos   — guarda las ligas a videos de ejecución
//
// Campo: liga, repetido. Las que vienen en blanco no cuentan.
func (a *App) videosDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	var escritas, ligas []string
	for _, liga := range r.PostForm["liga"] {
		liga = textoDeFormulario(liga)
		escritas = append(escritas, liga)
		if liga != "" {
			ligas = append(ligas, liga)
		}
	}
	var guardadas []string
	for _, video := range ab.p.Videos {
		guardadas = append(guardadas, video.URL)
	}

	fallo := ""
	guardado := false
	if !slices.Equal(ligas, guardadas) {
		err := a.datos.DefinirVideos(r.Context(), sesionDe(r).Usuario.ID, ab.p.ID, ligas)
		var respondido bool
		if fallo, respondido = a.rechazo(w, r, "guardar las ligas de video", err); respondido {
			return
		}
		guardado = fallo == ""
		if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
			return
		}
	}
	v := a.vistaDe(r, ab, models.SeccionDatos)
	v.GuardadoAhora, v.NotaVideos = guardado, fallo
	if fallo != "" {
		v.Videos = a.videosDeVista(idiomaDe(r), ab, escritas)
	}
	if !esHTMX(r) {
		if fallo != "" {
			a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, nil)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/postulaciones/%d/%s#videos", ab.p.ID, models.SeccionDatos), http.StatusSeeOther)
		return
	}
	a.pintarFragmento(w, r, "f_lista_guardada", v)
}

// --- Obras ---------------------------------------------------------------------

// POST /postulaciones/{id}/obras   — añade una obra en blanco al programa
func (a *App) agregarObraDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	obraID, err := a.datos.AgregarObra(r.Context(), sesionDe(r).Usuario.ID, ab.p.ID)
	fallo, respondido := a.rechazo(w, r, "añadir la obra", err)
	if respondido {
		return
	}
	if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
		return
	}
	v := a.vistaDe(r, ab, models.SeccionObras)
	v.Fallo, v.GuardadoAhora = fallo, fallo == ""
	v.enfocarObra(obraID)
	if !esHTMX(r) {
		if fallo != "" {
			a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, nil)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("%s/%s#obra-%d", v.Base, models.SeccionObras, obraID), http.StatusSeeOther)
		return
	}
	a.pintarFragmento(w, r, "f_obras", v)
}

// POST /postulaciones/{id}/obras/{obra}   — guarda, quita o mueve una obra
//
// Campos: los de la obra por su ruta («obra.titulo») y accion: nada
// (guardar), «quitar», «subir» o «bajar».
func (a *App) obraDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	obraID, err := strconv.ParseInt(r.PathValue("obra"), 10, 64)
	indice := slices.IndexFunc(ab.p.Obras, func(o models.Obra) bool { return o.ID == obraID })
	if err != nil || indice < 0 {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	usuarioID := sesionDe(r).Usuario.ID
	accion := r.PostFormValue("accion")
	destino := fmt.Sprintf("/postulaciones/%d/%s", ab.p.ID, models.SeccionObras)

	if accion != "" {
		// Quitar o mover: cambia la lista, así que se repinta entera.
		switch {
		case accion == "quitar":
			_, err = a.datos.BorrarObra(r.Context(), usuarioID, obraID)
		case accion == "subir" && indice > 0, accion == "bajar" && indice < len(ab.p.Obras)-1:
			otro := indice - 1
			if accion == "bajar" {
				otro = indice + 1
			}
			orden := make([]int64, len(ab.p.Obras))
			for i, obra := range ab.p.Obras {
				orden[i] = obra.ID
			}
			orden[indice], orden[otro] = orden[otro], orden[indice]
			err = a.datos.OrdenarObras(r.Context(), usuarioID, ab.p.ID, orden)
			destino += "#obra-" + strconv.FormatInt(obraID, 10)
		default:
			a.errorPagina(w, r, http.StatusNotFound)
			return
		}
		fallo, respondido := a.rechazo(w, r, "modificar el programa de obras", err)
		if respondido {
			return
		}
		if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
			return
		}
		v := a.vistaDe(r, ab, models.SeccionObras)
		v.Fallo, v.GuardadoAhora = fallo, fallo == ""
		v.enfocarObra(obraID)
		switch {
		case esHTMX(r):
			a.pintarFragmento(w, r, "f_obras", v)
		case fallo != "":
			a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, nil)
		default:
			http.Redirect(w, r, destino, http.StatusSeeOther)
		}
		return
	}

	// Guardar los campos de la obra.
	definiciones := slices.Clone(ab.formulario.CamposObra)
	for _, ext := range ab.extendidosDeObra {
		definiciones = append(definiciones, ext.Definicion())
	}
	escritos, cambios := cambiosDe(r, definiciones, ab.p.Obras[indice].Campos)
	var resultado models.ResultadoGuardado
	fallo := ""
	if len(cambios) > 0 {
		_, resultado, err = a.datos.GuardarCamposDeObra(r.Context(), usuarioID, obraID, cambios)
		var respondido bool
		if fallo, respondido = a.rechazo(w, r, "guardar la obra", err); respondido {
			return
		}
		if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
			return
		}
	}
	errores := a.mensajesDe(r, resultado.Errores)
	v := a.vistaDe(r, ab, models.SeccionObras)
	v.Fallo, v.GuardadoAhora = fallo, fallo == "" && len(resultado.Guardados) > 0
	v.Obras[indice].Campos = conErrores(v.Obras[indice].Campos, escritos, errores)

	if !esHTMX(r) {
		if fallo != "" || len(errores) > 0 {
			var aviso *views.Aviso
			if fallo == "" {
				aviso = &views.Aviso{Tipo: "error", Clave: "postulacion.hay_errores"}
			}
			a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, aviso)
			return
		}
		http.Redirect(w, r, destino+"#obra-"+strconv.FormatInt(obraID, 10), http.StatusSeeOther)
		return
	}
	if fallo == "" && len(errores) > 0 {
		v.Fallo = a.vistas.T(idiomaDe(r), "postulacion.hay_errores")
	}
	for _, campo := range v.Obras[indice].Campos {
		if _, enviado := escritos[campo.Ruta]; enviado {
			v.Errores = append(v.Errores, vistaErrorDeCampo{ID: campo.ID, Mensaje: campo.Error})
		}
	}
	a.pintarFragmento(w, r, "f_guardado", v)
}
