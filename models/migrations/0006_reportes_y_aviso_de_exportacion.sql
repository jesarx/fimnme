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
-- MIGRACION 0006: reportes y aviso de exportacion lista.
--
-- 1. Crea la tabla de plantillas de reporte (RF-REP-04), que la
--    especificacion menciona sin definir. Una plantilla guarda, con nombre,
--    la definicion de un reporte CSV: campos, granularidad, filtros, orden y
--    separador, como la cadena de consulta que se enviaria a la API. No
--    guarda la convocatoria: asi la misma plantilla sirve en las ediciones
--    siguientes. Las comparten quienes pueden generar reportes.
--
-- 2. Siembra la version 1 de la plantilla de correo «exportacion_lista», en
--    espanol e ingles: el aviso de que una descarga masiva o un expediente
--    por convocatoria ya puede bajarse (RF-ARCH-45).
--
--    Variables disponibles en asunto y cuerpo:
--      {{nombre}}   nombre de pila de quien la pidio
--      {{numero}}   numero de la descarga
--      {{liga}}     enlace para bajar el archivo
--      {{vence}}    fecha y hora hasta la que puede bajarse, en hora de la
--                   Ciudad de Mexico
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS.

CREATE TABLE plantilla_reporte (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre     TEXT NOT NULL UNIQUE,
    tipo       TEXT NOT NULL CHECK (tipo IN ('interpretes','compositores')),
    consulta   TEXT NOT NULL,   -- campos=...&granularidad=...&estado=...
    creado_por INTEGER NOT NULL REFERENCES usuario(id),
    creado_at  TEXT NOT NULL
);

INSERT INTO plantilla_correo (clave, idioma, version, asunto, cuerpo_md, activa, creado_at) VALUES
('exportacion_lista', 'es', 1, 'Tu descarga n.º {{numero}} está lista · FIMNME',
'Hola, {{nombre}}:

El archivo que pediste (descarga n.º {{numero}}) ya está listo. Puedes bajarlo aquí, con tu sesión iniciada:

{{liga}}

Estará disponible hasta el **{{vence}}**. Después tendrás que pedirlo de nuevo.

El material de las postulaciones es inédito y está protegido por derechos de autor: no lo compartas fuera del proceso de la convocatoria. Cada descarga queda registrada.

Foro Internacional de Música Nueva Manuel Enríquez',
1, '2026-11-14T00:00:00Z'),
('exportacion_lista', 'en', 1, 'Your download no. {{numero}} is ready · FIMNME',
'Hello, {{nombre}}:

The file you requested (download no. {{numero}}) is ready. You can download it here while signed in:

{{liga}}

It will be available until **{{vence}}**. After that you will need to request it again.

The material in the applications is unpublished and protected by copyright: do not share it outside the call process. Every download is recorded.

Foro Internacional de Música Nueva Manuel Enríquez',
1, '2026-11-14T00:00:00Z');
