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
| 2 | **Persistencia base.** SQLite sin CGO con los PRAGMAs en cada conexión; una conexión de escritura y grupo de lectura; migrador propio y esquema completo de §8.3; `models/repository.go`; bitácora de auditoría de sólo inserción; `fimnme-cli` con `migrate` y `sembrar-demo`; las páginas públicas leen de la base; `/readyz` comprueba la base; despliegue con migración. | ✅ Hecho |
| 3 | **Cuentas, sesiones y control de acceso.** Registro, verificación de correo, ingreso, recuperación, activación de cuentas internas y «Mi cuenta»; Argon2id; sesiones con expiración por perfil; enlaces de un solo uso; bloqueo por intentos y límite por IP; perfiles globales y asignaciones por convocatoria; consulta de la bitácora por API; bandeja de correo saliente con interfaz `Mailer` y su trabajador. | ✅ Hecho |
| 4 | **Configuración de ediciones y convocatorias.** API para ediciones, convocatorias con bases bilingües y límites, calendario, dotaciones, requisitos documentales, criterios, cuentas internas y asignaciones; cambio de etapa forzado y automático por calendario, con las cuotas de 3 jurados y 1+ revisores; duplicación de la configuración a otra edición. | ✅ Hecho |

### Backend

| # | Paso | Requerimientos | Estado |
|---|---|---|---|
| 5 | **Postulación: captura y reglas.** Una postulación por convocatoria; subcategoría; guardado parcial por campo (autoguardado); datos de solista, ensamble y compositor; integrantes; propuesta de concierto con la regla de sede fuera de CDMX; obras con sus límites; ligas de video y su validación; `ValidarParaEnvio()` con la lista de bloqueadores. La edición se acepta sólo dentro del plazo de la etapa, aunque la convocatoria no haya avanzado; y quien es jurado o revisor de una convocatoria no puede postular en ella. | RF-POST-01 a 09, §5.3.1 a 5.3.6, RF-DOC-10, RN-01 a 10 | Pendiente |
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
- **Imagen de cabecera y PDF de las bases** (RF-CONV-02) quedaron para el paso 6, que
  trae el almacén de archivos.
- **Prioridades S y C.** Los requerimientos «should» y «could» (2FA, antivirus, XLSX,
  excusa del jurado, volúmenes de ZIP, comparativas) no están en los pasos de arriba; se
  deciden al llegar al módulo correspondiente. En particular, el **2FA (TOTP) para
  administrador y observador** (RF-AUT-09, prioridad S) no se hizo en el paso 3.

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
5. **Identidad visual**: el diseño usa un acento granate y tipografías del sistema, sin
   logotipos. Si existe un manual de identidad del Foro o del INBAL, se ajusta.
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
