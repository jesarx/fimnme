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

// Pruebas de las reglas de la revisión documental y del motor de desbloqueo
// selectivo de la subsanación (§4.2).

// enSubsanacion devuelve una postulación de Compositores ya enviada, con una
// subsanación abierta que señala los problemas dados y vence en una semana.
func enSubsanacion(problemas ...ProblemaSubsanacion) Postulacion {
	p := postulacionCompleta(TipoCompositores, "")
	p.Estado, p.Folio, p.Envios = PostulacionEnSubsanacion, "FIM49-COM-0001", 1
	p.Subsanaciones = []Subsanacion{{ID: 1, FechaLimite: ahoraDePrueba.Add(7 * 24 * time.Hour), EnviadaAt: ahoraDePrueba, Problemas: problemas}}
	return p
}

func campoSenalado(ruta string, obraID int64) ProblemaSubsanacion {
	return ProblemaSubsanacion{Clase: ProblemaCampo, Campo: ruta, ObraID: obraID, Problema: "Corregir."}
}

func documentoSenalado(clave string, obraID int64) ProblemaSubsanacion {
	return ProblemaSubsanacion{Clase: ProblemaDocumento, Campo: clave, ObraID: obraID, Problema: "Ilegible."}
}

// El motor de desbloqueo: en una subsanación es editable EXACTAMENTE lo
// señalado, y nada más (RN-12 y RN-12b).
func TestPermisoDeEdicion(t *testing.T) {
	c := convocatoriaDePrueba(1, TipoCompositores, EstadoRevisionDocumental)

	// Borrador: todo, pero sólo en etapa de postulación y en plazo.
	borrador := postulacionCompleta(TipoCompositores, "")
	if _, errores := PermisoDeEdicionDe(borrador, c, ahoraDePrueba); claves(errores) != "[error.fuera_de_etapa]" {
		t.Errorf("borrador fuera de la etapa de postulación: %s", claves(errores))
	}
	abierta := convocatoriaDePrueba(1, TipoCompositores, EstadoPostulacion)
	enPlazo := time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC)
	total, errores := PermisoDeEdicionDe(borrador, abierta, enPlazo)
	if len(errores) != 0 || !total.Total || !total.Campo("datos.semblanza", 0) || !total.Campo(CampoObras, 0) || !total.Documento("partitura", 10) {
		t.Errorf("un borrador en plazo es editable por completo: %+v %s", total, claves(errores))
	}

	// Enviada, en revisión, validada, descartada: nada.
	for _, estado := range []EstadoDePostulacion{PostulacionEnviada, PostulacionEnRevision, PostulacionValidada, PostulacionDescartada, PostulacionEnEvaluacion, PostulacionEvaluada} {
		p := borrador
		p.Estado = estado
		if permiso, errores := PermisoDeEdicionDe(p, abierta, enPlazo); claves(errores) != "[error.postulacion_enviada]" || permiso.Campo("datos.semblanza", 0) {
			t.Errorf("una postulación %s no es editable: %s", estado, claves(errores))
		}
	}

	// En subsanación: sólo lo señalado. La etapa de la convocatoria ya no
	// importa: manda el plazo de la subsanación.
	p := enSubsanacion(campoSenalado("datos.telefono", 0), campoSenalado("obra.duracion_min", 10), documentoSenalado("partitura", 11))
	permiso, errores := PermisoDeEdicionDe(p, c, ahoraDePrueba)
	if len(errores) != 0 || permiso.Total {
		t.Fatalf("subsanación abierta: %+v %s", permiso, claves(errores))
	}
	editables := map[string]bool{
		"campo datos.telefono 0":     permiso.Campo("datos.telefono", 0),
		"campo obra.duracion_min 10": permiso.Campo("obra.duracion_min", 10),
		"documento partitura 11":     permiso.Documento("partitura", 11),
	}
	bloqueados := map[string]bool{
		"otro campo":                       permiso.Campo("datos.semblanza", 0),
		"el mismo campo en otra obra":      permiso.Campo("obra.duracion_min", 11),
		"otro campo de la misma obra":      permiso.Campo("obra.titulo", 10),
		"el mismo documento en otra obra":  permiso.Documento("partitura", 10),
		"otro documento de la misma obra":  permiso.Documento("audio", 11),
		"la fotografía":                    permiso.Documento("fotografia", 0),
		"un documento con nombre de campo": permiso.Documento("datos.telefono", 0),
		"un campo con nombre de documento": permiso.Campo("partitura", 11),
		"la lista de obras":                permiso.Campo(CampoObras, 0),
		"la lista de integrantes":          permiso.Campo(CampoIntegrantes, 0),
		"la lista de videos":               permiso.Campo(CampoVideos, 0),
	}
	for nombre, editable := range editables {
		if !editable {
			t.Errorf("lo señalado debe ser editable: %s", nombre)
		}
	}
	for nombre, editable := range bloqueados {
		if editable {
			t.Errorf("lo NO señalado debe seguir bloqueado: %s", nombre)
		}
	}

	// Si el revisor autoriza cambiar el programa (RN-12c), las obras pueden
	// añadirse, quitarse y describirse; los datos generales siguen cerrados.
	programa, _ := PermisoDeEdicionDe(enSubsanacion(campoSenalado(CampoObras, 0)), c, ahoraDePrueba)
	if !programa.Campo(CampoObras, 0) || !programa.Campo("obra.titulo", 10) || !programa.Documento("partitura", 99) ||
		programa.Campo("datos.semblanza", 0) || programa.Documento("fotografia", 0) {
		t.Error("con el programa abierto son editables las obras y sus documentos, y nada más")
	}

	// Vencido el plazo, nada (RN-02); y la postulación queda marcada para el revisor.
	vencida := ahoraDePrueba.Add(7*24*time.Hour + time.Second)
	if _, errores := PermisoDeEdicionDe(p, c, vencida); claves(errores) != "[error.subsanacion_vencida]" {
		t.Errorf("subsanación vencida: %s", claves(errores))
	}
	if !p.SubsanacionVencida(vencida) || p.SubsanacionVencida(ahoraDePrueba) || p.SubsanacionAbierta(vencida) != nil {
		t.Error("el vencimiento de la subsanación está mal calculado")
	}
	// En el instante exacto del límite todavía se puede.
	if _, errores := PermisoDeEdicionDe(p, c, ahoraDePrueba.Add(7*24*time.Hour)); len(errores) != 0 {
		t.Errorf("en el instante límite aún es editable: %s", claves(errores))
	}
	// Una subsanación ya atendida o cerrada no desbloquea nada.
	for _, cerrar := range []func(*Subsanacion){
		func(s *Subsanacion) { s.AtendidaAt = ahoraDePrueba },
		func(s *Subsanacion) { s.CerradaAt = ahoraDePrueba },
	} {
		otra := enSubsanacion(campoSenalado("datos.telefono", 0))
		cerrar(&otra.Subsanaciones[0])
		if _, errores := PermisoDeEdicionDe(otra, c, ahoraDePrueba); claves(errores) != "[error.subsanacion_vencida]" {
			t.Errorf("una subsanación atendida o cerrada no desbloquea: %s", claves(errores))
		}
	}
}

func TestPermisosDeRevision(t *testing.T) {
	revisor := &Sesion{Usuario: Usuario{ID: 5}, Asignaciones: []Asignacion{{ConvocatoriaID: 1, Perfil: PerfilRevisorDocumental}}}
	jurado := &Sesion{Usuario: Usuario{ID: 6}, Asignaciones: []Asignacion{{ConvocatoriaID: 1, Perfil: PerfilJurado}}}
	admin := &Sesion{Usuario: Usuario{ID: 1}, Perfiles: []Perfil{PerfilAdminSistema}}
	observador := &Sesion{Usuario: Usuario{ID: 2}, Perfiles: []Perfil{PerfilCoordinacionNacional}}
	titular := &Sesion{Usuario: Usuario{ID: 7}, Perfiles: []Perfil{PerfilPostulante}}
	enviada := Postulacion{ConvocatoriaID: 1, UsuarioID: 7, Estado: PostulacionEnviada}
	borrador := Postulacion{ConvocatoriaID: 1, UsuarioID: 7, Estado: PostulacionBorrador}
	deOtra := Postulacion{ConvocatoriaID: 2, UsuarioID: 7, Estado: PostulacionEnviada}

	casos := []struct {
		nombre                string
		sesion                *Sesion
		revisa, veRevision    bool
		veEnviada, veBorrador bool
		veDeOtraConvocatoria  bool
	}{
		{"revisor de la convocatoria", revisor, true, true, true, false, false},
		{"administrador", admin, true, true, true, true, true},
		{"observador", observador, false, true, true, true, true},
		{"titular", titular, false, false, true, true, true},
		{"jurado (su acceso llega en el paso 8)", jurado, false, false, false, false, false},
		{"sin sesión", nil, false, false, false, false, false},
	}
	for _, c := range casos {
		obtenido := []bool{PuedeRevisar(c.sesion, 1), PuedeVerRevision(c.sesion, 1), PuedeVerPostulacion(c.sesion, enviada),
			PuedeVerPostulacion(c.sesion, borrador), PuedeVerPostulacion(c.sesion, deOtra)}
		esperado := []bool{c.revisa, c.veRevision, c.veEnviada, c.veBorrador, c.veDeOtraConvocatoria}
		if fmt.Sprint(obtenido) != fmt.Sprint(esperado) {
			t.Errorf("%s: %v, se esperaba %v", c.nombre, obtenido, esperado)
		}
	}
}

func TestReglasDeLaSubsanacion(t *testing.T) {
	cfg := configuracionDeEnvio(TipoCompositores)
	cfg.Convocatoria.Estado = EstadoRevisionDocumental
	p := postulacionCompleta(TipoCompositores, "") // obras 10 y 11
	p.Estado = PostulacionEnRevision
	limite := ahoraDePrueba.Add(10 * 24 * time.Hour)
	valida := NuevaSubsanacion{FechaLimite: limite, Problemas: []ProblemaSubsanacion{campoSenalado("datos.telefono", 0)}}
	con := func(cambio func(*NuevaSubsanacion)) NuevaSubsanacion {
		n := valida
		n.Problemas = append([]ProblemaSubsanacion{}, valida.Problemas...)
		cambio(&n)
		return n
	}
	casos := map[string]struct {
		n     NuevaSubsanacion
		error string
	}{
		"válida": {valida, "[]"},
		"campo de obra, documento, documento faltante y listas": {con(func(n *NuevaSubsanacion) {
			n.Problemas = []ProblemaSubsanacion{campoSenalado("obra.duracion_min", 11), documentoSenalado("audio", 11),
				documentoSenalado("fotografia", 0), campoSenalado(CampoObras, 0)}
		}), "[]"},
		"sin problemas":                             {con(func(n *NuevaSubsanacion) { n.Problemas = nil }), "[error.sin_problemas]"},
		"problema sin motivo":                       {con(func(n *NuevaSubsanacion) { n.Problemas[0].Problema = "  " }), "[error.texto_requerido]"},
		"campo que no existe":                       {con(func(n *NuevaSubsanacion) { n.Problemas[0].Campo = "datos.curp" }), "[error.elemento_inexistente]"},
		"campo de postulación atribuido a una obra": {con(func(n *NuevaSubsanacion) { n.Problemas[0].ObraID = 10 }), "[error.elemento_inexistente]"},
		"campo de obra sin obra":                    {con(func(n *NuevaSubsanacion) { n.Problemas[0].Campo = "obra.titulo" }), "[error.elemento_inexistente]"},
		"obra de otra postulación":                  {con(func(n *NuevaSubsanacion) { n.Problemas[0] = campoSenalado("obra.titulo", 99) }), "[error.obra_ajena]"},
		"requisito que no se pide":                  {con(func(n *NuevaSubsanacion) { n.Problemas[0] = documentoSenalado("curriculum", 0) }), "[error.elemento_inexistente]"},
		"integrantes en Compositores":               {con(func(n *NuevaSubsanacion) { n.Problemas[0] = campoSenalado(CampoIntegrantes, 0) }), "[error.elemento_inexistente]"},
		"videos en Compositores":                    {con(func(n *NuevaSubsanacion) { n.Problemas[0] = campoSenalado(CampoVideos, 0) }), "[error.elemento_inexistente]"},
		"clase inventada":                           {con(func(n *NuevaSubsanacion) { n.Problemas[0].Clase = "obra" }), "[error.elemento_inexistente]"},
		"el mismo elemento dos veces":               {con(func(n *NuevaSubsanacion) { n.Problemas = append(n.Problemas, n.Problemas[0]) }), "[error.problema_repetido]"},
		"fecha límite pasada":                       {con(func(n *NuevaSubsanacion) { n.FechaLimite = ahoraDePrueba }), "[error.fecha_limite_invalida]"},
		"fecha límite a un año":                     {con(func(n *NuevaSubsanacion) { n.FechaLimite = ahoraDePrueba.Add(365 * 24 * time.Hour) }), "[error.fecha_limite_invalida]"},
		"nota larguísima":                           {con(func(n *NuevaSubsanacion) { n.Nota = strings.Repeat("a", LongitudMaxMotivo+1) }), "[error.texto_largo]"},
	}
	for nombre, c := range casos {
		if obtenido := claves(ValidarNuevaSubsanacion(p, cfg, c.n, ahoraDePrueba)); obtenido != c.error {
			t.Errorf("%s: %s, se esperaba %s", nombre, obtenido, c.error)
		}
	}
	// Fuera de la etapa de revisión, o con la postulación en otro estado, no se solicita.
	fuera := cfg
	fuera.Convocatoria.Estado = EstadoEvaluacion
	if obtenido := claves(ValidarNuevaSubsanacion(p, fuera, valida, ahoraDePrueba)); obtenido != "[error.fuera_de_revision]" {
		t.Errorf("fuera de la etapa de revisión: %s", obtenido)
	}
	for _, estado := range []EstadoDePostulacion{PostulacionBorrador, PostulacionEnSubsanacion, PostulacionValidada, PostulacionDescartada} {
		otra := p
		otra.Estado = estado
		if obtenido := claves(ValidarNuevaSubsanacion(otra, cfg, valida, ahoraDePrueba)); obtenido != "[error.estado_no_revisable]" {
			t.Errorf("subsanación de una postulación %s: %s", estado, obtenido)
		}
	}

	// Los documentos dictaminados inválidos se añaden solos, con su motivo, y
	// no se duplican si el revisor ya los señaló a mano.
	p.Documentos[1].ID, p.Documentos[1].Dictamen = 501, "invalido" // partitura de la obra 10
	p.Documentos[2].ID, p.Documentos[2].Dictamen = 502, "invalido" // particellas de la obra 10
	p.Documentos[0].Dictamen = "valido"
	revision := Revision{Dictamenes: []DictamenDocumento{{DocumentoID: 501, Motivo: "Faltan páginas."}, {DocumentoID: 502, Motivo: "Ilegible."}}}
	problemas := ProblemasDeSubsanacion(p, revision, []ProblemaSubsanacion{campoSenalado("datos.telefono", 0), documentoSenalado("particellas", 10)})
	if len(problemas) != 3 || problemas[2].Campo != "partitura" || problemas[2].ObraID != 10 || problemas[2].DocumentoID != 501 ||
		problemas[2].Problema != "Faltan páginas." || problemas[1].Problema != "Ilegible." {
		t.Errorf("problemas reunidos: %+v", problemas)
	}

	// Dictamen de un documento (RN-11).
	for dictamen, c := range map[string]struct{ motivo, error string }{
		"valido": {"", "[]"}, "pendiente": {"", "[]"}, "invalido": {"", "[error.texto_requerido]"}, "dudoso": {"x", "[error.opcion_invalida]"},
	} {
		if obtenido := claves(ValidarDictamenDeDocumento(dictamen, c.motivo)); obtenido != c.error {
			t.Errorf("dictamen %q: %s, se esperaba %s", dictamen, obtenido, c.error)
		}
	}
	if ValidarDictamenDeDocumento("invalido", "Está en blanco.") != nil {
		t.Error("un documento inválido con motivo es un dictamen correcto")
	}
}

func TestDictamenDePostulacion(t *testing.T) {
	c := convocatoriaDePrueba(1, TipoCompositores, EstadoRevisionDocumental)
	p := postulacionCompleta(TipoCompositores, "")
	p.Estado = PostulacionEnRevision
	conInvalido := p
	conInvalido.Documentos = append([]Documento{}, p.Documentos...)
	conInvalido.Documentos[0].Dictamen = "invalido"
	subsanando := enSubsanacion(campoSenalado("datos.telefono", 0))
	vencida := ahoraDePrueba.Add(8 * 24 * time.Hour)
	fuera := convocatoriaDePrueba(1, TipoCompositores, EstadoEvaluacion)
	casos := []struct {
		nombre   string
		c        Convocatoria
		p        Postulacion
		dictamen EstadoDePostulacion
		motivo   string
		ahora    time.Time
		error    string
	}{
		{"validar", c, p, PostulacionValidada, "", ahoraDePrueba, "[]"},
		{"descartar con motivo", c, p, PostulacionDescartada, "No cumple la base 3.", ahoraDePrueba, "[]"},
		{"descartar sin motivo", c, p, PostulacionDescartada, " ", ahoraDePrueba, "[error.texto_requerido]"},
		{"dictamen inventado", c, p, PostulacionEvaluada, "", ahoraDePrueba, "[error.opcion_invalida]"},
		{"validar con un documento inválido", c, conInvalido, PostulacionValidada, "", ahoraDePrueba, "[error.documentos_invalidos]"},
		{"descartar con un documento inválido", c, conInvalido, PostulacionDescartada, "No subsanó.", ahoraDePrueba, "[]"},
		{"con la subsanación en curso", c, subsanando, PostulacionDescartada, "No subsanó.", ahoraDePrueba, "[error.subsanacion_en_curso]"},
		{"con la subsanación vencida", c, subsanando, PostulacionDescartada, "No subsanó.", vencida, "[]"},
		{"fuera de la etapa de revisión", fuera, p, PostulacionValidada, "", ahoraDePrueba, "[error.fuera_de_revision]"},
	}
	for _, caso := range casos {
		if obtenido := claves(ValidarDictamenDePostulacion(caso.c, caso.p, caso.dictamen, caso.motivo, caso.ahora)); obtenido != caso.error {
			t.Errorf("%s: %s, se esperaba %s", caso.nombre, obtenido, caso.error)
		}
	}
	// Un dictamen ya emitido puede corregirse; un borrador no se dictamina.
	for estado, esperado := range map[EstadoDePostulacion]string{
		PostulacionEnviada: "[]", PostulacionValidada: "[]", PostulacionDescartada: "[]",
		PostulacionBorrador: "[error.estado_no_revisable]", PostulacionEnEvaluacion: "[error.estado_no_revisable]",
	} {
		otra := p
		otra.Estado = estado
		if obtenido := claves(ValidarDictamenDePostulacion(c, otra, PostulacionValidada, "", ahoraDePrueba)); obtenido != esperado {
			t.Errorf("dictaminar una postulación %s: %s, se esperaba %s", estado, obtenido, esperado)
		}
	}
}

func TestSolicitudDeDesbloqueo(t *testing.T) {
	cfg := configuracionDeEnvio(TipoCompositores)
	p := enSubsanacion(campoSenalado("datos.telefono", 0))
	p.Subsanaciones[0].Solicitudes = []SolicitudDesbloqueo{
		{ID: 1, Clase: ProblemaCampo, Campo: "datos.semblanza", Justificacion: "x"},
		{ID: 2, Clase: ProblemaCampo, Campo: "datos.genero", Justificacion: "x", Resuelta: true}, // ya negada: puede volver a pedirse
	}
	pedir := func(clase ClaseProblema, campo string, obraID int64, justificacion string) string {
		return claves(ValidarSolicitudDeDesbloqueo(p, cfg, SolicitudDesbloqueo{Clase: clase, Campo: campo, ObraID: obraID, Justificacion: justificacion}, ahoraDePrueba))
	}
	for nombre, c := range map[string]struct{ obtenido, esperado string }{
		"un campo no señalado":          {pedir(ProblemaCampo, "datos.email_contacto", 0, "Tiene una letra de más."), "[]"},
		"un documento":                  {pedir(ProblemaDocumento, "partitura", 10, "Subí la versión anterior."), "[]"},
		"una solicitud ya negada":       {pedir(ProblemaCampo, "datos.genero", 0, "Insisto."), "[]"},
		"sin justificación":             {pedir(ProblemaCampo, "datos.email_contacto", 0, ""), "[error.texto_requerido]"},
		"algo ya desbloqueado":          {pedir(ProblemaCampo, "datos.telefono", 0, "x"), "[error.ya_desbloqueado]"},
		"algo ya pedido y sin resolver": {pedir(ProblemaCampo, "datos.semblanza", 0, "x"), "[error.solicitud_repetida]"},
		"un campo que no existe":        {pedir(ProblemaCampo, "datos.curp", 0, "x"), "[error.elemento_inexistente]"},
		"una obra ajena":                {pedir(ProblemaCampo, "obra.titulo", 99, "x"), "[error.obra_ajena]"},
	} {
		if c.obtenido != c.esperado {
			t.Errorf("%s: %s, se esperaba %s", nombre, c.obtenido, c.esperado)
		}
	}
	// Sin subsanación abierta no hay nada que pedir.
	enviada := postulacionCompleta(TipoCompositores, "")
	enviada.Estado = PostulacionEnRevision
	if obtenido := claves(ValidarSolicitudDeDesbloqueo(enviada, cfg, SolicitudDesbloqueo{Clase: ProblemaCampo, Campo: "datos.telefono", Justificacion: "x"}, ahoraDePrueba)); obtenido != "[error.sin_subsanacion_abierta]" {
		t.Errorf("solicitud sin subsanación abierta: %s", obtenido)
	}
	// Negar exige motivo; conceder, no.
	if claves(ValidarRespuestaDeDesbloqueo(false, "")) != "[error.texto_requerido]" || ValidarRespuestaDeDesbloqueo(false, "No procede.") != nil ||
		ValidarRespuestaDeDesbloqueo(true, "") != nil {
		t.Error("respuesta a una solicitud de desbloqueo incorrecta")
	}
}

// El diff entre dos envíos: lo que el revisor ve al volver una postulación.
func TestDiferenciasDeEnvio(t *testing.T) {
	cfg := configuracionDeEnvio(TipoInterpretes)
	p := postulacionCompleta(TipoInterpretes, SubcategoriaEnsamble) // obra 10
	for i := range p.Documentos {
		p.Documentos[i].Archivo.SHA256 = strings.Repeat(fmt.Sprint(i), 64)
	}
	titular := Usuario{Idioma: "es"}
	anterior := SnapshotDeEnvio(p, cfg, titular, "FIM49-INT-0001", 1, ahoraDePrueba)
	if cambios := DiferenciasDeEnvio(anterior, anterior); len(cambios) != 0 {
		t.Fatalf("dos envíos iguales no tienen diferencias: %+v", cambios)
	}

	p.Campos["datos.telefono"] = "55 0000 0000"      // cambia
	delete(p.Campos, "datos.dotacion_texto")         // se vacía
	p.Campos["concierto.sede_nombre"] = "Sala Ponce" // aparece
	p.Integrantes = append(p.Integrantes, Integrante{"Eva Paz", "viola"})
	p.Videos[0].URL = "https://vimeo.com/2"
	p.Obras[0].Campos = map[string]string{"obra.titulo": "Obra", "obra.duracion_min": "15"}
	p.Obras = append(p.Obras, Obra{ID: 12, Orden: 2, Campos: map[string]string{"obra.titulo": "Obra nueva"}})
	p.Documentos[1].Archivo.SHA256 = strings.Repeat("f", 64) // partitura reemplazada
	p.Documentos = append(p.Documentos, Documento{Clave: "partitura", ObraID: 12, Archivo: Archivo{SHA256: strings.Repeat("a", 64)}})
	nuevo := SnapshotDeEnvio(p, cfg, titular, "FIM49-INT-0001", 2, ahoraDePrueba)

	var resumen []string
	for _, c := range DiferenciasDeEnvio(anterior, nuevo) {
		resumen = append(resumen, fmt.Sprintf("%s %s#%d [%s→%s]", c.Clase, c.Campo, c.ObraID, c.Antes, c.Despues))
	}
	esperado := []string{
		"campo concierto.sede_nombre#0 [→Sala Ponce]",
		"campo datos.dotacion_texto#0 [Cuarteto de cuerdas→]",
		"campo datos.telefono#0 [55 1234 5678→55 0000 0000]",
		"campo integrantes#0 [Ana López (violín); Luis Ruiz (violonchelo)→Ana López (violín); Luis Ruiz (violonchelo); Eva Paz (viola)]",
		"campo videos#0 [https://vimeo.com/1→https://vimeo.com/2]",
		"campo obra.compositor#10 [Julián Carrillo→]",
		"campo obra.duracion_min#10 [12→15]",
	}
	for _, linea := range esperado {
		if !strings.Contains(strings.Join(resumen, "\n"), linea) {
			t.Errorf("falta la diferencia %q en:\n%s", linea, strings.Join(resumen, "\n"))
		}
	}
	unido := strings.Join(resumen, "\n")
	if !strings.Contains(unido, "campo obras#12 [→Obra nueva]") || !strings.Contains(unido, "documento partitura#10 [111111111111→ffffffffffff]") ||
		!strings.Contains(unido, "documento partitura#12 [→aaaaaaaaaaaa]") || strings.Contains(unido, "fotografia") || strings.Contains(unido, "datos.semblanza") {
		t.Errorf("diferencias de obras y documentos:\n%s", unido)
	}
	// Al revés: una obra que desaparece.
	if cambios := DiferenciasDeEnvio(nuevo, anterior); !strings.Contains(fmt.Sprint(cambios), "{campo obras 12 Obra nueva }") {
		t.Errorf("una obra retirada debe aparecer como cambio: %+v", cambios)
	}
}
