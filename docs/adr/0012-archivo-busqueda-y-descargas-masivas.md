# ADR 0012 — Archivo histórico, búsqueda y descargas masivas

- **Estado:** aceptada
- **Fecha:** 2026-11-14

Decisiones del paso 9a (§5.12 de la especificación). No añade dependencias ni
migraciones: las tablas (`indice_busqueda`, las dos FTS5, `vista_guardada`,
`tarea_exportacion`, `descarga`) ya estaban en el esquema de §8.3.

El paso 9 se partió en dos entregas. Ésta, **9a**, cubre el archivo: listado, búsqueda,
filtros, CSV del resultado, vistas guardadas, enlaces por folio y descarga masiva. La
**9b** cubrirá el generador de reportes con plantillas, el expediente en PDF y los datos
de tableros.

## 1. El archivo es un listado, no un módulo aparte

**El requisito** (RF-ARCH-01 a 05, RN-23). Archivar es una etiqueta de estado. Una edición
de hace cinco años se consulta con la misma interfaz que la de este año.

**Decisión.** Hay **un** listado, `GET /api/v1/archivo`, que abarca todas las ediciones.
«El archivo de 2027» es ese listado con `edicion=…`. No existe una ruta, una tabla ni un
permiso distintos para lo archivado: así no hay un segundo camino que mantener ni uno
donde las restricciones puedan olvidarse.

## 2. La visibilidad no caduca, y se decide en un solo lugar

**El requisito** (RN-24 y 25). Un jurado o un revisor de una edición pasada sigue viendo
sólo las convocatorias en que participó; un jurado, sólo lo que pasó a evaluación.

**Decisión.** `models.AlcanceDe(sesión)` calcula el alcance —todo, como revisor de ciertas
convocatorias, como jurado de otras— y `condicionesDeListado` lo convierte en el `WHERE`.
El listado en pantalla, el CSV y la descarga masiva pasan por esa misma función: no hay
forma de pedir un ZIP de algo que el listado no mostraría.

| Perfil | Qué encuentra |
|---|---|
| `admin_sistema`, `coordinacion_nacional` | Todo, borradores incluidos |
| Revisor de una convocatoria | Las postulaciones enviadas de esa convocatoria (sin borradores) |
| Jurado de una convocatoria | Sólo las que pasaron a evaluación |
| Postulante | No consulta el archivo (403): tiene su propia lista |

**Filtrar también es leer.** Quien no puede ver un dato no puede filtrar por él: el filtro
`dictamen` (resultado del proceso) sólo surte efecto para la supervisión, y los de la
revisión documental (`con_subsanaciones`, `documentos_pendientes`) no surten efecto para
un jurado. Se ignoran en silencio, de modo que la respuesta no revela nada. El listado
tampoco devuelve el resultado del proceso ni ningún puntaje, a nadie: eso vive en la
matriz de resultados, que es de supervisión (ADR 0011).

El enlace por folio, `GET /api/v1/folios/{folio}`, responde lo mismo que la consulta por
id y con sus mismos permisos; a quien no puede ver la postulación le responde 404.

## 3. Búsqueda: dos índices FTS5 y ninguna sintaxis

**El requisito** (RF-ARCH-10 y 11). Buscar por folio, nombres, nombre artístico, ensamble,
integrantes, títulos de obra, compositores y correos; tolerar acentos y erratas.

**Decisión.**

- **Qué se indexa** lo decide `models.TextoDeBusqueda` y nada más: exactamente la lista de
  arriba. No entran semblanzas, teléfonos ni fechas de nacimiento.
- **Cuándo.** `reindexar` se llama **dentro de la transacción** que modifica la
  postulación; el buscador nunca queda atrasado. Si el texto no cambió no escribe (la
  mayoría de los autoguardados tocan campos que no se buscan). Los disparadores de la
  migración 0001 propagan `indice_busqueda` a las dos tablas FTS5.
- **Cómo se busca.** Primero por palabras, cada una como inicio de palabra, sin distinguir
  acentos ni mayúsculas (`unicode61 remove_diacritics 2`): «nunez carr» encuentra «Núñez
  Carrillo». Si eso no encuentra nada, por fragmento con el índice de trigramas: «arteto»
  encuentra «Cuarteto». Todas las palabras deben estar.
- **Lo escrito nunca es sintaxis.** `ConsultasDeBusqueda` conserva sólo letras y dígitos y
  entrecomilla cada palabra: comillas, asteriscos, `OR` o `NEAR` de quien busca no se
  interpretan.

`fimnme-cli migrate` rehace el índice completo cada vez. Es barato (cientos de
postulaciones) y deja buscables las postulaciones anteriores a este paso.

**Límite conocido.** La «tolerancia a erratas» es por fragmento, no por distancia de
edición: «Carillo» no encuentra «Carrillo», pero «arril» sí. SQLite no trae búsqueda
difusa sin extensiones, y añadir una es una dependencia. Para nombres propios mal
recordados el fragmento resuelve el caso habitual.

## 4. Filtros en la URL; el orden sale de una tabla

Los filtros son parámetros de la consulta (RF-ARCH-14), los mismos para el listado, el CSV
(`formato=csv`) y la descarga masiva. Una **vista guardada** (RF-ARCH-18) es un nombre y
una ruta con sus parámetros; sólo se admiten rutas de este sitio (empiezan con una sola
`/`), para que una vista no pueda llevar a otro. Son de cada cuenta; hasta 50.

El `ORDER BY` se toma de `columnasDeOrden`, nunca del texto de la petición. No se ordena
por puntaje: cruza a los jurados y sólo existe en la matriz.

## 5. Descarga masiva: en segundo plano, a disco y por siete días

**El requisito** (RF-ARCH-31 a 37, RN-27).

**Decisión.**

- `POST /api/v1/exportaciones` encola una tarea y responde 202. El
  `TrabajadorExportaciones` la arma: copia cada archivo del acervo al ZIP **por trozos y
  sin comprimir** (partituras y audio ya vienen comprimidos), sin cargar nada en memoria.
  Escribe en un temporal y lo renombra al terminar: nunca puede bajarse un ZIP a medias.
- **Alcance y tipos.** El ZIP trae los documentos **vigentes** de las postulaciones que el
  listado mostraría con esos filtros; `tipos=partitura,particellas` limita a esos
  documentos, para no bajar gigas de audio cuando sólo se quieren partituras.
- **Nomenclatura** (`models.RutaEnZIP`): una carpeta por postulación y, dentro, una por
  obra. `FIM49-COM-0007_Maria_Nunez/obra-02_Rapsodia/partitura.pdf`. Sin acentos ni
  caracteres que algún sistema de archivos rechace. Nunca el nombre original del archivo.
- **`manifiesto.csv`**: por cada archivo, folio, postulante, obra, documento, versión,
  tamaño y SHA-256 del original.
- **Los ZIP no entran al acervo.** Viven en `exportaciones/` dentro del directorio de
  datos, se borran al vencer y **no hace falta respaldarlos**: se vuelven a pedir.
- **Vigencia: siete días**, y sólo puede bajarlo quien lo pidió (a los demás, 404).
- **Queda registrado** quién lo pidió y cada vez que se baja, con cuenta, IP y momento
  (`descarga` y bitácora). La bitácora anota el alcance, no nombres.
- **Quién.** Administrador, observador y revisores (éstos, sólo su convocatoria). El
  **jurado no descarga en masa**: baja los documentos uno por uno, y cada descarga queda
  registrada. El postulante puede pedir el ZIP de **su** postulación.
- **Apagados.** Si el servicio se detiene a media tarea, al arrancar la devuelve a la cola
  (`ReencolarExportacionesInterrumpidas`). El trabajador es el único que las arma.

**No se hizo** (prioridad C): partir en volúmenes un ZIP muy grande, y el CSV en XLSX.

## Consecuencias

- El frontal del paso 13 sólo tiene que pintar: toda la lógica de alcance, búsqueda y
  filtros ya está detrás de `GET /api/v1/archivo`.
- El acervo crece ~15 GB al año; un ZIP de «todo» de una convocatoria ocupa en disco lo
  mismo que sus documentos durante siete días. Conviene dimensionar el disco con ese
  margen, o bajar la vigencia (`models.VigenciaDeExportacion`).
