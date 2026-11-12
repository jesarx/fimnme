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

**Paso 1 de 13: cimientos y páginas públicas del frontal.** Hoy el sistema sirve:

- Portada con las convocatorias vigentes y detalle de cada convocatoria (bases y calendario).
- Pantallas de cuenta: crear cuenta, ingresar y recuperar contraseña.
- Aviso de privacidad (estructura provisional), páginas de error y selector de idioma.

> **Aún no hay base de datos.** Las convocatorias que se ven son **datos de demostración**
> en memoria (`models/demo.go`), y los formularios de cuenta **validan pero no registran**:
> responden con un aviso explícito. La persistencia llega en el paso 7 del plan.

## Requisitos

| Dónde | Qué |
|---|---|
| Computadora de desarrollo | **Go 1.26** o posterior, `git` y `make`. Nada más: ni Node, ni npm, ni Docker. |
| VPS | **Debian 13 (trixie)** con acceso `ssh`. No necesita Go: recibe el binario ya compilado. |

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

Las plantillas, la hoja de estilo y las traducciones van **incrustadas en el binario**
(`//go:embed`), así que tras editar cualquiera de ellas hay que reiniciar `make dev`.

Antes de entregar cualquier cambio:

```bash
make verificar
```

Ejecuta el formato (`gofmt`), el análisis estático (`go vet`) y las pruebas.

## Estructura del proyecto (MVC estricto)

```
cmd/fimnme-web/     Ejecutable: servidor HTTP
models/             MODELO
  entities.go         entidades de dominio
  rules.go            reglas de negocio (la validación existe una sola vez, aquí)
  demo.go             datos de demostración — PROVISIONAL hasta repository.go
controllers/        CONTROLADOR
  app.go              rutas y dependencias
  middleware.go       seguridad (CSP, CSRF), idioma, log
  paginas.go          páginas públicas
  cuenta.go           registro, ingreso, recuperación
views/              VISTA (todo incrustado con //go:embed)
  views.go            motor de plantillas
  i18n.go             traducciones y formato de fechas
  markdown.go         intérprete mínimo de Markdown para las bases
  templates/          html/template
  static/             CSS propio, htmx, iconos
  i18n/               es.json, en.json
deploy/             systemd, proxy inverso y guion de instalación del VPS
docs/               perfil de dominio, requerimientos, plan y ADR
```

Reglas que el código hace cumplir con pruebas:

- **Ninguna cadena visible al usuario está en el código**: todas viven en
  `views/i18n/es.json` y `en.json`, con las mismas claves en ambos.
- **CSP estricta**: las plantillas no llevan `<script>` ni `style=""` en línea.
- **Fechas**: se guardan en UTC y se muestran en hora de la Ciudad de México.

## Compilación

```bash
make build
```

Genera `bin/fimnme-web`: un ELF estático para Linux/amd64, sin CGO y **reproducible** (el
mismo commit produce el mismo binario, bit a bit). Al terminar imprime su SHA-256.

Go compila de forma cruzada, de modo que el comando funciona igual desde Linux, macOS o
Windows. Si el VPS es ARM:

```bash
make build GOARCH=arm64
```

Sin `make` (por ejemplo en Windows con PowerShell) el equivalente es:

```powershell
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -buildvcs=false -ldflags "-s -w -buildid=" -o bin/fimnme-web ./cmd/fimnme-web
```

## Despliegue en un VPS con Debian 13

El flujo es: **compilar en su computadora → subir el binario → reiniciar el servicio**.
En el VPS, `fimnme-web` corre como servicio de systemd bajo un usuario sin privilegios y
escucha sólo en `127.0.0.1:8080`; un proxy inverso (Caddy o nginx) recibe el tráfico de
Internet y pone el TLS.

```
Internet ──► Caddy / nginx (TLS, puertos 80 y 443) ──► fimnme-web (127.0.0.1:8080)
```

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
- crea `/opt/fimnme/bin` (binarios) y `/etc/fimnme` (configuración);
- instala la unidad `fimnme-web.service` y la habilita para arrancar con la máquina;
- instala la configuración inicial en `/etc/fimnme/fimnme-web.env`.

El guion es idempotente: repetirlo no daña nada ni pisa la configuración existente.

### 2. Subir el binario (cada vez que haya cambios)

```bash
make deploy VPS=usuario@mi-vps
```

Compila, copia el binario a `/opt/fimnme/bin/fimnme-web`, reinicia el servicio y muestra
su estado (`active`) y la versión instalada. El reinicio es ordenado: las peticiones en
curso terminan antes de que el proceso salga.

Compruebe desde el VPS que responde:

```bash
curl -i http://127.0.0.1:8080/healthz
```

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

### Operación diaria

| Tarea | Comando (en el VPS) |
|---|---|
| Estado del servicio | `systemctl status fimnme-web` |
| Ver los logs en vivo (JSON) | `journalctl -u fimnme-web -f` |
| Reiniciar | `sudo systemctl restart fimnme-web` |
| Detener | `sudo systemctl stop fimnme-web` |
| Versión instalada | `/opt/fimnme/bin/fimnme-web --version` |

### Volver a una versión anterior

El binario es un solo archivo y aún no hay base de datos que migrar: basta con situarse
en el commit deseado y volver a desplegar.

```bash
git checkout <commit-o-etiqueta>
```

```bash
make deploy VPS=usuario@mi-vps
```

### Pendiente en despliegue

El despliegue conteinerizado que exige la política institucional (`Containerfile`,
`compose.yaml`, unidades Quadlet), los respaldos y el servidor espejo corresponden al
paso 13 del plan. Para el entorno de desarrollo se usa el flujo de binario + systemd
descrito aquí.

## Dependencias

Sólo la biblioteca estándar de Go. Las únicas dependencias externas previstas son
`modernc.org/sqlite` y `golang.org/x/crypto` (llegan con la persistencia); cualquier otra
requiere un ADR.

htmx va **incluido en el repositorio** (`views/static/js/htmx.min.js`), no se descarga al
compilar. Su versión, origen y suma de verificación están en
[`docs/adr/0001-htmx-incluido-en-el-repositorio.md`](docs/adr/0001-htmx-incluido-en-el-repositorio.md).

## Licencia

Software libre bajo la **GNU General Public License v3.0**. Véase [`LICENSE`](LICENSE).
