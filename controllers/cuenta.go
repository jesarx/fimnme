// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package controllers

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Cuentas (§5.1): registro, verificación de correo, inicio y cierre de sesión,
// recuperación de contraseña, activación de cuentas internas y «Mi cuenta».
//
// Los controladores sólo orquestan: validan con models/rules.go, calculan
// resúmenes con models/seguridad.go, arman los correos con views/correo.go y
// escriben a través del repositorio, que hace cada operación en una sola
// transacción junto con su asiento de auditoría y sus correos.

// --- Piezas compartidas ---------------------------------------------------------

// notas son los mensajes que una página puede mostrar tras una redirección,
// elegidos con ?aviso=clave. Es una lista cerrada: el visitante no puede hacer
// que la página muestre un texto arbitrario.
var notas = map[string]views.Aviso{
	"registro":              {Tipo: "exito", Clave: "nota.registro"},
	"verificado":            {Tipo: "exito", Clave: "nota.verificado"},
	"datos":                 {Tipo: "exito", Clave: "nota.datos"},
	"password":              {Tipo: "exito", Clave: "nota.password"},
	"email":                 {Tipo: "exito", Clave: "nota.email"},
	"verificacion_enviada":  {Tipo: "exito", Clave: "nota.verificacion_enviada"},
	"verificacion_reciente": {Tipo: "info", Clave: "nota.verificacion_reciente"},
	"password_restablecida": {Tipo: "exito", Clave: "nota.password_restablecida"},
	"activada":              {Tipo: "exito", Clave: "nota.activada"},
	"salida":                {Tipo: "info", Clave: "nota.salida"},
	"ingresa":               {Tipo: "info", Clave: "nota.ingresa"},
}

// notaDe devuelve el aviso pedido en la URL, o nil.
func notaDe(r *http.Request) *views.Aviso {
	if nota, ok := notas[r.URL.Query().Get("aviso")]; ok {
		return &nota
	}
	return nil
}

// errorInterno registra un fallo inesperado y muestra la página 500. El
// detalle queda en el log; al visitante nunca se le muestra.
func (a *App) errorInterno(w http.ResponseWriter, r *http.Request, operacion string, err error) {
	a.log.Error(operacion, "ruta", r.URL.Path, "error", err)
	a.errorPagina(w, r, http.StatusInternalServerError)
}

// nuevoToken genera un enlace de un solo uso: el valor en claro, que viajará
// en el correo, y lo que se guarda en la base.
func (a *App) nuevoToken(proposito models.PropositoToken) (string, models.NuevoToken, error) {
	claro, resumen, err := models.NuevoSecreto()
	return claro, models.NuevoToken{
		Resumen:   resumen,
		Proposito: proposito,
		ExpiraAt:  a.ahora().Add(models.VigenciaDeToken(proposito)),
	}, err
}

// liga arma la dirección pública de una página que recibe un enlace de un
// solo uso, a partir de la URL base configurada.
func (a *App) liga(ruta, token string) string {
	return a.urlBase + ruta + "?token=" + url.QueryEscape(token)
}

// correo arma un mensaje a partir de la plantilla activa en el idioma del
// destinatario (RF-I18N-07).
func (a *App) correo(r *http.Request, clave, idioma, destinatario string, variables map[string]string) (models.CorreoSaliente, error) {
	plantilla, err := a.datos.PlantillaCorreo(r.Context(), clave, idioma)
	if err != nil {
		return models.CorreoSaliente{}, err
	}
	return views.RenderCorreo(plantilla, destinatario, variables), nil
}

// pintarFormulario repinta una página con formulario: los valores capturados
// (nunca las contraseñas) y los problemas encontrados.
func (a *App) pintarFormulario(w http.ResponseWriter, r *http.Request, estado int, plantilla, claveTitulo string,
	valores map[string]string, errores []models.ErrorCampo) {
	p := a.pagina(r, claveTitulo)
	p.Formulario = views.Formulario{Valores: valores, Errores: errores}
	a.pintar(w, r, estado, plantilla, p)
}

// mensaje muestra una página de resultado con un enlace para continuar.
func (a *App) mensaje(w http.ResponseWriter, r *http.Request, estado int, prefijo, enlaceURL, enlaceTexto string) {
	p := a.pagina(r, prefijo+"_titulo")
	p.Datos = struct{ Titulo, Texto, EnlaceURL, EnlaceTexto string }{
		prefijo + "_titulo", prefijo + "_texto", enlaceURL, enlaceTexto,
	}
	a.pintar(w, r, estado, "mensaje.html", p)
}

// enlaceInvalido responde a un enlace vencido, ya usado o inexistente. No se
// distingue entre los tres casos: no se da información a quien prueba enlaces.
func (a *App) enlaceInvalido(w http.ResponseWriter, r *http.Request) {
	a.mensaje(w, r, http.StatusGone, "mensaje.enlace", "/cuenta/recuperar", "mensaje.enlace_pedir")
}

// --- Registro (RF-AUT-01) -----------------------------------------------------

// registroFormulario muestra el formulario de autorregistro.
func (a *App) registroFormulario(w http.ResponseWriter, r *http.Request) {
	p := a.pagina(r, "cuenta.registro_titulo")
	// El idioma preferido se propone igual al de la interfaz en uso.
	p.Formulario.Valores = map[string]string{"idioma": p.Idioma}
	a.pintar(w, r, http.StatusOK, "cuenta_registro.html", p)
}

// registro crea la cuenta de un postulante, le envía el correo de verificación
// y lo deja con la sesión iniciada: puede empezar a capturar sin esperar el
// correo, aunque no podrá enviar hasta verificarlo (RF-AUT-02).
func (a *App) registro(w http.ResponseWriter, r *http.Request) {
	datos := models.DatosRegistro{
		Email:       models.NormalizarEmail(r.PostFormValue("email")),
		Nombre:      strings.TrimSpace(r.PostFormValue("nombre")),
		Apellidos:   strings.TrimSpace(r.PostFormValue("apellidos")),
		Password:    r.PostFormValue("password"),
		Idioma:      r.PostFormValue("idioma"),
		AceptaAviso: r.PostFormValue("acepta_aviso") == "1",
	}
	valores := map[string]string{
		"email": datos.Email, "nombre": datos.Nombre, "apellidos": datos.Apellidos, "idioma": datos.Idioma,
	}
	if datos.AceptaAviso {
		valores["acepta_aviso"] = "1"
	}
	rechazar := func(errores []models.ErrorCampo) {
		a.pintarFormulario(w, r, http.StatusUnprocessableEntity, "cuenta_registro.html", "cuenta.registro_titulo", valores, errores)
	}
	if errores := models.ValidarRegistro(datos); len(errores) > 0 {
		rechazar(errores)
		return
	}

	resumen, err := models.HashPassword(datos.Password)
	if err != nil {
		a.errorInterno(w, r, "resumir contraseña", err)
		return
	}
	claro, token, err := a.nuevoToken(models.TokenVerificarEmail)
	if err != nil {
		a.errorInterno(w, r, "generar enlace de verificación", err)
		return
	}
	correo, err := a.correo(r, models.PlantillaVerificarEmail, datos.Idioma, datos.Email, map[string]string{
		"nombre": datos.Nombre, "liga": a.liga("/cuenta/verificar", claro),
	})
	if err != nil {
		a.errorInterno(w, r, "armar correo de verificación", err)
		return
	}
	usuarioID, err := a.datos.CrearUsuario(r.Context(), a.actorDe(r), models.NuevoUsuario{
		Email: datos.Email, Nombre: datos.Nombre, Apellidos: datos.Apellidos, Idioma: datos.Idioma,
		PasswordHash: resumen, AvisoVersion: models.VersionAvisoPrivacidad, Perfil: models.PerfilPostulante,
	}, &token, correo)
	switch {
	case errors.Is(err, models.ErrEmailEnUso):
		// Se dice con claridad: quien postuló el año pasado suele no recordar
		// que ya tiene cuenta. El límite de intentos acota el sondeo de correos.
		rechazar([]models.ErrorCampo{{Campo: "email", Clave: "error.email_en_uso"}})
		return
	case err != nil:
		a.errorInterno(w, r, "crear cuenta", err)
		return
	}
	if err := a.iniciarSesion(w, r, usuarioID); err != nil {
		a.errorInterno(w, r, "iniciar sesión tras el registro", err)
		return
	}
	http.Redirect(w, r, "/cuenta?aviso=registro", http.StatusSeeOther)
}

// --- Verificación de correo (RF-AUT-02) -------------------------------------

// verificarFormulario recibe el enlace del correo. NO verifica nada: muestra
// un botón. La verificación es un POST, de modo que un cliente de correo o un
// antivirus que «visita» los enlaces no gasta el enlace de un solo uso.
func (a *App) verificarFormulario(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	usuario, err := a.datos.TokenVigente(r.Context(), models.ResumenDeSecreto(token), models.TokenVerificarEmail)
	switch {
	case errors.Is(err, models.ErrTokenInvalido):
		a.enlaceInvalido(w, r)
		return
	case err != nil:
		a.errorInterno(w, r, "leer enlace de verificación", err)
		return
	}
	p := a.pagina(r, "cuenta.verificar_titulo")
	p.Datos = struct{ Token, Email string }{token, usuario.Email}
	a.pintar(w, r, http.StatusOK, "cuenta_verificar.html", p)
}

// verificar consume el enlace y marca el correo como verificado.
func (a *App) verificar(w http.ResponseWriter, r *http.Request) {
	_, err := a.datos.VerificarEmailConToken(r.Context(), a.actorDe(r), models.ResumenDeSecreto(r.PostFormValue("token")))
	switch {
	case errors.Is(err, models.ErrTokenInvalido):
		a.enlaceInvalido(w, r)
		return
	case err != nil:
		a.errorInterno(w, r, "verificar correo", err)
		return
	}
	if sesionDe(r) != nil {
		http.Redirect(w, r, "/cuenta?aviso=verificado", http.StatusSeeOther)
		return
	}
	a.mensaje(w, r, http.StatusOK, "mensaje.verificado", "/cuenta/ingresar", "nav.ingresar")
}

// reenviarVerificacion envía de nuevo el correo de verificación a la cuenta
// en sesión, con una espera mínima entre correos.
func (a *App) reenviarVerificacion(w http.ResponseWriter, r *http.Request) {
	usuario := sesionDe(r).Usuario
	if usuario.EmailVerificado() {
		http.Redirect(w, r, "/cuenta", http.StatusSeeOther)
		return
	}
	ultima, err := a.datos.UltimaExpiracionDeToken(r.Context(), usuario.ID, models.TokenVerificarEmail)
	if err != nil {
		a.errorInterno(w, r, "leer enlaces de verificación", err)
		return
	}
	if !models.PuedeEmitirseOtroToken(models.TokenVerificarEmail, ultima, a.ahora()) {
		http.Redirect(w, r, "/cuenta?aviso=verificacion_reciente", http.StatusSeeOther)
		return
	}
	claro, token, err := a.nuevoToken(models.TokenVerificarEmail)
	if err != nil {
		a.errorInterno(w, r, "generar enlace de verificación", err)
		return
	}
	correo, err := a.correo(r, models.PlantillaVerificarEmail, usuario.Idioma, usuario.Email, map[string]string{
		"nombre": usuario.Nombre, "liga": a.liga("/cuenta/verificar", claro),
	})
	if err != nil {
		a.errorInterno(w, r, "armar correo de verificación", err)
		return
	}
	if err := a.datos.EmitirToken(r.Context(), a.actorDe(r), usuario.ID, token, correo); err != nil {
		a.errorInterno(w, r, "emitir enlace de verificación", err)
		return
	}
	http.Redirect(w, r, "/cuenta?aviso=verificacion_enviada", http.StatusSeeOther)
}

// --- Inicio y cierre de sesión (RF-AUT-03, 08 y 10) ------------------------

// ingresarFormulario muestra el formulario de inicio de sesión.
func (a *App) ingresarFormulario(w http.ResponseWriter, r *http.Request) {
	if sesionDe(r) != nil {
		http.Redirect(w, r, "/cuenta", http.StatusSeeOther)
		return
	}
	p := a.pagina(r, "cuenta.ingresar_titulo")
	p.Aviso = notaDe(r)
	p.Formulario.Valores = map[string]string{"volver": rutaLocal(r.URL.Query().Get("volver"))}
	a.pintar(w, r, http.StatusOK, "cuenta_ingresar.html", p)
}

// ingresar comprueba la credencial y abre la sesión.
//
// El mensaje de error es el mismo si el correo no existe, si la contraseña es
// incorrecta o si la cuenta está desactivada, y en los tres casos se hace el
// mismo trabajo de cómputo: ni el texto ni el tiempo de respuesta delatan qué
// correos tienen cuenta.
func (a *App) ingresar(w http.ResponseWriter, r *http.Request) {
	email := models.NormalizarEmail(r.PostFormValue("email"))
	password := r.PostFormValue("password")
	volver := rutaLocal(r.PostFormValue("volver"))
	valores := map[string]string{"email": email, "volver": volver}
	rechazar := func(estado int, errores []models.ErrorCampo) {
		a.pintarFormulario(w, r, estado, "cuenta_ingresar.html", "cuenta.ingresar_titulo", valores, errores)
	}
	if errores := models.ValidarIngreso(email, password); len(errores) > 0 {
		rechazar(http.StatusUnprocessableEntity, errores)
		return
	}
	credencialIncorrecta := []models.ErrorCampo{{Campo: "email", Clave: "error.credenciales"}}
	actor := a.actorDe(r)

	usuario, resumen, err := a.datos.UsuarioPorEmail(r.Context(), email)
	if errors.Is(err, models.ErrNoEncontrado) {
		models.GastarVerificacion(password)
		if err := a.datos.RegistrarIngresoFallido(r.Context(), actor, 0, false); err != nil {
			a.errorInterno(w, r, "registrar intento de ingreso", err)
			return
		}
		rechazar(http.StatusUnauthorized, credencialIncorrecta)
		return
	}
	if err != nil {
		a.errorInterno(w, r, "leer cuenta", err)
		return
	}

	// Bloqueo temporal por intentos fallidos (RF-AUT-08).
	fallos, ultimoFallo, err := a.datos.FallosDeIngreso(r.Context(), usuario.ID)
	if err != nil {
		a.errorInterno(w, r, "contar intentos fallidos", err)
		return
	}
	if a.ahora().Before(models.BloqueoDeIngreso(fallos, ultimoFallo)) {
		if err := a.datos.RegistrarIngresoFallido(r.Context(), actor, usuario.ID, true); err != nil {
			a.errorInterno(w, r, "registrar intento de ingreso", err)
			return
		}
		rechazar(http.StatusTooManyRequests, []models.ErrorCampo{{Campo: "email", Clave: "error.bloqueo"}})
		return
	}

	if !models.VerificarPassword(resumen, password) || !usuario.Activo {
		if err := a.datos.RegistrarIngresoFallido(r.Context(), actor, usuario.ID, false); err != nil {
			a.errorInterno(w, r, "registrar intento de ingreso", err)
			return
		}
		rechazar(http.StatusUnauthorized, credencialIncorrecta)
		return
	}

	if err := a.iniciarSesion(w, r, usuario.ID); err != nil {
		a.errorInterno(w, r, "iniciar sesión", err)
		return
	}
	if volver == "/" {
		volver = "/cuenta"
	}
	http.Redirect(w, r, volver, http.StatusSeeOther)
}

// salir cierra la sesión en curso.
func (a *App) salir(w http.ResponseWriter, r *http.Request) {
	if s := sesionDe(r); s != nil {
		if err := a.datos.CerrarSesion(r.Context(), a.actorDe(r), s.ID); err != nil {
			a.errorInterno(w, r, "cerrar sesión", err)
			return
		}
	}
	a.borrarCookieSesion(w, r)
	http.Redirect(w, r, "/cuenta/ingresar?aviso=salida", http.StatusSeeOther)
}

// --- Recuperación de contraseña y activación (RF-AUT-04 y 07) ---------------

// recuperarFormulario muestra la solicitud de recuperación.
func (a *App) recuperarFormulario(w http.ResponseWriter, r *http.Request) {
	a.pintar(w, r, http.StatusOK, "cuenta_recuperar.html", a.pagina(r, "cuenta.recuperar_titulo"))
}

// recuperar envía un enlace para elegir una contraseña nueva. La respuesta es
// SIEMPRE la misma, exista o no la cuenta.
func (a *App) recuperar(w http.ResponseWriter, r *http.Request) {
	email := models.NormalizarEmail(r.PostFormValue("email"))
	if errores := models.ValidarRecuperacion(email); len(errores) > 0 {
		a.pintarFormulario(w, r, http.StatusUnprocessableEntity, "cuenta_recuperar.html", "cuenta.recuperar_titulo",
			map[string]string{"email": email}, errores)
		return
	}
	usuario, _, err := a.datos.UsuarioPorEmail(r.Context(), email)
	switch {
	case errors.Is(err, models.ErrNoEncontrado):
		// Nada que enviar; se responde igual que si existiera.
	case err != nil:
		a.errorInterno(w, r, "leer cuenta", err)
		return
	case usuario.Activo:
		if err := a.enviarRestablecimiento(r, usuario); err != nil {
			a.errorInterno(w, r, "enviar enlace de recuperación", err)
			return
		}
	}
	a.mensaje(w, r, http.StatusOK, "mensaje.recuperar", "/cuenta/ingresar", "cuenta.volver_ingresar")
}

// enviarRestablecimiento emite el enlace de recuperación, salvo que ya se haya
// enviado uno hace muy poco: nadie debe poder inundar un buzón ajeno.
func (a *App) enviarRestablecimiento(r *http.Request, usuario models.Usuario) error {
	ultima, err := a.datos.UltimaExpiracionDeToken(r.Context(), usuario.ID, models.TokenResetPassword)
	if err != nil {
		return err
	}
	if !models.PuedeEmitirseOtroToken(models.TokenResetPassword, ultima, a.ahora()) {
		return nil
	}
	claro, token, err := a.nuevoToken(models.TokenResetPassword)
	if err != nil {
		return err
	}
	correo, err := a.correo(r, models.PlantillaResetPassword, usuario.Idioma, usuario.Email, map[string]string{
		"nombre": usuario.Nombre, "liga": a.liga("/cuenta/restablecer", claro),
	})
	if err != nil {
		return err
	}
	return a.datos.EmitirToken(r.Context(), a.actorDe(r), usuario.ID, token, correo)
}

// vistaPasswordNueva es el modelo de vista de la página para elegir contraseña.
type vistaPasswordNueva struct {
	Accion, Titulo, Entrada, Token, Email string
}

// paginaPasswordNueva devuelve los textos y la ruta según el propósito del
// enlace: restablecer una contraseña olvidada o activar una cuenta interna.
func paginaPasswordNueva(proposito models.PropositoToken) vistaPasswordNueva {
	if proposito == models.TokenActivacion {
		return vistaPasswordNueva{Accion: "/cuenta/activar", Titulo: "cuenta.activar_titulo", Entrada: "cuenta.activar_entrada"}
	}
	return vistaPasswordNueva{Accion: "/cuenta/restablecer", Titulo: "cuenta.restablecer_titulo", Entrada: "cuenta.restablecer_entrada"}
}

// pintarPasswordNueva muestra el formulario para elegir contraseña, si el
// enlace sigue vigente.
func (a *App) pintarPasswordNueva(w http.ResponseWriter, r *http.Request, estado int,
	proposito models.PropositoToken, token string, errores []models.ErrorCampo) {
	usuario, err := a.datos.TokenVigente(r.Context(), models.ResumenDeSecreto(token), proposito)
	switch {
	case errors.Is(err, models.ErrTokenInvalido):
		a.enlaceInvalido(w, r)
		return
	case err != nil:
		a.errorInterno(w, r, "leer enlace", err)
		return
	}
	vista := paginaPasswordNueva(proposito)
	vista.Token, vista.Email = token, usuario.Email
	p := a.pagina(r, vista.Titulo)
	p.Formulario.Errores = errores
	p.Datos = vista
	a.pintar(w, r, estado, "cuenta_password_nueva.html", p)
}

// passwordNuevaFormulario recibe el enlace del correo y muestra el formulario.
func (a *App) passwordNuevaFormulario(proposito models.PropositoToken) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.pintarPasswordNueva(w, r, http.StatusOK, proposito, r.URL.Query().Get("token"), nil)
	}
}

// passwordNueva consume el enlace y guarda la contraseña elegida. Cierra todas
// las sesiones de la cuenta; quien lo hizo debe ingresar con la nueva.
func (a *App) passwordNueva(proposito models.PropositoToken) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, password := r.PostFormValue("token"), r.PostFormValue("password")
		if errores := models.ValidarPasswordNueva("password", password); len(errores) > 0 {
			a.pintarPasswordNueva(w, r, http.StatusUnprocessableEntity, proposito, token, errores)
			return
		}
		resumen, err := models.HashPassword(password)
		if err != nil {
			a.errorInterno(w, r, "resumir contraseña", err)
			return
		}
		_, err = a.datos.DefinirPasswordConToken(r.Context(), a.actorDe(r), models.ResumenDeSecreto(token), proposito, resumen)
		switch {
		case errors.Is(err, models.ErrTokenInvalido):
			a.enlaceInvalido(w, r)
			return
		case err != nil:
			a.errorInterno(w, r, "definir contraseña", err)
			return
		}
		// Si quien restablece tenía una sesión abierta, también se cerró.
		a.borrarCookieSesion(w, r)
		nota := "password_restablecida"
		if proposito == models.TokenActivacion {
			nota = "activada"
		}
		http.Redirect(w, r, "/cuenta/ingresar?aviso="+nota, http.StatusSeeOther)
	}
}

// --- Mi cuenta (RF-AUT-06) ---------------------------------------------------

// pintarMiCuenta muestra la página de la cuenta. Sin valores capturados, los
// campos se llenan con los datos actuales.
func (a *App) pintarMiCuenta(w http.ResponseWriter, r *http.Request, estado int,
	valores map[string]string, errores []models.ErrorCampo) {
	usuario := sesionDe(r).Usuario
	completos := map[string]string{"nombre": usuario.Nombre, "apellidos": usuario.Apellidos, "idioma": usuario.Idioma}
	for campo, valor := range valores {
		completos[campo] = valor
	}
	p := a.pagina(r, "cuenta.mi_cuenta")
	p.Aviso = notaDe(r)
	p.Formulario = views.Formulario{Valores: completos, Errores: errores}
	a.pintar(w, r, estado, "cuenta.html", p)
}

// miCuenta muestra los datos de la cuenta en sesión.
func (a *App) miCuenta(w http.ResponseWriter, r *http.Request) {
	a.pintarMiCuenta(w, r, http.StatusOK, nil, nil)
}

// guardarDatos cambia nombre, apellidos e idioma preferido.
func (a *App) guardarDatos(w http.ResponseWriter, r *http.Request) {
	nombre := strings.TrimSpace(r.PostFormValue("nombre"))
	apellidos := strings.TrimSpace(r.PostFormValue("apellidos"))
	idioma := r.PostFormValue("idioma")
	if errores := models.ValidarDatosPersonales(nombre, apellidos, idioma); len(errores) > 0 {
		a.pintarMiCuenta(w, r, http.StatusUnprocessableEntity,
			map[string]string{"nombre": nombre, "apellidos": apellidos, "idioma": idioma}, errores)
		return
	}
	if err := a.datos.ActualizarDatosPersonales(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, nombre, apellidos, idioma); err != nil {
		a.errorInterno(w, r, "guardar datos personales", err)
		return
	}
	http.Redirect(w, r, "/cuenta?aviso=datos", http.StatusSeeOther)
}

// passwordActualCorrecta comprueba la contraseña de la cuenta en sesión antes
// de un cambio sensible: tener la sesión abierta no basta.
func (a *App) passwordActualCorrecta(r *http.Request, password string) (bool, error) {
	resumen, err := a.datos.ResumenDePassword(r.Context(), sesionDe(r).Usuario.ID)
	if err != nil {
		return false, err
	}
	return models.VerificarPassword(resumen, password), nil
}

// cambiarPassword cambia la contraseña y cierra las demás sesiones.
func (a *App) cambiarPassword(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	correcta, err := a.passwordActualCorrecta(r, r.PostFormValue("password_actual"))
	if err != nil {
		a.errorInterno(w, r, "comprobar contraseña actual", err)
		return
	}
	var errores []models.ErrorCampo
	if !correcta {
		errores = append(errores, models.ErrorCampo{Campo: "password_actual", Clave: "error.password_actual"})
	}
	nueva := r.PostFormValue("password_nueva")
	errores = append(errores, models.ValidarPasswordNueva("password_nueva", nueva)...)
	if len(errores) > 0 {
		a.pintarMiCuenta(w, r, http.StatusUnprocessableEntity, nil, errores)
		return
	}
	resumen, err := models.HashPassword(nueva)
	if err != nil {
		a.errorInterno(w, r, "resumir contraseña", err)
		return
	}
	if err := a.datos.CambiarPassword(r.Context(), a.actorDe(r), sesion.Usuario.ID, resumen, sesion.ID); err != nil {
		a.errorInterno(w, r, "cambiar contraseña", err)
		return
	}
	http.Redirect(w, r, "/cuenta?aviso=password", http.StatusSeeOther)
}

// cambiarEmail cambia el correo de la cuenta. El correo nuevo queda sin
// verificar; se le envía su enlace y se avisa del cambio a la dirección
// anterior, para que su dueño se entere si no fue él.
func (a *App) cambiarEmail(w http.ResponseWriter, r *http.Request) {
	usuario := sesionDe(r).Usuario
	nuevo := models.NormalizarEmail(r.PostFormValue("email_nuevo"))
	valores := map[string]string{"email_nuevo": nuevo}
	rechazar := func(errores []models.ErrorCampo) {
		a.pintarMiCuenta(w, r, http.StatusUnprocessableEntity, valores, errores)
	}

	var errores []models.ErrorCampo
	switch {
	case !models.EmailValido(nuevo):
		errores = append(errores, models.ErrorCampo{Campo: "email_nuevo", Clave: "error.email_invalido"})
	case strings.EqualFold(nuevo, usuario.Email):
		errores = append(errores, models.ErrorCampo{Campo: "email_nuevo", Clave: "error.email_igual"})
	}
	correcta, err := a.passwordActualCorrecta(r, r.PostFormValue("password_email"))
	if err != nil {
		a.errorInterno(w, r, "comprobar contraseña actual", err)
		return
	}
	if !correcta {
		errores = append(errores, models.ErrorCampo{Campo: "password_email", Clave: "error.password_actual"})
	}
	if len(errores) > 0 {
		rechazar(errores)
		return
	}

	claro, token, err := a.nuevoToken(models.TokenVerificarEmail)
	if err != nil {
		a.errorInterno(w, r, "generar enlace de verificación", err)
		return
	}
	verificacion, err := a.correo(r, models.PlantillaVerificarEmail, usuario.Idioma, nuevo, map[string]string{
		"nombre": usuario.Nombre, "liga": a.liga("/cuenta/verificar", claro),
	})
	if err != nil {
		a.errorInterno(w, r, "armar correo de verificación", err)
		return
	}
	aviso, err := a.correo(r, models.PlantillaCambioEmail, usuario.Idioma, usuario.Email, map[string]string{
		"nombre": usuario.Nombre, "email_nuevo": nuevo,
	})
	if err != nil {
		a.errorInterno(w, r, "armar aviso de cambio de correo", err)
		return
	}
	err = a.datos.CambiarEmail(r.Context(), a.actorDe(r), usuario.ID, nuevo, token, verificacion, aviso)
	switch {
	case errors.Is(err, models.ErrEmailEnUso):
		rechazar([]models.ErrorCampo{{Campo: "email_nuevo", Clave: "error.email_en_uso"}})
		return
	case err != nil:
		a.errorInterno(w, r, "cambiar correo", err)
		return
	}
	http.Redirect(w, r, "/cuenta?aviso=email", http.StatusSeeOther)
}
