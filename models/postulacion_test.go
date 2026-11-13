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
	"slices"
	"strings"
	"testing"
	"time"
)

// Pruebas de ValidarParaEnvio, la función más importante del sistema (§8.4):
// se parte de una postulación completa, que no tiene bloqueadores, y se le
// quita o se le estropea una cosa a la vez para comprobar que cada regla
// produce exactamente su bloqueador.

var ahoraDePrueba = time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

// configuracionDeEnvio devuelve la configuración de una convocatoria con los
// requisitos iniciales de su tipo y, en Compositores, dos dotaciones: la 1 es
// un cuarteto (reglas generales) y la 2 una obra electroacústica, que no pide
// particellas y sí exige el audio.
func configuracionDeEnvio(tipo TipoConvocatoria) ConfiguracionConvocatoria {
	c := convocatoriaDePrueba(1, tipo, EstadoPostulacion)
	c.MaxVideos = 3
	cfg := ConfiguracionConvocatoria{Convocatoria: c}
	for i, rq := range RequisitosIniciales(tipo) {
		rq.ID = int64(i + 1)
		cfg.Requisitos = append(cfg.Requisitos, rq)
	}
	if tipo == TipoCompositores {
		cfg.Dotaciones = []Dotacion{{ID: 1, NombreES: "Cuarteto", Activa: true}, {ID: 2, NombreES: "Electroacústica", Activa: true}}
		for _, rq := range RequisitosIniciales(tipo) {
			switch rq.Clave {
			case "particellas":
				rq.ID, rq.DotacionID, rq.Obligatorio = 101, 2, false
				cfg.Requisitos = append(cfg.Requisitos, rq)
			case "audio":
				rq.ID, rq.DotacionID, rq.Obligatorio = 102, 2, true
				cfg.Requisitos = append(cfg.Requisitos, rq)
			}
		}
	}
	return cfg
}

// postulacionCompleta devuelve una postulación a la que no le falta nada.
func postulacionCompleta(tipo TipoConvocatoria, sub Subcategoria) Postulacion {
	p := Postulacion{ID: 1, Subcategoria: sub, Estado: PostulacionBorrador, Campos: map[string]string{}}
	persona := map[string]string{
		"datos.nombre_completo": "Ana López", "datos.genero": "mujer", "datos.fecha_nacimiento": "1990-05-17",
		"datos.telefono": "55 1234 5678", "datos.email_contacto": "ana@ejemplo.mx", "datos.nacionalidad": "MX",
		"datos.pais_residencia": "MX", "datos.entidad_residencia": "JAL", "datos.semblanza": "Semblanza.",
	}
	ensamble := map[string]string{
		"datos.nombre_ensamble": "Cuarteto Ónix", "datos.representante": "Ana López", "datos.telefono": "55 1234 5678",
		"datos.email_contacto": "onix@ejemplo.mx", "datos.dotacion_texto": "Cuarteto de cuerdas", "datos.anio_fundacion": "2010",
		"datos.pais_residencia": "AR", "datos.entidad_residencia": "Provincia de Buenos Aires", "datos.semblanza": "Semblanza.",
	}
	datos := persona
	if sub == SubcategoriaEnsamble {
		datos = ensamble
		p.Integrantes = []Integrante{{"Ana López", "violín"}, {"Luis Ruiz", "violonchelo"}}
	}
	for ruta, valor := range datos {
		p.Campos[ruta] = valor
	}
	p.Documentos = []Documento{{Clave: "fotografia"}}
	if tipo == TipoInterpretes {
		p.Campos["concierto.requerimientos_tecnicos"] = "Piano de cola."
		p.Campos["concierto.entidad"] = EntidadCDMX
		p.Videos = []EnlaceVideo{{URL: "https://vimeo.com/1", Estado: EnlaceOK, Plataforma: "vimeo"}}
		p.Obras = []Obra{{ID: 10, Orden: 1, Campos: map[string]string{
			"obra.titulo": "Obra", "obra.compositor": "Julián Carrillo", "obra.duracion_min": "12", "obra.estreno": "no_es_estreno",
			"obra.semblanza_compositor": "Semblanza.", "obra.nota_obra": "Nota.", "obra.liga_media": "https://ejemplo.mx/obra",
		}}}
		p.Documentos = append(p.Documentos, Documento{Clave: "partitura", ObraID: 10}, Documento{Clave: "autorizacion", ObraID: 10})
		return p
	}
	obra := func(id int64, dotacion string) Obra {
		return Obra{ID: id, Campos: map[string]string{
			"obra.titulo": "Obra", "obra.duracion_min": "9", "obra.estreno": "estreno_mundial", "obra.dotacion_id": dotacion,
			"obra.nota_obra": "Nota.", "obra.requerimientos_tecnicos": "Ninguno.",
		}}
	}
	// Una obra para cuarteto (partitura y particellas) y una electroacústica
	// (partitura y audio; sin particellas).
	p.Obras = []Obra{obra(10, "1"), obra(11, "2")}
	p.Documentos = append(p.Documentos, Documento{Clave: "partitura", ObraID: 10}, Documento{Clave: "particellas", ObraID: 10},
		Documento{Clave: "partitura", ObraID: 11}, Documento{Clave: "audio", ObraID: 11})
	return p
}

// resumen convierte los bloqueadores en texto comparable:
// «sección/campo[#obra]:motivo».
func resumen(bloqueadores []Bloqueador) string {
	var lista []string
	for _, b := range bloqueadores {
		campo := b.Campo
		if b.ObraID != 0 {
			campo += fmt.Sprintf("#%d", b.ObraID)
		}
		lista = append(lista, b.Seccion+"/"+campo+":"+strings.TrimPrefix(b.Clave, "bloqueo."))
	}
	slices.Sort(lista)
	return strings.Join(lista, " ")
}

var titularVerificado = Usuario{ID: 7, EmailVerificadoAt: ahoraDePrueba}

// Los tres formularios, completos, no tienen bloqueadores.
func TestPostulacionCompletaPuedeEnviarse(t *testing.T) {
	casos := []struct {
		tipo TipoConvocatoria
		sub  Subcategoria
	}{{TipoInterpretes, SubcategoriaSolista}, {TipoInterpretes, SubcategoriaEnsamble}, {TipoCompositores, ""}}
	for _, c := range casos {
		bloqueadores := ValidarParaEnvio(postulacionCompleta(c.tipo, c.sub), configuracionDeEnvio(c.tipo), titularVerificado, ahoraDePrueba)
		if len(bloqueadores) != 0 {
			t.Errorf("%s/%s completa no debería tener bloqueadores: %s", c.tipo, c.sub, resumen(bloqueadores))
		}
	}
}

// Cada campo obligatorio de cada formulario, vacío, produce su bloqueador; y
// ningún campo opcional lo produce.
func TestCadaCampoObligatorioBloquea(t *testing.T) {
	casos := []struct {
		tipo TipoConvocatoria
		sub  Subcategoria
	}{{TipoInterpretes, SubcategoriaSolista}, {TipoInterpretes, SubcategoriaEnsamble}, {TipoCompositores, ""}}
	for _, c := range casos {
		cfg := configuracionDeEnvio(c.tipo)
		formulario := FormularioDe(c.tipo, c.sub)
		for _, def := range formulario.Campos {
			p := postulacionCompleta(c.tipo, c.sub)
			p.Campos[def.Ruta] = "   " // sólo espacios cuenta como vacío
			obtenido := resumen(ValidarParaEnvio(p, cfg, titularVerificado, ahoraDePrueba))
			esperado := ""
			if def.Obligatorio {
				esperado = seccionDeRuta(def.Ruta) + "/" + def.Ruta + ":campo_requerido"
			}
			if obtenido != esperado {
				t.Errorf("%s/%s sin %s: %q, se esperaba %q", c.tipo, c.sub, def.Ruta, obtenido, esperado)
			}
		}
		for _, def := range formulario.CamposObra {
			p := postulacionCompleta(c.tipo, c.sub)
			delete(p.Obras[0].Campos, def.Ruta)
			obtenido := resumen(ValidarParaEnvio(p, cfg, titularVerificado, ahoraDePrueba))
			esperado := ""
			if def.Obligatorio {
				esperado = "obras/" + def.Ruta + "#10:campo_requerido"
			}
			// Sin dotación la obra se rige por las reglas generales: la del
			// cuarteto ya las cumple, así que no aparece nada más.
			if obtenido != esperado {
				t.Errorf("%s/%s, obra sin %s: %q, se esperaba %q", c.tipo, c.sub, def.Ruta, obtenido, esperado)
			}
		}
	}
	// El nombre artístico es opcional en ambas convocatorias (RN-09, D-04).
	for _, tipo := range []TipoConvocatoria{TipoInterpretes, TipoCompositores} {
		if def, ok := FormularioDe(tipo, SubcategoriaSolista).Campo("datos.nombre_artistico"); !ok || def.Obligatorio {
			t.Errorf("%s: el nombre artístico debe existir y ser opcional", tipo)
		}
	}
}

// Las demás reglas del envío, una por una.
func TestReglasDelEnvio(t *testing.T) {
	casos := []struct {
		nombre   string
		tipo     TipoConvocatoria
		sub      Subcategoria
		alterar  func(*Postulacion, *ConfiguracionConvocatoria, *Usuario)
		esperado string
	}{
		{"correo sin verificar", TipoInterpretes, SubcategoriaSolista,
			func(_ *Postulacion, _ *ConfiguracionConvocatoria, u *Usuario) { u.EmailVerificadoAt = time.Time{} },
			"envio/cuenta.email:correo_sin_verificar"},

		// RN-06: fuera de la Ciudad de México, sede, domicilio, fecha y hora.
		{"concierto fuera de CDMX sin datos de sede", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Campos["concierto.entidad"] = "JAL" },
			"concierto/concierto.fecha_concierto:campo_requerido concierto/concierto.hora_concierto:campo_requerido " +
				"concierto/concierto.sede_domicilio:campo_requerido concierto/concierto.sede_nombre:campo_requerido"},
		{"concierto fuera de CDMX con datos de sede", TipoInterpretes, SubcategoriaEnsamble,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) {
				p.Campos["concierto.entidad"], p.Campos["concierto.sede_nombre"] = "OAX", "Teatro Macedonio Alcalá"
				p.Campos["concierto.sede_domicilio"], p.Campos["concierto.fecha_concierto"] = "Independencia 900", "2027-10-12"
				p.Campos["concierto.hora_concierto"] = "19:30"
			}, ""},

		// RN-10: un ensamble declara al menos un integrante.
		{"ensamble sin integrantes", TipoInterpretes, SubcategoriaEnsamble,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Integrantes = nil },
			"datos/integrantes:sin_integrantes"},

		// RN-08: de una a tres ligas de video, sin problemas.
		{"sin videos", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Videos = nil }, "datos/videos:sin_videos"},
		{"cuatro videos", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) {
				p.Videos = append(p.Videos, p.Videos[0], p.Videos[0], p.Videos[0])
			}, "datos/videos:demasiados_videos"},
		{"video inaccesible", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Videos[0].Estado = EnlaceInaccesible },
			"datos/videos:video_con_problema"},
		{"video no público", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Videos[0].Estado = EnlaceNoPublico },
			"datos/videos:video_con_problema"},
		{"video aún por comprobar no bloquea", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Videos[0].Estado = EnlacePendiente }, ""},
		{"en Compositores no se piden videos", TipoCompositores, "",
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Videos = nil }, ""},

		// RN-05: al menos una obra; máximo en Compositores; duración total si se limita.
		{"sin obras", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Obras = nil }, "obras/obras:sin_obras"},
		{"más obras que el máximo", TipoCompositores, "",
			func(_ *Postulacion, cfg *ConfiguracionConvocatoria, _ *Usuario) { cfg.Convocatoria.MaxObras = 1 },
			"obras/obras:demasiadas_obras"},
		{"Intérpretes no tiene máximo de obras", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) {
				for i := int64(20); i < 40; i++ {
					otra := Obra{ID: i, Campos: p.Obras[0].Campos}
					p.Obras = append(p.Obras, otra)
					p.Documentos = append(p.Documentos, Documento{Clave: "partitura", ObraID: i}, Documento{Clave: "autorizacion", ObraID: i})
				}
			}, ""},
		{"duración total rebasada", TipoCompositores, "",
			func(_ *Postulacion, cfg *ConfiguracionConvocatoria, _ *Usuario) {
				cfg.Convocatoria.DuracionMaxProgramaMin = 17
			},
			"obras/obras:duracion_excedida"},
		{"duración total en el límite", TipoCompositores, "",
			func(_ *Postulacion, cfg *ConfiguracionConvocatoria, _ *Usuario) {
				cfg.Convocatoria.DuracionMaxProgramaMin = 18
			}, ""},

		// RN-07: los documentos de cada obra dependen de SU dotación.
		{"falta la fotografía", TipoCompositores, "",
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Documentos = p.Documentos[1:] },
			"documentos/documento.fotografia:documento_faltante"},
		{"sin ningún documento", TipoCompositores, "",
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) { p.Documentos = nil },
			"documentos/documento.audio#11:documento_faltante documentos/documento.fotografia:documento_faltante " +
				"documentos/documento.particellas#10:documento_faltante documentos/documento.partitura#10:documento_faltante " +
				"documentos/documento.partitura#11:documento_faltante"},
		{"la partitura de una obra no sirve para otra", TipoInterpretes, SubcategoriaSolista,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) {
				for i := range p.Documentos {
					if p.Documentos[i].Clave == "partitura" {
						p.Documentos[i].ObraID = 99
					}
				}
			}, "documentos/documento.partitura#10:documento_faltante"},
		{"al cambiar la dotación cambian los documentos exigidos", TipoCompositores, "",
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) {
				p.Obras[1].Campos["obra.dotacion_id"] = "1"
			},
			"documentos/documento.particellas#11:documento_faltante"},
		{"un documento opcional puede faltar", TipoCompositores, "",
			func(p *Postulacion, cfg *ConfiguracionConvocatoria, _ *Usuario) {
				for i := range cfg.Requisitos {
					cfg.Requisitos[i].Obligatorio = false
				}
				p.Documentos = nil
			}, ""},

		// Un valor guardado que dejó de ser válido también bloquea.
		{"dotación retirada del catálogo", TipoCompositores, "",
			func(_ *Postulacion, cfg *ConfiguracionConvocatoria, _ *Usuario) { cfg.Dotaciones[0].Activa = false },
			"obras/obra.dotacion_id#10:campo_invalido"},
		{"entidad escrita a mano y país cambiado a México", TipoInterpretes, SubcategoriaEnsamble,
			func(p *Postulacion, _ *ConfiguracionConvocatoria, _ *Usuario) {
				p.Campos["datos.pais_residencia"] = "MX"
			},
			"datos/datos.entidad_residencia:campo_invalido"},
	}
	for _, c := range casos {
		p, cfg, titular := postulacionCompleta(c.tipo, c.sub), configuracionDeEnvio(c.tipo), titularVerificado
		c.alterar(&p, &cfg, &titular)
		if obtenido := resumen(ValidarParaEnvio(p, cfg, titular, ahoraDePrueba)); obtenido != c.esperado {
			t.Errorf("%s:\n  %s\nse esperaba:\n  %s", c.nombre, obtenido, c.esperado)
		}
	}
	// El bloqueador de un documento lleva el requisito, para poder nombrarlo.
	p := postulacionCompleta(TipoInterpretes, SubcategoriaSolista)
	p.Documentos = nil
	for _, b := range ValidarParaEnvio(p, configuracionDeEnvio(TipoInterpretes), titularVerificado, ahoraDePrueba) {
		if b.Requisito == nil || b.Requisito.EtiquetaES == "" {
			t.Errorf("el bloqueador %s no lleva su requisito", b.Campo)
		}
	}
}

// El formato de cada tipo de campo.
func TestValidarValorDeCampo(t *testing.T) {
	persona := FormularioDe(TipoCompositores, "")
	ensamble := FormularioDe(TipoInterpretes, SubcategoriaEnsamble)
	mexico := ContextoCampos{Ahora: ahoraDePrueba, PaisResidencia: "MX", Dotaciones: []Dotacion{{ID: 3, Activa: true}, {ID: 4}}}
	extranjero := ContextoCampos{Ahora: ahoraDePrueba, PaisResidencia: "AR"}
	casos := []struct {
		formulario DefinicionFormulario
		ruta       string
		valor      string
		ctx        ContextoCampos
		error      string
	}{
		{persona, "datos.nombre_completo", "", mexico, ""}, // vacío siempre se acepta al guardar
		{persona, "datos.nombre_completo", strings.Repeat("ñ", 200), mexico, ""},
		{persona, "datos.nombre_completo", strings.Repeat("ñ", 201), mexico, "error.texto_largo_campo"},
		{persona, "datos.semblanza", strings.Repeat("a", LimiteSemblanza+1), mexico, "error.texto_largo_campo"},
		{persona, "datos.genero", "no_binario", mexico, ""},
		{persona, "datos.genero", "ninguno", mexico, "error.opcion_invalida"},
		{persona, "datos.fecha_nacimiento", "1990-05-17", mexico, ""},
		{persona, "datos.fecha_nacimiento", "17/05/1990", mexico, "error.fecha_invalida"},
		{persona, "datos.fecha_nacimiento", "2027-01-01", mexico, "error.fecha_invalida"}, // futura
		{persona, "datos.fecha_nacimiento", "1850-01-01", mexico, "error.fecha_invalida"},
		{persona, "datos.email_contacto", "ana@ejemplo.mx", mexico, ""},
		{persona, "datos.email_contacto", "ana@", mexico, "error.email_invalido"},
		{persona, "datos.nacionalidad", "MX", mexico, ""},
		{persona, "datos.nacionalidad", "México", mexico, "error.opcion_invalida"},
		{persona, "datos.nacionalidad", "mx", mexico, "error.opcion_invalida"},
		{persona, "datos.entidad_residencia", "JAL", mexico, ""},
		{persona, "datos.entidad_residencia", "Jalisco", mexico, "error.opcion_invalida"}, // en México, del catálogo
		{persona, "datos.entidad_residencia", "Provincia de Buenos Aires", extranjero, ""},
		{persona, "datos.entidad_residencia", strings.Repeat("a", 101), extranjero, "error.texto_largo_campo"},
		{ensamble, "datos.anio_fundacion", "2010", mexico, ""},
		{ensamble, "datos.anio_fundacion", "2026", mexico, ""},
		{ensamble, "datos.anio_fundacion", "2027", mexico, "error.numero_fuera_de_rango"}, // año aún no llegado
		{ensamble, "datos.anio_fundacion", "1899", mexico, "error.numero_fuera_de_rango"},
		{ensamble, "datos.anio_fundacion", "dos mil", mexico, "error.numero_fuera_de_rango"},
		{ensamble, "concierto.entidad", EntidadCDMX, mexico, ""},
		{ensamble, "concierto.entidad", "CMX", mexico, "error.opcion_invalida"},
		{ensamble, "concierto.fecha_concierto", "2027-10-12", mexico, ""}, // una fecha de concierto sí puede ser futura
		{ensamble, "concierto.hora_concierto", "19:30", mexico, ""},
		{ensamble, "concierto.hora_concierto", "7:30 pm", mexico, "error.hora_invalida"},
		{ensamble, "concierto.hora_concierto", "25:00", mexico, "error.hora_invalida"},
	}
	for _, c := range casos {
		def, ok := c.formulario.Campo(c.ruta)
		if !ok {
			t.Fatalf("el formulario no tiene el campo %s", c.ruta)
		}
		if obtenido := ValidarValorDeCampo(def, c.valor, c.ctx); obtenido != c.error {
			t.Errorf("%s = %q: %q, se esperaba %q", c.ruta, c.valor, obtenido, c.error)
		}
	}
	obra := func(ruta string) DefinicionCampo {
		def, _ := persona.CampoDeObra(ruta)
		return def
	}
	for valor, esperado := range map[string]string{"12": "", "1": "", "600": "", "0": "error.numero_fuera_de_rango",
		"601": "error.numero_fuera_de_rango", "12.5": "error.numero_fuera_de_rango", "-3": "error.numero_fuera_de_rango"} {
		if obtenido := ValidarValorDeCampo(obra("obra.duracion_min"), valor, mexico); obtenido != esperado {
			t.Errorf("duración %q: %q, se esperaba %q", valor, obtenido, esperado)
		}
	}
	// Sólo una dotación ACTIVA de la convocatoria.
	for valor, esperado := range map[string]string{"3": "", "4": "error.dotacion_inexistente", "9": "error.dotacion_inexistente", "x": "error.dotacion_inexistente"} {
		if obtenido := ValidarValorDeCampo(obra("obra.dotacion_id"), valor, mexico); obtenido != esperado {
			t.Errorf("dotación %q: %q, se esperaba %q", valor, obtenido, esperado)
		}
	}
}

func TestLigasYPlataformas(t *testing.T) {
	validas := []string{"https://www.youtube.com/watch?v=abc", "https://youtu.be/abc", "https://vimeo.com/123", "https://sitio.ejemplo.mx/v/1?x=1&y=2"}
	invalidas := []string{"", "http://vimeo.com/123", "vimeo.com/123", "https://", "https://localhost/x", "ftp://ejemplo.mx/x",
		"https://usuario:clave@ejemplo.mx/x", "https://ejemplo.mx/con espacio", "javascript:alert(1)", "https://" + strings.Repeat("a", 2000) + ".mx"}
	for _, liga := range validas {
		if !URLValida(liga) {
			t.Errorf("debió aceptarse %q", liga)
		}
	}
	for _, liga := range invalidas {
		if URLValida(liga) {
			t.Errorf("debió rechazarse %q", liga)
		}
	}
	plataformas := map[string]string{
		"https://www.youtube.com/watch?v=abc": "youtube", "https://youtu.be/abc": "youtube", "https://m.youtube.com/watch?v=1": "youtube",
		"https://vimeo.com/123": "vimeo", "https://player.vimeo.com/video/1": "vimeo", "https://soundcloud.com/a/b": "soundcloud",
		"https://artista.bandcamp.com/track/x": "bandcamp", "https://drive.google.com/file/d/1/view": "drive",
		"https://ejemplo.mx/video": "otro", "https://notyoutube.com/x": "otro", "https://youtube.com.ejemplo.mx/x": "otro",
	}
	for liga, esperada := range plataformas {
		if obtenida := PlataformaDeVideo(liga); obtenida != esperada {
			t.Errorf("%s: plataforma %q, se esperaba %q", liga, obtenida, esperada)
		}
	}
	if claves(ValidarVideos([]string{"https://a.mx/1", "https://a.mx/2", "https://a.mx/3", "https://a.mx/4"}, 3)) != "[error.demasiados_videos]" ||
		claves(ValidarVideos([]string{"https://a.mx/1", "https://a.mx/1"}, 3)) != "[error.video_repetido]" ||
		claves(ValidarVideos([]string{"http://a.mx/1"}, 3)) != "[error.url_invalida]" || ValidarVideos([]string{"https://a.mx/1"}, 3) != nil {
		t.Error("reglas de la lista de videos incorrectas")
	}
}

func TestCatalogos(t *testing.T) {
	if n := len(CodigosDePais()); n != 249 || !PaisValido("MX") || !PaisValido("AR") || PaisValido("XX") || PaisValido("mx") {
		t.Errorf("catálogo de países incorrecto: %d códigos", n)
	}
	if !slices.IsSorted(CodigosDePais()) {
		t.Error("los códigos de país deben estar ordenados: la búsqueda es binaria")
	}
	entidades := EntidadesFederativas()
	vistas := map[string]bool{}
	for _, e := range entidades {
		vistas[e.Clave] = true
	}
	if len(entidades) != 32 || len(vistas) != 32 || !EntidadValida(EntidadCDMX) || EntidadValida("CMX") || EntidadValida("") {
		t.Errorf("catálogo de entidades incorrecto: %d entidades, %d claves distintas", len(entidades), len(vistas))
	}
}

// La captura exige etapa Y fecha: si la convocatoria no avanzó al vencer el
// plazo, la captura queda cerrada de todos modos.
func TestPlazoDePostulacion(t *testing.T) {
	c := convocatoriaDePrueba(1, TipoInterpretes, EstadoPostulacion) // postulación: todo enero de 2027
	dia := func(mes time.Month, d int) time.Time { return time.Date(2027, mes, d, 12, 0, 0, 0, time.UTC) }
	casos := []struct {
		estado EstadoConvocatoria
		ahora  time.Time
		error  string
	}{
		{EstadoPostulacion, dia(1, 15), "[]"},
		{EstadoPostulacion, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "[error.plazo_vencido]"},
		{EstadoPostulacion, dia(2, 1), "[error.plazo_vencido]"}, // venció y la convocatoria no avanzó
		{EstadoPublicada, dia(1, 15), "[error.fuera_de_etapa]"},
		{EstadoRevisionDocumental, dia(1, 15), "[error.fuera_de_etapa]"},
	}
	for _, caso := range casos {
		c.Estado = caso.estado
		if obtenido := claves(EnPlazoDePostulacion(c, caso.ahora)); obtenido != caso.error {
			t.Errorf("%s el %s: %s, se esperaba %s", caso.estado, caso.ahora.Format(time.DateOnly), obtenido, caso.error)
		}
	}
	c.Estado = EstadoPostulacion
	enPlazo := dia(1, 15)
	for nombre, caso := range map[string]struct {
		tipo     TipoConvocatoria
		sub      Subcategoria
		asignado bool
		error    string
	}{
		"solista":                       {TipoInterpretes, SubcategoriaSolista, false, "[]"},
		"intérpretes sin subcategoría":  {TipoInterpretes, "", false, "[error.subcategoria_requerida]"},
		"subcategoría inventada":        {TipoInterpretes, "coro", false, "[error.subcategoria_requerida]"},
		"compositores":                  {TipoCompositores, "", false, "[]"},
		"compositores con subcategoría": {TipoCompositores, SubcategoriaSolista, false, "[error.subcategoria_no_aplica]"},
		"jurado de la convocatoria":     {TipoCompositores, "", true, "[error.postula_asignado]"},
	} {
		c.Tipo = caso.tipo
		if obtenido := claves(ValidarNuevaPostulacion(c, caso.sub, caso.asignado, enPlazo)); obtenido != caso.error {
			t.Errorf("%s: %s, se esperaba %s", nombre, obtenido, caso.error)
		}
	}
	enviada := Postulacion{Estado: PostulacionEnviada}
	if obtenido := claves(ValidarEdicionDePostulacion(enviada, c, enPlazo)); obtenido != "[error.postulacion_enviada]" {
		t.Errorf("una postulación enviada no se edita: %s", obtenido)
	}
	if !PuedePostular(&Sesion{Perfiles: []Perfil{PerfilPostulante}}) || !PuedePostular(&Sesion{Perfiles: []Perfil{PerfilAdminSistema}}) ||
		PuedePostular(&Sesion{Perfiles: []Perfil{PerfilCoordinacionNacional}}) || PuedePostular(nil) {
		t.Error("permiso de postular incorrecto")
	}
}
