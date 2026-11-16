#!/bin/bash
# FIMNME — comprobación del correo de salida (ADR 0030).
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Uso, en el servidor, después de instalar_correo.sh y de publicar el DNS:
#
#     sudo bash comprobar_correo.sh DOMINIO [NOMBRE_DEL_SERVIDOR]
#
# Revisa, uno por uno, lo que los grandes proveedores de correo miran antes
# de aceptar un mensaje, y dice qué falla. No envía nada. Sale con 0 si todo
# está bien y con el número de fallas si no.
#
# Variables: SELECTOR (por omisión, el de /etc/opendkim/KeyTable).
#
# Los cambios de DNS tardan en propagarse (minutos u horas, según el TTL):
# una falla recién publicado el registro puede ser sólo eso.

set -u

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Uso: bash $0 DOMINIO [NOMBRE_DEL_SERVIDOR]" >&2
    exit 2
fi
dominio=$1
servidor=${2:-$(hostname -f)}
selector=${SELECTOR:-$(awk '{print $1; exit}' /etc/opendkim/KeyTable 2>/dev/null | cut -d. -f1)}

fallas=0
bien() { printf '  [ok]    %s\n' "$1"; }
mal() { printf '  [FALLA] %s\n' "$1"; fallas=$((fallas + 1)); }
aviso() { printf '  [aviso] %s\n' "$1"; }

# Un TXT puede venir partido en varias cadenas entre comillas: se unen.
txt() { dig +short TXT "$1" | sed 's/" "//g; s/^"//; s/"$//'; }

# Sin DNS, todo lo demás parecería «registro faltante»: se dice la causa real.
if [ -z "$(dig +short +time=3 +tries=2 A gmail.com)" ]; then
    echo "Esta máquina no puede consultar el DNS (dig no obtiene respuesta);" >&2
    echo "nada de lo que sigue podría comprobarse. Revise /etc/resolv.conf." >&2
    exit 2
fi

echo "Servicios"
for servicio in postfix opendkim; do
    if systemctl is-active --quiet "$servicio" 2>/dev/null; then
        bien "$servicio en marcha"
    else
        mal "$servicio no está en marcha (systemctl status $servicio)"
    fi
done
if postconf -h inet_interfaces 2>/dev/null | grep -qx 'loopback-only'; then
    bien "Postfix sólo escucha en esta máquina (no es relevo abierto)"
else
    mal "Postfix escucha fuera de 127.0.0.1: revise inet_interfaces"
fi

echo "Nombre e IP del servidor"
ip=$(dig +short A "$servidor" | tail -n 1)
if [ -z "$ip" ]; then
    mal "$servidor no tiene registro A"
else
    bien "$servidor -> $ip"
    inverso=$(dig +short -x "$ip" | sed 's/\.$//' | head -n 1)
    if [ "$inverso" = "$servidor" ]; then
        bien "PTR $ip -> $servidor"
    else
        mal "PTR de $ip es «${inverso:-ninguno}», debe ser $servidor (se pide al proveedor)"
    fi
    nombre_postfix=$(postconf -h myhostname 2>/dev/null)
    if [ "$nombre_postfix" = "$servidor" ]; then
        bien "Postfix se presenta como $servidor"
    else
        mal "Postfix se presenta como «$nombre_postfix», no como $servidor"
    fi
fi

echo "Dominio del remitente: $dominio"
if [ -n "$(dig +short MX "$dominio")$(dig +short A "$dominio")" ]; then
    bien "$dominio existe en el DNS (MX o A)"
else
    mal "$dominio no tiene MX ni A: muchos servidores rechazan un remitente así"
fi
spf=$(txt "$dominio" | grep '^v=spf1' || true)
if [ -z "$spf" ]; then
    mal "sin registro SPF"
elif [ "$(printf '%s\n' "$spf" | wc -l)" -gt 1 ]; then
    mal "hay más de un registro SPF: debe haber uno solo"
elif [ -n "$ip" ] && printf '%s' "$spf" | grep -Fq "ip4:$ip"; then
    bien "SPF autoriza a $ip"
else
    aviso "SPF existe pero no menciona ip4:$ip: compruebe que la autoriza ($spf)"
fi
dmarc=$(txt "_dmarc.$dominio" | grep '^v=DMARC1' || true)
if [ -n "$dmarc" ]; then
    bien "DMARC: $dmarc"
else
    mal "sin registro DMARC en _dmarc.$dominio"
fi

echo "DKIM (selector $selector)"
if [ -z "$selector" ]; then
    mal "no hay selector: ¿se ejecutó instalar_correo.sh?"
else
    # Se compara la clave pública del DNS con la que generó instalar_correo.sh,
    # con el mismo resolvedor que las demás comprobaciones (opendkim-testkey
    # resuelve por su cuenta desde los servidores raíz y puede fallar donde el
    # DNS del sistema sí responde).
    local_p=$(sed -n 's/.*"\(.*\)".*/\1/p' "/etc/opendkim/keys/$dominio/$selector.txt" 2>/dev/null | tr -d '\n' | sed -n 's/.*p=\([A-Za-z0-9+\/=]*\).*/\1/p')
    dns_p=$(txt "$selector._domainkey.$dominio" | tr -d ' ' | sed -n 's/.*p=\([A-Za-z0-9+\/=]*\).*/\1/p')
    if [ -z "$local_p" ]; then
        mal "no está la clave local /etc/opendkim/keys/$dominio/$selector.txt"
    elif [ -z "$dns_p" ]; then
        mal "falta el registro TXT $selector._domainkey.$dominio"
    elif [ "$dns_p" = "$local_p" ]; then
        bien "el registro $selector._domainkey.$dominio corresponde a la clave local"
    else
        mal "el registro $selector._domainkey.$dominio NO corresponde a la clave local (¿se copió incompleto?)"
    fi
fi

echo "Salida a Internet por el puerto 25"
mx=$(dig +short MX gmail.com | sort -n | awk '{print $2; exit}' | sed 's/\.$//')
if [ -n "$mx" ] && timeout 10 bash -c "exec 3<>/dev/tcp/$mx/25" 2>/dev/null; then
    bien "se alcanza $mx:25"
else
    mal "no se alcanza el puerto 25 de ${mx:-gmail}: el proveedor lo bloquea"
fi

echo
if [ "$fallas" -eq 0 ]; then
    echo "Todo en orden. Falta la prueba de entrega (ADR 0030): «Enviar prueba» desde"
    echo "Administración > Plantillas de correo con una cuenta de Gmail y otra de Outlook,"
    echo "y revisar en las cabeceras: spf=pass, dkim=pass y dmarc=pass."
else
    echo "$fallas falla(s)."
fi
exit "$fallas"
