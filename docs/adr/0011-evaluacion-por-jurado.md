# ADR 0011 — Evaluación por jurado y aislamiento entre jurados

- **Estado:** aceptada
- **Fecha:** 2026-11-13

Decisiones del paso 8. No añade dependencias ni migraciones: todo cabe en el esquema de
§8.3.

## 1. El aislamiento entre jurados es estructural, y está probado

**El requisito** (RF-EVA-05, RN-15). Un jurado nunca ve los puntajes ni las observaciones
de otro, ni la suma, en ninguna etapa, ni años después en el archivo. SQLite no tiene
seguridad por fila: toda la garantía vive en el código. Por eso no puede depender de que
quien edite mañana se acuerde de poner un `if`.

**Decisión** (§8.4). Dos tipos en `models/repository.go`, y nada más:

| Tipo | Quién lo usa | Qué puede hacer |
|---|---|---|
| `JuradoRepo` | Los manejadores que atienden a un jurado | Leer y escribir **su** evaluación. Se construye con el id del jurado y lo pone en el `WHERE` de cada consulta. No tiene ningún método capaz de devolver una evaluación ajena |
| `SupervisionRepo` | Sólo `apiResultados`, envuelto en `a.supervisa` | La matriz: lo único que cruza jurados, con la vista `v_resultado` |

Fuera de esos dos tipos no hay SQL que lea las tablas de evaluación, con una excepción
acotada: `marcarEvaluadaSiCompleta` **cuenta** cuántas evaluaciones se han enviado, para
pasar la postulación a «evaluada». No lee puntajes ni observaciones y no devuelve nada.

**Cuatro pruebas lo vigilan.** Las tres primeras leen el código; la cuarta lo ejercita.

1. `TestAislamientoEstructuralDelRepositorio` analiza `repository.go` y falla si un método
   de `JuradoRepo` consulta evaluaciones sin `jurado_id = ?`, si usa `v_resultado`, o si
   aparece SQL sobre esas tablas fuera de los dos tipos.
2. `TestSupervisionSoloDesdeResultados` analiza los controladores y falla si
   `Supervision()` se usa en otro lugar que `apiResultados`, o si esa ruta deja de ir
   envuelta en `a.supervisa`.
3. `TestAislamientoEntreJurados` es **la prueba obligatoria de §8.4**. Cada jurado deja
   una marca única en sus observaciones. Con la sesión de cada uno recorre **todas** las
   operaciones de la API —probando cada identificador y variantes como `?jurado_id=`— y
   las páginas: más de 500 peticiones por jurado. Falla si en alguna respuesta aparece la
   marca de otro jurado, o la suma o el promedio.
4. Las reglas (`TestReglasDeLaEvaluacion`, `TestPermisosDeEvaluacion`).

**Se comprobó que detectan lo que deben.** A las pruebas 1 y 3 se les sembraron fallos a
propósito —una consulta sin filtro, una consulta ajena, un manejador que «confía» en un
`jurado_id` de la URL— y los tres hicieron fallar la prueba. Una prueba de seguridad que
nunca se ha visto fallar no prueba nada.

**Lo que esto no cubre.** La garantía es de la aplicación. Quien tenga acceso al archivo
de la base, o a un respaldo, lee todo. Al migrar a MariaDB conviene añadir la defensa
declarativa: un usuario de base distinto para las sesiones de jurado, sin permisos sobre
`v_resultado` (§8.4).

## 2. La bitácora no guarda calificaciones

**Decisión.** Guardar o enviar una evaluación deja un asiento con el jurado, la
postulación y la obra, pero **sin puntajes ni observaciones**. Quien consulta la bitácora
no debe conocer por esa vía calificaciones sueltas.

Cada consulta de la matriz deja también su asiento: quién miró los resultados y cuándo
(§8.4, punto 4). Las lecturas que un jurado hace de **su propia** evaluación no se
registran: no cruzan a nadie y llenarían la bitácora.

## 3. La evaluación no es a ciegas

Decisión de la CNMO del 13 de noviembre de 2026. El jurado ve la postulación completa:
nombre, semblanza, obras y todos los documentos, que puede descargar (queda registrado).
No ve la revisión documental ni el dictamen final.

Sólo ve las postulaciones que **pasaron a evaluación**. Una descartada no existe para él.

## 4. Unidad de evaluación

**Decisión** (RF-EVA-02, RN-16). En Intérpretes se califica la postulación completa; en
Compositores, cada obra. `models.UnidadesDeEvaluacion` lo resuelve, y la unidad viaja como
`obra_id` (ausente en Intérpretes).

## 5. Borrador, envío y congelación

**Decisión** (RF-EVA-06, RN-17 a 19).

- Un **borrador** puede estar incompleto.
- Para **enviar** hacen falta todos los criterios obligatorios, cada uno dentro de su
  escala.
- Una evaluación enviada **puede corregirse** mientras dure la etapa, pero no dejarse
  incompleta ni volver a borrador. Conserva la fecha de su primer envío.
- **Cerrada la etapa, queda congelada.** El jurado sigue viendo la suya.
- La postulación pasa a **evaluada** cuando los tres jurados activos han atendido todas
  sus unidades —cada uno la envió o se excusó—: en Compositores, las de todas las obras.

## 6. Paso a evaluación: la revisión no la detiene

**Decisión** (RN-20; decisión de la CNMO del 13 de noviembre de 2026). Al pasar la
convocatoria a evaluación, las postulaciones `validada` pasan a `en_evaluacion`. Si la
etapa se retrocede, vuelven a `validada`.

**La convocatoria pasa a evaluación aunque queden postulaciones por subsanar o por
dictaminar.** La revisión documental sigue durante la evaluación: el revisor dictamina,
solicita subsanaciones y valida o descarta igual que antes. Una postulación que se valida
con la evaluación ya en marcha **pasa directo al jurado** y aparece en su lista de
trabajo; su avance lo refleja (el total crece).

La configuración de la convocatoria informa cuántas quedan por dictaminar
(`postulaciones_sin_dictaminar`), para que el administrador lo tenga a la vista. No es un
requisito.

*(La primera versión de este paso impedía pasar a evaluación con postulaciones
pendientes. Se quitó: obligaba a detener a todo el jurado por una sola subsanación.)*

**A tener presente.** Al **cerrar** la evaluación, una postulación que siga en revisión
queda fuera. El sistema no lo impide; conviene mirar `postulaciones_sin_dictaminar` antes
de cerrar.

## 7. Matriz de resultados y excusas

**Decisión** (RF-EVA-08). Para administrador y observador: por unidad, lo que calificó
cada jurado, quién se excusó, la suma, el promedio, el puntaje comparable y el lugar.

**Sólo cuentan las evaluaciones enviadas.** Un borrador aparece como «sin enviar», sin
puntajes: aún no es una calificación.

### Excusa por conflicto de interés

**Decisión** (RF-EVA-12; decisión de la CNMO). Un jurado puede excusarse de calificar una
unidad, con motivo. En Compositores puede excusarse de una obra o, sin indicar obra, de
toda la postulación: el conflicto suele ser con la persona.

- La excusa surte efecto al declararla. Lo que el jurado hubiera calificado de esa unidad
  se retira. El motivo lo ve la supervisión en la matriz; **no** va a la bitácora (suele
  nombrar a una persona) ni llega a los demás jurados.
- Con la excusa vigente no se califica: primero hay que retirarla.
- Una unidad excusada cuenta como **atendida**: no queda pendiente en el avance del jurado
  y no impide que la postulación pase a «evaluada».

**Una excusa no perjudica a la postulación.** Si se ordenara por la suma, dos jurados
sumarían menos que tres sin que la propuesta fuera peor. Por eso la matriz ordena por un
**puntaje comparable**: el promedio por jurado, llevado a la cuota de tres
(`models.PuntajeComparable`).

| Unidad | Calificaron | Suma | Puntaje comparable |
|---|---|---|---|
| A: 3+3, 3+3, 3+3 | 3 | 18 | 18 |
| B: 4+4, 4+4, y una excusa | 2 | 16 | **24** |

B va primero, aunque sume menos. Con tres jurados el puntaje comparable es la suma: nada
cambia para quien no tiene excusas. La matriz muestra las dos cifras, y el promedio.

*(La especificación dice que la excusa «la resuelve el administrador». Aquí surte efecto
al declararla y el administrador la ve; no hay un paso de aprobación. Queda como punto por
confirmar.)*

## 8. Dictamen final

**Decisión** (RF-CONV-10). El administrador registra el resultado de cada postulación:
seleccionada, no seleccionada o lista de espera. Puede hacerlo con la evaluación en curso
o ya cerrada. **Sólo lo ve la supervisión**: ni el postulante, ni el jurado, ni el
revisor. El resultado se comunica fuera del sistema (D-09).

El administrador **no puede evaluar** (§2.2): no tiene acceso a las rutas del jurado.

## Lo que no se hizo

- **Correos al jurado** de inicio de etapa y recordatorio (RF-NOT-09 y 10, prioridad S).
- **Exportación de la matriz a CSV**: llega con los reportes (paso 9).
