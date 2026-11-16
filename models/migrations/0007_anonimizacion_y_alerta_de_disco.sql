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
-- MIGRACION 0007: anonimizacion ARCO y alerta de espacio en disco.
--
-- 1. Marca de anonimizacion en la postulacion (RF-ARCH-07, RN-28). La
--    anonimizacion borra los datos personales de una postulacion conservando
--    obra, dictamen y estadisticas; esta columna dice cuando ocurrio.
--
-- 2. Marca de purga en el archivo. Al anonimizar, los archivos con datos
--    personales (fotografia, autorizaciones, acuses) se borran del disco. La
--    fila se conserva, con su suma SHA-256, como constancia de que el archivo
--    existio; esta columna dice que su contenido ya no esta, para que la
--    verificacion del acervo no lo reporte como perdido y para que el espejo
--    lo retire tambien.
--
-- 3. Siembra la version 1 de la plantilla de correo «alerta_disco», en
--    espanol e ingles: el aviso a los administradores de que queda poco
--    espacio en el disco de datos (RF-OPS-07).
--
--    Variables disponibles en asunto y cuerpo:
--      {{nombre}}   nombre de pila del administrador
--      {{libre}}    espacio libre, legible («8.2 GB»)
--      {{umbral}}   umbral configurado, legible («10.0 GB»)
--      {{datos}}    directorio de datos
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS.

ALTER TABLE postulacion ADD COLUMN anonimizada_at TEXT;

ALTER TABLE archivo ADD COLUMN purgado_at TEXT;

INSERT INTO plantilla_correo (clave, idioma, version, asunto, cuerpo_md, activa, creado_at) VALUES
('alerta_disco', 'es', 1, 'Queda poco espacio en disco ({{libre}}) · FIMNME',
'Hola, {{nombre}}:

Al disco donde la plataforma guarda sus datos (`{{datos}}`) le quedan **{{libre}}** libres, por debajo del umbral de {{umbral}}.

Si el disco se llena, la plataforma deja de recibir documentos y postulaciones. Conviene ampliar el disco o liberar espacio antes de que eso ocurra.

Este aviso se repite una vez al día mientras el espacio siga por debajo del umbral.

Foro Internacional de Música Nueva Manuel Enríquez',
1, '2026-11-14T00:00:00Z'),
('alerta_disco', 'en', 1, 'Disk space is running low ({{libre}}) · FIMNME',
'Hello, {{nombre}}:

The disk where the platform stores its data (`{{datos}}`) has **{{libre}}** free, below the threshold of {{umbral}}.

If the disk fills up, the platform stops accepting documents and applications. Please enlarge the disk or free up space before that happens.

This notice repeats once a day while free space stays below the threshold.

Foro Internacional de Música Nueva Manuel Enríquez',
1, '2026-11-14T00:00:00Z');
