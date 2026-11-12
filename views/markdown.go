// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package views

import (
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

// Las bases de las convocatorias se capturan en Markdown (RF-CONV-02). Este
// intérprete cubre deliberadamente un SUBCONJUNTO pequeño, suficiente para
// unas bases y legible completo de una sentada:
//
//	# Título, ## Subtítulo, ### Apartado
//	párrafos separados por una línea en blanco
//	- viñetas            1. listas numeradas
//	**negritas**         *cursivas*
//	[texto](https://…)   (sólo http, https y mailto)
//
// Todo lo demás —incluido cualquier HTML— se muestra como texto literal. Esa
// es la garantía de seguridad: el texto se escapa PRIMERO y sólo después se
// añaden las pocas etiquetas de esta lista.

var (
	mdNegritas  = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdCursivas  = regexp.MustCompile(`\*([^*]+)\*`)
	mdEnlace    = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	mdNumerada  = regexp.MustCompile(`^\d+[.)]\s+(.*)$`)
	mdEsquemas  = map[string]bool{"http": true, "https": true, "mailto": true}
	mdTitulos   = []string{"### ", "## ", "# "}
	mdEtiquetas = map[string]string{"# ": "h2", "## ": "h3", "### ": "h4"}
)

// Markdown convierte el subconjunto descrito arriba en HTML seguro. Los
// títulos empiezan en <h2> porque el <h1> es el de la página.
func Markdown(fuente string) template.HTML {
	var salida strings.Builder
	var parrafo []string
	lista := "" // "", "ul" u "ol": la lista abierta en este momento

	cerrarParrafo := func() {
		if len(parrafo) > 0 {
			salida.WriteString("<p>" + mdEnLinea(strings.Join(parrafo, " ")) + "</p>\n")
			parrafo = nil
		}
	}
	cerrarLista := func() {
		if lista != "" {
			salida.WriteString("</" + lista + ">\n")
			lista = ""
		}
	}
	elemento := func(tipo, texto string) {
		cerrarParrafo()
		if lista != tipo {
			cerrarLista()
			salida.WriteString("<" + tipo + ">\n")
			lista = tipo
		}
		salida.WriteString("<li>" + mdEnLinea(texto) + "</li>\n")
	}

	fuente = strings.ReplaceAll(fuente, "\r\n", "\n")
lineas:
	for _, linea := range strings.Split(fuente, "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" {
			cerrarParrafo()
			cerrarLista()
			continue
		}
		for _, marca := range mdTitulos {
			if texto, ok := strings.CutPrefix(linea, marca); ok {
				cerrarParrafo()
				cerrarLista()
				etiqueta := mdEtiquetas[marca]
				salida.WriteString("<" + etiqueta + ">" + mdEnLinea(texto) + "</" + etiqueta + ">\n")
				continue lineas
			}
		}
		if texto, ok := strings.CutPrefix(linea, "- "); ok {
			elemento("ul", texto)
			continue
		}
		if m := mdNumerada.FindStringSubmatch(linea); m != nil {
			elemento("ol", m[1])
			continue
		}
		cerrarLista()
		parrafo = append(parrafo, linea)
	}
	cerrarParrafo()
	cerrarLista()
	return template.HTML(salida.String())
}

// mdEnLinea escapa el texto y luego aplica negritas, cursivas y enlaces. Los
// enlaces se localizan primero y su URL se copia intacta: así un asterisco
// dentro de una dirección nunca se confunde con énfasis.
func mdEnLinea(texto string) string {
	texto = html.EscapeString(texto)
	var salida strings.Builder
	resto := 0
	for _, m := range mdEnlace.FindAllStringSubmatchIndex(texto, -1) {
		etiqueta, destinoEscapado := texto[m[2]:m[3]], texto[m[4]:m[5]]
		destino, err := url.Parse(html.UnescapeString(destinoEscapado))
		if err != nil || !mdEsquemas[strings.ToLower(destino.Scheme)] {
			continue // esquema no admitido (javascript:, data:…): queda como texto
		}
		salida.WriteString(mdEnfasis(texto[resto:m[0]]))
		// destinoEscapado ya pasó por EscapeString: no puede cerrar el atributo.
		salida.WriteString(`<a href="` + destinoEscapado + `" rel="noopener noreferrer">` + mdEnfasis(etiqueta) + `</a>`)
		resto = m[1]
	}
	salida.WriteString(mdEnfasis(texto[resto:]))
	return salida.String()
}

// mdEnfasis aplica negritas y cursivas a un texto ya escapado.
func mdEnfasis(texto string) string {
	texto = mdNegritas.ReplaceAllString(texto, "<strong>$1</strong>")
	return mdCursivas.ReplaceAllString(texto, "<em>$1</em>")
}
