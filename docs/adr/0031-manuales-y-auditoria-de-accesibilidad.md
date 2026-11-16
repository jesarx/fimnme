# ADR 0031 — Manuales de Adopción y de Validación; auditoría de accesibilidad

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Última entrega del paso 14 (RNF-14: «Manual de Adopción y Manual de Validación»; §14 de
la especificación, documentos 04 y 05).

## Decisión

### 1. Dos manuales con funciones distintas

- **`docs/FIMNME-04-manual-de-adopcion.md`**: para quien recibe el sistema. Cubre
  qué se recibe, los perfiles, la instalación (servidor, respaldos, correo, espejo,
  contenedores), la configuración, la **operación anual** etapa por etapa, la operación
  continua, los problemas frecuentes y la entrega-recepción al siguiente equipo.
  Ordena el trabajo y **remite** al README y a las páginas de manual para el detalle de
  cada orden, en lugar de copiarlo. Copias distintas del mismo procedimiento acabarían
  diciendo cosas distintas.
- **`docs/FIMNME-05-manual-de-validacion.md`**:
  - casos de prueba por área, con requisito, procedimiento (prueba automática por su
    nombre, o pasos a mano) y criterio de aceptación;
  - los resultados de la verificación previa;
  - lo que condiciona la firma, el protocolo de firma y el acta.
  - **V-JUR-01** (aislamiento entre jurados) y **V-CAR-01** (carga de escritura
    concurrente) son condición de la firma sin excepción, como ordenan §8.4 y D-30.

Cada prueba que cita el Manual de Validación se comprobó contra el código: todas
existen. Se ajustaron los casos a lo que el sistema hace. La bitácora registra la
lectura de evaluaciones que cruza jurados, no la del jurado sobre la suya, y se anotó
como observación.

### 2. Auditoría automática de accesibilidad

Estaba pendiente desde el paso 11 (ADR 0017, §7). `deploy/accesibilidad/auditar.js`
corre **axe-core** en Chromium, con Playwright, sobre:

- las páginas públicas;
- el registro, también con errores;
- «Mi cuenta» y «Mis postulaciones»;
- cada paso del formulario de cada convocatoria abierta.

Lo hace en tres combinaciones: español en escritorio, inglés en móvil y tema oscuro.

- **No es parte del sistema.** No se compila ni viaja en los binarios. Playwright y
  axe-core se instalan **fuera** del repositorio, en un directorio temporal: el
  repositorio sigue sin `node_modules` ni `package.json`. Es una herramienta de
  verificación, como `deploy/carga`, y por eso vive en `deploy/`.
- La CSP del sistema impide inyectar axe-core. La herramienta abre su navegador con
  `bypassCSP`, sólo para sí misma.

**Resultado.**

- La primera corrida halló **una violación**, repetida en cada paso del formulario. El
  estado del paso actual en el indicador de avance tenía un contraste de **4.48:1**
  sobre el fondo tenue del acento, y el mínimo es 4.5:1.
- Se corrigió en `fimnme.css`: ahí se usa la tinta principal.
- Con eso: **63 páginas, 0 violaciones** de WCAG 2.1 A y AA.

Una auditoría automática no sustituye la prueba con teclado y con lector de pantalla:
quedan como casos manuales (V-ACC-02 y V-ACC-03).

## Verificación previa (16-nov-2026)

- 214 pruebas, 0 fallas.
- Cobertura de `models/` con todas las pruebas: 81.9 % (mínimo 70 %).
- Dos compilaciones con la caché vacía: binarios idénticos.
- Los 111 archivos Go con su aviso de licencia.
- Aislamiento y carga: pasan.

## Consecuencias

- El plan queda completo.
- Lo que falta para la firma no es código:
  - el aviso de privacidad oficial;
  - las bases oficiales;
  - la validación del correo en el servidor final;
  - la revisión de los puntos por confirmar.
- Quedan, como trabajo posible y no obligatorio, los requisitos de prioridad S y C sin
  implementar. Están listados en el FIMNME-05 §5; el más recomendable es el segundo
  factor de autenticación (RF-AUT-09).
