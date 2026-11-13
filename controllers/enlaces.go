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
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"syscall"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Comprobación en red de las ligas de video (RF-DOC-10).
//
// Comprobar una liga hace que el SERVIDOR visite una dirección escrita por
// cualquier persona en Internet. Sin cuidado, eso es una puerta a la red
// interna (SSRF): bastaría postular con la «liga» https://10.0.0.5/ para que
// el servidor sondeara máquinas que desde fuera no se ven. Las defensas:
//
//   - Sólo https y sólo el puerto 443.
//   - La dirección IP a la que de verdad se conecta se comprueba en el momento
//     de conectar, DESPUÉS de resolver el nombre. Así no sirve registrar un
//     dominio que resuelva a una dirección interna, ni cambiar la resolución
//     entre la comprobación y la conexión.
//   - Las redirecciones pasan por el mismo filtro, y son pocas.
//   - No se usa ningún proxy del entorno: delegaría la conexión en otro.
//   - Plazos cortos, y del cuerpo de la respuesta no se lee nada.
//
// Y la regla inversa, igual de importante: la comprobación NUNCA debe impedir
// un envío por un problema del propio servidor. Si no hay red, o no hay DNS,
// las ligas quedan «pendientes» y el revisor las abre a mano.
//
// La clasificación del resultado es una regla del dominio y vive en
// models.ClasificarEnlace.

const (
	plazoDeConexion        = 5 * time.Second
	plazoDeVerificacion    = 10 * time.Second // por liga, de principio a fin
	maxRedirecciones       = 5
	agenteDelVerificador   = "FIMNME-verificador-de-ligas/1.0"
	intervaloEnlaces       = 30 * time.Second // cada cuánto trabaja el verificador en segundo plano
	loteEnlaces            = 10               // ligas por tanda
	vigenciaDeComprobacion = 24 * time.Hour   // tras este tiempo una liga se vuelve a comprobar
	anfitrionDeIngreso     = "accounts.google.com"
)

// Errores con que el verificador distingue por qué no hubo respuesta.
var (
	errDireccionNoPermitida = errors.New("la liga apunta a una dirección que no es pública")
	errPideIngreso          = errors.New("la liga redirige a una pantalla de inicio de sesión")
	errRedireccion          = errors.New("redirección no admitida")
)

// soloDireccionesPublicas es el filtro que se ejecuta justo antes de abrir
// cada conexión, con la dirección IP ya resuelta. Rechaza todo lo que no sea
// una dirección pública en el puerto 443.
func soloDireccionesPublicas(_, direccion string, _ syscall.RawConn) error {
	destino, err := netip.ParseAddrPort(direccion)
	if err != nil || destino.Port() != 443 || !models.DireccionPublica(destino.Addr()) {
		return errDireccionNoPermitida
	}
	return nil
}

// VerificadorDeEnlaces comprueba si una liga responde públicamente.
type VerificadorDeEnlaces struct {
	// Cliente hace las peticiones. Las pruebas lo sustituyen por uno que no
	// sale a la red.
	Cliente *http.Client
	// HayDNS indica si el servidor puede resolver nombres en este momento.
	// Sólo se consulta cuando un dominio «no existe», para distinguir un
	// dominio mal escrito de un servidor sin DNS. Con nil se da por bueno.
	HayDNS func(ctx context.Context) bool
}

// dominiosTestigo son nombres que existen con certeza. Si el servidor no
// resuelve ninguno, el problema es su propia red y no la liga.
var dominiosTestigo = []string{"youtube.com", "vimeo.com", "wikipedia.org"}

// hayDNS intenta resolver los dominios testigo.
func hayDNS(ctx context.Context) bool {
	for _, dominio := range dominiosTestigo {
		if direcciones, err := net.DefaultResolver.LookupHost(ctx, dominio); err == nil && len(direcciones) > 0 {
			return true
		}
	}
	return false
}

// NuevoVerificadorDeEnlaces construye el verificador de producción, con todas
// las defensas descritas arriba.
func NuevoVerificadorDeEnlaces() *VerificadorDeEnlaces {
	marcador := &net.Dialer{Timeout: plazoDeConexion, Control: soloDireccionesPublicas}
	transporte := &http.Transport{
		Proxy:                  nil, // nunca a través de un proxy
		DialContext:            marcador.DialContext,
		TLSHandshakeTimeout:    plazoDeConexion,
		ResponseHeaderTimeout:  plazoDeVerificacion,
		MaxResponseHeaderBytes: 64 << 10,
		DisableKeepAlives:      true, // cada liga es de un sitio distinto
	}
	return &VerificadorDeEnlaces{HayDNS: hayDNS, Cliente: &http.Client{
		Transport: transporte,
		Timeout:   plazoDeVerificacion,
		CheckRedirect: func(siguiente *http.Request, anteriores []*http.Request) error {
			switch {
			case siguiente.URL.Scheme != "https" || len(anteriores) >= maxRedirecciones:
				return errRedireccion
			case siguiente.URL.Hostname() == anfitrionDeIngreso:
				// Así responde un archivo privado de Google Drive.
				return errPideIngreso
			}
			return nil
		},
	}}
}

// Verificar comprueba una liga y devuelve el resultado ya clasificado. Nunca
// falla: lo que no puede comprobarse queda «pendiente», para el revisor.
func (v *VerificadorDeEnlaces) Verificar(ctx context.Context, liga string) models.ResultadoEnlace {
	sinComprobar := models.ResultadoEnlace{Estado: models.EnlacePendiente, Detalle: models.DetalleEnlaceNoComprobable}
	if !models.URLValida(liga) {
		return sinComprobar
	}
	// A las plataformas que lo ofrecen se les pregunta por oEmbed, que dice si
	// el recurso existe y es público; a las demás se les pide el recurso.
	destino, porOEmbed := models.ConsultaOEmbed(liga), true
	if destino == "" {
		destino, porOEmbed = liga, false
	}
	ctx, cancelar := context.WithTimeout(ctx, plazoDeVerificacion)
	defer cancelar()
	peticion, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		return sinComprobar
	}
	peticion.Header.Set("User-Agent", agenteDelVerificador)
	peticion.Header.Set("Accept", "*/*")
	respuesta, err := v.Cliente.Do(peticion)
	var sinDominio *net.DNSError
	switch {
	case errors.Is(err, errPideIngreso):
		return models.ClasificarEnlace(porOEmbed, 0, true)
	case errors.Is(err, errDireccionNoPermitida):
		return models.ResultadoEnlace{Estado: models.EnlaceInaccesible, Detalle: models.DetalleEnlaceDireccion}
	case errors.As(err, &sinDominio) && sinDominio.IsNotFound:
		// «El dominio no existe» sólo es creíble si el servidor resuelve
		// otros nombres. Un servidor sin DNS —un cortafuegos mal puesto, una
		// avería del proveedor— diría lo mismo de TODAS las ligas, y darlas
		// por malas impediría enviar a todo el mundo.
		if v.HayDNS != nil && !v.HayDNS(ctx) {
			return sinComprobar
		}
		return models.ResultadoEnlace{Estado: models.EnlaceInaccesible, Detalle: models.DetalleEnlaceDominio}
	case err != nil:
		// Sin respuesta a tiempo, error de TLS, conexión rechazada…: no dice
		// nada seguro sobre la liga.
		return sinComprobar
	}
	// Del cuerpo no se lee nada: basta el código de estado.
	if err := respuesta.Body.Close(); err != nil {
		return sinComprobar
	}
	return models.ClasificarEnlace(porOEmbed, respuesta.StatusCode, false)
}

// verificarEnlacesDe comprueba, todas a la vez, las ligas de una postulación
// y anota los resultados. Se llama justo antes de un envío. Si no hay
// verificador configurado, o algo falla, las ligas quedan como estaban: el
// envío no depende de que Internet responda.
func (a *App) verificarEnlacesDe(ctx context.Context, postulacionID int64) {
	if a.enlaces == nil {
		return
	}
	ligas, err := a.datos.EnlacesDePostulacion(ctx, postulacionID)
	if err != nil {
		a.log.Error("leer ligas de la postulación", "postulacion", postulacionID, "error", err)
		return
	}
	var grupo sync.WaitGroup
	for _, liga := range ligas {
		grupo.Go(func() {
			resultado := a.enlaces.Verificar(ctx, liga.URL)
			if err := a.datos.AnotarVerificacionDeEnlace(ctx, liga, resultado); err != nil {
				a.log.Error("anotar verificación de liga", "liga", liga.ID, "error", err)
			}
		})
	}
	grupo.Wait()
}

// TrabajadorEnlaces comprueba en segundo plano las ligas recién capturadas y
// vuelve a comprobar, una vez al día, las de convocatorias en postulación o
// en revisión documental: una liga puede dejar de funcionar después de
// enviada, y el revisor debe verlo.
type TrabajadorEnlaces struct {
	Datos       models.Persistencia
	Verificador *VerificadorDeEnlaces
	Log         *slog.Logger
	// Ahora es el reloj; nil usa el del sistema.
	Ahora func() time.Time
}

// Ejecutar trabaja hasta que se cancele el contexto.
func (t *TrabajadorEnlaces) Ejecutar(ctx context.Context) {
	reloj := time.NewTicker(intervaloEnlaces)
	defer reloj.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reloj.C:
			t.Procesar(ctx)
		}
	}
}

// Procesar comprueba una tanda de ligas y devuelve cuántas comprobó.
func (t *TrabajadorEnlaces) Procesar(ctx context.Context) int {
	ahora := time.Now()
	if t.Ahora != nil {
		ahora = t.Ahora()
	}
	ligas, err := t.Datos.EnlacesPorVerificar(ctx, loteEnlaces, ahora.Add(-vigenciaDeComprobacion))
	if err != nil {
		t.Log.Error("leer ligas por comprobar", "error", err)
		return 0
	}
	comprobadas := 0
	for _, liga := range ligas {
		if ctx.Err() != nil {
			break // apagado en curso
		}
		resultado := t.Verificador.Verificar(ctx, liga.URL)
		if err := t.Datos.AnotarVerificacionDeEnlace(ctx, liga, resultado); err != nil {
			t.Log.Error("anotar verificación de liga", "liga", liga.ID, "error", err)
			continue
		}
		comprobadas++
	}
	return comprobadas
}
