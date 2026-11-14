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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jesarx/fimnme/internal/pdf"
	"github.com/jesarx/fimnme/models"
)

// Expediente en PDF (§5.12.4).
//
// A diferencia del acuse, el expediente es una herramienta de consulta
// interna: muestra la postulación como está HOY, lleva la fotografía y, según
// quién lo pida, la revisión documental y las evaluaciones.
//
// La vista no decide qué se muestra. Recibe un models.Expediente del que ya
// se quitó lo que quien lo pide no puede ver (models.RecortarExpediente) y lo
// pinta entero: lo que no está, no se imprime.

// OpcionesDeExpediente son las opciones con que se genera (RF-ARCH-43, 44, 46
// y 47).
type OpcionesDeExpediente struct {
	Idioma string
	// Solicitante es el nombre de quien lo pide. Va en el pie de cada página,
	// junto con la fecha: un PDF extraviado dice de quién era.
	Solicitante        string
	Generado           time.Time
	SinFotografia      bool
	HojaDeCalificacion bool // hoja en blanco al final de cada postulación, para calificar en papel
}

// ExpedientePDF compone un expediente de una o de muchas postulaciones. Se
// usa llamando a Agregar por cada postulación y, al final, a Bytes.
//
// Las postulaciones se agregan de una en una para que quien lo arma no tenga
// que cargar en memoria las fotografías de todas: cada una se reduce al
// agregarla y sólo se conserva la versión pequeña.
type ExpedientePDF struct {
	m        *Motor
	o        OpcionesDeExpediente
	d        *pdf.Documento
	entradas []entradaDeIndice
}

// entradaDeIndice es un renglón del índice: una postulación y la página en
// que empieza, contando desde 1 y sin las páginas del propio índice.
type entradaDeIndice struct {
	titulo string
	pagina int
}

// NuevoExpediente empieza un expediente vacío.
func (m *Motor) NuevoExpediente(o OpcionesDeExpediente) *ExpedientePDF {
	if o.Idioma != models.IdiomaEN {
		o.Idioma = models.IdiomaES
	}
	if utf8.RuneCountInString(o.Solicitante) > 40 {
		o.Solicitante = string([]rune(o.Solicitante)[:40]) + "..."
	}
	e := &ExpedientePDF{m: m, o: o}
	e.d = pdf.Nuevo(e.t("expediente.titulo"), e.pie(e.t("expediente.titulo")), e.t("acuse.pagina"), o.Generado)
	return e
}

// t traduce una clave al idioma del expediente.
func (e *ExpedientePDF) t(clave string, args ...any) string {
	return e.m.textos.T(e.o.Idioma, clave, args...)
}

// bilingue elige entre dos textos según el idioma del expediente; si falta el
// inglés usa el español.
func (e *ExpedientePDF) bilingue(es, en string) string {
	if e.o.Idioma == models.IdiomaEN && en != "" {
		return en
	}
	return es
}

// pie compone el pie de página (RF-ARCH-47): a qué se refiere la página, la
// leyenda de confidencialidad, cuándo se generó (hora de la Ciudad de México)
// y quién lo pidió. Es breve a propósito: comparte renglón con el número de
// página.
func (e *ExpedientePDF) pie(referencia string) string {
	return e.t("expediente.pie", referencia, e.o.Generado.In(zonaMexico).Format("2006-01-02 15:04"), e.o.Solicitante)
}

// fecha escribe un instante en la hora de la Ciudad de México, o una raya si
// no hay.
func (e *ExpedientePDF) fecha(momento time.Time) string {
	if momento.IsZero() {
		return "—"
	}
	return e.m.textos.FechaHora(e.o.Idioma, momento)
}

// Postulaciones devuelve cuántas postulaciones lleva el expediente.
func (e *ExpedientePDF) Postulaciones() int {
	return len(e.entradas)
}

// Agregar añade una postulación al expediente, en página nueva. fotografia es
// el archivo original de la fotografía (JPEG o PNG), o nil si no tiene o no
// se quiere.
func (e *ExpedientePDF) Agregar(x models.Expediente, fotografia []byte) {
	d, p, cfg := e.d, x.Postulacion, x.Configuracion
	folio := p.Folio
	if folio == "" {
		folio = e.t("expediente.borrador", p.ID)
	}
	nombre := p.Campos["datos.nombre_completo"]
	if nombre == "" {
		nombre = p.Campos["datos.nombre_ensamble"]
	}
	if len(e.entradas) > 0 {
		d.SaltoDePagina()
	}
	d.FijarPie(e.pie(folio))
	titulo := strings.TrimSuffix(folio+" · "+nombre, " · ")
	d.Marcador(titulo)
	e.entradas = append(e.entradas, entradaDeIndice{titulo: titulo, pagina: d.Paginas()})
	d.Encabezado(e.t("sitio.nombre"), e.t("expediente.titulo"), folio)

	if !e.o.SinFotografia && len(fotografia) > 0 {
		if reducida, ancho, alto, err := ReducirFotografia(fotografia); err == nil {
			d.Imagen(reducida, ancho, alto, 150, 150)
		} else {
			// Una fotografía ilegible no impide el expediente: se dice y se sigue.
			d.Parrafo(e.t("expediente.sin_fotografia"), pdf.TintaSuave)
		}
	}

	// 1. La postulación y su estado.
	d.Seccion(e.t("expediente.postulacion"))
	d.Campo(e.t("acuse.folio"), p.Folio)
	d.Campo(e.t("acuse.edicion"), cfg.Convocatoria.Edicion.Nombre(e.o.Idioma))
	convocatoria := cfg.Convocatoria.Titulo(e.o.Idioma)
	if p.Subcategoria != "" {
		convocatoria += " · " + e.t("subcategoria."+string(p.Subcategoria))
	}
	d.Campo(e.t("acuse.convocatoria"), convocatoria)
	d.Campo(e.t("expediente.estado"), e.t("estado.postulacion."+string(p.Estado)))
	if !p.EnviadaAt.IsZero() {
		d.Campo(e.t("acuse.enviada"), e.fecha(p.EnviadaAt))
		d.Campo(e.t("expediente.envios"), strconv.Itoa(p.Envios))
	}
	d.Campo(e.t("acuse.cuenta"), strings.TrimSpace(x.Titular.Nombre+" "+x.Titular.Apellidos)+" · "+x.Titular.Email)
	if p.DictamenFinal != "" {
		d.Campo(e.t("expediente.resultado"), e.t("resultado."+p.DictamenFinal))
	}

	// 2. El contenido, igual que en el acuse pero con los datos de hoy. Si
	//    quien lo pide ve la revisión, cada documento lleva su dictamen.
	snapshot := models.SnapshotDeEnvio(p, cfg, x.Titular, p.Folio, p.Envios, p.EnviadaAt)
	snapshot.Idioma = e.o.Idioma
	motivos := make(map[int64]string)
	for _, dictamen := range x.Revision.Dictamenes {
		motivos[dictamen.DocumentoID] = dictamen.Motivo
	}
	e.m.escribirPostulacion(d, snapshot, func(documento models.SnapshotDocumento) string {
		for _, original := range p.Documentos {
			if original.Clave != documento.Clave || original.ObraID != documento.ObraID || original.Version != documento.Version || original.Dictamen == "" {
				continue
			}
			nota := e.t("expediente.dictamen." + original.Dictamen)
			if motivo := motivos[original.ID]; motivo != "" {
				nota += ": " + motivo
			}
			return nota
		}
		return ""
	})

	if x.Recorte.Revision {
		e.escribirRevision(x)
	}
	if x.Recorte.EvaluacionPropia {
		e.escribirEvaluacionPropia(x)
	}
	if x.Recorte.Supervision && len(x.Resultados) > 0 {
		e.escribirResultados(x)
	}
	if e.o.HojaDeCalificacion {
		e.escribirHoja(x, folio, nombre)
	}
}

// tituloDeUnidad nombra una unidad de evaluación: en Compositores, la obra;
// en Intérpretes, que se califica entera, nada.
func (e *ExpedientePDF) tituloDeUnidad(p models.Postulacion, obraID int64) string {
	for _, obra := range p.Obras {
		if obra.ID == obraID {
			return strings.TrimSuffix(e.t("acuse.obra", obra.Orden)+" · "+obra.Campos["obra.titulo"], " · ")
		}
	}
	return ""
}

// escribirRevision escribe lo que dejó la revisión documental: su resultado,
// las observaciones internas y las subsanaciones solicitadas.
func (e *ExpedientePDF) escribirRevision(x models.Expediente) {
	d, p := e.d, x.Postulacion
	d.Seccion(e.t("expediente.revision"))
	if resultado := models.ResultadoDeRevision(p.Estado); resultado != "" {
		d.Campo(e.t("expediente.resultado_revision"), e.t("expediente.revision."+resultado))
	}
	if p.MotivoDescarte != "" {
		d.Campo(e.t("expediente.motivo_descarte"), p.MotivoDescarte)
	}
	if len(x.Revision.Observaciones) > 0 {
		d.Subtitulo(e.t("expediente.observaciones"))
		for _, observacion := range x.Revision.Observaciones {
			autor := e.fecha(observacion.CreadoAt) + "\n" + observacion.Revisor
			if titulo := e.tituloDeUnidad(p, observacion.ObraID); titulo != "" {
				autor += "\n" + titulo
			}
			d.Campo(autor, observacion.Texto)
		}
	}
	for i, subsanacion := range p.Subsanaciones {
		estado := "pendiente"
		switch {
		case !subsanacion.AtendidaAt.IsZero():
			estado = "atendida"
		case !subsanacion.CerradaAt.IsZero():
			estado = "cerrada"
		}
		d.Subtitulo(e.t("expediente.subsanacion", i+1))
		d.Campo(e.t("expediente.subsanacion.solicitada"), e.fecha(subsanacion.EnviadaAt))
		d.Campo(e.t("expediente.subsanacion.limite"), e.fecha(subsanacion.FechaLimite))
		d.Campo(e.t("expediente.estado"), e.t("expediente.subsanacion."+estado))
		for _, problema := range subsanacion.Problemas {
			d.Campo(e.nombreDeElemento(x, problema), problema.Problema)
		}
	}
}

// nombreDeElemento nombra lo que una subsanación señaló: un campo, por su
// etiqueta del formulario, o un documento, por la del requisito; con su obra
// si es de una.
func (e *ExpedientePDF) nombreDeElemento(x models.Expediente, problema models.ProblemaSubsanacion) string {
	nombre := e.t("campo." + problema.Campo)
	if problema.Clase == models.ProblemaDocumento {
		nombre = problema.Campo
		for _, requisito := range x.Configuracion.Requisitos {
			if requisito.Clave == problema.Campo {
				nombre = e.bilingue(requisito.EtiquetaES, requisito.EtiquetaEN)
				break
			}
		}
	}
	if titulo := e.tituloDeUnidad(x.Postulacion, problema.ObraID); titulo != "" {
		nombre += "\n" + titulo
	}
	return nombre
}

// puntajes escribe, un renglón por criterio, lo que un jurado calificó.
func (e *ExpedientePDF) puntajes(criterios []models.CriterioEvaluacion, evaluacion models.Evaluacion) {
	for _, criterio := range criterios {
		valor := "—"
		if puntaje, calificado := evaluacion.Puntajes[criterio.Clave]; calificado {
			valor = e.t("expediente.puntaje", puntaje, criterio.EscalaMax)
		}
		e.d.Campo(e.bilingue(criterio.EtiquetaES, criterio.EtiquetaEN), valor)
	}
	if evaluacion.Observaciones != "" {
		e.d.Campo(e.t("expediente.observaciones_jurado"), evaluacion.Observaciones)
	}
}

// escribirEvaluacionPropia escribe la evaluación del jurado que pidió el
// expediente: la suya y sólo la suya.
func (e *ExpedientePDF) escribirEvaluacionPropia(x models.Expediente) {
	d := e.d
	d.Seccion(e.t("expediente.evaluacion_propia"))
	for _, evaluacion := range x.Propias {
		if titulo := e.tituloDeUnidad(x.Postulacion, evaluacion.ObraID); titulo != "" {
			d.Subtitulo(titulo)
		}
		switch {
		case evaluacion.Excusado:
			d.Campo(e.t("expediente.excusa"), evaluacion.MotivoExcusa)
		case evaluacion.ID == 0:
			d.Campo(e.t("expediente.estado"), e.t("expediente.evaluacion.sin_iniciar"))
		default:
			estado := "borrador"
			if evaluacion.Enviada {
				estado = "enviada"
			}
			d.Campo(e.t("expediente.estado"), e.t("expediente.evaluacion."+estado))
			e.puntajes(x.Configuracion.Criterios, evaluacion)
		}
	}
}

// escribirResultados escribe, para la supervisión, lo que calificó cada
// jurado y los agregados de cada unidad.
func (e *ExpedientePDF) escribirResultados(x models.Expediente) {
	d := e.d
	d.Seccion(e.t("expediente.evaluacion"))
	for _, fila := range x.Resultados {
		if titulo := e.tituloDeUnidad(x.Postulacion, fila.Unidad.ObraID); titulo != "" {
			d.Subtitulo(titulo)
		}
		for _, jurado := range x.Jurados {
			d.Subtitulo(jurado.Nombre)
			evaluacion, enviada := fila.PorJurado[jurado.ID]
			switch motivo, excusado := fila.Excusas[jurado.ID]; {
			case excusado:
				d.Campo(e.t("expediente.excusa"), motivo)
			case !enviada:
				d.Campo(e.t("expediente.estado"), e.t("expediente.evaluacion.sin_enviar"))
			default:
				e.puntajes(x.Configuracion.Criterios, evaluacion)
			}
		}
		d.Subtitulo(e.t("expediente.agregados"))
		d.Campo(e.t("expediente.jurados_que_evaluaron"), strconv.Itoa(fila.JuradosQueEvaluaron))
		if fila.JuradosQueEvaluaron > 0 {
			d.Campo(e.t("expediente.suma"), strconv.Itoa(fila.Total))
			d.Campo(e.t("expediente.promedio"), strconv.FormatFloat(fila.Promedio, 'f', -1, 64))
			d.Campo(e.t("expediente.puntaje_comparable"), strconv.FormatFloat(fila.Comparable, 'f', -1, 64))
		}
	}
}

// escribirHoja añade, en página aparte, una hoja de calificación en blanco
// por cada unidad (RF-ARCH-43), para quien prefiere trabajar en papel.
func (e *ExpedientePDF) escribirHoja(x models.Expediente, folio, nombre string) {
	d, cfg := e.d, x.Configuracion
	for _, obraID := range models.UnidadesDeEvaluacion(cfg.Convocatoria.Tipo, x.Postulacion) {
		d.SaltoDePagina()
		d.Seccion(e.t("expediente.hoja"))
		d.Campo(e.t("acuse.folio"), folio)
		d.Campo(e.t("expediente.hoja.postulante"), nombre)
		if titulo := e.tituloDeUnidad(x.Postulacion, obraID); titulo != "" {
			d.Campo(e.t("seccion.obras"), titulo)
		}
		d.Espacio(10)
		for _, criterio := range cfg.Criterios {
			d.Campo(e.bilingue(criterio.EtiquetaES, criterio.EtiquetaEN), e.t("expediente.hoja.puntaje", criterio.EscalaMin, criterio.EscalaMax))
			d.Espacio(8)
		}
		d.Campo(e.t("expediente.observaciones_jurado"), strings.TrimSuffix(strings.Repeat("______________________________________________________\n", 6), "\n"))
		d.Espacio(10)
		d.Campo(e.t("expediente.hoja.firma"), "______________________________")
	}
}

// escribirIndice escribe la portada y el índice en el documento dado, a
// partir de su página en curso. desplazamiento es cuántas páginas ocupa el
// propio índice: se suma al número de página de cada postulación.
func (e *ExpedientePDF) escribirIndice(d *pdf.Documento, titulo string, desplazamiento int) {
	d.Encabezado(e.t("sitio.nombre"), e.t("expediente.titulo"), "")
	d.Seccion(titulo)
	d.Campo(e.t("expediente.postulaciones"), strconv.Itoa(len(e.entradas)))
	d.Campo(e.t("expediente.generado"), e.fecha(e.o.Generado)+" · "+e.o.Solicitante)
	d.Espacio(4)
	d.Parrafo(e.t("expediente.leyenda"), pdf.TintaSuave)
	d.Seccion(e.t("expediente.indice"))
	for _, entrada := range e.entradas {
		d.Renglon(entrada.titulo, strconv.Itoa(entrada.pagina+desplazamiento))
	}
}

// Bytes termina el expediente y devuelve el PDF. Con titulo, le antepone una
// portada con índice (RF-ARCH-42): es el caso del expediente por
// convocatoria; el de una sola postulación va sin él.
func (e *ExpedientePDF) Bytes(titulo string) ([]byte, error) {
	if titulo == "" || len(e.entradas) == 0 {
		e.d.QuitarMarcadores()
		return e.d.Bytes()
	}
	// El índice lleva números de página que dependen de cuántas páginas
	// ocupa él mismo. Se escribe primero en un documento aparte, sólo para
	// contarlas; después, de verdad, al final, y se pasa al frente.
	ensayo := pdf.Nuevo("", "", e.t("acuse.pagina"), e.o.Generado)
	e.escribirIndice(ensayo, titulo, 0)
	paginasDelCuerpo := e.d.Paginas()
	e.d.SaltoDePagina()
	e.d.FijarPie(e.pie(titulo))
	e.escribirIndice(e.d, titulo, ensayo.Paginas())
	e.d.MoverAlInicio(paginasDelCuerpo)
	return e.d.Bytes()
}
