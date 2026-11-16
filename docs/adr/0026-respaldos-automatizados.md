# ADR 0026 — Respaldos automatizados y prueba de restauración

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Segunda entrega del paso 14 (§3.D). Sin código Go nuevo: se apoya en
`fimnme-cli mirror` (instantánea en caliente con `VACUUM INTO`, acervo por contenido y
manifiesto HMAC-SHA256) y `fimnme-cli verify` (cada archivo registrado existe y coincide
con su SHA-256), del paso 10a.

## Decisión

- **`deploy/respaldo_automatizado.sh`**, cada 6 horas (`fimnme-respaldo.timer`, con
  `Persistent=true` para no perder uno si la máquina estuvo apagada): instantánea,
  verificación, `SHA256SUMS` de las instantáneas (comprobable sin la clave con
  `sha256sum -c`) y, opcional, copia por rsync/SSH. En la copia remota el acervo viaja
  primero y sin sobrescribir —cada archivo se nombra por su SHA-256, así que sólo viaja
  lo nuevo— y las instantáneas después, para que el remoto nunca tenga una base que
  apunte a archivos que no llegaron. La clave de firma no viaja.
- **`deploy/probar_restauracion.sh`**, cada domingo (`fimnme-restauracion.timer`): es la
  **prueba de restauración documentada** que pide el perfil. Comprueba las sumas, restaura
  la última instantánea en un directorio temporal dentro del destino, le da el acervo con
  enlaces duros (no ocupan espacio y no confunden la verificación, a diferencia de un
  enlace simbólico) y corre `verify`. Producción no se toca. El resultado queda en el
  diario de systemd.
- Ambas unidades corren como `fimnme`, con prioridad baja de CPU y disco, y sistema de
  archivos protegido salvo los directorios que necesitan.
- La clave se genera en el primer despliegue y **nunca** se reemplaza: sin ella no podrían
  verificarse las instantáneas anteriores.

## Comprobado

Sobre la base de demostración: respaldo y prueba de restauración correctos; con un
archivo del acervo alterado, la prueba falla y lo nombra; con la instantánea alterada,
falla por su suma; el directorio temporal no queda.

**Sin comprobar aquí:** los temporizadores en un systemd real y la copia remota por SSH
(el entorno de desarrollo no los tiene). Se comprueban en el VPS con
`systemctl list-timers 'fimnme-*'` y lanzando cada servicio a mano.
