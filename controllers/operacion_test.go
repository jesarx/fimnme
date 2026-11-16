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
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de las pantallas de operación (paso 13d): plantillas de correo con
// su API, bitácora, importación asistida y anonimización ARCO.

func TestPlantillasDeCorreo(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	observador := e.activarCuentaInterna("coordinacion@ejemplo.mx", models.PerfilCoordinacionNacional)
	editor := "/administracion/correos/verificar_email/es"

	// La lista y el editor; el observador los ve sin poder cambiar nada.
	esperar(t, admin.get("/administracion/correos"), http.StatusOK, "Verificación de correo", "Acuse de recibo", "Versión 1",
		`href="/administracion/correos/acuse/en"`)
	w := admin.get(editor)
	esperar(t, w, http.StatusOK, "<code>{{liga}}</code>", "Indispensable", "Vista previa", "Ver vista previa", "Enviarme una prueba",
		"https://postula.ejemplo.mx/cuenta")
	esperar(t, observador.get(editor), http.StatusOK, `<fieldset class="admin__campos" disabled>`)
	noAparece(t, observador.get(editor), "Guardar como versión nueva")
	if w := observador.post(editor, url.Values{"accion": {"guardar"}, "asunto": {"x"}, "cuerpo_md": {"{{liga}}"}}); w.Code != http.StatusForbidden {
		t.Errorf("el observador guardó una plantilla: %d", w.Code)
	}
	if estado, _ := observador.api("GET", "/api/v1/plantillas-correo", nil); estado != http.StatusForbidden {
		t.Errorf("el observador en la API de plantillas: %d", estado)
	}
	for _, ruta := range []string{"/administracion/correos/inventada/es", "/administracion/correos/acuse/fr"} {
		if w := admin.get(ruta); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", ruta, w.Code)
		}
	}

	// Vista previa sin guardar: lo escrito, con las variables señaladas.
	w = admin.post(editor, url.Values{"accion": {"vista_previa"}, "asunto": {"Hola {{nombre}}"}, "cuerpo_md": {"Visita {{sitio}}."}})
	esperar(t, w, http.StatusOK, "Variables que esta plantilla no recibe: <code>{{sitio}}</code>", "Variables indispensables que faltan: <code>{{liga}}</code>",
		"Visita {{sitio}}.", `value="Hola {{nombre}}"`)
	// Guardar lo que no vale: se rechaza con lo escrito.
	esperar(t, admin.post(editor, url.Values{"accion": {"guardar"}, "asunto": {"Hola"}, "cuerpo_md": {"Visita {{sitio}} y {{liga}}."}}),
		http.StatusUnprocessableEntity, "usa una variable que esta plantilla no recibe", "Visita {{sitio}} y {{liga}}.")
	esperar(t, admin.post(editor, url.Values{"accion": {"guardar"}, "asunto": {"Hola"}, "cuerpo_md": {"Sin enlace."}}),
		http.StatusUnprocessableEntity, "Falta una variable indispensable")
	esperar(t, admin.post(editor, url.Values{"accion": {"guardar"}, "asunto": {""}, "cuerpo_md": {"{{liga}}"}}), http.StatusUnprocessableEntity)

	// Una versión nueva: pasa a ser la vigente y la usa el siguiente correo.
	redirige(t, admin.post(editor, url.Values{"accion": {"guardar"}, "asunto": {"Confirma tu correo, {{nombre}}"},
		"cuerpo_md": {"Hola {{nombre}}:\r\n\r\nEntra a {{liga}} para confirmar."}}), editor+"?aviso=guardada#editor")
	esperar(t, admin.get(editor+"?aviso=guardada"), http.StatusOK, "Versión guardada", "Vigente", "Confirma tu correo, {{nombre}}", "Cargar en el editor")
	esperar(t, admin.get(editor+"?version=1"), http.StatusOK, "En el editor está la versión 1 (la vigente es la 2)")
	if len(e.asientos("plantilla_correo.version")) != 1 {
		t.Error("la versión nueva no quedó en la bitácora")
	}
	e.postulanteVerificado("lucia@ejemplo.mx")
	usado := false
	for _, correo := range e.correos() {
		if correo.Destinatario == "lucia@ejemplo.mx" && strings.HasPrefix(correo.Asunto, "Confirma tu correo, ") && strings.Contains(correo.Texto, "para confirmar") {
			usado = true
		}
	}
	if !usado {
		t.Error("el correo de verificación no usó la versión nueva")
	}

	// Correo de prueba: a quien lo pide, marcado.
	redirige(t, admin.post(editor, url.Values{"accion": {"prueba"}, "asunto": {"Prueba {{nombre}}"}, "cuerpo_md": {"Enlace: {{liga}}"}}), editor+"?aviso=prueba#editor")
	correos := e.correos()
	if ultimo := correos[len(correos)-1]; ultimo.Destinatario != "admin@ejemplo.mx" || !strings.HasPrefix(ultimo.Asunto, "[PRUEBA] Prueba Admin") {
		t.Errorf("correo de prueba: %s %q", ultimo.Destinatario, ultimo.Asunto)
	}

	// La API: catálogo, historial, versión nueva en inglés, vista previa y prueba.
	catalogo := admin.exige(http.StatusOK, "GET", "/api/v1/plantillas-correo", nil)
	if len(lista(catalogo, "plantillas")) != len(models.PlantillasDeCorreo) {
		t.Errorf("catálogo: %v", catalogo)
	}
	nueva := admin.exige(http.StatusCreated, "POST", "/api/v1/plantillas-correo/acuse/en", obj{"asunto": "Received: {{folio}}", "cuerpo_md": "Thanks, {{nombre}}."})
	if versiones := lista(nueva, "versiones"); len(versiones) < 1 || versiones[0]["asunto"] != "Received: {{folio}}" {
		t.Errorf("versión nueva por la API: %v", nueva)
	}
	if campos := admin.rechaza("POST", "/api/v1/plantillas-correo/acuse/en", obj{"asunto": "Received", "cuerpo_md": "Thanks, {{nombre}}."}); fmt.Sprint(campos) != "[cuerpo_md]" {
		t.Errorf("falta el folio: %v", campos)
	}
	previa := admin.exige(http.StatusOK, "POST", "/api/v1/plantillas-correo/acuse/es/vista-previa", obj{"asunto": "{{folio}}", "cuerpo_md": "<b>{{nombre}}</b> {{x}}"})
	if previa["asunto"] != "FIM49-INT-0137" || strings.Contains(previa["html"].(string), "<b>") || fmt.Sprint(previa["variables_desconocidas"]) != "[x]" {
		t.Errorf("vista previa por la API: %v", previa)
	}
	if prueba := admin.exige(http.StatusAccepted, "POST", "/api/v1/plantillas-correo/acuse/en/prueba", nil); prueba["destinatario"] != "admin@ejemplo.mx" {
		t.Errorf("prueba por la API: %v", prueba)
	}
	admin.exige(http.StatusNotFound, "GET", "/api/v1/plantillas-correo/acuse/fr", nil)
}

// TestPlantillasConLasVariablesQueLlegan comprueba que cada plantilla
// sembrada use sólo variables de su catálogo y tenga las indispensables: si
// un controlador deja de pasar una variable, o el catálogo se equivoca, aquí
// se nota.
func TestPlantillasConLasVariablesQueLlegan(t *testing.T) {
	e := nuevoEntorno(t)
	for _, d := range models.PlantillasDeCorreo {
		for _, idioma := range []string{models.IdiomaES, models.IdiomaEN} {
			p, err := e.repo.PlantillaCorreo(context.Background(), d.Clave, idioma)
			if err != nil {
				t.Fatalf("%s/%s: %v", d.Clave, idioma, err)
			}
			if errores := models.ValidarVersionDePlantilla(p); len(errores) > 0 {
				t.Errorf("la plantilla sembrada %s/%s no pasa su propia validación: %v (desconocidas %v, faltantes %v)",
					d.Clave, idioma, errores, d.VariablesDesconocidas(p.Asunto+p.CuerpoMD), d.VariablesFaltantes(p.Asunto+p.CuerpoMD))
			}
		}
	}
}

func TestBitacoraEnPantalla(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	observador := e.activarCuentaInterna("coordinacion@ejemplo.mx", models.PerfilCoordinacionNacional)
	revisor := e.postulanteVerificado("ana@ejemplo.mx")
	_, convocatoria := e.compositores2028(admin)

	if w := revisor.get("/administracion/bitacora"); w.Code != http.StatusForbidden {
		t.Errorf("una postulante en la bitácora: %d", w.Code)
	}
	w := observador.get("/administracion/bitacora?accion=convocatoria.crear")
	esperar(t, w, http.StatusOK, "Registros: 3", "<code>convocatoria.crear</code>", "Admin Sistema · admin@ejemplo.mx",
		fmt.Sprintf(`href="/administracion/bitacora?entidad=convocatoria&amp;entidad_id=%d"`, convocatoria), "/api/v1/auditoria?accion=convocatoria.crear&amp;formato=csv")
	esperar(t, admin.get(fmt.Sprintf("/administracion/bitacora?entidad=convocatoria&entidad_id=%d", convocatoria)), http.StatusOK, "&#34;titulo_es&#34;")
	esperar(t, admin.get("/administracion/bitacora?entidad_id=x"), http.StatusUnprocessableEntity, "El filtro «Número del registro» no es válido.")
	esperar(t, admin.get("/administracion/bitacora?accion=nada.parecido"), http.StatusOK, "No hay registros con estos filtros.")
}

// subirHojaDesdeLaPagina manda el formulario de importación con sus campos y
// la hoja al final.
func (c *cliente) subirHojaDesdeLaPagina(tipo string, convocatoriaID int64, nombre string, contenido []byte) *httptest.ResponseRecorder {
	c.t.Helper()
	var cuerpo bytes.Buffer
	formulario := multipart.NewWriter(&cuerpo)
	for _, campo := range [][2]string{{"tipo", tipo}, {"convocatoria_id", fmt.Sprint(convocatoriaID)}} {
		if err := formulario.WriteField(campo[0], campo[1]); err != nil {
			c.t.Fatal(err)
		}
	}
	parte, err := formulario.CreateFormFile("archivo", nombre)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := parte.Write(contenido); err != nil {
		c.t.Fatal(err)
	}
	if err := formulario.Close(); err != nil {
		c.t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/administracion/importaciones", &cuerpo)
	r.Header.Set("Content-Type", formulario.FormDataContentType())
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return c.hacer(r)
}

func TestImportacionEnPantalla(t *testing.T) {
	e := nuevoEntorno(t)
	admin, _, compositores, cuarteto, _ := e.convocatoriasAbiertas()
	observador := e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	if w := observador.get("/administracion/importaciones"); w.Code != http.StatusForbidden {
		t.Errorf("el observador en importaciones: %d", w.Code)
	}
	esperar(t, admin.get("/administracion/importaciones"), http.StatusOK, "Subir una hoja", "Aún no hay importaciones.")

	// Lo que no puede leerse vuelve con su motivo.
	esperar(t, admin.subirHojaDesdeLaPagina("dotaciones", compositores, "hoja.csv", []byte("nombre_es\nD\xFAo\n")), http.StatusUnprocessableEntity,
		"El archivo no pudo leerse")

	// Fase Analyze desde la pantalla: lleva a la importación, con sus conflictos.
	hoja := []byte("nombre_es;nombre_en\r\nCuarteto de cuerdas;String quartet\r\nQuinteto de alientos;Wind quintet\r\nCuarteto;Quartet\r\n")
	destino := destinoDe(t, admin.subirHojaDesdeLaPagina("dotaciones", compositores, "dotaciones.csv", hoja))
	w := admin.get(destino)
	esperar(t, w, http.StatusOK, "Esperando resolución", "Renglón 3", "Renglón 4", "nombre parecido", fmt.Sprintf(`value="vincular:%d"`, cuarteto),
		"1 renglones no se parecen a nada", "Faltan 2 renglones por resolver")
	noAparece(t, w, `name="confirmo"`)
	importacion, err := e.repo.ImportacionPorID(context.Background(), idDeRutaAPI(t, destino))
	if err != nil || len(importacion.Conflictos) != 2 {
		t.Fatalf("importación: %+v %v", importacion, err)
	}
	var parecido int64
	for _, c := range importacion.Conflictos {
		if len(c.Candidatos) > 0 {
			parecido = c.ID
		}
	}

	// Resolver: una elección inventada se rechaza; luego, vincular y crear.
	esperar(t, admin.post(fmt.Sprintf("%s/conflictos/%d", destino, parecido), url.Values{"eleccion": {"fusionar"}}), http.StatusUnprocessableEntity)
	redirige(t, admin.post(fmt.Sprintf("%s/conflictos/%d", destino, parecido), url.Values{"eleccion": {fmt.Sprintf("vincular:%d", cuarteto)}}), destino+"?aviso=resuelto")
	redirige(t, admin.post(destino+"/resoluciones", url.Values{"resolucion": {"crear_nuevo"}}), destino+"?aviso=resuelto")
	esperar(t, admin.get(destino), http.StatusOK, "Vinculado", "Se creará", `name="confirmo"`)

	// Confirmar exige la casilla; después, entra.
	esperar(t, admin.post(destino+"/confirmacion", nil), http.StatusUnprocessableEntity, "Esta acción exige confirmación expresa.")
	redirige(t, admin.post(destino+"/confirmacion", url.Values{"confirmo": {"1"}}), destino+"?aviso=confirmada")
	esperar(t, admin.get(destino+"?aviso=confirmada"), http.StatusOK, "Importación aplicada.", "Confirmada", "Lo que hizo")
	cfg, _ := e.repo.ConfiguracionDeConvocatoria(context.Background(), compositores)
	if !strings.Contains(fmt.Sprint(cfg.Dotaciones), "Quinteto de alientos") {
		t.Errorf("la dotación nueva no entró: %v", cfg.Dotaciones)
	}
	// Ya confirmada, no se cancela.
	esperar(t, admin.post(destino+"/cancelar", nil), http.StatusUnprocessableEntity, "ya fue confirmada o cancelada")
	esperar(t, admin.get("/administracion/importaciones"), http.StatusOK, "Dotaciones", "Confirmada")
}

func TestAnonimizacionEnPantalla(t *testing.T) {
	x := evaluada(t)
	ana := x.admin.exige(http.StatusOK, "GET", x.rutaAna, nil)
	folio := ana["folio"].(string)
	anaID := idDeRutaAPI(t, x.rutaAna)
	destino := fmt.Sprintf("/administracion/arco/%d", anaID)

	for nombre, c := range map[string]*cliente{"observador": x.observador, "revisora": x.revisora, "titular": x.ana} {
		if w := c.get("/administracion/arco?folio=" + folio); w.Code != http.StatusForbidden {
			t.Errorf("%s en ARCO: %d", nombre, w.Code)
		}
		if w := c.post(destino, url.Values{"folio": {folio}, "fundamento": {"Solicitud ARCO 2026/014"}, "confirmo": {"1"}}); w.Code != http.StatusForbidden {
			t.Errorf("%s al anonimizar: %d", nombre, w.Code)
		}
	}
	esperar(t, x.admin.get("/administracion/arco?folio=FIM00-XXX-0000"), http.StatusOK, "No hay ninguna postulación con ese folio.")
	esperar(t, x.admin.get("/administracion/arco?folio="+folio), http.StatusOK, "Ana López Ruiz", "Descartada", "no se puede deshacer",
		fmt.Sprintf(`action="%s"`, destino))

	// La doble confirmación: el folio otra vez, el fundamento y la casilla.
	esperar(t, x.admin.post(destino, url.Values{"folio": {"FIM50-INT-9999"}, "fundamento": {"Solicitud ARCO 2026/014"}, "confirmo": {"1"}}),
		http.StatusUnprocessableEntity, "El folio escrito para confirmar no coincide", "Solicitud ARCO 2026/014")
	esperar(t, x.admin.post(destino, url.Values{"folio": {folio}, "fundamento": {"Solicitud ARCO 2026/014"}}), http.StatusUnprocessableEntity,
		"Esta acción exige confirmación expresa.")
	if p, _ := x.e.repo.PostulacionPorID(context.Background(), anaID); !p.AnonimizadaAt.IsZero() {
		t.Fatal("se anonimizó sin la confirmación completa")
	}
	esperar(t, x.admin.post(destino, url.Values{"folio": {folio}, "fundamento": {"Solicitud ARCO 2026/014"}, "confirmo": {"1"}}), http.StatusOK,
		"Postulación "+folio+" anonimizada.", "Archivos personales borrados:")
	if p, _ := x.e.repo.PostulacionPorID(context.Background(), anaID); p.AnonimizadaAt.IsZero() {
		t.Fatal("no quedó anonimizada")
	}
	esperar(t, x.admin.get("/administracion/arco?folio="+folio), http.StatusOK, "Esta postulación ya fue anonimizada.")
	if len(x.e.asientos("arco.anonimizar")) != 1 {
		t.Error("la anonimización no quedó en la bitácora")
	}
}
