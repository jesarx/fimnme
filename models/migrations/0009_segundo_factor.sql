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
-- MIGRACION 0009: verificacion en dos pasos (RF-AUT-09).
--
-- Obligatoria para administrador y observador: tras la contrasena se pide un
-- codigo de una aplicacion autenticadora (TOTP, RFC 6238) o un codigo de
-- recuperacion de un solo uso.
--
-- 1. En la cuenta:
--    totp_secreto      clave compartida, en Base32. Existe desde que se
--                      muestra el alta; vale sólo con totp_activado_at.
--    totp_activado_at  cuando se confirmo el alta con un codigo correcto.
--    totp_ultimo_paso  ultimo intervalo de 30 s aceptado: un codigo ya usado
--                      no vale dos veces, ni dentro de su propio intervalo.
--
-- 2. Codigos de recuperacion: diez por alta, de un solo uso. Se guarda su
--    resumen SHA-256, nunca el codigo.
--
-- 3. En la sesion: pendiente_segundo_factor = 1 mientras falta el codigo. Esa
--    sesion sólo abre la pagina del codigo y caduca en minutos.

ALTER TABLE usuario ADD COLUMN totp_secreto TEXT;
ALTER TABLE usuario ADD COLUMN totp_activado_at TEXT;
ALTER TABLE usuario ADD COLUMN totp_ultimo_paso INTEGER NOT NULL DEFAULT 0
    CHECK (totp_ultimo_paso >= 0);

CREATE TABLE codigo_recuperacion (
    id         INTEGER PRIMARY KEY,
    usuario_id INTEGER NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    resumen    TEXT NOT NULL UNIQUE,
    creado_at  TEXT NOT NULL,
    usado_at   TEXT
);
CREATE INDEX idx_codigo_recuperacion_usuario ON codigo_recuperacion(usuario_id);

ALTER TABLE sesion ADD COLUMN pendiente_segundo_factor INTEGER NOT NULL DEFAULT 0
    CHECK (pendiente_segundo_factor IN (0, 1));
