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

## Pasos

### Hecho

| # | Paso | Estado |
|---|---|---|
| 1 | **Cimientos y páginas públicas.** Estructura MVC, servidor `fimnme-web`, frontal incrustado, CSS propio, i18n es/en, cabeceras de seguridad y CSRF, portada, detalle de convocatoria, pantallas de cuenta (sin conectar), README y despliegue en VPS. | ✅ Hecho |

### Backend

| # | Paso | Requerimientos | Estado |
|---|---|---|---|
| 2 | **Persistencia base.** `modernc.org/sqlite` sin CGO; PRAGMAs en cada conexión; una conexión de escritura y grupo de lectura; migrador propio y esquema completo de §8.3 (incluidos los triggers de sólo inserción de `auditoria`); `models/repository.go`; primer `fimnme-cli` con `migrate`; las páginas públicas leen de la base y desaparece `models/demo.go`; `/readyz` comprueba la base. | §7.5, §8.3, RF-CLI-01, RF-AUD-05 | Pendiente |
| 3 | **Cuentas, sesiones y control de acceso.** Registro, verificación de correo, ingreso, recuperación y cambio de datos; Argon2id; sesiones por cookie con expiración por perfil; tokens de un solo uso; bloqueo por intentos y límite de tasa; perfiles globales y asignaciones por convocatoria (ACL); bitácora de auditoría; bandeja `email_outbox` con interfaz `Mailer` y su trabajador. Los formularios de cuenta del paso 1 quedan funcionando. | RF-AUT-01 a 08 y 10, §2, RF-AUD-01 a 03, RF-NOT-01, 02 y 08, §5.8 | Pendiente |
| 4 | **Configuración de ediciones y convocatorias.** Ediciones, convocatorias y bases bilingües; calendario y cambio de etapa (automático por fecha y forzado), con las cuotas de 3 jurados y 1+ revisores; dotaciones, requisitos documentales y criterios; alta de cuentas internas con activación; asignación de jurados y revisores; duplicar la configuración del año anterior. | RF-CONV-01 a 09, §3, RN-14 | Pendiente |
| 5 | **Postulación: captura y reglas.** Una postulación por convocatoria; subcategoría; guardado parcial por campo (autoguardado); datos de solista, ensamble y compositor; integrantes; propuesta de concierto con la regla de sede fuera de CDMX; obras con sus límites; ligas de video y su validación; `ValidarParaEnvio()` con la lista de bloqueadores. | RF-POST-01 a 09, §5.3.1 a 5.3.6, RF-DOC-10, RN-01 a 10 | Pendiente |
| 6 | **Documentos, envío y acuse.** `BlobStore` en disco por SHA-256; MIME por firma, tamaño y deduplicación; versiones sin sobrescritura; descarga con permiso verificado y registrada; envío irreversible en una transacción (folio, snapshot, correo encolado); acuse PDF con escritor propio (< 100 KB); recordatorios de borrador. | RF-DOC-01 a 07, RF-POST-10 a 12, §4.1, §5.10, RF-NOT-03 y 04 | Pendiente |
| 7 | **Revisión documental y subsanación.** Bandeja y toma blanda; dictamen por documento con motivo; observaciones; señalamiento de campos; solicitud de subsanación con plantilla de correo; motor de desbloqueo selectivo; solicitudes de desbloqueo; reenvío con acuse nuevo y *diff*; dictamen validada / descartada; vencimiento sin descarte automático. | RF-REV-01 a 10, §4.2, RF-POST-13 y 14, RN-11 a 13 | Pendiente |
| 8 | **Evaluación por jurado.** `JuradoRepo` y `SupervisionRepo` separados; evaluación por postulación (Intérpretes) o por obra (Compositores); borrador y congelación al cierre; progreso propio; matriz y suma de los 3 jurados para supervisión; dictamen final; **prueba automatizada de aislamiento entre jurados**. | RF-EVA-01 a 11, RF-CONV-10, §8.4, RN-15 a 20 | Pendiente |
| 9 | **Archivo, búsqueda, reportes y descargas.** Archivo histórico de sólo lectura con visibilidad que no caduca; FTS5 (acentos y erratas); filtros en la URL, orden, paginación y vistas guardadas; reportes CSV con plantillas; ZIP asíncrono con manifiesto; expediente PDF recortado por perfil; datos de tableros. | §5.12, §5.7, §5.13, RN-23 a 27 | Pendiente |
| 10 | **`fimnme-cli` y continuidad.** Importación asistida en dos fases; `export-json` y endpoint de interoperabilidad con lista blanca; extensión dinámica de nodos con lista blanca; `verify` del acervo; anonimización ARCO; espejo con `VACUUM INTO` y HMAC-SHA256; TUI. | §5.14 a 5.17, RN-28 a 34 | Pendiente |

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
  frágil de la arquitectura (D-30). Se hará una primera medición al terminar el paso 5,
  cuando exista el autoguardado real, y la prueba formal en el paso 14.
- **Dependencias.** `modernc.org/sqlite` entra en el paso 2 y `golang.org/x/crypto`
  (Argon2id) en el paso 3. Son las dos previstas por la política; no se añade ninguna otra.
- **Despliegue.** Desde el paso 2 se suben dos binarios (`fimnme-web` y `fimnme-cli`) y el
  servicio usa `/var/lib/fimnme` para la base y el acervo. El README se actualiza en cada
  paso.
- **Correo en desarrollo.** Hasta el paso 14 el `Mailer` de desarrollo no envía: deja el
  mensaje en el log y en `email_outbox`, de donde se toma el enlace de verificación.
- **Prioridades S y C.** Los requerimientos «should» y «could» (2FA, antivirus, XLSX,
  excusa del jurado, volúmenes de ZIP, comparativas) no están en los pasos de arriba; se
  deciden al llegar al módulo correspondiente.

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
