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
	"time"

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

// datosConArchivos prepara un directorio de datos migrado, con la edición de
// demostración y dos archivos registrados en el acervo. Devuelve el
// directorio y las claves de los archivos.
func datosConArchivos(t *testing.T) (dir string, claves []string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "datos")
	for _, orden := range []func(context.Context, *models.Repositorio) error{migrar, sembrarDemo} {
		if err := conRepositorio(dir, orden); err != nil {
			t.Fatal(err)
		}
	}
	almacen, err := models.AbrirAlmacenEnDisco(directorioDeBlobs(dir))
	if err != nil {
		t.Fatal(err)
	}
	err = conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		for i, clase := range []models.ClaseArchivoConvocatoria{models.ArchivoBasesES, models.ArchivoBasesEN} {
			blob, err := almacen.Guardar(strings.NewReader("%PDF-1.7\nbases de la convocatoria, versión "+string(rune('A'+i))), 1<<20)
			if err != nil {
				return err
			}
			claves = append(claves, blob.Clave)
			if err := repositorio.FijarArchivoDeConvocatoria(ctx, actorCLI(), 1, clase, blob, "application/pdf", "bases.pdf"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir, claves
}

// capturar sustituye la salida de las órdenes por un búfer y lo devuelve.
func capturar(t *testing.T) *strings.Builder {
	t.Helper()
	var texto strings.Builder
	anterior := salida
	salida = &texto
	t.Cleanup(func() { salida = anterior })
	return &texto
}

// «verify» distingue un acervo íntegro de uno con pérdidas o alteraciones.
func TestVerificarElAcervo(t *testing.T) {
	dir, claves := datosConArchivos(t)
	texto := capturar(t)
	if err := conRepositorio(dir, verificar(dir, nil)); err != nil || !strings.Contains(texto.String(), "Archivos registrados: 2 · correctos: 2") ||
		!strings.Contains(texto.String(), "El acervo está íntegro.") || !strings.Contains(texto.String(), "Espacio libre") {
		t.Fatalf("acervo íntegro: %v\n%s", err, texto)
	}
	// Un archivo sin registro no rompe la integridad, pero se reporta.
	huerfano := filepath.Join(directorioDeBlobs(dir), "ab", "cd", "abcd"+strings.Repeat("0", 60))
	if err := os.MkdirAll(filepath.Dir(huerfano), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(huerfano, []byte("x"), 0o440); err != nil {
		t.Fatal(err)
	}
	texto.Reset()
	if err := conRepositorio(dir, verificar(dir, []string{"--rapido"})); err != nil || !strings.Contains(texto.String(), "En disco sin registro en la base (1)") {
		t.Fatalf("huérfano: %v\n%s", err, texto)
	}
	// Se pierde un archivo y se altera otro: la orden termina con error.
	primero, segundo := filepath.Join(directorioDeBlobs(dir), filepath.FromSlash(claves[0])), filepath.Join(directorioDeBlobs(dir), filepath.FromSlash(claves[1]))
	if err := os.Remove(primero); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(segundo, 0o640); err != nil {
		t.Fatal(err)
	}
	contenido, _ := os.ReadFile(segundo)
	contenido[len(contenido)-1]++ // un solo byte, mismo tamaño
	if err := os.WriteFile(segundo, contenido, 0o640); err != nil {
		t.Fatal(err)
	}
	texto.Reset()
	err := conRepositorio(dir, verificar(dir, nil))
	if err == nil || !strings.Contains(err.Error(), "NO está íntegro") || !strings.Contains(texto.String(), "FALTAN en disco (pérdida de datos) (1)") ||
		!strings.Contains(texto.String(), "ALTERADOS: el contenido no es el registrado (1)") {
		t.Fatalf("acervo dañado: %v\n%s", err, texto)
	}
	// La verificación queda en la bitácora.
	err = conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		asientos, _, err := repositorio.ConsultarAuditoria(ctx, models.FiltroAuditoria{Accion: "acervo.verificar", Pagina: 1, Tamano: 10})
		if err == nil && len(asientos) != 3 {
			t.Errorf("asientos de verificación: %d", len(asientos))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// El espejo: instantánea firmada, verificación sin la base en producción, y
// detección de cualquier manipulación.
func TestEspejo(t *testing.T) {
	dir, claves := datosConArchivos(t)
	destino, clave := filepath.Join(t.TempDir(), "espejo"), filepath.Join(t.TempDir(), "espejo.clave")
	texto := capturar(t)
	opciones := []string{"--destino", destino, "--clave", clave}

	// Sin clave no hay instantánea; la clave se genera una sola vez.
	if err := ordenDeEspejo(dir, append([]string{"snapshot"}, opciones...)); err == nil {
		t.Fatal("sin clave no debe tomarse una instantánea")
	}
	if err := ordenDeEspejo(dir, []string{"nueva-clave", "--clave", clave}); err != nil {
		t.Fatal(err)
	}
	if err := ordenDeEspejo(dir, []string{"nueva-clave", "--clave", clave}); err == nil {
		t.Error("la clave no debe sobrescribirse")
	}

	if err := ordenDeEspejo(dir, append([]string{"snapshot"}, opciones...)); err != nil {
		t.Fatalf("snapshot: %v\n%s", err, texto)
	}
	if !strings.Contains(texto.String(), "2 archivos registrados · 2 copiados ahora") || !strings.Contains(texto.String(), "HMAC-SHA256") {
		t.Errorf("salida de snapshot:\n%s", texto)
	}
	nombres, err := models.Instantaneas(destino)
	if err != nil || len(nombres) != 1 {
		t.Fatalf("instantáneas en el destino: %v %v", nombres, err)
	}
	instantanea := filepath.Join(destino, models.DirectorioDeInstantaneas, nombres[0])
	manifiesto := strings.TrimSuffix(instantanea, ".db") + ".json"

	// La verificación no necesita la base en producción: se le da un
	// directorio de datos que no existe.
	sinBase := filepath.Join(t.TempDir(), "no-existe")
	verificarEspejo := func() error { return ordenDeEspejo(sinBase, append([]string{"verify"}, opciones...)) }
	texto.Reset()
	if err := verificarEspejo(); err != nil || !strings.Contains(texto.String(), "Firma HMAC-SHA256 válida · base íntegra") ||
		!strings.Contains(texto.String(), "puede usarse para restaurar") {
		t.Fatalf("verify: %v\n%s", err, texto)
	}
	if _, err := os.Stat(sinBase); !os.IsNotExist(err) {
		t.Error("verificar el espejo no debe crear un directorio de datos")
	}
	// Queda registrada en la base de origen.
	texto.Reset()
	if err := ordenDeEspejo(dir, []string{"list"}); err != nil || !strings.Contains(texto.String(), nombres[0]) || !strings.Contains(texto.String(), "correcta") {
		t.Errorf("list: %v\n%s", err, texto)
	}

	// 1. Falta un archivo en el acervo del espejo.
	blobEspejo := filepath.Join(destino, models.DirectorioDeBlobsEspejo, filepath.FromSlash(claves[0]))
	respaldo, _ := os.ReadFile(blobEspejo)
	if err := os.Remove(blobEspejo); err != nil {
		t.Fatal(err)
	}
	if err := verificarEspejo(); err == nil || !strings.Contains(err.Error(), "NO está completo") {
		t.Errorf("espejo con un archivo de menos: %v", err)
	}
	// Otra instantánea lo repone: la réplica es por contenido.
	time.Sleep(1100 * time.Millisecond) // el nombre de la instantánea lleva el segundo
	texto.Reset()
	if err := ordenDeEspejo(dir, append([]string{"snapshot"}, opciones...)); err != nil || !strings.Contains(texto.String(), "1 copiados ahora") {
		t.Fatalf("segunda instantánea: %v\n%s", err, texto)
	}
	if repuesto, _ := os.ReadFile(blobEspejo); string(repuesto) != string(respaldo) {
		t.Error("la segunda instantánea no repuso el archivo")
	}
	if err := verificarEspejo(); err != nil {
		t.Errorf("tras reponer, la más reciente debe verificar: %v", err)
	}

	// 2. Alguien toca la base de la instantánea.
	verificarLaPrimera := func() error {
		return ordenDeEspejo(sinBase, append([]string{"verify", "--instantanea", nombres[0]}, opciones...))
	}
	original, _ := os.ReadFile(instantanea)
	if err := os.WriteFile(instantanea, append(append([]byte{}, original...), 0), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := verificarLaPrimera(); err == nil || !strings.Contains(err.Error(), "BASE NO ES LA DEL MANIFIESTO") {
		t.Errorf("base alterada: %v", err)
	}
	if err := os.WriteFile(instantanea, original, 0o640); err != nil {
		t.Fatal(err)
	}
	// 3. Alguien toca el manifiesto.
	firmado, _ := os.ReadFile(manifiesto)
	if err := os.WriteFile(manifiesto, []byte(strings.Replace(string(firmado), `"blobs_faltantes": 0`, `"blobs_faltantes": 1`, 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := verificarLaPrimera(); err == nil || !strings.Contains(err.Error(), "FIRMA NO ES VÁLIDA") {
		t.Errorf("manifiesto alterado: %v", err)
	}
	if err := os.WriteFile(manifiesto, firmado, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := verificarLaPrimera(); err != nil {
		t.Errorf("restituida, la primera instantánea debe verificar: %v", err)
	}
	// 4. Otra clave.
	otra := filepath.Join(t.TempDir(), "otra.clave")
	if err := ordenDeEspejo(dir, []string{"nueva-clave", "--clave", otra}); err != nil {
		t.Fatal(err)
	}
	if err := ordenDeEspejo(sinBase, []string{"verify", "--destino", destino, "--clave", otra}); err == nil || !strings.Contains(err.Error(), "FIRMA NO ES VÁLIDA") {
		t.Errorf("verificar con otra clave: %v", err)
	}
	// 5. Una clave que otros pueden leer se rechaza.
	if err := os.Chmod(clave, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verificarEspejo(); err == nil || !strings.Contains(err.Error(), "legible por otros") {
		t.Errorf("clave con permisos abiertos: %v", err)
	}
	if err := ordenDeEspejo(dir, []string{"restaurar"}); err == nil {
		t.Error("una orden de espejo desconocida debe rechazarse")
	}
}

// «export-json» entrega la exportación del portal y «arco» no actúa sin folio.
func TestExportarJSONYArco(t *testing.T) {
	dir, _ := datosConArchivos(t)
	texto := capturar(t)
	archivo := filepath.Join(t.TempDir(), "convocatoria.json")
	if err := conRepositorio(dir, exportarJSON(dir, []string{"--edicion", "49", "--salida", archivo})); err != nil {
		t.Fatal(err)
	}
	exportado, err := os.ReadFile(archivo)
	if err != nil || !strings.Contains(string(exportado), `"numero": 49`) || !strings.Contains(string(exportado), `"bases_md_es"`) ||
		!strings.Contains(string(exportado), `"nomina": null`) || strings.Contains(string(exportado), "@") {
		t.Errorf("exportación a archivo: %v\n%.300s", err, exportado)
	}
	// Sin --salida va a la salida estándar; una edición que no existe, vacía.
	texto.Reset()
	if err := conRepositorio(dir, exportarJSON(dir, []string{"--edicion", "12"})); err != nil || !strings.Contains(texto.String(), `"ediciones": []`) {
		t.Errorf("exportación de una edición inexistente: %v\n%s", err, texto)
	}

	// arco: sin folio, o con un folio que no existe, no hace nada.
	if err := conRepositorio(dir, anonimizar(dir, nil)); err == nil || !strings.Contains(err.Error(), "indique el folio") {
		t.Errorf("arco sin folio: %v", err)
	}
	err = conRepositorio(dir, anonimizar(dir, []string{"FIM49-INT-9999", "--fundamento", "Solicitud ARCO 2026/001"}))
	if err == nil || !strings.Contains(err.Error(), "no existe ninguna postulación con el folio FIM49-INT-9999") {
		t.Errorf("arco con un folio inexistente: %v", err)
	}
	// Los motivos de rechazo se explican en palabras.
	motivos := motivosDeRechazo(&models.ErrorValidacion{Errores: []models.ErrorCampo{
		{Campo: "confirmo", Clave: "error.confirmacion_requerida"}, {Campo: "folio", Clave: "error.folio_no_coincide"}, {Campo: "x", Clave: "error.nuevo"}}})
	if motivos != "no se escribió la palabra ANONIMIZAR; el folio escrito para confirmar no coincide; x: error.nuevo" {
		t.Errorf("motivos de rechazo: %s", motivos)
	}
}
