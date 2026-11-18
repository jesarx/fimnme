# Plataforma de Postulaciones FIMNME

Plataforma de recepción, revisión y evaluación de postulaciones del **Foro Internacional
de Música Nueva Manuel Enríquez** (FIMNME), de la Coordinación Nacional de Música y Ópera
(CNMO) del INBAL.

Es un servicio **público y bilingüe** (español / inglés), escrito en **Go sin frameworks**
y distribuido como **binario estático** con el frontal incrustado. Se rige por las
políticas de desarrollo de la CNMO: léalas antes de tocar el código.

**Versión 1.0.0-beta.1** (17 de noviembre de 2026): el desarrollo del plan está completo
y el sistema entra en pruebas con usuarios. Qué trae y qué falta para la entrega:
[notas de versión](docs/NOTAS_DE_VERSION.md).

## Documentación

| Documento | Contenido |
|---|---|
| [`docs/PERFIL_DOMINIO_FIMNME.md`](docs/PERFIL_DOMINIO_FIMNME.md) | Perfil de dominio y políticas institucionales. **Lectura obligatoria.** |
| [`FIMNME-01`](docs/FIMNME-01-definicion-de-requerimientos-y-modelo-de-datos.md) | Requerimientos y modelo de datos. |
| [`FIMNME-02`](docs/FIMNME-02-arquitectura-de-despliegue.md) | Arquitectura de despliegue: topología, servicios, red, secretos, correo, respaldos y espejo. |
| [`DICCIONARIO_DATOS_Y_ACL.md`](docs/DICCIONARIO_DATOS_Y_ACL.md) | Diccionario de datos y matriz de permisos por perfil y operación (02b). |
| [`FIMNME-03`](docs/FIMNME-03-especificacion-de-api-e-interfaz.md) | API e interfaz: convenciones, errores, mapa de pantallas, filtros en la URL, ZIP y ejemplos con `curl`. |
| [`FIMNME-04`](docs/FIMNME-04-manual-de-adopcion.md) | **Manual de Adopción**: instalación, configuración, operación anual y entrega-recepción. |
| [`FIMNME-05`](docs/FIMNME-05-manual-de-validacion.md) | **Manual de Validación**: casos de prueba, resultados, condiciones y acta de firma. |
| [`FIMNME-06`](docs/FIMNME-06-formato-de-intercambio-y-portal-publico.md) | Formato del JSON sanitizado y procedimiento de publicación en el portal (Hugo). |
| [`docs/NOTAS_DE_VERSION.md`](docs/NOTAS_DE_VERSION.md) | Qué contiene cada versión. |
| [`docs/PLAN_DE_DESARROLLO.md`](docs/PLAN_DE_DESARROLLO.md) | Plan por pasos, qué se entregó en cada uno y puntos por confirmar con la CNMO. |
| [`docs/adr/`](docs/adr/) | Registros de decisiones de arquitectura (ADR 0001 a 0032). |
| [`docs/man/`](docs/man/) | Páginas de manual: `fimnme-web(1)` y `fimnme-cli(1)`. |
| `/api/v1/openapi.json` | Descripción de la API (OpenAPI 3.1), servida por el propio sistema. |

## Qué hace

**Para quien postula** (`/postulaciones`)
- Registro con verificación de correo; «Mi cuenta»; historial de todas las ediciones.
- Formulario por pasos con **guardado automático**, que dice en todo momento qué falta.
  Funciona en móvil, en inglés y sin JavaScript.
- Documentos con barra de progreso, según la dotación de cada obra.
- **Envío irreversible** con folio y **acuse en PDF**, también por correo.
- **Subsanación** con desbloqueo selectivo: se corrige sólo lo señalado, y se puede pedir
  que se desbloquee algo más.

**Revisión documental** (`/revision`)
- Bandeja con filtros.
- **Visor** de PDF, audio e imagen.
- Dictamen de cada documento y campo, observaciones, subsanación, desbloqueos y dictamen
  de la postulación.

**Evaluación** (`/evaluacion`)
- Lista de trabajo con avance y ficha con el visor.
- Dos criterios en escala 1–5, en borrador o enviados; excusa por conflicto de interés.
- **Un jurado nunca ve lo de otro**: la restricción es estructural y la vigila una prueba
  que barre todas las rutas.

**Administración** (`/administracion`)
- Ediciones, y la ficha de cada convocatoria:
  - bases bilingües en Markdown, PDF e imagen;
  - calendario y límites;
  - dotaciones, requisitos documentales y criterios;
  - jurados y revisores.
- Cuentas internas.
- **Plantillas de correo** editables con versiones y vista previa.
- Bitácora de auditoría.
- **Importación asistida** en dos fases, que no infiere nada sin una persona.
- **Anonimización ARCO**, la única operación destructiva.
- Avance de etapa automático por calendario, con las cuotas del proceso (un revisor
  como mínimo; tres jurados exactos).

**Supervisión** (`/supervision`)
- Tablero de cada convocatoria (avance de los jurados sin puntajes).
- **Matriz de resultados** con empates.
- Reportes CSV a la medida, con plantillas guardadas.
- Estadísticas de la edición.
- Publicación de la nómina al portal, sólo por acción explícita tras el cierre.

**Archivo** (`/archivo`)
- Todas las ediciones en un solo buscador, tolerante a acentos y erratas, con filtros
  en la URL.
- Vistas guardadas y CSV.
- **Descarga masiva en ZIP** con `manifiesto.csv`.
- **Expedientes en PDF** recortados según quién los pide.
- Enlace permanente por folio.
- Toda descarga queda registrada.

**Transversal**
- **Bilingüe**, con el idioma de la cuenta o del navegador.
- **Tema claro u oscuro** a elección.
- **Fechas dd/mm/aaaa y hora de 24 h** en todos los formularios.
- **Accesible** (WCAG 2.1 AA, auditado).
- **Verificación en dos pasos** para administrador y observador.
- Argon2id, bloqueo por intentos, CSP estricta, protección CSRF y límites por IP.
- Acervo de archivos fuera de la base, direccionado por SHA-256, con versiones.
- Comprobación de ligas de video y recordatorios a borradores.

**Operación** (`fimnme-cli`)
- Migraciones y modo interactivo (`--tui`).
- Reportes programables.
- Campos extendidos sin recompilar.
- Importación.
- Instantáneas firmadas para el espejo y verificación del acervo.
- Exportación al portal.
- Migración verificada a MariaDB.
- Enlaces de activación y restablecimiento del segundo factor.
- Respaldos y prueba de restauración automáticos con systemd.
- Correo propio con DKIM.
- Contenedores (Podman/Quadlet).

> **El correo tiene tres transportes** (`--correo`): `registro` (sólo el log, por
> omisión), `archivo` (cada mensaje completo como `.eml`, para un VPS que no puede
> enviar correo) y `smtp` (al Postfix local o a un relevo externo). Véase «Correo».

## Requisitos

| Dónde | Qué |
|---|---|
| Computadora de desarrollo | **Go 1.26** o posterior, `git` y `make`. Nada más: ni Node, ni npm, ni Docker. |
| VPS | **Debian 13 (trixie)** con acceso `ssh`. No necesita Go ni SQLite instalados: recibe los binarios ya compilados, con SQLite dentro. |

## Desarrollo local

```bash
git clone https://github.com/jesarx/fimnme.git
```

```bash
cd fimnme
```

```bash
make dev
```

Abra <http://127.0.0.1:8080>. Para detener el servidor, `Ctrl+C`.

`make dev` hace tres cosas: aplica las migraciones, carga la edición de demostración si
la base está vacía y arranca el servidor. La base local queda en `./data/fimnme.db`
(fuera de git). Para empezar de cero:

```bash
make reiniciar-datos
```

### Correos en desarrollo

Los correos no se envían: aparecen en la salida del servidor como líneas
`correo NO enviado: transporte de desarrollo`, con su texto completo. Para probar un
registro, copie de ahí el enlace `…/cuenta/verificar?token=…` y ábralo en el navegador.

### Correo

| Situación | Configuración |
|---|---|
| Desarrollo | Nada: `--correo registro` escribe los mensajes en el log. |
| VPS que no puede enviar correo (puerto 25 bloqueado) | `FIMNME_CORREO=archivo`: cada mensaje queda como `.eml` en `<datos>/correo`. |
| VPS con un relevo externo | `FIMNME_CORREO=smtp`, `FIMNME_SMTP_SERVIDOR=smtp.proveedor.com:587`, `FIMNME_SMTP_SEGURIDAD=starttls`, `FIMNME_SMTP_USUARIO=…` y `FIMNME_SMTP_PASSWORD=…` |
| Instalación final, con SMTP propio | `FIMNME_CORREO=smtp` hacia el Postfix local (`127.0.0.1:25`, por omisión), que firma con DKIM y entrega. Se instala como se explica abajo (ADR 0030). |

Con `archivo`, para ver el último correo y copiar su enlace en el VPS (el cuerpo va en
base64, como en cualquier correo; un cliente de correo abre el `.eml` directamente):

```bash
sudo ls -t /var/lib/fimnme/correo | head -1
```

Para leer el texto del correo más reciente, con su enlace:

```bash
sudo python3 -c "import email,sys;m=email.message_from_binary_file(open(sys.argv[1],'rb'));[print(p.get_payload(decode=True).decode()) for p in m.walk() if p.get_content_type()=='text/plain']" "$(sudo ls -t /var/lib/fimnme/correo/*.eml | head -1)"
```

Para una cuenta interna sin activar (un administrador, revisor o jurado recién creado
en el panel), la terminal da un enlace nuevo sin pasar por el correo. Anula los
anteriores:

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme enlace-activacion --email persona@ejemplo.mx --url-base https://su-dominio
```

#### Servidor de correo propio (instalación final)

Sólo en un servidor con el puerto 25 de salida abierto. `make deploy` deja los guiones
en `/opt/fimnme/correo`. En el servidor:

1. Instalar Postfix (sólo de salida, escuchando en 127.0.0.1) y OpenDKIM. El guion
   muestra al terminar los registros DNS por publicar (SPF, DKIM, DMARC), el PTR que se
   pide al proveedor y lo que cambia en `/etc/fimnme/fimnme-web.env`. Con `REBOTES`, los
   avisos de no entrega se reenvían a ese buzón real; sin él, se descartan.

   ```bash
   sudo REBOTES=buzon-real@institucion.mx sh /opt/fimnme/correo/instalar_correo.sh fimnme.ejemplo.mx correo.ejemplo.mx
   ```

2. Publicar los registros y, cuando se propaguen, comprobar todo (servicios, A, PTR,
   SPF, DMARC, que la clave DKIM publicada sea la local y que el puerto 25 salga):

   ```bash
   sudo bash /opt/fimnme/correo/comprobar_correo.sh fimnme.ejemplo.mx correo.ejemplo.mx
   ```

3. Prueba de entrega: desde **Administración › Plantillas de correo › Enviar prueba**,
   con una cuenta de administrador de Gmail y otra de Outlook. En las cabeceras del
   correo recibido («Mostrar original») deben leerse `spf=pass`, `dkim=pass` y
   `dmarc=pass`, y el correo no debe caer en no deseado.

### Crear el primer administrador

Con el servidor detenido o en marcha, en otra terminal:

```bash
go run ./cmd/fimnme-cli crear-usuario --perfil admin_sistema --email admin@ejemplo.mx --nombre Ana --apellidos López
```

La orden imprime un **enlace de activación**: al abrirlo se elige la contraseña. El otro
perfil global disponible es `coordinacion_nacional` (observador).

**Administrador y observador entran con verificación en dos pasos** (ADR 0032).

- **Primer ingreso:** tras la contraseña se pide dar de alta una aplicación
  autenticadora (Google Authenticator, Microsoft Authenticator, Authy, 2FAS, Aegis,
  FreeOTP). Se escanea un QR o se escribe la clave, y se confirma con un código.
- **Códigos de recuperación:** se reciben diez, de un solo uso, para cuando no se
  tenga el teléfono.
- **Siguientes ingresos:** contraseña y el código del momento.
- **Teléfono y códigos perdidos:** se restablece desde el servidor. En el siguiente
  ingreso se da de alta otra vez.

  ```bash
  sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme segundo-factor restablecer --email persona@ejemplo.mx
  ```

Con la sesión abierta, **Administración** (`/administracion`) lleva a las ediciones y a la
ficha de configuración de cada convocatoria; desde ahí se crean las demás cuentas
internas, se asignan jurados y revisores, y se publica la convocatoria. Lo mismo puede
hacerse por la API o con `fimnme-cli --tui`.

### La API

Todas las operaciones de la API, con ejemplos de `curl` para cada módulo, sus
convenciones y códigos de error, están en
[`FIMNME-03`](docs/FIMNME-03-especificacion-de-api-e-interfaz.md). La descripción formal
(OpenAPI 3.1) la sirve el propio sistema en `/api/v1/openapi.json`.

## Estructura del proyecto (MVC estricto)

```
cmd/
  fimnme-web/         Ejecutable: servidor HTTP y trabajadores internos
  fimnme-cli/         Ejecutable: mantenimiento (migrate, crear-usuario, enlace-activacion,
                      segundo-factor, verify, mirror, export-json, export-csv,
                      exportar-mariadb, arco, schema, import, sembrar-demo)
                      y modo interactivo (--tui)
models/             MODELO
  entities.go         entidades de dominio
  rules.go            reglas de negocio (la validación existe una sola vez, aquí)
  blobs.go            almacén de archivos en disco, direccionado por SHA-256
  mirror.go           verificación del acervo e instantáneas firmadas del espejo
  schema_extender.go  extensión dinámica de campos: lista blanca y composición del DDL
  importacion.go      importación asistida: lectura de CSV y XLSX, análisis y reglas
  mariadb.go          migración a MariaDB: traducción del esquema y de los datos
  catalogos.go        catálogos fijos: países y entidades federativas
  persistencia.go     interfaz Persistencia: lo que el sistema le pide a la base
  repository.go       persistencia sobre SQLite: TODO el SQL del sistema
  seguridad.go        Argon2id, secretos de un solo uso, TOTP y códigos de recuperación
  migrations/         migraciones NNNN_nombre.sql, incrustadas en el binario
controllers/        CONTROLADOR
  app.go              rutas, dependencias y sondas /healthz y /readyz
  middleware.go       seguridad (CSP, CSRF), idioma, log
  sesion.go           sesiones, control de acceso y límite de intentos
  paginas.go          páginas públicas
  cuenta.go           registro, ingreso, recuperación, activación, «Mi cuenta»
  segundo_factor.go   verificación en dos pasos: sesión pendiente, alta, código
  fechas.go           fechas de los formularios: dd/mm/aaaa y 24 horas
  api.go              API REST /api/v1: sesión y bitácora
  configuracion.go    API de configuración de ediciones y convocatorias
  postulaciones.go    API de captura de postulaciones
  postulante.go       páginas del postulante: panel y formulario por pasos
  postulante_envio.go páginas del postulante: documentos, envío, estado y subsanación
  revisor.go          páginas del revisor: bandeja y vista de revisión
  jurado.go           páginas del jurado: lista de trabajo y ficha de evaluación
  administracion.go   páginas de administración: ediciones, configuración, cuentas
  supervision.go      páginas de supervisión: tablero, resultados, reportes, nómina
  archivo_paginas.go  páginas del archivo: buscador, vistas, descargas, expedientes
  plantillas_correo.go API y páginas de las plantillas de correo
  operacion.go        páginas de bitácora, importación asistida y anonimización ARCO
  documentos.go       subida y descarga de documentos, envío, acuse y bases
  revision.go         revisión documental, subsanación y desbloqueos
  evaluacion.go       evaluación por jurado y matriz de resultados
  archivo.go          archivo histórico, vistas guardadas y descargas masivas
  expediente.go       expediente en PDF: qué carga para cada quien
  interop.go          portal público, publicación de la nómina y anonimización
  importaciones.go    importación asistida en dos fases
  extendidos.go       campos extendidos: diccionario y captura
  disco.go            alerta de espacio en disco
  reportes.go         reportes CSV, plantillas de reporte y tableros
  enlaces.go          comprobación en red de las ligas de video
  trabajadores.go     correo saliente, limpieza de sesiones, avance de etapa y recordatorios
  correo_transporte.go transportes de correo: archivo .eml y SMTP
views/              VISTA (todo incrustado con //go:embed)
  views.go            motor de plantillas
  i18n.go             traducciones y formato de fechas
  markdown.go         intérprete mínimo de Markdown para las bases
  correo.go           armado de los correos a partir de sus plantillas
  datos.go            respuestas JSON y CSV de la API
  configuracion.go    respuestas JSON de la configuración
  postulaciones.go    respuestas JSON de las postulaciones y su formulario
  acuse.go            acuse de recibo en PDF, compuesto desde el snapshot
  revision.go         respuestas JSON de la revisión y la subsanación
  evaluacion.go       respuestas JSON del jurado y de la matriz
  archivo.go          listado del archivo, su CSV y el manifiesto de los ZIP
  expediente.go       composición del expediente en PDF
  fotografia.go       reducción de la fotografía para el PDF
  reportes.go         CSV de reportes y respuestas de tableros
  mensaje.go          mensaje de correo MIME (texto, HTML y adjuntos)
  qr.go               código QR del alta del segundo factor, como SVG
  api/openapi.json    descripción OpenAPI 3.1 de /api/v1
  templates/          html/template (páginas y parciales)
  static/css/         tema.css (identidad de la edición) y fimnme.css (todo lo demás)
  static/js/          htmx y fimnme.js (mejoras opcionales: todo funciona sin ellos)
  i18n/               es.json, en.json
deploy/             systemd, proxy inverso y guiones de instalación y actualización
  correo/             Postfix y OpenDKIM: instalación y comprobación
  contenedor/         Containerfile, compose.yaml y unidades Quadlet
  carga/              prueba de carga (RNF-01)
  accesibilidad/      auditoría con axe-core (sus dependencias, fuera del repositorio)
internal/
  pdf/                escritor mínimo de PDF, con la biblioteca estándar
  qr/                 codificador QR mínimo, con la biblioteca estándar
docs/               perfil de dominio, documentos 01 a 06, diccionario, plan y ADR
```

Reglas que el código hace cumplir con pruebas:

- **Ninguna cadena visible al usuario está en el código**: todas viven en
  `views/i18n/es.json` y `en.json`, con las mismas claves en ambos.
- **CSP estricta**: las plantillas no llevan `<script>` ni `style=""` en línea.
- **Fechas**: se guardan en UTC y se muestran en hora de la Ciudad de México.
- **SQL sólo en `models/repository.go`**, y los PRAGMAs de SQLite activos en cada
  conexión.
- **Bitácora inmutable**: la base rechaza modificar o borrar asientos de auditoría, y en
  ella no entra ningún dato personal.
- **Ningún GET modifica estado**: todo cambio es un `POST`, protegido contra CSRF.
- **Secretos sólo como resumen**: contraseñas, enlaces de un solo uso y sesiones nunca
  se guardan en claro.
- **Las reglas se aplican en la misma transacción que la escritura**: ninguna puede
  burlarse con dos peticiones simultáneas.

### Base de datos y migraciones

El esquema se cambia **sólo** con migraciones: archivos `models/migrations/NNNN_nombre.sql`
que `fimnme-cli migrate` aplica en orden, cada uno en su propia transacción.

- Para cambiar el esquema, **añada** un archivo con el número siguiente. Nunca edite uno
  ya aplicado: el sistema guarda su SHA-256 y se niega a continuar si cambia.
- `fimnme-web` no migra por su cuenta. Si encuentra migraciones pendientes, no arranca y
  el log indica el comando que falta.
- El SQL se escribe en el subconjunto común a SQLite y MariaDB (véase §8.3 de la
  especificación y el [ADR 0003](docs/adr/0003-persistencia-sqlite-sin-cgo.md)).

## Compilación

```bash
make build
```

Genera `bin/fimnme-web` y `bin/fimnme-cli`: ELF estáticos para Linux/amd64, sin CGO y
**reproducibles** (el mismo commit produce los mismos binarios, bit a bit). Al terminar
imprime sus SHA-256. La primera compilación descarga las dependencias (requiere red).

Go compila de forma cruzada, de modo que el comando funciona igual desde Linux, macOS o
Windows. Si el VPS es ARM:

```bash
make build GOARCH=arm64
```

Sin `make` (por ejemplo en Windows con PowerShell) el equivalente es:

```powershell
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -buildvcs=false -ldflags "-s -w -buildid=" -o bin/fimnme-web ./cmd/fimnme-web
go build -trimpath -buildvcs=false -ldflags "-s -w -buildid=" -o bin/fimnme-cli ./cmd/fimnme-cli
```

## Despliegue en un VPS con Debian 13

El flujo es: **compilar en su computadora → subir los binarios → migrar la base →
reiniciar el servicio**. En el VPS, `fimnme-web` corre como servicio de systemd bajo un
usuario sin privilegios y escucha sólo en `127.0.0.1:8080`; un proxy inverso (Caddy o
nginx) recibe el tráfico de Internet y pone el TLS.

```
Internet ──► Caddy / nginx (TLS, 80 y 443) ──► fimnme-web (127.0.0.1:8080) ──► /var/lib/fimnme/fimnme.db
```

| Ruta en el VPS | Contenido |
|---|---|
| `/opt/fimnme/bin/` | `fimnme-web` y `fimnme-cli` |
| `/etc/fimnme/fimnme-web.env` | Configuración |
| `/var/lib/fimnme/` | **Datos**: la base `fimnme.db` (y sus archivos `-wal` y `-shm`). Pertenece al usuario `fimnme`. |

En los ejemplos, `usuario@mi-vps` es su acceso `ssh` y `dev.fimnme.example` el dominio
del entorno de desarrollo. Si entra al VPS **como root**, añada `SUDO=` a los comandos
`make` (por ejemplo `make deploy VPS=root@mi-vps SUDO=`).

### 1. Preparación inicial del VPS (una sola vez)

Desde su computadora, en la carpeta del proyecto:

```bash
make instalar-vps VPS=usuario@mi-vps
```

Copia y ejecuta [`deploy/instalar_vps.sh`](deploy/instalar_vps.sh), que:

- crea el usuario de sistema `fimnme` (sin contraseña ni shell);
- crea `/opt/fimnme/bin` (binarios), `/etc/fimnme` (configuración) y `/var/lib/fimnme`
  (datos);
- instala la unidad `fimnme-web.service` y la habilita para arrancar con la máquina;
- instala la configuración inicial en `/etc/fimnme/fimnme-web.env`.

El guion es idempotente: repetirlo no daña nada ni pisa la configuración existente.

### 2. Desplegar (cada vez que haya cambios)

```bash
make deploy VPS=usuario@mi-vps
```

Compila los dos binarios, los sube y ejecuta en el VPS
[`deploy/actualizar_vps.sh`](deploy/actualizar_vps.sh), que:

1. instala `fimnme-web`, `fimnme-cli` y la unidad de systemd;
2. detiene el servicio (las peticiones en curso terminan antes);
3. aplica las migraciones pendientes de la base, como el usuario `fimnme`;
4. arranca el servicio y muestra su estado (`active`) y la versión instalada.

Si una migración falla, la base queda como estaba y el servicio **no** se arranca: revise
el mensaje, corrija y vuelva a desplegar.

Compruebe desde el VPS que el servicio y la base responden:

```bash
curl -i http://127.0.0.1:8080/readyz
```

`/readyz` responde `200 ok` si la base contesta y `503` si no. `/healthz` sólo indica que
el proceso está en pie.

**La primera vez**, defina la dirección pública del sitio, con la que se arman los
enlaces de los correos. Edite `/etc/fimnme/fimnme-web.env`, añada la línea con su dominio
y reinicie el servicio:

```
FIMNME_URL_BASE=https://dev.fimnme.example
```

```bash
sudo systemctl restart fimnme-web
```

Después cree al primer administrador. La orden imprime un enlace de activación; ábralo
en el navegador para elegir la contraseña:

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme crear-usuario --perfil admin_sistema --email SU-CORREO --nombre SU-NOMBRE --apellidos SUS-APELLIDOS --url-base https://dev.fimnme.example
```

**Sólo en el entorno de desarrollo**, cargue además la edición de demostración para que
la portada tenga contenido:

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme sembrar-demo
```

No requiere reiniciar el servicio. La orden no hace nada si la base ya tiene ediciones.

> **Ejecute `fimnme-cli` siempre como el usuario `fimnme`**, con `runuser` como en el
> ejemplo. Si se ejecutara como root, SQLite crearía archivos que el servicio no podría
> escribir; por eso la herramienta se niega a trabajar sobre datos de otro usuario.

### 3. Proxy inverso con TLS (una sola vez)

Requisitos previos: el dominio debe apuntar a la IP del VPS (registro DNS `A`) y los
puertos 80 y 443 deben estar abiertos en el cortafuegos.

**Opción recomendada: Caddy**, que obtiene y renueva el certificado por sí solo. En el VPS:

```bash
sudo apt update
```

```bash
sudo apt install -y caddy
```

Copie el contenido de [`deploy/Caddyfile.ejemplo`](deploy/Caddyfile.ejemplo) a
`/etc/caddy/Caddyfile`, cambie `dev.fimnme.example` por su dominio y recargue:

```bash
sudo systemctl reload caddy
```

Abra `https://su-dominio` en el navegador: debe verse la portada con la franja
«Entorno de desarrollo».

**Alternativa: nginx + certbot.** Use [`deploy/nginx.conf.ejemplo`](deploy/nginx.conf.ejemplo).
Las cabeceras `Host`, `X-Forwarded-For` y `X-Forwarded-Proto` de ese ejemplo son
imprescindibles: sin ellas fallan la protección CSRF y la cookie segura.

> **Proteja el entorno de desarrollo.** Un sitio de pruebas no debería quedar abierto al
> público. El ejemplo de Caddy trae comentado un bloque de contraseña (`basicauth`);
> actívelo. Además, en modo `desarrollo` la aplicación pide a los buscadores que no
> indexen el sitio.

### Configuración

Se configura con variables de entorno en `/etc/fimnme/fimnme-web.env` (o con banderas
equivalentes en la línea de comandos). Tras editar el archivo:
`sudo systemctl restart fimnme-web`.

| Variable | Bandera | Por omisión | Significado |
|---|---|---|---|
| `FIMNME_SERVE` | `-s`, `--serve` | `127.0.0.1` | IP en la que escucha. Manténgala en `127.0.0.1`: el servicio va siempre detrás del proxy. |
| `FIMNME_PORT` | `-p`, `--port` | `8080` | Puerto local. Debe coincidir con el del proxy. |
| `FIMNME_ENTORNO` | `--entorno` | `produccion` | `desarrollo` muestra la franja de advertencia y bloquea la indexación. |
| `FIMNME_DATOS` | `--datos` | `data` | Directorio de datos. En el VPS la unidad de systemd lo fija en `/var/lib/fimnme`. |
| `FIMNME_URL_BASE` | `--url-base` | — | Dirección pública del sitio, sin barra final (`https://dev.fimnme.example`). Con ella se arman los enlaces de los correos. En `desarrollo`, si falta, se usa la dirección local y se avisa en el log; en `produccion` el servicio no arranca sin ella. |
| `FIMNME_CORREO_POR_MINUTO` | `--correo-por-minuto` | `30` | Máximo de correos que se envían por minuto. |
| `FIMNME_DISCO_MIN_GB` | `--disco-min-gb` | `10` | Espacio libre mínimo del disco de datos, en GB. Por debajo, el servicio lo registra como error y avisa por correo a los administradores una vez al día. `0` no vigila. |

### Operación diaria

| Tarea | Comando (en el VPS) |
|---|---|
| Estado del servicio | `systemctl status fimnme-web` |
| Ver los logs en vivo (JSON) | `journalctl -u fimnme-web -f` |
| Reiniciar | `sudo systemctl restart fimnme-web` |
| Detener | `sudo systemctl stop fimnme-web` |
| Versión instalada | `/opt/fimnme/bin/fimnme-web --version` |
| Manual de los binarios | `man fimnme-web` · `man fimnme-cli` |
| Aplicar migraciones a mano | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme migrate` |
| Ver los correos (no se envían aún) | `journalctl -u fimnme-web \| grep "correo NO enviado"` |
| Verificar el acervo (rápido) | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme verify --rapido` |
| Verificar el acervo (completo) | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme verify` |
| Exportación para el portal | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme export-json --salida /tmp/convocatoria.json` |
| Administrar la configuración | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme --tui` |
| Plantillas de reporte | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme export-csv --listar` |

> **Con el transporte `registro`, los enlaces de los correos quedan en el log** sólo en
> el entorno `desarrollo`. Son secretos de un solo uso: en `produccion` el log muestra
> destinatario y asunto. En producción use `archivo` (servidor sin puerto 25) o `smtp`;
> véase «Correo».

### Copia de seguridad de los datos

`/var/lib/fimnme` contiene **la base** (`fimnme.db` y sus archivos `-wal` y `-shm`) y **el
acervo de archivos** (`blobs/`): las partituras, el audio, las fotografías y los acuses.
Una cosa sin la otra no sirve: se respaldan siempre juntas.

El directorio `exportaciones/` guarda los ZIP de las descargas masivas y los expedientes
por convocatoria durante siete días.
**No hace falta respaldarlo**: son copias de lo que ya está en `blobs/`.

#### Instantánea firmada (sin detener el servicio)

`fimnme-cli mirror snapshot` copia la base **en caliente**, replica el acervo y firma el
resultado. Es la forma de respaldar y de alimentar el servidor espejo.

Una sola vez, genere la clave de firma (fuera del directorio de datos y del destino):

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli mirror nueva-clave --clave /etc/fimnme/espejo.clave
```

Cada vez:

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme mirror snapshot --destino /var/backups/fimnme --clave /etc/fimnme/espejo.clave
```

En el destino quedan `instantaneas/fimnme-<fecha>.db` con su manifiesto firmado y
`blobs/` con el acervo. Para comprobar una instantánea —en este servidor o en el espejo,
donde basta con el destino y la clave—:

```bash
/opt/fimnme/bin/fimnme-cli mirror verify --destino /var/backups/fimnme --clave /etc/fimnme/espejo.clave
```

Verifica la firma, que la base sea la del manifiesto, su integridad interna y que estén
todos los archivos que registra. Las instantáneas se conservan 90 días
(`--conservar-dias`).

#### Respaldos automatizados

`make deploy` instala dos temporizadores de systemd (ADR 0026):

| Temporizador | Qué hace | Cuándo |
|---|---|---|
| `fimnme-respaldo.timer` | Instantánea firmada de la base y del acervo, verificada, con sumas SHA-256; y, si se configura, copia a otro servidor por rsync sobre SSH | Cada 6 horas |
| `fimnme-restauracion.timer` | **Prueba de restauración**: comprueba las sumas, restaura la última instantánea en un directorio aparte y verifica la base contra el acervo, sin tocar producción | Cada domingo |

El primer despliegue genera la clave de firma en `/etc/fimnme/espejo.clave` (nunca la
reemplaza). La configuración está en `/etc/fimnme/respaldo.env`: destino, días que se
conservan y, opcional, el servidor remoto. Para lanzar uno a mano y ver su resultado:

```bash
sudo systemctl start fimnme-respaldo.service
```

```bash
journalctl -u fimnme-respaldo -n 30
```

```bash
systemctl list-timers 'fimnme-*'
```

Para la copia remota, el usuario `fimnme` necesita una llave SSH sin frase autorizada en
el remoto; póngala en `FIMNME_RESPALDO_REMOTO=usuario@host:/ruta`. La clave de firma se
copia al espejo una sola vez por un canal seguro y **no** viaja con las instantáneas.
Durante la etapa de postulación conviene acortar la cadencia
(`sudo systemctl edit fimnme-respaldo.timer`).

#### Restaurar

Con el servicio detenido, copie la instantánea verificada como `fimnme.db` y el acervo
como `blobs/`, y borre los archivos `-wal` y `-shm` que hubiera:

```bash
sudo systemctl stop fimnme-web
```

```bash
sudo -u fimnme cp /var/backups/fimnme/instantaneas/fimnme-<fecha>.db /var/lib/fimnme/fimnme.db
```

```bash
sudo -u fimnme rm -f /var/lib/fimnme/fimnme.db-wal /var/lib/fimnme/fimnme.db-shm
```

```bash
sudo -u fimnme cp -a /var/backups/fimnme/blobs/. /var/lib/fimnme/blobs/
```

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme verify
```

```bash
sudo systemctl start fimnme-web
```

### Con contenedores (opcional)

La instalación de arriba, con systemd y binarios, es la de referencia. Para quien prefiera
contenedores hay una imagen mínima (ADR 0027) con los mismos binarios:

```bash
make imagen
```

```bash
cp deploy/fimnme-web.env.ejemplo deploy/contenedor/fimnme-web.env
```

```bash
docker compose -f deploy/contenedor/compose.yaml up -d
```

Compose aplica primero las migraciones y después arranca el servidor en 127.0.0.1:8080,
con el proxy con TLS delante como siempre. `fimnme-cli` corre desde la misma imagen:

```bash
docker compose -f deploy/contenedor/compose.yaml run --rm web fimnme-cli crear-usuario --perfil admin_sistema --email admin@ejemplo.mx --nombre Ana --apellidos López
```

Con Podman sin privilegios, `deploy/contenedor/fimnme-web.container` y
`fimnme-datos.volume` son unidades Quadlet: se copian a `~/.config/containers/systemd/`.

### Migración a MariaDB

Muda los datos a una base MariaDB **vacía** y comprueba que cada tabla quedó idéntica
(filas y SHA-256), con las columnas extendidas, las restricciones y los triggers de
protección (ADR 0029). La conexión se toma de `~/.my.cnf` del usuario que lo ejecuta:

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/verify_and_migrate_mariadb.sh fimnme
```

Mudar los datos no hace que la aplicación funcione sobre MariaDB: eso es un paso
posterior (véase el ADR).

### Prueba de carga

`deploy/carga` reproduce el pico de RNF-01 (200 postulantes, 50 subidas simultáneas) contra
una instalación **de prueba** en entorno de desarrollo, en la misma máquina (ADR 0028):

```bash
go run ./deploy/carga --datos /ruta/datos-de-prueba --url http://127.0.0.1:8080 --convocatoria 1
```

Deja cuentas `carga-…@carga.invalid` en esa base: nunca contra producción.

### Añadir un campo sin recompilar

`fimnme-cli schema` añade campos a las tablas que lo admiten (edición, convocatoria,
dotación, postulación, obra, datos generales, propuesta de concierto, integrantes y ligas
de video). Las tablas del proceso —evaluaciones, acuses, descargas, bitácora,
subsanaciones— no se dejan tocar.

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme schema add --tabla datos_solista --nombre como_se_entero --etiqueta-es "¿Cómo te enteraste?" --etiqueta-en "How did you hear about us?" --visible postulante
```

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme schema list
```

- La columna se llama `x_` + el nombre. Tipos: `texto` (por omisión), `entero`, `decimal`.
- Con `--visible postulante` el campo aparece en el formulario y lo llena quien postula;
  sin él, es un dato interno que captura la administración. Además del administrador y
  el observador, pueden verlo `revisor_documental` y `jurado` si se listan.
- Es siempre opcional, no entra en el acuse y no se desbloquea en una subsanación.
- No hace falta reiniciar el servicio. Cada `ALTER TABLE` queda en la bitácora.

`schema drop --tabla T --nombre N` quita el campo **y borra sus valores**; pide escribir
el nombre de la columna para confirmar.

### Importar un catálogo o un jurado

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme import --tipo dotaciones --convocatoria 2 --como admin@ejemplo.mx dotaciones.xlsx
```

La herramienta analiza la hoja, muestra cada conflicto con sus candidatos y pregunta qué
hacer (vincular, crear u omitir). Al final dice lo que va a ocurrir y pide escribir
`IMPORTAR`. Hasta entonces no ha escrito nada; si se interrumpe, se cancela. `--como` es
el correo del administrador que resuelve: queda registrado en cada decisión. Las cuentas
nuevas (`--tipo usuarios_jurado` o `usuarios_revisor`) nacen sin contraseña y sus enlaces
de activación se muestran al terminar.

### Administrar la configuración desde la terminal

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme --tui
```

Abre un diálogo por menús para administrar ediciones, convocatorias (títulos, límites,
bases, calendario y estado), dotaciones, requisitos documentales, criterios de evaluación
y jurados y revisores. Sirve cuando el panel no está a la mano: una instalación nueva, una
sesión por SSH, el servicio detenido.

- **Intro** conserva el valor que se muestra entre corchetes; **`-`** vacía un dato opcional.
- Las fechas se escriben `AAAA-MM-DD HH:MM`, en hora de la Ciudad de México.
- Las bases se cargan de un archivo Markdown: se indica su ruta.
- Aplica **las mismas reglas que el panel** y explica cada rechazo con sus mismos textos:
  no publica sin bases ni calendario, no pasa a evaluación sin tres jurados.
- Cada cambio se guarda al terminar su ficha y queda en la bitácora con el usuario del
  sistema que lo hizo. Puede usarse con el servicio en marcha.
- Una cuenta nueva de jurado o revisor nace sin contraseña; su enlace de activación se
  muestra en la terminal (defina `FIMNME_URL_BASE` para que lleve la dirección pública).

### Generar un reporte desde la terminal

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme export-csv --plantilla "Avance" --edicion 49 --como admin@ejemplo.mx --salida /tmp/avance.csv
```

Genera el mismo CSV que el generador de reportes del panel, desde una plantilla guardada
allí (`export-csv --listar` las muestra). La convocatoria se indica con `--edicion N` (la
del tipo de la plantilla) o con `--convocatoria ID`.

- `--como` es el correo de una cuenta de **administrador o de la coordinación nacional**:
  el reporte se genera con sus permisos y queda en la bitácora a su nombre.
- Sin `--salida` el CSV va a la salida estándar. Con `--salida`, el archivo nace con
  permisos `0600` y nunca queda a medias.
- `--idioma en` pone encabezados y valores en inglés.

Decisiones: [ADR 0016](docs/adr/0016-modo-interactivo-y-export-csv.md).

### Cambiar la imagen de la edición

La plataforma lleva por ahora la identidad del 48º Foro. Para la del 49º —o la de
cualquier edición— se cambian tres archivos y se recompila:

1. `views/static/css/tema.css`: colores (claro y oscuro), tipografía, radios. Su
   encabezado lista los pares de color que deben conservar el contraste AA.
2. `views/templates/parciales/logos.html`: el dibujo `logo_foro` (SVG).
3. `views/static/img/favicon.svg`.

Ningún otro archivo tiene colores: `make verificar` falla si alguien los añade a
`fimnme.css`. Detalles en el [ADR 0017](docs/adr/0017-pantallas-del-postulante-y-tema-visual.md).

### Anonimizar una postulación (solicitud ARCO)

Es **la única operación que borra datos**, y no se puede deshacer. Sólo procede con el
proceso terminado (la convocatoria cerrada o archivada, o la postulación descartada).

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme arco anonimizar FIM49-INT-0137 --fundamento "Solicitud ARCO 2031/014"
```

La herramienta muestra qué va a borrar y pide **dos confirmaciones**: volver a escribir el
folio y después la palabra `ANONIMIZAR`. Con `--con-cuenta` anonimiza también la cuenta
del titular, si no tiene otras postulaciones.

- **Se borran:** nombre, nombre artístico, teléfono, correo de contacto, semblanza, el día
  y el mes de nacimiento, los nombres de los integrantes, las ligas, la fotografía, las
  autorizaciones, los acuses y el texto de los correos que se le enviaron.
- **Se conservan:** la obra (títulos, partituras, particellas y audio), el dictamen, las
  evaluaciones y los datos estadísticos.

En `--fundamento` escriba la referencia de la solicitud, **no datos de la persona**: queda
en la bitácora. Después tome una instantánea nueva para retirar los archivos del espejo.
Las instantáneas y los respaldos anteriores conservan los datos hasta que vencen.

### Volver a una versión anterior

Si entre ambas versiones **no hubo migraciones**, basta con situarse en el commit deseado
y volver a desplegar:

```bash
git checkout <commit-o-etiqueta>
```

```bash
make deploy VPS=usuario@mi-vps
```

Si **sí hubo migraciones**, el binario anterior se negará a arrancar («el binario es
anterior a la base»): las migraciones no se deshacen. En ese caso restaure primero la
copia de seguridad de la base tomada antes del despliegue. Por eso conviene hacer esa
copia antes de desplegar una versión que traiga migraciones.

### Auditoría de accesibilidad

`deploy/accesibilidad/auditar.js` recorre las páginas del postulante con axe-core
(WCAG 2.1 A y AA) contra una instalación **de prueba**. Sus dependencias se instalan
fuera del repositorio; las instrucciones están en su encabezado (ADR 0031; Manual de
Validación, V-ACC-01).

## Dependencias

La biblioteca estándar de Go y las dos dependencias previstas por la política
institucional:

- **`modernc.org/sqlite`**: SQLite 3 en Go puro, sin CGO, con las dependencias indirectas
  que arrastra ([ADR 0003](docs/adr/0003-persistencia-sqlite-sin-cgo.md)).
- **`golang.org/x/crypto`**: Argon2id para las contraseñas
  ([ADR 0004](docs/adr/0004-cuentas-sesiones-y-correo.md)).

Cualquier otra requiere un ADR.

htmx va **incluido en el repositorio** (`views/static/js/htmx.min.js`), no se descarga al
compilar. Su versión, origen y suma de verificación están en
[`docs/adr/0001-htmx-incluido-en-el-repositorio.md`](docs/adr/0001-htmx-incluido-en-el-repositorio.md).

## Licencia

Software libre bajo la **GNU General Public License v3.0**. Véase [`LICENSE`](LICENSE).
