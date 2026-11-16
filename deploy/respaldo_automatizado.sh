#!/bin/sh
# FIMNME — respaldo automatizado de la base Y del acervo de archivos.
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Lo ejecuta el temporizador fimnme-respaldo.timer como el usuario «fimnme»;
# también puede lanzarse a mano:
#     sudo systemctl start fimnme-respaldo.service
#
# Qué hace, y si algo falla se detiene con error (systemd lo registra):
#   1. Instantánea en caliente y firmada (fimnme-cli mirror snapshot): la base
#      con VACUUM INTO, el acervo copiado por contenido (SHA-256) y un
#      manifiesto con firma HMAC-SHA256. El servicio sigue atendiendo.
#   2. La verifica: firma, base, integridad interna y que estén todos los
#      archivos que registra.
#   3. Deja en SHA256SUMS las sumas de las instantáneas, para comprobarlas
#      con «sha256sum -c» en cualquier máquina, sin la clave.
#   4. Si FIMNME_RESPALDO_REMOTO está definido, copia todo a otro servidor por
#      rsync sobre SSH. El acervo nunca se sobrescribe (cada archivo se llama
#      por su SHA-256): sólo viaja lo nuevo.
#
# Configuración: /etc/fimnme/respaldo.env (véase respaldo.env.ejemplo).

set -eu

CLI=${FIMNME_CLI:-/opt/fimnme/bin/fimnme-cli}
DATOS=${FIMNME_DATOS:-/var/lib/fimnme}
DESTINO=${FIMNME_RESPALDO_DESTINO:-/var/backups/fimnme}
CLAVE=${FIMNME_RESPALDO_CLAVE:-/etc/fimnme/espejo.clave}
CONSERVAR=${FIMNME_RESPALDO_CONSERVAR_DIAS:-90}
REMOTO=${FIMNME_RESPALDO_REMOTO:-}

echo "Respaldo de $DATOS hacia $DESTINO"
"$CLI" --datos "$DATOS" mirror snapshot --destino "$DESTINO" --clave "$CLAVE" --conservar-dias "$CONSERVAR"
"$CLI" mirror verify --destino "$DESTINO" --clave "$CLAVE"

# Sumas de las instantáneas (las bases; el acervo ya se nombra por su suma).
( cd "$DESTINO/instantaneas" && sha256sum -- *.db > SHA256SUMS.nuevo && mv SHA256SUMS.nuevo SHA256SUMS )
echo "Sumas SHA-256 en $DESTINO/instantaneas/SHA256SUMS"

if [ -n "$REMOTO" ]; then
    echo "Copia a $REMOTO"
    # El acervo primero y sin sobrescribir; las instantáneas después, para
    # que el remoto nunca tenga una base que apunte a archivos que aún no
    # llegaron. La clave NO viaja: el espejo tiene la suya, copiada aparte.
    rsync -a --ignore-existing -e "ssh -o BatchMode=yes" "$DESTINO/blobs/" "$REMOTO/blobs/"
    rsync -a --delete -e "ssh -o BatchMode=yes" "$DESTINO/instantaneas/" "$REMOTO/instantaneas/"
fi
echo "Respaldo terminado."
