-- Este archivo es parte de FIMNME, la plataforma de postulaciones del Foro
-- Internacional de Musica Nueva Manuel Enriquez (CNMO / INBAL).
--
-- FIMNME es software libre: usted puede redistribuirlo y/o modificarlo bajo
-- los terminos de la Licencia Publica General de GNU, tal como la publica la
-- Free Software Foundation, en la version 3 de la Licencia.
--
-- FIMNME se distribuye con la esperanza de que sea util, pero SIN GARANTIA
-- ALGUNA; ni siquiera la garantia implicita de COMERCIABILIDAD o de APTITUD
-- PARA UN PROPOSITO PARTICULAR. Consulte la Licencia Publica General de GNU
-- para mas detalles.
--
-- Debio recibir una copia de la Licencia Publica General de GNU junto con
-- este programa, en el archivo LICENSE. Si no, consulte
-- <https://www.gnu.org/licenses/>.
--
-- SPDX-License-Identifier: GPL-3.0-only
--
-- MIGRACION 0003: plantillas de correo del envio de la postulacion.
--
-- Siembra la version 1 de dos plantillas, en espanol y en ingles:
--
--   acuse                  acuse de recibo, con el PDF adjunto (RF-NOT-03)
--   recordatorio_borrador  aviso de borrador sin enviar a 7 dias, 2 dias y
--                          12 horas del cierre (RF-NOT-04)
--
-- Variables disponibles en asunto y cuerpo:
--   {{nombre}}        nombre de pila del destinatario
--   {{folio}}         folio de la postulacion (solo en acuse)
--   {{convocatoria}}  titulo de la convocatoria
--   {{edicion}}       nombre de la edicion
--   {{cierre}}        fecha y hora de cierre, en hora de la Ciudad de Mexico
--                     (solo en recordatorio_borrador)
--   {{liga}}          enlace a la postulacion en la plataforma
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS.

INSERT INTO plantilla_correo (clave, idioma, version, asunto, cuerpo_md, activa, creado_at) VALUES
('acuse', 'es', 1, 'Acuse de recibo {{folio}} · FIMNME',
'Hola, {{nombre}}:

Recibimos tu postulación a la {{convocatoria}} ({{edicion}}).

Tu folio es **{{folio}}**. Consérvalo: es la referencia de tu postulación en todo el proceso.

Adjunto a este mensaje va tu acuse de recibo en PDF, con todo lo que enviaste. También puedes descargarlo cuando quieras desde la plataforma:

{{liga}}

La postulación ya no puede modificarse. Si en la revisión documental se encuentra algo que corregir, recibirás un correo con las indicaciones.', 1, '2026-11-13T00:00:00Z'),

('acuse', 'en', 1, 'Acknowledgement of receipt {{folio}} · FIMNME',
'Hello, {{nombre}}:

We have received your application to the {{convocatoria}} ({{edicion}}).

Your reference number is **{{folio}}**. Please keep it: it identifies your application throughout the process.

Your acknowledgement of receipt is attached as a PDF, with everything you submitted. You can also download it at any time from the platform:

{{liga}}

The application can no longer be modified. If the document review finds anything to correct, you will receive an email with instructions.', 1, '2026-11-13T00:00:00Z'),

('recordatorio_borrador', 'es', 1, 'Tu postulación aún no se ha enviado · FIMNME',
'Hola, {{nombre}}:

Tienes una postulación sin enviar en la {{convocatoria}} ({{edicion}}).

El plazo cierra el **{{cierre}}**. Una postulación que no se envía antes del cierre no entra al proceso.

Puedes retomarla donde la dejaste:

{{liga}}

Si ya no deseas postular, ignora este mensaje.', 1, '2026-11-13T00:00:00Z'),

('recordatorio_borrador', 'en', 1, 'Your application has not been submitted yet · FIMNME',
'Hello, {{nombre}}:

You have an unsubmitted application to the {{convocatoria}} ({{edicion}}).

The deadline is **{{cierre}}**. An application that is not submitted before the deadline does not enter the process.

You can pick it up where you left off:

{{liga}}

If you no longer wish to apply, please ignore this message.', 1, '2026-11-13T00:00:00Z');
