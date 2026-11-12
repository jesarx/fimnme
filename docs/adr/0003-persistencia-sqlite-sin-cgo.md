# ADR 0003 — Persistencia: SQLite sin CGO, conexiones y migraciones

- **Estado:** aceptada
- **Fecha:** 2026-11-12

Decisiones del paso 2. La elección de motor y controlador ya estaba fijada por la política
institucional (D-24); aquí se registra **cómo** se usa.

## 1. Controlador y dependencias que arrastra

**Decisión.** `modernc.org/sqlite` v1.62.0, SQLite 3 traducido a Go puro. Permite
`CGO_ENABLED=0` y binarios estáticos. Trae FTS5 (con el tokenizador `trigram`), JSON1 y
R-Tree compilados.

Es la primera dependencia externa del proyecto. Arrastra estas dependencias indirectas,
todas fijadas con su suma en `go.sum`:

| Módulo | Para qué lo usa el controlador |
|---|---|
| `modernc.org/libc`, `modernc.org/memory`, `modernc.org/mathutil` | Emulación de la biblioteca de C sobre la que corre SQLite |
| `golang.org/x/sys` | Llamadas al sistema |
| `github.com/google/uuid`, `github.com/dustin/go-humanize`, `github.com/mattn/go-isatty`, `github.com/ncruces/go-strftime`, `github.com/remyoudompheng/bigfft` | Utilidades internas de `libc` |

Ninguna se importa desde el código del proyecto.

**No se incluye el directorio `vendor/`.** Copiar las dependencias al repositorio lo haría
crecer en cientos de megabytes (el controlador trae SQLite traducido para cada
plataforma). La reproducibilidad descansa en `go.sum`, que fija el contenido exacto de
cada módulo, y en el proxy de módulos de Go. Si la CNMO exige compilar sin red, se
resuelve con `go mod vendor` en ese momento, sin cambiar código.

## 2. Dos grupos de conexiones sobre el mismo archivo

**Decisión.** `models.Repositorio` abre:

- **Escritura: una sola conexión**, con `BEGIN IMMEDIATE`. SQLite escribe de una en una;
  así las escrituras hacen fila dentro del proceso en lugar de competir por el cerrojo
  del archivo, y una transacción nunca falla a medias por no poder ascender su cerrojo.
- **Lectura: ocho conexiones** con `PRAGMA query_only = 1`. No pueden escribir aunque una
  consulta lo intentara. En modo WAL leen sin esperar a la escritura en curso.

Los PRAGMAs de §7.5 (`journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`,
`synchronous=NORMAL`, `mmap_size`, `cache_size`, `temp_store`) viajan en la cadena de
conexión, de modo que el controlador los emite en **cada** conexión nueva. Una prueba lo
comprueba conexión por conexión.

`fimnme-cli` es otro proceso que escribe en la misma base; `busy_timeout` convierte esa
coincidencia en una espera breve. Una prueba lanza 600 escrituras simultáneas desde dos
«procesos» y exige cero errores. **No sustituye a la prueba de carga formal (D-30).**

## 3. Las migraciones son un acto explícito

**Decisión.** Las migraciones son archivos `models/migrations/NNNN_nombre.sql`
incrustados en el binario. Sólo las aplica `fimnme-cli migrate`. **`fimnme-web` nunca
migra**: si al arrancar encuentra migraciones pendientes, o una base más nueva que el
binario, se niega a servir y lo dice en el log.

**Motivo.** Cambiar el esquema de un sistema con efectos jurídicos debe ser una acción
deliberada del operador, visible y auditada, no un efecto lateral de reiniciar un
servicio. `make deploy` hace la secuencia completa: detener, migrar, arrancar.

Garantías:

- Cada migración va en **su propia transacción**, junto con su registro en
  `schema_migration` y su asiento en `auditoria`: entra completa o no entra.
- Se guarda el **SHA-256** de cada archivo aplicado. Si un archivo ya aplicado cambia, el
  sistema se niega a continuar: **una migración aplicada no se edita jamás**; los cambios
  van en una migración nueva.

## 4. Esquema inicial

**Decisión.** La migración `0001` es el DDL de §8.3 **copiado del documento por un
programa**, no transcrito a mano, con tres añadidos que el propio documento anuncia:

1. `toma_revision` se crea después de `postulacion`, a la que referencia, para que el
   orden valga también en MariaDB.
2. Triggers de respaldo que rechazan puntajes fuera de la escala del criterio.
3. Triggers que mantienen los dos índices FTS5 sincronizados con `indice_busqueda`.

Las tablas que §8.3 menciona sin definir (plantillas de reporte, motivos frecuentes,
versiones del aviso de privacidad) llegarán con migraciones propias en sus pasos.

## 5. Una sola regla, en un solo lugar

**Decisión.** Qué convocatorias son públicas o vigentes lo decide `models/rules.go`; el
SQL trae las filas y la regla se aplica en Go. Se evita así tener la misma regla escrita
dos veces, en SQL y en Go. Es barato porque el sistema acumula dos convocatorias por año.

## 6. Marcas de tiempo

**Decisión.** Todas se guardan como texto `AAAA-MM-DDTHH:MM:SSZ` en UTC, con precisión de
segundos. Un único par de funciones (`aMarca` / `deMarca`) hace la conversión.

## 7. Bitácora desde el primer día

**Decisión.** Todo método del repositorio que escribe recibe un `models.Actor` e inserta
su asiento de auditoría **dentro de la misma transacción**: si la operación se deshace,
el asiento también. Las operaciones de `fimnme-cli` quedan con perfil `cli` y el usuario
del sistema operativo que las ejecutó (RF-CLI-09).

## 8. `fimnme-cli` no trabaja sobre datos ajenos

**Decisión.** Si el directorio de datos pertenece a otro usuario, `fimnme-cli` se niega a
abrirlo. **Motivo:** ejecutarlo como root sobre los datos del servicio dejaría la base o
su archivo WAL con dueño root, y el servidor web dejaría de poder escribir.
