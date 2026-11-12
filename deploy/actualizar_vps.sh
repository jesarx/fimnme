#!/bin/sh
# FIMNME — actualización del servicio en el VPS.
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Lo lanza `make deploy VPS=usuario@host` en cada despliegue, como root. Espera
# encontrar junto a sí mismo (en /tmp/fimnme-despliegue) los dos binarios
# recién compilados y la unidad de systemd.
#
# Secuencia:
#   1. Instala fimnme-web y fimnme-cli en /opt/fimnme/bin y la unidad de systemd.
#   2. Detiene el servicio (las peticiones en curso terminan).
#   3. Aplica las migraciones pendientes del esquema, como el usuario «fimnme».
#   4. Arranca el servicio y comprueba que quedó activo.
#
# Si una migración falla, la base queda como estaba (cada migración es una
# transacción) y el guion se detiene SIN arrancar el servicio: un esquema a
# medias no debe atender al público.

set -eu

# Los archivos que se creen (la base y su bitácora WAL) no serán legibles por
# otros usuarios del sistema: contienen datos personales.
umask 027

if [ "$(id -u)" -ne 0 ]; then
    echo "Este guion debe ejecutarse como root (use sudo)." >&2
    exit 1
fi
if ! id fimnme >/dev/null 2>&1; then
    echo "Falta la preparación inicial del VPS. Ejecute primero: make instalar-vps" >&2
    exit 1
fi

origen=$(dirname "$0")
datos=/var/lib/fimnme

# 1. Binarios, unidad de systemd y directorio de datos.
install -o root -g root -m 0755 "$origen/fimnme-web" /opt/fimnme/bin/fimnme-web
install -o root -g root -m 0755 "$origen/fimnme-cli" /opt/fimnme/bin/fimnme-cli
install -o root -g root -m 0644 "$origen/fimnme-web.service" /etc/systemd/system/fimnme-web.service
install -d -o fimnme -g fimnme -m 0750 "$datos"
systemctl daemon-reload

# 2. y 3. Con el servicio detenido, se migra la base como su dueño. Ejecutar
#         la herramienta como root dejaría archivos que el servicio no podría
#         escribir; fimnme-cli se niega a hacerlo.
systemctl stop fimnme-web.service
runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos "$datos" migrate

# 4. Arranque y comprobación.
systemctl start fimnme-web.service
sleep 1
systemctl is-active fimnme-web.service
/opt/fimnme/bin/fimnme-web --version

# Los archivos temporales del despliegue ya no hacen falta.
rm -rf "$origen"
