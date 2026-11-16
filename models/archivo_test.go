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

// El recorte del expediente depende sólo de quién lo pide y de la
// postulación; y RecortarExpediente quita todo lo que el recorte no permite,
// aunque se haya cargado.
func TestRecorteDelExpediente(t *testing.T) {
	p := Postulacion{ID: 5, ConvocatoriaID: 7, UsuarioID: 100, Estado: PostulacionEnEvaluacion}
	sesion := func(usuarioID int64, perfiles []Perfil, asignaciones ...Asignacion) *Sesion {
		return &Sesion{Usuario: Usuario{ID: usuarioID}, Perfiles: perfiles, Asignaciones: asignaciones}
	}
	for nombre, caso := range map[string]struct {
		sesion   *Sesion
		esperado string // puede, revisión, evaluación propia, supervisión
	}{
		"sin sesión":      {nil, "false false false false"},
		"titular":         {sesion(100, []Perfil{PerfilPostulante}), "true false false false"},
		"otro postulante": {sesion(101, []Perfil{PerfilPostulante}), "false false false false"},
		"administrador":   {sesion(1, []Perfil{PerfilAdminSistema}), "true true false true"},
		"observador":      {sesion(2, []Perfil{PerfilCoordinacionNacional}), "true true false true"},
		"revisor":         {sesion(3, nil, Asignacion{ConvocatoriaID: 7, Perfil: PerfilRevisorDocumental}), "true true false false"},
		"jurado":          {sesion(4, nil, Asignacion{ConvocatoriaID: 7, Perfil: PerfilJurado}), "true false true false"},
		"jurado de otra":  {sesion(4, nil, Asignacion{ConvocatoriaID: 8, Perfil: PerfilJurado}), "false false false false"},
		"revisor de otra": {sesion(3, nil, Asignacion{ConvocatoriaID: 8, Perfil: PerfilRevisorDocumental}), "false false false false"},
	} {
		recorte, puede := RecorteDelExpediente(caso.sesion, p)
		if obtenido := fmt.Sprint(puede, recorte.Revision, recorte.EvaluacionPropia, recorte.Supervision); obtenido != caso.esperado {
			t.Errorf("%s: %s, se esperaba %s", nombre, obtenido, caso.esperado)
		}
	}
	// El jurado no obtiene el expediente de lo que no pasó a evaluación.
	p.Estado = PostulacionValidada
	if _, puede := RecorteDelExpediente(sesion(4, nil, Asignacion{ConvocatoriaID: 7, Perfil: PerfilJurado}), p); puede {
		t.Error("el jurado no ve una postulación que no está en evaluación")
	}

	// completo es un expediente cargado con todo, como si quien lo armó se
	// hubiera equivocado.
	completo := func(recorte RecorteDeExpediente) Expediente {
		return Expediente{
			Postulacion: Postulacion{Estado: PostulacionDescartada, MotivoDescarte: "No cumple.", DictamenFinal: DictamenSeleccionada,
				Subsanaciones: []Subsanacion{{ID: 1}}, Documentos: []Documento{{ID: 9, Dictamen: "invalido"}}},
			Recorte:    recorte,
			Revision:   Revision{Observaciones: []ObservacionRevision{{Texto: "interna"}}, Dictamenes: []DictamenDocumento{{DocumentoID: 9, Motivo: "ilegible"}}},
			Propias:    []Evaluacion{{Observaciones: "propia"}},
			Jurados:    []JuradoEnMatriz{{ID: 4}},
			Resultados: []FilaDeResultado{{Total: 27}},
		}
	}
	resumen := func(x Expediente) string {
		p := x.Postulacion
		return fmt.Sprint(p.Estado, "|", p.MotivoDescarte, "|", p.DictamenFinal, "|", len(p.Subsanaciones), p.Documentos[0].Dictamen, "|",
			len(x.Revision.Observaciones), len(x.Revision.Dictamenes), len(x.Propias), len(x.Jurados), len(x.Resultados))
	}
	for nombre, caso := range map[string]struct {
		recorte  RecorteDeExpediente
		esperado string
	}{
		"titular":     {RecorteDeExpediente{}, "en_revision|||0|0 0 0 0 0"},
		"jurado":      {RecorteDeExpediente{EvaluacionPropia: true}, "en_revision|||0|0 0 1 0 0"},
		"revisor":     {RecorteDeExpediente{Revision: true}, "descartada|No cumple.||1invalido|1 1 0 0 0"},
		"supervisión": {RecorteDeExpediente{Revision: true, Supervision: true}, "descartada|No cumple.|seleccionada|1invalido|1 1 0 1 1"},
	} {
		x := completo(caso.recorte)
		original := x.Postulacion.Documentos
		RecortarExpediente(&x)
		if obtenido := resumen(x); obtenido != caso.esperado {
			t.Errorf("%s: %s, se esperaba %s", nombre, obtenido, caso.esperado)
		}
		if original[0].Dictamen != "invalido" {
			t.Errorf("%s: recortar el expediente alteró los documentos de la postulación original", nombre)
		}
	}
}

// configuracionDeReporte es una convocatoria con dos criterios y tres jurados.
func configuracionDeReporte(tipo TipoConvocatoria) ConfiguracionConvocatoria {
	cfg := ConfiguracionConvocatoria{Convocatoria: Convocatoria{Tipo: tipo}, Criterios: CriteriosIniciales(),
		Dotaciones: []Dotacion{{ID: 3, NombreES: "Cuarteto de cuerdas"}}}
	for i := int64(1); i <= 3; i++ {
		cfg.Asignaciones = append(cfg.Asignaciones, AsignacionDetalle{Activo: true, Perfil: PerfilJurado, Usuario: Usuario{ID: 40 + i, Nombre: "Jurado", Apellidos: fmt.Sprint(i)}})
	}
	return cfg
}

func TestValidarReporte(t *testing.T) {
	interpretes, compositores := configuracionDeReporte(TipoInterpretes), configuracionDeReporte(TipoCompositores)
	claves := func(cfg ConfiguracionConvocatoria) string {
		var lista []string
		for _, campo := range CatalogoDeReporte(cfg) {
			lista = append(lista, campo.Clave)
		}
		return " " + strings.Join(lista, " ") + " "
	}
	// El catálogo sale del formulario: cada convocatoria tiene los suyos.
	for _, clave := range []string{"folio", "subcategoria", "datos.nombre_completo", "datos.nombre_ensamble", "datos.edad", "concierto.entidad",
		"integrantes", "videos", "obra.compositor", "revision.estado", "eval.j1.realizacion", "eval.j3.artistico", "eval.j2.observaciones", "eval.lugar", "dictamen"} {
		if !strings.Contains(claves(interpretes), " "+clave+" ") {
			t.Errorf("al catálogo de Intérpretes le falta %s", clave)
		}
	}
	for _, clave := range []string{"subcategoria", "datos.nombre_ensamble", "concierto.entidad", "integrantes", "videos", "obra.compositor"} {
		if strings.Contains(claves(compositores), " "+clave+" ") {
			t.Errorf("al catálogo de Compositores le sobra %s", clave)
		}
	}
	if !strings.Contains(claves(compositores), " obra.dotacion_id ") {
		t.Error("al catálogo de Compositores le falta la dotación de la obra")
	}

	// Valores por omisión y orden de los campos.
	def := DefinicionDeReporte{Campos: []string{"estado", "folio"}}
	campos, errores := ValidarReporte(&def, interpretes)
	if len(errores) != 0 || def.Granularidad != GranularidadPostulacion || def.Separador != "coma" || len(campos) != 2 || campos[0].Clave != "estado" {
		t.Errorf("reporte mínimo: %v %+v", errores, def)
	}
	minimo, maximo := 20.0, 10.0
	for nombre, caso := range map[string]struct {
		cfg   ConfiguracionConvocatoria
		def   DefinicionDeReporte
		campo string
		clave string
	}{
		"sin campos":                                 {interpretes, DefinicionDeReporte{}, "campos", "error.campos_de_reporte"},
		"campo inventado":                            {interpretes, DefinicionDeReporte{Campos: []string{"folio", "sueldo"}}, "campos", "error.campos_de_reporte"},
		"campo repetido":                             {interpretes, DefinicionDeReporte{Campos: []string{"folio", "folio"}}, "campos", "error.campos_de_reporte"},
		"campo de otra convocatoria":                 {compositores, DefinicionDeReporte{Campos: []string{"videos"}}, "campos", "error.campos_de_reporte"},
		"obra en reporte por postulación":            {interpretes, DefinicionDeReporte{Campos: []string{"obra.titulo"}}, "granularidad", "error.reporte_por_obra"},
		"evaluación por obra en Intérpretes":         {interpretes, DefinicionDeReporte{Campos: []string{"eval.total"}, Granularidad: GranularidadObra}, "granularidad", "error.reporte_por_unidad"},
		"evaluación por postulación en Compositores": {compositores, DefinicionDeReporte{Campos: []string{"eval.total"}}, "granularidad", "error.reporte_por_unidad"},
		"rango de puntaje fuera de su unidad":        {compositores, DefinicionDeReporte{Campos: []string{"folio"}, PuntajeMin: &minimo}, "granularidad", "error.reporte_por_unidad"},
		"rango al revés":                             {interpretes, DefinicionDeReporte{Campos: []string{"folio"}, PuntajeMin: &minimo, PuntajeMax: &maximo}, "puntaje_max", "error.filtro_invalido"},
		"granularidad inventada":                     {interpretes, DefinicionDeReporte{Campos: []string{"folio"}, Granularidad: "jurado"}, "granularidad", "error.opcion_invalida"},
		"separador inventado":                        {interpretes, DefinicionDeReporte{Campos: []string{"folio"}, Separador: "tab"}, "separador", "error.opcion_invalida"},
	} {
		_, errores := ValidarReporte(&caso.def, caso.cfg)
		if len(errores) != 1 || errores[0].Campo != caso.campo || errores[0].Clave != caso.clave {
			t.Errorf("%s: %v", nombre, errores)
		}
	}
	// Lo que sí cuadra: evaluación por obra en Compositores.
	if _, errores := ValidarReporte(&DefinicionDeReporte{Campos: []string{"obra.titulo", "eval.total"}, Granularidad: GranularidadObra}, compositores); len(errores) != 0 {
		t.Errorf("evaluación por obra en Compositores: %v", errores)
	}
}

func TestValorDeReporte(t *testing.T) {
	cfg := configuracionDeReporte(TipoCompositores)
	jurados := JuradosDe(cfg)
	obra := Obra{ID: 31, Orden: 2, Campos: map[string]string{"obra.titulo": "Rapsodia", "obra.duracion_min": "9", "obra.dotacion_id": "3"}}
	cierre := time.Date(2026, 12, 20, 5, 59, 0, 0, time.UTC)
	fila := FilaDeReporte{
		Postulacion: Postulacion{Folio: "FIM49-COM-0007", Estado: PostulacionEvaluada, Envios: 2, EnviadaAt: cierre.AddDate(0, 0, -3),
			Acuses:        []Acuse{{GeneradoAt: cierre.AddDate(0, 0, -9)}, {GeneradoAt: cierre.AddDate(0, 0, -3)}},
			Campos:        map[string]string{"datos.genero": "mujer", "datos.fecha_nacimiento": "1990-12-21"},
			Integrantes:   []Integrante{{Nombre: "Abel Cruz", Instrumento: "flauta"}, {Nombre: "Eva Luna", Instrumento: "arpa"}},
			Videos:        []EnlaceVideo{{URL: "https://vimeo.com/1"}, {URL: "https://vimeo.com/2"}},
			Obras:         []Obra{{Orden: 1, Campos: map[string]string{"obra.titulo": "Preludio", "obra.duracion_min": "12"}}, obra},
			Subsanaciones: []Subsanacion{{ID: 1}}, DictamenFinal: DictamenListaDeEspera},
		Titular:       Usuario{Nombre: "María", Apellidos: "Núñez", Email: "maria@ejemplo.mx", Idioma: "es"},
		Obra:          &obra,
		Observaciones: []ObservacionRevision{{Texto: "Uno."}, {Texto: "Dos."}},
		Resultado: &FilaDeResultado{JuradosQueEvaluaron: 2, Total: 17, Promedio: 4.25, Comparable: 25.5,
			PorJurado: map[int64]Evaluacion{41: {Enviada: true, Observaciones: "Sólida.", Puntajes: map[string]int{"realizacion": 4, "artistico": 5}}},
			Excusas:   map[int64]string{43: "Fue mi alumna."}},
		Lugar: 3, Jurados: jurados, Dotaciones: cfg.Dotaciones, Cierre: cierre,
	}
	catalogo := make(map[string]CampoDeReporte)
	for _, campo := range CatalogoDeReporte(cfg) {
		catalogo[campo.Clave] = campo
	}
	catalogo["integrantes"], catalogo["videos"] = CampoDeReporte{Clave: "integrantes"}, CampoDeReporte{Clave: "videos"} // de Intérpretes
	for clave, esperado := range map[string]string{
		"folio": "FIM49-COM-0007", "estado": "evaluada", "envios": "2", "primera_enviada_at": "2026-12-11T05:59:00Z", "enviada_at": "2026-12-17T05:59:00Z",
		"usuario.nombre": "María Núñez", "usuario.email": "maria@ejemplo.mx", "datos.genero": "mujer",
		"datos.edad":  "35", // cumple 36 al día siguiente del cierre
		"integrantes": "Abel Cruz – flauta; Eva Luna – arpa", "videos": "https://vimeo.com/1 https://vimeo.com/2",
		"obras.count": "2", "obras.duracion_total": "21", "obras.titulos": "Preludio; Rapsodia",
		"obra.orden": "2", "obra.titulo": "Rapsodia", "obra.dotacion_id": "Cuarteto de cuerdas",
		"revision.estado": "validada", "revision.observaciones": "Uno. | Dos.", "revision.subsanaciones": "1",
		"eval.j1.realizacion": "4", "eval.j1.observaciones": "Sólida.",
		"eval.j2.realizacion": "", // aún no envía
		"eval.j3.realizacion": "excusa", "eval.j3.observaciones": "",
		"eval.jurados": "2", "eval.total": "17", "eval.promedio": "4.25", "eval.puntaje": "25.5", "eval.lugar": "3", "dictamen": "lista_espera",
	} {
		campo, existe := catalogo[clave]
		if !existe {
			t.Errorf("%s no está en el catálogo", clave)
			continue
		}
		if obtenido := ValorDeReporte(campo, fila); obtenido != esperado {
			t.Errorf("%s: %q, se esperaba %q", clave, obtenido, esperado)
		}
	}
	// Sin evaluación no hay valores de evaluación, ni un lugar inventado.
	fila.Resultado, fila.Lugar = nil, 0
	for _, clave := range []string{"eval.j1.realizacion", "eval.total", "eval.puntaje", "eval.lugar", "eval.jurados"} {
		if obtenido := ValorDeReporte(catalogo[clave], fila); obtenido != "" {
			t.Errorf("%s sin evaluación: %q", clave, obtenido)
		}
	}
}

// Los empates comparten lugar y el siguiente se salta (RF-REP-08).
func TestLugaresDeLaMatriz(t *testing.T) {
	filas := []FilaDeResultado{
		{JuradosQueEvaluaron: 3, Comparable: 28}, {JuradosQueEvaluaron: 3, Comparable: 25}, {JuradosQueEvaluaron: 2, Comparable: 25},
		{JuradosQueEvaluaron: 3, Comparable: 20}, {JuradosQueEvaluaron: 0}, {JuradosQueEvaluaron: 0},
	}
	if lugares := fmt.Sprint(LugaresDeLaMatriz(filas)); lugares != "[1 2 2 4 0 0]" {
		t.Errorf("lugares: %s", lugares)
	}
}

func TestEdadYRangos(t *testing.T) {
	referencia := time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
	for nacimiento, esperado := range map[string]string{
		"1990-12-20": "36 true", "1990-12-21": "35 true", "2009-01-01": "17 true", "2027-01-01": "0 false", "": "0 false", "17/05/1990": "0 false",
	} {
		edad, ok := EdadEn(nacimiento, referencia)
		if obtenido := fmt.Sprint(edad, ok); obtenido != esperado {
			t.Errorf("edad de %q: %s", nacimiento, obtenido)
		}
	}
	for edad, rango := range map[int]string{17: "menos_de_18", 18: "18_a_25", 25: "18_a_25", 26: "26_a_35", 45: "36_a_45", 60: "46_a_60", 61: "mas_de_60"} {
		if obtenido := RangoDeEdad(edad); obtenido != rango {
			t.Errorf("rango de %d años: %s", edad, obtenido)
		}
	}
	for estado, resultado := range map[EstadoDePostulacion]string{PostulacionBorrador: "", PostulacionEnviada: "en_revision", PostulacionEnSubsanacion: "en_revision",
		PostulacionDescartada: "descartada", PostulacionValidada: "validada", PostulacionEvaluada: "validada"} {
		if obtenido := ResultadoDeRevision(estado); obtenido != resultado {
			t.Errorf("resultado de la revisión de %s: %q", estado, obtenido)
		}
	}
}
