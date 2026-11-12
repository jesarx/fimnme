# Plataforma de Postulaciones FIMNME

Plataforma de recepción, revisión y evaluación de postulaciones del **Foro Internacional
de Música Nueva Manuel Enríquez** (FIMNME), de la Coordinación Nacional de Música y Ópera
(CNMO) del INBAL.

Es un servicio **público y bilingüe** (español / inglés), escrito en **Go sin frameworks**
y distribuido como **binario estático** con el frontal incrustado. Se rige por las
políticas de desarrollo de la CNMO: léalas antes de tocar el código.

| Documento | Contenido |
|---|---|
| [`docs/PERFIL_DOMINIO_FIMNME.md`](docs/PERFIL_DOMINIO_FIMNME.md) | Perfil de dominio y políticas institucionales. **Lectura obligatoria.** |
| [`docs/FIMNME-01-definicion-de-requerimientos-y-modelo-de-datos.md`](docs/FIMNME-01-definicion-de-requerimientos-y-modelo-de-datos.md) | Requerimientos y modelo de datos. |
| [`docs/PLAN_DE_DESARROLLO.md`](docs/PLAN_DE_DESARROLLO.md) | Plan por pasos y estado de avance. |
| [`docs/adr/`](docs/adr/) | Registros de decisiones de arquitectura (ADR). |

## Estado actual

**Paso 3 de 14: cuentas, sesiones y control de acceso.** Hoy el sistema tiene:

- **Cuentas:** registro, verificación de correo, ingreso, recuperación de contraseña,
  activación de cuentas internas y «Mi cuenta» (datos, contraseña, correo).
- **Seguridad:** contraseñas con Argon2id, sesiones por cookie con expiración por
  inactividad, bloqueo por intentos fallidos y límite de intentos por IP.
- **Control de acceso:** perfiles globales y asignaciones por convocatoria, comprobados
  en cada petición.
- **Bitácora de auditoría** consultable por la API, y bandeja de correo saliente.
- **Base de datos SQLite** con el esquema completo y migraciones versionadas.
- **Páginas públicas:** portada, convocatorias, aviso de privacidad.

> **Los correos todavía no se envían.** El transporte real (SMTP propio) llega en el
> paso 14. Mientras tanto cada mensaje **se escribe en el log**, de donde se copia el
> enlace de verificación o de recuperación; véase «Correos en desarrollo» más abajo.
>
> **Aún no hay panel de administración** (paso 4). La edición que se ve en desarrollo se
> carga con `fimnme-cli sembrar-demo` y es **de demostración**; el primer administrador
> se crea con `fimnme-cli crear-usuario`.

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

### Crear el primer administrador

Con el servidor detenido o en marcha, en otra terminal:

```bash
go run ./cmd/fimnme-cli crear-usuario --perfil admin_sistema --email admin@ejemplo.mx --nombre Ana --apellidos López
```

La orden imprime un **enlace de activación**: al abrirlo se elige la contraseña. El otro
perfil global disponible es `coordinacion_nacional` (observador).

### Probar la API

La API usa la misma cookie de sesión que las páginas. Con `curl`:

```bash
curl -c cookies.txt -H 'Sec-Fetch-Site: same-origin' -d 'email=admin@ejemplo.mx&password=LA-CONTRASEÑA' http://127.0.0.1:8080/cuenta/ingresar
```

```bash
curl -b cookies.txt http://127.0.0.1:8080/api/v1/sesion
```

```bash
curl -b cookies.txt 'http://127.0.0.1:8080/api/v1/auditoria?accion=sesion.ingreso&tamano=5'
```

| Ruta | Quién | Qué devuelve |
|---|---|---|
| `GET /api/v1/sesion` | Cualquier cuenta | La cuenta en sesión, sus perfiles globales y sus asignaciones por convocatoria |
| `GET /api/v1/auditoria` | `admin_sistema`, `coordinacion_nacional` | La bitácora, paginada. Filtros: `accion`, `entidad`, `entidad_id`, `actor_id`, `desde`, `hasta` (día `AAAA-MM-DD` en hora de México, o instante ISO-8601), `pagina`, `tamano`. Con `formato=csv` descarga el resultado completo |

Los errores de la API son JSON: `{"error": {"codigo": "sin_permiso", "mensaje": "…"}}`.

Las plantillas, la hoja de estilo y las traducciones van **incrustadas en el binario**
(`//go:embed`), así que tras editar cualquiera de ellas hay que reiniciar `make dev`.

Antes de entregar cualquier cambio:

```bash
make verificar
```

Ejecuta el formato (`gofmt`), el análisis estático (`go vet`) y las pruebas. Las pruebas
crean sus propias bases SQLite temporales; no tocan `./data`.

## Estructura del proyecto (MVC estricto)

```
cmd/
  fimnme-web/         Ejecutable: servidor HTTP y trabajadores internos
  fimnme-cli/         Ejecutable: mantenimiento (migrate, crear-usuario, sembrar-demo)
models/             MODELO
  entities.go         entidades de dominio
  rules.go            reglas de negocio (la validación existe una sola vez, aquí)
  repository.go       persistencia: TODO el SQL del sistema
  seguridad.go        Argon2id y secretos de un solo uso
  migrations/         migraciones NNNN_nombre.sql, incrustadas en el binario
controllers/        CONTROLADOR
  app.go              rutas, dependencias y sondas /healthz y /readyz
  middleware.go       seguridad (CSP, CSRF), idioma, log
  sesion.go           sesiones, control de acceso y límite de intentos
  paginas.go          páginas públicas
  cuenta.go           registro, ingreso, recuperación, activación, «Mi cuenta»
  api.go              API REST /api/v1 (JSON)
  trabajadores.go     envío del correo saliente y limpieza de sesiones
views/              VISTA (todo incrustado con //go:embed)
  views.go            motor de plantillas
  i18n.go             traducciones y formato de fechas
  markdown.go         intérprete mínimo de Markdown para las bases
  correo.go           armado de los correos a partir de sus plantillas
  datos.go            respuestas JSON y CSV de la API
  templates/          html/template
  static/             CSS propio, htmx, iconos
  i18n/               es.json, en.json
deploy/             systemd, proxy inverso y guiones de instalación y actualización
docs/               perfil de dominio, requerimientos, plan y ADR
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
| `FIMNME_PORT` | `--port` | `8080` | Puerto local. Debe coincidir con el del proxy. |
| `FIMNME_ENTORNO` | `--entorno` | `produccion` | `desarrollo` muestra la franja de advertencia y bloquea la indexación. |
| `FIMNME_DATOS` | `--datos` | `data` | Directorio de datos. En el VPS la unidad de systemd lo fija en `/var/lib/fimnme`. |
| `FIMNME_URL_BASE` | `--url-base` | — | Dirección pública del sitio, sin barra final (`https://dev.fimnme.example`). Con ella se arman los enlaces de los correos. En `desarrollo`, si falta, se usa la dirección local y se avisa en el log; en `produccion` el servicio no arranca sin ella. |
| `FIMNME_CORREO_POR_MINUTO` | `--correo-por-minuto` | `30` | Máximo de correos que se envían por minuto. |

### Operación diaria

| Tarea | Comando (en el VPS) |
|---|---|
| Estado del servicio | `systemctl status fimnme-web` |
| Ver los logs en vivo (JSON) | `journalctl -u fimnme-web -f` |
| Reiniciar | `sudo systemctl restart fimnme-web` |
| Detener | `sudo systemctl stop fimnme-web` |
| Versión instalada | `/opt/fimnme/bin/fimnme-web --version` |
| Aplicar migraciones a mano | `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme migrate` |
| Ver los correos (no se envían aún) | `journalctl -u fimnme-web \| grep "correo NO enviado"` |

> **Los enlaces de los correos quedan en el log del servicio** mientras el entorno sea
> `desarrollo`. Son secretos de un solo uso: por eso en `produccion` el log sólo muestra
> destinatario y asunto, y las cuentas no podrán verificarse ni recuperarse hasta que
> exista el SMTP (paso 14).

### Copia de seguridad de la base

Los respaldos automatizados y el servidor espejo llegan en los pasos 10 y 14. Mientras
tanto, para copiar la base **sin riesgo de corrupción** detenga el servicio y copie el
directorio completo (el archivo `-wal` puede contener datos aún no integrados):

```bash
sudo systemctl stop fimnme-web
```

```bash
sudo cp -a /var/lib/fimnme /var/lib/fimnme.respaldo-$(date +%Y%m%d)
```

```bash
sudo systemctl start fimnme-web
```

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

### Pendiente en despliegue

El despliegue conteinerizado que exige la política institucional (`Containerfile`,
`compose.yaml`, unidades Quadlet) y los respaldos automatizados corresponden al paso 14
del plan; el servidor espejo, al paso 10. Para el entorno de desarrollo se usa el flujo
de binarios + systemd descrito aquí.

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
