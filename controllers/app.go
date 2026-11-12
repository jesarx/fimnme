// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

// Package controllers contiene el Controlador de la arquitectura MVC:
// enrutamiento, orquestación de peticiones y control de acceso. No contiene
// reglas de negocio (models/rules.go) ni consultas SQL (models/repository.go).
package controllers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Entornos de ejecución. En desarrollo se muestra una franja de advertencia y
// se pide a los buscadores que no indexen el sitio.
const (
	EntornoDesarrollo = "desarrollo"
	EntornoProduccion = "produccion"
)

// Catalogo es lo que las páginas públicas necesitan saber de las
// convocatorias. La interfaz se define aquí, del lado de quien la consume: hoy
// la satisface models.FuenteDemo y mañana el repositorio SQLite, sin tocar los
// controladores.
type Catalogo interface {
	// ConvocatoriasPublicas devuelve las convocatorias vigentes a anunciar.
	ConvocatoriasPublicas(ctx context.Context) ([]models.Convocatoria, error)
	// ConvocatoriaPublica devuelve una convocatoria por número de edición y
	// tipo, o models.ErrNoEncontrado si no existe o está en borrador.
	ConvocatoriaPublica(ctx context.Context, numeroEdicion int, tipo models.TipoConvocatoria) (models.Convocatoria, error)
}

// App reúne las dependencias de los controladores. Se construye una vez al
// arrancar y después sólo se lee, por lo que es segura entre peticiones.
type App struct {
	vistas   *views.Motor
	catalogo Catalogo
	entorno  string
	log      *slog.Logger
}

// Nueva construye la aplicación con sus dependencias explícitas.
func Nueva(vistas *views.Motor, catalogo Catalogo, entorno string, log *slog.Logger) *App {
	return &App{vistas: vistas, catalogo: catalogo, entorno: entorno, log: log}
}

// Manejador devuelve el http.Handler completo: las rutas envueltas en la
// cadena de middleware. El enrutador es http.ServeMux de la biblioteca
// estándar, que desde Go 1.22 admite método y variables de ruta.
func (a *App) Manejador() http.Handler {
	mux := http.NewServeMux()

	// Páginas públicas.
	mux.HandleFunc("GET /{$}", a.inicio)
	mux.HandleFunc("GET /convocatorias/{edicion}/{tipo}", a.convocatoria)
	mux.HandleFunc("GET /aviso-de-privacidad", a.avisoPrivacidad)

	// Cuenta (RF-AUT-01, 03 y 04).
	mux.HandleFunc("GET /cuenta/ingresar", a.ingresarFormulario)
	mux.HandleFunc("POST /cuenta/ingresar", a.ingresar)
	mux.HandleFunc("GET /cuenta/registro", a.registroFormulario)
	mux.HandleFunc("POST /cuenta/registro", a.registro)
	mux.HandleFunc("GET /cuenta/recuperar", a.recuperarFormulario)
	mux.HandleFunc("POST /cuenta/recuperar", a.recuperar)

	// Preferencia de idioma (RF-I18N-03).
	mux.HandleFunc("POST /idioma", a.cambiarIdioma)

	// Estáticos incrustados y sondas de vida para el proxy y systemd.
	mux.Handle("GET /static/", a.vistas.Estaticos())
	mux.HandleFunc("GET /healthz", a.salud)
	mux.HandleFunc("GET /readyz", a.salud)

	// Cualquier otra ruta: página 404 propia, en el idioma del visitante.
	mux.HandleFunc("/", a.noEncontrado)

	// El orden importa: el primero de la lista es el más externo.
	return encadenar(mux,
		a.recuperarPanico,
		a.registrarPeticion,
		a.cabecerasSeguridad,
		a.protegerOrigen,
		limitarCuerpo,
		resolverIdioma,
	)
}

// salud responde a las sondas /healthz y /readyz. Cuando exista la base de
// datos, /readyz comprobará además que responde.
func (a *App) salud(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok\n"))
}
