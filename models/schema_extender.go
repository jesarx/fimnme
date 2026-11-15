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

package models

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Motor de extensión dinámica de nodos (§5.14.1 y §7.6).
//
// Permite añadir y quitar campos a ciertas tablas con ALTER TABLE, sin
// recompilar: cada campo queda registrado en el diccionario de metadatos
// (campo_extendido) con su etiqueta en ambos idiomas, su tipo, quién lo ve y
// si se exporta.
//
// El mecanismo convive con un proceso que tiene efectos jurídicos, y por eso
// está acotado. Las reglas de este archivo son la cerca:
//
//   - Sólo opera sobre los nodos de la lista blanca (RF-CLI-23). Las tablas
//     probatorias —evaluaciones, acuses, descargas, bitácora, subsanaciones—
//     están vedadas (RF-CLI-24), y cualquier otra tabla, también.
//   - Toda columna dinámica se llama «x_…». Así nunca choca con una columna
//     del esquema, presente o futura, y «schema drop» no puede tocar una
//     columna nativa aunque alguien se lo pida.
//   - Un campo extendido es siempre opcional: no altera la obligatoriedad de
//     un documento ni los criterios de evaluación (RF-CLI-25), que viven en
//     sus propias tablas de configuración.
//   - No se desbloquea en una subsanación ni entra en un acuse (RF-CLI-26):
//     el acuse se compone sólo con los campos del formulario.
//
// Aquí no hay SQL que se ejecute: sólo se COMPONE la sentencia DDL, a partir
// de identificadores ya validados. La ejecuta repository.go, que la deja en
// la bitácora tal cual (RF-CLI-27).

const (
	// PrefijoDeColumnaExtendida es el prefijo de toda columna dinámica.
	PrefijoDeColumnaExtendida = "x_"
	// PrefijoDeRutaExtendida es el de la ruta canónica de un campo
	// extendido: «ext.<tabla>.<columna>» (§9).
	PrefijoDeRutaExtendida = "ext."
	// longitudDeTextoExtendido es lo más que mide un valor de texto.
	longitudDeTextoExtendido = 2000
	// maximoDeCamposExtendidos acota cuántos campos dinámicos puede tener
	// una tabla: es una escotilla, no un segundo esquema.
	maximoDeCamposExtendidos = 30
)

// NodosExtensibles es la lista blanca de tablas que admiten DDL dinámico
// (RF-CLI-23).
var NodosExtensibles = []string{
	"edicion", "convocatoria", "dotacion", "postulacion", "obra",
	"datos_solista", "datos_ensamble", "datos_compositor", "propuesta_concierto",
	"integrante", "enlace_video",
}

// NodosVedados son las tablas que NUNCA admiten DDL dinámico (RF-CLI-24):
// son material probatorio del proceso. No hace falta listarlas para
// prohibirlas —lo que no está en la lista blanca ya está prohibido—, pero
// nombrarlas permite decirle a quien lo intenta por qué no.
var NodosVedados = []string{
	"evaluacion", "evaluacion_puntaje", "acuse", "descarga", "auditoria",
	"subsanacion", "subsanacion_problema", "revision_documento",
}

// TipoExtendido es el tipo de un campo dinámico.
type TipoExtendido string

// Tipos de campo dinámico, con su tipo SQL (común a SQLite y MariaDB).
const (
	ExtendidoTexto   TipoExtendido = "texto"   // TEXT
	ExtendidoEntero  TipoExtendido = "entero"  // INTEGER
	ExtendidoDecimal TipoExtendido = "decimal" // REAL
)

// tipoSQLDeExtendido traduce el tipo de un campo dinámico a su tipo SQL.
var tipoSQLDeExtendido = map[TipoExtendido]string{ExtendidoTexto: "TEXT", ExtendidoEntero: "INTEGER", ExtendidoDecimal: "REAL"}

// TipoExtendidoDeSQL devuelve el tipo de campo que corresponde a un tipo SQL
// del diccionario.
func TipoExtendidoDeSQL(tipoSQL string) TipoExtendido {
	for tipo, sql := range tipoSQLDeExtendido {
		if sql == tipoSQL {
			return tipo
		}
	}
	return ExtendidoTexto
}

// PerfilesDeVisibilidad son los perfiles a los que puede darse a ver un campo
// extendido. El administrador y el observador los ven todos siempre.
var PerfilesDeVisibilidad = []Perfil{PerfilPostulante, PerfilRevisorDocumental, PerfilJurado}

// CampoExtendido es un campo añadido a un nodo sin recompilar, tal como lo
// describe el diccionario de metadatos (RF-CLI-21).
type CampoExtendido struct {
	ID         int64
	Tabla      string
	Columna    string // nombre físico, con su prefijo «x_»
	Tipo       TipoExtendido
	EtiquetaES string
	EtiquetaEN string
	AyudaES    string
	AyudaEN    string
	// VisibleEn son los perfiles que ven el campo, además del administrador
	// y el observador. Si incluye «postulante», el campo aparece en el
	// formulario y lo captura quien postula; si no, es un dato interno que
	// captura la administración.
	VisibleEn  []Perfil
	Exportable bool // entra en el catálogo de reportes y en la exportación a MariaDB
	CreadoAt   time.Time
}

// Ruta devuelve la ruta canónica del campo: «ext.<tabla>.<columna>».
func (c CampoExtendido) Ruta() string {
	return PrefijoDeRutaExtendida + c.Tabla + "." + c.Columna
}

// Etiqueta devuelve la etiqueta en el idioma pedido, con respaldo en español.
func (c CampoExtendido) Etiqueta(idioma string) string {
	if idioma == IdiomaEN && c.EtiquetaEN != "" {
		return c.EtiquetaEN
	}
	return c.EtiquetaES
}

// LoCapturaElPostulante indica que el campo aparece en el formulario de
// postulación y lo llena quien postula.
func (c CampoExtendido) LoCapturaElPostulante() bool {
	return NodoDePostulacion(c.Tabla) && slices.Contains(c.VisibleEn, PerfilPostulante)
}

// Definicion convierte el campo en una definición de formulario, para que lo
// valide y lo guarde el mismo código que a los campos nativos. Siempre es
// opcional.
func (c CampoExtendido) Definicion() DefinicionCampo {
	def := DefinicionCampo{Ruta: c.Ruta(), tabla: c.Tabla, columna: c.Columna, llave: llaveDeNodo(c.Tabla)}
	switch c.Tipo {
	case ExtendidoEntero:
		def.Tipo, def.Minimo, def.Maximo = CampoEntero, -1_000_000_000, 1_000_000_000
	case ExtendidoDecimal:
		def.Tipo = CampoDecimal
	default:
		def.Tipo, def.Longitud = CampoTextoLargo, longitudDeTextoExtendido
	}
	return def
}

// EsRutaExtendida indica si una ruta canónica es la de un campo extendido.
func EsRutaExtendida(ruta string) bool {
	return strings.HasPrefix(ruta, PrefijoDeRutaExtendida)
}

// NodoExtensible indica si una tabla está en la lista blanca.
func NodoExtensible(tabla string) bool {
	return slices.Contains(NodosExtensibles, tabla)
}

// NodoDePostulacion indica si una tabla guarda datos de una postulación (y no
// de la configuración): sus campos extendidos pueden mostrarse a quien
// postula, revisa o evalúa.
func NodoDePostulacion(tabla string) bool {
	return slices.Contains([]string{"postulacion", "obra", "datos_solista", "datos_ensamble", "datos_compositor", "propuesta_concierto"}, tabla)
}

// NodoConCaptura indica si los campos extendidos de una tabla pueden
// capturarse. Los de las listas de una postulación (integrantes y ligas de
// video) no: sus renglones se reescriben completos en cada guardado, y un
// valor capturado se perdería. En esas dos tablas el motor añade la columna y
// la registra, pero aún no hay cómo llenarla.
func NodoConCaptura(tabla string) bool {
	return NodoExtensible(tabla) && tabla != "integrante" && tabla != "enlace_video"
}

// llaveDeNodo es la columna que identifica una fila de un nodo: su propio id
// o, en las tablas que cuelgan una a una de la postulación, el de ésta.
func llaveDeNodo(tabla string) string {
	if slices.Contains([]string{"datos_solista", "datos_ensamble", "datos_compositor", "propuesta_concierto"}, tabla) {
		return "postulacion_id"
	}
	return "id"
}

// nombreDeColumnaExtendida es la única forma que puede tener el nombre de un
// campo dinámico: minúsculas, dígitos y guion bajo, empezando por letra. Es
// lo que hace seguro componer el DDL con él.
var nombreDeColumnaExtendida = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

// ColumnaExtendida devuelve el nombre físico de la columna de un campo
// dinámico a partir del nombre que le dio quien lo crea.
func ColumnaExtendida(nombre string) string {
	return PrefijoDeColumnaExtendida + strings.TrimPrefix(strings.TrimSpace(nombre), PrefijoDeColumnaExtendida)
}

// ValidarCampoExtendido valida un campo que se quiere añadir. existentes es
// cuántos campos dinámicos activos tiene ya su tabla.
func ValidarCampoExtendido(c CampoExtendido, existentes int) []ErrorCampo {
	var errores []ErrorCampo
	falla := func(campo, clave string) { errores = append(errores, ErrorCampo{Campo: campo, Clave: clave}) }
	switch {
	case slices.Contains(NodosVedados, c.Tabla):
		// Se dice por qué: son material probatorio del proceso.
		falla("tabla", "error.nodo_vedado")
	case !NodoExtensible(c.Tabla):
		falla("tabla", "error.nodo_no_extensible")
	}
	if !strings.HasPrefix(c.Columna, PrefijoDeColumnaExtendida) || !nombreDeColumnaExtendida.MatchString(strings.TrimPrefix(c.Columna, PrefijoDeColumnaExtendida)) {
		falla("nombre", "error.nombre_de_campo")
	}
	if _, conocido := tipoSQLDeExtendido[c.Tipo]; !conocido {
		falla("tipo", "error.opcion_invalida")
	}
	if largo := utf8.RuneCountInString(strings.TrimSpace(c.EtiquetaES)); largo == 0 || largo > 120 {
		falla("etiqueta_es", "error.etiqueta_requerida")
	}
	if utf8.RuneCountInString(c.EtiquetaEN) > 120 || utf8.RuneCountInString(c.AyudaES) > 500 || utf8.RuneCountInString(c.AyudaEN) > 500 {
		falla("etiqueta_en", "error.texto_largo_campo")
	}
	for _, perfil := range c.VisibleEn {
		if !slices.Contains(PerfilesDeVisibilidad, perfil) {
			falla("visible_en", "error.opcion_invalida")
			break
		}
	}
	if len(c.VisibleEn) > 0 && !NodoDePostulacion(c.Tabla) {
		// Los nodos de configuración sólo los ve la administración.
		falla("visible_en", "error.visibilidad_sin_postulacion")
	}
	if existentes >= maximoDeCamposExtendidos {
		falla("tabla", "error.demasiados_campos")
	}
	return errores
}

// identificadorSeguro comprueba, una vez más y justo antes de componer DDL,
// que la tabla esté en la lista blanca y que la columna sea dinámica y tenga
// la forma permitida. Es deliberadamente redundante con la validación: una
// sentencia DDL nunca debe salir de aquí con un identificador sin comprobar.
func identificadorSeguro(tabla, columna string) bool {
	return NodoExtensible(tabla) && strings.HasPrefix(columna, PrefijoDeColumnaExtendida) &&
		nombreDeColumnaExtendida.MatchString(strings.TrimPrefix(columna, PrefijoDeColumnaExtendida))
}

// SentenciaParaAgregar compone el DDL que añade un campo dinámico. Devuelve
// false si la tabla o la columna no son admisibles.
func SentenciaParaAgregar(c CampoExtendido) (string, bool) {
	tipoSQL, conocido := tipoSQLDeExtendido[c.Tipo]
	if !conocido || !identificadorSeguro(c.Tabla, c.Columna) {
		return "", false
	}
	return "ALTER TABLE " + c.Tabla + " ADD COLUMN " + c.Columna + " " + tipoSQL, true
}

// SentenciaParaEliminar compone el DDL que quita un campo dinámico. Sólo
// puede nombrar columnas «x_…» de tablas de la lista blanca.
func SentenciaParaEliminar(tabla, columna string) (string, bool) {
	if !identificadorSeguro(tabla, columna) {
		return "", false
	}
	return "ALTER TABLE " + tabla + " DROP COLUMN " + columna, true
}

// ExtendidosDePostulacion separa, de todos los campos dinámicos, los que
// aplican a una postulación de cierto tipo y subcategoría: los de la propia
// postulación, los de su tabla de datos generales y, en Intérpretes, los de
// la propuesta de concierto; y aparte, los de sus obras.
func ExtendidosDePostulacion(todos []CampoExtendido, tipo TipoConvocatoria, sub Subcategoria) (dePostulacion, deObra []CampoExtendido) {
	tablas := []string{"postulacion", tablaDeDatos(tipo, sub)}
	if tipo == TipoInterpretes {
		tablas = append(tablas, "propuesta_concierto")
	}
	for _, c := range todos {
		switch {
		case c.Tabla == "obra":
			deObra = append(deObra, c)
		case slices.Contains(tablas, c.Tabla):
			dePostulacion = append(dePostulacion, c)
		}
	}
	return dePostulacion, deObra
}

// VeExtendido indica si una sesión ve un campo extendido de una postulación.
// El administrador y el observador, siempre. Los demás, sólo si su perfil
// está entre los que el campo declara: el titular como «postulante»; el
// revisor y el jurado, si lo son de esa convocatoria.
func VeExtendido(c CampoExtendido, s *Sesion, p Postulacion) bool {
	if s == nil {
		return false
	}
	if PuedeSupervisar(s) {
		return true
	}
	for _, perfil := range c.VisibleEn {
		switch perfil {
		case PerfilPostulante:
			if s.Usuario.ID == p.UsuarioID {
				return true
			}
		case PerfilRevisorDocumental, PerfilJurado:
			if s.TieneAsignacion(p.ConvocatoriaID, perfil) {
				return true
			}
		}
	}
	return false
}

// ExtendidosVisibles devuelve, de los campos dinámicos que aplican a una
// postulación, los que la sesión puede ver, y borra de la postulación los
// valores de los que no. Se llama antes de mostrar una postulación a alguien:
// después de esto, lo que esa persona no debe ver no está.
func ExtendidosVisibles(p *Postulacion, tipo TipoConvocatoria, todos []CampoExtendido, s *Sesion) []CampoExtendido {
	dePostulacion, deObra := ExtendidosDePostulacion(todos, tipo, p.Subcategoria)
	var visibles []CampoExtendido
	for _, c := range dePostulacion {
		if VeExtendido(c, s, *p) {
			visibles = append(visibles, c)
		} else {
			delete(p.Campos, c.Ruta())
		}
	}
	for _, c := range deObra {
		if VeExtendido(c, s, *p) {
			visibles = append(visibles, c)
			continue
		}
		for i := range p.Obras {
			delete(p.Obras[i].Campos, c.Ruta())
		}
	}
	return visibles
}
