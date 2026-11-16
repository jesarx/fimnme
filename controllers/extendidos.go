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
	"net/http"

	"github.com/jesarx/fimnme/views"
)

// Campos extendidos (§5.14.1): consulta del diccionario y captura de valores
// por la administración.
//
// Añadir o quitar un campo es DDL y NO se hace desde aquí: sólo desde
// «fimnme-cli schema», por quien tiene acceso al servidor. El servicio web
// nunca altera el esquema. Los campos que captura el postulante entran por el
// autoguardado de su postulación, como cualquier otro campo del formulario.

// GET /api/v1/extendidos   — diccionario de campos extendidos
func (a *App) apiExtendidos(w http.ResponseWriter, r *http.Request) {
	campos, err := a.datos.CamposExtendidos(r.Context())
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.DiccionarioExtendidoJSON(campos))
}

// GET /api/v1/extendidos/{tabla}/{id}   — campos extendidos de una fila de un nodo
//
// El id es el de la fila: una edición, una convocatoria, una dotación, una
// obra o una postulación (también para sus tablas de datos generales y de
// propuesta de concierto, que cuelgan de ella).
func (a *App) apiValoresExtendidos(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	campos, valores, err := a.datos.ValoresExtendidos(r.Context(), r.PathValue("tabla"), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.ValoresExtendidosJSON(campos, valores))
}

// PUT /api/v1/extendidos/{tabla}/{id}   — captura campos extendidos
//
// Cuerpo: {"campos": {"x_sede_alterna": "Sala Ponce", …}}, por nombre de
// columna. Sirve para los nodos de configuración y para los campos INTERNOS
// de una postulación o de una obra; los que captura el postulante no se
// pueden escribir por aquí.
func (a *App) apiGuardarExtendidos(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	campos, ok := a.camposDeEntrada(w, r)
	if !ok {
		return
	}
	resultado, err := a.datos.GuardarExtendidos(r.Context(), a.actorDe(r), r.PathValue("tabla"), id, campos)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderGuardado(w, r, resultado)
}
