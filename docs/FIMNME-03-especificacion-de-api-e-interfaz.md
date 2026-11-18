# FIMNME-03 · Especificación de la API y de la interfaz

**Plataforma de Postulaciones FIMNME (CNMO / INBAL)** · Versión 1.0.0-beta.1

Este documento describe cómo se habla con el sistema: las convenciones de la API REST,
sus errores, el mapa de pantallas y las operaciones de cada módulo, con ejemplos.

- **Descripción formal:** OpenAPI 3.1, que sirve el propio sistema en
  `/api/v1/openapi.json`. `TestOpenAPICoincideConLasRutas` garantiza que describe todas
  las operaciones, y sólo ésas.
- **Quién puede hacer qué:** `DICCIONARIO_DATOS_Y_ACL.md`, §II.

## 1. Convenciones

| Aspecto | Regla |
|---|---|
| Base | `/api/v1`. Un cambio incompatible sería `/api/v2`; las rutas de la v1 no cambian. |
| Formato | JSON (UTF-8). Los cuerpos con campos desconocidos se rechazan. Las subidas de archivos son `multipart/form-data`: primero los datos, al final el archivo. |
| Autenticación | La cookie de sesión de las páginas (`fimnme_sesion`, `HttpOnly`, `Secure`, `SameSite=Lax`). No hay contraseñas ni claves de API en las peticiones. |
| Segundo factor | Administrador y observador lo completan en `/cuenta/segundo-factor`. Hasta entonces la API responde `401 segundo_factor_pendiente`. |
| CSRF | Toda petición que modifica algo (`POST`, `PUT`, `PATCH`, `DELETE`) debe venir del mismo origen (`Origin` o `Sec-Fetch-Site: same-origin`). Ningún `GET` modifica estado. |
| Fechas | Instantes en ISO-8601 con zona (`2027-01-29T23:59:00-06:00`); se guardan en UTC. Los días de los filtros admiten `AAAA-MM-DD` o `dd/mm/aaaa` y se entienden en hora de la Ciudad de México. |
| Idioma | Los mensajes de error y los textos salen en el idioma de la cuenta (o de `Accept-Language`, sin sesión). |
| Paginación | `pagina` (desde 1) y `tamano`; la respuesta trae `total`. |
| Límites | Límite por IP en los formularios de cuenta, bloqueo por intentos, tamaño máximo de cada subida según su requisito. |

## 2. Errores

Toda respuesta de error tiene la misma forma:

```json
{"error": {"codigo": "validacion", "mensaje": "Hay datos que corregir.", "campos": [{"campo": "titulo_es", "clave": "error.requerido", "mensaje": "…"}]}}
```

`campos` aparece en los errores de validación. `bloqueadores` aparece cuando una
postulación no puede enviarse: cada uno dice qué falta y en qué paso.

| Código | HTTP | Significado |
|---|---|---|
| `no_autenticado` | 401 | Se requiere iniciar sesión. |
| `segundo_factor_pendiente` | 401 | Falta el código de la verificación en dos pasos. |
| `sin_permiso` | 403 | El perfil no basta para la operación. |
| `no_encontrado` | 404 | No existe, o quien pregunta no puede verlo (no se revela cuál de las dos). |
| `parametro_invalido` | 400 | Un parámetro de la consulta no es válido; el mensaje lo nombra. |
| `json_invalido` | 400 | El cuerpo no es un objeto JSON válido, o trae campos desconocidos. |
| `subida_invalida` | 400 | La subida no tiene la forma esperada. |
| `duplicado` | 409 | Ya existe un registro con esos datos. |
| `exportacion_no_lista` | 409 | La descarga masiva aún se está armando. |
| `exportacion_vencida` | 410 | El enlace de la descarga venció (7 días). |
| `archivo_no_disponible` | 404 | El archivo no está en el acervo (p. ej., anonimizado). |
| `validacion` | 422 | Hay datos que corregir; vienen en `campos`. |
| `incompleta` | 422 | A la postulación le falta algo para enviarse; viene en `bloqueadores`. |
| `demasiadas` | 429 | Demasiadas solicitudes; esperar un minuto. |
| `error_interno` | 500 | Falla del servidor; queda en el log con su detalle. |

## 3. Mapa de pantallas

| Sección | Rutas | Para quién |
|---|---|---|
| Público | `/`, `/convocatorias/{edición}/{tipo}` (con `bases.pdf` e `imagen`), `/aviso-de-privacidad` | Todos |
| Cuenta | `/cuenta/registro`, `/cuenta/ingresar`, `/cuenta/recuperar`, `/cuenta/verificar`, `/cuenta/restablecer`, `/cuenta/activar`, `/cuenta/segundo-factor`, `/cuenta` | Según el caso |
| Postulante | `/postulaciones` (panel) y `/postulaciones/{id}/{paso}`: `datos`, `concierto`, `obras`, `documentos`, `envio` | Titular |
| Enlace permanente | `/postulaciones/{folio}` (p. ej. `FIM49-INT-0137`): no cambia entre versiones | Según visibilidad |
| Revisión documental | `/revision`, `/revision/convocatorias/{id}`, `/revision/postulaciones/{id}` | Revisor, Admin, Observador |
| Evaluación | `/evaluacion`, `/evaluacion/convocatorias/{id}`, `/evaluacion/postulaciones/{id}` | Jurado |
| Supervisión | `/supervision`, `/supervision/convocatorias/{id}` (tablero), `…/resultados`, `…/reporte`, `…/nomina`, `/supervision/ediciones/{id}/estadisticas` | Admin, Observador |
| Administración | `/administracion`, ediciones, `/administracion/convocatorias/{id}`, `/administracion/usuarios`, `…/correos`, `…/bitacora`, `…/importaciones`, `…/arco` | Admin (Observador en lectura, §II.7 del diccionario) |
| Archivo | `/archivo`, `/archivo/descargas` | Personal, según su alcance |
| Preferencias | `POST /idioma`, `POST /tema` | Todos |
| Sondas | `/healthz` (vivo), `/readyz` (listo: base accesible y migraciones al día) | Monitoreo |

Las páginas funcionan sin JavaScript. htmx sólo mejora el guardado automático y los
filtros, que se reflejan en la URL.

## 4. Filtros en la URL

Los filtros del archivo y de la bitácora viajan en la URL, de modo que una búsqueda se
comparte copiando el enlace y se guarda como vista. La misma gramática sirve en
`/archivo` y en `GET /api/v1/archivo`:

- **Igualdad:** `parametro=valor`, un valor por parámetro. Los filtros se combinan
  entre sí (Y).
- **Selección a mano:** `ids=3,8,21`, separados por comas.
- **Rangos de fecha:** `…_desde` / `…_hasta`, ambos incluidos.
- **Orden:** `orden=campo`, o `orden=-campo` para descendente.
- **Texto libre:** `q`, tolerante a acentos y a erratas.

Los parámetros de cada consulta se listan en §6.7 (archivo) y §6.8 (reportes).

## 5. Archivos que produce

| Archivo | Nombre al descargarlo |
|---|---|
| Expediente de una postulación | `expediente_{folio}.pdf` (`expediente_borrador_{id}.pdf` sin folio) |
| Descarga masiva y expediente de una convocatoria | `fimnme_documentos_{n}.zip` / `fimnme_expediente_{n}.pdf`; el ZIP, con una carpeta por folio y `manifiesto.csv` (§6.7) |
| Reportes y listados | CSV en UTF-8 con BOM (lo abre bien Excel), separado por comas |
| Portal | `convocatoria.json` (FIMNME-06) |

## 6. Operaciones por módulo

### 6.1 Ingresar y probar

La API usa la misma cookie de sesión que las páginas: se ingresa con el formulario y
se guarda la cookie. Con `curl`:

```bash
curl -b cookies.txt -c cookies.txt -H 'Sec-Fetch-Site: same-origin' -d 'email=admin@ejemplo.mx&password=LA-CONTRASEÑA' http://127.0.0.1:8080/cuenta/ingresar
```

Administrador y observador dan además el código de su aplicación autenticadora. Hasta
entonces la API responde `401 segundo_factor_pendiente`:

```bash
curl -b cookies.txt -c cookies.txt -H 'Sec-Fetch-Site: same-origin' -d 'codigo=123456' http://127.0.0.1:8080/cuenta/segundo-factor
```

```bash
curl -b cookies.txt http://127.0.0.1:8080/api/v1/sesion
```

```bash
curl -b cookies.txt 'http://127.0.0.1:8080/api/v1/auditoria?accion=sesion.ingreso&tamano=5'
```

| Ruta | Quién | Qué devuelve |
|---|---|---|
| `GET /api/v1/sesion` | Cualquier cuenta | La cuenta en sesión, sus perfiles globales y sus asignaciones por convocatoria |
| `GET /api/v1/auditoria` | `admin_sistema`, `coordinacion_nacional` | La bitácora, paginada. Filtros: `accion`, `entidad`, `entidad_id`, `actor_id`, `desde`, `hasta` (día `AAAA-MM-DD` o `dd/mm/aaaa` en hora de México, o instante ISO-8601), `pagina`, `tamano`. Con `formato=csv` descarga el resultado completo |

### 6.2 Configuración

Lee `admin_sistema` y `coordinacion_nacional`; **escribe sólo `admin_sistema`**. Los
cuerpos son JSON y las fechas van en ISO-8601 con desfase (`2027-01-29T23:59:00-06:00`);
se guardan en UTC. Toda respuesta de una convocatoria trae su configuración completa y,
en `siguiente`, **qué le falta para pasar a la etapa que sigue**.

| Método y ruta | Qué hace |
|---|---|
| `GET /api/v1/ediciones` | Lista las ediciones con el resumen de sus convocatorias |
| `POST /api/v1/ediciones` | Crea una edición: `numero`, `anio`, `nombre_es`, `nombre_en` |
| `GET` · `PUT /api/v1/ediciones/{id}` | Consulta o edita una edición |
| `POST /api/v1/ediciones/{id}/convocatorias` | Crea una convocatoria en borrador: `tipo` (`interpretes` o `compositores`), `titulo_es`, `titulo_en`, `bases_md_es`, `bases_md_en`, `max_obras`, `duracion_max_programa_min`, `max_videos`, `etapas`. Nace con los requisitos y criterios iniciales de su tipo |
| `GET` · `PUT /api/v1/convocatorias/{id}` | Consulta la configuración completa, o edita textos y límites |
| `PUT /api/v1/convocatorias/{id}/etapas` | Sustituye el calendario: `etapas` con `clave` (`postulacion`, `revision_documental`, `evaluacion`), `inicia_at` y `termina_at` |
| `POST /api/v1/convocatorias/{id}/estado` | Cambia de etapa: `estado` y, para retroceder, `motivo`. Sólo al estado inmediato anterior o siguiente |
| `POST /api/v1/convocatorias/{id}/duplicar` | Copia la configuración a otra edición: `edicion_id` |
| `POST /api/v1/convocatorias/{id}/dotaciones` · `PUT /api/v1/dotaciones/{id}` | Crea o edita una dotación (sólo Compositores). Para retirarla: `"activa": false` |
| `POST /api/v1/convocatorias/{id}/requisitos` · `PUT` · `DELETE /api/v1/requisitos/{id}` | Requisitos documentales: `dotacion_id` (0 = regla general), `clave`, `etiqueta_es`, `ambito` (`postulacion` u `obra`), `obligatorio`, `mimes_permitidos`, `tamano_max_bytes`, `max_archivos`. Borrar, sólo en borrador |
| `POST /api/v1/convocatorias/{id}/criterios` · `PUT` · `DELETE /api/v1/criterios/{id}` | Criterios de evaluación: `clave`, `etiqueta_es`, `escala_min`, `escala_max`, `obligatorio` |
| `POST /api/v1/convocatorias/{id}/asignaciones` · `DELETE /api/v1/asignaciones/{id}` | Asigna (`usuario_id`, `perfil`: `jurado` o `revisor_documental`) o retira |
| `GET /api/v1/usuarios?q=` | Busca cuentas por correo, nombre o apellidos |
| `POST /api/v1/usuarios` | Crea una cuenta interna: `email`, `nombre`, `apellidos`, `idioma`, `perfil_global` (vacío para un jurado o revisor; `coordinacion_nacional` o `admin_sistema`). Se le envía su enlace de activación |
| `POST /api/v1/usuarios/{id}/activacion` | Reenvía el enlace de activación |

Ejemplo: crear una edición, su convocatoria de Intérpretes, fijarle el calendario y
publicarla (con la sesión de administrador en `cookies.txt`, como arriba):

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"numero":49,"anio":2027,"nombre_es":"Edición 49 · 2027"}' http://127.0.0.1:8080/api/v1/ediciones
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"tipo":"interpretes","titulo_es":"Convocatoria de Intérpretes","bases_md_es":"# Bases\n\nTexto de las bases."}' http://127.0.0.1:8080/api/v1/ediciones/1/convocatorias
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -X PUT -d '{"etapas":[{"clave":"postulacion","inicia_at":"2026-12-01T00:00:00-06:00","termina_at":"2027-01-29T23:59:00-06:00"},{"clave":"revision_documental","inicia_at":"2027-02-01T00:00:00-06:00","termina_at":"2027-02-26T23:59:00-06:00"},{"clave":"evaluacion","inicia_at":"2027-03-01T00:00:00-06:00","termina_at":"2027-03-31T23:59:00-06:00"}]}' http://127.0.0.1:8080/api/v1/convocatorias/1/etapas
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"estado":"publicada"}' http://127.0.0.1:8080/api/v1/convocatorias/1/estado
```

A partir de ahí la convocatoria **avanza sola** al llegar cada fecha, siempre que cumpla
las reglas; si no, se queda donde está y el log lo avisa. Las reglas de cada transición
están en el [ADR 0005](adr/0005-configuracion-de-convocatorias.md).

### 6.3 Postulaciones

Para cualquier cuenta con sesión, sobre **sus propias** postulaciones; lo ajeno responde
`404`. `admin_sistema` y `coordinacion_nacional` pueden consultar cualquiera, sin
editarla. Sólo se captura con la convocatoria en etapa de postulación y dentro de su
plazo.

| Método y ruta | Qué hace |
|---|---|
| `GET /api/v1/catalogos` | Público. Países (códigos ISO 3166-1), entidades federativas, opciones de género y de estreno |
| `GET /api/v1/postulaciones` | Las postulaciones de la cuenta, de todas las ediciones |
| `POST /api/v1/convocatorias/{id}/postulaciones` | Abre un borrador. En Intérpretes: `subcategoria` (`solista` o `ensamble`). Una por convocatoria y cuenta |
| `GET /api/v1/postulaciones/{id}` | La postulación completa: `campos`, `integrantes`, `videos`, `obras`, la `definicion` de su formulario, las `secciones` con sus pendientes y los `bloqueadores` del envío |
| `PATCH /api/v1/postulaciones/{id}` | Autoguardado: `{"campos": {"datos.telefono": "…", "concierto.entidad": "CDMX"}}`. Guarda lo válido y devuelve en `errores` lo rechazado; `null` vacía un campo |
| `PUT /api/v1/postulaciones/{id}/subcategoria` | De solista a ensamble o al revés: `subcategoria` y `"confirmar": true`. **Borra los datos generales** |
| `PUT /api/v1/postulaciones/{id}/integrantes` | Sustituye la lista: `integrantes` con `nombre` e `instrumento` (sólo ensambles) |
| `PUT /api/v1/postulaciones/{id}/videos` | Sustituye las ligas: `urls` (sólo Intérpretes; `https`, sin repetir) |
| `POST /api/v1/postulaciones/{id}/obras` | Añade una obra vacía |
| `PUT /api/v1/postulaciones/{id}/obras/orden` | Fija el orden del programa: `ids` |
| `PATCH` · `DELETE /api/v1/obras/{id}` | Autoguardado de una obra (`{"campos": {"obra.titulo": "…"}}`), o la quita |

Los nombres de los campos (`datos.*`, `concierto.*`, `obra.*`) no hay que memorizarlos:
cada postulación trae en `definicion` la lista de los suyos, con tipo, obligatoriedad,
límites y etiqueta en el idioma de la cuenta.

Ejemplo, con la sesión de un postulante en `cookies.txt` y la convocatoria 1 de la
demostración (Intérpretes, abierta):

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"subcategoria":"solista"}' http://127.0.0.1:8080/api/v1/convocatorias/1/postulaciones
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -X PATCH -d '{"campos":{"datos.nombre_completo":"Ana López","datos.pais_residencia":"MX","datos.entidad_residencia":"JAL","concierto.entidad":"OAX"}}' http://127.0.0.1:8080/api/v1/postulaciones/1
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -X PUT -d '{"urls":["https://vimeo.com/123456"]}' http://127.0.0.1:8080/api/v1/postulaciones/1/videos
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -X POST http://127.0.0.1:8080/api/v1/postulaciones/1/obras
```

```bash
curl -b cookies.txt http://127.0.0.1:8080/api/v1/postulaciones/1
```

La última respuesta lista en `bloqueadores` todo lo que falta, y en `requisitos` los
documentos que se piden: a la postulación y a cada obra, según su dotación. Las decisiones
de este módulo están en el [ADR 0007](adr/0007-captura-de-postulaciones.md).

### 6.4 Documentos, envío y acuse

| Método y ruta | Qué hace |
|---|---|
| `POST /api/v1/postulaciones/{id}/documentos` | Sube un documento. Formulario `multipart/form-data` con `clave` (el requisito: `fotografia`, `partitura`, `autorizacion`, `particellas`, `audio`…), `obra_id` si es de una obra y, **al final**, `archivo`. Si ya había uno, lo reemplaza y conserva el anterior como versión |
| `DELETE /api/v1/documentos/{id}` | Retira un documento del borrador (no lo borra) |
| `GET /api/v1/documentos/{id}/archivo` | Descarga un documento. Con `?descargar=1` se ofrece como descarga en vez de mostrarse. Queda registrada |
| `POST /api/v1/postulaciones/{id}/envio` | **Envía la postulación; es irreversible.** Cuerpo: `{"confirmo": true, "acepto_bases": true}`. Si falta algo responde `422` con código `incompleta` y la lista |
| `GET /api/v1/postulaciones/{id}/acuses/{version}` | Descarga el acuse en PDF |
| `PUT` · `DELETE /api/v1/convocatorias/{id}/archivos/{clase}` | Sólo `admin_sistema`. Carga o retira `bases_pdf_es`, `bases_pdf_en` o `imagen` (formulario con `archivo`) |

El formato de cada archivo se reconoce **por su contenido**, no por la extensión. Se
admiten PDF, JPEG, PNG, WAV, FLAC y MP3, según lo que pida cada requisito.

Las bases y la imagen se publican, sin sesión, en `/convocatorias/{edicion}/{tipo}/bases.pdf`
(con `?idioma=es` o `?idioma=en`) y `/convocatorias/{edicion}/{tipo}/imagen`.

Ejemplo: subir la fotografía y la partitura de la obra 1, enviar y bajar el acuse.

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -F clave=fotografia -F archivo=@retrato.jpg http://127.0.0.1:8080/api/v1/postulaciones/1/documentos
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -F clave=partitura -F obra_id=1 -F archivo=@partitura.pdf http://127.0.0.1:8080/api/v1/postulaciones/1/documentos
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"confirmo":true,"acepto_bases":true}' http://127.0.0.1:8080/api/v1/postulaciones/1/envio
```

```bash
curl -b cookies.txt -o acuse.pdf http://127.0.0.1:8080/api/v1/postulaciones/1/acuses/1
```

Para enviar, el correo de la cuenta debe estar confirmado: en desarrollo, el enlace de
verificación está en el log (véase «Correos en desarrollo»). Decisiones de este módulo:
[ADR 0009](adr/0009-documentos-envio-y-acuse.md).

### 6.5 Revisión documental

Para los revisores asignados a la convocatoria y el administrador; el observador sólo
consulta. Se revisa con la convocatoria en etapa de revisión documental.

| Método y ruta | Qué hace |
|---|---|
| `GET /api/v1/convocatorias/{id}/revision` | Bandeja: todas las postulaciones enviadas y el tablero de avance. Filtros: `estado`, `subcategoria`, `toma` (`libre`, `mia`, `otra`) y `q` (folio o nombre) |
| `GET /api/v1/postulaciones/{id}` | La misma consulta de siempre; a quien revisa le añade `revision`: dictámenes, observaciones, subsanaciones y los `cambios` del último reenvío |
| `POST` · `DELETE /api/v1/postulaciones/{id}/toma` | «La reviso yo», o la libera. Informa a los demás; no bloquea |
| `PUT /api/v1/documentos/{id}/dictamen` | `dictamen`: `valido`, `invalido` (con `motivo`) o `pendiente` |
| `POST /api/v1/postulaciones/{id}/observaciones` | Nota interna: `texto` y, si es de una obra, `obra_id` |
| `POST /api/v1/postulaciones/{id}/subsanaciones` | Solicita una subsanación: `problemas` (cada uno con `clase`, `campo`, `obra_id` y `problema`), `fecha_limite` y `nota`. Los documentos inválidos se añaden solos |
| `PUT /api/v1/postulaciones/{id}/desbloqueos/{solicitud}` | Concede o niega una solicitud de desbloqueo: `concedida` y `motivo` |
| `POST /api/v1/postulaciones/{id}/dictamen` | `dictamen`: `validada` o `descartada` (con `motivo`) |

Durante una subsanación el postulante usa las mismas rutas de captura, pero sólo puede
modificar los elementos que lista `subsanacion.problemas`; lo demás responde con un error
por elemento. Además:

| Método y ruta | Qué hace |
|---|---|
| `POST /api/v1/postulaciones/{id}/desbloqueos` | Pide corregir algo no señalado: `clase`, `campo`, `obra_id` y `justificacion` |
| `POST /api/v1/postulaciones/{id}/envio` | Reenvía la postulación corregida: mismo folio, acuse nuevo |

Ejemplo, con la sesión de un revisor: marcar inválida la partitura (documento 2), pedir
que se corrija además el teléfono, y dar plazo hasta el 15 de enero.

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -X PUT -d '{"dictamen":"invalido","motivo":"Faltan las páginas 3 y 4."}' http://127.0.0.1:8080/api/v1/documentos/2/dictamen
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"fecha_limite":"2027-01-15T23:59:00-06:00","problemas":[{"clase":"campo","campo":"datos.telefono","problema":"Falta la clave lada."}]}' http://127.0.0.1:8080/api/v1/postulaciones/1/subsanaciones
```

Decisiones de este módulo: [ADR 0010](adr/0010-revision-documental-y-subsanacion.md).

### 6.6 Evaluación

| Método y ruta | Quién | Qué hace |
|---|---|---|
| `GET /api/v1/convocatorias/{id}/evaluacion` | Jurado de la convocatoria | Su lista de trabajo: unidades por evaluar, estado de **su** evaluación y avance |
| `GET /api/v1/postulaciones/{id}/evaluacion` | Jurado | Su evaluación de una unidad. En Compositores, `?obra_id=` |
| `PUT /api/v1/postulaciones/{id}/evaluacion` | Jurado | Guarda su evaluación: `puntajes` (por clave de criterio), `observaciones`, `obra_id` y `enviar` |
| `PUT` · `DELETE /api/v1/postulaciones/{id}/excusa` | Jurado | Se excusa por conflicto de interés (`motivo` y, en Compositores, `obra_id`; sin él, toda la postulación), o retira la excusa |
| `GET /api/v1/convocatorias/{id}/resultados` | `admin_sistema`, `coordinacion_nacional` | Matriz: puntaje por jurado y criterio, excusas, suma, promedio, **puntaje comparable** y lugar. Queda en la bitácora |
| `PUT /api/v1/postulaciones/{id}/dictamen-final` | `admin_sistema` | `dictamen`: `seleccionada`, `no_seleccionada`, `lista_espera` o vacío |

En Intérpretes se califica la postulación completa; en Compositores, cada obra. El jurado
ve la postulación entera con `GET /api/v1/postulaciones/{id}` y descarga sus documentos.
El administrador no puede evaluar.

La matriz ordena por el **puntaje comparable** (`puntaje`): el promedio por jurado,
llevado a tres. Así, si un jurado se excusa, la postulación no queda debajo por tener una
calificación menos. Con tres jurados es igual a la suma.

La convocatoria pasa a evaluación aunque queden postulaciones en revisión: se terminan de
revisar después y, al validarse, se suman a la lista del jurado.

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -X PUT -d '{"puntajes":{"realizacion":4,"artistico":5},"observaciones":"Propuesta sólida.","enviar":true}' http://127.0.0.1:8080/api/v1/postulaciones/1/evaluacion
```

Decisiones de este módulo, y cómo se garantiza el aislamiento entre jurados:
[ADR 0011](adr/0011-evaluacion-por-jurado.md).

### 6.7 Archivo, búsqueda y descargas masivas

| Método y ruta | Quién | Qué hace |
|---|---|---|
| `GET /api/v1/archivo` | Supervisión; revisores y jurados (sólo lo suyo) | Listado de postulaciones de todas las ediciones. Con `formato=csv`, el resultado completo |
| `GET /api/v1/folios/{folio}` | Quien pueda ver la postulación | La postulación de un folio: el enlace permanente |
| `GET` · `POST /api/v1/vistas` | Cualquier cuenta | Vistas guardadas: `nombre` y `ruta` (del propio sitio) |
| `DELETE /api/v1/vistas/{id}` | La cuenta que la guardó | Borra una vista |
| `POST /api/v1/exportaciones` | Supervisión y revisores; el postulante, sólo su postulación | Pide un ZIP de documentos. Responde `202`: se arma en segundo plano |
| `GET /api/v1/exportaciones/{id}` | Quien la pidió | Estado: `encolado`, `procesando`, `listo` o `error` |
| `GET /api/v1/exportaciones/{id}/archivo` | Quien la pidió | Baja el ZIP. Vale 7 días; cada descarga queda registrada |

Parámetros del listado, todos opcionales y combinables:

| Parámetro | Filtra por |
|---|---|
| `q` | Texto: folio, nombres, nombre artístico, ensamble, integrantes, títulos de obra, compositores y correos |
| `edicion`, `convocatoria`, `tipo`, `subcategoria`, `estado` | Número de edición, id de convocatoria, `interpretes` / `compositores`, `solista` / `ensamble`, estado |
| `dotacion`, `estreno` | Alguna de sus obras tiene esa dotación (id) o ese tipo de estreno |
| `entidad_residencia`, `entidad_concierto`, `nacionalidad`, `pais`, `genero` | Datos generales y propuesta de concierto |
| `enviada_desde`, `enviada_hasta` | Fecha de envío (`AAAA-MM-DD`, hora de México, o instante ISO-8601) |
| `con_subsanaciones`, `documentos_pendientes`, `ligas_con_problema` | `si` o `no` |
| `dictamen` | Resultado del proceso. Sólo surte efecto para la supervisión |
| `ids` | Selección a mano: `ids=3,8,21` |
| `orden` | `folio`, `nombre`, `enviada`, `estado`, `obras` o `duracion`; con `-` delante, descendente |
| `pagina`, `tamano` | Página (desde 1) y tamaño (50 por omisión, 200 como máximo) |

La descarga masiva toma **esos mismos parámetros**, más `tipos` (claves de documento
separadas por coma) y `postulacion` (una sola, por id). El ZIP trae los documentos vigentes
de lo que el listado mostraría:

```
FIM49-COM-0007_Maria_Nunez/fotografia.jpg
FIM49-COM-0007_Maria_Nunez/obra-01_Rapsodia/partitura.pdf
FIM49-COM-0007_Maria_Nunez/obra-01_Rapsodia/particellas.pdf
manifiesto.csv
```

```bash
curl -b cookies.txt 'http://127.0.0.1:8080/api/v1/archivo?q=nunez&tipo=compositores&orden=-enviada'
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -X POST 'http://127.0.0.1:8080/api/v1/exportaciones?convocatoria=2&tipos=partitura,particellas'
```

```bash
curl -b cookies.txt -o documentos.zip http://127.0.0.1:8080/api/v1/exportaciones/1/archivo
```

Decisiones de este módulo: [ADR 0012](adr/0012-archivo-busqueda-y-descargas-masivas.md).

### 6.8 Expediente, reportes y tableros

| Método y ruta | Quién | Qué hace |
|---|---|---|
| `GET /api/v1/postulaciones/{id}/expediente` | Quien pueda ver la postulación | Expediente en PDF. `idioma=es|en`, `fotografia=no`, `hoja=si` |
| `POST /api/v1/expedientes` | Supervisión, revisores y jurados (sólo lo suyo) | Expediente de una convocatoria en un solo PDF. `convocatoria` obligatorio; admite los filtros del archivo. Responde `202`; se consulta y se baja en `/api/v1/exportaciones/{id}` |
| `GET /api/v1/convocatorias/{id}/reporte/campos` | `admin_sistema`, `coordinacion_nacional` | Catálogo de campos para armar un reporte |
| `GET /api/v1/convocatorias/{id}/reporte` | `admin_sistema`, `coordinacion_nacional` | Reporte CSV |
| `GET` · `POST /api/v1/reportes/plantillas` | `admin_sistema`, `coordinacion_nacional` | Plantillas de reporte: `nombre`, `tipo` y `consulta` |
| `DELETE /api/v1/reportes/plantillas/{id}` | El administrador, cualquiera; el observador, las suyas | Borra una plantilla |
| `GET /api/v1/convocatorias/{id}/resultados?formato=csv` | `admin_sistema`, `coordinacion_nacional` | Reporte de resultados: la matriz en CSV |
| `GET /api/v1/convocatorias/{id}/tablero` | `admin_sistema`, `coordinacion_nacional` | Postulaciones por estado, envíos por día, documentos y subsanaciones pendientes, avance de cada jurado |
| `GET /api/v1/ediciones/{id}/estadisticas` | `admin_sistema`, `coordinacion_nacional` | Distribuciones de las postulaciones enviadas. Con `convocatoria`, sólo esa |

**Qué lleva el expediente, según quién lo pide:**

| Quien lo pide | Recibe |
|---|---|
| Titular | Su postulación. Nada interno |
| Jurado | La postulación y **su** evaluación; nunca la de otro jurado ni la suma |
| Revisor | La postulación y la revisión documental; ninguna evaluación |
| Administrador y observador | Todo |

Parámetros del reporte, además de los filtros del archivo:

| Parámetro | Qué hace |
|---|---|
| `campos` | Claves del catálogo separadas por coma, en el orden de las columnas |
| `granularidad` | `postulacion` (por omisión) u `obra`: una fila por obra, con los datos de la postulación repetidos |
| `separador` | `coma` (por omisión) o `punto_y_coma`, que es el que Excel en español espera |
| `puntaje_min`, `puntaje_max` | Rango de puntaje |
| `orden` | Los del archivo, o `puntaje` / `-puntaje` |
| `plantilla` | Id de una plantilla guardada; los demás parámetros mandan sobre ella |
| `idioma` | `es` o `en`, para encabezados y valores |

Los campos de obra piden `granularidad=obra`. Los de la evaluación (`eval.*`) piden la
granularidad de lo que califica el jurado: por postulación en Intérpretes, por obra en
Compositores.

```bash
curl -b cookies.txt -o expediente.pdf 'http://127.0.0.1:8080/api/v1/postulaciones/1/expediente'
```

```bash
curl -b cookies.txt -o reporte.csv 'http://127.0.0.1:8080/api/v1/convocatorias/1/reporte?campos=folio,datos.nombre_completo,datos.entidad_residencia,obras.titulos,eval.total,eval.lugar&orden=-puntaje&separador=punto_y_coma'
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"nombre":"Acta de resultados","tipo":"interpretes","consulta":"campos=folio,datos.nombre_completo,eval.total,eval.lugar&orden=-puntaje"}' http://127.0.0.1:8080/api/v1/reportes/plantillas
```

Decisiones de este módulo: [ADR 0013](adr/0013-reportes-expediente-y-tableros.md).

### 6.9 Portal público y protección de datos

| Método y ruta | Quién | Qué hace |
|---|---|---|
| `GET /api/v1/interop/convocatoria.json` | Público | Exportación para el portal: bases, calendario, dotaciones y la nómina publicada. `?edicion=N` |
| `GET /api/v1/convocatorias/{id}/publicacion` | `admin_sistema` | Vista previa de la nómina y publicaciones anteriores |
| `POST /api/v1/convocatorias/{id}/publicacion` | `admin_sistema` | `{"confirmo": true}` publica la nómina; con `"retirar": true` la retira. Sólo con la convocatoria cerrada |
| `POST /api/v1/postulaciones/{id}/anonimizacion` | `admin_sistema` | Anonimiza: `folio` (escrito de nuevo), `fundamento`, `confirmo` y `con_cuenta` |

**Qué sale al portal, y qué no.** Salen los títulos y las bases en ambos idiomas, las
fechas de cada etapa, el catálogo de dotaciones y, de cada seleccionado, el nombre
artístico, la semblanza y los títulos de sus obras. **Nunca** salen datos de contacto,
fechas de nacimiento, documentos, dictámenes, calificaciones, postulaciones no
seleccionadas ni borradores. Cerrar una convocatoria **no publica** la nómina: hace falta
la acción explícita del administrador, y lo publicado queda congelado hasta que vuelva a
publicar.

```bash
curl http://127.0.0.1:8080/api/v1/interop/convocatoria.json
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"confirmo":true}' http://127.0.0.1:8080/api/v1/convocatorias/1/publicacion
```

Decisiones de este módulo: [ADR 0014](adr/0014-continuidad-portal-publico-y-anonimizacion.md).

### 6.10 Plantillas de correo

Cada correo del sistema es una plantilla en Markdown con variables (`{{nombre}}`,
`{{liga}}`…), en español y en inglés. Guardar crea una versión nueva, que pasa a ser la
vigente; ninguna se edita ni se borra. Se rechaza una versión que use una variable que la
plantilla no recibe o que omita una indispensable. Sólo el administrador.

```bash
curl -b cookies.txt http://127.0.0.1:8080/api/v1/plantillas-correo
```

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d '{"asunto":"Recibimos tu postulación {{folio}}","cuerpo_md":"Hola, {{nombre}}:\n\nTu folio es **{{folio}}**."}' http://127.0.0.1:8080/api/v1/plantillas-correo/acuse/es
```

`…/vista-previa` arma el correo con valores de muestra sin guardar, y `…/prueba` envía
un correo de prueba a quien lo pide.

### 6.11 Importación asistida y campos extendidos

| Método y ruta | Quién | Qué hace |
|---|---|---|
| `POST /api/v1/importaciones` | `admin_sistema` | **Analyze**: multipart con `tipo`, `convocatoria_id` y `archivo` (.csv o .xlsx). No escribe nada en el dominio |
| `GET /api/v1/importaciones/{id}` | `admin_sistema` | Conflictos, candidatos y efecto que tendrá confirmarla |
| `PUT /api/v1/importaciones/{id}/conflictos/{conflicto}` | `admin_sistema` | Resuelve uno: `resolucion` (`vincular`, `crear_nuevo`, `omitir`) y `entidad_id` |
| `POST /api/v1/importaciones/{id}/resoluciones` | `admin_sistema` | Resuelve de una vez los que no tienen candidatos: `crear_nuevo` u `omitir` |
| `POST /api/v1/importaciones/{id}/confirmacion` | `admin_sistema` | **Commit**: `{"confirmo": true}`. Todo o nada |
| `DELETE /api/v1/importaciones/{id}` | `admin_sistema` | La cancela |
| `GET /api/v1/extendidos` | `admin_sistema`, `coordinacion_nacional` | Diccionario de campos extendidos |
| `GET` · `PUT /api/v1/extendidos/{tabla}/{id}` | Supervisión · `admin_sistema` | Valores de los campos extendidos de una fila; `PUT` los captura |

Tipos de importación y columnas de la hoja (el primer renglón, con estos nombres exactos):

| `tipo` | Columnas | Obligatorias |
|---|---|---|
| `dotaciones` (sólo Compositores) | `nombre_es`, `nombre_en`, `descripcion_es`, `descripcion_en` | `nombre_es` |
| `usuarios_jurado`, `usuarios_revisor` | `email`, `nombre`, `apellidos`, `idioma` | las tres primeras |

Una dotación sólo se da por existente si su nombre es **idéntico**; en una importación de
cuentas **todo** renglón pide una decisión, también cuando el correo coincide con el de
una cuenta. Una postulación nunca se importa.

```bash
curl -b cookies.txt -H 'Sec-Fetch-Site: same-origin' -F tipo=dotaciones -F convocatoria_id=2 -F archivo=@dotaciones.xlsx http://127.0.0.1:8080/api/v1/importaciones
```

Decisiones de este módulo: [ADR 0015](adr/0015-extension-dinamica-e-importacion-asistida.md).

Los errores de la API son JSON: `{"error": {"codigo": "sin_permiso", "mensaje": "…"}}`.
Cuando una regla no se cumple, la respuesta es `422` con código `validacion` y la lista
`campos`, que dice qué corregir en cada uno.

Las plantillas, la hoja de estilo y las traducciones van **incrustadas en el binario**
(`//go:embed`), así que tras editar cualquiera de ellas hay que reiniciar `make dev`.

Antes de entregar cualquier cambio:

```bash
make verificar
```

Ejecuta el formato (`gofmt`), el análisis estático (`go vet`) y las pruebas. Las pruebas
crean sus propias bases SQLite temporales; no tocan `./data`.

