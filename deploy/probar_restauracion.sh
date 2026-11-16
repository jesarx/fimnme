#!/bin/sh
# FIMNME — prueba de restauración del respaldo, sin tocar producción.
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Un respaldo que nunca se restauró no es un respaldo. Este guion lo hace
# cada semana (fimnme-restauracion.timer) o a mano:
#     sudo systemctl start fimnme-restauracion.service
#
#   1. Comprueba las sumas SHA-256 de las instantáneas (sha256sum -c).
#   2. Restaura la instantánea más reciente y el acervo en un directorio
#      TEMPORAL, como se haría en un servidor nuevo.
#   3. Corre «fimnme-cli verify» sobre la copia restaurada: abre la base y
#      comprueba que cada archivo registrado exista y coincida con su
#      SHA-256.
#   4. Borra la copia y deja el resultado en el diario de systemd
#      (journalctl -u fimnme-restauracion).
#
# El acervo no se copia: la copia restaurada lo recibe con enlaces duros
# (cp -al), que no ocupan espacio, dentro del mismo disco del respaldo.
# Copiar cientos de GB cada semana no aportaría nada a la prueba.

set -eu

CLI=${FIMNME_CLI:-/opt/fimnme/bin/fimnme-cli}
DESTINO=${FIMNME_RESPALDO_DESTINO:-/var/backups/fimnme}

cd "$DESTINO/instantaneas"
sha256sum -c --quiet SHA256SUMS
ultima=$(ls -1 fimnme-*.db | sort | tail -n 1)
[ -n "$ultima" ] || { echo "No hay instantáneas en $DESTINO/instantaneas" >&2; exit 1; }

prueba=$(mktemp -d "$DESTINO/restauracion.XXXXXX")
trap 'rm -rf "$prueba"' EXIT INT TERM
cp "$ultima" "$prueba/fimnme.db"
cp -al "$DESTINO/blobs" "$prueba/blobs"

echo "Restaurada $ultima en $prueba; verificando base y acervo…"
"$CLI" --datos "$prueba" verify
echo "Prueba de restauración correcta: $ultima"
