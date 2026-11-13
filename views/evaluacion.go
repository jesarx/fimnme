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

import "github.com/jesarx/fimnme/models"

// Respuestas JSON de la evaluación por jurado.
//
// Hay dos familias que no se mezclan: lo que ve un jurado (su lista de
// trabajo y SU evaluación) y lo que ve la supervisión (la matriz con los tres
// jurados). Las funciones del jurado no reciben nunca datos de otro jurado.

// unidadJSON describe una unidad de evaluación.
func unidadJSON(u models.UnidadDeEvaluacion) map[string]any {
	return map[string]any{
		"postulacion_id": u.PostulacionID, "obra_id": ceroANulo(u.ObraID), "folio": u.Folio, "nombre": u.Nombre,
		"obra": textoONulo(u.Obra), "orden_de_obra": ceroANulo(int64(u.OrdenDeObra)),
	}
}

// criteriosJSON describe los criterios con que se califica.
func criteriosJSON(criterios []models.CriterioEvaluacion) []map[string]any {
	lista := make([]map[string]any, 0, len(criterios))
	for _, c := range criterios {
		lista = append(lista, map[string]any{
			"clave": c.Clave, "etiqueta_es": c.EtiquetaES, "etiqueta_en": c.EtiquetaEN,
			"descripcion_es": c.DescripcionES, "descripcion_en": c.DescripcionEN,
			"escala_min": c.EscalaMin, "escala_max": c.EscalaMax, "obligatorio": c.Obligatorio,
		})
	}
	return lista
}

// EvaluacionJSON describe la evaluación de un jurado a una unidad: la suya.
func EvaluacionJSON(e models.Evaluacion) map[string]any {
	puntajes := e.Puntajes
	if puntajes == nil {
		puntajes = map[string]int{}
	}
	return map[string]any{
		"postulacion_id": e.PostulacionID, "obra_id": ceroANulo(e.ObraID), "puntajes": puntajes,
		"observaciones": e.Observaciones, "iniciada": e.ID != 0, "enviada": e.Enviada,
		"excusado": e.Excusado, "motivo_excusa": textoONulo(e.MotivoExcusa),
		"enviada_at": marcaJSON(e.EnviadaAt), "actualizado_at": marcaJSON(e.ActualizadoAt),
	}
}

// ListaDeJuradoJSON es la lista de trabajo de un jurado: las unidades por
// evaluar, el estado de su evaluación de cada una y su avance («has evaluado
// 34 de 87», RF-EVA-07). abierta indica si aún puede calificar.
func ListaDeJuradoJSON(unidades []models.UnidadPropia, criterios []models.CriterioEvaluacion, abierta bool) any {
	lista := make([]map[string]any, 0, len(unidades))
	enviadas, excusadas := 0, 0
	for _, u := range unidades {
		unidad := unidadJSON(u.UnidadDeEvaluacion)
		unidad["iniciada"], unidad["enviada"], unidad["excusada"] = u.Iniciada, u.Enviada, u.Excusada
		lista = append(lista, unidad)
		switch {
		case u.Excusada:
			excusadas++
		case u.Enviada:
			enviadas++
		}
	}
	return map[string]any{
		"abierta": abierta, "criterios": criteriosJSON(criterios), "unidades": lista,
		// pendientes: lo que al jurado le falta por enviar. Una unidad de la
		// que se excusó no está pendiente.
		"progreso": map[string]any{"enviadas": enviadas, "excusadas": excusadas, "total": len(unidades),
			"pendientes": len(unidades) - enviadas - excusadas},
	}
}

// MatrizJSON es la matriz de resultados, para supervisión (RF-EVA-08): por
// unidad, lo que calificó cada jurado, quién se excusó, la suma, el promedio,
// el puntaje comparable y el lugar.
func MatrizJSON(m models.MatrizDeResultados) any {
	jurados := make([]map[string]any, 0, len(m.Jurados))
	for _, j := range m.Jurados {
		jurados = append(jurados, map[string]any{"id": j.ID, "nombre": j.Nombre})
	}
	filas := make([]map[string]any, 0, len(m.Filas))
	for i, f := range m.Filas {
		evaluaciones := make([]map[string]any, 0, len(m.Jurados))
		for _, j := range m.Jurados {
			e, enviada := f.PorJurado[j.ID]
			if motivo, excusado := f.Excusas[j.ID]; excusado {
				// Conflicto de interés: no califica, y no cuenta en el puntaje.
				evaluaciones = append(evaluaciones, map[string]any{"jurado_id": j.ID, "enviada": false, "excusado": true, "motivo_excusa": motivo})
				continue
			}
			if !enviada {
				// Sin enviar: ni puntajes ni observaciones cuentan todavía.
				evaluaciones = append(evaluaciones, map[string]any{"jurado_id": j.ID, "enviada": false, "excusado": false})
				continue
			}
			evaluaciones = append(evaluaciones, map[string]any{
				"jurado_id": j.ID, "enviada": true, "excusado": false, "puntajes": e.Puntajes,
				"observaciones": e.Observaciones, "enviada_at": marcaJSON(e.EnviadaAt),
			})
		}
		fila := unidadJSON(f.Unidad)
		fila["lugar"] = i + 1
		fila["estado"], fila["dictamen_final"] = string(f.Estado), textoONulo(f.DictamenFinal)
		fila["evaluaciones"] = evaluaciones
		fila["jurados_que_evaluaron"], fila["total"], fila["promedio"] = f.JuradosQueEvaluaron, f.Total, f.Promedio
		// puntaje: la suma llevada a tres jurados. Es por lo que se ordena,
		// para que una excusa no perjudique a la postulación.
		fila["excusados"], fila["puntaje"] = len(f.Excusas), f.Comparable
		filas = append(filas, fila)
	}
	return map[string]any{"jurados": jurados, "criterios": criteriosJSON(m.Criterios), "filas": filas}
}
