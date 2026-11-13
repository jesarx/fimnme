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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

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
