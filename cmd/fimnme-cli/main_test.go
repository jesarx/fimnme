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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// enlace-activacion da un enlace nuevo, distinto en cada llamada, a una cuenta
// sin activar; rechaza correos inexistentes o inválidos y deja la emisión en
// la bitácora.
func TestEnlaceDeActivacion(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	if err := conRepositorio(dir, migrar); err != nil {
		t.Fatal(err)
	}
	if err := conRepositorio(dir, crearUsuario([]string{"--perfil", "admin_sistema", "--email", "admin@ejemplo.mx", "--nombre", "Admin", "--apellidos", "Sistema"})); err != nil {
		t.Fatal(err)
	}
	enlace := func(args ...string) (string, error) {
		texto := capturar(t)
		err := conRepositorio(dir, enlaceDeActivacion(append(args, "--url-base", "https://postula.ejemplo.mx/")))
		return texto.String(), err
	}
	primero, err := enlace("--email", "ADMIN@ejemplo.mx")
	if err != nil || !strings.Contains(primero, "https://postula.ejemplo.mx/cuenta/activar?token=") || !strings.Contains(primero, "admin@ejemplo.mx") {
		t.Fatalf("enlace: %q (%v)", primero, err)
	}
	segundo, err := enlace("--email", "admin@ejemplo.mx")
	if err != nil || segundo == primero {
		t.Errorf("cada llamada debe dar un enlace nuevo: %q (%v)", segundo, err)
	}
	if _, err := enlace("--email", "nadie@ejemplo.mx"); err == nil || !strings.Contains(err.Error(), "no hay ninguna cuenta") {
		t.Errorf("un correo inexistente debió rechazarse: %v", err)
	}
	if _, err := enlace("--email", "no-es-correo"); err == nil {
		t.Error("un correo inválido debió rechazarse")
	}
	repo, err := models.AbrirRepositorio(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Cerrar()
	asientos, _, err := repo.ConsultarAuditoria(ctx, models.FiltroAuditoria{Accion: "token.emitir", Pagina: 1, Tamano: 10})
	if err != nil || len(asientos) != 2 || asientos[0].ActorPerfil != models.ActorCLI {
		t.Errorf("las emisiones deben quedar en la bitácora: %+v (%v)", asientos, err)
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

// responder sustituye la entrada de las órdenes por las respuestas dadas, una
// por renglón.
func responder(t *testing.T, respuestas ...string) {
	t.Helper()
	anterior := entrada
	entrada = strings.NewReader(strings.Join(respuestas, "\n") + "\n")
	t.Cleanup(func() { entrada = anterior })
}

// «schema»: añade y quita campos sólo dentro de la cerca, y pide confirmar
// antes de borrar.
func TestOrdenDeEsquema(t *testing.T) {
	dir, _ := datosConArchivos(t)
	texto := capturar(t)
	esquema := func(args ...string) error { return conRepositorio(dir, ordenDeEsquema(args)) }

	if err := esquema("list"); err != nil || !strings.Contains(texto.String(), "No hay campos extendidos.") || !strings.Contains(texto.String(), "datos_solista") {
		t.Fatalf("schema list vacío: %v\n%s", err, texto)
	}
	texto.Reset()
	if err := esquema("add", "--tabla", "datos_solista", "--nombre", "como_se_entero", "--etiqueta-es", "¿Cómo te enteraste?",
		"--etiqueta-en", "How did you hear about us?", "--visible", "postulante,jurado"); err != nil {
		t.Fatal(err)
	}
	for _, fragmento := range []string{"Campo añadido: datos_solista.x_como_se_entero", "Se ejecutó: ALTER TABLE datos_solista ADD COLUMN x_como_se_entero TEXT",
		"Clave en reportes: ext.datos_solista.x_como_se_entero", "Aparece en el formulario de postulación"} {
		if !strings.Contains(texto.String(), fragmento) {
			t.Errorf("salida de schema add: falta %q\n%s", fragmento, texto)
		}
	}
	texto.Reset()
	if err := esquema("add", "--tabla", "dotacion", "--nombre", "tarifa", "--tipo", "decimal", "--etiqueta-es", "Tarifa", "--no-exportable"); err != nil ||
		!strings.Contains(texto.String(), "ADD COLUMN x_tarifa REAL") || !strings.Contains(texto.String(), "Lo captura la administración") {
		t.Errorf("campo de configuración: %v\n%s", err, texto)
	}
	texto.Reset()
	if err := esquema("add", "--tabla", "integrante", "--nombre", "anio", "--tipo", "entero", "--etiqueta-es", "Año"); err != nil || !strings.Contains(texto.String(), "AVISO") {
		t.Errorf("campo en una lista: %v\n%s", err, texto)
	}
	// Lo que la cerca rechaza, con su explicación.
	for nombre, caso := range map[string]struct {
		args   []string
		motivo string
	}{
		"tabla probatoria":  {[]string{"add", "--tabla", "evaluacion", "--nombre", "nota", "--etiqueta-es", "Nota"}, "material probatorio"},
		"tabla ajena":       {[]string{"add", "--tabla", "usuario", "--nombre", "nota", "--etiqueta-es", "Nota"}, "no admite campos"},
		"nombre inválido":   {[]string{"add", "--tabla", "obra", "--nombre", "Nota; DROP TABLE acuse", "--etiqueta-es", "Nota"}, "minúsculas"},
		"sin etiqueta":      {[]string{"add", "--tabla", "obra", "--nombre", "nota"}, "--etiqueta-es"},
		"tipo inventado":    {[]string{"add", "--tabla", "obra", "--nombre", "nota", "--etiqueta-es", "Nota", "--tipo", "blob"}, "texto, entero o decimal"},
		"duplicado":         {[]string{"add", "--tabla", "dotacion", "--nombre", "tarifa", "--etiqueta-es", "Tarifa"}, "ya existe el campo"},
		"orden desconocida": {[]string{"rename"}, "desconocida"},
		"sin orden":         {nil, "falta la orden"},
	} {
		if err := esquema(caso.args...); err == nil || !strings.Contains(err.Error(), caso.motivo) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	texto.Reset()
	if err := esquema("list"); err != nil || !strings.Contains(texto.String(), "datos_solista.x_como_se_entero  (texto, exportable)") ||
		!strings.Contains(texto.String(), "Lo ven: también postulante, jurado") || !strings.Contains(texto.String(), "dotacion.x_tarifa  (decimal, no exportable)") {
		t.Errorf("schema list: %v\n%s", err, texto)
	}

	// Eliminar pide escribir el nombre de la columna; y sólo alcanza campos
	// del diccionario.
	responder(t, "x_otra_cosa")
	if err := esquema("drop", "--tabla", "dotacion", "--nombre", "tarifa"); err == nil || !strings.Contains(err.Error(), "no se eliminó nada") {
		t.Errorf("drop sin confirmar: %v", err)
	}
	responder(t, "x_estado")
	if err := esquema("drop", "--tabla", "postulacion", "--nombre", "estado"); err == nil || !strings.Contains(err.Error(), "no existe el campo extendido") {
		t.Errorf("drop de una columna nativa: %v", err)
	}
	responder(t, "x_tarifa")
	texto.Reset()
	if err := esquema("drop", "--tabla", "dotacion", "--nombre", "x_tarifa"); err != nil || !strings.Contains(texto.String(), "ALTER TABLE dotacion DROP COLUMN x_tarifa") {
		t.Errorf("drop confirmado: %v\n%s", err, texto)
	}
	// Los tres ALTER TABLE quedaron en la bitácora, con el usuario del sistema.
	err := conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		asientos, _, err := repositorio.ConsultarAuditoria(ctx, models.FiltroAuditoria{Accion: "schema.alter", Pagina: 1, Tamano: 10})
		if err == nil && len(asientos) != 4 {
			t.Errorf("asientos de DDL: %d", len(asientos))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// «import»: analiza, pregunta y aplica; sin la palabra, no escribe nada.
func TestOrdenDeImportacion(t *testing.T) {
	dir, _ := datosConArchivos(t)
	texto := capturar(t)
	var compositores, interpretes int64
	var existente string
	// dotaciones devuelve los nombres del catálogo de Compositores.
	dotaciones := func() []string {
		var nombres []string
		err := conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
			cfg, err := repositorio.ConfiguracionDeConvocatoria(ctx, compositores)
			for _, d := range cfg.Dotaciones {
				nombres = append(nombres, d.NombreES)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return nombres
	}
	// Un administrador y una cuenta sin perfil, para --como.
	err := conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		convocatorias, err := repositorio.ConvocatoriasPublicas(ctx)
		if err != nil {
			return err
		}
		for _, c := range convocatorias {
			if c.Tipo == models.TipoCompositores {
				compositores = c.ID
			} else {
				interpretes = c.ID
			}
		}
		for correo, perfil := range map[string]models.Perfil{"admin@ejemplo.mx": models.PerfilAdminSistema, "nadie@ejemplo.mx": ""} {
			if _, err := repositorio.CrearUsuario(ctx, actorCLI(), models.NuevoUsuario{Email: correo, Nombre: "Persona", Apellidos: "De Prueba",
				Idioma: models.IdiomaES, PasswordHash: "x", Perfil: perfil}, nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	iniciales := dotaciones()
	if len(iniciales) < 2 {
		t.Fatal("la demostración debe traer al menos dos dotaciones")
	}
	existente = iniciales[0]
	hoja := filepath.Join(t.TempDir(), "dotaciones.csv")
	if err := os.WriteFile(hoja, []byte("nombre_es,nombre_en\n"+iniciales[1]+",\nDotación de prueba,Test ensemble\n"+strings.ToUpper(existente)+" ,\nOtra de prueba,\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	importar := func(args ...string) error { return conRepositorio(dir, importar(dir, args)) }
	base := []string{"--tipo", "dotaciones", "--convocatoria", fmt.Sprint(compositores)}

	// Quién importa: una cuenta de administrador, con nombre.
	for nombre, caso := range map[string]struct {
		args   []string
		motivo string
	}{
		"sin --como":                {append(slices.Clone(base), hoja), "--como"},
		"cuenta inexistente":        {append(slices.Clone(base), "--como", "fantasma@ejemplo.mx", hoja), "no existe ninguna cuenta"},
		"cuenta sin perfil":         {append(slices.Clone(base), "--como", "nadie@ejemplo.mx", hoja), "no es de un administrador"},
		"sin archivo":               {append(slices.Clone(base), "--como", "admin@ejemplo.mx"), "indique el archivo"},
		"tipo desconocido":          {[]string{"--tipo", "postulaciones", "--convocatoria", fmt.Sprint(compositores), "--como", "admin@ejemplo.mx", hoja}, "--tipo debe ser"},
		"dotaciones en Intérpretes": {[]string{"--tipo", "dotaciones", "--convocatoria", fmt.Sprint(interpretes), "--como", "admin@ejemplo.mx", hoja}, "sólo existen en la convocatoria de Compositores"},
		"convocatoria inexistente":  {[]string{"--tipo", "dotaciones", "--convocatoria", "999", "--como", "admin@ejemplo.mx", hoja}, "no existe la convocatoria 999"},
	} {
		if err := importar(caso.args...); err == nil || !strings.Contains(err.Error(), caso.motivo) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	conAdmin := append(slices.Clone(base), "--como", "admin@ejemplo.mx", hoja)

	// Se resuelve todo pero no se escribe la palabra: no entra nada.
	responder(t, "c", "1", "no")
	if err := importar(conAdmin...); err == nil || !strings.Contains(err.Error(), "no se escribió nada") {
		t.Fatalf("importación sin confirmar: %v", err)
	}
	// Se corta la entrada a media resolución: tampoco.
	responder(t, "c")
	if err := importar(conAdmin...); err == nil || !strings.Contains(err.Error(), "cancelada") {
		t.Fatalf("importación interrumpida: %v", err)
	}
	if obtenidas := dotaciones(); !slices.Equal(obtenidas, iniciales) {
		t.Fatalf("una importación sin confirmar escribió en el catálogo: %v", obtenidas)
	}

	// Completa: crear los dos que no se parecen a nada, vincular el que es
	// el mismo escrito distinto, y aplicar.
	texto.Reset()
	responder(t, "c", "x", "1", "IMPORTAR")
	if err := importar(conAdmin...); err != nil {
		t.Fatalf("importación: %v\n%s", err, texto)
	}
	for _, fragmento := range []string{"4 renglones, 3 por resolver", "2 renglones no se parecen a ningún registro existente", "fila 3: Dotación de prueba",
		"[1] " + existente + "  (mismo nombre)", "No es una opción.", "Se van a crear 2 registros, vincular 1 y omitir 0; 1 renglones ya existían tal cual.", "aplicada"} {
		if !strings.Contains(texto.String(), fragmento) {
			t.Errorf("salida de import: falta %q\n%s", fragmento, texto)
		}
	}
	if obtenidas := dotaciones(); len(obtenidas) != len(iniciales)+2 || !slices.Contains(obtenidas, "Dotación de prueba") || !slices.Contains(obtenidas, "Otra de prueba") {
		t.Errorf("catálogo tras importar: %v", obtenidas)
	}

	// Cuentas: la nueva recibe su enlace de activación, que se muestra aquí.
	jurados := filepath.Join(t.TempDir(), "jurado.csv")
	if err := os.WriteFile(jurados, []byte("email;nombre;apellidos\nnora@ejemplo.mx;Nora;Paz\nadmin@ejemplo.mx;Persona;De Prueba\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	texto.Reset()
	responder(t, "c", "o", "IMPORTAR")
	if err := importar("--tipo", "usuarios_jurado", "--convocatoria", fmt.Sprint(interpretes), "--como", "admin@ejemplo.mx", "--url-base", "https://postula.ejemplo.mx/", jurados); err != nil {
		t.Fatalf("importación de jurados: %v\n%s", err, texto)
	}
	if !strings.Contains(texto.String(), "(mismo correo)") || !strings.Contains(texto.String(), "nora@ejemplo.mx\n    https://postula.ejemplo.mx/cuenta/activar?token=") ||
		!strings.Contains(texto.String(), "crear 1 registros, vincular 0 y omitir 1") {
		t.Errorf("salida de la importación de jurados:\n%s", texto)
	}
	err = conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		cfg, err := repositorio.ConfiguracionDeConvocatoria(ctx, interpretes)
		if err == nil && (cfg.Activos(models.PerfilJurado) != 1) {
			t.Errorf("jurados tras importar: %d", cfg.Activos(models.PerfilJurado))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// dialogo abre el modo interactivo, da las respuestas, una por renglón, y
// devuelve lo que la herramienta escribió. Al acabarse las respuestas el
// diálogo termina solo, sin error.
func dialogo(t *testing.T, dir string, respuestas ...string) string {
	t.Helper()
	texto := capturar(t)
	responder(t, respuestas...)
	if err := conRepositorio(dir, modoInteractivo); err != nil {
		t.Fatalf("modo interactivo: %v\n%s", err, texto)
	}
	return texto.String()
}

// El modo interactivo administra las fichas con las mismas reglas que el
// panel: crea una edición y una convocatoria, la configura, no la publica
// hasta que está completa, y deja cada cambio en la bitácora.
func TestModoInteractivo(t *testing.T) {
	dir, _ := datosConArchivos(t)
	ctx := context.Background()
	const intro = ""
	// leer abre la base para comprobar lo que el diálogo dejó.
	leer := func(consulta func(repositorio *models.Repositorio) error) {
		t.Helper()
		err := conRepositorio(dir, func(_ context.Context, repositorio *models.Repositorio) error { return consulta(repositorio) })
		if err != nil {
			t.Fatal(err)
		}
	}
	// nueva devuelve la configuración de la convocatoria de Compositores de la edición 50.
	var edicion50 int64
	nueva := func() (cfg models.ConfiguracionConvocatoria) {
		t.Helper()
		leer(func(repositorio *models.Repositorio) error {
			convocatorias, err := repositorio.ConvocatoriasDeEdicion(ctx, edicion50)
			if err != nil || len(convocatorias) == 0 {
				return fmt.Errorf("la edición 50 no tiene convocatorias (%v)", err)
			}
			cfg, err = repositorio.ConfiguracionDeConvocatoria(ctx, convocatorias[0].ID)
			return err
		})
		return cfg
	}

	// 1. Una edición nueva. Con un año imposible no se guarda, y se dice por qué.
	salio := dialogo(t, dir, "n", "50", "28", "Edición 50 · 2028", intro)
	if !strings.Contains(salio, "No se guardó nada") || !strings.Contains(salio, "anio: ") {
		t.Errorf("una edición con año inválido debió rechazarse:\n%s", salio)
	}
	salio = dialogo(t, dir, "n", "cincuenta", "50", "2028", "Edición 50 · 2028", intro, "s")
	if !strings.Contains(salio, "Escriba un número entero") || !strings.Contains(salio, "Edición creada") {
		t.Errorf("alta de la edición:\n%s", salio)
	}
	leer(func(repositorio *models.Repositorio) error {
		ediciones, err := repositorio.ListarEdiciones(ctx)
		if err != nil || len(ediciones) != 2 || ediciones[0].Numero != 50 || ediciones[0].Anio != 2028 {
			return fmt.Errorf("ediciones tras el alta: %+v (%v)", ediciones, err)
		}
		edicion50 = ediciones[0].ID
		return nil
	})

	// 2. Su convocatoria de Compositores, en blanco: nace en borrador con los
	//    requisitos y criterios iniciales, y no se publica sin bases ni calendario.
	salio = dialogo(t, dir, "1", "n", "compositores", "Convocatoria de Compositores 50", intro,
		"1", "4", "a", "s")
	if !strings.Contains(salio, "Convocatoria creada en borrador") || !strings.Contains(salio, "No se guardó nada") ||
		!strings.Contains(salio, "bases_md_es: ") || !strings.Contains(salio, "etapas: ") {
		t.Errorf("publicar una convocatoria incompleta debió rechazarse con sus motivos:\n%s", salio)
	}
	if cfg := nueva(); cfg.Convocatoria.Estado != models.EstadoBorrador || cfg.Convocatoria.MaxObras != models.MaxObrasCompositores ||
		len(cfg.Requisitos) == 0 || len(cfg.Criterios) != 2 {
		t.Fatalf("convocatoria recién creada: %+v", cfg.Convocatoria)
	}

	// 3. Bases desde un archivo y calendario en hora de la Ciudad de México.
	bases := filepath.Join(t.TempDir(), "bases.md")
	if err := os.WriteFile(bases, []byte("# Bases\n\nTexto de las bases.\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	salio = dialogo(t, dir, "1", "1",
		"2", filepath.Join(t.TempDir(), "no-existe.md"),
		"2", bases, intro,
		"3", "2027-11-01 00:00", "2028-01-31 23:59", "2028-02-01 00:00", "2028-02-28 23:59", "31/03/2028", "2028-03-01 00:00", "2028-03-31 23:59")
	if !strings.Contains(salio, "No se pudo leer el archivo") || !strings.Contains(salio, "Bases actualizadas") ||
		!strings.Contains(salio, "Escriba la fecha como") || !strings.Contains(salio, "Calendario actualizado") {
		t.Errorf("bases y calendario:\n%s", salio)
	}
	cfg := nueva()
	postulacion, _ := cfg.Convocatoria.Etapa(models.EtapaPostulacion)
	if cfg.Convocatoria.BasesMDES != "# Bases\n\nTexto de las bases.\n" || len(cfg.Convocatoria.Etapas) != 3 ||
		!postulacion.IniciaAt.Equal(utc(2027, 11, 1, 6, 0)) || !postulacion.TerminaAt.Equal(utc(2028, 2, 1, 5, 59)) {
		t.Fatalf("bases o calendario mal guardados: %q, %+v", cfg.Convocatoria.BasesMDES, cfg.Convocatoria.Etapas)
	}

	// 4. Dotaciones: alta, rechazo de una sin nombre y baja lógica.
	salio = dialogo(t, dir, "1", "1", "5",
		"n", "Trío de cuerdas", "String trio", intro, intro, intro, intro,
		"n", intro, intro, intro, intro, intro, intro,
		"1", intro, intro, intro, intro, intro, "n")
	if !strings.Contains(salio, "Dotación creada") || !strings.Contains(salio, "nombre_es: ") || !strings.Contains(salio, "[inactiva]") {
		t.Errorf("dotaciones:\n%s", salio)
	}
	if cfg = nueva(); len(cfg.Dotaciones) != 1 || cfg.Dotaciones[0].NombreEN != "String trio" || cfg.Dotaciones[0].Activa {
		t.Fatalf("dotaciones tras el diálogo: %+v", cfg.Dotaciones)
	}

	// 5. Requisitos: alta ligada a la dotación, edición y eliminación (en borrador).
	requisitos := len(cfg.Requisitos)
	ultimo := fmt.Sprint(requisitos + 1)
	salio = dialogo(t, dir, "1", "1", "6",
		"n", "carta", "Carta de motivos", intro, intro, intro, "obra", "7", "1", "s", "application/pdf, image/png", "5", intro, intro,
		ultimo, "e", intro, intro, intro, intro, intro, intro, "0", "n", intro, "12", "2", intro)
	if !strings.Contains(salio, "Elija un número de la lista") || !strings.Contains(salio, "Requisito creado") || !strings.Contains(salio, "Requisito actualizado") {
		t.Errorf("requisitos:\n%s", salio)
	}
	cfg = nueva()
	if carta := cfg.Requisitos[len(cfg.Requisitos)-1]; len(cfg.Requisitos) != requisitos+1 || carta.Clave != "carta" || carta.DotacionID != 0 ||
		carta.Obligatorio || carta.TamanoMaxBytes != 12<<20 || carta.MaxArchivos != 2 || len(carta.MimesPermitidos) != 2 {
		t.Fatalf("requisito tras editarlo: %+v", carta)
	}
	salio = dialogo(t, dir, "1", "1", "6", ultimo, "b", "n", ultimo, "b", "s")
	if cfg = nueva(); !strings.Contains(salio, "Requisito eliminado") || len(cfg.Requisitos) != requisitos {
		t.Errorf("eliminar un requisito (primero sin confirmar, luego confirmando): quedan %d\n%s", len(cfg.Requisitos), salio)
	}

	// 6. Criterios: uno nuevo con su escala.
	salio = dialogo(t, dir, "1", "1", "7", "n", "originalidad", "Originalidad", intro, intro, intro, intro, "10", intro, intro)
	if cfg = nueva(); !strings.Contains(salio, "Criterio creado") || len(cfg.Criterios) != 3 || cfg.Criterios[2].EscalaMax != 10 || !cfg.Criterios[2].Obligatorio {
		t.Errorf("criterios: %+v\n%s", cfg.Criterios, salio)
	}

	// 7. Jurados y revisores: una cuenta nueva con su enlace, una existente
	//    que elige quien opera, y el retiro.
	leer(func(repositorio *models.Repositorio) error {
		_, err := repositorio.CrearUsuario(ctx, actorCLI(), models.NuevoUsuario{Email: "revisora@ejemplo.mx", Nombre: "Rita", Apellidos: "Revisora",
			Idioma: models.IdiomaES, PasswordHash: "x"}, nil)
		return err
	})
	salio = dialogo(t, dir, "1", "1", "8",
		"c", "j", "no-es-correo", "Julia", "Jurado", intro,
		"c", "j", "Julia.Jurado@ejemplo.mx", "Julia", "Jurado", "en",
		"a", "nadie-con-ese-nombre",
		"a", "revisora", "1", "r")
	if !strings.Contains(salio, "email: ") || !strings.Contains(salio, "/cuenta/activar?token=") || !strings.Contains(salio, "Jurado asignado") ||
		!strings.Contains(salio, "Ninguna cuenta coincide") || !strings.Contains(salio, "Revisor documental asignado") ||
		!strings.Contains(salio, "[cuenta sin activar]") {
		t.Errorf("asignaciones:\n%s", salio)
	}
	if cfg = nueva(); cfg.Activos(models.PerfilJurado) != 1 || cfg.Activos(models.PerfilRevisorDocumental) != 1 {
		t.Fatalf("asignaciones tras el diálogo: %+v", cfg.Asignaciones)
	}
	salio = dialogo(t, dir, "1", "1", "8", "1", "s")
	if cfg = nueva(); !strings.Contains(salio, "Asignación retirada") || cfg.Activos(models.PerfilJurado) != 0 {
		t.Errorf("retirar al jurado: %+v\n%s", cfg.Asignaciones, salio)
	}

	// 8. Ya completa, se publica; retroceder exige un motivo.
	salio = dialogo(t, dir, "1", "1", "4", "a", "s", "4", "r", intro, "s")
	if cfg = nueva(); !strings.Contains(salio, "La convocatoria está ahora en: publicada") || !strings.Contains(salio, "motivo: ") ||
		cfg.Convocatoria.Estado != models.EstadoPublicada {
		t.Errorf("publicar y retroceder sin motivo: estado %s\n%s", cfg.Convocatoria.Estado, salio)
	}

	// 9. La otra convocatoria se trae del año anterior: copia la
	//    configuración, no el calendario.
	salio = dialogo(t, dir, "1", "d", "1")
	leer(func(repositorio *models.Repositorio) error {
		convocatorias, err := repositorio.ConvocatoriasDeEdicion(ctx, edicion50)
		if err != nil || len(convocatorias) != 2 || convocatorias[1].Tipo != models.TipoInterpretes ||
			convocatorias[1].Estado != models.EstadoBorrador || len(convocatorias[1].Etapas) != 0 || convocatorias[1].BasesMDES == "" {
			return fmt.Errorf("convocatoria duplicada: %+v (%v)\n%s", convocatorias, err, salio)
		}
		return nil
	})

	// 10. Si la entrada se acaba a media ficha no se guarda nada, y no es un error.
	salio = dialogo(t, dir, "1", "e", "77")
	leer(func(repositorio *models.Repositorio) error {
		edicion, err := repositorio.EdicionPorID(ctx, edicion50)
		if err != nil || edicion.Numero != 50 || !strings.Contains(salio, "Fin de la entrada") {
			return fmt.Errorf("una ficha a medias no debe guardarse: %+v (%v)\n%s", edicion, err, salio)
		}
		// Todo lo hecho quedó en la bitácora, a nombre de la herramienta.
		for accion, minimo := range map[string]int{"edicion.crear": 1, "convocatoria.etapas": 1, "convocatoria.estado": 1} {
			asientos, _, err := repositorio.ConsultarAuditoria(ctx, models.FiltroAuditoria{Accion: accion, Pagina: 1, Tamano: 50})
			if err != nil {
				return err
			}
			deLaHerramienta := 0
			for _, asiento := range asientos {
				if asiento.ActorPerfil == string(models.ActorCLI) && strings.Contains(asiento.Agente, "usuario del sistema: ") {
					deLaHerramienta++
				}
			}
			if deLaHerramienta < minimo {
				return fmt.Errorf("la bitácora no tiene «%s» a nombre de la herramienta (%d asientos)", accion, len(asientos))
			}
		}
		return nil
	})
}

// «export-csv» genera un reporte desde una plantilla guardada, en nombre de
// una cuenta que supervisa, y lo deja en la bitácora a su nombre.
func TestExportarCSV(t *testing.T) {
	dir, _ := datosConArchivos(t)
	texto := capturar(t)
	exportar := func(args ...string) error { return conRepositorio(dir, exportarCSV(args)) }

	// Sin plantillas, lo dice.
	if err := exportar("--listar"); err != nil || !strings.Contains(texto.String(), "No hay plantillas") {
		t.Fatalf("listar sin plantillas: %v\n%s", err, texto)
	}

	// Un administrador, un observador, una cuenta sin perfil, una postulación
	// en Intérpretes y dos plantillas.
	var administrador, interpretes int64
	err := conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		cuentas := make(map[string]int64)
		for correo, perfil := range map[string]models.Perfil{"admin@ejemplo.mx": models.PerfilAdminSistema,
			"observa@ejemplo.mx": models.PerfilCoordinacionNacional, "eva@ejemplo.mx": ""} {
			id, err := repositorio.CrearUsuario(ctx, actorCLI(), models.NuevoUsuario{Email: correo, Nombre: "Eva", Apellidos: "Luna",
				Idioma: models.IdiomaES, PasswordHash: "x", Perfil: perfil}, nil)
			if err != nil {
				return err
			}
			cuentas[correo] = id
		}
		administrador = cuentas["admin@ejemplo.mx"]
		convocatoria, err := repositorio.ConvocatoriaPublica(ctx, 49, models.TipoInterpretes)
		if err != nil {
			return err
		}
		interpretes = convocatoria.ID
		postulacion, err := repositorio.CrearPostulacion(ctx, actorCLI(), interpretes, cuentas["eva@ejemplo.mx"], models.SubcategoriaSolista)
		if err != nil {
			return err
		}
		if _, err := repositorio.GuardarCamposDePostulacion(ctx, cuentas["eva@ejemplo.mx"], postulacion, map[string]string{"datos.nombre_completo": "Eva Luna; pianista"}); err != nil {
			return err
		}
		actor := actorCLI()
		actor.UsuarioID = administrador
		for nombre, consulta := range map[string]string{
			"Avance":     "campos=estado,usuario.email,datos.nombre_completo&separador=punto_y_coma",
			"Resultados": "campos=folio,eval.puntaje&orden=-puntaje",
		} {
			tipo := models.TipoInterpretes
			if _, err := repositorio.GuardarPlantillaDeReporte(ctx, actor, models.PlantillaDeReporte{Nombre: nombre, Tipo: tipo, Consulta: consulta}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	texto.Reset()
	if err := exportar("--listar"); err != nil || !strings.Contains(texto.String(), "Avance") || !strings.Contains(texto.String(), "interpretes") {
		t.Fatalf("listar las plantillas: %v\n%s", err, texto)
	}

	// Lo que falta o no puede ser, con su explicación.
	for motivo, args := range map[string][]string{
		"uso: fimnme-cli export-csv":          {"--plantilla", "Avance"},
		"indique con --como":                  {"--plantilla", "Avance", "--edicion", "49"},
		"no existe ninguna cuenta":            {"--plantilla", "Avance", "--edicion", "49", "--como", "fantasma@ejemplo.mx"},
		"no puede generar reportes":           {"--plantilla", "Avance", "--edicion", "49", "--como", "eva@ejemplo.mx"},
		"no existe ninguna plantilla llamada": {"--plantilla", "Inventada", "--edicion", "49", "--como", "admin@ejemplo.mx"},
		"no existe la plantilla 999":          {"--plantilla", "999", "--edicion", "49", "--como", "admin@ejemplo.mx"},
		"no existe la edición 12":             {"--plantilla", "Avance", "--edicion", "12", "--como", "admin@ejemplo.mx"},
		"--idioma debe ser":                   {"--plantilla", "Avance", "--edicion", "49", "--como", "admin@ejemplo.mx", "--idioma", "fr"},
		"el reporte no puede generarse":       {"--plantilla", "Avance", "--convocatoria", fmt.Sprint(interpretes + 1), "--como", "admin@ejemplo.mx"},
		"no existe la convocatoria 999":       {"--plantilla", "Avance", "--convocatoria", "999", "--como", "admin@ejemplo.mx"},
	} {
		if err := exportar(args...); err == nil || !strings.Contains(err.Error(), motivo) {
			t.Errorf("%v: se esperaba «%s», se obtuvo: %v", args, motivo, err)
		}
	}

	// A la salida estándar: el CSV y nada más, con BOM, separador de la
	// plantilla, comillas donde hacen falta y CRLF.
	texto.Reset()
	if err := exportar("--plantilla", "Avance", "--edicion", "49", "--como", "Admin@Ejemplo.mx"); err != nil {
		t.Fatal(err)
	}
	if esperado := "\xEF\xBB\xBFEstado;Correo de la cuenta;Nombre completo\r\nBorrador;eva@ejemplo.mx;\"Eva Luna; pianista\"\r\n"; texto.String() != esperado {
		t.Errorf("reporte a la salida estándar:\n%q\nse esperaba\n%q", texto.String(), esperado)
	}

	// A un archivo, en inglés y por id de convocatoria, como observador: nace
	// privado y no deja restos.
	archivo := filepath.Join(t.TempDir(), "avance.csv")
	texto.Reset()
	if err := exportar("--plantilla", "Avance", "--convocatoria", fmt.Sprint(interpretes), "--como", "observa@ejemplo.mx", "--salida", archivo, "--idioma", "en"); err != nil {
		t.Fatal(err)
	}
	contenido, err := os.ReadFile(archivo)
	if err != nil || !strings.Contains(string(contenido), "Draft;eva@ejemplo.mx") || !strings.Contains(texto.String(), "1 renglón.") {
		t.Errorf("reporte a un archivo: %v\n%s\n%s", err, contenido, texto)
	}
	if info, err := os.Stat(archivo); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("el reporte debe nacer privado (0600): %v", info.Mode())
	}
	if _, err := os.Stat(archivo + ".parcial"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("quedó el archivo parcial: %v", err)
	}

	// Una plantilla con calificaciones pasa por la puerta de la supervisión, y
	// cada generación queda en la bitácora a nombre de la cuenta.
	texto.Reset()
	if err := exportar("--plantilla", "Resultados", "--edicion", "49", "--como", "admin@ejemplo.mx"); err != nil || !strings.Contains(texto.String(), "Folio") {
		t.Errorf("reporte con calificaciones: %v\n%s", err, texto)
	}
	err = conRepositorio(dir, func(ctx context.Context, repositorio *models.Repositorio) error {
		asientos, total, err := repositorio.ConsultarAuditoria(ctx, models.FiltroAuditoria{ActorID: administrador, Entidad: "convocatoria", EntidadID: interpretes, Pagina: 1, Tamano: 50})
		if err != nil {
			return err
		}
		reportes := 0
		for _, asiento := range asientos {
			if strings.HasPrefix(asiento.Accion, "reporte.") && asiento.ActorPerfil == string(models.ActorCLI) {
				reportes++
			}
		}
		if reportes != 2 {
			return fmt.Errorf("se esperaban 2 reportes del administrador en la bitácora, hay %d de %d asientos: %+v", reportes, total, asientos)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
