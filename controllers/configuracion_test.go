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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de extremo a extremo de la API de configuración: un administrador
// arma una convocatoria desde cero y la lleva por todo su ciclo de vida.

// obj abrevia un objeto JSON en las pruebas.
type obj = map[string]any

// api hace una petición a la API con cuerpo JSON y decodifica la respuesta.
func (c *cliente) api(metodo, ruta string, cuerpo any) (int, obj) {
	c.t.Helper()
	var datos []byte
	if cuerpo != nil {
		var err error
		if datos, err = json.Marshal(cuerpo); err != nil {
			c.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(metodo, ruta, bytes.NewReader(datos))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := c.hacer(r)
	if w.Code == http.StatusNoContent {
		return w.Code, nil
	}
	var respuesta obj
	if err := json.Unmarshal(w.Body.Bytes(), &respuesta); err != nil {
		c.t.Fatalf("%s %s no devolvió JSON (%d): %s", metodo, ruta, w.Code, w.Body.String())
	}
	return w.Code, respuesta
}

// exige hace una petición y falla la prueba si el estado no es el esperado.
func (c *cliente) exige(estado int, metodo, ruta string, cuerpo any) obj {
	c.t.Helper()
	obtenido, respuesta := c.api(metodo, ruta, cuerpo)
	if obtenido != estado {
		c.t.Fatalf("%s %s: estado %d, se esperaba %d: %v", metodo, ruta, obtenido, estado, respuesta)
	}
	return respuesta
}

// rechaza hace una petición que debe fallar con 422 y devuelve los nombres de
// los campos señalados.
func (c *cliente) rechaza(metodo, ruta string, cuerpo any) []string {
	c.t.Helper()
	respuesta := c.exige(http.StatusUnprocessableEntity, metodo, ruta, cuerpo)
	fallo := respuesta["error"].(obj)
	if fallo["codigo"] != "validacion" {
		c.t.Fatalf("%s %s: código %v, se esperaba «validacion»", metodo, ruta, fallo["codigo"])
	}
	var campos []string
	for _, campo := range fallo["campos"].([]any) {
		if campo.(obj)["mensaje"] == "" || strings.HasPrefix(campo.(obj)["mensaje"].(string), "error.") {
			c.t.Errorf("%s %s: mensaje sin traducir en %v", metodo, ruta, campo)
		}
		campos = append(campos, campo.(obj)["campo"].(string))
	}
	return campos
}

// codigoDe devuelve el código de error de una respuesta de la API.
func codigoDe(respuesta obj) any {
	if fallo, ok := respuesta["error"].(obj); ok {
		return fallo["codigo"]
	}
	return nil
}

// id extrae un identificador numérico de una respuesta.
func id(valor any) int64 { return int64(valor.(float64)) }

// lista devuelve una lista de objetos de una respuesta.
func lista(respuesta obj, clave string) []obj {
	var elementos []obj
	for _, e := range respuesta[clave].([]any) {
		elementos = append(elementos, e.(obj))
	}
	return elementos
}

// bloqueadores devuelve los campos que impiden pasar a la siguiente etapa.
func bloqueadores(configuracion obj) []string {
	var campos []string
	for _, b := range lista(configuracion["siguiente"].(obj), "bloqueadores") {
		campos = append(campos, b["campo"].(string))
	}
	return campos
}

// etapasDePrueba es un calendario válido a partir de la fecha de las pruebas
// (12 de noviembre de 2026): la postulación abre el día 20.
func etapasDePrueba() obj {
	return obj{"etapas": []obj{
		{"clave": "postulacion", "inicia_at": "2026-11-20T06:00:00Z", "termina_at": "2026-12-20T05:59:00Z"},
		{"clave": "revision_documental", "inicia_at": "2026-12-21T06:00:00Z", "termina_at": "2027-01-10T05:59:00Z"},
		{"clave": "evaluacion", "inicia_at": "2027-01-11T06:00:00Z", "termina_at": "2027-01-31T05:59:00Z"},
	}}
}

// trabajadorEtapas crea el trabajador de avance por calendario sobre el reloj
// de la prueba.
func (e *entornoPrueba) trabajadorEtapas() *TrabajadorEtapas {
	return &TrabajadorEtapas{Datos: e.repo, Ahora: e.reloj.Ahora, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// hasta adelanta el reloj de la prueba a un instante dado.
func (e *entornoPrueba) hasta(instante string) {
	e.t.Helper()
	momento, err := time.Parse(time.RFC3339, instante)
	if err != nil {
		e.t.Fatal(err)
	}
	e.reloj.Avanzar(momento.Sub(e.reloj.Ahora()))
}

// crearInterno da de alta por la API una cuenta interna y devuelve su id.
func (e *entornoPrueba) crearInterno(admin *cliente, email string) int64 {
	e.t.Helper()
	cuenta := admin.exige(http.StatusCreated, "POST", "/api/v1/usuarios",
		obj{"email": email, "nombre": "Persona", "apellidos": "Interna"})
	return id(cuenta["id"])
}

// compositores2028 crea la edición 50 y su convocatoria de Compositores, con
// bases y calendario, lista para publicarse. Devuelve los id de ambas.
func (e *entornoPrueba) compositores2028(admin *cliente) (edicionID, convocatoriaID int64) {
	e.t.Helper()
	edicion := admin.exige(http.StatusCreated, "POST", "/api/v1/ediciones",
		obj{"numero": 50, "anio": 2028, "nombre_es": "Edición 50 · 2028"})
	edicionID = id(edicion["id"])
	c := admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/ediciones/%d/convocatorias", edicionID), obj{
		"tipo": "compositores", "titulo_es": "Convocatoria de Compositores 2028", "bases_md_es": "# Bases\n\nTexto.",
	})
	convocatoriaID = id(c["id"])
	admin.exige(http.StatusOK, "PUT", fmt.Sprintf("/api/v1/convocatorias/%d/etapas", convocatoriaID), etapasDePrueba())
	return edicionID, convocatoriaID
}

// estado fuerza un cambio de etapa y exige que se acepte.
func (c *cliente) estado(convocatoriaID int64, destino, motivo string) obj {
	c.t.Helper()
	return c.exige(http.StatusOK, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/estado", convocatoriaID),
		obj{"estado": destino, "motivo": motivo})
}

func TestEdicionesYPermisosDeConfiguracion(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	observador := e.activarCuentaInterna("coordinacion@ejemplo.mx", models.PerfilCoordinacionNacional)
	e.crearCuenta("ana@ejemplo.mx", models.PerfilPostulante)
	postulante := e.cliente()
	postulante.ingresar("ana@ejemplo.mx", passwordPrueba)

	// Alta de una edición. Falta el nombre en inglés: se acepta y se advierte.
	edicion := admin.exige(http.StatusCreated, "POST", "/api/v1/ediciones", obj{"numero": 50, "anio": 2028, "nombre_es": " Edición 50 "})
	if edicion["nombre_es"] != "Edición 50" || fmt.Sprint(edicion["faltan_en_ingles"]) != "[nombre_en]" || len(lista(edicion, "convocatorias")) != 0 {
		t.Errorf("edición creada inesperada: %v", edicion)
	}
	ruta := fmt.Sprintf("/api/v1/ediciones/%d", id(edicion["id"]))

	// Reglas, duplicados y cuerpos mal formados.
	if campos := admin.rechaza("POST", "/api/v1/ediciones", obj{"numero": 0, "anio": 1500, "nombre_es": " "}); fmt.Sprint(campos) != "[numero anio nombre_es]" {
		t.Errorf("campos rechazados: %v", campos)
	}
	if estado, r := admin.api("POST", "/api/v1/ediciones", obj{"numero": 50, "anio": 2029, "nombre_es": "Otra"}); estado != http.StatusConflict || codigoDe(r) != "duplicado" {
		t.Errorf("número de edición repetido: %d %v", estado, r)
	}
	if estado, r := admin.api("POST", "/api/v1/ediciones", obj{"numero": 51, "anio": 2029, "nombre_es": "X", "nombre": "campo que no existe"}); estado != http.StatusBadRequest || codigoDe(r) != "json_invalido" {
		t.Errorf("un campo desconocido debe rechazarse: %d %v", estado, r)
	}
	if estado, r := admin.api("GET", "/api/v1/ediciones/999", nil); estado != http.StatusNotFound || codigoDe(r) != "no_encontrado" {
		t.Errorf("edición inexistente: %d %v", estado, r)
	}

	// Lectura: administrador y observador. Escritura: sólo administrador (§2.2).
	for nombre, c := range map[string]*cliente{"administrador": admin, "observador": observador} {
		ediciones := lista(c.exige(http.StatusOK, "GET", "/api/v1/ediciones", nil), "ediciones")
		if len(ediciones) != 2 || ediciones[0]["numero"].(float64) != 50 || len(lista(ediciones[1], "convocatorias")) != 2 {
			t.Errorf("%s: listado de ediciones inesperado: %v", nombre, ediciones)
		}
	}
	nuevo := obj{"numero": 50, "anio": 2028, "nombre_es": "Edición 50 · 2028", "nombre_en": "50th Edition · 2028"}
	for nombre, caso := range map[string]struct {
		c      *cliente
		estado int
		codigo string
	}{
		"observador": {observador, http.StatusForbidden, "sin_permiso"},
		"postulante": {postulante, http.StatusForbidden, "sin_permiso"},
		"anónimo":    {e.cliente(), http.StatusUnauthorized, "no_autenticado"},
	} {
		if estado, r := caso.c.api("PUT", ruta, nuevo); estado != caso.estado || codigoDe(r) != caso.codigo {
			t.Errorf("%s no debe poder editar: %d %v", nombre, estado, r)
		}
	}
	if estado, _ := postulante.api("GET", "/api/v1/ediciones", nil); estado != http.StatusForbidden {
		t.Errorf("un postulante no debe leer la configuración: %d", estado)
	}
	if editada := admin.exige(http.StatusOK, "PUT", ruta, nuevo); editada["nombre_en"] != "50th Edition · 2028" || len(editada["faltan_en_ingles"].([]any)) != 0 {
		t.Errorf("edición editada inesperada: %v", editada)
	}
}

func TestConvocatoriaDesdeCeroHastaPublicarse(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	edicion := admin.exige(http.StatusCreated, "POST", "/api/v1/ediciones", obj{"numero": 50, "anio": 2028, "nombre_es": "Edición 50"})
	rutaEdicion := fmt.Sprintf("/api/v1/ediciones/%d", id(edicion["id"]))

	// Nace en borrador, con los valores por omisión de su tipo y los requisitos
	// y criterios iniciales.
	c := admin.exige(http.StatusCreated, "POST", rutaEdicion+"/convocatorias", obj{"tipo": "compositores", "titulo_es": "Compositores 2028"})
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d", id(c["id"]))
	claves := func(configuracion obj, coleccion string) (claves []string) {
		for _, elemento := range lista(configuracion, coleccion) {
			claves = append(claves, elemento["clave"].(string))
		}
		return claves
	}
	if c["estado"] != "borrador" || c["max_obras"].(float64) != 2 || c["max_videos"].(float64) != 3 || c["duracion_max_programa_min"] != nil ||
		fmt.Sprint(claves(c, "requisitos")) != "[fotografia partitura particellas audio]" ||
		fmt.Sprint(claves(c, "criterios")) != "[realizacion artistico]" {
		t.Errorf("convocatoria nueva inesperada: %v", c)
	}
	// Dice qué le falta para publicarse.
	if c["siguiente"].(obj)["estado"] != "publicada" || fmt.Sprint(bloqueadores(c)) != "[bases_md_es etapas]" {
		t.Errorf("bloqueadores para publicar: %v", c["siguiente"])
	}
	// En Intérpretes no hay límite de obras y se pide la autorización del titular.
	interpretes := admin.exige(http.StatusCreated, "POST", rutaEdicion+"/convocatorias", obj{"tipo": "interpretes", "titulo_es": "Intérpretes 2028"})
	if interpretes["max_obras"] != nil || fmt.Sprint(claves(interpretes, "requisitos")) != "[fotografia partitura autorizacion]" {
		t.Errorf("convocatoria de intérpretes inesperada: %v", interpretes)
	}
	// Una edición tiene una sola convocatoria de cada tipo.
	if estado, r := admin.api("POST", rutaEdicion+"/convocatorias", obj{"tipo": "compositores", "titulo_es": "Otra"}); estado != http.StatusConflict {
		t.Errorf("segunda convocatoria del mismo tipo: %d %v", estado, r)
	}
	if campos := admin.rechaza("POST", rutaEdicion+"/convocatorias", obj{"tipo": "directores", "titulo_es": ""}); fmt.Sprint(campos) != "[tipo titulo_es]" {
		t.Errorf("campos rechazados: %v", campos)
	}

	// No se publica sin bases ni calendario; tampoco se salta de etapa.
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "publicada"}); fmt.Sprint(campos) != "[bases_md_es etapas]" {
		t.Errorf("publicar sin bases ni calendario: %v", campos)
	}
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "evaluacion"}); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("salto de etapa: %v", campos)
	}
	// Textos y límites.
	if campos := admin.rechaza("PUT", ruta, obj{"titulo_es": "Compositores 2028", "max_obras": 0, "max_videos": 9}); fmt.Sprint(campos) != "[max_obras max_videos]" {
		t.Errorf("límites inválidos: %v", campos)
	}
	editada := admin.exige(http.StatusOK, "PUT", ruta, obj{"titulo_es": "Compositores 2028", "titulo_en": "Composers 2028",
		"bases_md_es": "# Bases\n\nTexto de las **bases**.", "max_obras": 3})
	if editada["max_obras"].(float64) != 3 || fmt.Sprint(editada["faltan_en_ingles"]) != "[bases_md_en]" || editada["tipo"] != "compositores" {
		t.Errorf("convocatoria editada inesperada: %v", editada)
	}
	// Calendario: coherente o nada.
	for nombre, etapas := range map[string][]obj{
		"etapa que termina antes de empezar": {{"clave": "postulacion", "inicia_at": "2026-12-01T00:00:00Z", "termina_at": "2026-11-01T00:00:00Z"}},
		"etapa repetida": {{"clave": "postulacion", "inicia_at": "2026-12-01T00:00:00Z", "termina_at": "2026-12-02T00:00:00Z"},
			{"clave": "postulacion", "inicia_at": "2026-12-03T00:00:00Z", "termina_at": "2026-12-04T00:00:00Z"}},
		"etapas traslapadas": {{"clave": "postulacion", "inicia_at": "2026-12-01T00:00:00Z", "termina_at": "2026-12-20T00:00:00Z"},
			{"clave": "evaluacion", "inicia_at": "2026-12-10T00:00:00Z", "termina_at": "2026-12-30T00:00:00Z"}},
		"etapa desconocida": {{"clave": "premiacion", "inicia_at": "2026-12-01T00:00:00Z", "termina_at": "2026-12-02T00:00:00Z"}},
	} {
		if campos := admin.rechaza("PUT", ruta+"/etapas", obj{"etapas": etapas}); fmt.Sprint(campos) != "[etapas]" {
			t.Errorf("%s: %v", nombre, campos)
		}
	}
	// Las fechas se aceptan con cualquier desfase horario y se guardan en UTC.
	conCalendario := admin.exige(http.StatusOK, "PUT", ruta+"/etapas", obj{"etapas": []obj{
		{"clave": "evaluacion", "inicia_at": "2027-01-11T00:00:00-06:00", "termina_at": "2027-01-30T23:59:00-06:00"},
		{"clave": "postulacion", "inicia_at": "2026-11-20T00:00:00-06:00", "termina_at": "2026-12-19T23:59:00-06:00"},
		{"clave": "revision_documental", "inicia_at": "2026-12-21T00:00:00-06:00", "termina_at": "2027-01-09T23:59:00-06:00"},
	}})
	if etapas := lista(conCalendario, "etapas"); len(etapas) != 3 || etapas[0]["clave"] != "postulacion" || etapas[0]["termina_at"] != "2026-12-20T05:59:00Z" {
		t.Errorf("calendario guardado inesperado: %v", conCalendario["etapas"])
	}

	// En borrador no existe para el público; al publicarse, aparece.
	esperar(t, e.cliente().get("/convocatorias/50/compositores"), http.StatusNotFound)
	publicada := admin.estado(id(c["id"]), "publicada", "")
	if publicada["estado"] != "publicada" || publicada["siguiente"].(obj)["estado"] != "postulacion" || fmt.Sprint(bloqueadores(publicada)) != "[dotaciones]" {
		t.Errorf("convocatoria publicada inesperada: %v", publicada)
	}
	esperar(t, e.cliente().get("/convocatorias/50/compositores"), http.StatusOK, "Compositores 2028", "<strong>bases</strong>", "Próxima apertura")
	esperar(t, e.cliente().get("/"), http.StatusOK, "Compositores 2028")

	// Con convocatorias publicadas, el número de la edición ya no cambia: de él
	// dependen los enlaces permanentes.
	if campos := admin.rechaza("PUT", rutaEdicion, obj{"numero": 60, "anio": 2028, "nombre_es": "Edición 50"}); fmt.Sprint(campos) != "[numero]" {
		t.Errorf("cambio de número con convocatorias publicadas: %v", campos)
	}
	// Despublicar es un retroceso: exige motivo.
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "borrador"}); fmt.Sprint(campos) != "[motivo]" {
		t.Errorf("despublicar sin motivo: %v", campos)
	}
	if despublicada := admin.estado(id(c["id"]), "borrador", "Se publicó por error"); despublicada["estado"] != "borrador" {
		t.Errorf("no se despublicó: %v", despublicada["estado"])
	}
	esperar(t, e.cliente().get("/convocatorias/50/compositores"), http.StatusNotFound)
}

func TestDotacionesRequisitosYCriterios(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	_, convocatoriaID := e.compositores2028(admin)
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d", convocatoriaID)

	// Catálogo de dotaciones.
	c := admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Cuarteto de cuerdas", "nombre_en": "String quartet", "orden": 1})
	c = admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Obra electroacústica", "orden": 2})
	dotaciones := lista(c, "dotaciones")
	if len(dotaciones) != 2 || dotaciones[0]["nombre_es"] != "Cuarteto de cuerdas" || dotaciones[1]["activa"] != true {
		t.Fatalf("dotaciones inesperadas: %v", dotaciones)
	}
	electro := id(dotaciones[1]["id"])
	if estado, _ := admin.api("POST", ruta+"/dotaciones", obj{"nombre_es": "Cuarteto de cuerdas"}); estado != http.StatusConflict {
		t.Errorf("nombre de dotación repetido: %d", estado)
	}
	if campos := admin.rechaza("POST", ruta+"/dotaciones", obj{"nombre_es": "", "orden": -1}); fmt.Sprint(campos) != "[nombre_es orden]" {
		t.Errorf("dotación inválida: %v", campos)
	}
	// Una dotación no se borra: se retira del catálogo.
	retirada := admin.exige(http.StatusOK, "PUT", fmt.Sprintf("/api/v1/dotaciones/%d", id(dotaciones[0]["id"])),
		obj{"nombre_es": "Cuarteto de cuerdas", "nombre_en": "String quartet", "orden": 1, "activa": false})
	if lista(retirada, "dotaciones")[0]["activa"] != false {
		t.Errorf("la dotación debió quedar inactiva: %v", retirada["dotaciones"])
	}

	// Requisito específico de una dotación: la electroacústica no lleva particellas.
	particellas := obj{"dotacion_id": electro, "clave": "particellas", "etiqueta_es": "Particellas", "ambito": "obra",
		"obligatorio": false, "mimes_permitidos": []string{"application/pdf"}, "tamano_max_bytes": 30 << 20}
	c = admin.exige(http.StatusCreated, "POST", ruta+"/requisitos", particellas)
	requisitos := lista(c, "requisitos")
	if len(requisitos) != 5 {
		t.Fatalf("se esperaban 5 requisitos, hay %d", len(requisitos))
	}
	especifico := requisitos[slices.IndexFunc(requisitos, func(r obj) bool { return id(r["dotacion_id"]) == electro })]
	if especifico["obligatorio"] != false || especifico["max_archivos"].(float64) != 1 {
		t.Errorf("requisito específico inesperado: %v", especifico)
	}
	if estado, _ := admin.api("POST", ruta+"/requisitos", particellas); estado != http.StatusConflict {
		t.Errorf("requisito repetido para la misma dotación: %d", estado)
	}
	// La regla efectiva la resuelve el dominio: general para el cuarteto, específica para la electroacústica.
	cfg, err := e.repo.ConfiguracionDeConvocatoria(context.Background(), convocatoriaID)
	if err != nil {
		t.Fatal(err)
	}
	obligatoria := func(dotacionID int64) bool {
		aplicables := models.RequisitosAplicables(cfg.Requisitos, dotacionID)
		if len(aplicables) != 4 {
			t.Fatalf("se esperaban 4 requisitos aplicables, hay %d", len(aplicables))
		}
		return aplicables[slices.IndexFunc(aplicables, func(r models.RequisitoDocumental) bool { return r.Clave == "particellas" })].Obligatorio
	}
	if !obligatoria(id(dotaciones[0]["id"])) || obligatoria(electro) {
		t.Error("las particellas deben ser obligatorias para el cuarteto y opcionales para la obra electroacústica")
	}

	for nombre, caso := range map[string]struct {
		cambio obj
		campos string
	}{
		"formato no admitido":         {obj{"mimes_permitidos": []string{"text/html"}}, "[mimes_permitidos]"},
		"sin formatos":                {obj{"mimes_permitidos": []string{}}, "[mimes_permitidos]"},
		"dotación de otra parte":      {obj{"dotacion_id": 9999}, "[dotacion_id]"},
		"de postulación por dotación": {obj{"ambito": "postulacion"}, "[ambito]"},
		"clave con espacios":          {obj{"clave": "Mi Clave"}, "[clave]"},
		"tamaño desmedido":            {obj{"tamano_max_bytes": 1 << 40}, "[tamano_max_bytes]"},
		"sin etiqueta":                {obj{"etiqueta_es": ""}, "[etiqueta_es]"},
		"demasiados archivos":         {obj{"max_archivos": 99}, "[max_archivos]"},
	} {
		invalido := obj{"clave": "otra"}
		for k, v := range particellas {
			if k != "clave" {
				invalido[k] = v
			}
		}
		for k, v := range caso.cambio {
			invalido[k] = v
		}
		if campos := admin.rechaza("POST", ruta+"/requisitos", invalido); fmt.Sprint(campos) != caso.campos {
			t.Errorf("%s: campos %v, se esperaban %s", nombre, campos, caso.campos)
		}
	}
	// Editar un requisito: subir el tamaño máximo del audio.
	audio := requisitos[slices.IndexFunc(requisitos, func(r obj) bool { return r["clave"] == "audio" })]
	audio["tamano_max_bytes"] = 150 << 20
	rutaAudio := fmt.Sprintf("/api/v1/requisitos/%d", id(audio["id"]))
	delete(audio, "id")
	editado := admin.exige(http.StatusOK, "PUT", rutaAudio, audio)
	if r := lista(editado, "requisitos"); r[slices.IndexFunc(r, func(r obj) bool { return r["clave"] == "audio" })]["tamano_max_bytes"].(float64) != 150<<20 {
		t.Errorf("el requisito no se actualizó: %v", editado["requisitos"])
	}

	// Criterios: un tercero, configurable (RF-EVA-10).
	c = admin.exige(http.StatusCreated, "POST", ruta+"/criterios", obj{"clave": "originalidad", "etiqueta_es": "Originalidad", "escala_min": 1, "escala_max": 10, "orden": 3})
	criterios := lista(c, "criterios")
	if len(criterios) != 3 || criterios[2]["obligatorio"] != true || criterios[2]["escala_max"].(float64) != 10 {
		t.Errorf("criterios inesperados: %v", criterios)
	}
	if campos := admin.rechaza("POST", ruta+"/criterios", obj{"clave": "x y", "etiqueta_es": "", "escala_min": 5, "escala_max": 5}); fmt.Sprint(campos) != "[clave etiqueta_es escala_max]" {
		t.Errorf("criterio inválido: %v", campos)
	}
	if estado, _ := admin.api("POST", ruta+"/criterios", obj{"clave": "artistico", "etiqueta_es": "Otro", "escala_min": 1, "escala_max": 5}); estado != http.StatusConflict {
		t.Errorf("clave de criterio repetida: %d", estado)
	}
	// En borrador puede eliminarse lo que sobra; después ya no.
	rutaCriterio := fmt.Sprintf("/api/v1/criterios/%d", id(criterios[2]["id"]))
	rutaRequisito := fmt.Sprintf("/api/v1/requisitos/%d", id(especifico["id"]))
	admin.exige(http.StatusNoContent, "DELETE", rutaCriterio, nil)
	if estado, _ := admin.api("DELETE", rutaCriterio, nil); estado != http.StatusNotFound {
		t.Errorf("borrar dos veces: %d", estado)
	}
	admin.estado(convocatoriaID, "publicada", "")
	if campos := admin.rechaza("DELETE", rutaRequisito, nil); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("borrar con la convocatoria publicada: %v", campos)
	}
	// En Intérpretes no hay dotaciones.
	interpretes := id(lista(admin.exige(http.StatusOK, "GET", "/api/v1/ediciones", nil), "ediciones")[1]["convocatorias"].([]any)[0].(obj)["id"])
	if campos := admin.rechaza("POST", fmt.Sprintf("/api/v1/convocatorias/%d/dotaciones", interpretes), obj{"nombre_es": "Dúo"}); fmt.Sprint(campos) != "[tipo]" {
		t.Errorf("dotación en Intérpretes: %v", campos)
	}
}

func TestCuentasInternasAsignacionesYCuotas(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	_, convocatoriaID := e.compositores2028(admin)
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d", convocatoriaID)
	admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Cuarteto de cuerdas"})
	admin.estado(convocatoriaID, "publicada", "")
	admin.estado(convocatoriaID, "postulacion", "")

	// Alta de una cuenta interna: sin contraseña, sin perfil global y con su
	// correo de activación en la bandeja de salida.
	cuenta := admin.exige(http.StatusCreated, "POST", "/api/v1/usuarios",
		obj{"email": "revisora@ejemplo.mx", "nombre": "Rita", "apellidos": "Revisora", "idioma": "en"})
	if cuenta["activada"] != false || len(cuenta["perfiles"].([]any)) != 0 || cuenta["idioma"] != "en" {
		t.Errorf("cuenta interna inesperada: %v", cuenta)
	}
	revisora := id(cuenta["id"])
	token := e.tokenDe("revisora@ejemplo.mx", models.PlantillaActivacion, "/cuenta/activar")
	if estado, r := admin.api("POST", "/api/v1/usuarios", obj{"email": "REVISORA@ejemplo.mx", "nombre": "X", "apellidos": "Y"}); estado != http.StatusConflict {
		t.Errorf("correo repetido: %d %v", estado, r)
	}
	if campos := admin.rechaza("POST", "/api/v1/usuarios", obj{"email": "no-es-correo", "nombre": "", "apellidos": "Y", "perfil_global": "postulante"}); fmt.Sprint(campos) != "[email nombre perfil_global]" {
		t.Errorf("cuenta interna inválida: %v", campos)
	}
	// Un observador se crea con su perfil global.
	if observador := admin.exige(http.StatusCreated, "POST", "/api/v1/usuarios", obj{"email": "observa@ejemplo.mx", "nombre": "O", "apellidos": "B", "perfil_global": "coordinacion_nacional"}); fmt.Sprint(observador["perfiles"]) != "[coordinacion_nacional]" {
		t.Errorf("observador inesperado: %v", observador)
	}
	// Reenviar la activación emite un enlace nuevo y anula el anterior.
	admin.exige(http.StatusOK, "POST", fmt.Sprintf("/api/v1/usuarios/%d/activacion", revisora), nil)
	esperar(t, e.cliente().get("/cuenta/activar?token="+token), http.StatusGone)
	token = e.tokenDe("revisora@ejemplo.mx", models.PlantillaActivacion, "/cuenta/activar")

	// Sin revisor no se pasa a revisión documental (RN-14).
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "revision_documental"}); fmt.Sprint(campos) != "[asignaciones]" {
		t.Errorf("revisión documental sin revisor: %v", campos)
	}
	c := admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones", obj{"usuario_id": revisora, "perfil": "revisor_documental"})
	if cuotas := c["cuotas"].(obj); cuotas["revisores"].(float64) != 1 || cuotas["jurados"].(float64) != 0 || cuotas["jurados_requeridos"].(float64) != 3 {
		t.Errorf("cuotas inesperadas: %v", cuotas)
	}
	if estado, _ := admin.api("POST", ruta+"/asignaciones", obj{"usuario_id": revisora, "perfil": "revisor_documental"}); estado != http.StatusConflict {
		t.Errorf("asignación repetida: %d", estado)
	}
	for nombre, cuerpo := range map[string]obj{
		"perfil global como asignación": {"usuario_id": revisora, "perfil": "admin_sistema"},
		"cuenta inexistente":            {"usuario_id": 9999, "perfil": "jurado"},
	} {
		if campos := admin.rechaza("POST", ruta+"/asignaciones", cuerpo); len(campos) != 1 {
			t.Errorf("%s: %v", nombre, campos)
		}
	}
	admin.estado(convocatoriaID, "revision_documental", "")

	// La revisora activa su cuenta: su sesión trae la asignación y dura 8 horas,
	// pero ser revisora no le da acceso a la configuración.
	rita := e.cliente()
	redirige(t, rita.post("/cuenta/activar", url.Values{"token": {token}, "password": {passwordPrueba}}), "/cuenta/ingresar?aviso=activada")
	rita.ingresar("revisora@ejemplo.mx", passwordPrueba)
	_, sesion := rita.json("/api/v1/sesion")
	asignaciones := lista(sesion, "asignaciones")
	if len(asignaciones) != 1 || id(asignaciones[0]["convocatoria_id"]) != convocatoriaID || asignaciones[0]["perfil"] != "revisor_documental" || len(sesion["perfiles"].([]any)) != 0 {
		t.Errorf("sesión de la revisora inesperada: %v", sesion)
	}
	if estado, _ := rita.api("GET", ruta, nil); estado != http.StatusForbidden {
		t.Errorf("una revisora no debe leer la configuración: %d", estado)
	}

	// Evaluación: exactamente tres jurados. Ni dos ni cuatro.
	var jurados []int64
	for i := 1; i <= 4; i++ {
		jurados = append(jurados, e.crearInterno(admin, fmt.Sprintf("jurado%d@ejemplo.mx", i)))
	}
	for _, jurado := range jurados[:2] {
		admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones", obj{"usuario_id": jurado, "perfil": "jurado"})
	}
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "evaluacion"}); fmt.Sprint(campos) != "[asignaciones]" {
		t.Errorf("evaluación con dos jurados: %v", campos)
	}
	c = admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones", obj{"usuario_id": jurados[2], "perfil": "jurado"})
	if campos := admin.rechaza("POST", ruta+"/asignaciones", obj{"usuario_id": jurados[3], "perfil": "jurado"}); fmt.Sprint(campos) != "[perfil]" {
		t.Errorf("cuarto jurado: %v", campos)
	}
	if len(bloqueadores(c)) != 0 || c["siguiente"].(obj)["estado"] != "evaluacion" {
		t.Errorf("con tres jurados no debe haber bloqueadores: %v", c["siguiente"])
	}
	// La revisora puede ser también jurado: no hay regla que lo impida... salvo la cuota.
	if campos := admin.rechaza("POST", ruta+"/asignaciones", obj{"usuario_id": revisora, "perfil": "jurado"}); fmt.Sprint(campos) != "[perfil]" {
		t.Errorf("revisora como cuarto jurado: %v", campos)
	}

	// Sustituir a un jurado: se retira uno (su fila se conserva) y entra otro.
	var asignacionRetirada int64
	for _, a := range lista(c, "asignaciones") {
		if id(a["usuario"].(obj)["id"]) == jurados[0] {
			asignacionRetirada = id(a["id"])
		}
	}
	c = admin.exige(http.StatusOK, "DELETE", fmt.Sprintf("/api/v1/asignaciones/%d", asignacionRetirada), nil)
	if c["cuotas"].(obj)["jurados"].(float64) != 2 || len(lista(c, "asignaciones")) != 4 {
		t.Errorf("tras retirar un jurado: %v", c["cuotas"])
	}
	if estado, _ := admin.api("DELETE", fmt.Sprintf("/api/v1/asignaciones/%d", asignacionRetirada), nil); estado != http.StatusNotFound {
		t.Errorf("retirar dos veces: %d", estado)
	}
	admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones", obj{"usuario_id": jurados[3], "perfil": "jurado"})
	// Si vuelve el primero ya no cabe; si se va el cuarto, el primero recupera SU asignación.
	if campos := admin.rechaza("POST", ruta+"/asignaciones", obj{"usuario_id": jurados[0], "perfil": "jurado"}); fmt.Sprint(campos) != "[perfil]" {
		t.Errorf("reasignar con la cuota llena: %v", campos)
	}
	c = admin.estado(convocatoriaID, "evaluacion", "")

	// Con la evaluación en marcha los criterios ya no se tocan.
	criterio := lista(c, "criterios")[0]
	if campos := admin.rechaza("PUT", fmt.Sprintf("/api/v1/criterios/%d", id(criterio["id"])),
		obj{"clave": "realizacion", "etiqueta_es": "Otro nombre", "escala_min": 1, "escala_max": 10}); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("editar criterios en evaluación: %v", campos)
	}

	// La búsqueda de cuentas localiza a quien ya tiene una.
	encontradas := admin.exige(http.StatusOK, "GET", "/api/v1/usuarios?q=JURADO&tamano=3", nil)
	if encontradas["total"].(float64) != 4 || len(lista(encontradas, "usuarios")) != 3 {
		t.Errorf("búsqueda de cuentas inesperada: %v", encontradas)
	}
	if todas := admin.exige(http.StatusOK, "GET", "/api/v1/usuarios?q=%25", nil); todas["total"].(float64) != 0 {
		t.Errorf("un comodín escrito por el usuario debe buscarse como texto: %v", todas["total"])
	}
	// La cuenta ya activada no admite otro enlace de activación.
	if campos := admin.rechaza("POST", fmt.Sprintf("/api/v1/usuarios/%d/activacion", revisora), nil); fmt.Sprint(campos) != "[id]" {
		t.Errorf("reenviar activación a una cuenta activada: %v", campos)
	}

	// Rastro: cada asignación y cada retiro, sin un solo correo en la bitácora.
	if n := len(e.asientos("asignacion.otorgar")); n != 5 {
		t.Errorf("%d asientos de asignación, se esperaban 5", n)
	}
	if retiros := e.asientos("asignacion.retirar"); len(retiros) != 1 || !strings.Contains(retiros[0].Antes, `"perfil":"jurado"`) {
		t.Errorf("asiento del retiro incorrecto: %+v", retiros)
	}
	asientos, _, _ := e.repo.ConsultarAuditoria(context.Background(), models.FiltroAuditoria{Pagina: 1, Tamano: 200})
	for _, a := range asientos {
		if strings.Contains(a.Antes+a.Despues, "@") {
			t.Errorf("el asiento %d (%s) contiene un correo: %s", a.ID, a.Accion, a.Despues)
		}
	}
}

func TestAvanceDeEtapaPorCalendario(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	_, convocatoriaID := e.compositores2028(admin)
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d", convocatoriaID)
	trabajador := e.trabajadorEtapas()
	ctx := context.Background()
	estado := func() string {
		trabajador.Procesar(ctx)
		return admin.exige(http.StatusOK, "GET", ruta, nil)["estado"].(string)
	}
	// saltar adelanta el reloj. Tras días sin uso la sesión del administrador
	// (8 horas de inactividad) ya venció: vuelve a ingresar.
	saltar := func(instante string) {
		e.hasta(instante)
		admin.ingresar("admin@ejemplo.mx", passwordPrueba)
	}

	// Un borrador no avanza solo: publicar es siempre un acto explícito.
	saltar("2026-11-25T00:00:00Z")
	if s := estado(); s != "borrador" {
		t.Fatalf("un borrador no debe publicarse solo: %s", s)
	}
	admin.estado(convocatoriaID, "publicada", "")
	// Ya pasó la fecha de apertura, pero falta una dotación: no avanza.
	if s := estado(); s != "publicada" {
		t.Fatalf("sin dotaciones no debe abrirse la postulación: %s", s)
	}
	admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Cuarteto de cuerdas"})
	if s := estado(); s != "postulacion" {
		t.Fatalf("cumplida la regla y llegada la fecha, debe abrirse la postulación: %s", s)
	}
	// El avance automático queda en la bitácora a nombre del sistema.
	cambios := e.asientos("convocatoria.estado")
	if ultimo := cambios[0]; ultimo.ActorPerfil != models.ActorSistema || !strings.Contains(ultimo.Despues, `"automatico":true`) ||
		!strings.Contains(ultimo.Antes, `"estado":"publicada"`) {
		t.Errorf("asiento del avance automático incorrecto: %+v", ultimo)
	}

	// Vence el plazo de postulación sin revisor asignado: la convocatoria se queda.
	saltar("2026-12-20T06:00:00Z")
	if s := estado(); s != "postulacion" {
		t.Fatalf("sin revisor no debe pasar a revisión documental: %s", s)
	}
	admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones",
		obj{"usuario_id": e.crearInterno(admin, "revisor@ejemplo.mx"), "perfil": "revisor_documental"})
	if s := estado(); s != "revision_documental" {
		t.Fatalf("con revisor debe pasar a revisión documental: %s", s)
	}

	// Reabrir un plazo vencido no se hace retrocediendo a ciegas: el calendario
	// volvería a cerrarlo de inmediato. Primero se cambia la fecha.
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "postulacion", "motivo": "Prórroga"}); fmt.Sprint(campos) != "[etapas]" {
		t.Errorf("retroceso contra el calendario: %v", campos)
	}
	prorroga := etapasDePrueba()
	prorroga["etapas"].([]obj)[0]["termina_at"] = "2026-12-21T05:59:00Z"
	admin.exige(http.StatusOK, "PUT", ruta+"/etapas", prorroga)
	admin.estado(convocatoriaID, "postulacion", "Prórroga de un día por falla del servidor")
	if s := estado(); s != "postulacion" {
		t.Fatalf("con la prórroga vigente no debe cerrarse: %s", s)
	}
	saltar("2026-12-21T06:00:00Z")
	if s := estado(); s != "revision_documental" {
		t.Fatalf("vencida la prórroga debe cerrarse: %s", s)
	}

	// Si el servicio estuvo detenido y vencieron varias fechas, avanza lo que
	// las reglas permitan: llega a evaluación sólo si hay tres jurados.
	saltar("2027-02-15T00:00:00Z")
	if s := estado(); s != "revision_documental" {
		t.Fatalf("sin jurados no debe pasar a evaluación: %s", s)
	}
	for i := 1; i <= 3; i++ {
		admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones",
			obj{"usuario_id": e.crearInterno(admin, fmt.Sprintf("jurado%d@ejemplo.mx", i)), "perfil": "jurado"})
	}
	// De revisión a evaluación y, vencida también ésta, a cerrada: dos pasos en una pasada.
	if n := trabajador.Procesar(ctx); n != 2 {
		t.Errorf("se esperaban dos avances en una pasada, hubo %d", n)
	}
	cerrada := admin.exige(http.StatusOK, "GET", ruta, nil)
	if cerrada["estado"] != "cerrada" || cerrada["siguiente"].(obj)["estado"] != "archivada" {
		t.Fatalf("debió quedar cerrada: %v", cerrada["estado"])
	}
	// Archivar nunca es automático.
	if s := estado(); s != "cerrada" {
		t.Fatalf("una convocatoria no se archiva sola: %s", s)
	}

	// Cerrada o archivada, su configuración es de sólo lectura, también para el administrador.
	for nombre, intento := range map[string]func() []string{
		"textos":     func() []string { return admin.rechaza("PUT", ruta, obj{"titulo_es": "Otro título"}) },
		"calendario": func() []string { return admin.rechaza("PUT", ruta+"/etapas", etapasDePrueba()) },
		"dotación":   func() []string { return admin.rechaza("POST", ruta+"/dotaciones", obj{"nombre_es": "Otra"}) },
		"asignación": func() []string {
			return admin.rechaza("POST", ruta+"/asignaciones", obj{"usuario_id": 1, "perfil": "revisor_documental"})
		},
	} {
		if campos := intento(); fmt.Sprint(campos) != "[estado]" {
			t.Errorf("%s en una convocatoria cerrada: %v", nombre, campos)
		}
	}
	archivada := admin.estado(convocatoriaID, "archivada", "")
	if archivada["siguiente"] != nil {
		t.Errorf("una convocatoria archivada no tiene etapa siguiente: %v", archivada["siguiente"])
	}
	// Archivada sigue consultable por su enlace permanente, pero ya no se anuncia.
	esperar(t, e.cliente().get("/convocatorias/50/compositores"), http.StatusOK, "Archivada")
	if strings.Contains(e.cliente().get("/").Body.String(), "Compositores 2028") {
		t.Error("una convocatoria archivada no debe anunciarse en la portada")
	}
	// Desarchivar es explícito y con motivo.
	if campos := admin.rechaza("POST", ruta+"/estado", obj{"estado": "cerrada"}); fmt.Sprint(campos) != "[motivo]" {
		t.Errorf("desarchivar sin motivo: %v", campos)
	}
	admin.estado(convocatoriaID, "cerrada", "Corrección de un dictamen")
}

func TestDuplicarConvocatoria(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	_, origenID := e.compositores2028(admin)
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d", origenID)
	c := admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Cuarteto de cuerdas", "orden": 1})
	c = admin.exige(http.StatusCreated, "POST", ruta+"/dotaciones", obj{"nombre_es": "Obra electroacústica", "orden": 2})
	electro := id(lista(c, "dotaciones")[1]["id"])
	admin.exige(http.StatusCreated, "POST", ruta+"/requisitos", obj{"dotacion_id": electro, "clave": "particellas", "etiqueta_es": "Particellas",
		"ambito": "obra", "obligatorio": false, "mimes_permitidos": []string{"application/pdf"}, "tamano_max_bytes": 1 << 20})
	admin.exige(http.StatusCreated, "POST", ruta+"/asignaciones", obj{"usuario_id": e.crearInterno(admin, "revisor@ejemplo.mx"), "perfil": "revisor_documental"})

	siguiente := admin.exige(http.StatusCreated, "POST", "/api/v1/ediciones", obj{"numero": 51, "anio": 2029, "nombre_es": "Edición 51"})
	copia := admin.exige(http.StatusCreated, "POST", ruta+"/duplicar", obj{"edicion_id": id(siguiente["id"])})

	// Se copian textos, límites, dotaciones, requisitos y criterios…
	if copia["estado"] != "borrador" || copia["titulo_es"] != "Convocatoria de Compositores 2028" || copia["bases_md_es"] != "# Bases\n\nTexto." ||
		copia["edicion"].(obj)["numero"].(float64) != 51 || copia["max_obras"].(float64) != 2 ||
		len(lista(copia, "dotaciones")) != 2 || len(lista(copia, "requisitos")) != 5 || len(lista(copia, "criterios")) != 2 {
		t.Errorf("copia inesperada: %v", copia)
	}
	// …pero no el calendario ni las asignaciones, que son de cada año.
	if len(copia["etapas"].([]any)) != 0 || len(copia["asignaciones"].([]any)) != 0 {
		t.Errorf("el calendario y las asignaciones no deben copiarse: %v / %v", copia["etapas"], copia["asignaciones"])
	}
	// El requisito específico apunta a la dotación COPIADA, no a la del año anterior.
	nuevaElectro := id(lista(copia, "dotaciones")[1]["id"])
	requisitos := lista(copia, "requisitos")
	especifico := requisitos[slices.IndexFunc(requisitos, func(r obj) bool { return id(r["dotacion_id"]) != 0 })]
	if nuevaElectro == electro || id(especifico["dotacion_id"]) != nuevaElectro || especifico["clave"] != "particellas" {
		t.Errorf("el requisito específico quedó apuntando a %v; la dotación copiada es %d (la original, %d)", especifico["dotacion_id"], nuevaElectro, electro)
	}
	// La original no cambió, y no se puede duplicar dos veces a la misma edición.
	if original := admin.exige(http.StatusOK, "GET", ruta, nil); len(lista(original, "asignaciones")) != 1 || len(original["etapas"].([]any)) != 3 {
		t.Errorf("la convocatoria de origen cambió: %v", original)
	}
	if estado, _ := admin.api("POST", ruta+"/duplicar", obj{"edicion_id": id(siguiente["id"])}); estado != http.StatusConflict {
		t.Errorf("duplicar dos veces a la misma edición: %d", estado)
	}
	if estado, _ := admin.api("POST", ruta+"/duplicar", obj{"edicion_id": 9999}); estado != http.StatusNotFound {
		t.Errorf("duplicar a una edición inexistente: %d", estado)
	}
}
