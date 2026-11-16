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
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Páginas del postulante, segunda parte (paso 11b): documentos, revisión y
// envío, estado de la postulación enviada y subsanación.
//
// Como en la primera parte, no hay reglas aquí: la subida y el envío son los
// mismos de la API (subirDocumento, enviarPostulacion); qué puede subirse o
// modificarse lo dice models.PermisoDeEdicion; qué se pide, el catálogo de
// requisitos de la convocatoria según la dotación de cada obra.

// --- Modelos de vista ----------------------------------------------------------

// vistaDocumento es un documento entregado.
type vistaDocumento struct {
	ID      int64
	Nombre  string // el de descarga: folio, requisito, obra y versión
	Tamano  string
	URL     string
	Version int
	Subido  time.Time
	Quitar  bool   // puede retirarse
	Tipo    string // pdf, audio o imagen: decide el visor
}

// vistaRequisito es una ranura de documento: lo que se pide y lo entregado.
type vistaRequisito struct {
	Ancla       string
	Clave       string
	ObraID      int64
	Etiqueta    string
	Ayuda       string
	Obligatorio bool
	Formatos    string // «PDF», «JPG o PNG»
	Accept      string // atributo accept del control de archivo
	TamanoMax   string
	MaxArchivos int
	Documentos  []vistaDocumento
	Editable    bool
	PuedeSubir  bool  // hay lugar para otro archivo, o el nuevo reemplaza al único
	Reemplaza   int64 // documento al que sustituye la subida, si sólo se admite uno
	Error       string
	Foco        bool // acaba de subirse o de rechazarse: recibe el foco
	Resaltado   bool
	Senalado    string
}

// vistaGrupoDocumentos son los documentos de la postulación o de una obra.
type vistaGrupoDocumentos struct {
	ID         string
	Titulo     string
	Nota       string
	Requisitos []vistaRequisito
}

// vistaProblema es un elemento señalado en una subsanación.
type vistaProblema struct {
	Etiqueta string
	Problema string
	URL      string
}

// vistaSolicitud es una solicitud de desbloqueo del postulante y su respuesta.
type vistaSolicitud struct {
	Etiqueta      string
	Justificacion string
	Estado        string // clave de traducción: pendiente, concedida o negada
	Clase         string
	Motivo        string
}

// vistaSubsanacion es la subsanación que el postulante debe atender.
type vistaSubsanacion struct {
	// Mensaje es el texto del correo que se le envió, con la nota del
	// revisor; ya convertido de Markdown a HTML seguro.
	Mensaje     template.HTML
	FechaLimite time.Time
	Problemas   []vistaProblema
	Solicitudes []vistaSolicitud
}

// vistaAcuse es un acuse de recibo descargable.
type vistaAcuse struct {
	Version int
	Fecha   time.Time
	URL     string
}

// vistaPasoResumido es un paso de la postulación, en sólo lectura, para revisarla
// completa antes de enviarla (RF-POST-09).
type vistaPasoResumido struct {
	Seccion     string
	Titulo      string
	URL         string
	Grupos      []vistaGrupo
	Integrantes []vistaIntegrante
	Videos      []vistaVideo
	Obras       []vistaObra
	Documentos  []vistaGrupoDocumentos
	Faltan      int
}

// vistaEnvio es el paso de revisión y envío y, una vez enviada, el estado de
// la postulación.
type vistaEnvio struct {
	PuedeEnviar   bool // borrador en plazo o subsanación abierta
	Reenvio       bool
	Lista         bool // nada falta
	Faltantes     []vistaFaltante
	Resumen       []vistaPasoResumido
	Acuses        []vistaAcuse
	EnviadaAt     time.Time
	TextoDeEstado string // clave de traducción
	ErrorConfirmo string
	ErrorBases    string

	// Solicitud de desbloqueo durante una subsanación (RF-POST-14).
	Elementos       []vistaOpcion
	ErrorDesbloqueo string
	Justificacion   string
}

// --- Documentos ----------------------------------------------------------------

// anclaDeDocumento es el identificador de HTML de la ranura de un requisito.
func anclaDeDocumento(clave string, obraID int64) string {
	if obraID == 0 {
		return "doc-" + clave
	}
	return fmt.Sprintf("doc-%s-o%d", clave, obraID)
}

// tamanoLegible expresa un tamaño en KB o MB, en la forma en que se dice.
func tamanoLegible(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		mb := float64(bytes) / (1 << 20)
		if mb == float64(int64(mb)) {
			return fmt.Sprintf("%d MB", int64(mb))
		}
		return fmt.Sprintf("%.1f MB", mb)
	case bytes >= 1<<10:
		return fmt.Sprintf("%d KB", (bytes+512)>>10)
	}
	return fmt.Sprintf("%d B", bytes)
}

// requisitoDeVista arma la ranura de un requisito con sus documentos.
func (a *App) requisitoDeVista(idioma string, ab *postulacionAbierta, rq models.RequisitoDocumental, obraID int64, ordenDeObra int) vistaRequisito {
	v := vistaRequisito{
		Ancla: anclaDeDocumento(rq.Clave, obraID), Clave: rq.Clave, ObraID: obraID,
		Etiqueta: rq.EtiquetaES, Ayuda: rq.AyudaES, Obligatorio: rq.Obligatorio,
		TamanoMax: tamanoLegible(rq.TamanoMaxBytes), MaxArchivos: max(rq.MaxArchivos, 1),
		Editable: ab.permiso.Documento(rq.Clave, obraID),
	}
	if idioma == models.IdiomaEN {
		if rq.EtiquetaEN != "" {
			v.Etiqueta = rq.EtiquetaEN
		}
		if rq.AyudaEN != "" {
			v.Ayuda = rq.AyudaEN
		}
	}
	var formatos, accept []string
	for _, mime := range rq.MimesPermitidos {
		extension := models.ExtensionDeMIME(mime)
		formatos = append(formatos, strings.ToUpper(strings.TrimPrefix(extension, ".")))
		accept = append(accept, extension, mime)
	}
	v.Formatos = strings.Join(formatos, a.vistas.T(idioma, "documentos.o"))
	v.Accept = strings.Join(accept, ",")
	identificador := identificadorDe(ab.p)
	for _, d := range ab.p.Documentos {
		if d.Clave != rq.Clave || d.ObraID != obraID {
			continue
		}
		v.Documentos = append(v.Documentos, vistaDocumento{
			ID: d.ID, Nombre: models.NombreDeDescarga(identificador, d, ordenDeObra), Tamano: tamanoLegible(d.Archivo.TamanoBytes),
			URL: fmt.Sprintf("/api/v1/documentos/%d/archivo", d.ID), Version: d.Version, Subido: d.SubidoAt, Quitar: v.Editable,
			Tipo: tipoDeVisor(d.Archivo.MIME),
		})
	}
	// Si se admite un solo archivo, uno nuevo sustituye al que hay: es lo que
	// espera quien vuelve a subir su partitura. Si se admiten varios, sólo hay
	// lugar mientras no estén todos.
	v.PuedeSubir = v.Editable && (v.MaxArchivos == 1 || len(v.Documentos) < v.MaxArchivos)
	if v.MaxArchivos == 1 && len(v.Documentos) == 1 {
		v.Reemplaza = v.Documentos[0].ID
	}
	if v.Editable && !ab.permiso.Total {
		v.Resaltado, v.Senalado = true, ab.senalados[claveDeSenalado(models.ProblemaDocumento, rq.Clave, obraID)]
	}
	return v
}

// documentosDeVista arma el paso de documentos: los de la postulación y, por
// cada obra, los que pide su dotación (RF-DOC-04).
func (a *App) documentosDeVista(idioma string, ab *postulacionAbierta) []vistaGrupoDocumentos {
	porAmbito := func(dotacionID int64, ambito models.AmbitoRequisito) []models.RequisitoDocumental {
		var lista []models.RequisitoDocumental
		for _, rq := range models.RequisitosAplicables(ab.cfg.Requisitos, dotacionID) {
			if rq.Ambito == ambito {
				lista = append(lista, rq)
			}
		}
		return lista
	}
	grupo := vistaGrupoDocumentos{ID: "docs-postulacion", Titulo: a.vistas.T(idioma, "documentos.de_postulacion")}
	for _, rq := range porAmbito(0, models.AmbitoPostulacion) {
		grupo.Requisitos = append(grupo.Requisitos, a.requisitoDeVista(idioma, ab, rq, 0, 0))
	}
	grupos := []vistaGrupoDocumentos{grupo}
	for i, obra := range ab.p.Obras {
		titulo := a.vistas.T(idioma, "postulacion.obra_n", i+1)
		if nombre := obra.Campos["obra.titulo"]; nombre != "" {
			titulo += " · " + nombre
		}
		g := vistaGrupoDocumentos{ID: fmt.Sprintf("docs-obra-%d", obra.ID), Titulo: titulo}
		dotacionID, _ := strconv.ParseInt(obra.Campos["obra.dotacion_id"], 10, 64)
		if ab.cfg.Convocatoria.Tipo == models.TipoCompositores && dotacionID == 0 {
			// Lo que se pide depende de la dotación: sin ella se muestran las
			// reglas generales, y se avisa.
			g.Nota = a.vistas.T(idioma, "documentos.sin_dotacion")
		}
		for _, rq := range porAmbito(dotacionID, models.AmbitoObra) {
			g.Requisitos = append(g.Requisitos, a.requisitoDeVista(idioma, ab, rq, obra.ID, obra.Orden))
		}
		grupos = append(grupos, g)
	}
	return grupos
}

// marcarRequisito pone el foco, y el error si lo hay, en la ranura de una
// subida.
func marcarRequisito(grupos []vistaGrupoDocumentos, clave string, obraID int64, mensaje string) {
	for i := range grupos {
		for j := range grupos[i].Requisitos {
			rq := &grupos[i].Requisitos[j]
			if rq.Clave == clave && rq.ObraID == obraID {
				rq.Foco, rq.Error = true, mensaje
			}
		}
	}
}

// POST /postulaciones/{id}/documentos   — sube un documento
//
// Formulario multipart con clave, obra_id, reemplaza (opcionales los dos
// últimos) y archivo, en ese orden. Es la misma subida de la API.
func (a *App) subirDocumentoDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	cual, err := a.subirDocumento(w, r, ab.p.ID)
	mensaje, aviso := "", (*views.Aviso)(nil)
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
	case errors.Is(err, errSubidaInvalida):
		aviso = &views.Aviso{Tipo: "error", Clave: "documentos.subida_invalida"}
	case errors.As(err, &regla) && len(regla.Errores) > 0:
		mensaje = a.vistas.T(idiomaDe(r), regla.Errores[0].Clave)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
		return
	default:
		a.errorInterno(w, r, "subir un documento", err)
		return
	}
	if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
		return
	}
	v := a.vistaDe(r, ab, models.SeccionDocumentos)
	v.GuardadoAhora = err == nil
	if mensaje != "" && cual.clave == "" {
		aviso, mensaje = &views.Aviso{Tipo: "error", Clave: regla.Errores[0].Clave}, ""
	}
	marcarRequisito(v.Documentos, cual.clave, cual.obraID, mensaje)
	if esHTMX(r) {
		if aviso != nil {
			v.Fallo = a.vistas.T(idiomaDe(r), aviso.Clave)
		}
		a.pintarFragmento(w, r, "f_documentos", v)
		return
	}
	if err != nil {
		a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, aviso)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("%s/%s#%s", v.Base, models.SeccionDocumentos, anclaDeDocumento(cual.clave, cual.obraID)), http.StatusSeeOther)
}

// POST /postulaciones/{id}/documentos/{documento}/quitar   — retira un documento
//
// No lo borra: deja de estar vigente y su archivo se conserva (RF-DOC-07).
func (a *App) quitarDocumentoDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	documentoID, err := strconv.ParseInt(r.PathValue("documento"), 10, 64)
	posicion := slices.IndexFunc(ab.p.Documentos, func(d models.Documento) bool { return d.ID == documentoID })
	if err != nil || posicion < 0 {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	documento := ab.p.Documentos[posicion]
	_, err = a.datos.QuitarDocumento(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, documentoID)
	fallo, respondido := a.rechazo(w, r, "quitar un documento", err)
	if respondido {
		return
	}
	if ab, ok = a.abrirPostulacionPorID(w, r, ab.p.ID); !ok {
		return
	}
	v := a.vistaDe(r, ab, models.SeccionDocumentos)
	v.GuardadoAhora = fallo == ""
	marcarRequisito(v.Documentos, documento.Clave, documento.ObraID, fallo)
	switch {
	case esHTMX(r):
		a.pintarFragmento(w, r, "f_documentos", v)
	case fallo != "":
		a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, nil)
	default:
		http.Redirect(w, r, fmt.Sprintf("%s/%s#%s", v.Base, models.SeccionDocumentos, anclaDeDocumento(documento.Clave, documento.ObraID)), http.StatusSeeOther)
	}
}

// --- Subsanación ---------------------------------------------------------------

// etiquetaDeElemento nombra un elemento señalable, con su obra delante si es
// de una obra: «Obra 2 · Duración en minutos».
func (a *App) etiquetaDeElemento(idioma string, ab *postulacionAbierta, clase models.ClaseProblema, campo string, obraID int64) string {
	etiqueta := a.nombreDeElemento(idioma, ab.cfg)(clase, campo)
	for i, obra := range ab.p.Obras {
		if obra.ID == obraID {
			etiqueta = a.vistas.T(idioma, "postulacion.obra_n", i+1) + " · " + etiqueta
		}
	}
	return etiqueta
}

// urlDeElemento lleva al control de un elemento señalable, en su paso.
func urlDeElemento(base string, clase models.ClaseProblema, campo string, obraID int64) string {
	ancla := idDeCampo(campo, obraID)
	switch {
	case clase == models.ProblemaDocumento:
		ancla = anclaDeDocumento(campo, obraID)
	case campo == models.CampoIntegrantes, campo == models.CampoVideos, campo == models.CampoObras:
		ancla = campo
	}
	return base + "/" + models.SeccionDeElemento(clase, campo) + "#" + ancla
}

// subsanacionDeVista arma lo que el postulante debe atender: el mensaje del
// revisor, el plazo, cada elemento señalado con su motivo y un enlace a él, y
// sus solicitudes de desbloqueo con su respuesta.
func (a *App) subsanacionDeVista(idioma string, ab *postulacionAbierta) *vistaSubsanacion {
	s := ab.subsanacion
	base := fmt.Sprintf("/postulaciones/%d", ab.p.ID)
	v := &vistaSubsanacion{Mensaje: views.Markdown(s.Mensaje), FechaLimite: s.FechaLimite}
	for _, problema := range s.Problemas {
		v.Problemas = append(v.Problemas, vistaProblema{
			Etiqueta: a.etiquetaDeElemento(idioma, ab, problema.Clase, problema.Campo, problema.ObraID),
			Problema: problema.Problema, URL: urlDeElemento(base, problema.Clase, problema.Campo, problema.ObraID),
		})
	}
	for _, solicitud := range s.Solicitudes {
		estado, clase := "subsanacion.solicitud_pendiente", "proxima"
		if solicitud.Resuelta && solicitud.Concedida {
			estado, clase = "subsanacion.solicitud_concedida", "abierta"
		} else if solicitud.Resuelta {
			estado, clase = "subsanacion.solicitud_negada", "cerrada"
		}
		v.Solicitudes = append(v.Solicitudes, vistaSolicitud{
			Etiqueta:      a.etiquetaDeElemento(idioma, ab, solicitud.Clase, solicitud.Campo, solicitud.ObraID),
			Justificacion: solicitud.Justificacion, Estado: estado, Clase: clase, Motivo: solicitud.MotivoRespuesta,
		})
	}
	return v
}

// elementoDeDesbloqueo codifica un elemento en el valor de una opción:
// «clase|campo|obra».
func elementoDeDesbloqueo(clase models.ClaseProblema, campo string, obraID int64) string {
	return fmt.Sprintf("%s|%s|%d", clase, campo, obraID)
}

// elementosParaDesbloqueo lista lo que el postulante puede pedir que se le
// desbloquee: todo campo, lista o documento de su postulación que hoy no
// puede modificar. Los campos extendidos no se desbloquean (RF-CLI-26).
func (a *App) elementosParaDesbloqueo(idioma string, ab *postulacionAbierta) []vistaOpcion {
	var opciones []vistaOpcion
	agregar := func(clase models.ClaseProblema, campo string, obraID int64, prefijo string) {
		desbloqueado := ab.permiso.Campo(campo, obraID)
		if clase == models.ProblemaDocumento {
			desbloqueado = ab.permiso.Documento(campo, obraID)
		}
		if !desbloqueado {
			opciones = append(opciones, vistaOpcion{Valor: elementoDeDesbloqueo(clase, campo, obraID),
				Texto: prefijo + a.etiquetaDeElemento(idioma, ab, clase, campo, obraID)})
		}
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

// POST /postulaciones/{id}/desbloqueos   — pide corregir algo que el revisor no señaló
//
// Campos: elemento («clase|campo|obra») y justificacion. Lo resuelve el
// revisor (RF-POST-14).
func (a *App) solicitarDesbloqueoDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	partes := strings.SplitN(r.PostFormValue("elemento"), "|", 3)
	solicitud := models.SolicitudDesbloqueo{Justificacion: textoDeFormulario(r.PostFormValue("justificacion"))}
	if len(partes) == 3 {
		solicitud.Clase, solicitud.Campo = models.ClaseProblema(partes[0]), partes[1]
		solicitud.ObraID, _ = strconv.ParseInt(partes[2], 10, 64)
	}
	err := a.datos.SolicitarDesbloqueo(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, ab.p.ID, solicitud)
	fallo, respondido := a.rechazo(w, r, "solicitar un desbloqueo", err)
	if respondido {
		return
	}
	if fallo == "" {
		http.Redirect(w, r, fmt.Sprintf("/postulaciones/%d/%s?aviso=desbloqueo#subsanacion", ab.p.ID, models.SeccionEnvio), http.StatusSeeOther)
		return
	}
	v := a.vistaDe(r, ab, models.SeccionEnvio)
	v.Envio = a.envioDeVista(r, ab)
	v.Envio.ErrorDesbloqueo, v.Envio.Justificacion = fallo, solicitud.Justificacion
	a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, nil)
}

// --- Revisión y envío ----------------------------------------------------------

// textosDeEstado explican al postulante en qué va su postulación una vez
// enviada. El descarte no aparece: al titular se le muestra «en revisión».
var textosDeEstado = map[models.EstadoDePostulacion]string{
	models.PostulacionEnviada:       "envio.estado.enviada",
	models.PostulacionEnRevision:    "envio.estado.en_revision",
	models.PostulacionEnSubsanacion: "envio.estado.en_subsanacion",
	models.PostulacionValidada:      "envio.estado.validada",
	models.PostulacionEnEvaluacion:  "envio.estado.en_evaluacion",
	models.PostulacionEvaluada:      "envio.estado.evaluada",
}

// envioDeVista arma el paso de revisión y envío: la postulación completa en
// sólo lectura, todo lo que falta con su enlace, y el envío; o, ya enviada,
// su estado y sus acuses.
func (a *App) envioDeVista(r *http.Request, ab *postulacionAbierta) *vistaEnvio {
	idioma := idiomaDe(r)
	base := fmt.Sprintf("/postulaciones/%d", ab.p.ID)
	e := &vistaEnvio{
		PuedeEnviar: ab.motivo == "", Reenvio: ab.p.Estado == models.PostulacionEnSubsanacion,
		Lista: len(ab.bloqueadores) == 0, EnviadaAt: ab.p.EnviadaAt,
		TextoDeEstado: textosDeEstado[models.EstadoParaTitular(ab.p.Estado)],
	}
	if ab.p.Estado == models.PostulacionEnSubsanacion && ab.subsanacion == nil {
		e.TextoDeEstado = "envio.estado.subsanacion_vencida"
	}
	etiqueta := a.etiquetas(r)
	for _, b := range ab.bloqueadores {
		mensaje := a.vistas.T(idioma, b.Clave)
		nombre := etiqueta(b.Campo)
		clase := models.ProblemaCampo
		campo := b.Campo
		if b.Requisito != nil {
			nombre, clase, campo = a.nombreDeElemento(idioma, ab.cfg)(models.ProblemaDocumento, b.Requisito.Clave), models.ProblemaDocumento, b.Requisito.Clave
		}
		if strings.Contains(mensaje, "%s") {
			mensaje = fmt.Sprintf(mensaje, nombre)
		}
		if b.ObraID != 0 {
			for i, obra := range ab.p.Obras {
				if obra.ID == b.ObraID {
					mensaje = a.vistas.T(idioma, "postulacion.obra_n", i+1) + ": " + mensaje
				}
			}
		}
		url := base + "/" + b.Seccion
		if b.Seccion != models.SeccionEnvio {
			url = urlDeElemento(base, clase, campo, b.ObraID)
		}
		e.Faltantes = append(e.Faltantes, vistaFaltante{Mensaje: a.vistas.T(idioma, "seccion."+b.Seccion) + " · " + mensaje, Ancla: url})
	}

	// La postulación completa, en sólo lectura: la misma vista de cada paso,
	// sin ningún permiso de edición.
	soloLectura := *ab
	soloLectura.permiso = models.PermisoDeEdicion{}
	pendientes := make(map[string]int)
	for _, b := range ab.bloqueadores {
		pendientes[b.Seccion]++
	}
	for _, seccion := range models.SeccionesDe(ab.cfg.Convocatoria.Tipo) {
		if seccion == models.SeccionEnvio {
			continue
		}
		paso := a.vistaDe(r, &soloLectura, seccion)
		resumen := vistaPasoResumido{Seccion: seccion, Titulo: a.vistas.T(idioma, "seccion."+seccion), URL: base + "/" + seccion,
			Grupos: paso.Grupos, Obras: paso.Obras, Documentos: paso.Documentos, Faltan: pendientes[seccion]}
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
		e.Resumen = append(e.Resumen, resumen)
	}
	for _, acuse := range ab.p.Acuses {
		e.Acuses = append(e.Acuses, vistaAcuse{Version: acuse.Version, Fecha: acuse.GeneradoAt,
			URL: fmt.Sprintf("/api/v1/postulaciones/%d/acuses/%d", ab.p.ID, acuse.Version)})
	}
	if ab.subsanacion != nil {
		e.Elementos = a.elementosParaDesbloqueo(idioma, ab)
	}
	return e
}

// POST /postulaciones/{id}/envio   — envía la postulación (irreversible)
//
// Campos: confirmo y acepto_bases, las dos declaraciones de RF-POST-10. Es el
// mismo envío de la API: congela la postulación, asigna el folio y emite el
// acuse.
func (a *App) enviarDelPaso(w http.ResponseWriter, r *http.Request) {
	ab, ok := a.abrirPostulacion(w, r)
	if !ok {
		return
	}
	reenvio := ab.p.Estado == models.PostulacionEnSubsanacion
	err := a.enviarPostulacion(r, ab.p.ID, models.ConfirmacionEnvio{
		Confirmo: r.PostFormValue("confirmo") != "", AceptoBases: r.PostFormValue("acepto_bases") != "",
	})
	var incompleta *models.ErrorDeEnvio
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
		aviso := "enviada"
		if reenvio {
			aviso = "reenviada"
		}
		http.Redirect(w, r, fmt.Sprintf("/postulaciones/%d/%s?aviso=%s", ab.p.ID, models.SeccionEnvio, aviso), http.StatusSeeOther)
		return
	case errors.As(err, &incompleta):
		// Lo que falta ya está en la página: se vuelve a pintar con el aviso.
		v := a.vistaDe(r, ab, models.SeccionEnvio)
		v.Envio = a.envioDeVista(r, ab)
		a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, &views.Aviso{Tipo: "error", Clave: "envio.incompleta"})
		return
	case errors.As(err, &regla):
		v := a.vistaDe(r, ab, models.SeccionEnvio)
		v.Envio = a.envioDeVista(r, ab)
		var aviso *views.Aviso
		for _, e := range regla.Errores {
			switch e.Campo {
			case "confirmo":
				v.Envio.ErrorConfirmo = a.vistas.T(idiomaDe(r), e.Clave)
			case "acepto_bases":
				v.Envio.ErrorBases = a.vistas.T(idiomaDe(r), e.Clave)
			default:
				aviso = &views.Aviso{Tipo: "error", Clave: e.Clave}
			}
		}
		a.pintarPaso(w, r, http.StatusUnprocessableEntity, v, aviso)
		return
	}
	if _, respondido := a.rechazo(w, r, "enviar la postulación", err); !respondido {
		a.errorInterno(w, r, "enviar la postulación", err)
	}
}
