# ADR 0014 — Continuidad operativa, portal público y anonimización

- **Estado:** aceptada
- **Fecha:** 2026-11-14

Decisiones del paso 10a (§5.14, §5.16 y §5.17 de la especificación, y RF-ARCH-07). No
añade dependencias. Añade la migración `0007`: la marca de anonimización de la
postulación, la marca de purga del archivo y la plantilla de correo `alerta_disco`.

El paso 10 se partió en dos. Ésta, **10a**, cubre lo que sostiene la operación y la
protección de datos: verificación del acervo, espejo, alerta de disco, exportación al
portal público y anonimización. La **10b** cubrirá la importación asistida, la extensión
dinámica de campos y la TUI.

## 1. Verificación del acervo (`fimnme-cli verify`)

**El requisito** (RF-CLI-08, RF-OPS-04). Que cada `archivo.sha256` registrado exista en
disco y coincida, y reportar huérfanos en ambos sentidos.

**Decisión.** `models.VerificarAcervo` compara lo que la base registra con lo que hay en
el almacén y distingue cuatro cosas que no son lo mismo:

| Hallazgo | Qué significa | ¿Rompe la integridad? |
|---|---|---|
| **Faltante** | Registrado, no está en disco | Sí: es pérdida de datos |
| **Alterado** | Está, pero su tamaño o su SHA-256 no son los registrados | Sí |
| **Purgado presente** | Se dio por borrado en una anonimización y sigue en disco | Sí: la purga quedó a medias (`verify --purgar` la termina) |
| **Huérfano** | En disco, sin registro | No: suele ser una subida cuya transacción no se confirmó |

La orden termina con error si la integridad está rota, para que un `timer` de systemd lo
note. `--rapido` compara sólo existencia y tamaño, sin leer cada archivo: para la
comprobación diaria; la completa, que vuelve a sumar los ~150 GB del acervo, para la
semanal. Cada verificación queda en la bitácora con sus conteos.

## 2. Espejo: instantánea firmada y acervo por contenido

**El requisito** (RF-CLI-06, RF-OPS-01 y 06). Snapshots en caliente con `VACUUM INTO`,
firmados con HMAC-SHA256, registrados en `snapshot_espejo`.

**Decisión.** `fimnme-cli mirror snapshot --destino DIR --clave ARCHIVO`:

1. copia la base **en caliente** a `DIR/instantaneas/fimnme-<fecha>.db` (`VACUUM INTO`,
   sin detener el servicio);
2. replica el acervo en `DIR/blobs/` **por contenido**: cada archivo se llama como su
   SHA-256, así que copiar es idempotente, sólo viaja lo nuevo, y lo que se copia se suma
   al copiarlo (un origen dañado no se da por bueno);
3. escribe junto a la base un **manifiesto** con su nombre, tamaño, SHA-256, fecha y
   estado del acervo, **firmado con HMAC-SHA256**;
4. lo registra en `snapshot_espejo` y en la bitácora;
5. borra las instantáneas de más de 90 días (`--conservar-dias`), nunca la más reciente
   ni el acervo.

`fimnme-cli mirror verify` comprueba, **sin la base en producción** —está pensada para
ejecutarse en el servidor espejo—: la firma, que el archivo de la base sea el del
manifiesto, la integridad interna de esa base (`PRAGMA integrity_check`) y que cada
archivo que **esa** base registra esté en el acervo del espejo con su contenido.

**Qué garantiza la firma, y qué no.** Quien tiene la clave puede comprobar que la
instantánea que recibió es la que se produjo y que nadie la tocó por el camino. No cifra:
la instantánea es la base completa, con datos personales, y debe viajar y guardarse como
tal. Se firma el texto de los campos, no el JSON, para que la firma no dependa de cómo se
escriba el archivo.

**La clave.** Va en un archivo (`mirror nueva-clave`), nunca en un argumento ni en una
variable de entorno, donde quedaría en el historial o en la lista de procesos. La
herramienta se niega a usar un archivo de clave que otros usuarios puedan leer, y a
sobrescribir una clave existente. No debe guardarse junto a las instantáneas.

**El transporte no es de esta herramienta.** `mirror snapshot` deja la instantánea en un
directorio; llevarla al otro servidor (rsync sobre SSH, un volumen montado) y programarlo
con un `timer` es infraestructura, y llega con los respaldos en el paso 14. La separación
es deliberada: el formato y la verificación se prueban aquí, sin red.

**Específico de SQLite.** `VACUUM INTO` no existe en MariaDB; tras la migración, el espejo
usaría las herramientas de réplica de ese motor. La verificación del acervo no cambia.

## 3. Alerta de espacio en disco

**El requisito** (RF-OPS-07). `fimnme-web` mide cada diez minutos el espacio libre del
disco de datos. Por debajo del umbral (`FIMNME_DISCO_MIN_GB`, 10 GB por omisión; 0 no
vigila) lo escribe en el log como error —donde lo recoge la supervisión del servidor— y
avisa por correo a los administradores **una vez al día** mientras dure. `fimnme-cli
verify` también lo muestra.

## 4. Portal público: la lista blanca es un tipo

**El requisito** (RF-PUB-01 a 06, RN-34). Una exportación JSON sanitizada para el sitio
estático; lista blanca de lo exportable; la nómina de seleccionados sólo después del
cierre y por acción explícita del administrador.

**Decisión.**

- **La lista blanca son los tipos `models.Interop*`.** La exportación sólo puede contener
  lo que cabe en ellos, y no tienen dónde llevar un correo, un teléfono, una fecha de
  nacimiento, un documento, un dictamen ni una calificación. `TestListaBlancaDelPortal`
  enumera, uno por uno, los campos que pueden salir: añadir, quitar o renombrar uno hace
  fallar la prueba, y corregirla obliga a preguntarse si ese dato puede hacerse público.
- **Una sola consulta saca datos de postulaciones** (`seleccionadosDe`), y la misma prueba
  lee su SQL: no puede mencionar ninguna columna de la lista negra ni ninguna tabla del
  proceso interno, y debe limitarse a `dictamen = 'seleccionada'` y a no anonimizadas.
- **Bases, calendario y dotaciones salen en vivo**, de las convocatorias que ya son
  públicas (una en borrador no sale).
- **La nómina nunca se calcula al exportar.** El administrador la **publica**
  (`POST /api/v1/convocatorias/{id}/publicacion`, con confirmación, sólo con la
  convocatoria cerrada o archivada): en ese momento se congela como JSON en el almacén,
  con su SHA-256 y quién la autorizó (`exportacion_interop`, bitácora). La exportación
  entrega **la última publicada**. Cerrar la convocatoria no publica nada; cambiar un
  dictamen tampoco cambia lo publicado hasta volver a publicar. Una prueba lo ejercita
  paso por paso.
- **Retirar** una nómina es publicar una vacía: queda el registro de que se retiró.
- La misma exportación sale por `GET /api/v1/interop/convocatoria.json` (pública) y por
  `fimnme-cli export-json`.

**De la nómina sale:** el nombre con que se anuncia (el artístico; si no hay, el completo
o el del ensamble), la subcategoría, la semblanza y los **títulos** de las obras. Nada
más: ni el compositor de cada obra, que la lista blanca no menciona.

## 5. Anonimización ARCO: la única operación destructiva

**El requisito** (RF-ARCH-07, RF-CLI-07, RN-28). Borrar los datos de contacto conservando
obra, dictamen y estadísticas; confirmación doble; auditada con su fundamento; sin alterar
conteos ni agregados.

**Decisión.** `AnonimizarPostulacion`, en una sola transacción:

| Se borra | Se conserva |
|---|---|
| Nombre, nombre artístico, teléfono, correo de contacto, semblanza | Género, nacionalidad, país y entidad (son los que cuentan las estadísticas) |
| Día y mes de la fecha de nacimiento | El **año** de nacimiento, para que el rango de edad no cambie |
| Nombres de los integrantes | Sus instrumentos |
| Ligas de video y ligas de las obras | Títulos, compositores, duración, dotación, estreno y notas de las obras |
| Fotografía, autorizaciones y cualquier otro documento que no sea la obra | **Partitura, particellas y audio**: la obra |
| Los acuses (el PDF y su snapshot, que repiten todos los datos) | Folio, estado, dictamen, evaluaciones, bitácora |
| El texto de los correos y de las subsanaciones que se le enviaron | Los problemas señalados en cada subsanación |

- **Ante la duda, un documento es dato personal.** Sólo sobreviven los tres requisitos que
  son la obra; un requisito que el administrador añada después se purga.
- **Los archivos se borran de verdad**, del disco. La fila de `archivo` queda, con su
  SHA-256, marcada como purgada: constancia de que existió, y señal para que la
  verificación no lo reporte como perdido. Si otra postulación usa el mismo archivo (el
  almacén guarda cada contenido una sola vez), no se borra.
- **Primero la base, después el disco.** La transacción marca; el borrado físico va
  después. Si se interrumpe, `verify` lo señala y `verify --purgar` lo termina.
- **El espejo también.** La siguiente instantánea retira del acervo del espejo los
  archivos purgados. **Las instantáneas y los respaldos anteriores siguen conteniendo los
  datos hasta que vence su conservación** (90 días); la herramienta lo dice al terminar.
  Si una solicitud exige menos, se borran a mano.
- **La cuenta** se anonimiza sólo si se pide (`--con-cuenta`) y sólo si no tiene otras
  postulaciones sin anonimizar: es la cuenta con la que esa persona puede seguir
  postulando. Sin ella, la postulación anonimizada **no muestra quién la envió** en
  expedientes, reportes ni búsquedas, aunque la cuenta exista.
- **Cuándo.** Sólo con el proceso terminado: la convocatoria cerrada o archivada, o la
  postulación descartada. Anonimizar a media evaluación dejaría al jurado calificando a
  nadie.
- **Confirmación doble.** En la herramienta: volver a escribir el folio y después la
  palabra `ANONIMIZAR`. En la API: `confirmo` y el folio escrito de nuevo. El modelo exige
  ambas; un folio que no coincide no anonimiza nada.
- **Sólo el administrador** (§2.2). Queda en la bitácora con el fundamento, que debe ser
  la referencia de la solicitud, no un dato de la persona.

**Límite conocido.** Las observaciones de revisores y jurados son texto libre: el sistema
no puede saber si nombran a la persona y no las modifica. La herramienta lo recuerda.

## Consecuencias

- El paso 14 sólo tiene que programar: un `timer` para `mirror snapshot` + `rsync`, otro
  para `verify --rapido` diario y `verify` semanal, y la prueba de restauración a partir
  de una instantánea verificada.
- La publicación de la nómina no tiene orden de línea: `exportacion_interop` exige la
  cuenta que la autoriza. Se hace desde la API (y, en el paso 13, desde el panel).
