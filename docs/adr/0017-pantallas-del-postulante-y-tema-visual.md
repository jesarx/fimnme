# ADR 0017 — Pantallas del postulante y tema visual intercambiable

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 11a (§5.3, RF-POST-01 a 06 y 12; RNF-02, 08 y 09). Es la primera
entrega del frontal del postulante. No añade dependencias, migraciones ni operaciones de
la API.

El paso 11 se parte en dos entregas: **11a** (esto: panel y formulario por pasos con
guardado automático) y **11b** (documentos con barra de progreso, revisión y envío,
estado de la postulación enviada, subsanación y solicitudes de desbloqueo).

## A. Identidad visual

### 1. La identidad vive en un solo archivo

**El contexto.** La CNMO pidió que la plataforma se parezca al sitio del 48º Foro
—sobre todo la cabecera, el pie institucional y el estilo general—, pero el 49º tendrá
colores, logotipos e imagen propios que aún no existen (ADR 0008).

**Decisión.** Todos los colores y tipografías pasan a `views/static/css/tema.css`, como
variables de CSS, con su versión para el modo oscuro. La hoja `fimnme.css` no contiene
**ningún** color ni nombre de tipografía: sólo usa esas variables. Una prueba
(`TestElTemaConcentraLaIdentidad`) falla si aparece un color literal en `fimnme.css`, si
se usa una variable que no existe o si el modo oscuro deja de redefinir los colores
básicos.

Para vestir la plataforma con la imagen del 49º se cambian tres cosas y ninguna más:

1. `views/static/css/tema.css` (colores, tipografía, radios, sombra);
2. el dibujo `logo_foro` de `views/templates/parciales/logos.html`;
3. `views/static/img/favicon.svg`.

El encabezado de `tema.css` lista los pares de color que deben conservar el contraste
WCAG 2.1 AA (4.5:1). Con los colores actuales se midieron todos, en claro y en oscuro;
el más justo es el texto secundario de la banda sobre su degradado (4.8:1).

### 2. Qué se tomó del sitio del 48º

Además de lo que ya tomó el ADR 0008 (paleta, cabecera, pie institucional, logotipos):

| Rasgo del sitio | En la plataforma |
|---|---|
| Bandas de título a todo lo ancho, con degradado (las páginas divisorias del programa) | Bloque `banda` de `base.html`, en el panel y en el formulario |
| Antetítulo en versalitas con un filete encima | `.banda__ante` |
| Títulos de sección en versalitas con barra de acento | `.titulo-seccion` |
| Botones en forma de píldora | `--radio-boton` |
| Tarjetas con filete de color | Panel «Mis postulaciones» |
| Menú plegado tras un botón en pantallas angostas | `.menu-boton`, activado por `fimnme.js` |

No se tomó la imagen de partitura del fondo de las bandas: es del diseño gráfico del
48º, y el 49º traerá la suya.

### 3. Tipografía

El ADR 0008 dejó para este paso decidir si se incrusta Noto Sans en el binario.
**No se incrusta todavía**: la tipografía es parte de la identidad que el 49º aún no
define, y cada peso cuesta ~0.5 MB. La plataforma pide Noto Sans y Montserrat por nombre
y, si el equipo no las tiene, usa la del sistema. Cuando exista la imagen del 49º, sus
tipografías se incrustan en `static/` (con su licencia) y se nombran en `tema.css`.

## B. Pantallas del postulante

### 4. Rutas

| Ruta | Qué es |
|---|---|
| `GET /postulaciones` | Panel: el historial de la cuenta, de todas las ediciones, y las convocatorias abiertas |
| `POST /postulaciones` | Abre un borrador (en Intérpretes, con su subcategoría) |
| `GET /postulaciones/{id o folio}` | Enlace permanente: lleva al primer paso |
| `GET /postulaciones/{id}/{datos, concierto, obras}` | Un paso del formulario |
| `POST /postulaciones/{id}/campos` | Guarda los campos de un paso |
| `POST /postulaciones/{id}/integrantes`, `/videos` | Guardan esas listas |
| `POST /postulaciones/{id}/obras`, `/obras/{obra}` | Añaden, guardan, mueven y quitan obras |
| `POST /postulaciones/{id}/subcategoria` | De solista a ensamble o al revés, confirmando |

Son páginas, no API: no van en `rutasAPI` ni en la descripción OpenAPI. Cada manejador
llama a **la misma operación del repositorio** que la API, que es donde se decide qué
puede modificarse y qué valor es aceptable. Lo ajeno responde 404, como en la API (una
prueba con mutación confirmó que se detecta si falta la comprobación).

### 5. El formulario se pinta del modelo

Los campos salen de `models.FormularioDe` —la misma definición declarativa con que el
servidor valida— y de los campos extendidos que llena quien postula. Ningún campo está
escrito a mano en una plantilla: el parcial `pcampo` pinta texto, área, número, fecha,
hora, correo, liga o lista según el tipo del campo. Un campo extendido nuevo aparece
solo, también aquí.

Lo que falta para enviar sale de `models.ValidarParaEnvio`, igual que en la API: la barra
de pasos dice cuánto le falta a cada uno y cada paso lista lo suyo, con un enlace al
control donde se captura. **No hay validación en el cliente**: la única es la del
servidor (§7.4 de la especificación).

Los dos detalles que dependen de otro dato los decide también el modelo: la entidad de
residencia es una lista si el país es México y texto libre si no (se repinta al cambiar
el país), y la nota de la sede del concierto dice si los cuatro datos son obligatorios
según la entidad elegida (`models.CondicionCumplida`).

### 6. Guardado automático con htmx, y todo funciona sin JavaScript

Cada formulario se envía solo 1.2 s después de que se deja de escribir y al salir de un
control. El servidor responde con fragmentos «fuera de banda»: la confirmación
«Guardado a las 14:32» (RF-POST-03), la barra de pasos, lo que falta y el error de cada
campo enviado. No repinta el control donde se está escribiendo.

- **Sólo se escribe lo que cambió.** El formulario llega entero; el controlador lo
  compara con lo guardado. Un guardado sin cambios no toca la postulación, y una liga que
  nadie tocó no vuelve a comprobarse en red.
- **Una sola cola por paso** (`hx-sync` sobre `.paso`): si se pulsa «Quitar» o «Añadir»
  con algo recién escrito, el guardado sale antes y termina antes de que la lista se
  repinte. Sin esto, la lista nueva llegaba antes que el guardado.
- **Nada se pierde en silencio** (RNF-02). Mientras algo no está confirmado el aviso dice
  «Guardando…»; si el servidor no responde lo dice, y lo escrito sigue en pantalla. Al
  salir de la página, lo no confirmado se envía con `sendBeacon`. Un formulario que el
  servidor ya sustituyó no se reenvía (pisaría lo nuevo con lo viejo; el recorrido en el
  navegador lo descubrió). Con la sesión vencida, htmx recibe `HX-Redirect` a la página
  de ingreso en vez de seguir una redirección a ciegas.
- **Un renglón de integrante a medias** (con nombre, sin instrumento) no se guarda ni
  hace rechazar a los demás: se avisa.
- **Sin JavaScript**, cada formulario se envía con su botón y la página se vuelve a
  pintar: «Guardar y continuar», «Añadir integrante», «Guardar ligas», los botones de
  cada obra. Son los mismos manejadores. Limitación: sin JavaScript cada lista es un
  formulario aparte, así que lo escrito en un formulario se pierde si se pulsa el botón
  de otro sin guardarlo antes.

`views/static/js/fimnme.js` (unas 100 líneas, sin reglas de negocio) hace tres cosas: el
menú plegable, los avisos del guardado y mostrar la hora del guardado en la hora local de
quien mira. La CSP sigue sin admitir nada en línea.

### 7. Accesibilidad y móvil (RNF-08 y 09)

- Cada control con su etiqueta; ayuda y error enlazados con `aria-describedby`; el aviso
  de guardado es una región viva; los pasos usan `aria-current="step"`.
- Al añadir un integrante o una obra, o al mover una obra, el foco va al renglón nuevo o a
  la obra movida: quien navega con teclado no pierde su lugar.
- Los botones de icono (↑ ↓) llevan texto para lectores de pantalla.
- En pantallas angostas la barra de pasos muestra sólo el número de los demás pasos y el
  nombre del actual; los controles tienen al menos 44 px de alto.
- Verificado con Playwright a 390 y a 1280 px, en claro y en oscuro, con y sin
  JavaScript. **No** se hizo una auditoría con lector de pantalla ni con una herramienta
  automática de accesibilidad: queda para el cierre del paso 11.

### 8. Nombres de países

El formulario muestra los países por su nombre, en español o en inglés. Los 249 nombres
de cada idioma se tomaron del repertorio CLDR de Unicode (vía `babel`, sólo al escribir
los archivos de traducción; no es una dependencia del sistema) y se ajustaron algunos a
su forma corta («Hong Kong», «Costa de Marfil», «Myanmar»). Viven en `es.json` y
`en.json` como cualquier otro texto. México va primero en la lista.

## Consecuencias

- El 11b añade los pasos de documentos y de revisión y envío sobre esta misma estructura
  (`seccionesConPagina`), y la vista de una postulación enviada o en subsanación.
- Cambiar la imagen de la plataforma es cambiar tres archivos, y una prueba impide que la
  identidad se vuelva a dispersar.
