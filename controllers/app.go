// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
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

// Datos es lo que los controladores necesitan de la persistencia. La interfaz
// se define aquí, del lado de quien la consume, y la satisface
// *models.Repositorio; los controladores nunca ven SQL ni conexiones.
type Datos interface {
	// Sonda.
	Verificar(ctx context.Context) error

	// Convocatorias públicas.
	ConvocatoriasPublicas(ctx context.Context) ([]models.Convocatoria, error)
	ConvocatoriaPublica(ctx context.Context, numeroEdicion int, tipo models.TipoConvocatoria) (models.Convocatoria, error)

	// Cuentas.
	CrearUsuario(ctx context.Context, actor models.Actor, n models.NuevoUsuario, token *models.NuevoToken, correos ...models.CorreoSaliente) (int64, error)
	UsuarioPorEmail(ctx context.Context, email string) (models.Usuario, string, error)
	ResumenDePassword(ctx context.Context, usuarioID int64) (string, error)
	ActualizarDatosPersonales(ctx context.Context, actor models.Actor, usuarioID int64, nombre, apellidos, idioma string) error
	CambiarPassword(ctx context.Context, actor models.Actor, usuarioID int64, resumen, sesionAConservar string) error
	CambiarEmail(ctx context.Context, actor models.Actor, usuarioID int64, email string, token models.NuevoToken, correos ...models.CorreoSaliente) error

	// Sesiones e inicios de sesión.
	AbrirSesion(ctx context.Context, actor models.Actor, usuarioID int64, idResumen string, expira time.Time) error
	RegistrarIngresoFallido(ctx context.Context, actor models.Actor, usuarioID int64, bloqueado bool) error
	FallosDeIngreso(ctx context.Context, usuarioID int64) (int, time.Time, error)
	SesionPorID(ctx context.Context, idResumen string) (*models.Sesion, error)
	RenovarSesion(ctx context.Context, idResumen string, expira time.Time) error
	CerrarSesion(ctx context.Context, actor models.Actor, idResumen string) error
	PurgarSesionesVencidas(ctx context.Context) (int64, error)

	// Enlaces de un solo uso.
	EmitirToken(ctx context.Context, actor models.Actor, usuarioID int64, token models.NuevoToken, correos ...models.CorreoSaliente) error
	UltimaExpiracionDeToken(ctx context.Context, usuarioID int64, proposito models.PropositoToken) (time.Time, error)
	TokenVigente(ctx context.Context, resumen string, proposito models.PropositoToken) (models.Usuario, error)
	VerificarEmailConToken(ctx context.Context, actor models.Actor, resumen string) (int64, error)
	DefinirPasswordConToken(ctx context.Context, actor models.Actor, resumenToken string, proposito models.PropositoToken, resumenPassword string) (int64, error)

	// Correo saliente.
	PlantillaCorreo(ctx context.Context, clave, idioma string) (models.PlantillaCorreo, error)
	CorreosPendientes(ctx context.Context, limite int) ([]models.CorreoSaliente, error)
	MarcarCorreoEnviado(ctx context.Context, correo models.CorreoSaliente, messageID string) error
	MarcarCorreoFallido(ctx context.Context, correo models.CorreoSaliente, motivo string, proximo time.Time, reintentar bool) error

	// Bitácora.
	ConsultarAuditoria(ctx context.Context, filtro models.FiltroAuditoria) ([]models.AsientoAuditoria, int, error)
}

// Config son las dependencias y ajustes de la aplicación.
type Config struct {
	Vistas  *views.Motor
	Datos   Datos
	Entorno string
	Log     *slog.Logger
	// URLBase es la dirección pública del sitio (p. ej. https://postula.ejemplo.mx),
	// con la que se arman los enlaces de los correos. Se toma de la
	// configuración y NUNCA de la cabecera Host de la petición, que el
	// visitante controla.
	URLBase string
	// Ahora es el reloj; nil usa el del sistema. Las pruebas lo sustituyen.
	Ahora func() time.Time
}

// App reúne las dependencias de los controladores. Se construye una vez al
// arrancar y después sólo se lee, por lo que es segura entre peticiones.
type App struct {
	vistas   *views.Motor
	datos    Datos
	entorno  string
	log      *slog.Logger
	urlBase  string
	ahora    func() time.Time
	limiteIP *limitador // intentos por IP sobre los formularios de cuenta
}

// Nueva construye la aplicación con sus dependencias explícitas.
func Nueva(c Config) *App {
	ahora := c.Ahora
	if ahora == nil {
		ahora = time.Now
	}
	return &App{
		vistas:   c.Vistas,
		datos:    c.Datos,
		entorno:  c.Entorno,
		log:      c.Log,
		urlBase:  strings.TrimRight(c.URLBase, "/"),
		ahora:    ahora,
		limiteIP: nuevoLimitador(intentosPorMinuto, ahora),
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

	// API REST: responde JSON.
	mux.HandleFunc("GET /api/v1/sesion", a.conSesion(a.apiSesion))
	mux.HandleFunc("GET /api/v1/auditoria", a.conSesion(a.apiAuditoria))

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
