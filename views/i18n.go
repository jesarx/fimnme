// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package views

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"time"

	// Incrusta la base de zonas horarias en el binario: el ejecutable estático
	// no debe depender de que el servidor tenga instalado el paquete tzdata.
	_ "time/tzdata"

	"github.com/jesarx/fimnme/models"
)

// Idiomas admitidos, en orden de preferencia. El primero es el de respaldo.
var Idiomas = []string{models.IdiomaES, models.IdiomaEN}

// zonaMexico es la zona en que se MUESTRAN todas las fechas. En la base todo
// se guarda en UTC (RNF-15); la conversión ocurre sólo aquí, en la vista.
var zonaMexico = func() *time.Location {
	zona, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		// Imposible con time/tzdata incrustado; si ocurre, el binario está roto.
		panic("zona horaria America/Mexico_City no disponible: " + err.Error())
	}
	return zona
}()

// ZonaMexico devuelve la zona horaria de presentación. Los controladores la
// usan para interpretar las fechas que escribe el usuario (p. ej. en un
// filtro): «12 de noviembre» es ese día en la Ciudad de México, no en UTC.
func ZonaMexico() *time.Location {
	return zonaMexico
}

// Traducciones guarda los textos de la interfaz por idioma. Son mapas planos
// cargados de JSON incrustado: no amerita una biblioteca de i18n.
type Traducciones struct {
	porIdioma map[string]map[string]string
}

// cargarTraducciones lee i18n/es.json e i18n/en.json del contenido incrustado.
func cargarTraducciones(contenido fs.FS) (*Traducciones, error) {
	t := &Traducciones{porIdioma: make(map[string]map[string]string)}
	for _, idioma := range Idiomas {
		datos, err := fs.ReadFile(contenido, "i18n/"+idioma+".json")
		if err != nil {
			return nil, fmt.Errorf("traducciones %s: %w", idioma, err)
		}
		textos := make(map[string]string)
		if err := json.Unmarshal(datos, &textos); err != nil {
			return nil, fmt.Errorf("traducciones %s: %w", idioma, err)
		}
		t.porIdioma[idioma] = textos
	}
	return t, nil
}

// T devuelve el texto de una clave en el idioma pedido. Si falta se recurre al
// español, y si tampoco existe se devuelve la propia clave: un hueco visible
// en pantalla es preferible a una página rota. Las pruebas impiden que esto
// llegue a producción.
func (t *Traducciones) T(idioma, clave string, args ...any) string {
	texto, ok := t.porIdioma[idioma][clave]
	if !ok {
		texto, ok = t.porIdioma[models.IdiomaES][clave]
	}
	if !ok {
		return clave
	}
	if len(args) > 0 {
		return fmt.Sprintf(texto, args...)
	}
	return texto
}

// Claves devuelve el conjunto de claves de un idioma (para las pruebas).
func (t *Traducciones) Claves(idioma string) map[string]string {
	return t.porIdioma[idioma]
}

// Fecha formatea un día según la localidad ("29 de enero de 2027" o
// "January 29, 2027"), convertido a hora de la Ciudad de México.
func (t *Traducciones) Fecha(idioma string, momento time.Time) string {
	local := momento.In(zonaMexico)
	mes := t.T(idioma, fmt.Sprintf("fecha.mes.%d", int(local.Month())))
	return t.T(idioma, "fecha.formato", local.Day(), mes, local.Year())
}

// FechaHora formatea día y hora. La zona horaria va SIEMPRE explícita, porque
// hay postulantes en otros husos (RF-I18N-06).
func (t *Traducciones) FechaHora(idioma string, momento time.Time) string {
	local := momento.In(zonaMexico)
	hora := local.Format("15:04")
	if t.T(idioma, "fecha.reloj") == "12" {
		hora = local.Format("3:04 PM")
	}
	return t.T(idioma, "fecha.formato_hora", t.Fecha(idioma, momento), hora, t.T(idioma, "fecha.zona"))
}
