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

package models

import (
	"fmt"
	"maps"
	"math"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
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

// ---------------------------------------------------------------------------
// Configuración de ediciones y convocatorias (RF-CONV, §3, RN-14)
// ---------------------------------------------------------------------------

// Límites de la configuración.
const (
	// MaxVideosPermitidos es el tope de ligas de video por postulación (D-10).
	MaxVideosPermitidos = 3
	// MaxObrasCompositores es el máximo de obras por omisión en Compositores (D-11).
	MaxObrasCompositores = 2
	// TamanoMaxArchivo es el tope absoluto de un documento, sea cual sea el
	// requisito: ningún archivo de la convocatoria debe ser mayor.
	TamanoMaxArchivo = 200 << 20 // 200 MiB
	// MaxArchivosPorRequisito acota cuántos archivos admite un requisito.
	MaxArchivosPorRequisito = 20
	// CuotaJurados es el número EXACTO de jurados por convocatoria (D-07).
	CuotaJurados = 3

	longitudMaxTitulo = 300
	longitudMaxTexto  = 200
)

// MimesAdmitidos son los formatos que la plataforma sabe reconocer por la
// firma del archivo (RF-DOC-01). Un requisito sólo puede pedir estos.
var MimesAdmitidos = []string{
	"image/jpeg", "image/png", "application/pdf", "audio/wav", "audio/flac", "audio/mpeg",
}

var claveValida = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// ordenEstados es el ciclo de vida de una convocatoria, en orden (§3).
var ordenEstados = []EstadoConvocatoria{
	EstadoBorrador, EstadoPublicada, EstadoPostulacion, EstadoRevisionDocumental,
	EstadoEvaluacion, EstadoCerrada, EstadoArchivada,
}

// ordenEtapas es el orden cronológico obligado del calendario.
var ordenEtapas = []ClaveEtapa{EtapaPostulacion, EtapaRevisionDocumental, EtapaEvaluacion}

// posicionEstado devuelve el lugar de un estado en el ciclo, o -1 si no existe.
func posicionEstado(estado EstadoConvocatoria) int {
	return slices.Index(ordenEstados, estado)
}

// EstadoSiguiente devuelve el estado que sigue en el ciclo de vida.
func EstadoSiguiente(estado EstadoConvocatoria) (EstadoConvocatoria, bool) {
	i := posicionEstado(estado)
	if i < 0 || i+1 >= len(ordenEstados) {
		return "", false
	}
	return ordenEstados[i+1], true
}

// requerido añade el error de dato obligatorio o demasiado largo de un texto.
func requerido(errores []ErrorCampo, campo, valor string, maximo int) []ErrorCampo {
	switch {
	case strings.TrimSpace(valor) == "":
		return append(errores, ErrorCampo{Campo: campo, Clave: "error.requerido"})
	case utf8.RuneCountInString(valor) > maximo:
		return append(errores, ErrorCampo{Campo: campo, Clave: "error.texto_largo_campo"})
	}
	return errores
}

// ValidarEdicion valida los datos de una edición (RF-CONV-01). El nombre en
// inglés puede faltar: se mostrará el español (RF-I18N-04).
func ValidarEdicion(e Edicion) []ErrorCampo {
	var errores []ErrorCampo
	if e.Numero < 1 {
		errores = append(errores, ErrorCampo{Campo: "numero", Clave: "error.edicion_numero"})
	}
	if e.Anio < 1900 || e.Anio > 2200 {
		errores = append(errores, ErrorCampo{Campo: "anio", Clave: "error.edicion_anio"})
	}
	return requerido(errores, "nombre_es", e.NombreES, longitudMaxTexto)
}

// ValidarConvocatoria valida textos y límites de una convocatoria (RF-CONV-02
// y 08). Las bases pueden estar vacías mientras es borrador; se exigen al
// publicar.
func ValidarConvocatoria(c Convocatoria) []ErrorCampo {
	var errores []ErrorCampo
	if !c.Tipo.Valido() {
		errores = append(errores, ErrorCampo{Campo: "tipo", Clave: "error.tipo_invalido"})
	}
	errores = requerido(errores, "titulo_es", c.TituloES, longitudMaxTitulo)
	if c.MaxObras < 0 {
		errores = append(errores, ErrorCampo{Campo: "max_obras", Clave: "error.limite_invalido"})
	} else if c.Tipo == TipoCompositores && c.MaxObras == 0 {
		// En Compositores el máximo de obras es una regla del concurso, no un
		// ajuste opcional: «sin límite» no es un valor admisible.
		errores = append(errores, ErrorCampo{Campo: "max_obras", Clave: "error.max_obras_requerido"})
	}
	if c.DuracionMaxProgramaMin < 0 {
		errores = append(errores, ErrorCampo{Campo: "duracion_max_programa_min", Clave: "error.limite_invalido"})
	}
	if c.MaxVideos < 0 || c.MaxVideos > MaxVideosPermitidos {
		errores = append(errores, ErrorCampo{Campo: "max_videos", Clave: "error.max_videos"})
	}
	return errores
}

// ValidarEtapas valida un calendario (RF-CONV-03). Puede estar incompleto
// mientras la convocatoria es borrador, pero lo que haya debe ser coherente:
// claves válidas y sin repetir, cada etapa termina después de empezar, y se
// siguen en el tiempo sin traslaparse.
func ValidarEtapas(etapas []Etapa) []ErrorCampo {
	porClave := make(map[ClaveEtapa]Etapa, len(etapas))
	for _, e := range etapas {
		if _, repetida := porClave[e.Clave]; repetida || !slices.Contains(ordenEtapas, e.Clave) {
			return []ErrorCampo{{Campo: "etapas", Clave: "error.etapa_clave"}}
		}
		if !e.TerminaAt.After(e.IniciaAt) {
			return []ErrorCampo{{Campo: "etapas", Clave: "error.etapa_rango"}}
		}
		porClave[e.Clave] = e
	}
	var anterior *Etapa
	for _, clave := range ordenEtapas {
		etapa, ok := porClave[clave]
		if !ok {
			continue
		}
		if anterior != nil && etapa.IniciaAt.Before(anterior.TerminaAt) {
			return []ErrorCampo{{Campo: "etapas", Clave: "error.etapa_orden"}}
		}
		anterior = &etapa
	}
	return nil
}

// ValidarEdicionDeConfiguracion: la configuración de una convocatoria cerrada
// o archivada ya no se toca. En el archivo todo es de sólo lectura, también
// para el administrador (RF-ARCH-03); modificarla exige desarchivarla.
func ValidarEdicionDeConfiguracion(estado EstadoConvocatoria) []ErrorCampo {
	if estado == EstadoCerrada || estado == EstadoArchivada {
		return []ErrorCampo{{Campo: "estado", Clave: "error.convocatoria_cerrada"}}
	}
	return nil
}

// ValidarEdicionDeCriterios: cambiar los criterios con la evaluación en marcha
// dejaría puntajes que ya no corresponden a lo que se califica.
func ValidarEdicionDeCriterios(estado EstadoConvocatoria) []ErrorCampo {
	if posicionEstado(estado) >= posicionEstado(EstadoEvaluacion) {
		return []ErrorCampo{{Campo: "estado", Clave: "error.criterios_en_evaluacion"}}
	}
	return nil
}

// ValidarBorradoDeConfiguracion: un requisito o un criterio sólo se elimina
// mientras la convocatoria es borrador, cuando nada depende aún de él.
// Después se edita; no se borra (RN-22).
func ValidarBorradoDeConfiguracion(estado EstadoConvocatoria) []ErrorCampo {
	if estado != EstadoBorrador {
		return []ErrorCampo{{Campo: "estado", Clave: "error.borrado_solo_borrador"}}
	}
	return nil
}

// ValidarDotacion valida una dotación del catálogo anual (RF-CONV-06).
func ValidarDotacion(d Dotacion, tipo TipoConvocatoria) []ErrorCampo {
	if tipo != TipoCompositores {
		return []ErrorCampo{{Campo: "tipo", Clave: "error.dotacion_solo_compositores"}}
	}
	errores := requerido(nil, "nombre_es", d.NombreES, longitudMaxTexto)
	if d.Orden < 0 {
		errores = append(errores, ErrorCampo{Campo: "orden", Clave: "error.limite_invalido"})
	}
	return errores
}

// ValidarRequisito valida un requisito documental (RF-CONV-07) contra la
// configuración de su convocatoria.
func ValidarRequisito(rq RequisitoDocumental, cfg ConfiguracionConvocatoria) []ErrorCampo {
	var errores []ErrorCampo
	if !claveValida.MatchString(rq.Clave) {
		errores = append(errores, ErrorCampo{Campo: "clave", Clave: "error.clave_invalida"})
	}
	errores = requerido(errores, "etiqueta_es", rq.EtiquetaES, longitudMaxTexto)
	if rq.Ambito != AmbitoPostulacion && rq.Ambito != AmbitoObra {
		errores = append(errores, ErrorCampo{Campo: "ambito", Clave: "error.ambito_invalido"})
	}
	if rq.DotacionID != 0 {
		// Una regla específica sólo tiene sentido para una dotación de esta
		// misma convocatoria y para documentos de la obra, que es lo que la
		// dotación describe.
		existe := slices.ContainsFunc(cfg.Dotaciones, func(d Dotacion) bool { return d.ID == rq.DotacionID })
		switch {
		case !existe:
			errores = append(errores, ErrorCampo{Campo: "dotacion_id", Clave: "error.dotacion_inexistente"})
		case rq.Ambito == AmbitoPostulacion:
			errores = append(errores, ErrorCampo{Campo: "ambito", Clave: "error.ambito_dotacion"})
		}
	}
	mimesValidos := len(rq.MimesPermitidos) > 0
	for _, mime := range rq.MimesPermitidos {
		mimesValidos = mimesValidos && slices.Contains(MimesAdmitidos, mime)
	}
	if !mimesValidos {
		errores = append(errores, ErrorCampo{Campo: "mimes_permitidos", Clave: "error.mimes_invalidos"})
	}
	if rq.TamanoMaxBytes < 1 || rq.TamanoMaxBytes > TamanoMaxArchivo {
		errores = append(errores, ErrorCampo{Campo: "tamano_max_bytes", Clave: "error.tamano_invalido"})
	}
	if rq.MaxArchivos < 1 || rq.MaxArchivos > MaxArchivosPorRequisito {
		errores = append(errores, ErrorCampo{Campo: "max_archivos", Clave: "error.max_archivos"})
	}
	return errores
}

// ValidarCriterio valida un criterio de evaluación (RF-EVA-10).
func ValidarCriterio(cr CriterioEvaluacion) []ErrorCampo {
	var errores []ErrorCampo
	if !claveValida.MatchString(cr.Clave) {
		errores = append(errores, ErrorCampo{Campo: "clave", Clave: "error.clave_invalida"})
	}
	errores = requerido(errores, "etiqueta_es", cr.EtiquetaES, longitudMaxTexto)
	if cr.EscalaMin < 0 || cr.EscalaMax <= cr.EscalaMin || cr.EscalaMax > 100 {
		errores = append(errores, ErrorCampo{Campo: "escala_max", Clave: "error.escala_invalida"})
	}
	return errores
}

// RequisitosAplicables resuelve qué documentos se piden para una dotación: la
// regla específica de la dotación sobrescribe a la general de la misma clave
// (RN-07). Con dotacionID 0 devuelve sólo las reglas generales. El resultado
// conserva el orden configurado.
func RequisitosAplicables(requisitos []RequisitoDocumental, dotacionID int64) []RequisitoDocumental {
	especificos := make(map[string]RequisitoDocumental)
	for _, rq := range requisitos {
		if dotacionID != 0 && rq.DotacionID == dotacionID {
			especificos[rq.Clave] = rq
		}
	}
	var aplicables []RequisitoDocumental
	for _, rq := range requisitos {
		if rq.DotacionID != 0 {
			continue
		}
		if especifico, ok := especificos[rq.Clave]; ok {
			rq = especifico
			delete(especificos, rq.Clave)
		}
		aplicables = append(aplicables, rq)
	}
	// Reglas de la dotación sin regla general de la misma clave.
	for _, rq := range requisitos {
		if _, pendiente := especificos[rq.Clave]; pendiente && rq.DotacionID == dotacionID {
			aplicables = append(aplicables, rq)
		}
	}
	slices.SortStableFunc(aplicables, func(a, b RequisitoDocumental) int { return a.Orden - b.Orden })
	return aplicables
}

// ProximoEstadoAutomatico dice a qué estado debe pasar una convocatoria por
// calendario, si es que ya toca (§3: «el sistema avanza de etapa
// automáticamente por fecha»). Sólo avanza, nunca retrocede, y de uno en uno.
// Publicar y archivar son siempre actos explícitos del administrador.
func ProximoEstadoAutomatico(c Convocatoria, ahora time.Time) (EstadoConvocatoria, bool) {
	llego := func(clave ClaveEtapa, alTermino bool) bool {
		etapa, ok := c.Etapa(clave)
		if !ok {
			return false
		}
		momento := etapa.IniciaAt
		if alTermino {
			momento = etapa.TerminaAt
		}
		return !ahora.Before(momento)
	}
	switch c.Estado {
	case EstadoPublicada:
		if llego(EtapaPostulacion, false) {
			return EstadoPostulacion, true
		}
	case EstadoPostulacion:
		// La captura se cierra en cuanto vence el plazo, empiece o no ese
		// mismo día la revisión.
		if llego(EtapaPostulacion, true) {
			return EstadoRevisionDocumental, true
		}
	case EstadoRevisionDocumental:
		if llego(EtapaEvaluacion, false) {
			return EstadoEvaluacion, true
		}
	case EstadoEvaluacion:
		if llego(EtapaEvaluacion, true) {
			return EstadoCerrada, true
		}
	}
	return "", false
}

// ValidarCambioDeEstado decide si una convocatoria puede pasar al estado
// destino, y devuelve los bloqueadores si no. Es la ÚNICA puerta de las
// transiciones: la usan por igual el administrador que fuerza un cambio y el
// trabajador que avanza por calendario.
//
// Reglas:
//   - Sólo se pasa al estado inmediato anterior o siguiente.
//   - Publicar exige título, bases en español y el calendario completo.
//   - Abrir la postulación exige, en Compositores, al menos una dotación.
//   - Pasar a revisión documental exige al menos un revisor (RN-14).
//   - Pasar a evaluación exige exactamente tres jurados (RN-14) y criterios.
//   - Retroceder exige un motivo, y que el calendario no vaya a deshacer el
//     retroceso de inmediato: para reabrir un plazo se cambia su fecha.
func ValidarCambioDeEstado(cfg ConfiguracionConvocatoria, destino EstadoConvocatoria, motivo string, ahora time.Time) []ErrorCampo {
	c := cfg.Convocatoria
	desde, hacia := posicionEstado(c.Estado), posicionEstado(destino)
	if hacia < 0 {
		return []ErrorCampo{{Campo: "estado", Clave: "error.estado_invalido"}}
	}
	if hacia != desde+1 && hacia != desde-1 {
		return []ErrorCampo{{Campo: "estado", Clave: "error.estado_salto"}}
	}

	if hacia < desde { // retroceso
		var errores []ErrorCampo
		if strings.TrimSpace(motivo) == "" {
			errores = append(errores, ErrorCampo{Campo: "motivo", Clave: "error.motivo_requerido"})
		}
		simulada := c
		simulada.Estado = destino
		if _, avanzaria := ProximoEstadoAutomatico(simulada, ahora); avanzaria {
			errores = append(errores, ErrorCampo{Campo: "etapas", Clave: "error.estado_fechas"})
		}
		return errores
	}

	var errores []ErrorCampo
	calendarioCompleto := true
	for _, clave := range ordenEtapas {
		if _, ok := c.Etapa(clave); !ok {
			calendarioCompleto = false
		}
	}
	switch destino {
	case EstadoPublicada:
		errores = requerido(errores, "titulo_es", c.TituloES, longitudMaxTitulo)
		if strings.TrimSpace(c.BasesMDES) == "" {
			errores = append(errores, ErrorCampo{Campo: "bases_md_es", Clave: "error.publicar_bases"})
		}
		if !calendarioCompleto {
			errores = append(errores, ErrorCampo{Campo: "etapas", Clave: "error.calendario_incompleto"})
		}
	case EstadoPostulacion:
		if !calendarioCompleto {
			errores = append(errores, ErrorCampo{Campo: "etapas", Clave: "error.calendario_incompleto"})
		}
		if c.Tipo == TipoCompositores && cfg.DotacionesActivas() == 0 {
			errores = append(errores, ErrorCampo{Campo: "dotaciones", Clave: "error.sin_dotaciones"})
		}
	case EstadoRevisionDocumental:
		if cfg.Activos(PerfilRevisorDocumental) < 1 {
			errores = append(errores, ErrorCampo{Campo: "asignaciones", Clave: "error.sin_revisor"})
		}
	case EstadoEvaluacion:
		if cfg.Activos(PerfilJurado) != CuotaJurados {
			errores = append(errores, ErrorCampo{Campo: "asignaciones", Clave: "error.jurados_exactos"})
		}
		if len(cfg.Criterios) == 0 {
			errores = append(errores, ErrorCampo{Campo: "criterios", Clave: "error.sin_criterios"})
		}
		// Las postulaciones que la revisión aún no dictamina NO detienen el
		// paso a evaluación: se revisan después y se suman al jurado.
	}
	return errores
}

// ValidarAsignacion decide si una cuenta puede quedar como jurado o revisor de
// una convocatoria (RF-CONV-09). tienePostulacion indica si esa cuenta postula
// en la misma convocatoria: nadie califica ni revisa un concurso en el que
// participa.
func ValidarAsignacion(cfg ConfiguracionConvocatoria, usuario Usuario, perfil Perfil, tienePostulacion bool) []ErrorCampo {
	if errores := ValidarEdicionDeConfiguracion(cfg.Convocatoria.Estado); errores != nil {
		return errores
	}
	if perfil != PerfilJurado && perfil != PerfilRevisorDocumental {
		return []ErrorCampo{{Campo: "perfil", Clave: "error.perfil_asignacion"}}
	}
	var errores []ErrorCampo
	if !usuario.Activo {
		errores = append(errores, ErrorCampo{Campo: "usuario_id", Clave: "error.usuario_inactivo"})
	}
	if tienePostulacion {
		errores = append(errores, ErrorCampo{Campo: "usuario_id", Clave: "error.asignado_postula"})
	}
	if perfil == PerfilJurado && cfg.Activos(PerfilJurado) >= CuotaJurados {
		errores = append(errores, ErrorCampo{Campo: "perfil", Clave: "error.jurados_completos"})
	}
	return errores
}

// ValidarCuentaInterna valida el alta de una cuenta que crea el administrador
// (RF-AUT-07). El perfil global puede ir vacío: es el caso de un jurado o un
// revisor, cuyos perfiles son por convocatoria.
func ValidarCuentaInterna(n NuevoUsuario) []ErrorCampo {
	var errores []ErrorCampo
	switch {
	case n.Email == "":
		errores = append(errores, ErrorCampo{Campo: "email", Clave: "error.email_requerido"})
	case !EmailValido(n.Email):
		errores = append(errores, ErrorCampo{Campo: "email", Clave: "error.email_invalido"})
	}
	errores = append(errores, ValidarDatosPersonales(n.Nombre, n.Apellidos, n.Idioma)...)
	if n.Perfil != "" && !PerfilGlobalAsignable(n.Perfil) {
		errores = append(errores, ErrorCampo{Campo: "perfil_global", Clave: "error.perfil_global"})
	}
	return errores
}

// PuedeAdministrar: configurar el sistema es exclusivo de admin_sistema.
func PuedeAdministrar(s *Sesion) bool {
	return s != nil && s.TienePerfil(PerfilAdminSistema)
}

// PuedeSupervisar: el observador ve la configuración completa, sin editarla.
func PuedeSupervisar(s *Sesion) bool {
	return s != nil && (s.TienePerfil(PerfilAdminSistema) || s.TienePerfil(PerfilCoordinacionNacional))
}

// megabytes expresa un tamaño en bytes.
func megabytes(n int64) int64 { return n << 20 }

// RequisitosIniciales son los documentos con que nace una convocatoria nueva,
// según su tipo (§5.3.4, §5.3.6 y RF-DOC-04). Son sólo el punto de partida:
// el administrador los edita, y en Compositores añade las excepciones por
// dotación (una obra electroacústica no lleva particellas). Las etiquetas son
// contenido editorial inicial, igual que las plantillas de correo.
func RequisitosIniciales(tipo TipoConvocatoria) []RequisitoDocumental {
	pdf := []string{"application/pdf"}
	fotografia := RequisitoDocumental{
		Clave: "fotografia", EtiquetaES: "Fotografía", EtiquetaEN: "Photograph", Ambito: AmbitoPostulacion,
		Obligatorio: true, MimesPermitidos: []string{"image/jpeg", "image/png"}, TamanoMaxBytes: megabytes(5), MaxArchivos: 1, Orden: 1,
	}
	partitura := RequisitoDocumental{
		Clave: "partitura", EtiquetaES: "Partitura", EtiquetaEN: "Score", Ambito: AmbitoObra,
		Obligatorio: true, MimesPermitidos: pdf, TamanoMaxBytes: megabytes(30), MaxArchivos: 1, Orden: 2,
	}
	if tipo == TipoCompositores {
		return []RequisitoDocumental{fotografia, partitura,
			// Las particellas van reunidas en un solo PDF (decisión de la CNMO).
			{Clave: "particellas", EtiquetaES: "Particellas", EtiquetaEN: "Parts", Ambito: AmbitoObra,
				AyudaES: "Todas las particellas reunidas en un solo archivo PDF.", AyudaEN: "All the parts together in a single PDF file.",
				Obligatorio: true, MimesPermitidos: pdf, TamanoMaxBytes: megabytes(30), MaxArchivos: 1, Orden: 3},
			{Clave: "audio", EtiquetaES: "Archivo de audio", EtiquetaEN: "Audio file", Ambito: AmbitoObra,
				Obligatorio: false, MimesPermitidos: []string{"audio/wav", "audio/flac", "audio/mpeg"},
				TamanoMaxBytes: megabytes(30), MaxArchivos: 1, Orden: 4},
		}
	}
	return []RequisitoDocumental{fotografia, partitura,
		{Clave: "autorizacion", EtiquetaES: "Autorización del titular de la obra", EtiquetaEN: "Authorisation from the rights holder",
			Ambito: AmbitoObra, Obligatorio: true, MimesPermitidos: pdf, TamanoMaxBytes: megabytes(10), MaxArchivos: 1, Orden: 3},
	}
}

// CriteriosIniciales son los dos criterios de la v1 (RF-EVA-03): ambos
// obligatorios, en escala entera de 1 a 5.
func CriteriosIniciales() []CriterioEvaluacion {
	return []CriterioEvaluacion{
		{Clave: "realizacion", EtiquetaES: "Puntaje de realización", EtiquetaEN: "Execution score",
			EscalaMin: 1, EscalaMax: 5, Obligatorio: true, Orden: 1},
		{Clave: "artistico", EtiquetaES: "Puntaje artístico", EtiquetaEN: "Artistic score",
			EscalaMin: 1, EscalaMax: 5, Obligatorio: true, Orden: 2},
	}
}

// ---------------------------------------------------------------------------
// Postulación: definición del formulario, captura y validación del envío
// (§5.3, RN-01 a RN-10)
// ---------------------------------------------------------------------------

// Límites de la captura.
const (
	// LimiteSemblanza es la longitud máxima de una semblanza (§5.3.1).
	LimiteSemblanza = 1500
	// LimiteTextoLargo acota los demás textos largos (notas, requerimientos).
	LimiteTextoLargo = 5000
	// DuracionMaxObra acota la duración de una obra, en minutos. No es una
	// regla del concurso: sólo impide capturar un número absurdo.
	DuracionMaxObra = 600
	// MaxIntegrantes acota la lista de integrantes de un ensamble. La
	// especificación no fija un máximo (D-05); éste sólo evita el abuso.
	MaxIntegrantes = 300
	// AnioMinimo es el año más antiguo admitido (nacimiento, fundación).
	AnioMinimo = 1900

	longitudMaxURL = 2000
)

// TipoCampo es el tipo de dato de un campo del formulario.
type TipoCampo string

const (
	CampoTexto             TipoCampo = "texto"
	CampoTextoLargo        TipoCampo = "texto_largo"
	CampoEntero            TipoCampo = "entero"
	CampoAnio              TipoCampo = "anio"               // de AnioMinimo al año en curso
	CampoFecha             TipoCampo = "fecha"              // AAAA-MM-DD
	CampoFechaNacimiento   TipoCampo = "fecha_nacimiento"   // AAAA-MM-DD, no futura
	CampoHora              TipoCampo = "hora"               // HH:MM
	CampoCorreo            TipoCampo = "correo"             //
	CampoURL               TipoCampo = "url"                // https
	CampoOpcion            TipoCampo = "opcion"             // uno de Opciones
	CampoPais              TipoCampo = "pais"               // ISO 3166-1 alfa-2
	CampoEntidad           TipoCampo = "entidad"            // una de las 32 entidades
	CampoEntidadResidencia TipoCampo = "entidad_residencia" // entidad si el país es México; texto libre si no
	CampoDotacion          TipoCampo = "dotacion"           // id de una dotación activa de la convocatoria
)

// CondicionFueraDeCDMX es la condición de los campos de sede: son
// obligatorios si el concierto se propone fuera de la Ciudad de México (RN-06).
const CondicionFueraDeCDMX = "fuera_de_cdmx"

// DefinicionCampo describe un campo del formulario de postulación. Es la
// definición DECLARATIVA de la que salen la captura, la validación del envío
// y la descripción que recibe el cliente: un campo se define una sola vez.
type DefinicionCampo struct {
	Ruta        string    // ruta canónica: «datos.telefono», «concierto.entidad», «obra.titulo»
	Tipo        TipoCampo //
	Obligatorio bool      // exigido para enviar
	Condicion   string    // si no está vacía, el campo es obligatorio cuando la condición se cumple
	Longitud    int       // máximo de caracteres (textos)
	Minimo      int       // rango (enteros)
	Maximo      int       //
	Opciones    []string  // valores admitidos (CampoOpcion)

	// Dónde se guarda. Son constantes del código: el nombre de una tabla o de
	// una columna nunca sale de un dato de la petición.
	tabla   string
	columna string
}

// DefinicionFormulario es el formulario que corresponde a una postulación
// según su convocatoria y su subcategoría.
type DefinicionFormulario struct {
	Campos           []DefinicionCampo // de la postulación: datos generales y propuesta de concierto
	CamposObra       []DefinicionCampo // de cada obra
	TieneIntegrantes bool              // ensambles
	TieneVideos      bool              // Intérpretes
}

// Campo busca una definición por su ruta entre los campos de la postulación.
func (f DefinicionFormulario) Campo(ruta string) (DefinicionCampo, bool) {
	return buscarCampo(f.Campos, ruta)
}

// CampoDeObra busca una definición por su ruta entre los campos de una obra.
func (f DefinicionFormulario) CampoDeObra(ruta string) (DefinicionCampo, bool) {
	return buscarCampo(f.CamposObra, ruta)
}

func buscarCampo(campos []DefinicionCampo, ruta string) (DefinicionCampo, bool) {
	for _, c := range campos {
		if c.Ruta == ruta {
			return c, true
		}
	}
	return DefinicionCampo{}, false
}

// campo abrevia la definición de un campo: grupo.columna guardado en tabla.
func campo(grupo, tabla, columna string, tipo TipoCampo, obligatorio bool, longitud int) DefinicionCampo {
	return DefinicionCampo{Ruta: grupo + "." + columna, Tipo: tipo, Obligatorio: obligatorio, Longitud: longitud, tabla: tabla, columna: columna}
}

// tablaDeDatos devuelve la tabla 1:1 de datos generales de una postulación.
func tablaDeDatos(tipo TipoConvocatoria, sub Subcategoria) string {
	switch {
	case tipo == TipoCompositores:
		return "datos_compositor"
	case sub == SubcategoriaEnsamble:
		return "datos_ensamble"
	default:
		return "datos_solista"
	}
}

// camposDePersona son los datos generales de un solista (§5.3.1) y de un
// compositor (§5.3.5), que son los mismos.
func camposDePersona(tabla string) []DefinicionCampo {
	genero := campo("datos", tabla, "genero", CampoOpcion, true, 0)
	genero.Opciones = OpcionesGenero
	return []DefinicionCampo{
		campo("datos", tabla, "nombre_completo", CampoTexto, true, 200),
		campo("datos", tabla, "nombre_artistico", CampoTexto, false, 200), // opcional (D-04)
		genero,
		campo("datos", tabla, "fecha_nacimiento", CampoFechaNacimiento, true, 0),
		campo("datos", tabla, "telefono", CampoTexto, true, 30),
		campo("datos", tabla, "email_contacto", CampoCorreo, true, 0),
		campo("datos", tabla, "nacionalidad", CampoPais, true, 0),
		campo("datos", tabla, "pais_residencia", CampoPais, true, 0),
		campo("datos", tabla, "entidad_residencia", CampoEntidadResidencia, true, 100),
		campo("datos", tabla, "semblanza", CampoTextoLargo, true, LimiteSemblanza),
	}
}

// camposDeEnsamble son los datos generales de un ensamble (§5.3.2).
func camposDeEnsamble() []DefinicionCampo {
	const tabla = "datos_ensamble"
	return []DefinicionCampo{
		campo("datos", tabla, "nombre_ensamble", CampoTexto, true, 200),
		campo("datos", tabla, "representante", CampoTexto, true, 200),
		campo("datos", tabla, "telefono", CampoTexto, true, 30),
		campo("datos", tabla, "email_contacto", CampoCorreo, true, 0),
		campo("datos", tabla, "dotacion_texto", CampoTexto, true, 500),
		campo("datos", tabla, "anio_fundacion", CampoAnio, true, 0),
		campo("datos", tabla, "pais_residencia", CampoPais, true, 0),
		campo("datos", tabla, "entidad_residencia", CampoEntidadResidencia, true, 100),
		campo("datos", tabla, "semblanza", CampoTextoLargo, true, LimiteSemblanza),
	}
}

// camposDeConcierto es la propuesta de concierto de Intérpretes (§5.3.3). Los
// cuatro campos de sede son obligatorios sólo fuera de la Ciudad de México.
func camposDeConcierto() []DefinicionCampo {
	const tabla = "propuesta_concierto"
	deSede := func(columna string, tipo TipoCampo, longitud int) DefinicionCampo {
		c := campo("concierto", tabla, columna, tipo, false, longitud)
		c.Condicion = CondicionFueraDeCDMX
		return c
	}
	return []DefinicionCampo{
		campo("concierto", tabla, "requerimientos_tecnicos", CampoTextoLargo, true, LimiteTextoLargo),
		campo("concierto", tabla, "entidad", CampoEntidad, true, 0),
		deSede("sede_nombre", CampoTexto, 200),
		deSede("sede_domicilio", CampoTextoLargo, LimiteTextoLargo),
		deSede("fecha_concierto", CampoFecha, 0),
		deSede("hora_concierto", CampoHora, 0),
	}
}

// camposDeObra son los datos de una obra: repertorio propuesto en Intérpretes
// (§5.3.4) u obra de autoría propia en Compositores (§5.3.6).
func camposDeObra(tipo TipoConvocatoria) []DefinicionCampo {
	const tabla = "obra"
	duracion := campo("obra", tabla, "duracion_min", CampoEntero, true, 0)
	duracion.Minimo, duracion.Maximo = 1, DuracionMaxObra
	estreno := campo("obra", tabla, "estreno", CampoOpcion, true, 0)
	estreno.Opciones = OpcionesEstreno
	if tipo == TipoCompositores {
		return []DefinicionCampo{
			campo("obra", tabla, "titulo", CampoTexto, true, 300),
			duracion,
			estreno,
			campo("obra", tabla, "dotacion_id", CampoDotacion, true, 0),
			campo("obra", tabla, "nota_obra", CampoTextoLargo, true, LimiteTextoLargo),
			campo("obra", tabla, "requerimientos_tecnicos", CampoTextoLargo, true, LimiteTextoLargo),
			campo("obra", tabla, "liga_media", CampoURL, false, 0),
		}
	}
	return []DefinicionCampo{
		campo("obra", tabla, "titulo", CampoTexto, true, 300),
		campo("obra", tabla, "compositor", CampoTexto, true, 200),
		duracion,
		estreno,
		campo("obra", tabla, "semblanza_compositor", CampoTextoLargo, true, LimiteSemblanza),
		campo("obra", tabla, "nota_obra", CampoTextoLargo, true, LimiteTextoLargo),
		campo("obra", tabla, "requerimientos_tecnicos", CampoTextoLargo, false, LimiteTextoLargo),
		campo("obra", tabla, "liga_media", CampoURL, true, 0),
	}
}

// FormularioDe devuelve la definición del formulario que corresponde a una
// convocatoria y, en Intérpretes, a una subcategoría.
func FormularioDe(tipo TipoConvocatoria, sub Subcategoria) DefinicionFormulario {
	f := DefinicionFormulario{CamposObra: camposDeObra(tipo)}
	switch {
	case tipo == TipoCompositores:
		f.Campos = camposDePersona(tablaDeDatos(tipo, sub))
	case sub == SubcategoriaEnsamble:
		f.Campos, f.TieneIntegrantes = camposDeEnsamble(), true
	default:
		f.Campos = camposDePersona(tablaDeDatos(tipo, sub))
	}
	if tipo == TipoInterpretes {
		f.Campos = append(f.Campos, camposDeConcierto()...)
		f.TieneVideos = true
	}
	return f
}

// ContextoCampos es lo que la validación de un valor necesita saber además
// del propio valor.
type ContextoCampos struct {
	Ahora          time.Time
	PaisResidencia string     // decide cómo se valida la entidad de residencia
	Dotaciones     []Dotacion // catálogo de la convocatoria
}

// URLValida indica si un texto es una liga aceptable: bien formada, con
// esquema https y un nombre de servidor (RF-DOC-10). Que además responda y sea
// pública lo comprueba el sistema por red, aparte.
func URLValida(texto string) bool {
	if texto == "" || len(texto) > longitudMaxURL || strings.ContainsAny(texto, " \t\r\n") {
		return false
	}
	u, err := url.Parse(texto)
	return err == nil && u.Scheme == "https" && u.User == nil && strings.Contains(u.Hostname(), ".")
}

// PlataformaDeVideo reconoce las plataformas conocidas por el nombre del
// servidor de la liga, para que la comprobación automática sepa cómo
// verificar que el recurso sea público. Lo que no reconoce es «otro».
func PlataformaDeVideo(liga string) string {
	u, err := url.Parse(liga)
	if err != nil {
		return "otro"
	}
	servidor := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	es := func(dominio string) bool { return servidor == dominio || strings.HasSuffix(servidor, "."+dominio) }
	switch {
	case es("youtube.com"), es("youtu.be"):
		return "youtube"
	case es("vimeo.com"):
		return "vimeo"
	case es("soundcloud.com"):
		return "soundcloud"
	case es("bandcamp.com"):
		return "bandcamp"
	case es("drive.google.com"), es("docs.google.com"):
		return "drive"
	}
	return "otro"
}

// ValidarValorDeCampo comprueba el FORMATO de un valor capturado y devuelve la
// clave de traducción del problema, o "" si es aceptable. Un valor vacío
// siempre es aceptable aquí: la captura es gradual, y que un campo obligatorio
// esté lleno es asunto del envío (ValidarParaEnvio), no del guardado.
func ValidarValorDeCampo(def DefinicionCampo, valor string, ctx ContextoCampos) string {
	if valor == "" {
		return ""
	}
	largo := utf8.RuneCountInString(valor)
	switch def.Tipo {
	case CampoTexto, CampoTextoLargo:
		if largo > def.Longitud {
			return "error.texto_largo_campo"
		}
	case CampoEntero, CampoAnio:
		n, err := strconv.Atoi(valor)
		minimo, maximo := def.Minimo, def.Maximo
		if def.Tipo == CampoAnio {
			minimo, maximo = AnioMinimo, ctx.Ahora.In(time.UTC).Year()
		}
		if err != nil || n < minimo || n > maximo {
			return "error.numero_fuera_de_rango"
		}
	case CampoFecha, CampoFechaNacimiento:
		fecha, err := time.Parse(time.DateOnly, valor)
		if err != nil {
			return "error.fecha_invalida"
		}
		if def.Tipo == CampoFechaNacimiento && (fecha.Year() < AnioMinimo || fecha.After(ctx.Ahora)) {
			return "error.fecha_invalida"
		}
	case CampoHora:
		if _, err := time.Parse("15:04", valor); err != nil {
			return "error.hora_invalida"
		}
	case CampoCorreo:
		if !EmailValido(valor) {
			return "error.email_invalido"
		}
	case CampoURL:
		if !URLValida(valor) {
			return "error.url_invalida"
		}
	case CampoOpcion:
		if !slices.Contains(def.Opciones, valor) {
			return "error.opcion_invalida"
		}
	case CampoPais:
		if !PaisValido(valor) {
			return "error.opcion_invalida"
		}
	case CampoEntidad:
		if !EntidadValida(valor) {
			return "error.opcion_invalida"
		}
	case CampoEntidadResidencia:
		// En México la entidad se elige del catálogo; en otro país es texto libre.
		if ctx.PaisResidencia == PaisMexico {
			if !EntidadValida(valor) {
				return "error.opcion_invalida"
			}
		} else if largo > def.Longitud {
			return "error.texto_largo_campo"
		}
	case CampoDotacion:
		id, err := strconv.ParseInt(valor, 10, 64)
		activa := slices.ContainsFunc(ctx.Dotaciones, func(d Dotacion) bool { return d.ID == id && d.Activa })
		if err != nil || !activa {
			return "error.dotacion_inexistente"
		}
	}
	return ""
}

// EnPlazoDePostulacion indica si la convocatoria admite captura en este
// momento. Exige las DOS cosas: que esté en la etapa de postulación y que la
// fecha esté dentro del plazo. Así, si la convocatoria no avanzó de etapa al
// vencer el plazo (p. ej. porque faltaba asignar un revisor), la captura
// queda cerrada de todos modos.
func EnPlazoDePostulacion(c Convocatoria, ahora time.Time) []ErrorCampo {
	if !AdmitePostulaciones(c) {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.fuera_de_etapa"}}
	}
	etapa, ok := c.Etapa(EtapaPostulacion)
	if !ok || ahora.Before(etapa.IniciaAt) || !ahora.Before(etapa.TerminaAt) {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.plazo_vencido"}}
	}
	return nil
}

// PuedePostular: el observador ve todas las evaluaciones, por lo que no
// participa como postulante (matriz de permisos, §2.2).
func PuedePostular(s *Sesion) bool {
	return s != nil && !s.TienePerfil(PerfilCoordinacionNacional)
}

// ValidarNuevaPostulacion decide si puede crearse una postulación (RN-01,
// RN-02). esJuradoORevisor indica si la cuenta tiene un perfil operativo
// activo en esa misma convocatoria: nadie postula en el concurso que califica
// o revisa.
func ValidarNuevaPostulacion(c Convocatoria, sub Subcategoria, esJuradoORevisor bool, ahora time.Time) []ErrorCampo {
	if errores := EnPlazoDePostulacion(c, ahora); errores != nil {
		return errores
	}
	var errores []ErrorCampo
	switch {
	case c.Tipo == TipoInterpretes && sub != SubcategoriaSolista && sub != SubcategoriaEnsamble:
		errores = append(errores, ErrorCampo{Campo: "subcategoria", Clave: "error.subcategoria_requerida"})
	case c.Tipo == TipoCompositores && sub != "":
		errores = append(errores, ErrorCampo{Campo: "subcategoria", Clave: "error.subcategoria_no_aplica"})
	}
	if esJuradoORevisor {
		errores = append(errores, ErrorCampo{Campo: "convocatoria", Clave: "error.postula_asignado"})
	}
	return errores
}

// ValidarEdicionDePostulacion decide si el postulante puede modificar su
// postulación (RN-02, RN-04): sólo mientras es borrador y la convocatoria está
// en plazo. La edición acotada de una subsanación llegará con la revisión
// documental.
func ValidarEdicionDePostulacion(p Postulacion, c Convocatoria, ahora time.Time) []ErrorCampo {
	_, errores := PermisoDeEdicionDe(p, c, ahora)
	return errores
}

// PuedeVerPostulacion: una postulación la ven su titular, el administrador y
// el observador. El acceso de revisores y jurados llegará con sus módulos.
func PuedeVerPostulacion(s *Sesion, p Postulacion) bool {
	if s == nil {
		return false
	}
	if s.Usuario.ID == p.UsuarioID || PuedeSupervisar(s) {
		return true
	}
	// El revisor ve las postulaciones ENVIADAS de su convocatoria. Un
	// borrador no ha entrado al proceso: no es asunto suyo.
	if p.Estado != PostulacionBorrador && s.TieneAsignacion(p.ConvocatoriaID, PerfilRevisorDocumental) {
		return true
	}
	// El jurado ve sólo las que pasaron a evaluación (RN-20), y las ve
	// completas: la evaluación NO es a ciegas (decisión de la CNMO).
	return EnEvaluacion(p.Estado) && s.TieneAsignacion(p.ConvocatoriaID, PerfilJurado)
}

// ValidarIntegrantes valida la lista de integrantes de un ensamble (RN-10):
// cada uno con nombre e instrumento.
func ValidarIntegrantes(integrantes []Integrante) []ErrorCampo {
	if len(integrantes) > MaxIntegrantes {
		return []ErrorCampo{{Campo: "integrantes", Clave: "error.demasiados_integrantes"}}
	}
	for _, i := range integrantes {
		if strings.TrimSpace(i.Nombre) == "" || strings.TrimSpace(i.Instrumento) == "" ||
			utf8.RuneCountInString(i.Nombre) > longitudMaxTexto || utf8.RuneCountInString(i.Instrumento) > longitudMaxTexto {
			return []ErrorCampo{{Campo: "integrantes", Clave: "error.integrante_incompleto"}}
		}
	}
	return nil
}

// ValidarVideos valida la lista de ligas a videos (RN-08): no más del máximo
// de la convocatoria, todas con formato válido y sin repetirse.
func ValidarVideos(ligas []string, maximo int) []ErrorCampo {
	if len(ligas) > maximo {
		return []ErrorCampo{{Campo: "videos", Clave: "error.demasiados_videos"}}
	}
	vistas := make(map[string]bool, len(ligas))
	for _, liga := range ligas {
		if !URLValida(liga) {
			return []ErrorCampo{{Campo: "videos", Clave: "error.url_invalida"}}
		}
		if vistas[liga] {
			return []ErrorCampo{{Campo: "videos", Clave: "error.video_repetido"}}
		}
		vistas[liga] = true
	}
	return nil
}

// ValidarNuevaObra decide si puede añadirse otra obra (RN-05): en
// Compositores hay un máximo; en Intérpretes, no.
func ValidarNuevaObra(c Convocatoria, obrasActuales int) []ErrorCampo {
	if c.MaxObras > 0 && obrasActuales >= c.MaxObras {
		return []ErrorCampo{{Campo: "obras", Clave: "error.demasiadas_obras"}}
	}
	return nil
}

// Secciones del formulario, en el orden en que se recorren (RF-POST-04).
const (
	SeccionDatos      = "datos"
	SeccionConcierto  = "concierto"
	SeccionObras      = "obras"
	SeccionDocumentos = "documentos"
	SeccionEnvio      = "envio"
)

// SeccionesDe devuelve las secciones del formulario de una convocatoria. La
// propuesta de concierto sólo existe en Intérpretes.
func SeccionesDe(tipo TipoConvocatoria) []string {
	if tipo == TipoInterpretes {
		return []string{SeccionDatos, SeccionConcierto, SeccionObras, SeccionDocumentos, SeccionEnvio}
	}
	return []string{SeccionDatos, SeccionObras, SeccionDocumentos, SeccionEnvio}
}

// seccionDeRuta devuelve la sección a la que pertenece un campo.
func seccionDeRuta(ruta string) string {
	switch {
	case strings.HasPrefix(ruta, "concierto."):
		return SeccionConcierto
	case strings.HasPrefix(ruta, "obra."):
		return SeccionObras
	}
	return SeccionDatos
}

// condicionCumplida evalúa la condición de un campo condicional sobre los
// valores capturados.
func condicionCumplida(condicion string, campos map[string]string) bool {
	if condicion != CondicionFueraDeCDMX {
		return false
	}
	// Sin entidad elegida todavía no se sabe si el concierto es fuera de la
	// Ciudad de México: sólo falta la entidad, no los datos de la sede.
	entidad := strings.TrimSpace(campos["concierto.entidad"])
	return entidad != "" && entidad != EntidadCDMX
}

// ValidarParaEnvio comprueba que una postulación esté completa y devuelve la
// lista exacta de lo que le falta (RN-03). Lista vacía = puede enviarse.
//
// Es la ÚNICA fuente de verdad de la completitud: la usan por igual la
// pantalla de revisión previa y el envío. Comprueba, en este orden:
//
//  1. los campos obligatorios de los datos generales y, en Intérpretes, de la
//     propuesta de concierto, incluidos los de sede si el concierto es fuera
//     de la Ciudad de México (RN-06);
//  2. en ensambles, al menos un integrante (RN-10);
//  3. en Intérpretes, de una a tres ligas de video sin problemas (RN-08);
//  4. al menos una obra, sin pasar del máximo ni de la duración total
//     permitida (RN-05), y cada obra con sus campos obligatorios;
//  5. los documentos obligatorios de la postulación y de cada obra, según la
//     dotación de cada una (RN-07);
//  6. que el titular haya verificado su correo (RF-AUT-02).
//
// Un valor capturado que ya no es válido también bloquea: por ejemplo, una
// entidad de residencia escrita a mano cuando el país se cambió a México, o
// una dotación que el administrador retiró del catálogo.
func ValidarParaEnvio(p Postulacion, cfg ConfiguracionConvocatoria, titular Usuario, ahora time.Time) []Bloqueador {
	c := cfg.Convocatoria
	formulario := FormularioDe(c.Tipo, p.Subcategoria)
	ctx := ContextoCampos{Ahora: ahora, PaisResidencia: p.Campos["datos.pais_residencia"], Dotaciones: cfg.Dotaciones}

	var bloqueadores []Bloqueador
	agregar := func(seccion, campo string, obraID int64, clave string) {
		bloqueadores = append(bloqueadores, Bloqueador{Seccion: seccion, Campo: campo, ObraID: obraID, Clave: clave})
	}
	revisar := func(definiciones []DefinicionCampo, valores map[string]string, obraID int64) {
		for _, def := range definiciones {
			valor := strings.TrimSpace(valores[def.Ruta])
			switch {
			case valor == "":
				if def.Obligatorio || condicionCumplida(def.Condicion, p.Campos) {
					agregar(seccionDeRuta(def.Ruta), def.Ruta, obraID, "bloqueo.campo_requerido")
				}
			case ValidarValorDeCampo(def, valor, ctx) != "":
				agregar(seccionDeRuta(def.Ruta), def.Ruta, obraID, "bloqueo.campo_invalido")
			}
		}
	}

	// 1. Datos generales y propuesta de concierto.
	revisar(formulario.Campos, p.Campos, 0)

	// 2. Integrantes del ensamble.
	if formulario.TieneIntegrantes && len(p.Integrantes) == 0 {
		agregar(SeccionDatos, "integrantes", 0, "bloqueo.sin_integrantes")
	}

	// 3. Ligas de video.
	if formulario.TieneVideos {
		switch {
		case len(p.Videos) == 0:
			agregar(SeccionDatos, "videos", 0, "bloqueo.sin_videos")
		case len(p.Videos) > c.MaxVideos:
			agregar(SeccionDatos, "videos", 0, "bloqueo.demasiados_videos")
		}
		for _, video := range p.Videos {
			if video.Estado == EnlaceInaccesible || video.Estado == EnlaceNoPublico {
				agregar(SeccionDatos, "videos", 0, "bloqueo.video_con_problema")
				break
			}
		}
	}

	// 4. Obras.
	switch {
	case len(p.Obras) == 0:
		agregar(SeccionObras, "obras", 0, "bloqueo.sin_obras")
	case c.MaxObras > 0 && len(p.Obras) > c.MaxObras:
		agregar(SeccionObras, "obras", 0, "bloqueo.demasiadas_obras")
	}
	duracionTotal := 0
	for _, obra := range p.Obras {
		revisar(formulario.CamposObra, obra.Campos, obra.ID)
		if minutos, err := strconv.Atoi(obra.Campos["obra.duracion_min"]); err == nil {
			duracionTotal += minutos
		}
	}
	if c.DuracionMaxProgramaMin > 0 && duracionTotal > c.DuracionMaxProgramaMin {
		agregar(SeccionObras, "obras", 0, "bloqueo.duracion_excedida")
	}

	// 5. Documentos obligatorios.
	tiene := func(clave string, obraID int64) bool {
		return slices.ContainsFunc(p.Documentos, func(d Documento) bool { return d.Clave == clave && d.ObraID == obraID })
	}
	exigir := func(dotacionID, obraID int64, ambito AmbitoRequisito) {
		for _, requisito := range RequisitosAplicables(cfg.Requisitos, dotacionID) {
			if requisito.Ambito == ambito && requisito.Obligatorio && !tiene(requisito.Clave, obraID) {
				bloqueadores = append(bloqueadores, Bloqueador{Seccion: SeccionDocumentos, Campo: "documento." + requisito.Clave,
					ObraID: obraID, Clave: "bloqueo.documento_faltante", Requisito: &requisito})
			}
		}
	}
	exigir(0, 0, AmbitoPostulacion)
	for _, obra := range p.Obras {
		// La dotación de CADA obra decide qué documentos se le piden. Si aún
		// no se elige, valen las reglas generales.
		dotacionID, _ := strconv.ParseInt(obra.Campos["obra.dotacion_id"], 10, 64)
		exigir(dotacionID, obra.ID, AmbitoObra)
	}

	// 6. Correo verificado.
	if !titular.EmailVerificado() {
		agregar(SeccionEnvio, "cuenta.email", 0, "bloqueo.correo_sin_verificar")
	}
	return bloqueadores
}

// --- Documentos y archivos (§5.4) ---------------------------------------------

const (
	// TamanoCabeceraMIME son los primeros bytes de un archivo que bastan para
	// reconocer su formato por la firma.
	TamanoCabeceraMIME = 512
	// MaxVersionesDeDocumento acota cuántas veces puede reemplazarse un mismo
	// documento. Como nada se borra, sin tope una cuenta podría llenar el
	// disco subiendo una y otra vez el mismo requisito.
	MaxVersionesDeDocumento = 30
	// Topes de los archivos públicos de una convocatoria (RF-CONV-02).
	TamanoMaxBasesPDF = 20 << 20 // 20 MiB
	TamanoMaxImagen   = 5 << 20  // 5 MiB
)

// extensionesDeMIME son las extensiones con que puede llegar cada formato
// admitido. La primera es la que se usa al nombrar una descarga.
var extensionesDeMIME = map[string][]string{
	"application/pdf": {".pdf"},
	"image/jpeg":      {".jpg", ".jpeg"},
	"image/png":       {".png"},
	"audio/wav":       {".wav"},
	"audio/flac":      {".flac"},
	"audio/mpeg":      {".mp3"},
}

// DetectarMIME reconoce el formato de un archivo por su firma: los primeros
// bytes de su contenido (RF-DOC-01). No mira el nombre ni lo que el navegador
// diga: una extensión se cambia en un segundo. Devuelve "" si el contenido no
// es de ninguno de los formatos de MimesAdmitidos.
func DetectarMIME(cabecera []byte) string {
	empieza := func(firma string) bool { return strings.HasPrefix(string(cabecera), firma) }
	switch {
	case empieza("%PDF-"):
		return "application/pdf"
	case empieza("\xFF\xD8\xFF"):
		return "image/jpeg"
	case empieza("\x89PNG\r\n\x1a\n"):
		return "image/png"
	case empieza("RIFF") && len(cabecera) >= 12 && string(cabecera[8:12]) == "WAVE":
		return "audio/wav"
	case empieza("fLaC"):
		return "audio/flac"
	case empieza("ID3"):
		return "audio/mpeg" // MP3 con etiqueta ID3v2 al inicio
	}
	// MP3 sin etiqueta: empieza por la cabecera de un cuadro MPEG. Son 11 bits
	// de sincronía a 1; la versión, la capa y la tasa de bits no pueden tener
	// sus valores reservados.
	if len(cabecera) >= 3 && cabecera[0] == 0xFF && cabecera[1]&0xE0 == 0xE0 &&
		cabecera[1]&0x18 != 0x08 && cabecera[1]&0x06 != 0 && cabecera[2]&0xF0 != 0xF0 {
		return "audio/mpeg"
	}
	return ""
}

// ExtensionDeMIME devuelve la extensión canónica de un formato admitido.
func ExtensionDeMIME(mime string) string {
	if extensiones := extensionesDeMIME[mime]; len(extensiones) > 0 {
		return extensiones[0]
	}
	return ""
}

// extensionDe devuelve la extensión de un nombre de archivo, en minúsculas.
func extensionDe(nombre string) string {
	if i := strings.LastIndexByte(nombre, '.'); i >= 0 {
		return strings.ToLower(nombre[i:])
	}
	return ""
}

// validarFormato comprueba un archivo contra los formatos y el tamaño que se
// admiten para él: que su contenido sea de un formato permitido, que la
// extensión de su nombre corresponda a ese contenido y que no pese de más.
func validarFormato(mimesPermitidos []string, tamanoMax int64, nombre, mime string, tamano int64) []ErrorCampo {
	fallo := func(clave string) []ErrorCampo { return []ErrorCampo{{Campo: "archivo", Clave: clave}} }
	switch {
	case tamano == 0:
		return fallo("error.archivo_vacio")
	case tamano > tamanoMax:
		return fallo("error.archivo_grande")
	case mime == "":
		return fallo("error.formato_no_reconocido")
	case !slices.Contains(mimesPermitidos, mime):
		return fallo("error.formato_no_admitido")
	case !slices.Contains(extensionesDeMIME[mime], extensionDe(nombre)):
		// Un PDF llamado «foto.jpg»: se rechaza para que el nombre no engañe
		// a quien lo descargue.
		return fallo("error.extension_no_coincide")
	}
	return nil
}

// ValidarArchivo comprueba un archivo recibido contra su requisito documental
// (RF-DOC-01 y 04). mime es el detectado por DetectarMIME.
func ValidarArchivo(rq RequisitoDocumental, nombre, mime string, tamano int64) []ErrorCampo {
	return validarFormato(rq.MimesPermitidos, rq.TamanoMaxBytes, nombre, mime, tamano)
}

// RequisitoParaDocumento resuelve qué requisito cumple un documento que se
// quiere subir, a partir de su clave y de la obra a la que pertenece.
//
// La regla efectiva depende de la dotación de la obra (RN-07): un mismo
// «audio» puede ser opcional para un cuarteto y obligatorio para una obra
// electroacústica, con formatos y tamaños distintos.
func RequisitoParaDocumento(p Postulacion, cfg ConfiguracionConvocatoria, clave string, obraID int64) (RequisitoDocumental, []ErrorCampo) {
	ambito, dotacionID := AmbitoPostulacion, int64(0)
	if obraID != 0 {
		posicion := slices.IndexFunc(p.Obras, func(o Obra) bool { return o.ID == obraID })
		if posicion < 0 {
			return RequisitoDocumental{}, []ErrorCampo{{Campo: "obra_id", Clave: "error.obra_ajena"}}
		}
		ambito = AmbitoObra
		// Sin dotación elegida valen las reglas generales (ParseInt da 0).
		dotacionID, _ = strconv.ParseInt(p.Obras[posicion].Campos["obra.dotacion_id"], 10, 64)
	}
	for _, rq := range RequisitosAplicables(cfg.Requisitos, dotacionID) {
		if rq.Clave == clave && rq.Ambito == ambito {
			return rq, nil
		}
	}
	return RequisitoDocumental{}, []ErrorCampo{{Campo: "clave", Clave: "error.requisito_inexistente"}}
}

// LugarDeNuevoDocumento decide qué hace una subida con los documentos que ya
// hay para el mismo requisito: devuelve el que queda reemplazado, o nil si el
// nuevo se añade.
//
//   - Si se indica reemplazaA, sustituye a ese documento.
//   - Si el requisito admite un solo archivo y ya hay uno, lo sustituye: es lo
//     que espera quien vuelve a subir su partitura.
//   - Si admite varios y ya están todos, se rechaza: hay que decir cuál se
//     reemplaza.
//
// versiones es cuántos documentos, vigentes o no, se han subido ya para ese
// requisito.
func LugarDeNuevoDocumento(rq RequisitoDocumental, vigentes []Documento, reemplazaA int64, versiones int) (*Documento, []ErrorCampo) {
	if versiones >= MaxVersionesDeDocumento {
		return nil, []ErrorCampo{{Campo: "archivo", Clave: "error.demasiadas_versiones"}}
	}
	if reemplazaA != 0 {
		posicion := slices.IndexFunc(vigentes, func(d Documento) bool { return d.ID == reemplazaA })
		if posicion < 0 {
			return nil, []ErrorCampo{{Campo: "reemplaza", Clave: "error.documento_no_vigente"}}
		}
		return &vigentes[posicion], nil
	}
	if len(vigentes) < max(rq.MaxArchivos, 1) {
		return nil, nil
	}
	if rq.MaxArchivos <= 1 {
		return &vigentes[0], nil
	}
	return nil, []ErrorCampo{{Campo: "archivo", Clave: "error.demasiados_archivos"}}
}

// NombreDeDescarga arma el nombre con que se descarga un documento. Es
// predecible —folio, requisito, obra y versión— y no usa el nombre que le
// puso quien lo subió, que puede traer datos personales o caracteres
// problemáticos. identificador es el folio, o «borrador-N» si aún no hay.
func NombreDeDescarga(identificador string, d Documento, ordenDeObra int) string {
	nombre := identificador + "_" + d.Clave
	if ordenDeObra > 0 {
		nombre += "_obra" + strconv.Itoa(ordenDeObra)
	}
	return nombre + "_v" + strconv.Itoa(d.Version) + ExtensionDeMIME(d.Archivo.MIME)
}

// ValidarArchivoDeConvocatoria comprueba un archivo público de la
// convocatoria: las bases en PDF, una por idioma, o la imagen de cabecera
// (RF-CONV-02).
func ValidarArchivoDeConvocatoria(clase ClaseArchivoConvocatoria, nombre, mime string, tamano int64) []ErrorCampo {
	switch clase {
	case ArchivoBasesES, ArchivoBasesEN:
		return validarFormato([]string{"application/pdf"}, TamanoMaxBasesPDF, nombre, mime, tamano)
	case ArchivoImagen:
		return validarFormato([]string{"image/jpeg", "image/png"}, TamanoMaxImagen, nombre, mime, tamano)
	}
	return []ErrorCampo{{Campo: "clase", Clave: "error.clase_de_archivo"}}
}

// --- Envío (§4.1) -------------------------------------------------------------

// ValidarConfirmacionDeEnvio exige las dos declaraciones del envío
// (RF-POST-10): que se entiende que es irreversible y que se aceptan las bases.
func ValidarConfirmacionDeEnvio(c ConfirmacionEnvio) []ErrorCampo {
	var errores []ErrorCampo
	if !c.Confirmo {
		errores = append(errores, ErrorCampo{Campo: "confirmo", Clave: "error.confirmar_envio"})
	}
	if !c.AceptoBases {
		errores = append(errores, ErrorCampo{Campo: "acepto_bases", Clave: "error.aceptar_bases"})
	}
	return errores
}

// FolioDe arma el folio de una postulación: FIM49-INT-0137. Lleva el número de
// la edición, la convocatoria y un consecutivo por convocatoria. Se asigna en
// el primer envío y no cambia nunca (RN-13): es lo que se cita en un acta.
func FolioDe(numeroDeEdicion int, tipo TipoConvocatoria, consecutivo int) string {
	sigla := "INT"
	if tipo == TipoCompositores {
		sigla = "COM"
	}
	return fmt.Sprintf("FIM%d-%s-%04d", numeroDeEdicion, sigla, consecutivo)
}

// SnapshotDeEnvio congela la postulación tal como se envía (§4.1). Recoge
// sólo lo que el formulario de la postulación define y los documentos
// vigentes; los campos añadidos por extensión dinámica no entran (§7.6).
func SnapshotDeEnvio(p Postulacion, cfg ConfiguracionConvocatoria, titular Usuario, folio string, version int, enviadaAt time.Time) SnapshotEnvio {
	c := cfg.Convocatoria
	formulario := FormularioDe(c.Tipo, p.Subcategoria)
	// definidos copia sólo los campos del formulario que tienen valor.
	definidos := func(definiciones []DefinicionCampo, valores map[string]string) map[string]string {
		copia := make(map[string]string)
		for _, def := range definiciones {
			if valor := strings.TrimSpace(valores[def.Ruta]); valor != "" {
				copia[def.Ruta] = valor
			}
		}
		return copia
	}
	s := SnapshotEnvio{
		Folio: folio, Version: version, EnviadaAt: enviadaAt.UTC().Format(formatoTiempo), Idioma: titular.Idioma,
		Edicion:      SnapshotEdicion{Numero: c.Edicion.Numero, Anio: c.Edicion.Anio, NombreES: c.Edicion.NombreES, NombreEN: c.Edicion.NombreEN},
		Convocatoria: SnapshotConvocatoria{ID: c.ID, Tipo: string(c.Tipo), TituloES: c.TituloES, TituloEN: c.TituloEN},
		Subcategoria: string(p.Subcategoria),
		Titular:      SnapshotTitular{Nombre: titular.Nombre, Apellidos: titular.Apellidos, Email: titular.Email},
		Campos:       definidos(formulario.Campos, p.Campos),
		Obras:        []SnapshotObra{},
		Documentos:   []SnapshotDocumento{},
	}
	if s.Idioma != IdiomaEN {
		s.Idioma = IdiomaES
	}
	for _, i := range p.Integrantes {
		s.Integrantes = append(s.Integrantes, SnapshotIntegrante{Nombre: i.Nombre, Instrumento: i.Instrumento})
	}
	for _, v := range p.Videos {
		s.Videos = append(s.Videos, SnapshotVideo{URL: v.URL, Estado: string(v.Estado), Plataforma: v.Plataforma})
	}
	for _, o := range p.Obras {
		obra := SnapshotObra{ID: o.ID, Orden: o.Orden, Campos: definidos(formulario.CamposObra, o.Campos)}
		dotacionID, _ := strconv.ParseInt(o.Campos["obra.dotacion_id"], 10, 64)
		for _, d := range cfg.Dotaciones {
			if d.ID == dotacionID && dotacionID != 0 {
				obra.DotacionES, obra.DotacionEN = d.NombreES, d.NombreEN
			}
		}
		s.Obras = append(s.Obras, obra)
	}
	etiquetas := make(map[string]RequisitoDocumental)
	for _, rq := range cfg.Requisitos {
		if _, visto := etiquetas[rq.Clave]; !visto || rq.DotacionID == 0 {
			etiquetas[rq.Clave] = rq
		}
	}
	for _, d := range p.Documentos {
		rq := etiquetas[d.Clave]
		s.Documentos = append(s.Documentos, SnapshotDocumento{
			Clave: d.Clave, EtiquetaES: rq.EtiquetaES, EtiquetaEN: rq.EtiquetaEN, ObraID: d.ObraID, Version: d.Version,
			SHA256: d.Archivo.SHA256, TamanoBytes: d.Archivo.TamanoBytes, MIME: d.Archivo.MIME,
		})
	}
	return s
}

// --- Recordatorios de borrador (RF-NOT-04) ------------------------------------

// plazosDeRecordatorio son los momentos, antes del cierre, en que se recuerda
// a quien tiene un borrador sin enviar: 7 días, 2 días y 12 horas.
var plazosDeRecordatorio = []time.Duration{7 * 24 * time.Hour, 48 * time.Hour, 12 * time.Hour}

// VentanaDeRecordatorio indica si en este momento toca recordar un borrador
// y, de ser así, desde cuándo corre el recordatorio vigente. Cada recordatorio
// vale desde su plazo hasta el siguiente: si ya se envió un correo dentro de
// esa ventana, no se repite. Un recordatorio cuya ventana pasó con el servicio
// detenido no se envía tarde.
func VentanaDeRecordatorio(termina, ahora time.Time) (inicio time.Time, toca bool) {
	if !ahora.Before(termina) {
		return time.Time{}, false // el plazo ya cerró: no hay nada que recordar
	}
	for i := len(plazosDeRecordatorio) - 1; i >= 0; i-- {
		if inicio = termina.Add(-plazosDeRecordatorio[i]); !ahora.Before(inicio) {
			return inicio, true
		}
	}
	return time.Time{}, false
}

// --- Comprobación de ligas en red (RF-DOC-10) ---------------------------------

// redesNoPublicas son rangos que nunca deben visitarse al comprobar una liga
// y que netip no clasifica por sí mismo: direcciones compartidas de operador,
// de documentación, de pruebas y reservadas.
var redesNoPublicas = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("2001:db8::/32"),
}

// DireccionPublica indica si una dirección IP es de Internet abierto. La
// comprobación de ligas hace que el servidor visite direcciones escritas por
// cualquiera; sin este filtro, una «liga de video» como https://127.0.0.1/ o
// https://10.0.0.5/ serviría para sondear la red interna del servidor (SSRF).
func DireccionPublica(ip netip.Addr) bool {
	ip = ip.Unmap() // una IPv4 escrita como IPv6 (::ffff:10.0.0.1) es la misma IPv4
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, red := range redesNoPublicas {
		if red.Contains(ip) {
			return false
		}
	}
	return true
}

// ConsultaOEmbed devuelve la dirección con que se pregunta a la plataforma de
// una liga si el recurso existe y es público, o "" si la plataforma no ofrece
// esa consulta. oEmbed distingue lo que una petición simple no puede: un video
// privado de YouTube responde 200 con una página de error.
func ConsultaOEmbed(liga string) string {
	consulta := url.QueryEscape(liga)
	switch PlataformaDeVideo(liga) {
	case "youtube":
		return "https://www.youtube.com/oembed?format=json&url=" + consulta
	case "vimeo":
		return "https://vimeo.com/api/oembed.json?url=" + consulta
	case "soundcloud":
		return "https://soundcloud.com/oembed?format=json&url=" + consulta
	}
	return ""
}

// Detalles del resultado de comprobar una liga. Son códigos, no textos: la
// vista los traduce («enlace.detalle.<código>»).
const (
	DetalleEnlaceOK             = "ok"
	DetalleEnlaceNoEncontrado   = "no_encontrado"
	DetalleEnlaceRequiereAcceso = "requiere_acceso"
	DetalleEnlaceDominio        = "dominio_inexistente"
	DetalleEnlaceDireccion      = "direccion_no_permitida"
	DetalleEnlaceNoComprobable  = "no_comprobable"
)

// ClasificarEnlace convierte la respuesta HTTP de una comprobación en el
// estado de la liga. porOEmbed indica que se preguntó a la plataforma y no al
// recurso; pideIngreso, que la respuesta fue una redirección a una pantalla de
// inicio de sesión.
//
// Sólo se da por mala una liga cuando la respuesta es inequívoca (no existe,
// o exige acceso). Todo lo dudoso —un 403 a un programa, un error del
// servidor— queda «pendiente»: la comprobación automática es un semáforo para
// el revisor, no un veredicto, y un falso negativo no debe impedir un envío.
func ClasificarEnlace(porOEmbed bool, estadoHTTP int, pideIngreso bool) ResultadoEnlace {
	r := ResultadoEnlace{Estado: EnlacePendiente, HTTPStatus: estadoHTTP, Detalle: DetalleEnlaceNoComprobable}
	switch {
	case pideIngreso:
		r.Estado, r.Detalle = EnlaceNoPublico, DetalleEnlaceRequiereAcceso
	case estadoHTTP >= 200 && estadoHTTP < 300:
		r.Estado, r.Detalle = EnlaceOK, DetalleEnlaceOK
	case estadoHTTP == 404 || estadoHTTP == 410, porOEmbed && estadoHTTP == 400:
		r.Estado, r.Detalle = EnlaceInaccesible, DetalleEnlaceNoEncontrado
	case estadoHTTP == 401, porOEmbed && estadoHTTP == 403:
		r.Estado, r.Detalle = EnlaceNoPublico, DetalleEnlaceRequiereAcceso
	}
	return r
}

// --- Revisión documental y subsanación (§4.2 y §5.5) --------------------------

const (
	// LongitudMaxMotivo acota los textos del revisor y del postulante en la
	// revisión: motivos, observaciones, justificaciones y la nota del correo.
	LongitudMaxMotivo = 2000
	// PlazoMaxSubsanacion es lo más lejos que puede fijarse la fecha límite
	// de una subsanación.
	PlazoMaxSubsanacion = 90 * 24 * time.Hour
	// MaxProblemasPorSubsanacion acota cuántos elementos se señalan de una vez.
	MaxProblemasPorSubsanacion = 100
	// MaxSolicitudesDeDesbloqueo acota las solicitudes sin resolver de una
	// misma subsanación.
	MaxSolicitudesDeDesbloqueo = 20
)

// PuedeRevisar indica si la sesión puede hacer la revisión documental de una
// convocatoria: sus revisores asignados y el administrador (§2.2).
func PuedeRevisar(s *Sesion, convocatoriaID int64) bool {
	return s != nil && (PuedeAdministrar(s) || s.TieneAsignacion(convocatoriaID, PerfilRevisorDocumental))
}

// PuedeVerRevision indica si la sesión puede ver lo que la revisión ha
// dejado sobre las postulaciones de una convocatoria: quienes revisan y
// quienes supervisan. El postulante no: los dictámenes y las observaciones
// son internos, y de ellos sólo le llega lo que el revisor decide solicitar.
func PuedeVerRevision(s *Sesion, convocatoriaID int64) bool {
	return PuedeRevisar(s, convocatoriaID) || PuedeSupervisar(s)
}

// PermisoDeEdicion dice qué puede modificar el postulante en este momento.
// Es el motor del desbloqueo selectivo (§4.2): en un borrador todo es
// editable; durante una subsanación, sólo lo que el revisor señaló.
type PermisoDeEdicion struct {
	// Total: la postulación es un borrador dentro de plazo.
	Total bool
	// desbloqueados son los elementos editables durante una subsanación.
	desbloqueados map[string]bool
}

// claveDeElemento identifica un elemento de la postulación: un campo o la
// ranura de un documento, de la postulación (obraID 0) o de una obra.
func claveDeElemento(clase ClaseProblema, campo string, obraID int64) string {
	return fmt.Sprintf("%s|%s|%d", clase, campo, obraID)
}

// PermisoDeEdicionDe calcula qué puede modificar el postulante ahora (RN-02 y
// RN-04). Si no puede modificar nada devuelve por qué.
//
//   - Borrador: todo, mientras la convocatoria esté en postulación y en plazo.
//   - En subsanación, sin vencer: sólo los elementos listados en los problemas
//     de la subsanación abierta. No existe otra vía (RN-12b).
//   - Cualquier otro estado: nada.
func PermisoDeEdicionDe(p Postulacion, c Convocatoria, ahora time.Time) (PermisoDeEdicion, []ErrorCampo) {
	switch p.Estado {
	case PostulacionBorrador:
		if errores := EnPlazoDePostulacion(c, ahora); len(errores) > 0 {
			return PermisoDeEdicion{}, errores
		}
		return PermisoDeEdicion{Total: true}, nil
	case PostulacionEnSubsanacion:
		subsanacion := p.SubsanacionAbierta(ahora)
		if subsanacion == nil {
			return PermisoDeEdicion{}, []ErrorCampo{{Campo: "estado", Clave: "error.subsanacion_vencida"}}
		}
		permiso := PermisoDeEdicion{desbloqueados: make(map[string]bool)}
		for _, problema := range subsanacion.Problemas {
			permiso.desbloqueados[claveDeElemento(problema.Clase, problema.Campo, problema.ObraID)] = true
		}
		return permiso, nil
	}
	return PermisoDeEdicion{}, []ErrorCampo{{Campo: "estado", Clave: "error.postulacion_enviada"}}
}

// programaAbierto indica que el revisor autorizó cambiar la lista de obras
// (RN-12c). Entonces también son editables los datos y los documentos de
// cada obra: de nada serviría poder añadir una obra y no poder describirla.
func (pe PermisoDeEdicion) programaAbierto() bool {
	return pe.desbloqueados[claveDeElemento(ProblemaCampo, CampoObras, 0)]
}

// Campo indica si puede modificarse un campo. obraID es 0 para los campos de
// la postulación. Vale también para las listas: CampoIntegrantes, CampoVideos
// y CampoObras.
func (pe PermisoDeEdicion) Campo(ruta string, obraID int64) bool {
	return pe.Total || pe.desbloqueados[claveDeElemento(ProblemaCampo, ruta, obraID)] || (obraID != 0 && pe.programaAbierto())
}

// Documento indica si puede subirse o retirarse el documento de un requisito.
func (pe PermisoDeEdicion) Documento(clave string, obraID int64) bool {
	return pe.Total || pe.desbloqueados[claveDeElemento(ProblemaDocumento, clave, obraID)] || (obraID != 0 && pe.programaAbierto())
}

// validarElemento comprueba que un elemento exista en la postulación: que el
// campo sea de su formulario, o que el requisito se le pida, y que la obra
// sea suya.
func validarElemento(p Postulacion, cfg ConfiguracionConvocatoria, clase ClaseProblema, campo string, obraID int64) []ErrorCampo {
	inexistente := []ErrorCampo{{Campo: "campo", Clave: "error.elemento_inexistente"}}
	formulario := FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria)
	esDeLaPostulacion := obraID == 0 || slices.ContainsFunc(p.Obras, func(o Obra) bool { return o.ID == obraID })
	switch {
	case !esDeLaPostulacion:
		return []ErrorCampo{{Campo: "obra_id", Clave: "error.obra_ajena"}}
	case clase == ProblemaDocumento:
		if _, errores := RequisitoParaDocumento(p, cfg, campo, obraID); len(errores) > 0 {
			return inexistente
		}
		return nil
	case clase != ProblemaCampo:
		return []ErrorCampo{{Campo: "clase", Clave: "error.elemento_inexistente"}}
	case obraID != 0:
		if _, existe := formulario.CampoDeObra(campo); !existe {
			return inexistente
		}
		return nil
	}
	_, esCampo := formulario.Campo(campo)
	esLista := campo == CampoObras || (campo == CampoIntegrantes && formulario.TieneIntegrantes) || (campo == CampoVideos && formulario.TieneVideos)
	if !esCampo && !esLista {
		return inexistente
	}
	return nil
}

// textoRequerido valida un texto libre obligatorio de la revisión.
func textoRequerido(texto, campo string) []ErrorCampo {
	switch largo := utf8.RuneCountInString(strings.TrimSpace(texto)); {
	case largo == 0:
		return []ErrorCampo{{Campo: campo, Clave: "error.texto_requerido"}}
	case largo > LongitudMaxMotivo:
		return []ErrorCampo{{Campo: campo, Clave: "error.texto_largo"}}
	}
	return nil
}

// AdmiteRevision indica si en esta etapa de la convocatoria puede hacerse
// revisión documental: en la etapa de revisión y TAMBIÉN durante la
// evaluación (decisión de la CNMO). La convocatoria pasa a evaluación aunque
// queden postulaciones por subsanar o por dictaminar; se terminan de revisar
// después y, al validarse, se suman a lo que el jurado tiene por calificar.
func AdmiteRevision(estado EstadoConvocatoria) bool {
	return estado == EstadoRevisionDocumental || estado == EstadoEvaluacion
}

// EstadoAlValidar es el estado en que queda una postulación al validarla: si
// la evaluación ya empezó pasa directamente al jurado (RN-20).
func EstadoAlValidar(estado EstadoConvocatoria) EstadoDePostulacion {
	if estado == EstadoEvaluacion {
		return PostulacionEnEvaluacion
	}
	return PostulacionValidada
}

// ValidarAccionDeRevision comprueba que pueda revisarse una postulación: la
// convocatoria debe admitir revisión y la postulación estar enviada y
// sin una subsanación en curso (mientras el postulante corrige, el revisor
// espera).
func ValidarAccionDeRevision(c Convocatoria, p Postulacion) []ErrorCampo {
	if !AdmiteRevision(c.Estado) {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.fuera_de_revision"}}
	}
	if p.Estado != PostulacionEnviada && p.Estado != PostulacionEnRevision {
		return []ErrorCampo{{Campo: "estado", Clave: "error.estado_no_revisable"}}
	}
	return nil
}

// ValidarDictamenDeDocumento: un documento inválido exige motivo (RN-11).
func ValidarDictamenDeDocumento(dictamen, motivo string) []ErrorCampo {
	switch dictamen {
	case "valido", "pendiente":
		return nil
	case "invalido":
		return textoRequerido(motivo, "motivo")
	}
	return []ErrorCampo{{Campo: "dictamen", Clave: "error.opcion_invalida"}}
}

// ValidarObservacion valida una observación del revisor sobre la postulación
// o sobre una de sus obras (RF-REV-04).
func ValidarObservacion(p Postulacion, obraID int64, texto string) []ErrorCampo {
	if obraID != 0 && !slices.ContainsFunc(p.Obras, func(o Obra) bool { return o.ID == obraID }) {
		return []ErrorCampo{{Campo: "obra_id", Clave: "error.obra_ajena"}}
	}
	return textoRequerido(texto, "texto")
}

// ProblemasDeSubsanacion arma la lista definitiva de problemas de una
// subsanación: los que el revisor señala a mano —campos y documentos que
// faltan— más, automáticamente, cada documento vigente que dictaminó como
// inválido, con el motivo de su dictamen. Así «solicitar subsanación» agrupa
// todo lo señalado (RF-REV-05b) sin que el revisor lo repita.
func ProblemasDeSubsanacion(p Postulacion, revision Revision, senalados []ProblemaSubsanacion) []ProblemaSubsanacion {
	problemas := slices.Clone(senalados)
	vistos := make(map[string]bool)
	for _, problema := range problemas {
		vistos[claveDeElemento(problema.Clase, problema.Campo, problema.ObraID)] = true
	}
	motivos := make(map[int64]string)
	for _, dictamen := range revision.Dictamenes {
		motivos[dictamen.DocumentoID] = dictamen.Motivo
	}
	for _, documento := range p.Documentos {
		clave := claveDeElemento(ProblemaDocumento, documento.Clave, documento.ObraID)
		if documento.Dictamen != "invalido" || vistos[clave] {
			continue
		}
		vistos[clave] = true
		problemas = append(problemas, ProblemaSubsanacion{Clase: ProblemaDocumento, Campo: documento.Clave,
			ObraID: documento.ObraID, DocumentoID: documento.ID, Problema: motivos[documento.ID]})
	}
	return problemas
}

// ValidarNuevaSubsanacion valida una solicitud de subsanación ya con su lista
// definitiva de problemas: al menos uno, cada uno sobre un elemento que existe
// y con su motivo, sin repetir, y con una fecha límite futura.
func ValidarNuevaSubsanacion(p Postulacion, cfg ConfiguracionConvocatoria, n NuevaSubsanacion, ahora time.Time) []ErrorCampo {
	if errores := ValidarAccionDeRevision(cfg.Convocatoria, p); len(errores) > 0 {
		return errores
	}
	var errores []ErrorCampo
	if len(n.Problemas) == 0 {
		errores = append(errores, ErrorCampo{Campo: "problemas", Clave: "error.sin_problemas"})
	}
	if len(n.Problemas) > MaxProblemasPorSubsanacion {
		errores = append(errores, ErrorCampo{Campo: "problemas", Clave: "error.demasiados_problemas"})
	}
	vistos := make(map[string]bool)
	for _, problema := range n.Problemas {
		clave := claveDeElemento(problema.Clase, problema.Campo, problema.ObraID)
		elemento := validarElemento(p, cfg, problema.Clase, problema.Campo, problema.ObraID)
		motivo := textoRequerido(problema.Problema, "problema")
		switch {
		case len(elemento) > 0:
			errores = append(errores, elemento...)
		case len(motivo) > 0:
			errores = append(errores, motivo...)
		case vistos[clave]:
			errores = append(errores, ErrorCampo{Campo: "problemas", Clave: "error.problema_repetido"})
		}
		vistos[clave] = true
	}
	if !n.FechaLimite.After(ahora) || n.FechaLimite.After(ahora.Add(PlazoMaxSubsanacion)) {
		errores = append(errores, ErrorCampo{Campo: "fecha_limite", Clave: "error.fecha_limite_invalida"})
	}
	if utf8.RuneCountInString(n.Nota) > LongitudMaxMotivo {
		errores = append(errores, ErrorCampo{Campo: "nota", Clave: "error.texto_largo"})
	}
	return errores
}

// ValidarSolicitudDeDesbloqueo valida la petición de un postulante para
// corregir algo que el revisor no señaló (RN-12d): debe haber una subsanación
// abierta, el elemento debe existir y no estar ya desbloqueado ni pedido, y
// la justificación es obligatoria.
func ValidarSolicitudDeDesbloqueo(p Postulacion, cfg ConfiguracionConvocatoria, s SolicitudDesbloqueo, ahora time.Time) []ErrorCampo {
	subsanacion := p.SubsanacionAbierta(ahora)
	if subsanacion == nil {
		return []ErrorCampo{{Campo: "estado", Clave: "error.sin_subsanacion_abierta"}}
	}
	if errores := validarElemento(p, cfg, s.Clase, s.Campo, s.ObraID); len(errores) > 0 {
		return errores
	}
	if errores := textoRequerido(s.Justificacion, "justificacion"); len(errores) > 0 {
		return errores
	}
	permiso, _ := PermisoDeEdicionDe(p, cfg.Convocatoria, ahora)
	if desbloqueado := permiso.desbloqueados[claveDeElemento(s.Clase, s.Campo, s.ObraID)]; desbloqueado {
		return []ErrorCampo{{Campo: "campo", Clave: "error.ya_desbloqueado"}}
	}
	pendientes := 0
	for _, otra := range subsanacion.Solicitudes {
		if otra.Resuelta {
			continue
		}
		pendientes++
		if otra.Clase == s.Clase && otra.Campo == s.Campo && otra.ObraID == s.ObraID {
			return []ErrorCampo{{Campo: "campo", Clave: "error.solicitud_repetida"}}
		}
	}
	if pendientes >= MaxSolicitudesDeDesbloqueo {
		return []ErrorCampo{{Campo: "campo", Clave: "error.demasiadas_solicitudes"}}
	}
	return nil
}

// ValidarRespuestaDeDesbloqueo: negar una solicitud exige decir por qué.
func ValidarRespuestaDeDesbloqueo(concedida bool, motivo string) []ErrorCampo {
	if concedida {
		if utf8.RuneCountInString(motivo) > LongitudMaxMotivo {
			return []ErrorCampo{{Campo: "motivo", Clave: "error.texto_largo"}}
		}
		return nil
	}
	return textoRequerido(motivo, "motivo")
}

// ValidarDictamenDePostulacion valida el dictamen con que termina la revisión
// de una postulación: validada o descartada (RF-REV-08).
//
//   - Durante la revisión documental o la evaluación (AdmiteRevision).
//   - Descartar exige motivo.
//   - No se valida una postulación con un documento dictaminado inválido.
//   - Mientras el postulante tiene una subsanación abierta no se dictamina:
//     se espera a que reenvíe o a que venza el plazo. Vencida, sí (RF-REV-09).
//   - Un dictamen ya emitido puede corregirse mientras dure la etapa.
func ValidarDictamenDePostulacion(c Convocatoria, p Postulacion, dictamen EstadoDePostulacion, motivo string, ahora time.Time) []ErrorCampo {
	if !AdmiteRevision(c.Estado) {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.fuera_de_revision"}}
	}
	switch p.Estado {
	case PostulacionEnviada, PostulacionEnRevision, PostulacionValidada, PostulacionDescartada:
	case PostulacionEnSubsanacion:
		if !p.SubsanacionVencida(ahora) {
			return []ErrorCampo{{Campo: "estado", Clave: "error.subsanacion_en_curso"}}
		}
	default:
		return []ErrorCampo{{Campo: "estado", Clave: "error.estado_no_revisable"}}
	}
	switch dictamen {
	case PostulacionValidada:
		if slices.ContainsFunc(p.Documentos, func(d Documento) bool { return d.Dictamen == "invalido" }) {
			return []ErrorCampo{{Campo: "dictamen", Clave: "error.documentos_invalidos"}}
		}
		if utf8.RuneCountInString(motivo) > LongitudMaxMotivo {
			return []ErrorCampo{{Campo: "motivo", Clave: "error.texto_largo"}}
		}
		return nil
	case PostulacionDescartada:
		return textoRequerido(motivo, "motivo")
	}
	return []ErrorCampo{{Campo: "dictamen", Clave: "error.opcion_invalida"}}
}

// DiferenciasDeEnvio compara dos envíos de la misma postulación y devuelve lo
// que cambió entre ellos: es lo que el revisor ve al volver una postulación
// subsanada (RF-REV-07). Compara los snapshots, que son lo que de verdad se
// envió cada vez.
func DiferenciasDeEnvio(anterior, nuevo SnapshotEnvio) []CambioDeEnvio {
	var cambios []CambioDeEnvio
	// camposDistintos compara dos mapas de campos y anota las diferencias.
	camposDistintos := func(antes, despues map[string]string, obraID int64) {
		rutas := make(map[string]bool)
		for ruta := range antes {
			rutas[ruta] = true
		}
		for ruta := range despues {
			rutas[ruta] = true
		}
		for _, ruta := range slices.Sorted(maps.Keys(rutas)) {
			if antes[ruta] != despues[ruta] {
				cambios = append(cambios, CambioDeEnvio{Clase: ProblemaCampo, Campo: ruta, ObraID: obraID, Antes: antes[ruta], Despues: despues[ruta]})
			}
		}
	}
	camposDistintos(anterior.Campos, nuevo.Campos, 0)

	// Listas: se comparan enteras, como texto.
	integrantes := func(s SnapshotEnvio) string {
		var lista []string
		for _, i := range s.Integrantes {
			lista = append(lista, i.Nombre+" ("+i.Instrumento+")")
		}
		return strings.Join(lista, "; ")
	}
	videos := func(s SnapshotEnvio) string {
		var lista []string
		for _, v := range s.Videos {
			lista = append(lista, v.URL)
		}
		return strings.Join(lista, " ")
	}
	if a, d := integrantes(anterior), integrantes(nuevo); a != d {
		cambios = append(cambios, CambioDeEnvio{Clase: ProblemaCampo, Campo: CampoIntegrantes, Antes: a, Despues: d})
	}
	if a, d := videos(anterior), videos(nuevo); a != d {
		cambios = append(cambios, CambioDeEnvio{Clase: ProblemaCampo, Campo: CampoVideos, Antes: a, Despues: d})
	}

	// Obras: por su id. Una obra que aparece o desaparece es un cambio del programa.
	obrasAntes := make(map[int64]SnapshotObra)
	for _, obra := range anterior.Obras {
		obrasAntes[obra.ID] = obra
	}
	for _, obra := range nuevo.Obras {
		previa, estaba := obrasAntes[obra.ID]
		delete(obrasAntes, obra.ID)
		if !estaba {
			cambios = append(cambios, CambioDeEnvio{Clase: ProblemaCampo, Campo: CampoObras, ObraID: obra.ID, Despues: obra.Campos["obra.titulo"]})
			continue
		}
		camposDistintos(previa.Campos, obra.Campos, obra.ID)
	}
	for _, id := range slices.Sorted(maps.Keys(obrasAntes)) {
		cambios = append(cambios, CambioDeEnvio{Clase: ProblemaCampo, Campo: CampoObras, ObraID: id, Antes: obrasAntes[id].Campos["obra.titulo"]})
	}

	// Documentos: por requisito y obra; cambia si cambia la suma del archivo.
	sumas := func(s SnapshotEnvio) map[string]SnapshotDocumento {
		porRanura := make(map[string]SnapshotDocumento)
		for _, d := range s.Documentos {
			porRanura[claveDeElemento(ProblemaDocumento, d.Clave, d.ObraID)] = d
		}
		return porRanura
	}
	antes, despues := sumas(anterior), sumas(nuevo)
	ranuras := make(map[string]bool)
	for ranura := range antes {
		ranuras[ranura] = true
	}
	for ranura := range despues {
		ranuras[ranura] = true
	}
	for _, ranura := range slices.Sorted(maps.Keys(ranuras)) {
		a, d := antes[ranura], despues[ranura]
		if a.SHA256 == d.SHA256 {
			continue
		}
		referencia := d
		if referencia.Clave == "" {
			referencia = a
		}
		cambios = append(cambios, CambioDeEnvio{Clase: ProblemaDocumento, Campo: referencia.Clave, ObraID: referencia.ObraID,
			Antes: inicioDeSuma(a.SHA256), Despues: inicioDeSuma(d.SHA256)})
	}
	return cambios
}

// inicioDeSuma devuelve los primeros 12 caracteres de un SHA-256, que bastan
// para distinguir a la vista un archivo de otro.
func inicioDeSuma(suma string) string {
	if len(suma) > 12 {
		return suma[:12]
	}
	return suma
}

// EstadoParaTitular devuelve el estado de una postulación tal como se le
// muestra a su titular. El descarte es un estado INTERNO de la revisión
// (decisión de la CNMO): al postulante no se le muestra, y su postulación
// sigue apareciéndole «en revisión». Los demás estados se muestran tal cual.
func EstadoParaTitular(estado EstadoDePostulacion) EstadoDePostulacion {
	if estado == PostulacionDescartada {
		return PostulacionEnRevision
	}
	return estado
}

// --- Evaluación por jurado (§5.6) ---------------------------------------------

// EnEvaluacion indica si una postulación está a la vista del jurado: pasó a
// evaluación o ya fue evaluada por los tres.
func EnEvaluacion(estado EstadoDePostulacion) bool {
	return estado == PostulacionEnEvaluacion || estado == PostulacionEvaluada
}

// PuedeEvaluar indica si la sesión es de un jurado de la convocatoria. El
// administrador NO puede: no emite ni altera evaluaciones en nombre de un
// jurado (§2.2).
func PuedeEvaluar(s *Sesion, convocatoriaID int64) bool {
	return s != nil && s.TieneAsignacion(convocatoriaID, PerfilJurado)
}

// UnidadesDeEvaluacion devuelve las unidades que se califican de una
// postulación, como ids de obra (RN-16): en Intérpretes una sola, la
// postulación completa (0); en Compositores, cada obra.
func UnidadesDeEvaluacion(tipo TipoConvocatoria, p Postulacion) []int64 {
	if tipo == TipoInterpretes {
		return []int64{0}
	}
	unidades := make([]int64, 0, len(p.Obras))
	for _, obra := range p.Obras {
		unidades = append(unidades, obra.ID)
	}
	return unidades
}

// ValidarEvaluacion valida lo que un jurado quiere guardar.
//
//   - Sólo durante la etapa de evaluación (RN-19): cerrada, queda congelada.
//   - Sólo sobre postulaciones que pasaron a evaluación (RN-20), y sobre una
//     unidad que exista en ellas.
//   - Cada puntaje es de un criterio de la convocatoria y cae en su escala (RN-17).
//   - Un borrador puede estar incompleto. Para enviarla —o para modificar una
//     ya enviada— deben estar todos los criterios obligatorios.
func ValidarEvaluacion(cfg ConfiguracionConvocatoria, p Postulacion, e EntradaEvaluacion, yaEnviada bool) []ErrorCampo {
	if cfg.Convocatoria.Estado != EstadoEvaluacion {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.fuera_de_evaluacion"}}
	}
	if !EnEvaluacion(p.Estado) {
		return []ErrorCampo{{Campo: "estado", Clave: "error.no_esta_en_evaluacion"}}
	}
	if !slices.Contains(UnidadesDeEvaluacion(cfg.Convocatoria.Tipo, p), e.ObraID) {
		return []ErrorCampo{{Campo: "obra_id", Clave: "error.unidad_inexistente"}}
	}
	var errores []ErrorCampo
	criterios := make(map[string]CriterioEvaluacion)
	for _, criterio := range cfg.Criterios {
		criterios[criterio.Clave] = criterio
	}
	for _, clave := range slices.Sorted(maps.Keys(e.Puntajes)) {
		criterio, existe := criterios[clave]
		switch puntaje := e.Puntajes[clave]; {
		case !existe:
			errores = append(errores, ErrorCampo{Campo: clave, Clave: "error.criterio_inexistente"})
		case puntaje < criterio.EscalaMin || puntaje > criterio.EscalaMax:
			errores = append(errores, ErrorCampo{Campo: clave, Clave: "error.puntaje_fuera_de_escala"})
		}
	}
	if e.Enviar || yaEnviada {
		for _, criterio := range cfg.Criterios {
			if _, calificado := e.Puntajes[criterio.Clave]; criterio.Obligatorio && !calificado {
				errores = append(errores, ErrorCampo{Campo: criterio.Clave, Clave: "error.puntaje_requerido"})
			}
		}
	}
	if utf8.RuneCountInString(e.Observaciones) > LongitudMaxMotivo {
		errores = append(errores, ErrorCampo{Campo: "observaciones", Clave: "error.texto_largo"})
	}
	return errores
}

// ValidarDictamenFinal valida el resultado que el administrador registra para
// una postulación. Sólo sobre postulaciones que llegaron a evaluación, y con
// la evaluación en curso o ya cerrada; en el archivo, no (RF-ARCH-03).
func ValidarDictamenFinal(c Convocatoria, p Postulacion, dictamen string) []ErrorCampo {
	if c.Estado != EstadoEvaluacion && c.Estado != EstadoCerrada {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.fuera_de_evaluacion"}}
	}
	if !EnEvaluacion(p.Estado) {
		return []ErrorCampo{{Campo: "estado", Clave: "error.no_esta_en_evaluacion"}}
	}
	if dictamen != "" && dictamen != DictamenSeleccionada && dictamen != DictamenNoSeleccionada && dictamen != DictamenListaDeEspera {
		return []ErrorCampo{{Campo: "dictamen", Clave: "error.opcion_invalida"}}
	}
	return nil
}

// --- Excusa por conflicto de interés (RF-EVA-12) ------------------------------

// UnidadesParaExcusa devuelve las unidades de las que un jurado se excusa. En
// Intérpretes, la postulación. En Compositores, la obra indicada o, si no se
// indica ninguna, todas las de la postulación: el conflicto suele ser con la
// persona, no con una obra.
func UnidadesParaExcusa(tipo TipoConvocatoria, p Postulacion, obraID int64) []int64 {
	unidades := UnidadesDeEvaluacion(tipo, p)
	if tipo == TipoCompositores && obraID == 0 {
		return unidades
	}
	if slices.Contains(unidades, obraID) {
		return []int64{obraID}
	}
	return nil
}

// ValidarExcusa valida que un jurado se excuse de evaluar: durante la etapa
// de evaluación, sobre una postulación a su vista, de una unidad que exista,
// y diciendo por qué.
func ValidarExcusa(cfg ConfiguracionConvocatoria, p Postulacion, obraID int64, motivo string) []ErrorCampo {
	if cfg.Convocatoria.Estado != EstadoEvaluacion {
		return []ErrorCampo{{Campo: "convocatoria", Clave: "error.fuera_de_evaluacion"}}
	}
	if !EnEvaluacion(p.Estado) {
		return []ErrorCampo{{Campo: "estado", Clave: "error.no_esta_en_evaluacion"}}
	}
	if len(UnidadesParaExcusa(cfg.Convocatoria.Tipo, p, obraID)) == 0 {
		return []ErrorCampo{{Campo: "obra_id", Clave: "error.unidad_inexistente"}}
	}
	return textoRequerido(motivo, "motivo")
}

// ValidarSinExcusa impide calificar una unidad de la que el jurado se excusó:
// primero debe retirar la excusa. Evita que una calificación se cuele por
// descuido donde se declaró un conflicto de interés.
func ValidarSinExcusa(previa Evaluacion) []ErrorCampo {
	if previa.Excusado {
		return []ErrorCampo{{Campo: "excusa", Clave: "error.excusa_vigente"}}
	}
	return nil
}

// PuntajeComparable convierte la suma de una unidad en un puntaje que puede
// compararse entre unidades aunque no las hayan calificado los mismos
// jurados: el promedio por jurado, llevado a la cuota de tres.
//
// Es lo que hace que una excusa no perjudique a nadie (decisión de la CNMO):
// si un jurado se excusa, la suma de los otros dos sería menor que la de tres
// sin que la propuesta sea peor. Con dos jurados que suman 16, el puntaje
// comparable es 24: lo que habrían sumado tres con el mismo criterio.
func PuntajeComparable(total, juradosQueEvaluaron int) float64 {
	if juradosQueEvaluaron == 0 {
		return 0
	}
	return math.Round(float64(total)*float64(CuotaJurados)/float64(juradosQueEvaluaron)*100) / 100
}

// --- Archivo histórico, búsqueda y descargas (§5.12) --------------------------

const (
	// TamanoDePaginaPorOmision y TamanoDePaginaMax acotan la paginación del
	// listado del archivo (RF-ARCH-16).
	TamanoDePaginaPorOmision = 50
	TamanoDePaginaMax        = 200
	// VigenciaDeExportacion es cuánto vive el enlace de una descarga masiva
	// ya armada (RF-ARCH-35).
	VigenciaDeExportacion = 7 * 24 * time.Hour
	// MaxVistasGuardadas acota las vistas guardadas por cuenta.
	MaxVistasGuardadas = 50
	// MaxSeleccion acota cuántas postulaciones pueden marcarse a mano.
	MaxSeleccion = 500
)

// OrdenesDeListado son las columnas por las que puede ordenarse el listado
// (RF-ARCH-15). El puntaje no está aquí: cruza a los jurados y sólo existe en
// la matriz de resultados, que es de supervisión.
var OrdenesDeListado = []string{"folio", "nombre", "enviada", "estado", "obras", "duracion"}

// AlcanceDe calcula qué puede ver una sesión en el archivo. El postulante no
// lo consulta: tiene su propia lista.
func AlcanceDe(s *Sesion) AlcanceDeConsulta {
	var alcance AlcanceDeConsulta
	if s == nil {
		return alcance
	}
	alcance.Todo = PuedeSupervisar(s)
	for _, asignacion := range s.Asignaciones {
		switch asignacion.Perfil {
		case PerfilRevisorDocumental:
			alcance.ComoRevisor = append(alcance.ComoRevisor, asignacion.ConvocatoriaID)
		case PerfilJurado:
			alcance.ComoJurado = append(alcance.ComoJurado, asignacion.ConvocatoriaID)
		}
	}
	return alcance
}

// PuedeDescargarEnMasa indica si la sesión puede pedir un ZIP de documentos
// de varias postulaciones: administrador, observador y revisores (§2.2). El
// jurado no; el postulante sólo baja lo suyo.
func PuedeDescargarEnMasa(s *Sesion) bool {
	alcance := AlcanceDe(s)
	return alcance.Todo || len(alcance.ComoRevisor) > 0
}

// ValidarFiltroDeListado comprueba los valores del listado que no son texto
// libre, y fija los que faltan: orden por folio y página de 50.
func ValidarFiltroDeListado(f *FiltroDeListado) []ErrorCampo {
	var errores []ErrorCampo
	invalido := func(campo string) {
		errores = append(errores, ErrorCampo{Campo: campo, Clave: "error.filtro_invalido"})
	}
	if f.Orden == "" {
		f.Orden = "folio"
	}
	if !slices.Contains(OrdenesDeListado, f.Orden) {
		invalido("orden")
	}
	if f.Pagina < 0 {
		invalido("pagina")
	}
	if f.Pagina > 0 && f.Tamano == 0 {
		f.Tamano = TamanoDePaginaPorOmision
	}
	if f.Tamano < 0 || f.Tamano > TamanoDePaginaMax {
		invalido("tamano")
	}
	for campo, valor := range map[string]string{"con_subsanaciones": f.ConSubsanaciones, "documentos_pendientes": f.DocumentosPendientes, "ligas_con_problema": f.LigasConProblema} {
		if valor != "" && valor != "si" && valor != "no" {
			invalido(campo)
		}
	}
	if f.Tipo != "" && !f.Tipo.Valido() {
		invalido("tipo")
	}
	if !f.EnviadaDesde.IsZero() && !f.EnviadaHasta.IsZero() && f.EnviadaHasta.Before(f.EnviadaDesde) {
		invalido("enviada_hasta")
	}
	if len(f.IDs) > MaxSeleccion {
		invalido("ids")
	}
	return errores
}

// TextoDeBusqueda reúne lo que se indexa de una postulación para el buscador
// global (RF-ARCH-10): folio, nombres, nombre artístico, ensamble y sus
// integrantes, títulos de obra, compositores y correos.
func TextoDeBusqueda(p Postulacion, titular Usuario) string {
	partes := []string{p.Folio, titular.Nombre, titular.Apellidos, titular.Email}
	for _, ruta := range []string{"datos.nombre_completo", "datos.nombre_artistico", "datos.nombre_ensamble", "datos.representante", "datos.email_contacto"} {
		partes = append(partes, p.Campos[ruta])
	}
	for _, integrante := range p.Integrantes {
		partes = append(partes, integrante.Nombre)
	}
	for _, obra := range p.Obras {
		partes = append(partes, obra.Campos["obra.titulo"], obra.Campos["obra.compositor"])
	}
	return strings.Join(slices.DeleteFunc(partes, func(parte string) bool { return strings.TrimSpace(parte) == "" }), " ")
}

// palabrasDeBusqueda parte lo que escribió quien busca en palabras: sólo
// letras y dígitos. Todo lo demás —comillas, asteriscos, paréntesis— se
// descarta, de modo que nada de lo escrito se interpreta como sintaxis del
// buscador.
func palabrasDeBusqueda(texto string) []string {
	return strings.FieldsFunc(strings.ToLower(texto), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// ConsultasDeBusqueda convierte lo que se escribió en las dos consultas del
// buscador (RF-ARCH-11):
//
//   - porPalabras: todas las palabras, cada una como inicio de palabra, sin
//     distinguir acentos ni mayúsculas («nunez carr» encuentra «Núñez Carrillo»).
//   - porSubcadena: todas las palabras como fragmento en cualquier lugar, para
//     el nombre mal recordado («arteto» encuentra «Cuarteto»). Los fragmentos
//     de menos de tres letras no pueden buscarse así y se omiten.
//
// Si no queda nada que buscar devuelve cadenas vacías.
func ConsultasDeBusqueda(texto string) (porPalabras, porSubcadena string) {
	var prefijos, fragmentos []string
	for _, palabra := range palabrasDeBusqueda(texto) {
		prefijos = append(prefijos, `"`+palabra+`"*`)
		if utf8.RuneCountInString(palabra) >= 3 {
			fragmentos = append(fragmentos, `"`+palabra+`"`)
		}
	}
	return strings.Join(prefijos, " "), strings.Join(fragmentos, " ")
}

// sinAcentos son las letras con diacrítico habituales en nombres y títulos,
// con la letra básica que les corresponde en un nombre de archivo.
var sinAcentos = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n", "ç", "c",
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N", "Ç", "C",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u", "â", "a", "ê", "e", "î", "i", "ô", "o", "û", "u",
	"ä", "a", "ë", "e", "ï", "i", "ö", "o", "ã", "a", "õ", "o", "å", "a", "ø", "o", "ß", "ss",
)

// NombreParaArchivo normaliza un texto para usarlo como nombre de archivo o
// de carpeta dentro de un ZIP (RF-ARCH-33): sin acentos, sin espacios y sin
// ningún carácter que Windows, macOS o Linux rechacen. Lo que no es letra,
// dígito, guion o punto se vuelve guion bajo. Se acota a 60 caracteres.
func NombreParaArchivo(texto string) string {
	var limpio strings.Builder
	for _, r := range sinAcentos.Replace(strings.TrimSpace(texto)) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			limpio.WriteRune(r)
		default:
			limpio.WriteByte('_')
		}
	}
	nombre := strings.Trim(regexp.MustCompile(`_+`).ReplaceAllString(limpio.String(), "_"), "_")
	if len(nombre) > 60 {
		nombre = strings.TrimRight(nombre[:60], "_")
	}
	if nombre == "" {
		return "sin_nombre"
	}
	return nombre
}

// RutaEnZIP da el lugar de un documento dentro de una descarga masiva:
//
//	FIM49-COM-0007_Maria_Nunez/fotografia.jpg
//	FIM49-COM-0007_Maria_Nunez/obra-02_Rapsodia/partitura.pdf
//
// Una carpeta por postulación y, dentro, una por obra: un ZIP con trescientos
// archivos llamados «partitura.pdf» sería inservible. Si un requisito admite
// varios archivos, del segundo en adelante llevan su número.
func RutaEnZIP(d DocumentoEnZIP, repetido int) string {
	ruta := NombreParaArchivo(d.Folio+"_"+d.Nombre) + "/"
	if d.Documento.ObraID != 0 {
		ruta += fmt.Sprintf("obra-%02d_%s/", d.OrdenDeObra, NombreParaArchivo(d.Obra))
	}
	nombre := NombreParaArchivo(d.Documento.Clave)
	if repetido > 1 {
		nombre += fmt.Sprintf("-%d", repetido)
	}
	return ruta + nombre + ExtensionDeMIME(d.Documento.Archivo.MIME)
}

// ValidarVistaGuardada valida una vista guardada: un nombre, y una ruta del
// propio sitio (nunca una dirección externa a la que alguien pudiera ser
// llevado al abrirla).
func ValidarVistaGuardada(nombre, ruta string, existentes int) []ErrorCampo {
	var errores []ErrorCampo
	if largo := utf8.RuneCountInString(strings.TrimSpace(nombre)); largo == 0 || largo > 100 {
		errores = append(errores, ErrorCampo{Campo: "nombre", Clave: "error.nombre_de_vista"})
	}
	if !strings.HasPrefix(ruta, "/") || strings.HasPrefix(ruta, "//") || strings.ContainsAny(ruta, "\\\r\n") || len(ruta) > 2000 {
		errores = append(errores, ErrorCampo{Campo: "ruta", Clave: "error.ruta_de_vista"})
	}
	if existentes >= MaxVistasGuardadas {
		errores = append(errores, ErrorCampo{Campo: "nombre", Clave: "error.demasiadas_vistas"})
	}
	return errores
}
