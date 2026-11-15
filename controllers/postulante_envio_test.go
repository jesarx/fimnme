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
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de las páginas del paso 11b: documentos, revisión y envío, estado
// de la postulación enviada y subsanación. Lo que queda guardado se comprueba
// en la base.

// subirDesdeLaPagina sube un archivo con el formulario de la página de
// documentos, con htmx o sin él.
func (c *cliente) subirDesdeLaPagina(base string, campos [][2]string, nombre string, contenido []byte, conHTMX bool) *httptest.ResponseRecorder {
	c.t.Helper()
	var cuerpo bytes.Buffer
	formulario := multipart.NewWriter(&cuerpo)
	for _, campo := range campos {
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
	r := httptest.NewRequest(http.MethodPost, base+"/documentos", &cuerpo)
	r.Header.Set("Content-Type", formulario.FormDataContentType())
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if conHTMX {
		r.Header.Set("HX-Request", "true")
	}
	return c.hacer(r)
}

// solistaSinDocumentos inicia desde el panel una postulación de solista y
// captura por la API todo menos los documentos. Devuelve su id, su ruta de
// página y la id de su obra.
func (e *entornoPrueba) solistaSinDocumentos(c *cliente, convocatoriaID int64) (id int64, base string, obraID int64) {
	e.t.Helper()
	id, base = c.iniciar(convocatoriaID, "solista")
	api := fmt.Sprintf("/api/v1/postulaciones/%d", id)
	guardar(e.t, c, api, datosDeSolista())
	c.exige(http.StatusOK, "PUT", api+"/videos", obj{"urls": []string{"https://vimeo.com/123"}})
	p := c.exige(http.StatusCreated, "POST", api+"/obras", nil)
	obraID = id64(lista(p, "obras")[0]["id"])
	guardar(e.t, c, fmt.Sprintf("/api/v1/obras/%d", obraID), obj{
		"obra.titulo": "Preludio a Colón", "obra.compositor": "Julián Carrillo", "obra.duracion_min": 12, "obra.estreno": "no_es_estreno",
		"obra.semblanza_compositor": "Semblanza.", "obra.nota_obra": "Nota.", "obra.liga_media": "https://ejemplo.mx/obra",
	})
	return id, base, obraID
}

// id64 es id(), con un nombre que no choca con las variables «id» de las pruebas.
func id64(v any) int64 { return id(v) }

func TestDocumentosDesdeLaPagina(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulanteVerificado("ana@ejemplo.mx")
	postulacionID, base, obraID := e.solistaSinDocumentos(ana, interpretes)
	deObra := [][2]string{{"clave", "partitura"}, {"obra_id", fmt.Sprint(obraID)}}

	// La página pide lo que dice el catálogo: la fotografía, y por la obra
	// la partitura y la autorización, con su formato y su tamaño.
	w := ana.get(base + "/documentos")
	esperar(t, w, http.StatusOK, "Paso 4 de 5", "De la postulación", "Obra 1 · Preludio a Colón", `id="doc-fotografia"`,
		fmt.Sprintf(`id="doc-partitura-o%d"`, obraID), "Formato: JPG o PNG. Tamaño máximo: 5 MB.", `accept=".jpg,image/jpeg,.png,image/png"`,
		`enctype="multipart/form-data"`, `hx-encoding="multipart/form-data"`, `class="subida__progreso"`, "Sin entregar",
		`href="#doc-fotografia"`) // lo que falta enlaza a la ranura

	// Con htmx: vuelve la sección entera, con el documento y el foco en su ranura.
	w = ana.subirDesdeLaPagina(base, [][2]string{{"clave", "fotografia"}}, "Retrato.JPG", contenidoJPEG, true)
	esperar(t, w, http.StatusOK, `<section id="documentos"`, fmt.Sprintf("borrador-%d_fotografia_v1.jpg", postulacionID),
		`<h4 class="requisito__titulo" id="t-doc-fotografia" tabindex="-1" autofocus>`, "Guardado a las", `id="avance"`,
		`name="reemplaza"`, "Reemplazar por otro archivo")
	if p := e.postulacionDe(postulacionID); len(p.Documentos) != 1 || p.Documentos[0].Clave != "fotografia" {
		t.Fatalf("documentos tras subir con htmx: %+v", p.Documentos)
	}
	// Un formato equivocado se dice en su ranura, y no deja nada.
	w = ana.subirDesdeLaPagina(base, deObra, "partitura.jpg", contenidoJPEG, true)
	esperar(t, w, http.StatusOK, fmt.Sprintf(`id="e-doc-partitura-o%d">`, obraID), "no es de los admitidos")
	// Sin JavaScript: la partitura (de 3 MB: más que el límite de un formulario) y de vuelta a su ranura.
	partitura := append(pdfDePrueba("partitura"), bytes.Repeat([]byte("0"), 3<<20)...)
	redirige(t, ana.subirDesdeLaPagina(base, deObra, "partitura.pdf", partitura, false), fmt.Sprintf("%s/documentos#doc-partitura-o%d", base, obraID))
	w = ana.subirDesdeLaPagina(base, [][2]string{{"clave", "autorizacion"}, {"obra_id", fmt.Sprint(obraID)}}, "a.txt", []byte("hola"), false)
	esperar(t, w, http.StatusUnprocessableEntity, "No se reconoce el formato", `<!DOCTYPE html>`)
	// Una petición cortada o sin archivo no rompe la página.
	r := httptest.NewRequest(http.MethodPost, base+"/documentos", strings.NewReader("no es multipart"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	esperar(t, ana.hacer(r), http.StatusUnprocessableEntity, "El archivo no llegó completo")

	// Quitar: el documento deja de estar vigente; uno ajeno o inexistente, 404.
	p := e.postulacionDe(postulacionID)
	var fotografia int64
	for _, d := range p.Documentos {
		if d.Clave == "fotografia" {
			fotografia = d.ID
		}
	}
	luis := e.postulante("luis@ejemplo.mx")
	for _, c := range []*cliente{luis, ana} {
		ruta := fmt.Sprintf("%s/documentos/%d/quitar", base, fotografia)
		if c == ana {
			ruta = fmt.Sprintf("%s/documentos/99999/quitar", base)
		}
		if w := c.htmx(ruta, nil); w.Code != http.StatusNotFound {
			t.Errorf("quitar %s: estado %d", ruta, w.Code)
		}
	}
	w = ana.htmx(fmt.Sprintf("%s/documentos/%d/quitar", base, fotografia), nil)
	esperar(t, w, http.StatusOK, `<section id="documentos"`)
	if len(e.postulacionDe(postulacionID).Documentos) != 1 {
		t.Error("la fotografía debió quitarse")
	}
	// Y otra persona no sube nada a esta postulación.
	if w := luis.subirDesdeLaPagina(base, [][2]string{{"clave", "fotografia"}}, "r.jpg", contenidoJPEG, true); w.Code != http.StatusNotFound {
		t.Errorf("subir a una postulación ajena: estado %d", w.Code)
	}
}

func TestRevisionYEnvioDesdeLaPagina(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulanteVerificado("ana@ejemplo.mx")
	postulacionID, base, obraID := e.solistaSinDocumentos(ana, interpretes)
	ana.subirDesdeLaPagina(base, [][2]string{{"clave", "fotografia"}}, "r.jpg", contenidoJPEG, false)
	ana.subirDesdeLaPagina(base, [][2]string{{"clave", "partitura"}, {"obra_id", fmt.Sprint(obraID)}}, "p.pdf", pdfDePrueba("p"), false)

	// Falta la autorización: la página lo dice, enlaza a su ranura y no deja enviar.
	w := ana.get(base + "/envio")
	esperar(t, w, http.StatusOK, "Paso 5 de 5", "Antes de enviar, completa lo siguiente", fmt.Sprintf(`href="%s/documentos#doc-autorizacion-o%d"`, base, obraID),
		`type="submit" disabled>Enviar postulación`, "Tu postulación completa", "Ana López Ruiz", "Preludio a Colón", "https://vimeo.com/123",
		"17 de mayo de 1990",
		`href="`+base+`/datos">Modificar`)
	noAparece(t, w, `name="datos.nombre_completo"`) // el resumen es de sólo lectura
	esperar(t, ana.post(base+"/envio", url.Values{"confirmo": {"1"}, "acepto_bases": {"1"}}), http.StatusUnprocessableEntity, "aún no puede enviarse")

	ana.subirDesdeLaPagina(base, [][2]string{{"clave", "autorizacion"}, {"obra_id", fmt.Sprint(obraID)}}, "a.pdf", pdfDePrueba("a"), false)
	w = ana.get(base + "/envio")
	esperar(t, w, http.StatusOK, "Tu postulación está completa", `<button class="boton" type="submit">Enviar postulación`)

	// Las dos declaraciones son obligatorias (RF-POST-10).
	w = ana.post(base+"/envio", url.Values{})
	esperar(t, w, http.StatusUnprocessableEntity, `id="e-confirmo">Debes confirmar`, `id="e-acepto_bases">Debes aceptar`)
	if p := e.postulacionDe(postulacionID); p.Estado != models.PostulacionBorrador {
		t.Fatalf("sin las declaraciones no debe enviarse: %s", p.Estado)
	}
	redirige(t, ana.post(base+"/envio", url.Values{"confirmo": {"1"}, "acepto_bases": {"1"}}), base+"/envio?aviso=enviada")
	p := e.postulacionDe(postulacionID)
	if p.Estado != models.PostulacionEnviada || p.Folio == "" || len(p.Acuses) != 1 {
		t.Fatalf("tras enviar: %s %q %d acuses", p.Estado, p.Folio, len(p.Acuses))
	}
	if len(e.correosDe(models.PlantillaAcuse)) != 1 {
		t.Error("el envío debe encolar el correo del acuse")
	}

	// La página pasa a ser la del estado: folio, acuse y qué sigue.
	w = ana.get(base + "/envio?aviso=enviada")
	esperar(t, w, http.StatusOK, "Tu postulación quedó enviada", p.Folio, "Acuse de recibo, versión 1",
		fmt.Sprintf(`href="/api/v1/postulaciones/%d/acuses/1"`, postulacionID), "espera la revisión documental")
	noAparece(t, w, "Enviar postulación", `name="confirmo"`)
	if acuse := ana.bajar(fmt.Sprintf("/api/v1/postulaciones/%d/acuses/1", postulacionID)); acuse.Code != http.StatusOK || !bytes.HasPrefix(acuse.Body.Bytes(), []byte("%PDF")) {
		t.Errorf("descarga del acuse: %d", acuse.Code)
	}
	// El enlace del correo (por folio) lleva aquí; el panel, también.
	redirige(t, ana.get("/postulaciones/"+p.Folio), base+"/envio")
	esperar(t, ana.get("/postulaciones"), http.StatusOK, p.Folio, "Enviada")

	// Enviada, nada se modifica: los pasos se leen, y lo que se intente se rechaza.
	w = ana.get(base + "/datos")
	esperar(t, w, http.StatusOK, "ya fue enviada", "Ana López Ruiz")
	noAparece(t, w, `name="datos.nombre_completo"`, "hx-post")
	esperar(t, ana.get(base+"/documentos"), http.StatusOK, "_fotografia_v1.jpg")
	noAparece(t, ana.get(base+"/documentos"), `enctype="multipart/form-data"`)
	esperar(t, ana.subirDesdeLaPagina(base, [][2]string{{"clave", "fotografia"}}, "r.jpg", contenidoJPEG, false), http.StatusUnprocessableEntity, "ya fue enviada")
	esperar(t, ana.post(base+"/envio", url.Values{"confirmo": {"1"}, "acepto_bases": {"1"}}), http.StatusUnprocessableEntity)
	if len(e.postulacionDe(postulacionID).Acuses) != 1 {
		t.Error("una postulación enviada no se vuelve a enviar")
	}
}

func TestSubsanacionDesdeLaPagina(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulanteVerificado("ana@ejemplo.mx")
	api, obraID := e.solistaCompleto(ana, interpretes, "ana")
	p := ana.exige(http.StatusOK, "POST", api+"/envio", obj{"confirmo": true, "acepto_bases": true})
	postulacionID := id(p["id"])
	base := fmt.Sprintf("/postulaciones/%d", postulacionID)

	// La revisora da por inválida la fotografía y señala dos campos.
	rita := e.revisor(admin, interpretes, "rita@ejemplo.mx")
	admin.estado(interpretes, "revision_documental", "Inicio de la revisión, en la prueba")
	rita.exige(http.StatusOK, "PUT", fmt.Sprintf("/api/v1/documentos/%d/dictamen", id(documentoDe(t, p, "fotografia")["id"])),
		obj{"dictamen": "invalido", "motivo": "La imagen está borrosa."})
	rita.exige(http.StatusCreated, "POST", api+"/subsanaciones", obj{
		"fecha_limite": "2027-01-05T05:59:00Z", "nota": "Gracias.",
		"problemas": []obj{
			{"clase": "campo", "campo": "datos.telefono", "problema": "Falta la clave lada."},
			{"clase": "campo", "campo": "obra.duracion_min", "obra_id": obraID, "problema": "No coincide con la partitura."},
		},
	})

	// El enlace lleva a la subsanación: lo señalado, con su motivo y un enlace a cada cosa.
	redirige(t, ana.get(base), base+"/envio")
	w := ana.get(base + "/envio")
	esperar(t, w, http.StatusOK, "Tu postulación tiene elementos por corregir", "4 de enero de 2027, 23:59 h", "Lo que la revisión te pide corregir",
		`href="`+base+`/datos#datos-telefono">Teléfono</a>`, "Falta la clave lada.", "La imagen está borrosa.",
		fmt.Sprintf(`href="%s/obras#o%d-obra-duracion_min">Obra 1 · Duración en minutos</a>`, base, obraID), `href="`+base+`/documentos#doc-fotografia"`,
		"Reenviar postulación", "¿Encontraste otro error?", "Ver el mensaje completo de la revisión", "Gracias.", `value="campo|datos.nombre_completo|0"`, "En subsanación")
	noAparece(t, w, `value="campo|datos.telefono|0"`) // ya está desbloqueado: no se pide

	// En los pasos, sólo lo señalado se edita, y se resalta con su motivo.
	w = ana.get(base + "/datos")
	esperar(t, w, http.StatusOK, `campo--resaltado" id="c-datos-telefono"`, "Por corregir.", `name="datos.telefono"`,
		"Ver todo lo señalado y reenviar")
	noAparece(t, w, `name="datos.nombre_completo"`, `name="liga"`, "Cambiar a «Ensamble»")
	w = ana.htmx(base+"/campos", url.Values{"seccion": {"datos"}, "datos.telefono": {"+52 55 1234 5678"}, "datos.nombre_completo": {"Otra persona"}})
	esperar(t, w, http.StatusOK, "Hay datos que no se guardaron")
	if p := e.postulacionDe(postulacionID); p.Campos["datos.telefono"] != "+52 55 1234 5678" || p.Campos["datos.nombre_completo"] != "Ana López Ruiz" {
		t.Errorf("en subsanación: teléfono %q, nombre %q", p.Campos["datos.telefono"], p.Campos["datos.nombre_completo"])
	}
	w = ana.get(base + "/documentos")
	esperar(t, w, http.StatusOK, `requisito--resaltado" id="doc-fotografia"`, "La imagen está borrosa.")
	if strings.Count(w.Body.String(), `enctype="multipart/form-data"`) != 1 {
		t.Error("en la subsanación sólo la fotografía admite una subida")
	}
	w = ana.subirDesdeLaPagina(base, [][2]string{{"clave", "partitura"}, {"obra_id", fmt.Sprint(obraID)}}, "p.pdf", pdfDePrueba("otra"), true)
	esperar(t, w, http.StatusOK, "no puede modificarse")

	// Pedir que se desbloquee otra cosa: con justificación; queda pendiente.
	esperar(t, ana.post(base+"/desbloqueos", url.Values{"elemento": {"campo|datos.nombre_completo|0"}}), http.StatusUnprocessableEntity, `id="e-desbloqueo">`)
	redirige(t, ana.post(base+"/desbloqueos", url.Values{"elemento": {"campo|datos.nombre_completo|0"}, "justificacion": {"Mi nombre lleva acento."}}),
		base+"/envio?aviso=desbloqueo#subsanacion")
	esperar(t, ana.get(base+"/envio?aviso=desbloqueo"), http.StatusOK, "Tu solicitud quedó enviada", "Pendiente", "Mi nombre lleva acento.")

	// Reenviar con la fotografía nueva: segundo acuse, y vuelve a revisión.
	ana.subirDesdeLaPagina(base, [][2]string{{"clave", "fotografia"}}, "nueva.png", contenidoPNG, false)
	ana.htmx(fmt.Sprintf("%s/obras/%d", base, obraID), url.Values{"obra.duracion_min": {"14"}})
	redirige(t, ana.post(base+"/envio", url.Values{"confirmo": {"1"}, "acepto_bases": {"1"}}), base+"/envio?aviso=reenviada")
	p2 := e.postulacionDe(postulacionID)
	if p2.Estado != models.PostulacionEnRevision || len(p2.Acuses) != 2 || p2.Obras[0].Campos["obra.duracion_min"] != "14" {
		t.Fatalf("tras reenviar: %s, %d acuses, duración %q", p2.Estado, len(p2.Acuses), p2.Obras[0].Campos["obra.duracion_min"])
	}
	esperar(t, ana.get(base+"/envio?aviso=reenviada"), http.StatusOK, "quedó reenviada", "Acuse de recibo, versión 2", "está en revisión documental")
}
