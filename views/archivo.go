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

package views

import (
	"bytes"
	"encoding/csv"
	"strconv"

	"github.com/jesarx/fimnme/models"
)

// Respuestas del archivo histórico: listado, vistas guardadas y descargas
// masivas.

// renglonJSON describe una postulación en el listado del archivo.
func renglonJSON(r models.RenglonDeListado) map[string]any {
	return map[string]any{
		"id": r.ID, "folio": textoONulo(r.Folio), "estado": string(r.Estado), "subcategoria": textoONulo(string(r.Subcategoria)),
		"nombre": r.Nombre, "edicion": r.Edicion, "convocatoria_id": r.ConvocatoriaID, "tipo": string(r.Tipo),
		"convocatoria": r.Convocatoria, "enviada_at": marcaJSON(r.EnviadaAt), "obras": r.Obras, "duracion_min": r.DuracionMin,
	}
}

// ListadoJSON es una página del archivo, con el total de coincidencias
// (RF-ARCH-16).
func ListadoJSON(renglones []models.RenglonDeListado, total int, f models.FiltroDeListado) any {
	lista := make([]map[string]any, 0, len(renglones))
	for _, r := range renglones {
		lista = append(lista, renglonJSON(r))
	}
	orden := f.Orden
	if f.Descendente {
		orden = "-" + orden
	}
	return map[string]any{"postulaciones": lista, "total": total, "pagina": f.Pagina, "tamano": f.Tamano, "orden": orden}
}

// ListadoCSV es el resultado de una búsqueda o un filtro, para exportarlo tal
// como se ve (RF-ARCH-17).
func ListadoCSV(renglones []models.RenglonDeListado) (encabezados []string, filas [][]string) {
	encabezados = []string{"folio", "estado", "edicion", "convocatoria", "subcategoria", "nombre", "enviada_at", "obras", "duracion_min", "id"}
	for _, r := range renglones {
		enviada := ""
		if !r.EnviadaAt.IsZero() {
			enviada = r.EnviadaAt.UTC().Format(formatoSnapshot)
		}
		filas = append(filas, []string{r.Folio, string(r.Estado), strconv.Itoa(r.Edicion), string(r.Tipo), string(r.Subcategoria), r.Nombre,
			enviada, strconv.Itoa(r.Obras), strconv.Itoa(r.DuracionMin), strconv.FormatInt(r.ID, 10)})
	}
	return encabezados, filas
}

// VistasJSON lista las vistas guardadas de una cuenta.
func VistasJSON(vistas []models.VistaGuardada) any {
	lista := make([]map[string]any, 0, len(vistas))
	for _, v := range vistas {
		lista = append(lista, map[string]any{"id": v.ID, "nombre": v.Nombre, "ruta": v.Ruta, "creado_at": marcaJSON(v.CreadoAt)})
	}
	return map[string]any{"vistas": lista}
}

// ExportacionJSON describe una descarga masiva: en qué va y, cuando está
// lista, cuánto pesa y hasta cuándo puede bajarse.
func ExportacionJSON(t models.TareaDeExportacion) any {
	return map[string]any{
		"id": t.ID, "estado": t.Estado, "progreso": t.Progreso, "error": textoONulo(t.MensajeError),
		"archivos": t.Archivos, "bytes": t.Bytes, "creado_at": marcaJSON(t.CreadoAt),
		"terminado_at": marcaJSON(t.TerminadoAt), "expira_at": marcaJSON(t.ExpiraAt),
	}
}

// ManifiestoCSV compone el manifiesto que acompaña a una descarga masiva
// (RF-ARCH-34): por cada archivo del ZIP, su folio, su postulante, su obra y
// el SHA-256 del original, para poder comprobar que nada se alteró. rutas[i]
// es el lugar del documento i dentro del ZIP.
func ManifiestoCSV(documentos []models.DocumentoEnZIP, rutas []string) ([]byte, error) {
	var salida bytes.Buffer
	salida.WriteString("\xEF\xBB\xBF") // marca de orden de bytes (BOM) de UTF-8, para Excel
	escritor := csv.NewWriter(&salida)
	escritor.UseCRLF = true
	if err := escritor.Write([]string{"archivo", "folio", "postulante", "obra_numero", "obra", "documento", "version", "bytes", "sha256"}); err != nil {
		return nil, err
	}
	for i, d := range documentos {
		obra := ""
		if d.OrdenDeObra > 0 {
			obra = strconv.Itoa(d.OrdenDeObra)
		}
		if err := escritor.Write([]string{rutas[i], d.Folio, d.Nombre, obra, d.Obra, d.Documento.Clave,
			strconv.Itoa(d.Documento.Version), strconv.FormatInt(d.Documento.Archivo.TamanoBytes, 10), d.Documento.Archivo.SHA256}); err != nil {
			return nil, err
		}
	}
	escritor.Flush()
	return salida.Bytes(), escritor.Error()
}

// PublicacionJSON describe la publicación de la nómina hacia el portal
// público: si puede publicarse, lo que se publicaría ahora mismo y lo que ya
// se publicó (RF-PUB-07).
func PublicacionJSON(puedePublicar bool, vistaPrevia []models.InteropSeleccionado, publicaciones []models.PublicacionInterop) any {
	historial := make([]map[string]any, 0, len(publicaciones))
	for _, p := range publicaciones {
		historial = append(historial, map[string]any{"id": p.ID, "sha256": p.SHA256, "autorizado_por": p.AutorizadoPor, "creado_at": marcaJSON(p.CreadoAt)})
	}
	return map[string]any{"puede_publicar": puedePublicar, "vista_previa": map[string]any{"seleccionados": vistaPrevia}, "publicaciones": historial}
}

// AnonimizacionJSON describe lo que hizo una anonimización. pendientes es
// cuántos archivos marcados como purgados no pudieron borrarse del disco.
func AnonimizacionJSON(r models.ResultadoDeAnonimizacion, pendientes int) any {
	return map[string]any{
		"postulacion_id": r.PostulacionID, "folio": r.Folio, "archivos_purgados": len(r.ArchivosPorPurgar) - pendientes,
		"archivos_por_purgar": pendientes, "archivos_conservados": r.ArchivosConservados, "cuenta_anonimizada": r.CuentaAnonimizada,
	}
}
