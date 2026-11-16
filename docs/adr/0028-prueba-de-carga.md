# ADR 0028 — Prueba de carga de RNF-01

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Cuarta entrega del paso 14. RNF-01: «200 usuarios concurrentes y 50 subidas simultáneas
en el pico del último día; p95 < 500 ms en lecturas».

## La herramienta

`deploy/carga/` (`go run ./deploy/carga …`): una herramienta del operador, no un binario
que se despliegue. Prepara N postulantes ya verificados directamente en la base —crear
cuentas no es lo que se mide— y, por HTTP contra el binario real:

1. **Ingreso** de todos a la vez, alta de su postulación y de una obra.
2. **Trabajo** durante un tiempo fijo: cada postulante autoguarda (70 %), abre su
   formulario (20 %) y consulta su postulación por la API (10 %), con pausas de 0,5 a
   1,5 s como una persona; al principio, 50 de ellos suben a la vez un PDF de 5 MB.
3. Reporte de p50, p95, p99 y errores por operación.

Cada usuario virtual llega con su propia IP en `X-Forwarded-For`, que el servidor sólo
acepta de conexiones locales: por eso se corre en la misma máquina, contra 127.0.0.1 y
no a través del proxy, y contra una instalación de **prueba** en entorno de desarrollo
(la cookie de producción exige HTTPS; las cuentas `carga-…@carga.invalid` quedan en la
base).

## Resultado (16-nov-2026, 4 núcleos, binario real por HTTP en la misma máquina)

200 postulantes, 50 subidas simultáneas de 5 MB, 60 s de trabajo: **12 622 peticiones,
0 errores.**

| Operación | Pedidas | p50 | p95 | p99 |
|---|---:|---:|---:|---:|
| Lectura: formulario (página) | 2 389 | 6 ms | **12 ms** | 18 ms |
| Lectura: postulación (API) | 1 128 | 2 ms | **4 ms** | 7 ms |
| Autoguardado | 8 455 | 2 ms | 4 ms | 304 ms |
| Subida de 5 MB (50 a la vez) | 50 | 411 ms | 608 ms | 646 ms |
| Alta de postulación | 200 | 232 ms | 1,04 s | 1,37 s |
| Ingreso (200 a la vez) | 200 | 2,09 s | 3,12 s | 3,21 s |

Memoria máxima del servidor durante la prueba: **320 MB**.

**RNF-01 se cumple**: el p95 de las lecturas es de 4 a 12 ms frente a 500 ms.

## Lectura de los números

- **El ingreso es lento a propósito.** Argon2id usa 19 MiB por cálculo y el servidor
  calcula como mucho 4 a la vez (`turnosArgon`): 200 ingresos en el mismo segundo hacen
  fila. Es el precio de que una ráfaga no agote la memoria de un VPS pequeño, y un caso
  extremo: 200 usuarios concurrentes no ingresan en el mismo segundo.
- El p99 del autoguardado (304 ms) y el alta de postulaciones reflejan la cola de la
  única conexión de escritura de SQLite mientras entran las subidas y los ingresos; aun
  así, ninguna petición falló.
- **El límite de 30 intentos de ingreso por minuto es por IP real.** La primera corrida,
  con todos los usuarios desde 127.0.0.1, recibió 170 respuestas 429: es la protección
  funcionando. En producción la IP la da el proxy en `X-Forwarded-For` (los dos
  ejemplos, nginx y Caddy, la envían). Visitantes detrás de una misma red (una
  universidad) comparten ese límite: 30 ingresos por minuto desde una sola IP es holgado
  para una convocatoria.

## Queda

Repetir la prueba en el VPS (mismo comando, contra 127.0.0.1:8080 de una instalación de
prueba) para medir con su disco y sus núcleos.
