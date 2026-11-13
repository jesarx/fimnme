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
	"strconv"

	"github.com/jesarx/fimnme/models"
)

// Respuestas JSON de las postulaciones.

// DefinicionJSON describe un formulario para el cliente: qué campos tiene,
// de qué tipo son y cuáles se exigen. Es la misma definición declarativa con
// la que el servidor valida, de modo que el cliente no necesita reglas
// propias que puedan desincronizarse. etiqueta traduce la ruta de un campo.
func DefinicionJSON(f models.DefinicionFormulario, etiqueta func(ruta string) string) map[string]any {
	campos := func(definiciones []models.DefinicionCampo) []map[string]any {
		lista := make([]map[string]any, 0, len(definiciones))
		for _, def := range definiciones {
			campo := map[string]any{
				"ruta": def.Ruta, "etiqueta": etiqueta(def.Ruta), "tipo": string(def.Tipo), "obligatorio": def.Obligatorio,
			}
			if def.Condicion != "" {
				campo["obligatorio_si"] = def.Condicion
			}
			if def.Longitud > 0 {
				campo["longitud_max"] = def.Longitud
			}
			if def.Maximo > 0 {
				campo["minimo"], campo["maximo"] = def.Minimo, def.Maximo
			}
			if len(def.Opciones) > 0 {
				campo["opciones"] = def.Opciones
			}
			lista = append(lista, campo)
		}
		return lista
	}
	return map[string]any{
		"campos": campos(f.Campos), "campos_de_obra": campos(f.CamposObra),
		"tiene_integrantes": f.TieneIntegrantes, "tiene_videos": f.TieneVideos,
	}
}

// valores devuelve el valor de cada campo de un formulario, o null si aún no
// se captura. Siempre aparecen todas las rutas: el cliente no adivina cuáles
// existen.
func valores(definiciones []models.DefinicionCampo, capturados map[string]string) map[string]any {
	resultado := make(map[string]any, len(definiciones))
	for _, def := range definiciones {
		if valor, ok := capturados[def.Ruta]; ok {
			resultado[def.Ruta] = valor
		} else {
			resultado[def.Ruta] = nil
		}
	}
	return resultado
}

// BloqueadorDeEnvio es un motivo, ya traducido, por el que una postulación
// aún no puede enviarse.
type BloqueadorDeEnvio struct {
	Seccion string `json:"seccion"`
	Campo   string `json:"campo"`
	ObraID  *int64 `json:"obra_id"` // null si es de la postulación
	Mensaje string `json:"mensaje"`
}

// resumenDeConvocatoria identifica la convocatoria de una postulación: lo
// que basta para listarla.
func resumenDeConvocatoria(c models.Convocatoria) map[string]any {
	return map[string]any{
		"id": c.ID, "tipo": string(c.Tipo), "titulo_es": c.TituloES, "titulo_en": c.TituloEN, "estado": string(c.Estado),
		"edicion": map[string]any{"numero": c.Edicion.Numero, "anio": c.Edicion.Anio, "nombre_es": c.Edicion.NombreES, "nombre_en": c.Edicion.NombreEN},
	}
}

// convocatoriaDePostulacion añade al resumen lo que el formulario necesita
// para guiar la captura: los límites del programa y el plazo.
func convocatoriaDePostulacion(c models.Convocatoria) map[string]any {
	resumen := resumenDeConvocatoria(c)
	resumen["max_obras"], resumen["duracion_max_programa_min"] = nuloSiCero(c.MaxObras), nuloSiCero(c.DuracionMaxProgramaMin)
	resumen["max_videos"] = c.MaxVideos
	if etapa, ok := c.Etapa(models.EtapaPostulacion); ok {
		resumen["postulacion_inicia_at"], resumen["postulacion_termina_at"] = marcaJSON(etapa.IniciaAt), marcaJSON(etapa.TerminaAt)
	}
	return resumen
}

// PostulacionJSON describe una postulación con todo lo capturado, la
// definición de su formulario y lo que le falta para poder enviarse.
func PostulacionJSON(p models.Postulacion, cfg models.ConfiguracionConvocatoria, editable bool,
	bloqueadores []BloqueadorDeEnvio, etiqueta func(ruta string) string) any {
	formulario := models.FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria)

	integrantes := make([]map[string]string, 0, len(p.Integrantes))
	for _, i := range p.Integrantes {
		integrantes = append(integrantes, map[string]string{"nombre": i.Nombre, "instrumento": i.Instrumento})
	}
	videos := make([]map[string]string, 0, len(p.Videos))
	for _, v := range p.Videos {
		video := map[string]string{"url": v.URL, "estado": string(v.Estado), "plataforma": v.Plataforma,
			"estado_texto": etiqueta("enlace.estado." + string(v.Estado))}
		if v.Detalle != "" {
			video["detalle"] = etiqueta("enlace.detalle." + v.Detalle)
		}
		videos = append(videos, video)
	}
	obras := make([]map[string]any, 0, len(p.Obras))
	for _, o := range p.Obras {
		obras = append(obras, map[string]any{"id": o.ID, "orden": o.Orden, "campos": valores(formulario.CamposObra, o.Campos)})
	}
	// Dotaciones que el postulante puede elegir para una obra.
	dotaciones := make([]map[string]any, 0, len(cfg.Dotaciones))
	for _, d := range cfg.Dotaciones {
		if d.Activa {
			dotaciones = append(dotaciones, map[string]any{"id": d.ID, "nombre_es": d.NombreES, "nombre_en": d.NombreEN,
				"descripcion_es": d.DescripcionES, "descripcion_en": d.DescripcionEN})
		}
	}

	// Documentos: qué se pide (para la postulación y para cada obra, según su
	// dotación) y qué se ha entregado.
	identificador := p.Folio
	if identificador == "" {
		identificador = "borrador-" + strconv.FormatInt(p.ID, 10)
	}
	ordenDeObra := make(map[int64]int)
	requisitos := requisitosJSON(cfg.Requisitos, 0, models.AmbitoPostulacion, nil)
	for _, o := range p.Obras {
		ordenDeObra[o.ID] = o.Orden
		dotacionID, _ := strconv.ParseInt(o.Campos["obra.dotacion_id"], 10, 64)
		obraID := o.ID
		requisitos = append(requisitos, requisitosJSON(cfg.Requisitos, dotacionID, models.AmbitoObra, &obraID)...)
	}
	documentos := make([]map[string]any, 0, len(p.Documentos))
	for _, d := range p.Documentos {
		var obraID any
		if d.ObraID != 0 {
			obraID = d.ObraID
		}
		documentos = append(documentos, map[string]any{
			"id": d.ID, "clave": d.Clave, "obra_id": obraID, "version": d.Version, "dictamen": d.Dictamen,
			"nombre": models.NombreDeDescarga(identificador, d, ordenDeObra[d.ObraID]),
			"mime":   d.Archivo.MIME, "tamano_bytes": d.Archivo.TamanoBytes, "sha256": d.Archivo.SHA256,
			"subido_at": marcaJSON(d.SubidoAt),
		})
	}
	acuses := make([]map[string]any, 0, len(p.Acuses))
	for _, ac := range p.Acuses {
		acuses = append(acuses, map[string]any{
			"version": ac.Version, "idioma": ac.Idioma, "generado_at": marcaJSON(ac.GeneradoAt),
			"tamano_bytes": ac.Archivo.TamanoBytes, "sha256": ac.Archivo.SHA256,
		})
	}

	if bloqueadores == nil {
		bloqueadores = []BloqueadorDeEnvio{}
	}
	pendientes := make(map[string]int)
	for _, b := range bloqueadores {
		pendientes[b.Seccion]++
	}
	secciones := make([]map[string]any, 0, 5)
	for _, seccion := range models.SeccionesDe(cfg.Convocatoria.Tipo) {
		secciones = append(secciones, map[string]any{
			"seccion": seccion, "etiqueta": etiqueta("seccion." + seccion), "pendientes": pendientes[seccion], "completa": pendientes[seccion] == 0,
		})
	}

	var subcategoria, folio any
	if p.Subcategoria != "" {
		subcategoria = string(p.Subcategoria)
	}
	if p.Folio != "" {
		folio = p.Folio
	}
	return map[string]any{
		"id": p.ID, "estado": string(p.Estado), "subcategoria": subcategoria, "folio": folio, "envios": p.Envios,
		"editable":  editable,
		"creado_at": marcaJSON(p.CreadoAt), "actualizado_at": marcaJSON(p.ActualizadoAt),
		"convocatoria": convocatoriaDePostulacion(cfg.Convocatoria),
		"campos":       valores(formulario.Campos, p.Campos),
		"integrantes":  integrantes, "videos": videos, "obras": obras,
		"requisitos": requisitos, "documentos": documentos, "acuses": acuses,
		"dotaciones": dotaciones,
		"definicion": DefinicionJSON(formulario, etiqueta),
		"secciones":  secciones, "bloqueadores": bloqueadores, "lista_para_enviar": len(bloqueadores) == 0,
	}
}

// requisitosJSON describe los documentos que se piden en un ámbito: a la
// postulación (obraID nil) o a una obra, según la dotación de ésta. Trae los
// dos idiomas de cada etiqueta, como el resto del contenido editorial.
func requisitosJSON(todos []models.RequisitoDocumental, dotacionID int64, ambito models.AmbitoRequisito, obraID *int64) []map[string]any {
	lista := []map[string]any{}
	for _, rq := range models.RequisitosAplicables(todos, dotacionID) {
		if rq.Ambito != ambito {
			continue
		}
		mimes := rq.MimesPermitidos
		if mimes == nil {
			mimes = []string{}
		}
		lista = append(lista, map[string]any{
			"clave": rq.Clave, "obra_id": obraID, "etiqueta_es": rq.EtiquetaES, "etiqueta_en": rq.EtiquetaEN,
			"ayuda_es": rq.AyudaES, "ayuda_en": rq.AyudaEN, "obligatorio": rq.Obligatorio,
			"mimes_permitidos": mimes, "tamano_max_bytes": rq.TamanoMaxBytes, "max_archivos": rq.MaxArchivos,
		})
	}
	return lista
}

// PostulacionesJSON es el historial de postulaciones de una cuenta.
func PostulacionesJSON(resumenes []models.ResumenPostulacion) any {
	lista := make([]map[string]any, 0, len(resumenes))
	for _, rp := range resumenes {
		var subcategoria, folio any
		if rp.Subcategoria != "" {
			subcategoria = string(rp.Subcategoria)
		}
		if rp.Folio != "" {
			folio = rp.Folio
		}
		lista = append(lista, map[string]any{
			"id": rp.ID, "estado": string(rp.Estado), "subcategoria": subcategoria, "folio": folio,
			"actualizado_at": marcaJSON(rp.ActualizadoAt), "convocatoria": resumenDeConvocatoria(rp.Convocatoria),
		})
	}
	return map[string]any{"postulaciones": lista}
}

// GuardadoJSON es la respuesta a un autoguardado: qué se guardó, qué se
// rechazó y a qué hora quedó guardado, para que la interfaz lo confirme
// («Guardado a las 14:32», RF-POST-03).
func GuardadoJSON(resultado models.ResultadoGuardado, errores []Bloqueador) any {
	guardados := resultado.Guardados
	if guardados == nil {
		guardados = []string{}
	}
	if errores == nil {
		errores = []Bloqueador{}
	}
	return map[string]any{"guardados": guardados, "errores": errores, "guardado_at": marcaJSON(resultado.GuardadoAt)}
}

// CatalogosJSON entrega los catálogos fijos del formulario. De los países
// sólo van los códigos ISO 3166-1 alfa-2; sus nombres son texto de interfaz.
func CatalogosJSON() any {
	entidades := make([]map[string]string, 0, 32)
	for _, e := range models.EntidadesFederativas() {
		entidades = append(entidades, map[string]string{"clave": e.Clave, "nombre": e.Nombre})
	}
	return map[string]any{
		"paises": models.CodigosDePais(), "pais_con_catalogo_de_entidades": models.PaisMexico,
		"entidades": entidades, "entidad_sin_sede": models.EntidadCDMX,
		"generos": models.OpcionesGenero, "estrenos": models.OpcionesEstreno,
		"subcategorias": []string{string(models.SubcategoriaSolista), string(models.SubcategoriaEnsamble)},
	}
}
