// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

// Package models contiene el Modelo de la arquitectura MVC: entidades de
// dominio (este archivo), reglas de negocio (rules.go) y, más adelante, la
// persistencia (repository.go).
package models

import (
	"context"
	"errors"
	"time"
)

// Errores de dominio. Los controladores los distinguen con errors.Is para
// decidir la respuesta; cualquier otro error es un fallo interno (500).
var (
	// ErrNoEncontrado: el registro no existe o no es visible para quien lo pide.
	ErrNoEncontrado = errors.New("registro no encontrado")
	// ErrEmailEnUso: ya existe una cuenta con ese correo.
	ErrEmailEnUso = errors.New("el correo ya está en uso")
	// ErrTokenInvalido: el enlace de un solo uso no existe, venció o ya se usó.
	ErrTokenInvalido = errors.New("enlace inválido, vencido o ya utilizado")
)

// Idiomas de primera clase de la plataforma (RF-I18N-01).
const (
	IdiomaES = "es"
	IdiomaEN = "en"
)

// TipoConvocatoria distingue las dos convocatorias de cada edición.
type TipoConvocatoria string

const (
	TipoInterpretes  TipoConvocatoria = "interpretes"
	TipoCompositores TipoConvocatoria = "compositores"
)

// Valido informa si el tipo es uno de los dos admitidos por el dominio.
func (t TipoConvocatoria) Valido() bool {
	return t == TipoInterpretes || t == TipoCompositores
}

// EstadoConvocatoria es la etapa del ciclo de vida de una convocatoria.
// La etapa es propiedad de la convocatoria, no de la edición: las dos
// convocatorias de un mismo año pueden llevar calendarios distintos.
type EstadoConvocatoria string

const (
	EstadoBorrador           EstadoConvocatoria = "borrador"
	EstadoPublicada          EstadoConvocatoria = "publicada"
	EstadoPostulacion        EstadoConvocatoria = "postulacion"
	EstadoRevisionDocumental EstadoConvocatoria = "revision_documental"
	EstadoEvaluacion         EstadoConvocatoria = "evaluacion"
	EstadoCerrada            EstadoConvocatoria = "cerrada"
	EstadoArchivada          EstadoConvocatoria = "archivada"
)

// ClaveEtapa identifica cada tramo del calendario de una convocatoria.
type ClaveEtapa string

const (
	EtapaPostulacion        ClaveEtapa = "postulacion"
	EtapaRevisionDocumental ClaveEtapa = "revision_documental"
	EtapaEvaluacion         ClaveEtapa = "evaluacion"
)

// Edicion es el año del Foro: contenedor de las convocatorias de ese año.
type Edicion struct {
	ID       int64
	Numero   int // p. ej. 49
	Anio     int // p. ej. 2027
	NombreES string
	NombreEN string
}

// Nombre devuelve el nombre de la edición en el idioma pedido. Si falta la
// versión en inglés se entrega la española (RF-I18N-04).
func (e Edicion) Nombre(idioma string) string {
	if idioma == IdiomaEN && e.NombreEN != "" {
		return e.NombreEN
	}
	return e.NombreES
}

// Etapa es un tramo del calendario. Las marcas de tiempo están SIEMPRE en UTC
// (RN-36); la conversión a America/Mexico_City ocurre en la vista.
type Etapa struct {
	Clave     ClaveEtapa
	IniciaAt  time.Time
	TerminaAt time.Time
}

// Convocatoria es una de las dos llamadas de cada edición, con sus bases,
// fechas y configuración propias.
type Convocatoria struct {
	ID        int64
	Edicion   Edicion
	Tipo      TipoConvocatoria
	TituloES  string
	TituloEN  string
	BasesMDES string // bases en Markdown, español
	BasesMDEN string // bases en Markdown, inglés
	Estado    EstadoConvocatoria
	MaxObras  int // 0 = sin límite (Intérpretes); Compositores: 2
	MaxVideos int
	Etapas    []Etapa
}

// Titulo devuelve el título en el idioma pedido, con respaldo en español.
func (c Convocatoria) Titulo(idioma string) string {
	if idioma == IdiomaEN && c.TituloEN != "" {
		return c.TituloEN
	}
	return c.TituloES
}

// BasesMD devuelve las bases en el idioma pedido. El segundo valor indica que
// se entregó la versión española como respaldo porque falta la inglesa
// (RF-I18N-04), para que la vista pueda advertirlo.
func (c Convocatoria) BasesMD(idioma string) (texto string, respaldo bool) {
	if idioma == IdiomaEN {
		if c.BasesMDEN != "" {
			return c.BasesMDEN, false
		}
		return c.BasesMDES, c.BasesMDES != ""
	}
	return c.BasesMDES, false
}

// Etapa busca un tramo del calendario por su clave.
func (c Convocatoria) Etapa(clave ClaveEtapa) (Etapa, bool) {
	for _, e := range c.Etapas {
		if e.Clave == clave {
			return e, true
		}
	}
	return Etapa{}, false
}

// Actor identifica a quien ejecuta una operación con efectos, para dejarla
// asentada en la bitácora de auditoría (RF-AUD-02).
type Actor struct {
	UsuarioID int64  // 0 = sin cuenta de usuario (p. ej. fimnme-cli)
	Perfil    string // perfil con el que actúa: admin_sistema, jurado, cli…
	IP        string
	Agente    string // user-agent del navegador, o identificación de fimnme-cli
}

// DatosRegistro son los datos del autorregistro de un postulante (RF-AUT-01).
// Sólo se piden correo, nombre, apellidos, contraseña e idioma: los datos
// artísticos pertenecen a la postulación, no a la cuenta.
type DatosRegistro struct {
	Email       string
	Nombre      string
	Apellidos   string
	Password    string
	Idioma      string
	AceptaAviso bool // aceptación del aviso de privacidad (LFPDPPP)
}

// ErrorCampo describe un problema de validación en un campo de formulario.
// Clave es una clave de traducción (views/i18n), nunca un texto visible: así
// ninguna cadena para el usuario queda escrita en el código (RF-I18N-02).
type ErrorCampo struct {
	Campo string
	Clave string
}

// ---------------------------------------------------------------------------
// Cuentas, perfiles y sesiones
// ---------------------------------------------------------------------------

// Perfil es un perfil del motor de control de acceso (ACL híbrido, §2).
type Perfil string

const (
	// Perfiles globales, registrados en usuario_perfil.
	PerfilAdminSistema         Perfil = "admin_sistema"
	PerfilCoordinacionNacional Perfil = "coordinacion_nacional"
	PerfilPostulante           Perfil = "postulante"
	// Perfiles operativos, SIEMPRE acotados a una convocatoria (tabla
	// asignacion): haber sido jurado un año no da acceso perpetuo.
	PerfilRevisorDocumental Perfil = "revisor_documental"
	PerfilJurado            Perfil = "jurado"
)

// Perfiles con los que se asientan en la bitácora las operaciones que no
// ejecuta una cuenta de usuario.
const (
	ActorAnonimo = "anonimo" // visitante sin sesión (registro, recuperación…)
	ActorSistema = "sistema" // trabajadores internos de fimnme-web
	ActorCLI     = "cli"     // fimnme-cli
)

// EsGlobal indica si el perfil se otorga para todo el sistema.
func (p Perfil) EsGlobal() bool {
	return p == PerfilAdminSistema || p == PerfilCoordinacionNacional || p == PerfilPostulante
}

// Usuario es una cuenta. Persiste entre ediciones: la misma persona postula
// cada año sin volver a registrarse (RF-AUT-05).
type Usuario struct {
	ID                int64
	Email             string
	Nombre            string
	Apellidos         string
	Idioma            string
	EmailVerificadoAt time.Time // valor cero = correo sin verificar
	Activo            bool
	CreadoAt          time.Time
}

// EmailVerificado indica si la cuenta confirmó su correo. Sin verificar se
// puede capturar una postulación, pero no enviarla (RF-AUT-02).
func (u Usuario) EmailVerificado() bool {
	return !u.EmailVerificadoAt.IsZero()
}

// Asignacion es un perfil operativo vigente en una convocatoria concreta.
type Asignacion struct {
	ConvocatoriaID int64
	Perfil         Perfil
}

// Sesion es una sesión iniciada, con todo lo necesario para decidir permisos
// en cada petición: la cuenta, sus perfiles globales y sus asignaciones.
type Sesion struct {
	ID           string // resumen del identificador; el valor en claro sólo vive en la cookie
	Usuario      Usuario
	Perfiles     []Perfil     // globales
	Asignaciones []Asignacion // operativos activos, por convocatoria
	ExpiraAt     time.Time
}

// TienePerfil indica si la cuenta tiene un perfil GLOBAL.
func (s *Sesion) TienePerfil(perfil Perfil) bool {
	for _, p := range s.Perfiles {
		if p == perfil {
			return true
		}
	}
	return false
}

// TieneAsignacion indica si la cuenta tiene un perfil operativo activo en una
// convocatoria. Es la única vía por la que un jurado o un revisor accede a algo.
func (s *Sesion) TieneAsignacion(convocatoriaID int64, perfil Perfil) bool {
	for _, a := range s.Asignaciones {
		if a.ConvocatoriaID == convocatoriaID && a.Perfil == perfil {
			return true
		}
	}
	return false
}

// PerfilPrincipal es el perfil con el que se asientan en la bitácora las
// acciones de la cuenta cuando no dependen de una convocatoria.
func (s *Sesion) PerfilPrincipal() Perfil {
	for _, p := range []Perfil{PerfilAdminSistema, PerfilCoordinacionNacional} {
		if s.TienePerfil(p) {
			return p
		}
	}
	return PerfilPostulante
}

// PropositoToken distingue los enlaces de un solo uso.
type PropositoToken string

const (
	TokenVerificarEmail PropositoToken = "verificar_email"
	TokenResetPassword  PropositoToken = "reset_password"
	TokenActivacion     PropositoToken = "activacion"
)

// NuevoToken es un enlace de un solo uso por emitir. Sólo lleva el resumen:
// el valor en claro nunca llega al repositorio.
type NuevoToken struct {
	Resumen   string
	Proposito PropositoToken
	ExpiraAt  time.Time
}

// NuevoUsuario son los datos para dar de alta una cuenta.
type NuevoUsuario struct {
	Email        string
	Nombre       string
	Apellidos    string
	Idioma       string
	PasswordHash string // resumen Argon2id, o PasswordSinDefinir
	AvisoVersion string // versión del aviso de privacidad aceptada; "" si no aplica
	Perfil       Perfil // perfil global inicial
}

// ---------------------------------------------------------------------------
// Correo saliente
// ---------------------------------------------------------------------------

// Claves de las plantillas de correo del módulo de cuentas.
const (
	PlantillaVerificarEmail = "verificar_email"
	PlantillaResetPassword  = "reset_password"
	PlantillaActivacion     = "activacion"
	PlantillaCambioEmail    = "cambio_email"
)

// PlantillaCorreo es la versión activa de una plantilla editable (RF-NOT).
type PlantillaCorreo struct {
	Clave    string
	Idioma   string
	Version  int
	Asunto   string
	CuerpoMD string
}

// CorreoSaliente es un mensaje de la bandeja de salida (email_outbox). La
// transacción de negocio lo escribe; un trabajador lo envía después (§5.8).
type CorreoSaliente struct {
	ID             int64
	Destinatario   string
	Idioma         string
	Asunto         string
	HTML           string
	Texto          string
	PlantillaClave string
	Intentos       int
}

// Mailer es el transporte de correo. La lógica de negocio sólo conoce esta
// interfaz: sustituir el SMTP propio por otro transporte no la toca (§5.9).
type Mailer interface {
	// Enviar entrega el mensaje y devuelve su Message-ID.
	Enviar(ctx context.Context, correo CorreoSaliente) (messageID string, err error)
}

// ---------------------------------------------------------------------------
// Consulta de la bitácora
// ---------------------------------------------------------------------------

// AsientoAuditoria es una fila de la bitácora, tal como se consulta.
type AsientoAuditoria struct {
	ID          int64
	ActorID     int64 // 0 = sin cuenta
	ActorPerfil string
	Accion      string
	Entidad     string
	EntidadID   int64  // 0 = sin entidad concreta
	Antes       string // JSON, o ""
	Despues     string // JSON, o ""
	IP          string
	Agente      string
	CreadoAt    time.Time
}

// FiltroAuditoria acota una consulta de la bitácora (RF-AUD-03). Los campos
// vacíos no filtran.
type FiltroAuditoria struct {
	Accion    string
	Entidad   string
	EntidadID int64
	ActorID   int64
	Desde     time.Time // inclusive
	Hasta     time.Time // exclusive
	Pagina    int       // desde 1
	Tamano    int
}
