// Este archivo es parte de FIMNME, la plataforma de postulaciones del Foro
// Internacional de Música Nueva Manuel Enríquez (CNMO / INBAL).
//
// FIMNME es software libre: usted puede redistribuirlo y/o modificarlo bajo
// los términos de la Licencia Pública General de GNU, tal como la publica la
// Free Software Foundation, en la versión 3 de la Licencia.
//
// FIMNME se distribuye con la esperanza de que sea útil, pero SIN GARANTÍA
// ALGUNA; ni siquiera la garantía implícita de COMERCIABILIDAD o de APTITUD
// PARA UN PROPÓSITO PARTICULAR. Consulte la Licencia Pública General de GNU
// para más detalles.
//
// Debió recibir una copia de la Licencia Pública General de GNU junto con
// este programa, en el archivo LICENSE. Si no, consulte
// <https://www.gnu.org/licenses/>.
//
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Primitivas de seguridad de las cuentas: resumen de contraseñas con Argon2id
// (RF-AUT-03) y generación de secretos de un solo uso (tokens y sesiones).
// Decisiones y parámetros: ADR 0004.

// Parámetros de Argon2id. Son los mínimos recomendados por OWASP para un
// servidor pequeño. Viajan dentro de cada resumen, de modo que subirlos
// mañana no invalida las contraseñas guardadas hoy.
const (
	argonMemoriaKiB  = 19 * 1024 // 19 MiB por operación
	argonIteraciones = 2
	argonHilos       = 1
	argonBytesSal    = 16
	argonBytesClave  = 32
)

// PasswordSinDefinir es el valor de password_hash de una cuenta interna que
// aún no se activa. No es un resumen válido: ninguna contraseña lo satisface.
const PasswordSinDefinir = "!"

// turnosArgon limita cuántos resúmenes se calculan a la vez. Cada uno ocupa
// 19 MiB: sin límite, una ráfaga de inicios de sesión agotaría la memoria.
var turnosArgon = make(chan struct{}, 4)

// HashPassword devuelve el resumen Argon2id de una contraseña en el formato
// estándar PHC: $argon2id$v=19$m=19456,t=2,p=1$<sal>$<clave>.
func HashPassword(password string) (string, error) {
	sal := make([]byte, argonBytesSal)
	if _, err := rand.Read(sal); err != nil {
		return "", fmt.Errorf("generar sal: %w", err)
	}
	turnosArgon <- struct{}{}
	clave := argon2.IDKey([]byte(password), sal, argonIteraciones, argonMemoriaKiB, argonHilos, argonBytesClave)
	<-turnosArgon
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemoriaKiB, argonIteraciones, argonHilos, b64.EncodeToString(sal), b64.EncodeToString(clave)), nil
}

// VerificarPassword indica si la contraseña corresponde al resumen. Usa los
// parámetros guardados en el propio resumen y compara en tiempo constante.
// Un resumen malformado (p. ej. PasswordSinDefinir) nunca verifica.
func VerificarPassword(resumen, password string) bool {
	// Formato: "", "argon2id", "v=19", "m=…,t=…,p=…", sal, clave.
	partes := strings.Split(resumen, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memoria, iteraciones uint32
	var hilos uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memoria, &iteraciones, &hilos); err != nil {
		return false
	}
	// Cotas de cordura: un resumen manipulado no debe poder pedir 4 GB de RAM.
	if memoria == 0 || memoria > 1<<20 || iteraciones == 0 || iteraciones > 16 || hilos == 0 {
		return false
	}
	b64 := base64.RawStdEncoding
	sal, err := b64.DecodeString(partes[4])
	if err != nil {
		return false
	}
	esperada, err := b64.DecodeString(partes[5])
	if err != nil || len(esperada) == 0 {
		return false
	}
	turnosArgon <- struct{}{}
	obtenida := argon2.IDKey([]byte(password), sal, iteraciones, memoria, hilos, uint32(len(esperada)))
	<-turnosArgon
	return subtle.ConstantTimeCompare(obtenida, esperada) == 1
}

var (
	senueloUnaVez sync.Once
	senuelo       string
)

// GastarVerificacion ejecuta una verificación de contraseña contra un resumen
// señuelo y descarta el resultado. Se llama cuando el correo no corresponde a
// ninguna cuenta, para que la respuesta tarde lo mismo que si existiera y el
// tiempo de respuesta no delate qué correos están registrados.
func GastarVerificacion(password string) {
	senueloUnaVez.Do(func() {
		resumen, err := HashPassword("contraseña señuelo que nadie usa")
		if err != nil {
			// Sólo falla si el sistema no puede dar números aleatorios. Sin
			// señuelo no hay verificación que gastar: se sigue sin ella, que
			// es preferible a negar el servicio.
			return
		}
		senuelo = resumen
	})
	if senuelo != "" {
		VerificarPassword(senuelo, password)
	}
}

// NuevoSecreto genera un secreto aleatorio de 256 bits para un token de un
// solo uso o un identificador de sesión. Devuelve el valor en claro —que sólo
// viaja al usuario, en un enlace o una cookie— y su resumen SHA-256, que es lo
// ÚNICO que se guarda en la base: quien robe la base o un respaldo no obtiene
// enlaces ni sesiones utilizables.
func NuevoSecreto() (claro, resumen string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generar secreto: %w", err)
	}
	claro = base64.RawURLEncoding.EncodeToString(bytes)
	return claro, ResumenDeSecreto(claro), nil
}

// ResumenDeSecreto calcula el resumen con el que un secreto se busca en la
// base. SHA-256 basta: el secreto ya es aleatorio y de 256 bits, no una
// contraseña elegida por una persona.
func ResumenDeSecreto(claro string) string {
	suma := sha256.Sum256([]byte(claro))
	return hex.EncodeToString(suma[:])
}

// ---------------------------------------------------------------------------
// Verificación en dos pasos: TOTP (RFC 6238) y códigos de recuperación
// ---------------------------------------------------------------------------
//
// Con la biblioteca estándar: HMAC-SHA1 sobre el número de intervalos de 30
// segundos desde 1970, truncado a 6 cifras (RFC 4226, §5.3). Son los
// parámetros que entienden todas las aplicaciones autenticadoras (Google
// Authenticator, Microsoft Authenticator, Authy, FreeOTP, 2FAS, Aegis…).

// Parámetros del TOTP.
const (
	intervaloTOTP = 30 * time.Second
	cifrasTOTP    = 6
	// toleranciaTOTP es cuántos intervalos antes y después del actual se
	// aceptan, para absorber la deriva del reloj del teléfono.
	toleranciaTOTP = 1
	// EmisorTOTP es el nombre con que la cuenta aparece en la aplicación.
	EmisorTOTP = "FIMNME"
	// CodigosDeRecuperacion es cuántos códigos de un solo uso se entregan.
	CodigosDeRecuperacion = 10
)

// codificacionTOTP es Base32 sin relleno, en mayúsculas: lo que esperan las
// aplicaciones en el URI otpauth://.
var codificacionTOTP = base32.StdEncoding.WithPadding(base32.NoPadding)

// NuevoSecretoTOTP genera una clave compartida de 160 bits (la longitud del
// HMAC-SHA1, como recomienda el RFC 4226), en Base32.
func NuevoSecretoTOTP() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generar clave TOTP: %w", err)
	}
	return codificacionTOTP.EncodeToString(bytes), nil
}

// PasoTOTP es el número de intervalo en que cae un instante.
func PasoTOTP(instante time.Time) int64 {
	return instante.Unix() / int64(intervaloTOTP/time.Second)
}

// CodigoTOTP calcula el código de un intervalo. secreto va en Base32
// (se toleran espacios y minúsculas, como lo copiaría una persona).
func CodigoTOTP(secreto string, paso int64) (string, error) {
	clave, err := codificacionTOTP.DecodeString(strings.ToUpper(strings.ReplaceAll(secreto, " ", "")))
	if err != nil {
		return "", fmt.Errorf("clave TOTP ilegible: %w", err)
	}
	var contador [8]byte
	binary.BigEndian.PutUint64(contador[:], uint64(paso))
	mac := hmac.New(sha1.New, clave)
	mac.Write(contador[:])
	suma := mac.Sum(nil)
	desplazamiento := suma[len(suma)-1] & 0x0f
	numero := binary.BigEndian.Uint32(suma[desplazamiento:desplazamiento+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", cifrasTOTP, numero%1_000_000), nil
}

// VerificarTOTP comprueba un código contra el intervalo de ahora y sus
// vecinos, y devuelve el intervalo que coincidió. No acepta intervalos
// iguales o anteriores a ultimoPaso: un código ya usado no vale otra vez
// (RFC 6238, §5.2). La comparación es de tiempo constante.
func VerificarTOTP(secreto, codigo string, ahora time.Time, ultimoPaso int64) (int64, bool) {
	codigo = strings.ReplaceAll(strings.TrimSpace(codigo), " ", "")
	if len(codigo) != cifrasTOTP {
		return 0, false
	}
	actual := PasoTOTP(ahora)
	for paso := actual - toleranciaTOTP; paso <= actual+toleranciaTOTP; paso++ {
		if paso <= ultimoPaso {
			continue
		}
		esperado, err := CodigoTOTP(secreto, paso)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(esperado), []byte(codigo)) == 1 {
			return paso, true
		}
	}
	return 0, false
}

// URIDeAltaTOTP es el enlace otpauth:// que lee la aplicación (desde el QR o
// al pulsarlo en el teléfono).
func URIDeAltaTOTP(secreto, cuenta string) string {
	etiqueta := url.PathEscape(EmisorTOTP + ":" + cuenta)
	parametros := url.Values{"secret": {secreto}, "issuer": {EmisorTOTP}, "algorithm": {"SHA1"},
		"digits": {fmt.Sprint(cifrasTOTP)}, "period": {fmt.Sprint(int(intervaloTOTP / time.Second))}}
	return "otpauth://totp/" + etiqueta + "?" + parametros.Encode()
}

// alfabetoDeRecuperacion evita los caracteres que se confunden al copiarlos
// a mano (0/o, 1/l/i).
const alfabetoDeRecuperacion = "abcdefghjkmnpqrstuvwxyz23456789"

// NuevosCodigosDeRecuperacion genera los códigos de un solo uso, con la forma
// «xxxx-xxxx» (unos 39 bits cada uno), y sus resúmenes para guardarlos.
func NuevosCodigosDeRecuperacion() (claros, resumenes []string, err error) {
	for range CodigosDeRecuperacion {
		bytes := make([]byte, 8)
		if _, err := rand.Read(bytes); err != nil {
			return nil, nil, fmt.Errorf("generar códigos de recuperación: %w", err)
		}
		var codigo strings.Builder
		for i, b := range bytes {
			if i == 4 {
				codigo.WriteByte('-')
			}
			codigo.WriteByte(alfabetoDeRecuperacion[int(b)%len(alfabetoDeRecuperacion)])
		}
		claros = append(claros, codigo.String())
		resumenes = append(resumenes, ResumenDeCodigoDeRecuperacion(codigo.String()))
	}
	return claros, resumenes, nil
}

// ResumenDeCodigoDeRecuperacion normaliza lo escrito (minúsculas, sin
// espacios, con o sin guion) y lo resume para buscarlo.
func ResumenDeCodigoDeRecuperacion(codigo string) string {
	limpio := strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(codigo)))
	if len(limpio) == 8 {
		limpio = limpio[:4] + "-" + limpio[4:]
	}
	return ResumenDeSecreto("recuperacion:" + limpio)
}
