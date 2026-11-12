# ADR 0004 — Cuentas, sesiones, control de acceso y correo saliente

- **Estado:** aceptada
- **Fecha:** 2026-11-12

Decisiones del paso 3 que la especificación no fijaba. La única dependencia nueva es
`golang.org/x/crypto` (Argon2id), prevista por la política institucional.

## 1. Contraseñas: Argon2id con los parámetros dentro del resumen

**Decisión.** Argon2id con 19 MiB de memoria, 2 iteraciones y 1 hilo (el mínimo que
recomienda OWASP), sal aleatoria de 16 bytes, en formato PHC:
`$argon2id$v=19$m=19456,t=2,p=1$<sal>$<clave>`.

- Los parámetros viajan en cada resumen: subirlos más adelante no invalida las
  contraseñas ya guardadas.
- Se calculan **como máximo 4 resúmenes a la vez**. Cada uno ocupa 19 MiB; sin ese tope,
  una ráfaga de inicios de sesión agotaría la memoria de un VPS pequeño.
- Queda pendiente recalcular el resumen al ingresar cuando los parámetros cambien. No
  hace falta hasta que se decida subirlos.

## 2. Secretos de un solo uso y sesiones: en la base sólo hay resúmenes

**Decisión.** Los enlaces de un solo uso (verificación, recuperación, activación) y los
identificadores de sesión son 256 bits aleatorios. El valor en claro sólo viaja al
usuario, en un correo o en una cookie; en la base se guarda su SHA-256.

**Desviación consciente respecto de §8.3.** El comentario de la tabla `sesion` dice
«UUID v4 en texto». La columna y su tipo no cambian, pero lo que se guarda en `sesion.id`
es el resumen del identificador, igual que ya hace la tabla `token`. **Motivo:** los
respaldos y los snapshots del espejo copian la base a otras máquinas; con identificadores
en claro, quien obtuviera una copia tendría sesiones de 30 días listas para usar.

Por la misma razón, el **cuerpo de un correo con enlace de un solo uso se retira de
`email_outbox` en cuanto se envía**. Se conservan destinatario, plantilla, estado y
Message-ID, que es lo que pide §5.8.

## 3. La bitácora no guarda datos personales

**Decisión.** En `auditoria.antes` y `auditoria.despues` nunca se escriben correos,
nombres ni otros datos de contacto: sólo identificadores, perfiles y los **nombres** de
los campos que cambiaron.

**Motivo.** La bitácora es de sólo inserción y la base rechaza modificarla. Un dato
personal que entrara ahí no podría borrarse ni ante una solicitud ARCO, y la
anonimización (RF-ARCH-07) sería imposible de cumplir. Una prueba recorre la bitácora
completa y falla si encuentra un correo.

## 4. Los inicios de sesión se cuentan en la bitácora

**Decisión.** Cada intento de ingreso —correcto, fallido o rechazado por bloqueo— es un
asiento de auditoría con IP y agente (RF-AUT-08). El bloqueo temporal se calcula
contando los fallos consecutivos **en esa misma bitácora**; no hay un contador aparte que
pueda desincronizarse.

- Bloqueo: 1 minuto al quinto fallo, y el doble por cada fallo adicional, **con tope de
  30 minutos**. El tope existe para que nadie pueda dejar fuera indefinidamente a otra
  persona equivocándose a propósito con su correo.
- Los intentos hechos durante un bloqueo se asientan aparte y no lo alargan.
- El contador se reinicia con un ingreso correcto, un restablecimiento o la activación.

## 5. Qué se revela y qué no sobre la existencia de una cuenta

| Flujo | Respuesta | Motivo |
|---|---|---|
| Ingreso | Mismo mensaje y mismo trabajo de cómputo si el correo no existe, la contraseña es incorrecta o la cuenta está desactivada | No delatar correos registrados, ni por el texto ni por el tiempo de respuesta |
| Recuperación | Idéntica exista o no la cuenta | Ídem |
| Registro | **Sí** dice «ya existe una cuenta con este correo» | Quien postuló el año anterior suele no recordar que ya tiene cuenta; el límite de intentos por IP acota el sondeo |
| Bloqueo | El mensaje de «demasiados intentos» sólo aparece en cuentas reales | Se prefiere que el titular entienda por qué su contraseña correcta no entra |

## 6. Sesiones

**Decisión.** Cookie `fimnme_sesion`: `HttpOnly`, `SameSite=Lax` y `Secure` cuando la
petición llegó por HTTPS. Expiración **por inactividad**: 8 horas si la cuenta tiene
cualquier perfil interno (administración, coordinación, jurado o revisor) y 30 días si
sólo es postulante. Una sesión en uso se prolonga como mucho cada 10 minutos, para no
escribir en la base en cada clic.

- Cambiar la contraseña cierra las demás sesiones; restablecerla por correo las cierra
  todas.
- Desactivar una cuenta o retirar una asignación surte efecto en la siguiente petición:
  los perfiles se leen de la base en cada una, no se guardan en la sesión.

## 7. Ningún GET modifica estado, tampoco los enlaces de los correos

**Decisión.** El enlace de verificación lleva a una página con un botón; la verificación
ocurre al pulsarlo (`POST`). **Motivo:** los antivirus y algunos clientes de correo
«visitan» los enlaces de los mensajes, y gastarían el enlace de un solo uso antes de que
la persona lo abriera.

## 8. Los enlaces se arman con una dirección configurada

**Decisión.** La dirección pública del sitio se toma de `FIMNME_URL_BASE`, nunca de la
cabecera `Host` de la petición, que el visitante controla: de lo contrario podría hacer
que la plataforma enviara a otra persona un enlace de recuperación apuntando a un sitio
suyo. En producción el servidor no arranca sin esa variable.

## 9. Límite de intentos por IP

**Decisión.** 30 envíos por minuto y por IP sobre los formularios de cuenta, con un «cubo
de fichas» en memoria. Es holgado a propósito: tras una misma IP puede haber un
conservatorio entero registrándose el último día. El límite general de tráfico
corresponde al proxy inverso.

## 10. Correo saliente

**Decisión.** Patrón *transactional outbox* (§5.8): la transacción de negocio escribe el
mensaje en `email_outbox` y un trabajador de `fimnme-web` lo entrega después, sin pasar
de un máximo por minuto (`FIMNME_CORREO_POR_MINUTO`, 30 por omisión) y con reintentos
exponenciales (2, 4, 8… minutos, tope de 6 horas, abandono al octavo intento).

- La lógica de negocio sólo conoce la interfaz `models.Mailer`.
- **Aún no hay transporte real.** El único `Mailer` es el de desarrollo, que escribe el
  mensaje en el log en lugar de enviarlo. El cuerpo, que lleva enlaces de un solo uso,
  sólo se escribe en el entorno `desarrollo`. El SMTP propio llega en el paso 14.
- Las plantillas viven en `plantilla_correo` (migración `0002`), en ambos idiomas. Los
  valores de sus variables se insertan como texto literal: no se interpretan como
  Markdown ni como HTML, y en el asunto se reducen a una línea.

## 11. Cambio de correo

**Decisión.** El correo cambia de inmediato, exige la contraseña actual, queda sin
verificar y se avisa a la dirección anterior. **Alternativa descartada:** conservar el
correo anterior hasta verificar el nuevo, que exigiría una columna que §8.3 no tiene.

## 12. Vigencia del enlace de activación: 7 días

La especificación fija 24 horas para verificar y 1 hora para recuperar, pero no la
activación de cuentas internas. Se eligen 7 días porque ese enlace suele entregarse a un
jurado externo que puede tardar en atenderlo. **Pendiente de confirmar con la CNMO.**
