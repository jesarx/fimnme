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
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pruebas de las pantallas del revisor (paso 12a): quién entra, la bandeja y
// cada acción de la vista de revisión, por formulario normal. Lo que queda
// guardado se comprueba en la base.

// revisionDePrueba son dos postulaciones enviadas en Intérpretes, sus
// titulares y quienes las revisan o supervisan.
type revisionDePrueba struct {
	e                                     *entornoPrueba
	admin, rita, otra, observa, luis, ana *cliente
	interpretes                           int64
	idLuis, idAna                         int64
	obraDeLuis                            int64
}

func nuevaRevisionDePrueba(t *testing.T) *revisionDePrueba {
	t.Helper()
	e := nuevoEntorno(t)
	admin, interpretes, compositores, _, _ := e.convocatoriasAbiertas()
	x := &revisionDePrueba{e: e, admin: admin, interpretes: interpretes}
	si := obj{"confirmo": true, "acepto_bases": true}
	x.luis, x.ana = e.postulanteVerificado("luis@ejemplo.mx"), e.postulanteVerificado("ana@ejemplo.mx")
	rutaLuis, obraID := e.solistaCompleto(x.luis, interpretes, "luis")
	x.idLuis, x.obraDeLuis = id(x.luis.exige(http.StatusOK, "POST", rutaLuis+"/envio", si)["id"]), obraID
	rutaAna, _ := e.solistaCompleto(x.ana, interpretes, "ana")
	x.idAna = id(x.ana.exige(http.StatusOK, "POST", rutaAna+"/envio", si)["id"])
	x.rita = e.revisor(admin, interpretes, "rita@ejemplo.mx")
	x.otra = e.revisor(admin, compositores, "otra@ejemplo.mx")
	x.observa = e.activarCuentaInterna("observa@ejemplo.mx", models.PerfilCoordinacionNacional)
	return x
}

// documentoDeLaBase devuelve el documento vigente de un requisito.
func (x *revisionDePrueba) documentoDeLaBase(postulacionID int64, clave string) models.Documento {
	for _, d := range x.e.postulacionDe(postulacionID).Documentos {
		if d.Clave == clave {
			return d
		}
	}
	x.e.t.Fatalf("la postulación %d no tiene %s", postulacionID, clave)
	return models.Documento{}
}

func TestBandejaDelRevisor(t *testing.T) {
	x := nuevaRevisionDePrueba(t)
	bandeja := fmt.Sprintf("/revision/convocatorias/%d", x.interpretes)

	// El menú ofrece la revisión a quien revisa o supervisa, y a nadie más.
	esperar(t, x.rita.get("/postulaciones"), http.StatusOK, `href="/revision"`)
	noAparece(t, x.luis.get("/postulaciones"), `href="/revision"`)
	if w := x.luis.get("/revision"); w.Code != http.StatusForbidden {
		t.Errorf("un postulante en /revision: estado %d", w.Code)
	}
	// Cada revisor ve sus convocatorias; quien supervisa, todas.
	w := x.rita.get("/revision")
	esperar(t, w, http.StatusOK, "Revisión documental", "Convocatoria de Intérpretes 2028", "Postulaciones enviadas: 2", `href="`+bandeja+`"`)
	noAparece(t, w, "Convocatoria de Compositores 2028")
	esperar(t, x.observa.get("/revision"), http.StatusOK, "Convocatoria de Intérpretes 2028", "Convocatoria de Compositores 2028")

	// La bandeja: todas las enviadas, con su tablero; la de otra convocatoria, no.
	luis, ana := x.e.postulacionDe(x.idLuis), x.e.postulacionDe(x.idAna)
	w = x.rita.get(bandeja)
	esperar(t, w, http.StatusOK, luis.Folio, ana.Folio, `href="/revision/postulaciones/`, "<b>2</b>", "Enviada", "Se muestran 2 de 2.",
		`hx-select="#resultado"`, `name="toma"`)
	if w := x.otra.get(bandeja); w.Code != http.StatusNotFound {
		t.Errorf("un revisor de otra convocatoria en la bandeja: estado %d", w.Code)
	}
	if w := x.luis.get(bandeja); w.Code != http.StatusNotFound {
		t.Errorf("un postulante en la bandeja: estado %d", w.Code)
	}
	// Los filtros van en la URL y combinan.
	w = x.rita.get(bandeja + "?q=" + url.QueryEscape(luis.Folio))
	esperar(t, w, http.StatusOK, luis.Folio, "Se muestran 1 de 2.", `value="`+luis.Folio+`"`)
	noAparece(t, w, ana.Folio)
	esperar(t, x.rita.get(bandeja+"?toma=mia"), http.StatusOK, "Ninguna postulación cumple los filtros.")
	esperar(t, x.rita.get(bandeja+"?estado=descartada"), http.StatusOK, "Ninguna postulación cumple los filtros.")
	// En inglés.
	x.rita.post("/idioma", url.Values{"idioma": {"en"}, "volver": {bandeja}})
	esperar(t, x.rita.get(bandeja), http.StatusOK, "Review inbox", "Submitted")
}

func TestVistaDeRevision(t *testing.T) {
	x := nuevaRevisionDePrueba(t)
	base := fmt.Sprintf("/revision/postulaciones/%d", x.idLuis)
	partitura := x.documentoDeLaBase(x.idLuis, "partitura")
	foto := x.documentoDeLaBase(x.idLuis, "fotografia")

	// Quién la ve: la revisora y quien supervisa. El titular, un revisor de
	// otra convocatoria o un postulante cualquiera: 404.
	w := x.rita.get(base)
	esperar(t, w, http.StatusOK, x.e.postulacionDe(x.idLuis).Folio, "Ana López Ruiz", "luis@ejemplo.mx", "Preludio a Colón",
		fmt.Sprintf(`<iframe class="visor__marco" src="/api/v1/documentos/%d/archivo"`, partitura.ID), `loading="lazy"`,
		fmt.Sprintf(`<img class="visor__imagen" src="/api/v1/documentos/%d/archivo"`, foto.ID),
		fmt.Sprintf(`action="%s/documentos/%d/dictamen"`, base, partitura.ID), `action="`+base+`/toma"`, "Pedir una subsanación",
		`value="campo|datos.telefono|0"`, "Sin dictaminar")
	for nombre, c := range map[string]*cliente{"titular": x.luis, "revisora de otra convocatoria": x.otra, "otra postulante": x.ana} {
		if w := c.get(base); w.Code != http.StatusNotFound {
			t.Errorf("%s en la vista de revisión: estado %d", nombre, w.Code)
		}
	}
	// El observador la ve entera, sin un solo formulario; y no puede actuar.
	w = x.observa.get(base)
	esperar(t, w, http.StatusOK, "Preludio a Colón", `class="visor__marco"`)
	noAparece(t, w, `method="post" action="`+base)
	if w := x.observa.post(base+"/toma", nil); w.Code != http.StatusForbidden {
		t.Errorf("el observador no toma postulaciones: estado %d", w.Code)
	}

	// Antes de la etapa de revisión, el modelo no deja actuar: se dice junto al formulario.
	w = x.rita.post(base+"/toma", nil)
	esperar(t, w, http.StatusUnprocessableEntity, `class="aviso aviso--error" role="alert"`)
	x.admin.estado(x.interpretes, "revision_documental", "Inicio de la revisión, en la prueba")

	// Toma.
	redirige(t, x.rita.post(base+"/toma", nil), base+"?aviso=tomada#toma")
	esperar(t, x.rita.get(base+"?aviso=tomada"), http.StatusOK, "Quedó tomada a tu nombre.", "Liberarla")
	esperar(t, x.rita.get(fmt.Sprintf("/revision/convocatorias/%d?toma=mia", x.interpretes)), http.StatusOK, "Se muestran 1 de 2.")

	// Dictamen de un documento: inválido exige motivo; uno de otra postulación no se alcanza por aquí.
	ruta := fmt.Sprintf("%s/documentos/%d/dictamen", base, partitura.ID)
	esperar(t, x.rita.post(ruta, url.Values{"dictamen": {"invalido"}}), http.StatusUnprocessableEntity, `role="alert"`)
	redirige(t, x.rita.post(ruta, url.Values{"dictamen": {"invalido"}, "motivo": {"Faltan las páginas 3 y 4."}}), fmt.Sprintf("%s?aviso=dictamen#documento-%d", base, partitura.ID))
	redirige(t, x.rita.post(fmt.Sprintf("%s/documentos/%d/dictamen", base, foto.ID), url.Values{"dictamen": {"valido"}}), fmt.Sprintf("%s?aviso=dictamen#documento-%d", base, foto.ID))
	ajeno := x.documentoDeLaBase(x.idAna, "partitura")
	if w := x.rita.post(fmt.Sprintf("%s/documentos/%d/dictamen", base, ajeno.ID), url.Values{"dictamen": {"valido"}}); w.Code != http.StatusNotFound {
		t.Errorf("dictaminar un documento de otra postulación por esta ruta: estado %d", w.Code)
	}
	if d := x.documentoDeLaBase(x.idAna, "partitura"); d.Dictamen != "pendiente" {
		t.Errorf("el documento ajeno no debía dictaminarse: %s", d.Dictamen)
	}
	w = x.rita.get(base)
	esperar(t, w, http.StatusOK, "documento-revision--invalido", "Faltan las páginas 3 y 4.", "documento-revision--valido",
		"Los 1 documentos dictaminados como inválidos se incluyen solos")

	// Observaciones: internas; vacía, no.
	esperar(t, x.rita.post(base+"/observaciones", url.Values{"texto": {"  "}}), http.StatusUnprocessableEntity)
	redirige(t, x.rita.post(base+"/observaciones", url.Values{"texto": {"Confirmar la duración."}, "obra": {fmt.Sprint(x.obraDeLuis)}}), base+"?aviso=observacion#observaciones")
	esperar(t, x.rita.get(base), http.StatusOK, "Confirmar la duración.", "Obra 1 · Preludio a Colón")
	if strings.Contains(x.luis.get(fmt.Sprintf("/postulaciones/%d/envio", x.idLuis)).Body.String(), "Confirmar la duración.") {
		t.Error("el titular no debe ver las observaciones internas")
	}

	// Subsanación: los renglones vacíos no cuentan; la partitura inválida se suma sola.
	limite := x.e.reloj.Ahora().In(views.ZonaMexico()).AddDate(0, 0, 10).Format("2006-01-02") + "T23:59"
	w = x.rita.post(base+"/subsanacion", url.Values{"elemento": {"campo|datos.telefono|0", ""}, "problema": {"", ""}, "fecha_limite": {limite}})
	esperar(t, w, http.StatusUnprocessableEntity, "<details class=\"nueva-subsanacion\" id=\"subsanacion\" open>")
	if p := x.e.postulacionDe(x.idLuis); p.Estado == models.PostulacionEnSubsanacion {
		t.Fatal("un elemento señalado sin explicación no debe pedir la subsanación")
	}
	redirige(t, x.rita.post(base+"/subsanacion", url.Values{
		"elemento": {"campo|datos.telefono|0", "", fmt.Sprintf("campo|obra.duracion_min|%d", x.obraDeLuis)},
		"problema": {"Falta la clave lada.", "", "No coincide con la partitura."}, "fecha_limite": {limite}, "nota": {"Gracias."},
	}), base+"?aviso=subsanacion#subsanacion")
	p := x.e.postulacionDe(x.idLuis)
	if p.Estado != models.PostulacionEnSubsanacion || len(p.Subsanaciones) != 1 || len(p.Subsanaciones[0].Problemas) != 3 {
		t.Fatalf("tras pedir la subsanación: %s, %+v", p.Estado, p.Subsanaciones)
	}
	if correos := x.e.correosDe(models.PlantillaSubsanacion); len(correos) != 1 || !strings.Contains(correos[0].Texto, "Gracias.") {
		t.Errorf("correo de subsanación: %d", len(correos))
	}
	w = x.rita.get(base)
	esperar(t, w, http.StatusOK, "Esperando al postulante", "Falta la clave lada.", "Obra 1 · Duración en minutos")
	noAparece(t, w, `action="`+base+`/subsanacion"`) // con una abierta no se pide otra

	// Desbloqueo: el titular pide; la revisora niega sin motivo (no), y concede.
	api := fmt.Sprintf("/api/v1/postulaciones/%d", x.idLuis)
	x.luis.exige(http.StatusCreated, "POST", api+"/desbloqueos", obj{"clase": "campo", "campo": "datos.nombre_completo", "justificacion": "Lleva acento."})
	solicitud := x.e.postulacionDe(x.idLuis).Subsanaciones[0].Solicitudes[0].ID
	rutaSolicitud := fmt.Sprintf("%s/desbloqueos/%d", base, solicitud)
	esperar(t, x.rita.get(base), http.StatusOK, "Lleva acento.", `action="`+rutaSolicitud+`"`)
	esperar(t, x.rita.post(rutaSolicitud, url.Values{"decision": {"negar"}}), http.StatusUnprocessableEntity)
	redirige(t, x.rita.post(rutaSolicitud, url.Values{"decision": {"conceder"}}), base+"?aviso=desbloqueo#subsanaciones")
	if s := x.e.postulacionDe(x.idLuis).Subsanaciones[0].Solicitudes[0]; !s.Resuelta || !s.Concedida {
		t.Errorf("solicitud tras concederla: %+v", s)
	}

	// Dictamen: con una subsanación en curso, no; en la otra postulación,
	// descartar exige motivo, y el titular no se entera.
	esperar(t, x.rita.post(base+"/dictamen", url.Values{"dictamen": {"validada"}}), http.StatusUnprocessableEntity)
	baseAna := fmt.Sprintf("/revision/postulaciones/%d", x.idAna)
	esperar(t, x.rita.post(baseAna+"/dictamen", url.Values{"dictamen": {"descartada"}}), http.StatusUnprocessableEntity)
	redirige(t, x.rita.post(baseAna+"/dictamen", url.Values{"dictamen": {"descartada"}, "motivo": {"No cumple las bases."}}), baseAna+"?aviso=descartada#dictamen")
	esperar(t, x.rita.get(baseAna), http.StatusOK, "Descartada. Motivo:", "No cumple las bases.")
	if propia := x.ana.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/postulaciones/%d", x.idAna), nil); propia["estado"] != "en_revision" {
		t.Errorf("la titular no debe ver el descarte: %v", propia["estado"])
	}
	redirige(t, x.rita.post(baseAna+"/dictamen", url.Values{"dictamen": {"validada"}}), baseAna+"?aviso=validada#dictamen")
	if p := x.e.postulacionDe(x.idAna); p.Estado != models.PostulacionValidada {
		t.Errorf("tras validar: %s", p.Estado)
	}
}

// Los documentos se dejan mostrar en un marco de la propia plataforma, y de
// ninguna otra; las páginas admiten marcos y audio del propio origen.
func TestCabecerasDelVisor(t *testing.T) {
	x := nuevaRevisionDePrueba(t)
	partitura := x.documentoDeLaBase(x.idLuis, "partitura")
	w := x.rita.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", partitura.ID))
	if w.Code != http.StatusOK || w.Header().Get("X-Frame-Options") != "SAMEORIGIN" ||
		w.Header().Get("Content-Security-Policy") != politicaDeArchivo || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline") {
		t.Errorf("cabeceras del documento: %d %v", w.Code, w.Header())
	}
	pagina := x.rita.get(fmt.Sprintf("/revision/postulaciones/%d", x.idLuis))
	csp := pagina.Header().Get("Content-Security-Policy")
	if pagina.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(csp, "frame-src 'self'") || !strings.Contains(csp, "media-src 'self'") ||
		!strings.Contains(csp, "frame-ancestors 'none'") || strings.Contains(csp, "unsafe") {
		t.Errorf("cabeceras de la página: %v", pagina.Header())
	}
}
