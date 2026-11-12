// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Sesiones (RF-AUT-10), control de acceso por petición y límite de intentos.

// cookieSesion lleva el identificador de la sesión: 256 bits aleatorios. En la
// base sólo se guarda su resumen (ADR 0004).
const cookieSesion = "fimnme_sesion"

// claveSesion es la llave privada de la sesión en el contexto de la petición.
type claveSesion struct{}

// ponerCookieSesion entrega al navegador el identificador de sesión.
// HttpOnly: JavaScript no la lee. Secure: sólo viaja por HTTPS. SameSite=Lax:
// no acompaña peticiones lanzadas desde otros sitios.
func (a *App) ponerCookieSesion(w http.ResponseWriter, r *http.Request, valor string, duracion time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieSesion,
		Value:    valor,
		Path:     "/",
		MaxAge:   int(duracion.Seconds()),
		HttpOnly: true,
		Secure:   esHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// borrarCookieSesion retira la cookie del navegador.
func (a *App) borrarCookieSesion(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieSesion, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: esHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

// cargarSesion identifica al visitante por su cookie y deja la sesión en el
// contexto de la petición. También aplica la expiración por INACTIVIDAD: una
// sesión en uso se prolonga; una abandonada vence sola.
func (a *App) cargarSesion(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieSesion)
		if err != nil || cookie.Value == "" {
			siguiente.ServeHTTP(w, r)
			return
		}
		sesion, err := a.datos.SesionPorID(r.Context(), models.ResumenDeSecreto(cookie.Value))
		switch {
		case errors.Is(err, models.ErrNoEncontrado):
			// Venció, se cerró o la cuenta se desactivó.
			a.borrarCookieSesion(w, r)
		case err != nil:
			// Sin base no hay forma de saber quién es: se sigue como anónimo
			// y la propia página fallará con su error 500.
			a.log.Error("no se pudo leer la sesión", "error", err)
		default:
			a.renovarSesion(w, r, sesion, cookie.Value)
			r = r.WithContext(context.WithValue(r.Context(), claveSesion{}, sesion))
		}
		siguiente.ServeHTTP(w, r)
	})
}

// renovarSesion prolonga la sesión si lleva un rato sin renovarse, y la
// acorta si la cuenta ganó un perfil interno (de 30 días a 8 horas).
func (a *App) renovarSesion(w http.ResponseWriter, r *http.Request, s *models.Sesion, valorCookie string) {
	duracion := models.DuracionSesion(s.Perfiles, s.Asignaciones)
	restante := s.ExpiraAt.Sub(a.ahora())
	if restante >= duracion-models.RenovacionSesion && restante <= duracion {
		return // se renovó hace poco
	}
	expira := a.ahora().Add(duracion)
	if err := a.datos.RenovarSesion(r.Context(), s.ID, expira); err != nil {
		a.log.Error("no se pudo renovar la sesión", "error", err)
		return
	}
	s.ExpiraAt = expira
	a.ponerCookieSesion(w, r, valorCookie, duracion)
}

// sesionDe devuelve la sesión de la petición, o nil si el visitante es anónimo.
func sesionDe(r *http.Request) *models.Sesion {
	sesion, _ := r.Context().Value(claveSesion{}).(*models.Sesion)
	return sesion
}

// esAPI indica si la petición es de la API REST, que responde JSON.
func esAPI(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

// conSesion protege un manejador: sólo lo alcanza quien inició sesión. Al
// anónimo se le lleva a ingresar (páginas) o se le responde 401 (API).
func (a *App) conSesion(manejador http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sesionDe(r) != nil {
			manejador(w, r)
			return
		}
		if esAPI(r) {
			a.errorAPI(w, r, http.StatusUnauthorized, "no_autenticado")
			return
		}
		destino := "/cuenta/ingresar?aviso=ingresa"
		if r.Method == http.MethodGet {
			// Tras ingresar se vuelve a la página que se pidió.
			destino += "&volver=" + url.QueryEscape(r.URL.Path)
		}
		http.Redirect(w, r, destino, http.StatusSeeOther)
	}
}

// actorDe identifica a quien hace la petición, para la bitácora.
func (a *App) actorDe(r *http.Request) models.Actor {
	agente := r.UserAgent()
	if len(agente) > 300 {
		agente = agente[:300] // un user-agent desmesurado no debe inflar la bitácora
	}
	actor := models.Actor{Perfil: models.ActorAnonimo, IP: ipCliente(r), Agente: agente}
	if s := sesionDe(r); s != nil {
		actor.UsuarioID = s.Usuario.ID
		actor.Perfil = string(s.PerfilPrincipal())
	}
	return actor
}

// iniciarSesion abre una sesión para la cuenta y entrega su cookie. El
// identificador es siempre nuevo: nunca se reutiliza uno anterior al ingreso.
func (a *App) iniciarSesion(w http.ResponseWriter, r *http.Request, usuarioID int64) error {
	claro, resumen, err := models.NuevoSecreto()
	if err != nil {
		return err
	}
	// En este momento aún no se han leído los perfiles de la cuenta: el
	// asiento del ingreso identifica al actor por su id, sin perfil.
	actor := a.actorDe(r)
	actor.UsuarioID, actor.Perfil = usuarioID, ""
	// Se abre con la duración corta. En la siguiente petición, ya con los
	// perfiles de la cuenta a la vista, cargarSesion la ajusta a la que le
	// corresponde (30 días si sólo es postulante).
	if err := a.datos.AbrirSesion(r.Context(), actor, usuarioID, resumen, a.ahora().Add(models.SesionInterna)); err != nil {
		return err
	}
	a.ponerCookieSesion(w, r, claro, models.SesionInterna)
	return nil
}

// --- Límite de intentos por IP ------------------------------------------------

// intentosPorMinuto es cuántos envíos de formularios de cuenta se admiten por
// IP y por minuto. Es holgado a propósito: tras una misma IP puede haber un
// conservatorio entero registrándose el último día.
const intentosPorMinuto = 30

// limitador es un «cubo de fichas» por IP, en memoria. Cada IP tiene hasta
// `capacidad` fichas que se reponen de forma continua; cada intento gasta una.
type limitador struct {
	mu        sync.Mutex
	cubos     map[string]*cubo
	capacidad float64
	ahora     func() time.Time
	limpieza  time.Time
}

type cubo struct {
	fichas float64
	visto  time.Time
}

func nuevoLimitador(porMinuto int, ahora func() time.Time) *limitador {
	return &limitador{cubos: make(map[string]*cubo), capacidad: float64(porMinuto), ahora: ahora}
}

// permitir gasta una ficha de la IP y dice si el intento procede.
func (l *limitador) permitir(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	ahora := l.ahora()

	// Cada minuto se olvidan las IP que ya recuperaron todas sus fichas, para
	// que el mapa no crezca sin límite.
	if ahora.Sub(l.limpieza) > time.Minute {
		for clave, c := range l.cubos {
			if ahora.Sub(c.visto) > time.Minute {
				delete(l.cubos, clave)
			}
		}
		l.limpieza = ahora
	}

	c, ok := l.cubos[ip]
	if !ok {
		c = &cubo{fichas: l.capacidad, visto: ahora}
		l.cubos[ip] = c
	}
	// Reposición continua: `capacidad` fichas por minuto.
	c.fichas = min(l.capacidad, c.fichas+ahora.Sub(c.visto).Minutes()*l.capacidad)
	c.visto = ahora
	if c.fichas < 1 {
		return false
	}
	c.fichas--
	return true
}

// limitarIntentos frena la fuerza bruta y el abuso sobre los formularios de
// cuenta (RNF-10): registro, ingreso, recuperación y cambios. El bloqueo por
// cuenta (RF-AUT-08) es aparte y lo decide models.BloqueoDeIngreso.
func (a *App) limitarIntentos(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/cuenta/") && !a.limiteIP.permitir(ipCliente(r)) {
			a.log.Warn("límite de intentos por IP alcanzado", "ruta", r.URL.Path, "ip", ipCliente(r))
			w.Header().Set("Retry-After", "60")
			a.errorPagina(w, r, http.StatusTooManyRequests)
			return
		}
		siguiente.ServeHTTP(w, r)
	})
}

// --- Errores de la API ----------------------------------------------------------

// errorAPI responde un error de la API en JSON: un código estable para
// programas y un mensaje en el idioma de la petición.
func (a *App) errorAPI(w http.ResponseWriter, r *http.Request, estado int, codigo string, args ...any) {
	mensaje := a.vistas.T(idiomaDe(r), "api."+codigo, args...)
	if err := views.JSON(w, estado, views.ErrorJSON(codigo, mensaje)); err != nil {
		a.log.Error("no se pudo escribir la respuesta JSON", "error", err)
	}
}
