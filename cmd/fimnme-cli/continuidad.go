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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Órdenes de continuidad operativa (§5.17):
//
//	verify                 compara el acervo de archivos con lo que la base registra
//	mirror nueva-clave     genera la clave con que se firman las instantáneas
//	mirror snapshot        toma una instantánea firmada de la base y replica el acervo
//	mirror verify          comprueba una instantánea en el destino del espejo
//	mirror list            muestra las últimas instantáneas registradas

// directorioDeBlobs es el acervo de archivos dentro del directorio de datos.
func directorioDeBlobs(dirDatos string) string {
	return filepath.Join(dirDatos, "blobs")
}

// legible escribe un tamaño en bytes como «812 KB», «4.3 MB» o «8.2 GB».
func legible(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%d KB", bytes>>10)
	}
	return fmt.Sprintf("%d B", bytes)
}

// mostrarArchivos imprime una lista de archivos con problemas, acotada: mil
// renglones no ayudan a nadie.
func mostrarArchivos(titulo string, archivos []models.Archivo) {
	if len(archivos) == 0 {
		return
	}
	fmt.Fprintf(salida, "\n%s (%d):\n", titulo, len(archivos))
	for i, archivo := range archivos {
		if i == 20 {
			fmt.Fprintf(salida, "  … y %d más\n", len(archivos)-20)
			break
		}
		fmt.Fprintf(salida, "  archivo %d · %s · %s\n", archivo.ID, archivo.SHA256, legible(archivo.TamanoBytes))
	}
}

// mostrarVerificacion imprime el resultado de comparar una base con un acervo.
func mostrarVerificacion(v models.VerificacionDeAcervo) {
	fmt.Fprintf(salida, "Archivos registrados: %d · correctos: %d\n", v.Registrados, v.Correctos)
	mostrarArchivos("FALTAN en disco (pérdida de datos)", v.Faltantes)
	mostrarArchivos("ALTERADOS: el contenido no es el registrado", v.Alterados)
	mostrarArchivos("Purgados por anonimización que siguen en disco", v.PurgadosPresentes)
	if len(v.Huerfanos) > 0 {
		fmt.Fprintf(salida, "\nEn disco sin registro en la base (%d). No es pérdida: suelen ser subidas que no llegaron a confirmarse.\n", len(v.Huerfanos))
		for i, clave := range v.Huerfanos {
			if i == 20 {
				fmt.Fprintf(salida, "  … y %d más\n", len(v.Huerfanos)-20)
				break
			}
			fmt.Fprintf(salida, "  %s\n", clave)
		}
	}
	for _, ajeno := range v.Ajenos {
		fmt.Fprintf(salida, "\nEn el almacén hay algo que no es un archivo guardado: %s\n", ajeno)
	}
}

// verificar ejecuta «verify» (RF-CLI-08): comprueba que cada archivo que la
// base registra exista en disco y coincida con su SHA-256, y busca lo que
// hay en disco sin registro. Termina con error si falta o se alteró algo.
func verificar(dirDatos string, args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas := flag.NewFlagSet("verify", flag.ContinueOnError)
		rapido := banderas.Bool("rapido", false, "sólo comprueba existencia y tamaño, sin leer cada archivo")
		purgar := banderas.Bool("purgar", false, "termina de borrar los archivos purgados por anonimización que sigan en disco")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		almacen, err := models.AbrirAlmacenExistente(directorioDeBlobs(dirDatos))
		if err != nil {
			return err
		}
		archivos, err := repositorio.ArchivosRegistrados(ctx)
		if err != nil {
			return err
		}
		v, err := models.VerificarAcervo(ctx, archivos, almacen, *rapido)
		if err != nil {
			return err
		}
		if *purgar && len(v.PurgadosPresentes) > 0 {
			pendientes := models.PurgarArchivos(almacen, v.PurgadosPresentes, func(archivo models.Archivo, err error) {
				fmt.Fprintf(os.Stderr, "No se pudo borrar el archivo %d: %v\n", archivo.ID, err)
			})
			fmt.Fprintf(salida, "Purga terminada: %d archivos borrados del disco.\n", len(v.PurgadosPresentes)-pendientes)
			v.PurgadosPresentes = v.PurgadosPresentes[:pendientes]
		}
		mostrarVerificacion(v)
		if libre, err := models.EspacioLibre(dirDatos); err == nil {
			fmt.Fprintf(salida, "\nEspacio libre en el disco de datos: %s\n", legible(int64(libre)))
		}
		if err := repositorio.RegistrarVerificacion(ctx, actorCLI(), v, *rapido); err != nil {
			return err
		}
		if !v.Integro() {
			return errors.New("el acervo NO está íntegro: revise la lista de arriba")
		}
		fmt.Fprintln(salida, "\nEl acervo está íntegro.")
		return nil
	}
}

// banderasDeEspejo declara las opciones comunes de las órdenes del espejo.
func banderasDeEspejo(nombre string) (banderas *flag.FlagSet, destino, clave *string) {
	banderas = flag.NewFlagSet(nombre, flag.ContinueOnError)
	destino = banderas.String("destino", entorno("FIMNME_ESPEJO", ""), "directorio del espejo (FIMNME_ESPEJO)")
	clave = banderas.String("clave", entorno("FIMNME_CLAVE_ESPEJO", ""), "archivo con la clave de firma (FIMNME_CLAVE_ESPEJO)")
	return banderas, destino, clave
}

// claveYDestino comprueba que se dieron el destino y la clave, y lee ésta.
func claveYDestino(destino, rutaDeLaClave string) ([]byte, error) {
	if destino == "" {
		return nil, errors.New("indique el directorio del espejo con --destino")
	}
	if rutaDeLaClave == "" {
		return nil, errors.New("indique el archivo de la clave con --clave (se genera con «mirror nueva-clave»)")
	}
	return models.LeerClaveDeEspejo(rutaDeLaClave)
}

// nuevaClave ejecuta «mirror nueva-clave»: genera la clave de firma.
func nuevaClave(args []string) error {
	banderas, _, clave := banderasDeEspejo("mirror nueva-clave")
	if err := banderas.Parse(args); err != nil {
		return err
	}
	if *clave == "" {
		return errors.New("indique dónde guardar la clave con --clave")
	}
	if err := models.NuevaClaveDeEspejo(*clave); err != nil {
		return err
	}
	fmt.Fprintf(salida, "Clave generada en %s (sólo su dueño puede leerla).\n\n", *clave)
	fmt.Fprintln(salida, "Cópiela al servidor espejo por un canal seguro: sin ella no pueden verificarse las instantáneas.")
	fmt.Fprintln(salida, "No la guarde junto a las instantáneas: quien tuviera ambas podría falsificar una.")
	return nil
}

// tomarInstantanea ejecuta «mirror snapshot» (RF-CLI-06, RF-OPS-01 y 06):
//
//  1. copia la base en caliente al destino (VACUUM INTO);
//  2. replica el acervo por contenido y retira del destino lo purgado;
//  3. escribe el manifiesto firmado con HMAC-SHA256;
//  4. lo registra en snapshot_espejo y en la bitácora;
//  5. borra las instantáneas más antiguas que el plazo de conservación.
func tomarInstantanea(dirDatos string, args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas, destino, rutaDeLaClave := banderasDeEspejo("mirror snapshot")
		dias := banderas.Int("conservar-dias", 90, "días que se conservan las instantáneas en el destino; 0 las conserva todas")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		clave, err := claveYDestino(*destino, *rutaDeLaClave)
		if err != nil {
			return err
		}
		almacen, err := models.AbrirAlmacenExistente(directorioDeBlobs(dirDatos))
		if err != nil {
			return err
		}
		directorio := filepath.Join(*destino, models.DirectorioDeInstantaneas)
		// 0750: la instantánea es la base completa, con datos personales.
		if err := os.MkdirAll(directorio, 0o750); err != nil {
			return fmt.Errorf("crear el directorio de instantáneas: %w", err)
		}
		momento := time.Now().UTC()
		ruta := filepath.Join(directorio, models.NombreDeInstantanea(momento))
		if err := repositorio.InstantaneaEn(ctx, ruta); err != nil {
			return err
		}
		// Los archivos se leen DESPUÉS de la instantánea: todo lo que ella
		// registra ya estaba en el almacén cuando se tomó.
		archivos, err := repositorio.ArchivosRegistrados(ctx)
		if err != nil {
			return err
		}
		registrados, copiados, faltantes, retirados, err := models.SincronizarBlobs(ctx, almacen, filepath.Join(*destino, models.DirectorioDeBlobsEspejo), archivos)
		if err != nil {
			return err
		}
		suma, tamano, err := models.SumaDeInstantanea(ruta)
		if err != nil {
			return err
		}
		manifiesto, err := models.EscribirManifiesto(ruta, models.ManifiestoDeInstantanea{
			SHA256: suma, TamanoBytes: tamano, CreadoAt: momento.Format(time.RFC3339), Version: version,
			BlobsRegistrados: registrados, BlobsCopiados: copiados, BlobsFaltantes: faltantes, BlobsRetirados: retirados,
		}, clave)
		if err != nil {
			return err
		}
		detalle := ""
		if faltantes > 0 {
			detalle = fmt.Sprintf("%d archivos registrados no están en el acervo de origen", faltantes)
		}
		if err := repositorio.RegistrarInstantanea(ctx, actorCLI(), models.InstantaneaDeEspejo{
			Ruta: ruta, SHA256: suma, HMACSHA256: manifiesto.HMACSHA256, TamanoBytes: tamano, ArchivosOK: registrados - faltantes,
			ArchivosFaltantes: faltantes, Destino: *destino, Correcta: faltantes == 0, Detalle: detalle,
		}); err != nil {
			return err
		}
		fmt.Fprintf(salida, "Instantánea: %s (%s)\n  SHA-256      %s\n  HMAC-SHA256  %s\n", ruta, legible(tamano), suma, manifiesto.HMACSHA256)
		fmt.Fprintf(salida, "Acervo: %d archivos registrados · %d copiados ahora · %d retirados por anonimización\n", registrados, copiados, retirados)
		if *dias > 0 {
			borradas, err := models.DepurarInstantaneas(*destino, momento.AddDate(0, 0, -*dias))
			if err != nil {
				return err
			}
			if borradas > 0 {
				fmt.Fprintf(salida, "Instantáneas de más de %d días borradas: %d\n", *dias, borradas)
			}
		}
		if faltantes > 0 {
			return fmt.Errorf("la instantánea quedó INCOMPLETA: %s. Ejecute «fimnme-cli verify»", detalle)
		}
		return nil
	}
}

// verificarInstantanea ejecuta «mirror verify». No abre la base en
// producción: está pensada para el servidor espejo, que sólo tiene el destino
// y la clave.
func verificarInstantanea(args []string) error {
	banderas, destino, rutaDeLaClave := banderasDeEspejo("mirror verify")
	nombre := banderas.String("instantanea", "", "nombre de la instantánea; por omisión, la más reciente")
	rapido := banderas.Bool("rapido", false, "sólo comprueba existencia y tamaño de los archivos, sin leerlos")
	if err := banderas.Parse(args); err != nil {
		return err
	}
	clave, err := claveYDestino(*destino, *rutaDeLaClave)
	if err != nil {
		return err
	}
	if *nombre == "" {
		nombres, err := models.Instantaneas(*destino)
		if err != nil {
			return err
		}
		if len(nombres) == 0 {
			return fmt.Errorf("no hay instantáneas en %s", *destino)
		}
		*nombre = nombres[len(nombres)-1]
	}
	v, err := models.VerificarInstantanea(context.Background(), *destino, *nombre, clave, *rapido, models.ArchivosDeInstantanea)
	if err != nil {
		return err
	}
	fmt.Fprintf(salida, "Instantánea %s, tomada el %s\n", *nombre, v.Manifiesto.CreadoAt)
	switch {
	case !v.FirmaValida:
		return errors.New("la FIRMA NO ES VÁLIDA: el manifiesto no corresponde a esta instantánea o a esta clave. No la use")
	case !v.BaseIntegra:
		return errors.New("la BASE NO ES LA DEL MANIFIESTO: el archivo cambió después de firmarse. No la use")
	}
	fmt.Fprintf(salida, "Firma HMAC-SHA256 válida · base íntegra (%s)\n", legible(v.Manifiesto.TamanoBytes))
	mostrarVerificacion(v.Acervo)
	if !v.Correcta() {
		return errors.New("el acervo del espejo NO está completo: revise la lista de arriba")
	}
	fmt.Fprintln(salida, "\nLa instantánea es correcta y puede usarse para restaurar.")
	return nil
}

// listarInstantaneas ejecuta «mirror list»: las últimas registradas.
func listarInstantaneas(ctx context.Context, repositorio *models.Repositorio) error {
	instantaneas, err := repositorio.UltimasInstantaneas(ctx, 20)
	if err != nil {
		return err
	}
	if len(instantaneas) == 0 {
		fmt.Fprintln(salida, "Aún no se ha tomado ninguna instantánea.")
		return nil
	}
	for _, i := range instantaneas {
		estado := "correcta"
		if !i.Correcta {
			estado = "INCOMPLETA: " + i.Detalle
		}
		fmt.Fprintf(salida, "%s  %s  %s  %d archivos  %s\n", i.CreadoAt.UTC().Format("2006-01-02 15:04Z"), filepath.Base(i.Ruta), legible(i.TamanoBytes), i.ArchivosOK, estado)
	}
	return nil
}
