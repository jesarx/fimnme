#!/bin/sh
# FIMNME — servidor de correo de SALIDA: Postfix con firma DKIM (ADR 0030).
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Uso, como root, en el servidor de la instalación final (make deploy lo deja
# en /opt/fimnme/correo junto con opendkim.conf y comprobar_correo.sh):
#
#     sudo sh /opt/fimnme/correo/instalar_correo.sh DOMINIO [NOMBRE_DEL_SERVIDOR]
#
#   DOMINIO              dominio del remitente de los correos, el que va en
#                        FIMNME_REMITENTE (no-responder@DOMINIO).
#   NOMBRE_DEL_SERVIDOR  nombre completo de esta máquina, el que debe dar el
#                        DNS inverso (PTR) de su IP. Por omisión, hostname -f.
#
# Variables opcionales:
#   SELECTOR    selector DKIM. Por omisión «fimnme» y el año: fimnme2026.
#               Para cambiar de clave basta con un selector nuevo.
#   PROTOCOLOS  ipv4 (omisión) o all. Use «all» sólo si la IPv6 del servidor
#               también tiene PTR y está en el registro SPF.
#   BUZON       parte local del remitente: «no-responder» por omisión, como en
#               FIMNME_REMITENTE=FIMNME <no-responder@DOMINIO>.
#   REBOTES     buzón REAL, en otro servidor, al que se reenvían los avisos de
#               no entrega (rebotes). Sin él, los rebotes se descartan aquí.
#
# Qué hace (y puede repetirse sin daño):
#   1. Instala postfix, opendkim, opendkim-tools y las herramientas de DNS.
#   2. Configura Postfix para ENVIAR solamente: escucha sólo en 127.0.0.1, no
#      recibe correo de Internet, no es relevo de nadie, entrega con TLS
#      cuando el destino lo ofrece y no deja salir nada sin firmar.
#   3. Configura OpenDKIM y genera la clave de 2048 bits del selector, si no
#      existe. Una clave existente nunca se reemplaza.
#   4. Reinicia los dos servicios y muestra los registros DNS que faltan.
#
# Qué NO hace: no publica los registros DNS (los da para copiarlos) ni toca la
# configuración de fimnme-web; al final dice qué cambiar en ella.

set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "Este guion debe ejecutarse como root (use sudo)." >&2
    exit 1
fi
if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Uso: sh $0 DOMINIO [NOMBRE_DEL_SERVIDOR]" >&2
    exit 2
fi

dominio=$1
servidor=${2:-$(hostname -f)}
selector=${SELECTOR:-fimnme$(date +%Y)}
protocolos=${PROTOCOLOS:-ipv4}
buzon=${BUZON:-no-responder}
rebotes=${REBOTES:-}
origen=$(dirname "$0")

# Los nombres terminan en archivos de configuración y en rutas: sólo se
# admite la forma de un nombre de dominio, para que nada más pueda colarse.
nombre_valido() {
    printf '%s' "$1" | grep -Eq '^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$'
}
if ! nombre_valido "$dominio"; then
    echo "Dominio inválido: «$dominio» (en minúsculas, p. ej. fimnme.ejemplo.mx)." >&2
    exit 2
fi
if ! nombre_valido "$servidor"; then
    echo "Nombre de servidor inválido: «$servidor». Debe ser el nombre completo" >&2
    echo "de la máquina (p. ej. correo.ejemplo.mx); páselo como segundo argumento." >&2
    exit 2
fi
if ! printf '%s' "$selector" | grep -Eq '^[a-z0-9][a-z0-9-]{0,62}$'; then
    echo "Selector inválido: «$selector»." >&2
    exit 2
fi
if ! printf '%s' "$buzon" | grep -Eq '^[a-z0-9][a-z0-9._-]{0,63}$'; then
    echo "BUZON inválido: «$buzon»." >&2
    exit 2
fi
if [ -n "$rebotes" ] && ! printf '%s' "$rebotes" | grep -Eq '^[A-Za-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$'; then
    echo "REBOTES inválido: «$rebotes»." >&2
    exit 2
fi
case $protocolos in
    ipv4 | all) ;;
    *) echo "PROTOCOLOS debe ser ipv4 o all." >&2; exit 2 ;;
esac

# 1. Paquetes. «No configuration» evita las preguntas del instalador de
#    Postfix: la configuración completa la escribe el paso 2.
echo "postfix postfix/main_mailer_type select No configuration" | debconf-set-selections
DEBIAN_FRONTEND=noninteractive apt-get install -y -q postfix opendkim opendkim-tools bind9-dnsutils

# 2. Postfix, sólo de salida. Cada parámetro con su razón:
#    - loopback-only: sólo fimnme-web (en esta máquina) le entrega correo.
#    - mydestination vacío y entrega local desactivada: no es buzón de nadie.
#    - mynetworks 127.0.0.0/8 y restricciones: no es relevo abierto.
#    - smtp_tls_security_level=may: TLS siempre que el destino lo ofrezca.
#    - milter_default_action=tempfail: si OpenDKIM no responde, el correo se
#      detiene y fimnme-web lo reintenta; nunca sale sin firma (DMARC lo
#      rechazaría y dañaría la reputación del dominio).
#    - colas cortas: un aviso que tarda cinco días ya no sirve.
# Con «No configuration» el paquete no crea main.cf: se parte de uno vacío
# (los valores no escritos son los de fábrica de Postfix).
[ -f /etc/postfix/main.cf ] || install -o root -g root -m 0644 /dev/null /etc/postfix/main.cf
postconf -e \
    "compatibility_level = 3.6" \
    "myhostname = $servidor" \
    "mydomain = $dominio" \
    "myorigin = $dominio" \
    "inet_interfaces = loopback-only" \
    "inet_protocols = $protocolos" \
    "mydestination =" \
    "local_transport = error:este servidor no recibe correo" \
    "alias_maps =" \
    "alias_database =" \
    "relayhost =" \
    "mynetworks = 127.0.0.0/8" \
    "smtpd_relay_restrictions = permit_mynetworks, reject" \
    "smtpd_recipient_restrictions = permit_mynetworks, reject" \
    "disable_vrfy_command = yes" \
    "smtpd_banner = \$myhostname ESMTP" \
    "smtp_tls_security_level = may" \
    "smtp_tls_CAfile = /etc/ssl/certs/ca-certificates.crt" \
    "smtp_tls_loglevel = 1" \
    "smtpd_milters = inet:127.0.0.1:8891" \
    "non_smtpd_milters = inet:127.0.0.1:8891" \
    "milter_default_action = tempfail" \
    "milter_protocol = 6" \
    "message_size_limit = 26214400" \
    "maximal_queue_lifetime = 2d" \
    "bounce_queue_lifetime = 2d"

# Rebotes. Cuando un servidor de destino rechaza un correo, Postfix avisa al
# remitente del sobre ($buzon@$dominio), que no es un buzón: este servidor
# no recibe correo. Con REBOTES, el aviso se reescribe hacia ese buzón real;
# sin él, se descarta aquí en lugar de quedar dos días reintentándose.
# texthash no necesita postmap: Postfix lee el archivo tal cual.
if [ -n "$rebotes" ]; then
    printf '%s@%s %s\n' "$buzon" "$dominio" "$rebotes" > /etc/postfix/fimnme_rebotes
    postconf -e "recipient_canonical_maps = texthash:/etc/postfix/fimnme_rebotes" "transport_maps ="
else
    printf '%s@%s discard:sin buzon de rebotes\n' "$buzon" "$dominio" > /etc/postfix/fimnme_transporte
    postconf -e "transport_maps = texthash:/etc/postfix/fimnme_transporte" "recipient_canonical_maps ="
fi
postfix check

# 3. OpenDKIM: configuración, tablas y clave.
install -o root -g root -m 0644 "$origen/opendkim.conf" /etc/opendkim.conf
install -d -o opendkim -g opendkim -m 0750 /etc/opendkim /etc/opendkim/keys "/etc/opendkim/keys/$dominio"
clave="/etc/opendkim/keys/$dominio/$selector.private"
if [ ! -f "$clave" ]; then
    opendkim-genkey -b 2048 -h sha256 -d "$dominio" -s "$selector" -D "/etc/opendkim/keys/$dominio"
    echo "Clave DKIM nueva: $clave"
else
    echo "Se conserva la clave DKIM existente: $clave"
fi
chown opendkim:opendkim "/etc/opendkim/keys/$dominio/$selector".*
chmod 0600 "$clave"

# Las tablas se reescriben completas: firman SÓLO lo que sale de este dominio
# y con este selector. Una clave anterior queda en disco por si hay que volver
# a ella, pero ya no firma.
cat > /etc/opendkim/KeyTable <<TABLA
$selector._domainkey.$dominio $dominio:$selector:$clave
TABLA
cat > /etc/opendkim/SigningTable <<TABLA
*@$dominio $selector._domainkey.$dominio
TABLA
cat > /etc/opendkim/TrustedHosts <<TABLA
127.0.0.1
::1
localhost
TABLA
chown opendkim:opendkim /etc/opendkim/KeyTable /etc/opendkim/SigningTable /etc/opendkim/TrustedHosts
opendkim -n -x /etc/opendkim.conf

# 4. Servicios. Sin systemd (una prueba en contenedor) se omite este paso.
if [ -d /run/systemd/system ]; then
    systemctl enable --now opendkim postfix
    systemctl restart opendkim postfix
else
    echo "Sin systemd: arranque opendkim y postfix a mano."
fi

# Registros DNS. La IP pública se toma del DNS del nombre del servidor, que
# es la que verá el destinatario (la de la interfaz puede ser privada).
ip=$(dig +short A "$servidor" | tail -n 1)
[ -n "$ip" ] || ip="<IP pública del servidor>"
publica=$(sed -n 's/.*"\(.*\)".*/\1/p' "/etc/opendkim/keys/$dominio/$selector.txt" | tr -d '\n')

cat <<FIN

Postfix y OpenDKIM listos. Falta publicar en el DNS de $dominio:

  SPF    $dominio.  TXT  "v=spf1 ip4:$ip -all"
  DKIM   $selector._domainkey.$dominio.  TXT  "$publica"
  DMARC  _dmarc.$dominio.  TXT  "v=DMARC1; p=quarantine; adkim=s; aspf=s; rua=mailto:${REBOTES:-postmaster@$dominio}"

y pedir al proveedor del servidor el DNS inverso:

  PTR    $ip  ->  $servidor

(Si el buzón de los informes DMARC es de otro dominio, ese dominio debe
autorizarlo con un registro $dominio._report._dmarc.<otro dominio> TXT "v=DMARC1".
El registro DKIM es largo: algunos paneles de DNS lo piden partido en
fragmentos de 255 caracteres; es el mismo valor.)

Después, en /etc/fimnme/fimnme-web.env:

  FIMNME_CORREO=smtp
  FIMNME_SMTP_SERVIDOR=127.0.0.1:25
  FIMNME_SMTP_SEGURIDAD=ninguno
  FIMNME_REMITENTE=FIMNME <$buzon@$dominio>

reinicie fimnme-web y compruebe todo con:

  sudo bash /opt/fimnme/correo/comprobar_correo.sh $dominio $servidor
FIN
