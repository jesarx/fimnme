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
	"context"
	"time"
)

// Persistencia es la abstracción de la base de datos: todo lo que el resto
// del sistema puede pedirle al Modelo. Los controladores y los trabajadores
// dependen de esta interfaz y nunca ven SQL ni conexiones.
//
// La implementa *Repositorio (repository.go) sobre SQLite 3. Cuando llegue la
// migración a MariaDB bastará otra implementación de esta misma interfaz: el
// cambio de motor se hace en el Modelo y en ningún otro lugar.
//
// Los métodos se agrupan por tema, en el mismo orden que repository.go.
type Persistencia interface {
	// Sonda.
	Verificar(ctx context.Context) error

	// Convocatorias públicas.
	ConvocatoriasPublicas(ctx context.Context) ([]Convocatoria, error)
	ConvocatoriaPublica(ctx context.Context, numeroEdicion int, tipo TipoConvocatoria) (Convocatoria, error)

	// Cuentas.
	CrearUsuario(ctx context.Context, actor Actor, n NuevoUsuario, token *NuevoToken, correos ...CorreoSaliente) (int64, error)
	UsuarioPorEmail(ctx context.Context, email string) (Usuario, string, error)
	ResumenDePassword(ctx context.Context, usuarioID int64) (string, error)
	ActualizarDatosPersonales(ctx context.Context, actor Actor, usuarioID int64, nombre, apellidos, idioma string) error
	CambiarPassword(ctx context.Context, actor Actor, usuarioID int64, resumen, sesionAConservar string) error
	CambiarEmail(ctx context.Context, actor Actor, usuarioID int64, email string, token NuevoToken, correos ...CorreoSaliente) error

	// Sesiones e inicios de sesión.
	AbrirSesion(ctx context.Context, actor Actor, usuarioID int64, idResumen string, expira time.Time) error
	RegistrarIngresoFallido(ctx context.Context, actor Actor, usuarioID int64, bloqueado bool) error
	FallosDeIngreso(ctx context.Context, usuarioID int64) (int, time.Time, error)
	SesionPorID(ctx context.Context, idResumen string) (*Sesion, error)
	SesionDeUsuario(ctx context.Context, usuarioID int64) (*Sesion, error)
	RenovarSesion(ctx context.Context, idResumen string, expira time.Time) error
	CerrarSesion(ctx context.Context, actor Actor, idResumen string) error
	PurgarSesionesVencidas(ctx context.Context) (int64, error)

	// Enlaces de un solo uso.
	EmitirToken(ctx context.Context, actor Actor, usuarioID int64, token NuevoToken, correos ...CorreoSaliente) error
	UltimaExpiracionDeToken(ctx context.Context, usuarioID int64, proposito PropositoToken) (time.Time, error)
	TokenVigente(ctx context.Context, resumen string, proposito PropositoToken) (Usuario, error)
	VerificarEmailConToken(ctx context.Context, actor Actor, resumen string) (int64, error)
	DefinirPasswordConToken(ctx context.Context, actor Actor, resumenToken string, proposito PropositoToken, resumenPassword string) (int64, error)

	// Correo saliente.
	PlantillaCorreo(ctx context.Context, clave, idioma string) (PlantillaCorreo, error)
	CorreosPendientes(ctx context.Context, limite int) ([]CorreoSaliente, error)
	MarcarCorreoEnviado(ctx context.Context, correo CorreoSaliente, messageID string) error
	MarcarCorreoFallido(ctx context.Context, correo CorreoSaliente, motivo string, proximo time.Time, reintentar bool) error

	// Bitácora.
	ConsultarAuditoria(ctx context.Context, filtro FiltroAuditoria) ([]AsientoAuditoria, int, error)

	// Configuración de ediciones y convocatorias.
	ListarEdiciones(ctx context.Context) ([]Edicion, error)
	EdicionPorID(ctx context.Context, id int64) (Edicion, error)
	CrearEdicion(ctx context.Context, actor Actor, e Edicion) (int64, error)
	ActualizarEdicion(ctx context.Context, actor Actor, e Edicion) error
	ConvocatoriasDeEdicion(ctx context.Context, edicionID int64) ([]Convocatoria, error)
	ConvocatoriasEnCurso(ctx context.Context) ([]int64, error)
	ConfiguracionDeConvocatoria(ctx context.Context, convocatoriaID int64) (ConfiguracionConvocatoria, error)
	CrearConvocatoriaConfigurada(ctx context.Context, actor Actor, c Convocatoria,
		requisitos []RequisitoDocumental, criterios []CriterioEvaluacion) (int64, error)
	ActualizarConvocatoria(ctx context.Context, actor Actor, c Convocatoria) error
	DefinirEtapas(ctx context.Context, actor Actor, convocatoriaID int64, etapas []Etapa) error
	CambiarEstadoConvocatoria(ctx context.Context, actor Actor, convocatoriaID int64,
		destino EstadoConvocatoria, motivo string, automatico bool) (EstadoConvocatoria, error)
	DuplicarConvocatoria(ctx context.Context, actor Actor, origenID, edicionID int64) (int64, error)
	GuardarDotacion(ctx context.Context, actor Actor, d Dotacion) (id, convocatoriaID int64, err error)
	GuardarRequisito(ctx context.Context, actor Actor, rq RequisitoDocumental) (id, convocatoriaID int64, err error)
	BorrarRequisito(ctx context.Context, actor Actor, id int64) error
	GuardarCriterio(ctx context.Context, actor Actor, c CriterioEvaluacion) (id, convocatoriaID int64, err error)
	BorrarCriterio(ctx context.Context, actor Actor, id int64) error
	Asignar(ctx context.Context, actor Actor, convocatoriaID, usuarioID int64, perfil Perfil) (int64, error)
	RetirarAsignacion(ctx context.Context, actor Actor, id int64) (int64, error)
	UsuarioPorID(ctx context.Context, id int64) (CuentaAdmin, error)
	BuscarUsuarios(ctx context.Context, texto string, pagina, tamano int) ([]CuentaAdmin, int, error)

	// Postulaciones. Las escrituras sólo actúan sobre postulaciones de la
	// cuenta indicada.
	PostulacionPorID(ctx context.Context, id int64) (Postulacion, error)
	PostulacionesDeUsuario(ctx context.Context, usuarioID int64) ([]ResumenPostulacion, error)
	CrearPostulacion(ctx context.Context, actor Actor, convocatoriaID, usuarioID int64, sub Subcategoria) (int64, error)
	GuardarCamposDePostulacion(ctx context.Context, usuarioID, postulacionID int64, cambios map[string]string) (ResultadoGuardado, error)
	GuardarCamposDeObra(ctx context.Context, usuarioID, obraID int64, cambios map[string]string) (int64, ResultadoGuardado, error)
	AgregarObra(ctx context.Context, usuarioID, postulacionID int64) (int64, error)
	BorrarObra(ctx context.Context, usuarioID, obraID int64) (int64, error)
	OrdenarObras(ctx context.Context, usuarioID, postulacionID int64, ids []int64) error
	DefinirIntegrantes(ctx context.Context, usuarioID, postulacionID int64, integrantes []Integrante) error
	DefinirVideos(ctx context.Context, usuarioID, postulacionID int64, ligas []string) error
	CambiarSubcategoria(ctx context.Context, actor Actor, usuarioID, postulacionID int64, sub Subcategoria) error

	// Documentos, envío y acuses. El contenido de los archivos vive en el
	// BlobStore; aquí sólo se guardan sus metadatos.
	RegistrarDocumento(ctx context.Context, actor Actor, usuarioID, postulacionID int64, n NuevoDocumento) (Documento, error)
	QuitarDocumento(ctx context.Context, actor Actor, usuarioID, documentoID int64) (int64, error)
	DocumentoPorID(ctx context.Context, id int64) (Documento, error)
	AcusePorVersion(ctx context.Context, postulacionID int64, version int) (Acuse, error)
	RegistrarDescarga(ctx context.Context, actor Actor, postulacionID, documentoID int64, archivo Archivo) error
	EnviarPostulacion(ctx context.Context, actor Actor, usuarioID, postulacionID int64,
		confirmacion ConfirmacionEnvio, componer func(SnapshotEnvio) (AcuseCompuesto, error)) error

	// Revisión documental y subsanación.
	RevisionDe(ctx context.Context, postulacionID int64) (Revision, error)
	BandejaDeRevision(ctx context.Context, convocatoriaID, revisorID int64, filtro FiltroBandeja) ([]PostulacionEnBandeja, error)
	TomarPostulacion(ctx context.Context, actor Actor, postulacionID int64, tomar bool) error
	DictaminarDocumento(ctx context.Context, actor Actor, documentoID int64, dictamen, motivo string) (int64, error)
	ObservarPostulacion(ctx context.Context, actor Actor, postulacionID, obraID int64, texto string) error
	SolicitarSubsanacion(ctx context.Context, actor Actor, postulacionID int64, n NuevaSubsanacion,
		componer func(Postulacion, ConfiguracionConvocatoria, Usuario, NuevaSubsanacion) (CorreoSaliente, error)) error
	SolicitarDesbloqueo(ctx context.Context, actor Actor, usuarioID, postulacionID int64, s SolicitudDesbloqueo) error
	ResolverDesbloqueo(ctx context.Context, actor Actor, postulacionID, solicitudID int64, concedida bool, motivo string) error
	DictaminarPostulacion(ctx context.Context, actor Actor, postulacionID int64, dictamen EstadoDePostulacion, motivo string) error
	DiferenciasDelUltimoEnvio(ctx context.Context, postulacionID int64) ([]CambioDeEnvio, error)

	// Evaluación por jurado. El aislamiento entre jurados es estructural:
	// ParaJurado sólo da acceso a las evaluaciones de ese jurado; lo que
	// cruza jurados vive en Supervision, para administrador y observador.
	ParaJurado(juradoID int64) JuradoRepo
	Supervision() SupervisionRepo
	RegistrarDictamenFinal(ctx context.Context, actor Actor, postulacionID int64, dictamen string) error

	// Archivo histórico, búsqueda y descargas masivas.
	ListarPostulaciones(ctx context.Context, alcance AlcanceDeConsulta, f FiltroDeListado) ([]RenglonDeListado, int, error)
	PostulacionPorFolio(ctx context.Context, folio string) (int64, error)
	VistasGuardadas(ctx context.Context, usuarioID int64) ([]VistaGuardada, error)
	GuardarVista(ctx context.Context, usuarioID int64, nombre, ruta string) (int64, error)
	BorrarVista(ctx context.Context, usuarioID, vistaID int64) error
	DocumentosParaZIP(ctx context.Context, parametros ParametrosDeExportacion) ([]DocumentoEnZIP, error)
	EncolarExportacion(ctx context.Context, actor Actor, tipo string, parametros ParametrosDeExportacion) (int64, error)
	ExportacionPorID(ctx context.Context, id int64) (TareaDeExportacion, error)
	TomarExportacion(ctx context.Context) (TareaDeExportacion, bool, error)
	ReencolarExportacionesInterrumpidas(ctx context.Context) (int, error)
	TerminarExportacion(ctx context.Context, id int64, archivos int, bytes int64, mensajeDeError string, aviso *CorreoSaliente) error
	RegistrarDescargaDeExportacion(ctx context.Context, actor Actor, tarea TareaDeExportacion) error

	// Expediente, reportes y tableros.
	RegistrarExpediente(ctx context.Context, actor Actor, postulacionID int64) error
	RegistrarReporte(ctx context.Context, actor Actor, convocatoriaID int64, campos []string, renglones int) error
	PlantillasDeReporte(ctx context.Context) ([]PlantillaDeReporte, error)
	PlantillaDeReportePorID(ctx context.Context, id int64) (PlantillaDeReporte, error)
	GuardarPlantillaDeReporte(ctx context.Context, actor Actor, p PlantillaDeReporte) (int64, error)
	BorrarPlantillaDeReporte(ctx context.Context, actor Actor, id, soloDe int64) error
	TableroDeConvocatoria(ctx context.Context, convocatoriaID int64) (TableroDeConvocatoria, error)
	EstadisticasDeEdicion(ctx context.Context, edicionID, convocatoriaID int64) (EstadisticasDeEdicion, error)

	// Continuidad operativa, portal público y anonimización.
	AdministradoresActivos(ctx context.Context) ([]Usuario, error)
	EncolarAviso(ctx context.Context, correo CorreoSaliente) error
	SeleccionadosDe(ctx context.Context, convocatoriaID int64) ([]InteropSeleccionado, error)
	PublicarNomina(ctx context.Context, actor Actor, convocatoriaID int64, blob Blob, seleccionados int, confirmo bool) error
	PublicacionesDe(ctx context.Context, convocatoriaID int64) ([]PublicacionInterop, []Archivo, error)
	PortalPublico(ctx context.Context, almacen BlobStore, numeroDeEdicion int) (InteropPortal, error)
	AnonimizarPostulacion(ctx context.Context, actor Actor, postulacionID int64, solicitud SolicitudDeAnonimizacion) (ResultadoDeAnonimizacion, error)

	// Comprobación en red de las ligas de video.
	EnlacesPorVerificar(ctx context.Context, limite int, revalidarAntesDe time.Time) ([]EnlacePorVerificar, error)
	EnlacesDePostulacion(ctx context.Context, postulacionID int64) ([]EnlacePorVerificar, error)
	AnotarVerificacionDeEnlace(ctx context.Context, enlace EnlacePorVerificar, resultado ResultadoEnlace) error

	// Recordatorios de borrador sin enviar.
	BorradoresPorRecordar(ctx context.Context) ([]RecordatorioDeBorrador, error)
	EncolarRecordatorio(ctx context.Context, rec RecordatorioDeBorrador, correo CorreoSaliente) (bool, error)

	// Archivos públicos de una convocatoria: bases en PDF e imagen de cabecera.
	ArchivosDeConvocatoria(ctx context.Context, convocatoriaID int64) (map[ClaseArchivoConvocatoria]Archivo, error)
	FijarArchivoDeConvocatoria(ctx context.Context, actor Actor, convocatoriaID int64,
		clase ClaseArchivoConvocatoria, blob Blob, mime, nombreOriginal string) error
}

// Comprobación en tiempo de compilación: si a *Repositorio le falta un método
// de la interfaz, el programa no compila.
var _ Persistencia = (*Repositorio)(nil)
