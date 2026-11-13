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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pruebas de extremo a extremo de las cuentas: recorren los flujos completos
// por HTTP, sobre una base SQLite real y con un reloj que la prueba adelanta
// para comprobar vencimientos y bloqueos sin esperar.

const (
	urlBasePrueba  = "https://postula.ejemplo.mx"
	passwordPrueba = "una-frase-bastante-larga"
)

// relojDePrueba es un reloj que sólo avanza cuando la prueba lo pide.
type relojDePrueba struct {
	mu sync.Mutex
	t  time.Time
}

func (r *relojDePrueba) Ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.t
}

func (r *relojDePrueba) Avanzar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.t = r.t.Add(d)
}

// entornoPrueba es la aplicación completa montada para una prueba.
type entornoPrueba struct {
	t        *testing.T
	repo     *models.Repositorio
	app      http.Handler
	reloj    *relojDePrueba
	red      *redDePrueba // la «Internet» que ve el verificador de ligas
	clientes int
}

func nuevoEntorno(t *testing.T) *entornoPrueba {
	t.Helper()
	repo := repoDePrueba(t)
	reloj := &relojDePrueba{t: time.Date(2026, 11, 12, 18, 0, 0, 0, time.UTC)}
	repo.FijarReloj(reloj.Ahora)
	vistas, err := views.Nuevo()
	if err != nil {
		t.Fatal(err)
	}
	red := &redDePrueba{estados: map[string]int{}}
	almacen, err := models.AbrirAlmacenEnDisco(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := Nueva(Config{
		Vistas: vistas, Datos: repo, Entorno: EntornoDesarrollo, URLBase: urlBasePrueba, Ahora: reloj.Ahora, Almacen: almacen,
		Enlaces: &VerificadorDeEnlaces{Cliente: &http.Client{Transport: red}},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return &entornoPrueba{t: t, repo: repo, app: app.Manejador(), reloj: reloj, red: red}
}

// cliente imita a un navegador: recuerda sus cookies y tiene su propia IP.
type cliente struct {
	t       *testing.T
	app     http.Handler
	cookies map[string]string
	ip      string
	idioma  string // cabecera Accept-Language
}

// cliente crea un navegador nuevo, cada uno desde una IP distinta para que el
// límite de intentos por IP de una prueba no afecte a otro navegador.
func (e *entornoPrueba) cliente() *cliente {
	e.clientes++
	return &cliente{t: e.t, app: e.app, cookies: map[string]string{}, ip: fmt.Sprintf("198.51.100.%d", e.clientes)}
}

func (c *cliente) hacer(r *http.Request) *httptest.ResponseRecorder {
	c.t.Helper()
	r.RemoteAddr = c.ip + ":50000"
	for nombre, valor := range c.cookies {
		r.AddCookie(&http.Cookie{Name: nombre, Value: valor})
	}
	if c.idioma != "" {
		r.Header.Set("Accept-Language", c.idioma)
	}
	w := httptest.NewRecorder()
	c.app.ServeHTTP(w, r)
	for _, cookie := range w.Result().Cookies() {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(c.cookies, cookie.Name)
		} else {
			c.cookies[cookie.Name] = cookie.Value
		}
	}
	return w
}

func (c *cliente) get(ruta string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.hacer(httptest.NewRequest(http.MethodGet, ruta, nil))
}

func (c *cliente) post(ruta string, valores url.Values) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.hacer(formulario(ruta, valores))
}

// json pide una ruta de la API y decodifica su respuesta.
func (c *cliente) json(ruta string) (int, map[string]any) {
	c.t.Helper()
	w := c.get(ruta)
	var cuerpo map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &cuerpo); err != nil {
		c.t.Fatalf("GET %s no devolvió JSON (%d): %s", ruta, w.Code, w.Body.String())
	}
	return w.Code, cuerpo
}

// ingresar inicia sesión y exige que funcione.
func (c *cliente) ingresar(email, password string) {
	c.t.Helper()
	w := c.post("/cuenta/ingresar", url.Values{"email": {email}, "password": {password}})
	if w.Code != http.StatusSeeOther || c.cookies[cookieSesion] == "" {
		c.t.Fatalf("no se pudo ingresar como %s: estado %d", email, w.Code)
	}
}

// esperar comprueba el estado de una respuesta y que contenga ciertos textos.
func esperar(t *testing.T, w *httptest.ResponseRecorder, estado int, fragmentos ...string) {
	t.Helper()
	if w.Code != estado {
		t.Errorf("estado %d, se esperaba %d (Location: %q)", w.Code, estado, w.Header().Get("Location"))
	}
	for _, fragmento := range fragmentos {
		if !strings.Contains(w.Body.String(), fragmento) {
			t.Errorf("no aparece %q en la respuesta", fragmento)
		}
	}
}

// redirige comprueba una redirección 303 a un destino.
func redirige(t *testing.T, w *httptest.ResponseRecorder, destino string) {
	t.Helper()
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != destino {
		t.Errorf("estado %d con Location %q; se esperaba 303 a %q", w.Code, w.Header().Get("Location"), destino)
	}
}

// crearCuenta da de alta una cuenta directamente en la base, con contraseña.
func (e *entornoPrueba) crearCuenta(email string, perfil models.Perfil) int64 {
	e.t.Helper()
	resumen, err := models.HashPassword(passwordPrueba)
	if err != nil {
		e.t.Fatal(err)
	}
	id, err := e.repo.CrearUsuario(context.Background(), models.Actor{Perfil: models.ActorCLI}, models.NuevoUsuario{
		Email: email, Nombre: "Ana", Apellidos: "López", Idioma: models.IdiomaES, PasswordHash: resumen, Perfil: perfil,
	}, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// correos devuelve los mensajes pendientes en la bandeja de salida.
func (e *entornoPrueba) correos() []models.CorreoSaliente {
	e.t.Helper()
	correos, err := e.repo.CorreosPendientes(context.Background(), 100)
	if err != nil {
		e.t.Fatal(err)
	}
	return correos
}

var tokenEnCorreo = regexp.MustCompile(`\?token=([A-Za-z0-9_-]+)`)

// tokenDe extrae el enlace de un solo uso del correo más reciente de una
// plantilla a un destinatario, como haría la persona al abrir su buzón.
func (e *entornoPrueba) tokenDe(destinatario, plantilla, ruta string) string {
	e.t.Helper()
	correos := e.correos()
	for i := len(correos) - 1; i >= 0; i-- {
		c := correos[i]
		if c.Destinatario != destinatario || c.PlantillaClave != plantilla {
			continue
		}
		if !strings.Contains(c.Texto, urlBasePrueba+ruta+"?token=") {
			e.t.Fatalf("el correo no lleva el enlace a %s: %s", ruta, c.Texto)
		}
		return tokenEnCorreo.FindStringSubmatch(c.Texto)[1]
	}
	e.t.Fatalf("no hay correo %q para %s", plantilla, destinatario)
	return ""
}

// asientos consulta la bitácora por acción.
func (e *entornoPrueba) asientos(accion string) []models.AsientoAuditoria {
	e.t.Helper()
	asientos, _, err := e.repo.ConsultarAuditoria(context.Background(),
		models.FiltroAuditoria{Accion: accion, Pagina: 1, Tamano: 200})
	if err != nil {
		e.t.Fatal(err)
	}
	return asientos
}

var datosRegistro = url.Values{
	"email": {"ana@ejemplo.mx"}, "nombre": {" Ana "}, "apellidos": {"López"},
	"password": {passwordPrueba}, "idioma": {"es"}, "acepta_aviso": {"1"},
}

// --- Registro y verificación ---------------------------------------------------

func TestRegistroVerificacionYSesion(t *testing.T) {
	e := nuevoEntorno(t)
	c := e.cliente()

	// El registro deja la sesión iniciada: se puede capturar sin esperar el correo.
	r := formulario("/cuenta/registro", datosRegistro)
	r.Header.Set("X-Forwarded-Proto", "https") // sin efecto: la petición no viene del proxy local
	w := c.hacer(r)
	redirige(t, w, "/cuenta?aviso=registro")
	cookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, cookieSesion+"=") || !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Errorf("cookie de sesión incorrecta: %q", cookie)
	}
	esperar(t, c.get("/cuenta?aviso=registro"), http.StatusOK,
		"Tu cuenta quedó creada", "aún no está confirmado", "Ana López", "ana@ejemplo.mx", "Salir")

	// La cuenta es de postulante, sin verificar, con la contraseña resumida.
	estado, sesion := c.json("/api/v1/sesion")
	usuario := sesion["usuario"].(map[string]any)
	if estado != http.StatusOK || usuario["email_verificado"] != false || usuario["nombre"] != "Ana" ||
		fmt.Sprint(sesion["perfiles"]) != "[postulante]" {
		t.Errorf("sesión inesperada tras el registro: %d %v", estado, sesion)
	}
	_, resumen, err := e.repo.UsuarioPorEmail(context.Background(), "ANA@ejemplo.mx") // sin distinguir mayúsculas
	if err != nil || !strings.HasPrefix(resumen, "$argon2id$") || strings.Contains(resumen, passwordPrueba) {
		t.Errorf("la contraseña debe guardarse como resumen Argon2id: %q (%v)", resumen, err)
	}

	// El enlace del correo lleva a una página con un botón; abrirla NO verifica.
	token := e.tokenDe("ana@ejemplo.mx", models.PlantillaVerificarEmail, "/cuenta/verificar")
	anonimo := e.cliente()
	for i := 0; i < 2; i++ { // un antivirus puede «visitar» el enlace varias veces
		esperar(t, anonimo.get("/cuenta/verificar?token="+token), http.StatusOK, "Confirmar mi correo", "ana@ejemplo.mx")
	}
	if _, sesion := c.json("/api/v1/sesion"); sesion["usuario"].(map[string]any)["email_verificado"] != false {
		t.Error("un GET no debe verificar el correo")
	}
	// El botón (POST) sí verifica, una sola vez.
	esperar(t, anonimo.post("/cuenta/verificar", url.Values{"token": {token}}), http.StatusOK, "Correo confirmado")
	if _, sesion := c.json("/api/v1/sesion"); sesion["usuario"].(map[string]any)["email_verificado"] != true {
		t.Error("el correo debió quedar verificado")
	}
	esperar(t, anonimo.post("/cuenta/verificar", url.Values{"token": {token}}), http.StatusGone, "ya no es válido")
	esperar(t, anonimo.get("/cuenta/verificar?token="+token), http.StatusGone)
	esperar(t, anonimo.get("/cuenta/verificar?token=inventado"), http.StatusGone)

	// Un segundo registro con el mismo correo se rechaza, sin distinguir mayúsculas.
	repetido := url.Values{}
	for k, v := range datosRegistro {
		repetido[k] = v
	}
	repetido.Set("email", "Ana@Ejemplo.MX")
	esperar(t, e.cliente().post("/cuenta/registro", repetido), http.StatusUnprocessableEntity, "Ya existe una cuenta con este correo")

	// Consentimiento y rastro: un alta, un ingreso y una verificación.
	for accion, n := range map[string]int{"usuario.crear": 1, "sesion.ingreso": 1, "cuenta.email_verificado": 1} {
		if obtenidos := len(e.asientos(accion)); obtenidos != n {
			t.Errorf("bitácora: %d asientos de %s, se esperaban %d", obtenidos, accion, n)
		}
	}
	if alta := e.asientos("usuario.crear")[0]; !strings.Contains(alta.Despues, `"aviso_privacidad_version":"provisional"`) ||
		alta.ActorPerfil != models.ActorAnonimo || alta.IP != c.ip {
		t.Errorf("asiento del alta incorrecto: %+v", alta)
	}
}

// --- Inicio de sesión y bloqueo --------------------------------------------------

func TestIngresoYBloqueoPorIntentos(t *testing.T) {
	e := nuevoEntorno(t)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	c := e.cliente()
	intentar := func(email, password string) *httptest.ResponseRecorder {
		return c.post("/cuenta/ingresar", url.Values{"email": {email}, "password": {password}})
	}

	// El mismo mensaje para contraseña incorrecta y para correo inexistente.
	const generico = "El correo o la contraseña no son correctos."
	esperar(t, intentar("ana@ejemplo.mx", "equivocada"), http.StatusUnauthorized, generico, `value="ana@ejemplo.mx"`)
	esperar(t, intentar("nadie@ejemplo.mx", "equivocada"), http.StatusUnauthorized, generico)
	esperar(t, intentar("ana@ejemplo.mx", ""), http.StatusUnprocessableEntity, "Escribe tu contraseña.")

	// Al quinto fallo la cuenta se bloquea, incluso para la contraseña correcta.
	for i := 0; i < 4; i++ {
		esperar(t, intentar("ana@ejemplo.mx", "equivocada"), http.StatusUnauthorized)
	}
	esperar(t, intentar("ana@ejemplo.mx", passwordPrueba), http.StatusTooManyRequests, "Demasiados intentos fallidos")
	if c.cookies[cookieSesion] != "" {
		t.Fatal("una cuenta bloqueada no debe abrir sesión")
	}
	// Pasado el bloqueo (1 minuto al quinto fallo) se puede entrar.
	e.reloj.Avanzar(61 * time.Second)
	redirige(t, intentar("ana@ejemplo.mx", passwordPrueba), "/cuenta")
	// El ingreso correcto reinicia la cuenta de fallos.
	otro := e.cliente()
	esperar(t, otro.post("/cuenta/ingresar", url.Values{"email": {"ana@ejemplo.mx"}, "password": {"equivocada"}}),
		http.StatusUnauthorized, generico)

	// Todos los intentos quedaron registrados, con su IP (RF-AUT-08).
	if n := len(e.asientos("sesion.ingreso_fallido")); n != 7 {
		t.Errorf("%d intentos fallidos registrados, se esperaban 7", n)
	}
	if n := len(e.asientos("sesion.ingreso_bloqueado")); n != 1 {
		t.Errorf("%d intentos bloqueados registrados, se esperaba 1", n)
	}
	if ingreso := e.asientos("sesion.ingreso"); len(ingreso) != 1 || ingreso[0].IP != c.ip || ingreso[0].ActorID == 0 {
		t.Errorf("asiento del ingreso incorrecto: %+v", ingreso)
	}
}

func TestSalirYPaginasProtegidas(t *testing.T) {
	e := nuevoEntorno(t)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	c := e.cliente()

	// Sin sesión, la página protegida lleva a ingresar y recuerda a dónde se iba.
	redirige(t, c.get("/cuenta"), "/cuenta/ingresar?aviso=ingresa&volver=%2Fcuenta")
	esperar(t, c.get("/cuenta/ingresar?aviso=ingresa&volver=%2Fcuenta"), http.StatusOK, "Ingresa para continuar", `name="volver" value="/cuenta"`)
	redirige(t, c.post("/cuenta/datos", url.Values{"nombre": {"X"}}), "/cuenta/ingresar?aviso=ingresa")
	// El destino tras ingresar sólo puede ser una ruta de este sitio.
	w := c.post("/cuenta/ingresar", url.Values{"email": {"ana@ejemplo.mx"}, "password": {passwordPrueba}, "volver": {"//sitio-ajeno.example"}})
	redirige(t, w, "/cuenta")

	esperar(t, c.get("/cuenta"), http.StatusOK, "Mi cuenta")
	redirige(t, c.get("/cuenta/ingresar"), "/cuenta") // ya con sesión, no se pide ingresar otra vez
	valor := c.cookies[cookieSesion]
	redirige(t, c.post("/cuenta/salir", nil), "/cuenta/ingresar?aviso=salida")
	if c.cookies[cookieSesion] != "" {
		t.Error("la cookie de sesión debió borrarse")
	}
	// La sesión dejó de existir en el servidor: la cookie vieja ya no sirve.
	c.cookies[cookieSesion] = valor
	if estado, _ := c.json("/api/v1/sesion"); estado != http.StatusUnauthorized {
		t.Errorf("una sesión cerrada siguió siendo válida: estado %d", estado)
	}
	if n := len(e.asientos("sesion.salida")); n != 1 {
		t.Errorf("%d asientos de salida, se esperaba 1", n)
	}
}

// --- Recuperación de contraseña --------------------------------------------------

func TestRecuperacionDePassword(t *testing.T) {
	e := nuevoEntorno(t)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	abierta := e.cliente() // una sesión que estaba abierta en otro dispositivo
	abierta.ingresar("ana@ejemplo.mx", passwordPrueba)
	c := e.cliente()
	pedir := func(email string) *httptest.ResponseRecorder {
		return c.post("/cuenta/recuperar", url.Values{"email": {email}})
	}

	// La respuesta es idéntica exista o no la cuenta.
	inexistente := pedir("nadie@ejemplo.mx")
	esperar(t, inexistente, http.StatusOK, "Revisa tu correo")
	if n := len(e.correos()); n != 0 {
		t.Fatalf("no debe enviarse correo a una cuenta inexistente: %d", n)
	}
	existente := pedir("ana@ejemplo.mx")
	if existente.Code != inexistente.Code || existente.Body.String() != inexistente.Body.String() {
		t.Error("la respuesta delata si el correo tiene cuenta")
	}
	// Pedirlo de nuevo enseguida no genera otro correo: nadie inunda un buzón ajeno.
	pedir("ana@ejemplo.mx")
	if n := len(e.correos()); n != 1 {
		t.Fatalf("se esperaba un solo correo de recuperación, hay %d", n)
	}

	token := e.tokenDe("ana@ejemplo.mx", models.PlantillaResetPassword, "/cuenta/restablecer")
	esperar(t, c.get("/cuenta/restablecer?token="+token), http.StatusOK, "Elige una contraseña nueva", "ana@ejemplo.mx")
	// Un enlace de recuperación no sirve para otra cosa.
	esperar(t, c.get("/cuenta/activar?token="+token), http.StatusGone)
	esperar(t, c.post("/cuenta/verificar", url.Values{"token": {token}}), http.StatusGone)
	// La contraseña nueva pasa por la misma política.
	esperar(t, c.post("/cuenta/restablecer", url.Values{"token": {token}, "password": {"corta"}}),
		http.StatusUnprocessableEntity, "al menos 10 caracteres")

	const nueva = "otra-frase-todavia-mas-larga"
	redirige(t, c.post("/cuenta/restablecer", url.Values{"token": {token}, "password": {nueva}}),
		"/cuenta/ingresar?aviso=password_restablecida")
	// El enlace es de un solo uso.
	esperar(t, c.post("/cuenta/restablecer", url.Values{"token": {token}, "password": {nueva}}), http.StatusGone)
	// La contraseña anterior ya no sirve, la nueva sí, y las sesiones abiertas se cerraron.
	esperar(t, c.post("/cuenta/ingresar", url.Values{"email": {"ana@ejemplo.mx"}, "password": {passwordPrueba}}), http.StatusUnauthorized)
	c.ingresar("ana@ejemplo.mx", nueva)
	if estado, _ := abierta.json("/api/v1/sesion"); estado != http.StatusUnauthorized {
		t.Errorf("restablecer la contraseña debió cerrar las sesiones abiertas: estado %d", estado)
	}
	// Abrir el enlace demostró control del buzón: el correo queda verificado.
	if _, sesion := c.json("/api/v1/sesion"); sesion["usuario"].(map[string]any)["email_verificado"] != true {
		t.Error("restablecer por correo debió verificar la dirección")
	}

	// Un enlace vence a la hora.
	e.reloj.Avanzar(6 * time.Minute)
	pedir("ana@ejemplo.mx")
	vencido := e.tokenDe("ana@ejemplo.mx", models.PlantillaResetPassword, "/cuenta/restablecer")
	e.reloj.Avanzar(61 * time.Minute)
	esperar(t, c.get("/cuenta/restablecer?token="+vencido), http.StatusGone, "ya no es válido")
	esperar(t, c.post("/cuenta/restablecer", url.Values{"token": {vencido}, "password": {nueva}}), http.StatusGone)
}

// --- Mi cuenta ---------------------------------------------------------------------

func TestMiCuenta(t *testing.T) {
	e := nuevoEntorno(t)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	e.crearCuenta("otra@ejemplo.mx", models.PerfilPostulante)
	c, otroDispositivo := e.cliente(), e.cliente()
	c.ingresar("ana@ejemplo.mx", passwordPrueba)
	otroDispositivo.ingresar("ana@ejemplo.mx", passwordPrueba)

	// Datos personales e idioma. La preferencia de la cuenta manda sobre el navegador.
	esperar(t, c.post("/cuenta/datos", url.Values{"nombre": {""}, "apellidos": {"López"}, "idioma": {"es"}}),
		http.StatusUnprocessableEntity, "Escribe tu nombre.")
	redirige(t, c.post("/cuenta/datos", url.Values{"nombre": {"Ana María"}, "apellidos": {"López Ruiz"}, "idioma": {"en"}}),
		"/cuenta?aviso=datos")
	c.idioma = "es-MX"
	esperar(t, c.get("/cuenta?aviso=datos"), http.StatusOK, `<html lang="en">`, "Your details have been saved.", "Ana María López Ruiz")
	// El selector de idioma de la cabecera también cambia la preferencia de la cuenta.
	redirige(t, c.post("/idioma", url.Values{"idioma": {"es"}, "volver": {"/cuenta"}}), "/cuenta")
	if _, sesion := c.json("/api/v1/sesion"); sesion["usuario"].(map[string]any)["idioma"] != "es" {
		t.Error("el selector de idioma debió guardar la preferencia en la cuenta")
	}

	// Contraseña: exige la actual y cierra las demás sesiones.
	const nueva = "otra-frase-todavia-mas-larga"
	esperar(t, c.post("/cuenta/password", url.Values{"password_actual": {"equivocada"}, "password_nueva": {nueva}}),
		http.StatusUnprocessableEntity, "La contraseña actual no es correcta.")
	esperar(t, c.post("/cuenta/password", url.Values{"password_actual": {passwordPrueba}, "password_nueva": {"corta"}}),
		http.StatusUnprocessableEntity, "al menos 10 caracteres")
	redirige(t, c.post("/cuenta/password", url.Values{"password_actual": {passwordPrueba}, "password_nueva": {nueva}}),
		"/cuenta?aviso=password")
	if estado, _ := c.json("/api/v1/sesion"); estado != http.StatusOK {
		t.Error("la sesión que cambió la contraseña debe conservarse")
	}
	if estado, _ := otroDispositivo.json("/api/v1/sesion"); estado != http.StatusUnauthorized {
		t.Error("cambiar la contraseña debió cerrar las demás sesiones")
	}

	// Correo: exige la contraseña, no admite el mismo ni uno ajeno, y obliga a verificar de nuevo.
	cambiar := func(email, password string) *httptest.ResponseRecorder {
		return c.post("/cuenta/email", url.Values{"email_nuevo": {email}, "password_email": {password}})
	}
	esperar(t, cambiar("nuevo@ejemplo.mx", "equivocada"), http.StatusUnprocessableEntity, "La contraseña actual no es correcta.")
	esperar(t, cambiar("no-es-correo", nueva), http.StatusUnprocessableEntity, "correo electrónico válido")
	esperar(t, cambiar("ANA@ejemplo.mx", nueva), http.StatusUnprocessableEntity, "Ese ya es el correo de tu cuenta.")
	esperar(t, cambiar("otra@ejemplo.mx", nueva), http.StatusUnprocessableEntity, "Ya existe una cuenta con este correo")
	redirige(t, cambiar("nuevo@ejemplo.mx", nueva), "/cuenta?aviso=email")
	esperar(t, c.get("/cuenta"), http.StatusOK, "nuevo@ejemplo.mx", "sin confirmar")
	// Se avisa a la dirección anterior y se pide confirmar la nueva.
	var aviso models.CorreoSaliente
	for _, correo := range e.correos() {
		if correo.PlantillaClave == models.PlantillaCambioEmail {
			aviso = correo
		}
	}
	if aviso.Destinatario != "ana@ejemplo.mx" || !strings.Contains(aviso.Texto, "nuevo@ejemplo.mx") {
		t.Errorf("falta el aviso a la dirección anterior: %+v", aviso)
	}
	token := e.tokenDe("nuevo@ejemplo.mx", models.PlantillaVerificarEmail, "/cuenta/verificar")
	redirige(t, c.post("/cuenta/verificar", url.Values{"token": {token}}), "/cuenta?aviso=verificado")
	esperar(t, c.get("/cuenta"), http.StatusOK, "confirmado")
	// Reenviar la verificación ya no aplica; y con el correo nuevo se ingresa.
	redirige(t, c.post("/cuenta/verificacion", nil), "/cuenta")
	e.cliente().ingresar("nuevo@ejemplo.mx", nueva)

	// La bitácora dice QUÉ campos cambiaron, pero no guarda datos personales.
	for _, a := range append(e.asientos("cuenta.datos_actualizados"), e.asientos("cuenta.email_cambiado")...) {
		if strings.Contains(a.Despues, "@") || strings.Contains(a.Despues, "Ruiz") || !strings.Contains(a.Despues, `"campos"`) {
			t.Errorf("el asiento %s guarda datos personales o no nombra los campos: %s", a.Accion, a.Despues)
		}
	}
}

func TestReenvioDeVerificacion(t *testing.T) {
	e := nuevoEntorno(t)
	c := e.cliente()
	redirige(t, c.post("/cuenta/registro", datosRegistro), "/cuenta?aviso=registro")
	primero := e.tokenDe("ana@ejemplo.mx", models.PlantillaVerificarEmail, "/cuenta/verificar")

	// Enseguida no se reenvía; pasados unos minutos sí, y el enlace anterior deja de valer.
	redirige(t, c.post("/cuenta/verificacion", nil), "/cuenta?aviso=verificacion_reciente")
	e.reloj.Avanzar(6 * time.Minute)
	redirige(t, c.post("/cuenta/verificacion", nil), "/cuenta?aviso=verificacion_enviada")
	segundo := e.tokenDe("ana@ejemplo.mx", models.PlantillaVerificarEmail, "/cuenta/verificar")
	if primero == segundo {
		t.Fatal("el reenvío debe llevar un enlace nuevo")
	}
	esperar(t, c.post("/cuenta/verificar", url.Values{"token": {primero}}), http.StatusGone)
	redirige(t, c.post("/cuenta/verificar", url.Values{"token": {segundo}}), "/cuenta?aviso=verificado")

	// El enlace de verificación vence a las 24 horas.
	otro := e.cliente()
	nuevos := url.Values{}
	for k, v := range datosRegistro {
		nuevos[k] = v
	}
	nuevos.Set("email", "luis@ejemplo.mx")
	redirige(t, otro.post("/cuenta/registro", nuevos), "/cuenta?aviso=registro")
	vencido := e.tokenDe("luis@ejemplo.mx", models.PlantillaVerificarEmail, "/cuenta/verificar")
	e.reloj.Avanzar(25 * time.Hour)
	esperar(t, otro.post("/cuenta/verificar", url.Values{"token": {vencido}}), http.StatusGone)
}

// --- Perfiles, activación y API ---------------------------------------------------

// activarCuentaInterna crea una cuenta interna como lo hace fimnme-cli (sin
// contraseña, con enlace de activación) y la activa por HTTP.
func (e *entornoPrueba) activarCuentaInterna(email string, perfil models.Perfil) *cliente {
	e.t.Helper()
	claro, resumen, err := models.NuevoSecreto()
	if err != nil {
		e.t.Fatal(err)
	}
	_, err = e.repo.CrearUsuario(context.Background(), models.Actor{Perfil: models.ActorCLI}, models.NuevoUsuario{
		Email: email, Nombre: "Admin", Apellidos: "Sistema", Idioma: models.IdiomaES,
		PasswordHash: models.PasswordSinDefinir, Perfil: perfil,
	}, &models.NuevoToken{Resumen: resumen, Proposito: models.TokenActivacion, ExpiraAt: e.reloj.Ahora().Add(models.VigenciaActivacion)})
	if err != nil {
		e.t.Fatal(err)
	}
	c := e.cliente()
	// Antes de activarse la cuenta no tiene contraseña: nada la abre.
	esperar(e.t, c.post("/cuenta/ingresar", url.Values{"email": {email}, "password": {models.PasswordSinDefinir}}), http.StatusUnauthorized)
	esperar(e.t, c.get("/cuenta/activar?token="+claro), http.StatusOK, "Activa tu cuenta", email)
	redirige(e.t, c.post("/cuenta/activar", url.Values{"token": {claro}, "password": {passwordPrueba}}), "/cuenta/ingresar?aviso=activada")
	c.ingresar(email, passwordPrueba)
	return c
}

func TestActivacionPerfilesYAuditoria(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	observador := e.activarCuentaInterna("coordinacion@ejemplo.mx", models.PerfilCoordinacionNacional)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	postulante := e.cliente()
	postulante.ingresar("ana@ejemplo.mx", passwordPrueba)
	anonimo := e.cliente()

	// La sesión describe la cuenta y sus perfiles.
	estado, sesion := admin.json("/api/v1/sesion")
	if estado != http.StatusOK || fmt.Sprint(sesion["perfiles"]) != "[admin_sistema]" ||
		sesion["usuario"].(map[string]any)["email_verificado"] != true { // activar verifica el correo
		t.Errorf("sesión del administrador inesperada: %d %v", estado, sesion)
	}

	// Bitácora: sólo administrador y observador (matriz de permisos, §2.2).
	if estado, cuerpo := anonimo.json("/api/v1/auditoria"); estado != http.StatusUnauthorized ||
		cuerpo["error"].(map[string]any)["codigo"] != "no_autenticado" {
		t.Errorf("anónimo: estado %d, cuerpo %v", estado, cuerpo)
	}
	if estado, cuerpo := postulante.json("/api/v1/auditoria"); estado != http.StatusForbidden ||
		cuerpo["error"].(map[string]any)["codigo"] != "sin_permiso" {
		t.Errorf("postulante: estado %d, cuerpo %v", estado, cuerpo)
	}
	for nombre, c := range map[string]*cliente{"administrador": admin, "observador": observador} {
		if estado, cuerpo := c.json("/api/v1/auditoria"); estado != http.StatusOK || cuerpo["total"].(float64) < 5 {
			t.Errorf("%s: estado %d, cuerpo %v", nombre, estado, cuerpo)
		}
	}

	// Filtros y paginación.
	_, cuerpo := admin.json("/api/v1/auditoria?accion=cuenta.activada&tamano=1")
	registros := cuerpo["registros"].([]any)
	if cuerpo["total"].(float64) != 2 || len(registros) != 1 || cuerpo["tamano"].(float64) != 1 {
		t.Errorf("filtro por acción y paginación incorrectos: %v", cuerpo)
	}
	if asiento := registros[0].(map[string]any); asiento["accion"] != "cuenta.activada" || asiento["entidad"] != "usuario" ||
		asiento["creado_at"] != "2026-11-12T18:00:00Z" || asiento["actor_id"] != nil {
		t.Errorf("asiento inesperado: %v", asiento)
	}
	// «hasta» incluye el día completo en hora de México; «desde» lo excluye si es posterior.
	if _, cuerpo := admin.json("/api/v1/auditoria?accion=cuenta.activada&desde=2026-11-12&hasta=2026-11-12"); cuerpo["total"].(float64) != 2 {
		t.Errorf("filtro por día: %v", cuerpo["total"])
	}
	// Se filtra además por acción: los asientos de la siembra de la base de
	// pruebas llevan la fecha real del sistema, no la del reloj de prueba.
	if _, cuerpo := admin.json("/api/v1/auditoria?accion=cuenta.activada&desde=2026-11-13"); cuerpo["total"].(float64) != 0 {
		t.Errorf("filtro «desde» posterior: %v", cuerpo["total"])
	}
	for _, mala := range []string{"pagina=0", "tamano=1000", "desde=ayer", "entidad_id=x", "formato=xml"} {
		if estado, cuerpo := admin.json("/api/v1/auditoria?" + mala); estado != http.StatusBadRequest ||
			cuerpo["error"].(map[string]any)["codigo"] != "parametro_invalido" {
			t.Errorf("%s: estado %d, cuerpo %v", mala, estado, cuerpo)
		}
	}
	// Los errores de la API también hablan el idioma de la petición.
	anonimo.idioma = "en"
	if _, cuerpo := anonimo.json("/api/v1/sesion"); cuerpo["error"].(map[string]any)["mensaje"] != "You must be signed in." {
		t.Errorf("mensaje de error sin traducir: %v", cuerpo)
	}

	// Exportación CSV: UTF-8 con BOM, encabezados y el filtro aplicado.
	w := observador.get("/api/v1/auditoria?accion=usuario.crear&formato=csv")
	lineas := strings.Split(strings.TrimSpace(w.Body.String()), "\r\n")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Body.String(), "\xEF\xBB\xBFid,creado_at,actor_id,") ||
		len(lineas) != 4 || !strings.Contains(w.Header().Get("Content-Disposition"), "auditoria.csv") {
		t.Errorf("CSV inesperado (%d, %d líneas): %q", w.Code, len(lineas), w.Body.String())
	}

	// En toda la bitácora no hay un solo correo electrónico (ARCO).
	asientos, _, err := e.repo.ConsultarAuditoria(context.Background(), models.FiltroAuditoria{Pagina: 1, Tamano: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range asientos {
		if strings.Contains(a.Antes+a.Despues, "@") {
			t.Errorf("el asiento %d (%s) contiene un correo: %s", a.ID, a.Accion, a.Despues)
		}
	}
}

// --- Expiración de la sesión ------------------------------------------------------

func TestExpiracionPorInactividad(t *testing.T) {
	e := nuevoEntorno(t)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	postulante := e.cliente()
	postulante.ingresar("ana@ejemplo.mx", passwordPrueba)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	viva := func(c *cliente) bool {
		estado, _ := c.json("/api/v1/sesion")
		return estado == http.StatusOK
	}
	expira := func(c *cliente) time.Duration {
		_, sesion := c.json("/api/v1/sesion")
		momento, err := time.Parse(time.RFC3339, sesion["expira_at"].(string))
		if err != nil {
			t.Fatal(err)
		}
		return momento.Sub(e.reloj.Ahora())
	}

	// La duración depende del perfil: 30 días el postulante, 8 horas el interno.
	if d := expira(postulante); d != models.SesionPostulante {
		t.Errorf("sesión de postulante: expira en %v, se esperaban 30 días", d)
	}
	if d := expira(admin); d != models.SesionInterna {
		t.Errorf("sesión interna: expira en %v, se esperaban 8 horas", d)
	}

	// En uso, la sesión se prolonga; abandonada, vence.
	for i := 0; i < 3; i++ {
		e.reloj.Avanzar(7 * time.Hour)
		if !viva(admin) {
			t.Fatalf("una sesión interna en uso venció a las %d h", 7*(i+1))
		}
	}
	e.reloj.Avanzar(9 * time.Hour)
	if viva(admin) {
		t.Error("una sesión interna sin uso por 9 horas debió vencer")
	}
	// Han pasado 30 horas: la del postulante sigue, y vence tras 30 días sin uso.
	if !viva(postulante) {
		t.Fatal("la sesión del postulante venció antes de tiempo")
	}
	e.reloj.Avanzar(31 * 24 * time.Hour)
	if viva(postulante) {
		t.Error("una sesión de postulante sin uso por 31 días debió vencer")
	}
	redirige(t, postulante.get("/cuenta"), "/cuenta/ingresar?aviso=ingresa&volver=%2Fcuenta")
	if postulante.cookies[cookieSesion] != "" {
		t.Error("la cookie de una sesión vencida debió retirarse")
	}
	// La limpieza periódica borra las sesiones vencidas.
	if n, err := e.repo.PurgarSesionesVencidas(context.Background()); err != nil || n != 2 {
		t.Errorf("purga de sesiones: %d (%v), se esperaban 2", n, err)
	}
}

// --- Límite de intentos por IP --------------------------------------------------

func TestLimiteDeIntentosPorIP(t *testing.T) {
	e := nuevoEntorno(t)
	atacante, vecino := e.cliente(), e.cliente()
	intentar := func(c *cliente) int {
		return c.post("/cuenta/ingresar", url.Values{"email": {"nadie@ejemplo.mx"}, "password": {"x"}}).Code
	}
	for i := 0; i < intentosPorMinuto; i++ {
		if estado := intentar(atacante); estado != http.StatusUnauthorized {
			t.Fatalf("intento %d: estado %d, se esperaba 401", i+1, estado)
		}
	}
	w := atacante.post("/cuenta/ingresar", url.Values{"email": {"nadie@ejemplo.mx"}, "password": {"x"}})
	esperar(t, w, http.StatusTooManyRequests, "Demasiadas solicitudes")
	if w.Header().Get("Retry-After") == "" {
		t.Error("falta la cabecera Retry-After")
	}
	// El límite es por IP: otro visitante no se ve afectado, ni las lecturas.
	if estado := intentar(vecino); estado != http.StatusUnauthorized {
		t.Errorf("otra IP no debe verse afectada: estado %d", estado)
	}
	esperar(t, atacante.get("/cuenta/ingresar"), http.StatusOK)
	// Las fichas se reponen con el tiempo.
	e.reloj.Avanzar(time.Minute)
	if estado := intentar(atacante); estado != http.StatusUnauthorized {
		t.Errorf("pasado un minuto debe poder intentarse de nuevo: estado %d", estado)
	}
}

// --- Trabajador de correo ---------------------------------------------------------

// mailerDePrueba falla las primeras veces y después entrega.
type mailerDePrueba struct {
	fallos    int
	enviados  []models.CorreoSaliente
	siempreNo bool
}

func (m *mailerDePrueba) Enviar(_ context.Context, c models.CorreoSaliente) (string, error) {
	if m.siempreNo || m.fallos > 0 {
		m.fallos--
		return "", errors.New("el servidor de correo no responde")
	}
	m.enviados = append(m.enviados, c)
	return fmt.Sprintf("<%d@prueba>", c.ID), nil
}

func TestTrabajadorDeCorreo(t *testing.T) {
	e := nuevoEntorno(t)
	redirige(t, e.cliente().post("/cuenta/registro", datosRegistro), "/cuenta?aviso=registro")
	mailer := &mailerDePrueba{fallos: 1}
	trabajador := &TrabajadorCorreo{
		Bandeja: e.repo, Mailer: mailer, PorMinuto: 30, Ahora: e.reloj.Ahora,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		esperar: func(context.Context, time.Duration) {}, // sin pausas en la prueba
	}
	ctx := context.Background()

	// Primer intento: falla. El mensaje sigue pendiente, pero reprogramado.
	if n := trabajador.Procesar(ctx); n != 0 {
		t.Fatalf("no debió enviarse nada: %d", n)
	}
	if n := len(e.correos()); n != 0 {
		t.Fatalf("tras un fallo el mensaje debe esperar su reintento: %d listos", n)
	}
	// Llegado el momento del reintento, sale.
	e.reloj.Avanzar(3 * time.Minute)
	if n := trabajador.Procesar(ctx); n != 1 || len(mailer.enviados) != 1 {
		t.Fatalf("el reintento debió enviar el mensaje: %d", n)
	}
	enviado := mailer.enviados[0]
	if enviado.Destinatario != "ana@ejemplo.mx" || enviado.Idioma != "es" ||
		enviado.Asunto != "Confirma tu correo electrónico · FIMNME" ||
		!strings.Contains(enviado.Texto, "Hola, Ana:") || !strings.Contains(enviado.HTML, `<a href="`+urlBasePrueba+`/cuenta/verificar?token=`) {
		t.Errorf("correo enviado inesperado: %+v", enviado)
	}
	// No se envía dos veces, y el envío queda en la bitácora (RF-AUD-01).
	if n := trabajador.Procesar(ctx); n != 0 || len(e.correos()) != 0 {
		t.Errorf("un mensaje enviado no debe salir de nuevo: %d", n)
	}
	if envios := e.asientos("correo.enviar"); len(envios) != 1 || envios[0].ActorPerfil != models.ActorSistema ||
		!strings.Contains(envios[0].Despues, "1@prueba") || !strings.Contains(envios[0].Despues, `"plantilla":"verificar_email"`) {
		t.Errorf("asiento del envío incorrecto: %+v", envios)
	}

	// Un mensaje que nunca logra salir se abandona tras los intentos máximos.
	otros := url.Values{}
	for k, v := range datosRegistro {
		otros[k] = v
	}
	otros.Set("email", "luis@ejemplo.mx")
	redirige(t, e.cliente().post("/cuenta/registro", otros), "/cuenta?aviso=registro")
	mailer.siempreNo = true
	for i := 0; i < models.IntentosMaxCorreo+2; i++ {
		trabajador.Procesar(ctx)
		e.reloj.Avanzar(7 * time.Hour)
	}
	if n := len(e.correos()); n != 0 {
		t.Errorf("el mensaje abandonado no debe seguir pendiente: %d", n)
	}
	if errores := e.asientos("correo.error"); len(errores) != 1 ||
		!strings.Contains(errores[0].Despues, fmt.Sprintf(`"intentos":%d`, models.IntentosMaxCorreo)) {
		t.Errorf("asiento del abandono incorrecto: %+v", errores)
	}
}

// El transporte de desarrollo no muestra los enlaces de un solo uso salvo que
// se le pida expresamente (sólo en el entorno de desarrollo).
func TestMailerDeRegistroNoMuestraEnlacesPorOmision(t *testing.T) {
	var salida strings.Builder
	log := slog.New(slog.NewTextHandler(&salida, nil))
	correo := models.CorreoSaliente{ID: 7, Destinatario: "ana@ejemplo.mx", Asunto: "Asunto", Texto: "enlace-secreto"}
	if _, err := (MailerRegistro{Log: log}).Enviar(context.Background(), correo); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(salida.String(), "enlace-secreto") {
		t.Error("el cuerpo del correo llegó al log sin haberlo pedido")
	}
	if _, err := (MailerRegistro{Log: log, MostrarCuerpo: true}).Enviar(context.Background(), correo); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(salida.String(), "enlace-secreto") {
		t.Error("en desarrollo el cuerpo debe aparecer en el log")
	}
}

// Toda clave de traducción que los controladores componen debe existir en
// ambos idiomas: avisos, mensajes y errores de la API.
func TestClavesDeLosControladoresExisten(t *testing.T) {
	vistas, err := views.Nuevo()
	if err != nil {
		t.Fatal(err)
	}
	var claves []string
	for _, nota := range notas {
		claves = append(claves, nota.Clave)
	}
	for _, prefijo := range []string{"mensaje.recuperar", "mensaje.enlace", "mensaje.verificado", "http.429"} {
		claves = append(claves, prefijo+"_titulo", prefijo+"_texto")
	}
	for _, p := range []models.PropositoToken{models.TokenResetPassword, models.TokenActivacion} {
		vista := paginaPasswordNueva(p)
		claves = append(claves, vista.Titulo, vista.Entrada)
	}
	for _, codigo := range []string{"no_autenticado", "sin_permiso", "parametro_invalido", "no_encontrado", "demasiadas", "error_interno"} {
		claves = append(claves, "api."+codigo)
	}
	claves = append(claves, "error.credenciales", "error.bloqueo", "error.email_en_uso", "error.password_actual",
		"error.email_igual", "mensaje.enlace_pedir", "cuenta.volver_ingresar", "nav.ingresar")
	for _, clave := range claves {
		for _, idioma := range views.Idiomas {
			if vistas.T(idioma, clave) == clave {
				t.Errorf("falta la traducción %q en %s", clave, idioma)
			}
		}
	}
}
