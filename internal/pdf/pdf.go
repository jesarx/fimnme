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

// Package pdf es un escritor mínimo de documentos PDF, hecho con la
// biblioteca estándar (RF-PDF-03, D-16).
//
// Nació para componer el acuse de recibo, que debe pesar menos de 100 KB y
// generarse siempre igual. Por eso hace muy poco, a propósito:
//
//   - Sólo usa las fuentes base del formato (Helvetica y Helvetica-Bold), que
//     todo visor trae consigo: no se incrusta ninguna fuente.
//   - La identidad visual se dibuja con rectángulos y líneas.
//   - Comprime el contenido de cada página con Flate.
//
// El expediente (§5.12.4) necesita un poco más, y sólo eso se añadió:
// imágenes JPEG (la fotografía), marcadores de navegación, salto de página,
// un pie distinto por página y mover páginas al inicio (para el índice). Un
// documento que no usa nada de eso —el acuse— sale byte por byte como antes.
//
// El texto se escribe en WinAnsiEncoding (Windows-1252), que cubre el español
// y el inglés completos. Las letras latinas que no están en esa codificación
// («ř», «ő», «ł») se escriben sin su diacrítico, y lo que no es alfabeto
// latino se sustituye por «?». El snapshot de la postulación conserva siempre
// el texto exacto; el PDF es su representación legible.
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"time"
)

// Medidas de la página, en puntos (1/72 de pulgada). Tamaño carta, el habitual
// en México.
const (
	anchoPagina   = 612.0
	altoPagina    = 792.0
	margen        = 54.0 // 1.9 cm
	margenPie     = 60.0 // espacio reservado al pie de página
	anchoUtil     = anchoPagina - 2*margen
	anchoEtiqueta = 150.0 // columna de etiquetas de los campos

	cuerpoTexto    = 9.5
	cuerpoSeccion  = 11.0
	cuerpoTitulo   = 20.0
	cuerpoPie      = 8.0
	interlinea     = 1.35 // respecto del cuerpo de la letra
	altoEncabezado = 96.0
)

// Color es un color RGB con componentes de 0 a 1.
type Color struct{ R, G, B float64 }

// Colores del documento. El acento es el verde de la identidad del Foro.
var (
	Tinta      = Color{0.09, 0.17, 0.125}
	TintaSuave = Color{0.33, 0.42, 0.36}
	Acento     = Color{0.165, 0.451, 0.314}
	Filete     = Color{0.725, 0.804, 0.749}
	Blanco     = Color{1, 1, 1}
)

// Documento compone un PDF página por página. Se usa llamando a sus métodos
// de arriba abajo —Encabezado, Seccion, Campo, Parrafo— y al final a Bytes.
// Cuando el contenido no cabe, pasa solo a la página siguiente.
type Documento struct {
	titulo        string          // título en los metadatos del archivo
	fecha         time.Time       // fecha de creación en los metadatos
	pie           string          // texto del pie, a la izquierda (p. ej. el folio)
	formatoPagina string          // «Página %d de %d», ya traducido
	paginas       []*bytes.Buffer // operadores de dibujo de cada página, sin comprimir
	y             float64         // posición vertical del siguiente renglón

	// Lo que sigue sólo lo usa el expediente.
	pies       []string   // pie de cada página, en paralelo a paginas
	imagenes   []imagen   // imágenes incrustadas, en el orden en que se añadieron
	usadas     [][]int    // por página, índices de las imágenes que dibuja
	marcadores []marcador // entradas del panel de navegación
}

// imagen es una fotografía JPEG incrustada tal cual (filtro DCTDecode).
type imagen struct {
	jpeg        []byte
	ancho, alto int // en pixeles
}

// marcador es una entrada del panel de navegación del visor: un título que
// lleva al inicio de una página.
type marcador struct {
	titulo string
	pagina int // índice en paginas
}

// Nuevo crea un documento vacío. fecha queda en los metadatos: para que el
// mismo contenido produzca siempre el mismo archivo debe ser un dato del
// documento (la fecha de envío), no la hora en que se genera.
func Nuevo(titulo, pie, formatoPagina string, fecha time.Time) *Documento {
	d := &Documento{titulo: titulo, fecha: fecha, pie: pie, formatoPagina: formatoPagina}
	d.nuevaPagina()
	return d
}

// nuevaPagina abre una página en blanco y coloca el cursor arriba.
func (d *Documento) nuevaPagina() {
	d.paginas = append(d.paginas, &bytes.Buffer{})
	d.pies = append(d.pies, d.pie)
	d.usadas = append(d.usadas, nil)
	d.y = altoPagina - margen
}

// SaltoDePagina termina la página en curso y abre otra.
func (d *Documento) SaltoDePagina() {
	d.nuevaPagina()
}

// FijarPie cambia el texto del pie para la página en curso y las siguientes.
// En un expediente de muchas postulaciones, cada página lleva el folio de la
// suya.
func (d *Documento) FijarPie(pie string) {
	d.pie = pie
	d.pies[len(d.pies)-1] = pie
}

// Paginas devuelve cuántas páginas lleva el documento.
func (d *Documento) Paginas() int {
	return len(d.paginas)
}

// Marcador añade al panel de navegación una entrada que lleva a la página en
// curso.
func (d *Documento) Marcador(titulo string) {
	d.marcadores = append(d.marcadores, marcador{titulo: titulo, pagina: len(d.paginas) - 1})
}

// QuitarMarcadores deja el documento sin panel de navegación. En el
// expediente de una sola postulación un único marcador sólo estorba.
func (d *Documento) QuitarMarcadores() {
	d.marcadores = nil
}

// MoverAlInicio pasa al principio del documento las páginas escritas a partir
// de la número desde (contando desde 0). Sirve para el índice: sólo puede
// escribirse cuando ya se sabe en qué página quedó cada cosa, pero va al
// frente. Los marcadores siguen apuntando a sus páginas.
func (d *Documento) MoverAlInicio(desde int) {
	if desde <= 0 || desde >= len(d.paginas) {
		return
	}
	movidas := len(d.paginas) - desde
	d.paginas = append(append([]*bytes.Buffer{}, d.paginas[desde:]...), d.paginas[:desde]...)
	d.pies = append(append([]string{}, d.pies[desde:]...), d.pies[:desde]...)
	d.usadas = append(append([][]int{}, d.usadas[desde:]...), d.usadas[:desde]...)
	for i := range d.marcadores {
		if d.marcadores[i].pagina >= desde {
			d.marcadores[i].pagina -= desde
		} else {
			d.marcadores[i].pagina += movidas
		}
	}
}

// Imagen dibuja una fotografía JPEG en el margen izquierdo, a continuación de
// lo ya escrito, ajustada para caber en anchoMax × altoMax puntos sin
// deformarse. ancho y alto son sus dimensiones en pixeles. El JPEG se
// incrusta tal cual: quien llama lo entrega ya reducido, en RGB.
func (d *Documento) Imagen(jpeg []byte, ancho, alto int, anchoMax, altoMax float64) {
	if len(jpeg) == 0 || ancho <= 0 || alto <= 0 {
		return
	}
	escala := min(anchoMax/float64(ancho), altoMax/float64(alto))
	anchoPuntos, altoPuntos := float64(ancho)*escala, float64(alto)*escala
	d.reservar(altoPuntos + 6)
	d.imagenes = append(d.imagenes, imagen{jpeg: jpeg, ancho: ancho, alto: alto})
	indice := len(d.imagenes) - 1
	d.usadas[len(d.usadas)-1] = append(d.usadas[len(d.usadas)-1], indice)
	// «cm» lleva el cuadrado unitario de la imagen a su lugar y tamaño.
	fmt.Fprintf(d.pagina(), "q %s 0 0 %s %s %s cm /Im%d Do Q\n",
		num(anchoPuntos), num(altoPuntos), num(margen), num(d.y-altoPuntos+cuerpoTexto), indice)
	d.y -= altoPuntos + 6
}

// Renglon escribe un renglón con un texto a la izquierda y otro alineado a la
// derecha: una entrada del índice con su número de página. El texto de la
// izquierda se recorta si no cabe.
func (d *Documento) Renglon(izquierda, derecha string) {
	paso := cuerpoTexto * interlinea
	d.reservar(paso)
	anchoDerecha := Ancho(derecha, false, cuerpoTexto)
	if renglones := Envolver(izquierda, false, cuerpoTexto, anchoUtil-anchoDerecha-12); len(renglones) > 0 {
		d.texto(margen, d.y, false, cuerpoTexto, Tinta, renglones[0])
	}
	d.texto(anchoPagina-margen-anchoDerecha, d.y, false, cuerpoTexto, TintaSuave, derecha)
	d.y -= paso + 1
}

// pagina devuelve la página en la que se está escribiendo.
func (d *Documento) pagina() *bytes.Buffer {
	return d.paginas[len(d.paginas)-1]
}

// reservar garantiza que quepan alto puntos en la página; si no, abre otra.
func (d *Documento) reservar(alto float64) {
	if d.y-alto < margenPie {
		d.nuevaPagina()
	}
}

// operadorTexto devuelve los operadores PDF que escriben un renglón. x, y es
// el inicio de la línea base del texto.
func operadorTexto(x, y float64, negrita bool, cuerpo float64, color Color, renglon string) string {
	fuente := "F1"
	if negrita {
		fuente = "F2"
	}
	return fmt.Sprintf("BT /%s %s Tf %s %s %s rg %s %s Td (%s) Tj ET\n",
		fuente, num(cuerpo), num(color.R), num(color.G), num(color.B), num(x), num(y), escapar(codificar(renglon)))
}

// operadorLinea devuelve los operadores PDF de un filete horizontal de lado a
// lado del área útil.
func operadorLinea(y float64, color Color) string {
	return fmt.Sprintf("%s %s %s RG 0.5 w %s %s m %s %s l S\n",
		num(color.R), num(color.G), num(color.B), num(margen), num(y), num(anchoPagina-margen), num(y))
}

// texto escribe un renglón en la página en curso.
func (d *Documento) texto(x, y float64, negrita bool, cuerpo float64, color Color, renglon string) {
	d.pagina().WriteString(operadorTexto(x, y, negrita, cuerpo, color, renglon))
}

// rectangulo dibuja un rectángulo relleno en la página en curso.
func (d *Documento) rectangulo(x, y, ancho, alto float64, color Color) {
	fmt.Fprintf(d.pagina(), "%s %s %s rg %s %s %s %s re f\n",
		num(color.R), num(color.G), num(color.B), num(x), num(y), num(ancho), num(alto))
}

// linea dibuja un filete horizontal en la página en curso.
func (d *Documento) linea(y float64, color Color) {
	d.pagina().WriteString(operadorLinea(y, color))
}

// Encabezado dibuja la franja superior de la primera página: una banda del
// color de acento con la institución, el título del documento y, a la
// derecha, un dato destacado (el folio).
func (d *Documento) Encabezado(institucion, titulo, destacado string) {
	d.rectangulo(0, altoPagina-altoEncabezado, anchoPagina, altoEncabezado, Acento)
	d.texto(margen, altoPagina-34, true, 8, Blanco, strings.ToUpper(institucion))
	d.texto(margen, altoPagina-66, true, cuerpoTitulo, Blanco, titulo)
	anchoDestacado := Ancho(destacado, true, 14)
	d.texto(anchoPagina-margen-anchoDestacado, altoPagina-66, true, 14, Blanco, destacado)
	d.y = altoPagina - altoEncabezado - 26
}

// Seccion escribe el título de una sección, con un filete debajo. Se asegura
// de que no quede solo al final de una página.
func (d *Documento) Seccion(titulo string) {
	d.reservar(cuerpoSeccion*interlinea + 4*cuerpoTexto*interlinea)
	d.y -= 10
	d.texto(margen, d.y, true, cuerpoSeccion, Acento, titulo)
	d.y -= 5
	d.linea(d.y, Filete)
	d.y -= cuerpoTexto*interlinea + 2
}

// Subtitulo escribe un rótulo en negrita dentro de una sección (p. ej. el
// título de cada obra).
func (d *Documento) Subtitulo(texto string) {
	d.reservar(3 * cuerpoTexto * interlinea)
	d.y -= 4
	for _, renglon := range Envolver(texto, true, cuerpoTexto+0.5, anchoUtil) {
		d.texto(margen, d.y, true, cuerpoTexto+0.5, Tinta, renglon)
		d.y -= (cuerpoTexto + 0.5) * interlinea
	}
}

// Campo escribe un dato: la etiqueta a la izquierda y el valor a la derecha,
// en tantos renglones como necesite. Un valor vacío se escribe como raya.
func (d *Documento) Campo(etiqueta, valor string) {
	if strings.TrimSpace(valor) == "" {
		valor = "—"
	}
	etiquetas := Envolver(etiqueta, false, cuerpoTexto, anchoEtiqueta-8)
	valores := Envolver(valor, false, cuerpoTexto, anchoUtil-anchoEtiqueta)
	paso := cuerpoTexto * interlinea
	for i := 0; i < max(len(etiquetas), len(valores)); i++ {
		d.reservar(paso)
		if i < len(etiquetas) {
			d.texto(margen, d.y, false, cuerpoTexto, TintaSuave, etiquetas[i])
		}
		if i < len(valores) {
			d.texto(margen+anchoEtiqueta, d.y, false, cuerpoTexto, Tinta, valores[i])
		}
		d.y -= paso
	}
	d.y -= 2
}

// Parrafo escribe un texto corrido a todo el ancho.
func (d *Documento) Parrafo(texto string, color Color) {
	paso := cuerpoTexto * interlinea
	for _, renglon := range Envolver(texto, false, cuerpoTexto, anchoUtil) {
		d.reservar(paso)
		d.texto(margen, d.y, false, cuerpoTexto, color, renglon)
		d.y -= paso
	}
	d.y -= 4
}

// Espacio deja un espacio vertical.
func (d *Documento) Espacio(puntos float64) {
	d.y -= puntos
}

// Bytes termina el documento y devuelve el archivo PDF completo.
//
// Un PDF es una lista de objetos numerados, seguida de una tabla que dice en
// qué byte empieza cada uno (xref) y de un tráiler que señala el objeto raíz.
// Aquí los objetos son: 1 catálogo, 2 árbol de páginas, 3 y 4 las dos
// fuentes, y por cada página dos más (la página y su contenido); después las
// imágenes y los marcadores, si los hay; al final, los metadatos.
func (d *Documento) Bytes() ([]byte, error) {
	var salida bytes.Buffer
	var posiciones []int // byte en que empieza cada objeto, en orden
	objeto := func(cuerpo string) {
		posiciones = append(posiciones, salida.Len())
		fmt.Fprintf(&salida, "%d 0 obj\n%s\nendobj\n", len(posiciones), cuerpo)
	}

	// Cabecera. La segunda línea, con bytes altos, indica a los programas de
	// transferencia que el archivo es binario.
	salida.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	total := len(d.paginas)
	primeraPagina := 5 // los objetos 1 a 4 son catálogo, páginas y fuentes
	var hijos []string
	for i := range d.paginas {
		hijos = append(hijos, fmt.Sprintf("%d 0 R", primeraPagina+2*i))
	}
	// Los números de objeto se conocen de antemano: tras las páginas van las
	// imágenes, y tras ellas la raíz de los marcadores y cada marcador.
	primeraImagen := primeraPagina + 2*total
	raizMarcadores := primeraImagen + len(d.imagenes)
	if len(d.marcadores) == 0 {
		objeto("<< /Type /Catalog /Pages 2 0 R >>")
	} else {
		objeto(fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /Outlines %d 0 R /PageMode /UseOutlines >>", raizMarcadores))
	}
	objeto(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", total, strings.Join(hijos, " ")))
	objeto("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	objeto("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")

	for i, pagina := range d.paginas {
		// El pie se añade aquí, cuando ya se sabe cuántas páginas hay. Se
		// escribe sobre una copia: llamar a Bytes dos veces da lo mismo.
		contenido := bytes.NewBuffer(bytes.Clone(pagina.Bytes()))
		numeracion := fmt.Sprintf(d.formatoPagina, i+1, total)
		contenido.WriteString(operadorLinea(margenPie-14, Filete))
		contenido.WriteString(operadorTexto(margen, margenPie-26, false, cuerpoPie, TintaSuave, d.pies[i]))
		contenido.WriteString(operadorTexto(anchoPagina-margen-Ancho(numeracion, false, cuerpoPie), margenPie-26,
			false, cuerpoPie, TintaSuave, numeracion))

		var comprimido bytes.Buffer
		compresor := zlib.NewWriter(&comprimido)
		if _, err := compresor.Write(contenido.Bytes()); err != nil {
			return nil, fmt.Errorf("comprimir página %d: %w", i+1, err)
		}
		if err := compresor.Close(); err != nil {
			return nil, fmt.Errorf("comprimir página %d: %w", i+1, err)
		}
		// Una página sólo declara las imágenes que dibuja.
		recursos := "/Font << /F1 3 0 R /F2 4 0 R >>"
		if len(d.usadas[i]) > 0 {
			var imagenes strings.Builder
			for _, indice := range d.usadas[i] {
				fmt.Fprintf(&imagenes, " /Im%d %d 0 R", indice, primeraImagen+indice)
			}
			recursos += " /XObject <<" + imagenes.String() + " >>"
		}
		objeto(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] "+
			"/Resources << %s >> /Contents %d 0 R >>",
			num(anchoPagina), num(altoPagina), recursos, primeraPagina+2*i+1))
		objeto(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", comprimido.Len(), comprimido.String()))
	}

	for _, img := range d.imagenes {
		objeto(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB "+
			"/BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
			img.ancho, img.alto, len(img.jpeg), img.jpeg))
	}

	// Marcadores: una lista de un solo nivel, encadenada con /Prev y /Next.
	if cantidad := len(d.marcadores); cantidad > 0 {
		objeto(fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
			raizMarcadores+1, raizMarcadores+cantidad, cantidad))
		for i, m := range d.marcadores {
			enlaces := ""
			if i > 0 {
				enlaces += fmt.Sprintf(" /Prev %d 0 R", raizMarcadores+i)
			}
			if i < cantidad-1 {
				enlaces += fmt.Sprintf(" /Next %d 0 R", raizMarcadores+i+2)
			}
			objeto(fmt.Sprintf("<< /Title (%s) /Parent %d 0 R%s /Dest [%d 0 R /XYZ 0 %s null] >>",
				escapar(codificar(m.titulo)), raizMarcadores, enlaces, primeraPagina+2*m.pagina, num(altoPagina)))
		}
	}

	objeto(fmt.Sprintf("<< /Title (%s) /Producer (FIMNME) /CreationDate (D:%sZ) >>",
		escapar(codificar(d.titulo)), d.fecha.UTC().Format("20060102150405")))

	// Tabla de referencias cruzadas: cada entrada mide exactamente 20 bytes.
	inicioTabla := salida.Len()
	fmt.Fprintf(&salida, "xref\n0 %d\n0000000000 65535 f \n", len(posiciones)+1)
	for _, posicion := range posiciones {
		fmt.Fprintf(&salida, "%010d 00000 n \n", posicion)
	}
	fmt.Fprintf(&salida, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(posiciones)+1, len(posiciones), inicioTabla)
	return salida.Bytes(), nil
}

// num escribe un número con dos decimales como máximo y sin ceros de sobra,
// para no engordar el archivo.
func num(valor float64) string {
	texto := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", valor), "0"), ".")
	if texto == "" || texto == "-" || texto == "-0" {
		return "0"
	}
	return texto
}

// escapar protege los tres caracteres con significado dentro de una cadena
// PDF: los paréntesis, que la delimitan, y la barra invertida.
func escapar(texto string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(texto)
}
