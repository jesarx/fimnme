# ADR 0015 — Extensión dinámica de campos e importación asistida

- **Estado:** aceptada
- **Fecha:** 2026-11-14

Decisiones del paso 10b (§5.14.1, §5.15 y §7.6 de la especificación). No añade
dependencias ni migraciones: las tablas (`campo_extendido`, `importacion`,
`importacion_conflicto`) ya estaban en el esquema de §8.3.

El paso 10 quedó en tres entregas: 10a (continuidad, portal y anonimización), **10b**
(ésta) y 10c (modo interactivo `--tui` y `export-csv`).

## A. Extensión dinámica de nodos

### 1. La cerca

**El requisito** (RF-CLI-20 a 27, RN-32 y 33). Añadir y quitar campos con `ALTER TABLE`,
sin recompilar, pero sólo sobre una lista blanca de tablas y nunca sobre las probatorias.

**Decisión.** Todo vive en `models/schema_extender.go`, que no ejecuta SQL: valida y
**compone** la sentencia DDL. La ejecuta `repository.go`.

- **Lista blanca** (`NodosExtensibles`): las once tablas de RF-CLI-23. Lo que no está en
  ella no admite DDL. Las ocho de RF-CLI-24 se nombran aparte (`NodosVedados`) sólo para
  explicarle a quien lo intenta por qué no.
- **Toda columna dinámica se llama `x_…`.** Quien crea el campo da un nombre; la columna
  es `x_` + ese nombre. Así una columna dinámica nunca choca con una del esquema, presente
  o futura, y `schema drop` **no puede** tocar una columna nativa: sólo alcanza columnas
  registradas en el diccionario, que por construcción llevan el prefijo.
- **El nombre tiene una sola forma** (`^[a-z][a-z0-9_]{1,39}$`) y la tabla sale de la
  lista, nunca del texto que se escribió. Con eso el DDL puede componerse sin riesgo. La
  comprobación se repite justo antes de componer (`identificadorSeguro`), a propósito.
- **Tipos**: texto, entero y decimal (`TEXT`, `INTEGER`, `REAL`), comunes a SQLite y
  MariaDB.
- **Sólo desde `fimnme-cli schema`.** El servidor web **nunca** ejecuta DDL: no hay
  endpoint para añadir ni quitar campos. Alterar el esquema exige acceso al servidor.
- **Cada `ALTER TABLE` queda en la bitácora tal como se ejecutó** (`schema.alter`).
- En SQLite el DDL es transaccional: el `ALTER` y su registro en el diccionario entran
  juntos o no entran. En MariaDB un `ALTER` confirma por su cuenta; al migrar habrá que
  registrar primero y alterar después (queda anotado en el código).

### 2. Un campo extendido es un campo más, con tres límites

Un campo se identifica por su ruta `ext.<tabla>.<columna>` y **viaja por los mismos
caminos que los campos nativos**: se lee en `completarPostulacion`, se valida con
`ValidarValorDeCampo` y se guarda con `guardarCampos`. No hay un segundo motor de
captura. Los límites (RF-CLI-25 y 26) son estructurales:

| Límite | Cómo se garantiza |
|---|---|
| No altera la obligatoriedad ni los criterios | Siempre es opcional: `ValidarParaEnvio` sólo conoce el formulario, los requisitos y los criterios, que viven en sus tablas |
| No entra en el acuse | `SnapshotDeEnvio` copia únicamente los campos del formulario |
| No se desbloquea en una subsanación | `PermisoDeEdicion.Campo` devuelve falso para toda ruta `ext.` salvo en un borrador, **aunque el revisor abra el programa de obras**; y el revisor no puede señalarlo |

### 3. Quién lo ve y quién lo captura

`visible_en` lista los perfiles que ven el campo además del administrador y el observador,
que los ven todos: `postulante`, `revisor_documental`, `jurado`.

- Con `postulante`, el campo **aparece en el formulario** y lo captura quien postula, por
  el autoguardado de siempre, mientras la postulación es un borrador.
- Sin él, es un dato **interno** que captura la administración
  (`PUT /api/v1/extendidos/{tabla}/{id}`), en cualquier etapa. Por esa vía no puede
  escribirse un campo del postulante.
- Antes de mostrar una postulación, `ExtendidosVisibles` quita los valores que quien
  pregunta no puede ver. Lo usan la respuesta JSON y el expediente.
- Los nodos de configuración (edición, convocatoria, dotación) sólo los ve y los captura
  la administración.

### 4. Dónde aparece solo (RF-CLI-22)

- En el **formulario**: la respuesta de la postulación trae `extendidos`, con la
  definición de cada campo visible y su valor. La interfaz del paso 11 los pinta sin
  conocerlos.
- En el **catálogo de reportes**, con la clave `ext.<tabla>.<columna>` y la etiqueta del
  diccionario, si es exportable.
- En el **expediente**, en una sección «Datos adicionales», para quien puede verlo.
- La exportación a MariaDB llega con el script de migración, en el paso 14.

Una **anonimización** vacía también los campos extendidos de la postulación y de sus
obras: no se sabe qué guardan.

### 5. Límite conocido: integrantes y ligas de video

RF-CLI-23 incluye `integrante` y `enlace_video`. El motor añade la columna y la registra,
pero **aún no hay forma de capturarla**: sus renglones se reescriben completos en cada
guardado de la lista, y un valor capturado se perdería. `schema add` lo avisa. Capturarlos
exige cambiar cómo se guardan esas listas; se hará si llega a necesitarse.

## B. Importación asistida

### 6. Dos fases, y nada entra sin una persona

**El requisito** (RF-IMP-01 a 09, RN-29 a 31).

**Decisión.** Un solo motor (`models/importacion.go`) para la interfaz web y para
`fimnme-cli import`.

1. **Analyze** (`AnalizarImportacion`): lee la hoja, valida su forma y levanta un
   **conflicto** por cada renglón sin correspondencia unívoca en la base. Guarda el
   archivo original en el acervo. **No escribe nada en el dominio.**
2. **Resolución**: una persona decide cada conflicto —vincular a un registro existente,
   crear uno nuevo u omitir—; queda quién y cuándo. Puede cambiarse hasta el Commit.
3. **Commit** (`ConfirmarImportacion`): sólo con todo resuelto, **en una sola
   transacción**. Cada escritura pasa por las mismas reglas que si se hiciera a mano
   (`asignarEn`, `crearUsuarioEn`, las de dotaciones): un jurado de más, una dotación
   repetida o una cuenta que postula hacen fallar el Commit **entero**, señalando el
   renglón. Si la base cambió desde el análisis, se nota aquí y no entra nada.

### 7. Qué es «correspondencia unívoca»

| Importación | Sin conflicto | Con conflicto |
|---|---|---|
| Dotaciones | El nombre es **idéntico** al de una dotación de la convocatoria | Todo lo demás: un nombre nuevo, o uno que sólo se parece (otras mayúsculas, sin acento, uno contenido en otro), con los parecidos como candidatos |
| Cuentas de jurado y de revisor | Nunca | **Siempre** |

**Una cuenta nunca se da por la misma persona sin que alguien lo decida** (RN-30), ni
cuando coincide el correo: la cuenta con ese correo se ofrece como candidata, y también
las que llevan el mismo nombre con otro correo. En una importación de cuentas sólo puede
vincularse a un candidato —vincular a una cuenta cualquiera por su número sería la forma
más fácil de equivocarse de persona—, y no puede crearse una cuenta con un correo ya
registrado.

Los conflictos **sin candidatos** —renglones que no se parecen a nada— pueden resolverse
de una vez («crear todos» u «omitir todos»): sigue siendo una decisión de una persona, y
así queda registrada en cada uno. Los que tienen candidatos se resuelven de uno en uno.

### 8. Una postulación nunca se importa

RN-31. El motor sólo conoce tres tipos (`dotaciones`, `usuarios_jurado`,
`usuarios_revisor`) y no tiene cómo crear una postulación. `TestNadaImportaPostulaciones`
recorre todo `repository.go`: sólo `CrearPostulacion` inserta en esa tabla.

**No se hizo** la «conciliación de postulantes recurrentes entre ediciones» que menciona
RF-IMP-06. En este modelo un postulante **es** su cuenta: quien vuelve a postular con el
mismo correo ya es la misma cuenta, y no hay una entidad «persona» sobre la cual
conciliar dos cuentas distintas. Hacerlo exigiría decidir primero qué significa fundir
dos cuentas (¿de quién son las postulaciones anteriores?); se deja como punto por
confirmar.

### 9. Hojas de cálculo con la biblioteca estándar

- **CSV**: UTF-8, con o sin marca inicial, separado por comas o por punto y coma (lo
  detecta por el encabezado). Un archivo en otra codificación se rechaza diciéndolo.
- **XLSX**: un `.xlsx` es un ZIP con XML. Se lee la primera hoja y la tabla de textos
  compartidos con `archive/zip` y `encoding/xml`: textos simples y con formato, texto en
  la celda, números, celdas vacías que Excel omite y el valor calculado de las fórmulas.
  Cada parte se descomprime con tope de tamaño. Probado con archivos escritos por
  LibreOffice, openpyxl y xlsxwriter.
- El formato se reconoce por el contenido, no por el nombre. Hasta 2 MB y 2000 renglones.
- El primer renglón son los encabezados, con los nombres exactos de las columnas; los
  errores de forma dicen el renglón.

### 10. Cuentas nuevas

Nacen sin contraseña, con su enlace de activación. Desde la web, el enlace va por correo,
encolado en la misma transacción del Commit. Desde `fimnme-cli`, que no envía correo, se
muestran al terminar, como en `crear-usuario`. `fimnme-cli import` exige `--como`, el
correo de un administrador: cada resolución queda con la persona que la dio, no con
«alguien desde la terminal».

## Consecuencias

- El panel del paso 13 tiene ya la API de importación completa, con la vista previa del
  efecto (RF-IMP-08).
- El modo interactivo (`--tui`) del paso 10c puede apoyarse en estas mismas órdenes.
