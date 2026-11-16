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
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Verificación en dos pasos (RF-AUT-09), obligatoria para administrador y
// observador. El flujo:
//
//  1. La contraseña correcta abre una sesión PENDIENTE, de 10 minutos.
//  2. Esa sesión no es una sesión para el resto del sistema: viaja en el
//     contexto con otra clave (claveSesionPendiente) y sesionDe() no la ve.
//     Las páginas públicas la tratan como anónima; las que piden sesión
//     llevan a /cuenta/segundo-factor y la API responde 401.
//  3. En /cuenta/segundo-factor se escribe el código de la aplicación
//     autenticadora o uno de recuperación. La primera vez, esa página es el
//     alta: clave en QR y en texto, y la confirmación con un primer código,
//     tras la cual se muestran, una sola vez, los códigos de recuperación.
//  4. Con el código correcto la sesión se vuelve plena.
//
// Los fallos cuentan para el bloqueo por intentos (RF-AUT-08) igual que una
// contraseña incorrecta.

// claveSesionPendiente guarda en el contexto la sesión que espera el código.
type claveSesionPendiente struct{}

// sesionPendienteDe devuelve la sesión que espera el segundo paso, o nil.
func sesionPendienteDe(r *http.Request) *models.Sesion {
	sesion, _ := r.Context().Value(claveSesionPendiente{}).(*models.Sesion)
	return sesion
}

// conSesionPendiente pone una sesión pendiente en el contexto.
func conSesionPendiente(ctx context.Context, s *models.Sesion) context.Context {
	return context.WithValue(ctx, claveSesionPendiente{}, s)
}

// rutaDelSegundoFactor es la página del código, con la ruta a la que volver.
func rutaDelSegundoFactor(volver string) string {
	if volver = rutaLocal(volver); volver == "/" {
		return "/cuenta/segundo-factor"
	}
	return "/cuenta/segundo-factor?volver=" + url.QueryEscape(volver)
}

// iniciarSesionPendiente abre la sesión de 10 minutos que espera el código.
func (a *App) iniciarSesionPendiente(w http.ResponseWriter, r *http.Request, usuarioID int64) error {
	claro, resumen, err := models.NuevoSecreto()
	if err != nil {
		return err
	}
	actor := a.actorDe(r)
	actor.UsuarioID, actor.Perfil = usuarioID, ""
	if err := a.datos.AbrirSesionPendiente(r.Context(), actor, usuarioID, resumen, a.ahora().Add(models.SesionPendiente)); err != nil {
		return err
	}
	a.ponerCookieSesion(w, r, claro, models.SesionPendiente)
	return nil
}

// vistaSegundoFactor es el modelo de la página del código.
type vistaSegundoFactor struct {
	Modo   string // «alta», «codigo» o «recuperacion» (los códigos recién emitidos)
	Volver string
	// Alta: la clave en grupos de cuatro, el enlace otpauth:// y su QR.
	Clave  string
	Enlace template.URL
	QR     template.HTML
	// Códigos de recuperación recién emitidos: se muestran una sola vez.
	Codigos []string
}

// actorPendiente identifica para la bitácora a quien está a medio ingreso.
func (a *App) actorPendiente(r *http.Request, s *models.Sesion) models.Actor {
	actor := a.actorDe(r)
	actor.UsuarioID, actor.Perfil = s.Usuario.ID, string(s.PerfilPrincipal())
	return actor
}

// GET /cuenta/segundo-factor   — el alta o la petición del código
func (a *App) segundoFactorFormulario(w http.ResponseWriter, r *http.Request) {
	s := sesionPendienteDe(r)
	if s == nil {
		// Sin sesión pendiente no hay nada que pedir: o ya entró, o no.
		http.Redirect(w, r, "/cuenta", http.StatusSeeOther)
		return
	}
	a.pintarSegundoFactor(w, r, http.StatusOK, s, nil)
}

// pintarSegundoFactor muestra el alta (si la cuenta aún no la hizo) o la
// petición del código.
func (a *App) pintarSegundoFactor(w http.ResponseWriter, r *http.Request, estado int, s *models.Sesion, errores []models.ErrorCampo) {
	factor, err := a.datos.SegundoFactorDe(r.Context(), s.Usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "leer segundo factor", err)
		return
	}
	v := vistaSegundoFactor{Modo: "codigo", Volver: rutaLocal(r.FormValue("volver"))}
	titulo := "segundo_factor.titulo"
	if !factor.Activo() {
		nuevo, err := models.NuevoSecretoTOTP()
		if err != nil {
			a.errorInterno(w, r, "generar clave TOTP", err)
			return
		}
		secreto, err := a.datos.PrepararSegundoFactor(r.Context(), s.Usuario.ID, nuevo)
		if err != nil {
			a.errorInterno(w, r, "preparar segundo factor", err)
			return
		}
		enlace := models.URIDeAltaTOTP(secreto, s.Usuario.Email)
		v.Modo, titulo = "alta", "segundo_factor.alta_titulo"
		v.Clave, v.Enlace, v.QR = enGrupos(secreto), template.URL(enlace), views.QRSVG(enlace)
	}
	p := a.pagina(r, titulo)
	p.Formulario = views.Formulario{Errores: errores}
	p.Datos = v
	// La página del código nunca se guarda: en el alta lleva la clave.
	w.Header().Set("Cache-Control", "no-store")
	a.pintar(w, r, estado, "cuenta_segundo_factor.html", p)
}

// enGrupos separa la clave en grupos de cuatro, para copiarla a mano.
func enGrupos(clave string) string {
	var grupos []string
	for len(clave) > 4 {
		grupos, clave = append(grupos, clave[:4]), clave[4:]
	}
	return strings.Join(append(grupos, clave), " ")
}

// POST /cuenta/segundo-factor   — comprueba el código (o uno de recuperación)
func (a *App) segundoFactor(w http.ResponseWriter, r *http.Request) {
	s := sesionPendienteDe(r)
	if s == nil {
		http.Redirect(w, r, "/cuenta/ingresar", http.StatusSeeOther)
		return
	}
	actor := a.actorPendiente(r, s)
	rechazar := func(estado int, clave string) {
		a.pintarSegundoFactor(w, r, estado, s, []models.ErrorCampo{{Campo: "codigo", Clave: clave}})
	}
	if bloqueado, err := a.bloqueado(r, s.Usuario.ID); err != nil {
		a.errorInterno(w, r, "contar intentos fallidos", err)
		return
	} else if bloqueado {
		if err := a.datos.RegistrarIngresoFallido(r.Context(), actor, s.Usuario.ID, true); err != nil {
			a.errorInterno(w, r, "registrar intento de ingreso", err)
			return
		}
		rechazar(http.StatusTooManyRequests, "error.bloqueo")
		return
	}
	factor, err := a.datos.SegundoFactorDe(r.Context(), s.Usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "leer segundo factor", err)
		return
	}
	codigo := strings.TrimSpace(r.PostFormValue("codigo"))
	var paso int64
	var resumenCodigo string
	if factor.Activo() {
		if p, ok := models.VerificarTOTP(factor.Secreto, codigo, a.ahora(), factor.UltimoPaso); ok {
			paso = p
		} else if len(strings.ReplaceAll(codigo, " ", "")) != 6 {
			// No son seis cifras: se prueba como código de recuperación.
			resumenCodigo = models.ResumenDeCodigoDeRecuperacion(codigo)
		}
	}
	err = models.ErrCodigoInvalido
	if paso > 0 || resumenCodigo != "" {
		err = a.datos.CompletarSegundoFactor(r.Context(), actor, s.ID, s.Usuario.ID, paso, resumenCodigo)
	}
	switch {
	case err == nil:
		http.Redirect(w, r, destinoTrasIngreso(r.PostFormValue("volver")), http.StatusSeeOther)
	case errors.Is(err, models.ErrCodigoInvalido):
		if err := a.datos.RegistrarIngresoFallido(r.Context(), actor, s.Usuario.ID, false); err != nil {
			a.errorInterno(w, r, "registrar intento de ingreso", err)
			return
		}
		rechazar(http.StatusUnauthorized, "error.codigo_incorrecto")
	default:
		a.errorInterno(w, r, "completar segundo factor", err)
	}
}

// POST /cuenta/segundo-factor/alta   — confirma el alta con un primer código
func (a *App) altaDeSegundoFactor(w http.ResponseWriter, r *http.Request) {
	s := sesionPendienteDe(r)
	if s == nil {
		http.Redirect(w, r, "/cuenta/ingresar", http.StatusSeeOther)
		return
	}
	actor := a.actorPendiente(r, s)
	if bloqueado, err := a.bloqueado(r, s.Usuario.ID); err != nil {
		a.errorInterno(w, r, "contar intentos fallidos", err)
		return
	} else if bloqueado {
		a.pintarSegundoFactor(w, r, http.StatusTooManyRequests, s, []models.ErrorCampo{{Campo: "codigo", Clave: "error.bloqueo"}})
		return
	}
	factor, err := a.datos.SegundoFactorDe(r.Context(), s.Usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "leer segundo factor", err)
		return
	}
	paso, ok := models.VerificarTOTP(factor.Secreto, r.PostFormValue("codigo"), a.ahora(), factor.UltimoPaso)
	if factor.Activo() || factor.Secreto == "" || !ok {
		if err := a.datos.RegistrarIngresoFallido(r.Context(), actor, s.Usuario.ID, false); err != nil {
			a.errorInterno(w, r, "registrar intento de ingreso", err)
			return
		}
		a.pintarSegundoFactor(w, r, http.StatusUnauthorized, s, []models.ErrorCampo{{Campo: "codigo", Clave: "error.codigo_incorrecto"}})
		return
	}
	claros, resumenes, err := models.NuevosCodigosDeRecuperacion()
	if err != nil {
		a.errorInterno(w, r, "generar códigos de recuperación", err)
		return
	}
	if err := a.datos.ActivarSegundoFactor(r.Context(), actor, s.ID, s.Usuario.ID, paso, resumenes); err != nil {
		a.errorInterno(w, r, "activar segundo factor", err)
		return
	}
	a.pintarCodigosDeRecuperacion(w, r, claros, destinoTrasIngreso(r.PostFormValue("volver")))
}

// pintarCodigosDeRecuperacion muestra, UNA sola vez, los códigos recién
// emitidos. Se responde con la página (no con una redirección) para que los
// códigos no pasen por ninguna URL ni por el historial.
func (a *App) pintarCodigosDeRecuperacion(w http.ResponseWriter, r *http.Request, codigos []string, volver string) {
	p := a.pagina(r, "segundo_factor.codigos_titulo")
	p.Datos = vistaSegundoFactor{Modo: "recuperacion", Codigos: codigos, Volver: volver}
	w.Header().Set("Cache-Control", "no-store")
	a.pintar(w, r, http.StatusOK, "cuenta_segundo_factor.html", p)
}

// POST /cuenta/segundo-factor/codigos   — códigos de recuperación nuevos
//
// Desde «Mi cuenta», con sesión plena y un código vigente de la aplicación:
// tener la sesión abierta no basta para cambiar la forma de entrar.
func (a *App) renovarCodigosDeRecuperacion(w http.ResponseWriter, r *http.Request) {
	s := sesionDe(r)
	factor, err := a.datos.SegundoFactorDe(r.Context(), s.Usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "leer segundo factor", err)
		return
	}
	if !factor.Activo() {
		http.Redirect(w, r, "/cuenta", http.StatusSeeOther)
		return
	}
	if _, ok := models.VerificarTOTP(factor.Secreto, r.PostFormValue("codigo"), a.ahora(), 0); !ok {
		a.pintarMiCuenta(w, r, http.StatusUnprocessableEntity, nil, []models.ErrorCampo{{Campo: "codigo", Clave: "error.codigo_incorrecto"}})
		return
	}
	claros, resumenes, err := models.NuevosCodigosDeRecuperacion()
	if err != nil {
		a.errorInterno(w, r, "generar códigos de recuperación", err)
		return
	}
	if err := a.datos.RenovarCodigosDeRecuperacion(r.Context(), a.actorDe(r), s.Usuario.ID, resumenes); err != nil {
		a.errorInterno(w, r, "renovar códigos de recuperación", err)
		return
	}
	a.pintarCodigosDeRecuperacion(w, r, claros, "/cuenta")
}

// bloqueado aplica al segundo paso el mismo bloqueo por intentos que al
// ingreso (RF-AUT-08): los códigos equivocados cuentan como contraseñas
// equivocadas.
func (a *App) bloqueado(r *http.Request, usuarioID int64) (bool, error) {
	fallos, ultimoFallo, err := a.datos.FallosDeIngreso(r.Context(), usuarioID)
	if err != nil {
		return false, err
	}
	return a.ahora().Before(models.BloqueoDeIngreso(fallos, ultimoFallo)), nil
}

// destinoTrasIngreso es la página a la que se va al terminar de ingresar.
func destinoTrasIngreso(volver string) string {
	if volver = rutaLocal(volver); volver == "/" {
		return "/cuenta"
	}
	return volver
}
