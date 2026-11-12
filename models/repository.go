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
//   5. Usuarios
//   6. Sesiones e inicios de sesión
//   7. Enlaces de un solo uso (tokens)
//   8. Correo saliente (bandeja de salida)
//   9. Consulta de la bitácora

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
	_, err := tx.ExecContext(ctx, `
		INSERT INTO email_outbox (destinatario, idioma, asunto, cuerpo_html, cuerpo_texto,
		                          plantilla_clave, programado_para, creado_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Destinatario, c.Idioma, c.Asunto, c.HTML, c.Texto, nuloSiVacio(c.PlantillaClave), marca, marca)
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
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO usuario_perfil (usuario_id, perfil, otorgado_por, creado_at) VALUES (?, ?, ?, ?)",
			id, string(n.Perfil), nuloSiCero(actor.UsuarioID), marca); err != nil {
			return err
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
		SELECT id, destinatario, idioma, asunto, cuerpo_html, cuerpo_texto, COALESCE(plantilla_clave, ''), intentos
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
		if err := filas.Scan(&c.ID, &c.Destinatario, &c.Idioma, &c.Asunto, &c.HTML, &c.Texto, &c.PlantillaClave, &c.Intentos); err != nil {
			return nil, fmt.Errorf("leer bandeja de salida: %w", err)
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
