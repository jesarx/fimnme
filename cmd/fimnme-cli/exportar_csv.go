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

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jesarx/fimnme/controllers"
	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Orden «export-csv»: genera un reporte desde una plantilla guardada, sin
// pasar por la interfaz web (RF-CLI-05).
//
//	fimnme-cli export-csv --listar
//	fimnme-cli export-csv --plantilla ID|NOMBRE --edicion N --como CORREO [--salida ARCHIVO]
//
// Sirve para programar un reporte (un cron que cada noche deja el avance de
// la convocatoria en una carpeta) y para sacarlo cuando el servidor web no
// está en marcha. No es un segundo generador: llama al mismo que la API
// (controllers.GenerarReporte), con las mismas validaciones, el mismo
// catálogo de campos y la misma puerta a las calificaciones.
//
// Por eso exige decir EN NOMBRE DE QUIÉN se genera (--como): un reporte puede
// llevar las calificaciones de los tres jurados, que sólo son de la
// supervisión. La cuenta debe ser de un administrador o de la coordinación
// nacional, y la generación queda en la bitácora a su nombre, igual que si la
// hubiera pedido desde el navegador.

// sesionPorCorreo devuelve los permisos que tiene hoy la cuenta de un correo.
// Es la forma en que la herramienta actúa en nombre de una persona: no abre
// ninguna sesión de navegador, sólo lee sus perfiles vigentes.
func sesionPorCorreo(ctx context.Context, repositorio *models.Repositorio, correo string) (*models.Sesion, error) {
	if correo == "" {
		return nil, errors.New("indique con --como el correo de la cuenta en cuyo nombre se genera el reporte")
	}
	usuario, _, err := repositorio.UsuarioPorEmail(ctx, models.NormalizarEmail(correo))
	if errors.Is(err, models.ErrNoEncontrado) {
		return nil, fmt.Errorf("no existe ninguna cuenta con el correo %s", correo)
	}
	if err != nil {
		return nil, err
	}
	sesion, err := repositorio.SesionDeUsuario(ctx, usuario.ID)
	if errors.Is(err, models.ErrNoEncontrado) {
		return nil, fmt.Errorf("la cuenta %s está desactivada", correo)
	}
	return sesion, err
}

// plantillaPedida localiza una plantilla de reporte por su id o por su
// nombre exacto (los nombres no se repiten).
func plantillaPedida(ctx context.Context, repositorio *models.Repositorio, pedida string) (models.PlantillaDeReporte, error) {
	if id, err := strconv.ParseInt(pedida, 10, 64); err == nil {
		plantilla, err := repositorio.PlantillaDeReportePorID(ctx, id)
		if errors.Is(err, models.ErrNoEncontrado) {
			return plantilla, fmt.Errorf("no existe la plantilla %d; vea las que hay con: fimnme-cli export-csv --listar", id)
		}
		return plantilla, err
	}
	todas, err := repositorio.PlantillasDeReporte(ctx)
	if err != nil {
		return models.PlantillaDeReporte{}, err
	}
	for _, plantilla := range todas {
		if plantilla.Nombre == pedida {
			return plantilla, nil
		}
	}
	return models.PlantillaDeReporte{}, fmt.Errorf("no existe ninguna plantilla llamada «%s»; vea las que hay con: fimnme-cli export-csv --listar", pedida)
}

// convocatoriaDeLaEdicion devuelve la convocatoria de un tipo en la edición
// de un número dado: «la de Compositores de la edición 50».
func convocatoriaDeLaEdicion(ctx context.Context, repositorio *models.Repositorio, numero int, tipo models.TipoConvocatoria) (int64, error) {
	ediciones, err := repositorio.ListarEdiciones(ctx)
	if err != nil {
		return 0, err
	}
	for _, edicion := range ediciones {
		if edicion.Numero != numero {
			continue
		}
		convocatorias, err := repositorio.ConvocatoriasDeEdicion(ctx, edicion.ID)
		if err != nil {
			return 0, err
		}
		for _, c := range convocatorias {
			if c.Tipo == tipo {
				return c.ID, nil
			}
		}
		return 0, fmt.Errorf("la edición %d no tiene convocatoria de %s", numero, tipo)
	}
	return 0, fmt.Errorf("no existe la edición %d", numero)
}

// listarPlantillas muestra las plantillas de reporte guardadas.
func listarPlantillas(ctx context.Context, repositorio *models.Repositorio) error {
	plantillas, err := repositorio.PlantillasDeReporte(ctx)
	if err != nil {
		return err
	}
	if len(plantillas) == 0 {
		fmt.Fprintln(salida, "No hay plantillas de reporte guardadas. Se crean desde el generador de reportes del panel.")
		return nil
	}
	fmt.Fprintf(salida, "%-5s %-13s %s\n", "ID", "CONVOCATORIA", "NOMBRE")
	for _, plantilla := range plantillas {
		fmt.Fprintf(salida, "%-5d %-13s %s\n", plantilla.ID, plantilla.Tipo, plantilla.Nombre)
	}
	return nil
}

// exportarCSV ejecuta «export-csv».
func exportarCSV(args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas := flag.NewFlagSet("export-csv", flag.ContinueOnError)
		listar := banderas.Bool("listar", false, "muestra las plantillas guardadas y termina")
		pedida := banderas.String("plantilla", "", "id o nombre exacto de la plantilla de reporte")
		numeroDeEdicion := banderas.Int("edicion", 0, "número de la edición cuya convocatoria se reporta")
		convocatoriaID := banderas.Int64("convocatoria", 0, "id de la convocatoria, en lugar de --edicion")
		como := banderas.String("como", "", "correo de la cuenta (administrador o coordinación nacional) en cuyo nombre se genera")
		archivo := banderas.String("salida", "", "archivo de destino; sin él, el reporte va a la salida estándar")
		idioma := banderas.String("idioma", models.IdiomaES, "idioma de encabezados y valores: es o en")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		if *listar {
			return listarPlantillas(ctx, repositorio)
		}
		if *pedida == "" || (*numeroDeEdicion == 0) == (*convocatoriaID == 0) {
			return errors.New("uso: fimnme-cli export-csv --plantilla ID|NOMBRE (--edicion N | --convocatoria ID) --como CORREO [--salida ARCHIVO] [--idioma es|en]")
		}
		if *idioma != models.IdiomaES && *idioma != models.IdiomaEN {
			return errors.New("--idioma debe ser es o en")
		}
		sesion, err := sesionPorCorreo(ctx, repositorio, *como)
		if err != nil {
			return err
		}
		if !models.PuedeGenerarReportes(sesion) {
			return fmt.Errorf("la cuenta %s no puede generar reportes: hace falta ser administrador o de la coordinación nacional", *como)
		}
		plantilla, err := plantillaPedida(ctx, repositorio, *pedida)
		if err != nil {
			return err
		}
		if *convocatoriaID == 0 {
			if *convocatoriaID, err = convocatoriaDeLaEdicion(ctx, repositorio, *numeroDeEdicion, plantilla.Tipo); err != nil {
				return err
			}
		}
		vistas, err := views.Nuevo()
		if err != nil {
			return err
		}

		// El mismo generador que la API, con la plantilla como único parámetro.
		actor := actorCLI()
		actor.UsuarioID = sesion.Usuario.ID
		reporte, err := controllers.GenerarReporte(ctx, repositorio, vistas, sesion, actor, *convocatoriaID,
			url.Values{"plantilla": {strconv.FormatInt(plantilla.ID, 10)}}, *idioma)
		var regla *models.ErrorValidacion
		switch {
		case errors.As(err, &regla):
			return fmt.Errorf("el reporte no puede generarse:\n  %s", motivosDeRegla(vistas, regla))
		case errors.Is(err, models.ErrNoEncontrado):
			return fmt.Errorf("no existe la convocatoria %d", *convocatoriaID)
		case err != nil:
			return err
		}

		if *archivo == "" {
			return views.EscribirCSV(salida, reporte.Separador, reporte.Encabezados, reporte.Filas)
		}
		// El reporte lleva datos personales: el archivo nace legible sólo
		// para quien lo genera. Se escribe aparte y se renombra al final,
		// para que quien lo recoja nunca encuentre medio archivo.
		var contenido bytes.Buffer
		if err := views.EscribirCSV(&contenido, reporte.Separador, reporte.Encabezados, reporte.Filas); err != nil {
			return err
		}
		temporal := *archivo + ".parcial"
		if err := os.WriteFile(temporal, contenido.Bytes(), 0o600); err != nil {
			return fmt.Errorf("escribir el reporte: %w", err)
		}
		if err := os.Rename(temporal, *archivo); err != nil {
			return fmt.Errorf("escribir el reporte: %w", errors.Join(err, os.Remove(temporal)))
		}
		renglones := "renglones"
		if len(reporte.Filas) == 1 {
			renglones = "renglón"
		}
		fmt.Fprintf(salida, "Reporte «%s» escrito en %s: %d %s.\n", plantilla.Nombre, *archivo, len(reporte.Filas), renglones)
		return nil
	}
}

// motivosDeRegla explica, con los mismos textos que muestra el panel, por
// qué el modelo rechazó una operación: un renglón por cada dato.
func motivosDeRegla(vistas *views.Motor, regla *models.ErrorValidacion) string {
	var lista []string
	for _, e := range regla.Errores {
		lista = append(lista, strings.TrimSpace(e.Campo+": "+vistas.T(models.IdiomaES, e.Clave)))
	}
	return strings.Join(lista, "\n  ")
}
