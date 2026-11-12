# Perfil de dominio y políticas de desarrollo
## Plataforma de Postulaciones FIMNME — CNMO / INBAL

> Documento de contexto permanente. Se entrega a cualquier asistente de IA, desarrollador
> o equipo de relevo **antes** de cualquier tarea sobre este repositorio.
> Las secciones 2 y 3 son **políticas institucionales compartidas** con los demás
> desarrollos de la CNMO: no se modifican por conveniencia de este proyecto.

---

### 1. Perfil y dominio del proyecto (FIMNME — CNMO / INBAL)

El proyecto es la **plataforma de recepción, revisión y evaluación de postulaciones** del
Foro Internacional de Música Nueva Manuel Enríquez (FIMNME), dependiente de la
Coordinación Nacional de Música y Ópera (CNMO) del Instituto Nacional de Bellas Artes y
Literatura (INBAL).

A diferencia de los sistemas de gestión interna de la CNMO, esta plataforma es
**de cara al público**: recibe postulaciones de artistas nacionales y extranjeros, y por
tanto opera en Internet abierto, en español e inglés, con material inédito y bajo
derechos de autor.

#### A. Las dos convocatorias y sus reglas de negocio

La convocatoria se publica **una vez al año** (una *edición*) y se divide en dos
convocatorias independientes, cada una con su propio calendario, catálogos, jurado y
revisores.

1. **Convocatoria de Intérpretes** (`INTERPRETES`)
   - El postulante propone un **programa de concierto** completo.
   - Dos subcategorías excluyentes:
     - **Solista** (`SOLISTA`): persona física.
     - **Ensamble** (`ENSAMBLE`): agrupación, con representante y **listado nominal de
       integrantes** (nombre + instrumento), de longitud libre.
   - **Sin límite** de número de obras ni de duración total del programa.
   - Regla de sede: si la entidad federativa del concierto propuesto **no es CDMX**, son
     obligatorios nombre de la sede, domicilio, fecha y hora. Si es CDMX, no se piden.
   - Documentos: **fotografía** (postulación); **partitura** y **autorización del titular
     de la obra** (por cada obra).
   - De 1 a **3 ligas** a videos de ejecución, validadas por el sistema.

2. **Convocatoria de Compositores** (`COMPOSITORES`)
   - El postulante propone obras de autoría propia, **máximo 2** por postulación.
   - Cada obra se adscribe a una **dotación instrumental** tomada de un catálogo que
     **cambia cada año** y se configura desde el sistema, sin recompilar.
   - Documentos: **fotografía** (postulación); **partitura**, **particellas** y
     **archivos de audio** (por obra), cada uno obligatorio u opcional **en función de la
     dotación** elegida. Esta condicionalidad es configurable, no codificada.

**Unidad de evaluación (regla crítica, asimétrica entre convocatorias)**

| Convocatoria | Unidad que el jurado califica |
|---|---|
| Intérpretes | La **postulación** (la propuesta de concierto completa) |
| Compositores | **Cada obra** postulada, por separado |

**Cuotas fijas del proceso**

- **3 jurados** por convocatoria, exactamente. Todos evalúan todas las postulaciones
  validadas.
- **1 o más revisores** por convocatoria. Todos revisan todas las postulaciones; no hay
  reparto. La *toma* de una postulación es informativa, no bloqueante.
- **2 criterios** de evaluación, ambos obligatorios, escala entera 1–5: *puntaje de
  realización* y *puntaje artístico*. Configurables por convocatoria.
- Administradores y observadores ven la **suma de los 3 jurados**. Los jurados **nunca**
  ven las calificaciones ajenas, ni antes ni después del cierre, ni años después en el
  archivo histórico.

#### B. Ciclo de vida y etapas

La **etapa** es propiedad de la convocatoria, no de la edición: las dos convocatorias de
un mismo año pueden llevar calendarios distintos.

```
BORRADOR → PUBLICADA → POSTULACION → REVISION_DOCUMENTAL → EVALUACION → CERRADA → ARCHIVADA
```

1. **Postulación.** Captura gradual, en varias sesiones y días, con autoguardado. El envío
   es **irreversible**: congela la postulación, asigna folio y emite acuse en PDF por
   correo.
2. **Revisión documental.** El revisor dictamina cada documento (válido / inválido, con
   motivo obligatorio) y puede señalar **campos** erróneos, no sólo documentos. Al
   solicitar subsanación, el sistema **desbloquea exactamente los elementos señalados y
   ninguno más** (desbloqueo selectivo). El postulante puede solicitar el desbloqueo de un
   elemento no señalado, con justificación; el revisor concede o niega.
3. **Evaluación.** El jurado califica. La plataforma **no emite ninguna comunicación al
   postulante después de la etapa de revisión documental**: el resultado se comunica fuera
   del sistema.

#### C. Catálogo y gestión del archivo histórico

- **Permanencia.** Archivar una edición es una **etiqueta de estado**, nunca un borrado ni
  una exportación fuera de línea. Toda edición cerrada permanece consultable en línea, con
  la misma interfaz que las convocatorias vivas, con sus documentos descargables.
- **Enlaces permanentes.** Las rutas por folio (`/postulaciones/{folio}`) no cambian entre
  versiones del software. Un folio citado en un acta debe seguir resolviendo en diez años.
- **Buscador y filtros.** Búsqueda de texto completo tolerante a acentos y erratas sobre
  folio, nombres, nombre artístico, ensamble, títulos de obra, compositores y correos.
  Filtros combinables reflejados en la URL, exportables a CSV y guardables con nombre.
- **Descarga de documentos.** Individual y masiva en ZIP, con selección de alcance **y de
  tipo de documento**, nomenclatura predecible y `manifiesto.csv`. Toda descarga se
  registra con usuario, IP y momento: el acervo es material inédito y bajo derechos.
- **Expediente en PDF.** Distinto del acuse. Herramienta de consulta interna, individual o
  por convocatoria, cuyo contenido **se recorta al generarlo** según el rol de quien lo
  solicita, porque el archivo sale del sistema y después no hay forma de controlarlo.
- **Visibilidad que no caduca.** Las restricciones de rol se aplican igual en el histórico:
  jurados y revisores acceden sólo a las convocatorias en que participaron.

#### D. Protección de datos personales

La plataforma trata datos personales de particulares (no de servidores públicos) y queda
sujeta a la **LFPDPPP**: aviso de privacidad versionado, consentimiento registrado con
sello de tiempo, política de retención y procedimiento ARCO.

La **anonimización** de una postulación histórica —borrar datos de contacto conservando
obra, dictamen y estadísticas— es la **única operación destructiva** del sistema. Exige
confirmación doble, queda auditada con el fundamento de la solicitud y no altera conteos
ni resultados agregados.

---

### 2. Metodologías de desarrollo y mantenimiento

#### A. Arquitectura MVC estricta (Modelo–Vista–Controlador)

- **Propósito de entrega-recepción ("Herencia de Código en México"):** diseñado para que
  cualquier equipo técnico o relevo institucional en el ámbito gubernamental mexicano
  pueda mantener, auditar y extender el sistema durante más de una década sin lidiar con
  dependencias rotas ni frameworks complejos o efímeros.
  - `models/`: entidades de dominio (`entities.go`), reglas de negocio (`rules.go`),
    persistencia (`repository.go`) y motor de extensión dinámica (`schema_extender.go`).
  - `controllers/`: orquestación de peticiones, endpoints REST y control de acceso.
  - `views/`: respuestas JSON y frontal web integrado mediante `//go:embed`.
- **Licenciamiento:** publicación estricta de todo el código bajo la licencia
  **GNU GPL v3.0** (`LICENSE`).

**Adscripción de las reglas propias del FIMNME dentro de MVC** (para que el modelo no se
disuelva en los controladores):

| Pieza | Ubicación |
|---|---|
| `ValidarParaEnvio()` — completitud de una postulación | `models/rules.go` |
| Obligatoriedad documental según dotación | `models/rules.go` |
| Regla de sede fuera de CDMX | `models/rules.go` |
| Cuotas 3 jurados / 1+ revisores y transiciones de etapa | `models/rules.go` |
| Motor de desbloqueo selectivo de subsanación | `models/rules.go` |
| Aislamiento entre jurados (consultas acotadas por `jurado_id`) | `models/repository.go` |
| Verificación de permisos por petición | `controllers/` |
| Acuse PDF, expediente PDF, CSV, ZIP | `views/` |

**Regla de oro del modelo:** ninguna consulta que pueda devolver la evaluación de un
jurado distinto al de la sesión existe en `repository.go`. La restricción es estructural,
no una condición dentro de un `if`.

#### B. Motor de control de acceso (ACL híbrido)

Perfiles granulares, documentados en `DICCIONARIO_DATOS_Y_ACL.md`. Para este desarrollo
se instancian **5 perfiles**, de los cuales dos son globales y dos se otorgan **por
convocatoria** (no por sistema), porque un jurado de un año no debe conservar acceso
perpetuo:

1. **`admin_sistema`**: configuración técnica completa, gestión de usuarios, extensiones
   DDL, respaldos y servidores espejo. Configura ediciones, convocatorias, catálogos,
   requisitos documentales, criterios y plantillas de correo. Asigna jurados y revisores.
   **No puede** emitir ni alterar evaluaciones en nombre de un jurado.
2. **`coordinacion_nacional`** *(observador)*: supervisión global de sólo lectura, incluidas
   las calificaciones individuales de cada jurado y los agregados. Sin permiso de edición
   sobre ningún nodo.
3. **`revisor_documental`** *(por convocatoria)*: dictamina documentos, señala campos
   erróneos, emite observaciones, solicita subsanaciones, resuelve solicitudes de
   desbloqueo y dictamina la postulación como validada o descartada. **Sin acceso** a
   evaluaciones del jurado.
4. **`jurado`** *(por convocatoria)*: consulta postulaciones y documentos de su
   convocatoria; captura puntajes y observaciones **propias**. Acceso **nulo** a lo
   emitido por otros jurados y a cualquier agregado, en toda etapa y también en el
   archivo histórico.
5. **`postulante`**: perfil de autorregistro público. Crea y edita **su propia**
   postulación mientras la etapa lo permita; consulta su historial y descarga sus acuses.
   Durante una subsanación sólo puede editar los elementos desbloqueados.

Equivalencias con el catálogo institucional, para mantener compatibilidad entre sistemas
de la CNMO: `admin_sistema` es el mismo perfil; `coordinacion_nacional` conserva su
semántica de supervisión global; `revisor_documental` ocupa el lugar funcional de
`subdireccion_programacion` acotado a este dominio; `jurado` y `postulante` son perfiles
nuevos, propios de un sistema de convocatoria pública, sin equivalente en los sistemas de
gestión interna.

#### C. Importación asistida con interacción humana

En la importación de datos desde hojas de cálculo (`.csv` / `.xlsx`), el flujo se ejecuta
en **2 fases (Analyze → Commit)**. Si un artista, compositor, dotación o entidad no existe
o no tiene correspondencia unívoca en la base de datos, **no se infiere automáticamente**:
el sistema exige intervención y resolución interactiva por parte del usuario.

Aplicaciones en este desarrollo: alta masiva de catálogos de dotaciones de una nueva
edición; alta de cuentas de jurado y revisores; conciliación de postulantes recurrentes
entre ediciones. **La postulación en sí nunca se importa**: siempre la captura el
postulante.

El mismo principio rige toda **resolución de identidad**: el sistema jamás funde ni asume
que dos registros son la misma persona sin confirmación humana explícita.

---

### 3. Estrategias técnicas y de infraestructura

#### A. Lenguaje y compilación (Golang)

- **Go 1.26** compilado **sin CGO** (`CGO_ENABLED=0`) para generar binarios ELF estáticos,
  autocontenidos y reproducibles para **Debian 13 (trixie)**.
- **Sin frameworks.** Ni de backend, ni de frontend, ni ORM. Se distingue entre *framework*
  (impone estructura, secuestra el flujo de control) y *biblioteca pequeña* (hace una cosa,
  se lee completa en una tarde, se reemplaza). Lo primero se prohíbe; lo segundo se admite
  con justificación escrita en un ADR.
- **Separación en dos ejecutables especializados:**
  1. **`fimnme-web`**: servidor HTTP. A diferencia de los sistemas de intranet de la CNMO,
     este servicio es **público**, por lo que escucha por omisión en `127.0.0.1:8080`
     **detrás de un proxy inverso con TLS obligatorio** (nginx o Caddy), con bandera
     `-s` / `--serve <ip>` para asignar interfaces de red específicas. Incluye los
     trabajadores internos: bandeja de correo saliente, generación de acuses y
     expedientes, armado de ZIP, validación de ligas de video y recordatorios programados.
  2. **`fimnme-cli`**: herramienta de mantenimiento, TUI, migraciones, importación
     asistida, exportaciones y anonimización ARCO.
- **Frontal integrado** mediante `//go:embed`: plantillas `html/template`, hojas de estilo
  propias, htmx servido desde el binario, traducciones (`es.json`, `en.json`) y
  migraciones. Cero `node_modules`, cero empaquetador, cero cadena de suministro de npm.
- **Dependencias externas previstas:** `modernc.org/sqlite`, `golang.org/x/crypto`
  (Argon2id) y, en su caso, el conector MariaDB. Cualquier adición requiere ADR.

#### B. Módulo CLI / TUI para extensión de nodos

- Modo interactivo `--tui` en `fimnme-cli` para administrar las fichas institucionales.
- Módulo `schema_extender.go` para añadir y eliminar campos dinámicos y relacionales a los
  nodos (`edicion`, `convocatoria`, `postulacion`, `obra`, `dotacion`, `documento`, etc.)
  mediante DDL dinámico (`ALTER TABLE`) y registro en diccionario de metadatos, **sin
  requerir recompilación del software**.

**Delimitación obligatoria para este desarrollo.** La extensión dinámica convive con un
proceso jurídicamente sensible, por lo que se acota:

- Un campo añadido por CLI **no puede** alterar la obligatoriedad de un documento ni los
  criterios de evaluación: eso vive en las tablas de configuración del dominio
  (`requisito_documental`, `criterio_evaluacion`), que ya son configurables sin DDL.
- **Prohibido** alterar por DDL dinámico las tablas del proceso evaluatorio
  (`evaluacion`, `evaluacion_puntaje`), de acuses (`acuse`), de descargas (`descarga`) y
  de auditoría (`auditoria`). Son material probatorio.
- Los campos añadidos **no se desbloquean** automáticamente durante una subsanación ni
  entran en el acuse PDF ya emitido.
- Todo DDL dinámico queda en `auditoria` y se refleja en el exportador CSV y en el
  diccionario de metadatos.

#### C. Persistencia y alto rendimiento (SQLite 3 → MariaDB)

- **SQLite 3 puro sin CGO** (`modernc.org/sqlite`):
  - Volumen proyectado a diez años: ~250 postulaciones/año; la base de datos propiamente
    dicha se mantiene en el orden de **decenas de MB**, porque los archivos binarios
    (partituras, particellas, audio, fotografías) **no se guardan en la base**: viven en
    disco, direccionados por SHA-256, y la base sólo guarda sus metadatos. El acervo de
    archivos sí crece ~15 GB/año (~150 GB a diez años) y se dimensiona aparte.
  - Modo WAL (`PRAGMA journal_mode = WAL;`) para concurrencia de lecturas y escrituras sin
    bloqueos.
  - Mapeo en memoria de 256 MB (`PRAGMA mmap_size = 268435456;`), permitiendo que el 100 %
    de la base histórica se sirva desde memoria RAM con latencias < 1 ms.
  - Caché B-Tree de 64 MB (`PRAGMA cache_size = -64000;`) y almacenamiento temporal en RAM
    (`PRAGMA temp_store = MEMORY;`).
  - `PRAGMA foreign_keys = ON;` en **cada** conexión (SQLite no lo activa por omisión) y
    `PRAGMA busy_timeout = 5000;` para absorber picos de escritura.
  - Búsqueda de texto completo con **FTS5**, tokenizador `unicode61 remove_diacritics 2`
    para tolerancia a acentos y tabla auxiliar con tokenizador `trigram` para tolerancia a
    erratas en nombres propios.
- **Migración transparente a MariaDB:**
  - Esquemas compatibles y script de migración verificada
    (`verify_and_migrate_mariadb.sh`) con soporte de tipos de datos, llaves foráneas
    (InnoDB) y cotejo `utf8mb4_unicode_ci`, exportando tanto tablas nativas como columnas
    agregadas por CLI.
  - Por ello el SQL se escribe en el **subconjunto común** a ambos motores: sin tipos
    `ENUM` nativos (se usan `TEXT` con `CHECK`), sin arreglos, sin tipos propietarios;
    fechas en **UTC ISO-8601** (`TEXT`), JSON en `TEXT` validado por la aplicación.
  - El acceso a datos pasa **siempre** por `repository.go`: ninguna consulta SQL vive en
    controladores ni en vistas, para que el cambio de motor se haga en un solo lugar.

#### D. Continuidad operativa: servidor espejo y respaldos automatizados

- **Servidor espejo:** mecanismo de sincronización y réplica basado en snapshots en
  caliente (`VACUUM INTO`) y firmado de integridad criptográfica vía **HMAC-SHA256**
  (`mirror.go`).
- **Respaldos:** scripts automatizados (`backup_automated.sh`) con cálculo de sumas de
  verificación SHA256 y servicios/timers de systemd (`deploy/`), instalables tanto en el
  host local como en servidores remotos.
- **Alcance ampliado para este desarrollo:** el respaldo cubre **la base de datos y el
  acervo de archivos**. Una base íntegra sin las partituras es inservible. El respaldo del
  acervo se hace por contenido (SHA-256), con verificación periódica de que cada
  `archivo.sha256` registrado existe físicamente y coincide. Debe existir una **prueba de
  restauración documentada**; la cadencia crítica es durante la etapa de postulación.

#### E. Despliegue e interoperabilidad hacia portal público (Hugo)

- **Despliegue conteinerizado:** empaquetado mínimo con Podman/Docker mediante
  `Containerfile` y orquestación con `compose.yaml`. Podman rootless, con unidades
  Quadlet/systemd.
- **Estrategia para portal público abierto:** para no exponer la base de datos de
  postulaciones ni arriesgar datos sensibles (datos de contacto, documentos inéditos,
  dictámenes y calificaciones):
  - El sistema genera exportaciones JSON **sanitizadas** mediante `fimnme-cli export-json`
    o el endpoint `/api/v1/interop/convocatoria.json`.
  - Los datos exportados quedan listos para consumirse en un generador de sitios estáticos
    (**Hugo**) equipado con motores de búsqueda en el cliente (Pagefind o Lunr),
    desacoplando totalmente la plataforma de postulaciones de la consulta pública
    ciudadana.

**Qué se exporta y qué no (regla dura).** La exportación JSON contiene **únicamente**:
bases de las convocatorias en ambos idiomas, fechas de cada etapa, catálogos de dotaciones
y, una vez cerrada la edición, la nómina de seleccionados con nombre artístico, obra y
semblanza pública. **Nunca** se exportan: datos de contacto, fechas de nacimiento,
documentos ni sus rutas, dictámenes del revisor, calificaciones, observaciones,
postulaciones no seleccionadas ni borradores.

**Separación operativa.** Las bases y el calendario se exportan en vivo durante la
convocatoria; la nómina de seleccionados **sólo después del cierre y por acción explícita
del administrador**, nunca de forma automática al cambiar de etapa.

---

### 4. Notas de compatibilidad con los demás desarrollos de la CNMO

- Mismo lenguaje, versión, bandera de compilación y distribución objetivo.
- Misma arquitectura MVC y mismos nombres de archivo dentro de `models/`.
- Mismo motor de persistencia y misma ruta de migración a MariaDB.
- Mismo mecanismo de espejo, respaldo y despliegue.
- Misma licencia GNU GPL v3.0.
- Mismo patrón de importación en dos fases con resolución humana.
- Misma estrategia de portal público desacoplado vía JSON sanitizado + Hugo.

**Diferencias deliberadas, propias de este dominio** (documentadas aquí para que un
relevo no las lea como desviaciones):

| Diferencia | Motivo |
|---|---|
| Servicio **público** en Internet, no intranet | La convocatoria recibe postulaciones del extranjero |
| TLS obligatorio, HSTS, CSP, CSRF, *rate limiting* | Superficie expuesta |
| Autorregistro de usuarios con verificación de correo | Los postulantes no son personal de la CNMO |
| Interfaz y contenidos **bilingües** (es/en) | Convocatoria internacional |
| Servidor **SMTP propio** con SPF/DKIM/DMARC/PTR | Independencia de la infraestructura de correo del INBAL |
| Acervo de archivos de gran volumen fuera de la base | Partituras y audio |
| Sujeción a la **LFPDPPP** y procedimiento ARCO | Se tratan datos de particulares |
