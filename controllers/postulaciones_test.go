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
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de extremo a extremo de la captura de postulaciones (paso 5): cada
// una recorre la API como lo haría el formulario, con su sesión y su cookie.

// convocatoriasAbiertas prepara, con la API de administración, una edición con
// sus dos convocatorias en etapa de postulación y dentro de plazo. La de
// Compositores lleva dos dotaciones: un cuarteto, con las reglas documentales
// generales, y una obra electroacústica, que no pide particellas y exige audio.
//
// El reloj queda en el 25 de noviembre de 2026; el plazo cierra el 20 de
// diciembre.
func (e *entornoPrueba) convocatoriasAbiertas() (admin *cliente, interpretes, compositores, cuarteto, electro int64) {
	e.t.Helper()
	e.hasta("2026-11-25T18:00:00Z")
	admin = e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	edicionID, compositores := e.compositores2028(admin)
	c := admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/ediciones/%d/convocatorias", edicionID),
		obj{"tipo": "interpretes", "titulo_es": "Convocatoria de Intérpretes 2028", "bases_md_es": "# Bases\n\nTexto."})
	interpretes = id(c["id"])
	admin.exige(http.StatusOK, "PUT", fmt.Sprintf("/api/v1/convocatorias/%d/etapas", interpretes), etapasDePrueba())

	ruta := fmt.Sprintf("/api/v1/convocatorias/%d", compositores)
	c = admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Cuarteto de cuerdas", "nombre_en": "String quartet", "orden": 1})
	cuarteto = id(lista(c, "dotaciones")[0]["id"])
	c = admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Obra electroacústica", "orden": 2})
	electro = id(lista(c, "dotaciones")[1]["id"])
	admin.exige(http.StatusCreated, "POST", ruta+"/requisitos", obj{"dotacion_id": electro, "clave": "particellas", "etiqueta_es": "Particellas",
		"ambito": "obra", "obligatorio": false, "mimes_permitidos": []string{"application/pdf"}, "tamano_max_bytes": 30 << 20})
	admin.exige(http.StatusCreated, "POST", ruta+"/requisitos", obj{"dotacion_id": electro, "clave": "audio", "etiqueta_es": "Archivo de audio",
		"etiqueta_en": "Audio file", "ambito": "obra", "obligatorio": true, "mimes_permitidos": []string{"audio/wav"}, "tamano_max_bytes": 100 << 20})

	for _, convocatoriaID := range []int64{interpretes, compositores} {
		admin.estado(convocatoriaID, "publicada", "")
		admin.estado(convocatoriaID, "postulacion", "")
	}
	return admin, interpretes, compositores, cuarteto, electro
}

// postulante crea una cuenta de postulante y devuelve su cliente con sesión.
func (e *entornoPrueba) postulante(email string) *cliente {
	e.t.Helper()
	e.crearCuenta(email, models.PerfilPostulante)
	c := e.cliente()
	c.ingresar(email, passwordPrueba)
	return c
}

// faltantes resume los bloqueadores de una postulación como
// «sección/campo[#obra]», ordenados, y comprueba que cada uno venga traducido.
func faltantes(t *testing.T, postulacion obj) string {
	t.Helper()
	var resumen []string
	for _, b := range lista(postulacion, "bloqueadores") {
		mensaje := b["mensaje"].(string)
		if mensaje == "" || strings.HasPrefix(mensaje, "bloqueo.") || strings.Contains(mensaje, "%s") || strings.Contains(mensaje, "campo.") {
			t.Errorf("bloqueador sin traducir: %v", b)
		}
		campo := b["campo"].(string)
		if b["obra_id"] != nil {
			campo += fmt.Sprintf("#%d", id(b["obra_id"]))
		}
		resumen = append(resumen, b["seccion"].(string)+"/"+campo)
	}
	slices.Sort(resumen)
	return strings.Join(resumen, " ")
}

// guardar hace un autoguardado y devuelve los campos guardados y rechazados.
func guardar(t *testing.T, c *cliente, ruta string, campos obj) (guardados, rechazados string) {
	t.Helper()
	respuesta := c.exige(http.StatusOK, "PATCH", ruta, obj{"campos": campos})
	if respuesta["guardado_at"] == nil {
		t.Errorf("PATCH %s: falta guardado_at", ruta)
	}
	var errores []string
	for _, e := range lista(respuesta, "errores") {
		errores = append(errores, e["campo"].(string))
	}
	return fmt.Sprint(respuesta["guardados"]), fmt.Sprint(errores)
}

// seccionesPendientes devuelve «sección=pendientes» de cada sección.
func seccionesPendientes(postulacion obj) string {
	var resumen []string
	for _, s := range lista(postulacion, "secciones") {
		resumen = append(resumen, fmt.Sprintf("%s=%d", s["seccion"], id(s["pendientes"])))
	}
	return strings.Join(resumen, " ")
}

// Datos completos de un solista y de una propuesta de concierto en la Ciudad
// de México.
func datosDeSolista() obj {
	return obj{
		"datos.nombre_completo": "Ana López Ruiz", "datos.nombre_artistico": "Ana L.", "datos.genero": "mujer",
		"datos.fecha_nacimiento": "1990-05-17", "datos.telefono": "55 1234 5678", "datos.email_contacto": "ana@ejemplo.mx",
		"datos.nacionalidad": "MX", "datos.pais_residencia": "MX", "datos.entidad_residencia": "JAL", "datos.semblanza": "Semblanza de Ana.",
		"concierto.requerimientos_tecnicos": "Piano de cola.", "concierto.entidad": "CDMX",
	}
}

func TestCatalogosPublicos(t *testing.T) {
	e := nuevoEntorno(t)
	catalogos := e.cliente().exige(http.StatusOK, "GET", "/api/v1/catalogos", nil)
	if len(catalogos["paises"].([]any)) != 249 || len(lista(catalogos, "entidades")) != 32 ||
		catalogos["entidad_sin_sede"] != "CDMX" || catalogos["pais_con_catalogo_de_entidades"] != "MX" ||
		fmt.Sprint(catalogos["subcategorias"]) != "[solista ensamble]" || len(catalogos["generos"].([]any)) == 0 || len(catalogos["estrenos"].([]any)) == 0 {
		t.Errorf("catálogos inesperados: %v", catalogos)
	}
}

// Un solista captura su postulación en varias sesiones, con autoguardado.
func TestPostulacionDeSolista(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()
	rutaConvocatoria := fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes)

	// Sin sesión no hay postulaciones.
	anonimo := e.cliente()
	for _, ruta := range []string{rutaConvocatoria, "/api/v1/postulaciones", "/api/v1/postulaciones/1"} {
		metodo := "GET"
		if ruta == rutaConvocatoria {
			metodo = "POST"
		}
		if estado, r := anonimo.api(metodo, ruta, obj{}); estado != http.StatusUnauthorized {
			t.Errorf("%s %s sin sesión: %d %v", metodo, ruta, estado, r)
		}
	}

	ana := e.postulante("ana@ejemplo.mx")
	if r := ana.exige(http.StatusOK, "GET", "/api/v1/postulaciones", nil); len(lista(r, "postulaciones")) != 0 {
		t.Errorf("la cuenta nueva no debería tener postulaciones: %v", r)
	}

	// En Intérpretes la subcategoría es obligatoria desde el alta (RF-POST-02).
	if campos := ana.rechaza("POST", rutaConvocatoria, obj{}); fmt.Sprint(campos) != "[subcategoria]" {
		t.Errorf("alta sin subcategoría: %v", campos)
	}
	if campos := ana.rechaza("POST", rutaConvocatoria, obj{"subcategoria": "coro"}); fmt.Sprint(campos) != "[subcategoria]" {
		t.Errorf("alta con subcategoría inventada: %v", campos)
	}
	if estado, _ := ana.api("POST", "/api/v1/convocatorias/9999/postulaciones", obj{"subcategoria": "solista"}); estado != http.StatusNotFound {
		t.Errorf("convocatoria inexistente: %d", estado)
	}

	p := ana.exige(http.StatusCreated, "POST", rutaConvocatoria, obj{"subcategoria": "solista"})
	postulacionID := id(p["id"])
	ruta := fmt.Sprintf("/api/v1/postulaciones/%d", postulacionID)
	if p["estado"] != "borrador" || p["subcategoria"] != "solista" || p["folio"] != nil || p["editable"] != true || p["lista_para_enviar"] != false ||
		p["convocatoria"].(obj)["tipo"] != "interpretes" || id(p["convocatoria"].(obj)["id"]) != interpretes {
		t.Errorf("borrador inesperado: %v", p)
	}
	definicion := p["definicion"].(obj)
	if definicion["tiene_integrantes"] != false || definicion["tiene_videos"] != true || len(lista(definicion, "campos")) < 12 || len(lista(definicion, "campos_de_obra")) < 6 {
		t.Errorf("definición del formulario inesperada: %v", definicion)
	}
	for _, campo := range append(lista(definicion, "campos"), lista(definicion, "campos_de_obra")...) {
		if etiqueta := campo["etiqueta"].(string); etiqueta == "" || strings.HasPrefix(etiqueta, "campo.") {
			t.Errorf("campo sin etiqueta: %v", campo)
		}
	}
	// Recién creada le falta todo: los nueve datos obligatorios del solista,
	// dos del concierto, los videos, las obras, la fotografía y verificar el correo.
	if s := seccionesPendientes(p); s != "datos=10 concierto=2 obras=1 documentos=1 envio=1" {
		t.Errorf("pendientes por sección: %s", s)
	}

	// Una postulación por convocatoria y cuenta (RN-01).
	if estado, r := ana.api("POST", rutaConvocatoria, obj{"subcategoria": "ensamble"}); estado != http.StatusConflict {
		t.Errorf("segunda postulación en la misma convocatoria: %d %v", estado, r)
	}
	misPostulaciones := lista(ana.exige(http.StatusOK, "GET", "/api/v1/postulaciones", nil), "postulaciones")
	if len(misPostulaciones) != 1 || id(misPostulaciones[0]["id"]) != postulacionID || misPostulaciones[0]["estado"] != "borrador" {
		t.Errorf("mis postulaciones: %v", misPostulaciones)
	}

	// Autoguardado de un campo; luego de varios, con uno inválido y uno ajeno
	// al formulario: se guarda lo válido y se señala lo demás.
	if guardados, rechazados := guardar(t, ana, ruta, obj{"datos.telefono": "  55 1234 5678  "}); guardados != "[datos.telefono]" || rechazados != "[]" {
		t.Errorf("autoguardado de un campo: %s %s", guardados, rechazados)
	}
	guardados, rechazados := guardar(t, ana, ruta, obj{
		"datos.nombre_completo": "Ana López Ruiz", "datos.genero": "ninguno", "datos.nombre_ensamble": "No aplica", "datos.fecha_nacimiento": "1990-05-17",
	})
	if guardados != "[datos.fecha_nacimiento datos.nombre_completo]" || rechazados != "[datos.genero datos.nombre_ensamble]" {
		t.Errorf("autoguardado parcial: %s %s", guardados, rechazados)
	}
	// Si nada pudo guardarse, 422.
	if campos := ana.rechaza("PATCH", ruta, obj{"campos": obj{"datos.email_contacto": "no-es-correo"}}); fmt.Sprint(campos) != "[datos.email_contacto]" {
		t.Errorf("autoguardado inválido: %v", campos)
	}
	for nombre, cuerpo := range map[string]any{"sin campos": obj{}, "valor que no es texto": obj{"campos": obj{"datos.telefono": []int{1}}},
		"decimal": obj{"campos": obj{"datos.telefono": 1.5}}} {
		if estado, r := ana.api("PATCH", ruta, cuerpo); estado != http.StatusBadRequest {
			t.Errorf("autoguardado %s: %d %v", nombre, estado, r)
		}
	}
	p = ana.exige(http.StatusOK, "GET", ruta, nil)
	campos := p["campos"].(obj)
	if campos["datos.telefono"] != "55 1234 5678" || campos["datos.nombre_completo"] != "Ana López Ruiz" || campos["datos.genero"] != nil {
		t.Errorf("campos tras el autoguardado: %v", campos)
	}
	// null vacía un campo.
	guardar(t, ana, ruta, obj{"datos.telefono": nil})
	if p = ana.exige(http.StatusOK, "GET", ruta, nil); p["campos"].(obj)["datos.telefono"] != nil {
		t.Errorf("el teléfono debió vaciarse: %v", p["campos"])
	}

	// Regla de sede (RN-06): fuera de la Ciudad de México se piden cuatro datos más.
	guardar(t, ana, ruta, datosDeSolista())
	guardar(t, ana, ruta, obj{"concierto.entidad": "OAX"})
	p = ana.exige(http.StatusOK, "GET", ruta, nil)
	if f := faltantes(t, p); !strings.Contains(f, "concierto/concierto.sede_nombre concierto/concierto.sede_domicilio") && !strings.Contains(f, "concierto/concierto.fecha_concierto concierto/concierto.hora_concierto concierto/concierto.sede_domicilio concierto/concierto.sede_nombre") {
		t.Errorf("fuera de CDMX deben pedirse los datos de la sede: %s", f)
	}
	guardar(t, ana, ruta, obj{"concierto.entidad": "CDMX"})

	// Ligas de video (RN-08): de una a tres, https, sin repetir. La
	// comprobación en red llega en el paso 6: por ahora quedan pendientes.
	for nombre, urls := range map[string][]string{
		"cuatro":   {"https://a.mx/1", "https://a.mx/2", "https://a.mx/3", "https://a.mx/4"},
		"sin TLS":  {"http://vimeo.com/1"},
		"repetida": {"https://vimeo.com/1", "https://vimeo.com/1"},
	} {
		if campos := ana.rechaza("PUT", ruta+"/videos", obj{"urls": urls}); fmt.Sprint(campos) != "[videos]" {
			t.Errorf("videos (%s): %v", nombre, campos)
		}
	}
	p = ana.exige(http.StatusOK, "PUT", ruta+"/videos", obj{"urls": []string{" https://www.youtube.com/watch?v=abc ", "https://vimeo.com/123"}})
	videos := lista(p, "videos")
	if len(videos) != 2 || videos[0]["url"] != "https://www.youtube.com/watch?v=abc" || videos[0]["plataforma"] != "youtube" ||
		videos[0]["estado"] != "pendiente" || videos[1]["plataforma"] != "vimeo" {
		t.Errorf("videos guardados: %v", videos)
	}
	// Un solista no tiene integrantes.
	if campos := ana.rechaza("PUT", ruta+"/integrantes", obj{"integrantes": []obj{{"nombre": "X", "instrumento": "Y"}}}); fmt.Sprint(campos) != "[integrantes]" {
		t.Errorf("integrantes en un solista: %v", campos)
	}

	// Programa: en Intérpretes no hay máximo de obras (RN-05).
	var obras []int64
	for i := 0; i < 4; i++ {
		p = ana.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
		creadas := lista(p, "obras")
		if len(creadas) != i+1 || id(creadas[i]["orden"]) != int64(i+1) {
			t.Fatalf("obras tras añadir la %d: %v", i+1, creadas)
		}
		obras = append(obras, id(creadas[i]["id"]))
	}
	rutaObra := func(obraID int64) string { return fmt.Sprintf("/api/v1/obras/%d", obraID) }
	for i, obraID := range obras {
		guardados, rechazados := guardar(t, ana, rutaObra(obraID), obj{
			"obra.titulo": fmt.Sprintf("Obra %d", i+1), "obra.compositor": "Julián Carrillo", "obra.duracion_min": 10 + i,
			"obra.estreno": "no_es_estreno", "obra.semblanza_compositor": "Semblanza.", "obra.nota_obra": "Nota.",
			"obra.liga_media": "https://ejemplo.mx/obra", "obra.dotacion_id": 1,
		})
		// La dotación es de Compositores: en Intérpretes no existe ese campo.
		if !strings.Contains(guardados, "obra.titulo") || !strings.Contains(guardados, "obra.duracion_min") || rechazados != "[obra.dotacion_id]" {
			t.Errorf("autoguardado de la obra %d: %s %s", i+1, guardados, rechazados)
		}
	}
	if campos := ana.rechaza("PATCH", rutaObra(obras[0]), obj{"campos": obj{"obra.duracion_min": 0}}); fmt.Sprint(campos) != "[obra.duracion_min]" {
		t.Errorf("duración inválida: %v", campos)
	}
	// Reordenar: exige exactamente las obras de la postulación.
	for nombre, ids := range map[string][]int64{"faltan": obras[:3], "ajena": {obras[0], obras[1], obras[2], 9999}, "repetida": {obras[0], obras[0], obras[1], obras[2]}} {
		if campos := ana.rechaza("PUT", ruta+"/obras/orden", obj{"ids": ids}); fmt.Sprint(campos) != "[ids]" {
			t.Errorf("orden (%s): %v", nombre, campos)
		}
	}
	p = ana.exige(http.StatusOK, "PUT", ruta+"/obras/orden", obj{"ids": []int64{obras[3], obras[0], obras[2], obras[1]}})
	var orden []string
	for _, o := range lista(p, "obras") {
		orden = append(orden, o["campos"].(obj)["obra.titulo"].(string))
	}
	if fmt.Sprint(orden) != "[Obra 4 Obra 1 Obra 3 Obra 2]" {
		t.Errorf("orden del programa: %v", orden)
	}
	// Quitar una obra cierra el hueco en el orden.
	p = ana.exige(http.StatusOK, "DELETE", rutaObra(obras[0]), nil)
	restantes := lista(p, "obras")
	if len(restantes) != 3 || id(restantes[0]["orden"]) != 1 || id(restantes[1]["orden"]) != 2 || id(restantes[2]["orden"]) != 3 ||
		restantes[1]["campos"].(obj)["obra.titulo"] != "Obra 3" {
		t.Errorf("obras tras borrar una: %v", restantes)
	}
	if estado, _ := ana.api("DELETE", rutaObra(obras[0]), nil); estado != http.StatusNotFound {
		t.Errorf("borrar una obra ya borrada: %d", estado)
	}

	// Con todo capturado sólo faltan los documentos (paso 6) y verificar el correo.
	esperado := "documentos/documento.fotografia"
	for _, obraID := range []int64{obras[3], obras[2], obras[1]} {
		esperado += fmt.Sprintf(" documentos/documento.autorizacion#%d documentos/documento.partitura#%d", obraID, obraID)
	}
	partes := strings.Fields(esperado + " envio/cuenta.email")
	slices.Sort(partes)
	if f := faltantes(t, p); f != strings.Join(partes, " ") {
		t.Errorf("bloqueadores con la captura completa:\n  %s\nse esperaba:\n  %s", f, strings.Join(partes, " "))
	}
	if s := seccionesPendientes(p); s != "datos=0 concierto=0 obras=0 documentos=7 envio=1" {
		t.Errorf("pendientes por sección: %s", s)
	}
	// Los mensajes nombran el documento con la etiqueta de la convocatoria, en
	// el idioma de la petición.
	if mensajes := fmt.Sprint(p["bloqueadores"]); !strings.Contains(mensajes, "Autorización del titular de la obra") {
		t.Errorf("los bloqueadores deben nombrar el documento: %s", mensajes)
	}

	// Lo ajeno no existe: otra cuenta no puede ver ni tocar la postulación.
	luis := e.postulante("luis@ejemplo.mx")
	for _, caso := range []struct {
		metodo, ruta string
		cuerpo       any
	}{
		{"GET", ruta, nil},
		{"PATCH", ruta, obj{"campos": obj{"datos.telefono": "1"}}},
		{"PUT", ruta + "/videos", obj{"urls": []string{}}},
		{"PUT", ruta + "/integrantes", obj{"integrantes": []obj{}}},
		{"PUT", ruta + "/subcategoria", obj{"subcategoria": "ensamble", "confirmar": true}},
		{"POST", ruta + "/obras", nil},
		{"PUT", ruta + "/obras/orden", obj{"ids": []int64{}}},
		{"PATCH", rutaObra(obras[1]), obj{"campos": obj{"obra.titulo": "Ajena"}}},
		{"DELETE", rutaObra(obras[1]), nil},
	} {
		if estado, r := luis.api(caso.metodo, caso.ruta, caso.cuerpo); estado != http.StatusNotFound {
			t.Errorf("%s %s desde otra cuenta: %d %v", caso.metodo, caso.ruta, estado, r)
		}
	}
	// El administrador la consulta, pero no la edita: no es suya.
	if visto := admin.exige(http.StatusOK, "GET", ruta, nil); visto["editable"] != false || len(lista(visto, "obras")) != 3 {
		t.Errorf("consulta del administrador: %v", visto)
	}
	if estado, _ := admin.api("PATCH", ruta, obj{"campos": obj{"datos.telefono": "1"}}); estado != http.StatusNotFound {
		t.Errorf("el administrador no edita una postulación ajena: %d", estado)
	}
	if p = ana.exige(http.StatusOK, "GET", ruta, nil); len(lista(p, "obras")) != 3 || p["campos"].(obj)["datos.telefono"] != "55 1234 5678" {
		t.Errorf("la postulación cambió por peticiones ajenas: %v", p)
	}

	// Sólo la creación queda en la bitácora: el autoguardado no se audita.
	if n := len(e.asientos("postulacion.crear")); n != 1 {
		t.Errorf("asientos de creación: %d", n)
	}

	// Vencido el plazo la captura se cierra, aunque la convocatoria aún no
	// haya cambiado de etapa (RN-02).
	e.hasta("2026-12-20T06:00:00Z")
	ana.ingresar("ana@ejemplo.mx", passwordPrueba)
	if campos := ana.rechaza("PATCH", ruta, obj{"campos": obj{"datos.telefono": "1"}}); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("autoguardado fuera de plazo: %v", campos)
	}
	if campos := ana.rechaza("POST", ruta+"/obras", nil); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("obra fuera de plazo: %v", campos)
	}
	if p = ana.exige(http.StatusOK, "GET", ruta, nil); p["editable"] != false {
		t.Error("fuera de plazo la postulación no es editable")
	}
	luis.ingresar("luis@ejemplo.mx", passwordPrueba)
	if campos := luis.rechaza("POST", rutaConvocatoria, obj{"subcategoria": "solista"}); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("alta fuera de plazo: %v", campos)
	}
}

// Un ensamble declara integrantes; cambiar de subcategoría exige confirmación
// porque borra los datos generales.
func TestPostulacionDeEnsambleYCambioDeSubcategoria(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, _, _, _ := e.convocatoriasAbiertas()
	onix := e.postulante("onix@ejemplo.mx")
	p := onix.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "ensamble"})
	ruta := fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	if p["definicion"].(obj)["tiene_integrantes"] != true || !strings.Contains(faltantes(t, p), "datos/integrantes") {
		t.Errorf("un ensamble debe pedir integrantes: %v", faltantes(t, p))
	}

	// Residencia en el extranjero: la entidad es texto libre.
	guardados, rechazados := guardar(t, onix, ruta, obj{
		"datos.nombre_ensamble": "Cuarteto Ónix", "datos.representante": "Ana López", "datos.telefono": "+54 11 5555 0000",
		"datos.email_contacto": "onix@ejemplo.mx", "datos.dotacion_texto": "Cuarteto de cuerdas", "datos.anio_fundacion": 2010,
		"datos.pais_residencia": "AR", "datos.semblanza": "Semblanza.", "datos.nombre_completo": "No aplica",
		"concierto.requerimientos_tecnicos": "Cuatro atriles.", "concierto.entidad": "OAX", "concierto.sede_nombre": "Teatro Macedonio Alcalá",
	})
	if strings.Contains(guardados, "datos.nombre_completo") || rechazados != "[datos.nombre_completo]" {
		t.Errorf("autoguardado del ensamble: %s %s", guardados, rechazados)
	}
	guardar(t, onix, ruta, obj{"datos.entidad_residencia": "Provincia de Buenos Aires"})

	// Integrantes: la lista se sustituye completa.
	if campos := onix.rechaza("PUT", ruta+"/integrantes", obj{"integrantes": []obj{{"nombre": "Ana López", "instrumento": ""}}}); fmt.Sprint(campos) != "[integrantes]" {
		t.Errorf("integrante sin instrumento: %v", campos)
	}
	onix.exige(http.StatusOK, "PUT", ruta+"/integrantes", obj{"integrantes": []obj{{"nombre": "Uno", "instrumento": "violín"}}})
	p = onix.exige(http.StatusOK, "PUT", ruta+"/integrantes", obj{"integrantes": []obj{
		{"nombre": " Ana López ", "instrumento": "violín"}, {"nombre": "Luis Ruiz", "instrumento": "viola"}, {"nombre": "Eva Paz", "instrumento": "violonchelo"},
	}})
	integrantes := lista(p, "integrantes")
	if len(integrantes) != 3 || integrantes[0]["nombre"] != "Ana López" || integrantes[2]["instrumento"] != "violonchelo" {
		t.Errorf("integrantes guardados: %v", integrantes)
	}
	p = onix.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	obraID := id(lista(p, "obras")[0]["id"])
	guardar(t, onix, fmt.Sprintf("/api/v1/obras/%d", obraID), obj{"obra.titulo": "Cuarteto n.º 1"})
	onix.exige(http.StatusOK, "PUT", ruta+"/videos", obj{"urls": []string{"https://vimeo.com/9"}})
	p = onix.exige(http.StatusOK, "GET", ruta, nil)
	if f := faltantes(t, p); strings.Contains(f, "datos/datos.") || strings.Contains(f, "integrantes") || strings.Contains(f, "videos") ||
		!strings.Contains(f, "concierto/concierto.sede_domicilio") || strings.Contains(f, "concierto/concierto.sede_nombre") {
		t.Errorf("bloqueadores del ensamble: %s", f)
	}

	// Cambio de subcategoría: sin confirmar no se hace nada.
	if campos := onix.rechaza("PUT", ruta+"/subcategoria", obj{"subcategoria": "solista"}); fmt.Sprint(campos) != "[confirmar]" {
		t.Errorf("cambio sin confirmar: %v", campos)
	}
	if campos := onix.rechaza("PUT", ruta+"/subcategoria", obj{"subcategoria": "coro", "confirmar": true}); fmt.Sprint(campos) != "[subcategoria]" {
		t.Errorf("cambio a una subcategoría inventada: %v", campos)
	}
	p = onix.exige(http.StatusOK, "PUT", ruta+"/subcategoria", obj{"subcategoria": "solista", "confirmar": true})
	campos := p["campos"].(obj)
	if p["subcategoria"] != "solista" || len(lista(p, "integrantes")) != 0 || campos["datos.telefono"] != nil || campos["datos.semblanza"] != nil {
		t.Errorf("tras el cambio los datos generales deben quedar vacíos: %v", campos)
	}
	if _, existe := campos["datos.nombre_ensamble"]; existe {
		t.Errorf("el formulario de solista no tiene nombre de ensamble: %v", campos)
	}
	// El concierto, las obras y los videos se conservan.
	if campos["concierto.sede_nombre"] != "Teatro Macedonio Alcalá" || len(lista(p, "obras")) != 1 || len(lista(p, "videos")) != 1 {
		t.Errorf("el cambio de subcategoría no debe tocar concierto, obras ni videos: %v", p)
	}
	// Repetir el cambio no hace nada; y queda en la bitácora una sola vez.
	onix.exige(http.StatusOK, "PUT", ruta+"/subcategoria", obj{"subcategoria": "solista", "confirmar": true})
	if n := len(e.asientos("postulacion.subcategoria")); n != 1 {
		t.Errorf("asientos del cambio de subcategoría: %d", n)
	}
	if guardados, _ := guardar(t, onix, ruta, obj{"datos.nombre_completo": "Ana López"}); guardados != "[datos.nombre_completo]" {
		t.Errorf("tras el cambio debe poder capturarse el formulario de solista: %s", guardados)
	}
}

// Un compositor postula hasta dos obras; los documentos de cada una dependen
// de su dotación.
func TestPostulacionDeCompositor(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, compositores, cuarteto, electro := e.convocatoriasAbiertas()
	rutaConvocatoria := fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", compositores)

	// Quien es jurado o revisor de la convocatoria no postula en ella; el
	// observador no postula en ninguna.
	juradoID := e.crearInterno(admin, "jurado@ejemplo.mx")
	admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/asignaciones", compositores), obj{"usuario_id": juradoID, "perfil": "jurado"})
	token := e.tokenDe("jurado@ejemplo.mx", models.PlantillaActivacion, "/cuenta/activar")
	jurado := e.cliente()
	redirige(t, jurado.post("/cuenta/activar", url.Values{"token": {token}, "password": {passwordPrueba}}), "/cuenta/ingresar?aviso=activada")
	jurado.ingresar("jurado@ejemplo.mx", passwordPrueba)
	if campos := jurado.rechaza("POST", rutaConvocatoria, obj{}); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("un jurado no postula en su convocatoria: %v", campos)
	}
	// En la otra convocatoria sí puede.
	jurado.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "solista"})
	observador := e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	if estado, r := observador.api("POST", rutaConvocatoria, obj{}); estado != http.StatusForbidden {
		t.Errorf("el observador no postula: %d %v", estado, r)
	}

	eva := e.postulante("eva@ejemplo.mx")
	if campos := eva.rechaza("POST", rutaConvocatoria, obj{"subcategoria": "solista"}); fmt.Sprint(campos) != "[subcategoria]" {
		t.Errorf("Compositores no tiene subcategorías: %v", campos)
	}
	p := eva.exige(http.StatusCreated, "POST", rutaConvocatoria, obj{})
	ruta := fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	definicion := p["definicion"].(obj)
	if p["subcategoria"] != nil || definicion["tiene_videos"] != false || definicion["tiene_integrantes"] != false || len(lista(p, "dotaciones")) != 2 {
		t.Errorf("borrador de compositor inesperado: %v", p)
	}
	if s := seccionesPendientes(p); !strings.HasPrefix(s, "datos=9 obras=1 ") || strings.Contains(s, "concierto") {
		t.Errorf("secciones de Compositores: %s", s)
	}
	// Ni videos, ni integrantes, ni cambio de subcategoría, ni campos de concierto.
	if campos := eva.rechaza("PUT", ruta+"/videos", obj{"urls": []string{"https://vimeo.com/1"}}); fmt.Sprint(campos) != "[videos]" {
		t.Errorf("videos en Compositores: %v", campos)
	}
	if campos := eva.rechaza("PUT", ruta+"/subcategoria", obj{"subcategoria": "solista", "confirmar": true}); fmt.Sprint(campos) != "[subcategoria]" {
		t.Errorf("subcategoría en Compositores: %v", campos)
	}
	if campos := eva.rechaza("PATCH", ruta, obj{"campos": obj{"concierto.entidad": "CDMX"}}); fmt.Sprint(campos) != "[concierto.entidad]" {
		t.Errorf("concierto en Compositores: %v", campos)
	}
	datos := datosDeSolista()
	delete(datos, "concierto.requerimientos_tecnicos")
	delete(datos, "concierto.entidad")
	if _, rechazados := guardar(t, eva, ruta, datos); rechazados != "[]" {
		t.Errorf("datos generales del compositor: %s", rechazados)
	}

	// Máximo dos obras (RN-05).
	p = eva.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	p = eva.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	if campos := eva.rechaza("POST", ruta+"/obras", nil); fmt.Sprint(campos) != "[obras]" {
		t.Errorf("tercera obra: %v", campos)
	}
	primera, segunda := id(lista(p, "obras")[0]["id"]), id(lista(p, "obras")[1]["id"])
	rutaObra := func(obraID int64) string { return fmt.Sprintf("/api/v1/obras/%d", obraID) }
	obra := func(dotacion int64) obj {
		return obj{"obra.titulo": "Obra", "obra.duracion_min": 9, "obra.estreno": "estreno_mundial", "obra.dotacion_id": dotacion,
			"obra.nota_obra": "Nota.", "obra.requerimientos_tecnicos": "Ninguno."}
	}
	// La dotación debe ser de ESTA convocatoria.
	if _, rechazados := guardar(t, eva, rutaObra(primera), obra(9999)); rechazados != "[obra.dotacion_id]" {
		t.Errorf("dotación inexistente: %s", rechazados)
	}
	guardar(t, eva, rutaObra(primera), obra(cuarteto))
	guardar(t, eva, rutaObra(segunda), obra(electro))

	// RN-07: el cuarteto pide partitura y particellas; la electroacústica,
	// partitura y audio.
	p = eva.exige(http.StatusOK, "GET", ruta, nil)
	esperado := []string{"documentos/documento.fotografia", "envio/cuenta.email",
		fmt.Sprintf("documentos/documento.partitura#%d", primera), fmt.Sprintf("documentos/documento.particellas#%d", primera),
		fmt.Sprintf("documentos/documento.partitura#%d", segunda), fmt.Sprintf("documentos/documento.audio#%d", segunda)}
	slices.Sort(esperado)
	if f := faltantes(t, p); f != strings.Join(esperado, " ") {
		t.Errorf("documentos por dotación:\n  %s\nse esperaba:\n  %s", f, strings.Join(esperado, " "))
	}
	// Al cambiar la dotación de una obra cambian sus documentos.
	guardar(t, eva, rutaObra(segunda), obj{"obra.dotacion_id": cuarteto})
	p = eva.exige(http.StatusOK, "GET", ruta, nil)
	if f := faltantes(t, p); !strings.Contains(f, fmt.Sprintf("documento.particellas#%d", segunda)) || strings.Contains(f, "documento.audio") {
		t.Errorf("documentos tras cambiar de dotación: %s", f)
	}
	// Con la cuenta en inglés, los bloqueadores vienen en inglés.
	redirige(t, eva.post("/cuenta/datos", url.Values{"nombre": {"Eva"}, "apellidos": {"Paz"}, "idioma": {"en"}}), "/cuenta?aviso=datos")
	if en := eva.exige(http.StatusOK, "GET", ruta, nil); !strings.Contains(fmt.Sprint(en["bloqueadores"]), "Score") {
		t.Errorf("bloqueadores en inglés: %v", en["bloqueadores"])
	}

	// Quien supervisa ve la postulación; el jurado, en esta etapa, no.
	if visto := observador.exige(http.StatusOK, "GET", ruta, nil); visto["editable"] != false {
		t.Errorf("consulta del observador: %v", visto)
	}
	if estado, _ := jurado.api("GET", ruta, nil); estado != http.StatusNotFound {
		t.Errorf("el jurado no ve borradores: %d", estado)
	}

	// Si la convocatoria sale de la etapa de postulación, la captura se cierra.
	admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/asignaciones", compositores),
		obj{"usuario_id": e.crearInterno(admin, "revisora@ejemplo.mx"), "perfil": "revisor_documental"})
	admin.estado(compositores, "revision_documental", "Cierre anticipado de prueba")
	if campos := eva.rechaza("PATCH", ruta, obj{"campos": obj{"datos.telefono": "1"}}); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("autoguardado fuera de etapa: %v", campos)
	}
}
