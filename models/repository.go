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
	"slices"
	"sort"
	"strconv"
	"strings"
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
//   5. Usuarios
//   6. Sesiones e inicios de sesión
//   7. Enlaces de un solo uso (tokens)
//   8. Correo saliente (bandeja de salida)
//   9. Consulta de la bitácora
//  10. Postulaciones

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

// FijarReloj sustituye el reloj del repositorio. Existe para las pruebas, que
// necesitan adelantar el tiempo (vencimiento de enlaces, sesiones y bloqueos).
func (r *Repositorio) FijarReloj(ahora func() time.Time) {
	r.ahora = ahora
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

// Todo método que escribe aplica ANTES las reglas de rules.go, dentro de la
// misma transacción que la escritura. Así una regla no puede quedar burlada
// por dos peticiones simultáneas (p. ej. dos altas que juntas dejarían cuatro
// jurados). Si una regla no se cumple se devuelve *ErrorValidacion.

// consultor es lo común a *sql.DB y *sql.Tx: permite que una misma función de
// lectura sirva fuera y dentro de una transacción.
type consultor interface {
	QueryContext(ctx context.Context, consulta string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, consulta string, args ...any) *sql.Row
}

// fila es lo común a *sql.Row y *sql.Rows, para leer con una sola función.
type fila interface {
	Scan(destino ...any) error
}

// validacion convierte una lista de problemas en error, o en nil si está vacía.
func validacion(errores []ErrorCampo) error {
	if len(errores) == 0 {
		return nil
	}
	return &ErrorValidacion{Errores: errores}
}

// esDeDominio indica si un error es una respuesta prevista (no encontrado,
// duplicado, regla incumplida) y no un fallo interno que haya que envolver.
func esDeDominio(err error) bool {
	var regla *ErrorValidacion
	var envio *ErrorDeEnvio
	return errors.Is(err, ErrNoEncontrado) || errors.Is(err, ErrDuplicado) || errors.As(err, &regla) || errors.As(err, &envio)
}

// operacion ejecuta una escritura en transacción y, si falla por un motivo
// interno, antepone el nombre de la operación al error.
func (r *Repositorio) operacion(ctx context.Context, nombre string, fn func(tx *sql.Tx) error) error {
	err := r.enTransaccion(ctx, fn)
	if err != nil && !esDeDominio(err) {
		return fmt.Errorf("%s: %w", nombre, err)
	}
	return err
}

// existe indica si una consulta de conteo devuelve al menos una fila.
func existe(ctx context.Context, q consultor, consulta string, args ...any) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, consulta, args...).Scan(&n)
	return n > 0, err
}

// --- Ediciones ----------------------------------------------------------------

const sqlEdicion = "SELECT id, numero, anio, nombre_es, nombre_en FROM edicion"

func leerEdicion(f fila) (Edicion, error) {
	var e Edicion
	err := f.Scan(&e.ID, &e.Numero, &e.Anio, &e.NombreES, &e.NombreEN)
	if errors.Is(err, sql.ErrNoRows) {
		return Edicion{}, ErrNoEncontrado
	}
	return e, err
}

// ContarEdiciones devuelve cuántas ediciones existen.
func (r *Repositorio) ContarEdiciones(ctx context.Context) (int, error) {
	var n int
	if err := r.lectura.QueryRowContext(ctx, "SELECT COUNT(*) FROM edicion").Scan(&n); err != nil {
		return 0, fmt.Errorf("contar ediciones: %w", err)
	}
	return n, nil
}

// ListarEdiciones devuelve todas las ediciones, la más reciente primero.
func (r *Repositorio) ListarEdiciones(ctx context.Context) ([]Edicion, error) {
	filas, err := r.lectura.QueryContext(ctx, sqlEdicion+" ORDER BY numero DESC")
	if err != nil {
		return nil, fmt.Errorf("listar ediciones: %w", err)
	}
	defer filas.Close()
	var ediciones []Edicion
	for filas.Next() {
		e, err := leerEdicion(filas)
		if err != nil {
			return nil, fmt.Errorf("listar ediciones: %w", err)
		}
		ediciones = append(ediciones, e)
	}
	return ediciones, filas.Err()
}

// EdicionPorID devuelve una edición, o ErrNoEncontrado.
func (r *Repositorio) EdicionPorID(ctx context.Context, id int64) (Edicion, error) {
	e, err := leerEdicion(r.lectura.QueryRowContext(ctx, sqlEdicion+" WHERE id = ?", id))
	if err != nil && !errors.Is(err, ErrNoEncontrado) {
		return Edicion{}, fmt.Errorf("leer edición: %w", err)
	}
	return e, err
}

// CrearEdicion inserta una edición y devuelve su id (RF-CONV-01).
func (r *Repositorio) CrearEdicion(ctx context.Context, actor Actor, e Edicion) (int64, error) {
	var id int64
	err := r.operacion(ctx, "crear edición", func(tx *sql.Tx) error {
		if err := validacion(ValidarEdicion(e)); err != nil {
			return err
		}
		if repetida, err := existe(ctx, tx, "SELECT COUNT(*) FROM edicion WHERE numero = ?", e.Numero); err != nil {
			return err
		} else if repetida {
			return ErrDuplicado
		}
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
	return id, err
}

// ActualizarEdicion cambia los datos de una edición. El número queda fijo en
// cuanto alguna de sus convocatorias deja de ser borrador: de él dependen los
// enlaces permanentes (/convocatorias/{número}/…) y los folios.
func (r *Repositorio) ActualizarEdicion(ctx context.Context, actor Actor, e Edicion) error {
	return r.operacion(ctx, "actualizar edición", func(tx *sql.Tx) error {
		antes, err := leerEdicion(tx.QueryRowContext(ctx, sqlEdicion+" WHERE id = ?", e.ID))
		if err != nil {
			return err
		}
		if err := validacion(ValidarEdicion(e)); err != nil {
			return err
		}
		if antes.Numero != e.Numero {
			if publicada, err := existe(ctx, tx,
				"SELECT COUNT(*) FROM convocatoria WHERE edicion_id = ? AND estado <> 'borrador'", e.ID); err != nil {
				return err
			} else if publicada {
				return validacion([]ErrorCampo{{Campo: "numero", Clave: "error.edicion_numero_fijo"}})
			}
			if repetida, err := existe(ctx, tx, "SELECT COUNT(*) FROM edicion WHERE numero = ? AND id <> ?", e.Numero, e.ID); err != nil {
				return err
			} else if repetida {
				return ErrDuplicado
			}
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE edicion SET numero = ?, anio = ?, nombre_es = ?, nombre_en = ? WHERE id = ?",
			e.Numero, e.Anio, e.NombreES, e.NombreEN, e.ID); err != nil {
			return err
		}
		datos := func(x Edicion) map[string]any {
			return map[string]any{"numero": x.Numero, "anio": x.Anio, "nombre_es": x.NombreES, "nombre_en": x.NombreEN}
		}
		return r.auditar(ctx, tx, actor, "edicion.actualizar", "edicion", e.ID, datos(antes), datos(e))
	})
}

// --- Convocatorias: lectura ---------------------------------------------------

// sqlConvocatoria selecciona una convocatoria con los datos de su edición.
// COALESCE convierte los NULL en el valor «vacío» del dominio.
const sqlConvocatoria = `
SELECT c.id, c.tipo, c.titulo_es, c.titulo_en,
       COALESCE(c.bases_md_es, ''), COALESCE(c.bases_md_en, ''),
       c.estado, COALESCE(c.max_obras, 0), COALESCE(c.duracion_max_programa_min, 0), c.max_videos,
       e.id, e.numero, e.anio, e.nombre_es, e.nombre_en
FROM convocatoria c
JOIN edicion e ON e.id = c.edicion_id`

// leerConvocatoria vuelca una fila de sqlConvocatoria en la entidad.
func leerConvocatoria(f fila) (Convocatoria, error) {
	var c Convocatoria
	var tipo, estado string
	err := f.Scan(&c.ID, &tipo, &c.TituloES, &c.TituloEN, &c.BasesMDES, &c.BasesMDEN,
		&estado, &c.MaxObras, &c.DuracionMaxProgramaMin, &c.MaxVideos,
		&c.Edicion.ID, &c.Edicion.Numero, &c.Edicion.Anio, &c.Edicion.NombreES, &c.Edicion.NombreEN)
	c.Tipo, c.Estado = TipoConvocatoria(tipo), EstadoConvocatoria(estado)
	if errors.Is(err, sql.ErrNoRows) {
		return Convocatoria{}, ErrNoEncontrado
	}
	return c, err
}

// etapasDe lee el calendario de una convocatoria, en orden cronológico.
func etapasDe(ctx context.Context, q consultor, convocatoriaID int64) ([]Etapa, error) {
	filas, err := q.QueryContext(ctx,
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

// convocatoriaPorID lee una convocatoria con su calendario, sea cual sea su
// estado. Es de uso interno y de administración: el público pasa por
// ConvocatoriaPublica.
func convocatoriaPorID(ctx context.Context, q consultor, id int64) (Convocatoria, error) {
	c, err := leerConvocatoria(q.QueryRowContext(ctx, sqlConvocatoria+" WHERE c.id = ?", id))
	if err != nil {
		return Convocatoria{}, err
	}
	c.Etapas, err = etapasDe(ctx, q, id)
	return c, err
}

// convocatorias lee las convocatorias que cumplen una condición, con su
// calendario. El cursor se cierra antes de pedir las etapas, para no retener
// una conexión mientras se usa otra.
func convocatorias(ctx context.Context, q consultor, condicion string, args ...any) ([]Convocatoria, error) {
	filas, err := q.QueryContext(ctx, sqlConvocatoria+" "+condicion, args...)
	if err != nil {
		return nil, err
	}
	var lista []Convocatoria
	for filas.Next() {
		c, err := leerConvocatoria(filas)
		if err != nil {
			filas.Close()
			return nil, err
		}
		lista = append(lista, c)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, err
	}
	for i := range lista {
		if lista[i].Etapas, err = etapasDe(ctx, q, lista[i].ID); err != nil {
			return nil, err
		}
	}
	return lista, nil
}

// ConvocatoriasPublicas devuelve las convocatorias vigentes que se anuncian en
// la portada, la edición más reciente primero. Qué es «vigente» lo decide
// rules.go, no el SQL: la regla existe en un solo lugar. El filtrado en
// memoria es barato porque el sistema acumula dos convocatorias por año.
func (r *Repositorio) ConvocatoriasPublicas(ctx context.Context) ([]Convocatoria, error) {
	todas, err := convocatorias(ctx, r.lectura, "ORDER BY e.numero DESC, c.id")
	if err != nil {
		return nil, fmt.Errorf("leer convocatorias: %w", err)
	}
	var vigentes []Convocatoria
	for _, c := range todas {
		if ConvocatoriaVigente(c) {
			vigentes = append(vigentes, c)
		}
	}
	return vigentes, nil
}

// ConvocatoriaPublica busca una convocatoria por número de edición y tipo. Si
// no existe o está en borrador devuelve ErrNoEncontrado: una convocatoria en
// borrador no debe poder distinguirse de una que no existe.
func (r *Repositorio) ConvocatoriaPublica(ctx context.Context, numeroEdicion int, tipo TipoConvocatoria) (Convocatoria, error) {
	lista, err := convocatorias(ctx, r.lectura, "WHERE e.numero = ? AND c.tipo = ?", numeroEdicion, string(tipo))
	if err != nil {
		return Convocatoria{}, fmt.Errorf("leer convocatoria: %w", err)
	}
	if len(lista) == 0 || !ConvocatoriaVisibleAlPublico(lista[0]) {
		return Convocatoria{}, ErrNoEncontrado
	}
	return lista[0], nil
}

// ConvocatoriasDeEdicion devuelve las convocatorias de una edición, en
// cualquier estado. Para administración y supervisión.
func (r *Repositorio) ConvocatoriasDeEdicion(ctx context.Context, edicionID int64) ([]Convocatoria, error) {
	lista, err := convocatorias(ctx, r.lectura, "WHERE c.edicion_id = ? ORDER BY c.id", edicionID)
	if err != nil {
		return nil, fmt.Errorf("leer convocatorias de la edición: %w", err)
	}
	return lista, nil
}

// ConvocatoriasEnCurso devuelve los id de las convocatorias que pueden
// avanzar de etapa por calendario: las publicadas que aún no cierran.
func (r *Repositorio) ConvocatoriasEnCurso(ctx context.Context) ([]int64, error) {
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT id FROM convocatoria
		WHERE estado IN ('publicada', 'postulacion', 'revision_documental', 'evaluacion') ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("leer convocatorias en curso: %w", err)
	}
	defer filas.Close()
	var ids []int64
	for filas.Next() {
		var id int64
		if err := filas.Scan(&id); err != nil {
			return nil, fmt.Errorf("leer convocatorias en curso: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, filas.Err()
}

// --- Configuración: lectura ---------------------------------------------------

func leerDotaciones(ctx context.Context, q consultor, convocatoriaID int64) ([]Dotacion, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT id, convocatoria_id, nombre_es, nombre_en, COALESCE(descripcion_es, ''), COALESCE(descripcion_en, ''), orden, activa
		FROM dotacion WHERE convocatoria_id = ? ORDER BY orden, id`, convocatoriaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var lista []Dotacion
	for filas.Next() {
		var d Dotacion
		if err := filas.Scan(&d.ID, &d.ConvocatoriaID, &d.NombreES, &d.NombreEN, &d.DescripcionES, &d.DescripcionEN, &d.Orden, &d.Activa); err != nil {
			return nil, err
		}
		lista = append(lista, d)
	}
	return lista, filas.Err()
}

func leerRequisitos(ctx context.Context, q consultor, convocatoriaID int64) ([]RequisitoDocumental, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT id, convocatoria_id, dotacion_id, clave, etiqueta_es, etiqueta_en, COALESCE(ayuda_es, ''), COALESCE(ayuda_en, ''),
		       ambito, obligatorio, mimes_permitidos, tamano_max_bytes, max_archivos, orden
		FROM requisito_documental WHERE convocatoria_id = ? ORDER BY orden, id`, convocatoriaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var lista []RequisitoDocumental
	for filas.Next() {
		var rq RequisitoDocumental
		var ambito, mimes string
		if err := filas.Scan(&rq.ID, &rq.ConvocatoriaID, &rq.DotacionID, &rq.Clave, &rq.EtiquetaES, &rq.EtiquetaEN,
			&rq.AyudaES, &rq.AyudaEN, &ambito, &rq.Obligatorio, &mimes, &rq.TamanoMaxBytes, &rq.MaxArchivos, &rq.Orden); err != nil {
			return nil, err
		}
		rq.Ambito = AmbitoRequisito(ambito)
		// mimes_permitidos es un arreglo JSON en texto (§8.3).
		if err := json.Unmarshal([]byte(mimes), &rq.MimesPermitidos); err != nil {
			return nil, fmt.Errorf("requisito %d: mimes_permitidos no es JSON válido: %w", rq.ID, err)
		}
		lista = append(lista, rq)
	}
	return lista, filas.Err()
}

func leerCriterios(ctx context.Context, q consultor, convocatoriaID int64) ([]CriterioEvaluacion, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT id, convocatoria_id, clave, etiqueta_es, etiqueta_en, COALESCE(descripcion_es, ''), COALESCE(descripcion_en, ''),
		       escala_min, escala_max, obligatorio, orden
		FROM criterio_evaluacion WHERE convocatoria_id = ? ORDER BY orden, id`, convocatoriaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var lista []CriterioEvaluacion
	for filas.Next() {
		var c CriterioEvaluacion
		if err := filas.Scan(&c.ID, &c.ConvocatoriaID, &c.Clave, &c.EtiquetaES, &c.EtiquetaEN, &c.DescripcionES,
			&c.DescripcionEN, &c.EscalaMin, &c.EscalaMax, &c.Obligatorio, &c.Orden); err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, filas.Err()
}

func leerAsignaciones(ctx context.Context, q consultor, convocatoriaID int64) ([]AsignacionDetalle, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT `+columnasUsuario+`, u.password_hash, a.id, a.perfil, a.activo
		FROM asignacion a
		JOIN usuario u ON u.id = a.usuario_id
		WHERE a.convocatoria_id = ? ORDER BY a.perfil, a.id`, convocatoriaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var lista []AsignacionDetalle
	for filas.Next() {
		a := AsignacionDetalle{ConvocatoriaID: convocatoriaID}
		var resumen, perfil string
		if a.Usuario, err = leerUsuario(filas, &resumen, &a.ID, &perfil, &a.Activo); err != nil {
			return nil, err
		}
		a.Perfil, a.CuentaActivada = Perfil(perfil), resumen != PasswordSinDefinir
		lista = append(lista, a)
	}
	return lista, filas.Err()
}

// configuracion reúne todo lo configurable de una convocatoria.
func configuracion(ctx context.Context, q consultor, convocatoriaID int64) (ConfiguracionConvocatoria, error) {
	var cfg ConfiguracionConvocatoria
	var err error
	if cfg.Convocatoria, err = convocatoriaPorID(ctx, q, convocatoriaID); err != nil {
		return cfg, err
	}
	if cfg.Dotaciones, err = leerDotaciones(ctx, q, convocatoriaID); err != nil {
		return cfg, err
	}
	if cfg.Requisitos, err = leerRequisitos(ctx, q, convocatoriaID); err != nil {
		return cfg, err
	}
	if cfg.Criterios, err = leerCriterios(ctx, q, convocatoriaID); err != nil {
		return cfg, err
	}
	cfg.Asignaciones, err = leerAsignaciones(ctx, q, convocatoriaID)
	return cfg, err
}

// ConfiguracionDeConvocatoria devuelve la configuración completa de una
// convocatoria en cualquier estado. Para administración y supervisión.
func (r *Repositorio) ConfiguracionDeConvocatoria(ctx context.Context, convocatoriaID int64) (ConfiguracionConvocatoria, error) {
	cfg, err := configuracion(ctx, r.lectura, convocatoriaID)
	if err != nil && !errors.Is(err, ErrNoEncontrado) {
		return cfg, fmt.Errorf("leer configuración de la convocatoria: %w", err)
	}
	return cfg, err
}

// --- Convocatorias: escritura -------------------------------------------------

// insertarEtapas guarda el calendario de una convocatoria.
func insertarEtapas(ctx context.Context, tx *sql.Tx, convocatoriaID int64, etapas []Etapa) ([]map[string]string, error) {
	resumen := make([]map[string]string, 0, len(etapas))
	for _, e := range etapas {
		inicia, termina := aMarca(e.IniciaAt), aMarca(e.TerminaAt)
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO etapa (convocatoria_id, clave, inicia_at, termina_at) VALUES (?, ?, ?, ?)",
			convocatoriaID, string(e.Clave), inicia, termina); err != nil {
			return nil, fmt.Errorf("etapa %s: %w", e.Clave, err)
		}
		resumen = append(resumen, map[string]string{"clave": string(e.Clave), "inicia_at": inicia, "termina_at": termina})
	}
	return resumen, nil
}

func insertarRequisito(ctx context.Context, tx *sql.Tx, rq RequisitoDocumental) (int64, error) {
	mimes, err := json.Marshal(rq.MimesPermitidos)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO requisito_documental (convocatoria_id, dotacion_id, clave, etiqueta_es, etiqueta_en, ayuda_es, ayuda_en,
		                                  ambito, obligatorio, mimes_permitidos, tamano_max_bytes, max_archivos, orden)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rq.ConvocatoriaID, rq.DotacionID, rq.Clave, rq.EtiquetaES, rq.EtiquetaEN, nuloSiVacio(rq.AyudaES), nuloSiVacio(rq.AyudaEN),
		string(rq.Ambito), rq.Obligatorio, string(mimes), rq.TamanoMaxBytes, rq.MaxArchivos, rq.Orden)
	if err != nil {
		return 0, fmt.Errorf("requisito %s: %w", rq.Clave, err)
	}
	return res.LastInsertId()
}

func insertarCriterio(ctx context.Context, tx *sql.Tx, c CriterioEvaluacion) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO criterio_evaluacion (convocatoria_id, clave, etiqueta_es, etiqueta_en, descripcion_es, descripcion_en,
		                                 escala_min, escala_max, obligatorio, orden)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ConvocatoriaID, c.Clave, c.EtiquetaES, c.EtiquetaEN, nuloSiVacio(c.DescripcionES), nuloSiVacio(c.DescripcionEN),
		c.EscalaMin, c.EscalaMax, c.Obligatorio, c.Orden)
	if err != nil {
		return 0, fmt.Errorf("criterio %s: %w", c.Clave, err)
	}
	return res.LastInsertId()
}

func insertarDotacion(ctx context.Context, tx *sql.Tx, d Dotacion) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO dotacion (convocatoria_id, nombre_es, nombre_en, descripcion_es, descripcion_en, orden, activa)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.ConvocatoriaID, d.NombreES, d.NombreEN, nuloSiVacio(d.DescripcionES), nuloSiVacio(d.DescripcionEN), d.Orden, d.Activa)
	if err != nil {
		return 0, fmt.Errorf("dotación %s: %w", d.NombreES, err)
	}
	return res.LastInsertId()
}

// datosConvocatoria es lo que se asienta en la bitácora de una convocatoria.
// Las bases no se copian: son texto largo y ya están en la propia fila.
func datosConvocatoria(c Convocatoria) map[string]any {
	return map[string]any{
		"edicion_id": c.Edicion.ID, "tipo": c.Tipo, "titulo_es": c.TituloES, "titulo_en": c.TituloEN,
		"estado": c.Estado, "max_obras": c.MaxObras, "duracion_max_programa_min": c.DuracionMaxProgramaMin,
		"max_videos": c.MaxVideos,
	}
}

// insertarConvocatoria guarda una convocatoria completa dentro de una
// transacción ya abierta: fila, calendario, requisitos y criterios.
func (r *Repositorio) insertarConvocatoria(ctx context.Context, tx *sql.Tx, c Convocatoria,
	requisitos []RequisitoDocumental, criterios []CriterioEvaluacion) (int64, []map[string]string, error) {
	if _, err := leerEdicion(tx.QueryRowContext(ctx, sqlEdicion+" WHERE id = ?", c.Edicion.ID)); err != nil {
		return 0, nil, err
	}
	if err := validacion(append(ValidarConvocatoria(c), ValidarEtapas(c.Etapas)...)); err != nil {
		return 0, nil, err
	}
	if repetida, err := existe(ctx, tx,
		"SELECT COUNT(*) FROM convocatoria WHERE edicion_id = ? AND tipo = ?", c.Edicion.ID, string(c.Tipo)); err != nil {
		return 0, nil, err
	} else if repetida {
		return 0, nil, ErrDuplicado
	}
	marca := r.marca()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO convocatoria (edicion_id, tipo, titulo_es, titulo_en, bases_md_es, bases_md_en, estado,
		                          max_obras, duracion_max_programa_min, max_videos, creado_at, actualizado_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Edicion.ID, string(c.Tipo), c.TituloES, c.TituloEN, nuloSiVacio(c.BasesMDES), nuloSiVacio(c.BasesMDEN),
		string(c.Estado), nuloSiCero(int64(c.MaxObras)), nuloSiCero(int64(c.DuracionMaxProgramaMin)), c.MaxVideos, marca, marca)
	if err != nil {
		return 0, nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, nil, err
	}
	etapas, err := insertarEtapas(ctx, tx, id, c.Etapas)
	if err != nil {
		return 0, nil, err
	}
	for _, rq := range requisitos {
		rq.ConvocatoriaID = id
		if _, err := insertarRequisito(ctx, tx, rq); err != nil {
			return 0, nil, err
		}
	}
	for _, cr := range criterios {
		cr.ConvocatoriaID = id
		if _, err := insertarCriterio(ctx, tx, cr); err != nil {
			return 0, nil, err
		}
	}
	return id, etapas, nil
}

// CrearConvocatoria inserta una convocatoria con su calendario, sin requisitos
// ni criterios. La edición se toma de c.Edicion.ID y el estado, de c.Estado.
func (r *Repositorio) CrearConvocatoria(ctx context.Context, actor Actor, c Convocatoria) (int64, error) {
	return r.CrearConvocatoriaConfigurada(ctx, actor, c, nil, nil)
}

// CrearConvocatoriaConfigurada inserta, en una sola transacción, una
// convocatoria con su calendario, sus requisitos documentales y sus criterios
// de evaluación iniciales (RF-CONV-02).
func (r *Repositorio) CrearConvocatoriaConfigurada(ctx context.Context, actor Actor, c Convocatoria,
	requisitos []RequisitoDocumental, criterios []CriterioEvaluacion) (int64, error) {
	var id int64
	err := r.operacion(ctx, "crear convocatoria", func(tx *sql.Tx) error {
		var etapas []map[string]string
		var err error
		if id, etapas, err = r.insertarConvocatoria(ctx, tx, c, requisitos, criterios); err != nil {
			return err
		}
		datos := datosConvocatoria(c)
		datos["etapas"] = etapas
		datos["requisitos"], datos["criterios"] = len(requisitos), len(criterios)
		return r.auditar(ctx, tx, actor, "convocatoria.crear", "convocatoria", id, nil, datos)
	})
	return id, err
}

// ActualizarConvocatoria cambia los textos y los límites de una convocatoria
// (RF-CONV-02 y 08). No cambia su tipo, su edición ni su estado.
func (r *Repositorio) ActualizarConvocatoria(ctx context.Context, actor Actor, c Convocatoria) error {
	return r.operacion(ctx, "actualizar convocatoria", func(tx *sql.Tx) error {
		antes, err := convocatoriaPorID(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		c.Tipo, c.Estado, c.Edicion = antes.Tipo, antes.Estado, antes.Edicion
		if err := validacion(append(ValidarEdicionDeConfiguracion(antes.Estado), ValidarConvocatoria(c)...)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE convocatoria
			SET titulo_es = ?, titulo_en = ?, bases_md_es = ?, bases_md_en = ?,
			    max_obras = ?, duracion_max_programa_min = ?, max_videos = ?, actualizado_at = ?
			WHERE id = ?`,
			c.TituloES, c.TituloEN, nuloSiVacio(c.BasesMDES), nuloSiVacio(c.BasesMDEN),
			nuloSiCero(int64(c.MaxObras)), nuloSiCero(int64(c.DuracionMaxProgramaMin)), c.MaxVideos, r.marca(), c.ID); err != nil {
			return err
		}
		despues := datosConvocatoria(c)
		despues["bases_modificadas"] = antes.BasesMDES != c.BasesMDES || antes.BasesMDEN != c.BasesMDEN
		return r.auditar(ctx, tx, actor, "convocatoria.actualizar", "convocatoria", c.ID, datosConvocatoria(antes), despues)
	})
}

// DefinirEtapas sustituye el calendario de una convocatoria (RF-CONV-03). Es
// también la forma de extender un plazo: se cambia la fecha y el avance
// automático la respeta.
func (r *Repositorio) DefinirEtapas(ctx context.Context, actor Actor, convocatoriaID int64, etapas []Etapa) error {
	return r.operacion(ctx, "definir etapas", func(tx *sql.Tx) error {
		antes, err := convocatoriaPorID(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(append(ValidarEdicionDeConfiguracion(antes.Estado), ValidarEtapas(etapas)...)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM etapa WHERE convocatoria_id = ?", convocatoriaID); err != nil {
			return err
		}
		despues, err := insertarEtapas(ctx, tx, convocatoriaID, etapas)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE convocatoria SET actualizado_at = ? WHERE id = ?", r.marca(), convocatoriaID); err != nil {
			return err
		}
		anteriores := make([]map[string]string, 0, len(antes.Etapas))
		for _, e := range antes.Etapas {
			anteriores = append(anteriores, map[string]string{
				"clave": string(e.Clave), "inicia_at": aMarca(e.IniciaAt), "termina_at": aMarca(e.TerminaAt)})
		}
		return r.auditar(ctx, tx, actor, "convocatoria.etapas", "convocatoria", convocatoriaID,
			map[string]any{"etapas": anteriores}, map[string]any{"etapas": despues})
	})
}

// CambiarEstadoConvocatoria pasa una convocatoria a otro estado de su ciclo de
// vida, si las reglas lo permiten (§3, RN-14), y devuelve el estado anterior.
// La lectura de la configuración, la comprobación de las reglas y el cambio
// ocurren en una sola transacción. automatico distingue en la bitácora el
// avance por calendario del cambio forzado por el administrador.
func (r *Repositorio) CambiarEstadoConvocatoria(ctx context.Context, actor Actor, convocatoriaID int64,
	destino EstadoConvocatoria, motivo string, automatico bool) (EstadoConvocatoria, error) {
	var anterior EstadoConvocatoria
	err := r.operacion(ctx, "cambiar estado de la convocatoria", func(tx *sql.Tx) error {
		cfg, err := configuracion(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		anterior = cfg.Convocatoria.Estado
		if err := validacion(ValidarCambioDeEstado(cfg, destino, motivo, r.ahora())); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE convocatoria SET estado = ?, actualizado_at = ? WHERE id = ?",
			string(destino), r.marca(), convocatoriaID); err != nil {
			return err
		}
		despues := map[string]any{"estado": destino, "automatico": automatico}
		if motivo != "" {
			despues["motivo"] = motivo
		}
		return r.auditar(ctx, tx, actor, "convocatoria.estado", "convocatoria", convocatoriaID,
			map[string]any{"estado": anterior}, despues)
	})
	return anterior, err
}

// DuplicarConvocatoria crea en otra edición una convocatoria del mismo tipo,
// en borrador, con la configuración de la de origen: textos, límites,
// dotaciones, requisitos documentales y criterios (RF-CONV-05). NO copia el
// calendario ni las asignaciones, que son propios de cada año.
func (r *Repositorio) DuplicarConvocatoria(ctx context.Context, actor Actor, origenID, edicionID int64) (int64, error) {
	var id int64
	err := r.operacion(ctx, "duplicar convocatoria", func(tx *sql.Tx) error {
		origen, err := configuracion(ctx, tx, origenID)
		if err != nil {
			return err
		}
		nueva := origen.Convocatoria
		nueva.Edicion, nueva.Estado, nueva.Etapas = Edicion{ID: edicionID}, EstadoBorrador, nil
		if id, _, err = r.insertarConvocatoria(ctx, tx, nueva, nil, origen.Criterios); err != nil {
			return err
		}
		// Las dotaciones reciben id nuevo: los requisitos específicos deben
		// apuntar a la copia, no a la dotación del año anterior.
		nuevoID := map[int64]int64{0: 0}
		for _, d := range origen.Dotaciones {
			anteriorID := d.ID
			d.ConvocatoriaID = id
			if nuevoID[anteriorID], err = insertarDotacion(ctx, tx, d); err != nil {
				return err
			}
		}
		for _, rq := range origen.Requisitos {
			rq.ConvocatoriaID, rq.DotacionID = id, nuevoID[rq.DotacionID]
			if _, err := insertarRequisito(ctx, tx, rq); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "convocatoria.duplicar", "convocatoria", id, nil, map[string]any{
			"origen_id": origenID, "edicion_id": edicionID, "tipo": nueva.Tipo, "dotaciones": len(origen.Dotaciones),
			"requisitos": len(origen.Requisitos), "criterios": len(origen.Criterios),
		})
	})
	return id, err
}

// --- Dotaciones, requisitos y criterios -----------------------------------------

// convocatoriaDe devuelve el id de la convocatoria a la que pertenece una fila
// de una tabla de configuración. El nombre de la tabla es siempre una
// constante del código, nunca un dato de la petición.
func convocatoriaDe(ctx context.Context, tx *sql.Tx, tabla string, id int64) (int64, error) {
	var convocatoriaID int64
	err := tx.QueryRowContext(ctx, "SELECT convocatoria_id FROM "+tabla+" WHERE id = ?", id).Scan(&convocatoriaID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoEncontrado
	}
	return convocatoriaID, err
}

// GuardarDotacion crea una dotación (d.ID == 0) o actualiza una existente
// (RF-CONV-06). Una dotación no se borra: se desactiva con Activa = false, y
// deja de ofrecerse sin afectar a las obras que ya la usan. Devuelve su id y
// el de su convocatoria.
func (r *Repositorio) GuardarDotacion(ctx context.Context, actor Actor, d Dotacion) (id, convocatoriaID int64, err error) {
	id = d.ID
	err = r.operacion(ctx, "guardar dotación", func(tx *sql.Tx) error {
		if d.ID != 0 {
			var err error
			if d.ConvocatoriaID, err = convocatoriaDe(ctx, tx, "dotacion", d.ID); err != nil {
				return err
			}
		}
		c, err := convocatoriaPorID(ctx, tx, d.ConvocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(append(ValidarEdicionDeConfiguracion(c.Estado), ValidarDotacion(d, c.Tipo)...)); err != nil {
			return err
		}
		if repetida, err := existe(ctx, tx,
			"SELECT COUNT(*) FROM dotacion WHERE convocatoria_id = ? AND nombre_es = ? AND id <> ?",
			d.ConvocatoriaID, d.NombreES, d.ID); err != nil {
			return err
		} else if repetida {
			return ErrDuplicado
		}
		accion := "dotacion.actualizar"
		if d.ID == 0 {
			accion = "dotacion.crear"
			if id, err = insertarDotacion(ctx, tx, d); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE dotacion SET nombre_es = ?, nombre_en = ?, descripcion_es = ?, descripcion_en = ?, orden = ?, activa = ?
			WHERE id = ?`, d.NombreES, d.NombreEN, nuloSiVacio(d.DescripcionES), nuloSiVacio(d.DescripcionEN),
			d.Orden, d.Activa, d.ID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, accion, "dotacion", id, nil, map[string]any{
			"convocatoria_id": d.ConvocatoriaID, "nombre_es": d.NombreES, "nombre_en": d.NombreEN,
			"orden": d.Orden, "activa": d.Activa,
		})
	})
	return id, d.ConvocatoriaID, err
}

// datosRequisito es lo que se asienta en la bitácora de un requisito.
func datosRequisito(rq RequisitoDocumental) map[string]any {
	return map[string]any{
		"convocatoria_id": rq.ConvocatoriaID, "dotacion_id": rq.DotacionID, "clave": rq.Clave, "ambito": rq.Ambito,
		"obligatorio": rq.Obligatorio, "mimes_permitidos": rq.MimesPermitidos,
		"tamano_max_bytes": rq.TamanoMaxBytes, "max_archivos": rq.MaxArchivos,
	}
}

// GuardarRequisito crea un requisito documental (rq.ID == 0) o actualiza uno
// existente (RF-CONV-07). Aquí vive la obligatoriedad de cada documento, que
// es configuración y no código. Devuelve su id y el de su convocatoria.
func (r *Repositorio) GuardarRequisito(ctx context.Context, actor Actor, rq RequisitoDocumental) (id, convocatoriaID int64, err error) {
	id = rq.ID
	err = r.operacion(ctx, "guardar requisito documental", func(tx *sql.Tx) error {
		if rq.ID != 0 {
			var err error
			if rq.ConvocatoriaID, err = convocatoriaDe(ctx, tx, "requisito_documental", rq.ID); err != nil {
				return err
			}
		}
		cfg, err := configuracion(ctx, tx, rq.ConvocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(append(ValidarEdicionDeConfiguracion(cfg.Convocatoria.Estado), ValidarRequisito(rq, cfg)...)); err != nil {
			return err
		}
		if repetido, err := existe(ctx, tx, `
			SELECT COUNT(*) FROM requisito_documental
			WHERE convocatoria_id = ? AND dotacion_id = ? AND clave = ? AND id <> ?`,
			rq.ConvocatoriaID, rq.DotacionID, rq.Clave, rq.ID); err != nil {
			return err
		} else if repetido {
			return ErrDuplicado
		}
		accion := "requisito.actualizar"
		if rq.ID == 0 {
			accion = "requisito.crear"
			if id, err = insertarRequisito(ctx, tx, rq); err != nil {
				return err
			}
		} else {
			mimes, err := json.Marshal(rq.MimesPermitidos)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE requisito_documental
				SET dotacion_id = ?, clave = ?, etiqueta_es = ?, etiqueta_en = ?, ayuda_es = ?, ayuda_en = ?, ambito = ?,
				    obligatorio = ?, mimes_permitidos = ?, tamano_max_bytes = ?, max_archivos = ?, orden = ?
				WHERE id = ?`,
				rq.DotacionID, rq.Clave, rq.EtiquetaES, rq.EtiquetaEN, nuloSiVacio(rq.AyudaES), nuloSiVacio(rq.AyudaEN),
				string(rq.Ambito), rq.Obligatorio, string(mimes), rq.TamanoMaxBytes, rq.MaxArchivos, rq.Orden, rq.ID); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, accion, "requisito_documental", id, nil, datosRequisito(rq))
	})
	return id, rq.ConvocatoriaID, err
}

// borrarDeConfiguracion elimina un requisito o un criterio. Sólo se permite
// mientras la convocatoria es borrador.
func (r *Repositorio) borrarDeConfiguracion(ctx context.Context, actor Actor, tabla, accion string, id int64) error {
	return r.operacion(ctx, accion, func(tx *sql.Tx) error {
		convocatoriaID, err := convocatoriaDe(ctx, tx, tabla, id)
		if err != nil {
			return err
		}
		c, err := convocatoriaPorID(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarBorradoDeConfiguracion(c.Estado)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+tabla+" WHERE id = ?", id); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, accion, tabla, id, map[string]any{"convocatoria_id": convocatoriaID}, nil)
	})
}

// BorrarRequisito elimina un requisito documental de una convocatoria en borrador.
func (r *Repositorio) BorrarRequisito(ctx context.Context, actor Actor, id int64) error {
	return r.borrarDeConfiguracion(ctx, actor, "requisito_documental", "requisito.borrar", id)
}

// GuardarCriterio crea un criterio de evaluación (c.ID == 0) o actualiza uno
// existente (RF-EVA-10). Devuelve su id y el de su convocatoria.
func (r *Repositorio) GuardarCriterio(ctx context.Context, actor Actor, c CriterioEvaluacion) (id, convocatoriaID int64, err error) {
	id = c.ID
	err = r.operacion(ctx, "guardar criterio de evaluación", func(tx *sql.Tx) error {
		if c.ID != 0 {
			var err error
			if c.ConvocatoriaID, err = convocatoriaDe(ctx, tx, "criterio_evaluacion", c.ID); err != nil {
				return err
			}
		}
		convocatoria, err := convocatoriaPorID(ctx, tx, c.ConvocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(append(ValidarEdicionDeCriterios(convocatoria.Estado), ValidarCriterio(c)...)); err != nil {
			return err
		}
		if repetido, err := existe(ctx, tx,
			"SELECT COUNT(*) FROM criterio_evaluacion WHERE convocatoria_id = ? AND clave = ? AND id <> ?",
			c.ConvocatoriaID, c.Clave, c.ID); err != nil {
			return err
		} else if repetido {
			return ErrDuplicado
		}
		accion := "criterio.actualizar"
		if c.ID == 0 {
			accion = "criterio.crear"
			if id, err = insertarCriterio(ctx, tx, c); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE criterio_evaluacion
			SET clave = ?, etiqueta_es = ?, etiqueta_en = ?, descripcion_es = ?, descripcion_en = ?,
			    escala_min = ?, escala_max = ?, obligatorio = ?, orden = ?
			WHERE id = ?`, c.Clave, c.EtiquetaES, c.EtiquetaEN, nuloSiVacio(c.DescripcionES), nuloSiVacio(c.DescripcionEN),
			c.EscalaMin, c.EscalaMax, c.Obligatorio, c.Orden, c.ID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, accion, "criterio_evaluacion", id, nil, map[string]any{
			"convocatoria_id": c.ConvocatoriaID, "clave": c.Clave, "escala_min": c.EscalaMin,
			"escala_max": c.EscalaMax, "obligatorio": c.Obligatorio,
		})
	})
	return id, c.ConvocatoriaID, err
}

// BorrarCriterio elimina un criterio de una convocatoria en borrador.
func (r *Repositorio) BorrarCriterio(ctx context.Context, actor Actor, id int64) error {
	return r.borrarDeConfiguracion(ctx, actor, "criterio_evaluacion", "criterio.borrar", id)
}

// --- Asignaciones de jurados y revisores ---------------------------------------

// Asignar otorga a una cuenta un perfil operativo en una convocatoria
// (RF-CONV-09). Si la cuenta ya tuvo ese perfil y se le retiró, se reactiva la
// misma fila en lugar de crear otra. Devuelve el id de la asignación.
func (r *Repositorio) Asignar(ctx context.Context, actor Actor, convocatoriaID, usuarioID int64, perfil Perfil) (int64, error) {
	var id int64
	err := r.operacion(ctx, "asignar", func(tx *sql.Tx) error {
		cfg, err := configuracion(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		usuario, err := leerUsuario(tx.QueryRowContext(ctx, "SELECT "+columnasUsuario+" FROM usuario u WHERE u.id = ?", usuarioID))
		if errors.Is(err, sql.ErrNoRows) {
			return validacion([]ErrorCampo{{Campo: "usuario_id", Clave: "error.usuario_inexistente"}})
		}
		if err != nil {
			return err
		}
		postula, err := existe(ctx, tx,
			"SELECT COUNT(*) FROM postulacion WHERE convocatoria_id = ? AND usuario_id = ?", convocatoriaID, usuarioID)
		if err != nil {
			return err
		}
		var activa bool
		err = tx.QueryRowContext(ctx,
			"SELECT id, activo FROM asignacion WHERE convocatoria_id = ? AND usuario_id = ? AND perfil = ?",
			convocatoriaID, usuarioID, string(perfil)).Scan(&id, &activa)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			id = 0
		case err != nil:
			return err
		case activa:
			return ErrDuplicado
		}
		if err := validacion(ValidarAsignacion(cfg, usuario, perfil, postula)); err != nil {
			return err
		}
		if id != 0 {
			if _, err := tx.ExecContext(ctx, "UPDATE asignacion SET activo = 1 WHERE id = ?", id); err != nil {
				return err
			}
		} else {
			res, err := tx.ExecContext(ctx,
				"INSERT INTO asignacion (convocatoria_id, usuario_id, perfil, activo, creado_at) VALUES (?, ?, ?, 1, ?)",
				convocatoriaID, usuarioID, string(perfil), r.marca())
			if err != nil {
				return err
			}
			if id, err = res.LastInsertId(); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "asignacion.otorgar", "asignacion", id, nil,
			map[string]any{"convocatoria_id": convocatoriaID, "usuario_id": usuarioID, "perfil": perfil})
	})
	return id, err
}

// RetirarAsignacion desactiva una asignación: la cuenta pierde ese perfil en
// la convocatoria desde su siguiente petición. La fila se conserva (RN-22), y
// con ella lo que esa persona haya dictaminado o evaluado. Devuelve el id de
// la convocatoria.
func (r *Repositorio) RetirarAsignacion(ctx context.Context, actor Actor, id int64) (int64, error) {
	var convocatoriaID int64
	err := r.operacion(ctx, "retirar asignación", func(tx *sql.Tx) error {
		var usuarioID int64
		var perfil string
		err := tx.QueryRowContext(ctx, "SELECT convocatoria_id, usuario_id, perfil FROM asignacion WHERE id = ? AND activo = 1", id).
			Scan(&convocatoriaID, &usuarioID, &perfil)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEncontrado
		}
		if err != nil {
			return err
		}
		c, err := convocatoriaPorID(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarEdicionDeConfiguracion(c.Estado)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE asignacion SET activo = 0 WHERE id = ?", id); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "asignacion.retirar", "asignacion", id,
			map[string]any{"convocatoria_id": convocatoriaID, "usuario_id": usuarioID, "perfil": perfil}, nil)
	})
	return convocatoriaID, err
}

// ---------------------------------------------------------------------------
// 5. Usuarios
// ---------------------------------------------------------------------------

// REGLA DE LA BITÁCORA PARA DATOS PERSONALES: la tabla auditoria es de sólo
// inserción, de modo que lo que entra no puede borrarse ni ante una solicitud
// ARCO. Por eso en «antes» y «despues» NUNCA se guardan correos, nombres ni
// otros datos de contacto: sólo identificadores, perfiles y los NOMBRES de los
// campos que cambiaron.

// columnasUsuario es la lista de columnas que lee leerUsuario.
const columnasUsuario = "u.id, u.email, u.nombre, u.apellidos, u.idioma, u.email_verificado_at, u.activo, u.creado_at"

// leerUsuario vuelca una fila con columnasUsuario (más las columnas extra que
// se indiquen al final) en la entidad.
func leerUsuario(f fila, extra ...any) (Usuario, error) {
	var u Usuario
	var verificado sql.NullString
	var creado string
	destinos := append([]any{&u.ID, &u.Email, &u.Nombre, &u.Apellidos, &u.Idioma, &verificado, &u.Activo, &creado}, extra...)
	if err := f.Scan(destinos...); err != nil {
		return Usuario{}, err
	}
	var err error
	if u.CreadoAt, err = deMarca(creado); err != nil {
		return Usuario{}, err
	}
	if verificado.Valid {
		if u.EmailVerificadoAt, err = deMarca(verificado.String); err != nil {
			return Usuario{}, err
		}
	}
	return u, nil
}

// emailEnUso indica si otra cuenta ya usa el correo. La comparación ignora
// mayúsculas (la columna es COLLATE NOCASE). Se ejecuta dentro de la
// transacción de escritura, que es única: no hay carrera posible.
func emailEnUso(ctx context.Context, tx *sql.Tx, email string, exceptoID int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM usuario WHERE email = ? AND id <> ?", email, exceptoID).Scan(&n)
	return n > 0, err
}

// encolarCorreo escribe un mensaje en la bandeja de salida, dentro de la
// transacción de negocio. Nunca se envía correo dentro de una transacción.
func (r *Repositorio) encolarCorreo(ctx context.Context, tx *sql.Tx, c CorreoSaliente) error {
	marca := r.marca()
	adjuntos := c.Adjuntos
	if adjuntos == nil {
		adjuntos = []Adjunto{}
	}
	// adjuntos es un arreglo JSON en texto (§8.3): [{archivo_id, nombre}].
	listaDeAdjuntos, err := json.Marshal(adjuntos)
	if err != nil {
		return fmt.Errorf("encolar correo: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO email_outbox (destinatario, idioma, asunto, cuerpo_html, cuerpo_texto, adjuntos,
		                          plantilla_clave, postulacion_id, programado_para, creado_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Destinatario, c.Idioma, c.Asunto, c.HTML, c.Texto, string(listaDeAdjuntos),
		nuloSiVacio(c.PlantillaClave), nuloSiCero(c.PostulacionID), marca, marca)
	if err != nil {
		return fmt.Errorf("encolar correo: %w", err)
	}
	return nil
}

// insertarToken guarda un enlace de un solo uso y anula los anteriores del
// mismo propósito que la cuenta aún no hubiera usado: sólo vale el último.
func (r *Repositorio) insertarToken(ctx context.Context, tx *sql.Tx, usuarioID int64, t NuevoToken) error {
	if _, err := tx.ExecContext(ctx,
		"UPDATE token SET usado_at = ? WHERE usuario_id = ? AND proposito = ? AND usado_at IS NULL",
		r.marca(), usuarioID, string(t.Proposito)); err != nil {
		return fmt.Errorf("anular enlaces anteriores: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO token (hash, usuario_id, proposito, expira_at) VALUES (?, ?, ?, ?)",
		t.Resumen, usuarioID, string(t.Proposito), aMarca(t.ExpiraAt)); err != nil {
		return fmt.Errorf("guardar enlace: %w", err)
	}
	return nil
}

// CrearUsuario da de alta una cuenta con su perfil global inicial y, en la
// misma transacción, el enlace de un solo uso (verificación o activación) y
// los correos que lo llevan. Devuelve ErrEmailEnUso si el correo ya existe.
func (r *Repositorio) CrearUsuario(ctx context.Context, actor Actor, n NuevoUsuario, token *NuevoToken, correos ...CorreoSaliente) (int64, error) {
	var id int64
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if enUso, err := emailEnUso(ctx, tx, n.Email, 0); err != nil {
			return err
		} else if enUso {
			return ErrEmailEnUso
		}
		marca := r.marca()
		var avisoAceptado sql.NullString
		if n.AvisoVersion != "" {
			// Consentimiento con sello de tiempo y versión del aviso (LFPDPPP).
			avisoAceptado = sql.NullString{String: marca, Valid: true}
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO usuario (email, password_hash, nombre, apellidos, idioma,
			                     aviso_privacidad_version, aviso_privacidad_aceptado_at,
			                     creado_at, actualizado_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.Email, n.PasswordHash, n.Nombre, n.Apellidos, n.Idioma,
			nuloSiVacio(n.AvisoVersion), avisoAceptado, marca, marca)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		// Una cuenta que sólo tendrá perfiles por convocatoria (jurado o
		// revisor) nace sin perfil global.
		if n.Perfil != "" {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO usuario_perfil (usuario_id, perfil, otorgado_por, creado_at) VALUES (?, ?, ?, ?)",
				id, string(n.Perfil), nuloSiCero(actor.UsuarioID), marca); err != nil {
				return err
			}
		}
		if token != nil {
			if err := r.insertarToken(ctx, tx, id, *token); err != nil {
				return err
			}
		}
		for _, c := range correos {
			if err := r.encolarCorreo(ctx, tx, c); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "usuario.crear", "usuario", id, nil, map[string]any{
			"perfil": n.Perfil, "idioma": n.Idioma, "aviso_privacidad_version": n.AvisoVersion,
		})
	})
	if err != nil {
		if errors.Is(err, ErrEmailEnUso) {
			return 0, err
		}
		return 0, fmt.Errorf("crear usuario: %w", err)
	}
	return id, nil
}

// UsuarioPorEmail busca una cuenta por su correo y devuelve además su resumen
// de contraseña, para el inicio de sesión. ErrNoEncontrado si no existe.
func (r *Repositorio) UsuarioPorEmail(ctx context.Context, email string) (Usuario, string, error) {
	var resumen string
	u, err := leerUsuario(r.lectura.QueryRowContext(ctx,
		"SELECT "+columnasUsuario+", u.password_hash FROM usuario u WHERE u.email = ?", email), &resumen)
	if errors.Is(err, sql.ErrNoRows) {
		return Usuario{}, "", ErrNoEncontrado
	}
	if err != nil {
		return Usuario{}, "", fmt.Errorf("leer usuario: %w", err)
	}
	return u, resumen, nil
}

// ResumenDePassword devuelve el resumen de contraseña de una cuenta, para
// comprobar la contraseña actual antes de un cambio sensible.
func (r *Repositorio) ResumenDePassword(ctx context.Context, usuarioID int64) (string, error) {
	var resumen string
	err := r.lectura.QueryRowContext(ctx, "SELECT password_hash FROM usuario WHERE id = ?", usuarioID).Scan(&resumen)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoEncontrado
	}
	if err != nil {
		return "", fmt.Errorf("leer contraseña: %w", err)
	}
	return resumen, nil
}

// ActualizarDatosPersonales cambia nombre, apellidos e idioma (RF-AUT-06). En
// la bitácora queda qué campos cambiaron, no sus valores.
func (r *Repositorio) ActualizarDatosPersonales(ctx context.Context, actor Actor, usuarioID int64, nombre, apellidos, idioma string) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		var antesNombre, antesApellidos, antesIdioma string
		err := tx.QueryRowContext(ctx, "SELECT nombre, apellidos, idioma FROM usuario WHERE id = ?", usuarioID).
			Scan(&antesNombre, &antesApellidos, &antesIdioma)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEncontrado
		}
		if err != nil {
			return err
		}
		var campos []string
		if antesNombre != nombre {
			campos = append(campos, "nombre")
		}
		if antesApellidos != apellidos {
			campos = append(campos, "apellidos")
		}
		if antesIdioma != idioma {
			campos = append(campos, "idioma")
		}
		if len(campos) == 0 {
			return nil // nada cambió: ni escritura ni asiento
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE usuario SET nombre = ?, apellidos = ?, idioma = ?, actualizado_at = ? WHERE id = ?",
			nombre, apellidos, idioma, r.marca(), usuarioID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "cuenta.datos_actualizados", "usuario", usuarioID, nil,
			map[string]any{"campos": campos})
	})
	if err != nil && !errors.Is(err, ErrNoEncontrado) {
		return fmt.Errorf("actualizar datos personales: %w", err)
	}
	return err
}

// CambiarPassword guarda una contraseña nueva y cierra las demás sesiones de
// la cuenta; se conserva sólo la que hizo el cambio.
func (r *Repositorio) CambiarPassword(ctx context.Context, actor Actor, usuarioID int64, resumen, sesionAConservar string) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE usuario SET password_hash = ?, actualizado_at = ? WHERE id = ?",
			resumen, r.marca(), usuarioID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM sesion WHERE usuario_id = ? AND id <> ?", usuarioID, sesionAConservar); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "cuenta.password_cambiada", "usuario", usuarioID, nil, nil)
	})
	if err != nil {
		return fmt.Errorf("cambiar contraseña: %w", err)
	}
	return nil
}

// CambiarEmail cambia el correo de la cuenta. El correo nuevo queda SIN
// verificar y se emite su enlace de verificación (RF-AUT-06). Devuelve
// ErrEmailEnUso si otra cuenta ya lo tiene.
func (r *Repositorio) CambiarEmail(ctx context.Context, actor Actor, usuarioID int64, email string, token NuevoToken, correos ...CorreoSaliente) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if enUso, err := emailEnUso(ctx, tx, email, usuarioID); err != nil {
			return err
		} else if enUso {
			return ErrEmailEnUso
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE usuario SET email = ?, email_verificado_at = NULL, actualizado_at = ? WHERE id = ?",
			email, r.marca(), usuarioID); err != nil {
			return err
		}
		if err := r.insertarToken(ctx, tx, usuarioID, token); err != nil {
			return err
		}
		for _, c := range correos {
			if err := r.encolarCorreo(ctx, tx, c); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "cuenta.email_cambiado", "usuario", usuarioID, nil,
			map[string]any{"campos": []string{"email"}})
	})
	if err != nil && !errors.Is(err, ErrEmailEnUso) {
		return fmt.Errorf("cambiar correo: %w", err)
	}
	return err
}

// UsuarioPorID devuelve una cuenta con sus perfiles globales y si ya está
// activada, o ErrNoEncontrado.
func (r *Repositorio) UsuarioPorID(ctx context.Context, id int64) (CuentaAdmin, error) {
	cuentas, _, err := r.cuentas(ctx, "WHERE u.id = ?", []any{id}, 1, 1)
	if err != nil {
		return CuentaAdmin{}, err
	}
	if len(cuentas) == 0 {
		return CuentaAdmin{}, ErrNoEncontrado
	}
	return cuentas[0], nil
}

// BuscarUsuarios busca cuentas por correo, nombre o apellidos (coincidencia
// parcial, sin distinguir mayúsculas) y devuelve una página y el total. Con
// texto vacío las lista todas. Es la forma de encontrar la cuenta que ya tiene
// alguien antes de asignarlo: el sistema nunca decide por sí solo que dos
// registros son la misma persona (RN-30).
func (r *Repositorio) BuscarUsuarios(ctx context.Context, texto string, pagina, tamano int) ([]CuentaAdmin, int, error) {
	if texto == "" {
		return r.cuentas(ctx, "", nil, pagina, tamano)
	}
	// Los comodines que escriba el usuario se buscan como texto literal.
	patron := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(texto) + "%"
	return r.cuentas(ctx, `WHERE u.email LIKE ? ESCAPE '\' OR u.nombre LIKE ? ESCAPE '\' OR u.apellidos LIKE ? ESCAPE '\'`,
		[]any{patron, patron, patron}, pagina, tamano)
}

// cuentas lee una página de cuentas que cumplen una condición.
func (r *Repositorio) cuentas(ctx context.Context, condicion string, args []any, pagina, tamano int) ([]CuentaAdmin, int, error) {
	var total int
	if err := r.lectura.QueryRowContext(ctx, "SELECT COUNT(*) FROM usuario u "+condicion, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contar cuentas: %w", err)
	}
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT `+columnasUsuario+`, u.password_hash,
		       COALESCE((SELECT GROUP_CONCAT(p.perfil) FROM usuario_perfil p WHERE p.usuario_id = u.id), '')
		FROM usuario u `+condicion+` ORDER BY u.apellidos, u.nombre, u.id LIMIT ? OFFSET ?`,
		append(args, tamano, (pagina-1)*tamano)...)
	if err != nil {
		return nil, 0, fmt.Errorf("leer cuentas: %w", err)
	}
	defer filas.Close()
	var lista []CuentaAdmin
	for filas.Next() {
		var c CuentaAdmin
		var resumen, perfiles string
		if c.Usuario, err = leerUsuario(filas, &resumen, &perfiles); err != nil {
			return nil, 0, fmt.Errorf("leer cuentas: %w", err)
		}
		c.Activada = resumen != PasswordSinDefinir
		for _, perfil := range strings.Split(perfiles, ",") {
			if perfil != "" {
				c.Perfiles = append(c.Perfiles, Perfil(perfil))
			}
		}
		lista = append(lista, c)
	}
	return lista, total, filas.Err()
}

// ---------------------------------------------------------------------------
// 6. Sesiones e inicios de sesión
// ---------------------------------------------------------------------------

// AbrirSesion registra una sesión nueva y asienta el inicio de sesión
// (RF-AUT-08). idResumen es el resumen del identificador que viaja en la
// cookie; el valor en claro nunca se guarda.
func (r *Repositorio) AbrirSesion(ctx context.Context, actor Actor, usuarioID int64, idResumen string, expira time.Time) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO sesion (id, usuario_id, ip, user_agent, expira_at, creado_at) VALUES (?, ?, ?, ?, ?, ?)",
			idResumen, usuarioID, nuloSiVacio(actor.IP), nuloSiVacio(actor.Agente), aMarca(expira), r.marca()); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "sesion.ingreso", "usuario", usuarioID, nil, nil)
	})
	if err != nil {
		return fmt.Errorf("abrir sesión: %w", err)
	}
	return nil
}

// RegistrarIngresoFallido asienta un intento de inicio de sesión que no
// prosperó. usuarioID es 0 si el correo no corresponde a ninguna cuenta.
// Los intentos rechazados por bloqueo se asientan aparte y no alargan el
// bloqueo: sólo cuentan las contraseñas incorrectas.
func (r *Repositorio) RegistrarIngresoFallido(ctx context.Context, actor Actor, usuarioID int64, bloqueado bool) error {
	accion := "sesion.ingreso_fallido"
	if bloqueado {
		accion = "sesion.ingreso_bloqueado"
	}
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		return r.auditar(ctx, tx, actor, accion, "usuario", usuarioID, nil, nil)
	})
	if err != nil {
		return fmt.Errorf("registrar intento de ingreso: %w", err)
	}
	return nil
}

// FallosDeIngreso devuelve cuántas contraseñas incorrectas seguidas lleva una
// cuenta y cuándo fue la última. La cuenta se reinicia con un ingreso
// correcto, un restablecimiento de contraseña o la activación. La bitácora es
// la única fuente: no hay un contador aparte que pueda desincronizarse.
func (r *Repositorio) FallosDeIngreso(ctx context.Context, usuarioID int64) (int, time.Time, error) {
	var fallos int
	var ultimo sql.NullString
	err := r.lectura.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(creado_at)
		FROM auditoria
		WHERE entidad = 'usuario' AND entidad_id = ? AND accion = 'sesion.ingreso_fallido'
		  AND id > COALESCE((SELECT MAX(id) FROM auditoria
		                     WHERE entidad = 'usuario' AND entidad_id = ?
		                       AND accion IN ('sesion.ingreso', 'cuenta.password_restablecida', 'cuenta.activada')), 0)`,
		usuarioID, usuarioID).Scan(&fallos, &ultimo)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("contar intentos fallidos: %w", err)
	}
	if !ultimo.Valid {
		return 0, time.Time{}, nil
	}
	momento, err := deMarca(ultimo.String)
	return fallos, momento, err
}

// SesionPorID devuelve la sesión vigente con ese resumen de identificador,
// junto con la cuenta, sus perfiles globales y sus asignaciones activas.
// ErrNoEncontrado si no existe, venció o la cuenta está desactivada.
func (r *Repositorio) SesionPorID(ctx context.Context, idResumen string) (*Sesion, error) {
	var expira string
	usuario, err := leerUsuario(r.lectura.QueryRowContext(ctx, `
		SELECT `+columnasUsuario+`, s.expira_at
		FROM sesion s
		JOIN usuario u ON u.id = s.usuario_id
		WHERE s.id = ? AND s.expira_at > ? AND u.activo = 1`, idResumen, r.marca()), &expira)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("leer sesión: %w", err)
	}
	s := &Sesion{ID: idResumen, Usuario: usuario}
	if s.ExpiraAt, err = deMarca(expira); err != nil {
		return nil, err
	}

	filas, err := r.lectura.QueryContext(ctx,
		"SELECT perfil FROM usuario_perfil WHERE usuario_id = ? ORDER BY perfil", usuario.ID)
	if err != nil {
		return nil, fmt.Errorf("leer perfiles: %w", err)
	}
	for filas.Next() {
		var perfil string
		if err := filas.Scan(&perfil); err != nil {
			filas.Close()
			return nil, fmt.Errorf("leer perfiles: %w", err)
		}
		s.Perfiles = append(s.Perfiles, Perfil(perfil))
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("leer perfiles: %w", err)
	}

	// Sólo las asignaciones ACTIVAS dan acceso: retirar a un jurado surte
	// efecto en su siguiente petición, sin esperar a que cierre sesión.
	filas, err = r.lectura.QueryContext(ctx,
		"SELECT convocatoria_id, perfil FROM asignacion WHERE usuario_id = ? AND activo = 1 ORDER BY convocatoria_id, perfil",
		usuario.ID)
	if err != nil {
		return nil, fmt.Errorf("leer asignaciones: %w", err)
	}
	defer filas.Close()
	for filas.Next() {
		var a Asignacion
		var perfil string
		if err := filas.Scan(&a.ConvocatoriaID, &perfil); err != nil {
			return nil, fmt.Errorf("leer asignaciones: %w", err)
		}
		a.Perfil = Perfil(perfil)
		s.Asignaciones = append(s.Asignaciones, a)
	}
	return s, filas.Err()
}

// RenovarSesion prolonga una sesión en uso (expiración por inactividad).
func (r *Repositorio) RenovarSesion(ctx context.Context, idResumen string, expira time.Time) error {
	if _, err := r.escritura.ExecContext(ctx,
		"UPDATE sesion SET expira_at = ? WHERE id = ?", aMarca(expira), idResumen); err != nil {
		return fmt.Errorf("renovar sesión: %w", err)
	}
	return nil
}

// CerrarSesion elimina una sesión (salida voluntaria) y lo asienta.
func (r *Repositorio) CerrarSesion(ctx context.Context, actor Actor, idResumen string) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sesion WHERE id = ?", idResumen); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "sesion.salida", "usuario", actor.UsuarioID, nil, nil)
	})
	if err != nil {
		return fmt.Errorf("cerrar sesión: %w", err)
	}
	return nil
}

// PurgarSesionesVencidas borra las sesiones que ya expiraron. Son estado
// técnico efímero: su rastro permanente es el asiento de inicio de sesión.
func (r *Repositorio) PurgarSesionesVencidas(ctx context.Context) (int64, error) {
	res, err := r.escritura.ExecContext(ctx, "DELETE FROM sesion WHERE expira_at <= ?", r.marca())
	if err != nil {
		return 0, fmt.Errorf("purgar sesiones: %w", err)
	}
	return res.RowsAffected()
}

// ---------------------------------------------------------------------------
// 7. Enlaces de un solo uso (tokens)
// ---------------------------------------------------------------------------

// EmitirToken guarda un enlace nuevo para una cuenta existente y encola los
// correos que lo llevan. Anula los enlaces anteriores del mismo propósito.
func (r *Repositorio) EmitirToken(ctx context.Context, actor Actor, usuarioID int64, token NuevoToken, correos ...CorreoSaliente) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if err := r.insertarToken(ctx, tx, usuarioID, token); err != nil {
			return err
		}
		for _, c := range correos {
			if err := r.encolarCorreo(ctx, tx, c); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "token.emitir", "usuario", usuarioID, nil,
			map[string]any{"proposito": token.Proposito})
	})
	if err != nil {
		return fmt.Errorf("emitir enlace: %w", err)
	}
	return nil
}

// UltimaExpiracionDeToken devuelve el vencimiento del enlace más reciente de
// ese propósito emitido a la cuenta, o el valor cero si nunca hubo. Con él,
// rules.go decide si ya puede emitirse otro.
func (r *Repositorio) UltimaExpiracionDeToken(ctx context.Context, usuarioID int64, proposito PropositoToken) (time.Time, error) {
	var expira sql.NullString
	err := r.lectura.QueryRowContext(ctx,
		"SELECT MAX(expira_at) FROM token WHERE usuario_id = ? AND proposito = ?",
		usuarioID, string(proposito)).Scan(&expira)
	if err != nil {
		return time.Time{}, fmt.Errorf("leer enlaces: %w", err)
	}
	if !expira.Valid {
		return time.Time{}, nil
	}
	return deMarca(expira.String)
}

// TokenVigente comprueba, SIN consumirlo, que un enlace sirve, y devuelve la
// cuenta a la que pertenece. Se usa para pintar la página a la que llega el
// enlace. ErrTokenInvalido si no existe, venció o ya se usó.
func (r *Repositorio) TokenVigente(ctx context.Context, resumen string, proposito PropositoToken) (Usuario, error) {
	u, err := leerUsuario(r.lectura.QueryRowContext(ctx, `
		SELECT `+columnasUsuario+`
		FROM token t
		JOIN usuario u ON u.id = t.usuario_id
		WHERE t.hash = ? AND t.proposito = ? AND t.usado_at IS NULL AND t.expira_at > ? AND u.activo = 1`,
		resumen, string(proposito), r.marca()))
	if errors.Is(err, sql.ErrNoRows) {
		return Usuario{}, ErrTokenInvalido
	}
	if err != nil {
		return Usuario{}, fmt.Errorf("leer enlace: %w", err)
	}
	return u, nil
}

// consumirToken marca un enlace como usado y devuelve su cuenta. El UPDATE
// condicionado hace atómico el «un solo uso»: de dos peticiones simultáneas
// con el mismo enlace, sólo una modifica la fila.
func (r *Repositorio) consumirToken(ctx context.Context, tx *sql.Tx, resumen string, proposito PropositoToken) (int64, error) {
	marca := r.marca()
	res, err := tx.ExecContext(ctx,
		"UPDATE token SET usado_at = ? WHERE hash = ? AND proposito = ? AND usado_at IS NULL AND expira_at > ?",
		marca, resumen, string(proposito), marca)
	if err != nil {
		return 0, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, err
	} else if n != 1 {
		return 0, ErrTokenInvalido
	}
	var usuarioID int64
	err = tx.QueryRowContext(ctx,
		"SELECT t.usuario_id FROM token t JOIN usuario u ON u.id = t.usuario_id WHERE t.hash = ? AND u.activo = 1",
		resumen).Scan(&usuarioID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTokenInvalido
	}
	return usuarioID, err
}

// VerificarEmailConToken consume un enlace de verificación y marca el correo
// de la cuenta como verificado (RF-AUT-02). Devuelve el id de la cuenta.
func (r *Repositorio) VerificarEmailConToken(ctx context.Context, actor Actor, resumen string) (int64, error) {
	var usuarioID int64
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		var err error
		if usuarioID, err = r.consumirToken(ctx, tx, resumen, TokenVerificarEmail); err != nil {
			return err
		}
		marca := r.marca()
		if _, err := tx.ExecContext(ctx,
			"UPDATE usuario SET email_verificado_at = ?, actualizado_at = ? WHERE id = ?",
			marca, marca, usuarioID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "cuenta.email_verificado", "usuario", usuarioID, nil, nil)
	})
	if err != nil && !errors.Is(err, ErrTokenInvalido) {
		return 0, fmt.Errorf("verificar correo: %w", err)
	}
	return usuarioID, err
}

// DefinirPasswordConToken consume un enlace de restablecimiento o de
// activación y guarda la contraseña nueva. Además:
//   - marca el correo como verificado, si no lo estaba: quien abrió el enlace
//     demostró que controla ese buzón;
//   - cierra TODAS las sesiones de la cuenta;
//   - anula cualquier otro enlace de restablecimiento o activación pendiente.
//
// Devuelve el id de la cuenta.
func (r *Repositorio) DefinirPasswordConToken(ctx context.Context, actor Actor, resumenToken string, proposito PropositoToken, resumenPassword string) (int64, error) {
	accion := "cuenta.password_restablecida"
	if proposito == TokenActivacion {
		accion = "cuenta.activada"
	}
	var usuarioID int64
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		var err error
		if usuarioID, err = r.consumirToken(ctx, tx, resumenToken, proposito); err != nil {
			return err
		}
		marca := r.marca()
		if _, err := tx.ExecContext(ctx, `
			UPDATE usuario
			SET password_hash = ?, email_verificado_at = COALESCE(email_verificado_at, ?), actualizado_at = ?
			WHERE id = ?`, resumenPassword, marca, marca, usuarioID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM sesion WHERE usuario_id = ?", usuarioID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE token SET usado_at = ?
			WHERE usuario_id = ? AND proposito IN ('reset_password', 'activacion') AND usado_at IS NULL`,
			marca, usuarioID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, accion, "usuario", usuarioID, nil, nil)
	})
	if err != nil && !errors.Is(err, ErrTokenInvalido) {
		return 0, fmt.Errorf("definir contraseña: %w", err)
	}
	return usuarioID, err
}

// ---------------------------------------------------------------------------
// 8. Correo saliente (bandeja de salida)
// ---------------------------------------------------------------------------

// PlantillaCorreo devuelve la versión activa más reciente de una plantilla en
// el idioma pedido. Si no existe en ese idioma se entrega la española.
func (r *Repositorio) PlantillaCorreo(ctx context.Context, clave, idioma string) (PlantillaCorreo, error) {
	for _, candidato := range []string{idioma, IdiomaES} {
		p := PlantillaCorreo{Clave: clave, Idioma: candidato}
		err := r.lectura.QueryRowContext(ctx, `
			SELECT version, asunto, cuerpo_md FROM plantilla_correo
			WHERE clave = ? AND idioma = ? AND activa = 1
			ORDER BY version DESC LIMIT 1`, clave, candidato).Scan(&p.Version, &p.Asunto, &p.CuerpoMD)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return PlantillaCorreo{}, fmt.Errorf("leer plantilla %s: %w", clave, err)
		}
	}
	return PlantillaCorreo{}, fmt.Errorf("plantilla de correo %q: %w", clave, ErrNoEncontrado)
}

// CorreosPendientes devuelve los mensajes de la bandeja de salida cuyo envío
// ya toca, los más antiguos primero.
func (r *Repositorio) CorreosPendientes(ctx context.Context, limite int) ([]CorreoSaliente, error) {
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT id, destinatario, idioma, asunto, cuerpo_html, cuerpo_texto, COALESCE(plantilla_clave, ''), intentos,
		       COALESCE(postulacion_id, 0), adjuntos
		FROM email_outbox
		WHERE estado = 'pendiente' AND programado_para <= ?
		ORDER BY id LIMIT ?`, r.marca(), limite)
	if err != nil {
		return nil, fmt.Errorf("leer bandeja de salida: %w", err)
	}
	defer filas.Close()
	var correos []CorreoSaliente
	for filas.Next() {
		var c CorreoSaliente
		var adjuntos string
		if err := filas.Scan(&c.ID, &c.Destinatario, &c.Idioma, &c.Asunto, &c.HTML, &c.Texto, &c.PlantillaClave, &c.Intentos,
			&c.PostulacionID, &adjuntos); err != nil {
			return nil, fmt.Errorf("leer bandeja de salida: %w", err)
		}
		if err := json.Unmarshal([]byte(adjuntos), &c.Adjuntos); err != nil {
			return nil, fmt.Errorf("correo %d: adjuntos no es JSON válido: %w", c.ID, err)
		}
		correos = append(correos, c)
	}
	return correos, filas.Err()
}

// cuerpoRetirado sustituye al contenido de un correo con enlace de un solo
// uso una vez enviado.
const cuerpoRetirado = "[contenido retirado tras el envío: llevaba un enlace de un solo uso]"

// MarcarCorreoEnviado registra un envío correcto con su Message-ID. Si el
// mensaje llevaba un enlace de un solo uso, su cuerpo se retira de la base.
func (r *Repositorio) MarcarCorreoEnviado(ctx context.Context, correo CorreoSaliente, messageID string) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		marca := r.marca()
		consulta := "UPDATE email_outbox SET estado = 'enviado', message_id = ?, enviado_at = ?, intentos = intentos + 1, ultimo_error = NULL"
		args := []any{messageID, marca}
		if PlantillaConEnlaceSecreto(correo.PlantillaClave) {
			consulta += ", cuerpo_html = ?, cuerpo_texto = ?"
			args = append(args, cuerpoRetirado, cuerpoRetirado)
		}
		if _, err := tx.ExecContext(ctx, consulta+" WHERE id = ?", append(args, correo.ID)...); err != nil {
			return err
		}
		return r.auditar(ctx, tx, Actor{Perfil: ActorSistema}, "correo.enviar", "email_outbox", correo.ID, nil,
			map[string]any{"plantilla": correo.PlantillaClave, "message_id": messageID})
	})
	if err != nil {
		return fmt.Errorf("marcar correo enviado: %w", err)
	}
	return nil
}

// MarcarCorreoFallido registra un intento de envío fallido. Si reintentar es
// true el mensaje sigue pendiente y se reprograma; si no, queda en estado
// «error» para atención del administrador, y se asienta en la bitácora.
func (r *Repositorio) MarcarCorreoFallido(ctx context.Context, correo CorreoSaliente, motivo string, proximo time.Time, reintentar bool) error {
	err := r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if reintentar {
			_, err := tx.ExecContext(ctx,
				"UPDATE email_outbox SET intentos = intentos + 1, ultimo_error = ?, programado_para = ? WHERE id = ?",
				motivo, aMarca(proximo), correo.ID)
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE email_outbox SET estado = 'error', intentos = intentos + 1, ultimo_error = ? WHERE id = ?",
			motivo, correo.ID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, Actor{Perfil: ActorSistema}, "correo.error", "email_outbox", correo.ID, nil,
			map[string]any{"plantilla": correo.PlantillaClave, "intentos": correo.Intentos + 1})
	})
	if err != nil {
		return fmt.Errorf("marcar correo fallido: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 9. Consulta de la bitácora
// ---------------------------------------------------------------------------

// ConsultarAuditoria devuelve una página de la bitácora, los asientos más
// recientes primero, y el total de asientos que cumplen el filtro (RF-AUD-03).
func (r *Repositorio) ConsultarAuditoria(ctx context.Context, f FiltroAuditoria) ([]AsientoAuditoria, int, error) {
	// El WHERE se arma sólo con fragmentos fijos; los valores van siempre
	// como parámetros, nunca concatenados.
	donde := " WHERE 1 = 1"
	var args []any
	if f.Accion != "" {
		donde += " AND accion = ?"
		args = append(args, f.Accion)
	}
	if f.Entidad != "" {
		donde += " AND entidad = ?"
		args = append(args, f.Entidad)
	}
	if f.EntidadID != 0 {
		donde += " AND entidad_id = ?"
		args = append(args, f.EntidadID)
	}
	if f.ActorID != 0 {
		donde += " AND actor_id = ?"
		args = append(args, f.ActorID)
	}
	if !f.Desde.IsZero() {
		donde += " AND creado_at >= ?"
		args = append(args, aMarca(f.Desde))
	}
	if !f.Hasta.IsZero() {
		donde += " AND creado_at < ?"
		args = append(args, aMarca(f.Hasta))
	}

	var total int
	if err := r.lectura.QueryRowContext(ctx, "SELECT COUNT(*) FROM auditoria"+donde, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("contar bitácora: %w", err)
	}
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT id, COALESCE(actor_id, 0), COALESCE(actor_perfil, ''), accion, entidad, COALESCE(entidad_id, 0),
		       COALESCE(antes, ''), COALESCE(despues, ''), COALESCE(ip, ''), COALESCE(user_agent, ''), creado_at
		FROM auditoria`+donde+" ORDER BY id DESC LIMIT ? OFFSET ?",
		append(args, f.Tamano, (f.Pagina-1)*f.Tamano)...)
	if err != nil {
		return nil, 0, fmt.Errorf("leer bitácora: %w", err)
	}
	defer filas.Close()
	var asientos []AsientoAuditoria
	for filas.Next() {
		var a AsientoAuditoria
		var creado string
		if err := filas.Scan(&a.ID, &a.ActorID, &a.ActorPerfil, &a.Accion, &a.Entidad, &a.EntidadID,
			&a.Antes, &a.Despues, &a.IP, &a.Agente, &creado); err != nil {
			return nil, 0, fmt.Errorf("leer bitácora: %w", err)
		}
		if a.CreadoAt, err = deMarca(creado); err != nil {
			return nil, 0, err
		}
		asientos = append(asientos, a)
	}
	return asientos, total, filas.Err()
}

// ---------------------------------------------------------------------------
// 10. Postulaciones
// ---------------------------------------------------------------------------

// Toda operación de escritura de esta sección recibe el id de la cuenta que la
// pide y sólo actúa sobre postulaciones SUYAS: la condición «usuario_id = ?»
// va en la propia consulta. Una postulación ajena responde ErrNoEncontrado,
// igual que una que no existe.

const sqlPostulacion = `
SELECT p.id, p.convocatoria_id, p.usuario_id, COALESCE(p.subcategoria, ''), COALESCE(p.folio, ''),
       p.estado, p.envios, p.creado_at, p.actualizado_at, COALESCE(p.enviada_at, ''), COALESCE(p.motivo_descarte, '')
FROM postulacion p`

// leerPostulacion vuelca una fila de sqlPostulacion en la entidad, sin lo capturado.
func leerPostulacion(f fila) (Postulacion, error) {
	var p Postulacion
	var subcategoria, estado, creado, actualizado, enviada string
	err := f.Scan(&p.ID, &p.ConvocatoriaID, &p.UsuarioID, &subcategoria, &p.Folio, &estado, &p.Envios, &creado, &actualizado,
		&enviada, &p.MotivoDescarte)
	if errors.Is(err, sql.ErrNoRows) {
		return Postulacion{}, ErrNoEncontrado
	}
	if err != nil {
		return Postulacion{}, err
	}
	p.Subcategoria, p.Estado = Subcategoria(subcategoria), EstadoDePostulacion(estado)
	if p.CreadoAt, err = deMarca(creado); err != nil {
		return Postulacion{}, err
	}
	if p.ActualizadoAt, err = deMarca(actualizado); err != nil {
		return Postulacion{}, err
	}
	if enviada != "" {
		p.EnviadaAt, err = deMarca(enviada)
	}
	return p, err
}

// leerCampos lee de una tabla las columnas de las definiciones que le
// corresponden y deja en destino, por ruta canónica, las que tienen valor.
// Los nombres de tabla y de columna salen de la definición del formulario,
// que es código, nunca de la petición.
func leerCampos(ctx context.Context, q consultor, tabla, llave string, id int64, definiciones []DefinicionCampo, destino map[string]string) error {
	var columnas []string
	var rutas []string
	for _, def := range definiciones {
		if def.tabla == tabla {
			columnas = append(columnas, def.columna)
			rutas = append(rutas, def.Ruta)
		}
	}
	if len(columnas) == 0 {
		return nil
	}
	valores := make([]sql.NullString, len(columnas))
	destinos := make([]any, len(columnas))
	for i := range valores {
		destinos[i] = &valores[i]
	}
	err := q.QueryRowContext(ctx, "SELECT "+strings.Join(columnas, ", ")+" FROM "+tabla+" WHERE "+llave+" = ?", id).Scan(destinos...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("leer %s: %w", tabla, err)
	}
	for i, valor := range valores {
		if valor.Valid && valor.String != "" {
			destino[rutas[i]] = valor.String
		}
	}
	return nil
}

// tablasDe devuelve, sin repetir, las tablas en que se guardan unos campos.
func tablasDe(definiciones []DefinicionCampo) []string {
	var tablas []string
	for _, def := range definiciones {
		if !slices.Contains(tablas, def.tabla) {
			tablas = append(tablas, def.tabla)
		}
	}
	return tablas
}

// completarPostulacion añade a una postulación todo lo capturado: campos,
// integrantes, videos, obras, documentos vigentes y acuses.
func completarPostulacion(ctx context.Context, q consultor, p *Postulacion, tipo TipoConvocatoria) error {
	formulario := FormularioDe(tipo, p.Subcategoria)
	p.Campos = make(map[string]string)
	for _, tabla := range tablasDe(formulario.Campos) {
		if err := leerCampos(ctx, q, tabla, "postulacion_id", p.ID, formulario.Campos, p.Campos); err != nil {
			return err
		}
	}

	filas, err := q.QueryContext(ctx, "SELECT nombre, instrumento FROM integrante WHERE postulacion_id = ? ORDER BY orden", p.ID)
	if err != nil {
		return fmt.Errorf("leer integrantes: %w", err)
	}
	for filas.Next() {
		var i Integrante
		if err := filas.Scan(&i.Nombre, &i.Instrumento); err != nil {
			filas.Close()
			return fmt.Errorf("leer integrantes: %w", err)
		}
		p.Integrantes = append(p.Integrantes, i)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return fmt.Errorf("leer integrantes: %w", err)
	}

	filas, err = q.QueryContext(ctx, "SELECT url, estado, COALESCE(plataforma, ''), COALESCE(detalle, '') FROM enlace_video WHERE postulacion_id = ? ORDER BY orden", p.ID)
	if err != nil {
		return fmt.Errorf("leer videos: %w", err)
	}
	for filas.Next() {
		var v EnlaceVideo
		var estado string
		if err := filas.Scan(&v.URL, &estado, &v.Plataforma, &v.Detalle); err != nil {
			filas.Close()
			return fmt.Errorf("leer videos: %w", err)
		}
		v.Estado = EstadoEnlace(estado)
		p.Videos = append(p.Videos, v)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return fmt.Errorf("leer videos: %w", err)
	}

	filas, err = q.QueryContext(ctx, "SELECT id, orden FROM obra WHERE postulacion_id = ? ORDER BY orden", p.ID)
	if err != nil {
		return fmt.Errorf("leer obras: %w", err)
	}
	for filas.Next() {
		obra := Obra{Campos: make(map[string]string)}
		if err := filas.Scan(&obra.ID, &obra.Orden); err != nil {
			filas.Close()
			return fmt.Errorf("leer obras: %w", err)
		}
		p.Obras = append(p.Obras, obra)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return fmt.Errorf("leer obras: %w", err)
	}
	for i := range p.Obras {
		if err := leerCampos(ctx, q, "obra", "id", p.Obras[i].ID, formulario.CamposObra, p.Obras[i].Campos); err != nil {
			return err
		}
	}
	if p.Documentos, err = documentosVigentes(ctx, q, p.ID); err != nil {
		return err
	}
	if p.Acuses, err = acusesDe(ctx, q, p.ID); err != nil {
		return err
	}
	p.Subsanaciones, err = subsanacionesDe(ctx, q, p.ID)
	return err
}

// tipoDeConvocatoria devuelve el tipo de una convocatoria.
func tipoDeConvocatoria(ctx context.Context, q consultor, convocatoriaID int64) (TipoConvocatoria, error) {
	var tipo string
	err := q.QueryRowContext(ctx, "SELECT tipo FROM convocatoria WHERE id = ?", convocatoriaID).Scan(&tipo)
	return TipoConvocatoria(tipo), err
}

// PostulacionPorID devuelve una postulación con todo lo capturado, o
// ErrNoEncontrado. NO comprueba quién la pide: decidir si puede verla es
// asunto del controlador, con la regla PuedeVerPostulacion.
func (r *Repositorio) PostulacionPorID(ctx context.Context, id int64) (Postulacion, error) {
	p, err := leerPostulacion(r.lectura.QueryRowContext(ctx, sqlPostulacion+" WHERE p.id = ?", id))
	if errors.Is(err, ErrNoEncontrado) {
		return Postulacion{}, err
	}
	if err != nil {
		return Postulacion{}, fmt.Errorf("leer postulación: %w", err)
	}
	tipo, err := tipoDeConvocatoria(ctx, r.lectura, p.ConvocatoriaID)
	if err != nil {
		return Postulacion{}, fmt.Errorf("leer postulación: %w", err)
	}
	if err := completarPostulacion(ctx, r.lectura, &p, tipo); err != nil {
		return Postulacion{}, fmt.Errorf("leer postulación: %w", err)
	}
	return p, nil
}

// PostulacionesDeUsuario devuelve las postulaciones de una cuenta en todas las
// ediciones, la más reciente primero: su historial (RF-POST-12).
func (r *Repositorio) PostulacionesDeUsuario(ctx context.Context, usuarioID int64) ([]ResumenPostulacion, error) {
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT p.id, COALESCE(p.subcategoria, ''), COALESCE(p.folio, ''), p.estado, p.actualizado_at,
		       c.id, c.tipo, c.titulo_es, c.titulo_en, c.estado, e.id, e.numero, e.anio, e.nombre_es, e.nombre_en
		FROM postulacion p
		JOIN convocatoria c ON c.id = p.convocatoria_id
		JOIN edicion e ON e.id = c.edicion_id
		WHERE p.usuario_id = ?
		ORDER BY e.numero DESC, c.id`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("leer postulaciones: %w", err)
	}
	defer filas.Close()
	var lista []ResumenPostulacion
	for filas.Next() {
		var rp ResumenPostulacion
		var subcategoria, estado, actualizado, tipo, estadoConvocatoria string
		c := &rp.Convocatoria
		if err := filas.Scan(&rp.ID, &subcategoria, &rp.Folio, &estado, &actualizado,
			&c.ID, &tipo, &c.TituloES, &c.TituloEN, &estadoConvocatoria,
			&c.Edicion.ID, &c.Edicion.Numero, &c.Edicion.Anio, &c.Edicion.NombreES, &c.Edicion.NombreEN); err != nil {
			return nil, fmt.Errorf("leer postulaciones: %w", err)
		}
		rp.Subcategoria, rp.Estado = Subcategoria(subcategoria), EstadoDePostulacion(estado)
		c.Tipo, c.Estado = TipoConvocatoria(tipo), EstadoConvocatoria(estadoConvocatoria)
		if rp.ActualizadoAt, err = deMarca(actualizado); err != nil {
			return nil, err
		}
		lista = append(lista, rp)
	}
	return lista, filas.Err()
}

// CrearPostulacion abre una postulación en borrador para una cuenta en una
// convocatoria (RF-POST-01). Aplica, en la misma transacción: que la
// convocatoria esté en plazo (RN-02), que la cuenta no sea jurado ni revisor
// de ella, y que no tenga ya una postulación ahí (RN-01).
func (r *Repositorio) CrearPostulacion(ctx context.Context, actor Actor, convocatoriaID, usuarioID int64, sub Subcategoria) (int64, error) {
	var id int64
	err := r.operacion(ctx, "crear postulación", func(tx *sql.Tx) error {
		c, err := convocatoriaPorID(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		if !ConvocatoriaVisibleAlPublico(c) {
			return ErrNoEncontrado // un borrador no existe para el público
		}
		asignado, err := existe(ctx, tx,
			"SELECT COUNT(*) FROM asignacion WHERE convocatoria_id = ? AND usuario_id = ? AND activo = 1", convocatoriaID, usuarioID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarNuevaPostulacion(c, sub, asignado, r.ahora())); err != nil {
			return err
		}
		if repetida, err := existe(ctx, tx,
			"SELECT COUNT(*) FROM postulacion WHERE convocatoria_id = ? AND usuario_id = ?", convocatoriaID, usuarioID); err != nil {
			return err
		} else if repetida {
			return ErrDuplicado
		}
		marca := r.marca()
		res, err := tx.ExecContext(ctx,
			"INSERT INTO postulacion (convocatoria_id, usuario_id, subcategoria, creado_at, actualizado_at) VALUES (?, ?, ?, ?, ?)",
			convocatoriaID, usuarioID, nuloSiVacio(string(sub)), marca, marca)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		// Las filas 1:1 de datos nacen vacías: así el autoguardado es siempre
		// un UPDATE de una sola fila (§7.5).
		for _, tabla := range tablasDe(FormularioDe(c.Tipo, sub).Campos) {
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+tabla+" (postulacion_id) VALUES (?)", id); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "postulacion.crear", "postulacion", id, nil,
			map[string]any{"convocatoria_id": convocatoriaID, "subcategoria": sub})
	})
	return id, err
}

// postulacionEditable lee una postulación de la cuenta dada, con su
// convocatoria, y calcula qué puede modificar el postulante ahora (RN-02,
// RN-04): todo si es un borrador en plazo; sólo lo señalado por el revisor si
// está en subsanación. Si no puede modificar nada devuelve el motivo.
//
// Quien llama debe consultar el permiso devuelto antes de escribir: es lo que
// hace cumplir el desbloqueo selectivo (§4.2).
func (r *Repositorio) postulacionEditable(ctx context.Context, tx *sql.Tx, usuarioID, postulacionID int64) (Postulacion, ConfiguracionConvocatoria, PermisoDeEdicion, error) {
	var sinPermiso PermisoDeEdicion
	p, err := leerPostulacion(tx.QueryRowContext(ctx, sqlPostulacion+" WHERE p.id = ? AND p.usuario_id = ?", postulacionID, usuarioID))
	if err != nil {
		return Postulacion{}, ConfiguracionConvocatoria{}, sinPermiso, err
	}
	cfg, err := configuracion(ctx, tx, p.ConvocatoriaID)
	if err != nil {
		return Postulacion{}, ConfiguracionConvocatoria{}, sinPermiso, err
	}
	if p.Subsanaciones, err = subsanacionesDe(ctx, tx, p.ID); err != nil {
		return Postulacion{}, ConfiguracionConvocatoria{}, sinPermiso, err
	}
	permiso, errores := PermisoDeEdicionDe(p, cfg.Convocatoria, r.ahora())
	return p, cfg, permiso, validacion(errores)
}

// errBloqueado es la respuesta a quien intenta modificar, durante una
// subsanación, un elemento que el revisor no señaló.
func errBloqueado(campo string) error {
	return validacion([]ErrorCampo{{Campo: campo, Clave: "error.elemento_bloqueado"}})
}

// tocar actualiza la marca de última modificación de una postulación.
func (r *Repositorio) tocar(ctx context.Context, tx *sql.Tx, postulacionID int64) (time.Time, error) {
	ahora := r.ahora().UTC().Truncate(time.Second)
	_, err := tx.ExecContext(ctx, "UPDATE postulacion SET actualizado_at = ? WHERE id = ?", aMarca(ahora), postulacionID)
	return ahora, err
}

// guardarCampos aplica un guardado parcial sobre una fila: valida el formato
// de cada valor y escribe los aceptables, cada uno en su columna. Los
// rechazados no se guardan y se devuelven con su motivo. Las rutas se
// recorren en orden alfabético para que el resultado sea determinista.
func guardarCampos(ctx context.Context, tx *sql.Tx, buscar func(string) (DefinicionCampo, bool), llave string, id int64,
	cambios map[string]string, contexto ContextoCampos, permitido func(ruta string) bool) (ResultadoGuardado, error) {
	var resultado ResultadoGuardado
	rutas := make([]string, 0, len(cambios))
	for ruta := range cambios {
		rutas = append(rutas, ruta)
	}
	sort.Strings(rutas)
	for _, ruta := range rutas {
		def, ok := buscar(ruta)
		if !ok {
			// Un campo que no es de este formulario: ni se guarda ni se ignora
			// en silencio.
			resultado.Errores = append(resultado.Errores, ErrorCampo{Campo: ruta, Clave: "error.campo_desconocido"})
			continue
		}
		if !permitido(ruta) {
			// Durante una subsanación sólo se guarda lo que el revisor señaló.
			resultado.Errores = append(resultado.Errores, ErrorCampo{Campo: ruta, Clave: "error.elemento_bloqueado"})
			continue
		}
		valor := strings.TrimSpace(cambios[ruta])
		if clave := ValidarValorDeCampo(def, valor, contexto); clave != "" {
			resultado.Errores = append(resultado.Errores, ErrorCampo{Campo: ruta, Clave: clave})
			continue
		}
		// Vacío se guarda como NULL; los enteros, como enteros.
		var dato any = nuloSiVacio(valor)
		if valor != "" && (def.Tipo == CampoEntero || def.Tipo == CampoAnio || def.Tipo == CampoDotacion) {
			n, err := strconv.ParseInt(valor, 10, 64)
			if err != nil {
				return resultado, err // ya validado: no debería ocurrir
			}
			dato = n
		}
		consulta := "UPDATE " + def.tabla + " SET " + def.columna + " = ?"
		if ruta == "obra.liga_media" {
			// Una liga nueva está por comprobar.
			consulta += ", liga_media_estado = 'pendiente'"
		}
		if _, err := tx.ExecContext(ctx, consulta+" WHERE "+llave+" = ?", dato, id); err != nil {
			return resultado, fmt.Errorf("guardar %s: %w", ruta, err)
		}
		resultado.Guardados = append(resultado.Guardados, ruta)
	}
	return resultado, nil
}

// GuardarCamposDePostulacion es el autoguardado de los datos generales y de la
// propuesta de concierto (RF-POST-03): recibe campos sueltos, por ruta
// canónica, y guarda cada uno en su columna. No exige que la postulación esté
// completa; sólo que cada valor tenga un formato aceptable.
func (r *Repositorio) GuardarCamposDePostulacion(ctx context.Context, usuarioID, postulacionID int64, cambios map[string]string) (ResultadoGuardado, error) {
	var resultado ResultadoGuardado
	err := r.operacion(ctx, "guardar postulación", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		formulario := FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria)
		// La entidad de residencia se valida contra el país que quedará
		// guardado: el de esta misma petición, si lo trae, o el actual.
		pais, viene := cambios["datos.pais_residencia"]
		if !viene {
			actuales := make(map[string]string)
			if err := leerCampos(ctx, tx, tablaDeDatos(cfg.Convocatoria.Tipo, p.Subcategoria), "postulacion_id", p.ID, formulario.Campos, actuales); err != nil {
				return err
			}
			pais = actuales["datos.pais_residencia"]
		}
		contexto := ContextoCampos{Ahora: r.ahora(), PaisResidencia: strings.TrimSpace(pais), Dotaciones: cfg.Dotaciones}
		if resultado, err = guardarCampos(ctx, tx, formulario.Campo, "postulacion_id", p.ID, cambios, contexto,
			func(ruta string) bool { return permiso.Campo(ruta, 0) }); err != nil {
			return err
		}
		resultado.GuardadoAt, err = r.tocar(ctx, tx, p.ID)
		return err
	})
	return resultado, err
}

// obraEditable localiza una obra de una postulación de la cuenta dada y
// comprueba que pueda modificarse.
func (r *Repositorio) obraEditable(ctx context.Context, tx *sql.Tx, usuarioID, obraID int64) (Postulacion, ConfiguracionConvocatoria, PermisoDeEdicion, error) {
	var postulacionID int64
	err := tx.QueryRowContext(ctx, `
		SELECT o.postulacion_id FROM obra o JOIN postulacion p ON p.id = o.postulacion_id
		WHERE o.id = ? AND p.usuario_id = ?`, obraID, usuarioID).Scan(&postulacionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Postulacion{}, ConfiguracionConvocatoria{}, PermisoDeEdicion{}, ErrNoEncontrado
	}
	if err != nil {
		return Postulacion{}, ConfiguracionConvocatoria{}, PermisoDeEdicion{}, err
	}
	return r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
}

// GuardarCamposDeObra es el autoguardado de una obra. Devuelve además el id
// de la postulación a la que pertenece.
func (r *Repositorio) GuardarCamposDeObra(ctx context.Context, usuarioID, obraID int64, cambios map[string]string) (int64, ResultadoGuardado, error) {
	var resultado ResultadoGuardado
	var postulacionID int64
	err := r.operacion(ctx, "guardar obra", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.obraEditable(ctx, tx, usuarioID, obraID)
		if err != nil {
			return err
		}
		postulacionID = p.ID
		formulario := FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria)
		contexto := ContextoCampos{Ahora: r.ahora(), Dotaciones: cfg.Dotaciones}
		if resultado, err = guardarCampos(ctx, tx, formulario.CampoDeObra, "id", obraID, cambios, contexto,
			func(ruta string) bool { return permiso.Campo(ruta, obraID) }); err != nil {
			return err
		}
		resultado.GuardadoAt, err = r.tocar(ctx, tx, p.ID)
		return err
	})
	return postulacionID, resultado, err
}

// AgregarObra añade una obra vacía al final del programa (RF-POST-05) y
// devuelve su id. En Compositores respeta el máximo de obras (RN-05).
func (r *Repositorio) AgregarObra(ctx context.Context, usuarioID, postulacionID int64) (int64, error) {
	var id int64
	err := r.operacion(ctx, "agregar obra", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Campo(CampoObras, 0) {
			return errBloqueado("obras")
		}
		var cuantas, ultimoOrden int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(MAX(orden), 0) FROM obra WHERE postulacion_id = ?", p.ID).
			Scan(&cuantas, &ultimoOrden); err != nil {
			return err
		}
		if err := validacion(ValidarNuevaObra(cfg.Convocatoria, cuantas)); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO obra (postulacion_id, orden, creado_at) VALUES (?, ?, ?)", p.ID, ultimoOrden+1, r.marca())
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = r.tocar(ctx, tx, p.ID)
		return err
	})
	return id, err
}

// BorrarObra quita una obra de un borrador, renumera las que quedan y
// devuelve el id de su postulación.
// Sólo es posible mientras la postulación es editable: lo que se envió queda
// en el snapshot del acuse y ya no se borra.
func (r *Repositorio) BorrarObra(ctx context.Context, usuarioID, obraID int64) (int64, error) {
	var postulacionID int64
	err := r.operacion(ctx, "borrar obra", func(tx *sql.Tx) error {
		p, _, permiso, err := r.obraEditable(ctx, tx, usuarioID, obraID)
		if err != nil {
			return err
		}
		if !permiso.Campo(CampoObras, 0) {
			return errBloqueado("obras")
		}
		postulacionID = p.ID
		// Con la obra se van sus documentos (los archivos siguen en el
		// almacén). Si alguno se descargó, el registro de esa descarga se
		// conserva, ligado a la postulación; el documento que ya no existe
		// queda identificado en la bitácora, que no se toca.
		if _, err := tx.ExecContext(ctx,
			"UPDATE descarga SET documento_id = NULL WHERE documento_id IN (SELECT id FROM documento WHERE obra_id = ?)", obraID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM obra WHERE id = ?", obraID); err != nil {
			return err
		}
		// Se cierra el hueco que deja la obra en el orden del programa, para
		// que siempre sea 1, 2, 3… En orden ascendente cada obra baja a un
		// lugar ya libre, así que no choca con la unicidad del orden.
		filas, err := tx.QueryContext(ctx, "SELECT id FROM obra WHERE postulacion_id = ? ORDER BY orden", p.ID)
		if err != nil {
			return err
		}
		var restantes []int64
		for filas.Next() {
			var id int64
			if err := filas.Scan(&id); err != nil {
				filas.Close()
				return err
			}
			restantes = append(restantes, id)
		}
		filas.Close()
		if err := filas.Err(); err != nil {
			return err
		}
		for i, id := range restantes {
			if _, err := tx.ExecContext(ctx, "UPDATE obra SET orden = ? WHERE id = ?", i+1, id); err != nil {
				return err
			}
		}
		_, err = r.tocar(ctx, tx, p.ID)
		return err
	})
	return postulacionID, err
}

// OrdenarObras fija el orden del programa. ids debe traer exactamente las
// obras de la postulación, en el orden deseado.
func (r *Repositorio) OrdenarObras(ctx context.Context, usuarioID, postulacionID int64, ids []int64) error {
	return r.operacion(ctx, "ordenar obras", func(tx *sql.Tx) error {
		p, _, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Campo(CampoObras, 0) {
			return errBloqueado("obras")
		}
		filas, err := tx.QueryContext(ctx, "SELECT id FROM obra WHERE postulacion_id = ?", p.ID)
		if err != nil {
			return err
		}
		var actuales []int64
		for filas.Next() {
			var id int64
			if err := filas.Scan(&id); err != nil {
				filas.Close()
				return err
			}
			actuales = append(actuales, id)
		}
		filas.Close()
		if err := filas.Err(); err != nil {
			return err
		}
		pedidas := slices.Clone(ids)
		slices.Sort(actuales)
		slices.Sort(pedidas)
		if !slices.Equal(actuales, pedidas) {
			return validacion([]ErrorCampo{{Campo: "ids", Clave: "error.orden_incompleto"}})
		}
		// El orden es único por postulación: se pasa por valores negativos
		// para no chocar con el orden que otra obra aún ocupa.
		for i, id := range ids {
			if _, err := tx.ExecContext(ctx, "UPDATE obra SET orden = ? WHERE id = ?", -(i + 1), id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE obra SET orden = -orden WHERE postulacion_id = ?", p.ID); err != nil {
			return err
		}
		_, err = r.tocar(ctx, tx, p.ID)
		return err
	})
}

// DefinirIntegrantes sustituye la lista de integrantes de un ensamble
// (RF-POST-06). El orden de la lista es el orden en que se guardan.
func (r *Repositorio) DefinirIntegrantes(ctx context.Context, usuarioID, postulacionID int64, integrantes []Integrante) error {
	return r.operacion(ctx, "definir integrantes", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Campo(CampoIntegrantes, 0) {
			return errBloqueado("integrantes")
		}
		if !FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria).TieneIntegrantes {
			return validacion([]ErrorCampo{{Campo: "integrantes", Clave: "error.integrantes_no_aplican"}})
		}
		if err := validacion(ValidarIntegrantes(integrantes)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM integrante WHERE postulacion_id = ?", p.ID); err != nil {
			return err
		}
		for i, integrante := range integrantes {
			if _, err := tx.ExecContext(ctx, "INSERT INTO integrante (postulacion_id, nombre, instrumento, orden) VALUES (?, ?, ?, ?)",
				p.ID, strings.TrimSpace(integrante.Nombre), strings.TrimSpace(integrante.Instrumento), i+1); err != nil {
				return err
			}
		}
		_, err = r.tocar(ctx, tx, p.ID)
		return err
	})
}

// DefinirVideos sustituye las ligas a videos de ejecución (RF-POST-08). Cada
// liga queda «pendiente» hasta que el sistema la compruebe por red.
func (r *Repositorio) DefinirVideos(ctx context.Context, usuarioID, postulacionID int64, ligas []string) error {
	return r.operacion(ctx, "definir videos", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Campo(CampoVideos, 0) {
			return errBloqueado("videos")
		}
		if !FormularioDe(cfg.Convocatoria.Tipo, p.Subcategoria).TieneVideos {
			return validacion([]ErrorCampo{{Campo: "videos", Clave: "error.videos_no_aplican"}})
		}
		if err := validacion(ValidarVideos(ligas, cfg.Convocatoria.MaxVideos)); err != nil {
			return err
		}
		// La lista se sustituye completa, pero una liga que no cambia conserva
		// el resultado de su comprobación: no hay por qué repetirla.
		type comprobacion struct {
			estado       string
			estadoHTTP   sql.NullInt64
			verificadoAt sql.NullString
			detalle      sql.NullString
		}
		anteriores := make(map[string]comprobacion)
		filas, err := tx.QueryContext(ctx, "SELECT url, estado, http_status, verificado_at, detalle FROM enlace_video WHERE postulacion_id = ?", p.ID)
		if err != nil {
			return err
		}
		for filas.Next() {
			var liga string
			var c comprobacion
			if err := filas.Scan(&liga, &c.estado, &c.estadoHTTP, &c.verificadoAt, &c.detalle); err != nil {
				filas.Close()
				return err
			}
			anteriores[liga] = c
		}
		filas.Close()
		if err := filas.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM enlace_video WHERE postulacion_id = ?", p.ID); err != nil {
			return err
		}
		for i, liga := range ligas {
			c, conocida := anteriores[liga]
			if !conocida {
				c.estado = string(EnlacePendiente)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO enlace_video (postulacion_id, url, orden, plataforma, estado, http_status, verificado_at, detalle)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, liga, i+1, PlataformaDeVideo(liga), c.estado, c.estadoHTTP, c.verificadoAt, c.detalle); err != nil {
				return err
			}
		}
		_, err = r.tocar(ctx, tx, p.ID)
		return err
	})
}

// CambiarSubcategoria cambia un borrador de solista a ensamble o al revés
// (RF-POST-02). Los datos generales de la subcategoría anterior —y los
// integrantes— SE PIERDEN: son formularios distintos. Se conservan la
// propuesta de concierto, las obras y los videos, que son comunes.
func (r *Repositorio) CambiarSubcategoria(ctx context.Context, actor Actor, usuarioID, postulacionID int64, sub Subcategoria) error {
	return r.operacion(ctx, "cambiar subcategoría", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Total {
			return errBloqueado("subcategoria")
		}
		if cfg.Convocatoria.Tipo != TipoInterpretes {
			return validacion([]ErrorCampo{{Campo: "subcategoria", Clave: "error.subcategoria_no_aplica"}})
		}
		if sub != SubcategoriaSolista && sub != SubcategoriaEnsamble {
			return validacion([]ErrorCampo{{Campo: "subcategoria", Clave: "error.subcategoria_requerida"}})
		}
		if sub == p.Subcategoria {
			return nil
		}
		for _, consulta := range []string{
			"DELETE FROM " + tablaDeDatos(TipoInterpretes, p.Subcategoria) + " WHERE postulacion_id = ?",
			"DELETE FROM integrante WHERE postulacion_id = ?",
			"INSERT INTO " + tablaDeDatos(TipoInterpretes, sub) + " (postulacion_id) VALUES (?)",
		} {
			if _, err := tx.ExecContext(ctx, consulta, p.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE postulacion SET subcategoria = ? WHERE id = ?", string(sub), p.ID); err != nil {
			return err
		}
		if _, err := r.tocar(ctx, tx, p.ID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "postulacion.subcategoria", "postulacion", p.ID,
			map[string]any{"subcategoria": p.Subcategoria}, map[string]any{"subcategoria": sub})
	})
}

// =============================================================================
// 11. Documentos, envío y acuses
// =============================================================================

// sqlDocumento lee un documento con los metadatos de su archivo y la clave de
// su requisito.
const sqlDocumento = `
SELECT d.id, d.postulacion_id, COALESCE(d.obra_id, 0), rq.clave, d.version, d.vigente, d.dictamen, d.subido_at,
       a.id, a.sha256, a.clave, a.mime, a.tamano_bytes
FROM documento d
JOIN requisito_documental rq ON rq.id = d.requisito_id
JOIN archivo a ON a.id = d.archivo_id`

// leerDocumento vuelca una fila de sqlDocumento en la entidad.
func leerDocumento(f fila) (Documento, error) {
	var d Documento
	var subido string
	err := f.Scan(&d.ID, &d.PostulacionID, &d.ObraID, &d.Clave, &d.Version, &d.Vigente, &d.Dictamen, &subido,
		&d.Archivo.ID, &d.Archivo.SHA256, &d.Archivo.Clave, &d.Archivo.MIME, &d.Archivo.TamanoBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return Documento{}, ErrNoEncontrado
	}
	if err != nil {
		return Documento{}, err
	}
	d.SubidoAt, err = deMarca(subido)
	return d, err
}

// documentosVigentes devuelve los documentos vigentes de una postulación:
// primero los de la postulación y luego los de cada obra, en el orden de sus
// requisitos.
func documentosVigentes(ctx context.Context, q consultor, postulacionID int64) ([]Documento, error) {
	filas, err := q.QueryContext(ctx, sqlDocumento+`
		WHERE d.postulacion_id = ? AND d.vigente = 1
		ORDER BY COALESCE(d.obra_id, 0), rq.orden, d.id`, postulacionID)
	if err != nil {
		return nil, fmt.Errorf("leer documentos: %w", err)
	}
	defer filas.Close()
	var documentos []Documento
	for filas.Next() {
		d, err := leerDocumento(filas)
		if err != nil {
			return nil, fmt.Errorf("leer documentos: %w", err)
		}
		documentos = append(documentos, d)
	}
	return documentos, filas.Err()
}

// acusesDe devuelve los acuses de una postulación, del primero al último.
func acusesDe(ctx context.Context, q consultor, postulacionID int64) ([]Acuse, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT ac.id, ac.postulacion_id, ac.version, ac.idioma, ac.generado_at,
		       a.id, a.sha256, a.clave, a.mime, a.tamano_bytes
		FROM acuse ac JOIN archivo a ON a.id = ac.archivo_id
		WHERE ac.postulacion_id = ? ORDER BY ac.version`, postulacionID)
	if err != nil {
		return nil, fmt.Errorf("leer acuses: %w", err)
	}
	defer filas.Close()
	var acuses []Acuse
	for filas.Next() {
		var ac Acuse
		var generado string
		if err := filas.Scan(&ac.ID, &ac.PostulacionID, &ac.Version, &ac.Idioma, &generado,
			&ac.Archivo.ID, &ac.Archivo.SHA256, &ac.Archivo.Clave, &ac.Archivo.MIME, &ac.Archivo.TamanoBytes); err != nil {
			return nil, fmt.Errorf("leer acuses: %w", err)
		}
		if ac.GeneradoAt, err = deMarca(generado); err != nil {
			return nil, err
		}
		acuses = append(acuses, ac)
	}
	return acuses, filas.Err()
}

// guardarArchivo registra en la tabla «archivo» un contenido que ya está en el
// almacén y devuelve su id. Si ese contenido ya estaba registrado (mismo
// SHA-256) devuelve el id existente: es la deduplicación de RF-DOC-02.
func (r *Repositorio) guardarArchivo(ctx context.Context, tx *sql.Tx, blob Blob, mime, nombreOriginal string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM archivo WHERE sha256 = ?", blob.SHA256).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("buscar archivo: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		"INSERT INTO archivo (sha256, clave, mime, tamano_bytes, nombre_original, creado_at) VALUES (?, ?, ?, ?, ?, ?)",
		blob.SHA256, blob.Clave, mime, blob.Tamano, nombreOriginal, r.marca())
	if err != nil {
		return 0, fmt.Errorf("registrar archivo: %w", err)
	}
	return res.LastInsertId()
}

// RegistrarDocumento añade a una postulación un documento cuyo contenido ya
// está en el almacén. Aplica en la misma transacción las reglas de la subida:
// que la postulación sea editable, que el requisito exista para esa obra, que
// el formato y el tamaño sean los admitidos, y qué documento queda
// reemplazado. El anterior no se borra: deja de estar vigente (RF-DOC-07).
func (r *Repositorio) RegistrarDocumento(ctx context.Context, actor Actor, usuarioID, postulacionID int64, n NuevoDocumento) (Documento, error) {
	var documento Documento
	err := r.operacion(ctx, "registrar documento", func(tx *sql.Tx) error {
		p, cfg, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Documento(n.Clave, n.ObraID) {
			return errBloqueado("archivo")
		}
		if err := completarPostulacion(ctx, tx, &p, cfg.Convocatoria.Tipo); err != nil {
			return err
		}
		requisito, errores := RequisitoParaDocumento(p, cfg, n.Clave, n.ObraID)
		if err := validacion(errores); err != nil {
			return err
		}
		if err := validacion(ValidarArchivo(requisito, n.NombreOriginal, n.MIME, n.Blob.Tamano)); err != nil {
			return err
		}
		var vigentes []Documento
		for _, d := range p.Documentos {
			if d.Clave == n.Clave && d.ObraID == n.ObraID {
				vigentes = append(vigentes, d)
			}
		}
		var versiones int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM documento d JOIN requisito_documental rq ON rq.id = d.requisito_id
			WHERE d.postulacion_id = ? AND COALESCE(d.obra_id, 0) = ? AND rq.clave = ?`,
			p.ID, n.ObraID, n.Clave).Scan(&versiones); err != nil {
			return err
		}
		anterior, errores := LugarDeNuevoDocumento(requisito, vigentes, n.ReemplazaA, versiones)
		if err := validacion(errores); err != nil {
			return err
		}

		archivoID, err := r.guardarArchivo(ctx, tx, n.Blob, n.MIME, n.NombreOriginal)
		if err != nil {
			return err
		}
		version, reemplazaA := 1, int64(0)
		if anterior != nil {
			version, reemplazaA = anterior.Version+1, anterior.ID
			if _, err := tx.ExecContext(ctx, "UPDATE documento SET vigente = 0 WHERE id = ?", anterior.ID); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO documento (postulacion_id, obra_id, requisito_id, archivo_id, version, reemplaza_a, vigente, subido_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
			p.ID, nuloSiCero(n.ObraID), requisito.ID, archivoID, version, nuloSiCero(reemplazaA), r.marca())
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if documento, err = leerDocumento(tx.QueryRowContext(ctx, sqlDocumento+" WHERE d.id = ?", id)); err != nil {
			return err
		}
		if _, err := r.tocar(ctx, tx, p.ID); err != nil {
			return err
		}
		// La bitácora guarda la suma del archivo, no su nombre: el nombre que
		// le puso el postulante puede traer datos personales.
		return r.auditar(ctx, tx, actor, "documento.subir", "documento", id, nil, map[string]any{
			"postulacion_id": p.ID, "obra_id": n.ObraID, "requisito": n.Clave, "version": version,
			"sha256": n.Blob.SHA256, "tamano_bytes": n.Blob.Tamano, "reemplaza_a": reemplazaA,
		})
	})
	return documento, err
}

// QuitarDocumento retira un documento de un borrador y devuelve el id de su
// postulación. No lo borra: deja de estar vigente, y su archivo se conserva.
func (r *Repositorio) QuitarDocumento(ctx context.Context, actor Actor, usuarioID, documentoID int64) (int64, error) {
	var postulacionID int64
	err := r.operacion(ctx, "quitar documento", func(tx *sql.Tx) error {
		var clave string
		var obraID int64
		err := tx.QueryRowContext(ctx, `
			SELECT d.postulacion_id, rq.clave, COALESCE(d.obra_id, 0)
			FROM documento d
			JOIN postulacion p ON p.id = d.postulacion_id
			JOIN requisito_documental rq ON rq.id = d.requisito_id
			WHERE d.id = ? AND d.vigente = 1 AND p.usuario_id = ?`, documentoID, usuarioID).Scan(&postulacionID, &clave, &obraID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEncontrado
		}
		if err != nil {
			return err
		}
		_, _, permiso, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		if !permiso.Documento(clave, obraID) {
			return errBloqueado("documento")
		}
		if _, err := tx.ExecContext(ctx, "UPDATE documento SET vigente = 0 WHERE id = ?", documentoID); err != nil {
			return err
		}
		if _, err := r.tocar(ctx, tx, postulacionID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "documento.quitar", "documento", documentoID, nil,
			map[string]any{"postulacion_id": postulacionID})
	})
	return postulacionID, err
}

// DocumentoPorID devuelve un documento, vigente o no, o ErrNoEncontrado. NO
// comprueba quién lo pide: eso lo decide el controlador sobre su postulación.
func (r *Repositorio) DocumentoPorID(ctx context.Context, id int64) (Documento, error) {
	d, err := leerDocumento(r.lectura.QueryRowContext(ctx, sqlDocumento+" WHERE d.id = ?", id))
	if err != nil && !errors.Is(err, ErrNoEncontrado) {
		return Documento{}, fmt.Errorf("leer documento: %w", err)
	}
	return d, err
}

// AcusePorVersion devuelve un acuse de una postulación, o ErrNoEncontrado.
func (r *Repositorio) AcusePorVersion(ctx context.Context, postulacionID int64, version int) (Acuse, error) {
	acuses, err := acusesDe(ctx, r.lectura, postulacionID)
	if err != nil {
		return Acuse{}, err
	}
	for _, ac := range acuses {
		if ac.Version == version {
			return ac, nil
		}
	}
	return Acuse{}, ErrNoEncontrado
}

// RegistrarDescarga deja constancia de que una cuenta descargó un documento o
// un acuse (RN-27): el acervo es material inédito y bajo derechos de autor.
// documentoID es 0 cuando lo descargado es un acuse. Queda en la tabla
// «descarga» y en la bitácora, en la misma transacción.
func (r *Repositorio) RegistrarDescarga(ctx context.Context, actor Actor, postulacionID, documentoID int64, archivo Archivo) error {
	return r.operacion(ctx, "registrar descarga", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO descarga (usuario_id, documento_id, postulacion_id, ip, creado_at) VALUES (?, ?, ?, ?, ?)",
			actor.UsuarioID, nuloSiCero(documentoID), postulacionID, nuloSiVacio(actor.IP), r.marca()); err != nil {
			return err
		}
		accion, entidad, entidadID := "documento.descargar", "documento", documentoID
		if documentoID == 0 {
			accion, entidad, entidadID = "acuse.descargar", "postulacion", postulacionID
		}
		return r.auditar(ctx, tx, actor, accion, entidad, entidadID, nil,
			map[string]any{"postulacion_id": postulacionID, "sha256": archivo.SHA256})
	})
}

// EnviarPostulacion envía una postulación: es irreversible (§4.1). En UNA sola
// transacción:
//
//  1. comprueba que sea un borrador de la cuenta, dentro de plazo, o una
//     postulación con una subsanación abierta (entonces es un REENVÍO: mismo
//     folio, acuse nuevo con la versión siguiente, y vuelve a revisión);
//  2. ejecuta la validación completa (ValidarParaEnvio) y las confirmaciones;
//  3. asigna el folio y congela lo enviado en un snapshot;
//  4. pide a la Vista el acuse en PDF y el correo, con la función componer;
//  5. guarda el acuse, marca la postulación como enviada, encola el correo con
//     el PDF adjunto y deja el asiento en la bitácora.
//
// Si algo falla no queda nada escrito: ni folio, ni acuse, ni correo. El
// correo no se envía dentro de la transacción: sale después, por la bandeja.
//
// componer recibe el snapshot y devuelve el PDF ya guardado en el almacén. Se
// pasa como función porque componer un PDF es trabajo de la Vista, que el
// Modelo no conoce.
//
// Si a la postulación le falta algo devuelve *ErrorDeEnvio con la lista.
func (r *Repositorio) EnviarPostulacion(ctx context.Context, actor Actor, usuarioID, postulacionID int64,
	confirmacion ConfirmacionEnvio, componer func(SnapshotEnvio) (AcuseCompuesto, error)) error {
	return r.operacion(ctx, "enviar postulación", func(tx *sql.Tx) error {
		// El permiso no se consulta: basta con que exista. Se envía un
		// borrador en plazo, o se reenvía una subsanación abierta.
		p, cfg, _, err := r.postulacionEditable(ctx, tx, usuarioID, postulacionID)
		if err != nil {
			return err
		}
		subsanacion := p.SubsanacionAbierta(r.ahora())
		if err := completarPostulacion(ctx, tx, &p, cfg.Convocatoria.Tipo); err != nil {
			return err
		}
		titular, err := leerUsuario(tx.QueryRowContext(ctx, "SELECT "+columnasUsuario+" FROM usuario u WHERE u.id = ?", usuarioID))
		if err != nil {
			return err
		}
		ahora := r.ahora().UTC().Truncate(time.Second)
		if bloqueadores := ValidarParaEnvio(p, cfg, titular, ahora); len(bloqueadores) > 0 {
			return &ErrorDeEnvio{Bloqueadores: bloqueadores}
		}
		if err := validacion(ValidarConfirmacionDeEnvio(confirmacion)); err != nil {
			return err
		}

		// El folio se asigna en el primer envío y ya no cambia. El consecutivo
		// es por convocatoria; con una sola conexión de escritura no hay dos
		// envíos que puedan leer el mismo.
		folio := p.Folio
		if folio == "" {
			var asignados int
			if err := tx.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM postulacion WHERE convocatoria_id = ? AND folio IS NOT NULL", p.ConvocatoriaID).Scan(&asignados); err != nil {
				return err
			}
			folio = FolioDe(cfg.Convocatoria.Edicion.Numero, cfg.Convocatoria.Tipo, asignados+1)
		}
		version := p.Envios + 1
		snapshot := SnapshotDeEnvio(p, cfg, titular, folio, version, ahora)
		congelado, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("serializar snapshot: %w", err)
		}
		compuesto, err := componer(snapshot)
		if err != nil {
			return fmt.Errorf("componer acuse: %w", err)
		}
		nombreAcuse := fmt.Sprintf("acuse_%s_v%d.pdf", folio, version)
		archivoID, err := r.guardarArchivo(ctx, tx, compuesto.Blob, "application/pdf", nombreAcuse)
		if err != nil {
			return err
		}
		marca := aMarca(ahora)
		// El primer envío deja la postulación «enviada»; el reenvío de una
		// subsanación la devuelve a revisión (§4).
		estado, accion := PostulacionEnviada, "postulacion.enviar"
		if subsanacion != nil {
			estado, accion = PostulacionEnRevision, "postulacion.reenviar"
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO acuse (postulacion_id, version, archivo_id, snapshot, idioma, generado_at) VALUES (?, ?, ?, ?, ?, ?)",
			p.ID, version, archivoID, string(congelado), snapshot.Idioma, marca); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE postulacion
			SET estado = ?, folio = ?, envios = ?, enviada_at = ?, primera_enviada_at = COALESCE(primera_enviada_at, ?),
			    bases_aceptadas_at = ?, actualizado_at = ?
			WHERE id = ?`, string(estado), folio, version, marca, marca, marca, marca, p.ID); err != nil {
			return err
		}
		if subsanacion != nil {
			// Reenvío: la subsanación queda atendida y sus problemas, resueltos
			// a juicio del postulante. El revisor decidirá si lo están.
			if _, err := tx.ExecContext(ctx, "UPDATE subsanacion SET atendida_at = ? WHERE id = ?", marca, subsanacion.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"UPDATE subsanacion_problema SET resuelto_at = ? WHERE subsanacion_id = ? AND resuelto_at IS NULL", marca, subsanacion.ID); err != nil {
				return err
			}
		}
		correo := compuesto.Correo
		correo.PostulacionID = p.ID
		correo.Adjuntos = []Adjunto{{ArchivoID: archivoID, Nombre: nombreAcuse}}
		if err := r.encolarCorreo(ctx, tx, correo); err != nil {
			return err
		}
		for _, aviso := range compuesto.Avisos {
			aviso.PostulacionID = p.ID
			if err := r.encolarCorreo(ctx, tx, aviso); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, accion, "postulacion", p.ID,
			map[string]any{"estado": p.Estado},
			map[string]any{"estado": estado, "folio": folio, "version": version, "acuse_sha256": compuesto.Blob.SHA256})
	})
}

// --- Ligas de video: comprobación en red (RF-DOC-10) --------------------------

// EnlacesPorVerificar devuelve ligas que toca comprobar: las que nunca se han
// comprobado y, mientras su convocatoria está en postulación o en revisión
// documental, las que se comprobaron antes de revalidarAntesDe.
func (r *Repositorio) EnlacesPorVerificar(ctx context.Context, limite int, revalidarAntesDe time.Time) ([]EnlacePorVerificar, error) {
	return r.enlaces(ctx, `
		SELECT v.id, v.url FROM enlace_video v
		JOIN postulacion p ON p.id = v.postulacion_id
		JOIN convocatoria c ON c.id = p.convocatoria_id
		WHERE v.verificado_at IS NULL
		   OR (v.verificado_at < ? AND c.estado IN ('postulacion', 'revision_documental'))
		ORDER BY v.verificado_at IS NOT NULL, v.verificado_at, v.id LIMIT ?`, aMarca(revalidarAntesDe), limite)
}

// EnlacesDePostulacion devuelve las ligas de una postulación, para
// comprobarlas todas antes de un envío (RN-08).
func (r *Repositorio) EnlacesDePostulacion(ctx context.Context, postulacionID int64) ([]EnlacePorVerificar, error) {
	return r.enlaces(ctx, "SELECT id, url FROM enlace_video WHERE postulacion_id = ? ORDER BY orden", postulacionID)
}

// enlaces ejecuta una consulta que devuelve id y url de ligas de video.
func (r *Repositorio) enlaces(ctx context.Context, consulta string, args ...any) ([]EnlacePorVerificar, error) {
	filas, err := r.lectura.QueryContext(ctx, consulta, args...)
	if err != nil {
		return nil, fmt.Errorf("leer ligas de video: %w", err)
	}
	defer filas.Close()
	var enlaces []EnlacePorVerificar
	for filas.Next() {
		var e EnlacePorVerificar
		if err := filas.Scan(&e.ID, &e.URL); err != nil {
			return nil, fmt.Errorf("leer ligas de video: %w", err)
		}
		enlaces = append(enlaces, e)
	}
	return enlaces, filas.Err()
}

// AnotarVerificacionDeEnlace guarda el resultado de comprobar una liga. Sólo
// lo anota si la liga sigue siendo la misma: si el postulante la cambió
// mientras se comprobaba, el resultado ya no le corresponde.
func (r *Repositorio) AnotarVerificacionDeEnlace(ctx context.Context, enlace EnlacePorVerificar, resultado ResultadoEnlace) error {
	return r.operacion(ctx, "anotar verificación de liga", func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE enlace_video SET estado = ?, http_status = ?, detalle = ?, verificado_at = ? WHERE id = ? AND url = ?",
			string(resultado.Estado), nuloSiCero(int64(resultado.HTTPStatus)), resultado.Detalle, r.marca(), enlace.ID, enlace.URL)
		return err
	})
}

// --- Recordatorios de borrador (RF-NOT-04) ------------------------------------

// recordatorioEnviado indica si a una postulación ya se le encoló un
// recordatorio dentro de la ventana que empieza en ventana.
func recordatorioEnviado(ctx context.Context, q consultor, postulacionID int64, ventana time.Time) (bool, error) {
	return existe(ctx, q, `
		SELECT COUNT(*) FROM email_outbox
		WHERE plantilla_clave = ? AND postulacion_id = ? AND creado_at >= ?`,
		PlantillaRecordatorio, postulacionID, aMarca(ventana))
}

// BorradoresPorRecordar devuelve los borradores a los que toca enviar, en
// este momento, un recordatorio de que el plazo está por cerrar: los de
// convocatorias en etapa de postulación cuya ventana de recordatorio está
// abierta y que aún no recibieron el de esa ventana.
func (r *Repositorio) BorradoresPorRecordar(ctx context.Context) ([]RecordatorioDeBorrador, error) {
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT `+columnasUsuario+`, p.id, p.convocatoria_id
		FROM postulacion p
		JOIN usuario u ON u.id = p.usuario_id
		JOIN convocatoria c ON c.id = p.convocatoria_id
		WHERE p.estado = 'borrador' AND c.estado = 'postulacion' AND u.activo = 1
		ORDER BY p.id`)
	if err != nil {
		return nil, fmt.Errorf("leer borradores: %w", err)
	}
	var candidatos []RecordatorioDeBorrador
	for filas.Next() {
		var rec RecordatorioDeBorrador
		var convocatoriaID int64
		if rec.Usuario, err = leerUsuario(filas, &rec.PostulacionID, &convocatoriaID); err != nil {
			filas.Close()
			return nil, fmt.Errorf("leer borradores: %w", err)
		}
		rec.Convocatoria.ID = convocatoriaID
		candidatos = append(candidatos, rec)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("leer borradores: %w", err)
	}

	ahora := r.ahora()
	convocatorias := make(map[int64]Convocatoria)
	var pendientes []RecordatorioDeBorrador
	for _, rec := range candidatos {
		c, leida := convocatorias[rec.Convocatoria.ID]
		if !leida {
			if c, err = convocatoriaPorID(ctx, r.lectura, rec.Convocatoria.ID); err != nil {
				return nil, fmt.Errorf("leer convocatoria: %w", err)
			}
			convocatorias[c.ID] = c
		}
		etapa, tiene := c.Etapa(EtapaPostulacion)
		if !tiene {
			continue
		}
		ventana, toca := VentanaDeRecordatorio(etapa.TerminaAt, ahora)
		if !toca {
			continue
		}
		enviado, err := recordatorioEnviado(ctx, r.lectura, rec.PostulacionID, ventana)
		if err != nil {
			return nil, fmt.Errorf("leer recordatorios: %w", err)
		}
		if !enviado {
			rec.Convocatoria, rec.TerminaAt, rec.Ventana = c, etapa.TerminaAt, ventana
			pendientes = append(pendientes, rec)
		}
	}
	return pendientes, nil
}

// EncolarRecordatorio encola el correo de un recordatorio, salvo que mientras
// se componía la postulación se haya enviado o el recordatorio ya exista.
// Devuelve si lo encoló.
func (r *Repositorio) EncolarRecordatorio(ctx context.Context, rec RecordatorioDeBorrador, correo CorreoSaliente) (bool, error) {
	encolado := false
	err := r.operacion(ctx, "encolar recordatorio", func(tx *sql.Tx) error {
		esBorrador, err := existe(ctx, tx, "SELECT COUNT(*) FROM postulacion WHERE id = ? AND estado = 'borrador'", rec.PostulacionID)
		if err != nil || !esBorrador {
			return err
		}
		enviado, err := recordatorioEnviado(ctx, tx, rec.PostulacionID, rec.Ventana)
		if err != nil || enviado {
			return err
		}
		correo.PlantillaClave, correo.PostulacionID = PlantillaRecordatorio, rec.PostulacionID
		encolado = true
		return r.encolarCorreo(ctx, tx, correo)
	})
	return encolado && err == nil, err
}

// --- Archivos públicos de una convocatoria (RF-CONV-02) -----------------------

// columnaDeArchivo es la columna de «convocatoria» que guarda cada clase de
// archivo. El nombre de la columna sale de esta tabla, nunca de la petición.
var columnaDeArchivo = map[ClaseArchivoConvocatoria]string{
	ArchivoBasesES: "bases_pdf_es_id",
	ArchivoBasesEN: "bases_pdf_en_id",
	ArchivoImagen:  "imagen_id",
}

// ArchivosDeConvocatoria devuelve los archivos públicos que una convocatoria
// tiene cargados: las bases en PDF de cada idioma y la imagen de cabecera.
func (r *Repositorio) ArchivosDeConvocatoria(ctx context.Context, convocatoriaID int64) (map[ClaseArchivoConvocatoria]Archivo, error) {
	archivos := make(map[ClaseArchivoConvocatoria]Archivo)
	for clase, columna := range columnaDeArchivo {
		var a Archivo
		err := r.lectura.QueryRowContext(ctx, `
			SELECT a.id, a.sha256, a.clave, a.mime, a.tamano_bytes
			FROM convocatoria c JOIN archivo a ON a.id = c.`+columna+` WHERE c.id = ?`, convocatoriaID).
			Scan(&a.ID, &a.SHA256, &a.Clave, &a.MIME, &a.TamanoBytes)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("leer archivos de la convocatoria: %w", err)
		}
		archivos[clase] = a
	}
	return archivos, nil
}

// FijarArchivoDeConvocatoria carga o sustituye un archivo público de la
// convocatoria. Con un Blob vacío lo retira. El archivo anterior no se borra
// del almacén: sólo deja de estar ligado.
func (r *Repositorio) FijarArchivoDeConvocatoria(ctx context.Context, actor Actor, convocatoriaID int64,
	clase ClaseArchivoConvocatoria, blob Blob, mime, nombreOriginal string) error {
	return r.operacion(ctx, "fijar archivo de la convocatoria", func(tx *sql.Tx) error {
		c, err := convocatoriaPorID(ctx, tx, convocatoriaID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarEdicionDeConfiguracion(c.Estado)); err != nil {
			return err
		}
		columna, conocida := columnaDeArchivo[clase]
		if !conocida {
			return validacion([]ErrorCampo{{Campo: "clase", Clave: "error.clase_de_archivo"}})
		}
		var archivoID int64
		if blob.SHA256 != "" {
			if err := validacion(ValidarArchivoDeConvocatoria(clase, nombreOriginal, mime, blob.Tamano)); err != nil {
				return err
			}
			if archivoID, err = r.guardarArchivo(ctx, tx, blob, mime, nombreOriginal); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE convocatoria SET "+columna+" = ? WHERE id = ?", nuloSiCero(archivoID), c.ID); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "convocatoria.archivo", "convocatoria", c.ID, nil,
			map[string]any{"clase": clase, "sha256": blob.SHA256})
	})
}

// =============================================================================
// 12. Revisión documental y subsanación
// =============================================================================

// subsanacionesDe devuelve las subsanaciones de una postulación, de la más
// antigua a la más reciente, con sus problemas y sus solicitudes de desbloqueo.
func subsanacionesDe(ctx context.Context, q consultor, postulacionID int64) ([]Subsanacion, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT id, postulacion_id, revisor_id, mensaje, fecha_limite, enviada_at, COALESCE(atendida_at, ''), COALESCE(cerrada_at, '')
		FROM subsanacion WHERE postulacion_id = ? ORDER BY id`, postulacionID)
	if err != nil {
		return nil, fmt.Errorf("leer subsanaciones: %w", err)
	}
	var subsanaciones []Subsanacion
	for filas.Next() {
		var s Subsanacion
		var limite, enviada, atendida, cerrada string
		if err := filas.Scan(&s.ID, &s.PostulacionID, &s.RevisorID, &s.Mensaje, &limite, &enviada, &atendida, &cerrada); err != nil {
			filas.Close()
			return nil, fmt.Errorf("leer subsanaciones: %w", err)
		}
		for _, marca := range []struct {
			texto   string
			destino *time.Time
		}{{limite, &s.FechaLimite}, {enviada, &s.EnviadaAt}, {atendida, &s.AtendidaAt}, {cerrada, &s.CerradaAt}} {
			if marca.texto == "" {
				continue
			}
			if *marca.destino, err = deMarca(marca.texto); err != nil {
				filas.Close()
				return nil, err
			}
		}
		subsanaciones = append(subsanaciones, s)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, fmt.Errorf("leer subsanaciones: %w", err)
	}
	for i := range subsanaciones {
		s := &subsanaciones[i]
		if s.Problemas, err = problemasDe(ctx, q, s.ID); err != nil {
			return nil, err
		}
		if s.Solicitudes, err = solicitudesDe(ctx, q, s.ID); err != nil {
			return nil, err
		}
	}
	return subsanaciones, nil
}

// problemasDe devuelve los elementos señalados en una subsanación. De un
// documento, el campo es la clave de su requisito.
func problemasDe(ctx context.Context, q consultor, subsanacionID int64) ([]ProblemaSubsanacion, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT sp.id, sp.clase, COALESCE(sp.campo, rq.clave, ''), COALESCE(sp.obra_id, 0), COALESCE(sp.documento_id, 0),
		       sp.problema, COALESCE(sp.resuelto_at, '')
		FROM subsanacion_problema sp
		LEFT JOIN requisito_documental rq ON rq.id = sp.requisito_id
		WHERE sp.subsanacion_id = ? ORDER BY sp.id`, subsanacionID)
	if err != nil {
		return nil, fmt.Errorf("leer problemas de la subsanación: %w", err)
	}
	defer filas.Close()
	var problemas []ProblemaSubsanacion
	for filas.Next() {
		var p ProblemaSubsanacion
		var clase, resuelto string
		if err := filas.Scan(&p.ID, &clase, &p.Campo, &p.ObraID, &p.DocumentoID, &p.Problema, &resuelto); err != nil {
			return nil, fmt.Errorf("leer problemas de la subsanación: %w", err)
		}
		p.Clase = ClaseProblema(clase)
		if resuelto != "" {
			if p.ResueltoAt, err = deMarca(resuelto); err != nil {
				return nil, err
			}
		}
		problemas = append(problemas, p)
	}
	return problemas, filas.Err()
}

// solicitudesDe devuelve las solicitudes de desbloqueo de una subsanación.
func solicitudesDe(ctx context.Context, q consultor, subsanacionID int64) ([]SolicitudDesbloqueo, error) {
	filas, err := q.QueryContext(ctx, `
		SELECT sd.id, COALESCE(sd.campo, rq.clave, ''), sd.requisito_id IS NOT NULL, COALESCE(sd.obra_id, 0), sd.justificacion,
		       sd.concedida IS NOT NULL, COALESCE(sd.concedida, 0), COALESCE(sd.motivo_respuesta, ''), sd.creado_at, COALESCE(sd.resuelta_at, '')
		FROM solicitud_desbloqueo sd
		LEFT JOIN requisito_documental rq ON rq.id = sd.requisito_id
		WHERE sd.subsanacion_id = ? ORDER BY sd.id`, subsanacionID)
	if err != nil {
		return nil, fmt.Errorf("leer solicitudes de desbloqueo: %w", err)
	}
	defer filas.Close()
	var solicitudes []SolicitudDesbloqueo
	for filas.Next() {
		s := SolicitudDesbloqueo{Clase: ProblemaCampo}
		var esDocumento bool
		var creado, resuelta string
		if err := filas.Scan(&s.ID, &s.Campo, &esDocumento, &s.ObraID, &s.Justificacion,
			&s.Resuelta, &s.Concedida, &s.MotivoRespuesta, &creado, &resuelta); err != nil {
			return nil, fmt.Errorf("leer solicitudes de desbloqueo: %w", err)
		}
		if esDocumento {
			s.Clase = ProblemaDocumento
		}
		if s.CreadoAt, err = deMarca(creado); err != nil {
			return nil, err
		}
		if resuelta != "" {
			if s.ResueltaAt, err = deMarca(resuelta); err != nil {
				return nil, err
			}
		}
		solicitudes = append(solicitudes, s)
	}
	return solicitudes, filas.Err()
}

// tomaDe devuelve quién tiene tomada una postulación, o nil si está libre.
func tomaDe(ctx context.Context, q consultor, postulacionID int64) (*TomaRevision, error) {
	var t TomaRevision
	var nombre, apellidos, tomada string
	err := q.QueryRowContext(ctx, `
		SELECT t.revisor_id, u.nombre, u.apellidos, t.tomada_at
		FROM toma_revision t JOIN usuario u ON u.id = t.revisor_id
		WHERE t.postulacion_id = ? AND t.liberada_at IS NULL`, postulacionID).Scan(&t.RevisorID, &nombre, &apellidos, &tomada)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leer toma de revisión: %w", err)
	}
	t.Revisor = strings.TrimSpace(nombre + " " + apellidos)
	if t.TomadaAt, err = deMarca(tomada); err != nil {
		return nil, err
	}
	return &t, nil
}

// revisionDe reúne lo que la revisión ha dejado sobre una postulación: el
// dictamen vigente de cada documento, las observaciones y la toma.
func revisionDe(ctx context.Context, q consultor, postulacionID int64) (Revision, error) {
	var revision Revision
	// El dictamen vigente de un documento es el último que se emitió.
	filas, err := q.QueryContext(ctx, `
		SELECT rd.documento_id, rd.dictamen, COALESCE(rd.motivo, ''), rd.revisor_id, rd.creado_at
		FROM revision_documento rd
		JOIN documento d ON d.id = rd.documento_id
		WHERE d.postulacion_id = ?
		  AND rd.id = (SELECT MAX(otro.id) FROM revision_documento otro WHERE otro.documento_id = rd.documento_id)
		ORDER BY rd.documento_id`, postulacionID)
	if err != nil {
		return revision, fmt.Errorf("leer dictámenes: %w", err)
	}
	for filas.Next() {
		var d DictamenDocumento
		var creado string
		if err := filas.Scan(&d.DocumentoID, &d.Dictamen, &d.Motivo, &d.RevisorID, &creado); err != nil {
			filas.Close()
			return revision, fmt.Errorf("leer dictámenes: %w", err)
		}
		if d.CreadoAt, err = deMarca(creado); err != nil {
			filas.Close()
			return revision, err
		}
		revision.Dictamenes = append(revision.Dictamenes, d)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return revision, fmt.Errorf("leer dictámenes: %w", err)
	}

	filas, err = q.QueryContext(ctx, `
		SELECT o.id, COALESCE(o.obra_id, 0), o.revisor_id, u.nombre, u.apellidos, o.texto, o.creado_at
		FROM observacion_revision o JOIN usuario u ON u.id = o.revisor_id
		WHERE o.postulacion_id = ? ORDER BY o.id`, postulacionID)
	if err != nil {
		return revision, fmt.Errorf("leer observaciones: %w", err)
	}
	for filas.Next() {
		var o ObservacionRevision
		var nombre, apellidos, creado string
		if err := filas.Scan(&o.ID, &o.ObraID, &o.RevisorID, &nombre, &apellidos, &o.Texto, &creado); err != nil {
			filas.Close()
			return revision, fmt.Errorf("leer observaciones: %w", err)
		}
		o.Revisor = strings.TrimSpace(nombre + " " + apellidos)
		if o.CreadoAt, err = deMarca(creado); err != nil {
			filas.Close()
			return revision, err
		}
		revision.Observaciones = append(revision.Observaciones, o)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return revision, fmt.Errorf("leer observaciones: %w", err)
	}
	revision.Toma, err = tomaDe(ctx, q, postulacionID)
	return revision, err
}

// RevisionDe devuelve lo que la revisión documental ha dejado sobre una
// postulación. NO comprueba quién lo pide: eso lo decide el controlador con
// la regla PuedeVerRevision.
func (r *Repositorio) RevisionDe(ctx context.Context, postulacionID int64) (Revision, error) {
	return revisionDe(ctx, r.lectura, postulacionID)
}

// BandejaDeRevision devuelve las postulaciones enviadas de una convocatoria,
// en el orden en que llegaron: TODAS, sin reparto entre revisores (RN-14b).
// Los borradores no aparecen: no entraron al proceso. revisorID es quien
// consulta, para el filtro «tomadas por mí».
func (r *Repositorio) BandejaDeRevision(ctx context.Context, convocatoriaID, revisorID int64, filtro FiltroBandeja) ([]PostulacionEnBandeja, error) {
	filas, err := r.lectura.QueryContext(ctx, `
		SELECT p.id, p.folio, p.estado, COALESCE(p.subcategoria, ''),
		       COALESCE(ds.nombre_completo, de.nombre_ensamble, dc.nombre_completo, ''),
		       COALESCE(p.enviada_at, ''), p.envios,
		       COALESCE(t.revisor_id, 0), COALESCE(u.nombre, ''), COALESCE(u.apellidos, ''), COALESCE(t.tomada_at, ''),
		       (SELECT COUNT(*) FROM observacion_revision o WHERE o.postulacion_id = p.id),
		       (SELECT COUNT(*) FROM documento d WHERE d.postulacion_id = p.id AND d.vigente = 1 AND d.dictamen = 'invalido'),
		       COALESCE((SELECT MAX(s.fecha_limite) FROM subsanacion s
		                 WHERE s.postulacion_id = p.id AND s.atendida_at IS NULL AND s.cerrada_at IS NULL), ''),
		       (SELECT COUNT(*) FROM subsanacion s
		        WHERE s.postulacion_id = p.id AND s.atendida_at IS NOT NULL AND s.cerrada_at IS NULL)
		FROM postulacion p
		LEFT JOIN datos_solista ds ON ds.postulacion_id = p.id
		LEFT JOIN datos_ensamble de ON de.postulacion_id = p.id
		LEFT JOIN datos_compositor dc ON dc.postulacion_id = p.id
		LEFT JOIN toma_revision t ON t.postulacion_id = p.id AND t.liberada_at IS NULL
		LEFT JOIN usuario u ON u.id = t.revisor_id
		WHERE p.convocatoria_id = ? AND p.estado <> 'borrador'
		ORDER BY p.enviada_at, p.id`, convocatoriaID)
	if err != nil {
		return nil, fmt.Errorf("leer bandeja de revisión: %w", err)
	}
	defer filas.Close()
	buscado := strings.ToLower(strings.TrimSpace(filtro.Texto))
	bandeja := []PostulacionEnBandeja{}
	for filas.Next() {
		var b PostulacionEnBandeja
		var estado, subcategoria, enviada, nombre, apellidos, tomada, limite string
		var revisor int64
		var atendidas int
		if err := filas.Scan(&b.ID, &b.Folio, &estado, &subcategoria, &b.Nombre, &enviada, &b.Envios,
			&revisor, &nombre, &apellidos, &tomada, &b.Observaciones, &b.DocumentosInvalidos, &limite, &atendidas); err != nil {
			return nil, fmt.Errorf("leer bandeja de revisión: %w", err)
		}
		b.Estado, b.Subcategoria = EstadoDePostulacion(estado), Subcategoria(subcategoria)
		b.Subsanada = atendidas > 0 && b.Estado == PostulacionEnRevision
		if enviada != "" {
			if b.EnviadaAt, err = deMarca(enviada); err != nil {
				return nil, err
			}
		}
		if limite != "" {
			if b.FechaLimite, err = deMarca(limite); err != nil {
				return nil, err
			}
		}
		if revisor != 0 {
			b.Toma = &TomaRevision{RevisorID: revisor, Revisor: strings.TrimSpace(nombre + " " + apellidos)}
			if b.Toma.TomadaAt, err = deMarca(tomada); err != nil {
				return nil, err
			}
		}
		// Los filtros se aplican aquí: la bandeja de una convocatoria son
		// unas cuantas centenas de renglones.
		switch {
		case filtro.Estado != "" && b.Estado != filtro.Estado,
			filtro.Subcategoria != "" && b.Subcategoria != filtro.Subcategoria,
			filtro.Toma == "libre" && b.Toma != nil,
			filtro.Toma == "mia" && (b.Toma == nil || b.Toma.RevisorID != revisorID),
			filtro.Toma == "otra" && (b.Toma == nil || b.Toma.RevisorID == revisorID),
			buscado != "" && !strings.Contains(strings.ToLower(b.Folio), buscado) && !strings.Contains(strings.ToLower(b.Nombre), buscado):
			continue
		}
		bandeja = append(bandeja, b)
	}
	return bandeja, filas.Err()
}

// postulacionEnRevision lee, dentro de una transacción, una postulación
// completa con la configuración de su convocatoria.
func (r *Repositorio) postulacionEnRevision(ctx context.Context, tx *sql.Tx, postulacionID int64) (Postulacion, ConfiguracionConvocatoria, error) {
	p, err := leerPostulacion(tx.QueryRowContext(ctx, sqlPostulacion+" WHERE p.id = ?", postulacionID))
	if err != nil {
		return Postulacion{}, ConfiguracionConvocatoria{}, err
	}
	cfg, err := configuracion(ctx, tx, p.ConvocatoriaID)
	if err != nil {
		return Postulacion{}, ConfiguracionConvocatoria{}, err
	}
	return p, cfg, completarPostulacion(ctx, tx, &p, cfg.Convocatoria.Tipo)
}

// iniciarRevision pasa la postulación de «enviada» a «en revisión» la primera
// vez que un revisor actúa sobre ella (§4).
func iniciarRevision(ctx context.Context, tx *sql.Tx, p Postulacion) error {
	if p.Estado != PostulacionEnviada {
		return nil
	}
	_, err := tx.ExecContext(ctx, "UPDATE postulacion SET estado = ? WHERE id = ?", string(PostulacionEnRevision), p.ID)
	return err
}

// TomarPostulacion marca una postulación como «en revisión por» el revisor
// que actúa, o la libera (RF-REV-01b). Es una toma blanda: sólo informa a los
// demás, no les impide intervenir, y cualquiera puede tomarla de nuevo.
func (r *Repositorio) TomarPostulacion(ctx context.Context, actor Actor, postulacionID int64, tomar bool) error {
	return r.operacion(ctx, "tomar postulación", func(tx *sql.Tx) error {
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarAccionDeRevision(cfg.Convocatoria, p)); err != nil {
			return err
		}
		// Una fila por postulación: la toma nueva sustituye a la anterior.
		if _, err := tx.ExecContext(ctx, "DELETE FROM toma_revision WHERE postulacion_id = ?", p.ID); err != nil {
			return err
		}
		accion := "revision.liberar"
		if tomar {
			accion = "revision.tomar"
			if _, err := tx.ExecContext(ctx, "INSERT INTO toma_revision (postulacion_id, revisor_id, tomada_at) VALUES (?, ?, ?)",
				p.ID, actor.UsuarioID, r.marca()); err != nil {
				return err
			}
			if err := iniciarRevision(ctx, tx, p); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, accion, "postulacion", p.ID, nil, nil)
	})
}

// DictaminarDocumento registra el dictamen de un revisor sobre un documento
// vigente: válido, inválido (con motivo, RN-11) o de vuelta a pendiente. Cada
// dictamen se conserva; el vigente es el último. Devuelve el id de la
// postulación.
func (r *Repositorio) DictaminarDocumento(ctx context.Context, actor Actor, documentoID int64, dictamen, motivo string) (int64, error) {
	var postulacionID int64
	err := r.operacion(ctx, "dictaminar documento", func(tx *sql.Tx) error {
		documento, err := leerDocumento(tx.QueryRowContext(ctx, sqlDocumento+" WHERE d.id = ? AND d.vigente = 1", documentoID))
		if err != nil {
			return err
		}
		postulacionID = documento.PostulacionID
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarAccionDeRevision(cfg.Convocatoria, p)); err != nil {
			return err
		}
		motivo = strings.TrimSpace(motivo)
		if err := validacion(ValidarDictamenDeDocumento(dictamen, motivo)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO revision_documento (documento_id, revisor_id, dictamen, motivo, creado_at) VALUES (?, ?, ?, ?, ?)",
			documentoID, actor.UsuarioID, dictamen, nuloSiVacio(motivo), r.marca()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE documento SET dictamen = ? WHERE id = ?", dictamen, documentoID); err != nil {
			return err
		}
		if err := iniciarRevision(ctx, tx, p); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "documento.dictaminar", "documento", documentoID,
			map[string]any{"dictamen": documento.Dictamen}, map[string]any{"dictamen": dictamen, "postulacion_id": p.ID})
	})
	return postulacionID, err
}

// ObservarPostulacion añade una observación del revisor a la postulación o a
// una de sus obras (RF-REV-04). Las observaciones son internas.
func (r *Repositorio) ObservarPostulacion(ctx context.Context, actor Actor, postulacionID, obraID int64, texto string) error {
	return r.operacion(ctx, "observar postulación", func(tx *sql.Tx) error {
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err != nil {
			return err
		}
		if err := validacion(ValidarAccionDeRevision(cfg.Convocatoria, p)); err != nil {
			return err
		}
		texto = strings.TrimSpace(texto)
		if err := validacion(ValidarObservacion(p, obraID, texto)); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO observacion_revision (postulacion_id, obra_id, revisor_id, texto, creado_at) VALUES (?, ?, ?, ?, ?)",
			p.ID, nuloSiCero(obraID), actor.UsuarioID, texto, r.marca())
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := iniciarRevision(ctx, tx, p); err != nil {
			return err
		}
		// La bitácora no guarda el texto: puede mencionar al postulante.
		return r.auditar(ctx, tx, actor, "revision.observar", "postulacion", p.ID, nil,
			map[string]any{"observacion_id": id, "obra_id": obraID})
	})
}

// insertarProblema guarda un elemento señalado en una subsanación. De un
// documento se guarda su requisito; de un campo, su ruta.
func (r *Repositorio) insertarProblema(ctx context.Context, tx *sql.Tx, subsanacionID int64, p Postulacion,
	cfg ConfiguracionConvocatoria, problema ProblemaSubsanacion) error {
	var campo sql.NullString
	var requisitoID sql.NullInt64
	if problema.Clase == ProblemaDocumento {
		requisito, errores := RequisitoParaDocumento(p, cfg, problema.Campo, problema.ObraID)
		if err := validacion(errores); err != nil {
			return err
		}
		requisitoID = nuloSiCero(requisito.ID)
	} else {
		campo = nuloSiVacio(problema.Campo)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO subsanacion_problema (subsanacion_id, clase, documento_id, requisito_id, campo, obra_id, problema)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		subsanacionID, string(problema.Clase), nuloSiCero(problema.DocumentoID), requisitoID, campo,
		nuloSiCero(problema.ObraID), strings.TrimSpace(problema.Problema))
	return err
}

// SolicitarSubsanacion abre una subsanación (RF-REV-05b): agrupa los
// problemas señalados —los campos y documentos faltantes que indica el
// revisor más los documentos que dictaminó inválidos—, pasa la postulación a
// «en subsanación» y encola el correo al postulante. Desde ese momento el
// postulante puede modificar EXACTAMENTE esos elementos, y ninguno más.
//
// componer arma el correo con la lista definitiva de problemas; es trabajo de
// la Vista, que el Modelo no conoce.
func (r *Repositorio) SolicitarSubsanacion(ctx context.Context, actor Actor, postulacionID int64, n NuevaSubsanacion,
	componer func(Postulacion, ConfiguracionConvocatoria, Usuario, NuevaSubsanacion) (CorreoSaliente, error)) error {
	return r.operacion(ctx, "solicitar subsanación", func(tx *sql.Tx) error {
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err != nil {
			return err
		}
		revision, err := revisionDe(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		n.Problemas = ProblemasDeSubsanacion(p, revision, n.Problemas)
		n.FechaLimite = n.FechaLimite.UTC().Truncate(time.Second)
		n.Nota = strings.TrimSpace(n.Nota)
		if err := validacion(ValidarNuevaSubsanacion(p, cfg, n, r.ahora())); err != nil {
			return err
		}
		titular, err := leerUsuario(tx.QueryRowContext(ctx, "SELECT "+columnasUsuario+" FROM usuario u WHERE u.id = ?", p.UsuarioID))
		if err != nil {
			return err
		}
		correo, err := componer(p, cfg, titular, n)
		if err != nil {
			return fmt.Errorf("componer correo de subsanación: %w", err)
		}
		marca := r.marca()
		// Las subsanaciones anteriores ya atendidas quedan cerradas: la nueva
		// es la que cuenta.
		if _, err := tx.ExecContext(ctx,
			"UPDATE subsanacion SET cerrada_at = ? WHERE postulacion_id = ? AND cerrada_at IS NULL", marca, p.ID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			"INSERT INTO subsanacion (postulacion_id, revisor_id, mensaje, fecha_limite, enviada_at) VALUES (?, ?, ?, ?, ?)",
			p.ID, actor.UsuarioID, correo.Texto, aMarca(n.FechaLimite), marca)
		if err != nil {
			return err
		}
		subsanacionID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, problema := range n.Problemas {
			if err := r.insertarProblema(ctx, tx, subsanacionID, p, cfg, problema); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE postulacion SET estado = ?, actualizado_at = ? WHERE id = ?",
			string(PostulacionEnSubsanacion), marca, p.ID); err != nil {
			return err
		}
		correo.PostulacionID = p.ID
		if err := r.encolarCorreo(ctx, tx, correo); err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "postulacion.subsanacion", "postulacion", p.ID,
			map[string]any{"estado": p.Estado},
			map[string]any{"estado": PostulacionEnSubsanacion, "subsanacion_id": subsanacionID,
				"problemas": len(n.Problemas), "fecha_limite": aMarca(n.FechaLimite)})
	})
}

// SolicitarDesbloqueo registra la petición de un postulante para corregir un
// elemento que el revisor no señaló (RN-12d). No desbloquea nada por sí sola.
func (r *Repositorio) SolicitarDesbloqueo(ctx context.Context, actor Actor, usuarioID, postulacionID int64, s SolicitudDesbloqueo) error {
	return r.operacion(ctx, "solicitar desbloqueo", func(tx *sql.Tx) error {
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err == nil && p.UsuarioID != usuarioID {
			err = ErrNoEncontrado // lo ajeno no existe
		}
		if err != nil {
			return err
		}
		s.Justificacion = strings.TrimSpace(s.Justificacion)
		if err := validacion(ValidarSolicitudDeDesbloqueo(p, cfg, s, r.ahora())); err != nil {
			return err
		}
		var campo sql.NullString
		var requisitoID sql.NullInt64
		if s.Clase == ProblemaDocumento {
			requisito, errores := RequisitoParaDocumento(p, cfg, s.Campo, s.ObraID)
			if err := validacion(errores); err != nil {
				return err
			}
			requisitoID = nuloSiCero(requisito.ID)
		} else {
			campo = nuloSiVacio(s.Campo)
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO solicitud_desbloqueo (subsanacion_id, campo, requisito_id, obra_id, justificacion, creado_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			p.SubsanacionAbierta(r.ahora()).ID, campo, requisitoID, nuloSiCero(s.ObraID), s.Justificacion, r.marca())
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		return r.auditar(ctx, tx, actor, "desbloqueo.solicitar", "postulacion", p.ID, nil,
			map[string]any{"solicitud_id": id, "clase": s.Clase, "campo": s.Campo, "obra_id": s.ObraID})
	})
}

// ResolverDesbloqueo concede o niega una solicitud de desbloqueo (RF-REV-05c).
// Si se concede, el elemento se añade a los problemas de la subsanación: ésa
// es la única vía por la que algo se vuelve editable (RN-12b).
func (r *Repositorio) ResolverDesbloqueo(ctx context.Context, actor Actor, postulacionID, solicitudID int64, concedida bool, motivo string) error {
	return r.operacion(ctx, "resolver desbloqueo", func(tx *sql.Tx) error {
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err != nil {
			return err
		}
		subsanacion := p.SubsanacionAbierta(r.ahora())
		if subsanacion == nil {
			return validacion([]ErrorCampo{{Campo: "estado", Clave: "error.sin_subsanacion_abierta"}})
		}
		posicion := slices.IndexFunc(subsanacion.Solicitudes, func(s SolicitudDesbloqueo) bool { return s.ID == solicitudID })
		if posicion < 0 {
			return ErrNoEncontrado
		}
		solicitud := subsanacion.Solicitudes[posicion]
		if solicitud.Resuelta {
			return validacion([]ErrorCampo{{Campo: "solicitud", Clave: "error.solicitud_resuelta"}})
		}
		motivo = strings.TrimSpace(motivo)
		if err := validacion(ValidarRespuestaDeDesbloqueo(concedida, motivo)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE solicitud_desbloqueo SET resuelta_por = ?, concedida = ?, motivo_respuesta = ?, resuelta_at = ? WHERE id = ?",
			actor.UsuarioID, concedida, nuloSiVacio(motivo), r.marca(), solicitud.ID); err != nil {
			return err
		}
		if concedida {
			// El motivo del problema es la justificación del propio postulante.
			problema := ProblemaSubsanacion{Clase: solicitud.Clase, Campo: solicitud.Campo, ObraID: solicitud.ObraID, Problema: solicitud.Justificacion}
			if err := r.insertarProblema(ctx, tx, subsanacion.ID, p, cfg, problema); err != nil {
				return err
			}
		}
		return r.auditar(ctx, tx, actor, "desbloqueo.resolver", "postulacion", p.ID, nil,
			map[string]any{"solicitud_id": solicitud.ID, "concedida": concedida})
	})
}

// DictaminarPostulacion termina la revisión documental de una postulación:
// validada o descartada, con motivo obligatorio al descartar (RF-REV-08).
// Cierra las subsanaciones que hubiera. No se comunica nada al postulante.
func (r *Repositorio) DictaminarPostulacion(ctx context.Context, actor Actor, postulacionID int64, dictamen EstadoDePostulacion, motivo string) error {
	return r.operacion(ctx, "dictaminar postulación", func(tx *sql.Tx) error {
		p, cfg, err := r.postulacionEnRevision(ctx, tx, postulacionID)
		if err != nil {
			return err
		}
		motivo = strings.TrimSpace(motivo)
		if err := validacion(ValidarDictamenDePostulacion(cfg.Convocatoria, p, dictamen, motivo, r.ahora())); err != nil {
			return err
		}
		marca := r.marca()
		var validadaAt, motivoDescarte sql.NullString
		if dictamen == PostulacionValidada {
			validadaAt = nuloSiVacio(marca)
		} else {
			motivoDescarte = nuloSiVacio(motivo)
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE postulacion SET estado = ?, validada_at = ?, motivo_descarte = ?, actualizado_at = ? WHERE id = ?",
			string(dictamen), validadaAt, motivoDescarte, marca, p.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE subsanacion SET cerrada_at = ? WHERE postulacion_id = ? AND cerrada_at IS NULL", marca, p.ID); err != nil {
			return err
		}
		// El motivo del descarte sí queda en la bitácora: es un acto del
		// proceso, no un dato personal.
		return r.auditar(ctx, tx, actor, "postulacion.dictaminar", "postulacion", p.ID,
			map[string]any{"estado": p.Estado}, map[string]any{"estado": dictamen, "motivo": motivo})
	})
}

// DiferenciasDelUltimoEnvio compara los dos últimos envíos de una postulación
// (RF-REV-07). Con un solo envío no hay nada que comparar.
func (r *Repositorio) DiferenciasDelUltimoEnvio(ctx context.Context, postulacionID int64) ([]CambioDeEnvio, error) {
	filas, err := r.lectura.QueryContext(ctx,
		"SELECT snapshot FROM acuse WHERE postulacion_id = ? ORDER BY version DESC LIMIT 2", postulacionID)
	if err != nil {
		return nil, fmt.Errorf("leer snapshots: %w", err)
	}
	defer filas.Close()
	var snapshots []SnapshotEnvio // el más reciente primero
	for filas.Next() {
		var texto string
		var snapshot SnapshotEnvio
		if err := filas.Scan(&texto); err != nil {
			return nil, fmt.Errorf("leer snapshots: %w", err)
		}
		if err := json.Unmarshal([]byte(texto), &snapshot); err != nil {
			return nil, fmt.Errorf("snapshot que no es JSON válido: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := filas.Err(); err != nil || len(snapshots) < 2 {
		return nil, err
	}
	return DiferenciasDeEnvio(snapshots[1], snapshots[0]), nil
}
