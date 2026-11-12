// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

//go:build unix

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// verificarDuenoDatos impide un tropiezo clásico de operación: ejecutar la
// herramienta como root (o como otro usuario) sobre los datos del servicio.
// SQLite crearía archivos —la base o su bitácora WAL— con otro dueño, y el
// servidor web, que corre como «fimnme», dejaría de poder escribir.
//
// Si el directorio de datos ya existe y pertenece a otro usuario, se rechaza
// la operación antes de tocar nada.
func verificarDuenoDatos(dirDatos string) error {
	info, err := os.Stat(dirDatos)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // se creará con el usuario actual
	}
	if err != nil {
		return fmt.Errorf("directorio de datos: %w", err)
	}
	estado, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if dueno, actual := int(estado.Uid), os.Geteuid(); dueno != actual {
		return fmt.Errorf("el directorio de datos %s pertenece al usuario con uid %d y usted es el uid %d.\n"+
			"Ejecute la herramienta como el dueño de los datos, por ejemplo:\n"+
			"    sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos %s ...",
			dirDatos, dueno, actual, dirDatos)
	}
	return nil
}
