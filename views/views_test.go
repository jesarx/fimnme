// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package views

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
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
