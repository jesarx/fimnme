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
