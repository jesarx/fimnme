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
	"fmt"
	"log/slog"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Trabajadores internos de fimnme-web: tareas en segundo plano que no
// responden a una petición. Por ahora, el envío del correo saliente y la
// limpieza de sesiones vencidas.

// Ritmo del trabajador.
const (
	intervaloBandeja = 5 * time.Second // cada cuánto se revisa la bandeja
	loteBandeja      = 20              // mensajes por revisión
	intervaloPurga   = time.Hour       // cada cuánto se limpian sesiones vencidas
)

// TrabajadorCorreo consume la bandeja de salida (patrón «transactional
// outbox», §5.8): las transacciones de negocio escriben los mensajes en la
// base y este trabajador los entrega después, con reintentos y sin pasar de
// un máximo por minuto (RF-SMTP-07). Ningún proceso de negocio espera a que
// un correo salga.
type TrabajadorCorreo struct {
	Bandeja models.Persistencia
	Mailer  models.Mailer
	Log     *slog.Logger
	// PorMinuto es el máximo de envíos por minuto. Las ráfagas disparan los
	// filtros de correo no deseado de los grandes proveedores.
	PorMinuto int
	// Ahora es el reloj; nil usa el del sistema.
	Ahora func() time.Time
	// esperar hace la pausa entre envíos; las pruebas la anulan.
	esperar func(ctx context.Context, d time.Duration)
}

// pausa espera d o hasta que se cancele el contexto.
func pausa(ctx context.Context, d time.Duration) {
	temporizador := time.NewTimer(d)
	defer temporizador.Stop()
	select {
	case <-ctx.Done():
	case <-temporizador.C:
	}
}

func (t *TrabajadorCorreo) reloj() time.Time {
	if t.Ahora != nil {
		return t.Ahora()
	}
	return time.Now()
}

// Ejecutar atiende la bandeja hasta que se cancele el contexto. Se lanza en
// una gorutina al arrancar el servidor.
func (t *TrabajadorCorreo) Ejecutar(ctx context.Context) {
	bandeja := time.NewTicker(intervaloBandeja)
	defer bandeja.Stop()
	purga := time.NewTicker(intervaloPurga)
	defer purga.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-bandeja.C:
			t.Procesar(ctx)
		case <-purga.C:
			if n, err := t.Bandeja.PurgarSesionesVencidas(ctx); err != nil {
				t.Log.Error("purgar sesiones vencidas", "error", err)
			} else if n > 0 {
				t.Log.Info("sesiones vencidas eliminadas", "cantidad", n)
			}
		}
	}
}

// Procesar envía un lote de mensajes pendientes y devuelve cuántos salieron.
func (t *TrabajadorCorreo) Procesar(ctx context.Context) int {
	correos, err := t.Bandeja.CorreosPendientes(ctx, loteBandeja)
	if err != nil {
		t.Log.Error("leer bandeja de salida", "error", err)
		return 0
	}
	esperar := t.esperar
	if esperar == nil {
		esperar = pausa
	}
	porMinuto := max(t.PorMinuto, 1)
	enviados := 0
	for _, correo := range correos {
		if ctx.Err() != nil {
			break // apagado en curso: lo pendiente saldrá al volver a arrancar
		}
		messageID, err := t.Mailer.Enviar(ctx, correo)
		if err != nil {
			t.fallo(ctx, correo, err)
		} else if err := t.Bandeja.MarcarCorreoEnviado(ctx, correo, messageID); err != nil {
			// El mensaje salió pero no se pudo anotar: se reintentará y
			// podría llegar repetido. Es preferible a que no llegue.
			t.Log.Error("anotar correo enviado", "id", correo.ID, "error", err)
		} else {
			enviados++
		}
		esperar(ctx, time.Minute/time.Duration(porMinuto))
	}
	return enviados
}

// fallo anota un envío fallido y decide, con la regla del dominio, si se
// reintenta más tarde o se da por perdido.
func (t *TrabajadorCorreo) fallo(ctx context.Context, correo models.CorreoSaliente, causa error) {
	proximo, reintentar := models.ReintentoDeCorreo(correo.Intentos+1, t.reloj())
	nivel := slog.LevelWarn
	if !reintentar {
		nivel = slog.LevelError
	}
	t.Log.Log(ctx, nivel, "no se pudo enviar un correo",
		"id", correo.ID, "plantilla", correo.PlantillaClave, "intento", correo.Intentos+1,
		"se_reintentara", reintentar, "error", causa)
	if err := t.Bandeja.MarcarCorreoFallido(ctx, correo, causa.Error(), proximo, reintentar); err != nil {
		t.Log.Error("anotar correo fallido", "id", correo.ID, "error", err)
	}
}

// MailerRegistro es el transporte de correo de DESARROLLO: no envía nada.
// Escribe cada mensaje en el log, de donde puede copiarse el enlace de
// verificación o de recuperación para probar los flujos.
//
// Los enlaces de un solo uso son secretos, así que el cuerpo sólo se escribe
// si MostrarCuerpo es true, que el servidor activa únicamente en el entorno
// de desarrollo. El transporte real (SMTP propio) llega en el paso 14.
type MailerRegistro struct {
	Log           *slog.Logger
	MostrarCuerpo bool
}

// Enviar escribe el mensaje en el log y lo da por entregado.
func (m MailerRegistro) Enviar(_ context.Context, correo models.CorreoSaliente) (string, error) {
	atributos := []any{
		"id", correo.ID, "para", correo.Destinatario, "asunto", correo.Asunto, "plantilla", correo.PlantillaClave,
	}
	if m.MostrarCuerpo {
		atributos = append(atributos, "cuerpo", correo.Texto)
	}
	m.Log.Info("correo NO enviado: transporte de desarrollo", atributos...)
	return fmt.Sprintf("<%d.sin-enviar@fimnme.invalid>", correo.ID), nil
}

// --- Avance de etapa por calendario (§3) --------------------------------------

// Ritmo del trabajador de etapas.
const (
	intervaloEtapas = 30 * time.Second // cada cuánto se revisa el calendario
	esperaAvisos    = time.Hour        // un avance bloqueado se avisa una vez por hora
)

// TrabajadorEtapas hace avanzar las convocatorias de etapa cuando llega su
// fecha («el sistema avanza de etapa automáticamente por fecha»). Pasa por
// las MISMAS reglas que el cambio forzado por el administrador: si a la fecha
// de cierre de la postulación no hay un revisor asignado, la convocatoria no
// avanza, y se deja un aviso en el log hasta que se corrija.
type TrabajadorEtapas struct {
	Datos models.Persistencia
	Log   *slog.Logger
	// Ahora es el reloj; nil usa el del sistema.
	Ahora func() time.Time
	// avisado recuerda cuándo se avisó por última vez de cada bloqueo, para
	// no llenar el log con el mismo mensaje cada 30 segundos.
	avisado map[int64]time.Time
}

func (t *TrabajadorEtapas) reloj() time.Time {
	if t.Ahora != nil {
		return t.Ahora()
	}
	return time.Now()
}

// Ejecutar revisa el calendario hasta que se cancele el contexto.
func (t *TrabajadorEtapas) Ejecutar(ctx context.Context) {
	t.Procesar(ctx) // al arrancar: puede haber vencido una fecha con el servicio detenido
	reloj := time.NewTicker(intervaloEtapas)
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

// Procesar avanza cada convocatoria en curso tantas etapas como su calendario
// indique y sus reglas permitan. Devuelve cuántos cambios de etapa hizo.
func (t *TrabajadorEtapas) Procesar(ctx context.Context) int {
	ids, err := t.Datos.ConvocatoriasEnCurso(ctx)
	if err != nil {
		t.Log.Error("leer convocatorias en curso", "error", err)
		return 0
	}
	cambios := 0
	for _, id := range ids {
		// De una en una, por si el servicio estuvo detenido y hay varias
		// fechas vencidas. El ciclo de vida tiene siete estados: no hay más
		// de seis avances posibles.
		for paso := 0; paso < 6; paso++ {
			cfg, err := t.Datos.ConfiguracionDeConvocatoria(ctx, id)
			if err != nil {
				t.Log.Error("leer configuración de la convocatoria", "convocatoria", id, "error", err)
				break
			}
			destino, toca := models.ProximoEstadoAutomatico(cfg.Convocatoria, t.reloj())
			if !toca {
				break
			}
			anterior, err := t.Datos.CambiarEstadoConvocatoria(ctx, models.Actor{Perfil: models.ActorSistema}, id, destino, "", true)
			if err != nil {
				t.avisarBloqueo(id, destino, err)
				break
			}
			cambios++
			delete(t.avisado, id)
			t.Log.Info("la convocatoria avanzó de etapa por calendario", "convocatoria", id, "de", anterior, "a", destino)
		}
	}
	return cambios
}

// avisarBloqueo deja constancia de que una convocatoria debió avanzar y no
// pudo, como mucho una vez por hora.
func (t *TrabajadorEtapas) avisarBloqueo(id int64, destino models.EstadoConvocatoria, causa error) {
	var regla *models.ErrorValidacion
	if !errors.As(causa, &regla) {
		t.Log.Error("cambiar de etapa por calendario", "convocatoria", id, "a", destino, "error", causa)
		return
	}
	if t.avisado == nil {
		t.avisado = make(map[int64]time.Time)
	}
	if ultimo, ok := t.avisado[id]; ok && t.reloj().Sub(ultimo) < esperaAvisos {
		return
	}
	t.avisado[id] = t.reloj()
	t.Log.Warn("la convocatoria debió avanzar de etapa por calendario y NO pudo: falta cumplir una regla",
		"convocatoria", id, "a", destino, "motivo", causa.Error())
}
