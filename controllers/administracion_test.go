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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de las pantallas de administración (paso 13a): quién las ve y
// quién puede cambiar algo, la configuración completa de una convocatoria
// desde los formularios, los rechazos del modelo junto al formulario (sin
// perder lo escrito), y las cuentas internas.

// destinoDe exige una redirección y devuelve su destino.
func destinoDe(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Code != http.StatusSeeOther {
		t.Fatalf("se esperaba una redirección, llegó %d: %s", w.Code, w.Body.String())
	}
	return w.Header().Get("Location")
}

// subirAlFormulario envía un archivo por un formulario de página (multipart).
func (c *cliente) subirAlFormulario(ruta, nombre string, contenido []byte) *httptest.ResponseRecorder {
	c.t.Helper()
	var cuerpo bytes.Buffer
	formulario := multipart.NewWriter(&cuerpo)
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
	r := httptest.NewRequest(http.MethodPost, ruta, &cuerpo)
	r.Header.Set("Content-Type", formulario.FormDataContentType())
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return c.hacer(r)
}

func TestQuienEntraALaAdministracion(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	observador := e.activarCuentaInterna("coordinacion@ejemplo.mx", models.PerfilCoordinacionNacional)
	_, convocatoria := e.compositores2028(admin)
	jurado, _ := e.asignado(admin, convocatoria, "jurado@ejemplo.mx", "jurado")
	postulante := e.postulanteVerificado("ana@ejemplo.mx")
	ficha := fmt.Sprintf("/administracion/convocatorias/%d", convocatoria)

	// El menú la ofrece a quien supervisa.
	esperar(t, admin.get("/postulaciones"), http.StatusOK, `href="/administracion"`)
	esperar(t, observador.get("/postulaciones"), http.StatusOK, `href="/administracion"`)
	noAparece(t, jurado.get("/evaluacion"), `href="/administracion"`)
	noAparece(t, postulante.get("/postulaciones"), `href="/administracion"`)

	// Ni el jurado ni el postulante entran; sin sesión, a ingresar.
	for nombre, c := range map[string]*cliente{"jurado": jurado, "postulante": postulante} {
		for _, ruta := range []string{"/administracion", ficha, "/administracion/usuarios", "/administracion/ediciones/1"} {
			if w := c.get(ruta); w.Code != http.StatusForbidden {
				t.Errorf("%s en %s: estado %d", nombre, ruta, w.Code)
			}
		}
		if w := c.post(ficha+"/datos", url.Values{"titulo_es": {"Otro"}}); w.Code != http.StatusForbidden {
			t.Errorf("%s al guardar: estado %d", nombre, w.Code)
		}
	}
	if w := e.cliente().get(ficha); w.Code != http.StatusSeeOther {
		t.Errorf("sin sesión: estado %d", w.Code)
	}

	// El observador ve todo, sin formularios que funcionen ni botones.
	w := observador.get(ficha)
	esperar(t, w, http.StatusOK, "Convocatoria de Compositores 2028", "Partitura", "Puntaje artístico", `<fieldset class="admin__campos" disabled>`,
		"jurado@ejemplo.mx")
	noAparece(t, w, ">Guardar</button>", "Crear y asignar", ">Retirar</button>", "Pasar a «")
	noAparece(t, observador.get("/administracion"), "Crear edición")
	noAparece(t, observador.get("/administracion/usuarios"), "Crear cuenta", "Reenviar enlace")
	for _, peticion := range []struct {
		ruta    string
		valores url.Values
	}{
		{"/administracion/ediciones", url.Values{"numero": {"51"}, "anio": {"2029"}, "nombre_es": {"X"}, "nombre_en": {"X"}}},
		{ficha + "/datos", url.Values{"titulo_es": {"Otro"}}},
		{ficha + "/estado", url.Values{"destino": {"publicada"}}},
		{ficha + "/cuentas", url.Values{"email": {"x@ejemplo.mx"}, "nombre": {"X"}, "apellidos": {"Y"}, "perfil": {"jurado"}}},
		{"/administracion/usuarios", url.Values{"email": {"x@ejemplo.mx"}, "nombre": {"X"}, "apellidos": {"Y"}}},
	} {
		if w := observador.post(peticion.ruta, peticion.valores); w.Code != http.StatusForbidden {
			t.Errorf("observador en %s: estado %d", peticion.ruta, w.Code)
		}
	}
	if cfg, _ := e.repo.ConfiguracionDeConvocatoria(context.Background(), convocatoria); cfg.Convocatoria.TituloES != "Convocatoria de Compositores 2028" {
		t.Errorf("el observador cambió el título: %q", cfg.Convocatoria.TituloES)
	}

	// Ids que no existen: 404.
	for _, ruta := range []string{"/administracion/convocatorias/999", "/administracion/ediciones/999", "/administracion/convocatorias/x"} {
		if w := admin.get(ruta); w.Code != http.StatusNotFound {
			t.Errorf("%s: estado %d", ruta, w.Code)
		}
	}
}

func TestConfigurarUnaConvocatoriaDesdeLasPantallas(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	ctx := context.Background()

	// Edición: alta, y el duplicado vuelve con lo escrito.
	esperar(t, admin.get("/administracion"), http.StatusOK, "Edición 49 · 2027", "Convocatoria de Intérpretes", "Crear edición")
	nueva := url.Values{"numero": {"50"}, "anio": {"2028"}, "nombre_es": {"Edición 50 · 2028"}, "nombre_en": {"50th edition · 2028"}}
	destino := destinoDe(t, admin.post("/administracion/ediciones", nueva))
	if !strings.HasSuffix(destino, "?aviso=edicion") {
		t.Fatalf("destino tras crear la edición: %s", destino)
	}
	edicion := strings.TrimSuffix(destino, "?aviso=edicion")
	esperar(t, admin.post("/administracion/ediciones", nueva), http.StatusUnprocessableEntity, "Ya existe un registro", `value="Edición 50 · 2028"`)
	esperar(t, admin.post(edicion, url.Values{"numero": {"50"}, "anio": {"1500"}, "nombre_es": {"Edición 50"}, "nombre_en": {"x"}}),
		http.StatusUnprocessableEntity, "El año no es válido", `value="1500"`)
	redirige(t, admin.post(edicion, url.Values{"numero": {"50"}, "anio": {"2028"}, "nombre_es": {"Edición L"}, "nombre_en": {"50th"}}), edicion+"?aviso=guardado")

	// Convocatoria en blanco: nace en borrador con requisitos y criterios.
	esperar(t, admin.get(edicion), http.StatusOK, "Edición L", "Crear una convocatoria", `<option value="interpretes">`)
	ficha := strings.TrimSuffix(destinoDe(t, admin.post(edicion+"/convocatorias",
		url.Values{"tipo": {"compositores"}, "titulo_es": {"Compositores 2028"}, "titulo_en": {"Composers 2028"}})), "?aviso=convocatoria")
	convocatoria := idDeRutaAPI(t, ficha)
	noAparece(t, admin.get(edicion), `<option value="compositores">`)
	w := admin.get(ficha + "?aviso=convocatoria")
	esperar(t, w, http.StatusOK, "Convocatoria creada en borrador", "Borrador", "Partitura", "Particellas", "Puntaje de realización",
		"Para pasar a esa etapa falta:", "Faltan las bases en español.", "Falta definir el calendario completo", `type="submit" disabled`)

	// Títulos y límites: el rechazo conserva lo escrito.
	w = admin.post(ficha+"/datos", url.Values{"titulo_es": {"Compositores · 2028"}, "titulo_en": {"Composers"}, "max_obras": {"0"}})
	esperar(t, w, http.StatusUnprocessableEntity, "debe indicarse el máximo de obras", `value="Compositores · 2028"`)
	redirige(t, admin.post(ficha+"/datos", url.Values{"titulo_es": {"Compositores · 2028"}, "titulo_en": {"Composers"}, "max_obras": {"2"}}),
		ficha+"?aviso=guardado#datos")

	// Bases: los saltos de línea del navegador se guardan como \n.
	redirige(t, admin.post(ficha+"/bases", url.Values{"bases_es": {"# Bases\r\n\r\nTexto."}, "bases_en": {""}}), ficha+"?aviso=guardado#bases")
	cfg, err := e.repo.ConfiguracionDeConvocatoria(ctx, convocatoria)
	if err != nil || cfg.Convocatoria.BasesMDES != "# Bases\n\nTexto." || cfg.Convocatoria.MaxObras != 2 {
		t.Fatalf("bases o límites mal guardados: %q %d (%v)", cfg.Convocatoria.BasesMDES, cfg.Convocatoria.MaxObras, err)
	}

	// Bases en PDF: se acepta un PDF y se rechaza lo que no lo es.
	redirige(t, admin.subirAlFormulario(ficha+"/archivos/bases_pdf_es", "bases.pdf", pdfDePrueba("bases")), ficha+"?aviso=guardado#archivos")
	esperar(t, admin.subirAlFormulario(ficha+"/archivos/bases_pdf_en", "bases.pdf", []byte("no soy un pdf")), http.StatusUnprocessableEntity)
	w = admin.get(ficha)
	esperar(t, w, http.StatusOK, "/convocatorias/50/compositores/bases.pdf?idioma=es")
	noAparece(t, w, "bases.pdf?idioma=en")
	redirige(t, admin.post(ficha+"/archivos/bases_pdf_es/quitar", nil), ficha+"?aviso=guardado#archivos")
	noAparece(t, admin.get(ficha), "bases.pdf?idioma=es")

	// Calendario en hora de la Ciudad de México; el rechazo conserva lo escrito.
	calendario := url.Values{
		"inicia_postulacion": {"2026-11-20T00:00"}, "termina_postulacion": {"2026-12-19T23:59"},
		"inicia_revision_documental": {"2026-12-21T00:00"}, "termina_revision_documental": {"2027-01-09T23:59"},
		"inicia_evaluacion": {"2027-01-05T00:00"}, "termina_evaluacion": {"2027-01-30T23:59"},
	}
	esperar(t, admin.post(ficha+"/calendario", calendario), http.StatusUnprocessableEntity, "Las etapas deben seguirse", `value="2027-01-05T00:00"`)
	calendario.Set("inicia_evaluacion", "2027-01-11T00:00")
	redirige(t, admin.post(ficha+"/calendario", calendario), ficha+"?aviso=guardado#calendario")
	cfg, _ = e.repo.ConfiguracionDeConvocatoria(ctx, convocatoria)
	if etapa, _ := cfg.Convocatoria.Etapa(models.EtapaPostulacion); !etapa.IniciaAt.Equal(time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("la hora de la Ciudad de México no se convirtió a UTC: %v", etapa.IniciaAt)
	}
	esperar(t, admin.get(ficha), http.StatusOK, `value="2026-11-20T00:00"`)

	// Dotaciones: alta, duplicado (vuelve abierto con lo escrito) y retiro.
	cuarteto := url.Values{"nombre_es": {"Cuarteto de cuerdas"}, "nombre_en": {"String quartet"}, "orden": {"1"}}
	redirige(t, admin.post(ficha+"/dotaciones", cuarteto), ficha+"?aviso=guardado#dotaciones")
	esperar(t, admin.post(ficha+"/dotaciones", cuarteto), http.StatusUnprocessableEntity, "Ya existe un registro",
		`admin__elemento--nuevo" open`, `value="String quartet"`)
	cfg, _ = e.repo.ConfiguracionDeConvocatoria(ctx, convocatoria)
	dotacion := cfg.Dotaciones[0].ID
	redirige(t, admin.post(fmt.Sprintf("%s/dotaciones/%d", ficha, dotacion), url.Values{"nombre_es": {"Cuarteto de cuerdas"}, "nombre_en": {"String quartet"}}),
		ficha+"?aviso=guardado#dotaciones")
	esperar(t, admin.get(ficha), http.StatusOK, "Retirada")
	redirige(t, admin.post(fmt.Sprintf("%s/dotaciones/%d", ficha, dotacion),
		url.Values{"nombre_es": {"Cuarteto de cuerdas"}, "nombre_en": {"String quartet"}, "activa": {"1"}}), ficha+"?aviso=guardado#dotaciones")

	// Requisitos: alta específica de una dotación, rechazo con lo escrito,
	// edición y borrado.
	requisito := url.Values{"clave": {"Grabacion"}, "etiqueta_es": {"Grabación de referencia"}, "etiqueta_en": {"Reference recording"},
		"ambito": {"obra"}, "dotacion": {fmt.Sprint(dotacion)}, "mimes": {"audio/mpeg", "audio/wav"}, "tamano_mb": {"40"}, "max_archivos": {"1"}, "orden": {"9"}}
	w = admin.post(ficha+"/requisitos", requisito)
	esperar(t, w, http.StatusUnprocessableEntity, "La clave debe empezar con una letra", `value="Grabacion"`, `value="audio/wav" checked`, `value="40"`)
	requisito.Set("clave", "grabacion")
	redirige(t, admin.post(ficha+"/requisitos", requisito), ficha+"?aviso=guardado#requisitos")
	cfg, _ = e.repo.ConfiguracionDeConvocatoria(ctx, convocatoria)
	i := slices.IndexFunc(cfg.Requisitos, func(r models.RequisitoDocumental) bool { return r.Clave == "grabacion" })
	if i < 0 || cfg.Requisitos[i].DotacionID != dotacion || cfg.Requisitos[i].TamanoMaxBytes != 40<<20 || len(cfg.Requisitos[i].MimesPermitidos) != 2 {
		t.Fatalf("requisito mal guardado: %+v", cfg.Requisitos)
	}
	grabacion := cfg.Requisitos[i].ID
	requisito.Set("obligatorio", "1")
	redirige(t, admin.post(fmt.Sprintf("%s/requisitos/%d", ficha, grabacion), requisito), ficha+"?aviso=guardado#requisitos")
	redirige(t, admin.post(fmt.Sprintf("%s/requisitos/%d", ficha, grabacion), url.Values{"accion": {"borrar"}}), ficha+"?aviso=borrado#requisitos")
	if w := admin.post(fmt.Sprintf("%s/requisitos/%d", ficha, grabacion), url.Values{"accion": {"borrar"}}); w.Code != http.StatusNotFound {
		t.Errorf("borrar un requisito que ya no existe: estado %d", w.Code)
	}

	// Criterios: el rechazo de la escala, junto a su formulario.
	criterio := cfg.Criterios[0]
	w = admin.post(fmt.Sprintf("%s/criterios/%d", ficha, criterio.ID), url.Values{"clave": {criterio.Clave}, "etiqueta_es": {criterio.EtiquetaES},
		"etiqueta_en": {criterio.EtiquetaEN}, "escala_min": {"5"}, "escala_max": {"3"}, "obligatorio": {"1"}})
	esperar(t, w, http.StatusUnprocessableEntity, "La escala debe ir", `value="5"`)
	redirige(t, admin.post(ficha+"/criterios", url.Values{"clave": {"originalidad"}, "etiqueta_es": {"Originalidad"}, "etiqueta_en": {"Originality"},
		"escala_min": {"1"}, "escala_max": {"10"}}), ficha+"?aviso=guardado#criterios")
	esperar(t, admin.get(ficha), http.StatusOK, "Originalidad", "Escala 1 a 10")

	// Lo listo: se publica desde la ficha; retroceder exige motivo.
	w = admin.get(ficha)
	esperar(t, w, http.StatusOK, "Cumple lo necesario para pasar a esa etapa.", "Pasar a «Publicada»")
	redirige(t, admin.post(ficha+"/estado", url.Values{"destino": {"publicada"}}), ficha+"?aviso=estado#estado")
	esperar(t, admin.post(ficha+"/estado", url.Values{"destino": {"borrador"}}), http.StatusUnprocessableEntity, "debe indicarse el motivo")
	esperar(t, admin.post(ficha+"/estado", url.Values{"destino": {"evaluacion"}}), http.StatusUnprocessableEntity, "inmediata anterior o siguiente")
	// Ya publicada, un requisito no se borra.
	esperar(t, admin.post(fmt.Sprintf("%s/requisitos/%d", ficha, cfg.Requisitos[0].ID), url.Values{"accion": {"borrar"}}),
		http.StatusUnprocessableEntity, "Sólo puede eliminarse mientras la convocatoria está en borrador")
	if asientos := e.asientos("convocatoria.estado"); len(asientos) == 0 {
		t.Error("el cambio de estado no quedó en la bitácora")
	}

	// Un elemento de otra convocatoria no se toca desde esta ficha.
	_, otra := e.compositores2028Otra(admin)
	ajena, _ := e.repo.ConfiguracionDeConvocatoria(ctx, otra)
	if w := admin.post(fmt.Sprintf("%s/criterios/%d", ficha, ajena.Criterios[0].ID), url.Values{"accion": {"borrar"}}); w.Code != http.StatusNotFound {
		t.Errorf("criterio de otra convocatoria: estado %d", w.Code)
	}

	// Copiar a otra edición: sin calendario, con el catálogo.
	edicion51 := strings.TrimSuffix(destinoDe(t, admin.post("/administracion/ediciones",
		url.Values{"numero": {"52"}, "anio": {"2030"}, "nombre_es": {"Edición 52"}, "nombre_en": {"52nd"}})), "?aviso=edicion")
	esperar(t, admin.get(edicion51), http.StatusOK, "Copiar de otra edición", fmt.Sprintf(`<option value="%d">`, convocatoria))
	copia := strings.TrimSuffix(destinoDe(t, admin.post(edicion51+"/duplicar", url.Values{"origen": {fmt.Sprint(convocatoria)}})), "?aviso=convocatoria")
	w = admin.get(copia)
	esperar(t, w, http.StatusOK, "Borrador", "Cuarteto de cuerdas", "Originalidad", "Falta definir el calendario completo")
	noAparece(t, w, `value="2026-11-20T00:00"`)
}

// compositores2028Otra crea por la API una segunda edición con su
// convocatoria de Compositores, para probar el aislamiento entre fichas.
func (e *entornoPrueba) compositores2028Otra(admin *cliente) (edicionID, convocatoriaID int64) {
	e.t.Helper()
	edicion := admin.exige(http.StatusCreated, "POST", "/api/v1/ediciones", obj{"numero": 51, "anio": 2029, "nombre_es": "Edición 51"})
	edicionID = id(edicion["id"])
	c := admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/ediciones/%d/convocatorias", edicionID),
		obj{"tipo": "compositores", "titulo_es": "Compositores 2029"})
	return edicionID, id(c["id"])
}

func TestJuradosRevisoresYCuentasDesdeLasPantallas(t *testing.T) {
	e := nuevoEntorno(t)
	admin := e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	_, convocatoria := e.compositores2028(admin)
	ficha := fmt.Sprintf("/administracion/convocatorias/%d", convocatoria)
	enviadosAntes := len(e.correos())

	// Crear una cuenta y asignarla: nace sin contraseña y recibe su enlace.
	nueva := url.Values{"email": {"Julia@Ejemplo.mx"}, "nombre": {"Julia"}, "apellidos": {"Ortega"}, "idioma": {"en"}, "perfil": {"jurado"}}
	redirige(t, admin.post(ficha+"/cuentas", nueva), ficha+"?aviso=asignacion#asignaciones")
	correos := e.correos()
	if len(correos) != enviadosAntes+1 || !strings.EqualFold(correos[len(correos)-1].Destinatario, "julia@ejemplo.mx") {
		t.Fatalf("no salió el enlace de activación: %+v", correos)
	}
	esperar(t, admin.get(ficha), http.StatusOK, "Julia Ortega", "Julia@Ejemplo.mx", "Sin activar", "Jurados: 1 de 3")
	// El mismo correo otra vez: el rechazo vuelve junto al alta, con lo escrito.
	esperar(t, admin.post(ficha+"/cuentas", nueva), http.StatusUnprocessableEntity, "Ya existe una cuenta con ese correo", `value="Ortega"`)

	// Buscar una cuenta existente y asignarla: la elige una persona.
	marta := e.crearInterno(admin, "marta@ejemplo.mx")
	w := admin.get(ficha + "?buscar=marta")
	esperar(t, w, http.StatusOK, "marta@ejemplo.mx", "Como jurado", "Como revisor")
	esperar(t, admin.get(ficha+"?buscar=nadie"), http.StatusOK, "No hay cuentas que coincidan.")
	redirige(t, admin.post(ficha+"/asignaciones", url.Values{"usuario": {fmt.Sprint(marta)}, "perfil": {"revisor_documental"}}),
		ficha+"?aviso=asignacion#asignaciones")
	esperar(t, admin.get(ficha), http.StatusOK, "Revisor documental", "Revisores: 1")

	// La cuota de tres jurados.
	for i := 2; i <= 3; i++ {
		redirige(t, admin.post(ficha+"/cuentas", url.Values{"email": {fmt.Sprintf("j%d@ejemplo.mx", i)}, "nombre": {"J"}, "apellidos": {"X"}, "perfil": {"jurado"}}),
			ficha+"?aviso=asignacion#asignaciones")
	}
	esperar(t, admin.post(ficha+"/cuentas", url.Values{"email": {"j4@ejemplo.mx"}, "nombre": {"J"}, "apellidos": {"X"}, "perfil": {"jurado"}}),
		http.StatusUnprocessableEntity, "La convocatoria ya tiene tres jurados.", `value="j4@ejemplo.mx"`)
	if _, _, err := e.repo.UsuarioPorEmail(context.Background(), "j4@ejemplo.mx"); err == nil {
		t.Error("la asignación rechazada dejó creada la cuenta")
	}

	// Retirar una asignación: quita el acceso, no la cuenta.
	cfg, _ := e.repo.ConfiguracionDeConvocatoria(context.Background(), convocatoria)
	var deMarta int64
	for _, a := range cfg.Asignaciones {
		if a.Usuario.ID == marta {
			deMarta = a.ID
		}
	}
	redirige(t, admin.post(fmt.Sprintf("%s/asignaciones/%d/retirar", ficha, deMarta), nil), ficha+"?aviso=retirada#asignaciones")
	noAparece(t, admin.get(ficha), "marta@ejemplo.mx")
	if w := admin.post(fmt.Sprintf("%s/asignaciones/%d/retirar", ficha, deMarta), nil); w.Code != http.StatusNotFound {
		t.Errorf("retirar dos veces: estado %d", w.Code)
	}
	esperar(t, admin.get("/administracion/usuarios?q=marta"), http.StatusOK, "marta@ejemplo.mx", "Cuentas que coinciden: 1")

	// Cuentas internas: alta con perfil global, rechazo y reenvío del enlace.
	redirige(t, admin.post("/administracion/usuarios", url.Values{"email": {"coord@ejemplo.mx"}, "nombre": {"Clara"}, "apellidos": {"Núñez"},
		"perfil_global": {"coordinacion_nacional"}}), "/administracion/usuarios?aviso=cuenta&q=coord%40ejemplo.mx")
	esperar(t, admin.get("/administracion/usuarios?aviso=cuenta&q=coord%40ejemplo.mx"), http.StatusOK, "Cuenta creada", "Clara Núñez",
		"Coordinación Nacional (observación)", "Sin activar", "Reenviar enlace")
	esperar(t, admin.post("/administracion/usuarios", url.Values{"email": {"no-es-correo"}, "nombre": {"Clara"}, "apellidos": {"Núñez"}}),
		http.StatusUnprocessableEntity, "Escribe un correo electrónico válido", `value="Núñez"`)
	esperar(t, admin.post("/administracion/usuarios", url.Values{"email": {"x@ejemplo.mx"}, "nombre": {"X"}, "apellidos": {"Y"}, "perfil_global": {"jurado"}}),
		http.StatusUnprocessableEntity, "El perfil global debe ser")
	coord, _, err := e.repo.UsuarioPorEmail(context.Background(), "coord@ejemplo.mx")
	if err != nil {
		t.Fatal(err)
	}
	antes := len(e.correos())
	redirige(t, admin.post(fmt.Sprintf("/administracion/usuarios/%d/activacion", coord.ID), nil), "/administracion/usuarios?aviso=activacion&q=coord%40ejemplo.mx")
	if len(e.correos()) != antes+1 {
		t.Error("no se reenvió el enlace")
	}
	adminCuenta, _, _ := e.repo.UsuarioPorEmail(context.Background(), "admin@ejemplo.mx")
	esperar(t, admin.post(fmt.Sprintf("/administracion/usuarios/%d/activacion", adminCuenta.ID), nil), http.StatusUnprocessableEntity, "La cuenta ya está activada.")
	if w := admin.post("/administracion/usuarios/999/activacion", nil); w.Code != http.StatusNotFound {
		t.Errorf("reenviar a una cuenta inexistente: estado %d", w.Code)
	}
}
