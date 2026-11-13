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
	"slices"
	"strings"
)

// Catálogos fijos del formulario de postulación (§5.3). Son estables entre
// ediciones, por lo que viven en el código y no en tablas. Lo que SÍ cambia
// cada año —las dotaciones— está en la base (tabla dotacion).

// codigosPais es la lista de códigos ISO 3166-1 alfa-2, tal como la publica la
// norma (249 códigos). Sólo se guardan los códigos: los nombres de los países,
// en español e inglés, son texto de interfaz y corresponden a la Vista.
var codigosPais = strings.Fields(`
	AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS
	BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE
	EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM
	HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC
	LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA
	NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW
	SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO
	TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW
`)

// PaisMexico es el código del país cuya entidad de residencia se elige de
// catálogo en lugar de escribirse.
const PaisMexico = "MX"

// CodigosDePais devuelve los códigos ISO 3166-1 alfa-2 admitidos, en orden
// alfabético.
func CodigosDePais() []string {
	return slices.Clone(codigosPais)
}

// PaisValido indica si el código es un país del catálogo ISO 3166-1.
func PaisValido(codigo string) bool {
	_, encontrado := slices.BinarySearch(codigosPais, codigo)
	return encontrado
}

// EntidadFederativa es una de las 32 entidades de la República.
type EntidadFederativa struct {
	Clave  string
	Nombre string
}

// EntidadCDMX es la clave de la Ciudad de México. De ella depende la regla de
// sede: un concierto propuesto en otra entidad debe indicar sede, domicilio,
// fecha y hora (RN-06).
const EntidadCDMX = "CDMX"

// entidadesFederativas es el catálogo de las 32 entidades. Las claves son las
// de la norma ISO 3166-2:MX, salvo la Ciudad de México, que lleva la clave
// «CDMX» que usa la especificación. Los nombres son nombres propios.
var entidadesFederativas = []EntidadFederativa{
	{"AGU", "Aguascalientes"}, {"BCN", "Baja California"}, {"BCS", "Baja California Sur"},
	{"CAM", "Campeche"}, {"CHP", "Chiapas"}, {"CHH", "Chihuahua"}, {EntidadCDMX, "Ciudad de México"},
	{"COA", "Coahuila"}, {"COL", "Colima"}, {"DUR", "Durango"}, {"GUA", "Guanajuato"},
	{"GRO", "Guerrero"}, {"HID", "Hidalgo"}, {"JAL", "Jalisco"}, {"MEX", "Estado de México"},
	{"MIC", "Michoacán"}, {"MOR", "Morelos"}, {"NAY", "Nayarit"}, {"NLE", "Nuevo León"},
	{"OAX", "Oaxaca"}, {"PUE", "Puebla"}, {"QUE", "Querétaro"}, {"ROO", "Quintana Roo"},
	{"SLP", "San Luis Potosí"}, {"SIN", "Sinaloa"}, {"SON", "Sonora"}, {"TAB", "Tabasco"},
	{"TAM", "Tamaulipas"}, {"TLA", "Tlaxcala"}, {"VER", "Veracruz"}, {"YUC", "Yucatán"},
	{"ZAC", "Zacatecas"},
}

// EntidadesFederativas devuelve el catálogo de las 32 entidades.
func EntidadesFederativas() []EntidadFederativa {
	return slices.Clone(entidadesFederativas)
}

// EntidadValida indica si la clave es una de las 32 entidades.
func EntidadValida(clave string) bool {
	return slices.ContainsFunc(entidadesFederativas, func(e EntidadFederativa) bool { return e.Clave == clave })
}

// OpcionesGenero son los valores del campo género (§5.3.1). Sus etiquetas son
// texto de interfaz.
var OpcionesGenero = []string{"mujer", "hombre", "no_binario", "otro", "prefiero_no_decir"}

// OpcionesEstreno son los valores del campo «¿es estreno?» de una obra.
var OpcionesEstreno = []string{"estreno_mundial", "estreno_mexico", "no_es_estreno"}
