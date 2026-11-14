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
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// API del archivo histórico (§5.12): listado con búsqueda y filtros, enlaces
// permanentes por folio, vistas guardadas y descarga masiva en ZIP.
//
// Las restricciones de perfil no caducan: lo que cada quien ve aquí lo decide
// models.AlcanceDe, igual para una convocatoria viva que para una archivada.

// filtroDeLaPeticion lee los filtros del listado de los parámetros de la URL.
// Son los mismos para consultar, para exportar a CSV y para pedir un ZIP: así
// «el resultado de esta búsqueda» es siempre la misma dirección (RF-ARCH-14).
func filtroDeLaPeticion(r *http.Request) (models.FiltroDeListado, []models.ErrorCampo) {
	consulta := r.URL.Query()
	f := models.FiltroDeListado{
		Texto: consulta.Get("q"), Tipo: models.TipoConvocatoria(consulta.Get("tipo")),
		Subcategoria: models.Subcategoria(consulta.Get("subcategoria")), Estado: models.EstadoDePostulacion(consulta.Get("estado")),
		Dictamen: consulta.Get("dictamen"), EntidadResidencia: consulta.Get("entidad_residencia"),
		EntidadConcierto: consulta.Get("entidad_concierto"), Nacionalidad: consulta.Get("nacionalidad"), Pais: consulta.Get("pais"),
		Genero: consulta.Get("genero"), Estreno: consulta.Get("estreno"), ConSubsanaciones: consulta.Get("con_subsanaciones"),
		DocumentosPendientes: consulta.Get("documentos_pendientes"), LigasConProblema: consulta.Get("ligas_con_problema"),
	}
	var errores []models.ErrorCampo
	invalido := func(campo string) {
		errores = append(errores, models.ErrorCampo{Campo: campo, Clave: "error.filtro_invalido"})
	}
	// entero lee un parámetro numérico opcional.
	entero := func(nombre string) int64 {
		n, err := enteroOpcional(consulta.Get(nombre))
		if err != nil {
			invalido(nombre)
		}
		return n
	}
	f.Edicion, f.ConvocatoriaID, f.DotacionID = int(entero("edicion")), entero("convocatoria"), entero("dotacion")
	f.Pagina, f.Tamano = int(entero("pagina")), int(entero("tamano"))
	if f.Pagina == 0 {
		f.Pagina = 1
	}
	// Un guion delante del orden lo invierte: orden=-enviada.
	f.Orden = consulta.Get("orden")
	if strings.HasPrefix(f.Orden, "-") {
		f.Orden, f.Descendente = f.Orden[1:], true
	}
	for nombre, destino := range map[string]*time.Time{"enviada_desde": &f.EnviadaDesde, "enviada_hasta": &f.EnviadaHasta} {
		if texto := consulta.Get(nombre); texto != "" {
			momento, ok := fechaDeFiltro(texto, nombre == "enviada_hasta")
			if !ok {
				invalido(nombre)
			}
			*destino = momento
		}
	}
	// Selección marcada a mano: ids=3,8,21. «postulacion=3» es lo mismo para una sola.
	for _, texto := range strings.FieldsFunc(consulta.Get("ids")+","+consulta.Get("postulacion"), func(r rune) bool { return r == ',' }) {
		id, err := strconv.ParseInt(texto, 10, 64)
		if err != nil || id < 1 {
			invalido("ids")
			continue
		}
		f.IDs = append(f.IDs, id)
	}
	return f, append(errores, models.ValidarFiltroDeListado(&f)...)
}

// GET /api/v1/archivo   — listado del archivo, con búsqueda y filtros
//
// Parámetros: q (texto), edicion, convocatoria, tipo, subcategoria, estado,
// dictamen, dotacion, entidad_residencia, entidad_concierto, nacionalidad,
// pais, genero, estreno, enviada_desde, enviada_hasta, con_subsanaciones,
// documentos_pendientes, ligas_con_problema, ids, orden, pagina y tamano.
// Con formato=csv descarga el resultado completo (RF-ARCH-17).
func (a *App) apiArchivo(w http.ResponseWriter, r *http.Request) {
	alcance := models.AlcanceDe(sesionDe(r))
	if alcance.Vacio() {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return
	}
	filtro, errores := filtroDeLaPeticion(r)
	if len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return
	}
	enCSV := r.URL.Query().Get("formato") == "csv"
	if enCSV {
		filtro.Pagina, filtro.Tamano = 0, 0 // el resultado completo
	}
	renglones, total, err := a.datos.ListarPostulaciones(r.Context(), alcance, filtro)
	if a.falloDatos(w, r, err) {
		return
	}
	if !enCSV {
		a.responder(w, http.StatusOK, views.ListadoJSON(renglones, total, filtro))
		return
	}
	encabezados, filas := views.ListadoCSV(renglones)
	if err := views.CSV(w, "fimnme_postulaciones.csv", encabezados, filas); err != nil {
		a.log.Error("no se pudo escribir el CSV del listado", "error", err)
	}
}

// GET /api/v1/folios/{folio}   — la postulación de un folio
//
// Es el enlace permanente (RF-ARCH-06): un folio citado en un acta resuelve
// aquí aunque pasen los años. Responde lo mismo que la consulta por id, con
// los mismos permisos.
func (a *App) apiPostulacionPorFolio(w http.ResponseWriter, r *http.Request) {
	id, err := a.datos.PostulacionPorFolio(r.Context(), r.PathValue("folio"))
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// GET /api/v1/vistas   — las vistas guardadas de la cuenta
func (a *App) apiVistas(w http.ResponseWriter, r *http.Request) {
	vistas, err := a.datos.VistasGuardadas(r.Context(), sesionDe(r).Usuario.ID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.VistasJSON(vistas))
}

// POST /api/v1/vistas   — guarda una combinación de filtros con nombre
//
// Cuerpo: {"nombre", "ruta"}; la ruta es la del listado con sus parámetros.
func (a *App) apiGuardarVista(w http.ResponseWriter, r *http.Request) {
	var entrada struct {
		Nombre string `json:"nombre"`
		Ruta   string `json:"ruta"`
	}
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	if _, err := a.datos.GuardarVista(r.Context(), sesionDe(r).Usuario.ID, entrada.Nombre, entrada.Ruta); a.falloDatos(w, r, err) {
		return
	}
	a.apiVistas(w, r)
}

// DELETE /api/v1/vistas/{id}   — borra una vista guardada
func (a *App) apiBorrarVista(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	if a.falloDatos(w, r, a.datos.BorrarVista(r.Context(), sesionDe(r).Usuario.ID, id)) {
		return
	}
	a.apiVistas(w, r)
}

// POST /api/v1/exportaciones   — pide una descarga masiva en ZIP
//
// Toma los MISMOS parámetros que el listado del archivo: el ZIP trae los
// documentos de las postulaciones que ese listado mostraría. Además, «tipos»
// limita a ciertos documentos (tipos=partitura,particellas), para no bajar
// gigas de audio cuando sólo se quieren las partituras (RF-ARCH-32).
//
// El ZIP se arma en segundo plano: responde 202 con la tarea, cuyo estado se
// consulta en GET /api/v1/exportaciones/{id}.
func (a *App) apiPedirExportacion(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	filtro, errores := filtroDeLaPeticion(r)
	if len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return
	}
	parametros := models.ParametrosDeZIP{Filtro: filtro, Alcance: models.AlcanceDe(sesion),
		Claves: strings.FieldsFunc(r.URL.Query().Get("tipos"), func(r rune) bool { return r == ',' })}
	if !models.PuedeDescargarEnMasa(sesion) {
		// Quien no puede descargar en masa sólo baja lo suyo: una
		// postulación propia. El jurado tampoco (§2.2).
		if len(filtro.IDs) != 1 {
			a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
			return
		}
		p, err := a.datos.PostulacionPorID(r.Context(), filtro.IDs[0])
		if err == nil && p.UsuarioID != sesion.Usuario.ID {
			err = models.ErrNoEncontrado
		}
		if a.falloDatos(w, r, err) {
			return
		}
		parametros.SoloDe, parametros.Alcance = sesion.Usuario.ID, models.AlcanceDeConsulta{}
	} else if !parametros.Alcance.Todo {
		// Un revisor que además es jurado descarga como revisor, no como jurado.
		parametros.Alcance.ComoJurado = nil
	}
	id, err := a.datos.EncolarExportacion(r.Context(), a.actorDe(r), parametros)
	if a.falloDatos(w, r, err) {
		return
	}
	tarea, err := a.datos.ExportacionPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusAccepted, views.ExportacionJSON(tarea))
}

// exportacionPropia lee una descarga masiva y comprueba que la pidió la
// cuenta en sesión. La de otra persona responde 404.
func (a *App) exportacionPropia(w http.ResponseWriter, r *http.Request) (models.TareaDeExportacion, bool) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return models.TareaDeExportacion{}, false
	}
	tarea, err := a.datos.ExportacionPorID(r.Context(), id)
	if err == nil && tarea.SolicitadoPor != sesionDe(r).Usuario.ID {
		err = models.ErrNoEncontrado
	}
	return tarea, !a.falloDatos(w, r, err)
}

// GET /api/v1/exportaciones/{id}   — estado de una descarga masiva
func (a *App) apiExportacion(w http.ResponseWriter, r *http.Request) {
	if tarea, ok := a.exportacionPropia(w, r); ok {
		a.responder(w, http.StatusOK, views.ExportacionJSON(tarea))
	}
}

// GET /api/v1/exportaciones/{id}/archivo   — baja el ZIP
//
// Sólo quien lo pidió, y durante siete días. Se sirve desde disco, por
// trozos: no se carga en memoria. Queda registrado (RF-ARCH-37).
func (a *App) apiDescargarExportacion(w http.ResponseWriter, r *http.Request) {
	tarea, ok := a.exportacionPropia(w, r)
	if !ok {
		return
	}
	switch {
	case tarea.Estado != models.ExportacionLista:
		a.errorAPI(w, r, http.StatusConflict, "exportacion_no_lista")
		return
	case a.ahora().After(tarea.ExpiraAt):
		a.errorAPI(w, r, http.StatusGone, "exportacion_vencida")
		return
	}
	archivo, err := os.Open(rutaDeExportacion(a.exportaciones, tarea.ID))
	if err != nil {
		a.log.Error("falta el archivo de una descarga masiva lista", "tarea", tarea.ID, "error", err)
		a.errorAPI(w, r, http.StatusGone, "exportacion_vencida")
		return
	}
	defer archivo.Close()
	if esPrimeraLectura(r) {
		if a.falloDatos(w, r, a.datos.RegistrarDescargaDeExportacion(r.Context(), a.actorDe(r), tarea)) {
			return
		}
	}
	cabeceras := w.Header()
	cabeceras.Set("Content-Type", "application/zip")
	cabeceras.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="fimnme_documentos_%d.zip"`, tarea.ID))
	cabeceras.Set("Cache-Control", "private, no-store")
	a.ampliarPlazos(w) // un ZIP de varios gigas tarda en bajar
	http.ServeContent(w, r, "", time.Time{}, archivo)
}

// rutaDeExportacion es el archivo en disco de una descarga masiva.
func rutaDeExportacion(directorio string, tareaID int64) string {
	return filepath.Join(directorio, strconv.FormatInt(tareaID, 10)+".zip")
}

// --- Trabajador de descargas masivas ------------------------------------------

// intervaloExportaciones es cada cuánto se revisa si hay descargas por armar.
const intervaloExportaciones = 5 * time.Second

// TrabajadorExportaciones arma en segundo plano los ZIP de las descargas
// masivas (RF-ARCH-35) y borra los que ya vencieron.
//
// Los ZIP no entran al acervo: son copias temporales que viven siete días en
// su propio directorio. Cada archivo se copia del almacén al ZIP por trozos,
// sin comprimir —partituras y audio ya vienen comprimidos— y sin cargarlo en
// memoria.
type TrabajadorExportaciones struct {
	Datos      models.Persistencia
	Almacen    models.BlobStore
	Directorio string
	Log        *slog.Logger
	// Ahora es el reloj; nil usa el del sistema.
	Ahora func() time.Time
}

// Ejecutar trabaja hasta que se cancele el contexto. Antes de empezar
// devuelve a la cola lo que un apagado anterior dejó a medias.
func (t *TrabajadorExportaciones) Ejecutar(ctx context.Context) {
	if reencoladas, err := t.Datos.ReencolarExportacionesInterrumpidas(ctx); err != nil {
		t.Log.Error("reencolar descargas masivas interrumpidas", "error", err)
	} else if reencoladas > 0 {
		t.Log.Info("descargas masivas interrumpidas devueltas a la cola", "tareas", reencoladas)
	}
	reloj := time.NewTicker(intervaloExportaciones)
	defer reloj.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reloj.C:
			t.Procesar(ctx)
		}
	}
}

// Procesar arma todas las descargas encoladas y limpia las vencidas.
// Devuelve cuántas armó.
func (t *TrabajadorExportaciones) Procesar(ctx context.Context) int {
	if err := os.MkdirAll(t.Directorio, 0o750); err != nil {
		t.Log.Error("crear directorio de descargas masivas", "error", err)
		return 0
	}
	armadas := 0
	for ctx.Err() == nil {
		tarea, hay, err := t.Datos.TomarExportacion(ctx)
		if err != nil {
			t.Log.Error("tomar descarga masiva", "error", err)
			break
		}
		if !hay {
			break
		}
		archivos, tamano, err := t.armar(ctx, tarea)
		if ctx.Err() != nil {
			break // apagado en curso: la tarea se rehará al arrancar
		}
		mensaje := ""
		if err != nil {
			// El detalle va al log; a quien la pidió le basta saber que falló.
			t.Log.Error("armar descarga masiva", "tarea", tarea.ID, "error", err)
			mensaje = "no_se_pudo_armar"
		}
		if err := t.Datos.TerminarExportacion(ctx, tarea.ID, archivos, tamano, mensaje); err != nil {
			t.Log.Error("anotar descarga masiva", "tarea", tarea.ID, "error", err)
			continue
		}
		if mensaje == "" {
			armadas++
		}
	}
	t.limpiar(ctx)
	return armadas
}

// armar escribe el ZIP de una tarea: cada documento en su carpeta y, al
// final, el manifiesto. Escribe en un temporal y lo renombra al terminar,
// para que nunca pueda bajarse un ZIP a medias.
func (t *TrabajadorExportaciones) armar(ctx context.Context, tarea models.TareaDeExportacion) (archivos int, tamano int64, err error) {
	documentos, err := t.Datos.DocumentosParaZIP(ctx, tarea.Parametros)
	if err != nil {
		return 0, 0, err
	}
	destino := rutaDeExportacion(t.Directorio, tarea.ID)
	temporal, err := os.CreateTemp(t.Directorio, "armando-*")
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if errBorrar := os.Remove(temporal.Name()); errBorrar != nil && !errors.Is(errBorrar, os.ErrNotExist) && err == nil {
			err = errBorrar
		}
	}()
	escritor := zip.NewWriter(temporal)
	rutas := make([]string, len(documentos))
	usadas := make(map[string]int)
	for i, d := range documentos {
		if ctx.Err() != nil {
			return 0, 0, ctx.Err() // apagado en curso: la tarea se rehará
		}
		// Si un requisito admite varios archivos, del segundo en adelante
		// llevan su número.
		base := models.RutaEnZIP(d, 1)
		usadas[base]++
		rutas[i] = models.RutaEnZIP(d, usadas[base])
		entrada, err := escritor.CreateHeader(&zip.FileHeader{Name: rutas[i], Method: zip.Store, Modified: d.Documento.SubidoAt})
		if err != nil {
			return 0, 0, err
		}
		contenido, err := t.Almacen.Abrir(d.Documento.Archivo.Clave)
		if err != nil {
			return 0, 0, fmt.Errorf("documento %d: %w", d.Documento.ID, err)
		}
		_, err = io.Copy(entrada, contenido)
		if errCerrar := contenido.Close(); err == nil {
			err = errCerrar
		}
		if err != nil {
			return 0, 0, fmt.Errorf("documento %d: %w", d.Documento.ID, err)
		}
	}
	manifiesto, err := views.ManifiestoCSV(documentos, rutas)
	if err != nil {
		return 0, 0, err
	}
	entrada, err := escritor.Create("manifiesto.csv")
	if err == nil {
		_, err = entrada.Write(manifiesto)
	}
	if err == nil {
		err = escritor.Close()
	}
	if err == nil {
		err = temporal.Sync()
	}
	info, errInfo := temporal.Stat()
	if errCerrar := temporal.Close(); err == nil {
		err = errCerrar
	}
	if err == nil {
		err = errInfo
	}
	if err != nil {
		return 0, 0, err
	}
	if err := os.Chmod(temporal.Name(), 0o640); err != nil {
		return 0, 0, err
	}
	return len(documentos), info.Size(), os.Rename(temporal.Name(), destino)
}

// limpiar borra los ZIP cuyo enlace ya venció, y los temporales que un
// apagado dejó a medias.
func (t *TrabajadorExportaciones) limpiar(ctx context.Context) {
	ahora := time.Now()
	if t.Ahora != nil {
		ahora = t.Ahora()
	}
	entradas, err := os.ReadDir(t.Directorio)
	if err != nil {
		t.Log.Error("leer directorio de descargas masivas", "error", err)
		return
	}
	for _, entrada := range entradas {
		ruta := filepath.Join(t.Directorio, entrada.Name())
		id, err := strconv.ParseInt(strings.TrimSuffix(entrada.Name(), ".zip"), 10, 64)
		vencido := false
		if err != nil {
			// No es un ZIP terminado: un temporal. Si lleva más de un día, sobra.
			info, errInfo := entrada.Info()
			vencido = errInfo == nil && ahora.Sub(info.ModTime()) > 24*time.Hour
		} else if tarea, err := t.Datos.ExportacionPorID(ctx, id); err == nil {
			vencido = !tarea.ExpiraAt.IsZero() && ahora.After(tarea.ExpiraAt)
		}
		if vencido {
			if err := os.Remove(ruta); err != nil {
				t.Log.Error("borrar descarga masiva vencida", "archivo", entrada.Name(), "error", err)
			}
		}
	}
}
