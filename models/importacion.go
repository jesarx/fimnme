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
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Importación asistida con interacción humana (§5.15).
//
// Una importación ocurre en dos fases. En la primera, ANALYZE, el sistema lee
// la hoja de cálculo, valida su forma y levanta un «conflicto» por cada
// renglón cuyo valor no tiene una correspondencia unívoca en la base. No
// escribe nada en el dominio. Cada conflicto lo resuelve una persona:
// vincular a un registro que ya existe, crear uno nuevo u omitir el renglón.
// Sólo con todos resueltos se habilita la segunda fase, COMMIT, que aplica
// todo en una sola transacción: o entra todo o no entra nada (RN-29).
//
// El sistema nunca infiere que dos registros son la misma persona (RN-30):
// aunque el correo de un renglón coincida con el de una cuenta, quien decide
// que se trata de ella es una persona, y queda registrado.
//
// Una postulación nunca se importa (RN-31): este motor sólo conoce catálogos
// de dotaciones y cuentas de jurado y de revisor. No tiene cómo crear una
// postulación.
//
// Aquí vive lo que no es SQL: leer el archivo, analizarlo y las reglas.

// Tipos de importación.
const (
	ImportacionDotaciones = "dotaciones"       // catálogo de dotaciones de una convocatoria
	ImportacionJurados    = "usuarios_jurado"  // cuentas de jurado de una convocatoria
	ImportacionRevisores  = "usuarios_revisor" // cuentas de revisor de una convocatoria
)

// Estados de una importación.
const (
	ImportacionEsperando  = "esperando_resolucion"
	ImportacionConfirmada = "confirmada"
	ImportacionCancelada  = "cancelada"
)

// Resoluciones de un conflicto (RF-IMP-03).
const (
	ResolucionVincular  = "vincular"    // el renglón es un registro que ya existe
	ResolucionCrear     = "crear_nuevo" // el renglón es un registro nuevo
	ResolucionOmitir    = "omitir"      // el renglón no se importa
	columnaDeIdentidad  = "identidad"   // columna del conflicto en las importaciones de cuentas
	maxFilasImportacion = 2000
	// TamanoMaxImportacion es lo más que puede medir una hoja por importar.
	TamanoMaxImportacion = 2 << 20
	// tamanoMaxDescomprimido acota cada parte de un .xlsx al descomprimirla:
	// un archivo de unos KB puede declarar gigas.
	tamanoMaxDescomprimido = 16 << 20
)

// PerfilDeImportacion devuelve el perfil que otorga una importación de
// cuentas, o vacío si el tipo no es de cuentas.
func PerfilDeImportacion(tipo string) Perfil {
	switch tipo {
	case ImportacionJurados:
		return PerfilJurado
	case ImportacionRevisores:
		return PerfilRevisorDocumental
	}
	return ""
}

// columnasDeImportacion son las columnas que admite cada tipo; las primeras
// (cuántas, lo dice obligatorias) son obligatorias.
var columnasDeImportacion = map[string]struct {
	columnas     []string
	obligatorias int
}{
	ImportacionDotaciones: {[]string{"nombre_es", "nombre_en", "descripcion_es", "descripcion_en"}, 1},
	ImportacionJurados:    {[]string{"email", "nombre", "apellidos", "idioma"}, 3},
	ImportacionRevisores:  {[]string{"email", "nombre", "apellidos", "idioma"}, 3},
}

// ColumnasDeImportacion devuelve las columnas que admite un tipo de
// importación y cuántas de ellas, las primeras, son obligatorias.
func ColumnasDeImportacion(tipo string) (columnas []string, obligatorias int) {
	definicion := columnasDeImportacion[tipo]
	return definicion.columnas, definicion.obligatorias
}

// HojaDeCalculo es el contenido de la primera hoja de un archivo: sus
// renglones, cada uno con sus celdas como texto.
type HojaDeCalculo [][]string

// LeerHoja lee un .csv o un .xlsx. El formato se reconoce por el contenido,
// no por el nombre: un .xlsx es un archivo ZIP.
func LeerHoja(contenido []byte) (HojaDeCalculo, error) {
	if len(contenido) > TamanoMaxImportacion {
		return nil, errors.New("el archivo es demasiado grande")
	}
	if bytes.HasPrefix(contenido, []byte("PK\x03\x04")) {
		return leerXLSX(contenido)
	}
	return leerCSV(contenido)
}

// leerCSV lee un CSV en UTF-8, con o sin marca inicial, separado por comas o
// por punto y coma (que es como lo guarda Excel en español).
func leerCSV(contenido []byte) (HojaDeCalculo, error) {
	contenido = bytes.TrimPrefix(contenido, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(contenido) {
		return nil, errors.New("el archivo no está en UTF-8: guárdelo como «CSV UTF-8»")
	}
	lector := csv.NewReader(bytes.NewReader(contenido))
	lector.FieldsPerRecord = -1 // los renglones pueden traer distinto número de celdas
	primera, _, _ := bytes.Cut(contenido, []byte("\n"))
	if bytes.Count(primera, []byte(";")) > bytes.Count(primera, []byte(",")) {
		lector.Comma = ';'
	}
	renglones, err := lector.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("el CSV no pudo leerse: %w", err)
	}
	return renglones, nil
}

// abrirParteDeXLSX lee, con tope de tamaño, un archivo de dentro del .xlsx.
func abrirParteDeXLSX(archivo *zip.File) ([]byte, error) {
	lector, err := archivo.Open()
	if err != nil {
		return nil, err
	}
	defer lector.Close()
	datos, err := io.ReadAll(io.LimitReader(lector, tamanoMaxDescomprimido+1))
	if err != nil {
		return nil, err
	}
	if len(datos) > tamanoMaxDescomprimido {
		return nil, errors.New("la hoja es demasiado grande")
	}
	return datos, nil
}

// textoXLSX es un texto dentro de un .xlsx: directo (<t>) o partido en
// tramos con formato (<r><t>).
type textoXLSX struct {
	T []string `xml:"t"`
	R []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

// String une los tramos del texto.
func (t textoXLSX) String() string {
	var texto strings.Builder
	for _, tramo := range t.T {
		texto.WriteString(tramo)
	}
	for _, tramo := range t.R {
		texto.WriteString(tramo.T)
	}
	return texto.String()
}

// leerXLSX lee la primera hoja de un libro de Excel. Un .xlsx es un ZIP con
// archivos XML: la hoja (xl/worksheets/sheet1.xml) y, aparte, la tabla de
// textos compartidos (xl/sharedStrings.xml), a la que las celdas de texto se
// refieren por número. No hace falta más para leer una hoja de datos: ni
// estilos, ni fórmulas —de una celda con fórmula se toma el valor que Excel
// dejó calculado—.
func leerXLSX(contenido []byte) (HojaDeCalculo, error) {
	libro, err := zip.NewReader(bytes.NewReader(contenido), int64(len(contenido)))
	if err != nil {
		return nil, fmt.Errorf("el archivo no es un .xlsx válido: %w", err)
	}
	var hoja, textos *zip.File
	for _, archivo := range libro.File {
		switch {
		case archivo.Name == "xl/sharedStrings.xml":
			textos = archivo
		case strings.HasPrefix(archivo.Name, "xl/worksheets/") && strings.HasSuffix(archivo.Name, ".xml") &&
			(hoja == nil || archivo.Name == "xl/worksheets/sheet1.xml" || (hoja.Name != "xl/worksheets/sheet1.xml" && archivo.Name < hoja.Name)):
			hoja = archivo
		}
	}
	if hoja == nil {
		return nil, errors.New("el archivo .xlsx no tiene ninguna hoja")
	}
	var compartidos []string
	if textos != nil {
		datos, err := abrirParteDeXLSX(textos)
		if err != nil {
			return nil, fmt.Errorf("leer los textos del .xlsx: %w", err)
		}
		var tabla struct {
			SI []textoXLSX `xml:"si"`
		}
		if err := xml.Unmarshal(datos, &tabla); err != nil {
			return nil, fmt.Errorf("leer los textos del .xlsx: %w", err)
		}
		for _, texto := range tabla.SI {
			compartidos = append(compartidos, texto.String())
		}
	}
	datos, err := abrirParteDeXLSX(hoja)
	if err != nil {
		return nil, fmt.Errorf("leer la hoja del .xlsx: %w", err)
	}
	var estructura struct {
		Filas []struct {
			Celdas []struct {
				R  string    `xml:"r,attr"` // referencia: «B7»
				T  string    `xml:"t,attr"` // tipo: s = texto compartido, inlineStr = texto en la celda
				V  string    `xml:"v"`
				IS textoXLSX `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(datos, &estructura); err != nil {
		return nil, fmt.Errorf("leer la hoja del .xlsx: %w", err)
	}
	var renglones HojaDeCalculo
	for _, fila := range estructura.Filas {
		var renglon []string
		for i, celda := range fila.Celdas {
			// Excel omite las celdas vacías: la columna sale de la referencia.
			columna := i
			if letras := strings.TrimRight(celda.R, "0123456789"); letras != "" {
				columna = 0
				for _, letra := range letras {
					if letra < 'A' || letra > 'Z' {
						return nil, fmt.Errorf("referencia de celda inválida en el .xlsx: %q", celda.R)
					}
					columna = columna*26 + int(letra-'A') + 1
				}
				columna--
			}
			if columna > 255 {
				return nil, errors.New("la hoja tiene demasiadas columnas")
			}
			for len(renglon) <= columna {
				renglon = append(renglon, "")
			}
			switch celda.T {
			case "s":
				indice, err := strconv.Atoi(strings.TrimSpace(celda.V))
				if err != nil || indice < 0 || indice >= len(compartidos) {
					return nil, errors.New("el .xlsx hace referencia a un texto que no existe")
				}
				renglon[columna] = compartidos[indice]
			case "inlineStr":
				renglon[columna] = celda.IS.String()
			default:
				renglon[columna] = celda.V
			}
		}
		renglones = append(renglones, renglon)
	}
	return renglones, nil
}

// FilaDeImportacion es un renglón de datos ya validado: su número en el
// archivo (el encabezado es el 1) y sus valores por nombre de columna.
type FilaDeImportacion struct {
	Numero  int
	Valores map[string]string
}

// compararSinDetalles reduce un texto a lo que importa para compararlo: sin
// mayúsculas, sin acentos y sin espacios de más.
func compararSinDetalles(texto string) string {
	return strings.Join(strings.Fields(strings.ToLower(sinAcentos.Replace(texto))), " ")
}

// FilasDeImportacion valida la forma de una hoja para un tipo de importación
// y devuelve sus renglones de datos. Los errores dicen el renglón («fila 7»)
// y qué tiene. Exige:
//
//   - un primer renglón de encabezados, con los nombres de columna exactos;
//     todas las obligatorias, y ninguna que no se conozca;
//   - en cada renglón, los datos obligatorios y con el formato de su tipo;
//   - que ningún renglón repita a otro (el mismo nombre de dotación, el
//     mismo correo).
//
// Los renglones vacíos se ignoran.
func FilasDeImportacion(tipo string, hoja HojaDeCalculo) ([]FilaDeImportacion, []ErrorCampo) {
	columnas, obligatorias := ColumnasDeImportacion(tipo)
	if len(columnas) == 0 {
		return nil, []ErrorCampo{{Campo: "tipo", Clave: "error.tipo_de_importacion"}}
	}
	if len(hoja) == 0 {
		return nil, []ErrorCampo{{Campo: "archivo", Clave: "error.hoja_vacia"}}
	}
	// Encabezados: dónde está cada columna.
	posicion := make(map[string]int)
	for i, encabezado := range hoja[0] {
		nombre := strings.ToLower(strings.TrimSpace(encabezado))
		if nombre == "" {
			continue
		}
		if _, repetido := posicion[nombre]; repetido || !slices.Contains(columnas, nombre) {
			return nil, []ErrorCampo{{Campo: "fila 1", Clave: "error.columnas_de_importacion"}}
		}
		posicion[nombre] = i
	}
	for _, obligatoria := range columnas[:obligatorias] {
		if _, presente := posicion[obligatoria]; !presente {
			return nil, []ErrorCampo{{Campo: "fila 1", Clave: "error.columnas_de_importacion"}}
		}
	}

	var filas []FilaDeImportacion
	var errores []ErrorCampo
	vistos := make(map[string]int)
	for i, renglon := range hoja[1:] {
		fila := FilaDeImportacion{Numero: i + 2, Valores: make(map[string]string)}
		vacio := true
		for nombre, columna := range posicion {
			if columna < len(renglon) {
				fila.Valores[nombre] = strings.TrimSpace(renglon[columna])
				vacio = vacio && fila.Valores[nombre] == ""
			}
		}
		if vacio {
			continue
		}
		lugar := "fila " + strconv.Itoa(fila.Numero)
		falla := func(clave string) { errores = append(errores, ErrorCampo{Campo: lugar, Clave: clave}) }
		llave := ""
		if tipo == ImportacionDotaciones {
			llave = compararSinDetalles(fila.Valores["nombre_es"])
			for _, columna := range columnas {
				// Los nombres, como en el resto del sistema; las
				// descripciones admiten un párrafo.
				limite := longitudMaxTexto
				if strings.HasPrefix(columna, "descripcion") {
					limite = 10 * longitudMaxTexto
				}
				if utf8.RuneCountInString(fila.Valores[columna]) > limite {
					falla("error.texto_largo_campo")
				}
			}
			if llave == "" {
				falla("error.fila_incompleta")
			}
		} else {
			fila.Valores["email"] = NormalizarEmail(fila.Valores["email"])
			if fila.Valores["idioma"] == "" {
				fila.Valores["idioma"] = IdiomaES
			}
			// Un correo es el mismo sin importar mayúsculas (así lo compara
			// también la base).
			llave = strings.ToLower(fila.Valores["email"])
			cuenta := NuevoUsuario{Email: fila.Valores["email"], Nombre: fila.Valores["nombre"], Apellidos: fila.Valores["apellidos"], Idioma: fila.Valores["idioma"]}
			if len(ValidarCuentaInterna(cuenta)) > 0 {
				falla("error.fila_incompleta")
			}
		}
		if _, repetida := vistos[llave]; repetida && llave != "" {
			falla("error.fila_repetida")
		}
		vistos[llave] = fila.Numero
		filas = append(filas, fila)
	}
	switch {
	case len(errores) > 0:
		return nil, errores
	case len(filas) == 0:
		return nil, []ErrorCampo{{Campo: "archivo", Clave: "error.hoja_vacia"}}
	case len(filas) > maxFilasImportacion:
		return nil, []ErrorCampo{{Campo: "archivo", Clave: "error.demasiadas_filas"}}
	}
	return filas, nil
}

// Candidato es un registro existente que podría ser el de un renglón.
type Candidato struct {
	ID          int64  `json:"id"`
	Descripcion string `json:"descripcion"`
	// Motivo dice por qué se propone: «mismo_correo», «mismo_nombre» o
	// «nombre_parecido».
	Motivo string `json:"motivo"`
}

// ConflictoDeImportacion es un renglón que una persona debe resolver.
type ConflictoDeImportacion struct {
	ID          int64
	Fila        int
	Columna     string
	ValorOrigen string
	Candidatos  []Candidato
	Resolucion  string // vacía mientras no se resuelve
	EntidadID   int64  // el registro elegido, si se vincula
	ResueltoPor int64
	ResueltoAt  time.Time
}

// Importacion es una importación asistida con sus conflictos.
type Importacion struct {
	ID             int64
	Tipo           string
	ConvocatoriaID int64
	UsuarioID      int64 // quien la inició
	Estado         string
	FilasTotal     int
	Archivo        Archivo // la hoja original, que se conserva (RF-IMP-09)
	CreadoAt       time.Time
	ConfirmadaAt   time.Time
	Conflictos     []ConflictoDeImportacion
}

// EfectoDeImportacion es lo que hará —o hizo— el Commit (RF-IMP-08).
type EfectoDeImportacion struct {
	Crean       int // renglones que crean un registro
	Vinculan    int // renglones que se vinculan a uno existente
	Omiten      int
	SinResolver int // conflictos que aún esperan a una persona
	// SinConflicto son los renglones con correspondencia unívoca: no
	// necesitan decisión y no crean nada.
	SinConflicto int
}

// Efecto calcula lo que hará el Commit de una importación.
func (i Importacion) Efecto() EfectoDeImportacion {
	efecto := EfectoDeImportacion{SinConflicto: i.FilasTotal - len(i.Conflictos)}
	for _, conflicto := range i.Conflictos {
		switch conflicto.Resolucion {
		case ResolucionCrear:
			efecto.Crean++
		case ResolucionVincular:
			efecto.Vinculan++
		case ResolucionOmitir:
			efecto.Omiten++
		default:
			efecto.SinResolver++
		}
	}
	return efecto
}

// PuedeConfirmarse indica si el Commit está habilitado: la importación
// espera resolución y no le queda ningún conflicto por resolver (RF-IMP-04).
func (i Importacion) PuedeConfirmarse() bool {
	return i.Estado == ImportacionEsperando && i.Efecto().SinResolver == 0
}

// AnalizarDotaciones compara cada renglón con las dotaciones que la
// convocatoria ya tiene. Sólo un nombre IDÉNTICO a uno existente es una
// correspondencia unívoca y no genera conflicto. Todo lo demás —un nombre
// que no existe, o que se parece a uno que sí— lo decide una persona; los
// parecidos se le ofrecen como candidatos.
func AnalizarDotaciones(filas []FilaDeImportacion, existentes []Dotacion) []ConflictoDeImportacion {
	var conflictos []ConflictoDeImportacion
	for _, fila := range filas {
		nombre := fila.Valores["nombre_es"]
		clave := compararSinDetalles(nombre)
		identica := false
		var candidatos []Candidato
		for _, existente := range existentes {
			otra := compararSinDetalles(existente.NombreES)
			switch {
			case existente.NombreES == nombre:
				identica = true
			case otra == clave:
				candidatos = append(candidatos, Candidato{ID: existente.ID, Descripcion: existente.NombreES, Motivo: "mismo_nombre"})
			case len(clave) >= 4 && len(otra) >= 4 && (strings.Contains(otra, clave) || strings.Contains(clave, otra)):
				candidatos = append(candidatos, Candidato{ID: existente.ID, Descripcion: existente.NombreES, Motivo: "nombre_parecido"})
			}
		}
		if identica {
			continue
		}
		conflictos = append(conflictos, ConflictoDeImportacion{Fila: fila.Numero, Columna: "nombre_es", ValorOrigen: nombre, Candidatos: candidatos})
	}
	return conflictos
}

// AnalizarCuentas compara cada renglón con las cuentas que ya existen. TODO
// renglón genera un conflicto: decidir que una cuenta existente es la
// persona del renglón —aunque coincida el correo— es una vinculación de
// identidad, y eso sólo lo hace una persona (RN-30). Se ofrecen como
// candidatas la cuenta con ese correo y las que llevan el mismo nombre.
func AnalizarCuentas(filas []FilaDeImportacion, cuentas []Usuario) []ConflictoDeImportacion {
	conflictos := make([]ConflictoDeImportacion, 0, len(filas))
	for _, fila := range filas {
		nombre := compararSinDetalles(fila.Valores["nombre"] + " " + fila.Valores["apellidos"])
		var candidatos []Candidato
		for _, cuenta := range cuentas {
			descripcion := strings.TrimSpace(cuenta.Nombre+" "+cuenta.Apellidos) + " <" + cuenta.Email + ">"
			switch {
			case strings.EqualFold(cuenta.Email, fila.Valores["email"]):
				// La del mismo correo va primero.
				candidatos = append([]Candidato{{ID: cuenta.ID, Descripcion: descripcion, Motivo: "mismo_correo"}}, candidatos...)
			case compararSinDetalles(cuenta.Nombre+" "+cuenta.Apellidos) == nombre:
				candidatos = append(candidatos, Candidato{ID: cuenta.ID, Descripcion: descripcion, Motivo: "mismo_nombre"})
			}
		}
		conflictos = append(conflictos, ConflictoDeImportacion{Fila: fila.Numero, Columna: columnaDeIdentidad,
			ValorOrigen: strings.TrimSpace(fila.Valores["nombre"]+" "+fila.Valores["apellidos"]) + " <" + fila.Valores["email"] + ">", Candidatos: candidatos})
	}
	return conflictos
}

// ValidarResolucion valida la resolución que una persona da a un conflicto:
//
//   - «vincular» exige elegir un registro. En una importación de cuentas sólo
//     puede ser uno de los candidatos: vincular a una cuenta cualquiera por
//     su número sería la forma más fácil de equivocarse de persona.
//   - «crear_nuevo» no cabe si ya hay una cuenta con ese correo: no puede
//     haber dos.
func ValidarResolucion(tipo string, conflicto ConflictoDeImportacion, resolucion string, entidadID int64) []ErrorCampo {
	switch resolucion {
	case ResolucionOmitir:
		return nil
	case ResolucionCrear:
		if PerfilDeImportacion(tipo) != "" && slices.ContainsFunc(conflicto.Candidatos, func(c Candidato) bool { return c.Motivo == "mismo_correo" }) {
			return []ErrorCampo{{Campo: "resolucion", Clave: "error.correo_ya_registrado"}}
		}
		return nil
	case ResolucionVincular:
		esCandidato := slices.ContainsFunc(conflicto.Candidatos, func(c Candidato) bool { return c.ID == entidadID })
		if entidadID <= 0 || (PerfilDeImportacion(tipo) != "" && !esCandidato) {
			return []ErrorCampo{{Campo: "entidad_id", Clave: "error.candidato_requerido"}}
		}
		return nil
	}
	return []ErrorCampo{{Campo: "resolucion", Clave: "error.opcion_invalida"}}
}

// ValidarNuevaImportacion valida el destino de una importación: las
// dotaciones son de Compositores, y nada se importa a una convocatoria ya
// cerrada.
func ValidarNuevaImportacion(tipo string, c Convocatoria) []ErrorCampo {
	if _, conocido := columnasDeImportacion[tipo]; !conocido {
		return []ErrorCampo{{Campo: "tipo", Clave: "error.tipo_de_importacion"}}
	}
	if errores := ValidarEdicionDeConfiguracion(c.Estado); len(errores) > 0 {
		return errores
	}
	if tipo == ImportacionDotaciones && c.Tipo != TipoCompositores {
		return []ErrorCampo{{Campo: "tipo", Clave: "error.dotacion_solo_compositores"}}
	}
	return nil
}

// ActivacionDeCuenta es lo que el Commit necesita para cada cuenta nueva: el
// enlace de activación, de un solo uso, y —si quien importa puede enviar
// correo— el mensaje que lo lleva.
type ActivacionDeCuenta struct {
	Token  NuevoToken
	Correo *CorreoSaliente
}

// ErrHojaIlegible indica que el archivo no pudo leerse como hoja de cálculo.
var ErrHojaIlegible = errors.New("el archivo no pudo leerse como hoja de cálculo")

// mimeDeHoja dice el tipo de una hoja por su contenido.
func mimeDeHoja(contenido []byte) string {
	if bytes.HasPrefix(contenido, []byte("PK\x03\x04")) {
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return "text/csv"
}

// AnalizarImportacion ejecuta la fase Analyze completa (RF-IMP-02): lee la
// hoja, valida su forma, busca en la base las correspondencias de cada
// renglón, guarda el archivo original y registra la importación con sus
// conflictos. No escribe nada en el dominio. Es el mismo motor para la
// interfaz web y para fimnme-cli (RF-IMP-05).
//
// Si la hoja no puede leerse devuelve ErrHojaIlegible (envuelto, con el
// detalle); si su forma no es la esperada, un *ErrorValidacion que dice en
// qué renglón.
func AnalizarImportacion(ctx context.Context, datos Persistencia, almacen BlobStore, actor Actor, tipo string, convocatoriaID int64,
	nombreOriginal string, contenido []byte) (int64, error) {
	cfg, err := datos.ConfiguracionDeConvocatoria(ctx, convocatoriaID)
	if err != nil {
		return 0, err
	}
	if errores := ValidarNuevaImportacion(tipo, cfg.Convocatoria); len(errores) > 0 {
		return 0, &ErrorValidacion{Errores: errores}
	}
	hoja, err := LeerHoja(contenido)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrHojaIlegible, err)
	}
	filas, errores := FilasDeImportacion(tipo, hoja)
	if len(errores) > 0 {
		return 0, &ErrorValidacion{Errores: errores}
	}
	var conflictos []ConflictoDeImportacion
	if tipo == ImportacionDotaciones {
		conflictos = AnalizarDotaciones(filas, cfg.Dotaciones)
	} else {
		cuentas, err := datos.CuentasParaConciliar(ctx)
		if err != nil {
			return 0, err
		}
		conflictos = AnalizarCuentas(filas, cuentas)
	}
	blob, err := almacen.Guardar(bytes.NewReader(contenido), TamanoMaxImportacion)
	if err != nil {
		return 0, err
	}
	return datos.CrearImportacion(ctx, actor, tipo, convocatoriaID, blob, mimeDeHoja(contenido), nombreOriginal, len(filas), conflictos)
}

// FilasDeLaImportacion vuelve a leer, del archivo original que se conservó,
// los renglones de una importación: son los que aplica el Commit. El archivo
// es el mismo que se analizó —está direccionado por su SHA-256—, así que los
// renglones son los mismos.
func FilasDeLaImportacion(almacen BlobStore, i Importacion) ([]FilaDeImportacion, error) {
	archivo, err := almacen.Abrir(i.Archivo.Clave)
	if err != nil {
		return nil, fmt.Errorf("abrir el archivo de la importación: %w", err)
	}
	defer archivo.Close()
	contenido, err := io.ReadAll(io.LimitReader(archivo, TamanoMaxImportacion+1))
	if err != nil {
		return nil, fmt.Errorf("leer el archivo de la importación: %w", err)
	}
	hoja, err := LeerHoja(contenido)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHojaIlegible, err)
	}
	filas, errores := FilasDeImportacion(i.Tipo, hoja)
	if len(errores) > 0 {
		return nil, &ErrorValidacion{Errores: errores}
	}
	return filas, nil
}

// CuentasPorCrear devuelve, de una importación de cuentas, los renglones que
// el Commit convertirá en cuentas nuevas: son los que necesitan un enlace de
// activación.
func CuentasPorCrear(i Importacion, filas []FilaDeImportacion) []FilaDeImportacion {
	if PerfilDeImportacion(i.Tipo) == "" {
		return nil
	}
	var nuevas []FilaDeImportacion
	for _, fila := range filas {
		if slices.ContainsFunc(i.Conflictos, func(c ConflictoDeImportacion) bool { return c.Fila == fila.Numero && c.Resolucion == ResolucionCrear }) {
			nuevas = append(nuevas, fila)
		}
	}
	return nuevas
}
