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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Prueba de extremo a extremo de la extensión dinámica de nodos (§5.14.1):
// un campo añadido sin recompilar aparece en el formulario, en los reportes
// y en el expediente; lo ve quien debe; no bloquea el envío, no entra en el
// acuse y no se desbloquea en una subsanación; y las tablas probatorias no
// se dejan tocar.

// campoDePrueba añade un campo extendido como lo haría «fimnme-cli schema add».
func (e *entornoPrueba) campoDePrueba(c models.CampoExtendido) models.CampoExtendido {
	e.t.Helper()
	c.Columna = models.ColumnaExtendida(c.Columna)
	creado, err := e.repo.AgregarCampoExtendido(context.Background(), models.Actor{Perfil: models.ActorCLI}, c)
	if err != nil {
		e.t.Fatalf("añadir %s.%s: %v", c.Tabla, c.Columna, err)
	}
	return creado
}

func TestCamposExtendidos(t *testing.T) {
	e := nuevoEntorno(t)
	ctx, cli := context.Background(), models.Actor{Perfil: models.ActorCLI}
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()

	// --- 1. La cerca: sólo la lista blanca, sólo columnas «x_», sólo tipos conocidos.
	for nombre, caso := range map[string]struct {
		campo models.CampoExtendido
		clave string
	}{
		"tabla probatoria":         {models.CampoExtendido{Tabla: "evaluacion", Columna: "x_nota", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota"}, "error.nodo_vedado"},
		"bitácora":                 {models.CampoExtendido{Tabla: "auditoria", Columna: "x_nota", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota"}, "error.nodo_vedado"},
		"tabla fuera de lista":     {models.CampoExtendido{Tabla: "usuario", Columna: "x_nota", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota"}, "error.nodo_no_extensible"},
		"tabla con SQL":            {models.CampoExtendido{Tabla: "obra; DROP TABLE acuse", Columna: "x_nota", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota"}, "error.nodo_no_extensible"},
		"columna sin prefijo":      {models.CampoExtendido{Tabla: "obra", Columna: "titulo", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota"}, "error.nombre_de_campo"},
		"columna con SQL":          {models.CampoExtendido{Tabla: "obra", Columna: "x_a TEXT; DROP TABLE acuse; --", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota"}, "error.nombre_de_campo"},
		"tipo inventado":           {models.CampoExtendido{Tabla: "obra", Columna: "x_nota", Tipo: "BLOB", EtiquetaES: "Nota"}, "error.opcion_invalida"},
		"sin etiqueta":             {models.CampoExtendido{Tabla: "obra", Columna: "x_nota", Tipo: models.ExtendidoTexto}, "error.etiqueta_requerida"},
		"visible en configuración": {models.CampoExtendido{Tabla: "dotacion", Columna: "x_nota", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota", VisibleEn: []models.Perfil{models.PerfilJurado}}, "error.visibilidad_sin_postulacion"},
		"perfil inventado":         {models.CampoExtendido{Tabla: "obra", Columna: "x_nota", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota", VisibleEn: []models.Perfil{"admin_sistema"}}, "error.opcion_invalida"},
	} {
		_, err := e.repo.AgregarCampoExtendido(ctx, cli, caso.campo)
		var regla *models.ErrorValidacion
		if !errors.As(err, &regla) || !slices.ContainsFunc(regla.Errores, func(c models.ErrorCampo) bool { return c.Clave == caso.clave }) {
			t.Errorf("%s: %v, se esperaba %s", nombre, err, caso.clave)
		}
	}
	if asientos := e.asientos("schema.alter"); len(asientos) != 0 {
		t.Fatalf("un intento rechazado dejó DDL en la bitácora: %v", asientos)
	}

	// --- 2. Se añaden cinco campos, uno por caso de uso.
	comoSeEntero := e.campoDePrueba(models.CampoExtendido{Tabla: "datos_solista", Columna: "como_se_entero", Tipo: models.ExtendidoTexto,
		EtiquetaES: "¿Cómo te enteraste?", EtiquetaEN: "How did you hear about us?", Exportable: true,
		VisibleEn: []models.Perfil{models.PerfilPostulante, models.PerfilJurado}})
	e.campoDePrueba(models.CampoExtendido{Tabla: "postulacion", Columna: "nota_interna", Tipo: models.ExtendidoTexto, EtiquetaES: "Nota interna", Exportable: true})
	e.campoDePrueba(models.CampoExtendido{Tabla: "obra", Columna: "num_interpretes", Tipo: models.ExtendidoEntero, EtiquetaES: "Número de intérpretes",
		Exportable: true, VisibleEn: []models.Perfil{models.PerfilPostulante}})
	e.campoDePrueba(models.CampoExtendido{Tabla: "convocatoria", Columna: "sede_clausura", Tipo: models.ExtendidoTexto, EtiquetaES: "Sede de la clausura"})
	e.campoDePrueba(models.CampoExtendido{Tabla: "integrante", Columna: "anio_ingreso", Tipo: models.ExtendidoEntero, EtiquetaES: "Año de ingreso"})
	if comoSeEntero.Ruta() != "ext.datos_solista.x_como_se_entero" {
		t.Errorf("ruta del campo: %s", comoSeEntero.Ruta())
	}
	if _, err := e.repo.AgregarCampoExtendido(ctx, cli, comoSeEntero); !errors.Is(err, models.ErrDuplicado) {
		t.Errorf("añadir dos veces el mismo campo: %v", err)
	}
	// El DDL exacto queda en la bitácora (RF-CLI-27).
	bitacora := fmt.Sprint(e.asientos("schema.alter"))
	for _, ddl := range []string{"ALTER TABLE datos_solista ADD COLUMN x_como_se_entero TEXT", "ALTER TABLE obra ADD COLUMN x_num_interpretes INTEGER",
		"ALTER TABLE postulacion ADD COLUMN x_nota_interna TEXT"} {
		if !strings.Contains(bitacora, ddl) {
			t.Errorf("falta en la bitácora: %s", ddl)
		}
	}

	// --- 3. El postulante: el campo aparece en su formulario y lo captura.
	ana := e.postulanteVerificado("ana@ejemplo.mx")
	ruta, obraID := e.solistaCompleto(ana, interpretes, "ana")
	rutaObra := fmt.Sprintf("/api/v1/obras/%d", obraID)
	propia := ana.exige(http.StatusOK, "GET", ruta, nil)
	extendidos := propia["extendidos"].(obj)
	rutas := func(campos []obj) string {
		var lista []string
		for _, campo := range campos {
			lista = append(lista, campo["ruta"].(string)+":"+campo["captura"].(string))
		}
		return strings.Join(lista, " ")
	}
	if rutas(lista(extendidos, "campos")) != "ext.datos_solista.x_como_se_entero:postulante" || rutas(lista(extendidos, "campos_de_obra")) != "ext.obra.x_num_interpretes:postulante" ||
		lista(extendidos, "campos")[0]["etiqueta_en"] != "How did you hear about us?" {
		t.Fatalf("campos extendidos en el formulario del postulante: %v", extendidos)
	}
	// Sin llenarlos, la postulación ya está lista: un campo extendido nunca es obligatorio (RF-CLI-25).
	if propia["lista_para_enviar"] != true {
		t.Errorf("un campo extendido no debe bloquear el envío: %v", propia["bloqueadores"])
	}
	if guardados, _ := guardar(t, ana, ruta, obj{"ext.datos_solista.x_como_se_entero": "Por la radio"}); guardados != "[ext.datos_solista.x_como_se_entero]" {
		t.Errorf("captura del campo extendido: %s", guardados)
	}
	if guardados, rechazados := guardar(t, ana, rutaObra, obj{"ext.obra.x_num_interpretes": 4, "obra.duracion_min": 12}); guardados != "[ext.obra.x_num_interpretes obra.duracion_min]" || rechazados != "[]" {
		t.Errorf("captura del campo extendido de la obra: %s %s", guardados, rechazados)
	}
	// Lo que no le toca no existe para él; y los tipos se validan.
	for campo, cuerpo := range map[string]obj{
		"ext.postulacion.x_nota_interna":      {"ext.postulacion.x_nota_interna": "Me la invento"},
		"ext.convocatoria.x_sede_clausura":    {"ext.convocatoria.x_sede_clausura": "Mi casa"},
		"ext.datos_solista.x_no_existe":       {"ext.datos_solista.x_no_existe": "x"},
		"ext.datos_solista.password_hash":     {"ext.datos_solista.password_hash": "x"},
		"ext.datos_ensamble.x_como_se_entero": {"ext.datos_ensamble.x_como_se_entero": "x"},
	} {
		if campos := ana.rechaza("PATCH", ruta, obj{"campos": cuerpo}); !slices.Contains(campos, campo) {
			t.Errorf("el postulante escribió %s: %v", campo, campos)
		}
	}
	if campos := ana.rechaza("PATCH", rutaObra, obj{"campos": obj{"ext.obra.x_num_interpretes": "muchos"}}); !slices.Contains(campos, "ext.obra.x_num_interpretes") {
		t.Errorf("un entero que no es entero: %v", campos)
	}
	propia = ana.exige(http.StatusOK, "GET", ruta, nil)
	if valores := propia["extendidos"].(obj)["valores"].(obj); valores["ext.datos_solista.x_como_se_entero"] != "Por la radio" || len(valores) != 1 ||
		lista(propia, "obras")[0]["extendidos"].(obj)["ext.obra.x_num_interpretes"] != "4" {
		t.Errorf("valores extendidos del postulante: %v", propia["extendidos"])
	}

	// --- 4. Envío: el acuse no lleva campos extendidos (RF-CLI-26).
	ana.exige(http.StatusOK, "POST", ruta+"/envio", obj{"confirmo": true, "acepto_bases": true})
	if acuse := textoDePDF(t, ana.bajar(ruta+"/acuses/1").Body.Bytes()); strings.Contains(acuse, "Por la radio") || strings.Contains(acuse, "enteraste") {
		t.Error("el acuse no debe llevar campos extendidos")
	}
	if campos := ana.rechaza("PATCH", ruta, obj{"campos": obj{"ext.datos_solista.x_como_se_entero": "Cambio"}}); len(campos) == 0 {
		t.Error("enviada la postulación, el campo extendido ya no se edita")
	}

	// --- 5. Subsanación: un campo extendido ni se señala ni se desbloquea,
	//        aunque el revisor abra el programa de obras.
	rita := e.revisor(admin, interpretes, "rita@ejemplo.mx")
	var jurado *cliente
	for i := 1; i <= 3; i++ {
		jurado, _ = e.asignado(admin, interpretes, fmt.Sprintf("jurado-%d@ejemplo.mx", i), "jurado")
	}
	observador := e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	admin.estado(interpretes, "revision_documental", "Inicio de la revisión, en la prueba")
	limite := "2027-01-05T05:59:00Z"
	if campos := rita.rechaza("POST", ruta+"/subsanaciones", obj{"fecha_limite": limite,
		"problemas": []obj{{"clase": "campo", "campo": "ext.datos_solista.x_como_se_entero", "problema": "x"}}}); !slices.Contains(campos, "campo") {
		t.Errorf("señalar un campo extendido: %v", campos)
	}
	rita.exige(http.StatusCreated, "POST", ruta+"/subsanaciones", obj{"fecha_limite": limite, "problemas": []obj{
		{"clase": "campo", "campo": "datos.telefono", "problema": "Falta la clave lada."},
		{"clase": "campo", "campo": "obras", "problema": "Falta una obra."}}})
	if guardados, _ := guardar(t, ana, ruta, obj{"datos.telefono": "+52 55 1234 5678"}); guardados != "[datos.telefono]" {
		t.Fatalf("lo señalado sí se edita: %s", guardados)
	}
	if guardados, _ := guardar(t, ana, rutaObra, obj{"obra.duracion_min": 13}); guardados != "[obra.duracion_min]" {
		t.Fatalf("con el programa abierto, la obra se edita: %s", guardados)
	}
	if campos := ana.rechaza("PATCH", ruta, obj{"campos": obj{"ext.datos_solista.x_como_se_entero": "Cambio"}}); !slices.Contains(campos, "ext.datos_solista.x_como_se_entero") {
		t.Errorf("un campo extendido se editó durante la subsanación: %v", campos)
	}
	if campos := ana.rechaza("PATCH", rutaObra, obj{"campos": obj{"ext.obra.x_num_interpretes": 9}}); !slices.Contains(campos, "ext.obra.x_num_interpretes") {
		t.Errorf("el campo extendido de la obra se editó con el programa abierto: %v", campos)
	}
	ana.exige(http.StatusOK, "POST", ruta+"/envio", obj{"confirmo": true, "acepto_bases": true})
	rita.exige(http.StatusOK, "POST", ruta+"/dictamen", obj{"dictamen": "validada"})
	admin.estado(interpretes, "evaluacion", "")

	// --- 6. La administración captura el campo interno y el de configuración.
	postulacionID := id(propia["id"])
	interna := fmt.Sprintf("/api/v1/extendidos/postulacion/%d", postulacionID)
	if respuesta := admin.exige(http.StatusOK, "PUT", interna, obj{"campos": obj{"x_nota_interna": "Revisar camerino"}}); fmt.Sprint(respuesta["guardados"]) != "[ext.postulacion.x_nota_interna]" {
		t.Errorf("captura interna: %v", respuesta)
	}
	// Lo que captura el postulante no se escribe por aquí; ni una columna nativa.
	if campos := admin.rechaza("PUT", fmt.Sprintf("/api/v1/extendidos/datos_solista/%d", postulacionID), obj{"campos": obj{"x_como_se_entero": "Corregido por el admin"}}); len(campos) != 1 {
		t.Errorf("la administración escribió un campo del postulante: %v", campos)
	}
	if campos := admin.rechaza("PUT", interna, obj{"campos": obj{"estado": "evaluada"}}); len(campos) != 1 {
		t.Errorf("la administración escribió una columna nativa: %v", campos)
	}
	sede := fmt.Sprintf("/api/v1/extendidos/convocatoria/%d", interpretes)
	admin.exige(http.StatusOK, "PUT", sede, obj{"campos": obj{"x_sede_clausura": "Sala Manuel M. Ponce"}})
	if valores := observador.exige(http.StatusOK, "GET", sede, nil); valores["valores"].(obj)["x_sede_clausura"] != "Sala Manuel M. Ponce" || len(lista(valores, "campos")) != 1 {
		t.Errorf("campo extendido de la convocatoria: %v", valores)
	}
	// Sólo el administrador captura; filas y tablas que no existen, 404; las
	// listas de la postulación aún no admiten captura.
	observador.exige(http.StatusForbidden, "PUT", sede, obj{"campos": obj{"x_sede_clausura": "Otra"}})
	rita.exige(http.StatusForbidden, "GET", sede, nil)
	ana.exige(http.StatusForbidden, "GET", "/api/v1/extendidos", nil)
	for _, inexistente := range []string{"/api/v1/extendidos/convocatoria/999", "/api/v1/extendidos/usuario/1", "/api/v1/extendidos/evaluacion/1", "/api/v1/extendidos/integrante/1"} {
		admin.exige(http.StatusNotFound, "PUT", inexistente, obj{"campos": obj{"x_sede_clausura": "x"}})
	}
	if diccionario := observador.exige(http.StatusOK, "GET", "/api/v1/extendidos", nil); len(lista(diccionario, "campos")) != 5 || len(diccionario["nodos"].([]any)) != 11 {
		t.Errorf("diccionario: %v", diccionario)
	}

	// --- 7. Cada quien ve lo que el campo declara.
	for nombre, caso := range map[string]struct {
		c             *cliente
		ve, noDebeVer []string
	}{
		"administrador": {admin, []string{"Por la radio", "Revisar camerino"}, nil},
		"observador":    {observador, []string{"Por la radio", "Revisar camerino"}, nil},
		"titular":       {ana, []string{"Por la radio"}, []string{"Revisar camerino", "x_nota_interna"}},
		"jurado":        {jurado, []string{"Por la radio"}, []string{"Revisar camerino", "x_nota_interna", "x_num_interpretes"}},
		"revisora":      {rita, nil, []string{"Por la radio", "Revisar camerino", "x_como_se_entero", "x_nota_interna"}},
	} {
		cuerpo := caso.c.bajar(ruta).Body.String()
		contiene(t, nombre+" (JSON)", cuerpo, caso.ve, caso.noDebeVer)
		expediente := expedienteDe(t, caso.c, ruta, "")
		contiene(t, nombre+" (expediente)", expediente, caso.ve, slices.DeleteFunc(slices.Clone(caso.noDebeVer), func(s string) bool { return strings.HasPrefix(s, "x_") }))
		if (len(caso.ve) > 0) != strings.Contains(expediente, "Datos adicionales") {
			t.Errorf("%s: sección de datos adicionales del expediente", nombre)
		}
	}
	// El número de intérpretes lo ve el titular, con su obra.
	contiene(t, "expediente del titular", expedienteDe(t, ana, ruta, ""), []string{"Número de intérpretes", "¿Cómo te enteraste?"}, nil)

	// --- 8. Reportes: entran solos al catálogo, con su etiqueta (RF-CLI-22).
	reporte := fmt.Sprintf("/api/v1/convocatorias/%d/reporte", interpretes)
	if renglones := csvDe(t, admin, reporte+"?campos=folio,ext.datos_solista.x_como_se_entero,ext.postulacion.x_nota_interna"); len(renglones) != 2 ||
		renglones[0] != "Folio,¿Cómo te enteraste?,Nota interna" || !strings.HasSuffix(renglones[1], ",Por la radio,Revisar camerino") {
		t.Errorf("reporte con campos extendidos: %v", renglones)
	}
	if renglones := csvDe(t, admin, reporte+"?granularidad=obra&idioma=en&campos=obra.titulo,ext.obra.x_num_interpretes,ext.datos_solista.x_como_se_entero"); len(renglones) != 2 ||
		!strings.HasSuffix(renglones[0], ",Número de intérpretes,How did you hear about us?") || !strings.HasSuffix(renglones[1], ",4,Por la radio") {
		t.Errorf("reporte por obra con campos extendidos: %v", renglones)
	}
	if campos := admin.rechaza("GET", reporte+"?campos=folio,ext.obra.x_num_interpretes", nil); !slices.Contains(campos, "granularidad") {
		t.Errorf("un campo extendido de obra pide reporte por obra: %v", campos)
	}

	// --- 9. Anonimizar vacía también los campos extendidos.
	admin.estado(interpretes, "cerrada", "")
	folio := admin.exige(http.StatusOK, "GET", ruta, nil)["folio"]
	admin.exige(http.StatusOK, "POST", ruta+"/anonimizacion", obj{"folio": folio, "fundamento": "Solicitud ARCO 2026/030", "confirmo": true})
	contiene(t, "tras anonimizar", admin.bajar(ruta).Body.String(), nil, []string{"Por la radio", "Revisar camerino"})

	// --- 10. Eliminar: sólo campos del diccionario; nunca una columna nativa.
	for nombre, par := range map[string][2]string{"columna nativa": {"postulacion", "estado"}, "tabla probatoria": {"evaluacion", "x_nota"},
		"campo que no existe": {"obra", "x_no_existe"}, "columna nativa con prefijo inventado": {"postulacion", "x_estado"}} {
		if err := e.repo.EliminarCampoExtendido(ctx, cli, par[0], par[1]); !errors.Is(err, models.ErrNoEncontrado) {
			t.Errorf("eliminar %s: %v", nombre, err)
		}
	}
	if err := e.repo.EliminarCampoExtendido(ctx, cli, "datos_solista", "x_como_se_entero"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(e.asientos("schema.alter")), "ALTER TABLE datos_solista DROP COLUMN x_como_se_entero") {
		t.Error("el DROP COLUMN debe quedar en la bitácora")
	}
	if diccionario := admin.exige(http.StatusOK, "GET", "/api/v1/extendidos", nil); len(lista(diccionario, "campos")) != 4 {
		t.Errorf("tras eliminar: %v", diccionario)
	}
	if campos := admin.rechaza("GET", reporte+"?campos=folio,ext.datos_solista.x_como_se_entero", nil); !slices.Contains(campos, "campos") {
		t.Errorf("el campo eliminado sigue en el catálogo: %v", campos)
	}
	// La postulación sigue leyéndose sin la columna, y el campo puede volver
	// a crearse: reutiliza su registro y nace vacío.
	admin.exige(http.StatusOK, "GET", ruta, nil)
	recreado := e.campoDePrueba(models.CampoExtendido{Tabla: "datos_solista", Columna: "como_se_entero", Tipo: models.ExtendidoTexto, EtiquetaES: "¿Cómo te enteraste?"})
	if recreado.ID != comoSeEntero.ID {
		t.Errorf("al recrearse, el campo debe reutilizar su registro: %d y %d", recreado.ID, comoSeEntero.ID)
	}
}
