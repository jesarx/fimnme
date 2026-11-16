# ADR 0025 — Transporte de correo: registro, archivo y SMTP

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Primera entrega del paso 14 (§5.9). Sin dependencias: `net/smtp`, `crypto/tls` y
`mime/multipart` son de la biblioteca estándar.

## Contexto

Las primeras pruebas en el VPS se harán en un proveedor que **bloquea el puerto 25**
de salida (el de SMTP entre servidores), así que ahí no puede entregarse correo
directamente. La instalación final sí tendrá servidor de correo propio. El diseño debe
servir a las dos sin cambiar código.

## Decisión

La bandeja de salida (`email_outbox`) y su trabajador no cambian: la transacción de
negocio sigue escribiendo el mensaje y el trabajador lo entrega con reintentos. Cambia
sólo el transporte (`models.Mailer`), que se elige con `--correo`:

| Transporte | Qué hace | Para qué |
|---|---|---|
| `registro` | Escribe el mensaje en el log (el cuerpo, sólo en desarrollo). | Desarrollo. |
| `archivo` | Guarda el mensaje **completo** —cabeceras, texto, HTML y adjuntos— como `.eml` (permisos 0600). | Un servidor que no puede enviar correo: se revisa exactamente lo que se habría enviado y se copian los enlaces. |
| `smtp` | Entrega el mismo mensaje por SMTP: `starttls` (587) o `tls` (465) hacia un relevo, o sin cifrar **sólo** hacia 127.0.0.1. | Un relevo externo, o el Postfix local en la instalación final. |

El mensaje lo compone una sola función (`views.MensajeMIME`): RFC 5322 con MIME,
texto y HTML como alternativas, adjuntos leídos del acervo, asunto codificado y
cabeceras sin saltos de línea (no se pueden inyectar), `Message-ID` propio y
`Auto-Submitted: auto-generated`. La contraseña SMTP sólo se lee de
`FIMNME_SMTP_PASSWORD`, nunca de una bandera visible en la lista de procesos.

## Instalación final (por hacer en el servidor)

El binario no firma ni entrega a Internet: entrega al **Postfix local** en
127.0.0.1:25, que es quien:

- firma con **DKIM** (OpenDKIM, clave de 2048 bits, selector por año);
- entrega a los servidores de destino, con TLS cuando lo ofrecen.

Y en el DNS del dominio de envío:

- **SPF**: `v=spf1 ip4:<IP del servidor> -all`;
- **DKIM**: el registro TXT de la clave pública;
- **DMARC**: `v=DMARC1; p=quarantine; rua=mailto:…`, para empezar, y `p=reject` cuando
  los informes estén limpios;
- **PTR** (DNS inverso) de la IP hacia el nombre del servidor, que lo pide el proveedor
  del VPS.

Separar así la entrega del binario mantiene a la aplicación sin dependencias y deja la
reputación de envío, las colas y los rebotes a un software hecho para eso.

## Lo que queda

- Configuración de Postfix y OpenDKIM en `deploy/` y la prueba de entregabilidad (un
  correo a una cuenta de Gmail y otra de Outlook, revisando SPF, DKIM y DMARC en las
  cabeceras).
- Registro de rebotes: hoy un error de entrega queda en la bandeja de salida con su
  mensaje; los rebotes asíncronos (DSN) llegarán al buzón del remitente.
