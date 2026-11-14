# Plan de desarrollo — Plataforma FIMNME

Plan por pasos para construir lo especificado en
[`FIMNME-01`](FIMNME-01-definicion-de-requerimientos-y-modelo-de-datos.md), bajo las
políticas de [`PERFIL_DOMINIO_FIMNME.md`](PERFIL_DOMINIO_FIMNME.md).

**Forma de trabajo.** Un paso a la vez. Cada paso se entrega compilando, con pruebas en
verde (`make verificar`), desplegable en el VPS de desarrollo, y se espera indicación
antes de iniciar el siguiente.

**Orden: primero el backend.** (Plan rehecho el 2026-11-12; la versión anterior ponía el
frontal primero.) El paso 1, ya entregado, dejó los cimientos y las páginas públicas. A
partir de aquí se construye todo el backend —persistencia, reglas, control de acceso y
procesos— y sólo después las pantallas de cada perfil.

**Cómo se revisa un paso de backend sin pantallas.** Cada paso entrega:

- las reglas en `models/rules.go` y el SQL en `models/repository.go`, con pruebas
  unitarias;
- sus endpoints REST en `/api/v1`, que responden JSON (los mismos manejadores
  responderán HTML para htmx cuando existan las pantallas, §7.4);
- pruebas de extremo a extremo sobre una base SQLite temporal, y ejemplos con `curl`
  para probarlo a mano en el VPS.

**Decisiones de la CNMO del 13 de noviembre de 2026.** Las decisiones tomadas hasta el
paso 5 quedan confirmadas. La evaluación no es a ciegas. La identidad visual es, por
ahora, la del 48º Foro ([ADR 0008](adr/0008-identidad-visual-del-48-foro.md)).

**Guía de programación.** Desde el 13 de noviembre de 2026 rige además la guía de
programación de la CNMO para proyectos afines, en lo que aplica a este sistema
([ADR 0006](adr/0006-adopcion-de-la-guia-soshe-peelee.md)). Al adoptarla se revisó lo ya
hecho en los pasos 1 a 4: aviso de licencia GNU completo, abstracción de la persistencia
en el Modelo, descripción OpenAPI de la API, convenciones de `--version` y de opciones, y
páginas de manual.

## Pasos

### Hecho

| # | Paso | Estado |
|---|---|---|
| 1 | **Cimientos y páginas públicas.** Estructura MVC, servidor `fimnme-web`, frontal incrustado, CSS propio, i18n es/en, cabeceras de seguridad y CSRF, portada, detalle de convocatoria, pantallas de cuenta (sin conectar), README y despliegue en VPS. | ✅ Hecho |
| 2 | **Persistencia base.** SQLite sin CGO con los PRAGMAs en cada conexión; una conexión de escritura y grupo de lectura; migrador propio y esquema completo de §8.3; `models/repository.go`; bitácora de auditoría de sólo inserción; `fimnme-cli` con `migrate` y `sembrar-demo`; las páginas públicas leen de la base; `/readyz` comprueba la base; despliegue con migración. | ✅ Hecho |
| 3 | **Cuentas, sesiones y control de acceso.** Registro, verificación de correo, ingreso, recuperación, activación de cuentas internas y «Mi cuenta»; Argon2id; sesiones con expiración por perfil; enlaces de un solo uso; bloqueo por intentos y límite por IP; perfiles globales y asignaciones por convocatoria; consulta de la bitácora por API; bandeja de correo saliente con interfaz `Mailer` y su trabajador. | ✅ Hecho |
| 4 | **Configuración de ediciones y convocatorias.** API para ediciones, convocatorias con bases bilingües y límites, calendario, dotaciones, requisitos documentales, criterios, cuentas internas y asignaciones; cambio de etapa forzado y automático por calendario, con las cuotas de 3 jurados y 1+ revisores; duplicación de la configuración a otra edición. | ✅ Hecho |
| 5 | **Postulación: captura y reglas.** Una postulación por convocatoria; subcategoría; autoguardado por campo; datos de solista, ensamble y compositor; integrantes; propuesta de concierto con la regla de sede fuera de CDMX; obras con sus límites; ligas de video; `ValidarParaEnvio()` con la lista de bloqueadores. La captura se acepta sólo dentro del plazo, y quien es jurado o revisor de una convocatoria no postula en ella. | ✅ Hecho |
| 6 | **Documentos, envío y acuse.** Almacén de archivos en disco por SHA-256; formato por firma, tamaño y deduplicación; versiones sin sobrescritura; descarga con permiso verificado y registrada; envío irreversible en una transacción (folio, snapshot, correo encolado); acuse PDF con escritor propio (3 KB); comprobación en red de las ligas de video, con defensas contra SSRF; recordatorios de borrador; bases en PDF e imagen de cabecera. | ✅ Hecho |
| 7 | **Revisión documental y subsanación.** Bandeja con filtros y tablero; toma blanda; dictamen por documento con motivo; observaciones; señalamiento de campos, documentos y listas; solicitud de subsanación con su correo; motor de desbloqueo selectivo; solicitudes de desbloqueo; reenvío con acuse nuevo y *diff*; dictamen validada / descartada; vencimiento sin descarte automático. | ✅ Hecho |
| 8 | **Evaluación por jurado.** `JuradoRepo` y `SupervisionRepo` separados; evaluación por postulación (Intérpretes) o por obra (Compositores); borrador, envío y congelación al cierre; avance propio; matriz y suma de los 3 jurados para supervisión; dictamen final; **aislamiento entre jurados probado** con un barrido de todos los endpoints. | ✅ Hecho |
| 9a | **Archivo, búsqueda y descargas masivas.** Un solo listado para todas las ediciones, con visibilidad que no caduca; búsqueda FTS5 tolerante a acentos y por fragmento; filtros combinables en la URL, orden y paginación; CSV del resultado; vistas guardadas; enlace permanente por folio; ZIP en segundo plano con selección de tipos, nomenclatura predecible y `manifiesto.csv`, registrado y con vigencia de 7 días. | ✅ Hecho |
| 9b | **Reportes, expediente y tableros.** Generador de reportes CSV con catálogo de campos, granularidad por postulación u obra, filtros, rango de puntaje y plantillas reutilizables; reporte de resultados con empates; expediente PDF individual y por convocatoria, **recortado según quién lo pide**, con fotografía, índice y marcadores; tablero de la convocatoria y estadísticas de la edición, con cifras navegables. | ✅ Hecho |
| 10a | **Continuidad, portal público y anonimización.** `fimnme-cli verify` del acervo; espejo con `VACUUM INTO`, manifiesto firmado con HMAC-SHA256 y acervo replicado por contenido; alerta de espacio en disco; exportación sanitizada al portal público (lista blanca hecha tipo) con publicación explícita de la nómina; anonimización ARCO con confirmación doble. | ✅ Hecho |

### Backend

| # | Paso | Requerimientos | Estado |
|---|---|---|---|
| 10b | **Importación, extensión de campos y TUI.** Importación asistida en dos fases (Analyze → Commit), por CLI y por API; extensión dinámica de nodos con lista blanca (`schema add` / `drop`); modo interactivo `--tui`; `export-csv` desde una plantilla. | §5.14, §5.14.1, §5.15, RN-29 a 33 | Pendiente |

### Frontal

| # | Paso | Estado |
|---|---|---|
| 11 | **Postulante.** Panel «Mis postulaciones»; formulario por pasos con autoguardado e indicador de completitud; obras e integrantes dinámicos; subida con barra de progreso; revisión y envío; vista de subsanación con lo editable resaltado. Accesibilidad AA y móvil. | Pendiente |
| 12 | **Revisor y jurado.** Bandeja con filtros; vista de revisión y dictámenes; solicitud de subsanación; ficha de evaluación; visor de PDF y reproductor de audio. | Pendiente |
| 13 | **Administración, supervisión y archivo.** Configuración de ediciones y convocatorias; usuarios y asignaciones; plantillas de correo; matriz de resultados; buscador global, filtros y descargas; bitácora; importación asistida; publicación al portal; tableros. | Pendiente |

### Operación

| # | Paso | Estado |
|---|---|---|
| 14 | **Puesta en operación.** `Containerfile`, `compose.yaml` y Quadlet; respaldos automatizados con prueba de restauración; script de migración a MariaDB; SMTP propio (SPF, DKIM, DMARC) con prueba de entregabilidad; prueba de carga final; manuales de Adopción y Validación. | Pendiente |

## Notas del plan

- **Prueba de carga adelantada.** La escritura concurrente sobre SQLite es el supuesto más
  frágil de la arquitectura (D-30). La primera medición se hizo al terminar el paso 5, con
  el autoguardado real: 60 postulantes simultáneos, 3 000 peticiones, ningún error y p95
  de ~100 ms ([ADR 0007](adr/0007-captura-de-postulaciones.md), §10). La prueba formal,
  en el paso 14.
- **Dependencias.** `modernc.org/sqlite` entró en el paso 2 (ADR 0003) y
  `golang.org/x/crypto` (Argon2id) entra en el paso 3. Son las dos previstas por la
  política; no se añade ninguna otra.
- **Despliegue.** Se suben dos binarios (`fimnme-web` y `fimnme-cli`) y el servicio usa
  `/var/lib/fimnme` para la base y el acervo. El README se actualiza en cada paso.
- **El esquema crece por migraciones.** La migración `0001` trae todo §8.3. Las tablas
  que la especificación menciona sin definir (plantillas de reporte, motivos frecuentes,
  versiones del aviso de privacidad, intentos de ingreso) se añaden con migraciones
  nuevas en el paso que las use.
- **Correo en desarrollo.** Hasta el paso 14 el `Mailer` no envía: escribe el mensaje en
  el log, de donde se toma el enlace de verificación. En el entorno `produccion` el log
  no muestra el cuerpo, de modo que las cuentas no pueden verificarse ni recuperarse
  hasta que exista el SMTP.
- **La configuración se hace por API hasta el paso 13.** No hay pantallas de
  administración todavía: se usa `curl` o cualquier cliente HTTP (ejemplos en el README).
- **Prioridades S y C.** Los requerimientos «should» y «could» (2FA, antivirus, XLSX,
  excusa del jurado, volúmenes de ZIP, comparativas) no están en los pasos de arriba; se
  deciden al llegar al módulo correspondiente. En particular, el **2FA (TOTP) para
  administrador y observador** (RF-AUT-09, prioridad S) no se hizo en el paso 3.

## Paso 10a — qué se entregó

El paso 10 se partió en dos: **10a** (esto) y **10b** (importación asistida, extensión
dinámica de campos y TUI).

**Decisiones:** [ADR 0014](adr/0014-continuidad-portal-publico-y-anonimizacion.md).

- **`fimnme-cli verify`**: compara el acervo con lo que la base registra. Distingue
  pérdida, alteración, purga pendiente y huérfanos; termina con error si falta o cambió
  algo. `--rapido` y `--purgar`.
- **`fimnme-cli mirror`**: `snapshot` copia la base en caliente, replica el acervo por
  contenido y firma un manifiesto con HMAC-SHA256; `verify` lo comprueba todo sin la base
  en producción; `nueva-clave` y `list`.
- **Alerta de espacio en disco** en `fimnme-web`: log y correo diario a los
  administradores por debajo de `FIMNME_DISCO_MIN_GB`.
- **Exportación al portal público**: `GET /api/v1/interop/convocatoria.json` y
  `fimnme-cli export-json`. Bases, calendario y dotaciones en vivo; la **nómina de
  seleccionados, sólo la que el administrador publicó** tras el cierre, congelada.
- **Anonimización ARCO**: `fimnme-cli arco anonimizar FOLIO` y
  `POST /api/v1/postulaciones/{id}/anonimizacion`. Borra los datos personales y los
  archivos que los llevan; conserva obra, dictamen, evaluaciones y estadísticas.
- Migración `0007`.

**Verificado.** `make verificar`. Pruebas del modelo (firma y manipulación del manifiesto,
verificación y réplica del acervo, lista blanca del portal campo por campo, reglas de la
anonimización), de extremo a extremo (el portal en cada momento del proceso, con una lista
negra que nunca debe aparecer; la anonimización, con los agregados idénticos antes y
después; la alerta de disco) y de la herramienta. Una mutación que quita el filtro de
seleccionadas hace fallar las pruebas. En vivo, con los binarios: migración de una base
del paso 9b, `verify`, instantánea y su verificación sin base, exportación, y una
anonimización completa —con sus rechazos— seguida de la retirada de los archivos del
espejo.

**Sin verificar aquí.** El transporte de la instantánea a otro servidor (rsync, `timer`
de systemd), que es del paso 14. La verificación completa sobre un acervo de decenas de
gigas.

**No se hizo:** `export-csv` (prioridad S; va con el 10b).

## Paso 9b — qué se entregó

**Decisiones:** [ADR 0013](adr/0013-reportes-expediente-y-tableros.md).

- **Expediente en PDF de una postulación** (`GET /api/v1/postulaciones/{id}/expediente`),
  con fotografía, en español o inglés. **Cada quien recibe lo suyo**: el titular, su
  postulación; el jurado, además su propia evaluación; el revisor, la revisión documental;
  la supervisión, todo. Queda registrado.
- **Expediente por convocatoria** (`POST /api/v1/expedientes`), en segundo plano: un solo
  PDF con portada, índice, marcadores y una postulación por sección; con hoja de
  calificación en blanco si se pide. El contenido se decide con los permisos vigentes
  **al generarse**. Avisa por correo al terminar.
- **Generador de reportes CSV**: catálogo de campos que sale del formulario, granularidad
  por postulación o por obra, los filtros del archivo, rango y orden por puntaje,
  separador, idioma y **plantillas** que sirven en ediciones posteriores.
- **Reporte de resultados** en CSV, con los empates a la vista.
- **Tablero de la convocatoria** y **estadísticas de la edición**; cada cifra trae el
  filtro del archivo que la produce.
- Migración `0006`: tabla `plantilla_reporte` y plantilla de correo `exportacion_lista`.
- El escritor de PDF ganó imágenes JPEG, marcadores, saltos de página e índice; el acuse
  sigue saliendo igual, byte por byte.

**Verificado.** `make verificar`. Pruebas de extremo a extremo de lo que cada perfil
recibe en el expediente, del expediente por convocatoria (índice, marcadores, permisos al
generar), de los reportes y de los tableros. Dos mutaciones del recorte —dar supervisión
a todos, leer la evaluación de otro jurado— hacen fallar las pruebas. En vivo, con los
binarios: migración de una base del paso 9a, expediente individual y por convocatoria
validados con `pdfinfo`, `pdftotext` y `pdfimages`, y revisados a la vista; reporte,
tablero, estadísticas y aviso por correo.

**Sin verificar aquí.** Un expediente de cientos de postulaciones con fotografías reales
de teléfono (probado con imágenes generadas, incluida la orientación EXIF).

**No se hizo:** anexar las partituras al expediente (RF-ARCH-44, prioridad S), reportes en
segundo plano (S), XLSX y comparativa entre ediciones (C).

## Paso 9a — qué se entregó

El paso 9 se partió en dos: **9a** (esto) y **9b** (reportes, expediente PDF y tableros).

**Decisiones:** [ADR 0012](adr/0012-archivo-busqueda-y-descargas-masivas.md).

- **Un solo listado**, `GET /api/v1/archivo`, para todas las ediciones, vivas y archivadas.
  Lo que cada cuenta ve lo decide `models.AlcanceDe` y no caduca: supervisión, todo;
  revisor, lo enviado en su convocatoria; jurado, sólo lo que pasó a evaluación.
- **Búsqueda de texto completo** (FTS5) sobre folio, nombres, nombre artístico, ensamble,
  integrantes, títulos de obra, compositores y correos. Sin distinguir acentos ni
  mayúsculas, por inicio de palabra y, si no hay resultados, por fragmento. El índice se
  actualiza en la misma transacción que la postulación.
- **Filtros combinables en la URL**: edición, convocatoria, tipo, subcategoría, estado,
  dotación, entidad de residencia y del concierto, nacionalidad, país, género, estreno,
  fechas de envío, con subsanaciones, documentos pendientes, ligas con problema y
  selección por ids. **Orden** por seis columnas y **paginación** con el total.
- **CSV del resultado** (`formato=csv`) y **vistas guardadas** por cuenta.
- **Enlace permanente por folio**: `GET /api/v1/folios/{folio}`.
- **Descarga masiva en ZIP**, en segundo plano: mismos filtros que el listado, selección
  de tipos de documento, carpetas por folio y por obra, `manifiesto.csv` con SHA-256,
  vigencia de siete días, sólo para quien la pidió, y registrada. El jurado no descarga
  en masa; el postulante, sólo su postulación.
- `fimnme-cli migrate` reconstruye el índice de búsqueda.

**Verificado.** `make verificar`; pruebas de extremo a extremo de búsqueda, filtros,
alcance por perfil, vistas y ZIP (contenido byte por byte, manifiesto, permisos,
vencimiento y tarea interrumpida); una mutación que relaja el alcance del jurado hace
fallar las pruebas. En vivo, con los binarios: migración de una base del paso 8, listado,
CSV, ZIP y descarga por rangos.

**Sin verificar aquí.** Un ZIP de varios gigas detrás de nginx o Caddy reales (los
tiempos de espera de los ejemplos de `deploy/` ya contemplan descargas largas).

**No se hizo** (prioridad C): volúmenes de ZIP y XLSX.

## Paso 8 — qué se entregó

- **Lista de trabajo del jurado** (RF-EVA-01 y 07). Las unidades por evaluar, el estado de
  su evaluación de cada una y su avance.
- **Unidad de evaluación** (RF-EVA-02, RN-16). La postulación en Intérpretes; cada obra en
  Compositores.
- **Calificación** (RF-EVA-03, 04 y 06, RN-17 a 19). Criterios de la convocatoria, enteros
  dentro de su escala; observaciones opcionales; borrador incompleto; envío completo;
  corrección mientras dure la etapa; congelación al cierre.
- **Aislamiento entre jurados** (RF-EVA-05, RN-15, §8.4). Estructural: `JuradoRepo` sólo
  alcanza lo propio; lo que cruza jurados vive en `SupervisionRepo`. Lo vigilan dos
  pruebas que leen el código y una que barre todos los endpoints con la sesión de cada
  jurado.
- **Evaluación no a ciegas** (decisión de la CNMO). El jurado ve la postulación completa
  y descarga sus documentos.
- **Paso a evaluación** (RN-20). Las validadas pasan al jurado. La convocatoria avanza
  aunque queden postulaciones en revisión: se validan después y se suman.
- **Excusa por conflicto de interés** (RF-EVA-12). El jurado se excusa de una unidad, con
  motivo; la matriz ordena por un puntaje que promedia entre quienes sí calificaron, para
  que la excusa no perjudique.
- **Postulación evaluada** (RF-EVA-09, RN-18) cuando los tres han enviado todo.
- **Matriz de resultados** (RF-EVA-08). Puntaje por jurado y criterio, suma, promedio y
  lugar. Sólo para supervisión; cada consulta queda en la bitácora.
- **Dictamen final** (RF-CONV-10). Lo registra el administrador; sólo lo ve la supervisión.
- **Sin migraciones nuevas**: todo cabía en el esquema de §8.3.

Decisiones: [ADR 0011](adr/0011-evaluacion-por-jurado.md).

**Lo que este paso NO incluye:**

- **Correos al jurado** (RF-NOT-09 y 10, prioridad S).
- **Matriz en CSV**: paso 9.
- **Pantallas del jurado y de la matriz**: pasos 12 y 13.

## Paso 7 — qué se entregó

- **Bandeja del revisor** (RF-REV-01 y 10). Todas las postulaciones enviadas de la
  convocatoria, sin reparto; filtros por estado, subcategoría, toma y texto; tablero de
  avance. Marca las subsanadas y las vencidas.
- **Toma blanda** (RF-REV-01b). «La reviso yo» informa a los demás y no bloquea.
- **Dictamen por documento** (RF-REV-02 y 03, RN-11). Válido, inválido —con motivo
  obligatorio— o pendiente. Se conserva el historial.
- **Observaciones** internas, sobre la postulación o sobre una obra (RF-REV-04).
- **Subsanación** (RF-REV-05 y 05b, §4.2). El revisor señala campos, documentos y listas;
  los documentos inválidos se añaden solos. Un solo correo al postulante con la lista y el
  plazo, en su idioma.
- **Desbloqueo selectivo** (RN-12, 12b y 12c). Durante la subsanación el postulante
  modifica **exactamente** lo señalado; todo lo demás se rechaza. Un solo objeto,
  `PermisoDeEdicion`, gobierna todas las escrituras.
- **Solicitudes de desbloqueo** (RF-POST-14, RF-REV-05c, RN-12d). El postulante pide
  corregir algo no señalado; el revisor concede o niega, con motivo.
- **Reenvío** (RF-REV-07, RN-13). Misma validación; mismo folio; acuse versión 2, sin
  tocar el anterior; aviso a los revisores; la vista de revisión trae el *diff* entre los
  dos envíos.
- **Dictamen** (RF-REV-08). Validada o descartada, con motivo al descartar. Sin correo.
- **Vencimiento** (RF-REV-09). No descarta: marca la postulación para el revisor.
- **Límites de los documentos**, por decisión de la CNMO: audio, partitura y particellas
  hasta 30 MB; las particellas, en un solo PDF.
- **El descarte no se muestra al postulante**: es un estado interno de la revisión.
- **Migraciones `0004` y `0005`**: plantillas de correo de la subsanación y los límites.
- **Pruebas.** El motor de desbloqueo caso por caso; y un recorrido completo por la API:
  revisión, subsanación, lo que el postulante puede y no puede tocar, desbloqueos,
  reenvío, *diff*, dictamen y vencimiento.

Decisiones: [ADR 0010](adr/0010-revision-documental-y-subsanacion.md).

**Lo que este paso NO incluye:**

- **Catálogo de motivos frecuentes** (parte de RF-REV-03): el motivo es texto libre.
- **Pantallas del revisor**: paso 12.
- **Paso de las validadas a evaluación**: paso 8.
- **Búsqueda tolerante a acentos y erratas** en la bandeja: paso 9.

## Paso 6 — qué se entregó

- **Almacén de archivos** (§7.8, RF-DOC-02). En disco, junto a la base, direccionado por
  SHA-256: deduplicación, integridad verificable y archivos inmutables. La subida va al
  vuelo, sin cargarse en memoria.
- **Subida de documentos** (RF-POST-07, RF-DOC-01 y 04). Formato reconocido por la firma
  del contenido, no por la extensión; tamaño y formatos según el requisito, que depende de
  la dotación de la obra. Un archivo de formato equivocado se rechaza antes de tocar el
  disco.
- **Versiones** (RF-DOC-07). Reemplazar o quitar un documento no borra nada: el anterior
  deja de estar vigente.
- **Descarga** (RF-DOC-03, RN-27). Siempre por la aplicación, con permiso comprobado,
  nombre predecible y registro de cuenta, IP y momento. Por rangos, para audio.
- **Envío** (§4.1, RF-POST-10, RN-03 y 13). Irreversible y en una sola transacción:
  validación completa, confirmaciones, folio (`FIM49-INT-0137`), snapshot, acuse y correo
  encolado con el PDF adjunto.
- **Acuse en PDF** (§5.10). Escritor propio en `internal/pdf`, sin fuentes incrustadas ni
  imágenes; bilingüe; reproducible byte por byte. Pesa unos 3 KB.
- **Ligas de video** (RF-DOC-10, RN-08). Comprobación en red con defensas contra SSRF;
  en segundo plano, antes de cada envío y una vez al día. Sólo bloquea una respuesta
  inequívoca.
- **Recordatorios de borrador** (RF-NOT-04) a 7 días, 2 días y 12 horas del cierre.
- **Bases en PDF e imagen de cabecera** (RF-CONV-02), públicas en la página de la
  convocatoria.
- **Migración `0003`**: plantillas de correo del acuse y del recordatorio.
- **Despliegue.** Los ejemplos de Caddy y nginx admiten subidas de hasta 210 MB y las
  pasan a la aplicación sin guardarlas antes.
- **Pruebas.** Las reglas caso por caso; el PDF verificado en su estructura y con
  herramientas externas (`pdfinfo`, `pdftotext`); los recorridos completos por la API; y
  una prueba en vivo que encontró un fallo real (véase abajo).

Decisiones: [ADR 0009](adr/0009-documentos-envio-y-acuse.md).

**Dos fallos encontrados al probar, ya corregidos:**

- Con un DNS que responde «no existe» a todo, el verificador daba por inexistente una
  liga válida y **bloqueaba el envío**. Ahora, si el servidor no resuelve ningún dominio
  testigo, la liga queda pendiente.
- La consulta de recordatorios leía las columnas en otro orden y no encontraba a nadie.

**Lo que este paso NO incluye:**

- **Antivirus** (RF-DOC-05, prioridad S).
- **Visor de PDF y reproductor integrados** (RF-DOC-08, prioridad S): paso 12. Exigirá
  abrir la CSP para las rutas de archivos.
- **Página de la postulación por folio** (`/postulaciones/{folio}`): paso 11. El correo
  de acuse ya enlaza a ella.
- **Reenvío tras subsanación** (acuse versión 2): paso 7. El envío ya numera las
  versiones.
- **Envío real del correo**, con su adjunto: paso 14.
- **Descarga masiva en ZIP**: paso 9.
- **Verificación periódica del acervo** (que cada archivo registrado exista y coincida)
  y limpieza de archivos huérfanos: paso 10.

## Paso 5 — qué se entregó

- **Una postulación por convocatoria y cuenta** (RF-POST-01, RN-01). En Intérpretes se
  elige la subcategoría al crearla; cambiarla después exige confirmación, porque borra
  los datos generales (RF-POST-02).
- **Autoguardado por campo** (RF-POST-03 y 04). `PATCH` con uno o varios campos: lo válido
  se guarda y lo demás se señala. Al guardar se valida el formato, nunca la
  obligatoriedad. Cada formulario es una definición declarativa en `models/rules.go`, de
  la que salen la validación, la completitud, el SQL y la descripción para el frontal.
- **Datos generales** de solista, ensamble y compositor (§5.3.1 a 5.3.3), con catálogos de
  países (ISO 3166-1) y entidades federativas; **integrantes** del ensamble (RN-10).
- **Propuesta de concierto** con la regla de sede: fuera de la Ciudad de México se exigen
  sede, domicilio, fecha y hora (RN-06).
- **Obras** (§5.3.5 y 5.3.6): añadir, editar, quitar y reordenar. Sin máximo en
  Intérpretes; máximo configurable en Compositores, cada obra con su dotación (RN-05).
- **Ligas de video** (RN-08): de una a tres, `https`, sin repetir, con su plataforma
  detectada. Quedan `pendiente` hasta la comprobación en red del paso 6.
- **`ValidarParaEnvio()`** (RN-03, §8.4): la lista exacta de lo que falta, incluidos los
  documentos obligatorios según la dotación de cada obra (RN-07) y el correo sin
  verificar. La API la devuelve en cada consulta, con el conteo por sección.
- **Plazo** (RN-02 y 04): la captura exige etapa de postulación **y** fecha dentro del
  plazo, aunque la convocatoria no haya avanzado.
- **Permisos.** Cada cuenta ve y edita sólo lo suyo; lo ajeno responde 404. Administrador
  y observador consultan sin editar. El observador no postula; el jurado o revisor de una
  convocatoria tampoco, en ella.
- **Sin migraciones nuevas**: todo cabía en el esquema de §8.3.
- **Pruebas.** `ValidarParaEnvio` regla por regla y campo por campo en los tres
  formularios; los tres recorridos completos por la API, con permisos, plazo y cambio de
  subcategoría. Primera medición de carga del autoguardado.
- **Correcciones de paso.** Al quitar una obra se renumera el programa. Una prueba del
  paso 3 dependía de la fecha real del sistema y empezó a fallar el 13 de noviembre: se
  corrigió.

Decisiones: [ADR 0007](adr/0007-captura-de-postulaciones.md).

**Lo que este paso NO incluye:**

- **Subir documentos y enviar**: paso 6. `ValidarParaEnvio` ya exige los documentos, así
  que ninguna postulación está «lista para enviar» todavía.
- **Comprobación en red de las ligas de video** (RF-POST-07): pasó al paso 6, porque
  exige defensas contra SSRF y un trabajador en segundo plano.
- **Nombres de los países**: el backend maneja códigos; los nombres van en el frontal
  (paso 11).
- **Acceso de jurados y revisores a las postulaciones**: pasos 7 y 8.
- **Pantallas**: paso 11. La captura se prueba por API.

## Paso 4 — qué se entregó

- **Ediciones y convocatorias** (RF-CONV-01, 02 y 08). Alta y edición por API; bases en
  Markdown en ambos idiomas, con aviso de lo que falta en inglés; límites de obras, de
  duración y de ligas de video. Una convocatoria nace en borrador con los requisitos
  documentales y los criterios iniciales de su tipo.
- **Calendario y etapas** (RF-CONV-03 y 04, §3). Calendario validado (coherente, sin
  traslapes). Cambio de etapa forzado, sólo al estado contiguo, con motivo obligatorio al
  retroceder. Avance automático por fecha cada 30 segundos y al arrancar. Ambos pasan por
  la misma regla.
- **Cuotas** (RF-CONV-09, RN-14). Sin revisor no se pasa a revisión documental; sin
  exactamente tres jurados, ni criterios, no se pasa a evaluación. La API anticipa en
  `siguiente.bloqueadores` qué falta.
- **Catálogos** (RF-CONV-06 y 07, RF-EVA-10). Dotaciones; requisitos documentales
  generales y por dotación, con formatos, tamaño y cantidad máximos; criterios de
  evaluación con su escala. `RequisitosAplicables` resuelve la regla efectiva por
  dotación.
- **Cuentas internas y asignaciones** (RF-AUT-07, RF-NOT-08). Alta con enlace de
  activación por correo; búsqueda de cuentas; asignar y retirar jurados y revisores.
- **Duplicar** (RF-CONV-05). Copia textos, límites, dotaciones, requisitos y criterios a
  otra edición.
- **Sólo lectura al cerrar** (RF-ARCH-03). Cerrada o archivada, la configuración no se
  modifica; desarchivar exige motivo.
- **Bitácora.** Cada cambio de configuración y de etapa queda asentado; los automáticos,
  a nombre del sistema.
- **Sin migraciones nuevas**: todo cabía en el esquema de §8.3.
- **Pruebas.** Una convocatoria armada desde cero y llevada por todo su ciclo por la API,
  con reloj controlado; reglas de transición caso por caso.

**Lo que este paso NO incluye:**

- **Pantallas de administración**: paso 13. La configuración se hace por API.
- **Imagen de cabecera y PDF de bases**: paso 6.
- **Desactivar cuentas y cambiar perfiles globales de cuentas existentes.**
- **Vista previa de la convocatoria** (RF-CONV-11, prioridad S): va con las pantallas.

## Paso 3 — qué se entregó

- **Registro y verificación** (RF-AUT-01 y 02). Alta con consentimiento del aviso de
  privacidad (versión y sello de tiempo); la cuenta queda con sesión iniciada y sin
  verificar; enlace de verificación de 24 horas y un solo uso, que se confirma con un
  botón; reenvío con espera mínima de 5 minutos.
- **Ingreso** (RF-AUT-03, 08 y 10). Argon2id; mensaje y tiempo de respuesta idénticos
  exista o no la cuenta; bloqueo temporal desde el quinto fallo con retroceso exponencial
  y tope de 30 minutos; cada intento queda en la bitácora con IP y agente; sesión por
  cookie con expiración por inactividad de 8 horas (perfiles internos) o 30 días
  (postulante).
- **Recuperación y activación** (RF-AUT-04 y 07). Enlace de 1 hora para restablecer;
  enlace de 7 días para activar una cuenta interna; ambos cierran las sesiones abiertas.
- **Mi cuenta** (RF-AUT-06). Cambio de nombre, apellidos e idioma; de contraseña (cierra
  las demás sesiones); y de correo (exige la contraseña, obliga a verificar de nuevo y
  avisa a la dirección anterior).
- **Control de acceso** (§2). Perfiles globales y asignaciones por convocatoria leídos de
  la base en cada petición; páginas y API protegidas; `GET /api/v1/sesion`.
- **Bitácora** (RF-AUD-01 a 03). `GET /api/v1/auditoria` con filtros, paginación y
  exportación CSV, sólo para `admin_sistema` y `coordinacion_nacional`. La bitácora no
  guarda datos personales.
- **Correo saliente** (§5.8, RF-NOT-01, 02 y 08). Plantillas bilingües en la base
  (migración `0002`); bandeja `email_outbox` escrita en la misma transacción que la
  operación; trabajador con límite por minuto y reintentos; interfaz `Mailer`.
- **Límite de intentos** (RNF-10): 30 por minuto y por IP en los formularios de cuenta.
- **`fimnme-cli crear-usuario`**: alta del primer administrador o de un observador.
- **Pruebas.** Flujos completos por HTTP sobre SQLite real con reloj controlado.
  Cobertura contando todas las pruebas: 80 % en modelos.

**Lo que este paso NO incluye:**

- **Envío real de correo.** Sólo existe el transporte de desarrollo, que escribe en el
  log. El SMTP propio es el paso 14.
- **2FA (TOTP)** para administrador y observador (RF-AUT-09, prioridad S).
- **Alta de cuentas internas desde la web y asignación de jurados y revisores**: paso 4.
  La tabla `asignacion` ya se lee, pero aún no hay forma de llenarla salvo en pruebas.

## Paso 2 — qué se entregó

- **Base de datos.** `modernc.org/sqlite` v1.62.0, sin CGO. Los siete PRAGMAs de §7.5 se
  emiten en cada conexión. Una conexión de escritura (`BEGIN IMMEDIATE`) y ocho de
  lectura marcadas `query_only`.
- **Esquema.** Migración `0001_esquema_inicial.sql`: el DDL de §8.3 copiado del documento
  por programa (42 tablas, la vista `v_resultado`, los dos índices FTS5), más los triggers
  que el documento anuncia: bitácora de sólo inserción, rango de puntajes y
  sincronización de los índices de búsqueda.
- **Migrador.** Archivos `NNNN_nombre.sql` incrustados; una transacción por migración;
  registro en `schema_migration` con SHA-256; rechazo de migraciones alteradas y de bases
  más nuevas que el binario. `fimnme-web` no migra y no arranca con migraciones
  pendientes.
- **Repositorio** (`models/repository.go`, único lugar con SQL): ediciones,
  convocatorias con su calendario, lectura pública y auditoría dentro de la misma
  transacción que cada escritura.
- **`fimnme-cli`**: `migrate`, `sembrar-demo` (sólo desarrollo) y `version`. Sus
  operaciones quedan en la bitácora con el usuario del sistema. Se niega a trabajar
  sobre datos de otro usuario.
- **Servidor.** Bandera `--datos` / `FIMNME_DATOS`; `/healthz` (proceso vivo) y `/readyz`
  (la base responde, o 503). Desaparece `models/demo.go`.
- **Despliegue.** `make deploy` sube ambos binarios y ejecuta `deploy/actualizar_vps.sh`:
  detener, migrar como usuario `fimnme`, arrancar. `make dev` prepara la base local.
- **Pruebas.** 4 paquetes; cobertura 94 % en controladores, 76 % en modelos, 75 % en
  vistas. Entre ellas: PRAGMAs conexión por conexión, llaves foráneas, bitácora
  inmutable, operación fallida sin rastro parcial, búsqueda FTS5 con acentos y por
  subcadena, vista de resultados, y 600 escrituras simultáneas desde dos procesos sin
  errores.

**No verificado en este paso:** el arranque bajo systemd real, con su endurecimiento
(`ProtectSystem=strict`, `MemoryDenyWriteExecute`), sobre la base SQLite. El entorno donde
se desarrolló no tiene systemd: el guion de actualización se probó con `systemctl`
simulado y el binario se ejecutó como usuario `fimnme` sobre `/var/lib/fimnme`. Conviene
confirmarlo en el primer despliegue al VPS con `curl -i http://127.0.0.1:8080/readyz`.

## Paso 1 — qué se entregó

- Servidor `fimnme-web`: `127.0.0.1:8080` por omisión, banderas `-s`/`--serve`, `--port`
  y `--entorno`; logs JSON (`log/slog`); apagado ordenado; `/healthz` y `/readyz`.
- Seguridad: CSP estricta sin excepciones, HSTS tras TLS, protección CSRF de la
  biblioteca estándar, límite de tamaño de formularios, redirecciones sólo locales.
- Frontal incrustado (`//go:embed`): plantillas, CSS propio con modo claro y oscuro,
  htmx 2.0.11, traducciones.
- Bilingüe: preferencia por cookie > `Accept-Language` > español; fechas en hora de la
  Ciudad de México con la zona explícita.
- Páginas: portada, convocatoria (`/convocatorias/{edición}/{tipo}`), crear cuenta,
  ingresar, recuperar contraseña, aviso de privacidad, errores 403/404/500.
- Reglas en `models/rules.go`: validación de registro, ingreso y recuperación;
  visibilidad pública de convocatorias.
- Despliegue: `make build` reproducible, `make instalar-vps`, `make deploy`, unidad de
  systemd endurecida y ejemplos de Caddy y nginx.
- Pruebas: 3 paquetes; cobertura 91 % en controladores, 76 % en modelos, 75 % en vistas.

## Puntos por confirmar con la CNMO

Decisiones tomadas provisionalmente en el paso 1 que conviene validar:

1. **Longitud mínima de contraseña: 10 caracteres** (la especificación no fija una).
2. **Identificador de licencia `GPL-3.0-only`** en los encabezados. La política dice
   «GNU GPL v3.0»; si se desea «v3 o posterior» se cambia a `GPL-3.0-or-later`.
3. **Titular de los derechos de autor** para el encabezado de licencia de cada archivo:
   hoy los encabezados no nombran titular.
4. **Texto oficial del aviso de privacidad**: la página actual es una estructura
   provisional sin validez legal.
5. ~~Identidad visual.~~ **Resuelto (13-nov-2026):** se usa la del 48º Foro —paleta,
   cabecera, pie institucional y logotipo— hasta que exista la del 49º
   ([ADR 0008](adr/0008-identidad-visual-del-48-foro.md)).
6. **Número y año de la edición de arranque**: la demostración usa «Edición 49 · 2027»,
   tomada del ejemplo de la especificación.
7. **Vigencia del enlace de activación de cuentas internas: 7 días** (la especificación
   no la fija).
8. **Bloqueo por intentos fallidos:** 1 minuto al quinto fallo, el doble por cada fallo
   adicional y tope de 30 minutos.
9. **El registro avisa cuando el correo ya tiene cuenta.** Es lo más claro para quien
   postuló otro año, a cambio de revelar que ese correo está registrado.
10. **`sesion.id` guarda el resumen del identificador, no un UUID en claro** como dice el
    comentario de §8.3, para que una copia de la base no contenga sesiones utilizables
    (ADR 0004).
11. **Quien postula en una convocatoria no puede ser su jurado ni su revisor.** La
    especificación no lo dice; se añadió como regla de integridad (ADR 0005).
12. **Si a la fecha de cierre falta un revisor, la convocatoria no pasa a revisión
    documental** y el log lo avisa. La alternativa sería avanzar de todos modos; se
    prefirió que la regla de la cuota no tenga excepciones.
13. **Requisitos documentales iniciales** de cada tipo de convocatoria (ADR 0005, §6).
14. ~~Dictaminación a ciegas.~~ **Resuelto (13-nov-2026): la evaluación NO es a ciegas.**
    El jurado ve toda la información de la postulación: nombre, semblanza, partituras y
    demás documentos. Se aplicará en el paso 8.
15. **Forma de los comentarios.** Se conservan con `//` (lo idiomático en Go). Si la CNMO
    quiere los bloques `/* … */` del estilo del kernel, la conversión es mecánica.
16. **El observador (`coordinacion_nacional`) no puede postular**, y quien es jurado o
    revisor de una convocatoria no puede postular en ella, aunque sí en la otra de la
    misma edición (ADR 0007).
17. **Cambiar de solista a ensamble borra los datos generales** ya capturados, previa
    confirmación; se conservan concierto, obras y videos.
18. **Rangos admitidos:** duración de una obra de 1 a 600 minutos; año de fundación del
    ensamble de 1900 al año en curso. La especificación no los fija. La lista de
    integrantes es de longitud libre, con un tope de cordura de 300.
19. ~~Formatos en Intérpretes.~~ **Resuelto (13-nov-2026):** la plataforma distingue sólo
    solista y ensamble. El tabulador por formato es para los pagos y no tiene que ver con
    la convocatoria.
20. **Bases oficiales de las convocatorias.** El repositorio del 48º Foro no las trae;
    las de la plataforma son texto de demostración hasta que se entreguen.
21. **El acuse imprime los países por su código** («MX») hasta que el backend tenga sus
    nombres, y escribe sin diacrítico las letras que la fuente base no tiene («Dvořák» →
    «Dvorák»). El texto exacto queda en el snapshot (ADR 0009, §7).
22. **Una liga de video sólo bloquea el envío si la respuesta es inequívoca** (no existe,
    o es privada). Lo dudoso queda «por comprobar» para el revisor. Y sólo se admiten
    ligas en el puerto 443 (ADR 0009, §8).
23. **Formato del folio:** `FIM49-INT-0137`, consecutivo por convocatoria, como el ejemplo
    de la especificación.
24. **Tope de 30 versiones por documento** y de **200 MB** por archivo (el máximo de cada
    documento lo fija el administrador; por omisión, 30 MB para audio).
25. ~~Formatos de audio.~~ **Resuelto (13-nov-2026):** WAV, FLAC y MP3, con un máximo de
    **30 MB** (ADR 0010, §11).
26. ~~Qué ve el postulante de un descarte.~~ **Resuelto (13-nov-2026):** nada. El descarte
    es un estado interno; al postulante su postulación le sigue apareciendo «en revisión».
    Los demás estados sí los ve (ADR 0010, §8).
27. **El revisor ve las postulaciones enviadas desde que llegan**, pero sólo puede actuar
    en la etapa de revisión documental.
28. **Un dictamen puede corregirse** (de validada a descartada o al revés) mientras la
    convocatoria siga en revisión documental; cada cambio queda en la bitácora.
29. **Plazo máximo de una subsanación: 90 días.** La especificación no lo fija.
30. ~~Tamaños de los demás documentos.~~ **Resuelto (13-nov-2026):** audio, partitura y
    particellas, 30 MB; las particellas, en un solo PDF; autorización (10 MB) y fotografía
    (5 MB), como estaban.
31. ~~Paso a evaluación con postulaciones sin dictaminar.~~ **Resuelto (13-nov-2026):** la
    convocatoria avanza aunque falte alguna por subsanar; la revisión sigue durante la
    evaluación y la postulación, al validarse, se suma a lo que el jurado tiene por
    calificar (ADR 0011, §6).
32. **Una evaluación enviada puede corregirse** mientras dure la etapa de evaluación; al
    cerrarse queda congelada. La especificación dice que «la modifica mientras la etapa
    esté abierta».
33. **El postulante ve «en evaluación» y «evaluada»**, pero no el dictamen final ni
    ninguna calificación.
34. **La excusa de un jurado surte efecto al declararla**, sin aprobación del
    administrador, que la ve en la matriz con su motivo. La especificación dice que «el
    administrador resuelve» (ADR 0011, §7).
35. **La matriz ordena por puntaje comparable** (promedio por jurado, llevado a tres), no
    por la suma, para que una excusa no perjudique. Con tres jurados es la suma.
36. **Al cerrar la evaluación, lo que siga en revisión queda fuera.** El sistema no lo
    impide; la configuración muestra cuántas postulaciones quedan por dictaminar.
37. **Una descarga masiva vive 7 días** y sólo puede bajarla quien la pidió. La
    especificación pide un «enlace temporal» sin fijar el plazo (ADR 0012, §5).
38. **El jurado no descarga en masa**: baja los documentos uno por uno. El postulante
    puede pedir el ZIP de su propia postulación.
39. **Administrador y observador encuentran también los borradores** en el archivo; los
    revisores, no.
40. **La tolerancia a erratas del buscador es por fragmento** («arril» encuentra
    «Carrillo»), no por parecido («Carillo» no la encuentra). Lo segundo exigiría una
    extensión de SQLite, es decir, una dependencia (ADR 0012, §3).
41. **El buscador incluye los correos** de la cuenta y de contacto. Como la evaluación no
    es a ciegas, el jurado también puede buscar por ellos.
42. **El titular puede generar el expediente de su propia postulación** (§2.2), sin nada
    interno. El pie de cada página lleva el nombre de quien lo generó.
43. **El jurado puede pedir el expediente de su convocatoria** —su carpeta de trabajo—,
    con su propia evaluación y ninguna otra (ADR 0013, §1).
44. **Los campos de la evaluación en un reporte piden la granularidad de lo que se
    califica**: por postulación en Intérpretes, por obra en Compositores.
45. **Las claves del catálogo de reportes son las rutas del formulario**
    (`datos.nombre_ensamble`), no las del extracto de §9 (`ensamble.nombre`).
46. **Las plantillas de reporte se comparten** entre administrador y observador; el
    observador puede guardar las suyas y borrar sólo ésas.
47. **Los empates comparten lugar** en la matriz y en el reporte de resultados (1, 2, 2, 4).
48. **El rango de puntaje de los reportes usa el puntaje comparable** (el mismo por el que
    se ordena la matriz).
49. **Las estadísticas cuentan las postulaciones enviadas**, también las descartadas; los
    borradores no.
50. **Las partituras no se anexan al expediente** por ahora: se bajan con la descarga
    masiva (ADR 0013, §3).
51. **De la nómina publicada salen el nombre artístico, la subcategoría, la semblanza y
    los títulos de las obras.** No sale el compositor de cada obra de un programa de
    Intérpretes: la lista blanca de la especificación no lo menciona (ADR 0014, §4).
52. **La semblanza que se publica es la que el postulante capturó** en su postulación. Si
    el portal debe llevar otra, hará falta un campo aparte.
53. **El resultado se registra por postulación**, también en Compositores, donde el jurado
    califica cada obra: una postulación seleccionada publica todas sus obras.
54. **Qué conserva una anonimización**: partitura, particellas y audio (la obra); el año
    de nacimiento, sin día ni mes; género, nacionalidad, país y entidad. Todo documento
    que no sea la obra se borra (ADR 0014, §5).
55. **Una postulación sólo se anonimiza con su proceso terminado**: convocatoria cerrada o
    archivada, o postulación descartada.
56. **La cuenta sólo se anonimiza si se pide** y si no tiene otras postulaciones.
57. **Las instantáneas del espejo se conservan 90 días.** Hasta entonces, las anteriores a
    una anonimización siguen conteniendo los datos borrados.
58. **Umbral de la alerta de disco: 10 GB libres**, configurable.
59. **La nómina se publica desde la API**, no desde la línea de órdenes: el registro exige
    la cuenta del administrador que la autoriza.
