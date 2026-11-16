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

package qr

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// La validez del codificador se comprobó leyendo con zbar QR de las diez
// versiones y de las ocho máscaras (ADR 0032). Estas pruebas cuidan que no
// cambie: propiedades del estándar y una huella de un símbolo conocido.

// Cada bloque con su corrección es divisible entre el generador: el resto de
// volver a dividir es cero. Es la propiedad que permite corregir errores.
func TestCorreccionReedSolomon(t *testing.T) {
	for _, grado := range []int{10, 16, 18, 22, 24, 26} {
		divisor := divisorReedSolomon(grado)
		datos := []byte("FIMNME · prueba de Reed-Solomon")
		completo := append(append([]byte{}, datos...), restoReedSolomon(datos, divisor)...)
		for _, b := range restoReedSolomon(completo, divisor) {
			if b != 0 {
				t.Fatalf("grado %d: el bloque con su corrección no es divisible", grado)
			}
		}
	}
	if multiplicar(0x80, 0x02) != 0x1D || multiplicar(0x53, 0xCA) != 0x8F {
		t.Error("multiplicación en GF(256) incorrecta")
	}
}

// Cada longitud cae en la versión más pequeña que la contiene, y un texto
// de más se rechaza.
func TestVersionYTamano(t *testing.T) {
	for longitud, version := range map[int]int{1: 1, 14: 1, 15: 2, 42: 3, 62: 4, 84: 5, 106: 6, 107: 7, 122: 7, 123: 8, 152: 8, 180: 9, 181: 10, 213: 10} {
		s, err := Codificar([]byte(strings.Repeat("a", longitud)))
		if err != nil {
			t.Errorf("%d bytes: %v", longitud, err)
		} else if s.Tamano != 17+4*version {
			t.Errorf("%d bytes: versión %d, se esperaba la %d", longitud, (s.Tamano-17)/4, version)
		}
	}
	if _, err := Codificar(make([]byte, 214)); !errors.Is(err, ErrDemasiadoLargo) {
		t.Errorf("214 bytes: %v", err)
	}
}

// Los patrones fijos están donde el lector los busca, y el símbolo de un
// enlace otpauth conocido no cambia (huella del dibujo leído con zbar).
func TestSimboloConocido(t *testing.T) {
	enlace := "otpauth://totp/FIMNME:ana%40ejemplo.mx?algorithm=SHA1&digits=6&issuer=FIMNME&period=30&secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	s, err := Codificar([]byte(enlace))
	if err != nil {
		t.Fatal(err)
	}
	for _, esquina := range [][2]int{{0, 0}, {s.Tamano - 7, 0}, {0, s.Tamano - 7}} {
		x, y := esquina[0], esquina[1]
		if !s.Oscuro(x, y) || !s.Oscuro(x+6, y+6) || s.Oscuro(x+1, y+1) || !s.Oscuro(x+3, y+3) {
			t.Errorf("localizador mal dibujado en %v", esquina)
		}
	}
	for i := 8; i < s.Tamano-8; i++ {
		if s.Oscuro(i, 6) != (i%2 == 0) || s.Oscuro(6, i) != (i%2 == 0) {
			t.Fatalf("patrón de sincronía mal en %d", i)
		}
	}
	var dibujo strings.Builder
	for y := range s.Tamano {
		for x := range s.Tamano {
			if s.Oscuro(x, y) {
				dibujo.WriteByte('#')
			} else {
				dibujo.WriteByte('.')
			}
		}
	}
	huella := fmt.Sprintf("%x", sha256.Sum256([]byte(dibujo.String())))
	t.Logf("versión %d, huella %s", (s.Tamano-17)/4, huella)
	if huella != huellaConocida {
		t.Errorf("el símbolo cambió: %s", huella)
	}
}

// huellaConocida es la del símbolo de TestSimboloConocido, comprobado con zbar.
const huellaConocida = "1ffe3f43404fedc95ebcc4183de03c43153b1cf0777b8e38ede8bd04ba4fbc2f"
