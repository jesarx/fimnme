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
	"testing"
	"time"
)

// Las fechas de los formularios se escriben dd/mm/aaaa y hh:mm de 24 horas;
// los formatos ISO de antes se siguen entendiendo; lo que no existe se
// rechaza.
func TestFechasDeCaptura(t *testing.T) {
	for texto, esperada := range map[string]string{
		"29/01/2027": "2027-01-29", "5/3/2027": "2027-03-05", "05-03-2027": "2027-03-05",
		"05.03.2027": "2027-03-05", " 29/01/2027 ": "2027-01-29", "2027-01-29": "2027-01-29",
		"31/02/2027": "", "01/13/2027": "", "12/31/2027": "", "29/01/27": "", "hoy": "", "": "",
	} {
		iso, ok := fechaDeCaptura(texto)
		if iso != esperada || ok != (esperada != "") {
			t.Errorf("fechaDeCaptura(%q) = %q, %v; se esperaba %q", texto, iso, ok, esperada)
		}
	}
	if fechaParaCaptura("2027-01-29") != "29/01/2027" || fechaParaCaptura("31/02/2027") != "31/02/2027" || fechaParaCaptura("") != "" {
		t.Error("fechaParaCaptura debe mostrar dd/mm/aaaa y dejar intacto lo que no es fecha")
	}
	for texto, esperada := range map[string]string{"9:30": "09:30", "19:30": "19:30", "24:00": "", "7 pm": "", "": ""} {
		if hora, ok := horaDeCaptura(texto); hora != esperada || ok != (esperada != "") {
			t.Errorf("horaDeCaptura(%q) = %q, %v", texto, hora, ok)
		}
	}
	// Hora de la Ciudad de México (UTC−6) ida y vuelta.
	instante := time.Date(2027, 1, 30, 5, 59, 0, 0, time.UTC)
	if aLocal(instante) != "29/01/2027 23:59" {
		t.Errorf("aLocal: %q", aLocal(instante))
	}
	for _, texto := range []string{"29/01/2027 23:59", "29/1/2027  23:59", "2027-01-29T23:59"} {
		if !deLocal(texto).Equal(instante) {
			t.Errorf("deLocal(%q) = %v", texto, deLocal(texto))
		}
	}
	for _, texto := range []string{"31/02/2027 10:00", "29/01/2027", "", "mañana"} {
		if !deLocal(texto).IsZero() {
			t.Errorf("deLocal(%q) debió ser cero", texto)
		}
	}
	// Los filtros del archivo y de la bitácora aceptan los dos formatos.
	desde, ok1 := fechaDeFiltro("29/01/2027", false)
	iso, ok2 := fechaDeFiltro("2027-01-29", false)
	if !ok1 || !ok2 || !desde.Equal(iso) {
		t.Errorf("fechaDeFiltro: %v %v %v %v", desde, ok1, iso, ok2)
	}
	if _, ok := fechaDeFiltro("31/02/2027", false); ok {
		t.Error("fechaDeFiltro aceptó una fecha que no existe")
	}
}
