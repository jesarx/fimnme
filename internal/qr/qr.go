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

// Package qr genera códigos QR (ISO/IEC 18004) para el alta de la
// verificación en dos pasos: el enlace otpauth:// que lee la aplicación
// autenticadora con la cámara.
//
// Es deliberadamente pequeño (ADR 0032): sólo modo byte, nivel de corrección
// M (recupera ~15 % de daño) y versiones 1 a 10, hasta 213 bytes. Basta para
// ese enlace y evita una dependencia externa. Sigue la estructura del
// estándar paso a paso: datos en bits, Reed-Solomon por bloques,
// intercalado, patrones fijos, colocación en zigzag, máscara y formato.
package qr

import (
	"errors"
	"fmt"
)

// ErrDemasiadoLargo: el texto no cabe en un QR de versión 10, nivel M.
var ErrDemasiadoLargo = errors.New("el texto no cabe en un código QR de versión 10")

// Simbolo es un código QR ya armado: una cuadrícula de módulos.
type Simbolo struct {
	Tamano  int // módulos por lado, sin margen
	modulos [][]bool
}

// Oscuro dice si el módulo de la columna x y el renglón y es oscuro.
func (s *Simbolo) Oscuro(x, y int) bool {
	return s.modulos[y][x]
}

// bloques describe, para cada versión, la corrección de nivel M: cuántos
// bloques hay de cada longitud de datos y cuántos códigos de corrección
// lleva cada bloque (tabla 9 de ISO/IEC 18004).
type bloques struct {
	correccion int   // códigos de corrección por bloque
	datos      []int // longitud en códigos de datos de cada bloque
}

var nivelM = [11]bloques{
	1:  {10, []int{16}},
	2:  {16, []int{28}},
	3:  {26, []int{44}},
	4:  {18, []int{32, 32}},
	5:  {24, []int{43, 43}},
	6:  {16, []int{27, 27, 27, 27}},
	7:  {18, []int{31, 31, 31, 31}},
	8:  {22, []int{38, 38, 39, 39}},
	9:  {22, []int{36, 36, 36, 37, 37}},
	10: {26, []int{43, 43, 43, 43, 44}},
}

// alineacion son los centros de los patrones de alineación por versión.
var alineacion = [11][]int{
	2: {6, 18}, 3: {6, 22}, 4: {6, 26}, 5: {6, 30}, 6: {6, 34},
	7: {6, 22, 38}, 8: {6, 24, 42}, 9: {6, 26, 46}, 10: {6, 28, 50},
}

// Codificar arma el QR más pequeño que contiene los datos.
func Codificar(datos []byte) (*Simbolo, error) {
	version := 0
	for v := 1; v <= 10; v++ {
		bitsDeCuenta := 8
		if v >= 10 {
			bitsDeCuenta = 16
		}
		if 4+bitsDeCuenta+8*len(datos) <= 8*capacidad(v) {
			version = v
			break
		}
	}
	if version == 0 {
		return nil, fmt.Errorf("%w (%d bytes)", ErrDemasiadoLargo, len(datos))
	}
	codigos := conCorreccion(version, codigosDeDatos(version, datos))
	s := nuevoSimbolo(version)
	s.colocar(codigos)
	s.enmascarar()
	return &Simbolo{Tamano: s.tamano, modulos: s.modulos}, nil
}

// capacidad es cuántos códigos de datos admite una versión en nivel M.
func capacidad(version int) int {
	total := 0
	for _, n := range nivelM[version].datos {
		total += n
	}
	return total
}

// codigosDeDatos arma la secuencia de bits: modo byte (0100), longitud,
// datos, terminador y relleno hasta la capacidad.
func codigosDeDatos(version int, datos []byte) []byte {
	var b bufferDeBits
	b.agregar(0b0100, 4)
	if version >= 10 {
		b.agregar(len(datos), 16)
	} else {
		b.agregar(len(datos), 8)
	}
	for _, d := range datos {
		b.agregar(int(d), 8)
	}
	limite := 8 * capacidad(version)
	b.agregar(0, min(4, limite-len(b.bits)))
	for len(b.bits)%8 != 0 {
		b.bits = append(b.bits, false)
	}
	for relleno := 0xEC; len(b.bits) < limite; relleno ^= 0xEC ^ 0x11 {
		b.agregar(relleno, 8)
	}
	salida := make([]byte, len(b.bits)/8)
	for i, bit := range b.bits {
		if bit {
			salida[i/8] |= 1 << (7 - i%8)
		}
	}
	return salida
}

// bufferDeBits acumula bits, del más significativo al menos.
type bufferDeBits struct{ bits []bool }

func (b *bufferDeBits) agregar(valor, cuantos int) {
	for i := cuantos - 1; i >= 0; i-- {
		b.bits = append(b.bits, valor>>i&1 == 1)
	}
}

// conCorreccion parte los datos en bloques, calcula la corrección
// Reed-Solomon de cada uno y los intercala: primero el código i de cada
// bloque de datos, luego el código i de cada bloque de corrección.
func conCorreccion(version int, datos []byte) []byte {
	tabla := nivelM[version]
	divisor := divisorReedSolomon(tabla.correccion)
	var partes, correcciones [][]byte
	inicio := 0
	for _, n := range tabla.datos {
		parte := datos[inicio : inicio+n]
		inicio += n
		partes = append(partes, parte)
		correcciones = append(correcciones, restoReedSolomon(parte, divisor))
	}
	var salida []byte
	for i := 0; i < tabla.datos[len(tabla.datos)-1]; i++ {
		for _, parte := range partes {
			if i < len(parte) {
				salida = append(salida, parte[i])
			}
		}
	}
	for i := 0; i < tabla.correccion; i++ {
		for _, correccion := range correcciones {
			salida = append(salida, correccion[i])
		}
	}
	return salida
}

// multiplicar multiplica en GF(2⁸) con el polinomio x⁸+x⁴+x³+x²+1 (0x11D).
func multiplicar(a, b byte) byte {
	var producto int
	for i := 7; i >= 0; i-- {
		producto = producto<<1 ^ (producto>>7)*0x11D
		producto ^= int(b>>i&1) * int(a)
	}
	return byte(producto)
}

// divisorReedSolomon es el polinomio generador de grado n, sin su
// coeficiente principal: (x − α⁰)(x − α¹)…(x − αⁿ⁻¹).
func divisorReedSolomon(grado int) []byte {
	resultado := make([]byte, grado)
	resultado[grado-1] = 1
	raiz := byte(1)
	for range grado {
		for j := range resultado {
			resultado[j] = multiplicar(resultado[j], raiz)
			if j+1 < grado {
				resultado[j] ^= resultado[j+1]
			}
		}
		raiz = multiplicar(raiz, 0x02)
	}
	return resultado
}

// restoReedSolomon es el resto de dividir los datos entre el generador: los
// códigos de corrección.
func restoReedSolomon(datos, divisor []byte) []byte {
	resto := make([]byte, len(divisor))
	for _, b := range datos {
		factor := b ^ resto[0]
		copy(resto, resto[1:])
		resto[len(resto)-1] = 0
		for i, d := range divisor {
			resto[i] ^= multiplicar(d, factor)
		}
	}
	return resto
}

// lienzo es el QR mientras se arma: los módulos y cuáles son fijos
// (patrones y formato), que ni los datos ni la máscara tocan.
type lienzo struct {
	version, tamano int
	modulos, fijos  [][]bool
}

func nuevoSimbolo(version int) *lienzo {
	tamano := 17 + 4*version
	l := &lienzo{version: version, tamano: tamano}
	l.modulos, l.fijos = make([][]bool, tamano), make([][]bool, tamano)
	for i := range tamano {
		l.modulos[i], l.fijos[i] = make([]bool, tamano), make([]bool, tamano)
	}
	// Patrones de sincronía: renglón y columna 6, alternados.
	for i := range tamano {
		l.fijar(6, i, i%2 == 0)
		l.fijar(i, 6, i%2 == 0)
	}
	// Patrones de localización (con su separador) en tres esquinas.
	l.localizador(3, 3)
	l.localizador(tamano-4, 3)
	l.localizador(3, tamano-4)
	// Patrones de alineación, salvo donde chocan con los de localización.
	centros := alineacion[version]
	for i, x := range centros {
		for j, y := range centros {
			esquina := (i == 0 && j == 0) || (i == 0 && j == len(centros)-1) || (i == len(centros)-1 && j == 0)
			if !esquina {
				l.alineador(x, y)
			}
		}
	}
	// Se reservan las zonas de formato y versión; se escriben al final.
	l.formato(0)
	l.versionInfo()
	return l
}

// fijar pinta un módulo de un patrón fijo.
func (l *lienzo) fijar(x, y int, oscuro bool) {
	l.modulos[y][x], l.fijos[y][x] = oscuro, true
}

// localizador dibuja un patrón de localización de 7×7 con centro (x, y) y
// su separador claro alrededor.
func (l *lienzo) localizador(x, y int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			xx, yy := x+dx, y+dy
			if xx < 0 || xx >= l.tamano || yy < 0 || yy >= l.tamano {
				continue
			}
			distancia := max(abs(dx), abs(dy))
			l.fijar(xx, yy, distancia != 2 && distancia != 4)
		}
	}
}

// alineador dibuja un patrón de alineación de 5×5 con centro (x, y).
func (l *lienzo) alineador(x, y int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			l.fijar(x+dx, y+dy, max(abs(dx), abs(dy)) != 1)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// formato escribe las dos copias de la información de formato: nivel M
// (00) y la máscara, con su BCH(15,5) y la máscara fija 0x5412.
func (l *lienzo) formato(mascara int) {
	datos := mascara // nivel M = 00
	resto := datos
	for range 10 {
		resto = resto<<1 ^ (resto>>9)*0x537
	}
	bits := (datos<<10 | resto) ^ 0x5412
	bit := func(i int) bool { return bits>>i&1 == 1 }
	for i := 0; i <= 5; i++ {
		l.fijar(8, i, bit(i))
	}
	l.fijar(8, 7, bit(6))
	l.fijar(8, 8, bit(7))
	l.fijar(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		l.fijar(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		l.fijar(l.tamano-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		l.fijar(8, l.tamano-15+i, bit(i))
	}
	l.fijar(8, l.tamano-8, true) // el módulo oscuro fijo
}

// versionInfo escribe la información de versión (desde la versión 7), con
// su BCH(18,6), en las dos esquinas que le corresponden.
func (l *lienzo) versionInfo() {
	if l.version < 7 {
		return
	}
	resto := l.version
	for range 12 {
		resto = resto<<1 ^ (resto>>11)*0x1F25
	}
	bits := l.version<<12 | resto
	for i := range 18 {
		oscuro := bits>>i&1 == 1
		a, b := l.tamano-11+i%3, i/3
		l.fijar(a, b, oscuro)
		l.fijar(b, a, oscuro)
	}
}

// colocar pone los códigos en zigzag: columnas de dos en dos, de derecha a
// izquierda, subiendo y bajando, saltando la columna 6 y los módulos fijos.
func (l *lienzo) colocar(codigos []byte) {
	i := 0
	for derecha := l.tamano - 1; derecha >= 1; derecha -= 2 {
		if derecha == 6 {
			derecha = 5
		}
		for vertical := range l.tamano {
			for j := range 2 {
				x := derecha - j
				y := vertical
				if (derecha+1)&2 == 0 {
					y = l.tamano - 1 - vertical // hacia arriba
				}
				if l.fijos[y][x] || i >= len(codigos)*8 {
					continue
				}
				l.modulos[y][x] = codigos[i>>3]>>(7-i&7)&1 == 1
				i++
			}
		}
	}
}

// aplicaMascara indica si la máscara invierte el módulo (x, y).
func aplicaMascara(mascara, x, y int) bool {
	switch mascara {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

// invertir aplica (o deshace) una máscara sobre los módulos de datos.
func (l *lienzo) invertir(mascara int) {
	for y := range l.tamano {
		for x := range l.tamano {
			if !l.fijos[y][x] && aplicaMascara(mascara, x, y) {
				l.modulos[y][x] = !l.modulos[y][x]
			}
		}
	}
}

// enmascarar prueba las ocho máscaras y se queda con la de menor
// penalización, como pide el estándar para que el lector no se confunda.
func (l *lienzo) enmascarar() {
	mejor, menor := 0, -1
	for mascara := range 8 {
		l.invertir(mascara)
		l.formato(mascara)
		if p := l.penalizacion(); menor < 0 || p < menor {
			mejor, menor = mascara, p
		}
		l.invertir(mascara) // se deshace
	}
	l.invertir(mejor)
	l.formato(mejor)
}

// penalizacion aplica las cuatro reglas del estándar: tramos largos del
// mismo color, bloques de 2×2, figuras parecidas a un localizador y
// desequilibrio entre oscuros y claros.
func (l *lienzo) penalizacion() int {
	n := l.tamano
	en := func(x, y int, porRenglon bool) bool {
		if porRenglon {
			return l.modulos[y][x]
		}
		return l.modulos[x][y]
	}
	total := 0
	for _, porRenglon := range []bool{true, false} {
		for y := range n {
			tramo := 1
			for x := 1; x < n; x++ {
				if en(x, y, porRenglon) == en(x-1, y, porRenglon) {
					tramo++
					continue
				}
				if tramo >= 5 {
					total += 3 + tramo - 5
				}
				tramo = 1
			}
			if tramo >= 5 {
				total += 3 + tramo - 5
			}
			// 1:1:3:1:1 con cuatro claros de un lado (regla 3).
			for x := 0; x+11 <= n; x++ {
				var patron [11]bool
				for k := range 11 {
					patron[k] = en(x+k, y, porRenglon)
				}
				if patron == [11]bool{true, false, true, true, true, false, true, false, false, false, false} ||
					patron == [11]bool{false, false, false, false, true, false, true, true, true, false, true} {
					total += 40
				}
			}
		}
	}
	oscuros := 0
	for y := range n {
		for x := range n {
			if l.modulos[y][x] {
				oscuros++
			}
			if x+1 < n && y+1 < n {
				c := l.modulos[y][x]
				if c == l.modulos[y][x+1] && c == l.modulos[y+1][x] && c == l.modulos[y+1][x+1] {
					total += 3
				}
			}
		}
	}
	porcentaje := oscuros * 100 / (n * n)
	return total + abs(porcentaje-50)/5*10
}
