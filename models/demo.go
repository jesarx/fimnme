// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"context"
	"time"
)

// FuenteDemo es una fuente de datos EN MEMORIA y de sólo lectura, con una
// edición ficticia. Existe únicamente para poder construir y revisar el frontal
// antes de que exista la persistencia.
//
// PROVISIONAL: se sustituye por models/repository.go (SQLite) en el paso de
// persistencia del plan (docs/PLAN_DE_DESARROLLO.md). Nada de lo que aquí
// aparece son bases oficiales.
type FuenteDemo struct {
	convocatorias []Convocatoria
}

// utc construye una marca de tiempo en UTC para los datos de demostración.
func utc(anio int, mes time.Month, dia, hora, minuto int) time.Time {
	return time.Date(anio, mes, dia, hora, minuto, 0, 0, time.UTC)
}

// NuevaFuenteDemo arma la edición de demostración con sus dos convocatorias.
// Las fechas se guardan en UTC: el cierre "29 de enero, 23:59" de la Ciudad de
// México (UTC−6) es el 30 de enero a las 05:59 UTC.
func NuevaFuenteDemo() *FuenteDemo {
	edicion := Edicion{
		ID:       1,
		Numero:   49,
		Anio:     2027,
		NombreES: "Edición 49 · 2027",
		NombreEN: "49th Edition · 2027",
	}
	return &FuenteDemo{convocatorias: []Convocatoria{
		{
			ID:        1,
			Edicion:   edicion,
			Tipo:      TipoInterpretes,
			TituloES:  "Convocatoria de Intérpretes",
			TituloEN:  "Call for Performers",
			BasesMDES: demoBasesInterpretesES,
			BasesMDEN: demoBasesInterpretesEN,
			Estado:    EstadoPostulacion,
			MaxVideos: 3,
			Etapas: []Etapa{
				{Clave: EtapaPostulacion, IniciaAt: utc(2026, 11, 2, 6, 0), TerminaAt: utc(2027, 1, 30, 5, 59)},
				{Clave: EtapaRevisionDocumental, IniciaAt: utc(2027, 2, 1, 6, 0), TerminaAt: utc(2027, 2, 27, 5, 59)},
				{Clave: EtapaEvaluacion, IniciaAt: utc(2027, 3, 1, 6, 0), TerminaAt: utc(2027, 4, 1, 5, 59)},
			},
		},
		{
			ID:        2,
			Edicion:   edicion,
			Tipo:      TipoCompositores,
			TituloES:  "Convocatoria de Compositores",
			TituloEN:  "Call for Composers",
			BasesMDES: demoBasesCompositoresES,
			BasesMDEN: demoBasesCompositoresEN,
			Estado:    EstadoPublicada,
			MaxObras:  2,
			MaxVideos: 3,
			Etapas: []Etapa{
				{Clave: EtapaPostulacion, IniciaAt: utc(2026, 11, 23, 6, 0), TerminaAt: utc(2027, 2, 13, 5, 59)},
				{Clave: EtapaRevisionDocumental, IniciaAt: utc(2027, 2, 15, 6, 0), TerminaAt: utc(2027, 3, 13, 5, 59)},
				{Clave: EtapaEvaluacion, IniciaAt: utc(2027, 3, 15, 6, 0), TerminaAt: utc(2027, 4, 15, 5, 59)},
			},
		},
	}}
}

// ConvocatoriasPublicas devuelve las convocatorias vigentes que se anuncian en
// la portada.
func (f *FuenteDemo) ConvocatoriasPublicas(_ context.Context) ([]Convocatoria, error) {
	var visibles []Convocatoria
	for _, c := range f.convocatorias {
		if ConvocatoriaVigente(c) {
			visibles = append(visibles, c)
		}
	}
	return visibles, nil
}

// ConvocatoriaPublica busca una convocatoria por número de edición y tipo. Una
// convocatoria en borrador se reporta como inexistente: no debe poder
// distinguirse de una que no existe.
func (f *FuenteDemo) ConvocatoriaPublica(_ context.Context, numeroEdicion int, tipo TipoConvocatoria) (Convocatoria, error) {
	for _, c := range f.convocatorias {
		if c.Edicion.Numero == numeroEdicion && c.Tipo == tipo && ConvocatoriaVisibleAlPublico(c) {
			return c, nil
		}
	}
	return Convocatoria{}, ErrNoEncontrado
}

// Textos de demostración. Resumen las reglas de la especificación para que las
// pantallas se vean con contenido verosímil; NO son las bases oficiales.

const demoBasesInterpretesES = `**Texto de demostración.** Las bases oficiales se capturan desde el panel de administración.

# A quién se dirige

A intérpretes de música nueva, nacionales y extranjeros, que propongan un **programa de concierto** completo. La postulación se presenta en una de dos subcategorías:

- **Solista:** persona física.
- **Ensamble:** agrupación con representante y listado de integrantes (nombre e instrumento).

# Qué se presenta

1. Datos generales y semblanza artística.
2. Propuesta de concierto, con requerimientos técnicos y entidad federativa donde se realizará.
3. Programa: las obras que integran el concierto. No hay límite de número de obras ni de duración total.
4. De una a tres ligas a videos de ejecución.

Si el concierto se propone fuera de la Ciudad de México, deben indicarse nombre y domicilio de la sede, fecha y hora.

# Documentos

- Fotografía (JPEG o PNG, hasta 5 MB).
- Por cada obra: partitura (PDF, hasta 30 MB) y autorización del titular de la obra (PDF, hasta 10 MB).

# Cómo se evalúa

Un jurado de tres especialistas califica **la propuesta de concierto completa** con dos criterios: realización y valor artístico.
`

const demoBasesInterpretesEN = `**Demonstration text.** The official call is entered from the administration panel.

# Who can apply

Performers of new music, from Mexico and abroad, who propose a complete **concert programme**. Applications fall into one of two subcategories:

- **Soloist:** an individual.
- **Ensemble:** a group with a representative and a list of members (name and instrument).

# What to submit

1. General information and artistic biography.
2. Concert proposal, with technical requirements and the Mexican state where it will take place.
3. Programme: the works that make up the concert. There is no limit on the number of works or on total duration.
4. One to three links to performance videos.

If the concert is proposed outside Mexico City, the venue name and address, date and time are required.

# Documents

- Photograph (JPEG or PNG, up to 5 MB).
- For each work: score (PDF, up to 30 MB) and authorisation from the rights holder (PDF, up to 10 MB).

# How applications are assessed

A jury of three specialists scores **the complete concert proposal** on two criteria: execution and artistic merit.
`

const demoBasesCompositoresES = `**Texto de demostración.** Las bases oficiales se capturan desde el panel de administración.

# A quién se dirige

A compositoras y compositores, nacionales y extranjeros, que presenten **obras de autoría propia**.

# Qué se presenta

1. Datos generales y semblanza artística.
2. Hasta **dos obras** por postulación. Cada obra se adscribe a una de las dotaciones instrumentales del catálogo de esta edición.

# Documentos

- Fotografía (JPEG o PNG, hasta 5 MB).
- Por cada obra: partitura, particellas y archivos de audio. Cuáles son obligatorios depende de la dotación elegida: una obra electroacústica, por ejemplo, no lleva particellas.

# Cómo se evalúa

Un jurado de tres especialistas califica **cada obra por separado** con dos criterios: realización y valor artístico.
`

const demoBasesCompositoresEN = `**Demonstration text.** The official call is entered from the administration panel.

# Who can apply

Composers, from Mexico and abroad, submitting **works of their own authorship**.

# What to submit

1. General information and artistic biography.
2. Up to **two works** per application. Each work is assigned to one of the instrumentations in this edition's catalogue.

# Documents

- Photograph (JPEG or PNG, up to 5 MB).
- For each work: score, parts and audio files. Which of them are mandatory depends on the chosen instrumentation: an electroacoustic work, for instance, has no parts.

# How applications are assessed

A jury of three specialists scores **each work separately** on two criteria: execution and artistic merit.
`
