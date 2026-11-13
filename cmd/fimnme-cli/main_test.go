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

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Recorre el uso real de la herramienta: migrar una base nueva y sembrar la
// edición de demostración, dos veces cada orden.
func TestMigrarYSembrarDemo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "datos") // aún no existe: la crea la herramienta
	ctx := context.Background()

	// Sembrar antes de migrar se rechaza con una indicación clara.
	err := conRepositorio(dir, sembrarDemo)
	if err == nil || !strings.Contains(err.Error(), "fimnme-cli migrate") {
		t.Fatalf("sembrar sin migrar debió rechazarse, se obtuvo: %v", err)
	}

	for i := 0; i < 2; i++ { // la segunda vuelta comprueba que nada se duplica
		if err := conRepositorio(dir, migrar); err != nil {
			t.Fatalf("migrate (vuelta %d): %v", i+1, err)
		}
		if err := conRepositorio(dir, sembrarDemo); err != nil {
			t.Fatalf("sembrar-demo (vuelta %d): %v", i+1, err)
		}
	}

	repo, err := models.AbrirRepositorio(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Cerrar()
	if n, err := repo.ContarEdiciones(ctx); err != nil || n != 1 {
		t.Fatalf("ediciones = %d (%v), se esperaba 1", n, err)
	}
	convocatorias, err := repo.ConvocatoriasPublicas(ctx)
	if err != nil || len(convocatorias) != 2 {
		t.Fatalf("convocatorias públicas = %d (%v), se esperaban 2", len(convocatorias), err)
	}
	for _, c := range convocatorias {
		if c.BasesMDES == "" || c.BasesMDEN == "" || len(c.Etapas) != 3 {
			t.Errorf("convocatoria %s incompleta: %d etapas", c.Tipo, len(c.Etapas))
		}
	}
	compositores, err := repo.ConvocatoriaPublica(ctx, 49, models.TipoCompositores)
	if err != nil || compositores.MaxObras != 2 {
		t.Errorf("Compositores debe limitar a 2 obras: %+v (%v)", compositores.MaxObras, err)
	}
	// La demostración trae la configuración inicial: requisitos, criterios y un
	// catálogo de dotaciones con su excepción (medios electrónicos fijos, sin particellas).
	cfg, err := repo.ConfiguracionDeConvocatoria(ctx, compositores.ID)
	if err != nil || len(cfg.Dotaciones) != 8 || len(cfg.Requisitos) != 5 || len(cfg.Criterios) != 2 {
		t.Fatalf("configuración de demostración incompleta: %d dotaciones, %d requisitos, %d criterios (%v)",
			len(cfg.Dotaciones), len(cfg.Requisitos), len(cfg.Criterios), err)
	}
	for _, rq := range models.RequisitosAplicables(cfg.Requisitos, cfg.Dotaciones[1].ID) {
		if rq.Clave == "particellas" && rq.Obligatorio {
			t.Error("en la obra electroacústica las particellas no deben ser obligatorias")
		}
	}
}

// La identificación del actor lleva el usuario del sistema (RF-CLI-09).
func TestActorDeLaHerramienta(t *testing.T) {
	actor := actorCLI()
	if actor.Perfil != "cli" || !strings.Contains(actor.Agente, "usuario del sistema: ") {
		t.Errorf("actor inesperado: %+v", actor)
	}
}

// Sobre datos ajenos no se trabaja. Sólo puede probarse como root, que es
// justamente el caso que se quiere impedir en el servidor.
func TestNoSeTrabajaSobreDatosDeOtroUsuario(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requiere root para cambiar el dueño de un directorio")
	}
	dir := t.TempDir()
	if err := os.Chown(dir, 12345, 12345); err != nil {
		t.Skipf("no se pudo cambiar el dueño: %v", err)
	}
	err := conRepositorio(dir, migrar)
	if err == nil || !strings.Contains(err.Error(), "pertenece al usuario") {
		t.Fatalf("se esperaba el rechazo por dueño distinto, se obtuvo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, models.NombreBase)); err == nil {
		t.Error("se creó la base a pesar del rechazo")
	}
}

// crear-usuario da de alta una cuenta interna sin contraseña y con enlace de
// activación; rechaza datos incompletos, perfiles no asignables y duplicados.
func TestCrearUsuario(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	if err := conRepositorio(dir, migrar); err != nil {
		t.Fatal(err)
	}
	crear := func(args ...string) error { return conRepositorio(dir, crearUsuario(args)) }

	err := crear("--perfil", "postulante", "--email", "no-es-correo")
	if err == nil {
		t.Fatal("se aceptaron datos inválidos")
	}
	for _, fragmento := range []string{"--perfil debe ser", "--email no es", "--nombre falta", "--apellidos falta"} {
		if !strings.Contains(err.Error(), fragmento) {
			t.Errorf("el error no menciona %q: %v", fragmento, err)
		}
	}
	admin := []string{"--perfil", "admin_sistema", "--email", "admin@ejemplo.mx", "--nombre", "Admin", "--apellidos", "Sistema"}
	if err := crear(admin...); err != nil {
		t.Fatalf("crear administrador: %v", err)
	}
	if err := crear(admin...); err == nil || !strings.Contains(err.Error(), "ya existe una cuenta") {
		t.Errorf("un correo repetido debió rechazarse: %v", err)
	}

	repo, err := models.AbrirRepositorio(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Cerrar()
	usuario, resumen, err := repo.UsuarioPorEmail(ctx, "admin@ejemplo.mx")
	if err != nil || resumen != models.PasswordSinDefinir || usuario.EmailVerificado() {
		t.Errorf("la cuenta debe nacer sin contraseña y sin verificar: %q (%v)", resumen, err)
	}
	// La operación quedó en la bitácora a nombre de la herramienta, sin datos personales.
	asientos, _, err := repo.ConsultarAuditoria(ctx, models.FiltroAuditoria{Accion: "usuario.crear", Pagina: 1, Tamano: 10})
	if err != nil || len(asientos) != 1 || asientos[0].ActorPerfil != models.ActorCLI ||
		!strings.Contains(asientos[0].Despues, `"perfil":"admin_sistema"`) || strings.Contains(asientos[0].Despues, "@") {
		t.Errorf("asiento del alta incorrecto: %+v (%v)", asientos, err)
	}
}
