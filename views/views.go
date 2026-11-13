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

// Package views contiene la Vista de la arquitectura MVC: el frontal web
// integrado en el binario con //go:embed (plantillas html/template, hoja de
// estilo propia, htmx y traducciones). Aquí no hay reglas de negocio ni
// acceso a datos: sólo presentación.
package views

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
)

// contenido es todo el frontal, incrustado en el binario al compilar. No hay
// node_modules, empaquetador ni archivos sueltos que desplegar.
//
//go:embed templates static i18n api
var contenido embed.FS

// OpenAPI devuelve la descripción de la API REST en formato OpenAPI 3.1. Es el
// contrato público de /api/v1: una prueba impide que se desincronice de las
// rutas que el servidor realmente atiende.
func OpenAPI() ([]byte, error) {
	return contenido.ReadFile("api/openapi.json")
}

// plantillaBase es el armazón común (cabecera, navegación, pie) de toda página.
const plantillaBase = "base.html"

// Motor carga las plantillas, las traducciones y los estáticos una sola vez,
// al arrancar. Es seguro para uso concurrente porque después sólo se lee.
type Motor struct {
	paginas   map[string]*template.Template
	textos    *Traducciones
	estaticos fs.FS
	version   string // huella del contenido estático, para el caché del navegador
}

// Nuevo construye el motor. Falla al arrancar —no a media petición— si alguna
// plantilla o archivo de traducción tiene un error.
func Nuevo() (*Motor, error) {
	textos, err := cargarTraducciones(contenido)
	if err != nil {
		return nil, err
	}
	estaticos, err := fs.Sub(contenido, "static")
	if err != nil {
		return nil, fmt.Errorf("estáticos: %w", err)
	}
	version, err := huellaEstaticos(estaticos)
	if err != nil {
		return nil, err
	}
	m := &Motor{
		paginas:   make(map[string]*template.Template),
		textos:    textos,
		estaticos: estaticos,
		version:   version,
	}

	funciones := template.FuncMap{
		"estatico": m.URLEstatico,
		"dict":     dict,
	}
	archivos, err := fs.Glob(contenido, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("plantillas: %w", err)
	}
	// Cada página se compila junto con la base y los parciales, de modo que
	// sus bloques "contenido" no choquen con los de otra página.
	for _, archivo := range archivos {
		nombre := path.Base(archivo)
		if nombre == plantillaBase {
			continue
		}
		t, err := template.New(plantillaBase).Funcs(funciones).ParseFS(contenido,
			"templates/"+plantillaBase, "templates/parciales/*.html", archivo)
		if err != nil {
			return nil, fmt.Errorf("plantilla %s: %w", nombre, err)
		}
		m.paginas[nombre] = t
	}
	return m, nil
}

// T traduce una clave al idioma dado. Lo usan los controladores para armar
// títulos; dentro de las plantillas se usa el método Pagina.T.
func (m *Motor) T(idioma, clave string, args ...any) string {
	return m.textos.T(idioma, clave, args...)
}

// URLEstatico devuelve la URL de un archivo estático con la huella del
// contenido. Al cambiar cualquier estático cambia la huella, y el navegador
// descarga la versión nueva sin intervención.
func (m *Motor) URLEstatico(ruta string) string {
	return "/static/" + ruta + "?v=" + m.version
}

// Estaticos sirve la hoja de estilo, htmx y demás archivos incrustados.
func (m *Motor) Estaticos() http.Handler {
	servidor := http.StripPrefix("/static/", http.FileServerFS(m.estaticos))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sin listados de directorio: sólo se sirven archivos concretos.
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("v") == m.version {
			// La URL lleva la huella: su contenido no cambiará jamás.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		servidor.ServeHTTP(w, r)
	})
}

// Render ejecuta una página completa y la escribe con el estado HTTP dado. Se
// genera primero en memoria: si la plantilla falla no se envía media página.
func (m *Motor) Render(w http.ResponseWriter, estado int, plantilla string, p *Pagina) error {
	t, ok := m.paginas[plantilla]
	if !ok {
		return fmt.Errorf("plantilla desconocida: %s", plantilla)
	}
	p.motor = m
	var salida bytes.Buffer
	if err := t.ExecuteTemplate(&salida, plantillaBase, p); err != nil {
		return fmt.Errorf("plantilla %s: %w", plantilla, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(estado)
	_, err := salida.WriteTo(w)
	return err
}

// Pagina es el dato raíz que recibe toda plantilla. Sus métodos resuelven
// traducción y formato en el idioma de la petición, por lo que dentro de un
// {{range}} se invocan con $ (p. ej. {{$.T "clave"}}).
type Pagina struct {
	Idioma     string         // "es" o "en"
	Ruta       string         // ruta actual; sirve para volver tras cambiar de idioma
	Titulo     string         // título ya traducido, para <title>
	Entorno    string         // "desarrollo" muestra la franja de advertencia
	Aviso      *Aviso         // mensaje destacado opcional
	Sesion     *models.Sesion // sesión en curso, o nil si el visitante es anónimo
	Formulario Formulario     // valores y errores del formulario, si la página tiene uno
	Datos      any            // modelo de vista propio de cada página

	motor *Motor
}

// Aviso es un mensaje destacado al inicio del contenido.
type Aviso struct {
	Tipo  string // "info", "exito" o "error": define el estilo
	Clave string // clave de traducción del mensaje
}

// Formulario lleva de vuelta a la plantilla lo que el usuario capturó y los
// problemas encontrados por models/rules.go.
type Formulario struct {
	Valores map[string]string
	Errores []models.ErrorCampo
}

// Valor devuelve lo capturado en un campo, para no perderlo al repintar.
func (f Formulario) Valor(campo string) string {
	return f.Valores[campo]
}

// Error devuelve la clave de traducción del error de un campo, o "" si no hay.
func (f Formulario) Error(campo string) string {
	for _, e := range f.Errores {
		if e.Campo == campo {
			return e.Clave
		}
	}
	return ""
}

// T traduce una clave al idioma de la página.
func (p *Pagina) T(clave string, args ...any) string {
	return p.motor.textos.T(p.Idioma, clave, args...)
}

// OtroIdioma devuelve el idioma al que conmuta el selector de la cabecera.
func (p *Pagina) OtroIdioma() string {
	if p.Idioma == models.IdiomaEN {
		return models.IdiomaES
	}
	return models.IdiomaEN
}

// EsDesarrollo indica si debe mostrarse la franja de entorno de desarrollo.
func (p *Pagina) EsDesarrollo() bool {
	return p.Entorno == "desarrollo"
}

// Fecha formatea un día en el idioma de la página, en hora de México.
func (p *Pagina) Fecha(t time.Time) string {
	return p.motor.textos.Fecha(p.Idioma, t)
}

// FechaHora formatea día y hora con la zona horaria explícita (RF-I18N-06).
func (p *Pagina) FechaHora(t time.Time) string {
	return p.motor.textos.FechaHora(p.Idioma, t)
}

// ISO devuelve la marca en ISO-8601 UTC para el atributo datetime de <time>.
func (p *Pagina) ISO(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// dict arma un mapa a partir de pares clave-valor, para pasar varios
// argumentos a un parcial: {{template "campo" (dict "P" $ "Nombre" "email")}}.
func dict(pares ...any) (map[string]any, error) {
	if len(pares)%2 != 0 {
		return nil, errors.New("dict: se requiere un número par de argumentos")
	}
	m := make(map[string]any, len(pares)/2)
	for i := 0; i < len(pares); i += 2 {
		clave, ok := pares[i].(string)
		if !ok {
			return nil, errors.New("dict: las claves deben ser texto")
		}
		m[clave] = pares[i+1]
	}
	return m, nil
}

// huellaEstaticos calcula un resumen del contenido de todos los estáticos.
// fs.WalkDir recorre en orden lexicográfico, así que el resultado es
// determinista: mismo código fuente, misma huella, mismo binario.
func huellaEstaticos(estaticos fs.FS) (string, error) {
	resumen := sha256.New()
	err := fs.WalkDir(estaticos, ".", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		datos, err := fs.ReadFile(estaticos, ruta)
		if err != nil {
			return err
		}
		fmt.Fprintf(resumen, "%s\x00%d\x00", ruta, len(datos))
		resumen.Write(datos)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("huella de estáticos: %w", err)
	}
	return hex.EncodeToString(resumen.Sum(nil))[:12], nil
}
