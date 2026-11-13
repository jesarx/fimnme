# ADR 0007 — Captura de postulaciones

- **Estado:** aceptada
- **Fecha:** 2026-11-13

Decisiones del paso 5. No añade dependencias ni migraciones: todo cabe en el esquema de
§8.3.

## 1. El formulario es una definición declarativa en el Modelo

**Decisión.** Cada uno de los tres formularios (solista, ensamble, compositor) se describe
con una lista de `models.DefinicionCampo`: ruta canónica (`datos.telefono`,
`concierto.entidad`, `obra.titulo`), tipo, obligatoriedad, condición, límites y la tabla y
columna donde se guarda. `models.FormularioDe(tipo, subcategoría)` devuelve la definición.

De esa única definición salen:

- la validación de cada valor al guardar (`ValidarValorDeCampo`);
- la completitud al enviar (`ValidarParaEnvio`);
- el SQL del autoguardado (la columna sale de la definición, nunca de la petición);
- la descripción que la API entrega al frontal (`definicion`), con etiquetas traducidas.

**Motivo.** Con ~40 campos repartidos en cuatro tablas, escribir a mano la validación, la
completitud, el SQL y la interfaz de cada uno daría cuatro listas que se desincronizan. No
es una abstracción nueva —no hay interfaces ni genéricos—, es una tabla de datos, como
`rutasAPI`. Añadir un campo es añadir una fila (y su columna, por migración).

**Límite.** La definición **no** es el motor de extensión dinámica (`schema_extender.go`,
paso 10). Los campos añadidos por CLI no entran al formulario del postulante ni a
`ValidarParaEnvio`, como manda el perfil de dominio (§3.B).

## 2. Autoguardado: campos sueltos, lo válido se guarda y lo demás se señala

**Decisión.** `PATCH /api/v1/postulaciones/{id}` y `PATCH /api/v1/obras/{id}` reciben
`{"campos": {ruta: valor}}` con uno o varios campos. Cada campo se valida por separado:
los válidos se guardan y los rechazados vuelven en `errores`. Sólo si **ninguno** pudo
guardarse la respuesta es `422`.

Al guardar se valida el **formato** (una fecha es una fecha, una opción es del catálogo),
nunca la **obligatoriedad**: un campo vacío siempre se acepta. Lo que falta lo dice
`ValidarParaEnvio`.

**Motivo.** RF-POST-03 pide guardado parcial sin exigir que el resto esté completo. Si un
valor inválido tumbara toda la petición, un error en un campo impediría guardar los demás.
Un campo que no pertenece al formulario se rechaza con su nombre: ni se guarda ni se
ignora en silencio.

**El autoguardado no se audita.** Sí la creación de la postulación y el cambio de
subcategoría. Un asiento por tecla llenaría la bitácora de ruido; lo que tiene valor
probatorio es lo que se **envía**, y eso lo fija el snapshot del acuse (paso 6).

## 3. `ValidarParaEnvio` es la única fuente de la completitud

**Decisión.** `models.ValidarParaEnvio(postulación, configuración, titular, ahora)` es una
función pura que devuelve la lista de `Bloqueador`: campos obligatorios y condicionales
vacíos, valores guardados que dejaron de ser válidos, integrantes, ligas de video, número
y duración de las obras, documentos obligatorios según la dotación de **cada** obra, y el
correo de la cuenta sin verificar. Lista vacía = puede enviarse.

La consulta de una postulación devuelve siempre esa lista (`bloqueadores`), el conteo por
sección (`secciones`) y `lista_para_enviar`. El envío del paso 6 llamará a la misma
función dentro de su transacción.

**Motivo.** El indicador de avance del formulario, la pantalla de revisión previa y el
envío deben decir lo mismo. Con una sola función no pueden discrepar.

**Consecuencia hasta el paso 6.** La función ya exige los documentos, pero aún no pueden
subirse: ninguna postulación está «lista para enviar» todavía.

## 4. La captura exige etapa **y** fecha

**Decisión.** Una postulación se crea o edita sólo si la convocatoria está en etapa de
`postulacion` **y** el instante cae dentro del plazo de esa etapa en el calendario.

**Motivo.** El avance de etapa por calendario puede quedar detenido por una regla (falta
el revisor, ADR 0005). Sin la comprobación de fecha, ese retraso administrativo regalaría
horas o días de captura después del cierre publicado en las bases.

## 5. Lo ajeno responde 404

**Decisión.** Toda escritura filtra por `usuario_id` en la propia consulta SQL. La
postulación de otra cuenta responde `404`, igual que una que no existe. La ven además
`admin_sistema` y `coordinacion_nacional`, que no pueden editarla.

**Motivo.** Un `403` confirmaría que el identificador existe. Y el filtro en la consulta,
no en un `if` posterior, sigue el criterio de la «regla de oro» del perfil de dominio.

Jurados y revisores **no** ven postulaciones todavía: su acceso llega con sus etapas
(pasos 7 y 8), acotado a las postulaciones enviadas de su convocatoria.

## 6. Quién no puede postular

- **`coordinacion_nacional`** en ninguna convocatoria: es un perfil de supervisión de sólo
  lectura.
- **Quien es jurado o revisor de una convocatoria**, en esa convocatoria (complemento de
  la regla del ADR 0005, que impide asignar a quien ya postuló). En la otra convocatoria
  de la edición sí puede.

## 7. Cambiar de subcategoría borra los datos generales

**Decisión.** Pasar de solista a ensamble, o al revés, exige `"confirmar": true`, borra
los datos generales y los integrantes, y conserva la propuesta de concierto, las obras y
las ligas de video. Queda en la bitácora.

**Motivo.** Los dos formularios son distintos (RF-POST-02) y viven en tablas distintas.
Conservar a escondidas los datos del formulario anterior los dejaría fuera de la vista del
postulante, pero dentro de la base.

## 8. Catálogos fijos: países por código, entidades por clave

**Decisión.** País y nacionalidad se guardan como código ISO 3166-1 alfa-2; la entidad
federativa, como la clave de tres letras de ISO 3166-2:MX (`CDMX` para la Ciudad de
México). Ambos catálogos están en `models/catalogos.go` y se sirven en
`GET /api/v1/catalogos`. Si el país de residencia no es México, la entidad es texto libre.

**Los nombres de los países no están en el backend.** Son 249 por idioma; se resolverán
en el frontal (paso 11), a partir del código.

## 9. Ligas de video: por ahora, sólo forma

**Decisión.** En este paso una liga se acepta si es una URL `https` bien formada, sin
credenciales y sin repetirse; se detecta su plataforma y queda en estado `pendiente`. La
**comprobación en red** (que el video exista y sea público, RF-POST-07) pasa al paso 6.

**Motivo.** Esa comprobación hace que el servidor visite direcciones escritas por
cualquiera en Internet: exige defensas contra SSRF (resolver el nombre, rechazar
direcciones privadas, límites de tiempo y tamaño) y un trabajador en segundo plano. Es el
mismo tipo de cuidado que el almacén de archivos del paso 6, y se hará junto con él.
`ValidarParaEnvio` ya bloquea las ligas marcadas `inaccesible` o `no_publico`; las
`pendiente` no bloquean.

## 10. Primera medición de carga del autoguardado

El plan pedía medir la escritura concurrente al existir el autoguardado real. Medición
del 13 de noviembre de 2026, en proceso (sin red), 4 núcleos, base en disco temporal:

| | |
|---|---|
| Postulantes simultáneos | 60 |
| Peticiones (80 % autoguardado, 20 % lectura completa) | 3 000 |
| Errores | 0 |
| Peticiones por segundo | ~1 800 |
| Latencia p50 / p95 / p99 | 18 ms / 101 ms / 163 ms |

El volumen previsto es de ~250 postulaciones **al año**. El supuesto de una sola conexión
de escritura con WAL se sostiene con margen amplio. La prueba formal, con red y en el
VPS, sigue en el paso 14.
