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
-- MIGRACION 0004: revision documental.
--
-- 1. Siembra la version 1 de tres plantillas de correo, en espanol e ingles:
--
--      subsanacion        al postulante: que debe corregir y hasta cuando
--                         (RF-NOT-05)
--      acuse_subsanacion  al postulante: acuse del reenvio, con el nuevo PDF
--                         adjunto (RF-NOT-06)
--      aviso_subsanada    a los revisores: una postulacion subsanada volvio a
--                         la bandeja (RF-NOT-07)
--
--    Variables disponibles en asunto y cuerpo:
--      {{nombre}}           nombre de pila del destinatario
--      {{folio}}            folio de la postulacion
--      {{convocatoria}}     titulo de la convocatoria
--      {{edicion}}          nombre de la edicion
--      {{lista_problemas}}  un renglon por cada elemento a corregir
--                           (solo en subsanacion)
--      {{fecha_limite}}     fecha y hora limite, en hora de la Ciudad de
--                           Mexico (solo en subsanacion)
--      {{nota}}             texto libre del revisor; puede ir vacio
--                           (solo en subsanacion)
--      {{liga}}             enlace a la postulacion en la plataforma
--
-- 2. Baja a 20 MB el tamano maximo del archivo de audio en los requisitos
--    que siguen con el valor inicial de 100 MB (decision de la CNMO del 13 de
--    noviembre de 2026). Un requisito que el administrador ya haya ajustado a
--    otro valor no se toca.
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS.

INSERT INTO plantilla_correo (clave, idioma, version, asunto, cuerpo_md, activa, creado_at) VALUES
('subsanacion', 'es', 1, 'Tu postulación {{folio}} necesita correcciones · FIMNME',
'Hola, {{nombre}}:

Revisamos tu postulación **{{folio}}** a la {{convocatoria}} ({{edicion}}) y encontramos lo siguiente por corregir:

{{lista_problemas}}

{{nota}}

Tienes hasta el **{{fecha_limite}}** para corregirlo y volver a enviar la postulación. En la plataforma sólo podrás modificar los elementos de esta lista; el resto queda como lo enviaste.

{{liga}}

Si no reenvías la postulación antes de esa fecha, se dictaminará tal como está.', 1, '2026-11-13T00:00:00Z'),

('subsanacion', 'en', 1, 'Your application {{folio}} needs corrections · FIMNME',
'Hello, {{nombre}}:

We have reviewed your application **{{folio}}** to the {{convocatoria}} ({{edicion}}) and found the following to be corrected:

{{lista_problemas}}

{{nota}}

You have until **{{fecha_limite}}** to correct it and submit the application again. On the platform you will only be able to modify the items on this list; everything else stays as you submitted it.

{{liga}}

If you do not resubmit the application before that date, it will be assessed as it stands.', 1, '2026-11-13T00:00:00Z'),

('acuse_subsanacion', 'es', 1, 'Acuse de recibo de tu postulación corregida {{folio}} · FIMNME',
'Hola, {{nombre}}:

Recibimos las correcciones a tu postulación **{{folio}}** a la {{convocatoria}} ({{edicion}}).

Adjunto a este mensaje va el nuevo acuse de recibo en PDF, con la postulación tal como quedó. El acuse anterior se conserva. Puedes descargar ambos desde la plataforma:

{{liga}}

La postulación vuelve a revisión documental y ya no puede modificarse.', 1, '2026-11-13T00:00:00Z'),

('acuse_subsanacion', 'en', 1, 'Acknowledgement of receipt of your corrected application {{folio}} · FIMNME',
'Hello, {{nombre}}:

We have received the corrections to your application **{{folio}}** to the {{convocatoria}} ({{edicion}}).

The new acknowledgement of receipt is attached as a PDF, with the application as it now stands. The previous acknowledgement is kept. You can download both from the platform:

{{liga}}

The application goes back to document review and can no longer be modified.', 1, '2026-11-13T00:00:00Z'),

('aviso_subsanada', 'es', 1, 'Postulación subsanada {{folio}} · FIMNME',
'Hola, {{nombre}}:

La postulación **{{folio}}** de la {{convocatoria}} ({{edicion}}) fue corregida y reenviada. Está de nuevo en la bandeja de revisión, con el detalle de lo que cambió:

{{liga}}', 1, '2026-11-13T00:00:00Z'),

('aviso_subsanada', 'en', 1, 'Corrected application {{folio}} · FIMNME',
'Hello, {{nombre}}:

Application **{{folio}}** to the {{convocatoria}} ({{edicion}}) has been corrected and resubmitted. It is back in the review tray, with the details of what changed:

{{liga}}', 1, '2026-11-13T00:00:00Z');

UPDATE requisito_documental
SET tamano_max_bytes = 20971520
WHERE clave = 'audio' AND tamano_max_bytes = 104857600;
