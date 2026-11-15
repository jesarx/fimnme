# ADR 0021 — Pantallas de administración: ediciones, convocatorias, jurados y cuentas

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 13a (§5.2, RF-CONV-01 a 09, RF-AUT-07). Es la primera de tres
entregas del paso 13: la 13b trae la supervisión (tablero, matriz, reportes, estadísticas
y publicación de la nómina) y la 13c el archivo, la bitácora y la importación asistida.
No añade dependencias, migraciones ni operaciones de la API.

## 1. Rutas

| Ruta | Qué es |
|---|---|
| `GET /administracion` | Las ediciones con sus convocatorias; alta de una edición |
| `POST /administracion/ediciones` | Crea una edición |
| `GET`, `POST /administracion/ediciones/{id}` | Una edición: sus datos y sus convocatorias |
| `POST /administracion/ediciones/{id}/convocatorias` | Convocatoria en blanco (tipo y títulos) |
| `POST /administracion/ediciones/{id}/duplicar` | Convocatoria copiada de otra edición |
| `GET /administracion/convocatorias/{id}` | Ficha de configuración |
| `POST …/convocatorias/{id}/datos` · `/bases` · `/calendario` · `/estado` | Cada sección de la ficha |
| `POST …/convocatorias/{id}/archivos/{clase}` (y `/quitar`) | Bases en PDF e imagen |
| `POST …/convocatorias/{id}/dotaciones` (y `/{dotacion}`) | Alta y edición de una dotación |
| `POST …/convocatorias/{id}/requisitos` (y `/{requisito}`) | Alta, edición y borrado de un requisito |
| `POST …/convocatorias/{id}/criterios` (y `/{criterio}`) | Alta, edición y borrado de un criterio |
| `POST …/convocatorias/{id}/asignaciones` · `/asignaciones/{a}/retirar` · `/cuentas` | Jurados y revisores |
| `GET`, `POST /administracion/usuarios` | Cuentas internas: búsqueda y alta |
| `POST /administracion/usuarios/{id}/activacion` | Reenvía el enlace de activación |

Son páginas, no API: no van en `rutasAPI` ni en `openapi.json`.

## 2. Quién ve y quién cambia

- El menú ofrece «Administración» a quien **supervisa** (administrador y observador).
- El **observador** ve las mismas páginas con los controles dentro de un
  `<fieldset disabled>` y sin botones: puede revisar toda la configuración, no
  cambiarla. Cada `POST` exige ser administrador (403 en otro caso), con independencia de
  lo que muestre la página.
- Jurados, revisores y postulantes reciben 403.

## 3. Una sola fuente de reglas

Cada formulario llama a la misma operación del repositorio que la API y que el modo
interactivo de `fimnme-cli`: `CrearEdicion`, `ActualizarConvocatoria`, `DefinirEtapas`,
`CambiarEstadoConvocatoria`, `GuardarDotacion`, `GuardarRequisito`, `GuardarCriterio`,
`Asignar`, `RetirarAsignacion`… Las pantallas no tienen reglas propias: lo que el modelo
rechaza vuelve a la ficha **junto al formulario que lo produjo y con lo escrito**, para
corregirlo sin reescribirlo. El alta de cuentas comparte con la API
`crearCuentaInterna` y `reenviarActivacion`.

**Un elemento de otra convocatoria no se toca.** La ruta de una dotación, un requisito,
un criterio o una asignación lleva el id de la convocatoria; antes de operar se comprueba
que el elemento esté entre los suyos (404 si no). Una mutación que quitó esa comprobación
dejó borrar desde una ficha el criterio de otra convocatoria, y las pruebas lo detectaron.

## 4. La ficha de configuración

Una página con índice lateral y una sección por tema:

- **Estado.** El actual y el siguiente, con **lo que falta para avanzar** (los mismos
  motivos que daría el cambio de etapa: bases, calendario, dotaciones, revisor, tres
  jurados…). El botón de avanzar está deshabilitado mientras falte algo. Volver a la etapa
  anterior pide motivo y queda en la bitácora. Los estados se nombran como el proceso
  (Publicada, Revisión documental), no como los ve el público («Próxima apertura»).
- **Títulos y límites**: máximo de obras en Compositores; duración, obras y ligas de video
  en Intérpretes.
- **Bases** en Markdown en los dos idiomas, y sus versiones en PDF y la imagen, con subida
  y retiro.
- **Calendario**: las tres etapas con controles de fecha y hora, **en hora de la Ciudad
  de México**, que se guardan en UTC. No se usa la zona del navegador: quien configura
  desde otro país vería fechas distintas de las que rigen la convocatoria.
- **Dotaciones** (Compositores): no se borran; se retiran desmarcando «Se ofrece», y las
  obras que ya la eligieron la conservan.
- **Requisitos documentales y criterios**: cada uno plegado en su resumen («Por cada obra
  · Cuarteto · Obligatorio»); se abre para editarlo. Los formatos se eligen de los que el
  sistema admite; el tamaño se escribe en MB. Borrar sólo procede en borrador (lo decide
  el modelo).
- **Jurados y revisores**: la lista con el estado de cada cuenta (activada o no, con
  reenvío del enlace) y su retiro; la búsqueda de una cuenta existente, que **elige una
  persona** —nunca se asigna por coincidencia de nombre—; y el alta de una cuenta nueva ya
  asignada.

**Crear y asignar.** Antes de crear la cuenta se comprueba con `ValidarAsignacion` que la
asignación procedería (la cuota de tres jurados, la etapa). Así un rechazo no deja una
cuenta huérfana con su correo de activación enviado. Sólo una carrera con otra asignación
simultánea podría hacer fallar la asignación después de crear la cuenta; entonces la
cuenta queda y el motivo se dice.

## 5. Cuentas internas

Búsqueda por nombre o correo, la lista con perfiles y estado, el alta (con perfil global
de administración u observación, o sin él para un jurado o un revisor) y el reenvío del
enlace de activación. La cuenta nace sin contraseña, como en la API y en `fimnme-cli`.

## 6. Sin JavaScript

Todas son formularios normales con POST-redirect-GET. No hay htmx en estas pantallas: se
usan pocas veces al año, y el repintado completo con el aviso y el ancla de la sección es
suficiente y más fácil de mantener.

## 7. Lo que no entra en la 13a

Las plantillas de correo no tienen API todavía; su pantalla, con su API, va en la 13c
junto con la bitácora. El tablero, la matriz y la publicación de la nómina, en la 13b.
