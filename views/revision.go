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
	"time"

	"github.com/jesarx/fimnme/models"
)

// Respuestas JSON de la revisión documental y de la subsanación.

// NombreDeElemento traduce un elemento señalado —un campo o un documento— a
// un nombre legible, en un idioma dado. Lo arma el controlador, que conoce
// los requisitos de la convocatoria.
type NombreDeElemento func(clase models.ClaseProblema, campo string) string

// problemaJSON describe un elemento señalado en una subsanación.
func problemaJSON(p models.ProblemaSubsanacion, nombrar NombreDeElemento) map[string]any {
	return map[string]any{
		"id": p.ID, "clase": string(p.Clase), "campo": p.Campo, "obra_id": ceroANulo(p.ObraID),
		"etiqueta": nombrar(p.Clase, p.Campo), "problema": p.Problema, "resuelto_at": marcaJSON(p.ResueltoAt),
	}
}

// subsanacionJSON describe una subsanación con sus problemas y solicitudes.
func subsanacionJSON(s models.Subsanacion, ahora time.Time, nombrar NombreDeElemento) map[string]any {
	problemas := make([]map[string]any, 0, len(s.Problemas))
	for _, p := range s.Problemas {
		problemas = append(problemas, problemaJSON(p, nombrar))
	}
	solicitudes := make([]map[string]any, 0, len(s.Solicitudes))
	for _, d := range s.Solicitudes {
		var concedida any
		if d.Resuelta {
			concedida = d.Concedida
		}
		solicitudes = append(solicitudes, map[string]any{
			"id": d.ID, "clase": string(d.Clase), "campo": d.Campo, "obra_id": ceroANulo(d.ObraID),
			"etiqueta": nombrar(d.Clase, d.Campo), "justificacion": d.Justificacion,
			"concedida": concedida, "motivo_respuesta": textoONulo(d.MotivoRespuesta),
			"creado_at": marcaJSON(d.CreadoAt), "resuelta_at": marcaJSON(d.ResueltaAt),
		})
	}
	return map[string]any{
		"id": s.ID, "mensaje": s.Mensaje, "fecha_limite": marcaJSON(s.FechaLimite), "enviada_at": marcaJSON(s.EnviadaAt),
		"atendida_at": marcaJSON(s.AtendidaAt), "cerrada_at": marcaJSON(s.CerradaAt),
		// vencida: sigue pendiente y su plazo ya pasó (RF-REV-09).
		"vencida":   s.Pendiente() && ahora.After(s.FechaLimite),
		"problemas": problemas, "solicitudes": solicitudes,
	}
}

// SubsanacionPendienteJSON describe la subsanación que espera al postulante,
// o null si no hay ninguna. Sus «problemas» son exactamente los elementos que
// puede modificar: nada más está desbloqueado.
func SubsanacionPendienteJSON(p models.Postulacion, ahora time.Time, nombrar NombreDeElemento) any {
	if p.Estado != models.PostulacionEnSubsanacion {
		return nil
	}
	for i := len(p.Subsanaciones) - 1; i >= 0; i-- {
		if s := p.Subsanaciones[i]; s.Pendiente() {
			return subsanacionJSON(s, ahora, nombrar)
		}
	}
	return nil
}

// RevisionJSON describe lo que la revisión documental ha dejado sobre una
// postulación. Sólo se entrega a quien revisa o supervisa: el postulante no
// ve dictámenes, observaciones ni el motivo de un descarte.
func RevisionJSON(p models.Postulacion, titular models.Usuario, revision models.Revision, cambios []models.CambioDeEnvio,
	ahora time.Time, nombrar NombreDeElemento) map[string]any {
	dictamenes := make([]map[string]any, 0, len(revision.Dictamenes))
	for _, d := range revision.Dictamenes {
		dictamenes = append(dictamenes, map[string]any{
			"documento_id": d.DocumentoID, "dictamen": d.Dictamen, "motivo": textoONulo(d.Motivo),
			"revisor_id": d.RevisorID, "creado_at": marcaJSON(d.CreadoAt),
		})
	}
	observaciones := make([]map[string]any, 0, len(revision.Observaciones))
	for _, o := range revision.Observaciones {
		observaciones = append(observaciones, map[string]any{
			"id": o.ID, "obra_id": ceroANulo(o.ObraID), "revisor_id": o.RevisorID, "revisor": o.Revisor,
			"texto": o.Texto, "creado_at": marcaJSON(o.CreadoAt),
		})
	}
	subsanaciones := make([]map[string]any, 0, len(p.Subsanaciones))
	for _, s := range p.Subsanaciones {
		subsanaciones = append(subsanaciones, subsanacionJSON(s, ahora, nombrar))
	}
	diferencias := make([]map[string]any, 0, len(cambios))
	for _, c := range cambios {
		diferencias = append(diferencias, map[string]any{
			"clase": string(c.Clase), "campo": c.Campo, "obra_id": ceroANulo(c.ObraID), "etiqueta": nombrar(c.Clase, c.Campo),
			"antes": textoONulo(c.Antes), "despues": textoONulo(c.Despues),
		})
	}
	return map[string]any{
		"titular":    map[string]any{"id": titular.ID, "nombre": titular.Nombre, "apellidos": titular.Apellidos, "email": titular.Email},
		"enviada_at": marcaJSON(p.EnviadaAt), "motivo_descarte": textoONulo(p.MotivoDescarte),
		"toma":       tomaJSON(revision.Toma),
		"dictamenes": dictamenes, "observaciones": observaciones, "subsanaciones": subsanaciones,
		// cambios: lo que el postulante modificó en su último reenvío.
		"cambios":             diferencias,
		"subsanacion_vencida": p.SubsanacionVencida(ahora),
	}
}

// tomaJSON describe quién tiene tomada una postulación; null si está libre.
func tomaJSON(t *models.TomaRevision) any {
	if t == nil {
		return nil
	}
	return map[string]any{"revisor_id": t.RevisorID, "revisor": t.Revisor, "tomada_at": marcaJSON(t.TomadaAt)}
}

// BandejaJSON es la bandeja del revisor: las postulaciones enviadas de una
// convocatoria y el tablero de avance (RF-REV-01 y RF-REV-10). tablero cuenta
// TODAS las postulaciones de la convocatoria, no sólo las que pasan el filtro.
func BandejaJSON(bandeja []models.PostulacionEnBandeja, tablero map[models.EstadoDePostulacion]int, ahora time.Time) any {
	renglones := make([]map[string]any, 0, len(bandeja))
	for _, b := range bandeja {
		renglones = append(renglones, map[string]any{
			"id": b.ID, "folio": b.Folio, "estado": string(b.Estado), "subcategoria": textoONulo(string(b.Subcategoria)),
			"nombre": b.Nombre, "enviada_at": marcaJSON(b.EnviadaAt), "envios": b.Envios,
			"toma": tomaJSON(b.Toma), "observaciones": b.Observaciones, "documentos_invalidos": b.DocumentosInvalidos,
			"fecha_limite": marcaJSON(b.FechaLimite),
			"vencida":      b.Estado == models.PostulacionEnSubsanacion && !b.FechaLimite.IsZero() && ahora.After(b.FechaLimite),
			"subsanada":    b.Subsanada,
		})
	}
	total := 0
	cuentas := make(map[string]int)
	for estado, n := range tablero {
		cuentas[string(estado)] = n
		total += n
	}
	return map[string]any{"postulaciones": renglones, "tablero": map[string]any{"total": total, "por_estado": cuentas}}
}

// textoONulo devuelve el texto, o null si está vacío.
func textoONulo(texto string) any {
	if texto == "" {
		return nil
	}
	return texto
}
