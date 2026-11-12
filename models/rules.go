// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"net/mail"
	"strings"
	"time"
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

	errores = append(errores, ValidarPasswordNueva("password", d.Password)...)

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

// ---------------------------------------------------------------------------
// Cuentas, sesiones y control de acceso
// ---------------------------------------------------------------------------

// VersionAvisoPrivacidad identifica el texto del aviso que acepta quien se
// registra; se guarda con el sello de tiempo del consentimiento (LFPDPPP).
// Mientras la CNMO no entregue el texto oficial, la versión es provisional:
// así podrá pedirse de nuevo el consentimiento cuando exista el definitivo.
const VersionAvisoPrivacidad = "provisional"

// Vigencia de los enlaces de un solo uso (RF-AUT-02 y RF-AUT-04).
const (
	VigenciaVerificacion = 24 * time.Hour
	VigenciaRestablecer  = time.Hour
	VigenciaActivacion   = 7 * 24 * time.Hour
)

// EsperaEntreCorreos es el tiempo mínimo entre dos correos del mismo tipo a
// una misma cuenta. Impide usar la plataforma para inundar un buzón ajeno.
const EsperaEntreCorreos = 5 * time.Minute

// Expiración de la sesión por inactividad (RF-AUT-10).
const (
	SesionInterna    = 8 * time.Hour
	SesionPostulante = 30 * 24 * time.Hour
	// RenovacionSesion es cada cuánto se prolonga una sesión en uso. Renovar
	// en cada petición sería una escritura por clic; con esto basta.
	RenovacionSesion = 10 * time.Minute
)

// Bloqueo temporal por intentos fallidos (RF-AUT-08).
const (
	IntentosAntesDeBloqueo = 5
	BloqueoBase            = time.Minute
	BloqueoMaximo          = 30 * time.Minute
)

// VigenciaDeToken devuelve cuánto vale un enlace según su propósito.
func VigenciaDeToken(proposito PropositoToken) time.Duration {
	switch proposito {
	case TokenResetPassword:
		return VigenciaRestablecer
	case TokenActivacion:
		return VigenciaActivacion
	default:
		return VigenciaVerificacion
	}
}

// PuedeEmitirseOtroToken indica si ya pasó la espera mínima desde el último
// enlace del mismo propósito. ultimaExpiracion es el vencimiento del enlace
// más reciente (cero si nunca hubo): su emisión fue una vigencia antes.
func PuedeEmitirseOtroToken(proposito PropositoToken, ultimaExpiracion, ahora time.Time) bool {
	if ultimaExpiracion.IsZero() {
		return true
	}
	emitido := ultimaExpiracion.Add(-VigenciaDeToken(proposito))
	return !ahora.Before(emitido.Add(EsperaEntreCorreos))
}

// DuracionSesion devuelve la expiración por inactividad que corresponde a una
// cuenta: 8 horas si tiene cualquier perfil interno (administración,
// coordinación, jurado o revisor) y 30 días si sólo es postulante.
func DuracionSesion(perfiles []Perfil, asignaciones []Asignacion) time.Duration {
	if len(asignaciones) > 0 {
		return SesionInterna
	}
	for _, p := range perfiles {
		if p == PerfilAdminSistema || p == PerfilCoordinacionNacional {
			return SesionInterna
		}
	}
	return SesionPostulante
}

// BloqueoDeIngreso calcula hasta cuándo queda bloqueado el inicio de sesión de
// una cuenta, dados sus fallos consecutivos y el momento del último. Devuelve
// el valor cero si no hay bloqueo. Retroceso exponencial: 1 minuto al quinto
// fallo, 2 al sexto, 4 al séptimo… con tope de 30 minutos, para que nadie
// pueda dejar fuera indefinidamente a otra persona equivocándose a propósito.
func BloqueoDeIngreso(fallos int, ultimoFallo time.Time) time.Time {
	if fallos < IntentosAntesDeBloqueo {
		return time.Time{}
	}
	duracion := BloqueoMaximo
	if exceso := fallos - IntentosAntesDeBloqueo; exceso < 5 { // 1<<5 minutos ya rebasa el tope
		duracion = min(BloqueoBase<<exceso, BloqueoMaximo)
	}
	return ultimoFallo.Add(duracion)
}

// ValidarPasswordNueva aplica la política de contraseñas a un campo dado.
func ValidarPasswordNueva(campo, password string) []ErrorCampo {
	// La longitud se mide en caracteres, no en bytes, para no penalizar a
	// quien usa acentos o alfabetos no latinos.
	switch n := utf8.RuneCountInString(password); {
	case n == 0:
		return []ErrorCampo{{Campo: campo, Clave: "error.password_requerida"}}
	case n < LongitudMinPassword:
		return []ErrorCampo{{Campo: campo, Clave: "error.password_corta"}}
	case n > LongitudMaxPassword:
		return []ErrorCampo{{Campo: campo, Clave: "error.password_larga"}}
	}
	return nil
}

// ValidarDatosPersonales valida lo que el usuario puede cambiar de su cuenta
// sin más trámite: nombre, apellidos e idioma (RF-AUT-06).
func ValidarDatosPersonales(nombre, apellidos, idioma string) []ErrorCampo {
	var errores []ErrorCampo
	switch {
	case strings.TrimSpace(nombre) == "":
		errores = append(errores, ErrorCampo{Campo: "nombre", Clave: "error.nombre_requerido"})
	case utf8.RuneCountInString(nombre) > longitudMaxNombre:
		errores = append(errores, ErrorCampo{Campo: "nombre", Clave: "error.texto_largo"})
	}
	switch {
	case strings.TrimSpace(apellidos) == "":
		errores = append(errores, ErrorCampo{Campo: "apellidos", Clave: "error.apellidos_requeridos"})
	case utf8.RuneCountInString(apellidos) > longitudMaxNombre:
		errores = append(errores, ErrorCampo{Campo: "apellidos", Clave: "error.texto_largo"})
	}
	if !IdiomaValido(idioma) {
		errores = append(errores, ErrorCampo{Campo: "idioma", Clave: "error.idioma_invalido"})
	}
	return errores
}

// PuedeConsultarAuditoria: la bitácora la consultan el administrador y el
// observador (matriz de permisos, §2.2). Nadie más.
func PuedeConsultarAuditoria(s *Sesion) bool {
	return s != nil && (s.TienePerfil(PerfilAdminSistema) || s.TienePerfil(PerfilCoordinacionNacional))
}

// PerfilGlobalAsignable indica si un perfil puede otorgarse al crear una
// cuenta interna. «postulante» no: ése sólo nace del autorregistro.
func PerfilGlobalAsignable(p Perfil) bool {
	return p == PerfilAdminSistema || p == PerfilCoordinacionNacional
}

// Reintentos del correo saliente.
const (
	IntentosMaxCorreo = 8
	reintentoBase     = time.Minute
	reintentoMaximo   = 6 * time.Hour
)

// ReintentoDeCorreo decide qué hacer tras un envío fallido. intentos es el
// número de intentos ya hechos, contando el que acaba de fallar. Devuelve
// cuándo reintentar, o false si se agotaron los intentos y el mensaje debe
// marcarse en error para atención del administrador.
func ReintentoDeCorreo(intentos int, ahora time.Time) (time.Time, bool) {
	if intentos >= IntentosMaxCorreo {
		return time.Time{}, false
	}
	espera := reintentoMaximo
	if intentos < 9 { // 1<<9 minutos ya rebasa el tope
		espera = min(reintentoBase<<intentos, reintentoMaximo)
	}
	return ahora.Add(espera), true
}

// PlantillaConEnlaceSecreto indica si los correos de una plantilla llevan un
// enlace de un solo uso. Su cuerpo se retira de la bandeja de salida en cuanto
// se envían: un enlace vigente no debe quedar guardado en claro en la base.
func PlantillaConEnlaceSecreto(clave string) bool {
	return clave == PlantillaVerificarEmail || clave == PlantillaResetPassword || clave == PlantillaActivacion
}
