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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de las páginas del postulante (paso 11a): el panel «Mis
// postulaciones» y el formulario por pasos, por sus dos caminos —con htmx,
// que recibe fragmentos, y sin JavaScript, que envía cada formulario con su
// botón—. Lo que queda guardado se comprueba en la base, no en el HTML.

// htmx envía un formulario como lo hace htmx: espera fragmentos.
func (c *cliente) htmx(ruta string, valores url.Values) *httptest.ResponseRecorder {
	c.t.Helper()
	r := formulario(ruta, valores)
	r.Header.Set("HX-Request", "true")
	return c.hacer(r)
}

// postulacionDe lee una postulación tal como quedó guardada.
func (e *entornoPrueba) postulacionDe(id int64) models.Postulacion {
	e.t.Helper()
	p, err := e.repo.PostulacionPorID(context.Background(), id)
	if err != nil {
		e.t.Fatalf("leer la postulación %d: %v", id, err)
	}
	return p
}

// noAparece comprueba que un texto no esté en una respuesta.
func noAparece(t *testing.T, w *httptest.ResponseRecorder, fragmentos ...string) {
	t.Helper()
	for _, fragmento := range fragmentos {
		if strings.Contains(w.Body.String(), fragmento) {
			t.Errorf("no debía aparecer %q en la respuesta", fragmento)
		}
	}
}

var destinoDePaso = regexp.MustCompile(`^/postulaciones/(\d+)/datos$`)

// iniciar abre una postulación desde el panel y devuelve su id y su ruta base.
func (c *cliente) iniciar(convocatoriaID int64, subcategoria string) (int64, string) {
	c.t.Helper()
	valores := url.Values{"convocatoria": {fmt.Sprint(convocatoriaID)}}
	if subcategoria != "" {
		valores.Set("subcategoria", subcategoria)
	}
	w := c.post("/postulaciones", valores)
	partes := destinoDePaso.FindStringSubmatch(w.Header().Get("Location"))
	if w.Code != http.StatusSeeOther || partes == nil {
		c.t.Fatalf("iniciar la postulación: estado %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	var id int64
	fmt.Sscan(partes[1], &id)
	return id, "/postulaciones/" + partes[1]
}

func TestPanelDelPostulante(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, compositores, _, _ := e.convocatoriasAbiertas()

	// Sin sesión, a ingresar; y un guardado automático con la sesión vencida
	// no sigue la redirección a ciegas: htmx recibe a dónde ir.
	anonimo := e.cliente()
	redirige(t, anonimo.get("/postulaciones"), "/cuenta/ingresar?aviso=ingresa&volver=%2Fpostulaciones")
	w := anonimo.htmx("/postulaciones/1/campos", url.Values{"seccion": {"datos"}})
	if w.Code != http.StatusUnauthorized || w.Header().Get("HX-Redirect") != "/cuenta/ingresar?aviso=ingresa" {
		t.Errorf("guardado sin sesión: estado %d, HX-Redirect %q", w.Code, w.Header().Get("HX-Redirect"))
	}

	// El panel ofrece las dos convocatorias abiertas, y el menú lleva a él.
	ana := e.postulante("ana@ejemplo.mx")
	w = ana.get("/postulaciones")
	esperar(t, w, http.StatusOK, "Mis postulaciones", "Aún no tienes postulaciones", "Convocatoria de Intérpretes 2028",
		"Iniciar postulación", `name="subcategoria" value="ensamble"`, `href="/postulaciones" aria-current="page"`)
	ofrecidas := strings.Count(w.Body.String(), "Iniciar postulación")

	// En Intérpretes hay que decir cómo se postula; lo dice el modelo.
	w = ana.post("/postulaciones", url.Values{"convocatoria": {fmt.Sprint(interpretes)}})
	esperar(t, w, http.StatusUnprocessableEntity, "Elige una subcategoría")
	id, base := ana.iniciar(interpretes, "ensamble")
	if p := e.postulacionDe(id); p.Subcategoria != models.SubcategoriaEnsamble || p.Estado != models.PostulacionBorrador {
		t.Fatalf("postulación recién iniciada: %+v", p)
	}
	// Una segunda en la misma convocatoria no se crea: se vuelve al panel.
	redirige(t, ana.post("/postulaciones", url.Values{"convocatoria": {fmt.Sprint(interpretes)}, "subcategoria": {"solista"}}), "/postulaciones")

	// Ya iniciada, aparece en el historial y deja de ofrecerse.
	w = ana.get("/postulaciones")
	esperar(t, w, http.StatusOK, `href="`+base+`"`, "Borrador", "Ensamble", "Continuar", "Convocatoria de Compositores 2028")
	if quedan := strings.Count(w.Body.String(), "Iniciar postulación"); quedan != ofrecidas-1 {
		t.Errorf("la convocatoria en la que ya postula no debe ofrecerse otra vez: se ofrecían %d y se ofrecen %d", ofrecidas, quedan)
	}

	// El enlace por id lleva al primer paso. Lo ajeno y lo inexistente, 404.
	redirige(t, ana.get(base), base+"/datos")
	luis := e.postulante("luis@ejemplo.mx")
	for _, ruta := range []string{base, base + "/datos", "/postulaciones/999/datos", "/postulaciones/FIM50-INT-0099", base + "/inventado"} {
		quien := luis
		if strings.HasSuffix(ruta, "inventado") {
			quien = ana // suya, pero ese paso no existe
		}
		if w := quien.get(ruta); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: estado %d, se esperaba 404", ruta, w.Code)
		}
	}
	for _, ruta := range []string{"/campos", "/integrantes", "/videos", "/obras", "/subcategoria"} {
		if w := luis.htmx(base+ruta, url.Values{"seccion": {"datos"}, "datos.telefono": {"55"}}); w.Code != http.StatusNotFound {
			t.Errorf("POST %s de una postulación ajena: estado %d, se esperaba 404", ruta, w.Code)
		}
	}

	// El observador no postula; en inglés, el panel habla inglés.
	observa := e.activarCuentaInterna("observa@ejemplo.mx", models.PerfilCoordinacionNacional)
	esperar(t, observa.get("/postulaciones"), http.StatusOK, "no participa como postulante")
	if w := observa.post("/postulaciones", url.Values{"convocatoria": {fmt.Sprint(compositores)}}); w.Code != http.StatusForbidden {
		t.Errorf("el observador no debe poder postular: estado %d", w.Code)
	}
	luis.post("/idioma", url.Values{"idioma": {"en"}, "volver": {"/postulaciones"}})
	esperar(t, luis.get("/postulaciones"), http.StatusOK, "My applications", "Start application", "Soloist")
}

func TestFormularioPorPasos(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulante("ana@ejemplo.mx")
	id, base := ana.iniciar(interpretes, "ensamble")

	// --- La página del paso se pinta de la definición del formulario.
	w := ana.get(base + "/datos")
	esperar(t, w, http.StatusOK, "Mi postulación", "Paso 1 de 5", `aria-current="step"`, "Nombre del ensamble",
		`id="datos-nombre_ensamble" name="datos.nombre_ensamble"`, `<textarea id="datos-semblanza"`, "Máximo 1500 caracteres.",
		`id="integrantes"`, `id="videos"`, `id="v-3"`, "Lo que falta en este paso", `href="#datos-telefono"`,
		`hx-post="`+base+`/campos"`, "Guardar y continuar", "Cambiar a «Solista»", "Cierra el")
	// El solista tiene otro formulario: ni integrantes ni año de fundación.
	noAparece(t, w, "Fecha de nacimiento", `id="datos-genero"`)
	// Sin país, la entidad de residencia se escribe; no es una lista.
	esperar(t, w, http.StatusOK, `<input id="datos-entidad_residencia"`)

	// --- Sin JavaScript: el botón guarda el paso y lleva al siguiente.
	w = ana.post(base+"/campos", url.Values{"seccion": {"datos"}, "ir": {"siguiente"},
		"datos.nombre_ensamble": {"  Ensamble Tálamo  "}, "datos.telefono": {"+52 55 1234 5678"},
		"datos.semblanza": {"Primera línea.\r\nSegunda línea."}, "datos.anio_fundacion": {"2015"}})
	redirige(t, w, base+"/concierto")
	p := e.postulacionDe(id)
	if p.Campos["datos.nombre_ensamble"] != "Ensamble Tálamo" || p.Campos["datos.anio_fundacion"] != "2015" ||
		p.Campos["datos.semblanza"] != "Primera línea.\nSegunda línea." {
		t.Fatalf("campos tras guardar con el botón: %q", p.Campos)
	}
	// Un valor inaceptable no se guarda: la página vuelve con el motivo y con
	// lo escrito; lo demás sí se guarda.
	w = ana.post(base+"/campos", url.Values{"seccion": {"datos"}, "ir": {"siguiente"},
		"datos.email_contacto": {"no-es-correo"}, "datos.representante": {"Ana López"}})
	esperar(t, w, http.StatusUnprocessableEntity, "Hay datos que no se guardaron", `value="no-es-correo"`, "Escribe un correo electrónico válido")
	if p = e.postulacionDe(id); p.Campos["datos.email_contacto"] != "" || p.Campos["datos.representante"] != "Ana López" {
		t.Errorf("tras un valor inválido: %q", p.Campos)
	}

	// --- Con htmx: fragmentos fuera de banda, sin la página.
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "datos.email_contacto": {"talamo@ejemplo.mx"}, "datos.representante": {"Ana López"}})
	esperar(t, w, http.StatusOK, `id="guardado"`, `hx-swap-oob="true"`, "Guardado a las", "data-hora-local", `id="avance"`, `id="faltantes"`,
		`<p class="campo__error" id="e-datos-email_contacto" hx-swap-oob="true"></p>`)
	noAparece(t, w, "<html", "<header")
	// Lo que no cambió no se vuelve a escribir: el aviso no dice «guardado a las».
	antes := e.postulacionDe(id).ActualizadoAt
	e.hasta("2026-11-25T19:00:00Z")
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "datos.email_contacto": {"talamo@ejemplo.mx"}})
	esperar(t, w, http.StatusOK, "Último guardado")
	noAparece(t, w, "Guardado a las")
	if !e.postulacionDe(id).ActualizadoAt.Equal(antes) {
		t.Error("un guardado sin cambios no debe tocar la postulación")
	}
	// Un rechazo se dice en el aviso y en el campo.
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "datos.anio_fundacion": {"1492"}})
	esperar(t, w, http.StatusOK, `guardado--error`, "Hay datos que no se guardaron", `id="e-datos-anio_fundacion" hx-swap-oob="true">El número no es válido`)
	// Al elegir México, la entidad de residencia se vuelve una lista.
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "datos.pais_residencia": {"MX"}})
	esperar(t, w, http.StatusOK, `id="c-datos-entidad_residencia" hx-swap-oob="true"`, `<select id="datos-entidad_residencia"`, `<option value="JAL">Jalisco</option>`)
	// Un campo que no es de este paso no se guarda por aquí.
	ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "concierto.entidad": {"JAL"}, "obra.titulo": {"X"}})
	if p = e.postulacionDe(id); p.Campos["concierto.entidad"] != "" {
		t.Error("el paso de datos no debe guardar campos de otro paso")
	}

	// --- Integrantes: se guardan los completos; el que va a medias se avisa.
	w = ana.htmx(base+"/integrantes", url.Values{"nombre": {"Ana López", "Luis Paz", ""}, "instrumento": {"flauta", "", ""}})
	esperar(t, w, http.StatusOK, `id="nota-integrantes" hx-swap-oob="true">Hay integrantes a medias`, "Guardado a las")
	noAparece(t, w, `<form id="integrantes"`) // mientras se escribe, los renglones no se tocan
	if p = e.postulacionDe(id); len(p.Integrantes) != 1 || p.Integrantes[0].Instrumento != "flauta" {
		t.Fatalf("integrantes tras el guardado parcial: %+v", p.Integrantes)
	}
	// «Añadir» devuelve la lista con lo escrito y un renglón más, con el foco.
	w = ana.htmx(base+"/integrantes", url.Values{"nombre": {"Ana López", "Luis Paz"}, "instrumento": {"flauta", "piano"}, "accion": {"agregar"}})
	esperar(t, w, http.StatusOK, `<form id="integrantes"`, `id="i-nombre-2" name="nombre" type="text" value="" maxlength="200" autocomplete="off" autofocus`, `value="Luis Paz"`)
	// «Quitar» guarda la lista sin ese renglón.
	w = ana.htmx(base+"/integrantes", url.Values{"nombre": {"Ana López", "Luis Paz"}, "instrumento": {"flauta", "piano"}, "accion": {"quitar:0"}})
	esperar(t, w, http.StatusOK, `<form id="integrantes"`, `id="i-nombre-0" name="nombre" type="text" value="Luis Paz"`)
	if p = e.postulacionDe(id); len(p.Integrantes) != 1 || p.Integrantes[0].Nombre != "Luis Paz" {
		t.Errorf("integrantes tras quitar: %+v", p.Integrantes)
	}
	// Sin JavaScript, los mismos botones: «Añadir» vuelve con un renglón más.
	redirige(t, ana.post(base+"/integrantes", url.Values{"nombre": {"Luis Paz"}, "instrumento": {"piano"}, "accion": {"agregar"}}), base+"/datos?integrante=nuevo#integrantes")
	esperar(t, ana.get(base+"/datos?integrante=nuevo"), http.StatusOK, `id="i-nombre-1"`)

	// --- Ligas de video: las vacías no cuentan; una mal escrita se rechaza y se dice.
	w = ana.htmx(base+"/videos", url.Values{"liga": {"https://vimeo.com/123456", "", ""}})
	esperar(t, w, http.StatusOK, `id="ve-1" hx-swap-oob="true">Por comprobar`, "Guardado a las")
	w = ana.htmx(base+"/videos", url.Values{"liga": {"https://vimeo.com/123456", "vimeo.com/sin-esquema", ""}})
	esperar(t, w, http.StatusOK, `id="nota-videos" hx-swap-oob="true">La liga debe ser una dirección completa`)
	if p = e.postulacionDe(id); len(p.Videos) != 1 || p.Videos[0].URL != "https://vimeo.com/123456" {
		t.Errorf("ligas tras un rechazo: %+v", p.Videos)
	}

	// --- Propuesta de concierto: fuera de la Ciudad de México, la sede es obligatoria.
	w = ana.get(base + "/concierto")
	esperar(t, w, http.StatusOK, "Paso 2 de 5", "Sede del concierto", "Si propones el concierto fuera de la Ciudad de México", "(si aplica)")
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"concierto"}, "concierto.entidad": {"JAL"}, "concierto.sede_nombre": {"Teatro Degollado"}})
	esperar(t, w, http.StatusOK, `id="g-sede" hx-swap-oob="true"`, "Propones el concierto fuera de la Ciudad de México", `value="Teatro Degollado"`)
	noAparece(t, w, "(si aplica)")
	// Fecha y hora con la convención mexicana: se escriben dd/mm/aaaa y hh:mm
	// de 24 horas (también d/m/aaaa y h:mm), se guardan en ISO y vuelven
	// dd/mm/aaaa. No son controles del navegador, que usarían SU formato.
	ana.htmx(base+"/campos", url.Values{"seccion": {"concierto"}, "concierto.fecha_concierto": {"5/3/2027"}, "concierto.hora_concierto": {"9:30"}})
	w = ana.get(base + "/concierto")
	esperar(t, w, http.StatusOK, `value="05/03/2027"`, `value="09:30"`, `placeholder="dd/mm/aaaa"`, "Formato: dd/mm/aaaa")
	noAparece(t, w, `type="date"`, `type="time"`)
	if p, _ := e.repo.PostulacionPorID(context.Background(), id); p.Campos["concierto.fecha_concierto"] != "2027-03-05" || p.Campos["concierto.hora_concierto"] != "09:30" {
		t.Errorf("fecha y hora deben guardarse en ISO: %q %q", p.Campos["concierto.fecha_concierto"], p.Campos["concierto.hora_concierto"])
	}
	// Una fecha que no existe vuelve con lo escrito y su error; lo guardado no cambia.
	w = ana.post(base+"/campos", url.Values{"seccion": {"concierto"}, "concierto.fecha_concierto": {"31/02/2027"}})
	esperar(t, w, http.StatusUnprocessableEntity, `value="31/02/2027"`, "escríbala como dd/mm/aaaa")
	if p, _ := e.repo.PostulacionPorID(context.Background(), id); p.Campos["concierto.fecha_concierto"] != "2027-03-05" {
		t.Errorf("una fecha inválida no debe guardarse: %q", p.Campos["concierto.fecha_concierto"])
	}
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"concierto"}, "concierto.entidad": {"CDMX"}})
	esperar(t, w, http.StatusOK, "no hace falta que indiques la sede")

	// --- Cambio de subcategoría: sólo confirmando, y borra los datos generales.
	esperar(t, ana.post(base+"/subcategoria", url.Values{"subcategoria": {"solista"}}), http.StatusUnprocessableEntity, "confirma el cambio")
	redirige(t, ana.post(base+"/subcategoria", url.Values{"subcategoria": {"solista"}, "confirmar": {"1"}}), base+"/datos?aviso=subcategoria")
	w = ana.get(base + "/datos?aviso=subcategoria")
	esperar(t, w, http.StatusOK, "Cambiaste de subcategoría", "Fecha de nacimiento", `<select id="datos-genero"`, `<option value="MX">México</option>`)
	noAparece(t, w, `id="integrantes"`, "Ensamble Tálamo")
	if p = e.postulacionDe(id); p.Subcategoria != models.SubcategoriaSolista || p.Campos["concierto.entidad"] != "CDMX" {
		t.Errorf("tras cambiar de subcategoría: %s, %q", p.Subcategoria, p.Campos)
	}

	// --- En inglés, el formulario habla inglés; los nombres de país, también.
	ana.post("/idioma", url.Values{"idioma": {"en"}, "volver": {base + "/datos"}})
	esperar(t, ana.get(base+"/datos"), http.StatusOK, "My application", "Step 1 of 5", "General information", "Save and continue",
		`<option value="MX">Mexico</option>`, `<option value="DE">Germany</option>`)
	ana.post("/idioma", url.Values{"idioma": {"es"}, "volver": {base + "/datos"}})

	// --- Un campo extendido que llena quien postula aparece solo en el formulario.
	e.campoDePrueba(models.CampoExtendido{Tabla: "datos_solista", Columna: "como_se_entero", Tipo: models.ExtendidoTexto,
		EtiquetaES: "¿Cómo te enteraste?", EtiquetaEN: "How did you hear about us?", VisibleEn: []models.Perfil{models.PerfilPostulante}})
	e.campoDePrueba(models.CampoExtendido{Tabla: "postulacion", Columna: "camerino", Tipo: models.ExtendidoTexto, EtiquetaES: "Camerino asignado"})
	w = ana.get(base + "/datos")
	esperar(t, w, http.StatusOK, "¿Cómo te enteraste?", `name="ext.datos_solista.x_como_se_entero"`)
	noAparece(t, w, "Camerino asignado") // lo captura la administración, no quien postula
	ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "ext.datos_solista.x_como_se_entero": {"Por la radio"}})
	if p = e.postulacionDe(id); p.Campos["ext.datos_solista.x_como_se_entero"] != "Por la radio" {
		t.Errorf("campo extendido capturado desde el formulario: %q", p.Campos)
	}

	// --- Fuera de plazo, el borrador se lee pero ya no se modifica.
	e.hasta("2026-12-21T12:00:00Z")
	w = ana.get(base + "/datos")
	esperar(t, w, http.StatusOK, "este borrador ya no puede modificarse", "Nombre completo")
	noAparece(t, w, `name="datos.nombre_completo"`, "hx-post", "Guardar y continuar", "Lo que falta")
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "datos.nombre_completo": {"Ana"}})
	esperar(t, w, http.StatusOK, "guardado--error", "El plazo de postulación no está vigente")
	if p = e.postulacionDe(id); p.Campos["datos.nombre_completo"] != "" {
		t.Error("fuera de plazo no debe guardarse nada")
	}
}

func TestObrasDelFormulario(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, compositores, cuarteto, _ := e.convocatoriasAbiertas()
	ana := e.postulante("ana@ejemplo.mx")
	id, base := ana.iniciar(compositores, "")

	// En Compositores no hay propuesta de concierto: las obras son el paso 2 de 4.
	w := ana.get(base + "/obras")
	esperar(t, w, http.StatusOK, "Paso 2 de 4", "Aún no has añadido ninguna obra", "Añadir obra", "Obras: 0 de 2 como máximo")
	if w := ana.get(base + "/concierto"); w.Code != http.StatusNotFound {
		t.Errorf("Compositores no tiene paso de concierto: estado %d", w.Code)
	}

	// Añadir: con htmx vuelve el programa entero, con el foco en la obra nueva.
	w = ana.htmx(base+"/obras", nil)
	esperar(t, w, http.StatusOK, `<section id="obras"`, "Obra 1", `tabindex="-1" autofocus`, "Dotación instrumental",
		`<option value="`+fmt.Sprint(cuarteto)+`">Cuarteto de cuerdas</option>`, `id="obras-resumen" class="nota" hx-swap-oob="true"`)
	p := e.postulacionDe(id)
	if len(p.Obras) != 1 {
		t.Fatalf("obras tras añadir: %d", len(p.Obras))
	}
	primera := p.Obras[0].ID
	rutaPrimera := fmt.Sprintf("%s/obras/%d", base, primera)
	idCampo := fmt.Sprintf("o%d-obra-titulo", primera)
	esperar(t, w, http.StatusOK, `id="`+idCampo+`" name="obra.titulo"`)

	// Guardar los campos de una obra; uno inaceptable se dice en su lugar.
	w = ana.htmx(rutaPrimera, url.Values{"obra.titulo": {"Cuarteto n.º 2"}, "obra.duracion_min": {"12"},
		"obra.dotacion_id": {fmt.Sprint(cuarteto)}, "obra.liga_media": {"sin-esquema"}})
	esperar(t, w, http.StatusOK, "Hay datos que no se guardaron", fmt.Sprintf(`id="e-o%d-obra-liga_media" hx-swap-oob="true">La liga debe ser`, primera),
		"Duración total: 12 min")
	if p = e.postulacionDe(id); p.Obras[0].Campos["obra.titulo"] != "Cuarteto n.º 2" || p.Obras[0].Campos["obra.liga_media"] != "" {
		t.Errorf("obra tras guardar: %q", p.Obras[0].Campos)
	}
	// Sin JavaScript: el botón de la obra guarda y vuelve a ella.
	redirige(t, ana.post(rutaPrimera, url.Values{"obra.estreno": {"estreno_mundial"}}), fmt.Sprintf("%s/obras#obra-%d", base, primera))

	// Segunda obra (sin JavaScript), y el tope de la convocatoria.
	w = ana.post(base+"/obras", nil)
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), base+"/obras#obra-") {
		t.Fatalf("añadir sin JavaScript: estado %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	p = e.postulacionDe(id)
	segunda := p.Obras[1].ID
	rutaSegunda := fmt.Sprintf("%s/obras/%d", base, segunda)
	w = ana.get(base + "/obras")
	esperar(t, w, http.StatusOK, "Obras: 2 de 2 como máximo", "Ya tienes el máximo de obras", `type="submit" disabled>Añadir obra`)
	w = ana.htmx(base+"/obras", nil)
	esperar(t, w, http.StatusOK, "guardado--error", "Ya tienes el máximo de obras")
	if len(e.postulacionDe(id).Obras) != 2 {
		t.Error("no debe poder añadirse una tercera obra")
	}

	// Mover: la segunda sube, y recibe el foco.
	w = ana.htmx(rutaSegunda, url.Values{"accion": {"subir"}})
	esperar(t, w, http.StatusOK, `<section id="obras"`, fmt.Sprintf(`<h3 id="t-obra-%d" tabindex="-1" autofocus>Obra 1`, segunda))
	if p = e.postulacionDe(id); p.Obras[0].ID != segunda || p.Obras[1].ID != primera {
		t.Errorf("orden tras subir: %d, %d", p.Obras[0].ID, p.Obras[1].ID)
	}
	// Subir la que ya es la primera no es una acción válida.
	if w := ana.htmx(rutaSegunda, url.Values{"accion": {"subir"}}); w.Code != http.StatusNotFound {
		t.Errorf("subir la primera: estado %d", w.Code)
	}
	redirige(t, ana.post(rutaSegunda, url.Values{"accion": {"bajar"}}), fmt.Sprintf("%s/obras#obra-%d", base, segunda))

	// Quitar; y una obra de otra postulación no se alcanza por esta ruta.
	w = ana.htmx(rutaSegunda, url.Values{"accion": {"quitar"}})
	esperar(t, w, http.StatusOK, "Obras: 1 de 2 como máximo")
	if p = e.postulacionDe(id); len(p.Obras) != 1 || p.Obras[0].ID != primera {
		t.Errorf("obras tras quitar: %+v", p.Obras)
	}
	luis := e.postulante("luis@ejemplo.mx")
	_, baseDeLuis := luis.iniciar(interpretes, "solista")
	if w := luis.htmx(fmt.Sprintf("%s/obras/%d", baseDeLuis, primera), url.Values{"obra.titulo": {"Ajena"}}); w.Code != http.StatusNotFound {
		t.Errorf("guardar una obra ajena: estado %d", w.Code)
	}
	if e.postulacionDe(id).Obras[0].Campos["obra.titulo"] != "Cuarteto n.º 2" {
		t.Error("la obra ajena se modificó")
	}

	// En Intérpretes las obras no tienen dotación, y sí compositor.
	luis.htmx(baseDeLuis+"/obras", nil)
	w = luis.get(baseDeLuis + "/obras")
	esperar(t, w, http.StatusOK, "Paso 3 de 5", "Compositor(a)", "Obras en el programa: 1")
	noAparece(t, w, "Dotación instrumental", "como máximo")
}
