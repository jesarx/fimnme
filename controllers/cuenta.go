// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package controllers

import (
	"net/http"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pantallas de cuenta: registro, inicio de sesión y recuperación de contraseña.
//
// ESTADO ACTUAL: los formularios validan con las reglas de models/rules.go y
// devuelven los errores campo por campo, pero todavía NO crean cuentas ni
// sesiones. Tras una validación correcta responden 501 con un aviso explícito,
// para que nadie crea que quedó registrado. La conexión real llega en el paso
// 3 del plan, «Cuentas, sesiones y control de acceso»
// (docs/PLAN_DE_DESARROLLO.md).

// avisoPendiente es el mensaje que se muestra mientras no hay cuentas.
var avisoPendiente = &views.Aviso{Tipo: "info", Clave: "cuenta.pendiente"}

// responderFormulario repinta un formulario tras recibirlo: con los errores
// (422) o, si los datos son válidos, con el aviso de función pendiente (501).
func (a *App) responderFormulario(w http.ResponseWriter, r *http.Request, plantilla, claveTitulo string,
	valores map[string]string, errores []models.ErrorCampo) {
	p := a.pagina(r, claveTitulo)
	p.Formulario = views.Formulario{Valores: valores, Errores: errores}
	estado := http.StatusUnprocessableEntity
	if len(errores) == 0 {
		p.Aviso = avisoPendiente
		estado = http.StatusNotImplemented
	}
	a.pintar(w, r, estado, plantilla, p)
}

// registroFormulario muestra el formulario de autorregistro (RF-AUT-01).
func (a *App) registroFormulario(w http.ResponseWriter, r *http.Request) {
	p := a.pagina(r, "cuenta.registro_titulo")
	// El idioma preferido se propone igual al de la interfaz en uso.
	p.Formulario.Valores = map[string]string{"idioma": p.Idioma}
	a.pintar(w, r, http.StatusOK, "cuenta_registro.html", p)
}

// registro recibe el formulario de autorregistro y lo valida.
func (a *App) registro(w http.ResponseWriter, r *http.Request) {
	datos := models.DatosRegistro{
		Email:       models.NormalizarEmail(r.PostFormValue("email")),
		Nombre:      r.PostFormValue("nombre"),
		Apellidos:   r.PostFormValue("apellidos"),
		Password:    r.PostFormValue("password"),
		Idioma:      r.PostFormValue("idioma"),
		AceptaAviso: r.PostFormValue("acepta_aviso") == "1",
	}
	// La contraseña NO se incluye entre los valores que vuelven a la página.
	valores := map[string]string{
		"email":     datos.Email,
		"nombre":    datos.Nombre,
		"apellidos": datos.Apellidos,
		"idioma":    datos.Idioma,
	}
	if datos.AceptaAviso {
		valores["acepta_aviso"] = "1"
	}
	a.responderFormulario(w, r, "cuenta_registro.html", "cuenta.registro_titulo",
		valores, models.ValidarRegistro(datos))
}

// ingresarFormulario muestra el formulario de inicio de sesión (RF-AUT-03).
func (a *App) ingresarFormulario(w http.ResponseWriter, r *http.Request) {
	a.pintar(w, r, http.StatusOK, "cuenta_ingresar.html", a.pagina(r, "cuenta.ingresar_titulo"))
}

// ingresar recibe el formulario de inicio de sesión y lo valida.
func (a *App) ingresar(w http.ResponseWriter, r *http.Request) {
	email := models.NormalizarEmail(r.PostFormValue("email"))
	a.responderFormulario(w, r, "cuenta_ingresar.html", "cuenta.ingresar_titulo",
		map[string]string{"email": email},
		models.ValidarIngreso(email, r.PostFormValue("password")))
}

// recuperarFormulario muestra la solicitud de recuperación (RF-AUT-04).
func (a *App) recuperarFormulario(w http.ResponseWriter, r *http.Request) {
	a.pintar(w, r, http.StatusOK, "cuenta_recuperar.html", a.pagina(r, "cuenta.recuperar_titulo"))
}

// recuperar recibe la solicitud de recuperación de contraseña y la valida.
func (a *App) recuperar(w http.ResponseWriter, r *http.Request) {
	email := models.NormalizarEmail(r.PostFormValue("email"))
	a.responderFormulario(w, r, "cuenta_recuperar.html", "cuenta.recuperar_titulo",
		map[string]string{"email": email}, models.ValidarRecuperacion(email))
}
