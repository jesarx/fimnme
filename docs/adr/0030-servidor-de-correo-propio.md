# ADR 0030 — Servidor de correo propio: Postfix con DKIM

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Sexta entrega del paso 14. Cumple lo que el ADR 0025 dejó para la instalación final:
el binario entrega al Postfix local y éste firma y entrega a Internet. Lo que se añade
son guiones de despliegue (`deploy/correo/`), no código de la aplicación ni dependencias
de Go.

## Decisión

### 1. Dos guiones

- **`instalar_correo.sh DOMINIO [SERVIDOR]`**: instala y configura Postfix y OpenDKIM,
  genera la clave y muestra los registros DNS que faltan y los cambios en
  `fimnme-web.env`. Puede repetirse sin daño.
- **`comprobar_correo.sh DOMINIO [SERVIDOR]`**: revisa lo que los proveedores grandes
  miran antes de aceptar un mensaje y dice qué falla. No envía nada.

`make deploy` los deja en `/opt/fimnme/correo` **sin ejecutarlos**: en un servidor con
el puerto 25 bloqueado, como el de pruebas, no deben correr.

### 2. Postfix sólo de salida

| Decisión | Por qué |
|---|---|
| Escucha sólo en 127.0.0.1 | Únicamente fimnme-web le entrega. No es relevo abierto ni recibe correo de Internet. |
| Sin entrega local (`mydestination` vacío) | El servidor no es buzón de nadie. |
| TLS hacia el destino cuando lo ofrece (`may`) | No se puede exigir TLS a todos los servidores de Internet. |
| Si OpenDKIM no responde, error temporal (`tempfail`) | Un correo sin firma lo rechazaría DMARC y dañaría la reputación del dominio. La bandeja de salida de fimnme-web reintenta (ADR 0025). |
| Colas de dos días, no de cinco | Un enlace de verificación que llega días después ya no sirve. |
| Sólo IPv4 por omisión | La IPv6 también necesitaría PTR y estar en el SPF (`PROTOCOLOS=all` cuando lo tenga). |

### 3. OpenDKIM

- Sólo firma (`Mode s`): el servidor no recibe correo.
- Clave RSA de 2048 bits. El selector lleva el año (`fimnme2026`): para cambiar de clave
  basta con otro selector. Una clave existente nunca se reemplaza.
- **Las cabeceras firmadas se nombran.** La lista de fábrica firmó sólo From, To,
  Subject y Date, lo que dejaba fuera Message-ID, MIME-Version y Content-Type.
  Sin Content-Type firmada, un intermediario podría cambiar la estructura del
  mensaje sin romper la firma. Se ve en la prueba de abajo.
- `From` se firma también por su ausencia (*oversign*): nadie puede añadir un segundo
  remitente.
- Escucha en `127.0.0.1:8891`, no en un socket de archivo: el `smtpd` de Postfix corre
  enjaulado en Debian.

### 4. DNS

El guion da los valores exactos:

- **SPF** con la IP del servidor y `-all`.
- **DKIM**, el registro de la clave pública.
- **DMARC** con `p=quarantine` y alineación estricta (`adkim=s`, `aspf=s`). El
  remitente del sobre y el `From` son el mismo dominio, así que ambos alinean. Pasar a
  `p=reject` cuando los informes estén limpios.
- **PTR** de la IP hacia el nombre del servidor, que se pide al proveedor.

### 5. Rebotes

Cuando un destino rechaza un mensaje, Postfix avisa al remitente del sobre
(`no-responder@DOMINIO`), que no es un buzón.

- Con `REBOTES=buzon@real`, el aviso se reescribe hacia ese buzón real
  (`recipient_canonical_maps`).
- Sin él, se descarta en el acto, en lugar de reintentarse dos días contra un
  servidor que no lo recibe.

Los rebotes que un destino envía *después* de aceptar el mensaje van al MX del
dominio: quedan fuera de este servidor.

## Comprobado

En una máquina con los paquetes de Debian, sin systemd, con los servicios arrancados a
mano y un DNS local (dnsmasq) con los registros que dio el guion:

- La primera ejecución falló: con «No configuration» el paquete no crea `main.cf`.
  Corregido.
- Un correo entregado por el `MailerSMTP` de fimnme-web, el mismo código que usa el
  servicio, queda firmado. Con `d=` del dominio y el selector, la firma se verifica
  (python3-dkim, con la clave publicada), y deja de verificarse si se altera el cuerpo
  o el Content-Type.
- Con OpenDKIM detenido, Postfix responde `451 4.7.1` y el mensaje no sale. Para la
  bandeja de salida es un error que se reintenta.
- Sin `REBOTES`, un aviso al remitente se descarta; con él, se reencamina al buzón real.
- Repetir la instalación conserva la clave. Un dominio o buzón con caracteres fuera de
  su forma se rechaza antes de tocar nada.
- El comprobador:
  - con los registros correctos, da todo por bueno, incluso con el DKIM partido en
    cadenas de 255, como lo publican muchos paneles;
  - detecta un DKIM truncado, dos registros SPF y un nombre sin registro A;
  - sin DNS lo dice, en lugar de reportar todo como «registro faltante».
  - No usa `opendkim-testkey`: éste resuelve por su cuenta desde los servidores raíz y
    falló donde el DNS del sistema respondía bien.

## Lo que queda

- **La prueba de entregabilidad**, que sólo puede hacerse en el servidor final: un
  correo de prueba desde el panel a una cuenta de Gmail y otra de Outlook, revisando
  `spf=pass`, `dkim=pass` y `dmarc=pass` en las cabeceras (README, «Servidor de correo
  propio»).
- **Contenedores** (ADR 0027): dentro del contenedor, 127.0.0.1 es el propio
  contenedor, no el Postfix del anfitrión. Con Quadlet hay que elegir una de dos:
  - `Network=host`;
  - un relevo por el puerto 587 con `starttls` (transporte 2 de `fimnme-web.env`).

  El despliegue con systemd, el que está en uso, no tiene este problema.
