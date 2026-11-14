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

package models

import (
	"fmt"
	"syscall"
)

// EspacioLibre devuelve los bytes que un usuario sin privilegios puede
// escribir todavía en el sistema de archivos del directorio dado. Es lo que
// le queda al servicio, que no corre como root.
func EspacioLibre(directorio string) (uint64, error) {
	var estado syscall.Statfs_t
	if err := syscall.Statfs(directorio, &estado); err != nil {
		return 0, fmt.Errorf("consultar el espacio en disco de %s: %w", directorio, err)
	}
	return uint64(estado.Bavail) * uint64(estado.Bsize), nil //nolint:unconvert // los tipos varían entre sistemas
}
