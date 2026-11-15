# ADR 0016 — Modo interactivo y `export-csv`

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 10c (§5.14 de la especificación, RF-CLI-02 y RF-CLI-05). Cierra el
paso 10 y, con él, el backend. No añade dependencias, migraciones ni operaciones de la
API.

## A. Modo interactivo (`fimnme-cli --tui`)

### 1. Un diálogo por renglones, no una pantalla dibujada

**El requisito** (RF-CLI-02). Un modo interactivo para administrar las fichas
institucionales: ediciones, convocatorias, dotaciones, requisitos documentales,
criterios y asignaciones. La tabla de tecnologías de la especificación (§7.1) preveía
«`golang.org/x/term` + dibujo propio; se evalúa en ADR si resulta insuficiente».

**Decisión.** El modo interactivo es un **diálogo por renglones**: muestra un menú
numerado, hace una pregunta, lee una respuesta. No dibuja pantallas, no pone la terminal
en modo crudo y no usa `golang.org/x/term`.

**Por qué.**

- **Ninguna dependencia.** `golang.org/x/term` no está entre las dependencias admitidas
  y habría exigido justificarla sólo para mover un cursor. La biblioteca estándar basta.
- **Funciona donde hace falta.** El modo interactivo es para cuando el panel no está a
  la mano: una instalación nueva, una sesión por SSH, dentro de `podman exec`, una
  consola serie, el servidor detenido por mantenimiento. En todos esos sitios un diálogo
  por renglones funciona igual; una pantalla dibujada depende de `TERM`, del tamaño de
  la ventana y de que haya una terminal de verdad.
- **Es accesible.** Un lector de pantalla lee un diálogo por renglones tal cual.
- **Se prueba entero.** Las pruebas le dan las respuestas por la entrada y leen lo que
  contesta: se recorre la herramienta real, no una simulación de teclas.
- **Se lee en una tarde.** Dos archivos, sin estado de pantalla que mantener. Es la
  clase de código que un relevo puede heredar (§2.A del perfil de dominio).

**Lo que se pierde.** No hay edición con flechas dentro de una lista ni formularios a
pantalla completa. Para el volumen de este sistema (dos convocatorias al año, una decena
de dotaciones) no hace falta; el panel web del paso 13 será la herramienta de diario.

### 2. No tiene reglas propias

Cada cambio llama a **la misma operación del repositorio que usa la API** (`CrearEdicion`,
`CrearConvocatoriaConfigurada`, `DefinirEtapas`, `CambiarEstadoConvocatoria`,
`GuardarDotacion`, `GuardarRequisito`, `GuardarCriterio`, `Asignar`…). Esas operaciones
validan y escriben en una sola transacción, así que el modo interactivo no puede dejar
nada que el panel habría rechazado: no publica sin bases ni calendario, no pasa a
evaluación sin tres jurados, no borra un requisito fuera de borrador.

Lo que el modelo rechaza se explica **con los mismos textos del panel** (`views/i18n`),
con el nombre del dato delante. No hay un segundo juego de mensajes que mantener.

### 3. Qué administra

| Ficha | Qué se puede hacer |
|---|---|
| Edición | Alta y edición (número, año, nombres). |
| Convocatoria | Alta en blanco (nace en borrador, con los requisitos y criterios iniciales de su tipo) o **copiando la configuración de otra edición**; títulos y límites; bases; calendario; cambio de estado. |
| Dotaciones | Alta, edición y baja lógica (una dotación no se borra: se desactiva). |
| Requisitos documentales | Alta, edición y eliminación (sólo en borrador), con su dotación, formatos y tamaño. |
| Criterios de evaluación | Alta, edición y eliminación (sólo en borrador). |
| Jurados y revisores | Asignar una cuenta existente, crear una cuenta y asignarla, retirar. |

- **Las bases se cargan de un archivo Markdown.** Un texto de varias páginas no se
  escribe en una terminal: se prepara aparte y se indica su ruta (hasta 1 MB, UTF-8).
- **Las fechas se escriben en hora de la Ciudad de México** (`AAAA-MM-DD HH:MM`) y se
  guardan en UTC, como en todo el sistema (RN-36).
- **El cambio de estado** es el forzado por el administrador: sólo al estado inmediato,
  con confirmación, y retroceder exige un motivo.
- **Asignar una cuenta existente** pasa por una búsqueda cuyo resultado elige quien
  opera: el sistema nunca decide que alguien es la misma persona (RN-30).
- **Una cuenta nueva** nace sin contraseña y su enlace de activación se muestra en la
  terminal, como en `crear-usuario`: la herramienta no envía correo.

### 4. Convenciones de captura

Las mismas en todas las fichas: **Intro** conserva el valor actual, que se muestra entre
corchetes; **`-`** deja vacío un dato opcional; un número o una fecha mal escritos se
vuelven a preguntar. Cada cambio se guarda al terminar su ficha, en una transacción.

**Si la entrada se acaba** (Ctrl+D, o la tubería que la alimentaba) a media ficha, esa
ficha no se guarda y el modo interactivo termina sin error: no hay nada a medias que
deshacer.

### 5. Quién queda en la bitácora

El **usuario del sistema** que ejecuta la herramienta, como en las demás órdenes
(RF-CLI-09). El modo interactivo no pide `--como`: administrar la configuración desde
la terminal ya exige acceso al servidor como el usuario del servicio, que es más de lo
que tiene cualquier cuenta del panel.

### 6. Lo que no está en el modo interactivo

- **Nada de postulaciones, revisión ni evaluación.** Son de las personas que tienen esos
  perfiles, y pasan por el panel con su sesión.
- **Campos extendidos, importación, anonimización, espejo:** tienen su propia orden, con
  sus confirmaciones. No se duplican en menús.
- **Editar plantillas de correo y de reporte:** quedan para el panel.

## B. `fimnme-cli export-csv`

### 7. Un solo generador

**El requisito** (RF-CLI-05, prioridad S). Generar reportes desde una plantilla guardada,
sin pasar por la interfaz web.

**Decisión.** El armado del reporte, que estaba dentro del manejador HTTP, pasó a una
función sin HTTP (`controllers.GenerarReporte`) que usan **la API y la herramienta**. El
mismo reporte sale idéntico por los dos caminos: mismas validaciones, mismo catálogo de
campos, mismo alcance, misma escritura del CSV (`views.EscribirCSV`).

`fimnme-cli` importa por primera vez `controllers/` y `views/`. Es deliberado: la
alternativa era copiar el generador, y dos generadores acaban por diferir.

### 8. En nombre de quién

Un reporte puede llevar las calificaciones de los tres jurados, que sólo son de la
supervisión. Por eso `export-csv` exige **`--como CORREO`**: la cuenta de un
administrador o de la coordinación nacional, activa. Se generan con **sus permisos de
hoy** (como el expediente en segundo plano, RN-26) y queda en la bitácora a su nombre.

`GenerarReporte` **comprueba por sí misma** que quien pide pueda generar reportes: quien
la llama desde fuera de la API no pasa por su control de acceso. Y las calificaciones
siguen saliendo sólo por la puerta de la supervisión (`matrizDeSupervision`, ADR 0013).
Son dos cerraduras, y una prueba con mutación confirmó que cada una se nota si falta.

### 9. Forma de la orden

```
fimnme-cli export-csv --listar
fimnme-cli export-csv --plantilla ID|NOMBRE (--edicion N | --convocatoria ID)
                      --como CORREO [--salida ARCHIVO] [--idioma es|en]
```

- Una plantilla no guarda la convocatoria (sirve para cualquier año), así que se indica
  aquí: por **número de edición** —la convocatoria es la del tipo de la plantilla— o por
  id.
- **Sin `--salida`, el CSV va a la salida estándar y nada más**, para encadenarlo.
- **Con `--salida`**, el archivo nace con permisos `0600` (lleva datos personales) y se
  escribe aparte para renombrarse al final: quien lo recoja nunca encuentra medio archivo.
- Las plantillas se crean en el panel (o por la API); la herramienta sólo las usa.

## Consecuencias

- Con el paso 10c **el backend está completo**. Siguen las pantallas (pasos 11 a 13) y
  la operación (paso 14).
- Un reporte puede programarse con un temporizador de systemd o un cron; el ejemplo
  queda para el paso 14, junto con los demás temporizadores.
- Si algún día hiciera falta una interfaz de terminal a pantalla completa, este ADR es
  el que habría que revisar; el diálogo por renglones podría quedar como modo de
  respaldo.
