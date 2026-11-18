# FIMNME-02 · Arquitectura de despliegue

**Plataforma de Postulaciones FIMNME (CNMO / INBAL)** · Versión 1.0.0-beta.1

Cómo se instala y corre el sistema en producción.

- **Cómo se ejecuta paso a paso:** Manual de Adopción (FIMNME-04) y README.
- **Por qué se decidió así:** ADR 0025 a 0030.

## 1. Topología

```
                         Internet
                            │ 443 (TLS)          25 (salida, correo)
                            ▼                          ▲
┌──────────────────── Servidor Debian 13 ─────────────│──────────────────────┐
│  nginx o Caddy ── TLS, HSTS ─┐                       │                      │
│                              ▼ 127.0.0.1:8080        │                      │
│  fimnme-web (systemd, usuario fimnme)                │                      │
│    ├─ HTTP: páginas, API, archivos                   │                      │
│    └─ trabajadores: correo ───────► Postfix + OpenDKIM (127.0.0.1:25) ──────┘
│                     etapas, ligas de video ───► HTTPS saliente (YouTube…)   │
│                     recordatorios, descargas masivas, alerta de disco       │
│                              │                                              │
│  /var/lib/fimnme ◄───────────┘   fimnme.db (SQLite WAL) · blobs/ ·           │
│                                  exportaciones/ · correo/                   │
│                                                                             │
│  fimnme-cli (a mano, como fimnme): migraciones, mantenimiento, ARCO…        │
│  temporizadores: fimnme-respaldo (6 h) ─► /var/backups/fimnme ─► rsync ─────┼──► espejo
│                  fimnme-restauracion (domingo) ─► restaura y verifica        │
└─────────────────────────────────────────────────────────────────────────────┘
```

- **Una sola máquina.** El volumen previsto (~250 postulaciones al año, 200 usuarios
  simultáneos en el pico) cabe con holgura: la prueba de carga usó 320 MB de memoria y
  dio p95 de 4 a 12 ms en lecturas (ADR 0028).
- **Sin base de datos externa:** SQLite en el mismo disco. La migración a MariaDB está
  preparada y verificada (ADR 0029) para cuando haga falta.

## 2. Procesos

| Proceso | Cómo corre | Qué hace |
|---|---|---|
| `fimnme-web` | Servicio systemd `fimnme-web.service`, usuario sin privilegios `fimnme`, endurecido (`ProtectSystem=strict`, sin capacidades, sólo escribe en `/var/lib/fimnme`). | Servidor HTTP y trabajadores internos. Si hay migraciones pendientes no arranca y lo dice en el log. |
| `fimnme-cli` | A mano: `sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme …`. | Migraciones, cuentas, verificación del acervo, espejo, exportaciones, ARCO, esquema, importación, modo interactivo. |
| Respaldo | `fimnme-respaldo.timer` → `.service`, cada 6 h (00:20, 06:20, 12:20, 18:20). | Instantánea firmada de la base, acervo por contenido, `SHA256SUMS`, réplica opcional. |
| Prueba de restauración | `fimnme-restauracion.timer`, domingos 04:40. | Restaura la última instantánea aparte y la verifica completa. |
| Proxy | nginx o Caddy, del sistema. | TLS, HSTS, cabeceras `X-Forwarded-*`; el servicio confía en ellas sólo desde 127.0.0.1. |
| Correo | Postfix + OpenDKIM, sólo en la instalación final (ADR 0030). | Firma DKIM y entrega; escucha sólo en 127.0.0.1. |

## 3. Red

| Dirección | Puerto | Para qué |
|---|---|---|
| Entrada | 443 (y 80, que redirige) | El sitio, por el proxy. |
| Entrada | SSH | Administración. |
| Local | 127.0.0.1:8080 (configurable) | Proxy → `fimnme-web`. Nunca se expone. |
| Local | 127.0.0.1:25 | `fimnme-web` → Postfix. |
| Local | 127.0.0.1:8891 | Postfix → OpenDKIM. |
| Salida | 25 | Postfix → servidores de correo de destino. |
| Salida | 443 | Comprobación de las ligas de video. |
| Salida | 22 | rsync del respaldo al espejo, si se configura. |

## 4. Almacenamiento

| Ruta | Contenido | Crece |
|---|---|---|
| `/opt/fimnme/bin` | Los dos binarios y los guiones de respaldo, restauración y MariaDB. | No |
| `/opt/fimnme/correo` | Guiones de Postfix y OpenDKIM. | No |
| `/etc/fimnme` | Configuración y secretos (§5). | No |
| `/var/lib/fimnme/fimnme.db` (+ `-wal`, `-shm`) | La base. | Decenas de MB a diez años |
| `/var/lib/fimnme/blobs/` | Acervo: un archivo por SHA-256, con versiones; nunca se sobrescribe. | ~15 GB al año |
| `/var/lib/fimnme/exportaciones/` | ZIP y expedientes armados; se borran a los 7 días. | Acotado |
| `/var/lib/fimnme/correo/` | Correos guardados como `.eml` (transporte `archivo`). | Sólo en pruebas |
| `/var/backups/fimnme` | Instantáneas firmadas, 90 días, y el acervo respaldado por contenido. | Como el acervo, más 90 días de base |

**Dimensionamiento:** el disco de datos y el de respaldos deben tener al menos 150 GB
para diez años, más margen. Por debajo de 10 GB libres (`FIMNME_DISCO_MIN_GB`), el
sistema avisa por correo a los administradores una vez al día.

## 5. Configuración y secretos

| Archivo | Permisos | Contenido |
|---|---|---|
| `/etc/fimnme/fimnme-web.env` | 0640 `root:fimnme` | Configuración del servicio (`man fimnme-web`). La única variable secreta es `FIMNME_SMTP_PASSWORD`, si se usa un relevo. |
| `/etc/fimnme/respaldo.env` | 0640 `root:fimnme` | Destino, retención y espejo de los respaldos. |
| `/etc/fimnme/espejo.clave` | 0400 `fimnme` | Clave HMAC de las instantáneas. **Se guarda también fuera del servidor**: sin ella no se verifican los respaldos. |
| `/etc/opendkim/keys/…` | 0600 `opendkim` | Clave privada DKIM. También se resguarda fuera. |

**Ningún secreto vive en el repositorio**, en una bandera visible en la lista de
procesos ni en la base. Las contraseñas y los enlaces se guardan sólo como resumen.
La clave TOTP sí está en la base; véase ADR 0032.

## 6. Ciclo de cambios

1. **Compilar** en la máquina del desarrollador: `make build`. Son binarios estáticos,
   sin CGO y reproducibles bit a bit (`-trimpath`, `-buildid=`).
2. **Verificar:** `make verificar` (formato, `go vet`, todas las pruebas). No hay un
   servicio de integración continua: esta orden es la puerta.
3. **Desplegar:** `make deploy VPS=…` sube los binarios, detiene el servicio, migra la
   base y lo arranca.
4. **Volver atrás:** restaurar la instantánea previa y desplegar la versión anterior
   (README, «Volver a una versión anterior»). Las migraciones no se deshacen.

La versión la toma el binario de la etiqueta de git (`fimnme-cli version`).

## 7. Continuidad

- **Respaldos:**
  - cada 6 horas, base y acervo, con SHA-256 y firma HMAC;
  - prueba de restauración automática cada semana;
  - retención de 90 días (ADR 0026).
- **Espejo:** recibe por rsync las instantáneas firmadas. Para activarlo:
  1. restaurar la última instantánea verificada;
  2. desplegar los mismos binarios;
  3. apuntar el DNS.
- **Verificación del acervo:** `fimnme-cli verify` comprueba que cada archivo registrado
  exista y coincida con su suma.
- **Disponibilidad comprometida:** 99.5 % en la etapa de postulación. El mantenimiento
  se hace fuera de las etapas activas.

## 8. Alternativa en contenedores

`deploy/contenedor/` trae una imagen mínima sin distribución (`distroless/static`,
usuario sin privilegios), `compose.yaml` y unidades Quadlet para Podman sin root
(ADR 0027):

- La base y el acervo van en un volumen.
- El sistema de archivos del contenedor es de sólo lectura.
- Las migraciones son un paso aparte.
- El correo debe ir a un relevo por 587, o el contenedor usar `Network=host` para
  alcanzar el Postfix del anfitrión (ADR 0030).

El despliegue de referencia es el de binarios con systemd.
