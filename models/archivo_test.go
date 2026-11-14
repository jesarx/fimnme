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
	"fmt"
	"strings"
	"testing"
	"time"
)

// Pruebas de las reglas del archivo histórico: alcance por perfil, filtros,
// búsqueda, nomenclatura de las descargas masivas y vistas guardadas.

// El alcance sale de la sesión y no depende de la etapa: por eso no caduca.
func TestAlcanceDeConsulta(t *testing.T) {
	sesion := func(perfiles []Perfil, asignaciones ...Asignacion) *Sesion {
		return &Sesion{Perfiles: perfiles, Asignaciones: asignaciones}
	}
	for nombre, caso := range map[string]struct {
		sesion   *Sesion
		esperado string // Todo / revisor / jurado
		enMasa   bool
	}{
		"sin sesión":    {nil, "false [] []", false},
		"postulante":    {sesion([]Perfil{PerfilPostulante}), "false [] []", false},
		"administrador": {sesion([]Perfil{PerfilAdminSistema}), "true [] []", true},
		"observador":    {sesion([]Perfil{PerfilCoordinacionNacional}), "true [] []", true},
		"revisor":       {sesion(nil, Asignacion{ConvocatoriaID: 7, Perfil: PerfilRevisorDocumental}), "false [7] []", true},
		"jurado":        {sesion(nil, Asignacion{ConvocatoriaID: 7, Perfil: PerfilJurado}), "false [] [7]", false},
		"revisor de una y jurado de otra": {sesion(nil, Asignacion{ConvocatoriaID: 7, Perfil: PerfilRevisorDocumental},
			Asignacion{ConvocatoriaID: 9, Perfil: PerfilJurado}), "false [7] [9]", true},
	} {
		alcance := AlcanceDe(caso.sesion)
		if obtenido := fmt.Sprint(alcance.Todo, alcance.ComoRevisor, alcance.ComoJurado); obtenido != caso.esperado {
			t.Errorf("%s: alcance %s, se esperaba %s", nombre, obtenido, caso.esperado)
		}
		if alcance.Vacio() != (caso.esperado == "false [] []") {
			t.Errorf("%s: Vacio() = %v", nombre, alcance.Vacio())
		}
		if PuedeDescargarEnMasa(caso.sesion) != caso.enMasa {
			t.Errorf("%s: descarga en masa = %v", nombre, !caso.enMasa)
		}
	}
}

func TestValidarFiltroDeListado(t *testing.T) {
	// Lo que falta se fija: orden por folio y página de 50.
	f := FiltroDeListado{Pagina: 1}
	if errores := ValidarFiltroDeListado(&f); len(errores) != 0 || f.Orden != "folio" || f.Tamano != TamanoDePaginaPorOmision {
		t.Errorf("valores por omisión: %v %+v", errores, f)
	}
	// Sin página no se pagina: es el resultado completo, para CSV y ZIP.
	f = FiltroDeListado{}
	if errores := ValidarFiltroDeListado(&f); len(errores) != 0 || f.Tamano != 0 {
		t.Errorf("sin paginar: %v %+v", errores, f)
	}
	dia := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	for nombre, caso := range map[string]struct {
		filtro FiltroDeListado
		campo  string
	}{
		"orden por puntaje":   {FiltroDeListado{Orden: "puntaje"}, "orden"},
		"orden con SQL":       {FiltroDeListado{Orden: "folio; DROP TABLE postulacion"}, "orden"},
		"página negativa":     {FiltroDeListado{Pagina: -1}, "pagina"},
		"página enorme":       {FiltroDeListado{Pagina: 1, Tamano: TamanoDePaginaMax + 1}, "tamano"},
		"tipo desconocido":    {FiltroDeListado{Tipo: "coro"}, "tipo"},
		"sí o no":             {FiltroDeListado{ConSubsanaciones: "tal vez"}, "con_subsanaciones"},
		"fechas al revés":     {FiltroDeListado{EnviadaDesde: dia, EnviadaHasta: dia.Add(-time.Hour)}, "enviada_hasta"},
		"selección desmedida": {FiltroDeListado{IDs: make([]int64, MaxSeleccion+1)}, "ids"},
	} {
		errores := ValidarFiltroDeListado(&caso.filtro)
		if len(errores) != 1 || errores[0].Campo != caso.campo || errores[0].Clave != "error.filtro_invalido" {
			t.Errorf("%s: %v", nombre, errores)
		}
	}
}

// Lo que se escribe en el buscador nunca se interpreta como su sintaxis.
func TestConsultasDeBusqueda(t *testing.T) {
	for texto, esperado := range map[string][2]string{
		"Núñez Carr":         {`"núñez"* "carr"*`, `"núñez" "carr"`},
		"  FIM49-INT-0137 ":  {`"fim49"* "int"* "0137"*`, `"fim49" "int" "0137"`},
		"ana@ejemplo.mx":     {`"ana"* "ejemplo"* "mx"*`, `"ana" "ejemplo"`}, // «mx» es corto para buscarse como fragmento
		`"x" OR NEAR(a b) *`: {`"x"* "or"* "near"* "a"* "b"*`, `"near"`},
		"":                   {"", ""},
		` "' ( ) * : `:       {"", ""},
	} {
		porPalabras, porSubcadena := ConsultasDeBusqueda(texto)
		if porPalabras != esperado[0] || porSubcadena != esperado[1] {
			t.Errorf("%q: %s | %s", texto, porPalabras, porSubcadena)
		}
	}
}

// El índice lleva lo que RF-ARCH-10 pide buscar, y nada más: ni semblanzas,
// ni teléfonos, ni fechas de nacimiento.
func TestTextoDeBusqueda(t *testing.T) {
	p := Postulacion{Folio: "FIM49-INT-0137", Campos: map[string]string{
		"datos.nombre_ensamble": "Ónix Ensamble", "datos.representante": "Rita Paz", "datos.email_contacto": "onix@ejemplo.mx",
		"datos.telefono": "55 1234 5678", "datos.semblanza": "Fundado en 1996.", "datos.fecha_nacimiento": "1990-05-17",
	},
		Integrantes: []Integrante{{Nombre: "Abel Cruz", Instrumento: "flauta"}},
		Obras:       []Obra{{Campos: map[string]string{"obra.titulo": "Preludio a Colón", "obra.compositor": "Julián Carrillo", "obra.nota_obra": "Nota."}}},
	}
	texto := TextoDeBusqueda(p, Usuario{Nombre: "Rita", Apellidos: "Paz", Email: "rita@ejemplo.mx"})
	esperado := "FIM49-INT-0137 Rita Paz rita@ejemplo.mx Ónix Ensamble Rita Paz onix@ejemplo.mx Abel Cruz Preludio a Colón Julián Carrillo"
	if texto != esperado {
		t.Errorf("texto indexado:\n%s\nse esperaba:\n%s", texto, esperado)
	}
}

// Nombres predecibles y válidos en cualquier sistema de archivos.
func TestNomenclaturaDelZIP(t *testing.T) {
	for texto, esperado := range map[string]string{
		"María Núñez":           "Maria_Nunez",
		"  Cuarteto n.º 2  ":    "Cuarteto_n_2",
		`a/b\c:d*e?f"g<h>i|j`:   "a_b_c_d_e_f_g_h_i_j",
		"../../etc/passwd":      "etc_passwd",
		"":                      "sin_nombre",
		"***":                   "sin_nombre",
		strings.Repeat("ñ", 80): strings.Repeat("n", 60),
	} {
		if obtenido := NombreParaArchivo(texto); obtenido != esperado {
			t.Errorf("%q: %q, se esperaba %q", texto, obtenido, esperado)
		}
	}
	documento := func(clave, mime string, obraID int64) DocumentoEnZIP {
		return DocumentoEnZIP{Folio: "FIM49-COM-0007", Nombre: "María Núñez", OrdenDeObra: 2, Obra: "Rapsodia / II",
			Documento: Documento{Clave: clave, ObraID: obraID, Archivo: Archivo{MIME: mime}}}
	}
	for esperado, obtenido := range map[string]string{
		"FIM49-COM-0007_Maria_Nunez/fotografia.jpg":                    RutaEnZIP(documento("fotografia", "image/jpeg", 0), 1),
		"FIM49-COM-0007_Maria_Nunez/obra-02_Rapsodia_II/partitura.pdf": RutaEnZIP(documento("partitura", "application/pdf", 31), 1),
		"FIM49-COM-0007_Maria_Nunez/obra-02_Rapsodia_II/audio-3.mp3":   RutaEnZIP(documento("audio", "audio/mpeg", 31), 3),
	} {
		if obtenido != esperado {
			t.Errorf("ruta en el ZIP: %q, se esperaba %q", obtenido, esperado)
		}
	}
}

// Una vista guardada sólo puede llevar a una dirección de este sitio.
func TestValidarVistaGuardada(t *testing.T) {
	if errores := ValidarVistaGuardada("Por validar", "/archivo?estado=enviada", 0); len(errores) != 0 {
		t.Errorf("vista válida: %v", errores)
	}
	for nombre, caso := range map[string]struct {
		nombre, ruta string
		existentes   int
		clave        string
	}{
		"sin nombre":              {"  ", "/archivo", 0, "error.nombre_de_vista"},
		"nombre larguísimo":       {strings.Repeat("a", 101), "/archivo", 0, "error.nombre_de_vista"},
		"otro sitio":              {"x", "https://malo.example/archivo", 0, "error.ruta_de_vista"},
		"otro sitio, sin esquema": {"x", "//malo.example/archivo", 0, "error.ruta_de_vista"},
		"barra invertida":         {"x", `/\malo.example`, 0, "error.ruta_de_vista"},
		"salto de línea":          {"x", "/archivo\r\nLocation: x", 0, "error.ruta_de_vista"},
		"demasiadas":              {"x", "/archivo", MaxVistasGuardadas, "error.demasiadas_vistas"},
	} {
		errores := ValidarVistaGuardada(caso.nombre, caso.ruta, caso.existentes)
		if len(errores) != 1 || errores[0].Clave != caso.clave {
			t.Errorf("%s: %v", nombre, errores)
		}
	}
}
