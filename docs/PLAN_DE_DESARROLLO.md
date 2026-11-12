# Plan de desarrollo — Plataforma FIMNME

Plan por pasos para construir lo especificado en
[`FIMNME-01`](FIMNME-01-definicion-de-requerimientos-y-modelo-de-datos.md), bajo las
políticas de [`PERFIL_DOMINIO_FIMNME.md`](PERFIL_DOMINIO_FIMNME.md).

**Forma de trabajo.** Un paso a la vez. Cada paso se entrega compilando, con pruebas en
verde (`make verificar`), desplegable en el VPS de desarrollo, y se espera indicación
antes de iniciar el siguiente.

**Por qué el frontal va primero y qué significa aquí.** No hay SPA: el frontal es
`html/template` + htmx dentro del mismo binario. «Frontal primero» significa construir
las pantallas y su interacción sobre **datos de demostración en memoria**
(`models/demo.go`), para validar el flujo con usuarios reales antes de fijar la
persistencia. Las reglas de negocio se escriben desde el principio en `models/rules.go`,
de modo que al llegar la base de datos sólo se sustituye la fuente de datos.

## Pasos

### Frontal

| # | Paso | Estado |
|---|---|---|
| 1 | **Cimientos y páginas públicas.** Estructura MVC, servidor `fimnme-web`, frontal incrustado, sistema de diseño (CSS propio), i18n es/en, cabeceras de seguridad y CSRF, portada, detalle de convocatoria, pantallas de cuenta, README y despliegue en VPS. | ✅ Hecho |
| 2 | **Postulante I: captura.** Panel «Mis postulaciones»; formulario por pasos con indicador de completitud; datos generales (solista, ensamble, compositor); propuesta de concierto con la regla de sede fuera de CDMX; integrantes dinámicos; autoguardado con confirmación «Guardado a las HH:MM». | Pendiente |
| 3 | **Postulante II: obras, documentos y envío.** Obras dinámicas; subida de archivos con barra de progreso; ligas de video; pantalla «Revisión y envío» con bloqueadores; confirmación de envío; acuses; vista de subsanación con desbloqueo selectivo y solicitud de desbloqueo. | Pendiente |
| 4 | **Revisor documental.** Bandeja con filtros reflejados en la URL y toma blanda; vista de revisión; dictamen por documento; señalamiento de campos; solicitud de subsanación; solicitudes de desbloqueo; tablero de avance. | Pendiente |
| 5 | **Jurado.** Listado con progreso propio; ficha de evaluación (por postulación en Intérpretes, por obra en Compositores); visor de PDF y reproductor de audio. | Pendiente |
| 6 | **Administración y supervisión.** Ediciones, convocatorias y etapas; dotaciones, requisitos documentales y criterios; usuarios y asignaciones; plantillas de correo; matriz de resultados; archivo histórico, buscador, filtros y descargas; bitácora; importación asistida; publicación al portal. | Pendiente |

### Backend

| # | Paso | Estado |
|---|---|---|
| 7 | **Persistencia y autenticación.** Migrador y esquema (§8.3); `models/repository.go`; Argon2id, sesiones, tokens y verificación de correo; límite de tasa; auditoría; `fimnme-cli migrate`. Sustituye `models/demo.go`. | Pendiente |
| 8 | **Postulación.** `ValidarParaEnvio()`; autoguardado real; `BlobStore` y documentos (MIME por firma, SHA-256, versiones); validación de ligas; envío con folio y snapshot; acuse PDF propio; `email_outbox` e interfaz `Mailer`. | Pendiente |
| 9 | **Revisión documental y subsanación.** Dictámenes, motor de desbloqueo selectivo, reenvío con *diff*, recordatorios. | Pendiente |
| 10 | **Evaluación.** `JuradoRepo` y `SupervisionRepo` separados; prueba automatizada de aislamiento entre jurados; resultados y dictamen final. | Pendiente |
| 11 | **Archivo, búsqueda y reportes.** FTS5, filtros, vistas guardadas, CSV, ZIP asíncrono con manifiesto, expediente PDF por rol, tableros. | Pendiente |
| 12 | **`fimnme-cli`.** TUI, importación en dos fases, `export-json`, extensión dinámica de nodos, `verify`, ARCO, espejo. | Pendiente |
| 13 | **Operación.** `Containerfile`, `compose.yaml` y Quadlet; respaldos y espejo con prueba de restauración; script de migración a MariaDB; SMTP propio; prueba de carga de escritura concurrente; manuales de Adopción y Validación. | Pendiente |

> La **prueba de carga de escritura concurrente sobre SQLite** (D-30) es condición de
> aceptación. Aunque su infraestructura está en el paso 13, conviene adelantar una
> medición en cuanto exista el autoguardado real (paso 8).

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
5. **Identidad visual**: el diseño usa un acento granate y tipografías del sistema, sin
   logotipos. Si existe un manual de identidad del Foro o del INBAL, se ajusta.
6. **Número y año de la edición de arranque**: la demostración usa «Edición 49 · 2027»,
   tomada del ejemplo de la especificación.
