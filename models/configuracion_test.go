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

package models

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// claves resume una lista de problemas para compararla en las pruebas.
func claves(errores []ErrorCampo) string {
	var lista []string
	for _, e := range errores {
		lista = append(lista, e.Clave)
	}
	return fmt.Sprint(lista)
}

// configuracionCompleta devuelve una convocatoria de Compositores con todo lo
// necesario para recorrer el ciclo de vida, en el estado dado.
func configuracionCompleta(estado EstadoConvocatoria) ConfiguracionConvocatoria {
	c := convocatoriaDePrueba(1, TipoCompositores, estado)
	return ConfiguracionConvocatoria{
		Convocatoria: c,
		Dotaciones:   []Dotacion{{ID: 1, Activa: true}},
		Criterios:    CriteriosIniciales(),
		Asignaciones: []AsignacionDetalle{
			{Perfil: PerfilRevisorDocumental, Activo: true},
			{Perfil: PerfilJurado, Activo: true}, {Perfil: PerfilJurado, Activo: true}, {Perfil: PerfilJurado, Activo: true},
		},
	}
}

// El ciclo de vida completo, hacia adelante, con todo en regla.
func TestCicloDeVidaCompleto(t *testing.T) {
	antes := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC) // antes de que abra la postulación
	cfg := configuracionCompleta(EstadoBorrador)
	recorrido := []EstadoConvocatoria{EstadoPublicada, EstadoPostulacion, EstadoRevisionDocumental,
		EstadoEvaluacion, EstadoCerrada, EstadoArchivada}
	for _, destino := range recorrido {
		if siguiente, ok := EstadoSiguiente(cfg.Convocatoria.Estado); !ok || siguiente != destino {
			t.Fatalf("tras %s se esperaba %s, se obtuvo %s", cfg.Convocatoria.Estado, destino, siguiente)
		}
		if errores := ValidarCambioDeEstado(cfg, destino, "", antes); len(errores) != 0 {
			t.Fatalf("%s → %s rechazado: %s", cfg.Convocatoria.Estado, destino, claves(errores))
		}
		cfg.Convocatoria.Estado = destino
	}
	if _, ok := EstadoSiguiente(EstadoArchivada); ok {
		t.Error("tras archivada no hay etapa siguiente")
	}
}

// Cada transición tiene sus condiciones (§3, RN-14).
func TestCondicionesDeCadaTransicion(t *testing.T) {
	antes := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	casos := []struct {
		nombre  string
		estado  EstadoConvocatoria
		destino EstadoConvocatoria
		alterar func(*ConfiguracionConvocatoria)
		motivo  string
		errores string
	}{
		{"publicar sin bases", EstadoBorrador, EstadoPublicada,
			func(c *ConfiguracionConvocatoria) { c.Convocatoria.BasesMDES = "  " }, "", "[error.publicar_bases]"},
		{"publicar sin calendario completo", EstadoBorrador, EstadoPublicada,
			func(c *ConfiguracionConvocatoria) { c.Convocatoria.Etapas = c.Convocatoria.Etapas[:2] }, "", "[error.calendario_incompleto]"},
		{"abrir Compositores sin dotaciones activas", EstadoPublicada, EstadoPostulacion,
			func(c *ConfiguracionConvocatoria) { c.Dotaciones[0].Activa = false }, "", "[error.sin_dotaciones]"},
		{"abrir Intérpretes sin dotaciones", EstadoPublicada, EstadoPostulacion,
			func(c *ConfiguracionConvocatoria) { c.Convocatoria.Tipo, c.Dotaciones = TipoInterpretes, nil }, "", "[]"},
		{"revisión sin revisor activo", EstadoPostulacion, EstadoRevisionDocumental,
			func(c *ConfiguracionConvocatoria) { c.Asignaciones[0].Activo = false }, "", "[error.sin_revisor]"},
		{"evaluación con dos jurados", EstadoRevisionDocumental, EstadoEvaluacion,
			func(c *ConfiguracionConvocatoria) { c.Asignaciones[3].Activo = false }, "", "[error.jurados_exactos]"},
		{"evaluación con cuatro jurados", EstadoRevisionDocumental, EstadoEvaluacion,
			func(c *ConfiguracionConvocatoria) {
				c.Asignaciones = append(c.Asignaciones, AsignacionDetalle{Perfil: PerfilJurado, Activo: true})
			}, "", "[error.jurados_exactos]"},
		{"evaluación sin criterios", EstadoRevisionDocumental, EstadoEvaluacion,
			func(c *ConfiguracionConvocatoria) { c.Criterios = nil }, "", "[error.sin_criterios]"},
		{"salto de dos etapas", EstadoBorrador, EstadoPostulacion, nil, "", "[error.estado_salto]"},
		{"al mismo estado", EstadoPublicada, EstadoPublicada, nil, "", "[error.estado_salto]"},
		{"estado inexistente", EstadoPublicada, "premiacion", nil, "", "[error.estado_invalido]"},
		{"retroceso sin motivo", EstadoPublicada, EstadoBorrador, nil, "", "[error.motivo_requerido]"},
		{"retroceso con motivo", EstadoPublicada, EstadoBorrador, nil, "Se publicó por error", "[]"},
		{"desarchivar con motivo", EstadoArchivada, EstadoCerrada, nil, "Corrección", "[]"},
	}
	for _, c := range casos {
		cfg := configuracionCompleta(c.estado)
		if c.alterar != nil {
			c.alterar(&cfg)
		}
		if got := claves(ValidarCambioDeEstado(cfg, c.destino, c.motivo, antes)); got != c.errores {
			t.Errorf("%s: %s, se esperaba %s", c.nombre, got, c.errores)
		}
	}
	// Retroceder a una etapa cuya fecha ya venció se rechaza: el calendario la
	// volvería a avanzar de inmediato.
	despues := time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC) // la postulación venció el 31 de enero
	cfg := configuracionCompleta(EstadoRevisionDocumental)
	if got := claves(ValidarCambioDeEstado(cfg, EstadoPostulacion, "Prórroga", despues)); got != "[error.estado_fechas]" {
		t.Errorf("retroceso contra el calendario: %s", got)
	}
}

// El avance por calendario: sólo hacia adelante y de uno en uno.
func TestProximoEstadoAutomatico(t *testing.T) {
	c := convocatoriaDePrueba(1, TipoInterpretes, EstadoBorrador)
	// Calendario de prueba: postulación en enero, revisión en febrero, evaluación en marzo de 2027.
	dia := func(mes time.Month, d int) time.Time { return time.Date(2027, mes, d, 12, 0, 0, 0, time.UTC) }
	casos := []struct {
		estado  EstadoConvocatoria
		ahora   time.Time
		destino EstadoConvocatoria // "" = no toca avanzar
	}{
		{EstadoBorrador, dia(6, 1), ""}, // publicar es siempre manual
		{EstadoPublicada, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), ""},
		{EstadoPublicada, dia(1, 1), EstadoPostulacion},
		{EstadoPostulacion, dia(1, 15), ""},
		{EstadoPostulacion, dia(1, 31), EstadoRevisionDocumental}, // al vencer el plazo
		{EstadoRevisionDocumental, dia(2, 20), ""},
		{EstadoRevisionDocumental, dia(3, 1), EstadoEvaluacion},
		{EstadoEvaluacion, dia(3, 15), ""},
		{EstadoEvaluacion, dia(3, 31), EstadoCerrada},
		{EstadoCerrada, dia(12, 31), ""}, // archivar es siempre manual
		{EstadoArchivada, dia(12, 31), ""},
		{EstadoPublicada, dia(6, 1), EstadoPostulacion}, // de uno en uno, aunque hayan vencido todas
	}
	for _, caso := range casos {
		c.Estado = caso.estado
		destino, toca := ProximoEstadoAutomatico(c, caso.ahora)
		if destino != caso.destino || toca != (caso.destino != "") {
			t.Errorf("%s el %s: %q (%v), se esperaba %q", caso.estado, caso.ahora.Format("2006-01-02"), destino, toca, caso.destino)
		}
	}
	// Sin calendario no hay avance automático.
	c.Estado, c.Etapas = EstadoPublicada, nil
	if _, toca := ProximoEstadoAutomatico(c, dia(6, 1)); toca {
		t.Error("sin calendario no debe avanzar")
	}
}

// La regla específica de una dotación sobrescribe a la general (RN-07).
func TestRequisitosAplicablesPorDotacion(t *testing.T) {
	requisitos := []RequisitoDocumental{
		{ID: 1, Clave: "fotografia", Ambito: AmbitoPostulacion, Obligatorio: true, Orden: 1},
		{ID: 2, Clave: "partitura", Ambito: AmbitoObra, Obligatorio: true, Orden: 2},
		{ID: 3, Clave: "particellas", Ambito: AmbitoObra, Obligatorio: true, Orden: 3},
		{ID: 4, Clave: "audio", Ambito: AmbitoObra, Obligatorio: false, Orden: 4},
		{ID: 5, DotacionID: 7, Clave: "particellas", Ambito: AmbitoObra, Obligatorio: false, Orden: 3}, // electroacústica
		{ID: 6, DotacionID: 7, Clave: "audio", Ambito: AmbitoObra, Obligatorio: true, Orden: 4},
		{ID: 7, DotacionID: 7, Clave: "patch", Ambito: AmbitoObra, Obligatorio: true, Orden: 5}, // sólo de esta dotación
		{ID: 8, DotacionID: 9, Clave: "partitura", Ambito: AmbitoObra, Obligatorio: false, Orden: 2},
	}
	resumen := func(dotacionID int64) string {
		var lista []string
		for _, rq := range RequisitosAplicables(requisitos, dotacionID) {
			lista = append(lista, fmt.Sprintf("%s:%d:%v", rq.Clave, rq.ID, rq.Obligatorio))
		}
		return fmt.Sprint(lista)
	}
	casos := map[int64]string{
		0: "[fotografia:1:true partitura:2:true particellas:3:true audio:4:false]",              // reglas generales
		3: "[fotografia:1:true partitura:2:true particellas:3:true audio:4:false]",              // dotación sin reglas propias
		7: "[fotografia:1:true partitura:2:true particellas:5:false audio:6:true patch:7:true]", // electroacústica
		9: "[fotografia:1:true partitura:8:false particellas:3:true audio:4:false]",
	}
	for dotacionID, esperado := range casos {
		if got := resumen(dotacionID); got != esperado {
			t.Errorf("dotación %d:\n  %s\nse esperaba:\n  %s", dotacionID, got, esperado)
		}
	}
}

// Los valores iniciales de una convocatoria cumplen sus propias reglas.
func TestValoresInicialesSonValidos(t *testing.T) {
	for _, tipo := range []TipoConvocatoria{TipoInterpretes, TipoCompositores} {
		for _, rq := range RequisitosIniciales(tipo) {
			if errores := ValidarRequisito(rq, ConfiguracionConvocatoria{}); len(errores) != 0 {
				t.Errorf("%s/%s: %s", tipo, rq.Clave, claves(errores))
			}
		}
	}
	for _, c := range CriteriosIniciales() {
		if errores := ValidarCriterio(c); len(errores) != 0 || c.EscalaMin != 1 || c.EscalaMax != 5 || !c.Obligatorio {
			t.Errorf("criterio %s: %s", c.Clave, claves(errores))
		}
	}
}

// Quien postula en una convocatoria no puede ser su jurado ni su revisor, y
// una asignación retirada se reactiva en la misma fila.
func TestAsignacionesEnElRepositorio(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	convocatoriaID, postulanteID, _ := sembrarPostulacion(t, r)
	juradoID := cuentaDePrueba(t, r, "jurado@ejemplo.mx")

	var regla *ErrorValidacion
	_, err := r.Asignar(ctx, actorPrueba, convocatoriaID, postulanteID, PerfilJurado)
	if !errors.As(err, &regla) || claves(regla.Errores) != "[error.asignado_postula]" {
		t.Fatalf("un postulante de la convocatoria no debe poder ser su jurado: %v", err)
	}
	_, err = r.Asignar(ctx, actorPrueba, convocatoriaID, postulanteID, PerfilRevisorDocumental)
	if !errors.As(err, &regla) || claves(regla.Errores) != "[error.asignado_postula]" {
		t.Fatalf("ni su revisor: %v", err)
	}

	id, err := r.Asignar(ctx, actorPrueba, convocatoriaID, juradoID, PerfilJurado)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Asignar(ctx, actorPrueba, convocatoriaID, juradoID, PerfilJurado); !errors.Is(err, ErrDuplicado) {
		t.Errorf("asignación repetida: %v", err)
	}
	if _, err := r.RetirarAsignacion(ctx, actorPrueba, id); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RetirarAsignacion(ctx, actorPrueba, id); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("retirar dos veces: %v", err)
	}
	// La fila se conserva inactiva y se reactiva al volver a asignar.
	if n := contar(t, r, "SELECT COUNT(*) FROM asignacion WHERE id = ? AND activo = 0", id); n != 1 {
		t.Error("la asignación retirada debe conservarse, inactiva")
	}
	otra, err := r.Asignar(ctx, actorPrueba, convocatoriaID, juradoID, PerfilJurado)
	if err != nil || otra != id || contar(t, r, "SELECT COUNT(*) FROM asignacion") != 1 {
		t.Errorf("reasignar debe reactivar la misma fila: id %d (antes %d), %v", otra, id, err)
	}
	// Una cuenta desactivada no puede asignarse.
	inactivaID := cuentaDePrueba(t, r, "inactiva@ejemplo.mx")
	ejecutar(t, r, "UPDATE usuario SET activo = 0 WHERE id = ?", inactivaID)
	if _, err := r.Asignar(ctx, actorPrueba, convocatoriaID, inactivaID, PerfilRevisorDocumental); !errors.As(err, &regla) ||
		claves(regla.Errores) != "[error.usuario_inactivo]" {
		t.Errorf("cuenta desactivada: %v", err)
	}
}

// Una regla incumplida no deja escritura alguna, ni asiento en la bitácora.
func TestReglaIncumplidaNoEscribe(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	edicionID, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2027, NombreES: "Edición 49"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := r.CrearConvocatoria(ctx, actorPrueba, convocatoriaDePrueba(edicionID, TipoInterpretes, EstadoPostulacion))
	if err != nil {
		t.Fatal(err)
	}
	asientos := contar(t, r, "SELECT COUNT(*) FROM auditoria")
	var regla *ErrorValidacion
	if _, err := r.CambiarEstadoConvocatoria(ctx, actorPrueba, id, EstadoRevisionDocumental, "", false); !errors.As(err, &regla) {
		t.Fatalf("se esperaba el rechazo por falta de revisor: %v", err)
	}
	if _, _, err := r.GuardarDotacion(ctx, actorPrueba, Dotacion{ConvocatoriaID: id, NombreES: "Dúo"}); !errors.As(err, &regla) {
		t.Fatalf("se esperaba el rechazo de una dotación en Intérpretes: %v", err)
	}
	if _, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2028, NombreES: "Repetida"}); !errors.Is(err, ErrDuplicado) {
		t.Fatalf("se esperaba ErrDuplicado: %v", err)
	}
	var estado string
	r.lectura.QueryRow("SELECT estado FROM convocatoria WHERE id = ?", id).Scan(&estado)
	if estado != "postulacion" || contar(t, r, "SELECT COUNT(*) FROM dotacion") != 0 || contar(t, r, "SELECT COUNT(*) FROM auditoria") != asientos {
		t.Errorf("una operación rechazada dejó rastro: estado %s", estado)
	}
}
