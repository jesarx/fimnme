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
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Pruebas de las reglas de la evaluación por jurado y, sobre todo, del
// aislamiento entre jurados, que es estructural (§8.4).

func TestReglasDeLaEvaluacion(t *testing.T) {
	cfg := configuracionDeEnvio(TipoCompositores)
	cfg.Convocatoria.Estado = EstadoEvaluacion
	cfg.Criterios = CriteriosIniciales()
	p := postulacionCompleta(TipoCompositores, "") // obras 10 y 11
	p.Estado = PostulacionEnEvaluacion
	completa := map[string]int{"realizacion": 4, "artistico": 5}
	casos := []struct {
		nombre    string
		entrada   EntradaEvaluacion
		yaEnviada bool
		error     string
	}{
		{"borrador vacío", EntradaEvaluacion{ObraID: 10}, false, "[]"},
		{"borrador con un solo criterio", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"realizacion": 3}}, false, "[]"},
		{"envío completo", EntradaEvaluacion{ObraID: 11, Puntajes: completa, Enviar: true}, false, "[]"},
		{"envío con observaciones", EntradaEvaluacion{ObraID: 11, Puntajes: completa, Observaciones: "Sólida.", Enviar: true}, false, "[]"},
		{"envío sin un criterio", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"realizacion": 3}, Enviar: true}, false, "[error.puntaje_requerido]"},
		{"envío sin puntajes", EntradaEvaluacion{ObraID: 10, Enviar: true}, false, "[error.puntaje_requerido error.puntaje_requerido]"},
		// Una evaluación ya enviada no puede quedar incompleta al corregirla.
		{"corregir una enviada, completa", EntradaEvaluacion{ObraID: 10, Puntajes: completa}, true, "[]"},
		{"corregir una enviada, incompleta", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"artistico": 5}}, true, "[error.puntaje_requerido]"},
		// RN-17: enteros dentro de la escala del criterio (1 a 5).
		{"puntaje 0", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"realizacion": 0}}, false, "[error.puntaje_fuera_de_escala]"},
		{"puntaje 6", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"artistico": 6}}, false, "[error.puntaje_fuera_de_escala]"},
		{"puntaje negativo", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"artistico": -1}}, false, "[error.puntaje_fuera_de_escala]"},
		{"extremos de la escala", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"realizacion": 1, "artistico": 5}, Enviar: true}, false, "[]"},
		{"criterio inventado", EntradaEvaluacion{ObraID: 10, Puntajes: map[string]int{"originalidad": 3}}, false, "[error.criterio_inexistente]"},
		// RN-16: en Compositores la unidad es la obra.
		{"sin obra en Compositores", EntradaEvaluacion{Puntajes: completa}, false, "[error.unidad_inexistente]"},
		{"obra de otra postulación", EntradaEvaluacion{ObraID: 99, Puntajes: completa}, false, "[error.unidad_inexistente]"},
		{"observaciones larguísimas", EntradaEvaluacion{ObraID: 10, Observaciones: strings.Repeat("a", LongitudMaxMotivo+1)}, false, "[error.texto_largo]"},
	}
	for _, c := range casos {
		if obtenido := claves(ValidarEvaluacion(cfg, p, c.entrada, c.yaEnviada)); obtenido != c.error {
			t.Errorf("%s: %s, se esperaba %s", c.nombre, obtenido, c.error)
		}
	}

	// En Intérpretes la unidad es la postulación completa.
	interpretes := configuracionDeEnvio(TipoInterpretes)
	interpretes.Convocatoria.Estado, interpretes.Criterios = EstadoEvaluacion, CriteriosIniciales()
	solista := postulacionCompleta(TipoInterpretes, SubcategoriaSolista)
	solista.Estado = PostulacionEnEvaluacion
	if claves(ValidarEvaluacion(interpretes, solista, EntradaEvaluacion{Puntajes: completa, Enviar: true}, false)) != "[]" ||
		claves(ValidarEvaluacion(interpretes, solista, EntradaEvaluacion{ObraID: 10, Puntajes: completa}, false)) != "[error.unidad_inexistente]" {
		t.Error("en Intérpretes se califica la postulación, no cada obra")
	}
	if u := UnidadesDeEvaluacion(TipoInterpretes, solista); len(u) != 1 || u[0] != 0 {
		t.Errorf("unidades de Intérpretes: %v", u)
	}
	if u := UnidadesDeEvaluacion(TipoCompositores, p); len(u) != 2 || u[0] != 10 || u[1] != 11 {
		t.Errorf("unidades de Compositores: %v", u)
	}

	// RN-19: cerrada la etapa, nada es editable. RN-20: sólo lo que pasó a evaluación.
	for _, estado := range []EstadoConvocatoria{EstadoRevisionDocumental, EstadoCerrada, EstadoArchivada} {
		fuera := cfg
		fuera.Convocatoria.Estado = estado
		if obtenido := claves(ValidarEvaluacion(fuera, p, EntradaEvaluacion{ObraID: 10, Puntajes: completa}, true)); obtenido != "[error.fuera_de_evaluacion]" {
			t.Errorf("evaluar con la convocatoria en %s: %s", estado, obtenido)
		}
	}
	for _, estado := range []EstadoDePostulacion{PostulacionBorrador, PostulacionEnviada, PostulacionEnRevision, PostulacionValidada, PostulacionDescartada} {
		otra := p
		otra.Estado = estado
		if obtenido := claves(ValidarEvaluacion(cfg, otra, EntradaEvaluacion{ObraID: 10, Puntajes: completa}, false)); obtenido != "[error.no_esta_en_evaluacion]" {
			t.Errorf("evaluar una postulación %s: %s", estado, obtenido)
		}
	}

	// Dictamen final.
	for dictamen, esperado := range map[string]string{DictamenSeleccionada: "[]", DictamenNoSeleccionada: "[]", DictamenListaDeEspera: "[]", "": "[]", "ganadora": "[error.opcion_invalida]"} {
		if obtenido := claves(ValidarDictamenFinal(cfg.Convocatoria, p, dictamen)); obtenido != esperado {
			t.Errorf("dictamen final %q: %s, se esperaba %s", dictamen, obtenido, esperado)
		}
	}
	descartada := p
	descartada.Estado = PostulacionDescartada
	archivada := cfg.Convocatoria
	archivada.Estado = EstadoArchivada
	if claves(ValidarDictamenFinal(cfg.Convocatoria, descartada, DictamenSeleccionada)) != "[error.no_esta_en_evaluacion]" ||
		claves(ValidarDictamenFinal(archivada, p, DictamenSeleccionada)) != "[error.fuera_de_evaluacion]" {
		t.Error("el dictamen final sólo se registra sobre lo evaluado y antes de archivar")
	}
}

func TestPermisosDeEvaluacion(t *testing.T) {
	jurado := &Sesion{Usuario: Usuario{ID: 6}, Asignaciones: []Asignacion{{ConvocatoriaID: 1, Perfil: PerfilJurado}}}
	admin := &Sesion{Usuario: Usuario{ID: 1}, Perfiles: []Perfil{PerfilAdminSistema}}
	revisor := &Sesion{Usuario: Usuario{ID: 5}, Asignaciones: []Asignacion{{ConvocatoriaID: 1, Perfil: PerfilRevisorDocumental}}}
	// El administrador NO evalúa (§2.2); el jurado, sólo en su convocatoria.
	if !PuedeEvaluar(jurado, 1) || PuedeEvaluar(jurado, 2) || PuedeEvaluar(admin, 1) || PuedeEvaluar(revisor, 1) || PuedeEvaluar(nil, 1) {
		t.Error("permiso de evaluar incorrecto")
	}
	// El jurado no supervisa ni ve la revisión.
	if PuedeSupervisar(jurado) || PuedeVerRevision(jurado, 1) || PuedeRevisar(jurado, 1) {
		t.Error("un jurado no supervisa ni revisa")
	}
	// Ve sólo las postulaciones que pasaron a evaluación, de su convocatoria.
	for estado, visible := range map[EstadoDePostulacion]bool{
		PostulacionEnEvaluacion: true, PostulacionEvaluada: true,
		PostulacionBorrador: false, PostulacionEnviada: false, PostulacionEnRevision: false, PostulacionEnSubsanacion: false,
		PostulacionValidada: false, PostulacionDescartada: false,
	} {
		if PuedeVerPostulacion(jurado, Postulacion{ConvocatoriaID: 1, UsuarioID: 9, Estado: estado}) != visible {
			t.Errorf("un jurado y una postulación %s: visible debe ser %v", estado, visible)
		}
	}
	if PuedeVerPostulacion(jurado, Postulacion{ConvocatoriaID: 2, UsuarioID: 9, Estado: PostulacionEnEvaluacion}) {
		t.Error("un jurado no ve postulaciones de otra convocatoria")
	}

	// Las postulaciones sin dictaminar NO detienen el paso a evaluación
	// (decisión de la CNMO): se revisan después y se suman al jurado.
	cfg := configuracionCompleta(EstadoRevisionDocumental)
	cfg.SinDictaminar = 3
	if obtenido := claves(ValidarCambioDeEstado(cfg, EstadoEvaluacion, "", ahoraDePrueba)); obtenido != "[]" {
		t.Errorf("con postulaciones sin dictaminar se pasa a evaluación de todos modos: %s", obtenido)
	}
}

// Una excusa por conflicto de interés no perjudica a la postulación: se
// ordena por el promedio entre los jurados que sí calificaron.
func TestExcusaYPuntajeComparable(t *testing.T) {
	for _, c := range []struct {
		total, jurados int
		esperado       float64
	}{
		{24, 3, 24},   // tres jurados: la suma tal cual
		{16, 2, 24},   // dos con el mismo criterio: equivale a 24
		{9, 1, 27},    // uno solo
		{17, 2, 25.5}, // puede no ser entero
		{20, 3, 20},
		{0, 0, 0}, // nadie ha calificado
	} {
		if obtenido := PuntajeComparable(c.total, c.jurados); obtenido != c.esperado {
			t.Errorf("suma %d de %d jurados: %v, se esperaba %v", c.total, c.jurados, obtenido, c.esperado)
		}
	}
	// Con dos jurados que dan 8 y 8, la suma (16) quedaría debajo de una
	// postulación mediocre con tres jurados (6, 6 y 6 = 18). El comparable no.
	if PuntajeComparable(16, 2) <= PuntajeComparable(18, 3) {
		t.Error("una excusa no debe hacer que una postulación mejor calificada quede debajo")
	}

	cfg := configuracionDeEnvio(TipoCompositores)
	cfg.Convocatoria.Estado, cfg.Criterios = EstadoEvaluacion, CriteriosIniciales()
	p := postulacionCompleta(TipoCompositores, "") // obras 10 y 11
	p.Estado = PostulacionEnEvaluacion
	for nombre, c := range map[string]struct {
		obraID int64
		motivo string
		error  string
	}{
		"de una obra":               {10, "Fui su maestro.", "[]"},
		"de toda la postulación":    {0, "Fui su maestro.", "[]"},
		"sin motivo":                {10, "  ", "[error.texto_requerido]"},
		"de una obra que no existe": {99, "x", "[error.unidad_inexistente]"},
	} {
		if obtenido := claves(ValidarExcusa(cfg, p, c.obraID, c.motivo)); obtenido != c.error {
			t.Errorf("excusa %s: %s, se esperaba %s", nombre, obtenido, c.error)
		}
	}
	// En Compositores, sin obra, la excusa cubre todas las obras; en
	// Intérpretes cubre la postulación.
	if u := UnidadesParaExcusa(TipoCompositores, p, 0); len(u) != 2 {
		t.Errorf("excusa de toda la postulación en Compositores: %v", u)
	}
	if u := UnidadesParaExcusa(TipoInterpretes, p, 0); len(u) != 1 || u[0] != 0 {
		t.Errorf("excusa en Intérpretes: %v", u)
	}
	if UnidadesParaExcusa(TipoInterpretes, p, 10) != nil {
		t.Error("en Intérpretes no hay excusa por obra")
	}
	cerrada := cfg
	cerrada.Convocatoria.Estado = EstadoCerrada
	if claves(ValidarExcusa(cerrada, p, 10, "x")) != "[error.fuera_de_evaluacion]" {
		t.Error("cerrada la etapa ya no hay excusas")
	}
	// Con la excusa vigente no se califica: primero hay que retirarla.
	if claves(ValidarSinExcusa(Evaluacion{Excusado: true})) != "[error.excusa_vigente]" || ValidarSinExcusa(Evaluacion{}) != nil {
		t.Error("una unidad con excusa vigente no se califica")
	}
}

// consultaDeEvaluacion reconoce el SQL que lee o escribe las tablas de la
// evaluación.
var consultaDeEvaluacion = regexp.MustCompile(`(?i)\b(FROM|JOIN|INTO|UPDATE)\s+(evaluacion|evaluacion_puntaje|v_resultado)\b`)

// TestAislamientoEstructuralDelRepositorio lee repository.go y comprueba que
// el aislamiento entre jurados siga siendo ESTRUCTURAL (RN-15, §8.4):
//
//  1. En los métodos de JuradoRepo, toda consulta sobre las tablas de
//     evaluación lleva «jurado_id = ?»: no hay forma de leer ni modificar la
//     evaluación de otro jurado. Las inserciones, que no leen, se exceptúan.
//  2. Fuera de JuradoRepo y de SupervisionRepo no hay SQL sobre esas tablas,
//     salvo el conteo de marcarEvaluadaSiCompleta, que no lee puntajes ni
//     observaciones.
//  3. Ningún método de JuradoRepo usa la vista v_resultado.
//
// Si alguien añade mañana una consulta que rompa esto, esta prueba falla.
func TestAislamientoEstructuralDelRepositorio(t *testing.T) {
	archivos := token.NewFileSet()
	archivo, err := parser.ParseFile(archivos, "repository.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consultasDeJurado := 0
	for _, declaracion := range archivo.Decls {
		// receptor es el tipo sobre el que está declarado el método; nombre,
		// el de la función o el de la primera constante del bloque.
		receptor, nombre := "", ""
		switch d := declaracion.(type) {
		case *ast.FuncDecl:
			nombre = d.Name.Name
			if d.Recv != nil && len(d.Recv.List) == 1 {
				tipo := d.Recv.List[0].Type
				if puntero, esPuntero := tipo.(*ast.StarExpr); esPuntero {
					tipo = puntero.X
				}
				if identificador, ok := tipo.(*ast.Ident); ok {
					receptor = identificador.Name
				}
			}
		case *ast.GenDecl:
			nombre = "declaración de paquete"
		}
		ast.Inspect(declaracion, func(nodo ast.Node) bool {
			literal, esLiteral := nodo.(*ast.BasicLit)
			if !esLiteral || literal.Kind != token.STRING {
				return true
			}
			sql, err := strconv.Unquote(literal.Value)
			if err != nil || !consultaDeEvaluacion.MatchString(sql) {
				return true
			}
			lugar := archivos.Position(literal.Pos())
			esInsercion := regexp.MustCompile(`(?i)^\s*INSERT\s+INTO`).MatchString(sql) && !strings.Contains(strings.ToUpper(sql), "SELECT")
			switch {
			case receptor == "SupervisionRepo":
				// Aquí sí se cruzan jurados: es su único lugar.
			case receptor == "JuradoRepo":
				consultasDeJurado++
				if strings.Contains(sql, "v_resultado") {
					t.Errorf("%s: JuradoRepo.%s usa la vista de resultados", lugar, nombre)
				}
				if !esInsercion && !strings.Contains(sql, "jurado_id = ?") {
					t.Errorf("%s: JuradoRepo.%s consulta evaluaciones sin acotar por «jurado_id = ?»:\n%s", lugar, nombre, sql)
				}
			case nombre == "marcarEvaluadaSiCompleta":
				if !strings.Contains(sql, "COUNT(*)") || strings.Contains(sql, "puntaje") || strings.Contains(sql, "observaciones") {
					t.Errorf("%s: marcarEvaluadaSiCompleta sólo puede CONTAR evaluaciones enviadas:\n%s", lugar, sql)
				}
			default:
				t.Errorf("%s: %s consulta las tablas de evaluación fuera de JuradoRepo y SupervisionRepo:\n%s", lugar, nombre, sql)
			}
			return true
		})
	}
	if consultasDeJurado < 4 {
		t.Errorf("la prueba no encontró las consultas de JuradoRepo (%d): ¿cambió su forma?", consultasDeJurado)
	}
}
