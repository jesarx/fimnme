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
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// actorPrueba es el actor que firma las operaciones en estas pruebas.
var actorPrueba = Actor{Perfil: "cli", Agente: "pruebas"}

// repoDePrueba abre una base nueva en un directorio temporal y le aplica todas
// las migraciones. Cada prueba trabaja sobre su propia base.
func repoDePrueba(t *testing.T) *Repositorio {
	t.Helper()
	r, err := AbrirRepositorio(t.TempDir())
	if err != nil {
		t.Fatalf("abrir repositorio: %v", err)
	}
	t.Cleanup(func() { r.Cerrar() })
	if _, err := r.Migrar(context.Background(), actorPrueba); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	return r
}

// ejecutar lanza una sentencia de escritura directa. Sólo para preparar datos
// de prueba en tablas que aún no tienen métodos de repositorio.
func ejecutar(t *testing.T, r *Repositorio, consulta string, args ...any) int64 {
	t.Helper()
	res, err := r.escritura.Exec(consulta, args...)
	if err != nil {
		t.Fatalf("%s: %v", consulta, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// contar devuelve el resultado entero de una consulta de una sola celda.
func contar(t *testing.T, r *Repositorio, consulta string, args ...any) int {
	t.Helper()
	var n int
	if err := r.lectura.QueryRow(consulta, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", consulta, err)
	}
	return n
}

// convocatoriaDePrueba arma una convocatoria con su calendario completo.
func convocatoriaDePrueba(edicionID int64, tipo TipoConvocatoria, estado EstadoConvocatoria) Convocatoria {
	dia := func(mes time.Month, d int) time.Time { return time.Date(2027, mes, d, 6, 0, 0, 0, time.UTC) }
	maxObras := 0 // Intérpretes: sin límite
	if tipo == TipoCompositores {
		maxObras = MaxObrasCompositores
	}
	return Convocatoria{
		MaxObras:  maxObras,
		Edicion:   Edicion{ID: edicionID},
		Tipo:      tipo,
		TituloES:  "Convocatoria de " + string(tipo),
		TituloEN:  "Call for " + string(tipo),
		BasesMDES: "# Bases",
		Estado:    estado,
		MaxVideos: 3,
		Etapas: []Etapa{
			// Desordenadas a propósito: la lectura debe devolverlas en orden.
			{Clave: EtapaEvaluacion, IniciaAt: dia(3, 1), TerminaAt: dia(3, 31)},
			{Clave: EtapaPostulacion, IniciaAt: dia(1, 1), TerminaAt: dia(1, 31)},
			{Clave: EtapaRevisionDocumental, IniciaAt: dia(2, 1), TerminaAt: dia(2, 28)},
		},
	}
}

// sembrarPostulacion crea lo mínimo para tener una postulación: edición,
// convocatoria, usuario y la postulación en borrador. Devuelve sus ids.
func sembrarPostulacion(t *testing.T, r *Repositorio) (convocatoriaID, usuarioID, postulacionID int64) {
	t.Helper()
	ctx := context.Background()
	edicionID, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2027, NombreES: "Edición 49", NombreEN: "49th Edition"})
	if err != nil {
		t.Fatal(err)
	}
	convocatoriaID, err = r.CrearConvocatoria(ctx, actorPrueba, convocatoriaDePrueba(edicionID, TipoCompositores, EstadoEvaluacion))
	if err != nil {
		t.Fatal(err)
	}
	usuarioID = ejecutar(t, r, `INSERT INTO usuario (email, password_hash, nombre, apellidos, creado_at, actualizado_at)
		VALUES ('ana@ejemplo.mx', 'x', 'Ana', 'López', '2027-01-01T00:00:00Z', '2027-01-01T00:00:00Z')`)
	postulacionID = ejecutar(t, r, `INSERT INTO postulacion (convocatoria_id, usuario_id, creado_at, actualizado_at)
		VALUES (?, ?, '2027-01-01T00:00:00Z', '2027-01-01T00:00:00Z')`, convocatoriaID, usuarioID)
	return convocatoriaID, usuarioID, postulacionID
}

// --- Migraciones ------------------------------------------------------------

func TestMigrarCreaElEsquemaYEsIdempotente(t *testing.T) {
	ctx := context.Background()
	r, err := AbrirRepositorio(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Cerrar()

	pendientes, err := r.MigracionesPendientes(ctx)
	if err != nil || len(pendientes) == 0 {
		t.Fatalf("una base nueva debe tener migraciones pendientes: %v, %v", pendientes, err)
	}
	aplicadas, err := r.Migrar(ctx, actorPrueba)
	if err != nil {
		t.Fatalf("migrar: %v", err)
	}
	if len(aplicadas) != len(pendientes) || aplicadas[0].Version != 1 || aplicadas[0].Nombre != "esquema_inicial" {
		t.Fatalf("migraciones aplicadas inesperadas: %+v", aplicadas)
	}
	// Segunda corrida: nada que hacer.
	if otra, err := r.Migrar(ctx, actorPrueba); err != nil || len(otra) != 0 {
		t.Fatalf("la segunda migración no debe aplicar nada: %v, %v", otra, err)
	}
	if pendientes, err := r.MigracionesPendientes(ctx); err != nil || len(pendientes) != 0 {
		t.Fatalf("no deben quedar pendientes: %v, %v", pendientes, err)
	}
	// Cada migración deja su registro y su asiento de auditoría (RF-CLI-09).
	if n := contar(t, r, "SELECT COUNT(*) FROM schema_migration"); n != len(aplicadas) {
		t.Errorf("schema_migration tiene %d filas, se esperaban %d", n, len(aplicadas))
	}
	if n := contar(t, r, "SELECT COUNT(*) FROM auditoria WHERE accion = 'schema.migrar' AND actor_perfil = 'cli'"); n != len(aplicadas) {
		t.Errorf("auditoría de migraciones: %d asientos, se esperaban %d", n, len(aplicadas))
	}
}

// El esquema debe contener todas las tablas de la sección 8.3.
func TestEsquemaCompleto(t *testing.T) {
	r := repoDePrueba(t)
	tablas := []string{
		"usuario", "usuario_perfil", "sesion", "token", "edicion", "archivo", "convocatoria", "etapa",
		"asignacion", "toma_revision", "dotacion", "requisito_documental", "criterio_evaluacion",
		"postulacion", "datos_solista", "datos_ensamble", "integrante", "datos_compositor", "enlace_video",
		"propuesta_concierto", "obra", "documento", "revision_documento", "observacion_revision",
		"subsanacion", "subsanacion_problema", "solicitud_desbloqueo", "acuse", "evaluacion",
		"evaluacion_puntaje", "indice_busqueda", "fts_postulacion", "fts_postulacion_trgm", "vista_guardada",
		"tarea_exportacion", "descarga", "importacion", "importacion_conflicto", "campo_extendido",
		"plantilla_correo", "email_outbox", "exportacion_interop", "snapshot_espejo", "auditoria",
	}
	for _, tabla := range tablas {
		if n := contar(t, r, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", tabla); n != 1 {
			t.Errorf("falta la tabla %s", tabla)
		}
	}
	if n := contar(t, r, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'view' AND name = 'v_resultado'"); n != 1 {
		t.Error("falta la vista v_resultado")
	}
}

// Una migración ya aplicada no se edita: si su resumen cambia, el sistema se
// niega a continuar. Tampoco se admite un binario anterior a la base.
func TestMigracionesInconsistentesSeRechazan(t *testing.T) {
	ctx := context.Background()

	r := repoDePrueba(t)
	ejecutar(t, r, "UPDATE schema_migration SET sha256 = 'alterado' WHERE version = 1")
	if _, err := r.MigracionesPendientes(ctx); err == nil || !strings.Contains(err.Error(), "nunca se edita") {
		t.Errorf("se esperaba el rechazo de una migración alterada, se obtuvo: %v", err)
	}

	r = repoDePrueba(t)
	siguiente := contar(t, r, "SELECT MAX(version) FROM schema_migration") + 1
	ejecutar(t, r, "INSERT INTO schema_migration (version, nombre, sha256, aplicada_at) VALUES (?, 'del_futuro', 'x', 'x')", siguiente)
	if _, err := r.MigracionesPendientes(ctx); err == nil || !strings.Contains(err.Error(), "anterior a la base") {
		t.Errorf("se esperaba el rechazo de una base más nueva que el binario, se obtuvo: %v", err)
	}
}

// --- Conexiones ---------------------------------------------------------------

// Los PRAGMAs deben estar activos en CADA conexión, no sólo en la primera
// (§7.5): sin foreign_keys las llaves foráneas serían decorativas.
func TestPragmasEnCadaConexion(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	esperados := map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
		"synchronous":  "1", // NORMAL
		"temp_store":   "2", // MEMORY
		"cache_size":   "-64000",
		"mmap_size":    "268435456",
	}
	comprobar := func(nombre string, bd *sql.DB, cuantas int, soloLectura string) {
		// Se toman todas las conexiones a la vez para obligar al grupo a abrirlas.
		var conexiones []*sql.Conn
		for i := 0; i < cuantas; i++ {
			c, err := bd.Conn(ctx)
			if err != nil {
				t.Fatalf("%s: %v", nombre, err)
			}
			conexiones = append(conexiones, c)
		}
		for i, c := range conexiones {
			for pragma, esperado := range esperados {
				var valor string
				if err := c.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&valor); err != nil {
					t.Fatalf("%s #%d PRAGMA %s: %v", nombre, i, pragma, err)
				}
				if valor != esperado {
					t.Errorf("%s #%d: PRAGMA %s = %s, se esperaba %s", nombre, i, pragma, valor, esperado)
				}
			}
			var valor string
			if err := c.QueryRowContext(ctx, "PRAGMA query_only").Scan(&valor); err != nil || valor != soloLectura {
				t.Errorf("%s #%d: PRAGMA query_only = %s (%v), se esperaba %s", nombre, i, valor, err, soloLectura)
			}
			c.Close()
		}
	}
	comprobar("escritura", r.escritura, 1, "0")
	comprobar("lectura", r.lectura, conexionesLectura, "1")
}

// El grupo de lectura no puede escribir, ni por error de programación.
func TestElGrupoDeLecturaNoEscribe(t *testing.T) {
	r := repoDePrueba(t)
	_, err := r.lectura.Exec("INSERT INTO edicion (numero, anio, nombre_es, nombre_en, creado_at) VALUES (1, 2027, 'a', 'b', 'c')")
	if err == nil {
		t.Fatal("una conexión de lectura aceptó una escritura")
	}
}

func TestLlavesForaneasActivas(t *testing.T) {
	r := repoDePrueba(t)
	// Directo a la base, sin pasar por las reglas: la rechaza la propia llave foránea.
	_, err := r.escritura.Exec(`INSERT INTO convocatoria (edicion_id, tipo, titulo_es, titulo_en, creado_at, actualizado_at)
		VALUES (999, 'interpretes', 'a', 'b', 'x', 'x')`)
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("la base aceptó una convocatoria de una edición que no existe: %v", err)
	}
	// Y el repositorio lo informa como «no encontrado», antes de llegar a la base.
	c := convocatoriaDePrueba(999, TipoInterpretes, EstadoPublicada)
	if _, err := r.CrearConvocatoria(context.Background(), actorPrueba, c); !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// --- Auditoría ----------------------------------------------------------------

// La bitácora es de sólo inserción, garantizado por la base (RF-AUD-05).
func TestAuditoriaEsDeSoloInsercion(t *testing.T) {
	r := repoDePrueba(t)
	if n := contar(t, r, "SELECT COUNT(*) FROM auditoria"); n == 0 {
		t.Fatal("se esperaba al menos el asiento de la migración")
	}
	for _, sentencia := range []string{"UPDATE auditoria SET accion = 'otra'", "DELETE FROM auditoria"} {
		if _, err := r.escritura.Exec(sentencia); err == nil || !strings.Contains(err.Error(), "solo insercion") {
			t.Errorf("%q debió rechazarse, se obtuvo: %v", sentencia, err)
		}
	}
}

// El asiento de auditoría vive en la misma transacción que la operación: si
// ésta falla, no queda ni el dato ni el asiento.
func TestOperacionFallidaNoDejaRastroParcial(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	edicionID, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2027, NombreES: "a", NombreEN: "b"})
	if err != nil {
		t.Fatal(err)
	}
	c := convocatoriaDePrueba(edicionID, TipoInterpretes, EstadoPublicada)
	// Etapa imposible: termina antes de empezar. La rechaza el CHECK de la tabla.
	c.Etapas[0].TerminaAt = c.Etapas[0].IniciaAt.Add(-time.Hour)
	if _, err := r.CrearConvocatoria(ctx, actorPrueba, c); err == nil {
		t.Fatal("se aceptó una etapa que termina antes de empezar")
	}
	if n := contar(t, r, "SELECT COUNT(*) FROM convocatoria"); n != 0 {
		t.Errorf("quedó una convocatoria a medias: %d", n)
	}
	if n := contar(t, r, "SELECT COUNT(*) FROM etapa"); n != 0 {
		t.Errorf("quedaron etapas huérfanas: %d", n)
	}
	if n := contar(t, r, "SELECT COUNT(*) FROM auditoria WHERE accion = 'convocatoria.crear'"); n != 0 {
		t.Errorf("quedó un asiento de auditoría de una operación deshecha: %d", n)
	}
	// La edición, que sí se creó, tiene su asiento con el actor.
	var perfil, agente, despues string
	err = r.lectura.QueryRow("SELECT actor_perfil, user_agent, despues FROM auditoria WHERE accion = 'edicion.crear' AND entidad_id = ?", edicionID).
		Scan(&perfil, &agente, &despues)
	if err != nil || perfil != "cli" || agente != "pruebas" || !strings.Contains(despues, `"numero":49`) {
		t.Errorf("asiento de edicion.crear incorrecto: %q %q %q (%v)", perfil, agente, despues, err)
	}
}

// --- Ediciones y convocatorias ---------------------------------------------

func TestConvocatoriasPublicas(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	ed49, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2027, NombreES: "Edición 49", NombreEN: "49th Edition"})
	if err != nil {
		t.Fatal(err)
	}
	ed48, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 48, Anio: 2026, NombreES: "Edición 48", NombreEN: "48th Edition"})
	if err != nil {
		t.Fatal(err)
	}
	compositores := convocatoriaDePrueba(ed49, TipoCompositores, EstadoBorrador)
	for _, c := range []Convocatoria{
		convocatoriaDePrueba(ed48, TipoInterpretes, EstadoArchivada),
		convocatoriaDePrueba(ed49, TipoInterpretes, EstadoPostulacion),
		compositores,
	} {
		if _, err := r.CrearConvocatoria(ctx, actorPrueba, c); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := r.ContarEdiciones(ctx); err != nil || n != 2 {
		t.Fatalf("ContarEdiciones = %d, %v", n, err)
	}

	// La portada anuncia sólo lo vigente: ni borradores ni archivadas.
	vigentes, err := r.ConvocatoriasPublicas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(vigentes) != 1 || vigentes[0].Edicion.Numero != 49 || vigentes[0].Tipo != TipoInterpretes {
		t.Fatalf("convocatorias vigentes inesperadas: %+v", vigentes)
	}
	c := vigentes[0]
	if c.Edicion.NombreEN != "49th Edition" || c.TituloES != "Convocatoria de interpretes" || c.BasesMDES != "# Bases" ||
		c.BasesMDEN != "" || c.MaxObras != 0 || c.MaxVideos != 3 || c.Estado != EstadoPostulacion {
		t.Errorf("convocatoria leída con datos incorrectos: %+v", c)
	}
	// Las etapas vuelven en orden cronológico y en UTC, sin pérdida.
	if len(c.Etapas) != 3 || c.Etapas[0].Clave != EtapaPostulacion || c.Etapas[2].Clave != EtapaEvaluacion {
		t.Fatalf("etapas fuera de orden: %+v", c.Etapas)
	}
	inicio := time.Date(2027, 1, 1, 6, 0, 0, 0, time.UTC)
	if !c.Etapas[0].IniciaAt.Equal(inicio) || c.Etapas[0].IniciaAt.Location() != time.UTC {
		t.Errorf("la marca de tiempo no volvió intacta en UTC: %v", c.Etapas[0].IniciaAt)
	}

	// Un borrador es indistinguible de algo inexistente.
	if _, err := r.ConvocatoriaPublica(ctx, 49, TipoCompositores); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("borrador: se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	if _, err := r.ConvocatoriaPublica(ctx, 50, TipoInterpretes); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("inexistente: se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	// Una archivada no se anuncia, pero su enlace permanente sigue resolviendo.
	archivada, err := r.ConvocatoriaPublica(ctx, 48, TipoInterpretes)
	if err != nil || archivada.Estado != EstadoArchivada || len(archivada.Etapas) != 3 {
		t.Errorf("la convocatoria archivada debe seguir consultable: %+v, %v", archivada, err)
	}
	// Una edición no admite dos convocatorias del mismo tipo.
	if _, err := r.CrearConvocatoria(ctx, actorPrueba, convocatoriaDePrueba(ed49, TipoInterpretes, EstadoBorrador)); err == nil {
		t.Error("se aceptó una segunda convocatoria de intérpretes en la misma edición")
	}
}

// Las marcas se guardan como texto ISO-8601 en UTC aunque lleguen en otra zona.
func TestMarcasDeTiempoEnUTC(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	mexico := time.FixedZone("CDMX", -6*3600)
	r.ahora = func() time.Time { return time.Date(2027, 3, 14, 12, 5, 0, 0, mexico) }
	id, err := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2027, NombreES: "a", NombreEN: "b"})
	if err != nil {
		t.Fatal(err)
	}
	var creado string
	if err := r.lectura.QueryRow("SELECT creado_at FROM edicion WHERE id = ?", id).Scan(&creado); err != nil {
		t.Fatal(err)
	}
	if creado != "2027-03-14T18:05:00Z" {
		t.Errorf("creado_at = %q, se esperaba 2027-03-14T18:05:00Z", creado)
	}
}

// --- Piezas del esquema que usarán pasos posteriores ----------------------
// Se prueban desde ahora porque de ellas depende la arquitectura: si el
// controlador sin CGO no trajera FTS5, convenía saberlo en el paso 2.

func TestBusquedaDeTextoCompleto(t *testing.T) {
	r := repoDePrueba(t)
	_, _, postulacionID := sembrarPostulacion(t, r)
	ejecutar(t, r, "INSERT INTO indice_busqueda (postulacion_id, texto, actualizado_at) VALUES (?, ?, 'x')",
		postulacionID, "FIM49-COM-0007 María Núñez Cuarteto Ónix Rapsodia")

	buscar := func(tabla, consulta string) int {
		return contar(t, r, "SELECT COUNT(*) FROM "+tabla+" WHERE "+tabla+" MATCH ?", consulta)
	}
	// Tolerancia a acentos y mayúsculas (unicode61 remove_diacritics 2).
	for _, consulta := range []string{"nunez", "NÚÑEZ", "maria", "onix", "rapso*"} {
		if buscar("fts_postulacion", consulta) != 1 {
			t.Errorf("fts_postulacion no encontró %q", consulta)
		}
	}
	// Coincidencia por subcadena (trigram), para nombres mal recordados.
	for _, consulta := range []string{"arteto", "psodi", "0007"} {
		if buscar("fts_postulacion_trgm", consulta) != 1 {
			t.Errorf("fts_postulacion_trgm no encontró %q", consulta)
		}
	}
	// Los triggers mantienen el índice al actualizar y al borrar.
	ejecutar(t, r, "UPDATE indice_busqueda SET texto = 'FIM49-COM-0007 Julián Carrillo' WHERE postulacion_id = ?", postulacionID)
	if buscar("fts_postulacion", "nunez") != 0 || buscar("fts_postulacion", "julian") != 1 {
		t.Error("el índice no reflejó la actualización")
	}
	if buscar("fts_postulacion_trgm", "arteto") != 0 || buscar("fts_postulacion_trgm", "rrill") != 1 {
		t.Error("el índice de trigramas no reflejó la actualización")
	}
	ejecutar(t, r, "DELETE FROM indice_busqueda WHERE postulacion_id = ?", postulacionID)
	if buscar("fts_postulacion", "julian") != 0 || buscar("fts_postulacion_trgm", "rrill") != 0 {
		t.Error("el índice no reflejó el borrado")
	}
}

// El trigger de respaldo rechaza puntajes fuera de la escala, y la vista de
// resultados suma sólo las evaluaciones enviadas.
func TestPuntajesYVistaDeResultados(t *testing.T) {
	r := repoDePrueba(t)
	convocatoriaID, usuarioID, postulacionID := sembrarPostulacion(t, r)
	obraID := ejecutar(t, r, "INSERT INTO obra (postulacion_id, titulo, creado_at) VALUES (?, 'Obra', 'x')", postulacionID)
	criterios := []int64{
		ejecutar(t, r, "INSERT INTO criterio_evaluacion (convocatoria_id, clave, etiqueta_es, etiqueta_en) VALUES (?, 'realizacion', 'a', 'b')", convocatoriaID),
		ejecutar(t, r, "INSERT INTO criterio_evaluacion (convocatoria_id, clave, etiqueta_es, etiqueta_en) VALUES (?, 'artistico', 'a', 'b')", convocatoriaID),
	}
	evaluar := func(email string, enviada int, puntajes [2]int) int64 {
		jurado := usuarioID
		if email != "" {
			jurado = ejecutar(t, r, `INSERT INTO usuario (email, password_hash, nombre, apellidos, creado_at, actualizado_at)
				VALUES (?, 'x', 'J', 'J', 'x', 'x')`, email)
		}
		evaluacion := ejecutar(t, r, `INSERT INTO evaluacion (convocatoria_id, jurado_id, postulacion_id, obra_id, obra_clave, enviada, creado_at, actualizado_at)
			VALUES (?, ?, ?, ?, ?, ?, 'x', 'x')`, convocatoriaID, jurado, postulacionID, obraID, obraID, enviada)
		for i, p := range puntajes {
			ejecutar(t, r, "INSERT INTO evaluacion_puntaje (evaluacion_id, criterio_id, puntaje) VALUES (?, ?, ?)", evaluacion, criterios[i], p)
		}
		return evaluacion
	}
	evaluar("j1@ejemplo.mx", 1, [2]int{5, 4})
	evaluar("j2@ejemplo.mx", 1, [2]int{3, 3})
	borrador := evaluar("j3@ejemplo.mx", 0, [2]int{1, 1}) // aún no enviada: no cuenta

	var jurados, total int
	err := r.lectura.QueryRow("SELECT jurados_que_evaluaron, puntaje_total FROM v_resultado WHERE postulacion_id = ? AND obra_clave = ?",
		postulacionID, obraID).Scan(&jurados, &total)
	if err != nil || jurados != 2 || total != 15 {
		t.Errorf("v_resultado: jurados=%d total=%d (%v); se esperaban 2 y 15", jurados, total, err)
	}

	// Fuera de la escala 1–5: lo rechaza la base, al insertar y al actualizar.
	if _, err := r.escritura.Exec("UPDATE evaluacion_puntaje SET puntaje = 6 WHERE evaluacion_id = ?", borrador); err == nil {
		t.Error("se aceptó un puntaje de 6 al actualizar")
	}
	if _, err := r.escritura.Exec("DELETE FROM evaluacion_puntaje WHERE evaluacion_id = ?", borrador); err != nil {
		t.Fatal(err)
	}
	if _, err := r.escritura.Exec("INSERT INTO evaluacion_puntaje (evaluacion_id, criterio_id, puntaje) VALUES (?, ?, 0)", borrador, criterios[0]); err == nil {
		t.Error("se aceptó un puntaje de 0 al insertar")
	}
	// Un mismo jurado no puede evaluar dos veces la misma unidad (obra_clave).
	_, err = r.escritura.Exec(`INSERT INTO evaluacion (convocatoria_id, jurado_id, postulacion_id, obra_id, obra_clave, creado_at, actualizado_at)
		SELECT convocatoria_id, jurado_id, postulacion_id, obra_id, obra_clave, 'x', 'x' FROM evaluacion WHERE id = ?`, borrador)
	if err == nil {
		t.Error("se aceptó una segunda evaluación del mismo jurado sobre la misma obra")
	}
}

// --- Escritura concurrente ----------------------------------------------------

// Prueba de humo del supuesto más frágil de la arquitectura: muchas
// escrituras simultáneas, desde este proceso y desde otro (como fimnme-cli
// trabajando mientras el servidor atiende), sin un solo SQLITE_BUSY. No
// sustituye a la prueba de carga formal (D-30).
func TestEscriturasConcurrentesSinErrores(t *testing.T) {
	dir := t.TempDir()
	abrir := func() *Repositorio {
		r, err := AbrirRepositorio(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { r.Cerrar() })
		return r
	}
	web, cli := abrir(), abrir() // dos «procesos» sobre el mismo archivo
	ctx := context.Background()
	if _, err := web.Migrar(ctx, actorPrueba); err != nil {
		t.Fatal(err)
	}

	const escritores, porEscritor = 24, 25
	errores := make(chan error, escritores*porEscritor*2)
	var grupo sync.WaitGroup
	for i := 0; i < escritores; i++ {
		grupo.Add(1)
		go func(i int) {
			defer grupo.Done()
			r := web
			if i%4 == 0 {
				r = cli
			}
			for j := 0; j < porEscritor; j++ {
				err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
					return r.auditar(ctx, tx, actorPrueba, "prueba.carga", "prueba", int64(i*1000+j+1), nil, nil)
				})
				if err != nil {
					errores <- fmt.Errorf("escritor %d: %w", i, err)
				}
				// Lecturas intercaladas: en WAL no esperan a la escritura.
				if err := r.Verificar(ctx); err != nil {
					errores <- err
				}
			}
		}(i)
	}
	grupo.Wait()
	close(errores)
	for err := range errores {
		t.Error(err)
	}
	if n := contar(t, web, "SELECT COUNT(*) FROM auditoria WHERE accion = 'prueba.carga'"); n != escritores*porEscritor {
		t.Errorf("se registraron %d escrituras, se esperaban %d", n, escritores*porEscritor)
	}
}
