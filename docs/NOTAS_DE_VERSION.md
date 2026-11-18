# Notas de versión

## 1.0.0-beta.1 · 17 de noviembre de 2026

Primera versión **beta**: el plan de desarrollo está completo (pasos 1 a 14, más los
ajustes del ADR 0032) y el sistema entra en pruebas con usuarios reales.

- Lo que salga de las pruebas se corregirá en betas siguientes (1.0.0-beta.2, …).
- La **1.0.0** será la versión que se firme con el Manual de Validación.

### Qué hace

Todo el ciclo de las dos convocatorias anuales del Foro, Intérpretes y Compositores:

- publicación de las bases y postulación en línea, con guardado automático y envío con
  acuse en PDF;
- revisión documental con subsanación selectiva;
- evaluación por tres jurados aislados entre sí;
- matriz de resultados y nómina al portal público;
- archivo histórico de todas las ediciones, con buscador, descargas registradas y
  expedientes.

Bilingüe, accesible (WCAG 2.1 AA), con verificación en dos pasos para administrador y
observador. Las funciones por perfil están en el README, «Qué hace».

### Respecto de lo ya desplegado en el VPS de pruebas

Si el VPS tiene la versión de los PR #3 y #4, `make deploy` aplica sola la migración
nueva y no hay que cambiar la configuración.

| Cambio | Detalle |
|---|---|
| **Migración 0010** | Retira la columna `usuario.totp_secret`, que el esquema inicial previó y nunca se usó. La verificación en dos pasos usa sus propias columnas desde la 0009. Estaba vacía. |
| **Anonimización ARCO** | Ahora borra también la clave y los códigos de recuperación del segundo factor de la cuenta anonimizada. Antes los dejaba (lo halló la limpieza). |
| **`fimnme-cli` en una instalación nueva** | `export-json`, `verify`, `mirror`, `arco` e `import` crean el directorio del acervo si aún no existe (recién hecho `init`, antes de arrancar el servidor), en vez de fallar. |
| **Documentación completa** | Arquitectura de despliegue (02), diccionario de datos y permisos (02b), API e interfaz (03), formato del portal (06). El README se reorganizó: los ejemplos de la API pasaron al 03. |
| **El diccionario no puede desactualizarse** | Dos pruebas fallan si una tabla, columna u operación de la API no está documentada. |
| **Ejemplo de la API con segundo factor** | El ingreso por `curl` de un administrador incluye ahora el paso del código. |
| **Comentarios** | Se corrigieron los que anunciaban como futuro lo que ya existe. |

### Cifras

| | |
|---|---|
| Pruebas | 230, todas en verde (`make verificar`) |
| Cobertura de la capa de dominio | 82.8 % (mínimo 70 %) |
| Migraciones | 10 |
| Decisiones registradas | ADR 0001 a 0032 |
| Dependencias de Go | 2 directas: `modernc.org/sqlite` y `golang.org/x/crypto` |
| Compilación | Reproducible bit a bit |

### Lo que falta para la 1.0.0

No es código: es de la CNMO o del servidor final (Manual de Validación, §5).

1. Texto oficial del **aviso de privacidad**.
2. **Bases oficiales** de las dos convocatorias.
3. **Correo** validado en el servidor final (SPF, DKIM, DMARC; entrega a Gmail y
   Outlook).
4. Revisión de los **puntos por confirmar** del plan.
5. Las pruebas con usuarios y la firma del acta.

Los requisitos opcionales (prioridad S y C) sin implementar están listados en el Manual
de Validación, §5. Se decidirán con lo que salga de las pruebas.

### Cómo se identifica la versión

Los binarios toman su versión de la etiqueta de git al compilar:

```bash
fimnme-cli version
```

Debe decir `v1.0.0-beta.1`.
