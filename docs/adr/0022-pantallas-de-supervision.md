# ADR 0022 — Pantallas de supervisión: tablero, resultados, reportes, estadísticas y nómina

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 13b (§5.6, §5.7, §5.13, §5.16; RF-EVA-08 y 09, RF-REP-01 a 08,
RF-DASH-01 a 05, RF-PUB-03 a 05). Segunda de las tres entregas del paso 13. No añade
dependencias, migraciones ni operaciones de la API.

## 1. Rutas

| Ruta | Qué es | Quién |
|---|---|---|
| `GET /supervision` | Ediciones y convocatorias, hacia su tablero | supervisión |
| `GET /supervision/convocatorias/{id}` | Tablero | supervisión |
| `GET /supervision/convocatorias/{id}/resultados` | Matriz de resultados | supervisión |
| `POST /supervision/postulaciones/{id}/dictamen-final` | Resultado del proceso | administrador |
| `GET /supervision/convocatorias/{id}/reporte` (`?plantilla=`) | Generador de reportes | supervisión |
| `GET /supervision/convocatorias/{id}/reporte.csv` | Descarga del reporte | supervisión |
| `POST …/reporte/plantillas` · `…/reporte/plantillas/{p}/borrar` | Plantillas de reporte | supervisión |
| `GET`, `POST /supervision/convocatorias/{id}/nomina` | Nómina: vista previa, publicación y retiro | administrador |
| `GET /supervision/ediciones/{id}/estadisticas` (`?convocatoria=`) | Estadísticas | supervisión |

Las páginas de una convocatoria comparten pestañas (Tablero, Resultados, Reportes y, para
el administrador, Nómina). Son páginas, no API.

## 2. Una sola puerta a lo que cruza a los jurados

El tablero usa `avanceDeJurados` y la matriz `matrizDeSupervision`, las mismas puertas
que la API, el expediente y los reportes, que comprueban ahí mismo el perfil. La prueba
de estructura `TestSupervisionSoloTrasComprobar` sigue en verde: ninguna pantalla nueva
llama a `Supervision()`. **El tablero no muestra ningún puntaje ni observación** (sólo
cuántas lleva cada jurado de cuántas); una prueba lo comprueba.

## 3. Tablero

Postulaciones por estado —las de la revisión llevan a la bandeja del revisor filtrada
(RF-DASH-05)—, documentos por dictaminar, subsanaciones abiertas, por vencer y vencidas,
avance de cada jurado y ritmo de envíos por día de la Ciudad de México. El ritmo se
calcula con `views.EnviosPorDia`, que ahora usa también la API.

Las barras son `<meter>` y `<progress>`: la CSP no admite `style=""`, y estos elementos
dibujan una proporción sin estilos en línea ni JavaScript, y se leen bien en una tabla.

## 4. Matriz de resultados y dictamen final

Lugar (los empates lo comparten), postulante, **puntaje comparable, suma, promedio y
resultado primero**, y después lo de cada jurado por criterio, con la excusa a la vista.
El orden de las columnas se eligió en el navegador: con las de los jurados primero, lo
importante quedaba fuera de la pantalla. Las observaciones de los jurados van plegadas
bajo el folio. Se descarga en CSV con el reporte de resultados de la API.

El **resultado del proceso** lo captura el administrador con la convocatoria en evaluación
o cerrada; en Compositores, donde hay una fila por obra, el formulario aparece una vez
por postulación. Queda en la bitácora y no se comunica al postulante. Un rechazo vuelve
junto a su fila.

## 5. Generador de reportes

Un formulario con el catálogo de la convocatoria agrupado por nivel (postulación, obra,
evaluación; ésta sólo desde la evaluación), granularidad, orden, separador, idioma y
filtros. Se manda por **GET a `reporte.csv`** y el navegador descarga el archivo; las
casillas llegan como varios `campos` y se juntan con comas antes de llamar a
`GenerarReporte`, el mismo generador de la API y de `fimnme-cli export-csv` (mismas
reglas, mismo alcance, misma bitácora). Si el reporte no procede, vuelve el formulario con
el motivo y lo elegido, no un JSON.

**«Guardar como plantilla»** es un segundo botón del mismo formulario (`formmethod=post`):
guarda la definición sin el nombre ni la convocatoria, después de comprobar que el reporte
procede. Las plantillas del tipo de la convocatoria se listan con Descargar, Cargar (llena
el formulario) y Borrar (el administrador cualquiera; el observador, las suyas). Todo
funciona sin JavaScript.

## 6. Estadísticas

Las distribuciones de la edición —o de una de sus convocatorias— como tablas con barras:
convocatoria, entidad, país, nacionalidad, género, edad al cierre, dotación y tipo de
estreno, más la duración media de las obras. Los valores se traducen (países, entidades,
opciones); lo vacío aparece como «Sin dato».

## 7. Nómina

Sólo el administrador. Muestra **exactamente lo que saldría al portal** —las seleccionadas
con nombre artístico, subcategoría, semblanza y títulos— y las publicaciones anteriores
con quién las autorizó y su SHA-256. Publicar o retirar exige marcar la confirmación y la
convocatoria cerrada. La publicación se extrajo a `publicarNomina`, que comparten la API
y la página.

## 8. Defectos encontrados en el navegador

- **Las tablas desplazables ensanchaban la página en el teléfono.** Los textos `.oculto`
  (posición absoluta) dentro de una tabla se posicionaban respecto de la página. Se
  corrige con `position: relative` en `.tabla-contenedor`; afectaba también, en potencia,
  a las tablas de pasos anteriores.
- La página de la nómina formateaba la fecha con una página sin motor de plantillas
  (pánico): ahora la plantilla recibe el instante y lo formatea ella.
