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
	"github.com/jesarx/fimnme/models"
)

// Respuestas JSON de la configuración de ediciones y convocatorias. Sólo las
// reciben administradores y observadores.

// nuloSiCero convierte el 0 del dominio («sin límite») en null.
func nuloSiCero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// faltantes devuelve los nombres de los campos en inglés que están vacíos. El
// público verá en su lugar el texto en español; aquí se le advierte al
// administrador (RF-I18N-04).
func faltantes(campos map[string]string) []string {
	faltan := []string{}
	for _, nombre := range []string{"nombre_en", "titulo_en", "bases_md_en"} {
		if valor, ok := campos[nombre]; ok && valor == "" {
			faltan = append(faltan, nombre)
		}
	}
	return faltan
}

func etapasJSON(etapas []models.Etapa) []map[string]any {
	lista := make([]map[string]any, 0, len(etapas))
	for _, e := range etapas {
		lista = append(lista, map[string]any{
			"clave": string(e.Clave), "inicia_at": marcaJSON(e.IniciaAt), "termina_at": marcaJSON(e.TerminaAt),
		})
	}
	return lista
}

// resumenConvocatoria es una convocatoria sin sus textos largos, para listados.
func resumenConvocatoria(c models.Convocatoria) map[string]any {
	return map[string]any{
		"id": c.ID, "tipo": string(c.Tipo), "titulo_es": c.TituloES, "titulo_en": c.TituloEN,
		"estado": string(c.Estado), "etapas": etapasJSON(c.Etapas),
	}
}

// EdicionJSON describe una edición con el resumen de sus convocatorias.
func EdicionJSON(e models.Edicion, convocatorias []models.Convocatoria) map[string]any {
	lista := make([]map[string]any, 0, len(convocatorias))
	for _, c := range convocatorias {
		lista = append(lista, resumenConvocatoria(c))
	}
	return map[string]any{
		"id": e.ID, "numero": e.Numero, "anio": e.Anio, "nombre_es": e.NombreES, "nombre_en": e.NombreEN,
		"faltan_en_ingles": faltantes(map[string]string{"nombre_en": e.NombreEN}),
		"convocatorias":    lista,
	}
}

// EdicionesJSON es el listado de ediciones.
func EdicionesJSON(ediciones []map[string]any) any {
	if ediciones == nil {
		ediciones = []map[string]any{}
	}
	return map[string]any{"ediciones": ediciones}
}

// Bloqueador es un motivo, ya traducido, por el que una convocatoria no puede
// pasar a su siguiente etapa.
type Bloqueador struct {
	Campo   string `json:"campo"`
	Mensaje string `json:"mensaje"`
}

// UsuarioJSON describe una cuenta para el administrador.
func UsuarioJSON(u models.Usuario, activada bool) map[string]any {
	return map[string]any{
		"id": u.ID, "email": u.Email, "nombre": u.Nombre, "apellidos": u.Apellidos, "idioma": u.Idioma,
		"activo": u.Activo, "activada": activada, "email_verificado": u.EmailVerificado(),
	}
}

// CuentaJSON describe una cuenta con sus perfiles globales.
func CuentaJSON(c models.CuentaAdmin) map[string]any {
	cuenta := UsuarioJSON(c.Usuario, c.Activada)
	perfiles := make([]string, 0, len(c.Perfiles))
	for _, p := range c.Perfiles {
		perfiles = append(perfiles, string(p))
	}
	cuenta["perfiles"] = perfiles
	return cuenta
}

// CuentasJSON es una página del listado de cuentas.
func CuentasJSON(cuentas []models.CuentaAdmin, total, pagina, tamano int) any {
	lista := make([]map[string]any, 0, len(cuentas))
	for _, c := range cuentas {
		lista = append(lista, CuentaJSON(c))
	}
	return map[string]any{"total": total, "pagina": pagina, "tamano": tamano, "usuarios": lista}
}

// ConfiguracionJSON describe una convocatoria con toda su configuración:
// textos, límites, calendario, dotaciones, requisitos documentales, criterios
// y asignaciones. «cuotas» resume cuántos jurados y revisores activos hay, y
// «siguiente», qué falta para pasar a la etapa que sigue.
func ConfiguracionJSON(cfg models.ConfiguracionConvocatoria, archivos map[models.ClaseArchivoConvocatoria]models.Archivo,
	siguiente models.EstadoConvocatoria, bloqueadores []Bloqueador) any {
	c := cfg.Convocatoria

	dotaciones := make([]map[string]any, 0, len(cfg.Dotaciones))
	for _, d := range cfg.Dotaciones {
		dotaciones = append(dotaciones, map[string]any{
			"id": d.ID, "nombre_es": d.NombreES, "nombre_en": d.NombreEN,
			"descripcion_es": d.DescripcionES, "descripcion_en": d.DescripcionEN, "orden": d.Orden, "activa": d.Activa,
		})
	}
	requisitos := make([]map[string]any, 0, len(cfg.Requisitos))
	for _, rq := range cfg.Requisitos {
		mimes := rq.MimesPermitidos
		if mimes == nil {
			mimes = []string{}
		}
		requisitos = append(requisitos, map[string]any{
			"id": rq.ID, "dotacion_id": rq.DotacionID, "clave": rq.Clave,
			"etiqueta_es": rq.EtiquetaES, "etiqueta_en": rq.EtiquetaEN, "ayuda_es": rq.AyudaES, "ayuda_en": rq.AyudaEN,
			"ambito": string(rq.Ambito), "obligatorio": rq.Obligatorio, "mimes_permitidos": mimes,
			"tamano_max_bytes": rq.TamanoMaxBytes, "max_archivos": rq.MaxArchivos, "orden": rq.Orden,
		})
	}
	criterios := make([]map[string]any, 0, len(cfg.Criterios))
	for _, cr := range cfg.Criterios {
		criterios = append(criterios, map[string]any{
			"id": cr.ID, "clave": cr.Clave, "etiqueta_es": cr.EtiquetaES, "etiqueta_en": cr.EtiquetaEN,
			"descripcion_es": cr.DescripcionES, "descripcion_en": cr.DescripcionEN,
			"escala_min": cr.EscalaMin, "escala_max": cr.EscalaMax, "obligatorio": cr.Obligatorio, "orden": cr.Orden,
		})
	}
	asignaciones := make([]map[string]any, 0, len(cfg.Asignaciones))
	for _, a := range cfg.Asignaciones {
		asignaciones = append(asignaciones, map[string]any{
			"id": a.ID, "perfil": string(a.Perfil), "activo": a.Activo, "usuario": UsuarioJSON(a.Usuario, a.CuentaActivada),
		})
	}

	var transicion any
	if siguiente != "" {
		if bloqueadores == nil {
			bloqueadores = []Bloqueador{}
		}
		transicion = map[string]any{"estado": string(siguiente), "bloqueadores": bloqueadores}
	}
	return map[string]any{
		"id": c.ID, "tipo": string(c.Tipo), "estado": string(c.Estado),
		"edicion": map[string]any{
			"id": c.Edicion.ID, "numero": c.Edicion.Numero, "anio": c.Edicion.Anio,
			"nombre_es": c.Edicion.NombreES, "nombre_en": c.Edicion.NombreEN,
		},
		"titulo_es": c.TituloES, "titulo_en": c.TituloEN, "bases_md_es": c.BasesMDES, "bases_md_en": c.BasesMDEN,
		"faltan_en_ingles": faltantes(map[string]string{"titulo_en": c.TituloEN, "bases_md_en": c.BasesMDEN}),
		"max_obras":        nuloSiCero(c.MaxObras), "duracion_max_programa_min": nuloSiCero(c.DuracionMaxProgramaMin),
		"max_videos": c.MaxVideos,
		"etapas":     etapasJSON(c.Etapas),
		"dotaciones": dotaciones, "requisitos": requisitos, "criterios": criterios, "asignaciones": asignaciones,
		"cuotas": map[string]any{
			"jurados": cfg.Activos(models.PerfilJurado), "jurados_requeridos": models.CuotaJurados,
			"revisores": cfg.Activos(models.PerfilRevisorDocumental),
		},
		// Informativo: postulaciones enviadas que la revisión aún no valida
		// ni descarta. No impiden pasar a evaluación.
		"postulaciones_sin_dictaminar": cfg.SinDictaminar,
		"archivos":                     archivosDeConvocatoriaJSON(archivos),
		"siguiente":                    transicion,
	}
}

// archivosDeConvocatoriaJSON describe los archivos públicos cargados: las
// bases en PDF de cada idioma y la imagen de cabecera. Lo que no está cargado
// va como null.
func archivosDeConvocatoriaJSON(archivos map[models.ClaseArchivoConvocatoria]models.Archivo) map[string]any {
	resumen := make(map[string]any)
	for _, clase := range []models.ClaseArchivoConvocatoria{models.ArchivoBasesES, models.ArchivoBasesEN, models.ArchivoImagen} {
		if a, cargado := archivos[clase]; cargado {
			resumen[string(clase)] = map[string]any{"mime": a.MIME, "tamano_bytes": a.TamanoBytes, "sha256": a.SHA256}
		} else {
			resumen[string(clase)] = nil
		}
	}
	return resumen
}

// ErrorDeEnvioJSON es el cuerpo de la respuesta 422 de un envío al que le
// falta algo: el error general y la lista de bloqueadores, con la misma forma
// que en la consulta de la postulación.
func ErrorDeEnvioJSON(mensaje string, bloqueadores []BloqueadorDeEnvio) any {
	return map[string]any{"error": map[string]any{"codigo": "incompleta", "mensaje": mensaje, "bloqueadores": bloqueadores}}
}

// ErrorValidacionJSON es el cuerpo de una respuesta 422: el error general y,
// campo por campo, qué hay que corregir.
func ErrorValidacionJSON(mensaje string, campos []Bloqueador) any {
	return map[string]any{"error": map[string]any{"codigo": "validacion", "mensaje": mensaje, "campos": campos}}
}
