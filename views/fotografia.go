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

package views

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // registra el decodificador de PNG
)

// Preparación de la fotografía para el expediente en PDF (§5.12.4).
//
// Una fotografía de 5 MB multiplicada por doscientas postulaciones produce un
// PDF inmanejable. Por eso cada una se reduce a 800 pixeles de lado como
// máximo y se vuelve a comprimir como JPEG de calidad media antes de
// incrustarla: quedan unas decenas de KB. Todo con la biblioteca estándar.

const (
	// ladoMaxFotografia es el lado mayor, en pixeles, de la fotografía ya
	// reducida.
	ladoMaxFotografia = 800
	// calidadFotografia es la calidad JPEG con que se recomprime.
	calidadFotografia = 72
	// pixelesMaxFotografia acota lo que se acepta decodificar. Una imagen
	// declara sus dimensiones en unos pocos bytes: sin este tope, un archivo
	// pequeño podría pedir gigas de memoria al abrirse.
	pixelesMaxFotografia = 60_000_000
)

// ErrFotografiaIlegible indica que la fotografía no pudo prepararse: no es
// una imagen JPEG o PNG válida, o es desmesurada.
var ErrFotografiaIlegible = errors.New("la fotografía no pudo leerse")

// ReducirFotografia lee una fotografía JPEG o PNG y la devuelve lista para el
// PDF: derecha (según su orientación EXIF), sobre fondo blanco, de 800
// pixeles de lado como máximo y en JPEG RGB. Devuelve también sus
// dimensiones.
func ReducirFotografia(original []byte) (reducida []byte, ancho, alto int, err error) {
	dimensiones, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %v", ErrFotografiaIlegible, err)
	}
	if dimensiones.Width <= 0 || dimensiones.Height <= 0 || dimensiones.Width*dimensiones.Height > pixelesMaxFotografia {
		return nil, 0, 0, fmt.Errorf("%w: mide %d × %d pixeles", ErrFotografiaIlegible, dimensiones.Width, dimensiones.Height)
	}
	imagen, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %v", ErrFotografiaIlegible, err)
	}
	// Se pasa a RGBA sobre blanco: un PNG con transparencia no debe quedar
	// con el fondo negro.
	limites := imagen.Bounds()
	lienzo := image.NewRGBA(image.Rect(0, 0, limites.Dx(), limites.Dy()))
	draw.Draw(lienzo, lienzo.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(lienzo, lienzo.Bounds(), imagen, limites.Min, draw.Over)

	lienzo = enderezar(reducir(lienzo, ladoMaxFotografia), orientacionEXIF(original))
	var salida bytes.Buffer
	if err := jpeg.Encode(&salida, lienzo, &jpeg.Options{Quality: calidadFotografia}); err != nil {
		return nil, 0, 0, fmt.Errorf("recomprimir la fotografía: %w", err)
	}
	return salida.Bytes(), lienzo.Bounds().Dx(), lienzo.Bounds().Dy(), nil
}

// reducir achica una imagen para que su lado mayor no pase de ladoMax,
// promediando los pixeles de origen que caen en cada pixel de destino (así no
// aparecen los dientes de sierra de tomar uno de cada tantos). Una imagen que
// ya cabe se devuelve igual.
func reducir(origen *image.RGBA, ladoMax int) *image.RGBA {
	ancho, alto := origen.Bounds().Dx(), origen.Bounds().Dy()
	if ancho <= ladoMax && alto <= ladoMax {
		return origen
	}
	nuevoAncho, nuevoAlto := ladoMax, max(1, alto*ladoMax/ancho)
	if alto > ancho {
		nuevoAncho, nuevoAlto = max(1, ancho*ladoMax/alto), ladoMax
	}
	destino := image.NewRGBA(image.Rect(0, 0, nuevoAncho, nuevoAlto))
	for y := 0; y < nuevoAlto; y++ {
		// Renglones de origen que cubre este renglón de destino.
		y0, y1 := y*alto/nuevoAlto, max((y+1)*alto/nuevoAlto, y*alto/nuevoAlto+1)
		for x := 0; x < nuevoAncho; x++ {
			x0, x1 := x*ancho/nuevoAncho, max((x+1)*ancho/nuevoAncho, x*ancho/nuevoAncho+1)
			var rojo, verde, azul, cuenta int
			for sy := y0; sy < y1; sy++ {
				pixeles := origen.Pix[sy*origen.Stride+x0*4 : sy*origen.Stride+x1*4]
				for i := 0; i < len(pixeles); i += 4 {
					rojo, verde, azul = rojo+int(pixeles[i]), verde+int(pixeles[i+1]), azul+int(pixeles[i+2])
					cuenta++
				}
			}
			i := y*destino.Stride + x*4
			destino.Pix[i], destino.Pix[i+1], destino.Pix[i+2], destino.Pix[i+3] =
				uint8(rojo/cuenta), uint8(verde/cuenta), uint8(azul/cuenta), 255
		}
	}
	return destino
}

// enderezar gira una imagen según su orientación EXIF. Los teléfonos guardan
// la fotografía como salió del sensor y anotan aparte cómo hay que girarla;
// sin esto, un retrato tomado en vertical saldría acostado. Se atienden los
// tres giros (valores 3, 6 y 8); las orientaciones en espejo, que ninguna
// cámara produce al tomar una fotografía, se dejan como están.
func enderezar(origen *image.RGBA, orientacion int) *image.RGBA {
	ancho, alto := origen.Bounds().Dx(), origen.Bounds().Dy()
	var destino *image.RGBA
	// lugar dice a qué pixel de destino va el pixel (x, y) de origen.
	var lugar func(x, y int) (int, int)
	switch orientacion {
	case 3: // media vuelta
		destino = image.NewRGBA(image.Rect(0, 0, ancho, alto))
		lugar = func(x, y int) (int, int) { return ancho - 1 - x, alto - 1 - y }
	case 6: // un cuarto de vuelta a la derecha
		destino = image.NewRGBA(image.Rect(0, 0, alto, ancho))
		lugar = func(x, y int) (int, int) { return alto - 1 - y, x }
	case 8: // un cuarto de vuelta a la izquierda
		destino = image.NewRGBA(image.Rect(0, 0, alto, ancho))
		lugar = func(x, y int) (int, int) { return y, ancho - 1 - x }
	default:
		return origen
	}
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			dx, dy := lugar(x, y)
			copy(destino.Pix[dy*destino.Stride+dx*4:dy*destino.Stride+dx*4+4], origen.Pix[y*origen.Stride+x*4:y*origen.Stride+x*4+4])
		}
	}
	return destino
}

// orientacionEXIF lee la orientación que un JPEG declara en sus metadatos
// EXIF: un número de 1 a 8, o 1 (derecha) si no la declara o no es un JPEG.
//
// Un JPEG es una serie de segmentos, cada uno con una marca (0xFF y un byte)
// y su longitud. El segmento APP1 que empieza con «Exif» contiene un pequeño
// archivo TIFF: una lista de entradas de 12 bytes entre las que está la
// orientación (etiqueta 0x0112). Todos los accesos comprueban los límites: el
// archivo lo subió un desconocido.
func orientacionEXIF(datos []byte) int {
	const derecha = 1
	if len(datos) < 4 || datos[0] != 0xFF || datos[1] != 0xD8 {
		return derecha
	}
	for i := 2; i+4 <= len(datos); {
		if datos[i] != 0xFF {
			return derecha
		}
		marca, largo := datos[i+1], int(binary.BigEndian.Uint16(datos[i+2:i+4]))
		if marca == 0xDA || largo < 2 || i+2+largo > len(datos) { // 0xDA: empieza la imagen
			return derecha
		}
		segmento := datos[i+4 : i+2+largo]
		i += 2 + largo
		if marca != 0xE1 || len(segmento) < 14 || string(segmento[:6]) != "Exif\x00\x00" {
			continue
		}
		tiff := segmento[6:]
		var orden binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			orden = binary.LittleEndian
		case "MM":
			orden = binary.BigEndian
		default:
			return derecha
		}
		inicio := int(orden.Uint32(tiff[4:8]))
		if inicio < 8 || inicio+2 > len(tiff) {
			return derecha
		}
		entradas := int(orden.Uint16(tiff[inicio : inicio+2]))
		for n := 0; n < entradas; n++ {
			entrada := inicio + 2 + 12*n
			if entrada+12 > len(tiff) {
				return derecha
			}
			if orden.Uint16(tiff[entrada:entrada+2]) == 0x0112 {
				if valor := int(orden.Uint16(tiff[entrada+8 : entrada+10])); valor >= 1 && valor <= 8 {
					return valor
				}
				return derecha
			}
		}
		return derecha
	}
	return derecha
}
