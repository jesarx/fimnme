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

	"github.com/jesarx/fimnme/models"
)

// exportarMariaDB atiende «fimnme-cli exportar-mariadb --salida ARCHIVO.sql
// --manifiesto ARCHIVO.tsv» (ADR 0029): escribe la base completa como un .sql
// para MariaDB y el manifiesto con que deploy/verify_and_migrate_mariadb.sh
// comprueba la carga. Lee dentro de una transacción de lectura: el servicio
// puede seguir en marcha, aunque lo que escriba después no entra.
//
// Los dos archivos llevan datos personales: se crean legibles sólo por su
// dueño, y no deben quedar fuera del servidor.
func exportarMariaDB(args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repo *models.Repositorio) error {
		banderas := flag.NewFlagSet("exportar-mariadb", flag.ContinueOnError)
		salida := banderas.String("salida", "", "archivo .sql para MariaDB")
		manifiesto := banderas.String("manifiesto", "", "manifiesto de verificación (filas y SHA-256 por tabla)")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		if *salida == "" || *manifiesto == "" {
			return errors.New("faltan --salida y --manifiesto")
		}
		archivoSQL, err := os.OpenFile(*salida, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer archivoSQL.Close()
		archivoManifiesto, err := os.OpenFile(*manifiesto, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer archivoManifiesto.Close()
		tablas := 0
		err = repo.ExportarParaMigrar(ctx, func(lista []models.TablaParaMigrar, filas func(models.TablaParaMigrar, func([]any) error) error) error {
			tablas = len(lista)
			return models.EscribirMigracionMariaDB(archivoSQL, archivoManifiesto, lista, filas)
		})
		if err != nil {
			return fmt.Errorf("exportar para MariaDB: %w", err)
		}
		if err := archivoSQL.Close(); err != nil {
			return err
		}
		if err := archivoManifiesto.Close(); err != nil {
			return err
		}
		fmt.Printf("%d tablas exportadas a %s; manifiesto en %s.\n", tablas, *salida, *manifiesto)
		return nil
	}
}
