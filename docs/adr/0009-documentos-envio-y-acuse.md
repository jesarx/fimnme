# ADR 0009 — Documentos, envío y acuse

- **Estado:** aceptada
- **Fecha:** 2026-11-13

Decisiones del paso 6. No añade dependencias. Una migración nueva (`0003`), sólo de
datos: las plantillas de correo del acuse y del recordatorio.

## 1. Los archivos viven en disco, direccionados por su SHA-256

**Decisión.** `models.BlobStore` es la interfaz del almacén (§7.8) y `AlmacenEnDisco` su
única implementación: cada contenido se guarda en `blobs/ab/cd/abcd…`, donde `abcd…` es
su SHA-256. La base sólo guarda metadatos (tabla `archivo`) y una **clave abstracta**,
nunca una ruta.

- **Subida al vuelo.** El contenido va de la petición a un temporal dentro del almacén
  mientras se calcula la suma; al terminar se mueve a su lugar con un cambio de nombre,
  que es atómico. Nunca se carga entero en memoria. Si la subida se corta o rebasa el
  tamaño, el temporal se borra.
- **Deduplicación** (RF-DOC-02): el mismo contenido subido dos veces ocupa una vez.
- **Inmutable**: un archivo guardado queda de sólo lectura (`0440`) y no cambia jamás. El
  respaldo incremental es trivial: sólo aparecen archivos nuevos.

**Por qué una interfaz aquí**, cuando la guía pide no abstraer sin necesidad: la
especificación la exige de forma explícita, para que mudar el acervo a S3 o MinIO no
toque el esquema.

## 2. El formato se reconoce por la firma, antes de escribir en disco

**Decisión.** `models.DetectarMIME` mira los primeros bytes del contenido (RF-DOC-01). El
nombre y lo que diga el navegador no cuentan. Además, la extensión debe corresponder al
contenido: un PDF llamado `foto.jpg` se rechaza.

El controlador lee esos primeros bytes **sin consumirlos** y valida formato y extensión
antes de guardar nada: un archivo de formato equivocado no llega a tocar el disco.

Formatos admitidos: PDF, JPEG, PNG, WAV, FLAC y MP3. Sólo se comprueba la firma, no que
el archivo completo sea válido: un PDF dañado pasa, y lo detectará el revisor.

**No hay antivirus** (RF-DOC-05, prioridad S). La columna `escaneo_av` queda en
`pendiente`.

## 3. Nada se sobrescribe: versiones

**Decisión** (RF-DOC-07). Reemplazar un documento crea otro con la versión siguiente y
deja el anterior como no vigente, con `reemplaza_a`. Quitar un documento también lo deja
no vigente. El archivo nunca se borra.

- Si el requisito admite **un** archivo y ya hay uno, la subida nueva lo reemplaza sin
  preguntar: es lo que espera quien vuelve a subir su partitura.
- Si admite **varios** y ya están todos, hay que indicar cuál se reemplaza.
- **Tope de 30 versiones** por requisito. Como nada se borra, sin tope una cuenta podría
  llenar el disco.

**Excepción.** Al quitar una **obra** de un borrador, sus documentos se van con ella (la
base los borra en cascada; los archivos siguen en el almacén). El registro de sus
descargas se conserva, ligado a la postulación.

## 4. Los nombres de descarga se generan; el original no se muestra

**Decisión.** Un documento se descarga como `FIM49-INT-0137_partitura_obra2_v1.pdf`
(folio, requisito, obra, versión). Antes del envío, `borrador-N` en lugar del folio.

**Motivo.** Es la «nomenclatura predecible» que pide el archivo histórico. Y hay una razón
de privacidad: la tabla `archivo` se deduplica por contenido, así que dos personas que
suban el mismo archivo comparten registro; mostrar el nombre original enseñaría a una el
nombre que le puso la otra. El nombre original se guarda sólo como referencia interna, y
la bitácora registra la suma, no el nombre.

## 5. Toda descarga pasa por la aplicación y se registra

**Decisión** (RF-DOC-03, RN-27). No hay ruta estática al acervo. Cada descarga comprueba
el permiso sobre la postulación y deja una fila en `descarga` (cuenta, IP, momento) y un
asiento en la bitácora, en la misma transacción.

- Por omisión el archivo se muestra en el navegador (`inline`); con `?descargar=1`, como
  descarga. Se sirve por rangos, para el reproductor de audio.
- De una lectura por rangos **sólo el primer trozo cuenta** como descarga: un visor pide
  el mismo archivo en decenas de trozos.
- Hoy descargan el titular, el administrador y el observador. Revisores y jurados, en los
  pasos 7 y 8.

**Pendiente para el paso 12.** La CSP actual impide incrustar un PDF o un audio en una
página propia (`frame-ancestors 'none'`, sin `media-src`). Habrá que abrirla sólo para
estas rutas cuando exista el visor.

## 6. El envío es una sola transacción

**Decisión** (§4.1). `Repositorio.EnviarPostulacion` hace todo o nada:

1. comprueba que sea un borrador de la cuenta, dentro de plazo;
2. ejecuta `ValidarParaEnvio` y exige las dos confirmaciones;
3. asigna el folio y congela lo enviado en un snapshot JSON;
4. pide a la Vista el acuse en PDF y el correo;
5. guarda el acuse, marca la postulación como `enviada`, encola el correo con el PDF
   adjunto y deja el asiento en la bitácora.

**El Modelo no conoce la Vista.** El repositorio recibe del controlador una función
`componer(snapshot)` que devuelve el PDF ya guardado en el almacén. Así el acuse se compone
*dentro* de la transacción —si falla, no hay folio ni envío— sin que `models/` importe
`views/`.

**Folio:** `FIM49-INT-0137`: edición, convocatoria y consecutivo por convocatoria. Se
asigna en el primer envío y no cambia (RN-13). Con una sola conexión de escritura no hay
dos envíos que lean el mismo consecutivo.

**El correo no se envía en la transacción:** se encola, con el acuse como adjunto
(`email_outbox.adjuntos`).

## 7. El acuse sale únicamente del snapshot

**Decisión** (§5.10). `views.AcusePDF` compone el PDF a partir del snapshot y de nada más:
no consulta la base ni el reloj. El mismo snapshot produce **el mismo archivo, byte por
byte**. Años después puede regenerarse idéntico, aunque la postulación haya cambiado por
una subsanación.

El escritor está en `internal/pdf`, con la biblioteca estándar (~300 líneas):

- Fuentes base del formato (Helvetica), sin incrustar; sin imágenes. La identidad es una
  banda del color del Foro, dibujada.
- Contenido comprimido con Flate. **Un acuse típico pesa 3 KB**; uno de 40 obras, menos
  de 100 KB. El límite duro de 300 KB se comprueba al guardarlo.
- Texto en WinAnsiEncoding, que cubre el español y el inglés.

**Límite conocido.** Las letras latinas fuera de esa codificación se escriben sin
diacrítico («Dvořák» → «Dvorák») y lo que no es alfabeto latino, como «?». El snapshot
conserva siempre el texto exacto; el PDF es su representación legible. Si llegan
postulantes con nombres en otros alfabetos habrá que incrustar una fuente (y aceptar un
acuse mayor).

**Países por código.** El acuse imprime «MX», no «México»: los nombres de los países aún
no están en el backend (ADR 0007, §8).

## 8. Ligas de video: comprobación en red, con dos reglas opuestas

Comprobar una liga hace que el **servidor** visite una dirección escrita por cualquiera.

**Regla 1: que no sirva para atacar la red interna (SSRF).**

- Sólo `https` y sólo el puerto 443.
- La dirección IP se comprueba **al conectar**, después de resolver el nombre: no sirve
  registrar un dominio que apunte a una dirección interna.
- Se rechazan direcciones privadas, de bucle, de enlace local, de metadatos de nube,
  compartidas de operador y reservadas, en IPv4 e IPv6.
- Las redirecciones pasan por el mismo filtro; son cinco como máximo.
- No se usa ningún proxy del entorno. Plazo de 10 segundos. Del cuerpo no se lee nada.

**Regla 2: que un fallo del servidor no impida enviar.** Sólo se da por mala una liga
cuando la respuesta es inequívoca:

| Respuesta | Estado |
|---|---|
| 2xx | `ok` |
| 404, 410 (o 400 de la plataforma) | `inaccesible` — **bloquea el envío** |
| 401, 403 de la plataforma, o redirección a iniciar sesión | `no_publico` — **bloquea el envío** |
| Dominio inexistente, **si el servidor resuelve otros nombres** | `inaccesible` |
| Todo lo demás: 403 a un programa, 5xx, sin respuesta, sin DNS | `pendiente` — no bloquea |

A YouTube, Vimeo y SoundCloud se les pregunta por **oEmbed**, que distingue un video
privado de uno público; a los demás sitios se les pide el recurso.

**Hallazgo de la prueba en vivo.** En un servidor cuyo DNS responde «no existe» a todo,
la primera versión dio por inexistente una liga válida y bloqueó el envío. Por eso «el
dominio no existe» sólo se cree si el servidor resuelve algún dominio testigo; si no, la
liga queda pendiente. Un servidor sin salida a Internet no impide ningún envío.

**Cuándo se comprueba:** en segundo plano al capturarla; todas, justo antes de un envío
(RN-08), fuera de la transacción; y una vez al día mientras la convocatoria está en
postulación o en revisión documental. La comprobación es un **semáforo para el revisor**,
no un veredicto (nota a RF-DOC-10).

## 9. Recordatorios de borrador sin tabla nueva

**Decisión** (RF-NOT-04). A 7 días, 2 días y 12 horas del cierre se recuerda a quien tiene
un borrador. Cada recordatorio vale desde su plazo hasta el siguiente; «ya se envió» se
deduce de la propia bandeja de salida (un correo de esa plantilla para esa postulación
creado dentro de la ventana). No hace falta una tabla que lleve la cuenta.

Un recordatorio cuya ventana pasó con el servicio detenido no se envía tarde.

## 10. Bases en PDF e imagen de cabecera

**Decisión** (RF-CONV-02, pendiente del paso 4). El administrador carga las bases en PDF,
una por idioma, y una imagen de cabecera. Se sirven en
`/convocatorias/{edicion}/{tipo}/bases.pdf` e `/imagen`, sin sesión y sin registro: son
material de difusión, no del acervo. Si no hay versión inglesa, la página en inglés
ofrece la española y lo avisa.
