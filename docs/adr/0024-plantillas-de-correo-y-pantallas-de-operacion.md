# ADR 0024 — Plantillas de correo editables y pantallas de operación

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Decisiones del paso 13d (§5.8, §5.15, §1.D; RF-NOT, RF-AUD-03, RF-IMP-01 a 09, RN-28).
Última entrega del paso 13: con ella, todo lo que la API permite hacer tiene pantalla.
Sin dependencias ni migraciones: la tabla `plantilla_correo` ya era versionada desde la
migración `0001`.

## 1. Plantillas de correo

**Catálogo con sus variables.** `models.PlantillasDeCorreo` lista las once plantillas que
el sistema envía hoy, con las variables que cada controlador o trabajador les pasa y las
**indispensables** (sin `{{liga}}` un correo de activación no sirve; sin
`{{lista_problemas}}` y `{{fecha_limite}}`, una subsanación tampoco). Una prueba exige
que cada plantilla sembrada pase su propia validación: si un controlador deja de pasar
una variable o el catálogo se equivoca, falla (una mutación que quitó `nota` del
catálogo lo demostró).

**Versiones.** Ninguna versión se edita ni se borra. Guardar crea la versión siguiente,
que pasa a ser la vigente (`PlantillaCorreo` ya leía la más reciente). Volver a una
anterior es cargarla en el editor y guardarla de nuevo. Cada versión queda en la
bitácora (`plantilla_correo.version`). Los correos ya encolados no cambian. Si falta la
versión en inglés, se usa la española y la pantalla lo dice.

**Validación** (`ValidarVersionDePlantilla`): plantilla del catálogo, idioma es/en,
asunto de una línea (≤ 200; un salto permitiría inyectar cabeceras), cuerpo ≤ 20 000,
**ninguna variable que la plantilla no reciba** —quedaría escrita tal cual en el
correo— y todas las indispensables.

**Vista previa y prueba.** La composición del correo se separó en `componerCorreo`, que
usan `RenderCorreo` y `VistaPreviaDeCorreo`: la vista previa es exactamente el HTML que
llevará el correo, con valores de muestra. La prueba se arma igual, va a la cuenta de
quien la pide con «[PRUEBA]» en el asunto y sale por la bandeja de salida como cualquier
correo.

**API** (en `rutasAPI` y `openapi.json`, sólo administrador):

| Operación | Qué hace |
|---|---|
| `GET /api/v1/plantillas-correo` | Catálogo con la versión vigente en cada idioma |
| `GET /api/v1/plantillas-correo/{clave}/{idioma}` | Historial de versiones |
| `POST /api/v1/plantillas-correo/{clave}/{idioma}` | Versión nueva (201) |
| `POST …/{clave}/{idioma}/vista-previa` | Arma el correo con valores de muestra; no guarda |
| `POST …/{clave}/{idioma}/prueba` | Correo de prueba a quien lo pide (202) |

**Pantalla** (`/administracion/correos`): lista con la versión vigente por idioma; editor
con un solo formulario y tres botones —vista previa, guardar, prueba—, las variables con
su descripción, la vista previa a un lado y el historial. Lo rechazado vuelve con lo
escrito y con las variables desconocidas o faltantes señaladas. El observador lo ve en
sólo lectura.

## 2. Bitácora

`/administracion/bitacora`, para administrador y observador: filtros por acción, entidad,
número de registro, cuenta y fechas (los mismos de la API, con su lector compartido
`filtroDeAuditoria`); quién (nombre y correo), perfil e IP; el antes y el después
plegados y legibles; cada entidad enlaza a su propia historia; CSV de la API.

## 3. Importación asistida

`/administracion/importaciones`, sólo administrador: subir la hoja (fase Analyze, sin
escribir nada), y la importación con lo que hará al confirmarla, cada renglón dudoso con
sus candidatos y su motivo («nombre parecido», «mismo correo») para **vincular, crear u
omitir**, la resolución en bloque de los que no se parecen a nada, la confirmación con
casilla (fase Commit, todo o nada) y la cancelación. El análisis y la confirmación se
extrajeron de la API (`analizarImportacion`, `confirmarImportacion`) y la comparten.
Repositorio: `ImportacionesRecientes`.

## 4. Anonimización ARCO

`/administracion/arco`, sólo administrador: se busca por folio, se muestra la postulación
y una advertencia exacta de lo que se borra y lo que se conserva, y se pide el
fundamento, **volver a escribir el folio** y marcar la confirmación. Llama a
`AnonimizarPostulacion` y purga los archivos del disco igual que la API; el resultado
dice cuántos se borraron y cuántos se conservaron.

## 5. Navegación

La portada de la administración enlaza a cuentas, plantillas de correo, importación,
ARCO (las tres últimas sólo para el administrador) y bitácora.
