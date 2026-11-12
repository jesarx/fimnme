// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Este archivo concentra las reglas de negocio. La validación existe UNA sola
// vez, aquí: la interfaz no duplica reglas que puedan desincronizarse (§7.4 de
// la especificación). Los controladores sólo orquestan y las vistas sólo pintan.

// Límites de los datos de cuenta.
const (
	// LongitudMinPassword privilegia la longitud sobre las reglas de
	// composición, que empujan a contraseñas predecibles.
	LongitudMinPassword = 10
	// LongitudMaxPassword acota el costo de Argon2id ante entradas abusivas.
	LongitudMaxPassword = 128
	longitudMaxEmail    = 254
	longitudMaxNombre   = 100
)

// ConvocatoriaVisibleAlPublico indica si las bases de una convocatoria pueden
// mostrarse sin sesión. En BORRADOR el postulante no tiene acceso alguno (§3).
func ConvocatoriaVisibleAlPublico(c Convocatoria) bool {
	return c.Estado != EstadoBorrador
}

// ConvocatoriaVigente indica si la convocatoria pertenece al ciclo en curso y
// por tanto se anuncia en la portada. Las archivadas siguen consultables por
// su enlace permanente, pero ya no se anuncian.
func ConvocatoriaVigente(c Convocatoria) bool {
	return ConvocatoriaVisibleAlPublico(c) && c.Estado != EstadoArchivada
}

// AdmitePostulaciones indica si puede crearse o editarse una postulación por
// etapa (primera mitad de RN-02; la subsanación se resolverá por separado).
func AdmitePostulaciones(c Convocatoria) bool {
	return c.Estado == EstadoPostulacion
}

// NormalizarEmail recorta espacios. La comparación insensible a mayúsculas la
// resuelve la base de datos (COLLATE NOCASE), por lo que no se altera la caja.
func NormalizarEmail(email string) string {
	return strings.TrimSpace(email)
}

// EmailValido comprueba que el texto sea una dirección de correo sola (sin
// nombre para mostrar) y con dominio. La prueba real de que el buzón existe es
// el enlace de verificación (RF-AUT-02), no esta sintaxis.
func EmailValido(email string) bool {
	if email == "" || len(email) > longitudMaxEmail {
		return false
	}
	dir, err := mail.ParseAddress(email)
	if err != nil || dir.Address != email {
		return false
	}
	_, dominio, ok := strings.Cut(email, "@")
	return ok && strings.Contains(dominio, ".") && !strings.HasSuffix(dominio, ".")
}

// IdiomaValido informa si el idioma es uno de los dos de la plataforma.
func IdiomaValido(idioma string) bool {
	return idioma == IdiomaES || idioma == IdiomaEN
}

// ValidarRegistro aplica las reglas del autorregistro (RF-AUT-01) y devuelve la
// lista de problemas, vacía si los datos son aceptables. El orden de la lista
// es el orden de los campos en el formulario.
func ValidarRegistro(d DatosRegistro) []ErrorCampo {
	var errores []ErrorCampo
	agregar := func(campo, clave string) {
		errores = append(errores, ErrorCampo{Campo: campo, Clave: clave})
	}

	switch {
	case d.Email == "":
		agregar("email", "error.email_requerido")
	case !EmailValido(d.Email):
		agregar("email", "error.email_invalido")
	}

	switch {
	case strings.TrimSpace(d.Nombre) == "":
		agregar("nombre", "error.nombre_requerido")
	case utf8.RuneCountInString(d.Nombre) > longitudMaxNombre:
		agregar("nombre", "error.texto_largo")
	}

	switch {
	case strings.TrimSpace(d.Apellidos) == "":
		agregar("apellidos", "error.apellidos_requeridos")
	case utf8.RuneCountInString(d.Apellidos) > longitudMaxNombre:
		agregar("apellidos", "error.texto_largo")
	}

	// La longitud se mide en caracteres, no en bytes, para no penalizar a
	// quien usa acentos o alfabetos no latinos.
	switch n := utf8.RuneCountInString(d.Password); {
	case n == 0:
		agregar("password", "error.password_requerida")
	case n < LongitudMinPassword:
		agregar("password", "error.password_corta")
	case n > LongitudMaxPassword:
		agregar("password", "error.password_larga")
	}

	if !IdiomaValido(d.Idioma) {
		agregar("idioma", "error.idioma_invalido")
	}

	// Sin consentimiento registrado no puede existir la cuenta (LFPDPPP).
	if !d.AceptaAviso {
		agregar("acepta_aviso", "error.aviso_requerido")
	}
	return errores
}

// ValidarIngreso comprueba que el formulario de inicio de sesión venga
// completo. La verificación de la credencial corresponde a la persistencia.
func ValidarIngreso(email, password string) []ErrorCampo {
	var errores []ErrorCampo
	switch {
	case email == "":
		errores = append(errores, ErrorCampo{Campo: "email", Clave: "error.email_requerido"})
	case !EmailValido(email):
		errores = append(errores, ErrorCampo{Campo: "email", Clave: "error.email_invalido"})
	}
	if password == "" {
		errores = append(errores, ErrorCampo{Campo: "password", Clave: "error.password_requerida"})
	}
	return errores
}

// ValidarRecuperacion comprueba el correo de una solicitud de recuperación de
// contraseña (RF-AUT-04).
func ValidarRecuperacion(email string) []ErrorCampo {
	switch {
	case email == "":
		return []ErrorCampo{{Campo: "email", Clave: "error.email_requerido"}}
	case !EmailValido(email):
		return []ErrorCampo{{Campo: "email", Clave: "error.email_invalido"}}
	}
	return nil
}
