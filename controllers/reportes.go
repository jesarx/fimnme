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
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Generador de reportes CSV (§5.7), plantillas de reporte y tableros (§5.13).
// Todo es de la supervisión: administrador y observador (§2.2).

// idiomaDelReporte es el idioma de los encabezados y de los valores: el
// parámetro «idioma», o el de quien lo pide.
func idiomaDelReporte(r *http.Request) string {
	if idioma := r.URL.Query().Get("idioma"); idioma == models.IdiomaES || idioma == models.IdiomaEN {
		return idioma
	}
	return idiomaDe(r)
}

// GET /api/v1/convocatorias/{id}/reporte/campos   — catálogo de campos
//
// Los campos que puede llevar un reporte de esa convocatoria, con su
// etiqueta y su nivel (postulación, obra o evaluación).
func (a *App) apiCamposDeReporte(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, a.vistas.CatalogoDeReporteJSON(idiomaDelReporte(r), cfg, models.JuradosDe(cfg)))
}

// decimalOpcional lee un parámetro numérico opcional. Devuelve nil si viene
// vacío, y false si no es un número.
func decimalOpcional(texto string) (*float64, bool) {
	if texto == "" {
		return nil, true
	}
	valor, err := strconv.ParseFloat(texto, 64)
	return &valor, err == nil
}

// GET /api/v1/convocatorias/{id}/reporte   — genera un reporte CSV
//
// Parámetros:
//
//	campos        claves del catálogo, separadas por coma, en el orden de las columnas
//	granularidad  «postulacion» (por omisión) u «obra»
//	separador     «coma» (por omisión) o «punto_y_coma»
//	puntaje_min, puntaje_max   rango de puntaje
//	orden         los del archivo, o «puntaje» / «-puntaje»
//	plantilla     id de una plantilla guardada: aporta los parámetros que falten
//	idioma        es / en, para encabezados y valores
//
// y cualquiera de los filtros del listado del archivo.
func (a *App) apiReporte(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	sesion := sesionDe(r)
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	var errores []models.ErrorCampo
	falla := func(campo, clave string) { errores = append(errores, models.ErrorCampo{Campo: campo, Clave: clave}) }

	// 1. Los parámetros: los de la plantilla, si se pide una, y encima los de
	//    la petición.
	consulta := r.URL.Query()
	if texto := consulta.Get("plantilla"); texto != "" {
		plantillaID, _ := strconv.ParseInt(texto, 10, 64)
		plantilla, err := a.datos.PlantillaDeReportePorID(r.Context(), plantillaID)
		if a.falloDatos(w, r, err) {
			return
		}
		guardados, err := url.ParseQuery(plantilla.Consulta)
		if err != nil || plantilla.Tipo != cfg.Convocatoria.Tipo {
			a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "plantilla", Clave: "error.plantilla_de_otro_tipo"}}})
			return
		}
		for nombre, valores := range consulta {
			guardados[nombre] = valores
		}
		consulta = guardados
	}
	def := models.DefinicionDeReporte{Granularidad: consulta.Get("granularidad"), Separador: consulta.Get("separador"),
		Campos: strings.FieldsFunc(consulta.Get("campos"), func(r rune) bool { return r == ',' })}
	var valido bool
	if def.PuntajeMin, valido = decimalOpcional(consulta.Get("puntaje_min")); !valido {
		falla("puntaje_min", "error.filtro_invalido")
	}
	if def.PuntajeMax, valido = decimalOpcional(consulta.Get("puntaje_max")); !valido {
		falla("puntaje_max", "error.filtro_invalido")
	}
	// El orden por puntaje es propio de los reportes: el listado del archivo
	// no lo conoce, porque el puntaje no es para todos los que lo consultan.
	descendente := false
	if orden := consulta.Get("orden"); orden == "puntaje" || orden == "-puntaje" {
		def.PorPuntaje, descendente = true, orden == "-puntaje"
		consulta.Del("orden")
	}
	filtro, erroresDeFiltro := filtroDeLaConsulta(consulta)
	filtro.ConvocatoriaID, filtro.Pagina, filtro.Tamano = convocatoriaID, 0, 0
	def.Filtro = filtro
	campos, erroresDeReporte := models.ValidarReporte(&def, cfg)
	if errores = append(append(errores, erroresDeFiltro...), erroresDeReporte...); len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return
	}

	// 2. Qué hace falta leer, según los campos pedidos.
	conEvaluacion := def.PorPuntaje || def.PuntajeMin != nil || def.PuntajeMax != nil
	conObservaciones := false
	for _, campo := range campos {
		conEvaluacion = conEvaluacion || campo.Nivel == models.NivelEvaluacion
		conObservaciones = conObservaciones || campo.Clave == "revision.observaciones"
	}
	type unidad struct{ postulacionID, obraID int64 }
	resultados := make(map[unidad]*models.FilaDeResultado)
	lugares := make(map[unidad]int)
	if conEvaluacion {
		matriz, err := matrizDeSupervision(r.Context(), a.datos, sesion, a.actorDe(r), convocatoriaID)
		if a.falloDatos(w, r, err) {
			return
		}
		lugar := models.LugaresDeLaMatriz(matriz.Filas)
		for i := range matriz.Filas {
			u := unidad{matriz.Filas[i].Unidad.PostulacionID, matriz.Filas[i].Unidad.ObraID}
			resultados[u], lugares[u] = &matriz.Filas[i], lugar[i]
		}
	}

	// 3. Los renglones: las postulaciones del filtro, una fila cada una o una
	//    por obra.
	listado, _, err := a.datos.ListarPostulaciones(r.Context(), models.AlcanceDe(sesion), filtro)
	if a.falloDatos(w, r, err) {
		return
	}
	jurados, cierre := models.JuradosDe(cfg), models.CierreDePostulacion(cfg.Convocatoria)
	var renglones []models.FilaDeReporte
	for _, renglon := range listado {
		p, err := a.datos.PostulacionPorID(r.Context(), renglon.ID)
		if a.falloDatos(w, r, err) {
			return
		}
		titular, err := a.datos.UsuarioPorID(r.Context(), p.UsuarioID)
		if a.falloDatos(w, r, err) {
			return
		}
		base := models.FilaDeReporte{Postulacion: p, Titular: titular.Usuario, Jurados: jurados, Dotaciones: cfg.Dotaciones, Cierre: cierre}
		if conObservaciones {
			revision, err := a.datos.RevisionDe(r.Context(), p.ID)
			if a.falloDatos(w, r, err) {
				return
			}
			base.Observaciones = revision.Observaciones
		}
		// Una fila por postulación, o una por obra. Una postulación sin
		// obras (un borrador) conserva su fila, con los campos de obra vacíos.
		obras := []*models.Obra{nil}
		if def.Granularidad == models.GranularidadObra && len(p.Obras) > 0 {
			obras = obras[:0]
			for i := range p.Obras {
				obras = append(obras, &p.Obras[i])
			}
		}
		for _, obra := range obras {
			fila := base
			fila.Obra = obra
			if conEvaluacion {
				u := unidad{postulacionID: p.ID}
				if obra != nil && cfg.Convocatoria.Tipo == models.TipoCompositores {
					u.obraID = obra.ID
				}
				fila.Resultado, fila.Lugar = resultados[u], lugares[u]
				if (def.PuntajeMin != nil || def.PuntajeMax != nil) && !enRangoDePuntaje(fila.Resultado, def.PuntajeMin, def.PuntajeMax) {
					continue
				}
			}
			renglones = append(renglones, fila)
		}
	}
	if def.PorPuntaje {
		// Lo que aún no tiene puntaje va al final, en cualquier sentido.
		slices.SortStableFunc(renglones, func(a, b models.FilaDeReporte) int {
			puntajeA, tieneA := puntajeDe(a.Resultado)
			puntajeB, tieneB := puntajeDe(b.Resultado)
			switch {
			case tieneA != tieneB:
				if tieneA {
					return -1
				}
				return 1
			case puntajeA == puntajeB:
				return 0
			case (puntajeA < puntajeB) != descendente:
				return -1
			}
			return 1
		})
	}

	// 4. El archivo. Generar un reporte queda en la bitácora.
	idioma := idiomaDelReporte(r)
	encabezados, filas := a.vistas.ReporteCSV(idioma, campos, renglones, jurados)
	if a.falloDatos(w, r, a.datos.RegistrarReporte(r.Context(), a.actorDe(r), convocatoriaID, def.Campos, len(filas))) {
		return
	}
	nombre := fmt.Sprintf("fimnme_%d_%s_reporte.csv", cfg.Convocatoria.Edicion.Numero, cfg.Convocatoria.Tipo)
	if err := views.CSVConSeparador(w, nombre, def.Separador, encabezados, filas); err != nil {
		a.log.Error("no se pudo escribir el reporte", "error", err)
	}
}

// puntajeDe devuelve el puntaje comparable de una unidad, y si lo tiene: una
// unidad que no llegó a evaluación o que nadie ha calificado no lo tiene.
func puntajeDe(resultado *models.FilaDeResultado) (float64, bool) {
	if resultado == nil || resultado.JuradosQueEvaluaron == 0 {
		return 0, false
	}
	return resultado.Comparable, true
}

// enRangoDePuntaje indica si una unidad tiene puntaje y cae en el rango.
func enRangoDePuntaje(resultado *models.FilaDeResultado, minimo, maximo *float64) bool {
	puntaje, tiene := puntajeDe(resultado)
	return tiene && (minimo == nil || puntaje >= *minimo) && (maximo == nil || puntaje <= *maximo)
}

// GET /api/v1/reportes/plantillas   — plantillas de reporte guardadas
func (a *App) apiPlantillasDeReporte(w http.ResponseWriter, r *http.Request) {
	plantillas, err := a.datos.PlantillasDeReporte(r.Context())
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.PlantillasDeReporteJSON(plantillas))
}

// POST /api/v1/reportes/plantillas   — guarda la definición de un reporte
//
// Cuerpo: {"nombre", "tipo", "consulta"}. La consulta son los parámetros del
// reporte («campos=folio,estado&granularidad=obra&estado=validada»), sin la
// convocatoria: la plantilla sirve para cualquier convocatoria de su tipo,
// este año y los siguientes (RF-REP-04).
func (a *App) apiGuardarPlantillaDeReporte(w http.ResponseWriter, r *http.Request) {
	var entrada struct {
		Nombre   string `json:"nombre"`
		Tipo     string `json:"tipo"`
		Consulta string `json:"consulta"`
	}
	if !a.leerJSON(w, r, &entrada) {
		return
	}
	plantilla := models.PlantillaDeReporte{Nombre: entrada.Nombre, Tipo: models.TipoConvocatoria(entrada.Tipo), Consulta: strings.TrimPrefix(entrada.Consulta, "?")}
	if _, err := a.datos.GuardarPlantillaDeReporte(r.Context(), a.actorDe(r), plantilla); a.falloDatos(w, r, err) {
		return
	}
	a.apiPlantillasDeReporte(w, r)
}

// DELETE /api/v1/reportes/plantillas/{id}   — borra una plantilla de reporte
//
// El administrador borra cualquiera; el observador, las que él guardó.
func (a *App) apiBorrarPlantillaDeReporte(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	sesion := sesionDe(r)
	soloDe := sesion.Usuario.ID
	if models.PuedeAdministrar(sesion) {
		soloDe = 0
	}
	if a.falloDatos(w, r, a.datos.BorrarPlantillaDeReporte(r.Context(), a.actorDe(r), id, soloDe)) {
		return
	}
	a.apiPlantillasDeReporte(w, r)
}

// GET /api/v1/convocatorias/{id}/tablero   — tablero de la convocatoria
//
// Postulaciones por estado, ritmo de envíos por día, documentos por
// dictaminar, subsanaciones abiertas y por vencer, y el avance de cada jurado
// —cuántas lleva de cuántas—, sin ningún puntaje (RF-DASH-01 y 02). Cada
// cifra trae el filtro del archivo que la produce (RF-DASH-05).
func (a *App) apiTablero(w http.ResponseWriter, r *http.Request) {
	convocatoriaID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	// El avance de los jurados va primero: lee la configuración de la
	// convocatoria y, si ésta no existe, responde 404.
	avance, err := avanceDeJurados(r.Context(), a.datos, sesionDe(r), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	tablero, err := a.datos.TableroDeConvocatoria(r.Context(), convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.TableroJSON(convocatoriaID, tablero, avance))
}

// GET /api/v1/ediciones/{id}/estadisticas   — estadísticas de una edición
//
// Distribución de las postulaciones enviadas por entidad, país, nacionalidad,
// género, rango de edad, dotación y tipo de estreno, y duración media de las
// obras (RF-DASH-03). Con «convocatoria», sólo las de esa.
func (a *App) apiEstadisticas(w http.ResponseWriter, r *http.Request) {
	edicionID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	convocatoriaID, err := enteroOpcional(r.URL.Query().Get("convocatoria"))
	if err != nil {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "convocatoria", Clave: "error.filtro_invalido"}}})
		return
	}
	edicion, err := a.datos.EdicionPorID(r.Context(), edicionID)
	if a.falloDatos(w, r, err) {
		return
	}
	estadisticas, err := a.datos.EstadisticasDeEdicion(r.Context(), edicionID, convocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusOK, views.EstadisticasJSON(edicion.Numero, convocatoriaID, estadisticas))
}
