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
--
-- MIGRACION 0008: duracion minima del programa.
--
-- Junto a la duracion maxima (duracion_max_programa_min), la convocatoria
-- puede fijar una duracion MINIMA del programa, en minutos: la suma de las
-- duraciones de las obras no puede quedar por debajo para enviar. 0 = sin
-- minimo, que es el valor de toda convocatoria existente.
--
-- A diferencia de la maxima, que usa NULL para «sin limite», aqui el 0 es el
-- valor por omision y la columna nunca es nula: no hay nada que distinguir
-- entre «sin minimo» y «minimo cero».

ALTER TABLE convocatoria
    ADD COLUMN duracion_min_programa_min INTEGER NOT NULL DEFAULT 0
    CHECK (duracion_min_programa_min >= 0);
