// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

// fimnme-cli es la herramienta de mantenimiento de la plataforma: todo lo que
// no es atender peticiones HTTP. Comparte models/ con fimnme-web y no duplica
// ninguna regla de negocio.
//
// Órdenes disponibles hoy:
//
//	fimnme-cli migrate        aplica las migraciones pendientes del esquema
//	fimnme-cli sembrar-demo   carga una edición de demostración (sólo desarrollo)
//	fimnme-cli version        muestra la versión
//
// Las demás (TUI, importación asistida, exportaciones, espejo, ARCO…) llegan
// en el paso 10 del plan.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"

	"github.com/jesarx/fimnme/models"
)

// version se fija al compilar con -ldflags "-X main.version=…" (ver Makefile).
var version = "desarrollo"

// Códigos de salida: 0 correcto, 1 falló la operación, 2 uso incorrecto.
const (
	salidaError = 1
	salidaUso   = 2
)

// entorno devuelve una variable de entorno o su valor por omisión.
func entorno(nombre, porOmision string) string {
	if valor := os.Getenv(nombre); valor != "" {
		return valor
	}
	return porOmision
}

// ayuda describe el uso de la herramienta.
func ayuda(banderas *flag.FlagSet) {
	fmt.Fprint(os.Stderr, `fimnme-cli — mantenimiento de la Plataforma de Postulaciones FIMNME

Uso:
  fimnme-cli [--datos DIRECTORIO] ORDEN

Órdenes:
  migrate        Aplica, en orden, las migraciones pendientes del esquema.
  sembrar-demo   Carga una edición de demostración en una base sin ediciones.
                 Sólo para entornos de desarrollo.
  version        Muestra la versión.

Opciones:
`)
	banderas.PrintDefaults()
	fmt.Fprint(os.Stderr, `
En el servidor, ejecute la herramienta como el usuario del servicio:
  sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme migrate
`)
}

// actorCLI identifica a esta herramienta en la bitácora: toda operación con
// efectos queda asentada con el usuario del sistema que la ejecutó (RF-CLI-09).
func actorCLI() models.Actor {
	nombre := "desconocido"
	if u, err := user.Current(); err == nil && u.Username != "" {
		nombre = u.Username
	}
	return models.Actor{
		Perfil: "cli",
		Agente: fmt.Sprintf("fimnme-cli %s (usuario del sistema: %s)", version, nombre),
	}
}

func main() {
	banderas := flag.NewFlagSet("fimnme-cli", flag.ContinueOnError)
	dirDatos := banderas.String("datos", entorno("FIMNME_DATOS", "data"),
		"directorio de datos: base SQLite y acervo de archivos (FIMNME_DATOS)")
	banderas.Usage = func() { ayuda(banderas) }
	if err := banderas.Parse(os.Args[1:]); err != nil {
		os.Exit(salidaUso)
	}
	if banderas.NArg() != 1 {
		ayuda(banderas)
		os.Exit(salidaUso)
	}

	var err error
	switch orden := banderas.Arg(0); orden {
	case "version":
		fmt.Println("fimnme-cli", version)
		return
	case "migrate":
		err = conRepositorio(*dirDatos, migrar)
	case "sembrar-demo":
		err = conRepositorio(*dirDatos, sembrarDemo)
	default:
		fmt.Fprintf(os.Stderr, "Orden desconocida: %s\n\n", orden)
		ayuda(banderas)
		os.Exit(salidaUso)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(salidaError)
	}
}

// conRepositorio abre la base, ejecuta la orden y cierra la base.
func conRepositorio(dirDatos string, orden func(context.Context, *models.Repositorio) error) error {
	// Antes de tocar nada: si los datos son de otro usuario, no se sigue.
	if err := verificarDuenoDatos(dirDatos); err != nil {
		return err
	}
	repositorio, err := models.AbrirRepositorio(dirDatos)
	if err != nil {
		return err
	}
	defer repositorio.Cerrar()
	return orden(context.Background(), repositorio)
}

// migrar aplica las migraciones pendientes e informa cuáles fueron.
func migrar(ctx context.Context, repositorio *models.Repositorio) error {
	aplicadas, err := repositorio.Migrar(ctx, actorCLI())
	for _, m := range aplicadas {
		fmt.Printf("Migración aplicada: %04d_%s\n", m.Version, m.Nombre)
	}
	if err != nil {
		return err
	}
	if len(aplicadas) == 0 {
		fmt.Println("El esquema ya está al día; no había migraciones pendientes.")
	}
	return nil
}
