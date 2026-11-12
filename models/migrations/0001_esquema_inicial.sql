-- Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
-- Software libre bajo la licencia GNU GPL v3.0; vease el archivo LICENSE.
-- SPDX-License-Identifier: GPL-3.0-only
--
-- MIGRACION 0001: esquema inicial.
--
-- Es el DDL de la seccion 8.3 del documento FIMNME-01, copiado tal cual, en la
-- forma canonica de SQLite 3 y dentro del subconjunto comun con MariaDB. Los
-- unicos cambios respecto del documento son tres, todos anunciados en el:
--   1. toma_revision se crea despues de postulacion (a la que referencia).
--   2. Se anade el trigger de respaldo del rango de puntajes.
--   3. Se anaden los triggers que mantienen los indices FTS5.
--
-- UNA MIGRACION YA APLICADA NO SE EDITA JAMAS: el migrador guarda su SHA-256 y
-- se niega a continuar si el archivo cambia. Todo cambio de esquema va en una
-- migracion nueva (0002_..., 0003_...).

-- ============================================================
-- CONVENCIONES DE TIPO
--   Fechas:     TEXT ISO-8601 UTC  ('2027-03-14T18:05:00Z')
--   Booleanos:  INTEGER 0 | 1
--   Enumerados: TEXT + CHECK        (portable a MariaDB sin ENUM)
--   JSON:       TEXT validado por la aplicacion
-- ============================================================

-- ============================================================
-- USUARIOS Y PERFILES (ACL hibrido)
-- ============================================================
CREATE TABLE usuario (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    email               TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash       TEXT NOT NULL,              -- argon2id
    nombre              TEXT NOT NULL,
    apellidos           TEXT NOT NULL,
    idioma              TEXT NOT NULL DEFAULT 'es'
                        CHECK (idioma IN ('es','en')),
    email_verificado_at TEXT,
    totp_secret         TEXT,
    aviso_privacidad_version     TEXT,
    aviso_privacidad_aceptado_at TEXT,
    activo              INTEGER NOT NULL DEFAULT 1 CHECK (activo IN (0,1)),
    anonimizado_at      TEXT,                       -- procedimiento ARCO
    creado_at           TEXT NOT NULL,
    actualizado_at      TEXT NOT NULL
);

-- Perfiles globales del catalogo institucional.
-- 'postulante' es el perfil base de toda cuenta de autorregistro.
CREATE TABLE usuario_perfil (
    usuario_id   INTEGER NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    perfil       TEXT NOT NULL
                 CHECK (perfil IN ('admin_sistema','coordinacion_nacional','postulante')),
    otorgado_por INTEGER REFERENCES usuario(id),
    creado_at    TEXT NOT NULL,
    PRIMARY KEY (usuario_id, perfil)
);

CREATE TABLE sesion (
    id         TEXT PRIMARY KEY,                    -- UUID v4 en texto
    usuario_id INTEGER NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    ip         TEXT,
    user_agent TEXT,
    expira_at  TEXT NOT NULL,
    creado_at  TEXT NOT NULL
);

CREATE TABLE token (            -- verificacion, reset de contrasena, activacion
    hash       TEXT PRIMARY KEY,  -- SHA-256 del token; nunca se guarda en claro
    usuario_id INTEGER NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    proposito  TEXT NOT NULL
               CHECK (proposito IN ('verificar_email','reset_password','activacion')),
    expira_at  TEXT NOT NULL,
    usado_at   TEXT
);

-- ============================================================
-- EDICIONES Y CONVOCATORIAS
-- ============================================================
CREATE TABLE edicion (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    numero    INTEGER NOT NULL UNIQUE,   -- 49
    anio      INTEGER NOT NULL,          -- 2027
    nombre_es TEXT NOT NULL,
    nombre_en TEXT NOT NULL,
    creado_at TEXT NOT NULL
);

CREATE TABLE archivo (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    sha256          TEXT NOT NULL UNIQUE,   -- 64 hex
    clave           TEXT NOT NULL,          -- clave abstracta de almacenamiento
    mime            TEXT NOT NULL,          -- detectado por firma, no por extension
    tamano_bytes    INTEGER NOT NULL,
    nombre_original TEXT NOT NULL,
    escaneo_av      TEXT NOT NULL DEFAULT 'pendiente'
                    CHECK (escaneo_av IN ('pendiente','limpio','infectado')),
    creado_at       TEXT NOT NULL
);

CREATE TABLE convocatoria (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    edicion_id      INTEGER NOT NULL REFERENCES edicion(id) ON DELETE RESTRICT,
    tipo            TEXT NOT NULL CHECK (tipo IN ('interpretes','compositores')),
    titulo_es       TEXT NOT NULL,
    titulo_en       TEXT NOT NULL,
    bases_md_es     TEXT,
    bases_md_en     TEXT,
    bases_pdf_es_id INTEGER REFERENCES archivo(id),
    bases_pdf_en_id INTEGER REFERENCES archivo(id),
    imagen_id       INTEGER REFERENCES archivo(id),
    estado          TEXT NOT NULL DEFAULT 'borrador'
                    CHECK (estado IN ('borrador','publicada','postulacion',
                                      'revision_documental','evaluacion',
                                      'cerrada','archivada')),
    max_obras       INTEGER,        -- compositores: 2; interpretes: NULL = sin limite
    duracion_max_programa_min INTEGER,   -- NULL = sin limite (decision v1: NULL)
    max_videos      INTEGER NOT NULL DEFAULT 3,
    config          TEXT NOT NULL DEFAULT '{}',   -- JSON: limites de caracteres, banderas
    creado_at       TEXT NOT NULL,
    actualizado_at  TEXT NOT NULL,
    UNIQUE (edicion_id, tipo)
);

CREATE TABLE etapa (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE CASCADE,
    clave           TEXT NOT NULL
                    CHECK (clave IN ('postulacion','revision_documental','evaluacion')),
    inicia_at       TEXT NOT NULL,
    termina_at      TEXT NOT NULL,
    UNIQUE (convocatoria_id, clave),
    CHECK (termina_at > inicia_at)
);

-- Perfiles operativos, SIEMPRE acotados a una convocatoria: un jurado de un ano
-- no conserva acceso el ano siguiente.
CREATE TABLE asignacion (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE CASCADE,
    usuario_id      INTEGER NOT NULL REFERENCES usuario(id),
    perfil          TEXT NOT NULL CHECK (perfil IN ('jurado','revisor_documental')),
    activo          INTEGER NOT NULL DEFAULT 1 CHECK (activo IN (0,1)),
    creado_at       TEXT NOT NULL,
    UNIQUE (convocatoria_id, usuario_id, perfil)
);
-- Reglas verificadas en models/rules.go antes de cambiar de etapa:
--   COUNT(perfil='jurado'             AND activo=1) = 3   (exactamente tres)
--   COUNT(perfil='revisor_documental' AND activo=1) >= 1  (uno o mas; todos ven todo)

-- Catalogo anual de dotaciones (aplica a 'compositores').
CREATE TABLE dotacion (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE CASCADE,
    nombre_es       TEXT NOT NULL,
    nombre_en       TEXT NOT NULL,
    descripcion_es  TEXT,
    descripcion_en  TEXT,
    orden           INTEGER NOT NULL DEFAULT 0,
    activa          INTEGER NOT NULL DEFAULT 1 CHECK (activa IN (0,1)),
    UNIQUE (convocatoria_id, nombre_es)
);

-- Que documentos se piden, donde, y si son obligatorios.
--   dotacion_id = 0  => regla general de la convocatoria.
--   dotacion_id > 0  => regla especifica de esa dotacion (sobrescribe la general).
-- Se usa 0 y no NULL porque SQLite y MariaDB tratan los NULL como distintos en
-- un UNIQUE, y la regla general debe ser unica por (convocatoria, clave).
CREATE TABLE requisito_documental (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id  INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE CASCADE,
    dotacion_id      INTEGER NOT NULL DEFAULT 0,
    clave            TEXT NOT NULL,   -- fotografia|partitura|autorizacion|particellas|audio
    etiqueta_es      TEXT NOT NULL,
    etiqueta_en      TEXT NOT NULL,
    ayuda_es         TEXT,
    ayuda_en         TEXT,
    ambito           TEXT NOT NULL CHECK (ambito IN ('postulacion','obra')),
    obligatorio      INTEGER NOT NULL DEFAULT 1 CHECK (obligatorio IN (0,1)),
    mimes_permitidos TEXT NOT NULL,   -- JSON: ["application/pdf"]
    tamano_max_bytes INTEGER NOT NULL,
    max_archivos     INTEGER NOT NULL DEFAULT 1,
    orden            INTEGER NOT NULL DEFAULT 0,
    UNIQUE (convocatoria_id, dotacion_id, clave)
);

CREATE TABLE criterio_evaluacion (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE CASCADE,
    clave           TEXT NOT NULL,   -- 'realizacion' | 'artistico'
    etiqueta_es     TEXT NOT NULL,   -- "Puntaje de realizacion"
    etiqueta_en     TEXT NOT NULL,   -- "Execution score"
    descripcion_es  TEXT,
    descripcion_en  TEXT,
    escala_min      INTEGER NOT NULL DEFAULT 1,
    escala_max      INTEGER NOT NULL DEFAULT 5,
    obligatorio     INTEGER NOT NULL DEFAULT 1 CHECK (obligatorio IN (0,1)),
    orden           INTEGER NOT NULL DEFAULT 0,
    UNIQUE (convocatoria_id, clave),
    CHECK (escala_max > escala_min)
);

-- ============================================================
-- POSTULACIONES
-- ============================================================
CREATE TABLE postulacion (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id    INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE RESTRICT,
    usuario_id         INTEGER NOT NULL REFERENCES usuario(id) ON DELETE RESTRICT,
    subcategoria       TEXT CHECK (subcategoria IN ('solista','ensamble')),  -- NULL en compositores
    folio              TEXT UNIQUE,    -- asignado al primer envio: FIM49-INT-0137
    estado             TEXT NOT NULL DEFAULT 'borrador'
                       CHECK (estado IN ('borrador','enviada','en_revision',
                                         'en_subsanacion','validada','descartada',
                                         'en_evaluacion','evaluada')),
    dictamen           TEXT CHECK (dictamen IN ('seleccionada','no_seleccionada','lista_espera')),
    motivo_descarte    TEXT,
    envios             INTEGER NOT NULL DEFAULT 0,
    enviada_at         TEXT,           -- fecha del ultimo envio
    primera_enviada_at TEXT,
    validada_at        TEXT,
    bases_aceptadas_at TEXT,
    campos_extra       TEXT NOT NULL DEFAULT '{}',  -- JSON
    creado_at          TEXT NOT NULL,
    actualizado_at     TEXT NOT NULL,
    UNIQUE (convocatoria_id, usuario_id),  -- una postulacion por convocatoria por usuario
    CHECK (estado = 'borrador' OR folio IS NOT NULL)
);

-- "Toma" blanda de una postulacion por un revisor. Informativa, no bloqueante.
CREATE TABLE toma_revision (
    postulacion_id INTEGER PRIMARY KEY REFERENCES postulacion(id) ON DELETE CASCADE,
    revisor_id     INTEGER NOT NULL REFERENCES usuario(id),
    tomada_at      TEXT NOT NULL,
    liberada_at    TEXT
);

-- Nota general sobre las tablas de datos:
-- las columnas son NULLABLE a proposito. La postulacion se captura en varias
-- sesiones; si fueran NOT NULL no se podria guardar un borrador incompleto.
-- La obligatoriedad es una regla del MOMENTO DEL ENVIO, no del almacenamiento,
-- y vive en models/rules.go -> ValidarParaEnvio(), unica fuente de verdad tanto
-- para la pantalla de revision previa como para el endpoint de envio.

CREATE TABLE datos_solista (
    postulacion_id     INTEGER PRIMARY KEY REFERENCES postulacion(id) ON DELETE CASCADE,
    nombre_completo    TEXT,
    nombre_artistico   TEXT,          -- OPCIONAL
    genero             TEXT,
    fecha_nacimiento   TEXT,          -- ISO-8601 'AAAA-MM-DD'
    telefono           TEXT,
    email_contacto     TEXT COLLATE NOCASE,
    nacionalidad       TEXT,          -- ISO 3166-1 alpha-2
    pais_residencia    TEXT,
    entidad_residencia TEXT,
    semblanza          TEXT
);

CREATE TABLE datos_ensamble (
    postulacion_id     INTEGER PRIMARY KEY REFERENCES postulacion(id) ON DELETE CASCADE,
    nombre_ensamble    TEXT,
    representante      TEXT,
    telefono           TEXT,
    email_contacto     TEXT COLLATE NOCASE,
    dotacion_texto     TEXT,          -- descripcion libre de la plantilla
    anio_fundacion     INTEGER,
    pais_residencia    TEXT,
    entidad_residencia TEXT,
    semblanza          TEXT
);

-- Lista dinamica de integrantes del ensamble. Sin limite de filas.
CREATE TABLE integrante (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    nombre         TEXT NOT NULL,
    instrumento    TEXT NOT NULL,
    orden          INTEGER NOT NULL DEFAULT 0,
    UNIQUE (postulacion_id, orden)
);

CREATE TABLE datos_compositor (
    postulacion_id     INTEGER PRIMARY KEY REFERENCES postulacion(id) ON DELETE CASCADE,
    nombre_completo    TEXT,
    nombre_artistico   TEXT,          -- OPCIONAL
    genero             TEXT,
    fecha_nacimiento   TEXT,
    telefono           TEXT,
    email_contacto     TEXT COLLATE NOCASE,
    nacionalidad       TEXT,
    pais_residencia    TEXT,
    entidad_residencia TEXT,
    semblanza          TEXT
);

-- Ligas a videos de ejecucion (solo interpretes). Maximo 3, validadas.
CREATE TABLE enlace_video (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    url            TEXT NOT NULL,
    orden          INTEGER NOT NULL DEFAULT 0,
    estado         TEXT NOT NULL DEFAULT 'pendiente'
                   CHECK (estado IN ('pendiente','ok','inaccesible','no_publico')),
    http_status    INTEGER,
    plataforma     TEXT,            -- youtube|vimeo|soundcloud|bandcamp|drive|otro
    verificado_at  TEXT,
    detalle        TEXT,
    UNIQUE (postulacion_id, orden)
);
-- Maximo convocatoria.max_videos (3) filas por postulacion: validado en rules.go.

-- Solo interpretes.
CREATE TABLE propuesta_concierto (
    postulacion_id          INTEGER PRIMARY KEY REFERENCES postulacion(id) ON DELETE CASCADE,
    requerimientos_tecnicos TEXT,
    entidad                 TEXT,   -- clave de entidad federativa
    sede_nombre             TEXT,   -- obligatorios si entidad <> 'CDMX'
    sede_domicilio          TEXT,
    fecha_concierto         TEXT,
    hora_concierto          TEXT    -- 'HH:MM'
);

-- Obras. Sirve a ambas convocatorias; los campos no aplicables quedan NULL.
CREATE TABLE obra (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id          INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    orden                   INTEGER NOT NULL DEFAULT 0,
    titulo                  TEXT,
    compositor              TEXT,   -- solo interpretes
    semblanza_compositor    TEXT,   -- solo interpretes
    duracion_min            INTEGER,
    estreno                 TEXT CHECK (estreno IN ('estreno_mundial','estreno_mexico','no_es_estreno')),
    dotacion_id             INTEGER REFERENCES dotacion(id),  -- solo compositores
    nota_obra               TEXT,
    requerimientos_tecnicos TEXT,   -- opcional en interpretes, obligatorio en compositores
    liga_media              TEXT,   -- obligatoria en interpretes, opcional en compositores
    liga_media_estado       TEXT NOT NULL DEFAULT 'pendiente'
                            CHECK (liga_media_estado IN ('pendiente','ok','inaccesible','no_publico')),
    campos_extra            TEXT NOT NULL DEFAULT '{}',
    creado_at               TEXT NOT NULL,
    UNIQUE (postulacion_id, orden)
);

-- ============================================================
-- DOCUMENTOS
-- ============================================================
CREATE TABLE documento (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    obra_id        INTEGER REFERENCES obra(id) ON DELETE CASCADE,  -- NULL si ambito=postulacion
    requisito_id   INTEGER NOT NULL REFERENCES requisito_documental(id),
    archivo_id     INTEGER NOT NULL REFERENCES archivo(id),
    version        INTEGER NOT NULL DEFAULT 1,
    reemplaza_a    INTEGER REFERENCES documento(id),
    vigente        INTEGER NOT NULL DEFAULT 1 CHECK (vigente IN (0,1)),
    dictamen       TEXT NOT NULL DEFAULT 'pendiente'
                   CHECK (dictamen IN ('pendiente','valido','invalido')),
    subido_at      TEXT NOT NULL
);
-- Indice total (no parcial) para que MariaDB lo reproduzca sin cambios.
CREATE INDEX idx_documento_postulacion ON documento (postulacion_id, vigente);

-- Historico de dictamenes. No se sobrescribe.
CREATE TABLE revision_documento (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    documento_id INTEGER NOT NULL REFERENCES documento(id) ON DELETE CASCADE,
    revisor_id   INTEGER NOT NULL REFERENCES usuario(id),
    dictamen     TEXT NOT NULL CHECK (dictamen IN ('pendiente','valido','invalido')),
    motivo       TEXT,   -- OBLIGATORIO cuando dictamen='invalido'
    creado_at    TEXT NOT NULL,
    CHECK (dictamen <> 'invalido' OR motivo IS NOT NULL)
);

CREATE TABLE observacion_revision (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    obra_id        INTEGER REFERENCES obra(id) ON DELETE CASCADE,
    revisor_id     INTEGER NOT NULL REFERENCES usuario(id),
    texto          TEXT NOT NULL,
    creado_at      TEXT NOT NULL
);

-- ============================================================
-- SUBSANACION  (desbloqueo selectivo -- ver 4.2)
-- ============================================================
-- La subsanacion NO reabre la postulacion completa: reabre exactamente los
-- elementos listados en subsanacion_problema, y ninguno mas.
CREATE TABLE subsanacion (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    revisor_id     INTEGER NOT NULL REFERENCES usuario(id),
    mensaje        TEXT NOT NULL,   -- cuerpo enviado al postulante
    fecha_limite   TEXT NOT NULL,
    enviada_at     TEXT NOT NULL,
    atendida_at    TEXT,            -- cuando el postulante reenvia
    cerrada_at     TEXT             -- cuando el revisor la resuelve
);

-- Cada fila es, a la vez, la descripcion del error y el PERMISO DE EDICION.
-- El motor de autorizacion de la etapa de subsanacion consulta esta tabla:
-- un campo o una ranura de documento es editable si y solo si aparece aqui,
-- en una subsanacion abierta y no vencida.
CREATE TABLE subsanacion_problema (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    subsanacion_id INTEGER NOT NULL REFERENCES subsanacion(id) ON DELETE CASCADE,
    clase          TEXT NOT NULL CHECK (clase IN ('documento','campo')),
    -- clase = 'documento':
    documento_id   INTEGER REFERENCES documento(id),           -- NULL si el documento falta
    requisito_id   INTEGER REFERENCES requisito_documental(id),
    -- clase = 'campo':
    campo          TEXT,            -- ruta canonica: 'datos.email_contacto',
                                    -- 'concierto.sede_domicilio', 'obra.duracion_min'
    obra_id        INTEGER REFERENCES obra(id),                -- si el campo o doc es de una obra
    problema       TEXT NOT NULL,   -- motivo, obligatorio
    resuelto_at    TEXT,
    CHECK (
        (clase = 'documento' AND requisito_id IS NOT NULL)
     OR (clase = 'campo'     AND campo IS NOT NULL)
    )
);

-- Peticion del postulante para desbloquear un elemento que el revisor no senalo.
CREATE TABLE solicitud_desbloqueo (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    subsanacion_id   INTEGER NOT NULL REFERENCES subsanacion(id) ON DELETE CASCADE,
    campo            TEXT,
    requisito_id     INTEGER REFERENCES requisito_documental(id),
    obra_id          INTEGER REFERENCES obra(id),
    justificacion    TEXT NOT NULL,
    resuelta_por     INTEGER REFERENCES usuario(id),
    concedida        INTEGER CHECK (concedida IN (0,1)),
    motivo_respuesta TEXT,
    creado_at        TEXT NOT NULL,
    resuelta_at      TEXT
);
-- Si concedida = 1, se inserta la fila correspondiente en subsanacion_problema.

-- Snapshot inmutable de lo que se envio, por cada envio. Base probatoria.
CREATE TABLE acuse (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    postulacion_id INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    version        INTEGER NOT NULL,   -- 1 = envio original; 2+ = reenvios
    archivo_id     INTEGER NOT NULL REFERENCES archivo(id),   -- el PDF
    snapshot       TEXT NOT NULL,      -- JSON: la postulacion tal como se envio
    idioma         TEXT NOT NULL CHECK (idioma IN ('es','en')),
    generado_at    TEXT NOT NULL,
    UNIQUE (postulacion_id, version)
);

-- ============================================================
-- EVALUACION
-- ============================================================
-- Unidad de evaluacion: la postulacion (interpretes) o la obra (compositores).
-- obra_clave normaliza el NULL a 0 para que el UNIQUE funcione igual en SQLite
-- y en MariaDB, que tratan los NULL como distintos en indices unicos.
CREATE TABLE evaluacion (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id INTEGER NOT NULL REFERENCES convocatoria(id) ON DELETE CASCADE,
    jurado_id       INTEGER NOT NULL REFERENCES usuario(id),
    postulacion_id  INTEGER NOT NULL REFERENCES postulacion(id) ON DELETE CASCADE,
    obra_id         INTEGER REFERENCES obra(id) ON DELETE CASCADE,  -- NULL en interpretes
    obra_clave      INTEGER NOT NULL DEFAULT 0,   -- = COALESCE(obra_id, 0)
    observaciones   TEXT,            -- opcional
    enviada         INTEGER NOT NULL DEFAULT 0 CHECK (enviada IN (0,1)),
    enviada_at      TEXT,
    excusado        INTEGER NOT NULL DEFAULT 0 CHECK (excusado IN (0,1)),
    motivo_excusa   TEXT,
    creado_at       TEXT NOT NULL,
    actualizado_at  TEXT NOT NULL,
    UNIQUE (jurado_id, postulacion_id, obra_clave),
    CHECK (obra_clave = COALESCE(obra_id, 0))
);

CREATE TABLE evaluacion_puntaje (
    evaluacion_id INTEGER NOT NULL REFERENCES evaluacion(id) ON DELETE CASCADE,
    criterio_id   INTEGER NOT NULL REFERENCES criterio_evaluacion(id),
    puntaje       INTEGER NOT NULL,
    PRIMARY KEY (evaluacion_id, criterio_id)
);
-- El rango (1..5) se valida contra criterio_evaluacion.escala_min/max en
-- models/rules.go, con un trigger de respaldo en la base.

-- Trigger de respaldo: el puntaje debe caer dentro de la escala del criterio.
-- La validacion primaria esta en models/rules.go; esto es la segunda linea.
CREATE TRIGGER evaluacion_puntaje_rango_ins BEFORE INSERT ON evaluacion_puntaje
WHEN NOT EXISTS (SELECT 1 FROM criterio_evaluacion c
                 WHERE c.id = NEW.criterio_id
                   AND NEW.puntaje BETWEEN c.escala_min AND c.escala_max)
BEGIN SELECT RAISE(ABORT, 'puntaje fuera de la escala del criterio'); END;
CREATE TRIGGER evaluacion_puntaje_rango_upd BEFORE UPDATE ON evaluacion_puntaje
WHEN NOT EXISTS (SELECT 1 FROM criterio_evaluacion c
                 WHERE c.id = NEW.criterio_id
                   AND NEW.puntaje BETWEEN c.escala_min AND c.escala_max)
BEGIN SELECT RAISE(ABORT, 'puntaje fuera de la escala del criterio'); END;

-- Agregados. Consultable SOLO por admin_sistema y coordinacion_nacional.
-- Se escribe con CASE WHEN en lugar de FILTER para que MariaDB la reproduzca.
CREATE VIEW v_resultado AS
SELECT
    e.convocatoria_id,
    e.postulacion_id,
    e.obra_clave,
    COUNT(DISTINCT CASE WHEN e.enviada = 1 THEN e.jurado_id END) AS jurados_que_evaluaron,
    SUM(CASE WHEN e.enviada = 1 THEN ep.puntaje ELSE 0 END)      AS puntaje_total,
    ROUND(AVG(CASE WHEN e.enviada = 1 THEN ep.puntaje END), 2)   AS puntaje_promedio
FROM evaluacion e
JOIN evaluacion_puntaje ep ON ep.evaluacion_id = e.id
GROUP BY e.convocatoria_id, e.postulacion_id, e.obra_clave;

-- ============================================================
-- BUSQUEDA (FTS5)
-- ============================================================
-- Indice desnormalizado, mantenido por trigger desde repository.go. Evita un
-- JOIN de seis tablas en cada busqueda.
CREATE TABLE indice_busqueda (
    postulacion_id INTEGER PRIMARY KEY REFERENCES postulacion(id) ON DELETE CASCADE,
    texto          TEXT NOT NULL,   -- folio + nombres + ensamble + obras + compositores + correos
    actualizado_at TEXT NOT NULL
);

-- Tolerancia a acentos y mayusculas.
CREATE VIRTUAL TABLE fts_postulacion USING fts5(
    texto,
    content = 'indice_busqueda',
    content_rowid = 'postulacion_id',
    tokenize = "unicode61 remove_diacritics 2"
);

-- Tolerancia a erratas en nombres propios (busqueda por subcadena).
CREATE VIRTUAL TABLE fts_postulacion_trgm USING fts5(
    texto,
    content = 'indice_busqueda',
    content_rowid = 'postulacion_id',
    tokenize = "trigram"
);

-- Los dos indices FTS5 son de "contenido externo": no guardan el texto, solo
-- lo indexan. Estos triggers los mantienen al dia con indice_busqueda, que es
-- la unica tabla que escribe repository.go.
CREATE TRIGGER indice_busqueda_ai AFTER INSERT ON indice_busqueda BEGIN
    INSERT INTO fts_postulacion (rowid, texto) VALUES (NEW.postulacion_id, NEW.texto);
    INSERT INTO fts_postulacion_trgm (rowid, texto) VALUES (NEW.postulacion_id, NEW.texto);
END;
CREATE TRIGGER indice_busqueda_ad AFTER DELETE ON indice_busqueda BEGIN
    INSERT INTO fts_postulacion (fts_postulacion, rowid, texto)
        VALUES ('delete', OLD.postulacion_id, OLD.texto);
    INSERT INTO fts_postulacion_trgm (fts_postulacion_trgm, rowid, texto)
        VALUES ('delete', OLD.postulacion_id, OLD.texto);
END;
CREATE TRIGGER indice_busqueda_au AFTER UPDATE ON indice_busqueda BEGIN
    INSERT INTO fts_postulacion (fts_postulacion, rowid, texto)
        VALUES ('delete', OLD.postulacion_id, OLD.texto);
    INSERT INTO fts_postulacion_trgm (fts_postulacion_trgm, rowid, texto)
        VALUES ('delete', OLD.postulacion_id, OLD.texto);
    INSERT INTO fts_postulacion (rowid, texto) VALUES (NEW.postulacion_id, NEW.texto);
    INSERT INTO fts_postulacion_trgm (rowid, texto) VALUES (NEW.postulacion_id, NEW.texto);
END;

-- Indices de apoyo para los filtros mas frecuentes.
CREATE INDEX idx_post_conv_estado ON postulacion (convocatoria_id, estado);
CREATE INDEX idx_post_usuario     ON postulacion (usuario_id);
CREATE INDEX idx_post_enviada     ON postulacion (enviada_at);
CREATE INDEX idx_obra_dotacion    ON obra (dotacion_id);
CREATE INDEX idx_obra_estreno     ON obra (estreno);

-- Vistas guardadas de filtros (RF-ARCH-18).
CREATE TABLE vista_guardada (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    usuario_id INTEGER NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    nombre     TEXT NOT NULL,
    ruta       TEXT NOT NULL,   -- la URL con sus parametros de filtro
    creado_at  TEXT NOT NULL,
    UNIQUE (usuario_id, nombre)
);

-- ============================================================
-- DESCARGAS Y EXPEDIENTES ASINCRONOS
-- ============================================================
CREATE TABLE tarea_exportacion (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tipo            TEXT NOT NULL
                    CHECK (tipo IN ('zip_documentos','expediente_pdf','reporte_csv','interop_json')),
    solicitado_por  INTEGER NOT NULL REFERENCES usuario(id),
    convocatoria_id INTEGER REFERENCES convocatoria(id),
    parametros      TEXT NOT NULL,   -- JSON: filtro, tipos de documento, opciones del PDF
    estado          TEXT NOT NULL DEFAULT 'encolado'
                    CHECK (estado IN ('encolado','procesando','listo','error')),
    archivo_id      INTEGER REFERENCES archivo(id),
    progreso        INTEGER NOT NULL DEFAULT 0,   -- 0..100
    mensaje_error   TEXT,
    expira_at       TEXT,
    creado_at       TEXT NOT NULL,
    terminado_at    TEXT
);
CREATE INDEX idx_tarea_estado ON tarea_exportacion (estado, creado_at);

-- Registro de descargas. Material inedito y bajo derechos de autor: la
-- trazabilidad de quien bajo que no es opcional.
CREATE TABLE descarga (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    usuario_id     INTEGER NOT NULL REFERENCES usuario(id),
    documento_id   INTEGER REFERENCES documento(id),
    tarea_id       INTEGER REFERENCES tarea_exportacion(id),
    postulacion_id INTEGER REFERENCES postulacion(id),
    ip             TEXT,
    creado_at      TEXT NOT NULL
);
CREATE INDEX idx_descarga_doc ON descarga (documento_id, creado_at);

-- ============================================================
-- IMPORTACION ASISTIDA EN DOS FASES (Analyze -> Commit)
-- ============================================================
CREATE TABLE importacion (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    tipo           TEXT NOT NULL
                   CHECK (tipo IN ('dotaciones','usuarios_jurado','usuarios_revisor','catalogo')),
    archivo_id     INTEGER NOT NULL REFERENCES archivo(id),  -- el .csv/.xlsx original
    usuario_id     INTEGER NOT NULL REFERENCES usuario(id),
    convocatoria_id INTEGER REFERENCES convocatoria(id),
    estado         TEXT NOT NULL DEFAULT 'analizando'
                   CHECK (estado IN ('analizando','esperando_resolucion','confirmada','cancelada','error')),
    filas_total    INTEGER NOT NULL DEFAULT 0,
    filas_resueltas INTEGER NOT NULL DEFAULT 0,
    creado_at      TEXT NOT NULL,
    confirmada_at  TEXT
);

-- Una fila por ambiguedad detectada en la fase Analyze. NADA se escribe en el
-- dominio hasta que todas estan resueltas por una persona y se ejecuta Commit.
CREATE TABLE importacion_conflicto (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    importacion_id INTEGER NOT NULL REFERENCES importacion(id) ON DELETE CASCADE,
    fila           INTEGER NOT NULL,
    columna        TEXT NOT NULL,
    valor_origen   TEXT NOT NULL,
    candidatos     TEXT NOT NULL DEFAULT '[]',  -- JSON: coincidencias posibles
    resolucion     TEXT
                   CHECK (resolucion IN ('vincular','crear_nuevo','omitir')),
    entidad_id     INTEGER,       -- id elegido cuando resolucion='vincular'
    resuelto_por   INTEGER REFERENCES usuario(id),
    resuelto_at    TEXT
);

-- ============================================================
-- EXTENSION DINAMICA DE NODOS (schema_extender.go)
-- ============================================================
-- Diccionario de metadatos de los campos anadidos por CLI sin recompilar.
-- Las tablas probatorias estan vedadas a este mecanismo (ver 7.6).
CREATE TABLE campo_extendido (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    tabla           TEXT NOT NULL,
    columna         TEXT NOT NULL,
    tipo_sql        TEXT NOT NULL,   -- TEXT|INTEGER|REAL
    etiqueta_es     TEXT NOT NULL,
    etiqueta_en     TEXT NOT NULL,
    ayuda_es        TEXT,
    ayuda_en        TEXT,
    visible_en      TEXT NOT NULL DEFAULT '[]',  -- JSON: perfiles que lo ven
    exportable      INTEGER NOT NULL DEFAULT 1 CHECK (exportable IN (0,1)),
    creado_por      INTEGER REFERENCES usuario(id),
    creado_at       TEXT NOT NULL,
    eliminado_at    TEXT,
    UNIQUE (tabla, columna)
);
-- Lista blanca aplicada en schema_extender.go. Las tablas ausentes de esta
-- lista NO admiten DDL dinamico:
--   edicion, convocatoria, dotacion, postulacion, obra,
--   datos_solista, datos_ensamble, datos_compositor, propuesta_concierto,
--   integrante, enlace_video
-- Vedadas: evaluacion, evaluacion_puntaje, acuse, descarga, auditoria,
--          subsanacion, subsanacion_problema, revision_documento.

-- ============================================================
-- CORREO SALIENTE
-- ============================================================
CREATE TABLE plantilla_correo (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    clave     TEXT NOT NULL,     -- 'acuse','subsanacion','verificar_email',...
    idioma    TEXT NOT NULL CHECK (idioma IN ('es','en')),
    version   INTEGER NOT NULL DEFAULT 1,
    asunto    TEXT NOT NULL,
    cuerpo_md TEXT NOT NULL,     -- con variables {{folio}}, {{nombre}}...
    activa    INTEGER NOT NULL DEFAULT 1 CHECK (activa IN (0,1)),
    creado_at TEXT NOT NULL,
    UNIQUE (clave, idioma, version)
);

CREATE TABLE email_outbox (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    destinatario    TEXT NOT NULL COLLATE NOCASE,
    idioma          TEXT NOT NULL DEFAULT 'es' CHECK (idioma IN ('es','en')),
    asunto          TEXT NOT NULL,
    cuerpo_html     TEXT NOT NULL,
    cuerpo_texto    TEXT NOT NULL,
    adjuntos        TEXT NOT NULL DEFAULT '[]',  -- JSON: [{archivo_id, nombre}]
    plantilla_clave TEXT,
    postulacion_id  INTEGER REFERENCES postulacion(id),
    estado          TEXT NOT NULL DEFAULT 'pendiente'
                    CHECK (estado IN ('pendiente','enviado','rebotado','error')),
    intentos        INTEGER NOT NULL DEFAULT 0,
    ultimo_error    TEXT,
    message_id      TEXT,
    programado_para TEXT NOT NULL,
    enviado_at      TEXT,
    creado_at       TEXT NOT NULL
);
CREATE INDEX idx_outbox_pendiente ON email_outbox (estado, programado_para);

-- ============================================================
-- INTEROPERABILIDAD CON EL PORTAL PUBLICO (Hugo)
-- ============================================================
-- Registro de cada exportacion JSON sanitizada. La nomina de seleccionados solo
-- se publica por accion explicita del administrador, nunca automaticamente.
CREATE TABLE exportacion_interop (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    convocatoria_id INTEGER NOT NULL REFERENCES convocatoria(id),
    alcance         TEXT NOT NULL CHECK (alcance IN ('bases','calendario','seleccionados')),
    archivo_id      INTEGER REFERENCES archivo(id),
    sha256          TEXT,
    autorizado_por  INTEGER NOT NULL REFERENCES usuario(id),
    creado_at       TEXT NOT NULL
);

-- ============================================================
-- CONTINUIDAD OPERATIVA
-- ============================================================
-- Snapshots del espejo (VACUUM INTO) con firma HMAC-SHA256 (mirror.go).
CREATE TABLE snapshot_espejo (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    ruta          TEXT NOT NULL,
    sha256        TEXT NOT NULL,
    hmac_sha256   TEXT NOT NULL,
    tamano_bytes  INTEGER NOT NULL,
    archivos_ok   INTEGER,          -- blobs verificados contra archivo.sha256
    archivos_faltantes INTEGER,
    destino       TEXT,             -- host espejo
    estado        TEXT NOT NULL DEFAULT 'ok' CHECK (estado IN ('ok','error')),
    detalle       TEXT,
    creado_at     TEXT NOT NULL
);

-- ============================================================
-- AUDITORIA
-- ============================================================
CREATE TABLE auditoria (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id   INTEGER REFERENCES usuario(id),
    actor_perfil TEXT,
    accion     TEXT NOT NULL,   -- 'postulacion.enviar', 'documento.dictaminar',
                                -- 'schema.alter', 'interop.publicar', 'arco.anonimizar'
    entidad    TEXT NOT NULL,
    entidad_id INTEGER,
    antes      TEXT,            -- JSON
    despues    TEXT,            -- JSON
    ip         TEXT,
    user_agent TEXT,
    creado_at  TEXT NOT NULL
);
CREATE INDEX idx_auditoria_entidad ON auditoria (entidad, entidad_id, creado_at);
-- Solo insercion. En MariaDB se revocan UPDATE y DELETE al usuario de la
-- aplicacion; en SQLite se impide con triggers BEFORE UPDATE/DELETE que lanzan
-- RAISE(ABORT), ya que no hay permisos por tabla.
CREATE TRIGGER auditoria_no_update BEFORE UPDATE ON auditoria
BEGIN SELECT RAISE(ABORT, 'auditoria es de solo insercion'); END;
CREATE TRIGGER auditoria_no_delete BEFORE DELETE ON auditoria
BEGIN SELECT RAISE(ABORT, 'auditoria es de solo insercion'); END;
