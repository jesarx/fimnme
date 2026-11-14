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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Reportes CSV (§5.7), plantillas de reporte y tableros (§5.13).
//
// El modelo entrega cada valor como se guarda —la clave de una opción, la
// fecha en ISO-8601— y aquí se escribe en la lengua del reporte: «Estreno
// mundial», «Jalisco», «Validada».

// etiquetaDeCampo devuelve el encabezado de una columna del reporte.
//
//   - Un puntaje por jurado lleva el nombre del jurado, si la convocatoria ya
//     lo tiene asignado, y el criterio: «María Ruiz · Realización».
//   - Los campos calculados tienen su propia clave de traducción.
//   - Los del formulario usan la misma etiqueta que el formulario.
func (m *Motor) etiquetaDeCampo(idioma string, campo models.CampoDeReporte, jurados []models.JuradoEnMatriz) string {
	if campo.Jurado > 0 {
		jurado := m.textos.T(idioma, "reporte.jurado", campo.Jurado)
		if campo.Jurado <= len(jurados) {
			jurado = jurados[campo.Jurado-1].Nombre
		}
		if campo.Criterio == nil {
			return jurado + " · " + m.textos.T(idioma, "expediente.observaciones_jurado")
		}
		criterio := campo.Criterio.EtiquetaES
		if idioma == models.IdiomaEN && campo.Criterio.EtiquetaEN != "" {
			criterio = campo.Criterio.EtiquetaEN
		}
		return jurado + " · " + criterio
	}
	if _, calculado := m.textos.Claves(models.IdiomaES)["reporte.campo."+campo.Clave]; calculado {
		return m.textos.T(idioma, "reporte.campo."+campo.Clave)
	}
	return m.textos.T(idioma, "campo."+campo.Clave)
}

// presentar escribe el valor de un campo en la lengua del reporte.
func (m *Motor) presentar(idioma string, campo models.CampoDeReporte, valor string) string {
	if valor == "" {
		return ""
	}
	traducido := func(prefijo string) string { return m.textos.T(idioma, prefijo+valor) }
	switch campo.Tipo {
	case models.CampoOpcion:
		return traducido("opcion.")
	case models.CampoEstado:
		return traducido("estado.postulacion.")
	case models.CampoResultado:
		return traducido("resultado.")
	case models.CampoRevision:
		return traducido("expediente.revision.")
	case models.CampoSubcategoria:
		return traducido("subcategoria.")
	case models.CampoEntidad, models.CampoEntidadResidencia:
		for _, entidad := range models.EntidadesFederativas() {
			if entidad.Clave == valor {
				return entidad.Nombre
			}
		}
	case models.CampoMarca:
		// Fecha y hora de la Ciudad de México, en un formato que una hoja de
		// cálculo ordena bien.
		if momento, err := time.Parse(time.RFC3339, valor); err == nil {
			return momento.In(zonaMexico).Format("2006-01-02 15:04")
		}
	}
	if campo.Jurado > 0 && valor == "excusa" {
		return m.textos.T(idioma, "expediente.excusa")
	}
	return valor
}

// CatalogoDeReporteJSON es el catálogo de campos de una convocatoria, con sus
// etiquetas en el idioma pedido, para armar un reporte (RF-REP-02).
func (m *Motor) CatalogoDeReporteJSON(idioma string, cfg models.ConfiguracionConvocatoria, jurados []models.JuradoEnMatriz) any {
	campos := []map[string]any{}
	for _, campo := range models.CatalogoDeReporte(cfg) {
		campos = append(campos, map[string]any{"clave": campo.Clave, "etiqueta": m.etiquetaDeCampo(idioma, campo, jurados), "nivel": campo.Nivel})
	}
	return map[string]any{
		"campos":                     campos,
		"granularidades":             []string{models.GranularidadPostulacion, models.GranularidadObra},
		"granularidad_de_evaluacion": models.GranularidadDeEvaluacion(cfg.Convocatoria.Tipo),
		"separadores":                []string{"coma", "punto_y_coma"},
	}
}

// ReporteCSV convierte los renglones de un reporte en encabezados y filas de
// texto, en el idioma pedido.
func (m *Motor) ReporteCSV(idioma string, campos []models.CampoDeReporte, renglones []models.FilaDeReporte, jurados []models.JuradoEnMatriz) (encabezados []string, filas [][]string) {
	for _, campo := range campos {
		encabezados = append(encabezados, m.etiquetaDeCampo(idioma, campo, jurados))
	}
	for _, renglon := range renglones {
		fila := make([]string, 0, len(campos))
		for _, campo := range campos {
			fila = append(fila, m.presentar(idioma, campo, models.ValorDeReporte(campo, renglon)))
		}
		filas = append(filas, fila)
	}
	return encabezados, filas
}

// ResultadosCSV es el reporte de resultados de una convocatoria (RF-REP-08):
// la matriz en el orden de su puntaje, con el lugar de cada unidad —los
// empates comparten lugar— y lo que calificó cada jurado.
func (m *Motor) ResultadosCSV(idioma string, matriz models.MatrizDeResultados) (encabezados []string, filas [][]string) {
	t := func(clave string, args ...any) string { return m.textos.T(idioma, clave, args...) }
	criterio := func(c models.CriterioEvaluacion) string {
		if idioma == models.IdiomaEN && c.EtiquetaEN != "" {
			return c.EtiquetaEN
		}
		return c.EtiquetaES
	}
	encabezados = []string{t("reporte.campo.eval.lugar"), t("reporte.campo.folio"), t("reporte.postulante"), t("reporte.obra"),
		t("reporte.campo.estado"), t("reporte.campo.eval.puntaje"), t("reporte.campo.eval.total"), t("reporte.campo.eval.promedio"),
		t("reporte.campo.eval.jurados")}
	for _, jurado := range matriz.Jurados {
		for _, c := range matriz.Criterios {
			encabezados = append(encabezados, jurado.Nombre+" · "+criterio(c))
		}
	}
	encabezados = append(encabezados, t("reporte.campo.dictamen"))
	decimal := func(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
	lugares := models.LugaresDeLaMatriz(matriz.Filas)
	for i, f := range matriz.Filas {
		fila := []string{"", f.Unidad.Folio, f.Unidad.Nombre, f.Unidad.Obra, t("estado.postulacion." + string(f.Estado)), "", "", "",
			strconv.Itoa(f.JuradosQueEvaluaron)}
		if lugares[i] > 0 {
			fila[0], fila[5], fila[6], fila[7] = strconv.Itoa(lugares[i]), decimal(f.Comparable), strconv.Itoa(f.Total), decimal(f.Promedio)
		}
		for _, jurado := range matriz.Jurados {
			evaluacion, enviada := f.PorJurado[jurado.ID]
			_, excusado := f.Excusas[jurado.ID]
			for _, c := range matriz.Criterios {
				switch puntaje, calificado := evaluacion.Puntajes[c.Clave]; {
				case excusado:
					fila = append(fila, t("expediente.excusa"))
				case enviada && calificado:
					fila = append(fila, strconv.Itoa(puntaje))
				default:
					fila = append(fila, "")
				}
			}
		}
		resultado := ""
		if f.DictamenFinal != "" {
			resultado = t("resultado." + f.DictamenFinal)
		}
		filas = append(filas, append(fila, resultado))
	}
	return encabezados, filas
}

// CSVConSeparador escribe una descarga CSV conforme a RF-REP-05: UTF-8 con
// BOM (para que Excel en Windows abra bien los acentos), separador a elegir
// («coma» o «punto_y_coma», que es el que espera Excel en español), comillas
// según RFC 4180 y fin de línea CRLF. Se arma en memoria: si algo falla no se
// envía medio archivo.
func CSVConSeparador(w http.ResponseWriter, archivo, separador string, encabezados []string, filas [][]string) error {
	var salida bytes.Buffer
	salida.WriteString("\xEF\xBB\xBF") // marca de orden de bytes (BOM) de UTF-8
	escritor := csv.NewWriter(&salida)
	escritor.UseCRLF = true
	if separador == "punto_y_coma" {
		escritor.Comma = ';'
	}
	if err := escritor.Write(encabezados); err != nil {
		return err
	}
	if err := escritor.WriteAll(filas); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+archivo+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, err := salida.WriteTo(w)
	return err
}

// PlantillasDeReporteJSON lista las plantillas de reporte guardadas.
func PlantillasDeReporteJSON(plantillas []models.PlantillaDeReporte) any {
	lista := make([]map[string]any, 0, len(plantillas))
	for _, p := range plantillas {
		lista = append(lista, map[string]any{"id": p.ID, "nombre": p.Nombre, "tipo": string(p.Tipo), "consulta": p.Consulta,
			"creado_por": p.CreadoPor, "creado_at": marcaJSON(p.CreadoAt)})
	}
	return map[string]any{"plantillas": lista}
}

// cifraJSON describe una cifra de un tablero. «filtro» es la consulta del
// archivo que la produce (RF-DASH-05): con ella, la interfaz lleva de la
// cifra al listado de las postulaciones que cuenta. Es null cuando el
// listado no tiene un filtro equivalente.
func cifraJSON(clave string, cantidad int, filtro url.Values) map[string]any {
	cifra := map[string]any{"clave": clave, "cantidad": cantidad, "filtro": nil}
	if filtro != nil {
		cifra["filtro"] = filtro.Encode()
	}
	return cifra
}

// TableroJSON es el tablero de una convocatoria (RF-DASH-01 y 02). avance es
// el de cada jurado, sin puntajes; nil si la convocatoria no tiene jurado.
func TableroJSON(convocatoriaID int64, t models.TableroDeConvocatoria, avance []models.AvanceDeJurado) any {
	// base es el filtro de la convocatoria; con crea una copia con algo más.
	con := func(pares ...string) url.Values {
		filtro := url.Values{"convocatoria": {strconv.FormatInt(convocatoriaID, 10)}}
		for i := 0; i+1 < len(pares); i += 2 {
			filtro.Set(pares[i], pares[i+1])
		}
		return filtro
	}
	porEstado := []map[string]any{}
	total := 0
	for _, c := range t.PorEstado {
		porEstado = append(porEstado, cifraJSON(c.Clave, c.Cantidad, con("estado", c.Clave)))
		total += c.Cantidad
	}
	// Ritmo de envíos: primeros envíos por día de la Ciudad de México.
	porDia := []map[string]any{}
	for _, momento := range t.PrimerosEnvios {
		dia := momento.In(zonaMexico).Format("2006-01-02")
		if ultimo := len(porDia) - 1; ultimo >= 0 && porDia[ultimo]["clave"] == dia {
			porDia[ultimo]["cantidad"] = porDia[ultimo]["cantidad"].(int) + 1
			continue
		}
		porDia = append(porDia, cifraJSON(dia, 1, nil))
	}
	jurados := []map[string]any{}
	for _, a := range avance {
		jurados = append(jurados, map[string]any{"jurado_id": a.JuradoID, "nombre": a.Nombre, "enviadas": a.Enviadas,
			"excusadas": a.Excusadas, "pendientes": a.Total - a.Enviadas - a.Excusadas, "total": a.Total})
	}
	return map[string]any{
		"postulaciones":  cifraJSON("total", total, con()),
		"por_estado":     porEstado,
		"envios_por_dia": porDia,
		"documentos_pendientes": map[string]any{
			"documentos":    t.DocumentosPendientes,
			"postulaciones": cifraJSON("documentos_pendientes", t.PostulacionesConPendientes, con("documentos_pendientes", "si")),
		},
		"subsanaciones": map[string]any{
			"abiertas":        cifraJSON("abiertas", t.SubsanacionesAbiertas, con("estado", "en_subsanacion")),
			"por_vencer":      cifraJSON("por_vencer", t.SubsanacionesPorVencer, nil),
			"vencidas":        cifraJSON("vencidas", t.SubsanacionesVencidas, nil),
			"dias_por_vencer": models.DiasPorVencer,
		},
		"evaluacion": jurados,
	}
}

// EstadisticasJSON son las estadísticas descriptivas de una edición
// (RF-DASH-03), cada cifra con el filtro del archivo que la produce.
func EstadisticasJSON(numeroDeEdicion int, convocatoriaID int64, e models.EstadisticasDeEdicion) any {
	// lista convierte una distribución; parametro es el filtro del archivo
	// que corresponde a esa dimensión, o vacío si no lo hay.
	lista := func(cifras []models.Cifra, parametro string) []map[string]any {
		salida := []map[string]any{}
		for _, c := range cifras {
			var filtro url.Values
			if parametro != "" {
				filtro = url.Values{"edicion": {strconv.Itoa(numeroDeEdicion)}, parametro: {c.Clave}}
				if convocatoriaID != 0 {
					filtro.Set("convocatoria", strconv.FormatInt(convocatoriaID, 10))
				}
			}
			cifra := cifraJSON(c.Clave, c.Cantidad, filtro)
			if c.Nombre != "" {
				cifra["nombre"] = c.Nombre
			}
			salida = append(salida, cifra)
		}
		return salida
	}
	return map[string]any{
		"postulaciones": e.Postulaciones, "obras": e.Obras, "duracion_media_min": e.DuracionMediaMin,
		"por_convocatoria": lista(e.PorConvocatoria, "tipo"), "por_entidad": lista(e.PorEntidad, "entidad_residencia"),
		"por_pais": lista(e.PorPais, "pais"), "por_nacionalidad": lista(e.PorNacionalidad, "nacionalidad"),
		"por_genero": lista(e.PorGenero, "genero"), "por_edad": lista(e.PorEdad, ""),
		"por_dotacion": lista(e.PorDotacion, "dotacion"), "por_estreno": lista(e.PorEstreno, "estreno"),
		"rangos_de_edad": strings.Join(models.RangosDeEdad, ","),
	}
}
