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
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Respuestas de datos de la API (/api/v1): JSON y CSV. Las formas del JSON se
// definen aquí, en la vista, y no se derivan de las entidades: así un cambio
// interno del modelo no altera sin querer el contrato público.

// JSON escribe una respuesta JSON con el estado HTTP dado.
func JSON(w http.ResponseWriter, estado int, datos any) error {
	cuerpo, err := json.Marshal(datos)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(estado)
	_, err = w.Write(append(cuerpo, '\n'))
	return err
}

// ErrorJSON es el cuerpo de toda respuesta de error de la API: un código
// estable, para programas, y un mensaje en el idioma de la petición.
func ErrorJSON(codigo, mensaje string) any {
	return map[string]any{"error": map[string]string{"codigo": codigo, "mensaje": mensaje}}
}

// marcaJSON formatea un instante para la API: ISO-8601 en UTC, o null.
func marcaJSON(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// SesionJSON describe la sesión en curso: quién es y qué puede hacer.
func SesionJSON(s *models.Sesion) any {
	perfiles := make([]string, 0, len(s.Perfiles))
	for _, p := range s.Perfiles {
		perfiles = append(perfiles, string(p))
	}
	asignaciones := make([]map[string]any, 0, len(s.Asignaciones))
	for _, a := range s.Asignaciones {
		asignaciones = append(asignaciones, map[string]any{"convocatoria_id": a.ConvocatoriaID, "perfil": string(a.Perfil)})
	}
	return map[string]any{
		"usuario": map[string]any{
			"id":               s.Usuario.ID,
			"email":            s.Usuario.Email,
			"nombre":           s.Usuario.Nombre,
			"apellidos":        s.Usuario.Apellidos,
			"idioma":           s.Usuario.Idioma,
			"email_verificado": s.Usuario.EmailVerificado(),
		},
		"perfiles":     perfiles,
		"asignaciones": asignaciones,
		"expira_at":    marcaJSON(s.ExpiraAt),
	}
}

// jsonONulo incrusta un documento JSON guardado como texto, o null si falta.
func jsonONulo(texto string) any {
	if texto == "" || !json.Valid([]byte(texto)) {
		return nil
	}
	return json.RawMessage(texto)
}

// ceroANulo convierte el 0 del dominio («sin valor») en null.
func ceroANulo(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// AuditoriaJSON es una página de la bitácora con su paginación.
func AuditoriaJSON(asientos []models.AsientoAuditoria, total, pagina, tamano int) any {
	registros := make([]map[string]any, 0, len(asientos))
	for _, a := range asientos {
		registros = append(registros, map[string]any{
			"id":           a.ID,
			"creado_at":    marcaJSON(a.CreadoAt),
			"actor_id":     ceroANulo(a.ActorID),
			"actor_perfil": a.ActorPerfil,
			"accion":       a.Accion,
			"entidad":      a.Entidad,
			"entidad_id":   ceroANulo(a.EntidadID),
			"antes":        jsonONulo(a.Antes),
			"despues":      jsonONulo(a.Despues),
			"ip":           a.IP,
			"user_agent":   a.Agente,
		})
	}
	return map[string]any{"total": total, "pagina": pagina, "tamano": tamano, "registros": registros}
}

// AuditoriaCSV convierte asientos de la bitácora en filas de CSV. Los
// encabezados son los nombres de columna del diccionario de datos.
func AuditoriaCSV(asientos []models.AsientoAuditoria) (encabezados []string, filas [][]string) {
	encabezados = []string{"id", "creado_at", "actor_id", "actor_perfil", "accion", "entidad",
		"entidad_id", "antes", "despues", "ip", "user_agent"}
	id := func(n int64) string {
		if n == 0 {
			return ""
		}
		return strconv.FormatInt(n, 10)
	}
	for _, a := range asientos {
		filas = append(filas, []string{
			strconv.FormatInt(a.ID, 10), a.CreadoAt.UTC().Format(time.RFC3339), id(a.ActorID), a.ActorPerfil,
			a.Accion, a.Entidad, id(a.EntidadID), a.Antes, a.Despues, a.IP, a.Agente,
		})
	}
	return encabezados, filas
}

// CSV escribe una descarga CSV separada por comas. Véase CSVConSeparador.
func CSV(w http.ResponseWriter, archivo string, encabezados []string, filas [][]string) error {
	return CSVConSeparador(w, archivo, "coma", encabezados, filas)
}
