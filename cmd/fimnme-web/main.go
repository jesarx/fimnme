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

// fimnme-web es el servidor HTTP de la plataforma: sirve el frontal integrado
// sobre la base SQLite y, más adelante, la API REST y los trabajadores internos.
//
// Es un servicio PÚBLICO: escucha por omisión en 127.0.0.1:8080 y debe quedar
// SIEMPRE detrás de un proxy inverso con TLS (Caddy o nginx). Nunca se expone
// directamente a Internet.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/jesarx/fimnme/controllers"
	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// version se fija al compilar con -ldflags "-X main.version=…" (ver Makefile).
var version = "desarrollo"

// entorno devuelve el valor de una variable de entorno o, si está vacía, el
// valor por omisión. Permite configurar el servicio desde systemd
// (EnvironmentFile) sin tocar la línea de comandos.
func entorno(nombre, porOmision string) string {
	if valor := os.Getenv(nombre); valor != "" {
		return valor
	}
	return porOmision
}

// avisoVersion es lo que imprime --version, según la convención de GNU: nombre
// y versión del programa, su licencia y la advertencia de que no hay garantía.
func avisoVersion(programa string) string {
	return programa + " " + version + `
Licencia GPLv3: GNU GPL versión 3 <https://www.gnu.org/licenses/gpl-3.0.html>.
Esto es software libre: usted es libre de cambiarlo y redistribuirlo.
NO HAY GARANTÍA, en la medida en que lo permita la ley.
`
}

// enteroDeEntorno lee un entero de una variable de entorno; si falta o no es
// un número, devuelve el valor por omisión.
func enteroDeEntorno(nombre string, porOmision int) int {
	if n, err := strconv.Atoi(os.Getenv(nombre)); err == nil {
		return n
	}
	return porOmision
}

func main() {
	var (
		ip             string
		puerto         string
		modo           string
		dirDatos       string
		urlBase        string
		correoMinuto   int
		discoMinGB     int
		mostrarVersion bool
	)
	// -s y --serve son la misma bandera, como en los demás sistemas de la CNMO.
	const ayudaServe = "dirección IP en la que escucha el servidor (FIMNME_SERVE)"
	flag.StringVar(&ip, "s", entorno("FIMNME_SERVE", "127.0.0.1"), ayudaServe+" (abreviatura de --serve)")
	flag.StringVar(&ip, "serve", entorno("FIMNME_SERVE", "127.0.0.1"), ayudaServe)
	// -p y --port son la misma bandera.
	const ayudaPort = "puerto TCP en el que escucha el servidor (FIMNME_PORT)"
	flag.StringVar(&puerto, "p", entorno("FIMNME_PORT", "8080"), ayudaPort+" (abreviatura de --port)")
	flag.StringVar(&puerto, "port", entorno("FIMNME_PORT", "8080"), ayudaPort)
	flag.StringVar(&modo, "entorno", entorno("FIMNME_ENTORNO", controllers.EntornoProduccion),
		"entorno de ejecución: desarrollo o produccion (FIMNME_ENTORNO)")
	flag.StringVar(&dirDatos, "datos", entorno("FIMNME_DATOS", "data"),
		"directorio de datos: base SQLite y acervo de archivos (FIMNME_DATOS)")
	flag.StringVar(&urlBase, "url-base", entorno("FIMNME_URL_BASE", ""),
		"dirección pública del sitio, para los enlaces de los correos; p. ej. https://postula.ejemplo.mx (FIMNME_URL_BASE)")
	flag.IntVar(&correoMinuto, "correo-por-minuto", enteroDeEntorno("FIMNME_CORREO_POR_MINUTO", 30),
		"máximo de correos que se envían por minuto (FIMNME_CORREO_POR_MINUTO)")
	flag.IntVar(&discoMinGB, "disco-min-gb", enteroDeEntorno("FIMNME_DISCO_MIN_GB", 10),
		"espacio libre mínimo del disco de datos, en GB; por debajo se alerta. 0 no vigila (FIMNME_DISCO_MIN_GB)")
	flag.BoolVar(&mostrarVersion, "version", false, "muestra la versión y termina")
	flag.Parse()

	if mostrarVersion {
		fmt.Print(avisoVersion("fimnme-web"))
		return
	}

	// Logs estructurados en JSON a la salida estándar (RNF-12); systemd los
	// recoge en el journal.
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if modo != controllers.EntornoDesarrollo && modo != controllers.EntornoProduccion {
		log.Error("entorno no reconocido", "entorno", modo)
		os.Exit(2)
	}

	vistas, err := views.Nuevo()
	if err != nil {
		log.Error("no se pudo cargar el frontal incrustado", "error", err)
		os.Exit(1)
	}

	repositorio, err := models.AbrirRepositorio(dirDatos)
	if err != nil {
		log.Error("no se pudo abrir la base de datos", "datos", dirDatos, "error", err)
		os.Exit(1)
	}
	defer repositorio.Cerrar()

	// El servidor NO migra por su cuenta: cambiar el esquema es un acto
	// explícito del operador (fimnme-cli migrate). Con el esquema
	// desactualizado, o con una base más nueva que este binario, no se sirve.
	pendientes, err := repositorio.MigracionesPendientes(context.Background())
	if err != nil {
		log.Error("la base de datos no corresponde a este binario", "datos", dirDatos, "error", err)
		repositorio.Cerrar()
		os.Exit(1)
	}
	if len(pendientes) > 0 {
		log.Error("hay migraciones sin aplicar; ejecute: fimnme-cli --datos "+dirDatos+" migrate",
			"pendientes", len(pendientes), "primera", pendientes[0].Nombre)
		repositorio.Cerrar()
		os.Exit(1)
	}

	// Los enlaces de los correos se arman con la dirección pública del sitio.
	// Nunca se deduce de la petición: la cabecera Host la controla el visitante.
	if urlBase == "" {
		if modo == controllers.EntornoProduccion {
			log.Error("falta la dirección pública del sitio: defina FIMNME_URL_BASE (p. ej. https://postula.ejemplo.mx)")
			repositorio.Cerrar()
			os.Exit(2)
		}
		urlBase = "http://" + net.JoinHostPort(ip, puerto)
		log.Warn("FIMNME_URL_BASE sin definir: los enlaces de los correos apuntarán a "+urlBase,
			"consecuencia", "sólo servirán desde esta misma máquina")
	}

	// Almacén de archivos: los documentos y los acuses viven en disco, junto
	// a la base, direccionados por su SHA-256.
	almacen, err := models.AbrirAlmacenEnDisco(filepath.Join(dirDatos, "blobs"))
	if err != nil {
		log.Error("no se pudo abrir el almacén de archivos", "error", err)
		repositorio.Cerrar()
		os.Exit(1)
	}
	verificador := controllers.NuevoVerificadorDeEnlaces()

	app := controllers.Nueva(controllers.Config{
		Vistas: vistas, Datos: repositorio, Entorno: modo, Log: log, URLBase: urlBase,
		Almacen: almacen, Enlaces: verificador, Exportaciones: filepath.Join(dirDatos, "exportaciones"),
	})

	// Transporte de correo. Aún no existe el SMTP propio (paso 14 del plan):
	// los mensajes se escriben en el log en vez de enviarse. El cuerpo, que
	// lleva enlaces de un solo uso, sólo se muestra en desarrollo.
	mailer := controllers.MailerRegistro{Log: log, MostrarCuerpo: modo == controllers.EntornoDesarrollo}
	if modo == controllers.EntornoProduccion {
		log.Warn("NO hay transporte de correo real: los mensajes no se envían y sus enlaces no se muestran; " +
			"las cuentas no podrán verificarse ni recuperarse hasta configurar el SMTP")
	}
	trabajador := &controllers.TrabajadorCorreo{Bandeja: repositorio, Mailer: mailer, Log: log, PorMinuto: correoMinuto}
	servidor := &http.Server{
		Addr:    net.JoinHostPort(ip, puerto),
		Handler: app.Manejador(),
		// Plazos que impiden que conexiones lentas o abandonadas retengan
		// recursos. La subida de archivos, que puede tardar mucho más, amplía
		// los suyos petición por petición (controllers/documentos.go).
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// Apagado ordenado: al recibir SIGINT o SIGTERM (systemctl stop/restart)
	// se dejan terminar las peticiones en curso antes de salir.
	ctx, detener := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer detener()

	// Trabajadores internos: se detienen con la misma señal que el servidor.
	var trabajadores sync.WaitGroup
	etapas := &controllers.TrabajadorEtapas{Datos: repositorio, Log: log}
	enlaces := &controllers.TrabajadorEnlaces{Datos: repositorio, Verificador: verificador, Log: log}
	recordatorios := &controllers.TrabajadorRecordatorios{Datos: repositorio, Vistas: vistas, Log: log, URLBase: urlBase}
	exportaciones := &controllers.TrabajadorExportaciones{Datos: repositorio, Almacen: almacen, Vistas: vistas, Log: log,
		Directorio: filepath.Join(dirDatos, "exportaciones"), URLBase: urlBase}
	// Vigilante del espacio en disco: alerta antes de que el disco se llene.
	disco := &controllers.TrabajadorDisco{Datos: repositorio, Vistas: vistas, Log: log, Directorio: dirDatos,
		MinimoLibre: uint64(max(discoMinGB, 0)) << 30}
	for _, ejecutar := range []func(context.Context){trabajador.Ejecutar, etapas.Ejecutar, enlaces.Ejecutar, recordatorios.Ejecutar,
		exportaciones.Ejecutar, disco.Ejecutar} {
		trabajadores.Go(func() { ejecutar(ctx) })
	}

	errServidor := make(chan error, 1)
	go func() {
		log.Info("fimnme-web escuchando", "direccion", servidor.Addr, "entorno", modo,
			"datos", dirDatos, "version", version)
		errServidor <- servidor.ListenAndServe()
	}()

	select {
	case err := <-errServidor:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("el servidor se detuvo", "error", err)
			repositorio.Cerrar() // os.Exit no ejecuta los defer
			os.Exit(1)
		}
	case <-ctx.Done():
		log.Info("señal de apagado recibida; terminando peticiones en curso")
		ctxApagado, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		if err := servidor.Shutdown(ctxApagado); err != nil {
			log.Error("apagado forzado", "error", err)
			repositorio.Cerrar() // os.Exit no ejecuta los defer
			os.Exit(1)
		}
	}
	// Se espera a que los trabajadores suelten la base antes de cerrarla.
	detener()
	trabajadores.Wait()
	log.Info("fimnme-web detenido")
}
