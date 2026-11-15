# ADR 0020 — Pantallas del jurado

- **Estado:** aceptada
- **Fecha:** 2026-11-15

Decisiones del paso 12b (§5.6, RF-EVA-01 a 07 y 12). Completa, con el 12a (ADR 0019),
el paso 12. No añade dependencias, migraciones ni operaciones de la API.

## 1. Rutas

| Ruta | Qué es |
|---|---|
| `GET /evaluacion` | Las convocatorias en que la sesión es jurado, con su avance |
| `GET /evaluacion/convocatorias/{id}` | La lista de trabajo: cada unidad y en qué va |
| `GET /evaluacion/postulaciones/{id}?obra=` | La ficha de evaluación de una unidad |
| `POST /evaluacion/postulaciones/{id}/evaluacion` | Guarda la calificación (borrador o envío) |
| `POST /evaluacion/postulaciones/{id}/excusa` | Se excusa por conflicto de interés |
| `POST /evaluacion/postulaciones/{id}/excusa/retirar` | Retira la excusa |

La unidad de evaluación es la de la regla del dominio: la postulación en Intérpretes, y
**cada obra** en Compositores (`?obra=`). Una ruta que no corresponde a una unidad de la
lista propia —una postulación descartada, una obra ajena, Compositores sin obra—
responde 404.

## 2. Aislamiento entre jurados

Las páginas sólo usan `ParaJurado(id de la sesión)`, igual que la API: no hay forma de
pedir lo de otro jurado ni con parámetros inventados. Tampoco muestran nada de la revisión
documental (dictámenes, observaciones del revisor, descarte). La prueba
`TestPantallasDelJurado` barre todas las páginas con la sesión de cada jurado buscando lo
que escribió el otro, y comprueba que cada uno sí ve lo suyo (para que la prueba tenga con
qué fallar). La prueba de estructura (`TestSupervisionSoloTrasComprobar`) sigue vigilando
que nada de esto llame a la supervisión. Una mutación que quitó la comprobación de que la
sesión sea jurado de la convocatoria hizo fallar las pruebas.

El menú ofrece «Evaluación» sólo a quien es jurado de alguna convocatoria; el observador y
el administrador ven los resultados en la matriz (paso 13).

## 3. La ficha

- La postulación en sólo lectura —la misma vista de cada paso del postulante—, con sus
  documentos en el visor del 12a. En Compositores, sólo la obra que se califica.
- La evaluación **no es a ciegas** (decisión de la CNMO): el jurado ve la postulación
  completa, también sus datos de contacto.
- **La escala** de cada criterio son botones grandes (radios con su etiqueta), que se
  eligen con un toque en el móvil y con las flechas en el teclado.
- **Borrador o envío**: el borrador puede quedar incompleto y no cuenta; enviar exige todos
  los criterios obligatorios. Una vez enviada puede modificarse hasta que cierre la etapa,
  y sigue enviada. Lo rechazado se dice junto a cada criterio, con lo escrito conservado.
- **Excusa** por conflicto de interés, con motivo, de la unidad que se está viendo; se
  puede retirar. No perjudica a la postulación (decisión de la CNMO).
- Navegación a la unidad anterior y a la **siguiente por evaluar**.
- Fuera de la etapa de evaluación, se consulta pero no se modifica.

## 4. Lista de trabajo

«Has evaluado 34 de 87», con una barra de avance; cada unidad con su estado (sin empezar,
borrador, enviada, te excusaste) y un botón a la siguiente por evaluar. Una unidad de la
que se excusó no cuenta como pendiente.
