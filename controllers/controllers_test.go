// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package controllers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// appDePrueba arma la aplicación completa (rutas + middleware) sobre los datos
// de demostración, con el log silenciado.
func appDePrueba(t *testing.T) http.Handler {
	t.Helper()
	vistas, err := views.Nuevo()
	if err != nil {
		t.Fatalf("no se pudo cargar el frontal: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return Nueva(vistas, models.NuevaFuenteDemo(), EntornoDesarrollo, log).Manejador()
}

// pedir ejecuta una petición contra la aplicación y devuelve la respuesta.
func pedir(t *testing.T, app http.Handler, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

// formulario construye un POST como el que envía un navegador desde el propio
// sitio (Sec-Fetch-Site: same-origin).
func formulario(ruta string, valores url.Values) *http.Request {
	r := httptest.NewRequest(http.MethodPost, ruta, strings.NewReader(valores.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return r
}

func TestRutasPublicas(t *testing.T) {
	app := appDePrueba(t)
	casos := []struct {
		ruta   string
		estado int
		texto  string // fragmento que debe aparecer en la página
	}{
		{"/", http.StatusOK, "Convocatorias vigentes"},
		{"/convocatorias/49/interpretes", http.StatusOK, "Convocatoria de Intérpretes"},
		{"/convocatorias/49/compositores", http.StatusOK, "Convocatoria de Compositores"},
		{"/convocatorias/50/interpretes", http.StatusNotFound, "Página no encontrada"},
		{"/convocatorias/49/directores", http.StatusNotFound, "Página no encontrada"},
		{"/convocatorias/abc/interpretes", http.StatusNotFound, "Página no encontrada"},
		{"/cuenta/ingresar", http.StatusOK, "Ingresar"},
		{"/cuenta/registro", http.StatusOK, "Crear cuenta"},
		{"/cuenta/recuperar", http.StatusOK, "Recuperar contraseña"},
		{"/aviso-de-privacidad", http.StatusOK, "Aviso de privacidad"},
		{"/no-existe", http.StatusNotFound, "Página no encontrada"},
		{"/healthz", http.StatusOK, "ok"},
		{"/readyz", http.StatusOK, "ok"},
	}
	for _, c := range casos {
		w := pedir(t, app, httptest.NewRequest(http.MethodGet, c.ruta, nil))
		if w.Code != c.estado {
			t.Errorf("GET %s: estado %d, se esperaba %d", c.ruta, w.Code, c.estado)
		}
		if !strings.Contains(w.Body.String(), c.texto) {
			t.Errorf("GET %s: no aparece %q", c.ruta, c.texto)
		}
	}
}

// RF-I18N-03: selector explícito (cookie) > Accept-Language > español.
func TestResolucionDeIdioma(t *testing.T) {
	app := appDePrueba(t)
	casos := []struct {
		nombre   string
		cabecera string
		cookie   string
		idioma   string
	}{
		{"sin preferencias: español", "", "", "es"},
		{"navegador en inglés", "en-US,en;q=0.9", "", "en"},
		{"navegador prefiere español de México", "es-MX,es;q=0.9,en;q=0.8", "", "es"},
		{"q decide el orden", "en;q=0.3,es;q=0.9", "", "es"},
		{"idioma no admitido: español", "fr-FR,fr;q=0.9", "", "es"},
		{"la cookie gana a la cabecera", "en-US", "es", "es"},
		{"cookie inválida se ignora", "en-US", "fr", "en"},
	}
	for _, c := range casos {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if c.cabecera != "" {
			r.Header.Set("Accept-Language", c.cabecera)
		}
		if c.cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieIdioma, Value: c.cookie})
		}
		w := pedir(t, app, r)
		if !strings.Contains(w.Body.String(), `<html lang="`+c.idioma+`">`) {
			t.Errorf("%s: se esperaba la página en %q", c.nombre, c.idioma)
		}
	}
}

func TestCambioDeIdiomaYRedireccionSegura(t *testing.T) {
	app := appDePrueba(t)
	casos := []struct{ volver, destino string }{
		{"/convocatorias/49/interpretes", "/convocatorias/49/interpretes"},
		{"//sitio-ajeno.example", "/"},
		{"https://sitio-ajeno.example/", "/"},
		{`/\sitio-ajeno.example`, "/"},
		{"", "/"},
	}
	for _, c := range casos {
		w := pedir(t, app, formulario("/idioma", url.Values{"idioma": {"en"}, "volver": {c.volver}}))
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != c.destino {
			t.Errorf("volver=%q: estado %d, Location %q; se esperaba 303 a %q",
				c.volver, w.Code, w.Header().Get("Location"), c.destino)
		}
		if cookie := w.Header().Get("Set-Cookie"); !strings.Contains(cookie, cookieIdioma+"=en") ||
			!strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
			t.Errorf("cookie de idioma incorrecta: %q", cookie)
		}
	}
	// Un idioma que no es de la plataforma no deja cookie.
	w := pedir(t, app, formulario("/idioma", url.Values{"idioma": {"fr"}, "volver": {"/"}}))
	if w.Header().Get("Set-Cookie") != "" {
		t.Errorf("no debe guardarse un idioma inválido: %q", w.Header().Get("Set-Cookie"))
	}
}

// Detrás del proxy con TLS la cookie debe llevar el atributo Secure.
func TestCookieSeguraDetrasDelProxy(t *testing.T) {
	app := appDePrueba(t)
	r := formulario("/idioma", url.Values{"idioma": {"en"}, "volver": {"/"}})
	r.RemoteAddr = "127.0.0.1:40000"
	r.Header.Set("X-Forwarded-Proto", "https")
	w := pedir(t, app, r)
	if !strings.Contains(w.Header().Get("Set-Cookie"), "Secure") {
		t.Errorf("falta Secure en la cookie: %q", w.Header().Get("Set-Cookie"))
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("falta la cabecera HSTS en una petición HTTPS")
	}
	// Un cliente remoto NO puede fingir HTTPS con la cabecera.
	r = formulario("/idioma", url.Values{"idioma": {"en"}, "volver": {"/"}})
	r.RemoteAddr = "203.0.113.7:40000"
	r.Header.Set("X-Forwarded-Proto", "https")
	if w := pedir(t, app, r); strings.Contains(w.Header().Get("Set-Cookie"), "Secure") {
		t.Error("X-Forwarded-Proto sólo es de fiar si viene del proxy local")
	}
}

func TestCabecerasDeSeguridad(t *testing.T) {
	app := appDePrueba(t)
	w := pedir(t, app, httptest.NewRequest(http.MethodGet, "/", nil))
	esperadas := map[string]string{
		"Content-Security-Policy": "default-src 'none'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "same-origin",
		"X-Robots-Tag":            "noindex", // entorno de desarrollo
		"Cache-Control":           "no-store",
	}
	for cabecera, fragmento := range esperadas {
		if !strings.Contains(w.Header().Get(cabecera), fragmento) {
			t.Errorf("%s = %q, debe contener %q", cabecera, w.Header().Get(cabecera), fragmento)
		}
	}
	// La CSP no admite excepciones para código o estilo en línea.
	if csp := w.Header().Get("Content-Security-Policy"); strings.Contains(csp, "unsafe") {
		t.Errorf("la CSP no debe contener 'unsafe-*': %s", csp)
	}
}

// Protección CSRF: un POST lanzado desde otro sitio se rechaza.
func TestPostDeOtroOrigenRechazado(t *testing.T) {
	app := appDePrueba(t)
	r := formulario("/idioma", url.Values{"idioma": {"en"}, "volver": {"/"}})
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := pedir(t, app, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("estado %d, se esperaba 403", w.Code)
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Error("una petición rechazada no debe dejar cookies")
	}
}

func TestRegistroDevuelveErroresPorCampo(t *testing.T) {
	app := appDePrueba(t)
	w := pedir(t, app, formulario("/cuenta/registro", url.Values{
		"email": {"correo-malo"}, "nombre": {"Ana"}, "apellidos": {""},
		"password": {"contraseña-secreta-1"}, "idioma": {"es"},
	}))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("estado %d, se esperaba 422", w.Code)
	}
	cuerpo := w.Body.String()
	for _, fragmento := range []string{
		"Escribe un correo electrónico válido",
		"Escribe tus apellidos.",
		"debes aceptar el aviso de privacidad",
		`value="correo-malo"`, // lo capturado no se pierde
		`value="Ana"`,
		`aria-invalid="true"`,
	} {
		if !strings.Contains(cuerpo, fragmento) {
			t.Errorf("no aparece %q", fragmento)
		}
	}
	// La contraseña jamás vuelve al navegador.
	if strings.Contains(cuerpo, "contraseña-secreta-1") {
		t.Error("la contraseña no debe reaparecer en la página")
	}
}

// Mientras no haya persistencia, un formulario válido responde 501 con un
// aviso explícito: nadie debe creer que quedó registrado.
func TestFormulariosValidosAvisanQueFaltaElBackend(t *testing.T) {
	app := appDePrueba(t)
	casos := map[string]url.Values{
		"/cuenta/registro": {"email": {"ana@ejemplo.mx"}, "nombre": {"Ana"}, "apellidos": {"López"},
			"password": {"una-frase-larga"}, "idioma": {"en"}, "acepta_aviso": {"1"}},
		"/cuenta/ingresar":  {"email": {"ana@ejemplo.mx"}, "password": {"x"}},
		"/cuenta/recuperar": {"email": {"ana@ejemplo.mx"}},
	}
	for ruta, valores := range casos {
		w := pedir(t, app, formulario(ruta, valores))
		if w.Code != http.StatusNotImplemented {
			t.Errorf("POST %s: estado %d, se esperaba 501", ruta, w.Code)
		}
		if !strings.Contains(w.Body.String(), "todavía no está conectada") {
			t.Errorf("POST %s: falta el aviso de función pendiente", ruta)
		}
	}
}

// Un cuerpo mayor al límite no debe procesarse como formulario válido.
func TestCuerpoDesmesuradoNoSeAcepta(t *testing.T) {
	app := appDePrueba(t)
	enorme := url.Values{"email": {"ana@ejemplo.mx"}, "relleno": {strings.Repeat("x", cuerpoMaximo+1)}}
	w := pedir(t, app, formulario("/cuenta/recuperar", enorme))
	if w.Code == http.StatusNotImplemented || w.Code == http.StatusOK {
		t.Errorf("un cuerpo de más de 1 MiB no debe aceptarse: estado %d", w.Code)
	}
}

func TestIPDelCliente(t *testing.T) {
	// Tras el proxy local se toma la última IP reenviada, la que añadió él.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 198.51.100.20")
	if ip := ipCliente(r); ip != "198.51.100.20" {
		t.Errorf("ip = %q, se esperaba 198.51.100.20", ip)
	}
	// Una conexión directa no puede falsear su IP con la cabecera.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:5000"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if ip := ipCliente(r); ip != "203.0.113.7" {
		t.Errorf("ip = %q, se esperaba 203.0.113.7", ip)
	}
}
