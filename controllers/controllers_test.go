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
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// repoDePrueba crea una base SQLite temporal, migrada, con la edición 49 y sus
// dos convocatorias: Intérpretes abierta y Compositores publicada.
func repoDePrueba(t *testing.T) *models.Repositorio {
	t.Helper()
	ctx := context.Background()
	actor := models.Actor{Perfil: "cli", Agente: "pruebas"}
	repo, err := models.AbrirRepositorio(t.TempDir())
	if err != nil {
		t.Fatalf("abrir repositorio: %v", err)
	}
	t.Cleanup(func() { repo.Cerrar() })
	if _, err := repo.Migrar(ctx, actor); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	edicion := models.Edicion{Numero: 49, Anio: 2027, NombreES: "Edición 49 · 2027", NombreEN: "49th Edition · 2027"}
	if edicion.ID, err = repo.CrearEdicion(ctx, actor, edicion); err != nil {
		t.Fatal(err)
	}
	// Cierre: 29 de enero de 2027 a las 23:59 en la Ciudad de México.
	etapas := []models.Etapa{{
		Clave:     models.EtapaPostulacion,
		IniciaAt:  time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC),
		TerminaAt: time.Date(2027, 1, 30, 5, 59, 0, 0, time.UTC),
	}}
	for _, c := range []models.Convocatoria{
		{Edicion: edicion, Tipo: models.TipoInterpretes, TituloES: "Convocatoria de Intérpretes",
			TituloEN: "Call for Performers", BasesMDES: "# A quién se dirige\n\nA **intérpretes**.",
			Estado: models.EstadoPostulacion, MaxVideos: 3, Etapas: etapas},
		{Edicion: edicion, Tipo: models.TipoCompositores, TituloES: "Convocatoria de Compositores",
			TituloEN: "Call for Composers", BasesMDES: "# Bases", BasesMDEN: "# Terms",
			Estado: models.EstadoPublicada, MaxObras: 2, MaxVideos: 3, Etapas: etapas},
	} {
		if _, err := repo.CrearConvocatoria(ctx, actor, c); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// appSobre arma la aplicación completa (rutas + middleware) sobre una fuente
// de datos, con el log silenciado.
func appSobre(t *testing.T, datos models.Persistencia) http.Handler {
	t.Helper()
	vistas, err := views.Nuevo()
	if err != nil {
		t.Fatalf("no se pudo cargar el frontal: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return Nueva(Config{Vistas: vistas, Datos: datos, Entorno: EntornoDesarrollo, Log: log,
		URLBase: "https://postula.ejemplo.mx"}).Manejador()
}

// appDePrueba arma la aplicación sobre una base SQLite real y temporal.
func appDePrueba(t *testing.T) http.Handler {
	t.Helper()
	return appSobre(t, repoDePrueba(t))
}

// datosCaidos simula una base de datos que no responde. Incrusta la interfaz
// para heredar su conjunto de métodos y redefine sólo los que estas pruebas
// usan; llamar a cualquier otro provocaría un pánico, que es lo deseable.
type datosCaidos struct{ models.Persistencia }

var errBaseCaida = errors.New("base caída")

func (datosCaidos) ConvocatoriasPublicas(context.Context) ([]models.Convocatoria, error) {
	return nil, errBaseCaida
}

func (datosCaidos) ConvocatoriaPublica(context.Context, int, models.TipoConvocatoria) (models.Convocatoria, error) {
	return models.Convocatoria{}, errBaseCaida
}

func (datosCaidos) Verificar(context.Context) error { return errBaseCaida }

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

// Un cuerpo mayor al límite no debe procesarse como formulario válido.
func TestCuerpoDesmesuradoNoSeAcepta(t *testing.T) {
	app := appDePrueba(t)
	enorme := url.Values{"email": {"ana@ejemplo.mx"}, "relleno": {strings.Repeat("x", cuerpoMaximo+1)}}
	w := pedir(t, app, formulario("/cuenta/recuperar", enorme))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("un cuerpo de más de 1 MiB no debe aceptarse: estado %d, se esperaba 422", w.Code)
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

// Las páginas públicas salen de la base: bases en Markdown, calendario en hora
// de México y respaldo en español cuando falta el texto en inglés.
func TestConvocatoriaLeidaDeLaBase(t *testing.T) {
	app := appDePrueba(t)
	w := pedir(t, app, httptest.NewRequest(http.MethodGet, "/convocatorias/49/interpretes", nil))
	cuerpo := w.Body.String()
	for _, fragmento := range []string{
		"<h2>A quién se dirige</h2>",
		"<strong>intérpretes</strong>",
		"Postulación abierta",
		"29 de enero de 2027, 23:59 h (hora de la Ciudad de México)",
		`datetime="2027-01-30T05:59:00Z"`,
	} {
		if !strings.Contains(cuerpo, fragmento) {
			t.Errorf("no aparece %q", fragmento)
		}
	}
	// En inglés, Intérpretes no tiene bases traducidas: se avisa y se muestran en español.
	r := httptest.NewRequest(http.MethodGet, "/convocatorias/49/interpretes", nil)
	r.Header.Set("Accept-Language", "en")
	cuerpo = pedir(t, app, r).Body.String()
	for _, fragmento := range []string{"Call for Performers", "only available in Spanish", `<div lang="es">`} {
		if !strings.Contains(cuerpo, fragmento) {
			t.Errorf("en inglés no aparece %q", fragmento)
		}
	}
}

// /healthz dice que el proceso vive; /readyz, que además la base responde.
func TestSondasConLaBaseCaida(t *testing.T) {
	app := appSobre(t, datosCaidos{})
	if w := pedir(t, app, httptest.NewRequest(http.MethodGet, "/healthz", nil)); w.Code != http.StatusOK {
		t.Errorf("/healthz no debe depender de la base: estado %d", w.Code)
	}
	if w := pedir(t, app, httptest.NewRequest(http.MethodGet, "/readyz", nil)); w.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz con la base caída: estado %d, se esperaba 503", w.Code)
	}
	// Un fallo de la base se presenta como error 500, sin filtrar el detalle.
	for _, ruta := range []string{"/", "/convocatorias/49/interpretes"} {
		w := pedir(t, app, httptest.NewRequest(http.MethodGet, ruta, nil))
		if w.Code != http.StatusInternalServerError {
			t.Errorf("GET %s con la base caída: estado %d, se esperaba 500", ruta, w.Code)
		}
		if strings.Contains(w.Body.String(), errBaseCaida.Error()) {
			t.Errorf("GET %s: el detalle interno del error llegó al visitante", ruta)
		}
	}
}

// La descripción OpenAPI es el contrato público de la API: debe describir
// exactamente las operaciones que el servidor atiende, ni una más ni una menos.
func TestOpenAPICoincideConLasRutas(t *testing.T) {
	documento, err := views.OpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	var openapi struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(documento, &openapi); err != nil {
		t.Fatalf("openapi.json no es JSON válido: %v", err)
	}
	if !strings.HasPrefix(openapi.OpenAPI, "3.1") {
		t.Errorf("versión de OpenAPI inesperada: %q", openapi.OpenAPI)
	}
	descritas := map[string]bool{}
	for ruta, operaciones := range openapi.Paths {
		for metodo := range operaciones {
			descritas[strings.ToUpper(metodo)+" "+ruta] = true
		}
	}
	vistas, err := views.Nuevo()
	if err != nil {
		t.Fatal(err)
	}
	app := Nueva(Config{Vistas: vistas, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for _, ruta := range app.rutasAPI() {
		operacion := ruta.metodo + " " + ruta.patron
		if !descritas[operacion] {
			t.Errorf("la operación %s existe en el servidor y falta en openapi.json", operacion)
		}
		delete(descritas, operacion)
	}
	for operacion := range descritas {
		t.Errorf("openapi.json describe %s, que el servidor no atiende", operacion)
	}
	// Y el documento se sirve tal cual, sin sesión.
	w := pedir(t, appDePrueba(t), httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "application/json") || w.Body.Len() != len(documento) {
		t.Errorf("GET /api/v1/openapi.json: estado %d, %d bytes", w.Code, w.Body.Len())
	}
}
