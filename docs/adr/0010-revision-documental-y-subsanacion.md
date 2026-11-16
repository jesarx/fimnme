# ADR 0010 — Revisión documental y subsanación

- **Estado:** aceptada
- **Fecha:** 2026-11-13

Decisiones del paso 7. No añade dependencias. Dos migraciones nuevas, sólo de datos:
`0004` (tres plantillas de correo y el límite del audio) y `0005` (límites definitivos de
los documentos).

## 1. El desbloqueo selectivo es un solo objeto: `PermisoDeEdicion`

**Decisión** (§4.2, D-06). `models.PermisoDeEdicionDe(postulación, convocatoria, ahora)`
dice qué puede modificar el postulante en este momento:

| Estado | Qué puede modificar |
|---|---|
| Borrador, en etapa de postulación y en plazo | Todo |
| En subsanación, sin vencer | **Exactamente** los elementos listados en `subsanacion_problema` |
| Cualquier otro | Nada |

**Toda** escritura de la postulación pasa por él: `Repositorio.postulacionEditable` lo
calcula dentro de la transacción y cada método lo consulta antes de escribir —campos,
obras, integrantes, videos, documentos, cambio de subcategoría—. No hay un segundo camino:
es la forma de cumplir RN-12b («no existe otra vía de autorización»).

En el autoguardado, un campo no señalado se rechaza **él solo**: los demás campos de la
misma petición se guardan.

## 2. Qué se puede señalar

- **Un campo**, por su ruta: `datos.telefono`, `obra.duracion_min` (con su obra).
- **Un documento**, por la clave de su requisito, esté inválido o falte.
- **Una lista entera**: `integrantes`, `videos` u `obras`. Señalar `obras` es la
  «autorización explícita» de RN-12c: permite añadir, quitar y reordenar obras, y entonces
  también editar los datos y documentos de cada obra, porque de nada serviría poder añadir
  una obra y no poder describirla.

Los campos añadidos por extensión dinámica **no** pueden señalarse (perfil de dominio,
§3.B): no están en la definición del formulario.

## 3. «Solicitar subsanación» agrupa lo señalado

**Decisión** (RF-REV-05b). La petición lleva los campos erróneos y los documentos que
faltan; **los documentos que el revisor dictaminó inválidos se añaden solos**, con el
motivo de su dictamen. En una transacción: se crea la subsanación, la postulación pasa a
`en_subsanacion` y se encola **un solo correo** con la lista, el plazo y una nota libre
del revisor, en el idioma del postulante.

- **Fecha límite:** futura y a no más de 90 días.
- **Mientras el postulante corrige, el revisor espera:** no dictamina documentos, no abre
  otra subsanación ni dictamina la postulación.
- Los señalamientos de campos no se guardan antes de solicitar: viajan en la petición. El
  esquema no tiene dónde guardarlos sueltos, y no hace falta.

## 4. Solicitud de desbloqueo: concederla es añadir un problema

**Decisión** (RN-12d). El postulante puede pedir, con justificación, corregir algo no
señalado. Pedirlo no desbloquea nada. Si el revisor lo **concede**, el elemento se inserta
en `subsanacion_problema`: sigue habiendo una sola fuente de lo editable. Negar exige
motivo.

## 5. Reenvío: el mismo envío, con versión 2

**Decisión** (§4.1, RN-13). El reenvío usa `EnviarPostulacion`: la misma validación
completa, las mismas confirmaciones, la misma comprobación de ligas. Cambia que el folio
se conserva, el acuse lleva la versión siguiente (el anterior no se toca), la subsanación
queda atendida y la postulación vuelve a `en_revision`.

Sale un correo al postulante con el acuse nuevo adjunto (RF-NOT-06) y un aviso a cada
revisor activo de la convocatoria (RF-REV-07).

## 6. El diff sale de los snapshots

**Decisión** (RF-REV-07). `models.DiferenciasDeEnvio` compara los snapshots de los dos
últimos acuses: campos, listas, obras añadidas o retiradas y documentos (por la suma de su
archivo). Se compara lo que **de verdad se envió** cada vez, no el estado actual de la
base.

## 7. Estados y dictamen

- `enviada → en_revision`: la primera vez que un revisor actúa (toma, dictamina un
  documento u observa).
- **Sólo se revisa en la etapa de revisión documental.** Antes, el revisor ya ve lo
  enviado, pero no actúa.
- **Dictamen** (RF-REV-08): `validada` o `descartada`; descartar exige motivo. No se
  valida una postulación con un documento dictaminado inválido. Un dictamen puede
  corregirse mientras dure la etapa.
- **Vencimiento** (RF-REV-09): vencido el plazo, el postulante ya no puede editar ni
  reenviar; la postulación **no se descarta sola**, queda marcada (`vencida`) y el revisor
  decide.
- **Sin correo de dictamen.** La plataforma no comunica el resultado (D-09).

## 8. Qué ve cada quien

| | Estado | Subsanación abierta | Dictámenes, observaciones, motivo de descarte, diff |
|---|:--:|:--:|:--:|
| Titular | ✅ | ✅ | ❌ |
| Revisor de la convocatoria, administrador | ✅ | ✅ | ✅ |
| Observador | ✅ | ✅ | ✅ (sólo lectura) |
| Revisor de otra convocatoria | ❌ (404) | | |

El revisor ve las postulaciones **enviadas** de su convocatoria, nunca los borradores, y
puede descargar sus documentos (queda registrado). Las observaciones son internas.

**El descarte es un estado interno** (decisión de la CNMO del 13 de noviembre de 2026).
Al postulante no se le muestra: ni el motivo ni el estado. Su postulación descartada le
sigue apareciendo «en revisión», en el detalle y en su lista (`models.EstadoParaTitular`).
Los demás estados sí los ve. Quien revisa o supervisa ve el estado real.

## 9. Toma blanda

**Decisión** (RF-REV-01b, RN-14b). «La reviso yo» es una marca informativa: aparece en la
bandeja de los demás, pero cualquier revisor puede intervenir o tomarla. No hay cerrojo
que se quede trabado si alguien se ausenta.

## 10. Bandeja

**Decisión** (RF-REV-01 y 10). Todas las postulaciones enviadas de la convocatoria, sin
reparto, con filtros por estado, subcategoría, toma y texto (folio o nombre), y un tablero
con el total y la cuenta por estado. Los filtros se aplican en memoria: son unas centenas
de renglones. La búsqueda tolerante a acentos y erratas llega con el archivo (paso 9).

## 11. Límites de los documentos

Decisiones de la CNMO del 13 de noviembre de 2026:

| Documento | Máximo | Formato |
|---|---|---|
| Audio | **30 MB** | WAV, FLAC o MP3 |
| Partitura | 30 MB | PDF |
| Particellas | 30 MB | **Un solo PDF** con todas |
| Autorización | 10 MB | PDF |
| Fotografía | 5 MB | JPEG o PNG |

Las convocatorias nuevas nacen así. Para las existentes, la migración `0004` bajó el
audio de 100 a 20 MB (primera decisión) y la `0005` lo deja en 30 MB y fija las
particellas en un solo archivo, con su texto de ayuda. Todo sigue siendo configurable por
convocatoria.

**A tener presente:** en 30 MB caben unos 3 minutos de WAV estéreo con calidad de CD,
unos 6 de FLAC, y de 12 a 20 minutos de MP3 según su calidad.

## Lo que no se hizo

- **Catálogo de motivos frecuentes** (parte de RF-REV-03): el motivo es texto libre. El
  catálogo exige una tabla que la especificación no define; se añadirá con el panel de
  administración.
- **Editar la plantilla del correo antes de enviar**: el revisor añade una nota; el texto
  de la plantilla lo cambia el administrador (paso 13).
