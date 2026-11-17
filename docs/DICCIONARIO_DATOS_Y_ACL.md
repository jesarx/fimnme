# Diccionario de datos y control de acceso (DICCIONARIO_DATOS_Y_ACL)

**Plataforma de Postulaciones FIMNME (CNMO / INBAL)** · Documento 02b · Versión 1.0.0-beta.1

Es el documento al que remite el perfil de dominio (§2.B) para los perfiles y sus
permisos, en el formato común de los sistemas de la CNMO. Tiene dos partes:

- **I. Diccionario de datos:** cada tabla y cada columna del esquema vigente, después
  de la migración 0010.
- **II. Control de acceso:** perfiles, cómo se obtienen, las reglas de visibilidad, y la
  matriz de quién puede hacer cada operación de la API y abrir cada página.

**Se mantiene al día con pruebas.**

- `TestDiccionarioCubreElEsquema` (`models`) falla si una tabla o columna de la base no
  aparece aquí.
- `TestDiccionarioCubreLaAPI` (`controllers`) falla si una operación de la API no está
  en la matriz.
- Quien añade una migración o una ruta actualiza este documento en el mismo cambio.


## Parte I · Diccionario de datos

**Convenciones del esquema** (§3.C del perfil de dominio; el SQL es el subconjunto
común a SQLite y MariaDB):

- **Fechas:** TEXT en UTC, ISO-8601 (`2027-01-30T05:59:00Z`). Toda columna `*_at` es
  un instante así; se muestra en hora de la Ciudad de México.
- **Booleanos:** INTEGER 0 / 1.
- **Enumerados:** TEXT con CHECK. Los valores admitidos se listan en cada columna.
- **JSON:** TEXT que valida la aplicación.
- **Llaves foráneas:** activas en cada conexión (`PRAGMA foreign_keys = ON`).
- **Campos extendidos:** las columnas añadidas por `fimnme-cli schema` se llaman `x_…`
  y se registran en `campo_extendido`. No aparecen aquí porque dependen de cada
  instalación.

**Clases de tabla:**

| Clase | Significado |
|---|---|
| **datos personales** | Contiene datos de particulares sujetos a la LFPDPPP. La anonimización ARCO las vacía. |
| **probatoria** | Material probatorio del proceso. Prohibido alterarla por DDL dinámico (§3.B). |
| **admite campos extendidos** | Lista blanca de `fimnme-cli schema add`. |
| **configuración** | La edita el administrador; cambia cada año. |
| **técnica** | Estado interno del sistema. |

**Tablas que no se describen:** las virtuales de búsqueda (`fts_postulacion`,
`fts_postulacion_trgm` y sus tablas internas) y las internas de SQLite (`sqlite_*`).
No contienen datos propios: se reconstruyen de `indice_busqueda`.


### Cuentas y acceso

#### `usuario`

Una cuenta: postulante o personal. Guarda el resumen Argon2id de la contraseña, nunca la contraseña; el consentimiento del aviso de privacidad con su versión y sello de tiempo; y el estado de la verificación en dos pasos.

*Clase:* datos personales, técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `email` | TEXT NOCASE | no |  | Correo de la cuenta; único sin distinguir mayúsculas. |
| `password_hash` | TEXT | no |  | Resumen Argon2id de la contraseña. Un valor especial marca la cuenta sin activar. |
| `nombre` | TEXT | no |  |  |
| `apellidos` | TEXT | no |  |  |
| `idioma` | TEXT | no | `'es'` | Idioma preferido. Valores: `es`, `en`. |
| `email_verificado_at` | TEXT | sí |  | Cuándo se confirmó el correo; sin él no se puede enviar una postulación. |
| `aviso_privacidad_version` | TEXT | sí |  | Versión del aviso de privacidad que aceptó al registrarse. |
| `aviso_privacidad_aceptado_at` | TEXT | sí |  | Sello de tiempo del consentimiento (LFPDPPP). |
| `activo` | INTEGER | no | `1` | 0 = cuenta desactivada: ninguna sesión la abre. Valores: `0`, `1`. |
| `anonimizado_at` | TEXT | sí |  | Cuándo se anonimizó la cuenta (procedimiento ARCO). |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `actualizado_at` | TEXT | no |  | Última modificación. |
| `totp_secreto` | TEXT | sí |  | Clave TOTP en Base32. Existe desde que se muestra el alta; vale sólo con `totp_activado_at`. |
| `totp_activado_at` | TEXT | sí |  | Cuándo se completó el alta de la verificación en dos pasos. |
| `totp_ultimo_paso` | INTEGER | no | `0` | Último intervalo TOTP aceptado: un código ya usado no vale otra vez. |

*Únicos:* (email).  
*Restricciones:* `totp_ultimo_paso >= 0`.

#### `usuario_perfil`

Perfiles GLOBALES de una cuenta: `admin_sistema`, `coordinacion_nacional` o `postulante`. Los perfiles por convocatoria (jurado, revisor) van en `asignacion`.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `usuario_id` (llave) | INTEGER | no |  | → `usuario`, se borra con ella. |
| `perfil` (llave) | TEXT | no |  | Valores: `admin_sistema`, `coordinacion_nacional`, `postulante`. |
| `otorgado_por` | INTEGER | sí |  | Quién otorgó el perfil. → `usuario`. |
| `creado_at` | TEXT | no |  | Alta del registro. |

#### `asignacion`

Perfil operativo de una cuenta EN UNA CONVOCATORIA: jurado o revisor documental. Sólo las asignaciones activas dan acceso; retirar una surte efecto en la siguiente petición.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, se borra con ella. |
| `usuario_id` | INTEGER | no |  | → `usuario`. |
| `perfil` | TEXT | no |  | Perfil operativo en la convocatoria. Valores: `jurado`, `revisor_documental`. |
| `activo` | INTEGER | no | `1` | 0 = retirada: deja de dar acceso. Valores: `0`, `1`. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Únicos:* (convocatoria_id, usuario_id, perfil).

#### `sesion`

Sesiones abiertas. El identificador es el resumen SHA-256 del valor de la cookie: el valor en claro nunca se guarda. Una sesión pendiente del segundo paso sólo abre la página del código.

*Clase:* datos personales, técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | TEXT | no |  | Identificador. |
| `usuario_id` | INTEGER | no |  | → `usuario`, se borra con ella. |
| `ip` | TEXT | sí |  | IP desde la que se abrió. |
| `user_agent` | TEXT | sí |  | Navegador con que se abrió. |
| `expira_at` | TEXT | no |  | Vencimiento por inactividad (8 h el personal, 30 días el postulante, 10 min una pendiente). |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `pendiente_segundo_factor` | INTEGER | no | `0` | 1 = falta el código del segundo paso. Valores: `0`, `1`. |

#### `token`

Enlaces de un solo uso (verificar correo, restablecer contraseña, activar cuenta), guardados como resumen. Emitir uno nuevo anula los anteriores del mismo propósito.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `hash` (llave) | TEXT | no |  | Sha-256 del token; nunca se guarda en claro. |
| `usuario_id` | INTEGER | no |  | → `usuario`, se borra con ella. |
| `proposito` | TEXT | no |  | Para qué sirve el enlace. Valores: `verificar_email`, `reset_password`, `activacion`. |
| `expira_at` | TEXT | no |  |  |
| `usado_at` | TEXT | sí |  | Cuándo se usó; un enlace usado no vale otra vez. |

#### `codigo_recuperacion`

Códigos de recuperación de la verificación en dos pasos: diez por alta, de un solo uso, guardados como resumen.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `usuario_id` | INTEGER | no |  | → `usuario`, se borra con ella. |
| `resumen` | TEXT | no |  | Resumen SHA-256 del código. |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `usado_at` | TEXT | sí |  | Cuándo se gastó. |

*Únicos:* (resumen).  
*Índices:* (usuario_id).

### Ediciones y convocatorias

#### `edicion`

Una edición anual del Foro (p. ej. la 49.ª, 2027).

*Clase:* configuración, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `numero` | INTEGER | no |  | 49. |
| `anio` | INTEGER | no |  | 2027. |
| `nombre_es` | TEXT | no |  |  |
| `nombre_en` | TEXT | no |  |  |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Únicos:* (numero).

#### `convocatoria`

Una de las dos convocatorias de una edición (Intérpretes o Compositores), con sus bases bilingües, límites y estado del ciclo de vida. La etapa es de la convocatoria, no de la edición.

*Clase:* configuración, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `edicion_id` | INTEGER | no |  | → `edicion`, impide borrarla. |
| `tipo` | TEXT | no |  | Tipo de convocatoria. Valores: `interpretes`, `compositores`. |
| `titulo_es` | TEXT | no |  |  |
| `titulo_en` | TEXT | no |  |  |
| `bases_md_es` | TEXT | sí |  | Bases en Markdown, español. |
| `bases_md_en` | TEXT | sí |  | Bases en Markdown, inglés. |
| `bases_pdf_es_id` | INTEGER | sí |  | → `archivo`. |
| `bases_pdf_en_id` | INTEGER | sí |  | → `archivo`. |
| `imagen_id` | INTEGER | sí |  | → `archivo`. |
| `estado` | TEXT | no | `'borrador'` | Etapa del ciclo de vida; avanza en ese orden. Valores: `borrador`, `publicada`, `postulacion`, `revision_documental`, `evaluacion`, `cerrada`, `archivada`. |
| `max_obras` | INTEGER | sí |  | Compositores: 2; interpretes: null = sin limite. |
| `duracion_max_programa_min` | INTEGER | sí |  | Null = sin limite (decision v1: null). |
| `max_videos` | INTEGER | no | `3` |  |
| `config` | TEXT | no | `'{}'` | Reservada; sin uso en 1.0. |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `actualizado_at` | TEXT | no |  | Última modificación. |
| `duracion_min_programa_min` | INTEGER | no | `0` | Duración mínima del programa, en minutos; 0 = sin mínimo. |

*Únicos:* (edicion_id, tipo).  
*Restricciones:* `duracion_min_programa_min >= 0`.

#### `etapa`

Calendario de una convocatoria: postulación, revisión documental y evaluación, en UTC. El sistema avanza de estado al llegar cada fecha.

*Clase:* configuración.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, se borra con ella. |
| `clave` | TEXT | no |  | Qué etapa del calendario. Valores: `postulacion`, `revision_documental`, `evaluacion`. |
| `inicia_at` | TEXT | no |  |  |
| `termina_at` | TEXT | no |  |  |

*Únicos:* (convocatoria_id, clave).  
*Restricciones:* `termina_at > inicia_at`.

#### `dotacion`

Catálogo de dotaciones instrumentales de una convocatoria de Compositores; cambia cada año.

*Clase:* configuración, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, se borra con ella. |
| `nombre_es` | TEXT | no |  |  |
| `nombre_en` | TEXT | no |  |  |
| `descripcion_es` | TEXT | sí |  |  |
| `descripcion_en` | TEXT | sí |  |  |
| `orden` | INTEGER | no | `0` |  |
| `activa` | INTEGER | no | `1` | 0 = ya no se ofrece al postular. Valores: `0`, `1`. |

*Únicos:* (convocatoria_id, nombre_es).

#### `requisito_documental`

Qué documento se pide, para qué ámbito (postulación u obra), con qué formatos y tamaños, y si es obligatorio. Con `dotacion_id` aplica sólo a esa dotación: así la obligatoriedad depende de la dotación sin estar en el código.

*Clase:* configuración.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, se borra con ella. |
| `dotacion_id` | INTEGER | no | `0` |  |
| `clave` | TEXT | no |  | Fotografia|partitura|autorizacion|particellas|audio. |
| `etiqueta_es` | TEXT | no |  |  |
| `etiqueta_en` | TEXT | no |  |  |
| `ayuda_es` | TEXT | sí |  |  |
| `ayuda_en` | TEXT | sí |  |  |
| `ambito` | TEXT | no |  | Una vez por postulación, o una vez por cada obra. Valores: `postulacion`, `obra`. |
| `obligatorio` | INTEGER | no | `1` | Valores: `0`, `1`. |
| `mimes_permitidos` | TEXT | no |  | Json: ["application/pdf"]. |
| `tamano_max_bytes` | INTEGER | no |  | Tamaño máximo por archivo. |
| `max_archivos` | INTEGER | no | `1` | Cuántos archivos admite. |
| `orden` | INTEGER | no | `0` |  |

*Únicos:* (convocatoria_id, dotacion_id, clave).

#### `criterio_evaluacion`

Criterios con que califica el jurado (en la v1, realización y artístico, escala 1–5).

*Clase:* configuración.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, se borra con ella. |
| `clave` | TEXT | no |  | 'realizacion' | 'artistico'. |
| `etiqueta_es` | TEXT | no |  | "puntaje de realizacion". |
| `etiqueta_en` | TEXT | no |  | "execution score". |
| `descripcion_es` | TEXT | sí |  |  |
| `descripcion_en` | TEXT | sí |  |  |
| `escala_min` | INTEGER | no | `1` | Mínimo de la escala (1 en la v1). |
| `escala_max` | INTEGER | no | `5` | Máximo de la escala (5 en la v1). |
| `obligatorio` | INTEGER | no | `1` | Valores: `0`, `1`. |
| `orden` | INTEGER | no | `0` |  |

*Únicos:* (convocatoria_id, clave).  
*Restricciones:* `escala_max > escala_min`.

### Postulaciones

#### `postulacion`

Una postulación: su estado, folio, subcategoría, número de envíos y dictamen. Pertenece a una cuenta y a una convocatoria.

*Clase:* datos personales, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, impide borrarla. |
| `usuario_id` | INTEGER | no |  | → `usuario`, impide borrarla. |
| `subcategoria` | TEXT | sí |  | Null en compositores. Valores: `solista`, `ensamble`. |
| `folio` | TEXT | sí |  | Asignado al primer envio: fim49-int-0137. |
| `estado` | TEXT | no | `'borrador'` | Estado del proceso. El postulante no ve el descarte. Valores: `borrador`, `enviada`, `en_revision`, `en_subsanacion`, `validada`, `descartada`, `en_evaluacion`, `evaluada`. |
| `dictamen` | TEXT | sí |  | Resultado final del proceso; sólo lo ve la supervisión. Valores: `seleccionada`, `no_seleccionada`, `lista_espera`. |
| `motivo_descarte` | TEXT | sí |  | Motivo del descarte en revisión documental; el postulante no lo ve. |
| `envios` | INTEGER | no | `0` | Cuántas veces se ha enviado (el primero y los reenvíos por subsanación). |
| `enviada_at` | TEXT | sí |  | Fecha del ultimo envio. |
| `primera_enviada_at` | TEXT | sí |  |  |
| `validada_at` | TEXT | sí |  | Cuándo la revisión la validó. |
| `bases_aceptadas_at` | TEXT | sí |  | Cuándo aceptó las bases al enviar. |
| `campos_extra` | TEXT | no | `'{}'` | Reservada; los campos extendidos usan columnas `x_…`. |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `actualizado_at` | TEXT | no |  | Última modificación. |
| `anonimizada_at` | TEXT | sí |  | Cuándo se anonimizó (ARCO). |

*Únicos:* (folio); (convocatoria_id, usuario_id).  
*Índices:* (convocatoria_id, estado); (enviada_at); (usuario_id).  
*Restricciones:* `estado = 'borrador' OR folio IS NOT NULL`.

#### `datos_solista`

Datos generales de una postulación de solista (Intérpretes).

*Clase:* datos personales, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `postulacion_id` (llave) | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `nombre_completo` | TEXT | sí |  |  |
| `nombre_artistico` | TEXT | sí |  | Opcional. |
| `genero` | TEXT | sí |  |  |
| `fecha_nacimiento` | TEXT | sí |  | Iso-8601 'aaaa-mm-dd'. |
| `telefono` | TEXT | sí |  |  |
| `email_contacto` | TEXT NOCASE | sí |  |  |
| `nacionalidad` | TEXT | sí |  | Iso 3166-1 alpha-2. |
| `pais_residencia` | TEXT | sí |  |  |
| `entidad_residencia` | TEXT | sí |  |  |
| `semblanza` | TEXT | sí |  |  |

#### `datos_ensamble`

Datos generales de una postulación de ensamble (Intérpretes).

*Clase:* datos personales, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `postulacion_id` (llave) | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `nombre_ensamble` | TEXT | sí |  |  |
| `representante` | TEXT | sí |  |  |
| `telefono` | TEXT | sí |  |  |
| `email_contacto` | TEXT NOCASE | sí |  |  |
| `dotacion_texto` | TEXT | sí |  | Descripcion libre de la plantilla. |
| `anio_fundacion` | INTEGER | sí |  |  |
| `pais_residencia` | TEXT | sí |  |  |
| `entidad_residencia` | TEXT | sí |  |  |
| `semblanza` | TEXT | sí |  |  |

#### `datos_compositor`

Datos generales de una postulación de Compositores.

*Clase:* datos personales, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `postulacion_id` (llave) | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `nombre_completo` | TEXT | sí |  |  |
| `nombre_artistico` | TEXT | sí |  | Opcional. |
| `genero` | TEXT | sí |  |  |
| `fecha_nacimiento` | TEXT | sí |  |  |
| `telefono` | TEXT | sí |  |  |
| `email_contacto` | TEXT NOCASE | sí |  |  |
| `nacionalidad` | TEXT | sí |  |  |
| `pais_residencia` | TEXT | sí |  |  |
| `entidad_residencia` | TEXT | sí |  |  |
| `semblanza` | TEXT | sí |  |  |

#### `integrante`

Lista nominal de integrantes de un ensamble (nombre e instrumento).

*Clase:* datos personales, admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `nombre` | TEXT | no |  |  |
| `instrumento` | TEXT | no |  |  |
| `orden` | INTEGER | no | `0` |  |

*Únicos:* (postulacion_id, orden).

#### `propuesta_concierto`

Propuesta de concierto de Intérpretes. Fuera de la Ciudad de México se exigen sede, domicilio, fecha y hora.

*Clase:* admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `postulacion_id` (llave) | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `requerimientos_tecnicos` | TEXT | sí |  |  |
| `entidad` | TEXT | sí |  | Clave de entidad federativa. |
| `sede_nombre` | TEXT | sí |  | Obligatorios si entidad <> 'cdmx'. |
| `sede_domicilio` | TEXT | sí |  |  |
| `fecha_concierto` | TEXT | sí |  |  |
| `hora_concierto` | TEXT | sí |  | 'hh:mm'. |

#### `obra`

Una obra del programa (Intérpretes) o una obra postulada (Compositores, hasta dos), con su dotación.

*Clase:* admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `orden` | INTEGER | no | `0` |  |
| `titulo` | TEXT | sí |  |  |
| `compositor` | TEXT | sí |  | Solo interpretes. |
| `semblanza_compositor` | TEXT | sí |  | Solo interpretes. |
| `duracion_min` | INTEGER | sí |  | Duración en minutos; su suma es la duración del programa. |
| `estreno` | TEXT | sí |  | Si la obra es estreno. Valores: `estreno_mundial`, `estreno_mexico`, `no_es_estreno`. |
| `dotacion_id` | INTEGER | sí |  | Solo compositores. → `dotacion`. |
| `nota_obra` | TEXT | sí |  |  |
| `requerimientos_tecnicos` | TEXT | sí |  | Opcional en interpretes, obligatorio en compositores. |
| `liga_media` | TEXT | sí |  | Obligatoria en interpretes, opcional en compositores. |
| `liga_media_estado` | TEXT | no | `'pendiente'` | Valores: `pendiente`, `ok`, `inaccesible`, `no_publico`. |
| `campos_extra` | TEXT | no | `'{}'` | Reservada; los campos extendidos usan columnas `x_…`. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Únicos:* (postulacion_id, orden).  
*Índices:* (dotacion_id); (estreno).

#### `enlace_video`

Ligas a videos de ejecución (1 a 3), con el resultado de su comprobación en red.

*Clase:* admite campos extendidos.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `url` | TEXT | no |  |  |
| `orden` | INTEGER | no | `0` |  |
| `estado` | TEXT | no | `'pendiente'` | Resultado de la comprobación en red; `inaccesible` y `no_publico` bloquean el envío. Valores: `pendiente`, `ok`, `inaccesible`, `no_publico`. |
| `http_status` | INTEGER | sí |  | Código HTTP de la última comprobación. |
| `plataforma` | TEXT | sí |  | Youtube|vimeo|soundcloud|bandcamp|drive|otro. |
| `verificado_at` | TEXT | sí |  |  |
| `detalle` | TEXT | sí |  | Explicación del estado. |

*Únicos:* (postulacion_id, orden).

#### `archivo`

Metadatos de un archivo del acervo. El contenido vive en disco, direccionado por su SHA-256; nunca en la base.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `sha256` | TEXT | no |  | 64 hex. |
| `clave` | TEXT | no |  | Clave abstracta de almacenamiento. |
| `mime` | TEXT | no |  | Detectado por firma, no por extension. |
| `tamano_bytes` | INTEGER | no |  |  |
| `nombre_original` | TEXT | no |  |  |
| `escaneo_av` | TEXT | no | `'pendiente'` | Reservada para el antivirus (RF-DOC-05, no implementado): siempre `pendiente`. Valores: `pendiente`, `limpio`, `infectado`. |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `purgado_at` | TEXT | sí |  | Cuándo se borró su contenido del disco (anonimización); la fila queda como constancia. |

*Únicos:* (sha256).

#### `documento`

Un documento de una postulación u obra, con sus versiones: subir otro no sobrescribe, crea la siguiente versión.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `obra_id` | INTEGER | sí |  | Null si ambito=postulacion. → `obra`, se borra con ella. |
| `requisito_id` | INTEGER | no |  | → `requisito_documental`. |
| `archivo_id` | INTEGER | no |  | → `archivo`. |
| `version` | INTEGER | no | `1` | Número de versión del documento. |
| `reemplaza_a` | INTEGER | sí |  | Versión anterior a la que sustituye. → `documento`. |
| `vigente` | INTEGER | no | `1` | 1 = la versión en vigor. Valores: `0`, `1`. |
| `dictamen` | TEXT | no | `'pendiente'` | Dictamen vigente del revisor sobre esta versión. Valores: `pendiente`, `valido`, `invalido`. |
| `subido_at` | TEXT | no |  |  |

*Índices:* (postulacion_id, vigente).

#### `acuse`

Acuse de recibo de cada envío: la fotografía (snapshot) de lo enviado con que se genera el PDF. Material probatorio: nunca se modifica.

*Clase:* probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `version` | INTEGER | no |  | Número de envío al que corresponde. |
| `archivo_id` | INTEGER | no |  | El pdf. → `archivo`. |
| `snapshot` | TEXT | no |  | Json: la postulacion tal como se envio. |
| `idioma` | TEXT | no |  | Idioma en que se generó. Valores: `es`, `en`. |
| `generado_at` | TEXT | no |  |  |

*Únicos:* (postulacion_id, version).

### Revisión documental

#### `toma_revision`

Qué revisor tomó una postulación. Es informativa, no bloquea a los demás revisores.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `postulacion_id` (llave) | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `revisor_id` | INTEGER | no |  | → `usuario`. |
| `tomada_at` | TEXT | no |  |  |
| `liberada_at` | TEXT | sí |  | Cuándo la soltó. |

#### `revision_documento`

Dictamen del revisor sobre cada documento (válido o inválido), con motivo obligatorio si es inválido.

*Clase:* probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `documento_id` | INTEGER | no |  | → `documento`, se borra con ella. |
| `revisor_id` | INTEGER | no |  | → `usuario`. |
| `dictamen` | TEXT | no |  | Valores: `pendiente`, `valido`, `invalido`. |
| `motivo` | TEXT | sí |  | Obligatorio cuando dictamen='invalido'. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Restricciones:* `dictamen <> 'invalido' OR motivo IS NOT NULL`.

#### `observacion_revision`

Observaciones del revisor sobre una postulación.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `obra_id` | INTEGER | sí |  | → `obra`, se borra con ella. |
| `revisor_id` | INTEGER | no |  | → `usuario`. |
| `texto` | TEXT | no |  |  |
| `creado_at` | TEXT | no |  | Alta del registro. |

#### `subsanacion`

Solicitud de subsanación: el mensaje al postulante, la fecha límite y cuándo se atendió.

*Clase:* probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `revisor_id` | INTEGER | no |  | → `usuario`. |
| `mensaje` | TEXT | no |  | Cuerpo enviado al postulante. |
| `fecha_limite` | TEXT | no |  | Plazo para corregir (UTC). |
| `enviada_at` | TEXT | no |  |  |
| `atendida_at` | TEXT | sí |  | Cuando el postulante reenvia. |
| `cerrada_at` | TEXT | sí |  | Cuando el revisor la resuelve. |

#### `subsanacion_problema`

Cada elemento señalado en una subsanación (documento o campo) con su problema: es exactamente lo que se desbloquea.

*Clase:* probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `subsanacion_id` | INTEGER | no |  | → `subsanacion`, se borra con ella. |
| `clase` | TEXT | no |  | Valores: `documento`, `campo`. |
| `documento_id` | INTEGER | sí |  | Null si el documento falta. → `documento`. |
| `requisito_id` | INTEGER | sí |  | → `requisito_documental`. |
| `campo` | TEXT | sí |  | Ruta canonica: 'datos.email_contacto',. |
| `obra_id` | INTEGER | sí |  | Si el campo o doc es de una obra. → `obra`. |
| `problema` | TEXT | no |  | Motivo, obligatorio. |
| `resuelto_at` | TEXT | sí |  |  |

*Restricciones:* `(clase = 'documento' AND requisito_id IS NOT NULL) OR (clase = 'campo' AND campo IS NOT NULL)`.

#### `solicitud_desbloqueo`

Petición del postulante para corregir algo no señalado, con su justificación y la resolución del revisor.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `subsanacion_id` | INTEGER | no |  | → `subsanacion`, se borra con ella. |
| `campo` | TEXT | sí |  |  |
| `requisito_id` | INTEGER | sí |  | → `requisito_documental`. |
| `obra_id` | INTEGER | sí |  | → `obra`. |
| `justificacion` | TEXT | no |  |  |
| `resuelta_por` | INTEGER | sí |  | → `usuario`. |
| `concedida` | INTEGER | sí |  | 1 = concedida; 0 = negada; NULL = sin resolver. Valores: `0`, `1`. |
| `motivo_respuesta` | TEXT | sí |  |  |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `resuelta_at` | TEXT | sí |  |  |

### Evaluación

#### `evaluacion`

La evaluación de UN jurado sobre una postulación (Intérpretes) o una obra (Compositores): borrador o enviada, observaciones y excusa. Sólo la alcanza su jurado (`JuradoRepo`) y la supervisión (`SupervisionRepo`); no se altera por DDL.

*Clase:* probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`, se borra con ella. |
| `jurado_id` | INTEGER | no |  | → `usuario`. |
| `postulacion_id` | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `obra_id` | INTEGER | sí |  | Null en interpretes. → `obra`, se borra con ella. |
| `obra_clave` | INTEGER | no | `0` | = coalesce(obra_id, 0). |
| `observaciones` | TEXT | sí |  | Opcional. |
| `enviada` | INTEGER | no | `0` | 1 = enviada; 0 = borrador (no cuenta en resultados). Valores: `0`, `1`. |
| `enviada_at` | TEXT | sí |  |  |
| `excusado` | INTEGER | no | `0` | 1 = el jurado se excusó por conflicto de interés. Valores: `0`, `1`. |
| `motivo_excusa` | TEXT | sí |  | Motivo de la excusa. |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `actualizado_at` | TEXT | no |  | Última modificación. |

*Únicos:* (jurado_id, postulacion_id, obra_clave).  
*Restricciones:* `obra_clave = COALESCE(obra_id, 0)`.

#### `evaluacion_puntaje`

Puntaje de una evaluación en un criterio, entero en la escala del criterio (un trigger lo exige).

*Clase:* probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `evaluacion_id` (llave) | INTEGER | no |  | → `evaluacion`, se borra con ella. |
| `criterio_id` (llave) | INTEGER | no |  | → `criterio_evaluacion`. |
| `puntaje` | INTEGER | no |  | Entero dentro de la escala del criterio. |

### Archivo y descargas

#### `indice_busqueda`

Texto de búsqueda derivado de cada postulación (nombres, títulos, correos), que alimenta las tablas FTS5 `fts_postulacion` (sin acentos) y `fts_postulacion_trgm` (erratas). Se reconstruye con `fimnme-cli migrate`. Las tablas FTS5 son técnicas y no se describen aquí.

*Clase:* datos personales, técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `postulacion_id` (llave) | INTEGER | no |  | → `postulacion`, se borra con ella. |
| `texto` | TEXT | no |  | Folio + nombres + ensamble + obras + compositores + correos. |
| `actualizado_at` | TEXT | no |  | Última modificación. |

#### `vista_guardada`

Combinaciones de filtros del archivo guardadas con nombre por una cuenta.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `usuario_id` | INTEGER | no |  | → `usuario`, se borra con ella. |
| `nombre` | TEXT | no |  |  |
| `ruta` | TEXT | no |  | La url con sus parametros de filtro. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Únicos:* (usuario_id, nombre).

#### `tarea_exportacion`

Descargas masivas en ZIP y expedientes de convocatoria, armados en segundo plano; el resultado vive 7 días.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `tipo` | TEXT | no |  | Qué se arma; en 1.0 se usan `zip_documentos` y `expediente_pdf`. Valores: `zip_documentos`, `expediente_pdf`, `reporte_csv`, `interop_json`. |
| `solicitado_por` | INTEGER | no |  | → `usuario`. |
| `convocatoria_id` | INTEGER | sí |  | → `convocatoria`. |
| `parametros` | TEXT | no |  | Json: filtro, tipos de documento, opciones del pdf. |
| `estado` | TEXT | no | `'encolado'` | Valores: `encolado`, `procesando`, `listo`, `error`. |
| `archivo_id` | INTEGER | sí |  | → `archivo`. |
| `progreso` | INTEGER | no | `0` | 0..100. |
| `mensaje_error` | TEXT | sí |  |  |
| `expira_at` | TEXT | sí |  | Cuándo se borra el resultado (7 días). |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `terminado_at` | TEXT | sí |  |  |

*Índices:* (estado, creado_at).

#### `descarga`

Registro de cada descarga de un documento: quién, desde qué IP y cuándo. El acervo es inédito y bajo derechos.

*Clase:* datos personales, probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `usuario_id` | INTEGER | no |  | → `usuario`. |
| `documento_id` | INTEGER | sí |  | → `documento`. |
| `tarea_id` | INTEGER | sí |  | → `tarea_exportacion`. |
| `postulacion_id` | INTEGER | sí |  | → `postulacion`. |
| `ip` | TEXT | sí |  | IP desde la que se descargó. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Índices:* (documento_id, creado_at).

#### `plantilla_reporte`

Definiciones guardadas del generador de reportes CSV, reutilizables año con año.

*Clase:* configuración.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `nombre` | TEXT | no |  |  |
| `tipo` | TEXT | no |  | Valores: `interpretes`, `compositores`. |
| `consulta` | TEXT | no |  | Campos=...&granularidad=...&estado=. |
| `creado_por` | INTEGER | no |  | → `usuario`. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Únicos:* (nombre).

### Correo

#### `plantilla_correo`

Plantillas de correo editables, por clave e idioma, con versiones: la activa es la que se usa.

*Clase:* configuración.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `clave` | TEXT | no |  | 'acuse','subsanacion','verificar_email',. |
| `idioma` | TEXT | no |  | Valores: `es`, `en`. |
| `version` | INTEGER | no | `1` |  |
| `asunto` | TEXT | no |  |  |
| `cuerpo_md` | TEXT | no |  | Con variables {{folio}}, {{nombre}}. |
| `activa` | INTEGER | no | `1` | Valores: `0`, `1`. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Únicos:* (clave, idioma, version).

#### `email_outbox`

Bandeja de salida: cada correo se escribe en la misma transacción que el hecho que lo causa y un trabajador lo entrega con reintentos. Los cuerpos con enlaces secretos se vacían al enviarse.

*Clase:* datos personales.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `destinatario` | TEXT NOCASE | no |  |  |
| `idioma` | TEXT | no | `'es'` | Valores: `es`, `en`. |
| `asunto` | TEXT | no |  |  |
| `cuerpo_html` | TEXT | no |  |  |
| `cuerpo_texto` | TEXT | no |  |  |
| `adjuntos` | TEXT | no | `'[]'` | Json: [{archivo_id, nombre}]. |
| `plantilla_clave` | TEXT | sí |  |  |
| `postulacion_id` | INTEGER | sí |  | → `postulacion`. |
| `estado` | TEXT | no | `'pendiente'` | Estado de la entrega. Valores: `pendiente`, `enviado`, `rebotado`, `error`. |
| `intentos` | INTEGER | no | `0` | Intentos de entrega hechos. |
| `ultimo_error` | TEXT | sí |  |  |
| `message_id` | TEXT | sí |  | Message-ID con que salió. |
| `programado_para` | TEXT | no |  | Cuándo toca el siguiente intento. |
| `enviado_at` | TEXT | sí |  |  |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Índices:* (estado, programado_para).

### Operación

#### `auditoria`

Bitácora de auditoría: cada operación con quién, desde dónde, qué y cuándo. De sólo inserción (los triggers rechazan modificar o borrar); no guarda datos personales en `antes` ni `despues`.

*Clase:* datos personales, probatoria.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `actor_id` | INTEGER | sí |  | → `usuario`. |
| `actor_perfil` | TEXT | sí |  | Perfil con que actuó (o `cli`, `sistema`, `anonimo`). |
| `accion` | TEXT | no |  | 'postulacion.enviar', 'documento.dictaminar',. |
| `entidad` | TEXT | no |  | Tipo de lo afectado (`usuario`, `postulacion`…). |
| `entidad_id` | INTEGER | sí |  | Identificador de lo afectado. |
| `antes` | TEXT | sí |  | Json. |
| `despues` | TEXT | sí |  | Json. |
| `ip` | TEXT | sí |  | IP de quien actuó. |
| `user_agent` | TEXT | sí |  | Navegador de quien actuó. |
| `creado_at` | TEXT | no |  | Alta del registro. |

*Índices:* (entidad, entidad_id, creado_at).

#### `importacion`

Una importación asistida (dotaciones o cuentas de jurado y revisor), con su estado en dos fases: análisis y confirmación.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `tipo` | TEXT | no |  | Qué se importa; en 1.0 no se usa `catalogo`. Valores: `dotaciones`, `usuarios_jurado`, `usuarios_revisor`, `catalogo`. |
| `archivo_id` | INTEGER | no |  | El .csv/.xlsx original. → `archivo`. |
| `usuario_id` | INTEGER | no |  | → `usuario`. |
| `convocatoria_id` | INTEGER | sí |  | → `convocatoria`. |
| `estado` | TEXT | no | `'analizando'` | Fase de la importación. Valores: `analizando`, `esperando_resolucion`, `confirmada`, `cancelada`, `error`. |
| `filas_total` | INTEGER | no | `0` |  |
| `filas_resueltas` | INTEGER | no | `0` |  |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `confirmada_at` | TEXT | sí |  |  |

#### `importacion_conflicto`

Cada renglón de una importación sin correspondencia unívoca, con sus candidatos y la resolución que dio una persona.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `importacion_id` | INTEGER | no |  | → `importacion`, se borra con ella. |
| `fila` | INTEGER | no |  |  |
| `columna` | TEXT | no |  |  |
| `valor_origen` | TEXT | no |  |  |
| `candidatos` | TEXT | no | `'[]'` | Json: coincidencias posibles. |
| `resolucion` | TEXT | sí |  | Lo que decidió la persona; NULL = sin resolver. Valores: `vincular`, `crear_nuevo`, `omitir`. |
| `entidad_id` | INTEGER | sí |  | Id elegido cuando resolucion='vincular'. |
| `resuelto_por` | INTEGER | sí |  | → `usuario`. |
| `resuelto_at` | TEXT | sí |  |  |

#### `campo_extendido`

Diccionario de los campos añadidos por `fimnme-cli schema` (columnas `x_…`): etiqueta, tipo, quién lo ve y si se exporta.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `tabla` | TEXT | no |  |  |
| `columna` | TEXT | no |  |  |
| `tipo_sql` | TEXT | no |  | Text|integer|real. |
| `etiqueta_es` | TEXT | no |  |  |
| `etiqueta_en` | TEXT | no |  |  |
| `ayuda_es` | TEXT | sí |  |  |
| `ayuda_en` | TEXT | sí |  |  |
| `visible_en` | TEXT | no | `'[]'` | Json: perfiles que lo ven. |
| `exportable` | INTEGER | no | `1` | 1 = aparece en los reportes. Valores: `0`, `1`. |
| `creado_por` | INTEGER | sí |  | → `usuario`. |
| `creado_at` | TEXT | no |  | Alta del registro. |
| `eliminado_at` | TEXT | sí |  | Cuándo se retiró el campo. |

*Únicos:* (tabla, columna).

#### `exportacion_interop`

Cada exportación al portal público: qué alcance, la SHA-256 del JSON y quién la autorizó.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `convocatoria_id` | INTEGER | no |  | → `convocatoria`. |
| `alcance` | TEXT | no |  | Qué se exportó. Valores: `bases`, `calendario`, `seleccionados`. |
| `archivo_id` | INTEGER | sí |  | → `archivo`. |
| `sha256` | TEXT | sí |  |  |
| `autorizado_por` | INTEGER | no |  | → `usuario`. |
| `creado_at` | TEXT | no |  | Alta del registro. |

#### `snapshot_espejo`

Instantáneas firmadas (HMAC-SHA256) de la base para el servidor espejo, con su verificación.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `id` (llave) | INTEGER | no |  | Identificador. |
| `ruta` | TEXT | no |  |  |
| `sha256` | TEXT | no |  |  |
| `hmac_sha256` | TEXT | no |  | Firma de la instantánea con la clave del espejo. |
| `tamano_bytes` | INTEGER | no |  |  |
| `archivos_ok` | INTEGER | sí |  | Blobs verificados contra archivo.sha256. |
| `archivos_faltantes` | INTEGER | sí |  |  |
| `destino` | TEXT | sí |  | Host espejo. |
| `estado` | TEXT | no | `'ok'` | Resultado de la instantánea. Valores: `ok`, `error`. |
| `detalle` | TEXT | sí |  |  |
| `creado_at` | TEXT | no |  | Alta del registro. |

#### `schema_migration`

Migraciones aplicadas, con la SHA-256 de cada archivo: si una cambia, el sistema no arranca.

*Clase:* técnica.

| Columna | Tipo | Nula | Por omisión | Descripción |
|---|---|---|---|---|
| `version` (llave) | INTEGER | no |  | Número de la migración. |
| `nombre` | TEXT | no |  |  |
| `sha256` | TEXT | no |  |  |
| `aplicada_at` | TEXT | no |  |  |

## Parte II · Control de acceso (ACL híbrido)

### II.1 Perfiles

| Perfil | Alcance | Cómo se obtiene | Segundo factor |
|---|---|---|---|
| **Anónimo** | — | Sin sesión. | — |
| `postulante` | Global | Autorregistro con verificación de correo. | No |
| `revisor_documental` | **Por convocatoria** | Un administrador lo asigna (en pantalla o por importación). | No |
| `jurado` | **Por convocatoria**; exactamente 3 | Un administrador lo asigna. | No |
| `coordinacion_nacional` (observador) | Global, sólo lectura | `fimnme-cli crear-usuario` o un administrador. | **Obligatorio** |
| `admin_sistema` | Global | `fimnme-cli crear-usuario` o otro administrador. | **Obligatorio** |

**Asignaciones y cuentas:**

- Una cuenta puede tener varios perfiles. Las asignaciones de jurado y revisor se
  retiran sin borrar la cuenta y dejan de dar acceso en la siguiente petición.
- Quien postula en una convocatoria no puede ser su jurado ni su revisor, ni al revés.
- Las cuentas de personal nacen sin contraseña: su titular la elige con el enlace de
  activación.

**Equivalencias con el catálogo institucional** (perfil de dominio, §2.B):

- `admin_sistema` es el mismo perfil.
- `coordinacion_nacional` conserva su semántica de supervisión global.
- `revisor_documental` ocupa el lugar de `subdireccion_programacion`, acotado a este
  dominio.
- `jurado` y `postulante` son propios de este sistema.

### II.2 Sesiones

| Situación | Duración por inactividad |
|---|---|
| Postulante | 30 días |
| Cualquier perfil interno o asignación | 8 horas |
| Pendiente del segundo paso | 10 minutos; sólo abre `/cuenta/segundo-factor` |

**Bloqueo por intentos:**

- Cinco contraseñas o códigos equivocados seguidos bloquean la cuenta 1 minuto, el
  doble con cada fallo siguiente, hasta 30 minutos.
- Además, cada IP tiene un límite de envíos de formularios de cuenta por minuto.

### II.3 Reglas de visibilidad de una postulación

Una postulación la **ven**:

| Quién | Cuándo |
|---|---|
| Su **titular** | Siempre. Ve también «en evaluación» y «evaluada», pero nunca el descarte ni el dictamen final. |
| **Admin** y **Observador** | Siempre, incluidos los borradores. |
| **Revisor** de su convocatoria | Desde que se envía (nunca un borrador). |
| **Jurado** de su convocatoria | Desde que pasa a evaluación, completa: la evaluación no es a ciegas. |

**Modificación:**

- Sólo la **modifica su titular**, mientras sea borrador y la convocatoria esté en
  plazo.
- Durante una subsanación, sólo puede modificar **exactamente** lo señalado o lo que se
  le desbloqueó.
- Ningún otro perfil escribe en una postulación. El revisor dictamina; no edita.

### II.4 Alcance del archivo histórico

El archivo (`/archivo`, `GET /api/v1/archivo`) y los expedientes se acotan por el
alcance de la sesión:

| Perfil | Alcance |
|---|---|
| Admin, Observador | Todas las ediciones y convocatorias, incluidos los borradores. |
| Revisor | Las convocatorias en que es o fue revisor, desde el envío. |
| Jurado | Las convocatorias en que es o fue jurado, desde la evaluación. Descarga documento por documento, no en masa. |
| Postulante | No usa el archivo: ve sus postulaciones en «Mis postulaciones». |

Estas restricciones **no caducan**: se aplican igual a una edición archivada años
después.

### II.5 Reglas que no admiten excepción

1. **Aislamiento entre jurados (§8.4).** Un jurado nunca ve la evaluación de otro ni
   ningún agregado, en ninguna etapa ni en el archivo.
   - Las consultas del jurado viven en `JuradoRepo` y llevan siempre `jurado_id = ?`.
   - Las que cruzan jurados viven en `SupervisionRepo`, al que ningún controlador del
     jurado llega.
   - Lo vigilan `TestAislamientoEntreJurados` (todas las rutas) y
     `TestAislamientoEstructuralDelRepositorio` (el código).
2. **Sólo el jurado asignado califica.** El perfil de administrador no da acceso a
   la evaluación; las rutas de evaluación sólo responden a una asignación de jurado.
3. **El observador no escribe.** Toda operación que modifica algo exige
   `admin_sistema` o el perfil operativo correspondiente.
4. **El expediente se recorta al generarse** según quien lo pide, porque el PDF sale
   del sistema: el de un jurado sólo trae su propia evaluación.
5. **La nómina sale al portal sólo por acción explícita del administrador**, después
   del cierre.
6. **Toda descarga queda registrada** (`descarga`), y toda operación en la bitácora
   (`auditoria`).

### II.6 Operaciones de la API

Cada operación de `/api/v1`, con quién puede ejecutarla. Las rutas con «(de la
convocatoria)» exigen la asignación en la convocatoria de que se trata. «Ver
postulación» remite a §II.3; «Según alcance» a §II.4. La descripción formal de cada una
está en `/api/v1/openapi.json`.

Respuestas de acceso:

| Respuesta | Cuándo |
|---|---|
| `401 no_autenticado` | Sin sesión. |
| `401 segundo_factor_pendiente` | Falta el código del segundo paso. |
| `403 sin_permiso` | El perfil no basta. |
| `404` | Lo pedido existe pero quien pregunta no puede verlo, para no revelar que existe. |


| Operación | Quién |
|---|---|
| `GET /api/v1/openapi.json` | Público |
| `GET /api/v1/catalogos` | Público |
| `GET /api/v1/sesion` | Cualquier sesión |
| `GET /api/v1/auditoria` | Admin, Observador |
| `GET /api/v1/ediciones` | Admin, Observador |
| `POST /api/v1/ediciones` | Admin |
| `GET /api/v1/ediciones/{id}` | Admin, Observador |
| `PUT /api/v1/ediciones/{id}` | Admin |
| `POST /api/v1/ediciones/{id}/convocatorias` | Admin |
| `GET /api/v1/convocatorias/{id}` | Admin, Observador |
| `PUT /api/v1/convocatorias/{id}` | Admin |
| `PUT /api/v1/convocatorias/{id}/etapas` | Admin |
| `POST /api/v1/convocatorias/{id}/estado` | Admin |
| `POST /api/v1/convocatorias/{id}/duplicar` | Admin |
| `POST /api/v1/convocatorias/{id}/dotaciones` | Admin |
| `PUT /api/v1/dotaciones/{id}` | Admin |
| `POST /api/v1/convocatorias/{id}/requisitos` | Admin |
| `PUT /api/v1/requisitos/{id}` | Admin |
| `DELETE /api/v1/requisitos/{id}` | Admin |
| `POST /api/v1/convocatorias/{id}/criterios` | Admin |
| `PUT /api/v1/criterios/{id}` | Admin |
| `DELETE /api/v1/criterios/{id}` | Admin |
| `POST /api/v1/convocatorias/{id}/asignaciones` | Admin |
| `DELETE /api/v1/asignaciones/{id}` | Admin |
| `GET /api/v1/usuarios` | Admin |
| `POST /api/v1/usuarios` | Admin |
| `POST /api/v1/usuarios/{id}/activacion` | Admin |
| `GET /api/v1/postulaciones` | Cualquier sesión (las propias) |
| `POST /api/v1/convocatorias/{id}/postulaciones` | Cualquier cuenta salvo Observador; no Jurado ni Revisor de esa convocatoria |
| `GET /api/v1/postulaciones/{id}` | Ver postulación (§II.3) |
| `PATCH /api/v1/postulaciones/{id}` | Titular, mientras pueda editar (§II.3) |
| `PUT /api/v1/postulaciones/{id}/subcategoria` | Titular, mientras pueda editar (§II.3) |
| `PUT /api/v1/postulaciones/{id}/integrantes` | Titular, mientras pueda editar (§II.3) |
| `PUT /api/v1/postulaciones/{id}/videos` | Titular, mientras pueda editar (§II.3) |
| `POST /api/v1/postulaciones/{id}/obras` | Titular, mientras pueda editar (§II.3) |
| `PUT /api/v1/postulaciones/{id}/obras/orden` | Titular, mientras pueda editar (§II.3) |
| `PATCH /api/v1/obras/{id}` | Titular, mientras pueda editar (§II.3) |
| `DELETE /api/v1/obras/{id}` | Titular, mientras pueda editar (§II.3) |
| `POST /api/v1/postulaciones/{id}/documentos` | Titular, mientras pueda editar (§II.3) |
| `DELETE /api/v1/documentos/{id}` | Titular, mientras pueda editar (§II.3) |
| `GET /api/v1/documentos/{id}/archivo` | Ver postulación (§II.3) |
| `POST /api/v1/postulaciones/{id}/envio` | Titular, mientras pueda editar (§II.3) |
| `GET /api/v1/postulaciones/{id}/acuses/{version}` | Ver postulación (§II.3) |
| `GET /api/v1/convocatorias/{id}/revision` | Revisor (de la convocatoria), Admin, Observador |
| `POST /api/v1/postulaciones/{id}/toma` | Revisor (de la convocatoria), Admin |
| `DELETE /api/v1/postulaciones/{id}/toma` | Revisor (de la convocatoria), Admin |
| `PUT /api/v1/documentos/{id}/dictamen` | Revisor (de la convocatoria), Admin |
| `POST /api/v1/postulaciones/{id}/observaciones` | Revisor (de la convocatoria), Admin |
| `POST /api/v1/postulaciones/{id}/subsanaciones` | Revisor (de la convocatoria), Admin |
| `POST /api/v1/postulaciones/{id}/desbloqueos` | Titular, durante su subsanación |
| `PUT /api/v1/postulaciones/{id}/desbloqueos/{solicitud}` | Revisor (de la convocatoria), Admin |
| `POST /api/v1/postulaciones/{id}/dictamen` | Revisor (de la convocatoria), Admin |
| `GET /api/v1/convocatorias/{id}/evaluacion` | Jurado (de la convocatoria) |
| `GET /api/v1/postulaciones/{id}/evaluacion` | Jurado (de la convocatoria): sólo la propia |
| `PUT /api/v1/postulaciones/{id}/evaluacion` | Jurado (de la convocatoria): sólo la propia |
| `PUT /api/v1/postulaciones/{id}/excusa` | Jurado (de la convocatoria): sólo la propia |
| `DELETE /api/v1/postulaciones/{id}/excusa` | Jurado (de la convocatoria): sólo la propia |
| `GET /api/v1/convocatorias/{id}/resultados` | Admin, Observador |
| `PUT /api/v1/postulaciones/{id}/dictamen-final` | Admin |
| `GET /api/v1/archivo` | Según alcance (§II.4) |
| `GET /api/v1/folios/{folio}` | Ver postulación (§II.3) |
| `GET /api/v1/vistas` | Cualquier sesión (las propias) |
| `POST /api/v1/vistas` | Cualquier sesión (las propias) |
| `DELETE /api/v1/vistas/{id}` | Cualquier sesión (las propias) |
| `POST /api/v1/exportaciones` | Admin, Observador; Revisor (sus convocatorias) |
| `GET /api/v1/exportaciones/{id}` | Quien la pidió |
| `GET /api/v1/exportaciones/{id}/archivo` | Quien la pidió |
| `GET /api/v1/postulaciones/{id}/expediente` | Ver postulación (§II.3); contenido recortado |
| `POST /api/v1/expedientes` | Según alcance (§II.4) |
| `GET /api/v1/convocatorias/{id}/reporte/campos` | Admin, Observador |
| `GET /api/v1/convocatorias/{id}/reporte` | Admin, Observador |
| `GET /api/v1/reportes/plantillas` | Admin, Observador |
| `POST /api/v1/reportes/plantillas` | Admin, Observador |
| `DELETE /api/v1/reportes/plantillas/{id}` | Admin, Observador |
| `GET /api/v1/convocatorias/{id}/tablero` | Admin, Observador |
| `GET /api/v1/ediciones/{id}/estadisticas` | Admin, Observador |
| `GET /api/v1/plantillas-correo` | Admin |
| `GET /api/v1/plantillas-correo/{clave}/{idioma}` | Admin |
| `POST /api/v1/plantillas-correo/{clave}/{idioma}` | Admin |
| `POST /api/v1/plantillas-correo/{clave}/{idioma}/vista-previa` | Admin |
| `POST /api/v1/plantillas-correo/{clave}/{idioma}/prueba` | Admin |
| `POST /api/v1/importaciones` | Admin |
| `GET /api/v1/importaciones/{id}` | Admin |
| `PUT /api/v1/importaciones/{id}/conflictos/{conflicto}` | Admin |
| `POST /api/v1/importaciones/{id}/resoluciones` | Admin |
| `POST /api/v1/importaciones/{id}/confirmacion` | Admin |
| `DELETE /api/v1/importaciones/{id}` | Admin |
| `GET /api/v1/extendidos` | Admin, Observador |
| `GET /api/v1/extendidos/{tabla}/{id}` | Admin, Observador |
| `PUT /api/v1/extendidos/{tabla}/{id}` | Admin |
| `GET /api/v1/interop/convocatoria.json` | Público |
| `GET /api/v1/convocatorias/{id}/publicacion` | Admin |
| `POST /api/v1/convocatorias/{id}/publicacion` | Admin |
| `POST /api/v1/postulaciones/{id}/anonimizacion` | Admin |
| `PUT /api/v1/convocatorias/{id}/archivos/{clase}` | Admin |
| `DELETE /api/v1/convocatorias/{id}/archivos/{clase}` | Admin |

### II.7 Páginas

| Ruta | Quién |
|---|---|
| `/`, `/convocatorias/…`, `/aviso-de-privacidad` | Público |
| `/cuenta/registro`, `/cuenta/ingresar`, `/cuenta/recuperar`, `/cuenta/verificar`, `/cuenta/restablecer`, `/cuenta/activar` | Público (enlaces de un solo uso donde aplica) |
| `/cuenta/segundo-factor` | Sesión pendiente del segundo paso |
| `/cuenta` y sus formularios | Cualquier sesión |
| `/postulaciones…` | Cualquier sesión: las propias. El enlace por folio, según §II.3 |
| `/revision…` | Revisor (de la convocatoria), Admin; Observador en lectura |
| `/evaluacion…` | Jurado (de la convocatoria) |
| `/supervision…` | Admin, Observador. Publicar la nómina y el dictamen final: sólo Admin |
| `/administracion…` | Admin. El Observador ve, sin formularios, ediciones, convocatorias, cuentas y la bitácora; la importación asistida y la anonimización ARCO son sólo del Admin |
| `/archivo…` | Según alcance (§II.4) |

### II.8 Fuera de la web: `fimnme-cli`

`fimnme-cli` no tiene perfiles: lo usa quien tiene acceso al servidor como el usuario
`fimnme`. Por eso las operaciones que no deben existir en la web viven sólo ahí:

- crear el primer administrador;
- restablecer la verificación en dos pasos;
- extensión dinámica del esquema;
- migraciones;
- instantáneas del espejo;
- migración a MariaDB.

Cada orden que modifica datos queda en la bitácora con el actor `cli`.
