# ADR 0013 — Reportes, expediente en PDF y tableros

- **Estado:** aceptada
- **Fecha:** 2026-11-14

Decisiones del paso 9b (§5.7, §5.12.4 y §5.13 de la especificación). No añade
dependencias. Añade la migración `0006`: la tabla `plantilla_reporte`, que la
especificación menciona sin definir, y la plantilla de correo `exportacion_lista`.

## 1. El expediente se recorta al generarlo, con dos cerraduras

**El requisito** (RF-ARCH-41, RN-26). El contenido depende de quién lo pide, y se decide al
generar el PDF: el archivo sale del sistema y después no hay forma de controlarlo.

**Decisión.**

| Quien lo pide | Recibe |
|---|---|
| Titular | Su postulación como está hoy. Nada interno: ni revisión, ni evaluaciones, ni resultado. Si fue descartada, la ve «en revisión» (ADR 0010, §8) |
| Jurado de la convocatoria | La postulación completa —la evaluación no es a ciegas— y **su** evaluación. Nunca la de otro jurado, ni la suma, ni el resultado, ni la revisión |
| Revisor de la convocatoria | La postulación, el dictamen de cada documento, las observaciones, las subsanaciones y el motivo de descarte. Ninguna evaluación |
| Administrador y observador | Todo: además, lo que calificó cada jurado, las excusas, la suma, el puntaje y el resultado del proceso |

Lo garantizan dos cerraduras, a propósito redundantes:

1. **Se carga sólo lo permitido.** `models.RecorteDelExpediente` dice qué partes puede ver
   la persona; el armador (`controllers/expediente.go`) lee únicamente esas. La evaluación
   propia sale de `JuradoRepo`, que no sabe leer la de otro jurado; las de todos, de
   `matrizDeSupervision`, que se niega si quien pide no supervisa.
2. **Se borra lo que sobre.** `models.RecortarExpediente` quita del resultado todo lo que
   el recorte no permite, antes de entregarlo a la vista. La vista no decide nada: pinta
   lo que recibe.

El expediente por convocatoria se genera en segundo plano. El trabajador no usa los
permisos que la persona tenía al pedirlo: los vuelve a leer (`SesionDeUsuario`) **al
generar**. Si entre tanto le retiraron la asignación, el expediente sale vacío. Una prueba
lo ejercita.

## 2. Una sola puerta a lo que cruza jurados

Hasta el paso 8, la matriz de resultados sólo se alcanzaba desde una ruta. Ahora la
necesitan también los reportes y el expediente. En vez de repetir la comprobación en cada
sitio, los controladores llegan a `Supervision()` únicamente por dos funciones:

- `matrizDeSupervision` — puntajes, observaciones y suma;
- `avanceDeJurados` — cuántas lleva cada jurado, sin puntajes (tablero).

Cada una **empieza** comprobando `models.PuedeSupervisar` sobre la persona para la que se
va a producir el dato. `TestSupervisionSoloTrasComprobar` analiza el código y falla si
`Supervision()` se llama desde cualquier otra función, o si alguna de las dos deja de
comprobar primero. La garantía ya no depende de que cada ruta nueva se acuerde de
envolverse: no hay otra puerta.

En el modelo nada cambia: `SupervisionRepo.Avance` sólo **cuenta** evaluaciones, y
`TestAislamientoEstructuralDelRepositorio` sigue vigilando que fuera de `JuradoRepo` y
`SupervisionRepo` no haya SQL sobre las tablas de evaluación.

## 3. El PDF: lo mínimo que hacía falta

El escritor propio (`internal/pdf`, ADR 0009) sólo sabía de texto. Se le añadió
exactamente lo que el expediente pide, y un documento que no lo usa —el acuse— sigue
saliendo byte por byte igual:

- **Imágenes JPEG** incrustadas tal cual (`DCTDecode`).
- **Marcadores** de navegación, uno por postulación (RF-ARCH-42).
- **Salto de página** y **pie por página**: cada página lleva el folio de su postulación,
  la leyenda `CONFIDENCIAL`, la fecha de generación y el nombre de quien lo generó
  (RF-ARCH-47), para que un PDF extraviado diga de quién era.
- **Índice al frente**: se escribe al final, cuando ya se sabe en qué página quedó cada
  postulación, y esas páginas se pasan al inicio.

**La fotografía** (`views/fotografia.go`) se reduce a 800 pixeles de lado y se recomprime
como JPEG de calidad media, como pide la nota técnica de §5.12.4: unas decenas de KB en
vez de megas. Con la biblioteca estándar:

- Antes de decodificar se leen sus dimensiones; una imagen de más de 60 megapixeles se
  rechaza (un archivo pequeño puede declarar un tamaño que agote la memoria).
- Se respeta la **orientación EXIF** (los teléfonos guardan la foto acostada y anotan el
  giro aparte). Se leen sólo los bytes necesarios, comprobando cada límite.
- Un PNG con transparencia queda sobre blanco.
- Si la fotografía no puede leerse, el expediente sale sin ella y lo dice: un archivo
  dañado no impide consultar lo demás.

El contenido de la postulación lo escribe la misma función que el acuse
(`escribirPostulacion`), para que ambos digan lo mismo de la misma manera.

**No se hizo:** anexar las partituras al final de cada obra (RF-ARCH-44, prioridad S). Es,
como advierte la especificación, la parte más delicada: obliga a leer y reescribir PDF
ajenos. Se deja para cuando haga falta; las partituras se bajan hoy con la descarga
masiva. Sí se hicieron las otras opciones de ese requerimiento que no cuestan nada: omitir
la fotografía (`fotografia=no`) y la **hoja de calificación en blanco** (`hoja=si`,
RF-ARCH-43).

## 4. Reportes: el catálogo sale del formulario

**El requisito** (RF-REP-01 a 05 y 08, §9).

**Decisión.**

- **Catálogo de campos** (`models.CatalogoDeReporte`). Los campos de la postulación y de
  la obra **no son una lista aparte**: salen de la definición del formulario, con sus
  mismas rutas (`datos.genero`, `obra.titulo`) y sus mismas etiquetas. Un campo nuevo del
  formulario queda disponible en los reportes sin tocar nada. Se suman los calculados
  (edad al cierre, totales, listas concatenadas), los de la revisión y los de la
  evaluación: un puntaje por jurado y criterio, sus observaciones y los agregados.
  Por eso algunas claves difieren del extracto de §9 (`datos.nombre_ensamble` en vez de
  `ensamble.nombre`): se prefirió una sola nomenclatura en todo el sistema.
- **Granularidad** por postulación o por obra. Un campo de obra sólo cabe en un reporte
  por obra. Los campos de la evaluación —y el rango de puntaje— piden la granularidad de
  **lo que el jurado califica**: la postulación en Intérpretes, la obra en Compositores
  (RN-16). La suma de una obra no es un dato de la postulación entera; el sistema lo dice
  en vez de inventar un promedio.
- **Filtros**: los mismos del listado del archivo (ADR 0012), más el rango de puntaje.
  **Orden**: los del archivo, o por puntaje.
- **CSV**: UTF-8 con BOM, comillas RFC 4180, separador coma o punto y coma. Los valores
  van en claro («Jalisco», «Estreno mundial», «Validada»), en español o en inglés.
- **Plantillas** (RF-REP-04): un nombre, el tipo de convocatoria y los parámetros del
  reporte, **sin la convocatoria**. Así la plantilla de este año sirve el siguiente. Los
  parámetros de la petición mandan sobre los de la plantilla. Las comparten quienes
  generan reportes.
- **Reporte de resultados** (RF-REP-08): la matriz en CSV
  (`/resultados?formato=csv`). **Los empates comparten lugar** y el siguiente se salta
  (1, 2, 2, 4); lo que nadie ha calificado no tiene lugar. La matriz en JSON usa la misma
  regla (`models.LugaresDeLaMatriz`).
- Sólo administrador y observador (§2.2). Cada reporte queda en la bitácora: qué campos y
  cuántos renglones, ningún valor.

**Se generan al momento**, no en segundo plano (RF-REP-06, prioridad S): con unas
doscientas cincuenta postulaciones por convocatoria tardan menos de un segundo. No hay
XLSX (prioridad C).

## 5. Tableros: cifras que llevan a su listado

- **Tablero de la convocatoria** (RF-DASH-01 y 02): postulaciones por estado, primeros
  envíos por día, documentos por dictaminar, subsanaciones abiertas, por vencer (menos de
  tres días) y vencidas, y avance de cada jurado, sin puntajes.
- **Estadísticas de la edición** (RF-DASH-03): por convocatoria, entidad, país,
  nacionalidad, género, rango de edad, dotación y tipo de estreno, y duración media de
  las obras. Cuentan las postulaciones **enviadas**; los borradores no. La edad se calcula
  al cierre de la etapa de postulación, igual que en los reportes, y se cuenta por rangos
  en Go porque la aritmética de fechas difiere entre SQLite y MariaDB.
- **Cada cifra es navegable** (RF-DASH-05): trae `filtro`, la consulta de
  `/api/v1/archivo` que lista lo que cuenta. Es `null` donde el listado no tiene un filtro
  equivalente (rangos de edad, subsanaciones por vencer).

No se hizo la comparativa entre ediciones (RF-DASH-04, prioridad C): con las estadísticas
de cada edición, la interfaz puede ponerlas lado a lado.

## 6. Aviso al terminar

Las tareas en segundo plano —ZIP y expediente por convocatoria— avisan por correo a quien
las pidió cuando ya pueden bajarse (RF-ARCH-45), con el enlace y su vigencia. El aviso se
encola en la misma transacción que marca la tarea como lista. Si el aviso no puede
componerse, la tarea termina igual: es una cortesía.

## Consecuencias

- Con esto el backend de consulta está completo; las pantallas del paso 13 sólo pintan.
- El expediente por convocatoria se compone en memoria. Doscientas postulaciones con
  fotografía ocupan del orden de 20 MB; si se anexaran partituras habría que escribirlo
  por partes.
