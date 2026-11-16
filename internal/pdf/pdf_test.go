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

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fechaDePrueba es la fecha de creación de los documentos de prueba.
var fechaDePrueba = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// TextoDe devuelve los operadores de dibujo de todas las páginas de un PDF
// producido por este paquete, ya descomprimidos. Sirve para comprobar qué
// texto se escribió.
func textoDe(t *testing.T, archivo []byte) string {
	t.Helper()
	var texto strings.Builder
	for _, flujo := range regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`).FindAllSubmatch(archivo, -1) {
		lector, err := zlib.NewReader(bytes.NewReader(flujo[1]))
		if err != nil {
			t.Fatalf("flujo que no es Flate: %v", err)
		}
		contenido, err := io.ReadAll(lector)
		if err != nil {
			t.Fatal(err)
		}
		texto.Write(contenido)
	}
	return texto.String()
}

// El archivo debe ser un PDF bien formado: cabecera, objetos numerados, una
// tabla xref cuyas posiciones apunten de verdad a cada objeto, y el tráiler.
func TestEstructuraDelArchivo(t *testing.T) {
	d := Nuevo("Título (con paréntesis)", "pie", "Página %d de %d", fechaDePrueba)
	d.Encabezado("Institución", "Documento", "FOLIO-1")
	d.Seccion("Sección")
	d.Campo("Etiqueta", "Valor")
	archivo, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(archivo, []byte("%PDF-1.4\n")) || !bytes.HasSuffix(archivo, []byte("%%EOF\n")) {
		t.Fatal("faltan la cabecera o el final del archivo")
	}
	// startxref dice dónde empieza la tabla.
	final := regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`).FindSubmatch(archivo)
	if final == nil {
		t.Fatal("falta startxref")
	}
	inicioTabla, _ := strconv.Atoi(string(final[1]))
	if !bytes.HasPrefix(archivo[inicioTabla:], []byte("xref\n")) {
		t.Fatalf("startxref no apunta a la tabla: %q", archivo[inicioTabla:inicioTabla+10])
	}
	// Cada entrada de la tabla apunta al inicio de «N 0 obj».
	entradas := regexp.MustCompile(`(\d{10}) 00000 n \n`).FindAllSubmatch(archivo[inicioTabla:], -1)
	if len(entradas) != 7 { // catálogo, páginas, 2 fuentes, página, contenido, metadatos
		t.Fatalf("la tabla tiene %d objetos, se esperaban 7", len(entradas))
	}
	for i, entrada := range entradas {
		posicion, _ := strconv.Atoi(string(entrada[1]))
		if esperado := fmt.Sprintf("%d 0 obj\n", i+1); !bytes.HasPrefix(archivo[posicion:], []byte(esperado)) {
			t.Errorf("el objeto %d no está donde dice la tabla: %q", i+1, archivo[posicion:posicion+12])
		}
	}
	for _, fragmento := range []string{
		"/Type /Catalog", "/Count 1", "/BaseFont /Helvetica /Encoding /WinAnsiEncoding", "/BaseFont /Helvetica-Bold",
		"/MediaBox [0 0 612 792]", "/Filter /FlateDecode", `/Title (T\355tulo \(con par\351ntesis\))`, "/CreationDate (D:20261113120000Z)",
		"/Size 8 /Root 1 0 R /Info 7 0 R",
	} {
		// El título va en WinAnsi: se compara con sus bytes, no con UTF-8.
		esperado := strings.NewReplacer(`\355`, "\xED", `\351`, "\xE9").Replace(fragmento)
		if !bytes.Contains(archivo, []byte(esperado)) {
			t.Errorf("falta %q en el archivo", fragmento)
		}
	}
	// Sin fuentes incrustadas ni imágenes (D-16).
	for _, prohibido := range []string{"/FontFile", "/Image", "/XObject"} {
		if bytes.Contains(archivo, []byte(prohibido)) {
			t.Errorf("el archivo no debe contener %s", prohibido)
		}
	}
	texto := textoDe(t, archivo)
	for _, fragmento := range []string{"(INSTITUCI\xD3N) Tj", "(Documento) Tj", "(FOLIO-1) Tj", "(Etiqueta) Tj", "(Valor) Tj", "(pie) Tj", "(P\xE1gina 1 de 1) Tj"} {
		if !strings.Contains(texto, fragmento) {
			t.Errorf("falta %q en el contenido de la página", fragmento)
		}
	}
}

// El mismo contenido produce exactamente el mismo archivo (RF-PDF-03), y
// pedir los bytes dos veces no lo altera.
func TestEsReproducible(t *testing.T) {
	componer := func() *Documento {
		d := Nuevo("t", "pie", "%d/%d", fechaDePrueba)
		d.Encabezado("i", "t", "f")
		for i := 0; i < 80; i++ {
			d.Campo("Campo", strings.Repeat("valor ", i))
		}
		return d
	}
	d := componer()
	primero, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	segundo, _ := d.Bytes()
	otro, _ := componer().Bytes()
	if !bytes.Equal(primero, segundo) || !bytes.Equal(primero, otro) {
		t.Error("el mismo contenido debe producir el mismo archivo, byte por byte")
	}
}

// El contenido que no cabe pasa a otra página, y cada página lleva su pie.
func TestPaginacion(t *testing.T) {
	d := Nuevo("t", "FOLIO-9", "Página %d de %d", fechaDePrueba)
	d.Encabezado("i", "t", "f")
	for i := 0; i < 150; i++ {
		d.Campo("Renglón", strconv.Itoa(i))
	}
	archivo, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	paginas := bytes.Count(archivo, []byte("/Type /Page "))
	if paginas < 3 || !bytes.Contains(archivo, []byte(fmt.Sprintf("/Count %d", paginas))) {
		t.Fatalf("150 renglones deben ocupar varias páginas: %d", paginas)
	}
	texto := textoDe(t, archivo)
	for n := 1; n <= paginas; n++ {
		if !strings.Contains(texto, fmt.Sprintf("(P\xE1gina %d de %d) Tj", n, paginas)) {
			t.Errorf("falta el pie de la página %d", n)
		}
	}
	if strings.Count(texto, "(FOLIO-9) Tj") != paginas {
		t.Error("el pie debe ir en todas las páginas")
	}
	// Ningún renglón se escribe por debajo del margen inferior.
	for _, y := range regexp.MustCompile(`rg [\d.]+ ([\d.]+) Td \(\d+\) Tj`).FindAllStringSubmatch(texto, -1) {
		if altura, _ := strconv.ParseFloat(y[1], 64); altura < margenPie {
			t.Errorf("un renglón quedó en y=%v, dentro del pie", altura)
		}
	}
	// Texto, comprimido: aun con 150 campos el archivo es diminuto.
	if len(archivo) > 20<<10 {
		t.Errorf("el archivo pesa %d bytes", len(archivo))
	}
}

func TestCodificacionDelTexto(t *testing.T) {
	casos := map[string]string{
		"áéíóú ÁÉÍÓÚ ñÑ üÜ ¿? ¡!":                          "\xE1\xE9\xED\xF3\xFA \xC1\xC9\xCD\xD3\xDA \xF1\xD1 \xFC\xDC \xBF? \xA1!",
		"«comillas» “dobles” ‘simples’ — raya – guion … €": "\xABcomillas\xBB \x93dobles\x94 \x91simples\x92 \x97 raya \x96 guion \x85 \x80",
		"Dvořák, Łukasz, Győrgy, Çağlar, Šostakovič":       "Dvor\xE1k, Lukasz, Gyorgy, \xC7aglar, \x8Aostakovic",
		"ñ combinante":     "n combinante",
		"日本語 кириллица":     "??? ?????????",
		"línea\nnueva\ttab": "l\xEDnea nueva tab",
	}
	for entrada, esperado := range casos {
		if obtenido := codificar(entrada); obtenido != esperado {
			t.Errorf("codificar(%q) = %q, se esperaba %q", entrada, obtenido, esperado)
		}
	}
	if escapar(`a(b)c\d`) != `a\(b\)c\\d` {
		t.Errorf("escape incorrecto: %s", escapar(`a(b)c\d`))
	}
	for valor, esperado := range map[float64]string{0: "0", 1: "1", 54: "54", 9.5: "9.5", 0.126: "0.13", 12.80: "12.8", -0.001: "0", 0.451: "0.45"} {
		if obtenido := num(valor); obtenido != esperado {
			t.Errorf("num(%v) = %s, se esperaba %s", valor, obtenido, esperado)
		}
	}
}

func TestMedidaYAjusteDeRenglones(t *testing.T) {
	// Anchos conocidos de las métricas de Helvetica, en milésimas.
	for texto, milesimas := range map[string]float64{"": 0, " ": 278, "A": 667, "i": 222, "W": 944, "Á": 667, "ñ": 556, "í": 278, "FIM49": 611 + 278 + 833 + 556 + 556} {
		if obtenido := Ancho(texto, false, 1000); obtenido != milesimas {
			t.Errorf("ancho de %q: %v, se esperaba %v", texto, obtenido, milesimas)
		}
	}
	if Ancho("A", true, 1000) != 722 || Ancho("Hola", true, 10) <= Ancho("Hola", false, 10) {
		t.Error("la negrita debe medir más")
	}
	if len(parecidoA) != 128 || len(sinDiacritico) != 128 {
		t.Fatalf("las tablas de equivalencias deben cubrir 128 códigos: %d y %d", len(parecidoA), len(sinDiacritico))
	}

	largo := strings.Repeat("palabra ", 60)
	renglones := Envolver(largo, false, 10, 200)
	if len(renglones) < 5 {
		t.Fatalf("un texto largo debe partirse: %d renglones", len(renglones))
	}
	for _, renglon := range renglones {
		if Ancho(renglon, false, 10) > 200 {
			t.Errorf("renglón más ancho que el límite: %q", renglon)
		}
	}
	if strings.Join(renglones, " ") != strings.TrimSpace(largo) {
		t.Error("al partir no debe perderse ni duplicarse texto")
	}
	// Una palabra más larga que el renglón (una liga, una suma) se parte.
	suma := strings.Repeat("0123456789abcdef", 8)
	partes := Envolver(suma, false, 10, 150)
	if len(partes) < 3 || strings.Join(partes, "") != suma {
		t.Errorf("una palabra larga debe partirse sin perder caracteres: %v", partes)
	}
	// Los saltos de línea se respetan; los renglones en blanco finales, no.
	if obtenido := Envolver("uno\n\ndos\r\ntres\n\n", false, 10, 500); fmt.Sprint(obtenido) != "[uno  dos tres]" || len(obtenido) != 4 {
		t.Errorf("saltos de línea: %q", obtenido)
	}
	if obtenido := Envolver("", false, 10, 100); len(obtenido) != 1 || obtenido[0] != "" {
		t.Errorf("un texto vacío es un renglón vacío: %q", obtenido)
	}
}

// Lo que el expediente añade al escritor: imágenes, marcadores, un pie por
// página y páginas que pasan al inicio. La tabla xref debe seguir apuntando
// a cada objeto, también con el contenido binario de una imagen en medio.
func TestImagenesMarcadoresEIndice(t *testing.T) {
	// Un «JPEG» de mentira: al escritor no le importa su contenido, lo
	// incrusta tal cual. Lleva a propósito la palabra «endobj».
	jpeg := []byte("\xFF\xD8\xFF bytes de la imagen, endobj incluido \xFF\xD9")
	d := Nuevo("Expediente", "pie general", "Página %d de %d", fechaDePrueba)
	for i, folio := range []string{"FOLIO-1", "FOLIO-2"} {
		if i > 0 {
			d.SaltoDePagina()
		}
		d.FijarPie("pie de " + folio)
		d.Marcador(folio + " (Núñez)")
		d.Encabezado("Institución", "Expediente", folio)
		d.Imagen(jpeg, 400, 300, 150, 150)
		d.Campo("Etiqueta", "Contenido de "+folio)
	}
	if d.Paginas() != 2 {
		t.Fatalf("páginas del cuerpo: %d", d.Paginas())
	}
	// El índice se escribe al final y pasa al frente.
	d.SaltoDePagina()
	d.FijarPie("pie del índice")
	d.Renglon("FOLIO-1", "2")
	d.Renglon("FOLIO-2 con un título tan largo que no cabe en el renglón "+strings.Repeat("y sigue ", 20)+"hasta el FINAL", "3")
	d.MoverAlInicio(2)
	archivo, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// La tabla sigue siendo exacta.
	final := regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`).FindSubmatch(archivo)
	inicioTabla, _ := strconv.Atoi(string(final[1]))
	entradas := regexp.MustCompile(`(\d{10}) 00000 n \n`).FindAllSubmatch(archivo[inicioTabla:], -1)
	// catálogo, páginas, 2 fuentes, 3 páginas con su contenido, 2 imágenes,
	// raíz de marcadores, 2 marcadores y metadatos.
	if len(entradas) != 4+6+2+3+1 {
		t.Fatalf("la tabla tiene %d objetos", len(entradas))
	}
	for i, entrada := range entradas {
		posicion, _ := strconv.Atoi(string(entrada[1]))
		if esperado := fmt.Sprintf("%d 0 obj\n", i+1); !bytes.HasPrefix(archivo[posicion:], []byte(esperado)) {
			t.Errorf("el objeto %d no está donde dice la tabla", i+1)
		}
	}
	for _, fragmento := range []string{
		"/Type /Catalog /Pages 2 0 R /Outlines 13 0 R /PageMode /UseOutlines", "/Count 3 /Kids [5 0 R 7 0 R 9 0 R]",
		"/Subtype /Image /Width 400 /Height 300 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length " + strconv.Itoa(len(jpeg)),
		"/Type /Outlines /First 14 0 R /Last 15 0 R /Count 2",
		// Los marcadores siguen a sus páginas: el índice ocupa la primera
		// (objeto 5), de modo que apuntan a la segunda y a la tercera.
		"/Title (FOLIO-1 \\(N\xFA\xF1ez\\)) /Parent 13 0 R /Next 15 0 R /Dest [7 0 R /XYZ 0 792 null]",
		"/Title (FOLIO-2 \\(N\xFA\xF1ez\\)) /Parent 13 0 R /Prev 14 0 R /Dest [9 0 R /XYZ 0 792 null]",
		// Cada página declara sólo la imagen que dibuja; el índice, ninguna.
		"/Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents 6 0 R",
		"/XObject << /Im0 11 0 R >> >> /Contents 8 0 R", "/XObject << /Im1 12 0 R >> >> /Contents 10 0 R",
		string(jpeg),
	} {
		if !bytes.Contains(archivo, []byte(fragmento)) {
			t.Errorf("falta %q en el archivo", fragmento)
		}
	}

	// El contenido de cada página, en su nuevo orden, con su propio pie.
	var paginas []string
	for _, flujo := range regexp.MustCompile(`(?s)/FlateDecode >>\nstream\n(.*?)\nendstream`).FindAllSubmatch(archivo, -1) {
		lector, err := zlib.NewReader(bytes.NewReader(flujo[1]))
		if err != nil {
			t.Fatal(err)
		}
		contenido, err := io.ReadAll(lector)
		if err != nil {
			t.Fatal(err)
		}
		paginas = append(paginas, string(contenido))
	}
	if len(paginas) != 3 {
		t.Fatalf("páginas: %d", len(paginas))
	}
	for i, esperados := range [][]string{
		{"(FOLIO-1) Tj", "(2) Tj", "(3) Tj", "(pie del \xEDndice) Tj", "(P\xE1gina 1 de 3) Tj"},
		{"(Contenido de FOLIO-1) Tj", "(pie de FOLIO-1) Tj", "(P\xE1gina 2 de 3) Tj", "q 150 0 0 112.5 54 ", "/Im0 Do Q"},
		{"(Contenido de FOLIO-2) Tj", "(pie de FOLIO-2) Tj", "(P\xE1gina 3 de 3) Tj", "/Im1 Do Q"},
	} {
		for _, esperado := range esperados {
			if !strings.Contains(paginas[i], esperado) {
				t.Errorf("página %d: falta %q", i+1, esperado)
			}
		}
	}
	if strings.Contains(paginas[0], "FINAL") || !strings.Contains(paginas[0], "(FOLIO-2 con un t\xEDtulo") || strings.Contains(paginas[0], "Do Q") {
		t.Error("el renglón largo del índice no se recortó, o el índice dibuja una imagen")
	}
	// Mover lo que no existe, o nada, no altera el documento.
	antes := d.Paginas()
	d.MoverAlInicio(0)
	d.MoverAlInicio(99)
	if d.Paginas() != antes {
		t.Error("MoverAlInicio con un índice fuera de rango alteró el documento")
	}
	// Sin marcadores, el archivo no trae panel de navegación.
	d.QuitarMarcadores()
	if archivo, err = d.Bytes(); err != nil || bytes.Contains(archivo, []byte("/Outlines")) {
		t.Errorf("QuitarMarcadores: %v", err)
	}
}
