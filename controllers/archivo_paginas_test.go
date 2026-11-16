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
	"net/url"
	"strings"
	"testing"
)

// Pruebas de las pantallas del archivo (paso 13c): el buscador con el
// alcance de cada perfil, las vistas guardadas, la descarga masiva y el
// expediente pedidos desde la pantalla, la lista de descargas y el enlace
// permanente por folio para el personal.

func TestBuscadorDelArchivoEnPantalla(t *testing.T) {
	x := nuevoArchivoDePrueba(t)

	// El menú lo ofrece al personal; el postulante no entra.
	for nombre, c := range map[string]*cliente{"admin": x.admin, "observador": x.observador, "revisora": x.revisora, "jurado": x.juradoInt} {
		if !strings.Contains(c.get("/cuenta").Body.String(), `href="/archivo"`) {
			t.Errorf("%s no ve el archivo en el menú", nombre)
		}
	}
	noAparece(t, x.luis.get("/postulaciones"), `href="/archivo"`)
	for _, ruta := range []string{"/archivo", "/archivo/descargas"} {
		if w := x.luis.get(ruta); w.Code != http.StatusForbidden {
			t.Errorf("el postulante en %s: %d", ruta, w.Code)
		}
	}

	// La supervisión ve todo, también el borrador (sin folio, sin enlace permanente).
	w := x.admin.get("/archivo")
	esperar(t, w, http.StatusOK, "Postulaciones encontradas: 4", `href="/postulaciones/FIM50-INT-0001"`, "Bruno Díaz", "Eva Brouwer",
		`href="/api/v1/archivo?formato=csv"`, `action="/archivo/descargas"`, "elige la convocatoria en los filtros")
	esperar(t, x.admin.get("/archivo?q=nunez"), http.StatusOK, "Postulaciones encontradas: 1", "Luis Núñez Carrillo")
	esperar(t, x.admin.get("/archivo?q=arril"), http.StatusOK, "Postulaciones encontradas: 2")
	esperar(t, x.observador.get(fmt.Sprintf("/archivo?convocatoria=%d&estado=en_evaluacion", x.interpretes)), http.StatusOK,
		"Postulaciones encontradas: 1", `action="/archivo/expedientes"`, `<details class="archivo__avanzados">`)
	esperar(t, x.admin.get("/archivo?genero=mujer"), http.StatusOK, `<details class="archivo__avanzados" open>`)

	// La revisora, sus enviadas; el jurado, sólo lo que pasó a evaluación, y sin descarga masiva.
	w = x.revisora.get("/archivo")
	esperar(t, w, http.StatusOK, "Postulaciones encontradas: 2", "Luis Núñez Carrillo", `action="/archivo/descargas"`)
	noAparece(t, w, "Eva Brouwer", "Bruno Díaz")
	w = x.juradoInt.get("/archivo")
	esperar(t, w, http.StatusOK, "Postulaciones encontradas: 1", "Luis Núñez Carrillo")
	noAparece(t, w, `action="/archivo/descargas"`, "FIM50-INT-0002")

	// Filtros que no valen y páginas.
	esperar(t, x.admin.get("/archivo?orden=altura&enviada_desde=ayer"), http.StatusUnprocessableEntity, "Hay filtros que corregir",
		"Orden: El valor de este filtro no es válido.", "Enviada desde:")
	w = x.admin.get("/archivo?tamano=1&orden=-enviada")
	esperar(t, w, http.StatusOK, "página 1 de 4", `href="/archivo?orden=-enviada&amp;pagina=2&amp;tamano=1"`)
	esperar(t, x.admin.get("/archivo?tamano=1&orden=-enviada&pagina=4"), http.StatusOK, "página 4 de 4", `rel="prev"`)

	// El CSV del resultado es el de la API.
	csv := x.admin.get("/api/v1/archivo?q=arril&formato=csv")
	if csv.Code != http.StatusOK || !strings.Contains(csv.Body.String(), "FIM50-INT-0001") {
		t.Errorf("CSV del resultado: %d %s", csv.Code, csv.Body.String())
	}
}

func TestVistasYDescargasDesdeElArchivo(t *testing.T) {
	x := nuevoArchivoDePrueba(t)

	// Vistas guardadas: se guarda la búsqueda (sin la página) y siempre dentro de /archivo.
	redirige(t, x.admin.post("/archivo/vistas", url.Values{"nombre": {"Intérpretes en evaluación"}, "consulta": {"tipo=interpretes&estado=en_evaluacion&pagina=3"}}),
		"/archivo?estado=en_evaluacion&tipo=interpretes&aviso=vista")
	esperar(t, x.admin.get("/archivo?estado=en_evaluacion&tipo=interpretes"), http.StatusOK,
		`<a href="/archivo?estado=en_evaluacion&amp;tipo=interpretes" aria-current="true">Intérpretes en evaluación</a>`)
	esperar(t, x.admin.post("/archivo/vistas", url.Values{"nombre": {"Intérpretes en evaluación"}, "consulta": {"tipo=interpretes"}}),
		http.StatusUnprocessableEntity, "Ya tienes una vista con ese nombre.")
	redirige(t, x.admin.post("/archivo/vistas", url.Values{"nombre": {"Rara"}, "consulta": {"https://otro.sitio/x"}}), "/archivo?aviso=vista")
	vistas, err := x.e.repo.VistasGuardadas(context.Background(), idDeCuenta(t, x, "admin@ejemplo.mx"))
	if err != nil || len(vistas) != 2 {
		t.Fatalf("vistas: %v %v", vistas, err)
	}
	for _, v := range vistas {
		if !strings.HasPrefix(v.Ruta, "/archivo") {
			t.Errorf("una vista salió del archivo: %q", v.Ruta)
		}
	}
	noAparece(t, x.observador.get("/archivo"), "Intérpretes en evaluación")
	if w := x.observador.post(fmt.Sprintf("/archivo/vistas/%d/borrar", vistas[0].ID), nil); w.Code != http.StatusNotFound {
		t.Errorf("el observador borró una vista ajena: %d", w.Code)
	}
	redirige(t, x.admin.post(fmt.Sprintf("/archivo/vistas/%d/borrar", vistas[0].ID), nil), "/archivo?aviso=vista_borrada")

	// Descarga masiva: sólo partituras de Intérpretes; se arma y se baja desde «Mis descargas».
	consulta := fmt.Sprintf("convocatoria=%d", x.interpretes)
	redirige(t, x.admin.post("/archivo/descargas", url.Values{"consulta": {consulta}, "tipos": {"partitura"}}), "/archivo/descargas?aviso=encolada")
	w := x.admin.get("/archivo/descargas?aviso=encolada")
	esperar(t, w, http.StatusOK, "Pedido. Se está armando", "Documentos en ZIP", "En espera", `hx-trigger="every 5s"`)
	if armadas := x.trabajador.Procesar(context.Background()); armadas != 1 {
		t.Fatalf("el trabajador armó %d", armadas)
	}
	w = x.admin.get("/archivo/descargas")
	esperar(t, w, http.StatusOK, "Lista", "2 archivos", `href="/api/v1/exportaciones/`)
	noAparece(t, w, `hx-trigger="every 5s"`)
	tareas, err := x.e.repo.ExportacionesDe(context.Background(), idDeCuenta(t, x, "admin@ejemplo.mx"), 10)
	if err != nil || len(tareas) != 1 {
		t.Fatalf("tareas: %v %v", tareas, err)
	}
	for nombre := range zipDe(t, x.admin, tareas[0].ID) {
		if nombre != "manifiesto.csv" && !strings.Contains(nombre, "partitura") {
			t.Errorf("el ZIP lleva algo que no es partitura: %s", nombre)
		}
	}
	// Las descargas son de quien las pidió.
	noAparece(t, x.observador.get("/archivo/descargas"), "Documentos en ZIP")
	// El jurado no descarga en masa.
	if w := x.juradoInt.post("/archivo/descargas", url.Values{"consulta": {consulta}}); w.Code != http.StatusForbidden {
		t.Errorf("descarga masiva del jurado: %d", w.Code)
	}

	// Expediente de una convocatoria: exige la convocatoria; la revisora lo pide.
	esperar(t, x.revisora.post("/archivo/expedientes", url.Values{"consulta": {"estado=enviada"}}), http.StatusUnprocessableEntity, "Indica la convocatoria.")
	redirige(t, x.revisora.post("/archivo/expedientes", url.Values{"consulta": {consulta}, "fotografia": {"no"}}), "/archivo/descargas?aviso=encolada")
	x.trabajador.Procesar(context.Background())
	esperar(t, x.revisora.get("/archivo/descargas"), http.StatusOK, "Expediente en PDF", "Lista", "2 archivos")
	if w := x.luis.post("/archivo/expedientes", url.Values{"consulta": {consulta}}); w.Code != http.StatusForbidden {
		t.Errorf("expediente pedido por un postulante: %d", w.Code)
	}
}

// idDeCuenta busca el id de una cuenta por su correo.
func idDeCuenta(t *testing.T, x *archivoDePrueba, email string) int64 {
	t.Helper()
	u, _, err := x.e.repo.UsuarioPorEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestEnlacePermanenteParaElPersonal(t *testing.T) {
	x := nuevoArchivoDePrueba(t)
	luis := idDeRutaAPI(t, x.rutaLuis)
	ana := idDeRutaAPI(t, x.rutaAna)
	for nombre, caso := range map[string]struct {
		c       *cliente
		folio   string
		destino string
	}{
		"titular":          {x.luis, "FIM50-INT-0001", fmt.Sprintf("/postulaciones/%d/envio", luis)},
		"administrador":    {x.admin, "FIM50-INT-0001", fmt.Sprintf("/revision/postulaciones/%d", luis)},
		"observador":       {x.observador, "FIM50-INT-0002", fmt.Sprintf("/revision/postulaciones/%d", ana)},
		"revisora":         {x.revisora, "FIM50-INT-0002", fmt.Sprintf("/revision/postulaciones/%d", ana)},
		"jurado":           {x.juradoInt, "FIM50-INT-0001", fmt.Sprintf("/evaluacion/postulaciones/%d", luis)},
		"jurado, sin eval": {x.juradoInt, "FIM50-INT-0002", ""},
		"otro jurado":      {x.juradoCom, "FIM50-INT-0001", ""},
		"revisora ajena":   {x.revisora, "FIM50-COM-0001", ""},
		"otra postulante":  {x.ana, "FIM50-INT-0001", ""},
		"no existe":        {x.admin, "FIM50-INT-9999", ""},
	} {
		w := caso.c.get("/postulaciones/" + caso.folio)
		if caso.destino == "" {
			if w.Code != http.StatusNotFound {
				t.Errorf("%s: se esperaba 404, llegó %d %s", nombre, w.Code, w.Header().Get("Location"))
			}
			continue
		}
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != caso.destino {
			t.Errorf("%s: %d → %q, se esperaba %q", nombre, w.Code, w.Header().Get("Location"), caso.destino)
		}
	}
}
