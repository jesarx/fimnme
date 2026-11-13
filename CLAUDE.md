# FIMNME — instrucciones permanentes para asistentes de IA y relevos

Este repositorio es la **Plataforma de Postulaciones FIMNME (CNMO / INBAL)**.

Antes de cualquier tarea rige, sin excepción, el perfil de dominio y las políticas
institucionales. Se importa completo a continuación y **no se modifica por conveniencia
del proyecto**:

@docs/PERFIL_DOMINIO_FIMNME.md

La especificación funcional y el modelo de datos están en
`docs/FIMNME-01-definicion-de-requerimientos-y-modelo-de-datos.md`. El avance por pasos
se lleva en `docs/PLAN_DE_DESARROLLO.md`.

## Guía de programación (soshe-peelee)

Además del perfil de dominio rige la guía de programación de la CNMO para proyectos
afines, **sólo en lo que aplica a este sistema**. El documento no es distribuible y no
está en el repositorio; lo adoptado y lo descartado se registra en
`docs/adr/0006-adopcion-de-la-guia-soshe-peelee.md`. Si la guía y el perfil de dominio
difieren, **rige el perfil de dominio**.

Estilo de código, de cumplimiento obligatorio:

- **Comentarios claros**, de varias líneas cuando hace falta, al inicio de cada
  declaración: qué hace y por qué. En español y con `//`.
- **Nombres explícitos** en funciones, tipos y variables de paquete.
- **Sin abstracciones innecesarias**: nada de interfaces, genéricos o capas que no
  necesite algo concreto hoy.
- **Manejo idiomático de errores**: ninguno se descarta en silencio; se envuelven con
  contexto y se distinguen con `errors.Is` / `errors.As`.
- **Integridad desde la primera versión**: restricciones en la base, reglas aplicadas en
  la misma transacción que la escritura, límites de tamaño y de frecuencia.

Además:

- La abstracción de la persistencia es `models.Persistencia`; controladores y
  trabajadores dependen de ella, nunca de `*models.Repositorio`.
- **Toda operación de la API va en la tabla `rutasAPI` y en `views/api/openapi.json`.**
  Una prueba falla si no coinciden.
- Aviso de licencia GNU completo en cada archivo Go. **Una migración ya aplicada no se
  edita**, ni siquiera su encabezado.
- Al cambiar órdenes u opciones de los binarios se actualizan `docs/man/*.1`.

## Reglas de trabajo en este repositorio

- **Idioma:** todo comentario de código, mensaje de commit y documentación va en español.
- **Paso a paso:** se trabaja un paso del plan a la vez; al terminar se actualiza
  `docs/PLAN_DE_DESARROLLO.md` y se espera indicación antes de iniciar el siguiente.
- **MVC estricto:** `models/` (entidades, reglas, persistencia), `controllers/`
  (peticiones y control de acceso), `views/` (plantillas, estáticos, i18n, PDF).
  Las reglas de negocio viven en `models/rules.go`; todo el SQL en `models/repository.go`.
- **Sin frameworks ni dependencias nuevas.** Sólo biblioteca estándar,
  `modernc.org/sqlite` y `golang.org/x/crypto`. Cualquier adición exige un ADR en
  `docs/adr/`.
- **Go 1.26, `CGO_ENABLED=0`**, binarios estáticos para Debian 13. Se compila con
  `make build`; la compilación se hace en la máquina del desarrollador y el binario se
  sube al VPS (`make deploy`).
- **Ninguna cadena visible al usuario en el código:** todas en `views/i18n/es.json` y
  `views/i18n/en.json`, con las mismas claves en ambos.
- **CSP estricta:** nada de `<script>` ni `style=""` en línea en las plantillas.
- **Encabezado de licencia** (GPL v3.0) en cada archivo fuente.
- Antes de entregar un paso: `make verificar` (formato, `go vet` y pruebas).
