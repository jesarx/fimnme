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
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// API de postulaciones (§5.3): el postulante crea su postulación, la captura
// en varias sesiones con autoguardado y consulta en todo momento qué le falta
// para enviarla.
//
// Permisos: cada cuenta actúa sólo sobre SUS postulaciones; el repositorio lo
// exige en la propia consulta. Las ven además el administrador y el
// observador. Lo que no es suyo responde 404, igual que lo que no existe.

// etiquetas devuelve la función que traduce la ruta de un campo o de una
// sección a su etiqueta, en el idioma de la petición. Las rutas de campo
// viven bajo «campo.»; las de sección («seccion.datos») y las del estado de
// una liga («enlace.estado.ok») ya vienen completas.
func (a *App) etiquetas(r *http.Request) func(string) string {
	idioma := idiomaDe(r)
	return func(ruta string) string {
		if strings.HasPrefix(ruta, "seccion.") || strings.HasPrefix(ruta, "enlace.") {
			return a.vistas.T(idioma, ruta)
		}
		return a.vistas.T(idioma, "campo."+ruta)
	}
}

// bloqueadoresDeEnvio traduce los bloqueadores al idioma de la petición. El
// mensaje nombra el campo o el documento que falta.
func (a *App) bloqueadoresDeEnvio(r *http.Request, bloqueadores []models.Bloqueador) []views.BloqueadorDeEnvio {
	idioma := idiomaDe(r)
	etiqueta := a.etiquetas(r)
	lista := make([]views.BloqueadorDeEnvio, 0, len(bloqueadores))
	for _, b := range bloqueadores {
		nombre := etiqueta(b.Campo)
		if b.Requisito != nil {
			// El nombre de un documento es contenido editorial de la convocatoria.
			nombre = b.Requisito.EtiquetaES
			if idioma == models.IdiomaEN && b.Requisito.EtiquetaEN != "" {
				nombre = b.Requisito.EtiquetaEN
			}
		}
		mensaje := a.vistas.T(idioma, b.Clave)
		if strings.Contains(mensaje, "%s") {
			mensaje = fmt.Sprintf(mensaje, nombre)
		}
		traducido := views.BloqueadorDeEnvio{Seccion: b.Seccion, Campo: b.Campo, Mensaje: mensaje}
		if b.ObraID != 0 {
			obraID := b.ObraID
			traducido.ObraID = &obraID
		}
		lista = append(lista, traducido)
	}
	return lista
}

// responderPostulacion responde con una postulación completa: lo capturado,
// la definición de su formulario y lo que le falta para enviarse. Si quien la
// pide no puede verla, responde 404.
func (a *App) responderPostulacion(w http.ResponseWriter, r *http.Request, estado int, id int64) {
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	sesion := sesionDe(r)
	if !models.PuedeVerPostulacion(sesion, p) {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), p.ConvocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	titular, err := a.datos.UsuarioPorID(r.Context(), p.UsuarioID)
	if a.falloDatos(w, r, err) {
		return
	}
	ahora := a.ahora()
	bloqueadores := models.ValidarParaEnvio(p, cfg, titular.Usuario, ahora)
	// editable: el titular puede modificar algo ahora mismo. En un borrador,
	// todo; durante una subsanación, sólo lo que lista «subsanacion».
	esTitular := sesion.Usuario.ID == p.UsuarioID
	editable := esTitular && len(models.ValidarEdicionDePostulacion(p, cfg.Convocatoria, ahora)) == 0
	cuerpo := views.PostulacionJSON(p, cfg, editable, a.bloqueadoresDeEnvio(r, bloqueadores), a.etiquetas(r))
	nombrar := a.nombreDeElemento(idiomaDe(r), cfg)
	cuerpo["subsanacion"] = views.SubsanacionPendienteJSON(p, ahora, nombrar)

	// Lo que dejó la revisión sólo lo ven quienes revisan o supervisan. A los
	// demás —el titular— ni siquiera se les muestra que fue descartada.
	if !models.PuedeVerRevision(sesion, p.ConvocatoriaID) {
		cuerpo["estado"] = string(models.EstadoParaTitular(p.Estado))
	} else {
		revision, err := a.datos.RevisionDe(r.Context(), p.ID)
		if a.falloDatos(w, r, err) {
			return
		}
		cambios, err := a.datos.DiferenciasDelUltimoEnvio(r.Context(), p.ID)
		if a.falloDatos(w, r, err) {
			return
		}
		cuerpo["revision"] = views.RevisionJSON(p, titular.Usuario, revision, cambios, ahora, nombrar)
	}
	// El resultado final sólo lo ve la supervisión: ni el postulante ni el
	// jurado ni el revisor (el resultado se comunica fuera del sistema).
	if models.PuedeSupervisar(sesion) {
		cuerpo["dictamen_final"] = p.DictamenFinal
	}
	a.responder(w, estado, cuerpo)
}

// GET /api/v1/catalogos   — catálogos fijos del formulario (público)
func (a *App) apiCatalogos(w http.ResponseWriter, _ *http.Request) {
	a.responder(w, http.StatusOK, views.CatalogosJSON())
}

// GET /api/v1/postulaciones   — las postulaciones de la cuenta, de todas las ediciones
func (a *App) apiMisPostulaciones(w http.ResponseWriter, r *http.Request) {
	resumenes, err := a.datos.PostulacionesDeUsuario(r.Context(), sesionDe(r).Usuario.ID)
	if a.falloDatos(w, r, err) {
		return
	}
	// Es la lista del propio postulante: el descarte no se le muestra.
	for i := range resumenes {
		resumenes[i].Estado = models.EstadoParaTitular(resumenes[i].Estado)
	}
	a.responder(w, http.StatusOK, views.PostulacionesJSON(resumenes))
}

// POST /api/v1/convocatorias/{id}/postulaciones   — abre un borrador
//
// En Intérpretes debe indicarse la subcategoría: «solista» o «ensamble».
func (a *App) apiCrearPostulacion(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	var entrada struct {
		Subcategoria string `json:"subcategoria"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	sesion := sesionDe(r)
	if !models.PuedePostular(sesion) {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return
	}
	id, err := a.datos.CrearPostulacion(r.Context(), a.actorDe(r), convocatoriaID, sesion.Usuario.ID, models.Subcategoria(entrada.Subcategoria))
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusCreated, id)
}

// GET /api/v1/postulaciones/{id}
func (a *App) apiPostulacion(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.idDeRuta(w, r); ok {
		a.responderPostulacion(w, r, http.StatusOK, id)
	}
}

// camposDeEntrada lee el cuerpo {"campos": {ruta: valor}} de un autoguardado
// y convierte cada valor a texto. Se admiten textos, enteros y null (que
// vacía el campo); cualquier otra cosa se rechaza.
func (a *App) camposDeEntrada(w http.ResponseWriter, r *http.Request) (map[string]string, bool) {
	var entrada struct {
		Campos map[string]any `json:"campos"`
	}
	if !a.leerJSON(w, r, &entrada) {
		return nil, false
	}
	if len(entrada.Campos) == 0 {
		a.errorAPI(w, r, http.StatusBadRequest, "json_invalido")
		return nil, false
	}
	campos := make(map[string]string, len(entrada.Campos))
	for ruta, valor := range entrada.Campos {
		switch v := valor.(type) {
		case nil:
			campos[ruta] = ""
		case string:
			campos[ruta] = v
		case float64:
			if v != float64(int64(v)) {
				a.errorAPI(w, r, http.StatusBadRequest, "json_invalido")
				return nil, false
			}
			campos[ruta] = strconv.FormatInt(int64(v), 10)
		default:
			a.errorAPI(w, r, http.StatusBadRequest, "json_invalido")
			return nil, false
		}
	}
	return campos, true
}

// responderGuardado responde a un autoguardado. Si no pudo guardarse ningún
// campo responde 422; si se guardó al menos uno, 200, con la lista de los
// rechazados para que la interfaz los señale.
func (a *App) responderGuardado(w http.ResponseWriter, r *http.Request, resultado models.ResultadoGuardado) {
	if len(resultado.Guardados) == 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: resultado.Errores})
		return
	}
	a.responder(w, http.StatusOK, views.GuardadoJSON(resultado, a.bloqueadores(r, resultado.Errores)))
}

// PATCH /api/v1/postulaciones/{id}   — autoguardado de datos generales y concierto
//
// Cuerpo: {"campos": {"datos.telefono": "55 1234 5678", …}}. Puede traer un
// solo campo (lo normal en el autoguardado) o varios.
func (a *App) apiGuardarPostulacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	campos, ok := a.camposDeEntrada(w, r)
	if !ok {
		return
	}
	resultado, err := a.datos.GuardarCamposDePostulacion(r.Context(), sesionDe(r).Usuario.ID, id, campos)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderGuardado(w, r, resultado)
}

// PUT /api/v1/postulaciones/{id}/subcategoria   — de solista a ensamble o al revés
//
// Cambiar de subcategoría borra los datos generales ya capturados, porque los
// formularios son distintos; por eso exige "confirmar": true (RF-POST-02).
func (a *App) apiCambiarSubcategoria(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Subcategoria string `json:"subcategoria"`
		Confirmar    bool   `json:"confirmar"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if !entrada.Confirmar {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "confirmar", Clave: "error.confirmar_cambio"}}})
		return
	}
	err := a.datos.CambiarSubcategoria(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, id, models.Subcategoria(entrada.Subcategoria))
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// PUT /api/v1/postulaciones/{id}/integrantes   — sustituye la lista de integrantes
func (a *App) apiDefinirIntegrantes(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Integrantes []struct {
			Nombre      string `json:"nombre"`
			Instrumento string `json:"instrumento"`
		} `json:"integrantes"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	integrantes := make([]models.Integrante, 0, len(entrada.Integrantes))
	for _, i := range entrada.Integrantes {
		integrantes = append(integrantes, models.Integrante{Nombre: i.Nombre, Instrumento: i.Instrumento})
	}
	if a.falloDatos(w, r, a.datos.DefinirIntegrantes(r.Context(), sesionDe(r).Usuario.ID, id, integrantes)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// PUT /api/v1/postulaciones/{id}/videos   — sustituye las ligas de video
func (a *App) apiDefinirVideos(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		URLs []string `json:"urls"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	ligas := make([]string, 0, len(entrada.URLs))
	for _, liga := range entrada.URLs {
		ligas = append(ligas, strings.TrimSpace(liga))
	}
	if a.falloDatos(w, r, a.datos.DefinirVideos(r.Context(), sesionDe(r).Usuario.ID, id, ligas)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// POST /api/v1/postulaciones/{id}/obras   — añade una obra vacía al programa
func (a *App) apiAgregarObra(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	if _, err := a.datos.AgregarObra(r.Context(), sesionDe(r).Usuario.ID, id); a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusCreated, id)
}

// PUT /api/v1/postulaciones/{id}/obras/orden   — fija el orden del programa
func (a *App) apiOrdenarObras(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		IDs []int64 `json:"ids"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	if a.falloDatos(w, r, a.datos.OrdenarObras(r.Context(), sesionDe(r).Usuario.ID, id, entrada.IDs)) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// PATCH /api/v1/obras/{id}   — autoguardado de una obra
func (a *App) apiGuardarObra(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	campos, ok := a.camposDeEntrada(w, r)
	if !ok {
		return
	}
	_, resultado, err := a.datos.GuardarCamposDeObra(r.Context(), sesionDe(r).Usuario.ID, id, campos)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderGuardado(w, r, resultado)
}

// DELETE /api/v1/obras/{id}   — quita una obra del borrador
func (a *App) apiBorrarObra(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	postulacionID, err := a.datos.BorrarObra(r.Context(), sesionDe(r).Usuario.ID, id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, postulacionID)
}
