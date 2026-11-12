# ADR 0005 — Configuración de convocatorias y ciclo de etapas

- **Estado:** aceptada
- **Fecha:** 2026-11-12

Decisiones del paso 4. No añade dependencias ni migraciones: todo cabe en el esquema de
§8.3.

## 1. Las reglas se aplican dentro de la transacción que escribe

**Decisión.** Cada método del repositorio que modifica la configuración lee lo que
necesita, llama a la regla correspondiente de `models/rules.go` y escribe, **todo en una
misma transacción**. Si la regla no se cumple devuelve `*models.ErrorValidacion`, con la
lista de problemas, y no escribe nada.

**Motivo.** Comprobar la regla en el controlador y escribir después deja una ventana: dos
peticiones simultáneas podrían, cada una por su lado, ver «dos jurados» y dejar cuatro.
Como la base tiene una sola conexión de escritura, dentro de la transacción no hay
carrera posible. Las reglas siguen siendo funciones puras en `rules.go`, probadas sin
base de datos; el repositorio sólo las invoca en el momento correcto.

## 2. Una sola puerta para cambiar de etapa

**Decisión.** `models.ValidarCambioDeEstado` decide toda transición del ciclo
`borrador → publicada → postulacion → revision_documental → evaluacion → cerrada →
archivada`. La usan por igual el administrador que fuerza un cambio y el trabajador que
avanza por calendario.

| Hacia | Condición |
|---|---|
| Cualquiera | Sólo el estado inmediato anterior o siguiente |
| `publicada` | Título y bases en español, y el calendario completo |
| `postulacion` | Calendario completo; en Compositores, al menos una dotación activa |
| `revision_documental` | Al menos un revisor activo (RN-14) |
| `evaluacion` | Exactamente tres jurados activos (RN-14) y al menos un criterio |
| Un estado anterior | Motivo obligatorio, y que el calendario no vaya a deshacer el retroceso |

Publicar y archivar son siempre actos explícitos: el calendario nunca los hace.

**Retroceder contra el calendario se rechaza.** Si un plazo ya venció, volver a esa etapa
no sirve: el avance automático la cerraría en el siguiente ciclo. La especificación dice
que un plazo se extiende «cambiando una fecha en configuración»; el sistema lo exige:
primero se cambia la fecha, después se retrocede.

## 3. Avance automático por calendario

**Decisión.** Un trabajador de `fimnme-web` revisa cada 30 segundos, y al arrancar, las
convocatorias en curso. Avanza de una etapa en una; si el servicio estuvo detenido y
vencieron varias fechas, recorre las que las reglas permitan.

| Estado | Avanza cuando… |
|---|---|
| `publicada` | empieza la etapa de postulación |
| `postulacion` | **termina** la etapa de postulación |
| `revision_documental` | empieza la etapa de evaluación |
| `evaluacion` | termina la etapa de evaluación |

**Consecuencia que hay que conocer.** El avance automático respeta las cuotas: si al
vencer el plazo de postulación no hay un revisor asignado, la convocatoria **no avanza**
y sigue en `postulacion`. El trabajador lo avisa en el log una vez por hora, y la API
muestra el bloqueo por adelantado en el campo `siguiente.bloqueadores`. Para que un
descuido de configuración no deje la captura abierta fuera de plazo, **el paso 5
comprobará además la fecha de cierre** antes de aceptar cambios en una postulación.

## 4. Qué se puede cambiar y cuándo

| Qué | Hasta cuándo |
|---|---|
| Textos, límites, calendario, dotaciones, requisitos, asignaciones | Mientras la convocatoria no esté `cerrada` ni `archivada` |
| Criterios de evaluación | Antes de `evaluacion` |
| Borrar un requisito o un criterio | Sólo en `borrador` |
| Número de la edición | Mientras todas sus convocatorias sean `borrador` |

- **Cerrada o archivada es de sólo lectura también para el administrador** (RF-ARCH-03).
  Para corregir algo hay que desarchivar explícitamente, con motivo, y queda en bitácora.
- **Nada se borra salvo en borrador** (RN-22). Una dotación se retira con `activa:
  false`; una asignación se desactiva y su fila se conserva, con lo que esa persona haya
  dictaminado o evaluado. Si vuelve a asignarse, se reactiva la misma fila.
- **El número de la edición se fija al publicar**, porque de él dependen los enlaces
  permanentes (`/convocatorias/{número}/…`) y los folios.

## 5. Quien postula no revisa ni califica

**Decisión.** No puede asignarse como jurado ni como revisor de una convocatoria a una
cuenta que tiene una postulación en ella. **La especificación no lo dice** (sólo prevé,
con prioridad baja, que un jurado se excuse por conflicto de interés); se añade porque el
caso contrario invalidaría el proceso. La regla recíproca —un jurado o revisor no puede
postular en su convocatoria— se aplicará en el paso 5. **Pendiente de confirmar.**

Sí se permite que una misma persona sea revisor y jurado de una convocatoria.

## 6. Valores iniciales de una convocatoria nueva

**Decisión.** Al crearse, una convocatoria recibe los requisitos documentales de su tipo
(§5.3.4, §5.3.6, RF-DOC-04) y los dos criterios de la v1 (RF-EVA-03), para que el
administrador ajuste en lugar de capturar desde cero:

| Tipo | Requisitos iniciales |
|---|---|
| Intérpretes | fotografía (postulación); partitura y autorización del titular (por obra) |
| Compositores | fotografía (postulación); partitura, particellas y audio opcional (por obra) |

Las excepciones por dotación —una obra electroacústica no lleva particellas— son
requisitos con `dotacion_id` distinto de cero, que sobrescriben al general de la misma
clave. `models.RequisitosAplicables` resuelve la regla efectiva.

## 7. Duplicar copia la configuración, no el año

**Decisión.** `POST /api/v1/convocatorias/{id}/duplicar` copia a otra edición textos,
límites, dotaciones, requisitos y criterios. **No** copia el calendario ni las
asignaciones. Las dotaciones reciben identificador nuevo y los requisitos específicos se
reapuntan a la copia.

## 8. Cuentas internas sin perfil global

**Decisión.** Un jurado o un revisor se da de alta **sin perfil global**: sus perfiles
son siempre por convocatoria (tabla `asignacion`). Así, terminada su participación, la
cuenta no conserva acceso a nada.

El sistema **no vincula identidades por su cuenta** (RN-30): si el correo ya existe, el
alta responde «duplicado» y el administrador busca la cuenta existente
(`GET /api/v1/usuarios?q=`) y la asigna.

## 9. Lo que este paso deja pendiente

- **Imagen de cabecera y PDF de bases** (RF-CONV-02): necesitan el almacén de archivos,
  que llega en el paso 6.
- **Gestión de cuentas existentes** (desactivar una cuenta, otorgar o retirar un perfil
  global a alguien que ya tiene cuenta): no está en RF-CONV; se decidirá con el panel de
  administración.
