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

// Package models contiene el Modelo de la arquitectura MVC: entidades de
// dominio (este archivo), reglas de negocio (rules.go) y, más adelante, la
// persistencia (repository.go).
package models

import (
	"context"
	"errors"
	"strings"
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
	// DuracionMaxProgramaMin es el límite de duración del programa, en
	// minutos; 0 = sin límite (decisión de la v1, D-11).
	DuracionMaxProgramaMin int
	MaxVideos              int
	Etapas                 []Etapa
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
	// Perfil es el perfil global inicial. Vacío para una cuenta que sólo
	// tendrá perfiles por convocatoria (jurado o revisor).
	Perfil Perfil
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

// ---------------------------------------------------------------------------
// Configuración de una convocatoria
// ---------------------------------------------------------------------------

// ErrDuplicado indica que ya existe un registro con la misma identidad (el
// número de una edición, el tipo de convocatoria dentro de una edición, la
// clave de un requisito…). Los controladores responden 409.
var ErrDuplicado = errors.New("ya existe un registro con esos datos")

// ErrorValidacion es el error que devuelve el repositorio cuando una operación
// viola una regla de negocio. Lleva la lista de problemas, con claves de
// traducción, para que la respuesta los explique campo por campo (422).
type ErrorValidacion struct {
	Errores []ErrorCampo
}

func (e *ErrorValidacion) Error() string {
	partes := make([]string, 0, len(e.Errores))
	for _, c := range e.Errores {
		partes = append(partes, c.Campo+": "+c.Clave)
	}
	return "datos inválidos (" + strings.Join(partes, "; ") + ")"
}

// Dotacion es una plantilla instrumental del catálogo anual de Compositores.
type Dotacion struct {
	ID             int64
	ConvocatoriaID int64
	NombreES       string
	NombreEN       string
	DescripcionES  string
	DescripcionEN  string
	Orden          int
	Activa         bool
}

// AmbitoRequisito indica a qué se adjunta un documento.
type AmbitoRequisito string

const (
	AmbitoPostulacion AmbitoRequisito = "postulacion" // uno por postulación (fotografía)
	AmbitoObra        AmbitoRequisito = "obra"        // uno por cada obra (partitura…)
)

// RequisitoDocumental define un documento que se pide, dónde, y si es
// obligatorio. DotacionID 0 es la regla general de la convocatoria; mayor que
// cero, la regla específica de esa dotación, que sobrescribe a la general.
type RequisitoDocumental struct {
	ID              int64
	ConvocatoriaID  int64
	DotacionID      int64
	Clave           string // fotografia, partitura, autorizacion, particellas, audio…
	EtiquetaES      string
	EtiquetaEN      string
	AyudaES         string
	AyudaEN         string
	Ambito          AmbitoRequisito
	Obligatorio     bool
	MimesPermitidos []string
	TamanoMaxBytes  int64
	MaxArchivos     int
	Orden           int
}

// CriterioEvaluacion es un criterio con el que el jurado califica.
type CriterioEvaluacion struct {
	ID             int64
	ConvocatoriaID int64
	Clave          string // realizacion, artistico…
	EtiquetaES     string
	EtiquetaEN     string
	DescripcionES  string
	DescripcionEN  string
	EscalaMin      int
	EscalaMax      int
	Obligatorio    bool
	Orden          int
}

// AsignacionDetalle es un jurado o revisor de una convocatoria, con su cuenta.
type AsignacionDetalle struct {
	ID             int64
	ConvocatoriaID int64
	Perfil         Perfil
	Activo         bool
	Usuario        Usuario
	CuentaActivada bool // la persona ya eligió su contraseña
}

// ConfiguracionConvocatoria reúne todo lo configurable de una convocatoria.
// Es lo que las reglas necesitan para decidir si puede cambiar de etapa.
type ConfiguracionConvocatoria struct {
	Convocatoria Convocatoria
	Dotaciones   []Dotacion
	Requisitos   []RequisitoDocumental
	Criterios    []CriterioEvaluacion
	Asignaciones []AsignacionDetalle
}

// Activos cuenta las asignaciones activas de un perfil.
func (c ConfiguracionConvocatoria) Activos(perfil Perfil) int {
	n := 0
	for _, a := range c.Asignaciones {
		if a.Activo && a.Perfil == perfil {
			n++
		}
	}
	return n
}

// DotacionesActivas cuenta las dotaciones que puede elegir un postulante.
func (c ConfiguracionConvocatoria) DotacionesActivas() int {
	n := 0
	for _, d := range c.Dotaciones {
		if d.Activa {
			n++
		}
	}
	return n
}

// CuentaAdmin es una cuenta tal como la ve el administrador al buscar a quién
// asignar: con sus perfiles globales y si ya está activada.
type CuentaAdmin struct {
	Usuario  Usuario
	Perfiles []Perfil
	Activada bool
}

// ---------------------------------------------------------------------------
// Postulaciones
// ---------------------------------------------------------------------------

// Subcategoria distingue, sólo en Intérpretes, a quien postula.
type Subcategoria string

const (
	SubcategoriaSolista  Subcategoria = "solista"
	SubcategoriaEnsamble Subcategoria = "ensamble"
)

// EstadoDePostulacion es el estado de una postulación en su ciclo de vida
// (§4). No confundir con EstadoPostulacion, que es la etapa «postulación» de
// una convocatoria.
type EstadoDePostulacion string

const (
	PostulacionBorrador      EstadoDePostulacion = "borrador"
	PostulacionEnviada       EstadoDePostulacion = "enviada"
	PostulacionEnRevision    EstadoDePostulacion = "en_revision"
	PostulacionEnSubsanacion EstadoDePostulacion = "en_subsanacion"
	PostulacionValidada      EstadoDePostulacion = "validada"
	PostulacionDescartada    EstadoDePostulacion = "descartada"
	PostulacionEnEvaluacion  EstadoDePostulacion = "en_evaluacion"
	PostulacionEvaluada      EstadoDePostulacion = "evaluada"
)

// EstadoEnlace es el resultado de la comprobación automática de una liga.
type EstadoEnlace string

const (
	EnlacePendiente   EstadoEnlace = "pendiente"
	EnlaceOK          EstadoEnlace = "ok"
	EnlaceInaccesible EstadoEnlace = "inaccesible"
	EnlaceNoPublico   EstadoEnlace = "no_publico"
)

// Integrante es un miembro de un ensamble: nombre e instrumento (D-05).
type Integrante struct {
	Nombre      string
	Instrumento string
}

// EnlaceVideo es una liga a un video de ejecución (sólo Intérpretes).
type EnlaceVideo struct {
	URL        string
	Estado     EstadoEnlace
	Plataforma string // youtube, vimeo, soundcloud, bandcamp, drive u otro
}

// Obra es una pieza incluida en una postulación. Sus datos van en Campos, por
// ruta canónica («obra.titulo», «obra.duracion_min»…), igual que los de la
// postulación.
type Obra struct {
	ID     int64
	Orden  int
	Campos map[string]string
}

// DocumentoPresente indica que la postulación ya tiene un documento vigente
// para un requisito. ObraID es 0 si el documento es de la postulación.
type DocumentoPresente struct {
	Clave  string // clave del requisito: fotografia, partitura…
	ObraID int64
}

// Postulacion es la solicitud de un postulante a una convocatoria, con todo lo
// que ha capturado.
//
// Los datos capturados se manejan como un mapa de ruta canónica a valor
// («datos.telefono», «concierto.entidad»…), siempre como texto; un campo sin
// capturar no aparece en el mapa. En la base cada ruta es una columna tipada
// (§8.1); el mapa existe para que una sola definición declarativa del
// formulario (rules.go) gobierne la captura, la validación del envío y, más
// adelante, el desbloqueo selectivo de la subsanación.
type Postulacion struct {
	ID             int64
	ConvocatoriaID int64
	UsuarioID      int64
	Subcategoria   Subcategoria // vacía en Compositores
	Folio          string       // vacío mientras es borrador
	Estado         EstadoDePostulacion
	Envios         int
	CreadoAt       time.Time
	ActualizadoAt  time.Time

	Campos      map[string]string
	Integrantes []Integrante
	Videos      []EnlaceVideo
	Obras       []Obra
	Documentos  []DocumentoPresente
}

// ResumenPostulacion es una postulación en un listado, con lo esencial de su
// convocatoria.
type ResumenPostulacion struct {
	ID            int64
	Subcategoria  Subcategoria
	Folio         string
	Estado        EstadoDePostulacion
	ActualizadoAt time.Time
	Convocatoria  Convocatoria // sin bases ni calendario
}

// ResultadoGuardado es la respuesta a un guardado parcial (autoguardado): qué
// campos se guardaron, cuáles se rechazaron y por qué, y a qué hora.
type ResultadoGuardado struct {
	Guardados  []string
	Errores    []ErrorCampo
	GuardadoAt time.Time
}

// Bloqueador es un motivo por el que una postulación aún no puede enviarse.
type Bloqueador struct {
	Seccion   string               // datos, concierto, obras, documentos o envio
	Campo     string               // ruta canónica, «integrantes», «videos», «obras» o «documento.<clave>»
	ObraID    int64                // obra a la que se refiere; 0 si es de la postulación
	Clave     string               // clave de traducción del motivo
	Requisito *RequisitoDocumental // el documento que falta, si el bloqueador es de un documento
}
