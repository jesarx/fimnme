#!/bin/sh
# FIMNME — migración VERIFICADA de SQLite a MariaDB (§3.C, ADR 0029).
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Uso (como el usuario fimnme, en el servidor donde están los datos):
#     verify_and_migrate_mariadb.sh BASE_DE_DESTINO
#
#   1. Exporta la base con «fimnme-cli exportar-mariadb»: esquema (con las
#      columnas extendidas), datos, restricciones y triggers, y un manifiesto
#      con filas y SHA-256 de cada tabla.
#   2. Crea la base de destino en utf8mb4 y se niega a seguir si ya tiene
#      tablas: nunca mezcla ni sobrescribe.
#   3. La carga en MariaDB, que comprueba las llaves foráneas y las
#      restricciones CHECK de cada fila.
#   4. Vuelve a leer cada tabla desde MariaDB y compara filas y SHA-256 con el
#      manifiesto. Termina con error si una sola tabla difiere.
#
# La conexión: las opciones del cliente de MariaDB (~/.my.cnf del usuario que
# lo ejecuta, con host, user y password), o el comando completo en
# MARIADB_CLIENTE (por omisión «mariadb»). Los archivos intermedios llevan
# datos personales: viven en un directorio temporal privado y se borran.

set -eu

base=${1:?Uso: verify_and_migrate_mariadb.sh BASE_DE_DESTINO}
CLI=${FIMNME_CLI:-/opt/fimnme/bin/fimnme-cli}
DATOS=${FIMNME_DATOS:-/var/lib/fimnme}
CLIENTE=${MARIADB_CLIENTE:-mariadb}

trabajo=$(mktemp -d)
chmod 0700 "$trabajo"
trap 'rm -rf "$trabajo"' EXIT INT TERM

echo "1. Exportando $DATOS"
"$CLI" --datos "$DATOS" exportar-mariadb --salida "$trabajo/migracion.sql" --manifiesto "$trabajo/manifiesto.tsv"

echo "2. Base de destino «$base»"
$CLIENTE -e "CREATE DATABASE IF NOT EXISTS \`$base\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
existentes=$($CLIENTE --batch --skip-column-names -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = '$base'")
if [ "$existentes" -ne 0 ]; then
    echo "La base «$base» ya tiene $existentes tablas: la migración sólo se hace sobre una base vacía." >&2
    exit 1
fi

echo "3. Cargando"
$CLIENTE "$base" < "$trabajo/migracion.sql"

echo "4. Verificando tabla por tabla"
fallas=0
verificadas=0
tab=$(printf '\t')
while IFS="$tab" read -r tabla filas suma columnas; do
    lista=$(printf '%s' "$columnas" | sed 's/,/`, `/g')
    # </dev/null: el cliente no debe leer la entrada del bucle (el manifiesto);
    # si lo hiciera, se saltaría tablas sin decir nada.
    obtenidas=$($CLIENTE --batch --skip-column-names "$base" -e "SELECT COUNT(*) FROM \`$tabla\`" </dev/null)
    obtenida=$($CLIENTE --batch --skip-column-names "$base" -e "SELECT \`$lista\` FROM \`$tabla\`" </dev/null | LC_ALL=C sort | sha256sum | cut -d' ' -f1)
    verificadas=$((verificadas + 1))
    if [ "$obtenidas" = "$filas" ] && [ "$obtenida" = "$suma" ]; then
        printf '   %-32s %8s filas  correcta\n' "$tabla" "$filas"
    else
        printf '   %-32s esperadas %s filas, hay %s; SHA-256 %s\n' "$tabla" "$filas" "$obtenidas" \
            "$( [ "$obtenida" = "$suma" ] && echo coincide || echo 'NO coincide')" >&2
        fallas=$((fallas + 1))
    fi
done < "$trabajo/manifiesto.tsv"

triggers=$($CLIENTE --batch --skip-column-names -e "SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema = '$base'" </dev/null)
echo "   triggers de protección: $triggers"
esperadas=$(wc -l < "$trabajo/manifiesto.tsv")
if [ "$verificadas" -ne "$esperadas" ]; then
    echo "Se verificaron $verificadas tablas de $esperadas: la verificación no está completa." >&2
    fallas=$((fallas + 1))
fi
if [ "$triggers" -ne 4 ]; then
    echo "Se esperaban 4 triggers de protección y hay $triggers." >&2
    fallas=$((fallas + 1))
fi
if [ "$fallas" -ne 0 ]; then
    echo "MIGRACIÓN NO VERIFICADA ($fallas problemas). No use la base «$base»." >&2
    exit 1
fi
echo "Migración verificada: cada tabla de MariaDB es idéntica a la de SQLite."
