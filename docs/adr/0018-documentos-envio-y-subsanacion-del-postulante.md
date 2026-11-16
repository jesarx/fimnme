# ADR 0018 — Documentos, envío y subsanación en las pantallas del postulante

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 11b (RF-POST-07, 09 a 11, 13 y 14; RF-DOC-01 a 04 y 07). Completa,
con el 11a (ADR 0017), las pantallas del postulante. No añade dependencias, migraciones
ni operaciones de la API.

## 1. Un defecto de la subida, corregido

**Lo que pasaba.** Desde el paso 6, **toda subida de más de 1 MiB se rechazaba** como
«demasiado grande»: el límite general de los formularios (1 MiB) envolvía el cuerpo de
la petición, y la subida lo envolvía después con su propio límite, más amplio. Envolver
un límite con otro más amplio no lo amplía: seguía mandando el de 1 MiB. Una partitura
real no habría podido subirse. Las pruebas no lo vieron porque usaban archivos de unos
cuantos bytes; apareció al probar la página con archivos de tamaño real.

**Corrección.** El middleware guarda el cuerpo original en el contexto de la petición, y
sólo `cuerpoDeSubida` —que recibe el límite del requisito— lo recupera. Los demás
manejadores siguen sin poder leer más de 1 MiB. Una prueba nueva
(`TestSubidaDeArchivosGrandes`) sube una partitura de 25 MB, rechaza una de 31 MB y
comprueba que un formulario de 2 MiB sigue rechazándose; sin la corrección, falla.

## 2. La API y las páginas comparten la subida y el envío

La subida de un documento y el envío de una postulación estaban dentro de sus manejadores
de la API, que respondían JSON. Pasaron a dos funciones sin respuesta
(`subirDocumento`, `enviarPostulacion`) que devuelven un error; la API lo traduce a JSON
y la página a HTML. Lo mismo `abrirSubida` y `guardarSubida`, que antes respondían
ellas mismas. El comportamiento de la API no cambia (sus pruebas siguen iguales).

## 3. Documentos

- La página agrupa lo que se pide **de la postulación** y **de cada obra**, según la
  dotación de cada una (RF-DOC-04). Cada ranura dice formato y tamaño máximo, muestra lo
  entregado (con descarga, tamaño y fecha) y ofrece subir.
- **Subida con barra de progreso** (RF-POST-07): un formulario multipart normal. Con htmx,
  `fimnme.js` llena un `<progress>` con el avance de la petición; sin JavaScript se envía
  con su botón y el navegador muestra su propio avance.
- Si el requisito admite un solo archivo, el nuevo **reemplaza** al anterior, que se
  conserva como versión anterior (RF-DOC-07). Si admite varios, hay lugar mientras no
  estén todos.
- Un error (formato, tamaño) se dice en la ranura misma, y el foco va ahí.
- Retirar un documento no lo borra: deja de estar vigente.

## 4. Revisión y envío, y el estado de la postulación

El último paso es dos pantallas en una:

- **Antes de enviar** (RF-POST-09 y 10): la postulación completa en sólo lectura, paso por
  paso, con un enlace para modificar cada uno; la lista de todo lo que falta, con un
  enlace al control exacto; las dos declaraciones (no podré modificarla; acepto las
  bases), y el botón, deshabilitado mientras falte algo. El servidor vuelve a comprobarlo
  todo al enviar.
- **Después** (RF-POST-11 y 12): folio, fecha de envío, un texto sobre en qué va y los
  acuses descargables. El enlace del correo del acuse (`/postulaciones/{folio}`) lleva
  aquí. Los demás pasos siguen abiertos en sólo lectura.

Qué se le dice en cada estado sigue las decisiones ya tomadas: el descarte no se muestra
(aparece «en revisión»), y a partir de la validación se dice que el resultado se comunica
por los medios oficiales del Foro, porque la plataforma no emite comunicaciones después de
la revisión documental.

## 5. Subsanación (RF-POST-13 y 14)

- En **todos los pasos**, un aviso dice que hay elementos por corregir, hasta cuándo, y
  enlaza a la lista.
- Lo que puede modificarse se **resalta**, con el motivo que dio el revisor junto al
  control («Por corregir. Falta la clave lada.»); lo demás se muestra en sólo lectura.
  Qué es editable lo decide `models.PermisoDeEdicion`, igual que en la API.
- El paso de envío lista cada elemento señalado con su motivo y un enlace a él, y ofrece
  el mensaje completo de la revisión, plegado (es el texto del correo, con la nota del
  revisor).
- **Solicitud de desbloqueo**: un formulario con la lista de lo que hoy no puede
  modificarse —campos, listas y documentos, nombrados por paso y por obra— y la
  justificación. Las solicitudes aparecen con su estado (pendiente, concedida, negada) y
  la respuesta. Los campos extendidos no se ofrecen: no se desbloquean (RF-CLI-26).
- **Reenvío** con la misma confirmación y un nuevo acuse.

## 6. Fechas sin hora

Las fechas de nacimiento y de concierto se muestran como «17 de mayo de 1990», sin
convertirlas de zona horaria: un día del calendario es el mismo en todas, y convertirlo lo
habría corrido al día anterior.

## Verificación

Pruebas de extremo a extremo de las tres pantallas por sus dos caminos (htmx y sin
JavaScript), comprobando en la base lo guardado: subidas aceptadas y rechazadas, una de 3
MB desde la página, retiro, documentos ajenos, envío incompleto, sin declaraciones y
completo, estado, acuse descargable, enlace por folio, sólo lectura tras el envío, y una
subsanación entera (lo resaltado, lo bloqueado, la solicitud de desbloqueo y el reenvío).
En un navegador real (Playwright), el recorrido completo a 1280 y a 390 px, con archivos
de 2 y 12 MB y la barra de progreso, una revisora que pide la subsanación y la postulante
que la atiende; y la subida sin JavaScript en modo oscuro.
