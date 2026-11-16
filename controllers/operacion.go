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

package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pantallas de operación (paso 13d): la bitácora de auditoría, la importación
// asistida en dos fases y la anonimización ARCO. Cada una llama a la misma
// operación que la API y que fimnme-cli.

// --- Bitácora -----------------------------------------------------------------

// vistaAsiento es un asiento de la bitácora.
type vistaAsiento struct {
	Momento   time.Time
	Actor     string
	Perfil    string
	Accion    string
	Entidad   string
	Antes     string // JSON con sangría, o vacío
	Despues   string
	IP        string
	Agente    string
	FiltroDel string // enlace que filtra por esta entidad
}

// vistaBitacora es la página de la bitácora.
type vistaBitacora struct {
	Accion, Entidad, EntidadID, ActorID, Desde, Hasta string
	Asientos                                          []vistaAsiento
	Total, Pagina, Paginas                            int
	Anterior, Siguiente, CSV                          string
	Invalido                                          string // nombre del filtro que no vale
}

// jsonLegible da sangría a un JSON guardado en la bitácora, para leerlo.
func jsonLegible(texto string) string {
	if strings.TrimSpace(texto) == "" || texto == "null" {
		return ""
	}
	var valor any
	if err := json.Unmarshal([]byte(texto), &valor); err != nil {
		return texto
	}
	legible, err := json.MarshalIndent(valor, "", "  ")
	if err != nil {
		return texto
	}
	return string(legible)
}

// GET /administracion/bitacora   — la bitácora de auditoría con sus filtros
//
// La ven el administrador y el observador (RF-AUD-03). Mismos filtros que la
// API; el CSV es el de la API.
func (a *App) bitacoraDeAdmin(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeConsultarAuditoria(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	consulta := r.URL.Query()
	v := vistaBitacora{Accion: consulta.Get("accion"), Entidad: consulta.Get("entidad"), EntidadID: consulta.Get("entidad_id"),
		ActorID: consulta.Get("actor_id"), Desde: consulta.Get("desde"), Hasta: consulta.Get("hasta")}
	filtro, invalido := filtroDeAuditoria(consulta)
	estado := http.StatusOK
	if invalido != "" {
		v.Invalido, estado = invalido, http.StatusUnprocessableEntity
	} else {
		asientos, total, err := a.datos.ConsultarAuditoria(r.Context(), filtro)
		if err != nil {
			a.errorInterno(w, r, "consultar bitácora", err)
			return
		}
		v.Total, v.Pagina = total, filtro.Pagina
		v.Paginas = max(1, (total+filtro.Tamano-1)/filtro.Tamano)
		pagina := func(n int) string {
			copia := consultaSinPagina(consulta)
			if n > 1 {
				copia.Set("pagina", strconv.Itoa(n))
			}
			return "/administracion/bitacora?" + copia.Encode()
		}
		if v.Pagina > 1 {
			v.Anterior = pagina(v.Pagina - 1)
		}
		if v.Pagina < v.Paginas {
			v.Siguiente = pagina(v.Pagina + 1)
		}
		paraCSV := consultaSinPagina(consulta)
		paraCSV.Set("formato", "csv")
		v.CSV = "/api/v1/auditoria?" + paraCSV.Encode()
		nombres := map[int64]string{}
		for _, asiento := range asientos {
			actor := a.vistas.T(idiomaDe(r), "bitacora.sistema")
			if asiento.ActorID != 0 {
				if _, visto := nombres[asiento.ActorID]; !visto {
					nombres[asiento.ActorID] = fmt.Sprintf("#%d", asiento.ActorID)
					if cuenta, err := a.datos.UsuarioPorID(r.Context(), asiento.ActorID); err == nil {
						nombres[asiento.ActorID] = strings.TrimSpace(cuenta.Usuario.Nombre+" "+cuenta.Usuario.Apellidos) + " · " + cuenta.Usuario.Email
					}
				}
				actor = nombres[asiento.ActorID]
			}
			entidad := asiento.Entidad
			filtroDel := ""
			if asiento.EntidadID != 0 {
				entidad = fmt.Sprintf("%s #%d", asiento.Entidad, asiento.EntidadID)
				filtroDel = "/administracion/bitacora?" + url.Values{"entidad": {asiento.Entidad}, "entidad_id": {strconv.FormatInt(asiento.EntidadID, 10)}}.Encode()
			}
			v.Asientos = append(v.Asientos, vistaAsiento{Momento: asiento.CreadoAt, Actor: actor, Perfil: asiento.ActorPerfil, Accion: asiento.Accion,
				Entidad: entidad, Antes: jsonLegible(asiento.Antes), Despues: jsonLegible(asiento.Despues), IP: asiento.IP, Agente: asiento.Agente,
				FiltroDel: filtroDel})
		}
	}
	p := a.pagina(r, "bitacora.titulo")
	p.Datos = v
	a.pintar(w, r, estado, "admin_bitacora.html", p)
}

// --- Importación asistida ---------------------------------------------------------

// vistaImportacionEnLista es una importación en la lista.
type vistaImportacionEnLista struct {
	URL          string
	Tipo         string // clave de traducción
	Convocatoria string
	Estado       string // clave de traducción
	Clase        string
	Filas        int
	Pendientes   int
	CreadoAt     time.Time
}

// vistaImportaciones es la página de importaciones.
type vistaImportaciones struct {
	Importaciones []vistaImportacionEnLista
	Convocatorias []vistaOpcion
	Tipos         []vistaOpcion
	Error         string
}

// vistaCandidato es un registro existente que podría ser el del renglón.
type vistaCandidato struct {
	ID          int64
	Descripcion string
	Motivo      string // clave de traducción
	Elegido     bool
}

// vistaConflicto es un renglón que una persona debe resolver.
type vistaConflicto struct {
	ID         int64
	Fila       int
	Columna    string
	Valor      string
	Candidatos []vistaCandidato
	Resolucion string
	Resuelto   string // clave de traducción, si está resuelto
}

// vistaImportacion es una importación con sus conflictos.
type vistaImportacion struct {
	ID                      int64
	Base                    string
	Tipo                    string
	Convocatoria            string
	Estado                  string
	Clase                   string
	Abierta                 bool
	Filas                   int
	CreadoAt                time.Time
	SHA256                  string
	Efecto                  models.EfectoDeImportacion
	Conflictos              []vistaConflicto
	SinCandidatosPendientes int
	PuedeConfirmarse        bool
	Error                   string
	CreaCuentas             bool
}

// clasesDeImportacion son los estilos de cada estado.
var clasesDeImportacion = map[string]string{models.ImportacionEsperando: "proxima", models.ImportacionConfirmada: "abierta", models.ImportacionCancelada: "cerrada"}

// tituloDeConvocatoria escribe «Edición · convocatoria» para una lista.
func (a *App) tituloDeConvocatoria(r *http.Request, id int64) string {
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), id)
	if err != nil {
		return fmt.Sprintf("#%d", id)
	}
	idioma := idiomaDe(r)
	return cfg.Convocatoria.Edicion.Nombre(idioma) + " · " + cfg.Convocatoria.Titulo(idioma)
}

// GET /administracion/importaciones   — importaciones recientes y alta de una nueva
func (a *App) importacionesDeAdmin(w http.ResponseWriter, r *http.Request) {
	a.pintarImportaciones(w, r, http.StatusOK, "")
}

// pintarImportaciones muestra la página de importaciones, con el motivo de
// un rechazo si lo hay.
func (a *App) pintarImportaciones(w http.ResponseWriter, r *http.Request, estado int, mensaje string) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	importaciones, err := a.datos.ImportacionesRecientes(r.Context(), 30)
	if err != nil {
		a.errorInterno(w, r, "listar importaciones", err)
		return
	}
	idioma := idiomaDe(r)
	v := vistaImportaciones{Error: mensaje}
	for _, i := range importaciones {
		v.Importaciones = append(v.Importaciones, vistaImportacionEnLista{URL: fmt.Sprintf("/administracion/importaciones/%d", i.ID),
			Tipo: "importacion.tipo_" + i.Tipo, Convocatoria: a.tituloDeConvocatoria(r, i.ConvocatoriaID), Estado: "importacion.estado_" + i.Estado,
			Clase: clasesDeImportacion[i.Estado], Filas: i.FilasTotal, Pendientes: i.Efecto().SinResolver, CreadoAt: i.CreadoAt})
	}
	ediciones, err := a.datos.ListarEdiciones(r.Context())
	if err != nil {
		a.errorInterno(w, r, "listar ediciones", err)
		return
	}
	for _, e := range ediciones {
		convocatorias, err := a.datos.ConvocatoriasDeEdicion(r.Context(), e.ID)
		if err != nil {
			a.errorInterno(w, r, "listar convocatorias", err)
			return
		}
		for _, c := range convocatorias {
			v.Convocatorias = append(v.Convocatorias, vistaOpcion{Valor: strconv.FormatInt(c.ID, 10), Texto: e.Nombre(idioma) + " · " + c.Titulo(idioma)})
		}
	}
	for _, tipo := range []string{models.ImportacionDotaciones, models.ImportacionJurados, models.ImportacionRevisores} {
		v.Tipos = append(v.Tipos, vistaOpcion{Valor: tipo, Texto: a.vistas.T(idioma, "importacion.tipo_"+tipo)})
	}
	p := a.pagina(r, "importacion.titulo")
	p.Datos = v
	a.pintar(w, r, estado, "admin_importaciones.html", p)
}

// POST /administracion/importaciones   — fase Analyze: sube la hoja (tipo, convocatoria_id, archivo)
//
// No escribe nada en el dominio: lleva a la importación, con sus conflictos.
func (a *App) analizarImportacionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := a.analizarImportacion(w, r)
	switch {
	case err == nil:
		http.Redirect(w, r, fmt.Sprintf("/administracion/importaciones/%d", id), http.StatusSeeOther)
		return
	case errors.Is(err, errSubidaInvalida):
		err = &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "documentos.subida_invalida"}}}
	case errors.Is(err, models.ErrHojaIlegible):
		err = &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "error.archivo_ilegible"}}}
	}
	mensaje, rechazo := a.mensajeDeRechazo(r, err)
	if !rechazo {
		a.errorInterno(w, r, "analizar importación", err)
		return
	}
	a.pintarImportaciones(w, r, http.StatusUnprocessableEntity, mensaje)
}

// GET /administracion/importaciones/{id}   — una importación con sus conflictos
func (a *App) importacionDeAdmin(w http.ResponseWriter, r *http.Request) {
	a.pintarImportacion(w, r, http.StatusOK, "")
}

// pintarImportacion muestra una importación, con un rechazo si lo hay.
func (a *App) pintarImportacion(w http.ResponseWriter, r *http.Request, estado int, mensaje string) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	i, err := a.datos.ImportacionPorID(r.Context(), id)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		a.errorInterno(w, r, "leer importación", err)
		return
	}
	efecto := i.Efecto()
	v := vistaImportacion{ID: i.ID, Base: fmt.Sprintf("/administracion/importaciones/%d", i.ID), Tipo: "importacion.tipo_" + i.Tipo,
		Convocatoria: a.tituloDeConvocatoria(r, i.ConvocatoriaID), Estado: "importacion.estado_" + i.Estado, Clase: clasesDeImportacion[i.Estado],
		Abierta: i.Estado == models.ImportacionEsperando, Filas: i.FilasTotal, CreadoAt: i.CreadoAt, SHA256: i.Archivo.SHA256,
		Efecto: efecto, PuedeConfirmarse: i.Estado == models.ImportacionEsperando && efecto.SinResolver == 0, Error: mensaje,
		CreaCuentas: i.Tipo != models.ImportacionDotaciones}
	for _, c := range i.Conflictos {
		vc := vistaConflicto{ID: c.ID, Fila: c.Fila, Columna: c.Columna, Valor: c.ValorOrigen, Resolucion: c.Resolucion}
		if c.Resolucion != "" {
			vc.Resuelto = "importacion.resolucion_" + c.Resolucion
		}
		for _, candidato := range c.Candidatos {
			vc.Candidatos = append(vc.Candidatos, vistaCandidato{ID: candidato.ID, Descripcion: candidato.Descripcion,
				Motivo: "importacion.motivo_" + candidato.Motivo, Elegido: c.Resolucion == models.ResolucionVincular && c.EntidadID == candidato.ID})
			if c.Resolucion == models.ResolucionVincular && c.EntidadID == candidato.ID {
				vc.Resuelto = "importacion.resolucion_vincular"
			}
		}
		if len(c.Candidatos) == 0 && c.Resolucion == "" {
			v.SinCandidatosPendientes++
		}
		v.Conflictos = append(v.Conflictos, vc)
	}
	p := a.pagina(r, "importacion.titulo")
	p.Datos = v
	if aviso, hay := avisosDeImportacion[r.URL.Query().Get("aviso")]; hay {
		p.Aviso = &aviso
	}
	a.pintar(w, r, estado, "admin_importacion.html", p)
}

// avisosDeImportacion son los mensajes que una redirección pide mostrar.
var avisosDeImportacion = map[string]views.Aviso{
	"resuelto":   {Tipo: "exito", Clave: "importacion.aviso_resuelto"},
	"confirmada": {Tipo: "exito", Clave: "importacion.aviso_confirmada"},
	"cancelada":  {Tipo: "exito", Clave: "importacion.aviso_cancelada"},
}

// accionDeImportacion es el esqueleto de las acciones sobre una importación:
// sólo el administrador; si sale bien, vuelve a la importación con un aviso;
// si el modelo la rechaza, la repinta con el motivo.
func (a *App) accionDeImportacion(w http.ResponseWriter, r *http.Request, aviso string, operacion func(id int64) error) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	err = operacion(id)
	if errors.Is(err, models.ErrHojaIlegible) {
		err = &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "error.archivo_ilegible"}}}
	}
	switch mensaje, rechazo := a.mensajeDeRechazo(r, err); {
	case err == nil:
		http.Redirect(w, r, fmt.Sprintf("/administracion/importaciones/%d?aviso=%s", id, aviso), http.StatusSeeOther)
	case errors.Is(err, models.ErrNoEncontrado):
		a.errorPagina(w, r, http.StatusNotFound)
	case rechazo:
		a.pintarImportacion(w, r, http.StatusUnprocessableEntity, mensaje)
	default:
		a.errorInterno(w, r, "importación", err)
	}
}

// POST /administracion/importaciones/{id}/conflictos/{conflicto}   — resuelve un conflicto (eleccion)
//
// «eleccion» es «vincular:<id>», «crear_nuevo» u «omitir».
func (a *App) resolverConflictoDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeImportacion(w, r, "resuelto", func(id int64) error {
		conflictoID, err := strconv.ParseInt(r.PathValue("conflicto"), 10, 64)
		if err != nil {
			return models.ErrNoEncontrado
		}
		resolucion, entidad, _ := strings.Cut(r.PostFormValue("eleccion"), ":")
		entidadID, _ := strconv.ParseInt(entidad, 10, 64)
		return a.datos.ResolverConflicto(r.Context(), a.actorDe(r), id, conflictoID, resolucion, entidadID)
	})
}

// POST /administracion/importaciones/{id}/resoluciones   — resuelve los conflictos sin candidatos (resolucion)
func (a *App) resolverSinCandidatosDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeImportacion(w, r, "resuelto", func(id int64) error {
		_, err := a.datos.ResolverSinCandidatos(r.Context(), a.actorDe(r), id, r.PostFormValue("resolucion"))
		return err
	})
}

// POST /administracion/importaciones/{id}/confirmacion   — fase Commit (confirmo)
func (a *App) confirmarImportacionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeImportacion(w, r, "confirmada", func(id int64) error {
		if r.PostFormValue("confirmo") == "" {
			return &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "confirmo", Clave: "error.confirmacion_requerida"}}}
		}
		return a.confirmarImportacion(r, id)
	})
}

// POST /administracion/importaciones/{id}/cancelar   — cancela sin aplicar
func (a *App) cancelarImportacionDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	a.accionDeImportacion(w, r, "cancelada", func(id int64) error {
		return a.datos.CancelarImportacion(r.Context(), a.actorDe(r), id)
	})
}

// --- Anonimización ARCO -----------------------------------------------------------

// vistaARCO es la página de la anonimización.
type vistaARCO struct {
	Folio        string
	Encontrada   bool
	NoExiste     bool
	ID           int64
	Nombre       string
	Convocatoria string
	Estado       string
	Anonimizada  bool
	Fundamento   string
	ConCuenta    bool
	Error        string
	Resultado    *vistaResultadoARCO
}

// vistaResultadoARCO es lo que hizo una anonimización.
type vistaResultadoARCO struct {
	Folio                             string
	Purgados, Pendientes, Conservados int
	CuentaAnonimizada                 bool
}

// GET /administracion/arco?folio=   — busca la postulación que se anonimizará
//
// Sólo el administrador. Es la única operación destructiva del sistema: la
// página pide el fundamento, volver a escribir el folio y marcar la
// confirmación (RN-28).
func (a *App) arcoDeAdmin(w http.ResponseWriter, r *http.Request) {
	a.pintarARCO(w, r, http.StatusOK, strings.TrimSpace(r.URL.Query().Get("folio")), "", nil)
}

// pintarARCO muestra la página de anonimización para un folio.
func (a *App) pintarARCO(w http.ResponseWriter, r *http.Request, estado int, folio, mensaje string, resultado *vistaResultadoARCO) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	v := vistaARCO{Folio: folio, Error: mensaje, Resultado: resultado, Fundamento: r.PostFormValue("fundamento"), ConCuenta: r.PostFormValue("con_cuenta") != ""}
	if folio != "" && resultado == nil {
		id, err := a.datos.PostulacionPorFolio(r.Context(), folio)
		switch {
		case errors.Is(err, models.ErrNoEncontrado):
			v.NoExiste = true
		case err != nil:
			a.errorInterno(w, r, "buscar folio", err)
			return
		default:
			p, err := a.datos.PostulacionPorID(r.Context(), id)
			if err != nil {
				a.errorInterno(w, r, "leer postulación", err)
				return
			}
			titular, err := a.datos.UsuarioPorID(r.Context(), p.UsuarioID)
			if err != nil {
				a.errorInterno(w, r, "leer titular", err)
				return
			}
			v.Encontrada, v.ID, v.Folio = true, p.ID, p.Folio
			mostrado := models.TitularParaMostrar(p, titular.Usuario)
			v.Nombre = primeroNoVacio(p.Campos["datos.nombre_completo"], p.Campos["datos.nombre_ensamble"],
				strings.TrimSpace(mostrado.Nombre+" "+mostrado.Apellidos))
			v.Convocatoria = a.tituloDeConvocatoria(r, p.ConvocatoriaID)
			v.Estado = "estado.postulacion." + string(p.Estado)
			v.Anonimizada = !p.AnonimizadaAt.IsZero()
		}
	}
	p := a.pagina(r, "arco.titulo")
	p.Datos = v
	a.pintar(w, r, estado, "admin_arco.html", p)
}

// POST /administracion/arco/{id}   — anonimiza una postulación (folio, fundamento, confirmo, con_cuenta)
//
// Borra los datos de contacto y los archivos personales, y conserva la obra,
// el dictamen y las estadísticas. Queda en la bitácora con su fundamento.
func (a *App) anonimizarDesdeAdmin(w http.ResponseWriter, r *http.Request) {
	if !models.PuedeAdministrar(sesionDe(r)) {
		a.errorPagina(w, r, http.StatusForbidden)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if errors.Is(err, models.ErrNoEncontrado) {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		a.errorInterno(w, r, "leer postulación", err)
		return
	}
	resultado, err := a.datos.AnonimizarPostulacion(r.Context(), a.actorDe(r), id, models.SolicitudDeAnonimizacion{
		Folio: r.PostFormValue("folio"), Fundamento: r.PostFormValue("fundamento"), Confirmo: r.PostFormValue("confirmo") != "",
		ConCuenta: r.PostFormValue("con_cuenta") != ""})
	if err != nil {
		mensaje, rechazo := a.mensajeDeRechazo(r, err)
		if !rechazo {
			a.errorInterno(w, r, "anonimizar", err)
			return
		}
		a.pintarARCO(w, r, http.StatusUnprocessableEntity, p.Folio, mensaje, nil)
		return
	}
	// Los archivos ya están marcados como purgados; se borran del disco. Lo
	// que no pueda borrarse lo señalará «fimnme-cli verify».
	pendientes := models.PurgarArchivos(a.almacen, resultado.ArchivosPorPurgar, func(archivo models.Archivo, err error) {
		a.log.Error("no se pudo borrar del disco un archivo purgado", "archivo", archivo.ID, "error", err)
	})
	a.pintarARCO(w, r, http.StatusOK, "", "", &vistaResultadoARCO{Folio: resultado.Folio, Purgados: len(resultado.ArchivosPorPurgar) - pendientes,
		Pendientes: pendientes, Conservados: resultado.ArchivosConservados, CuentaAnonimizada: resultado.CuentaAnonimizada})
}
