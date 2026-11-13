# ADR 0008 — Identidad visual: por ahora, la del 48º Foro

- **Estado:** aceptada (provisional hasta que exista la imagen del 49º Foro)
- **Fecha:** 2026-11-13

## Contexto

La plataforma se diseña para el 49º Foro, cuya imagen visual aún no existe. La CNMO
indicó usar entretanto la del 48º, tomada del repositorio de materiales de esa edición
(`jesarx/48fimnme`, sitio web), y subrayó que **el pie institucional debe estar**.

De ese repositorio se usa sólo lo que sirve a la plataforma. El resto (programa de mano,
postales, boletín, fotografías, logotipos de las sedes) no aplica.

## Decisión

**Se toma:**

| Qué | De dónde | Dónde queda |
|---|---|---|
| Paleta: verdes sobre fondo beige | `sitio/assets/css/sitio.css` | Variables de `views/static/css/fimnme.css` |
| Cabecera: barra oscura, logotipo y menú en versalitas | `sitio/_includes/cabecera.html` | `views/templates/base.html` |
| **Pie institucional**: datos y enlaces, logotipo de la CNMO, logotipos de Cultura e INBAL, franja de derechos | `sitio/_includes/pie.html` | `views/templates/base.html` |
| Logotipo del 48º Foro e icono | `sitio/assets/img/logo-48.svg`, `icono.svg` | `views/templates/parciales/logos.html`, `views/static/img/favicon.svg` |
| Logotipos de la CNMO, Cultura e INBAL | `sitio/assets/img/` | `parciales/logos.html`, `views/static/img/` |
| Catálogo de dotaciones de 2026 | Numeralia de la conferencia de prensa | Datos de demostración (`sembrar-demo`) |

**Adaptaciones obligadas por las políticas de la plataforma:**

- **Sin fuentes externas.** El sitio del 48º carga Noto Sans y Montserrat desde Google
  Fonts. La plataforma no descarga nada de terceros (CSP, RNF de privacidad): pide esas
  familias por nombre y, si el equipo no las tiene, usa la del sistema. Incrustar Noto
  Sans en el binario (licencia OFL, ~0.5 MB por peso) se decidirá en el paso 11.
- **Logotipos de Cultura e INBAL sin hoja de estilo interna.** Los originales traen un
  `<style>`; la CSP lo bloquearía y se verían en negro. Se convirtieron a atributos
  `fill`, sin cambiar el dibujo.
- **Modo oscuro y contraste AA.** Se conserva el modo oscuro del esqueleto, con la misma
  paleta; el verde de acento se oscureció un punto (`#2A7350`) para cumplir contraste AA
  sobre el fondo beige en texto pequeño.
- **Bilingüe.** Los textos del pie pasan por `es.json` y `en.json`. Los nombres de las
  instituciones no se traducen.

**Para cambiar a la imagen del 49º** basta con: las variables de color de la hoja de
estilo, el dibujo `logo_foro` en `parciales/logos.html` y `static/img/favicon.svg`.

## Lo que el repositorio del 48º no trae

**No contiene las bases ni los PDF de las convocatorias**, sólo el programa del Foro. Las
bases de la plataforma siguen siendo texto de demostración hasta que la CNMO entregue las
oficiales. Sí aportó cifras útiles (numeralia de 2026):

- 73 postulaciones de intérpretes y 151 personas con 240 obras en composición: coincide
  con el volumen previsto (~250 al año).
- Las categorías de estreno (mundial, en México, ya estrenada) coinciden con las de la
  plataforma.
- En Intérpretes se reportan **cinco formatos** (solista, dúo, trío, cuarteto, ensamble de
  cinco o más), con tabulador de pagos por formato. La especificación de la plataforma
  sólo distingue solista y ensamble; el formato puede derivarse del número de
  integrantes. Queda como punto por confirmar.

## Derechos

Los logotipos pertenecen a sus instituciones (Secretaría de Cultura, INBAL, CNMO) y se
usan en un sistema de la propia CNMO. No quedan cubiertos por la licencia GPL del código.
