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
	"fmt"
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

// Claves de las plantillas de correo del envío de la postulación.
const (
	PlantillaAcuse        = "acuse"
	PlantillaRecordatorio = "recordatorio_borrador"
)

// Claves de las plantillas de correo de la revisión documental.
const (
	PlantillaSubsanacion      = "subsanacion"       // al postulante: qué corregir y hasta cuándo
	PlantillaAcuseSubsanacion = "acuse_subsanacion" // al postulante: acuse del reenvío
	PlantillaAvisoSubsanada   = "aviso_subsanada"   // a los revisores: una postulación volvió
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
	// PostulacionID liga el mensaje con una postulación (acuse, recordatorio);
	// 0 si es un correo de cuenta.
	PostulacionID int64
	Adjuntos      []Adjunto
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
	Detalle    string // código del resultado de la comprobación en red
}

// Obra es una pieza incluida en una postulación. Sus datos van en Campos, por
// ruta canónica («obra.titulo», «obra.duracion_min»…), igual que los de la
// postulación.
type Obra struct {
	ID     int64
	Orden  int
	Campos map[string]string
}

// Archivo son los metadatos de un contenido guardado en el almacén (tabla
// «archivo»). El contenido en sí vive en disco, direccionado por su SHA-256.
type Archivo struct {
	ID          int64
	SHA256      string
	Clave       string // clave de almacenamiento, para BlobStore.Abrir
	MIME        string // detectado por la firma del archivo, no por su extensión
	TamanoBytes int64
}

// Documento es un archivo entregado para cumplir un requisito documental.
// ObraID es 0 si el documento es de la postulación (la fotografía) y no de
// una obra.
//
// Nada se sobrescribe (RF-DOC-07): reemplazar un documento crea otro con la
// versión siguiente y deja el anterior con Vigente en false.
type Documento struct {
	ID            int64
	PostulacionID int64
	ObraID        int64
	Clave         string // clave del requisito: fotografia, partitura…
	Version       int
	Vigente       bool
	Dictamen      string // pendiente, valido o invalido (lo fija el revisor)
	SubidoAt      time.Time
	Archivo       Archivo
}

// NuevoDocumento es lo que el controlador entrega al repositorio tras recibir
// una subida: el contenido ya está en el almacén y su formato ya se detectó.
type NuevoDocumento struct {
	Clave          string // clave del requisito que cumple
	ObraID         int64  // 0 si es de la postulación
	ReemplazaA     int64  // id del documento vigente que sustituye; 0 si es nuevo
	Blob           Blob
	MIME           string
	NombreOriginal string
}

// Acuse es el comprobante de un envío: un PDF y el snapshot de lo enviado
// (RN-13). Cada reenvío tras una subsanación genera otro, con la versión
// siguiente; los anteriores se conservan.
type Acuse struct {
	ID            int64
	PostulacionID int64
	Version       int
	Idioma        string
	GeneradoAt    time.Time
	Archivo       Archivo
}

// ConfirmacionEnvio son las dos declaraciones que el postulante hace al
// enviar (RF-POST-10).
type ConfirmacionEnvio struct {
	Confirmo    bool // «entiendo que ya no podré editar mi postulación»
	AceptoBases bool
}

// ErrorDeEnvio indica que la postulación no pudo enviarse porque le falta
// algo. Lleva la lista exacta de bloqueadores (RN-03).
type ErrorDeEnvio struct {
	Bloqueadores []Bloqueador
}

func (e *ErrorDeEnvio) Error() string {
	return fmt.Sprintf("la postulación no puede enviarse: faltan %d elementos", len(e.Bloqueadores))
}

// SnapshotEnvio es la postulación tal como se envió: la base probatoria del
// proceso (§4.1). Se guarda como JSON junto a cada acuse y no cambia nunca,
// aunque después una subsanación modifique la postulación. El acuse en PDF se
// compone únicamente a partir de él, de modo que puede regenerarse idéntico.
//
// Es un formato persistente: sus nombres de campo no se cambian; sólo se
// añaden campos nuevos.
type SnapshotEnvio struct {
	Folio        string               `json:"folio"`
	Version      int                  `json:"version"`
	EnviadaAt    string               `json:"enviada_at"` // UTC, ISO-8601
	Idioma       string               `json:"idioma"`     // idioma del acuse
	Edicion      SnapshotEdicion      `json:"edicion"`
	Convocatoria SnapshotConvocatoria `json:"convocatoria"`
	Subcategoria string               `json:"subcategoria,omitempty"`
	Titular      SnapshotTitular      `json:"titular"`
	Campos       map[string]string    `json:"campos"`
	Integrantes  []SnapshotIntegrante `json:"integrantes,omitempty"`
	Videos       []SnapshotVideo      `json:"videos,omitempty"`
	Obras        []SnapshotObra       `json:"obras"`
	Documentos   []SnapshotDocumento  `json:"documentos"`
}

// SnapshotEdicion identifica la edición del Foro.
type SnapshotEdicion struct {
	Numero   int    `json:"numero"`
	Anio     int    `json:"anio"`
	NombreES string `json:"nombre_es"`
	NombreEN string `json:"nombre_en"`
}

// SnapshotConvocatoria identifica la convocatoria.
type SnapshotConvocatoria struct {
	ID       int64  `json:"id"`
	Tipo     string `json:"tipo"`
	TituloES string `json:"titulo_es"`
	TituloEN string `json:"titulo_en"`
}

// SnapshotTitular es la cuenta que envió la postulación.
type SnapshotTitular struct {
	Nombre    string `json:"nombre"`
	Apellidos string `json:"apellidos"`
	Email     string `json:"email"`
}

// SnapshotIntegrante es un integrante del ensamble.
type SnapshotIntegrante struct {
	Nombre      string `json:"nombre"`
	Instrumento string `json:"instrumento"`
}

// SnapshotVideo es una liga de video con el resultado de su comprobación.
type SnapshotVideo struct {
	URL        string `json:"url"`
	Estado     string `json:"estado"`
	Plataforma string `json:"plataforma"`
}

// SnapshotObra es una obra con sus campos y el nombre de su dotación.
type SnapshotObra struct {
	ID         int64             `json:"id"`
	Orden      int               `json:"orden"`
	Campos     map[string]string `json:"campos"`
	DotacionES string            `json:"dotacion_es,omitempty"`
	DotacionEN string            `json:"dotacion_en,omitempty"`
}

// SnapshotDocumento es un documento entregado. Su SHA-256 permite comprobar,
// años después, que el archivo del acervo es exactamente el que se envió.
type SnapshotDocumento struct {
	Clave       string `json:"clave"`
	EtiquetaES  string `json:"etiqueta_es"`
	EtiquetaEN  string `json:"etiqueta_en"`
	ObraID      int64  `json:"obra_id,omitempty"`
	Version     int    `json:"version"`
	SHA256      string `json:"sha256"`
	TamanoBytes int64  `json:"tamano_bytes"`
	MIME        string `json:"mime"`
}

// AcuseCompuesto es lo que devuelve la Vista al componer un acuse: el PDF, ya
// guardado en el almacén, y el correo que lo lleva al postulante.
type AcuseCompuesto struct {
	Blob   Blob
	Correo CorreoSaliente
	// Avisos son correos para otras personas que salen con el envío: en un
	// reenvío tras subsanación, el aviso a los revisores (RF-REV-07).
	Avisos []CorreoSaliente
}

// Adjunto es un archivo del almacén que acompaña a un correo.
type Adjunto struct {
	ArchivoID int64  `json:"archivo_id"`
	Nombre    string `json:"nombre"`
}

// EnlacePorVerificar es una liga de video pendiente de comprobar en red.
type EnlacePorVerificar struct {
	ID  int64
	URL string
}

// ResultadoEnlace es el resultado de comprobar una liga en red (RF-DOC-10).
type ResultadoEnlace struct {
	Estado     EstadoEnlace
	HTTPStatus int    // 0 si no hubo respuesta
	Detalle    string // explicación breve, para el revisor
}

// RecordatorioDeBorrador es un borrador al que toca recordarle que el plazo
// está por vencer (RF-NOT-04).
type RecordatorioDeBorrador struct {
	PostulacionID int64
	Usuario       Usuario
	Convocatoria  Convocatoria
	TerminaAt     time.Time // cierre de la etapa de postulación
	Ventana       time.Time // inicio de la ventana del recordatorio que toca
}

// ClaseArchivoConvocatoria distingue los archivos públicos de una
// convocatoria (RF-CONV-02).
type ClaseArchivoConvocatoria string

const (
	ArchivoBasesES ClaseArchivoConvocatoria = "bases_pdf_es"
	ArchivoBasesEN ClaseArchivoConvocatoria = "bases_pdf_en"
	ArchivoImagen  ClaseArchivoConvocatoria = "imagen"
)

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
	Documentos  []Documento // sólo los vigentes
	Acuses      []Acuse     // uno por envío

	// Revisión documental.
	EnviadaAt      time.Time     // último envío; cero si es borrador
	MotivoDescarte string        // sólo si fue descartada
	Subsanaciones  []Subsanacion // de la más antigua a la más reciente
}

// SubsanacionAbierta devuelve la subsanación que el postulante puede atender
// en este momento: la que no ha reenviado, el revisor no ha cerrado y cuyo
// plazo no ha vencido. Devuelve nil si no hay ninguna.
func (p Postulacion) SubsanacionAbierta(ahora time.Time) *Subsanacion {
	if p.Estado != PostulacionEnSubsanacion {
		return nil
	}
	for i := len(p.Subsanaciones) - 1; i >= 0; i-- {
		if s := &p.Subsanaciones[i]; s.Pendiente() {
			if ahora.After(s.FechaLimite) {
				return nil
			}
			return s
		}
	}
	return nil
}

// SubsanacionVencida indica si la postulación espera una subsanación cuyo
// plazo ya pasó sin respuesta. No se descarta sola: queda para atención del
// revisor (RF-REV-09).
func (p Postulacion) SubsanacionVencida(ahora time.Time) bool {
	return p.Estado == PostulacionEnSubsanacion && p.SubsanacionAbierta(ahora) == nil
}

// ClaseProblema distingue lo que el revisor puede señalar (§4.2).
type ClaseProblema string

const (
	ProblemaDocumento ClaseProblema = "documento" // un documento inválido o faltante
	ProblemaCampo     ClaseProblema = "campo"     // un dato erróneo
)

// Campos «de estructura»: no son un dato sino una lista entera de la
// postulación. Señalarlos desbloquea la lista: añadir o quitar integrantes,
// ligas de video u obras (RN-12c).
const (
	CampoIntegrantes = "integrantes"
	CampoVideos      = "videos"
	CampoObras       = "obras"
)

// ProblemaSubsanacion es un elemento que el revisor señaló y que, por eso,
// el postulante puede corregir. Es la ÚNICA vía por la que algo se desbloquea
// durante una subsanación (RN-12b).
type ProblemaSubsanacion struct {
	ID          int64
	Clase       ClaseProblema
	Campo       string // ruta canónica del campo, o la clave del requisito si es un documento
	ObraID      int64  // 0 si el elemento es de la postulación
	DocumentoID int64  // documento señalado; 0 si falta o si es un campo
	Problema    string // motivo, obligatorio
	ResueltoAt  time.Time
}

// SolicitudDesbloqueo es la petición de un postulante para corregir algo que
// el revisor no señaló (RN-12d). Si se concede, se convierte en un problema
// más de la subsanación.
type SolicitudDesbloqueo struct {
	ID              int64
	Clase           ClaseProblema
	Campo           string
	ObraID          int64
	Justificacion   string
	Resuelta        bool
	Concedida       bool
	MotivoRespuesta string
	CreadoAt        time.Time
	ResueltaAt      time.Time
}

// Subsanacion es una solicitud del revisor para que el postulante corrija
// elementos concretos de una postulación ya enviada, con un plazo.
type Subsanacion struct {
	ID            int64
	PostulacionID int64
	RevisorID     int64
	Mensaje       string // el texto que se envió al postulante
	FechaLimite   time.Time
	EnviadaAt     time.Time
	AtendidaAt    time.Time // cuando el postulante reenvió
	CerradaAt     time.Time // cuando el revisor la dio por terminada
	Problemas     []ProblemaSubsanacion
	Solicitudes   []SolicitudDesbloqueo
}

// Pendiente indica que la subsanación sigue esperando al postulante: ni la
// atendió ni el revisor la cerró. Puede estar vencida.
func (s Subsanacion) Pendiente() bool {
	return s.AtendidaAt.IsZero() && s.CerradaAt.IsZero()
}

// NuevaSubsanacion es lo que el revisor pide al solicitar una subsanación.
type NuevaSubsanacion struct {
	Problemas   []ProblemaSubsanacion // campos y documentos faltantes; los inválidos se añaden solos
	FechaLimite time.Time
	Nota        string // texto libre del revisor para el correo (RF-REV-06)
}

// DictamenDocumento es el dictamen de un revisor sobre un documento.
type DictamenDocumento struct {
	DocumentoID int64
	Dictamen    string // pendiente, valido o invalido
	Motivo      string // obligatorio si es inválido (RN-11)
	RevisorID   int64
	CreadoAt    time.Time
}

// ObservacionRevision es una nota del revisor sobre la postulación o sobre
// una de sus obras (RF-REV-04). Es interna: el postulante no la ve.
type ObservacionRevision struct {
	ID        int64
	ObraID    int64 // 0 si es de la postulación
	RevisorID int64
	Revisor   string // nombre de quien la escribió
	Texto     string
	CreadoAt  time.Time
}

// TomaRevision indica que un revisor marcó la postulación como «en revisión
// por mí». Es informativa: no impide que otro revisor intervenga (RN-14b).
type TomaRevision struct {
	RevisorID int64
	Revisor   string
	TomadaAt  time.Time
}

// Revision reúne lo que la revisión documental ha dejado sobre una
// postulación. Sólo lo ven quienes revisan o supervisan.
type Revision struct {
	Dictamenes    []DictamenDocumento // el vigente de cada documento
	Observaciones []ObservacionRevision
	Toma          *TomaRevision // nil si está libre
}

// PostulacionEnBandeja es un renglón de la bandeja del revisor (RF-REV-01).
type PostulacionEnBandeja struct {
	ID                  int64
	Folio               string
	Estado              EstadoDePostulacion
	Subcategoria        Subcategoria
	Nombre              string // nombre completo del solista o compositor, o nombre del ensamble
	EnviadaAt           time.Time
	Envios              int
	Toma                *TomaRevision
	Observaciones       int
	DocumentosInvalidos int
	// FechaLimite es el plazo de la subsanación pendiente; cero si no hay.
	FechaLimite time.Time
	// Subsanada indica que el postulante ya reenvió y falta que el revisor
	// vuelva a mirarla (RF-REV-07).
	Subsanada bool
}

// FiltroBandeja son los filtros de la bandeja del revisor.
type FiltroBandeja struct {
	Estado       EstadoDePostulacion
	Subcategoria Subcategoria
	Toma         string // "libre", "mia" u "otra"; vacío = todas
	Texto        string // folio o nombre
}

// CambioDeEnvio es una diferencia entre dos envíos de la misma postulación:
// lo que el postulante cambió al subsanar (RF-REV-07).
type CambioDeEnvio struct {
	Clase   ClaseProblema
	Campo   string // ruta del campo o clave del requisito
	ObraID  int64
	Antes   string // valor anterior, o inicio de la suma del archivo anterior
	Despues string
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
