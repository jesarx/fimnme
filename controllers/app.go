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

// Package controllers contiene el Controlador de la arquitectura MVC:
// enrutamiento, orquestación de peticiones y control de acceso. No contiene
// reglas de negocio (models/rules.go) ni consultas SQL (models/repository.go).
package controllers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Entornos de ejecución. En desarrollo se muestra una franja de advertencia y
// se pide a los buscadores que no indexen el sitio.
const (
	EntornoDesarrollo = "desarrollo"
	EntornoProduccion = "produccion"
)

// Config son las dependencias y ajustes de la aplicación.
type Config struct {
	Vistas  *views.Motor
	Datos   models.Persistencia
	Entorno string
	Log     *slog.Logger
	// URLBase es la dirección pública del sitio (p. ej. https://postula.ejemplo.mx),
	// con la que se arman los enlaces de los correos. Se toma de la
	// configuración y NUNCA de la cabecera Host de la petición, que el
	// visitante controla.
	URLBase string
	// Almacen guarda el contenido de los documentos y de los acuses.
	Almacen models.BlobStore
	// Exportaciones es el directorio donde se arman las descargas masivas.
	Exportaciones string
	// Enlaces comprueba en red las ligas de video antes de un envío. Con nil
	// no se comprueban: quedan «pendientes» para el revisor.
	Enlaces *VerificadorDeEnlaces
	// Ahora es el reloj; nil usa el del sistema. Las pruebas lo sustituyen.
	Ahora func() time.Time
}

// App reúne las dependencias de los controladores. Se construye una vez al
// arrancar y después sólo se lee, por lo que es segura entre peticiones.
type App struct {
	vistas  *views.Motor
	datos   models.Persistencia
	entorno string
	log     *slog.Logger
	urlBase string
	almacen models.BlobStore
	// exportaciones es el directorio de las descargas masivas ya armadas.
	exportaciones string
	enlaces       *VerificadorDeEnlaces
	ahora         func() time.Time
	limiteIP      *limitador // intentos por IP sobre los formularios de cuenta
}

// Nueva construye la aplicación con sus dependencias explícitas.
func Nueva(c Config) *App {
	ahora := c.Ahora
	if ahora == nil {
		ahora = time.Now
	}
	return &App{
		vistas:        c.Vistas,
		datos:         c.Datos,
		entorno:       c.Entorno,
		log:           c.Log,
		urlBase:       strings.TrimRight(c.URLBase, "/"),
		almacen:       c.Almacen,
		exportaciones: c.Exportaciones,
		enlaces:       c.Enlaces,
		ahora:         ahora,
		limiteIP:      nuevoLimitador(intentosPorMinuto, ahora),
	}
}

// Manejador devuelve el http.Handler completo: las rutas envueltas en la
// cadena de middleware. El enrutador es http.ServeMux de la biblioteca
// estándar, que desde Go 1.22 admite método y variables de ruta.
//
// Regla de las rutas: NINGÚN GET modifica estado. Todo lo que cambia algo es
// un POST, y los POST están protegidos contra CSRF por protegerOrigen.
func (a *App) Manejador() http.Handler {
	mux := http.NewServeMux()

	// Páginas públicas.
	mux.HandleFunc("GET /{$}", a.inicio)
	mux.HandleFunc("GET /convocatorias/{edicion}/{tipo}", a.convocatoria)
	mux.HandleFunc("GET /convocatorias/{edicion}/{tipo}/bases.pdf", a.basesPDF)
	mux.HandleFunc("GET /convocatorias/{edicion}/{tipo}/imagen", a.imagenDeConvocatoria)
	mux.HandleFunc("GET /aviso-de-privacidad", a.avisoPrivacidad)

	// Cuenta sin sesión: registro, ingreso y enlaces de un solo uso.
	mux.HandleFunc("GET /cuenta/registro", a.registroFormulario)
	mux.HandleFunc("POST /cuenta/registro", a.registro)
	mux.HandleFunc("GET /cuenta/ingresar", a.ingresarFormulario)
	mux.HandleFunc("POST /cuenta/ingresar", a.ingresar)
	mux.HandleFunc("GET /cuenta/recuperar", a.recuperarFormulario)
	mux.HandleFunc("POST /cuenta/recuperar", a.recuperar)
	mux.HandleFunc("GET /cuenta/verificar", a.verificarFormulario)
	mux.HandleFunc("POST /cuenta/verificar", a.verificar)
	mux.HandleFunc("GET /cuenta/restablecer", a.passwordNuevaFormulario(models.TokenResetPassword))
	mux.HandleFunc("POST /cuenta/restablecer", a.passwordNueva(models.TokenResetPassword))
	mux.HandleFunc("GET /cuenta/activar", a.passwordNuevaFormulario(models.TokenActivacion))
	mux.HandleFunc("POST /cuenta/activar", a.passwordNueva(models.TokenActivacion))

	// Cuenta con sesión (RF-AUT-06).
	mux.HandleFunc("GET /cuenta", a.conSesion(a.miCuenta))
	mux.HandleFunc("POST /cuenta/datos", a.conSesion(a.guardarDatos))
	mux.HandleFunc("POST /cuenta/password", a.conSesion(a.cambiarPassword))
	mux.HandleFunc("POST /cuenta/email", a.conSesion(a.cambiarEmail))
	mux.HandleFunc("POST /cuenta/verificacion", a.conSesion(a.reenviarVerificacion))
	mux.HandleFunc("POST /cuenta/salir", a.salir)

	// Preferencia de idioma (RF-I18N-03).
	mux.HandleFunc("POST /idioma", a.cambiarIdioma)

	// API REST: responde JSON. Sus rutas viven en una tabla (rutasAPI) para
	// poder cotejarlas contra la descripción OpenAPI.
	for _, ruta := range a.rutasAPI() {
		mux.HandleFunc(ruta.metodo+" "+ruta.patron, ruta.manejador)
	}

	// Estáticos incrustados y sondas de vida para el proxy y systemd.
	mux.Handle("GET /static/", a.vistas.Estaticos())
	mux.HandleFunc("GET /healthz", a.vivo)
	mux.HandleFunc("GET /readyz", a.listo)

	// Cualquier otra ruta: 404 propio, en el idioma del visitante.
	mux.HandleFunc("/", a.noEncontrado)

	// El orden importa: el primero de la lista es el más externo.
	return encadenar(mux,
		a.recuperarPanico,
		a.registrarPeticion,
		a.cabecerasSeguridad,
		a.protegerOrigen,
		limitarCuerpo,
		a.limitarIntentos, // antes de tocar la base
		a.cargarSesion,    // antes del idioma: la preferencia de la cuenta manda
		resolverIdioma,
	)
}

// rutaAPI es una operación de la API REST.
type rutaAPI struct {
	metodo    string
	patron    string
	manejador http.HandlerFunc
}

// rutasAPI es la lista completa de operaciones de /api/v1, cada una con su
// control de acceso: conSesion (cualquier cuenta), supervisa (administrador y
// observador) o administra (sólo administrador). Toda operación que se añada
// aquí debe describirse también en views/api/openapi.json; la prueba
// TestOpenAPICoincideConLasRutas falla si falta en cualquiera de los dos lados.
func (a *App) rutasAPI() []rutaAPI {
	return []rutaAPI{
		{"GET", "/api/v1/openapi.json", a.apiOpenAPI},
		{"GET", "/api/v1/catalogos", a.apiCatalogos},
		{"GET", "/api/v1/sesion", a.conSesion(a.apiSesion)},
		{"GET", "/api/v1/auditoria", a.conSesion(a.apiAuditoria)},
		{"GET", "/api/v1/ediciones", a.supervisa(a.apiEdiciones)},
		{"POST", "/api/v1/ediciones", a.administra(a.apiCrearEdicion)},
		{"GET", "/api/v1/ediciones/{id}", a.supervisa(a.apiEdicion)},
		{"PUT", "/api/v1/ediciones/{id}", a.administra(a.apiActualizarEdicion)},
		{"POST", "/api/v1/ediciones/{id}/convocatorias", a.administra(a.apiCrearConvocatoria)},
		{"GET", "/api/v1/convocatorias/{id}", a.supervisa(a.apiConvocatoria)},
		{"PUT", "/api/v1/convocatorias/{id}", a.administra(a.apiActualizarConvocatoria)},
		{"PUT", "/api/v1/convocatorias/{id}/etapas", a.administra(a.apiDefinirEtapas)},
		{"POST", "/api/v1/convocatorias/{id}/estado", a.administra(a.apiCambiarEstado)},
		{"POST", "/api/v1/convocatorias/{id}/duplicar", a.administra(a.apiDuplicarConvocatoria)},
		{"POST", "/api/v1/convocatorias/{id}/dotaciones", a.administra(a.apiCrearDotacion)},
		{"PUT", "/api/v1/dotaciones/{id}", a.administra(a.apiActualizarDotacion)},
		{"POST", "/api/v1/convocatorias/{id}/requisitos", a.administra(a.apiCrearRequisito)},
		{"PUT", "/api/v1/requisitos/{id}", a.administra(a.apiActualizarRequisito)},
		{"DELETE", "/api/v1/requisitos/{id}", a.administra(a.apiBorrarRequisito)},
		{"POST", "/api/v1/convocatorias/{id}/criterios", a.administra(a.apiCrearCriterio)},
		{"PUT", "/api/v1/criterios/{id}", a.administra(a.apiActualizarCriterio)},
		{"DELETE", "/api/v1/criterios/{id}", a.administra(a.apiBorrarCriterio)},
		{"POST", "/api/v1/convocatorias/{id}/asignaciones", a.administra(a.apiAsignar)},
		{"DELETE", "/api/v1/asignaciones/{id}", a.administra(a.apiRetirarAsignacion)},
		{"GET", "/api/v1/usuarios", a.administra(a.apiUsuarios)},
		{"POST", "/api/v1/usuarios", a.administra(a.apiCrearUsuario)},
		{"POST", "/api/v1/usuarios/{id}/activacion", a.administra(a.apiReenviarActivacion)},

		// Postulaciones: cada cuenta sobre las suyas.
		{"GET", "/api/v1/postulaciones", a.conSesion(a.apiMisPostulaciones)},
		{"POST", "/api/v1/convocatorias/{id}/postulaciones", a.conSesion(a.apiCrearPostulacion)},
		{"GET", "/api/v1/postulaciones/{id}", a.conSesion(a.apiPostulacion)},
		{"PATCH", "/api/v1/postulaciones/{id}", a.conSesion(a.apiGuardarPostulacion)},
		{"PUT", "/api/v1/postulaciones/{id}/subcategoria", a.conSesion(a.apiCambiarSubcategoria)},
		{"PUT", "/api/v1/postulaciones/{id}/integrantes", a.conSesion(a.apiDefinirIntegrantes)},
		{"PUT", "/api/v1/postulaciones/{id}/videos", a.conSesion(a.apiDefinirVideos)},
		{"POST", "/api/v1/postulaciones/{id}/obras", a.conSesion(a.apiAgregarObra)},
		{"PUT", "/api/v1/postulaciones/{id}/obras/orden", a.conSesion(a.apiOrdenarObras)},
		{"PATCH", "/api/v1/obras/{id}", a.conSesion(a.apiGuardarObra)},
		{"DELETE", "/api/v1/obras/{id}", a.conSesion(a.apiBorrarObra)},

		// Documentos, envío y acuse.
		{"POST", "/api/v1/postulaciones/{id}/documentos", a.conSesion(a.apiSubirDocumento)},
		{"DELETE", "/api/v1/documentos/{id}", a.conSesion(a.apiQuitarDocumento)},
		{"GET", "/api/v1/documentos/{id}/archivo", a.conSesion(a.apiDescargarDocumento)},
		{"POST", "/api/v1/postulaciones/{id}/envio", a.conSesion(a.apiEnviarPostulacion)},
		{"GET", "/api/v1/postulaciones/{id}/acuses/{version}", a.conSesion(a.apiDescargarAcuse)},

		// Revisión documental y subsanación.
		{"GET", "/api/v1/convocatorias/{id}/revision", a.conSesion(a.apiBandejaDeRevision)},
		{"POST", "/api/v1/postulaciones/{id}/toma", a.conSesion(a.apiTomarPostulacion(true))},
		{"DELETE", "/api/v1/postulaciones/{id}/toma", a.conSesion(a.apiTomarPostulacion(false))},
		{"PUT", "/api/v1/documentos/{id}/dictamen", a.conSesion(a.apiDictaminarDocumento)},
		{"POST", "/api/v1/postulaciones/{id}/observaciones", a.conSesion(a.apiObservarPostulacion)},
		{"POST", "/api/v1/postulaciones/{id}/subsanaciones", a.conSesion(a.apiSolicitarSubsanacion)},
		{"POST", "/api/v1/postulaciones/{id}/desbloqueos", a.conSesion(a.apiSolicitarDesbloqueo)},
		{"PUT", "/api/v1/postulaciones/{id}/desbloqueos/{solicitud}", a.conSesion(a.apiResolverDesbloqueo)},
		{"POST", "/api/v1/postulaciones/{id}/dictamen", a.conSesion(a.apiDictaminarPostulacion)},

		// Evaluación por jurado. Las cinco primeras son del jurado, sobre lo
		// suyo; la matriz de resultados es sólo de supervisión.
		{"GET", "/api/v1/convocatorias/{id}/evaluacion", a.conSesion(a.apiListaDeJurado)},
		{"GET", "/api/v1/postulaciones/{id}/evaluacion", a.conSesion(a.apiEvaluacionPropia)},
		{"PUT", "/api/v1/postulaciones/{id}/evaluacion", a.conSesion(a.apiGuardarEvaluacion)},
		{"PUT", "/api/v1/postulaciones/{id}/excusa", a.conSesion(a.apiExcusarse)},
		{"DELETE", "/api/v1/postulaciones/{id}/excusa", a.conSesion(a.apiRetirarExcusa)},
		{"GET", "/api/v1/convocatorias/{id}/resultados", a.supervisa(a.apiResultados)},
		{"PUT", "/api/v1/postulaciones/{id}/dictamen-final", a.administra(a.apiDictamenFinal)},

		// Archivo histórico: listado con búsqueda y filtros, enlaces por
		// folio, vistas guardadas y descargas masivas.
		{"GET", "/api/v1/archivo", a.conSesion(a.apiArchivo)},
		{"GET", "/api/v1/folios/{folio}", a.conSesion(a.apiPostulacionPorFolio)},
		{"GET", "/api/v1/vistas", a.conSesion(a.apiVistas)},
		{"POST", "/api/v1/vistas", a.conSesion(a.apiGuardarVista)},
		{"DELETE", "/api/v1/vistas/{id}", a.conSesion(a.apiBorrarVista)},
		{"POST", "/api/v1/exportaciones", a.conSesion(a.apiPedirExportacion)},
		{"GET", "/api/v1/exportaciones/{id}", a.conSesion(a.apiExportacion)},
		{"GET", "/api/v1/exportaciones/{id}/archivo", a.conSesion(a.apiDescargarExportacion)},

		// Archivos públicos de una convocatoria: bases en PDF e imagen.
		{"PUT", "/api/v1/convocatorias/{id}/archivos/{clase}", a.administra(a.apiSubirArchivoDeConvocatoria)},
		{"DELETE", "/api/v1/convocatorias/{id}/archivos/{clase}", a.administra(a.apiQuitarArchivoDeConvocatoria)},
	}
}

// plazoSonda acota la comprobación de la base en /readyz.
const plazoSonda = 2 * time.Second

// vivo responde a /healthz: el proceso está en pie. No toca la base, para que
// un problema de datos no haga que systemd reinicie el servicio en bucle.
func (a *App) vivo(w http.ResponseWriter, _ *http.Request) {
	responderSonda(w, http.StatusOK, "ok\n")
}

// listo responde a /readyz: el servicio puede atender, es decir, la base de
// datos responde. Si no, 503, para que el proxy o el monitoreo lo detecten.
func (a *App) listo(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := context.WithTimeout(r.Context(), plazoSonda)
	defer cancelar()
	if err := a.datos.Verificar(ctx); err != nil {
		a.log.Error("sonda /readyz: la base de datos no responde", "error", err)
		responderSonda(w, http.StatusServiceUnavailable, "no disponible\n")
		return
	}
	responderSonda(w, http.StatusOK, "ok\n")
}

// responderSonda escribe la respuesta de texto de una sonda.
func responderSonda(w http.ResponseWriter, estado int, texto string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(estado)
	_, _ = w.Write([]byte(texto))
}
