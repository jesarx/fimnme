# FIMNME-04 · Manual de Adopción

**Plataforma de Postulaciones del Foro Internacional de Música Nueva Manuel Enríquez
(CNMO / INBAL)**

Este manual sirve a quien **recibe** el sistema: instalarlo, configurarlo, operarlo año
con año y entregarlo al siguiente equipo. No supone que el lector participó en el
desarrollo.

Ordena lo que hay que hacer y en qué momento. El detalle técnico de cada orden está en el
`README.md` y en las páginas de manual (`man fimnme-web`, `man fimnme-cli`), que se citan
en vez de copiarse. Las razones de cada decisión están en los ADR (`docs/adr/`).

El manual de casos de prueba y del protocolo de firma es el **FIMNME-05, Manual de
Validación**.

---

## 1. Qué se recibe

| Pieza | Dónde | Para qué |
|---|---|---|
| Código fuente con su historial | Repositorio Git | Todo el sistema, bajo GNU GPL v3.0 (`LICENSE`). |
| Dos binarios estáticos | `make build` → `bin/fimnme-web`, `bin/fimnme-cli` | El servidor y la herramienta de mantenimiento. No necesitan nada instalado en el servidor. |
| Especificación | `docs/FIMNME-01-…` | Qué debe hacer el sistema y por qué. |
| Perfil de dominio y políticas | `docs/PERFIL_DOMINIO_FIMNME.md` | Reglas institucionales, compartidas con los demás sistemas de la CNMO. |
| Plan y bitácora del desarrollo | `docs/PLAN_DE_DESARROLLO.md` | Qué se entregó en cada paso y los **puntos por confirmar con la CNMO**. |
| Decisiones de arquitectura | `docs/adr/0001…` | Una por decisión, con sus alternativas descartadas. |
| Manuales de órdenes | `docs/man/*.1` | Cada orden y opción de los dos binarios. |
| Guiones de despliegue | `deploy/` | Instalación en el VPS, respaldos, restauración, correo, contenedores, migración a MariaDB, carga y accesibilidad. |
| Este manual y el de Validación | `docs/FIMNME-04…`, `docs/FIMNME-05…` | Adopción y aceptación. |

**Dependencias de Go:** sólo `modernc.org/sqlite` (SQLite sin CGO) y
`golang.org/x/crypto` (Argon2id). Agregar cualquier otra exige un ADR.

## 2. Perfiles y responsabilidades

| Perfil | Quién | Qué hace en el sistema | Alcance |
|---|---|---|---|
| Responsable técnico | Personal de sistemas | No es una cuenta: tiene el servidor, sus respaldos, el DNS y las claves. | El servidor. |
| `admin_sistema` | Coordinación del Foro | Configura ediciones, convocatorias, catálogos, requisitos, criterios y correos. Asigna jurados y revisores. Publica la nómina. Atiende solicitudes ARCO. **No** califica en nombre de un jurado. | Global. |
| `coordinacion_nacional` | Supervisión de la CNMO | Ve todo, incluidas las calificaciones de cada jurado, sin poder cambiar nada. | Global, de sólo lectura. |
| `revisor_documental` | Personal del Foro | Dictamina documentos y campos, pide subsanaciones, resuelve desbloqueos y dictamina la postulación como validada o descartada. | Las convocatorias a las que se le asigna. |
| `jurado` | Tres especialistas por convocatoria | Califica, y sólo ve lo suyo. | Las convocatorias a las que se le asigna, también en el archivo histórico. |
| `postulante` | Público | Se registra, captura y envía su postulación, subsana lo que se le señala y descarga sus acuses. | Sus propias postulaciones. |

**Administrador y observador entran con verificación en dos pasos** (contraseña más un
código de una aplicación autenticadora del teléfono). En su primer ingreso el sistema
los guía para darla de alta y les entrega diez códigos de recuperación.

Las cuentas de personal (todas salvo `postulante`) las crea un administrador y nacen
**sin contraseña**. Se envía por correo un enlace de activación, vigente 7 días, con el
que su titular elige la suya. Jurados y revisores **se asignan por convocatoria**: el
jurado de un año no conserva acceso al siguiente.

## 3. Instalación

### 3.1 El servidor

- **Debian 13 (trixie)**, 2 núcleos y 4 GB de RAM bastan para el pico previsto. En la
  prueba de carga, 200 postulantes simultáneos consumieron 320 MB (ADR 0028).
- **Disco:** la base ocupa decenas de MB. El acervo (partituras, audio, fotografías)
  crece ~15 GB al año, unos 150 GB a diez años, y los respaldos ocupan otro tanto.
  Debe haber espacio para varios años más un margen.
- **DNS:** un nombre para el sitio (p. ej. `postula.fimnme.mx`) que apunte al servidor.
  Para el correo propio, además, registros SPF, DKIM y DMARC y el DNS inverso (§3.5).
- **Puertos de entrada:** 80 y 443, para el proxy con TLS, y el de SSH. El servicio
  escucha sólo en `127.0.0.1` y nunca se expone directo.

### 3.2 Primera instalación

Desde la computadora del responsable técnico, con el repositorio y Go 1.26
(README, «Despliegue en un VPS con Debian 13»):

```bash
make instalar-vps VPS=usuario@servidor
```

Crea el usuario de sistema `fimnme`, los directorios y la unidad de systemd. Se hace una
sola vez.

```bash
make deploy VPS=usuario@servidor
```

Compila, sube los binarios, migra la base y reinicia el servicio. Es la misma orden en
cada actualización.

Luego, en el servidor:

1. Editar `/etc/fimnme/fimnme-web.env` (§4): al menos `FIMNME_URL_BASE`,
   `FIMNME_ENTORNO=produccion`, `FIMNME_PORT` y el correo.
2. Configurar el **proxy inverso con TLS**: nginx o Caddy, con los ejemplos
   `deploy/nginx.conf.ejemplo` y `deploy/Caddyfile.ejemplo` (README, «Proxy inverso con
   TLS»). El `server_name` debe ser exactamente el dominio del sitio. Si no coincide,
   nginx responde con otro sitio del mismo servidor.
3. Crear el primer administrador. La orden imprime el enlace de activación, que se
   entrega a su titular por un canal seguro:

   ```bash
   sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme crear-usuario --perfil admin_sistema --email persona@institucion.mx --nombre Nombre --apellidos Apellidos --url-base https://dominio
   ```

   Si el correo aún no sale, el enlace de una cuenta creada desde el panel se obtiene
   en la terminal: `fimnme-cli enlace-activacion --email …`.
   En su primer ingreso, el administrador da de alta la verificación en dos pasos
   (§2).
4. Comprobar que responde: `curl -s https://dominio/readyz`.

### 3.3 Respaldos y restauración (no es opcional)

`make deploy` instala y habilita los temporizadores (ADR 0026):

| Temporizador | Cuándo | Qué hace |
|---|---|---|
| `fimnme-respaldo.timer` | Cada 6 horas | Instantánea **firmada** de la base (HMAC-SHA256) y respaldo del acervo por contenido, con `SHA256SUMS`. Opcionalmente los copia por rsync a otro servidor. |
| `fimnme-restauracion.timer` | Domingos | **Prueba de restauración**: restaura la última instantánea en un directorio aparte y la verifica completa. |

- **Configuración:** `/etc/fimnme/respaldo.env`. Destino remoto, retención de 90 días.
- **Clave de firma:** `/etc/fimnme/espejo.clave`. Se genera una sola vez. **Debe
  guardarse fuera del servidor**: sin ella no pueden verificarse los respaldos.
- **Revisión:** `systemctl list-timers 'fimnme-*'` y
  `journalctl -u fimnme-respaldo -u fimnme-restauracion`.
- **Restaurar:** README, «Restaurar».

### 3.4 Correo

| Situación | Configuración |
|---|---|
| Servidor de pruebas que no puede enviar correo | `FIMNME_CORREO=archivo`. Cada mensaje queda completo en `/var/lib/fimnme/correo` y de ahí se copian los enlaces. |
| Relevo externo por el puerto 587 | `FIMNME_CORREO=smtp` con `FIMNME_SMTP_SERVIDOR`, `…_SEGURIDAD=starttls`, `…_USUARIO` y `FIMNME_SMTP_PASSWORD`. |
| Instalación final, servidor de correo propio | §3.5. |

### 3.5 Servidor de correo propio (instalación final)

Requiere el puerto 25 de salida abierto. Se instala así (ADR 0030; README, «Servidor de
correo propio»):

```bash
sudo REBOTES=buzon-real@institucion.mx sh /opt/fimnme/correo/instalar_correo.sh dominio-del-remitente nombre-del-servidor
```

El guion muestra los registros DNS por publicar (SPF, DKIM, DMARC) y el DNS inverso que
hay que pedir al proveedor. Cuando estén publicados:

```bash
sudo bash /opt/fimnme/correo/comprobar_correo.sh dominio-del-remitente nombre-del-servidor
```

Al final, una **prueba de entrega**: «Enviar prueba» desde *Administración › Plantillas
de correo* hacia una cuenta de Gmail y otra de Outlook. En las cabeceras deben leerse
`spf=pass`, `dkim=pass` y `dmarc=pass`.

La clave DKIM (`/etc/opendkim/keys/…`) es un secreto más: se respalda fuera del
servidor.

### 3.6 Servidor espejo

El espejo recibe por rsync las instantáneas firmadas que deja el respaldo
(`FIMNME_RESPALDO_REMOTO` en `respaldo.env`). Para levantarlo como servidor activo:
restaurar la última instantánea verificada (README, «Restaurar»), desplegar los mismos
binarios y apuntar el DNS. Conviene ensayarlo una vez al año, fuera de la etapa de
postulación.

### 3.7 Con contenedores (alternativa)

`deploy/contenedor/` trae el `Containerfile`, `compose.yaml` y las unidades Quadlet para
Podman sin privilegios (ADR 0027; README, «Con contenedores»). El despliegue de
referencia es el de binarios con systemd. En contenedor, el correo debe ir a un relevo
(587) o con `Network=host` (ADR 0030).

## 4. Configuración

Toda la configuración del servicio está en `/etc/fimnme/fimnme-web.env`, permisos 0640
`root:fimnme`. La tabla completa está en el README, «Configuración», y en
`man fimnme-web`. Tras editarla: `sudo systemctl restart fimnme-web`.

| Variable | Valor en producción |
|---|---|
| `FIMNME_ENTORNO` | `produccion`. En `desarrollo` se muestra una franja de advertencia y se pide a los buscadores no indexar. |
| `FIMNME_URL_BASE` | `https://dominio`, sin barra final. Con ella se arman los enlaces de los correos; sin ella, el servicio no arranca en producción. |
| `FIMNME_PORT` | El que el proxy espera; si el 8080 está ocupado por otro servicio, otro libre. |
| `FIMNME_CORREO`, `FIMNME_REMITENTE`, `FIMNME_SMTP_*` | Según §3.4 y §3.5. |
| `FIMNME_CORREO_POR_MINUTO` | 30. Las ráfagas disparan los filtros de correo no deseado. |
| `FIMNME_DISCO_MIN_GB` | 10. Por debajo, se avisa a los administradores una vez al día. |

**Secretos** (nunca en el repositorio ni en una bandera visible):

- `FIMNME_SMTP_PASSWORD`, si hay relevo;
- `/etc/fimnme/espejo.clave`;
- la clave DKIM;
- el acceso SSH al servidor.

**Identidad visual de la edición:** un solo archivo, `views/static/css/tema.css`, más los
logotipos. Al cambiarla hay que conservar el contraste de cada par de colores que
enumera su encabezado (WCAG AA, 4.5:1). Después, `make deploy` y repetir la auditoría
de accesibilidad (FIMNME-05, V-ACC-01). README, «Cambiar la imagen de la edición».

## 5. Operación anual

Cada año es una **edición** con dos convocatorias independientes, Intérpretes y
Compositores. Cada una lleva su propio calendario, jurado y revisores. Todo se hace
desde el navegador, en `https://dominio/administracion`. Las mismas acciones existen
en la API y, para lo más frecuente, en `fimnme-cli --tui`.

### 5.1 Ciclo de una convocatoria

```
BORRADOR → PUBLICADA → POSTULACION → REVISION_DOCUMENTAL → EVALUACION → CERRADA → ARCHIVADA
```

- **Publicar** y **archivar** son siempre actos del administrador.
- Las tres etapas intermedias **avanzan solas** en las fechas del calendario.
- El administrador puede adelantar una etapa, o retroceder con un motivo que queda en
  la bitácora.
- Para reabrir un plazo se cambia su fecha.
- La ficha de cada convocatoria dice **qué falta** para pasar a la etapa siguiente. Las
  reglas son las mismas para el avance automático y para el manual:

| Para pasar a | Se exige |
|---|---|
| PUBLICADA | Título, bases en español y el calendario completo. |
| POSTULACION | Calendario completo; en Compositores, al menos una dotación activa. |
| REVISION_DOCUMENTAL | **Al menos un revisor** asignado. |
| EVALUACION | **Exactamente tres jurados** y los criterios de evaluación. |

Si en la fecha de cierre falta un revisor o un jurado, la convocatoria **no avanza** y
el tablero lo señala.

### 5.2 Antes de abrir (cuatro a seis semanas antes)

1. **Crear la edición** (*Administración › Ediciones*): número, año y nombre en español
   e inglés.
2. **Crear las dos convocatorias**, o **duplicar** las del año anterior. Duplicar copia
   textos, bases, límites, dotaciones, requisitos y criterios; **no** copia el calendario
   ni las asignaciones, que son de cada año, ni las postulaciones.
3. En la **ficha de cada convocatoria**:
   - títulos y límites (en Compositores, el máximo de obras; el de ligas de video);
   - **bases** en Markdown, en español e inglés, más el PDF de las bases y la imagen de
     cabecera;
   - **calendario** de las tres etapas, en hora de la Ciudad de México;
   - **dotaciones** (Compositores): a mano o por **importación asistida** desde una hoja
     de cálculo (*Administración › Importaciones*). Lo que no coincide exactamente con
     una dotación existente lo resuelve una persona, renglón por renglón;
   - **requisitos documentales**: qué documento es obligatorio u opcional, y para qué
     dotaciones;
   - **criterios** de evaluación: los dos de la especificación, escala entera 1–5.
4. **Revisores y jurados:** crearlos y asignarlos desde la ficha (también en bloque, por
   importación). Cada uno recibe su enlace de activación. Quien postula en una
   convocatoria no puede ser su jurado ni su revisor.
5. **Plantillas de correo** (*Administración › Plantillas de correo*): revisar los textos
   del año en ambos idiomas y enviarse una prueba de cada una.
6. **Publicar** la convocatoria. Desde ese momento aparece en la portada, con sus bases.
7. **Portal público:** exportar el JSON sanitizado para el sitio en Hugo.

   ```bash
   sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme export-json --edicion N --salida convocatoria.json
   ```

   También está en `/api/v1/interop/convocatoria.json`. Sólo lleva bases, fechas y
   catálogos: nunca datos de postulantes.

### 5.3 Etapa de postulación

- La convocatoria se abre sola en la fecha de inicio.
- **Tablero** (*Supervisión › convocatoria*): borradores, envíos por día, documentos. El
  sistema manda **recordatorios** a quien tiene un borrador sin enviar.
- **Respaldos:** es la etapa crítica. Hay que confirmar cada semana que corren y que la
  prueba de restauración del domingo salió bien (§6.1).
- **No hacer mantenimiento** ni actualizaciones durante la etapa, salvo una corrección
  urgente. La disponibilidad comprometida es del 99.5 %.
- **Atención a postulantes:**
  - la contraseña la recuperan solos;
  - para enviar, el correo de la cuenta debe estar confirmado;
  - el envío es **irreversible**: asigna folio y emite el acuse en PDF, también por
    correo.
- El plazo se cierra solo en la fecha de término. Los borradores quedan como vencidos.

### 5.4 Revisión documental

En `/revision`, cada revisor trabaja la bandeja de su convocatoria. Todos ven todas las
postulaciones; «tomar» una sólo avisa a los demás que alguien la atiende.

- Dictamina cada documento (válido o inválido, con motivo) y puede señalar **campos**.
- **Subsanación:** el postulante puede corregir **exactamente** lo señalado y nada más.
  Recibe un correo y vuelve a enviar, con un acuse nuevo.
- **Desbloqueos:** el postulante puede pedir corregir algo no señalado, con
  justificación; el revisor lo concede o lo niega.
- **Dictamen:** validada o descartada. Al postulante no se le muestra un descarte.

Ésta es la **última etapa con comunicación al postulante**. Los resultados se
comunican fuera del sistema.

### 5.5 Evaluación

- Exactamente tres jurados por convocatoria. En `/evaluacion` cada uno califica las
  postulaciones validadas (en Compositores, cada obra por separado) con los dos
  criterios, en borrador o enviada. Puede corregir lo enviado mientras dure la etapa.
- Un jurado con conflicto de interés **se excusa** desde la ficha.
- Un jurado **nunca** ve la calificación de otro, ni la suma, en ninguna etapa ni en el
  archivo.
- El tablero muestra el **avance** de cada jurado, sin puntajes.

### 5.6 Cierre y resultados

1. La evaluación se cierra sola en la fecha de término y las calificaciones quedan
   congeladas.
2. **Matriz de resultados** (*Supervisión › Resultados*): la suma de los tres jurados,
   con los empates a la vista. Desde ahí se registra el **dictamen final** de cada
   postulación.
3. **Reportes CSV** a la medida, desde el generador de reportes. Las plantillas se
   guardan para los años siguientes. Programables con `fimnme-cli export-csv`.
4. **Nómina de seleccionados al portal:** sólo por **acción explícita** del
   administrador, después del cierre, con vista previa (*Supervisión › Nómina*). Nunca
   sale sola.
5. **Comunicar los resultados fuera del sistema.**

### 5.7 Archivo

Archivar la convocatoria (CERRADA → ARCHIVADA) sólo le pone una etiqueta: no borra ni
exporta nada.

- **Consulta:** todo sigue en `/archivo`, con buscador (tolerante a acentos y erratas),
  filtros en la URL, vistas guardadas, CSV, descarga masiva en ZIP con
  `manifiesto.csv` y expedientes en PDF.
- **Registro:** cada descarga queda con usuario, IP y momento.
- **Enlaces permanentes:** `/postulaciones/{folio}` no cambia entre versiones. Lo que
  cita un acta debe seguir resolviendo.
- **Permisos en el histórico:** jurados y revisores ven sólo las convocatorias en que
  participaron.

## 6. Operación continua

### 6.1 Semanal

- `systemctl list-timers 'fimnme-*'`: los respaldos corrieron en las últimas 6 horas y
  la prueba de restauración del domingo terminó bien.
- `journalctl -u fimnme-web --since -7d -p err`: errores del servicio.
- Espacio en disco (`df -h /var/lib/fimnme /var/backups/fimnme`). El servicio avisa por
  correo por debajo del umbral.

### 6.2 Mensual, y antes y después de cada etapa

- **Verificación del acervo:** que cada archivo registrado exista y coincida con su
  SHA-256.

  ```bash
  sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme verify
  ```

  `fimnme-cli` se ejecuta **siempre** como el usuario `fimnme` (README, «Despliegue»).

- **Instantánea del espejo:** `fimnme-cli mirror verify` con el destino y la clave de
  `respaldo.env` (`man fimnme-cli`).
- **Bitácora** (*Administración › Bitácora*): cambios de etapa, asignaciones,
  importaciones, anonimizaciones.

### 6.3 Actualizar el sistema

Fuera de las etapas activas:

1. Respaldo manual (README, «Instantánea firmada»).
2. `make deploy`.
3. Comprobar `/readyz`.

Para volver a la versión anterior, README, «Volver a una versión anterior». Una
migración ya aplicada **nunca se edita**: un cambio de esquema es siempre una migración
nueva.

### 6.4 Tareas ocasionales

| Tarea | Cómo | Referencia |
|---|---|---|
| Teléfono perdido de un administrador u observador | Si conserva sus códigos de recuperación, entra con uno y genera nuevos en «Mi cuenta». Si no: `fimnme-cli segundo-factor restablecer --email …`; en su siguiente ingreso se da de alta otra vez. | `man fimnme-cli`; ADR 0032 |
| Cuenta interna sin activar y sin correo | `fimnme-cli enlace-activacion --email …` imprime un enlace nuevo. | `man fimnme-cli` |
| Solicitud ARCO (anonimizar) | *Administración › ARCO*, con doble confirmación y el fundamento de la solicitud. Es la **única operación destructiva**: borra datos de contacto y conserva obra, dictamen y estadísticas. | README; ADR 0014 |
| Añadir un campo sin recompilar | `fimnme-cli schema add`. Nunca sobre las tablas del proceso evaluatorio, acuses, descargas ni bitácora. | `man fimnme-cli`; ADR 0015 |
| Importar dotaciones o cuentas | *Administración › Importaciones*, en dos fases: análisis y confirmación. Nada entra sin que una persona resuelva cada conflicto. | ADR 0015 |
| Migrar a MariaDB | `verify_and_migrate_mariadb.sh`. Muda los datos con verificación; la aplicación sobre MariaDB es un paso posterior. | README; ADR 0029 |

### 6.5 Problemas frecuentes

| Síntoma | Causa probable | Qué hacer |
|---|---|---|
| El dominio muestra **otro sitio** del servidor | El `server_name` de nginx no coincide con el dominio, o el sitio no está habilitado | `sudo nginx -T \| grep server_name`; corregir, `sudo nginx -t && sudo systemctl reload nginx`. |
| `fimnme-web` no arranca: «address already in use» | Otro servicio usa el puerto | Otro `FIMNME_PORT` y el mismo en el `proxy_pass`. |
| No arranca en producción | Falta `FIMNME_URL_BASE` | Ponerla en `fimnme-web.env`. |
| Los correos no llegan | Transporte `archivo` o `registro`, puerto 25 bloqueado, o DNS de correo incompleto | `journalctl -u fimnme-web \| grep -i correo`; `comprobar_correo.sh`. Los mensajes fallidos se reintentan solos. |
| Los correos caen en no deseado | Falta DKIM, DMARC o PTR, o no coinciden | `comprobar_correo.sh` y las cabeceras `Authentication-Results`. |
| Un administrador no puede entrar: «el código no es correcto» | La hora del teléfono está desajustada, o ya usó ese código | Activar la hora automática del teléfono y esperar al siguiente código; si no, un código de recuperación o el restablecimiento (§6.4). |
| Un postulante no puede enviar | Le falta algo, o no confirmó su correo | La página de envío lista lo que falta; reenviar la verificación desde «Mi cuenta». |
| La convocatoria no avanzó de etapa | Falta un revisor o un jurado (§5.1) | La ficha lo dice; asignar y el avance ocurre solo. |
| «Queda poco espacio» | Crece el acervo o los respaldos | Ampliar el disco; revisar la retención en `respaldo.env`. |
| La prueba de restauración falló | Respaldo incompleto o clave distinta | `journalctl -u fimnme-restauracion`; no esperar al siguiente domingo. |

## 7. Entrega-recepción al siguiente equipo

El sistema está hecho para cambiar de manos. Para que el relevo lo reciba completo:

1. **Acceso al repositorio**, con su historial. Se compila con `make build`.
2. **Secretos**, en sobre cerrado o gestor de contraseñas institucional, **nunca** en el
   repositorio:
   - acceso SSH al servidor y al espejo;
   - `/etc/fimnme/espejo.clave`;
   - la clave DKIM;
   - las credenciales del panel de DNS y del proveedor del servidor.
3. **Al menos dos cuentas `admin_sistema`** activas, de personas distintas, cada una
   con su verificación en dos pasos y sus códigos de recuperación a resguardo.
4. **Prueba de restauración** reciente, con su fecha (§3.3).
5. **Este manual y el de Validación**, con el acta de la última firma (FIMNME-05, anexo).
6. **Puntos abiertos:** la lista «Puntos por confirmar con la CNMO» de
   `docs/PLAN_DE_DESARROLLO.md`.

Quien recibe debería poder hacer, sin ayuda, lo de la sección 3 en un servidor de
pruebas y lo de la sección 5 con la edición de demostración (`make dev`). Si no puede,
el manual está incompleto y debe corregirse.
