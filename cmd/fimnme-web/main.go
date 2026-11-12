// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

// fimnme-web es el servidor HTTP de la plataforma: sirve el frontal integrado
// y, más adelante, la API REST y los trabajadores internos.
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

func main() {
	var (
		ip             string
		puerto         string
		modo           string
		mostrarVersion bool
	)
	// -s y --serve son la misma bandera, como en los demás sistemas de la CNMO.
	const ayudaServe = "dirección IP en la que escucha el servidor (FIMNME_SERVE)"
	flag.StringVar(&ip, "s", entorno("FIMNME_SERVE", "127.0.0.1"), ayudaServe+" (abreviatura de --serve)")
	flag.StringVar(&ip, "serve", entorno("FIMNME_SERVE", "127.0.0.1"), ayudaServe)
	flag.StringVar(&puerto, "port", entorno("FIMNME_PORT", "8080"), "puerto TCP en el que escucha el servidor (FIMNME_PORT)")
	flag.StringVar(&modo, "entorno", entorno("FIMNME_ENTORNO", controllers.EntornoProduccion),
		"entorno de ejecución: desarrollo o produccion (FIMNME_ENTORNO)")
	flag.BoolVar(&mostrarVersion, "version", false, "muestra la versión y termina")
	flag.Parse()

	if mostrarVersion {
		fmt.Println("fimnme-web", version)
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

	// PROVISIONAL: mientras no exista models/repository.go, las páginas se
	// alimentan de datos de demostración en memoria.
	catalogo := models.NuevaFuenteDemo()
	log.Warn("fuente de datos de DEMOSTRACIÓN en memoria: aún no hay persistencia")

	app := controllers.Nueva(vistas, catalogo, modo, log)
	servidor := &http.Server{
		Addr:    net.JoinHostPort(ip, puerto),
		Handler: app.Manejador(),
		// Plazos que impiden que conexiones lentas o abandonadas retengan
		// recursos. Se revisarán al implementar la subida de archivos grandes.
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

	errServidor := make(chan error, 1)
	go func() {
		log.Info("fimnme-web escuchando", "direccion", servidor.Addr, "entorno", modo, "version", version)
		errServidor <- servidor.ListenAndServe()
	}()

	select {
	case err := <-errServidor:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("el servidor se detuvo", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		log.Info("señal de apagado recibida; terminando peticiones en curso")
		ctxApagado, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		if err := servidor.Shutdown(ctxApagado); err != nil {
			log.Error("apagado forzado", "error", err)
			os.Exit(1)
		}
	}
	log.Info("fimnme-web detenido")
}
