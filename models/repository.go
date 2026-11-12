// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	// Controlador SQLite 3 escrito en Go puro: permite compilar con
	// CGO_ENABLED=0 y obtener un binario estático (ADR 0003).
	_ "modernc.org/sqlite"
)

// Este archivo es la ÚNICA puerta a la base de datos (RN-37): ninguna consulta
// SQL vive fuera de él. Cuando llegue la migración a MariaDB, el cambio de
// motor se hace aquí y en ningún otro lugar. Los puntos que dependen de SQLite
// están marcados con «ESPECÍFICO DE SQLITE».
//
// Contenido, en este orden:
//   1. Apertura y conexiones (PRAGMAs, escritura única, grupo de lectura)
//   2. Migraciones
//   3. Auditoría
//   4. Ediciones y convocatorias

// ---------------------------------------------------------------------------
// 1. Apertura y conexiones
// ---------------------------------------------------------------------------

// NombreBase es el archivo de la base dentro del directorio de datos.
const NombreBase = "fimnme.db"

// formatoTiempo es la forma en que se guarda TODA marca de tiempo: texto
// ISO-8601 en UTC, p. ej. «2027-03-14T18:05:00Z» (RN-36). Nunca hora local.
const formatoTiempo = "2006-01-02T15:04:05Z"

// conexionesLectura es el tamaño del grupo de conexiones de sólo lectura.
const conexionesLectura = 8

// pragmas se emiten en CADA conexión nueva, de escritura o de lectura (§7.5).
// ESPECÍFICO DE SQLITE.
var pragmas = []string{
	"busy_timeout(5000)",   // una colisión de escritura es una espera corta, no un error
	"journal_mode(WAL)",    // lecturas y escrituras sin bloqueos mutuos
	"synchronous(NORMAL)",  // seguro con WAL
	"foreign_keys(1)",      // SQLite NO lo activa por omisión (RN-35)
	"mmap_size(268435456)", // 256 MB: la base histórica completa cabe en RAM
	"cache_size(-64000)",   // 64 MB de caché B-Tree
	"temp_store(MEMORY)",   // temporales en RAM
}

// Repositorio da acceso a la base de datos. Mantiene dos grupos de conexiones
// sobre el mismo archivo:
//
//   - escritura: UNA sola conexión. SQLite escribe de una en una; con una
//     conexión única las escrituras hacen fila dentro del proceso en vez de
//     competir entre sí por el cerrojo del archivo.
//   - lectura: varias conexiones marcadas query_only, que no pueden escribir
//     aunque una consulta lo intentara. En modo WAL leen sin esperar a la
//     escritura en curso.
type Repositorio struct {
	escritura *sql.DB
	lectura   *sql.DB
	// ahora es el reloj; las pruebas lo sustituyen para fijar la hora.
	ahora func() time.Time
}

// dsn arma la cadena de conexión con los PRAGMAs. ESPECÍFICO DE SQLITE.
func dsn(ruta string, soloLectura bool) string {
	parametros := url.Values{}
	for _, p := range pragmas {
		parametros.Add("_pragma", p)
	}
	if soloLectura {
		parametros.Add("_pragma", "query_only(1)")
	} else {
		// BEGIN IMMEDIATE: la transacción toma el cerrojo de escritura al
		// empezar, no a media transacción, y así nunca falla por «ascenso».
		parametros.Set("_txlock", "immediate")
	}
	return "file:" + (&url.URL{Path: ruta}).EscapedPath() + "?" + parametros.Encode()
}

// AbrirRepositorio abre (y crea, si no existe) la base en el directorio de
// datos. No aplica migraciones: eso es un acto explícito (Migrar).
func AbrirRepositorio(dirDatos string) (*Repositorio, error) {
	// 0750: la base contiene datos personales; nadie ajeno al servicio la lee.
	if err := os.MkdirAll(dirDatos, 0o750); err != nil {
		return nil, fmt.Errorf("directorio de datos: %w", err)
	}
	ruta, err := filepath.Abs(filepath.Join(dirDatos, NombreBase))
	if err != nil {
		return nil, fmt.Errorf("ruta de la base: %w", err)
	}

	escritura, err := sql.Open("sqlite", dsn(ruta, false))
	if err != nil {
		return nil, fmt.Errorf("abrir base (escritura): %w", err)
	}
	escritura.SetMaxOpenConns(1)
	// La primera conexión crea el archivo y lo deja en modo WAL.
	if err := escritura.Ping(); err != nil {
		escritura.Close()
		return nil, fmt.Errorf("abrir base (escritura) %s: %w", ruta, err)
	}

	lectura, err := sql.Open("sqlite", dsn(ruta, true))
	if err != nil {
		escritura.Close()
		return nil, fmt.Errorf("abrir base (lectura): %w", err)
	}
	lectura.SetMaxOpenConns(conexionesLectura)
	lectura.SetMaxIdleConns(conexionesLectura)
	if err := lectura.Ping(); err != nil {
		escritura.Close()
		lectura.Close()
		return nil, fmt.Errorf("abrir base (lectura) %s: %w", ruta, err)
	}
	return &Repositorio{escritura: escritura, lectura: lectura, ahora: time.Now}, nil
}

// Cerrar libera las conexiones. Debe llamarse al terminar el proceso.
func (r *Repositorio) Cerrar() error {
	return errors.Join(r.lectura.Close(), r.escritura.Close())
}

// Verificar comprueba que la base responde. La usa la sonda /readyz.
func (r *Repositorio) Verificar(ctx context.Context) error {
	var uno int
	if err := r.lectura.QueryRowContext(ctx, "SELECT 1").Scan(&uno); err != nil {
		return fmt.Errorf("la base de datos no responde: %w", err)
	}
	return nil
}

// marca devuelve la hora actual en el formato de almacenamiento (UTC).
func (r *Repositorio) marca() string {
	return r.ahora().UTC().Format(formatoTiempo)
}

// aMarca convierte un instante al formato de almacenamiento (UTC).
func aMarca(t time.Time) string {
	return t.UTC().Format(formatoTiempo)
}

// deMarca interpreta una marca de tiempo leída de la base.
func deMarca(texto string) (time.Time, error) {
	t, err := time.Parse(formatoTiempo, texto)
	if err != nil {
		return time.Time{}, fmt.Errorf("marca de tiempo inválida en la base %q: %w", texto, err)
	}
	return t, nil
}

// enTransaccion ejecuta fn dentro de una transacción de escritura: si fn
// devuelve error se deshace todo; si no, se confirma.
func (r *Repositorio) enTransaccion(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.escritura.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transacción: %w", err)
	}
	// Tras un Commit correcto, Rollback no hace nada.
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmar transacción: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 2. Migraciones
// ---------------------------------------------------------------------------

// Las migraciones son archivos NNNN_nombre.sql incrustados en el binario. Se
// aplican en orden, cada una dentro de su propia transacción, y quedan
// registradas en schema_migration (RF-CLI-01).
//
//go:embed migrations/*.sql
var migracionesFS embed.FS

var nombreMigracion = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// sqlTablaMigraciones crea el registro de migraciones si no existe.
const sqlTablaMigraciones = `
CREATE TABLE IF NOT EXISTS schema_migration (
    version     INTEGER PRIMARY KEY,
    nombre      TEXT NOT NULL,
    sha256      TEXT NOT NULL,   -- del archivo .sql, para detectar alteraciones
    aplicada_at TEXT NOT NULL
)`

// Migracion describe una migración incrustada en el binario.
type Migracion struct {
	Version int
	Nombre  string
	SHA256  string // resumen del archivo .sql
	sql     string
}

// migracionesIncrustadas lee y valida las migraciones del binario: deben
// numerarse 0001, 0002, … sin huecos ni repeticiones.
func migracionesIncrustadas() ([]Migracion, error) {
	archivos, err := fs.Glob(migracionesFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("migraciones: %w", err)
	}
	sort.Strings(archivos)
	migraciones := make([]Migracion, 0, len(archivos))
	for i, archivo := range archivos {
		partes := nombreMigracion.FindStringSubmatch(filepath.Base(archivo))
		if partes == nil {
			return nil, fmt.Errorf("migración con nombre inválido: %s (se espera NNNN_nombre.sql)", archivo)
		}
		version, _ := strconv.Atoi(partes[1])
		if version != i+1 {
			return nil, fmt.Errorf("migraciones fuera de secuencia: se esperaba %04d y se encontró %s", i+1, archivo)
		}
		contenido, err := migracionesFS.ReadFile(archivo)
		if err != nil {
			return nil, fmt.Errorf("migración %s: %w", archivo, err)
		}
		resumen := sha256.Sum256(contenido)
		migraciones = append(migraciones, Migracion{
			Version: version,
			Nombre:  partes[2],
			SHA256:  hex.EncodeToString(resumen[:]),
			sql:     string(contenido),
		})
	}
	return migraciones, nil
}

// migracionesPendientes compara las migraciones del binario con las aplicadas
// en la base y devuelve las que faltan. Falla si la base y el binario no
// cuadran: una migración aplicada cuyo archivo cambió, o una base más nueva
// que el binario.
func (r *Repositorio) migracionesPendientes(ctx context.Context, disponibles []Migracion) ([]Migracion, error) {
	if _, err := r.escritura.ExecContext(ctx, sqlTablaMigraciones); err != nil {
		return nil, fmt.Errorf("registro de migraciones: %w", err)
	}
	filas, err := r.escritura.QueryContext(ctx,
		"SELECT version, nombre, sha256 FROM schema_migration ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("leer registro de migraciones: %w", err)
	}
	defer filas.Close()

	aplicadas := 0
	for filas.Next() {
		var version int
		var nombre, resumen string
		if err := filas.Scan(&version, &nombre, &resumen); err != nil {
			return nil, fmt.Errorf("leer registro de migraciones: %w", err)
		}
		aplicadas++
		if version != aplicadas {
			return nil, fmt.Errorf("registro de migraciones con huecos: se esperaba %04d y está %04d", aplicadas, version)
		}
		if version > len(disponibles) {
			return nil, fmt.Errorf("la base tiene la migración %04d_%s, que este binario no conoce: "+
				"el binario es anterior a la base", version, nombre)
		}
		if disponibles[version-1].SHA256 != resumen {
			return nil, fmt.Errorf("la migración %04d_%s ya aplicada no coincide con la del binario: "+
				"una migración aplicada nunca se edita", version, nombre)
		}
	}
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("leer registro de migraciones: %w", err)
	}
	return disponibles[aplicadas:], nil
}

// MigracionesPendientes devuelve las migraciones que faltan por aplicar. El
// servidor web la consulta al arrancar y se niega a servir con el esquema
// desactualizado.
func (r *Repositorio) MigracionesPendientes(ctx context.Context) ([]Migracion, error) {
	disponibles, err := migracionesIncrustadas()
	if err != nil {
		return nil, err
	}
	return r.migracionesPendientes(ctx, disponibles)
}

// Migrar aplica en orden las migraciones pendientes y devuelve las que aplicó.
// Cada migración va en su propia transacción junto con su registro y su
// asiento de auditoría: o entra completa o no entra.
func (r *Repositorio) Migrar(ctx context.Context, actor Actor) ([]Migracion, error) {
	disponibles, err := migracionesIncrustadas()
	if err != nil {
		return nil, err
	}
	pendientes, err := r.migracionesPendientes(ctx, disponibles)
	if err != nil {
		return nil, err
	}
	for i, m := range pendientes {
		err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO schema_migration (version, nombre, sha256, aplicada_at) VALUES (?, ?, ?, ?)",
				m.Version, m.Nombre, m.SHA256, r.marca()); err != nil {
				return err
			}
			// La tabla auditoria existe desde la migración 0001.
			return r.auditar(ctx, tx, actor, "schema.migrar", "schema_migration", int64(m.Version), nil,
				map[string]any{"version": m.Version, "nombre": m.Nombre, "sha256": m.SHA256})
		})
		if err != nil {
			return pendientes[:i], fmt.Errorf("migración %04d_%s: %w", m.Version, m.Nombre, err)
		}
	}
	return pendientes, nil
}

// ---------------------------------------------------------------------------
// 3. Auditoría
// ---------------------------------------------------------------------------

// auditar inserta un asiento en la bitácora DENTRO de la transacción de la
// operación que registra: si la operación se deshace, su asiento también. La
// tabla es de sólo inserción; la base rechaza UPDATE y DELETE (RF-AUD-05).
// antes y despues se guardan como JSON; nil se guarda como NULL.
func (r *Repositorio) auditar(ctx context.Context, tx *sql.Tx, actor Actor,
	accion, entidad string, entidadID int64, antes, despues any) error {
	aJSON := func(valor any) (sql.NullString, error) {
		if valor == nil {
			return sql.NullString{}, nil
		}
		datos, err := json.Marshal(valor)
		if err != nil {
			return sql.NullString{}, fmt.Errorf("auditoría %s: %w", accion, err)
		}
		return sql.NullString{String: string(datos), Valid: true}, nil
	}
	antesJSON, err := aJSON(antes)
	if err != nil {
		return err
	}
	despuesJSON, err := aJSON(despues)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO auditoria (actor_id, actor_perfil, accion, entidad, entidad_id,
		                       antes, despues, ip, user_agent, creado_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nuloSiCero(actor.UsuarioID), nuloSiVacio(actor.Perfil), accion, entidad, nuloSiCero(entidadID),
		antesJSON, despuesJSON, nuloSiVacio(actor.IP), nuloSiVacio(actor.Agente), r.marca())
	if err != nil {
		return fmt.Errorf("auditoría %s: %w", accion, err)
	}
	return nil
}

// nuloSiCero convierte 0 en NULL: en el dominio 0 significa «sin valor».
func nuloSiCero(n int64) sql.NullInt64 {
	return sql.NullInt64{Int64: n, Valid: n != 0}
}

// nuloSiVacio convierte "" en NULL.
func nuloSiVacio(texto string) sql.NullString {
	return sql.NullString{String: texto, Valid: texto != ""}
}

// ---------------------------------------------------------------------------
// 4. Ediciones y convocatorias
// ---------------------------------------------------------------------------

// ContarEdiciones devuelve cuántas ediciones existen.
func (r *Repositorio) ContarEdiciones(ctx context.Context) (int, error) {
	var n int
	if err := r.lectura.QueryRowContext(ctx, "SELECT COUNT(*) FROM edicion").Scan(&n); err != nil {
		return 0, fmt.Errorf("contar ediciones: %w", err)
	}
	return n, nil
}

// CrearEdicion inserta una edición y devuelve su id.
func (r *Repositorio) CrearEdicion(ctx context.Context, actor Actor, e Edicion) (int64, error) {
	var id int64
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			"INSERT INTO edicion (numero, anio, nombre_es, nombre_en, creado_at) VALUES (?, ?, ?, ?, ?)",
			e.Numero, e.Anio, e.NombreES, e.NombreEN, r.marca())
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "edicion.crear", "edicion", id, nil,
			map[string]any{"numero": e.Numero, "anio": e.Anio, "nombre_es": e.NombreES, "nombre_en": e.NombreEN})
	})
	if err != nil {
		return 0, fmt.Errorf("crear edición %d: %w", e.Numero, err)
	}
	return id, nil
}

// CrearConvocatoria inserta una convocatoria con su calendario de etapas, en
// una sola transacción, y devuelve su id. La edición se toma de c.Edicion.ID.
func (r *Repositorio) CrearConvocatoria(ctx context.Context, actor Actor, c Convocatoria) (int64, error) {
	var id int64
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		marca := r.marca()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO convocatoria (edicion_id, tipo, titulo_es, titulo_en, bases_md_es, bases_md_en,
			                          estado, max_obras, max_videos, creado_at, actualizado_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Edicion.ID, string(c.Tipo), c.TituloES, c.TituloEN,
			nuloSiVacio(c.BasesMDES), nuloSiVacio(c.BasesMDEN),
			string(c.Estado), nuloSiCero(int64(c.MaxObras)), c.MaxVideos, marca, marca)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		etapas := make([]map[string]string, 0, len(c.Etapas))
		for _, e := range c.Etapas {
			inicia, termina := aMarca(e.IniciaAt), aMarca(e.TerminaAt)
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO etapa (convocatoria_id, clave, inicia_at, termina_at) VALUES (?, ?, ?, ?)",
				id, string(e.Clave), inicia, termina); err != nil {
				return fmt.Errorf("etapa %s: %w", e.Clave, err)
			}
			etapas = append(etapas, map[string]string{"clave": string(e.Clave), "inicia_at": inicia, "termina_at": termina})
		}
		// Las bases no se copian a la bitácora: son texto largo y ya están en
		// la propia convocatoria.
		return r.auditar(ctx, tx, actor, "convocatoria.crear", "convocatoria", id, nil, map[string]any{
			"edicion_id": c.Edicion.ID, "tipo": c.Tipo, "titulo_es": c.TituloES, "titulo_en": c.TituloEN,
			"estado": c.Estado, "max_obras": c.MaxObras, "max_videos": c.MaxVideos, "etapas": etapas,
		})
	})
	if err != nil {
		return 0, fmt.Errorf("crear convocatoria %s: %w", c.Tipo, err)
	}
	return id, nil
}

// sqlConvocatoria selecciona una convocatoria con los datos de su edición.
// COALESCE convierte los NULL en el valor «vacío» del dominio.
const sqlConvocatoria = `
SELECT c.id, c.tipo, c.titulo_es, c.titulo_en,
       COALESCE(c.bases_md_es, ''), COALESCE(c.bases_md_en, ''),
       c.estado, COALESCE(c.max_obras, 0), c.max_videos,
       e.id, e.numero, e.anio, e.nombre_es, e.nombre_en
FROM convocatoria c
JOIN edicion e ON e.id = c.edicion_id`

// fila es lo común a *sql.Row y *sql.Rows, para leer con una sola función.
type fila interface {
	Scan(destino ...any) error
}

// leerConvocatoria vuelca una fila de sqlConvocatoria en la entidad.
func leerConvocatoria(f fila) (Convocatoria, error) {
	var c Convocatoria
	var tipo, estado string
	err := f.Scan(&c.ID, &tipo, &c.TituloES, &c.TituloEN, &c.BasesMDES, &c.BasesMDEN,
		&estado, &c.MaxObras, &c.MaxVideos,
		&c.Edicion.ID, &c.Edicion.Numero, &c.Edicion.Anio, &c.Edicion.NombreES, &c.Edicion.NombreEN)
	c.Tipo, c.Estado = TipoConvocatoria(tipo), EstadoConvocatoria(estado)
	return c, err
}

// etapasDe lee el calendario de una convocatoria, en orden cronológico.
func (r *Repositorio) etapasDe(ctx context.Context, convocatoriaID int64) ([]Etapa, error) {
	filas, err := r.lectura.QueryContext(ctx,
		"SELECT clave, inicia_at, termina_at FROM etapa WHERE convocatoria_id = ? ORDER BY inicia_at",
		convocatoriaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var etapas []Etapa
	for filas.Next() {
		var clave, inicia, termina string
		if err := filas.Scan(&clave, &inicia, &termina); err != nil {
			return nil, err
		}
		e := Etapa{Clave: ClaveEtapa(clave)}
		if e.IniciaAt, err = deMarca(inicia); err != nil {
			return nil, err
		}
		if e.TerminaAt, err = deMarca(termina); err != nil {
			return nil, err
		}
		etapas = append(etapas, e)
	}
	return etapas, filas.Err()
}

// ConvocatoriasPublicas devuelve las convocatorias vigentes que se anuncian en
// la portada, la edición más reciente primero. Qué es «vigente» lo decide
// rules.go, no el SQL: la regla existe en un solo lugar. El filtrado en
// memoria es barato porque el sistema acumula dos convocatorias por año.
func (r *Repositorio) ConvocatoriasPublicas(ctx context.Context) ([]Convocatoria, error) {
	filas, err := r.lectura.QueryContext(ctx, sqlConvocatoria+" ORDER BY e.numero DESC, c.id")
	if err != nil {
		return nil, fmt.Errorf("leer convocatorias: %w", err)
	}
	var vigentes []Convocatoria
	for filas.Next() {
		c, err := leerConvocatoria(filas)
		if err != nil {
			filas.Close()
			return nil, fmt.Errorf("leer convocatorias: %w", err)
		}
		if ConvocatoriaVigente(c) {
			vigentes = append(vigentes, c)
		}
	}
	// Se cierra el cursor antes de pedir las etapas, para no retener una
	// conexión de lectura mientras se usa otra.
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("leer convocatorias: %w", err)
	}
	for i := range vigentes {
		if vigentes[i].Etapas, err = r.etapasDe(ctx, vigentes[i].ID); err != nil {
			return nil, fmt.Errorf("leer etapas: %w", err)
		}
	}
	return vigentes, nil
}

// ConvocatoriaPublica busca una convocatoria por número de edición y tipo. Si
// no existe o está en borrador devuelve ErrNoEncontrado: una convocatoria en
// borrador no debe poder distinguirse de una que no existe.
func (r *Repositorio) ConvocatoriaPublica(ctx context.Context, numeroEdicion int, tipo TipoConvocatoria) (Convocatoria, error) {
	c, err := leerConvocatoria(r.lectura.QueryRowContext(ctx,
		sqlConvocatoria+" WHERE e.numero = ? AND c.tipo = ?", numeroEdicion, string(tipo)))
	if errors.Is(err, sql.ErrNoRows) {
		return Convocatoria{}, ErrNoEncontrado
	}
	if err != nil {
		return Convocatoria{}, fmt.Errorf("leer convocatoria: %w", err)
	}
	if !ConvocatoriaVisibleAlPublico(c) {
		return Convocatoria{}, ErrNoEncontrado
	}
	if c.Etapas, err = r.etapasDe(ctx, c.ID); err != nil {
		return Convocatoria{}, fmt.Errorf("leer etapas: %w", err)
	}
	return c, nil
}
