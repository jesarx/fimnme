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
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
)

// cuentaInternaSinIngresar da de alta y activa una cuenta interna, sin
// iniciar sesión: la prueba del segundo paso quiere ver cada pantalla.
func (e *entornoPrueba) cuentaInternaSinIngresar(email string, perfil models.Perfil) int64 {
	e.t.Helper()
	claro, resumen, err := models.NuevoSecreto()
	if err != nil {
		e.t.Fatal(err)
	}
	id, err := e.repo.CrearUsuario(context.Background(), models.Actor{Perfil: models.ActorCLI}, models.NuevoUsuario{
		Email: email, Nombre: "Ada", Apellidos: "Ruiz", Idioma: models.IdiomaES,
		PasswordHash: models.PasswordSinDefinir, Perfil: perfil,
	}, &models.NuevoToken{Resumen: resumen, Proposito: models.TokenActivacion, ExpiraAt: e.reloj.Ahora().Add(models.VigenciaActivacion)})
	if err != nil {
		e.t.Fatal(err)
	}
	redirige(e.t, e.cliente().post("/cuenta/activar", url.Values{"token": {claro}, "password": {passwordPrueba}}), "/cuenta/ingresar?aviso=activada")
	return id
}

// codigoActual es el código que mostraría la aplicación de la cuenta ahora.
func (e *entornoPrueba) codigoActual(usuarioID int64) string {
	e.t.Helper()
	factor, err := e.repo.SegundoFactorDe(context.Background(), usuarioID)
	if err != nil {
		e.t.Fatal(err)
	}
	codigo, err := models.CodigoTOTP(factor.Secreto, models.PasoTOTP(e.reloj.Ahora()))
	if err != nil {
		e.t.Fatal(err)
	}
	return codigo
}

// RF-AUT-09: administrador y observador dan, además de la contraseña, un
// código de su aplicación autenticadora. La primera vez se dan de alta.
func TestSegundoFactor(t *testing.T) {
	e := nuevoEntorno(t)
	adaID := e.cuentaInternaSinIngresar("ada@ejemplo.mx", models.PerfilAdminSistema)
	ada := e.cliente()

	// --- La contraseña sola abre una sesión PENDIENTE, que no sirve para nada más.
	redirige(t, ada.post("/cuenta/ingresar", url.Values{"email": {"ada@ejemplo.mx"}, "password": {passwordPrueba}, "volver": {"/administracion"}}),
		"/cuenta/segundo-factor?volver=%2Fadministracion")
	redirige(t, ada.get("/administracion"), "/cuenta/segundo-factor?volver=%2Fadministracion")
	redirige(t, ada.get("/cuenta"), "/cuenta/segundo-factor?volver=%2Fcuenta")
	if estado, cuerpo := ada.json("/api/v1/sesion"); estado != http.StatusUnauthorized || codigoDe(cuerpo) != "segundo_factor_pendiente" {
		t.Errorf("la API no debe atender una sesión pendiente: %d %v", estado, cuerpo)
	}
	// Las páginas públicas la tratan como anónima.
	esperar(t, ada.get("/"), http.StatusOK, `href="/cuenta/ingresar"`)

	// --- Alta: QR, clave en texto y enlace otpauth; recargar no cambia la clave.
	w := ada.get("/cuenta/segundo-factor?volver=/administracion")
	esperar(t, w, http.StatusOK, "Activa la verificación en dos pasos", `<svg class="qr"`, "otpauth://totp/FIMNME:ada@ejemplo.mx?", `autocomplete="one-time-code"`)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("la página del alta lleva la clave: no debe guardarse en caché")
	}
	antes, _ := e.repo.SegundoFactorDe(context.Background(), adaID)
	ada.get("/cuenta/segundo-factor")
	if despues, _ := e.repo.SegundoFactorDe(context.Background(), adaID); despues.Secreto != antes.Secreto || antes.Secreto == "" || antes.Activo() {
		t.Error("recargar el alta no debe cambiar la clave ni activarla")
	}
	if !strings.Contains(w.Body.String(), enGrupos(antes.Secreto)) {
		t.Error("la clave debe mostrarse en grupos de cuatro")
	}
	esperar(t, ada.post("/cuenta/segundo-factor/alta", url.Values{"codigo": {"000000"}}), http.StatusUnauthorized, "El código no es correcto")
	w = ada.post("/cuenta/segundo-factor/alta", url.Values{"codigo": {e.codigoActual(adaID)}, "volver": {"/administracion"}})
	esperar(t, w, http.StatusOK, "Tus códigos de recuperación", `href="/administracion"`)
	codigos := codigosDeLaPagina(w.Body.String())
	if len(codigos) != models.CodigosDeRecuperacion {
		t.Fatalf("se esperaban %d códigos de recuperación: %v", models.CodigosDeRecuperacion, codigos)
	}
	esperar(t, ada.get("/administracion"), http.StatusOK)
	esperar(t, ada.get("/cuenta"), http.StatusOK, "Verificación en dos pasos", "Códigos de recuperación sin usar: 10.")

	// --- Siguiente ingreso: el mismo código no vale dos veces.
	salirYEntrar := func(c *cliente) {
		redirige(t, c.post("/cuenta/salir", nil), "/cuenta/ingresar?aviso=salida")
		redirige(t, c.post("/cuenta/ingresar", url.Values{"email": {"ada@ejemplo.mx"}, "password": {passwordPrueba}}), "/cuenta/segundo-factor")
	}
	salirYEntrar(ada)
	esperar(t, ada.get("/cuenta/segundo-factor"), http.StatusOK, "Escribe el código de seis cifras")
	esperar(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {e.codigoActual(adaID)}}), http.StatusUnauthorized, "ya se usó")
	e.reloj.Avanzar(30 * time.Second)
	redirige(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {e.codigoActual(adaID)}}), "/cuenta")

	// --- Un código de recuperación vale una sola vez.
	salirYEntrar(ada)
	redirige(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {strings.ToUpper(codigos[0])}}), "/cuenta")
	salirYEntrar(ada)
	esperar(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {codigos[0]}}), http.StatusUnauthorized)
	redirige(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {codigos[1]}}), "/cuenta")
	esperar(t, ada.get("/cuenta"), http.StatusOK, "Códigos de recuperación sin usar: 8.")

	// --- Códigos nuevos desde «Mi cuenta», con un código vigente: los anteriores mueren.
	e.reloj.Avanzar(30 * time.Second)
	esperar(t, ada.post("/cuenta/segundo-factor/codigos", url.Values{"codigo": {"123456"}}), http.StatusUnprocessableEntity, "El código no es correcto")
	w = ada.post("/cuenta/segundo-factor/codigos", url.Values{"codigo": {e.codigoActual(adaID)}})
	esperar(t, w, http.StatusOK, "Tus códigos de recuperación")
	nuevos := codigosDeLaPagina(w.Body.String())
	salirYEntrar(ada)
	esperar(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {codigos[2]}}), http.StatusUnauthorized)
	redirige(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {nuevos[0]}}), "/cuenta")

	// --- Bloqueo: los códigos equivocados cuentan como contraseñas equivocadas,
	// y volver a escribir la contraseña NO reinicia la cuenta.
	salirYEntrar(ada)
	for range models.IntentosAntesDeBloqueo - 1 {
		esperar(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {"999999"}}), http.StatusUnauthorized)
	}
	salirYEntrar(ada)
	esperar(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {"999999"}}), http.StatusUnauthorized)
	e.reloj.Avanzar(30 * time.Second)
	esperar(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {e.codigoActual(adaID)}}), http.StatusTooManyRequests)
	e.reloj.Avanzar(models.BloqueoBase + time.Second)
	redirige(t, ada.post("/cuenta/segundo-factor", url.Values{"codigo": {e.codigoActual(adaID)}}), "/cuenta")

	// --- La sesión pendiente caduca a los 10 minutos.
	salirYEntrar(ada)
	e.reloj.Avanzar(models.SesionPendiente + time.Second)
	redirige(t, ada.get("/cuenta/segundo-factor"), "/cuenta")
	redirige(t, ada.get("/cuenta"), "/cuenta/ingresar?aviso=ingresa&volver=%2Fcuenta")

	// --- Cancelar a medio camino cierra la sesión pendiente.
	redirige(t, ada.post("/cuenta/ingresar", url.Values{"email": {"ada@ejemplo.mx"}, "password": {passwordPrueba}}), "/cuenta/segundo-factor")
	redirige(t, ada.post("/cuenta/salir", nil), "/cuenta/ingresar?aviso=salida")
	if _, cuerpo := ada.json("/api/v1/sesion"); codigoDe(cuerpo) != "no_autenticado" {
		t.Errorf("tras cancelar no debe quedar sesión: %v", cuerpo)
	}

	// --- Bitácora: la contraseña correcta y el ingreso con su segundo paso.
	asientos, _, err := e.repo.ConsultarAuditoria(context.Background(), models.FiltroAuditoria{EntidadID: adaID, Accion: "sesion.ingreso", Pagina: 1, Tamano: 50})
	if err != nil || len(asientos) == 0 || !strings.Contains(asientos[0].Despues, "segundo_factor") {
		t.Errorf("el ingreso debe asentar su segundo paso: %+v (%v)", asientos, err)
	}

	// --- Postulantes, jurados y revisores no tienen segundo paso.
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	ana := e.cliente()
	redirige(t, ana.post("/cuenta/ingresar", url.Values{"email": {"ana@ejemplo.mx"}, "password": {passwordPrueba}}), "/cuenta")
	esperar(t, ana.get("/cuenta"), http.StatusOK)
	noAparece(t, ana.get("/cuenta"), "Verificación en dos pasos")
}
