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
	"strings"
	"time"

	"github.com/jesarx/fimnme/views"
)

// Fechas y horas en los formularios HTML, con la convención mexicana:
// dd/mm/aaaa y hora de 24 horas.
//
// Los controles nativos del navegador (<input type="date">, "datetime-local"
// y "time") muestran el formato del IDIOMA DEL NAVEGADOR, no el de la página:
// en un navegador en inglés salen como mm/dd/yyyy y con AM/PM, y la página no
// puede cambiarlo. Por eso los formularios usan campos de texto con el
// formato a la vista, y estas funciones traducen entre lo que se escribe y lo
// que se guarda (AAAA-MM-DD, HH:MM y UTC). La API no cambia: sigue en ISO-8601.
//
// Al leer se aceptan también los formatos ISO, para que sigan sirviendo los
// enlaces y las vistas guardadas con filtros de antes de este cambio.

// Formatos de captura.
const (
	formatoFechaMX     = "02/01/2006"
	formatoFechaHoraMX = "02/01/2006 15:04"
)

// fechaDeCaptura convierte una fecha escrita como dd/mm/aaaa (también d/m/aaaa,
// o con guiones o puntos) a AAAA-MM-DD. Una fecha ya en AAAA-MM-DD se deja
// igual. Devuelve false si el texto no es una fecha existente.
func fechaDeCaptura(texto string) (string, bool) {
	texto = strings.TrimSpace(texto)
	if dia, err := time.Parse(time.DateOnly, texto); err == nil {
		return dia.Format(time.DateOnly), true
	}
	normalizado := strings.NewReplacer("-", "/", ".", "/").Replace(texto)
	dia, err := time.Parse("2/1/2006", normalizado)
	if err != nil {
		return "", false
	}
	return dia.Format(time.DateOnly), true
}

// fechaParaCaptura muestra una fecha guardada (AAAA-MM-DD) como dd/mm/aaaa.
// Lo que no es una fecha válida se devuelve tal cual: es lo que la persona
// escribió y el formulario se lo devuelve con su error.
func fechaParaCaptura(iso string) string {
	if dia, err := time.Parse(time.DateOnly, iso); err == nil {
		return dia.Format(formatoFechaMX)
	}
	return iso
}

// horaDeCaptura normaliza una hora de 24 horas: «9:30» → «09:30». Devuelve
// false si no es una hora válida.
func horaDeCaptura(texto string) (string, bool) {
	hora, err := time.Parse("15:04", strings.TrimSpace(texto))
	if err != nil {
		return "", false
	}
	return hora.Format("15:04"), true
}

// aLocal escribe un instante para un campo de fecha y hora: dd/mm/aaaa hh:mm,
// en hora de la Ciudad de México.
func aLocal(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(views.ZonaMexico()).Format(formatoFechaHoraMX)
}

// deLocal lee un campo de fecha y hora escrito en hora de la Ciudad de México:
// dd/mm/aaaa hh:mm (también d/m/aaaa h:mm), o el formato ISO de los controles
// datetime-local (AAAA-MM-DDThh:mm). Vacío o ilegible es el instante cero.
func deLocal(texto string) time.Time {
	texto = strings.Join(strings.Fields(texto), " ")
	for _, formato := range []string{"2/1/2006 15:04", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(formato, texto, views.ZonaMexico()); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
