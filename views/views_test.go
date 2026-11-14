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
	"compress/zlib"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"errors"
	"fmt"
	"github.com/jesarx/fimnme/models"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

// motorDePrueba carga el frontal incrustado; si una plantilla o un JSON de
// traducción está roto, todas las pruebas de este paquete fallan aquí.
func motorDePrueba(t *testing.T) *Motor {
	t.Helper()
	m, err := Nuevo()
	if err != nil {
		t.Fatalf("no se pudo cargar el frontal: %v", err)
	}
	return m
}

// Los dos idiomas son de primera clase (RF-I18N-01): deben tener exactamente
// las mismas claves, y ninguna vacía.
func TestTraduccionesConLasMismasClaves(t *testing.T) {
	m := motorDePrueba(t)
	es, en := m.textos.Claves(models.IdiomaES), m.textos.Claves(models.IdiomaEN)
	for clave, texto := range es {
		if _, ok := en[clave]; !ok {
			t.Errorf("falta en en.json: %s", clave)
		}
		if strings.TrimSpace(texto) == "" {
			t.Errorf("texto vacío en es.json: %s", clave)
		}
	}
	for clave, texto := range en {
		if _, ok := es[clave]; !ok {
			t.Errorf("falta en es.json: %s", clave)
		}
		if strings.TrimSpace(texto) == "" {
			t.Errorf("texto vacío en en.json: %s", clave)
		}
	}
}

// Toda clave de traducción escrita literalmente en una plantilla debe existir.
// Evita que una errata muestre "inicio.tiulo" en pantalla.
func TestClavesDeLasPlantillasExisten(t *testing.T) {
	m := motorDePrueba(t)
	es := m.textos.Claves(models.IdiomaES)
	// Llamadas {{.T "clave"}} / {{$.T "clave"}} y argumentos de los parciales.
	patrones := []*regexp.Regexp{
		regexp.MustCompile(`\.T "([^"]+)"`),
		regexp.MustCompile(`"(?:Etiqueta|Ayuda)" "([^"]+)"`),
	}
	err := fs.WalkDir(contenido, "templates", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		datos, err := fs.ReadFile(contenido, ruta)
		if err != nil {
			return err
		}
		for _, patron := range patrones {
			for _, m := range patron.FindAllStringSubmatch(string(datos), -1) {
				if _, ok := es[m[1]]; !ok {
					t.Errorf("%s usa una clave inexistente: %s", ruta, m[1])
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Las claves que el dominio y los controladores componen en tiempo de
// ejecución (estados, etapas, meses, errores) también deben existir.
func TestClavesDinamicasExisten(t *testing.T) {
	m := motorDePrueba(t)
	es := m.textos.Claves(models.IdiomaES)
	var claves []string
	for _, estado := range []models.EstadoConvocatoria{
		models.EstadoPublicada, models.EstadoPostulacion, models.EstadoRevisionDocumental,
		models.EstadoEvaluacion, models.EstadoCerrada, models.EstadoArchivada,
	} {
		claves = append(claves, "estado.convocatoria."+string(estado))
	}
	for _, etapa := range []models.ClaveEtapa{
		models.EtapaPostulacion, models.EtapaRevisionDocumental, models.EtapaEvaluacion,
	} {
		claves = append(claves, "etapa."+string(etapa))
	}
	// Se provocan todos los errores de validación para recoger sus claves.
	for _, e := range models.ValidarRegistro(models.DatosRegistro{}) {
		claves = append(claves, e.Clave)
	}
	for _, d := range []models.DatosRegistro{
		{Email: "x", Nombre: strings.Repeat("a", 200), Apellidos: strings.Repeat("a", 200), Password: "corta"},
		{Password: strings.Repeat("a", 200)},
	} {
		for _, e := range models.ValidarRegistro(d) {
			claves = append(claves, e.Clave)
		}
	}
	for _, codigo := range []string{"404", "403", "500"} {
		claves = append(claves, "http."+codigo+"_titulo", "http."+codigo+"_texto")
	}
	sort.Strings(claves)
	for _, clave := range claves {
		if _, ok := es[clave]; !ok {
			t.Errorf("clave dinámica sin traducción: %s", clave)
		}
	}
}

// Las fechas se guardan en UTC y se muestran en hora de la Ciudad de México,
// con la zona explícita (RF-I18N-06, RNF-15).
func TestFechasEnHoraDeMexico(t *testing.T) {
	m := motorDePrueba(t)
	// 30 de enero 05:59 UTC = 29 de enero 23:59 en la Ciudad de México (UTC−6).
	cierre := time.Date(2027, 1, 30, 5, 59, 0, 0, time.UTC)
	casos := map[string][2]string{
		models.IdiomaES: {"29 de enero de 2027", "29 de enero de 2027, 23:59 h (hora de la Ciudad de México)"},
		models.IdiomaEN: {"January 29, 2027", "January 29, 2027, 11:59 PM (Mexico City time)"},
	}
	for idioma, esperado := range casos {
		if got := m.textos.Fecha(idioma, cierre); got != esperado[0] {
			t.Errorf("Fecha(%s) = %q, se esperaba %q", idioma, got, esperado[0])
		}
		if got := m.textos.FechaHora(idioma, cierre); got != esperado[1] {
			t.Errorf("FechaHora(%s) = %q, se esperaba %q", idioma, got, esperado[1])
		}
	}
}

// Una clave inexistente se devuelve tal cual: hueco visible, no página rota.
func TestClaveInexistenteNoRompe(t *testing.T) {
	m := motorDePrueba(t)
	if got := m.T(models.IdiomaEN, "no.existe"); got != "no.existe" {
		t.Fatalf("se esperaba la propia clave, se obtuvo %q", got)
	}
}

func TestMarkdownSubconjunto(t *testing.T) {
	html := string(Markdown("# Título\n\nUn **párrafo** con *énfasis* y [liga](https://ejemplo.mx/a?b=1&c=2).\n\n- uno\n- dos\n\n1. primero\n2. segundo\n"))
	for _, fragmento := range []string{
		"<h2>Título</h2>",
		"<strong>párrafo</strong>",
		"<em>énfasis</em>",
		`<a href="https://ejemplo.mx/a?b=1&amp;c=2" rel="noopener noreferrer">liga</a>`,
		"<ul>\n<li>uno</li>\n<li>dos</li>\n</ul>",
		"<ol>\n<li>primero</li>\n<li>segundo</li>\n</ol>",
	} {
		if !strings.Contains(html, fragmento) {
			t.Errorf("falta %q en:\n%s", fragmento, html)
		}
	}
}

// Las bases las escribe un administrador, pero aun así el intérprete no debe
// dejar pasar HTML ni enlaces con esquemas peligrosos.
func TestMarkdownNoDejaPasarHTMLNiEsquemasPeligrosos(t *testing.T) {
	html := string(Markdown(`<script>alert(1)</script> [x](javascript:alert(1)) [y](https://a.mx/"onmouseover="z) <img src=x onerror=y>`))
	// No debe sobrevivir ninguna etiqueta salvo las que el intérprete genera,
	// ni un enlace con esquema javascript:, ni una comilla capaz de cerrar el
	// atributo href e inyectar otro.
	for _, prohibido := range []string{"<script", "<img", `href="javascript:`, `"onmouseover=`} {
		if strings.Contains(html, prohibido) {
			t.Errorf("el resultado contiene %q:\n%s", prohibido, html)
		}
	}
}

// Los estáticos se sirven con caché larga sólo si la URL lleva la huella, y
// nunca se listan directorios.
func TestEstaticos(t *testing.T) {
	m := motorDePrueba(t)
	servidor := m.Estaticos()

	pedir := func(url string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		servidor.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		return w
	}

	w := pedir(m.URLEstatico("css/fimnme.css"))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("CSS con huella: estado %d, Cache-Control %q", w.Code, w.Header().Get("Cache-Control"))
	}
	if w := pedir("/static/css/fimnme.css"); w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("CSS sin huella debe ir con no-cache, se obtuvo %q", w.Header().Get("Cache-Control"))
	}
	if w := pedir("/static/css/"); w.Code != http.StatusNotFound {
		t.Errorf("no deben listarse directorios: estado %d", w.Code)
	}
	if w := pedir("/static/js/htmx.min.js"); w.Code != http.StatusOK {
		t.Errorf("htmx debe servirse desde el binario: estado %d", w.Code)
	}
}

// La CSP prohíbe código y estilos en línea: ninguna plantilla puede traerlos.
func TestPlantillasSinCodigoEnLinea(t *testing.T) {
	comentarios := regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`)
	prohibidos := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script(\s[^>]*)?>\s*[^<\s]`), // <script> con cuerpo
		regexp.MustCompile(`(?i)\sstyle\s*=`),                 // style=""
		regexp.MustCompile(`(?i)<style`),                      // <style>
		regexp.MustCompile(`(?i)\son[a-z]+\s*=`),              // onclick="" y similares
	}
	err := fs.WalkDir(contenido, "templates", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		datos, err := fs.ReadFile(contenido, ruta)
		if err != nil {
			return err
		}
		// Los comentarios de plantilla no llegan al navegador: se descartan
		// para que mencionar "style" en uno de ellos no sea un falso positivo.
		datos = comentarios.ReplaceAll(datos, nil)
		for _, patron := range prohibidos {
			if hallado := patron.Find(datos); hallado != nil {
				t.Errorf("%s contiene código o estilo en línea: %q", ruta, hallado)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Los valores de las variables de un correo son texto literal: no se
// interpretan como Markdown ni como HTML.
func TestRenderCorreo(t *testing.T) {
	plantilla := models.PlantillaCorreo{
		Clave: models.PlantillaVerificarEmail, Idioma: models.IdiomaES,
		Asunto:   "Hola, {{nombre}}",
		CuerpoMD: "Hola, **{{nombre}}**:\n\nAbre este enlace:\n\n{{liga}}\n\n{{desconocida}}",
	}
	liga := "https://postula.ejemplo.mx/cuenta/verificar?token=abc&x=1"
	c := RenderCorreo(plantilla, "ana@ejemplo.mx", map[string]string{
		"nombre": "<script>alert(1)</script> *Ana*\r\nBcc: otro@ejemplo.mx", "liga": liga,
	})
	if c.Destinatario != "ana@ejemplo.mx" || c.Idioma != models.IdiomaES || c.PlantillaClave != models.PlantillaVerificarEmail {
		t.Errorf("metadatos incorrectos: %+v", c)
	}
	// El asunto queda en una sola línea: no hay forma de inyectar cabeceras.
	if strings.ContainsAny(c.Asunto, "\r\n") || !strings.HasPrefix(c.Asunto, "Hola, <script>") {
		t.Errorf("asunto incorrecto: %q", c.Asunto)
	}
	for _, fragmento := range []string{
		`<html lang="es">`,
		"<strong>&lt;script&gt;alert(1)&lt;/script&gt; *Ana*", // ni HTML ni Markdown en el valor
		`<a href="https://postula.ejemplo.mx/cuenta/verificar?token=abc&amp;x=1">`,
		"{{desconocida}}", // una variable sin valor queda visible
	} {
		if !strings.Contains(c.HTML, fragmento) {
			t.Errorf("falta %q en el HTML:\n%s", fragmento, c.HTML)
		}
	}
	if strings.Contains(c.HTML, "<script>") || strings.Contains(c.HTML, "<em>Ana</em>") {
		t.Errorf("el valor de una variable se interpretó:\n%s", c.HTML)
	}
	// La versión de texto es la plantilla legible, con el enlace tal cual.
	if !strings.Contains(c.Texto, liga) || !strings.Contains(c.Texto, "Abre este enlace:") {
		t.Errorf("texto incorrecto:\n%s", c.Texto)
	}
}

// El CSV sale en UTF-8 con BOM, con comillas RFC 4180 y fin de línea CRLF.
func TestCSV(t *testing.T) {
	w := httptest.NewRecorder()
	err := CSV(w, "reporte.csv", []string{"folio", "nombre"}, [][]string{
		{"FIM49-INT-0001", `María "la Güera", Núñez`},
		{"FIM49-INT-0002", "dos\nrenglones"},
	})
	if err != nil {
		t.Fatal(err)
	}
	esperado := "\xEF\xBB\xBFfolio,nombre\r\n" +
		"FIM49-INT-0001,\"María \"\"la Güera\"\", Núñez\"\r\n" +
		"FIM49-INT-0002,\"dos\r\nrenglones\"\r\n" // el salto dentro de un campo también va en CRLF
	if w.Body.String() != esperado {
		t.Errorf("CSV inesperado:\n%q\nse esperaba:\n%q", w.Body.String(), esperado)
	}
	if w.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(w.Header().Get("Content-Disposition"), `filename="reporte.csv"`) {
		t.Errorf("cabeceras incorrectas: %v", w.Header())
	}
}

// snapshotDePrueba es un envío de Compositores con dos obras y sus documentos.
func snapshotDePrueba(idioma string) models.SnapshotEnvio {
	obra := func(id int64, orden int, dotacion string) models.SnapshotObra {
		return models.SnapshotObra{ID: id, Orden: orden, DotacionES: dotacion, DotacionEN: "String quartet", Campos: map[string]string{
			"obra.titulo": "Estudio n.º " + strconv.Itoa(orden), "obra.duracion_min": "9", "obra.estreno": "estreno_mundial",
			"obra.nota_obra": strings.Repeat("Nota de la obra. ", 40),
		}}
	}
	return models.SnapshotEnvio{
		Folio: "FIM49-COM-0007", Version: 1, EnviadaAt: "2027-01-30T05:30:00Z", Idioma: idioma,
		Edicion:      models.SnapshotEdicion{Numero: 49, Anio: 2027, NombreES: "Edición 49 · 2027", NombreEN: "49th Edition · 2027"},
		Convocatoria: models.SnapshotConvocatoria{ID: 2, Tipo: "compositores", TituloES: "Convocatoria de Compositores", TituloEN: "Call for Composers"},
		Titular:      models.SnapshotTitular{Nombre: "Eva", Apellidos: "Paz", Email: "eva@ejemplo.mx"},
		Campos: map[string]string{
			"datos.nombre_completo": "Eva Paz Dvořák", "datos.genero": "mujer", "datos.pais_residencia": "MX",
			"datos.entidad_residencia": "JAL", "datos.semblanza": strings.Repeat("Semblanza de la compositora. ", 30),
		},
		Obras: []models.SnapshotObra{obra(10, 1, "Cuarteto de cuerdas"), obra(11, 2, "Medios electrónicos fijos")},
		Documentos: []models.SnapshotDocumento{
			{Clave: "fotografia", EtiquetaES: "Fotografía", EtiquetaEN: "Photograph", Version: 1, SHA256: strings.Repeat("ab", 32), TamanoBytes: 812 << 10, MIME: "image/jpeg"},
			{Clave: "partitura", EtiquetaES: "Partitura", EtiquetaEN: "Score", ObraID: 11, Version: 3, SHA256: strings.Repeat("cd", 32), TamanoBytes: 4500000, MIME: "application/pdf"},
		},
	}
}

// textoDelPDF descomprime las páginas de un PDF del paquete internal/pdf.
func textoDelPDF(t *testing.T, archivo []byte) string {
	t.Helper()
	var texto strings.Builder
	for _, flujo := range regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`).FindAllSubmatch(archivo, -1) {
		lector, err := zlib.NewReader(bytes.NewReader(flujo[1]))
		if err != nil {
			t.Fatal(err)
		}
		contenido, err := io.ReadAll(lector)
		if err != nil {
			t.Fatal(err)
		}
		texto.Write(contenido)
	}
	return texto.String()
}

// El acuse lleva todo lo que pide RF-PDF-01, en el idioma del postulante, y
// se compone sólo del snapshot: siempre sale igual.
func TestAcusePDF(t *testing.T) {
	m, err := Nuevo()
	if err != nil {
		t.Fatal(err)
	}
	acuse, err := m.AcusePDF(snapshotDePrueba("es"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(acuse, []byte("%PDF-1.4")) {
		t.Fatal("el acuse no es un PDF")
	}
	// RF-PDF-02: objetivo, menos de 100 KB. Un acuse típico pesa unos pocos KB.
	if len(acuse) > 20<<10 {
		t.Errorf("el acuse pesa %d bytes", len(acuse))
	}
	otro, _ := m.AcusePDF(snapshotDePrueba("es"))
	if !bytes.Equal(acuse, otro) {
		t.Error("el mismo snapshot debe producir el mismo acuse, byte por byte")
	}
	texto := textoDelPDF(t, acuse)
	for _, fragmento := range []string{
		"(Acuse de recibo) Tj", "(FIM49-COM-0007) Tj", "(Edici\xF3n 49 \xB7 2027) Tj", "(Convocatoria de Compositores) Tj",
		// La hora de envío, en la Ciudad de México (UTC−6) y en UTC.
		"29 de enero de 2027, 23:30 h", "2027-01-30 05:30 UTC",
		"(Eva Paz \xB7 eva@ejemplo.mx) Tj",
		"(Eva Paz Dvor\xE1k) Tj", "(Mujer) Tj", "(Jalisco) Tj", // opción y entidad, por su nombre
		"(Obra 1) Tj", "(Obra 2) Tj", "(Estudio n.\xBA 2) Tj", "(Estreno mundial) Tj", "(Medios electr\xF3nicos fijos) Tj",
		"(Fotograf\xEDa) Tj", "(Versi\xF3n 1 \xB7 812 KB \xB7 SHA-256 abababababab\x85) Tj",
		"(Partitura \xB7 obra 2) Tj", "(Versi\xF3n 3 \xB7 4.3 MB \xB7 SHA-256 cdcdcdcdcdcd\x85) Tj",
		"No implica que los documentos hayan sido validados",
		"(FIM49-COM-0007 \xB7 Acuse de recibo, versi\xF3n 1) Tj", "(P\xE1gina 1 de ",
	} {
		if !strings.Contains(texto, fragmento) {
			t.Errorf("falta en el acuse: %q", fragmento)
		}
	}
	// De la suma sólo van los primeros 12 caracteres; nunca la clave de una
	// opción ni la de una traducción sin resolver.
	for _, prohibido := range []string{strings.Repeat("ab", 32), "estreno_mundial", "(mujer)", "acuse.", "campo.", "opcion."} {
		if strings.Contains(texto, prohibido) {
			t.Errorf("no debe aparecer en el acuse: %q", prohibido)
		}
	}

	// En inglés.
	ingles, err := m.AcusePDF(snapshotDePrueba("en"))
	if err != nil {
		t.Fatal(err)
	}
	texto = textoDelPDF(t, ingles)
	for _, fragmento := range []string{"(Acknowledgement of receipt) Tj", "(Call for Composers) Tj", "(49th Edition \xB7 2027) Tj",
		"(Woman) Tj", "(World premiere) Tj", "(Score \xB7 work 2) Tj", "(String quartet) Tj", "(Page 1 of ", "January 29, 2027"} {
		if !strings.Contains(texto, fragmento) {
			t.Errorf("falta en el acuse en inglés: %q", fragmento)
		}
	}

	// Un programa de cuarenta obras sigue muy por debajo del objetivo.
	grande := snapshotDePrueba("es")
	for i := 3; i <= 40; i++ {
		obra := grande.Obras[0]
		obra.ID, obra.Orden = int64(100+i), i
		grande.Obras = append(grande.Obras, obra)
	}
	if acuse, err := m.AcusePDF(grande); err != nil || len(acuse) > 100<<10 {
		t.Errorf("acuse de 40 obras: %d bytes (%v)", len(acuse), err)
	}
	// Un snapshot con la fecha dañada se rechaza en vez de inventar una.
	roto := snapshotDePrueba("es")
	roto.EnviadaAt = "ayer"
	if _, err := m.AcusePDF(roto); err == nil {
		t.Error("un snapshot con fecha inválida debe dar error")
	}
}

// fotografiaDePrueba compone un JPEG de ancho × alto con la mitad izquierda
// roja y la derecha azul, y la orientación EXIF indicada (0 = sin EXIF).
func fotografiaDePrueba(t *testing.T, ancho, alto, orientacion int) []byte {
	t.Helper()
	lienzo := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := 0; y < alto; y++ {
		for x := 0; x < ancho; x++ {
			tono := color.RGBA{R: 220, A: 255}
			if x >= ancho/2 {
				tono = color.RGBA{B: 220, A: 255}
			}
			lienzo.Set(x, y, tono)
		}
	}
	var salida bytes.Buffer
	if err := jpeg.Encode(&salida, lienzo, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if orientacion == 0 {
		return salida.Bytes()
	}
	// Segmento APP1 con un TIFF mínimo: una sola entrada, la orientación.
	tiff := []byte("II*\x00\x08\x00\x00\x00\x01\x00\x12\x01\x03\x00\x01\x00\x00\x00" + string(rune(orientacion)) + "\x00\x00\x00\x00\x00\x00\x00")
	exif := append([]byte("Exif\x00\x00"), tiff...)
	segmento := append([]byte{0xFF, 0xE1, byte((len(exif) + 2) >> 8), byte(len(exif) + 2)}, exif...)
	original := salida.Bytes()
	return append(append(append([]byte{}, original[:2]...), segmento...), original[2:]...)
}

// La fotografía se reduce a 800 pixeles, se endereza según su EXIF y queda
// en unas decenas de KB; lo que no es una imagen se rechaza sin romper nada.
func TestReducirFotografia(t *testing.T) {
	// colorEn decodifica el resultado y dice si un punto es rojo o azul.
	colorEn := func(reducida []byte, x, y int) string {
		imagen, err := jpeg.Decode(bytes.NewReader(reducida))
		if err != nil {
			t.Fatal(err)
		}
		rojo, _, azul, _ := imagen.At(x, y).RGBA()
		if rojo > azul {
			return "rojo"
		}
		return "azul"
	}
	// Apaisada y grande: se reduce conservando la proporción.
	reducida, ancho, alto, err := ReducirFotografia(fotografiaDePrueba(t, 2400, 1600, 0))
	if err != nil || ancho != 800 || alto != 533 || len(reducida) > 60<<10 {
		t.Fatalf("fotografía reducida: %d × %d, %d bytes, %v", ancho, alto, len(reducida), err)
	}
	if colorEn(reducida, 10, 10) != "rojo" || colorEn(reducida, 790, 500) != "azul" {
		t.Error("la reducción cambió de lugar los colores")
	}
	// Pequeña: no se agranda.
	if _, ancho, alto, err = ReducirFotografia(fotografiaDePrueba(t, 300, 200, 0)); err != nil || ancho != 300 || alto != 200 {
		t.Errorf("una fotografía pequeña no se toca: %d × %d, %v", ancho, alto, err)
	}
	// Tomada con el teléfono en vertical (EXIF 6): sale de pie, con lo que
	// estaba a la izquierda del sensor arriba.
	reducida, ancho, alto, err = ReducirFotografia(fotografiaDePrueba(t, 1200, 800, 6))
	if err != nil || ancho != 533 || alto != 800 || colorEn(reducida, 266, 20) != "rojo" || colorEn(reducida, 266, 780) != "azul" {
		t.Errorf("orientación 6: %d × %d, %v", ancho, alto, err)
	}
	// EXIF 8: al revés. EXIF 3: de cabeza.
	if reducida, _, _, err = ReducirFotografia(fotografiaDePrueba(t, 1200, 800, 8)); err != nil || colorEn(reducida, 266, 20) != "azul" {
		t.Errorf("orientación 8: %v", err)
	}
	if reducida, ancho, _, err = ReducirFotografia(fotografiaDePrueba(t, 1200, 800, 3)); err != nil || ancho != 800 || colorEn(reducida, 20, 266) != "azul" {
		t.Errorf("orientación 3: %v", err)
	}
	// PNG con transparencia: fondo blanco, no negro.
	transparente := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	var salidaPNG bytes.Buffer
	if err := png.Encode(&salidaPNG, transparente); err != nil {
		t.Fatal(err)
	}
	reducida, _, _, err = ReducirFotografia(salidaPNG.Bytes())
	if err != nil {
		t.Fatalf("PNG transparente: %v", err)
	}
	if imagen, _ := jpeg.Decode(bytes.NewReader(reducida)); imagen != nil {
		if rojo, verde, azul, _ := imagen.At(20, 20).RGBA(); rojo < 0xF000 || verde < 0xF000 || azul < 0xF000 {
			t.Error("la transparencia debe quedar blanca")
		}
	}
	// Lo que no es una imagen, o es desmesurado, se rechaza.
	for nombre, datos := range map[string][]byte{
		"vacío": nil, "texto": []byte("no soy una imagen"), "JPEG trunco": fotografiaDePrueba(t, 100, 100, 0)[:40],
		"EXIF trunco": append([]byte("\xFF\xD8\xFF\xE1\x00\x10Exif\x00\x00II*\x00"), 0xFF),
	} {
		if _, _, _, err := ReducirFotografia(datos); !errors.Is(err, ErrFotografiaIlegible) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	if orientacionEXIF([]byte("\xFF\xD8\xFF\xE1\xFF\xFFExif")) != 1 || orientacionEXIF(nil) != 1 {
		t.Error("un EXIF malformado debe leerse como «derecha»")
	}
}

// El expediente lleva la fotografía incrustada, índice y marcadores cuando es
// de varias postulaciones, y pinta sólo lo que el recorte dejó.
func TestExpedientePDF(t *testing.T) {
	m, err := Nuevo()
	if err != nil {
		t.Fatal(err)
	}
	cfg := models.ConfiguracionConvocatoria{
		Convocatoria: models.Convocatoria{ID: 1, Tipo: models.TipoCompositores, TituloES: "Convocatoria de Compositores",
			Edicion: models.Edicion{Numero: 49, NombreES: "49.º Foro"}},
		Criterios:  models.CriteriosIniciales(),
		Requisitos: []models.RequisitoDocumental{{Clave: "partitura", EtiquetaES: "Partitura", EtiquetaEN: "Score"}},
	}
	postulacion := func(n int64) models.Postulacion {
		return models.Postulacion{ID: n, ConvocatoriaID: 1, Folio: fmt.Sprintf("FIM49-COM-%04d", n), Estado: models.PostulacionEvaluada, Envios: 1,
			EnviadaAt: time.Date(2026, 12, 1, 18, 0, 0, 0, time.UTC),
			Campos:    map[string]string{"datos.nombre_completo": fmt.Sprintf("Compositora %d", n), "datos.semblanza": strings.Repeat("Texto de la semblanza. ", 200)},
			Obras:     []models.Obra{{ID: 10 * n, Orden: 1, Campos: map[string]string{"obra.titulo": "Rapsodia"}}},
			Documentos: []models.Documento{{ID: 7, Clave: "partitura", ObraID: 10 * n, Version: 1, Dictamen: "invalido",
				Archivo: models.Archivo{SHA256: strings.Repeat("ab", 32), TamanoBytes: 4000, MIME: "application/pdf"}}},
		}
	}
	opciones := OpcionesDeExpediente{Idioma: "es", Solicitante: "María Núñez", Generado: time.Date(2026, 12, 21, 16, 0, 0, 0, time.UTC), HojaDeCalificacion: true}
	expediente := m.NuevoExpediente(opciones)
	for n := int64(1); n <= 3; n++ {
		x := models.Expediente{Postulacion: postulacion(n), Configuracion: cfg, Titular: models.Usuario{Nombre: "Ana", Email: "ana@ejemplo.mx"},
			Recorte:  models.RecorteDeExpediente{Revision: true, Supervision: true},
			Revision: models.Revision{Dictamenes: []models.DictamenDocumento{{DocumentoID: 7, Dictamen: "invalido", Motivo: "Ilegible."}}},
			Jurados:  []models.JuradoEnMatriz{{ID: 41, Nombre: "Jurado Uno"}, {ID: 42, Nombre: "Jurado Dos"}},
			Resultados: []models.FilaDeResultado{{Unidad: models.UnidadDeEvaluacion{PostulacionID: n, ObraID: 10 * n}, JuradosQueEvaluaron: 1, Total: 9, Promedio: 4.5, Comparable: 27,
				PorJurado: map[int64]models.Evaluacion{41: {Enviada: true, Puntajes: map[string]int{"realizacion": 4, "artistico": 5}, Observaciones: "Muy sólida."}},
				Excusas:   map[int64]string{42: "Fue mi alumna."}}},
		}
		expediente.Agregar(x, fotografiaDePrueba(t, 1200, 900, 0))
	}
	if expediente.Postulaciones() != 3 {
		t.Fatalf("postulaciones: %d", expediente.Postulaciones())
	}
	archivo, err := expediente.Bytes("Convocatoria de Compositores · 49.º Foro")
	if err != nil {
		t.Fatal(err)
	}
	// Tres fotografías incrustadas, reducidas; y tres marcadores.
	if bytes.Count(archivo, []byte("/Filter /DCTDecode")) != 3 || !bytes.Contains(archivo, []byte("/Width 800 /Height 600")) ||
		!bytes.Contains(archivo, []byte("/Type /Outlines")) || bytes.Count(archivo, []byte("/Dest [")) != 3 {
		t.Error("el expediente no trae sus fotografías o sus marcadores")
	}
	if len(archivo) > 400<<10 {
		t.Errorf("el expediente de tres postulaciones pesa %d KB", len(archivo)>>10)
	}
	texto := textoDelPDF(t, archivo[:bytes.Index(archivo, []byte("/Subtype /Image"))]) // sólo las páginas
	for _, fragmento := range []string{
		"(\xCDndice) Tj", "(FIM49-COM-0001 \xB7 Compositora 1) Tj", "(FIM49-COM-0003 \xB7 Compositora 3) Tj", "(Convocatoria de Compositores \xB7 49.\xBA Foro) Tj",
		"(Evaluada) Tj", "(Rapsodia) Tj", "(Revisi\xF3n: inv\xE1lido: Ilegible.) Tj", "(Jurado Uno) Tj", "(4 de 5) Tj", "(Muy s\xF3lida.) Tj",
		"(Excusa por conflicto de inter\xE9s) Tj", "(Fue mi alumna.) Tj", "(Puntaje comparable) Tj", "(27) Tj", "(Hoja de calificaci\xF3n) Tj",
		"(FIM49-COM-0002 \xB7 CONFIDENCIAL \xB7 2026-12-21 10:00 \xB7 Mar\xEDa N\xFA\xF1ez) Tj", "/Im2 Do Q",
	} {
		if !strings.Contains(texto, fragmento) {
			t.Errorf("falta %q en el expediente", fragmento)
		}
	}
	// El índice da la página real de cada postulación: la primera empieza
	// justo después del índice, y su folio está en el pie de esa página.
	paginas := regexp.MustCompile(`(?s)/FlateDecode >>\nstream\n(.*?)\nendstream`).FindAllSubmatch(archivo, -1)
	primera := textoDelPDF(t, []byte("stream\n"+string(paginas[1][1])+"\nendstream"))
	if !strings.Contains(textoDelPDF(t, []byte("stream\n"+string(paginas[0][1])+"\nendstream")), "(2) Tj") || !strings.Contains(primera, "FIM49-COM-0001 \xB7 CONFIDENCIAL") {
		t.Error("el índice no apunta a la página en que empieza la primera postulación")
	}

	// La vista pinta lo que recibe: un expediente recortado no trae nada de
	// lo reservado. De una sola postulación va sin índice.
	recortado := models.Expediente{Postulacion: postulacion(1), Configuracion: cfg, Recorte: models.RecorteDeExpediente{}}
	models.RecortarExpediente(&recortado)
	solo := m.NuevoExpediente(OpcionesDeExpediente{Idioma: "en", Solicitante: "Ana", Generado: opciones.Generado, SinFotografia: true})
	solo.Agregar(recortado, fotografiaDePrueba(t, 100, 100, 0))
	archivo, err = solo.Bytes("")
	if err != nil {
		t.Fatal(err)
	}
	texto = textoDelPDF(t, archivo)
	for _, prohibido := range []string{"Review:", "Jury evaluation", "Document review", "Contents", "Do Q"} {
		if strings.Contains(texto, prohibido) {
			t.Errorf("el expediente recortado no debe traer %q", prohibido)
		}
	}
	if !strings.Contains(texto, "(Application file) Tj") || !strings.Contains(texto, "CONFIDENTIAL") || bytes.Contains(archivo, []byte("/Outlines")) {
		t.Error("expediente individual en inglés mal compuesto")
	}
}
