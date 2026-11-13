# ADR 0006 — Adopción de la guía de programación soshe-peelee

- **Estado:** aceptada
- **Fecha:** 2026-11-13

## Contexto

La CNMO entregó la guía de programación que usa para proyectos afines («Documento de
desarrollo: framework soshe-peelee», borrador 1.0), con la indicación de aplicar a este
proyecto **sólo lo que le corresponda**. El documento está marcado como no distribuible,
por lo que **no se incluye en este repositorio**; aquí se registra únicamente qué se
adoptó y qué no.

Cuando la guía y el perfil de dominio de FIMNME (`PERFIL_DOMINIO_FIMNME.md`) difieren,
**rige el perfil de dominio**, que es específico de este sistema.

## Qué se adopta

### Estilo de código

Rige para todo el código Go del proyecto:

1. **Comentarios claros, de varias líneas cuando hace falta**, al inicio de cada
   declaración: dicen *qué* hace y *por qué*, no repiten el *cómo* que ya dice el código.
2. **Nombres explícitos** para funciones, tipos y variables de paquete. Las variables
   locales pueden ser breves.
3. **Sin abstracciones innecesarias**: una interfaz, un tipo genérico o una capa
   intermedia existen sólo si algo concreto los necesita hoy.
4. **Manejo idiomático de errores**: todo error se devuelve o se atiende; se envuelve con
   contexto (`%w`) y se distingue con `errors.Is` / `errors.As`. Ninguno se descarta en
   silencio.
5. **Integridad desde la primera versión**: llaves foráneas activas, restricciones en la
   base, reglas aplicadas en la misma transacción que la escritura, límites de tamaño y
   de frecuencia, y rechazo de campos desconocidos en la API.

Sobre el punto 1: los comentarios siguen en la forma `//`, que es la que esperan `gofmt`
y `go doc`. El estilo del kernel de Linux usa bloques `/* … */` con un asterisco por
línea; en Go ese asterisco aparecería como texto en la documentación generada. Si la
CNMO prefiere esa forma, la conversión es mecánica.

### Arquitectura

6. **La abstracción de la persistencia vive en el Modelo.** La interfaz
   `models.Persistencia` describe todo lo que puede pedírsele a la base de datos;
   `*models.Repositorio` la implementa sobre SQLite. Controladores y trabajadores dependen
   de la interfaz. Antes estaba definida del lado de los controladores.
7. **Entrega de datos en JSON con descripción OpenAPI.** La API `/api/v1` se describe en
   `views/api/openapi.json` (OpenAPI 3.1), que se sirve en `/api/v1/openapi.json`. Una
   prueba falla si una operación existe en el servidor y no en el documento, o al revés.

### Prácticas GNU

8. **Aviso de licencia completo** en cada archivo fuente Go, con la redacción estándar de
   GNU, además del identificador SPDX.
9. **`--version`** imprime versión, licencia y ausencia de garantía, según la convención
   de GNU.
10. **Páginas de manual** `fimnme-web(1)` y `fimnme-cli(1)` en `docs/man/`, instaladas
    con el despliegue.

### Convenciones de la familia de sistemas

11. **`-p` / `--port`** para el puerto, además de `-s` / `--serve`.
12. **La herramienta de línea de órdenes inicializa el entorno.** `fimnme-cli init` crea
    la base y su esquema, y `fimnme-cli` sin argumentos hace lo mismo **sólo si aún no
    hay base**. Con una base existente no la toca: aplicar migraciones a una base en uso
    sigue siendo un acto explícito (`migrate`, ADR 0003).

## Qué no se adopta, y por qué

| Elemento de la guía | Motivo |
|---|---|
| Nombres `soshe` y `peelee` de los binarios | El perfil de dominio fija `fimnme-cli` y `fimnme-web` |
| Entidades base (Persona, Agrupación, Evento, Sede, Programa) y campo `uuid` | El modelo de datos de FIMNME está fijado por su especificación (§8.3) |
| Ontologías FRBRoo y LODE | Describen catálogos de obras y eventos; esto es un sistema de postulaciones |
| Federación entre instancias y servicios de consulta para investigadores | Fuera del alcance de la v1; la salida pública es el JSON sanitizado hacia Hugo |
| Categorías de ACL (Administrador, Gestor, Revisor/Jurado, Persona) | El perfil de dominio define cinco perfiles propios, dos de ellos por convocatoria |
| Espejo con SSH y PGP (`--mirror`) y clúster (`--invoque`) | El perfil de dominio fija `VACUUM INTO` con HMAC-SHA256; en la guía son «próxima versión» |
| Constructor dinámico de formularios | Los campos del formulario son estables y van en columnas tipadas (§8.1); la variabilidad anual está en tablas de configuración |
| Reglas de negocio en el controlador | El perfil de dominio las adscribe a `models/rules.go`; el controlador las invoca |
| `--db RUTA` | Se conserva `--datos DIRECTORIO`, que abarca la base y el acervo de archivos |

## Temas que la guía plantea y quedan por decidir

- **Dictaminación a ciegas.** La guía prevé ocultar al jurado los datos sensibles del
  postulante «si la convocatoria lo requiere». La especificación de FIMNME no lo pide.
  Debe decidirse antes del paso 8 (evaluación).
- **Titular de los derechos de autor.** La práctica de GNU pide una línea de copyright en
  cada archivo. Sigue pendiente saber a nombre de quién va.

## Consecuencias

- Toda operación nueva de la API se añade a la tabla de rutas y a `openapi.json`.
- Las páginas de manual se actualizan cuando cambian las órdenes o las opciones.
- Las migraciones ya aplicadas conservan su encabezado anterior: una migración aplicada
  no se edita (ADR 0003).
