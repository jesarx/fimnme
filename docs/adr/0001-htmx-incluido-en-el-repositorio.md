# ADR 0001 — htmx incluido en el repositorio

- **Estado:** aceptada
- **Fecha:** 2026-11-12

## Contexto

La política de la CNMO prohíbe frameworks y cadenas de suministro de npm, y admite
bibliotecas pequeñas con justificación escrita. La especificación (D-02, §7.4) fija
`html/template` + **htmx** como frontal, «servido desde el binario».

## Decisión

Se incluye **un solo archivo**, `views/static/js/htmx.min.js`, versionado en el
repositorio e incrustado en el binario con `//go:embed`. No se descarga nada al compilar
ni al ejecutar.

| Dato | Valor |
|---|---|
| Biblioteca | htmx |
| Versión | 2.0.11 |
| Origen | paquete `htmx.org@2.0.11` del registro npm, archivo `dist/htmx.min.js` |
| Tamaño | 52 182 bytes |
| SHA-256 | `d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717` |
| Licencia | Zero-Clause BSD (0BSD), compatible con GPL v3.0; texto en `views/static/js/htmx.LICENSE.txt` |

Configuración aplicada en `views/templates/base.html` para que la CSP estricta no
necesite excepciones: `includeIndicatorStyles: false` (htmx no inyecta estilos; los del
indicador están en `fimnme.css`), `allowEval: false` y `selfRequestsOnly: true`.

## Consecuencias

- Compilar no requiere red ni Node. El binario es reproducible.
- **Actualizar htmx es un acto deliberado**: se reemplaza el archivo, se verifica su
  SHA-256 contra el publicado, se actualiza esta tabla y se ejecuta `make verificar`.
- Se permanece en la línea 2.x. El salto a una versión mayor exige un ADR nuevo.

## Comprobación

```bash
sha256sum views/static/js/htmx.min.js
```
