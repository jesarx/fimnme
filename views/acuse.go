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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/internal/pdf"
	"github.com/jesarx/fimnme/models"
)

// Acuse de recibo en PDF (§5.10).
//
// El acuse se compone ÚNICAMENTE a partir del snapshot del envío: no consulta
// la base ni el reloj. Por eso es reproducible (RF-PDF-03): el mismo snapshot
// produce siempre el mismo archivo, hoy o dentro de diez años, aunque la
// postulación haya cambiado después por una subsanación.
//
// No lleva imágenes ni fuentes incrustadas (D-16): pesa unos pocos KB.

// formatoSnapshot es el formato de las marcas de tiempo dentro del snapshot.
const formatoSnapshot = "2006-01-02T15:04:05Z"

// longitudSuma es cuántos caracteres del SHA-256 de cada documento se
// imprimen en el acuse (RF-PDF-01). Bastan para identificarlo; la suma
// completa está en el snapshot.
const longitudSuma = 12

// AcusePDF compone el acuse de un envío, en el idioma del snapshot.
func (m *Motor) AcusePDF(s models.SnapshotEnvio) ([]byte, error) {
	idioma := s.Idioma
	t := func(clave string, args ...any) string { return m.textos.T(idioma, clave, args...) }
	// bilingue elige entre dos textos según el idioma del acuse; si falta el
	// inglés usa el español.
	bilingue := func(es, en string) string {
		if idioma == models.IdiomaEN && en != "" {
			return en
		}
		return es
	}
	enviada, err := time.Parse(formatoSnapshot, s.EnviadaAt)
	if err != nil {
		return nil, fmt.Errorf("fecha de envío del snapshot: %w", err)
	}

	d := pdf.Nuevo(t("acuse.titulo")+" "+s.Folio, t("acuse.pie", s.Folio, s.Version), t("acuse.pagina"), enviada)
	d.Encabezado(t("sitio.nombre"), t("acuse.titulo"), s.Folio)

	// 1. Datos del envío.
	d.Seccion(t("acuse.envio"))
	d.Campo(t("acuse.folio"), s.Folio)
	d.Campo(t("acuse.edicion"), bilingue(s.Edicion.NombreES, s.Edicion.NombreEN))
	convocatoria := bilingue(s.Convocatoria.TituloES, s.Convocatoria.TituloEN)
	if s.Subcategoria != "" {
		convocatoria += " · " + t("subcategoria."+s.Subcategoria)
	}
	d.Campo(t("acuse.convocatoria"), convocatoria)
	// La hora va en la zona de la Ciudad de México, que es la de las bases, y
	// además en UTC, para quien postula desde otro huso.
	d.Campo(t("acuse.enviada"), m.textos.FechaHora(idioma, enviada)+"\n"+enviada.UTC().Format("2006-01-02 15:04")+" UTC")
	d.Campo(t("acuse.version"), strconv.Itoa(s.Version))
	d.Campo(t("acuse.cuenta"), strings.TrimSpace(s.Titular.Nombre+" "+s.Titular.Apellidos)+" · "+s.Titular.Email)

	// 2. Datos generales y propuesta de concierto, en el orden del formulario.
	formulario := models.FormularioDe(models.TipoConvocatoria(s.Convocatoria.Tipo), models.Subcategoria(s.Subcategoria))
	valor := func(def models.DefinicionCampo, crudo string) string {
		switch def.Tipo {
		case models.CampoOpcion:
			if crudo != "" {
				return t("opcion." + crudo)
			}
		case models.CampoEntidad, models.CampoEntidadResidencia:
			for _, e := range models.EntidadesFederativas() {
				if e.Clave == crudo {
					return e.Nombre
				}
			}
		}
		return crudo
	}
	seccion := ""
	for _, def := range formulario.Campos {
		// Los campos traen su sección en la ruta: «datos.» o «concierto.».
		if actual, _, _ := strings.Cut(def.Ruta, "."); actual != seccion {
			seccion = actual
			d.Seccion(t("seccion." + seccion))
		}
		// Un campo condicional que no aplicó (los datos de la sede, cuando el
		// concierto es en la Ciudad de México) no se imprime vacío.
		if def.Condicion != "" && s.Campos[def.Ruta] == "" {
			continue
		}
		d.Campo(t("campo."+def.Ruta), valor(def, s.Campos[def.Ruta]))
	}
	if formulario.TieneIntegrantes {
		d.Subtitulo(t("campo.integrantes"))
		for i, integrante := range s.Integrantes {
			d.Campo(strconv.Itoa(i+1), integrante.Nombre+" · "+integrante.Instrumento)
		}
	}
	if formulario.TieneVideos {
		d.Subtitulo(t("campo.videos"))
		for i, video := range s.Videos {
			d.Campo(strconv.Itoa(i+1), video.URL+"\n"+t("enlace.estado."+video.Estado))
		}
	}

	// 3. Obras.
	d.Seccion(t("seccion.obras"))
	ordenDeObra := make(map[int64]int)
	for _, obra := range s.Obras {
		ordenDeObra[obra.ID] = obra.Orden
		d.Subtitulo(t("acuse.obra", obra.Orden))
		for _, def := range formulario.CamposObra {
			if def.Tipo == models.CampoDotacion {
				d.Campo(t("campo."+def.Ruta), bilingue(obra.DotacionES, obra.DotacionEN))
				continue
			}
			d.Campo(t("campo."+def.Ruta), valor(def, obra.Campos[def.Ruta]))
		}
	}

	// 4. Documentos entregados, con el inicio de su SHA-256.
	d.Seccion(t("seccion.documentos"))
	for _, documento := range s.Documentos {
		etiqueta := bilingue(documento.EtiquetaES, documento.EtiquetaEN)
		if documento.ObraID != 0 {
			etiqueta = t("acuse.documento_de_obra", etiqueta, ordenDeObra[documento.ObraID])
		}
		suma := documento.SHA256
		if len(suma) > longitudSuma {
			suma = suma[:longitudSuma]
		}
		d.Campo(etiqueta, t("acuse.documento", documento.Version, tamanoLegible(documento.TamanoBytes), suma))
	}

	// 5. Leyenda.
	d.Espacio(10)
	d.Parrafo(t("acuse.leyenda"), pdf.TintaSuave)
	return d.Bytes()
}

// tamanoLegible escribe un tamaño en bytes como «812 KB» o «4.3 MB».
func tamanoLegible(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return strconv.FormatFloat(float64(bytes)/(1<<20), 'f', 1, 64) + " MB"
	case bytes >= 1<<10:
		return strconv.FormatInt(bytes>>10, 10) + " KB"
	}
	return strconv.FormatInt(bytes, 10) + " B"
}
