// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Orden «sembrar-demo»: carga una edición ficticia con sus dos convocatorias,
// para poder revisar el sistema en un entorno de desarrollo mientras no exista
// el panel de administración (paso 4 del plan).
//
// Salvaguardas:
//   - Sólo actúa sobre una base SIN ediciones; si ya hay alguna, no hace nada.
//   - Escribe a través del repositorio, igual que lo hará el administrador, y
//     por tanto queda asentada en la bitácora de auditoría.
//
// Nada de lo que aquí aparece son bases oficiales.

// utc construye una marca de tiempo en UTC.
func utc(anio int, mes time.Month, dia, hora, minuto int) time.Time {
	return time.Date(anio, mes, dia, hora, minuto, 0, 0, time.UTC)
}

// sembrarDemo crea la edición de demostración. Las fechas se dan en UTC: el
// cierre «29 de enero, 23:59» de la Ciudad de México (UTC−6) es el 30 de enero
// a las 05:59 UTC.
func sembrarDemo(ctx context.Context, repositorio *models.Repositorio) error {
	pendientes, err := repositorio.MigracionesPendientes(ctx)
	if err != nil {
		return err
	}
	if len(pendientes) > 0 {
		return fmt.Errorf("el esquema no está al día; ejecute primero: fimnme-cli migrate")
	}
	existentes, err := repositorio.ContarEdiciones(ctx)
	if err != nil {
		return err
	}
	if existentes > 0 {
		fmt.Printf("La base ya tiene %d edición(es); no se siembra nada.\n", existentes)
		return nil
	}

	actor := actorCLI()
	edicion := models.Edicion{Numero: 49, Anio: 2027, NombreES: "Edición 49 · 2027", NombreEN: "49th Edition · 2027"}
	if edicion.ID, err = repositorio.CrearEdicion(ctx, actor, edicion); err != nil {
		return err
	}
	convocatorias := []models.Convocatoria{
		{
			Edicion:   edicion,
			Tipo:      models.TipoInterpretes,
			TituloES:  "Convocatoria de Intérpretes",
			TituloEN:  "Call for Performers",
			BasesMDES: demoBasesInterpretesES,
			BasesMDEN: demoBasesInterpretesEN,
			Estado:    models.EstadoPostulacion,
			MaxVideos: 3,
			Etapas: []models.Etapa{
				{Clave: models.EtapaPostulacion, IniciaAt: utc(2026, 11, 2, 6, 0), TerminaAt: utc(2027, 1, 30, 5, 59)},
				{Clave: models.EtapaRevisionDocumental, IniciaAt: utc(2027, 2, 1, 6, 0), TerminaAt: utc(2027, 2, 27, 5, 59)},
				{Clave: models.EtapaEvaluacion, IniciaAt: utc(2027, 3, 1, 6, 0), TerminaAt: utc(2027, 4, 1, 5, 59)},
			},
		},
		{
			Edicion:   edicion,
			Tipo:      models.TipoCompositores,
			TituloES:  "Convocatoria de Compositores",
			TituloEN:  "Call for Composers",
			BasesMDES: demoBasesCompositoresES,
			BasesMDEN: demoBasesCompositoresEN,
			Estado:    models.EstadoPublicada,
			MaxObras:  2,
			MaxVideos: 3,
			Etapas: []models.Etapa{
				{Clave: models.EtapaPostulacion, IniciaAt: utc(2026, 11, 23, 6, 0), TerminaAt: utc(2027, 2, 13, 5, 59)},
				{Clave: models.EtapaRevisionDocumental, IniciaAt: utc(2027, 2, 15, 6, 0), TerminaAt: utc(2027, 3, 13, 5, 59)},
				{Clave: models.EtapaEvaluacion, IniciaAt: utc(2027, 3, 15, 6, 0), TerminaAt: utc(2027, 4, 15, 5, 59)},
			},
		},
	}
	for _, c := range convocatorias {
		// Con los requisitos documentales y los criterios iniciales de su tipo.
		id, err := repositorio.CrearConvocatoriaConfigurada(ctx, actor, c,
			models.RequisitosIniciales(c.Tipo), models.CriteriosIniciales())
		if err != nil {
			return err
		}
		if c.Tipo != models.TipoCompositores {
			continue
		}
		// Catálogo de dotaciones de ejemplo. En la segunda, la excepción
		// típica: una obra electroacústica no lleva particellas.
		for orden, d := range []models.Dotacion{
			{NombreES: "Cuarteto de cuerdas", NombreEN: "String quartet"},
			{NombreES: "Obra electroacústica", NombreEN: "Electroacoustic work"},
			{NombreES: "Ensamble mixto (hasta 8 intérpretes)", NombreEN: "Mixed ensemble (up to 8 performers)"},
		} {
			d.ConvocatoriaID, d.Orden, d.Activa = id, orden+1, true
			dotacionID, _, err := repositorio.GuardarDotacion(ctx, actor, d)
			if err != nil {
				return err
			}
			if orden != 1 {
				continue
			}
			for _, rq := range models.RequisitosIniciales(c.Tipo) {
				if rq.Clave == "particellas" {
					rq.ConvocatoriaID, rq.DotacionID, rq.Obligatorio = id, dotacionID, false
					if _, _, err := repositorio.GuardarRequisito(ctx, actor, rq); err != nil {
						return err
					}
				}
			}
		}
	}
	fmt.Printf("Edición de demostración creada: %s, con %d convocatorias.\n", edicion.NombreES, len(convocatorias))
	return nil
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
