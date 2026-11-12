-- Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
-- Software libre bajo la licencia GNU GPL v3.0; vease el archivo LICENSE.
-- SPDX-License-Identifier: GPL-3.0-only
--
-- MIGRACION 0002: plantillas de correo iniciales de las cuentas.
--
-- Siembra la version 1 de las cuatro plantillas que necesita el modulo de
-- cuentas, en espanol y en ingles (RF-NOT-01, 02 y 08). El administrador podra
-- editarlas mas adelante: cada edicion crea una version nueva, nunca
-- sobrescribe esta.
--
-- Variables disponibles en asunto y cuerpo:
--   {{nombre}}       nombre de pila del destinatario
--   {{liga}}         enlace de un solo uso (verificar, restablecer, activar)
--   {{email_nuevo}}  solo en cambio_email
--
-- El cuerpo es Markdown del subconjunto de views/markdown.go. La liga va sola
-- en su renglon: la vista la convierte en enlace en la version HTML.
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS.

INSERT INTO plantilla_correo (clave, idioma, version, asunto, cuerpo_md, activa, creado_at) VALUES
('verificar_email', 'es', 1, 'Confirma tu correo electrónico · FIMNME',
'Hola, {{nombre}}:

Recibimos el registro de una cuenta en la plataforma de postulaciones del Foro Internacional de Música Nueva Manuel Enríquez con esta dirección de correo.

Para confirmar que la dirección es tuya, abre este enlace:

{{liga}}

El enlace vale 24 horas y sólo puede usarse una vez. Puedes capturar tu postulación sin confirmar el correo, pero no podrás enviarla.

Si no creaste esta cuenta, ignora este mensaje.', 1, '2026-11-12T00:00:00Z'),

('verificar_email', 'en', 1, 'Confirm your email address · FIMNME',
'Hello, {{nombre}}:

An account was registered on the application platform of the Foro Internacional de Música Nueva Manuel Enríquez with this email address.

To confirm that the address is yours, open this link:

{{liga}}

The link is valid for 24 hours and can only be used once. You can work on your application without confirming your email, but you will not be able to submit it.

If you did not create this account, please ignore this message.', 1, '2026-11-12T00:00:00Z'),

('reset_password', 'es', 1, 'Restablece tu contraseña · FIMNME',
'Hola, {{nombre}}:

Recibimos una solicitud para restablecer la contraseña de tu cuenta en la plataforma de postulaciones del Foro Internacional de Música Nueva Manuel Enríquez.

Para elegir una contraseña nueva, abre este enlace:

{{liga}}

El enlace vale una hora y sólo puede usarse una vez.

Si no hiciste esta solicitud, ignora este mensaje: tu contraseña no cambia.', 1, '2026-11-12T00:00:00Z'),

('reset_password', 'en', 1, 'Reset your password · FIMNME',
'Hello, {{nombre}}:

We received a request to reset the password of your account on the application platform of the Foro Internacional de Música Nueva Manuel Enríquez.

To choose a new password, open this link:

{{liga}}

The link is valid for one hour and can only be used once.

If you did not make this request, please ignore this message: your password stays the same.', 1, '2026-11-12T00:00:00Z'),

('activacion', 'es', 1, 'Activa tu cuenta · FIMNME',
'Hola, {{nombre}}:

Se creó para ti una cuenta en la plataforma de postulaciones del Foro Internacional de Música Nueva Manuel Enríquez.

Para activarla y elegir tu contraseña, abre este enlace:

{{liga}}

El enlace vale 7 días y sólo puede usarse una vez.

Si no esperabas este mensaje, ignóralo.', 1, '2026-11-12T00:00:00Z'),

('activacion', 'en', 1, 'Activate your account · FIMNME',
'Hello, {{nombre}}:

An account has been created for you on the application platform of the Foro Internacional de Música Nueva Manuel Enríquez.

To activate it and choose your password, open this link:

{{liga}}

The link is valid for 7 days and can only be used once.

If you were not expecting this message, please ignore it.', 1, '2026-11-12T00:00:00Z'),

('cambio_email', 'es', 1, 'Cambió el correo de tu cuenta · FIMNME',
'Hola, {{nombre}}:

El correo de tu cuenta en la plataforma de postulaciones del Foro Internacional de Música Nueva Manuel Enríquez se cambió a:

{{email_nuevo}}

A partir de ahora los mensajes de la plataforma llegarán a esa dirección.

Si no hiciste este cambio, alguien más tiene acceso a tu cuenta: escribe de inmediato a la coordinación del Foro.', 1, '2026-11-12T00:00:00Z'),

('cambio_email', 'en', 1, 'The email address of your account has changed · FIMNME',
'Hello, {{nombre}}:

The email address of your account on the application platform of the Foro Internacional de Música Nueva Manuel Enríquez has been changed to:

{{email_nuevo}}

From now on, messages from the platform will be sent to that address.

If you did not make this change, someone else has access to your account: please contact the Forum coordination immediately.', 1, '2026-11-12T00:00:00Z');
