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

package controllers

import (
	"context"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
)

// El middleware es composición de http.Handler a mano: sin framework.

// middleware envuelve un manejador con comportamiento adicional.
type middleware func(http.Handler) http.Handler

// encadenar aplica los middleware de modo que el primero de la lista sea el
// más externo (el primero en ver la petición y el último en ver la respuesta).
func encadenar(h http.Handler, capas ...middleware) http.Handler {
	for i := len(capas) - 1; i >= 0; i-- {
		h = capas[i](h)
	}
	return h
}

// cuerpoMaximo acota el tamaño de los formularios. Las subidas de archivos
// tienen su propio límite, el de cada requisito documental: véase
// cuerpoDeSubida.
const cuerpoMaximo = 1 << 20 // 1 MiB

// politicaContenido es la CSP estricta (RNF-10): todo recurso sale del propio
// origen y no se permite código ni estilo en línea. frame-src y media-src
// admiten el propio origen para el visor de documentos (RF-DOC-08): un PDF en
// un marco y un audio con su reproductor, servidos por la aplicación.
const politicaContenido = "default-src 'none'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-src 'self'; media-src 'self'; " +
	"form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// politicaDeArchivo es la CSP de un documento del acervo servido en el
// navegador. No admite ningún recurso ni código propio del archivo; sólo
// deja que las páginas de la plataforma lo muestren en un marco
// (frame-ancestors 'self'), y que el visor de PDF del navegador lo incruste
// (object-src 'self').
const politicaDeArchivo = "default-src 'none'; object-src 'self'; frame-ancestors 'self'"

// respuesta envuelve http.ResponseWriter para conocer el estado que se envió.
type respuesta struct {
	http.ResponseWriter
	estado int
}

func (r *respuesta) WriteHeader(estado int) {
	if r.estado == 0 {
		r.estado = estado
	}
	r.ResponseWriter.WriteHeader(estado)
}

func (r *respuesta) Write(datos []byte) (int, error) {
	if r.estado == 0 {
		r.estado = http.StatusOK
	}
	return r.ResponseWriter.Write(datos)
}

// Unwrap permite a http.ResponseController alcanzar el escritor original.
func (r *respuesta) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// recuperarPanico convierte un pánico en una página 500 y lo deja en el log,
// para que un error de programación no tumbe el servidor.
func (a *App) recuperarPanico(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if causa := recover(); causa != nil {
				a.log.Error("pánico atendiendo petición",
					"ruta", r.URL.Path, "causa", causa, "pila", string(debug.Stack()))
				a.errorPagina(w, r, http.StatusInternalServerError)
			}
		}()
		siguiente.ServeHTTP(w, r)
	})
}

// registrarPeticion escribe una línea de log estructurado por petición. Se
// registra la ruta pero NUNCA la cadena de consulta, que puede llevar tokens
// de verificación o de recuperación de contraseña.
func (a *App) registrarPeticion(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		resp := &respuesta{ResponseWriter: w}
		siguiente.ServeHTTP(resp, r)
		// Las sondas de vida se omiten para no ahogar el log.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return
		}
		a.log.Info("petición",
			"metodo", r.Method,
			"ruta", r.URL.Path,
			"estado", resp.estado,
			"ms", time.Since(inicio).Milliseconds(),
			"ip", ipCliente(r),
		)
	})
}

// cabecerasSeguridad añade las cabeceras de endurecimiento a toda respuesta.
func (a *App) cabecerasSeguridad(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := w.Header()
		c.Set("Content-Security-Policy", politicaContenido)
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		// same-origin: un folio en la URL no se filtra a sitios externos.
		c.Set("Referrer-Policy", "same-origin")
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		c.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		if esHTTPS(r) {
			// HSTS sin includeSubDomains: el dominio puede alojar otros
			// servicios de la institución que no controlamos.
			c.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if a.entorno == EntornoDesarrollo {
			c.Set("X-Robots-Tag", "noindex, nofollow")
		}
		siguiente.ServeHTTP(w, r)
	})
}

// protegerOrigen rechaza las peticiones que modifican estado (POST, etc.)
// cuando provienen de otro sitio: protección CSRF de la biblioteca estándar
// (http.CrossOriginProtection, Go 1.25+), basada en las cabeceras
// Sec-Fetch-Site y Origin que envía todo navegador vigente. No requiere
// tokens ocultos en los formularios.
func (a *App) protegerOrigen(siguiente http.Handler) http.Handler {
	proteccion := http.NewCrossOriginProtection()
	proteccion.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.log.Warn("petición de otro origen rechazada", "ruta", r.URL.Path, "ip", ipCliente(r))
		a.errorPagina(w, r, http.StatusForbidden)
	}))
	return proteccion.Handler(siguiente)
}

// claveCuerpoOriginal es la llave privada, en el contexto de la petición, del
// cuerpo tal como llegó, antes de acotarlo.
type claveCuerpoOriginal struct{}

// limitarCuerpo impide que un cuerpo desmesurado agote la memoria: ninguna
// petición puede leer más de cuerpoMaximo.
//
// La única excepción son las subidas de archivos, que miden hasta 100 MB y
// tienen el límite de su requisito. Para ellas se guarda el cuerpo original
// en el contexto, y sólo cuerpoDeSubida —con un límite explícito— lo
// recupera. Envolver un límite con otro más amplio no lo amplía: el de 1 MiB
// seguía mandando, y toda partitura de más de 1 MiB se rechazaba.
func limitarCuerpo(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), claveCuerpoOriginal{}, r.Body)
		r = r.WithContext(ctx)
		r.Body = http.MaxBytesReader(w, r.Body, cuerpoMaximo)
		siguiente.ServeHTTP(w, r)
	})
}

// cuerpoDeSubida devuelve el cuerpo original de la petición, acotado a
// tamanoMax en lugar de a cuerpoMaximo. Sólo lo usa abrirSubida, que ya sabe
// cuánto puede medir el archivo que espera.
func cuerpoDeSubida(w http.ResponseWriter, r *http.Request, tamanoMax int64) io.ReadCloser {
	original, ok := r.Context().Value(claveCuerpoOriginal{}).(io.ReadCloser)
	if !ok {
		original = r.Body // sin el middleware (no ocurre en el servidor)
	}
	return http.MaxBytesReader(w, original, tamanoMax)
}

// --- Idioma ---------------------------------------------------------------

// claveIdioma es la llave privada del idioma en el contexto de la petición.
type claveIdioma struct{}

// cookieIdioma guarda el idioma elegido con el selector de la cabecera.
const cookieIdioma = "fimnme_idioma"

// resolverIdioma determina el idioma de la petición (RF-I18N-03):
// preferencia de la cuenta > selector explícito (cookie) > cabecera
// Accept-Language > español. Por eso corre después de cargarSesion.
func resolverIdioma(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// La página depende de la cookie y de la cabecera: un caché intermedio
		// no debe servir la versión en inglés a quien pidió español.
		w.Header().Add("Vary", "Cookie, Accept-Language")
		ctx := context.WithValue(r.Context(), claveIdioma{}, idiomaPreferido(r))
		siguiente.ServeHTTP(w, r.WithContext(ctx))
	})
}

// idiomaPreferido aplica el orden de precedencia sobre una petición.
func idiomaPreferido(r *http.Request) string {
	if s := sesionDe(r); s != nil && models.IdiomaValido(s.Usuario.Idioma) {
		return s.Usuario.Idioma
	}
	if c, err := r.Cookie(cookieIdioma); err == nil && models.IdiomaValido(c.Value) {
		return c.Value
	}
	if preferido := idiomaDeCabecera(r.Header.Get("Accept-Language")); preferido != "" {
		return preferido
	}
	return models.IdiomaES
}

// idiomaDe devuelve el idioma resuelto para la petición. Si el middleware aún
// no corrió (p. ej. al pintar un error desde una capa más externa) lo calcula
// en el momento, para que también esas páginas salgan en el idioma correcto.
func idiomaDe(r *http.Request) string {
	if idioma, ok := r.Context().Value(claveIdioma{}).(string); ok {
		return idioma
	}
	return idiomaPreferido(r)
}

// idiomaDeCabecera elige, entre los idiomas de la plataforma, el de mayor
// preferencia (q) en una cabecera Accept-Language como "en-US,en;q=0.9,es;q=0.8".
// Devuelve "" si el visitante no pide ninguno de los dos.
func idiomaDeCabecera(cabecera string) string {
	mejor, mejorQ := "", 0.0
	for _, parte := range strings.Split(cabecera, ",") {
		etiqueta, parametro, _ := strings.Cut(strings.TrimSpace(parte), ";")
		q := 1.0
		if valor, ok := strings.CutPrefix(strings.TrimSpace(parametro), "q="); ok {
			var err error
			if q, err = strconv.ParseFloat(valor, 64); err != nil {
				continue
			}
		}
		// "es-MX" y "es-419" cuentan como "es".
		base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(etiqueta)), "-")
		if models.IdiomaValido(base) && q > mejorQ {
			mejor, mejorQ = base, q
		}
	}
	return mejor
}

// --- Datos de la conexión ---------------------------------------------------

// esLocal indica si la conexión TCP viene de esta misma máquina, es decir,
// del proxy inverso. Sólo entonces son de fiar las cabeceras X-Forwarded-*.
func esLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// esHTTPS indica si el visitante llegó por TLS, directamente o a través del
// proxy inverso (que lo declara en X-Forwarded-Proto).
func esHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return esLocal(r) && r.Header.Get("X-Forwarded-Proto") == "https"
}

// ipCliente devuelve la IP del visitante para el log y, más adelante, para la
// bitácora y el límite de tasa. Tras el proxy se toma el ÚLTIMO valor de
// X-Forwarded-For, que es el que añadió nuestro proxy; los anteriores los
// controla el cliente y podrían estar falsificados.
func ipCliente(r *http.Request) string {
	if esLocal(r) {
		if reenviadas := r.Header.Get("X-Forwarded-For"); reenviadas != "" {
			partes := strings.Split(reenviadas, ",")
			if ip := net.ParseIP(strings.TrimSpace(partes[len(partes)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
