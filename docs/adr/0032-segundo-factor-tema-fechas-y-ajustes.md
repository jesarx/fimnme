# ADR 0032 — Verificación en dos pasos, tema elegible, fechas dd/mm/aaaa y ajustes

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Ajustes pedidos tras la puesta en operación del paso 14, al probar el sistema en el VPS.
Sin dependencias nuevas: el TOTP usa `crypto/hmac` y `crypto/sha1`, y el código QR es
un codificador propio y pequeño (§2).

## 1. Verificación en dos pasos (RF-AUT-09)

**Requisito.** Segundo factor **obligatorio** para administrador y observador, las dos
cuentas que lo ven todo, calificaciones incluidas. Era el pendiente más recomendable
del Manual de Validación.

**Decisión.**

- **TOTP (RFC 6238).**
  - Parámetros: SHA-1, 6 cifras e intervalos de 30 s, los que entienden todas las
    aplicaciones autenticadoras.
  - Clave: 160 bits en Base32.
  - Se acepta el intervalo anterior y el siguiente, por la deriva del reloj del
    teléfono.
  - Comprobado con los vectores del apéndice B del RFC.
- **Flujo.**
  - La contraseña correcta abre una sesión **pendiente** de 10 minutos (columna
    `sesion.pendiente_segundo_factor`).
  - Esa sesión viaja en el contexto con otra clave: `sesionDe()` no la ve.
    Para el resto del sistema es anónima, las páginas con sesión llevan a
    `/cuenta/segundo-factor` y la API responde 401 `segundo_factor_pendiente`.
  - Con el código, la sesión se vuelve plena.
- **Alta en el primer ingreso.**
  - La página muestra la clave en QR, en texto (en grupos de cuatro) y como enlace
    `otpauth://`.
  - Se confirma con un primer código. Recargar la página no cambia la clave.
  - Después se muestran **una sola vez** diez códigos de recuperación
    (`xxxx-xxxx`, de un solo uso; se guarda su resumen SHA-256). Se responden con
    la página, no con una redirección, para que no pasen por ninguna URL.
- **Un código no vale dos veces.**
  - Se guarda el último intervalo aceptado (`usuario.totp_ultimo_paso`) y sólo se
    acepta uno posterior.
  - La condición va en el `UPDATE` de la misma transacción que completa la sesión.
    Así, dos peticiones simultáneas no pueden usar el mismo código. Lo mismo vale
    para los códigos de recuperación.
- **Bloqueo por intentos.**
  - Un código equivocado cuenta como una contraseña equivocada (RF-AUT-08).
  - La contraseña correcta se asienta como `sesion.contrasena_correcta`, **no** como
    `sesion.ingreso`, que es lo que reinicia el contador. Si no fuera así, quien
    conoce la contraseña podría probar códigos sin fin volviendo a escribirla.
  - El ingreso se asienta al completar el segundo paso, con el medio usado.
- **Códigos nuevos** desde «Mi cuenta», con un código vigente de la aplicación.
- **Teléfono perdido:** `fimnme-cli segundo-factor restablecer --email`.
  - Borra la clave y los códigos y cierra las sesiones.
  - No existe en la web: quitar el segundo paso de un administrador exige acceso
    al servidor.
- **La clave se guarda en claro en la base.** Cifrarla exigiría otra clave guardada
  en el mismo servidor, sin ganancia real frente a quien ya lee la base. La base y
  sus respaldos ya se tratan como material sensible (ADR 0026).

**Límite conocido.** Si a una cuenta con sesión abierta se le **otorga** el perfil de
administrador u observador, el segundo paso se le pide en su siguiente ingreso, no en
esa sesión. Hoy esos perfiles sólo se otorgan al crear la cuenta.

## 2. Código QR propio (`internal/qr`)

Generar el QR en el servidor evita depender de un servicio externo, que vería la
clave, o de JavaScript. Una biblioteca de QR exigiría una dependencia. El codificador:

- tiene unas 400 líneas;
- sigue el estándar paso a paso: modo byte, nivel M, versiones 1 a 10 (hasta 213
  bytes), Reed-Solomon por bloques, intercalado, patrones, zigzag, ocho máscaras con
  sus penalizaciones, formato y versión;
- se dibuja como SVG en línea, negro sobre blanco con su margen, también en el tema
  oscuro (muchas cámaras no leen un QR invertido).

**Comprobado con un decodificador independiente (zbar):** 17 tamaños que cubren las
diez versiones y sus bordes, las ocho máscaras forzadas en cuatro versiones, y el QR
de la página real fotografiado en el navegador. Todo se leyó bien. Las pruebas
permanentes cuidan propiedades del estándar y la huella de un símbolo verificado.

## 3. Tema claro u oscuro, a elección

Antes el tema seguía sólo al sistema operativo, sin forma de cambiarlo.

- Un botón en la cabecera recorre automático → claro → oscuro.
- La elección se guarda en una cookie y el servidor la pinta en
  `<html data-tema="…">`: sin JavaScript y sin el parpadeo de una página que primero
  sale en el otro tema.
- CSS no deja unir una consulta de medios y un selector en una sola regla, así que la
  paleta oscura va dos veces en `tema.css`. `TestPaletasOscurasIdenticas` exige que
  sean iguales.

## 4. Fechas con la convención mexicana: dd/mm/aaaa y 24 horas

Los controles `date`, `datetime-local` y `time` muestran el formato del **idioma del
navegador**, no el de la página: en uno en inglés salen como mm/dd/yyyy con AM/PM, y la
página no puede cambiarlo. Por eso son ahora campos de texto con el formato a la vista
(«dd/mm/aaaa», «hh:mm»). Aplica a:

- el formulario del postulante;
- el calendario del panel;
- la fecha límite de una subsanación;
- los filtros del archivo y de la bitácora;
- el modo interactivo de `fimnme-cli`.

`controllers/fechas.go` traduce entre lo que se escribe y lo que se guarda, que no
cambia: AAAA-MM-DD, HH:MM y UTC.

- **Se acepta también** d/m/aaaa, con guiones o puntos, y el formato ISO de antes,
  para que sirvan los enlaces y las vistas guardadas.
- **La API no cambia.**
- **Una fecha ilegible se rechaza con su error**, en vez de ignorarse: antes el
  navegador lo impedía, y tomar otra en su lugar daría un plazo que nadie eligió.

## 5. Duración mínima del programa

Junto a la máxima, la convocatoria fija una duración **mínima** del programa en minutos.

- **Valor por omisión:** 0, sin mínimo (migración 0008).
- **Regla de envío:** se exige al enviar, sólo si hay obras.
- **Coherencia:** no puede ser mayor que la máxima, porque haría imposible enviar.
- **Dónde se edita:** en el panel, la API (y su OpenAPI) y el modo interactivo.

## 6. Enlace de activación desde la terminal

`fimnme-cli enlace-activacion --email` emite e imprime un enlace nuevo para una cuenta
interna sin activar. Sirve en un servidor que no envía correo, o cuando el enlace se
perdió. Anula los anteriores y queda en la bitácora.

## Consecuencias

- Pruebas: 227, todas en verde. Cobertura del dominio: 81.8 %.
- Auditoría de accesibilidad, después de estos cambios: 0 violaciones, en el flujo del
  postulante y en las páginas del segundo paso, en claro y en oscuro.
- **La cobertura debe medirse con `-count=1`.** Un resultado de prueba en caché trae
  el perfil de cobertura de una versión anterior del código, con otros números de
  línea, y la suma sale falsa (49.8 % en lugar de 81.8 %). Se corrigió en `make test` y
  en el Manual de Validación.
