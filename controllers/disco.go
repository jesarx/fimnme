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
	"log/slog"
	"strconv"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Alerta de espacio en disco (RF-OPS-07). Un sistema que se queda sin disco
// en pleno cierre de convocatoria falla aunque el software esté impecable:
// las subidas dejan de entrar y SQLite deja de poder escribir.

const (
	// intervaloDisco es cada cuánto se mide el espacio libre.
	intervaloDisco = 10 * time.Minute
	// intervaloAlertaDisco es cada cuánto se repite el aviso por correo
	// mientras el espacio siga por debajo del umbral.
	intervaloAlertaDisco = 24 * time.Hour
)

// TrabajadorDisco vigila el espacio libre del disco de datos. Por debajo del
// umbral lo escribe en el log como error —donde lo recoge la supervisión del
// servidor— y avisa por correo a los administradores, una vez al día.
type TrabajadorDisco struct {
	Datos      models.Persistencia
	Vistas     *views.Motor
	Log        *slog.Logger
	Directorio string
	// MinimoLibre es el umbral, en bytes. Con 0 no se vigila.
	MinimoLibre uint64
	// EspacioLibre mide el espacio libre de un directorio; nil usa el del
	// sistema. Las pruebas lo sustituyen.
	EspacioLibre func(directorio string) (uint64, error)
	// Ahora es el reloj; nil usa el del sistema.
	Ahora func() time.Time

	ultimoAviso time.Time
}

// Ejecutar trabaja hasta que se cancele el contexto.
func (t *TrabajadorDisco) Ejecutar(ctx context.Context) {
	if t.MinimoLibre == 0 {
		return
	}
	t.Procesar(ctx) // al arrancar: si ya falta espacio, que se sepa desde el principio
	reloj := time.NewTicker(intervaloDisco)
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

// Procesar mide el espacio libre y, si está por debajo del umbral, alerta.
// Devuelve true si alertó por correo.
func (t *TrabajadorDisco) Procesar(ctx context.Context) bool {
	medir, ahora := t.EspacioLibre, time.Now()
	if medir == nil {
		medir = models.EspacioLibre
	}
	if t.Ahora != nil {
		ahora = t.Ahora()
	}
	libre, err := medir(t.Directorio)
	if err != nil {
		t.Log.Error("no se pudo medir el espacio en disco", "directorio", t.Directorio, "error", err)
		return false
	}
	if libre >= t.MinimoLibre {
		t.ultimoAviso = time.Time{} // si vuelve a bajar, se avisa de inmediato
		return false
	}
	t.Log.Error("queda poco espacio en el disco de datos", "directorio", t.Directorio,
		"libre_bytes", libre, "umbral_bytes", t.MinimoLibre)
	if !t.ultimoAviso.IsZero() && ahora.Sub(t.ultimoAviso) < intervaloAlertaDisco {
		return false
	}
	administradores, err := t.Datos.AdministradoresActivos(ctx)
	if err != nil {
		t.Log.Error("leer administradores para la alerta de disco", "error", err)
		return false
	}
	avisados := 0
	for _, administrador := range administradores {
		plantilla, err := t.Datos.PlantillaCorreo(ctx, models.PlantillaAlertaDisco, administrador.Idioma)
		if err != nil {
			t.Log.Error("leer plantilla de la alerta de disco", "error", err)
			return false
		}
		correo := views.RenderCorreo(plantilla, administrador.Email, map[string]string{
			"nombre": administrador.Nombre, "libre": gigabytes(libre), "umbral": gigabytes(t.MinimoLibre), "datos": t.Directorio,
		})
		if err := t.Datos.EncolarAviso(ctx, correo); err != nil {
			t.Log.Error("encolar alerta de disco", "error", err)
			continue
		}
		avisados++
	}
	if avisados > 0 {
		t.ultimoAviso = ahora
	}
	return avisados > 0
}

// gigabytes escribe un tamaño en bytes como «8.2 GB».
func gigabytes(n uint64) string {
	return strconv.FormatFloat(float64(n)/(1<<30), 'f', 1, 64) + " GB"
}
