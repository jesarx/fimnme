// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

//go:build !unix

package main

// verificarDuenoDatos no comprueba nada fuera de Unix: la verificación del
// dueño de los datos sólo importa en el servidor (Debian). Esta variante
// existe para que la herramienta compile en Windows durante el desarrollo.
func verificarDuenoDatos(string) error {
	return nil
}
