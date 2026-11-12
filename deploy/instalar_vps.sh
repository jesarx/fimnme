#!/bin/sh
# FIMNME — preparación INICIAL de un VPS con Debian 13 (trixie).
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Se ejecuta UNA sola vez, como root, y puede repetirse sin daño (es
# idempotente). Normalmente lo lanza `make instalar-vps VPS=usuario@host`,
# que copia este guion y sus dos archivos acompañantes a /tmp.
#
# Qué hace:
#   1. Crea el usuario de sistema «fimnme», sin contraseña ni shell.
#   2. Crea /opt/fimnme/bin (binarios) y /etc/fimnme (configuración).
#   3. Instala la unidad de systemd y la configuración de ejemplo.
#   4. Habilita el servicio para que arranque con la máquina.
#
# Qué NO hace: no instala el binario (eso es `make deploy`) ni el proxy
# inverso con TLS (ver README, sección de despliegue).

set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "Este guion debe ejecutarse como root (use sudo)." >&2
    exit 1
fi

# Los archivos acompañantes están junto a este guion.
origen=$(dirname "$0")

# 1. Usuario de sistema sin privilegios, dedicado al servicio.
if ! id fimnme >/dev/null 2>&1; then
    useradd --system --user-group --home-dir /var/lib/fimnme --no-create-home \
        --shell /usr/sbin/nologin fimnme
    echo "Usuario de sistema «fimnme» creado."
fi

# 2. Directorios. /var/lib/fimnme lo crea systemd (StateDirectory).
install -d -o root -g root -m 0755 /opt/fimnme /opt/fimnme/bin
install -d -o root -g fimnme -m 0750 /etc/fimnme

# 3. Unidad de systemd y configuración. La configuración existente se respeta.
install -o root -g root -m 0644 "$origen/fimnme-web.service" /etc/systemd/system/fimnme-web.service
if [ ! -f /etc/fimnme/fimnme-web.env ]; then
    install -o root -g fimnme -m 0640 "$origen/fimnme-web.env.ejemplo" /etc/fimnme/fimnme-web.env
    echo "Configuración inicial en /etc/fimnme/fimnme-web.env"
fi

# 4. Alta del servicio. Arrancará cuando exista el binario (make deploy).
systemctl daemon-reload
systemctl enable fimnme-web.service

echo
echo "VPS preparado. Siguiente paso, desde su computadora:"
echo "    make deploy VPS=usuario@este-servidor"
