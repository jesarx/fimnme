# ADR 0027 — Imagen de contenedor, Compose y Quadlet

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Tercera entrega del paso 14 (§3.E: «empaquetado mínimo con Podman/Docker mediante
`Containerfile` y orquestación con `compose.yaml`; Podman rootless, con unidades
Quadlet/systemd»). Sin código Go nuevo.

## Decisión

- **La imagen no compila nada**: lleva los mismos binarios estáticos que `make build`
  sube al VPS. Hay un solo proceso de compilación y un solo binario que probar.
- **Base `gcr.io/distroless/static-debian12:nonroot`**: sin shell ni gestor de paquetes;
  sólo certificados raíz (SMTP con TLS, ligas de video), `/tmp` y el usuario 65532. Una
  etapa auxiliar de Debian crea el directorio de datos con su dueño, sin red. Se
  descartó `scratch` con `apt-get install ca-certificates`: exige red en la construcción
  y añade una etapa más sin ganar nada. Imagen: 49 MB.
- **`.dockerignore`** (que Podman también lee) deja entrar al contexto **sólo** los dos
  binarios y las páginas de manual: nunca `data/`, que tiene datos personales.
- **La migración es un paso aparte**, porque el servidor no migra por su cuenta: en
  Compose, el servicio `migrar` corre `fimnme-cli migrate` y `web` espera a que termine
  bien; en Quadlet, `ExecStartPre`.
- **Endurecimiento**: sistema de archivos de sólo lectura salvo el volumen y `/tmp`, sin
  capacidades, `no-new-privileges`, y el puerto publicado sólo en 127.0.0.1 (el proxy con
  TLS va delante, igual que sin contenedores).
- **`make imagen`** usa Podman si está y, si no, Docker.

## Comprobado (con Docker 29)

`make imagen`; `compose up`: la migración termina con 0, el servidor arranca en
producción con el sistema de archivos de sólo lectura, `/readyz` responde 200 y la
portada se sirve; `fimnme-cli crear-usuario` y `verify` corren desde la misma imagen; tras
`down` y `up` la cuenta creada sigue ahí (el volumen persiste).

**Sin comprobar:** la unidad Quadlet (no hay Podman en el entorno de desarrollo). Se
prueba en el servidor con `systemctl --user start fimnme-web` y `podman ps`.
