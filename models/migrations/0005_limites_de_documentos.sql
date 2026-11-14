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
-- MIGRACION 0005: limites de los documentos (decision de la CNMO del 13 de
-- noviembre de 2026).
--
-- 1. El archivo de audio admite hasta 30 MB, igual que la partitura y las
--    particellas. Se actualizan los requisitos que siguen con alguno de los
--    valores iniciales anteriores (100 MB, o los 20 MB de la migracion 0004).
--    Un requisito que el administrador haya ajustado a otro valor no se toca.
--
-- 2. Las particellas se entregan reunidas en UN solo archivo PDF. Se fija en
--    1 el maximo de archivos y se anade el texto de ayuda a los requisitos
--    que no tienen uno propio.
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS.

UPDATE requisito_documental
SET tamano_max_bytes = 31457280
WHERE clave = 'audio' AND tamano_max_bytes IN (20971520, 104857600);

UPDATE requisito_documental
SET max_archivos = 1
WHERE clave = 'particellas';

UPDATE requisito_documental
SET ayuda_es = 'Todas las particellas reunidas en un solo archivo PDF.',
    ayuda_en = 'All the parts together in a single PDF file.'
WHERE clave = 'particellas' AND COALESCE(ayuda_es, '') = '';
