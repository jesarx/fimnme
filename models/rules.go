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
	"net/mail"
	"regexp"
	"slices"
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
			{Clave: "particellas", EtiquetaES: "Particellas", EtiquetaEN: "Parts", Ambito: AmbitoObra,
				Obligatorio: true, MimesPermitidos: pdf, TamanoMaxBytes: megabytes(30), MaxArchivos: 1, Orden: 3},
			{Clave: "audio", EtiquetaES: "Archivo de audio", EtiquetaEN: "Audio file", Ambito: AmbitoObra,
				Obligatorio: false, MimesPermitidos: []string{"audio/wav", "audio/flac", "audio/mpeg"},
				TamanoMaxBytes: megabytes(100), MaxArchivos: 1, Orden: 4},
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
