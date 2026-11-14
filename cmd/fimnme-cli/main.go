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

// fimnme-cli es la herramienta de mantenimiento de la plataforma: todo lo que
// no es atender peticiones HTTP. Comparte models/ con fimnme-web y no duplica
// ninguna regla de negocio.
//
// Órdenes disponibles hoy:
//
//	fimnme-cli                sin argumentos y sin base: la inicializa
//	fimnme-cli init           inicializa la base (crea el esquema)
//	fimnme-cli migrate        aplica las migraciones pendientes del esquema
//	fimnme-cli crear-usuario  crea una cuenta interna y da su enlace de activación
//	fimnme-cli sembrar-demo   carga una edición de demostración (sólo desarrollo)
//	fimnme-cli verify         compara el acervo de archivos con la base
//	fimnme-cli mirror         instantáneas firmadas para el servidor espejo
//	fimnme-cli export-json    exportación sanitizada para el portal público
//	fimnme-cli arco           anonimización ante una solicitud ARCO
//	fimnme-cli version        muestra la versión
//
// Las demás (TUI, importación asistida, extensión de campos) llegan en el
// paso 10b del plan.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"

	"github.com/jesarx/fimnme/models"
)

// version se fija al compilar con -ldflags "-X main.version=…" (ver Makefile).
var version = "desarrollo"

// salida y entrada son por donde las órdenes escriben sus resultados y leen
// las confirmaciones. Las pruebas los sustituyen.
var (
	salida  io.Writer = os.Stdout
	entrada io.Reader = os.Stdin
)

// Códigos de salida: 0 correcto, 1 falló la operación, 2 uso incorrecto.
const (
	salidaError = 1
	salidaUso   = 2
)

// avisoVersion es lo que imprime --version, según la convención de GNU: nombre
// y versión del programa, su licencia y la advertencia de que no hay garantía.
func avisoVersion(programa string) string {
	return programa + " " + version + `
Licencia GPLv3: GNU GPL versión 3 <https://www.gnu.org/licenses/gpl-3.0.html>.
Esto es software libre: usted es libre de cambiarlo y redistribuirlo.
NO HAY GARANTÍA, en la medida en que lo permita la ley.
`
}

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
  fimnme-cli [--datos DIRECTORIO] ORDEN [opciones de la orden]

Sin ORDEN, y si el directorio de datos aún no tiene base, la inicializa.

Órdenes:
  init           Inicializa la base de datos: la crea y aplica el esquema.
  migrate        Aplica, en orden, las migraciones pendientes del esquema.
  crear-usuario  Crea una cuenta interna y muestra su enlace de activación.
                 Así se da de alta al primer administrador:
                   crear-usuario --perfil admin_sistema --email CORREO \
                                 --nombre NOMBRE --apellidos APELLIDOS
                 Perfiles: admin_sistema, coordinacion_nacional.
  sembrar-demo   Carga una edición de demostración en una base sin ediciones.
                 Sólo para entornos de desarrollo.
  verify         Comprueba que cada archivo registrado exista en disco y
                 coincida con su SHA-256. Opciones: --rapido, --purgar.
  mirror ORDEN   Servidor espejo. Órdenes:
                   nueva-clave --clave ARCHIVO
                   snapshot --destino DIR --clave ARCHIVO [--conservar-dias N]
                   verify   --destino DIR --clave ARCHIVO [--instantanea NOMBRE]
                   list
  export-json    Exportación sanitizada para el portal público.
                 Opciones: --edicion N, --salida ARCHIVO.
  arco anonimizar FOLIO --fundamento TEXTO [--con-cuenta]
                 Borra los datos personales de una postulación (solicitud
                 ARCO). Pide confirmación doble. No se puede deshacer.
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
		Perfil: models.ActorCLI,
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
	if banderas.NArg() < 1 {
		// Sin argumentos, y sobre un directorio de datos que aún no tiene
		// base, la herramienta inicializa el entorno: crea la base y aplica
		// el esquema. Es el primer paso de una instalación nueva. Si la base
		// ya existe NO se toca: aplicar migraciones a una base en uso es
		// siempre un acto explícito («migrate»).
		if _, err := os.Stat(filepath.Join(*dirDatos, models.NombreBase)); errors.Is(err, fs.ErrNotExist) {
			fmt.Printf("No hay base de datos en %s: se inicializa.\n", *dirDatos)
			if err := conRepositorio(*dirDatos, migrar); err != nil {
				fmt.Fprintln(os.Stderr, "ERROR:", err)
				os.Exit(salidaError)
			}
			fmt.Println("\nEntorno inicializado. Siguiente paso: crear al primer administrador con «crear-usuario».")
			return
		}
		ayuda(banderas)
		os.Exit(salidaUso)
	}

	var err error
	switch orden := banderas.Arg(0); orden {
	case "version":
		fmt.Print(avisoVersion("fimnme-cli"))
		return
	case "init", "migrate":
		// «init» inicializa una base nueva y «migrate» actualiza una
		// existente; hacen lo mismo: aplicar las migraciones que falten.
		err = conRepositorio(*dirDatos, migrar)
	case "crear-usuario":
		err = conRepositorio(*dirDatos, crearUsuario(banderas.Args()[1:]))
	case "sembrar-demo":
		err = conRepositorio(*dirDatos, sembrarDemo)
	case "verify":
		err = conRepositorio(*dirDatos, verificar(*dirDatos, banderas.Args()[1:]))
	case "export-json":
		err = conRepositorio(*dirDatos, exportarJSON(*dirDatos, banderas.Args()[1:]))
	case "mirror":
		err = ordenDeEspejo(*dirDatos, banderas.Args()[1:])
	case "arco":
		if banderas.NArg() < 2 || banderas.Arg(1) != "anonimizar" {
			fmt.Fprint(os.Stderr, "Uso: fimnme-cli arco anonimizar FOLIO --fundamento TEXTO [--con-cuenta]\n")
			os.Exit(salidaUso)
		}
		err = conRepositorio(*dirDatos, anonimizar(*dirDatos, banderas.Args()[2:]))
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

// ordenDeEspejo reparte las órdenes de «mirror». Sólo dos abren la base:
// generar la clave y verificar una instantánea no la necesitan, y la segunda
// debe poder ejecutarse en el servidor espejo, donde no hay base en uso.
func ordenDeEspejo(dirDatos string, args []string) error {
	if len(args) == 0 {
		return errors.New("falta la orden: mirror nueva-clave | snapshot | verify | list")
	}
	switch args[0] {
	case "nueva-clave":
		return nuevaClave(args[1:])
	case "verify":
		return verificarInstantanea(args[1:])
	case "snapshot":
		return conRepositorio(dirDatos, tomarInstantanea(dirDatos, args[1:]))
	case "list":
		return conRepositorio(dirDatos, listarInstantaneas)
	}
	return fmt.Errorf("orden de espejo desconocida: %s (use nueva-clave, snapshot, verify o list)", args[0])
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
	// El índice de búsqueda se deriva de las postulaciones: se rehace en
	// cada migración, por si esta versión indexa algo que la anterior no.
	indexadas, err := repositorio.ReconstruirIndiceDeBusqueda(ctx)
	if err != nil {
		return fmt.Errorf("reconstruir el índice de búsqueda: %w", err)
	}
	fmt.Printf("Índice de búsqueda reconstruido: %d postulaciones.\n", indexadas)
	return nil
}
