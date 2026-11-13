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

package pdf

import "strings"

// Este archivo se ocupa del texto: convertirlo a la codificación de las
// fuentes base (WinAnsiEncoding), medirlo y partirlo en renglones.

// especialesWinAnsi son los caracteres de Windows-1252 que no coinciden con
// Latin-1: ocupan los códigos 0x80 a 0x9F.
var especialesWinAnsi = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89,
	'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
	'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// sinDiacritico da, para cada letra del bloque Latín Extendido-A de Unicode
// (U+0100 a U+017F), la letra básica que más se le parece. Son las letras de
// nombres checos, polacos, húngaros, turcos o rumanos que WinAnsi no tiene:
// «Dvořák» se escribe «Dvorák», que es preferible a «Dvo?ák».
const sinDiacritico = "AaAaAaCcCcCcCcDd" + "DdEeEeEeEeEeGgGg" + "GgGgHhHhIiIiIiIi" + "IiIiJjKkkLlLlLlL" +
	"lLlNnNnNnnNnOoOo" + "OoOoRrRrRrSsSsSs" + "SsTtTtTtUuUuUuUu" + "UuUuWwYyYZzZzZzs"

// codificar convierte un texto UTF-8 a los bytes de WinAnsiEncoding. Lo que
// no puede representarse se translitera o se sustituye por «?»; los saltos de
// línea y demás caracteres de control se vuelven espacios.
func codificar(texto string) string {
	var salida strings.Builder
	for _, r := range texto {
		switch {
		case r < 0x20 || r == 0x7F:
			salida.WriteByte(' ')
		case r < 0x7F, r >= 0xA0 && r <= 0xFF:
			salida.WriteByte(byte(r))
		case especialesWinAnsi[r] != 0:
			salida.WriteByte(especialesWinAnsi[r])
		case r >= 0x100 && r <= 0x17F:
			salida.WriteByte(sinDiacritico[r-0x100])
		case r >= 0x300 && r <= 0x36F:
			// Diacrítico combinante suelto: la letra anterior ya se escribió.
		case r == 'Ș' || r == 'Ş':
			salida.WriteByte('S')
		case r == 'ș':
			salida.WriteByte('s')
		case r == 'Ț':
			salida.WriteByte('T')
		case r == 'ț':
			salida.WriteByte('t')
		default:
			salida.WriteByte('?')
		}
	}
	return salida.String()
}

// Anchos de los caracteres ASCII imprimibles (del espacio a la tilde) en
// milésimas del cuerpo de la letra, tomados de las métricas públicas de las
// fuentes base (AFM de Adobe).
var anchosHelvetica = [95]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, // espacio ! " # $ % & ' ( ) * + , - . /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556, // 0-9 : ; < = > ?
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, // @ A-O
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556, // P-Z [ \ ] ^ _
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, // ` a-o
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584, // p-z { | } ~
}

var anchosHelveticaNegrita = [95]int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611,
	975, 722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333, 584, 556,
	333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611,
	611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584,
}

// parecidoA da, para cada código de WinAnsi de 0x80 a 0xFF, un carácter ASCII
// de ancho igual o muy cercano. Evita llevar otra tabla de métricas: «Á» mide
// lo que «A», «ñ» lo que «n», y las íes acentuadas lo que la «I» mayúscula.
// El ancho sólo se usa para partir renglones y alinear a la derecha, así que
// una diferencia de centésimas de punto no se nota.
const parecidoA = "0 ,0\"W00`WS(W Z " + " ii``*0W`Ws)W zY" +
	" (0000|0`G*0+-G`" + "*+```u0.``*0mmm?" +
	"AAAAAAWCEEEEIIII" + "DNOOOOO+OUUUUYPb" +
	"aaaaaamceeeeIIII" + "onooooo+ouuuuypy"

// anchoDeByte devuelve el ancho de un carácter ya codificado en WinAnsi.
func anchoDeByte(b byte, negrita bool) int {
	if b >= 0x80 {
		b = parecidoA[b-0x80]
	}
	if b < 0x20 || b > 0x7E {
		b = ' '
	}
	if negrita {
		return anchosHelveticaNegrita[b-0x20]
	}
	return anchosHelvetica[b-0x20]
}

// Ancho mide un texto, en puntos, para un cuerpo de letra dado.
func Ancho(texto string, negrita bool, cuerpo float64) float64 {
	milesimas := 0
	for _, b := range []byte(codificar(texto)) {
		milesimas += anchoDeByte(b, negrita)
	}
	return float64(milesimas) * cuerpo / 1000
}

// Envolver parte un texto en renglones que no pasen de anchoMax puntos. Corta
// en los espacios; respeta los saltos de línea del texto como cambios de
// renglón; y una palabra más larga que el renglón —una liga, una suma
// SHA-256— la parte donde haga falta.
func Envolver(texto string, negrita bool, cuerpo, anchoMax float64) []string {
	var renglones []string
	for _, parrafo := range strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n") {
		renglon := ""
		for _, palabra := range strings.Fields(parrafo) {
			candidato := palabra
			if renglon != "" {
				candidato = renglon + " " + palabra
			}
			if Ancho(candidato, negrita, cuerpo) <= anchoMax {
				renglon = candidato
				continue
			}
			if renglon != "" {
				renglones = append(renglones, renglon)
			}
			// La palabra empieza renglón; si aun así no cabe, se parte.
			renglon = ""
			for _, r := range palabra {
				if Ancho(renglon+string(r), negrita, cuerpo) > anchoMax && renglon != "" {
					renglones = append(renglones, renglon)
					renglon = ""
				}
				renglon += string(r)
			}
		}
		// Un párrafo vacío es un renglón en blanco deliberado.
		renglones = append(renglones, renglon)
	}
	// Sin renglones en blanco al final.
	for len(renglones) > 1 && renglones[len(renglones)-1] == "" {
		renglones = renglones[:len(renglones)-1]
	}
	return renglones
}
