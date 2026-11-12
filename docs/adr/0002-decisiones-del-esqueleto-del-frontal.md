# ADR 0002 — Decisiones del esqueleto del frontal

- **Estado:** aceptada
- **Fecha:** 2026-11-12

Decisiones técnicas tomadas al construir el paso 1 que no estaban fijadas por la
especificación. Ninguna añade dependencias.

## 1. Protección CSRF con la biblioteca estándar, sin tokens

**Decisión.** Se usa `http.CrossOriginProtection` (Go 1.25+), que rechaza las peticiones
que modifican estado cuando provienen de otro sitio, a partir de las cabeceras
`Sec-Fetch-Site` y `Origin`.

**Motivo.** Cumple RNF-10 con cero código propio y sin campos ocultos en cada formulario,
lo que además simplifica htmx. Todos los navegadores vigentes envían `Sec-Fetch-Site`
desde 2023.

**Condición.** El proxy inverso debe conservar la cabecera `Host` (Caddy lo hace por
omisión; en nginx es `proxy_set_header Host $host`). Regla derivada: **ninguna petición
GET modifica estado**; por eso el cambio de idioma es un `POST`.

## 2. Intérprete propio de Markdown, de subconjunto mínimo

**Decisión.** `views/markdown.go` admite sólo títulos, párrafos, listas, negritas,
cursivas y enlaces `http`/`https`/`mailto`. Todo lo demás, incluido el HTML, se muestra
como texto literal.

**Motivo.** Las bases (RF-CONV-02) no necesitan más, y un intérprete completo sería una
dependencia. El texto se escapa primero y sólo después se añaden esas etiquetas: es
seguro por construcción y está cubierto por pruebas.

## 3. Zonas horarias incrustadas

**Decisión.** Se importa `time/tzdata` para que el binario estático no dependa del
paquete `tzdata` del sistema. Costo: unos 450 KB.

## 4. Caché de estáticos por huella de contenido

**Decisión.** Las URL de CSS y JS llevan `?v=<huella>`, calculada del contenido
incrustado. Con huella se sirven como inmutables por un año; al cambiar un archivo cambia
la huella y el navegador descarga la versión nueva. La huella es determinista, por lo que
no afecta la reproducibilidad del binario.

## 5. Contraseñas: longitud antes que composición

**Decisión.** Mínimo 10 y máximo 128 caracteres, contados en caracteres y no en bytes,
sin reglas de mayúsculas o símbolos. **Pendiente de confirmar con la CNMO.**

## 6. Confianza en las cabeceras del proxy

**Decisión.** `X-Forwarded-For` y `X-Forwarded-Proto` sólo se creen cuando la conexión
viene de la propia máquina (el proxy). De `X-Forwarded-For` se toma el último valor, que
es el que añadió nuestro proxy; los anteriores los controla el cliente.

**Motivo.** La IP alimentará la bitácora de descargas y el límite de tasa: no puede ser
falsificable por el visitante.
