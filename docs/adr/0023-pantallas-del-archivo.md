# ADR 0023 — Pantallas del archivo: buscador, vistas, descargas, expedientes y enlace por folio

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Decisiones del paso 13c (§5.12; RF-ARCH-06, 10 a 18, 30 a 45; RNF-17). El paso 13 se
divide en cuatro entregas: 13a administración, 13b supervisión, **13c archivo** y 13d
operación (bitácora, importación asistida, plantillas de correo y anonimización). Sin
dependencias ni migraciones; no cambia la API: añade una consulta al repositorio
(`ExportacionesDe`).

## 1. Rutas

| Ruta | Qué es |
|---|---|
| `GET /archivo` | Buscador: texto, filtros, orden, páginas; vistas guardadas; CSV; acciones |
| `POST /archivo/vistas` · `/archivo/vistas/{v}/borrar` | Guardar y borrar una vista propia |
| `POST /archivo/descargas` | Descarga masiva en ZIP de la búsqueda (supervisión y revisores) |
| `POST /archivo/expedientes` | Expediente en PDF de una convocatoria |
| `GET /archivo/descargas` | Las descargas y expedientes que pidió la cuenta |
| `GET /postulaciones/{folio}` | Enlace permanente, ahora también para el personal |

## 2. Un solo alcance

El buscador usa `ListarPostulaciones` con `AlcanceDe(sesión)`, igual que la API: la
supervisión ve todo (borradores incluidos), cada revisor las enviadas de sus
convocatorias y cada jurado sólo lo que pasó a evaluación en las suyas, este año o dentro
de diez. Las opciones de los filtros (ediciones, convocatorias, tipos de documento) salen
de las convocatorias visibles. El menú ofrece «Archivo» a quien tiene algo que consultar.

## 3. El buscador

- Texto (folio, nombres, ensamble, integrantes, obra, compositor, correo; sin acentos y
  por fragmento) y los filtros principales a la vista; los demás, plegados en «Más
  filtros», que se abre solo si alguno está en uso.
- Con htmx, al cambiar un filtro se reemplaza el resultado y la URL se actualiza; los
  **filtros vacíos no viajan** (`fimnme.js`), de modo que la URL es corta y la misma que
  daría el formulario. Sin JavaScript, el botón «Buscar».
- Todo lo que depende de la búsqueda —CSV, guardar vista, ZIP, expediente— vive dentro
  del resultado, para que lleve siempre los filtros vigentes.
- Un filtro inválido se explica arriba del resultado (422), por nombre de filtro.
- Cada folio enlaza a su **enlace permanente**; un borrador (sin folio) sólo tiene su
  expediente.

## 4. Vistas guardadas

La ruta de una vista se arma en el servidor a partir de los filtros —sin la página— y
siempre dentro de `/archivo`: lo que llega en el formulario no puede convertirse en un
enlace a otra parte (una mutación que lo permitía hizo fallar las pruebas). Las vistas
guardadas desde la API (`/api/v1/archivo?…`) se muestran como su equivalente en pantalla.

## 5. Descargas y expedientes

Las mismas tareas en segundo plano que la API: el ZIP con `parametrosDeDescargaMasiva`
(compartido) y el expediente con el alcance que la persona tenga al generarse. «Mis
descargas» lista sólo las propias y se actualiza sola cada cinco segundos (htmx) mientras
alguna se arma; se bajan por la ruta de la API, que las registra y vence a los siete días.

## 6. Enlace permanente para el personal

`/postulaciones/{folio}` llevaba sólo al titular. Ahora, a quien no lo es, lo lleva a la
vista de revisión si puede verla (supervisión y revisores de la convocatoria) o a la
ficha de evaluación si es jurado y la postulación llegó a evaluación (en Compositores, la
primera obra). En cualquier otro caso, 404, como si no existiera.
