# ADR 0019 — Pantallas del revisor y visor de documentos

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 12a (§5.5, RF-REV-01 a 09; RF-DOC-08). El paso 12 se parte en dos
entregas: **12a** (esto: el revisor documental y el visor) y **12b** (el jurado: lista de
trabajo, ficha de evaluación y excusa, que reutilizan el visor). No añade dependencias,
migraciones ni operaciones de la API.

## 1. Rutas

| Ruta | Qué es |
|---|---|
| `GET /revision` | Las convocatorias que la sesión revisa (o todas, si supervisa) |
| `GET /revision/convocatorias/{id}` | La bandeja, con tablero por estado y filtros en la URL |
| `GET /revision/postulaciones/{id}` | La vista de revisión de una postulación |
| `POST /revision/postulaciones/{id}/toma` | «La reviso yo» o la libera |
| `POST …/documentos/{documento}/dictamen` | Válido, inválido (con motivo) o sin dictaminar |
| `POST …/observaciones` | Nota interna, de la postulación o de una obra |
| `POST …/subsanacion` | Pide una subsanación |
| `POST …/desbloqueos/{solicitud}` | Concede o niega una solicitud de desbloqueo |
| `POST …/dictamen` | Validada o descartada |

Cada acción llama a **la misma operación del repositorio que la API**; el envío del
correo de subsanación pasó a una función compartida (`solicitarSubsanacion`). Son
formularios normales: el resultado vuelve a la vista con un aviso o, si el modelo lo
rechaza, con el motivo junto al formulario que lo produjo. No hace falta JavaScript
salvo para filtrar la bandeja sin recargar.

## 2. Quién ve y quién actúa

- **Ver**: los revisores de la convocatoria y quien supervisa (`PuedeVerRevision`). El
  titular, un revisor de otra convocatoria o cualquier otra persona reciben 404, igual que
  si no existiera. Un borrador no aparece: no entró al proceso.
- **Actuar**: el revisor asignado y el administrador (`PuedeRevisar`). El observador ve
  la vista entera **sin un solo formulario**, y una acción suya directa recibe 403.
- El menú ofrece «Revisión» a quien revisa alguna convocatoria o supervisa
  (`PuedeEntrarARevision`).
- El revisor sí ve el descarte y su motivo; el titular sigue viendo «en revisión».

## 3. Bandeja

Todas las postulaciones enviadas, sin reparto entre revisores (RN-14b). Arriba, una cifra
por estado que lleva al filtro. Los filtros —estado, subcategoría, quién la tomó, folio o
nombre— son un formulario GET: la bandeja filtrada es un enlace. Con htmx se aplican al
cambiar un filtro o al escribir, y sólo se sustituye el resultado (`hx-select`), con la
URL al día (`hx-push-url`). Cada renglón dice si se subsanó y hay que volver a mirarla,
si el plazo de una subsanación venció, cuántos documentos inválidos y cuántas
observaciones tiene.

## 4. Vista de revisión

La postulación entera en sólo lectura —la misma vista de cada paso del postulante, sin
permisos de edición—, más lo propio de la revisión: quién la tomó, lo que cambió en el
último reenvío, los documentos con su visor y su dictamen, las observaciones internas,
las subsanaciones con sus solicitudes de desbloqueo y el dictamen. Un índice lateral
lleva a cada parte.

**Subsanación.** Seis renglones de «elemento + qué debe corregir», con todos los campos,
listas y documentos nombrados por paso y por obra; los vacíos no cuentan, y los
documentos dictaminados como inválidos se suman solos con su motivo. La fecha límite se
escribe en hora de la Ciudad de México, con una propuesta de diez días. Mientras hay una
subsanación abierta no se ofrece otra. Si hacen falta más de seis elementos, se pide otra
subsanación cuando el postulante atienda la primera, o se usa la API.

## 5. Visor de documentos (RF-DOC-08)

Para que revisor y jurado no tengan que descargar nada:

- **PDF**: en un marco, con el visor del propio navegador (sin pdf.js, §7.4). Va dentro
  de un `<details>`: el marco, con `loading="lazy"`, no pide el archivo hasta que se abre.
- **Audio**: el reproductor del navegador, con `preload="none"`; la aplicación sirve el
  archivo por rangos, así que se puede saltar a la mitad sin bajarlo entero.
- **Imagen**: la imagen misma.
- Siempre, enlaces para abrir en otra pestaña y para descargar.

Toda consulta pasa por la aplicación, con el permiso comprobado, y la primera lectura
queda registrada como descarga (RN-27).

**Cabeceras.** La CSP de las páginas admite marcos y audio **del propio origen**
(`frame-src 'self'; media-src 'self'`). Un documento se sirve con su propia CSP
(`default-src 'none'; object-src 'self'; frame-ancestors 'self'`) y
`X-Frame-Options: SAMEORIGIN`: la plataforma puede mostrarlo en un marco y ningún otro
sitio. Las páginas siguen sin poder enmarcarse (`frame-ancestors 'none'`, `DENY`). Una
prueba vigila las dos.

**Límite conocido.** Que el navegador dibuje el PDF dentro del marco depende de que tenga
visor de PDF; el Chromium sin interfaz de este entorno no lo trae, así que se verificó que
el marco pide el documento y lo recibe con las cabeceras correctas, no que lo dibuje. Si
un navegador no lo muestra, quedan los enlaces.
