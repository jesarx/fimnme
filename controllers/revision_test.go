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
)

// Pruebas de extremo a extremo del paso 7: revisión documental, subsanación
// con desbloqueo selectivo, reenvío y dictamen.

// revisor da de alta una cuenta interna, la activa con su enlace y la asigna
// como revisora de la convocatoria. Devuelve su cliente con sesión.
func (e *entornoPrueba) revisor(admin *cliente, convocatoriaID int64, email string) *cliente {
	e.t.Helper()
	usuarioID := e.crearInterno(admin, email)
	token := e.tokenDe(email, models.PlantillaActivacion, "/cuenta/activar")
	c := e.cliente()
	redirige(e.t, c.post("/cuenta/activar", url.Values{"token": {token}, "password": {passwordPrueba}}), "/cuenta/ingresar?aviso=activada")
	c.ingresar(email, passwordPrueba)
	admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/asignaciones", convocatoriaID),
		obj{"usuario_id": usuarioID, "perfil": "revisor_documental"})
	return c
}

// correosDe devuelve los correos de una plantilla en la bandeja de salida.
func (e *entornoPrueba) correosDe(plantilla string) []models.CorreoSaliente {
	var lista []models.CorreoSaliente
	for _, c := range e.correos() {
		if c.PlantillaClave == plantilla {
			lista = append(lista, c)
		}
	}
	return lista
}

// documentoDe devuelve, de una postulación, el documento vigente de un requisito.
func documentoDe(t *testing.T, postulacion obj, clave string) obj {
	t.Helper()
	for _, d := range lista(postulacion, "documentos") {
		if d["clave"] == clave {
			return d
		}
	}
	t.Fatalf("la postulación no tiene el documento %s", clave)
	return nil
}

func TestRevisionSubsanacionYDictamen(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, compositores, _, _ := e.convocatoriasAbiertas()
	si := obj{"confirmo": true, "acepto_bases": true}

	// Dos postulaciones enviadas y un borrador, que nunca entra a la revisión.
	luis := e.postulanteVerificado("luis@ejemplo.mx")
	ruta, obraID := e.solistaCompleto(luis, interpretes, "luis")
	p := luis.exige(http.StatusOK, "POST", ruta+"/envio", si)
	postulacionID := id(p["id"])
	ana := e.postulanteVerificado("ana@ejemplo.mx")
	rutaDeAna, _ := e.solistaCompleto(ana, interpretes, "ana")
	ana.exige(http.StatusOK, "POST", rutaDeAna+"/envio", si)
	eva := e.postulante("eva@ejemplo.mx")
	borrador := eva.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "ensamble"})

	rita := e.revisor(admin, interpretes, "rita@ejemplo.mx")
	raul := e.revisor(admin, interpretes, "raul@ejemplo.mx")
	deOtra := e.revisor(admin, compositores, "otra@ejemplo.mx")
	observador := e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	bandeja := fmt.Sprintf("/api/v1/convocatorias/%d/revision", interpretes)

	// Antes de la etapa de revisión el revisor ya ve lo enviado, pero no actúa.
	b := rita.exige(http.StatusOK, "GET", bandeja, nil)
	if len(lista(b, "postulaciones")) != 2 {
		t.Fatalf("la bandeja debe traer las dos enviadas y no el borrador: %v", b)
	}
	if campos := rita.rechaza("POST", ruta+"/toma", nil); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("tomar antes de la etapa de revisión: %v", campos)
	}
	admin.estado(interpretes, "revision_documental", "Inicio de la revisión, en la prueba")

	// Quién ve la bandeja: revisores de ESA convocatoria, administrador y observador.
	for nombre, caso := range map[string]struct {
		c      *cliente
		estado int
	}{"revisora": {rita, 200}, "administrador": {admin, 200}, "observador": {observador, 200},
		"revisor de otra convocatoria": {deOtra, 403}, "postulante": {luis, 403}, "sin sesión": {e.cliente(), 401}} {
		if estado, _ := caso.c.api("GET", bandeja, nil); estado != caso.estado {
			t.Errorf("bandeja para %s: %d, se esperaba %d", nombre, estado, caso.estado)
		}
	}
	// Y quién ve una postulación: el revisor de otra convocatoria, no; nadie ve el borrador ajeno salvo quien supervisa.
	if estado, _ := deOtra.api("GET", ruta, nil); estado != http.StatusNotFound {
		t.Errorf("el revisor de otra convocatoria no ve la postulación: %d", estado)
	}
	if estado, _ := rita.api("GET", fmt.Sprintf("/api/v1/postulaciones/%d", id(borrador["id"])), nil); estado != http.StatusNotFound {
		t.Errorf("un revisor no ve borradores: %d", estado)
	}
	// El observador consulta, pero no revisa; el titular tampoco.
	if estado, _ := observador.api("POST", ruta+"/toma", nil); estado != http.StatusForbidden {
		t.Errorf("el observador no toma postulaciones: %d", estado)
	}
	if estado, _ := luis.api("POST", ruta+"/dictamen", obj{"dictamen": "validada"}); estado != http.StatusForbidden {
		t.Errorf("el postulante no dictamina su postulación: %d", estado)
	}

	// La revisora ve la postulación con el bloque de revisión; el titular, sin él.
	vista := rita.exige(http.StatusOK, "GET", ruta, nil)
	revision := vista["revision"].(obj)
	if vista["editable"] != false || revision["titular"].(obj)["email"] != "luis@ejemplo.mx" || revision["toma"] != nil ||
		len(lista(revision, "dictamenes")) != 0 || len(lista(vista, "documentos")) != 3 {
		t.Errorf("vista de revisión: %v", revision)
	}
	if propia := luis.exige(http.StatusOK, "GET", ruta, nil); propia["revision"] != nil || propia["subsanacion"] != nil {
		t.Errorf("el titular no recibe el bloque de revisión: %v", propia["revision"])
	}

	// Toma blanda: informa, no bloquea (RN-14b).
	vista = rita.exige(http.StatusOK, "POST", ruta+"/toma", nil)
	if vista["estado"] != "en_revision" || vista["revision"].(obj)["toma"].(obj)["revisor"] != "Persona Interna" {
		t.Errorf("tras tomarla: %v %v", vista["estado"], vista["revision"].(obj)["toma"])
	}
	if mias := lista(rita.exige(http.StatusOK, "GET", bandeja+"?toma=mia", nil), "postulaciones"); len(mias) != 1 || id(mias[0]["id"]) != postulacionID {
		t.Errorf("filtro «tomadas por mí»: %v", mias)
	}
	if otras := lista(raul.exige(http.StatusOK, "GET", bandeja+"?toma=otra", nil), "postulaciones"); len(otras) != 1 {
		t.Errorf("filtro «tomadas por otro»: %v", otras)
	}
	if libres := lista(raul.exige(http.StatusOK, "GET", bandeja+"?toma=libre&estado=enviada", nil), "postulaciones"); len(libres) != 1 || libres[0]["folio"] != "FIM50-INT-0002" {
		t.Errorf("filtro de libres: %v", libres)
	}
	if porFolio := lista(raul.exige(http.StatusOK, "GET", bandeja+"?q=int-0001", nil), "postulaciones"); len(porFolio) != 1 || porFolio[0]["nombre"] != "Ana López Ruiz" {
		t.Errorf("búsqueda por folio: %v", porFolio)
	}
	if estado, _ := raul.api("GET", bandeja+"?toma=todas", nil); estado != http.StatusBadRequest {
		t.Errorf("filtro de toma inválido: %d", estado)
	}
	// Otro revisor puede intervenir aunque esté tomada, y tomarla él.
	raul.exige(http.StatusOK, "POST", ruta+"/toma", nil)
	vista = raul.exige(http.StatusOK, "DELETE", ruta+"/toma", nil)
	if vista["revision"].(obj)["toma"] != nil {
		t.Errorf("la toma debió liberarse: %v", vista["revision"].(obj)["toma"])
	}

	// Dictamen de documentos: inválido exige motivo (RN-11).
	partitura := documentoDe(t, vista, "partitura")
	rutaDictamen := fmt.Sprintf("/api/v1/documentos/%d/dictamen", id(partitura["id"]))
	if campos := rita.rechaza("PUT", rutaDictamen, obj{"dictamen": "invalido"}); fmt.Sprint(campos) != "[motivo]" {
		t.Errorf("documento inválido sin motivo: %v", campos)
	}
	if campos := rita.rechaza("PUT", rutaDictamen, obj{"dictamen": "regular"}); fmt.Sprint(campos) != "[dictamen]" {
		t.Errorf("dictamen inventado: %v", campos)
	}
	if estado, _ := deOtra.api("PUT", rutaDictamen, obj{"dictamen": "valido"}); estado != http.StatusNotFound {
		t.Errorf("dictamen de un revisor de otra convocatoria: %d", estado)
	}
	vista = rita.exige(http.StatusOK, "PUT", rutaDictamen, obj{"dictamen": "invalido", "motivo": "Faltan las páginas 3 y 4."})
	rita.exige(http.StatusOK, "PUT", fmt.Sprintf("/api/v1/documentos/%d/dictamen", id(documentoDe(t, vista, "fotografia")["id"])), obj{"dictamen": "valido"})
	vista = rita.exige(http.StatusOK, "GET", ruta, nil)
	if documentoDe(t, vista, "partitura")["dictamen"] != "invalido" || documentoDe(t, vista, "fotografia")["dictamen"] != "valido" ||
		len(lista(vista["revision"].(obj), "dictamenes")) != 2 {
		t.Errorf("dictámenes: %v", vista["revision"].(obj)["dictamenes"])
	}
	// El revisor descarga los documentos, y queda registrado.
	if d := rita.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", id(partitura["id"]))); d.Code != http.StatusOK {
		t.Errorf("descarga del revisor: %d", d.Code)
	}

	// Observaciones: internas.
	if campos := rita.rechaza("POST", ruta+"/observaciones", obj{"texto": " "}); fmt.Sprint(campos) != "[texto]" {
		t.Errorf("observación vacía: %v", campos)
	}
	rita.exige(http.StatusCreated, "POST", ruta+"/observaciones", obj{"texto": "La semblanza menciona otra convocatoria."})
	vista = rita.exige(http.StatusCreated, "POST", ruta+"/observaciones", obj{"obra_id": obraID, "texto": "Confirmar la duración."})
	if observaciones := lista(vista["revision"].(obj), "observaciones"); len(observaciones) != 2 || id(observaciones[1]["obra_id"]) != obraID {
		t.Errorf("observaciones: %v", observaciones)
	}
	if propia := luis.exige(http.StatusOK, "GET", ruta, nil); strings.Contains(fmt.Sprint(propia), "otra convocatoria") || propia["estado"] != "en_revision" {
		t.Error("el titular ve el estado, pero no las observaciones")
	}

	// No se valida con un documento inválido; descartar exige motivo.
	if campos := rita.rechaza("POST", ruta+"/dictamen", obj{"dictamen": "validada"}); fmt.Sprint(campos) != "[dictamen]" {
		t.Errorf("validar con un documento inválido: %v", campos)
	}
	if campos := rita.rechaza("POST", ruta+"/dictamen", obj{"dictamen": "descartada"}); fmt.Sprint(campos) != "[motivo]" {
		t.Errorf("descartar sin motivo: %v", campos)
	}

	// --- Subsanación --------------------------------------------------------
	limite := "2027-01-05T05:59:00Z" // 4 de enero, 23:59 en la Ciudad de México
	for nombre, caso := range map[string]struct {
		c      *cliente
		ruta   string
		cuerpo obj
		campos string
	}{
		"sin nada que corregir":    {rita, rutaDeAna, obj{"fecha_limite": limite}, "[problemas]"},
		"fecha límite ya pasada":   {rita, ruta, obj{"fecha_limite": "2026-11-01T00:00:00Z"}, "[fecha_limite]"},
		"campo que no existe":      {rita, ruta, obj{"fecha_limite": limite, "problemas": []obj{{"clase": "campo", "campo": "datos.curp", "problema": "x"}}}, "[campo]"},
		"problema sin explicación": {rita, ruta, obj{"fecha_limite": limite, "problemas": []obj{{"clase": "campo", "campo": "datos.telefono"}}}, "[problema]"},
	} {
		if campos := caso.c.rechaza("POST", caso.ruta+"/subsanaciones", caso.cuerpo); fmt.Sprint(campos) != caso.campos {
			t.Errorf("subsanación (%s): %v, se esperaba %s", nombre, campos, caso.campos)
		}
	}
	correosAntes := len(e.correos())
	vista = rita.exige(http.StatusCreated, "POST", ruta+"/subsanaciones", obj{
		"fecha_limite": limite, "nota": "Cualquier duda, escríbenos.",
		"problemas": []obj{
			{"clase": "campo", "campo": "datos.telefono", "problema": "Falta la clave lada."},
			{"clase": "campo", "campo": "obra.duracion_min", "obra_id": obraID, "problema": "No coincide con la partitura."},
		},
	})
	// La partitura inválida se añadió sola, con el motivo de su dictamen.
	subsanacion := vista["subsanacion"].(obj)
	problemas := lista(subsanacion, "problemas")
	if vista["estado"] != "en_subsanacion" || len(problemas) != 3 || problemas[2]["clase"] != "documento" || problemas[2]["campo"] != "partitura" ||
		problemas[2]["problema"] != "Faltan las páginas 3 y 4." || problemas[0]["etiqueta"] != "Teléfono" || problemas[2]["etiqueta"] != "Partitura" ||
		subsanacion["vencida"] != false || subsanacion["fecha_limite"] != limite {
		t.Fatalf("subsanación abierta: %v %v", vista["estado"], subsanacion)
	}
	// Un solo correo, al postulante, con la lista, el plazo y la nota (RF-REV-05b).
	enviados := e.correosDe(models.PlantillaSubsanacion)
	if len(e.correos()) != correosAntes+1 || len(enviados) != 1 {
		t.Fatalf("debe encolarse un solo correo de subsanación: %d", len(enviados))
	}
	correo := enviados[0]
	for _, fragmento := range []string{"**FIM50-INT-0001**", "• Teléfono: Falta la clave lada.", "• Duración en minutos (obra 1): No coincide con la partitura.",
		"• Partitura (obra 1): Faltan las páginas 3 y 4.", "4 de enero de 2027, 23:59 h (hora de la Ciudad de México)", "Cualquier duda, escríbenos.",
		urlBasePrueba + "/postulaciones/FIM50-INT-0001"} {
		if !strings.Contains(correo.Texto, fragmento) {
			t.Errorf("falta en el correo de subsanación: %q\n%s", fragmento, correo.Texto)
		}
	}
	if correo.Destinatario != "luis@ejemplo.mx" || strings.Contains(correo.Texto, "{{") || !strings.Contains(correo.HTML, "Falta la clave lada.<br>") {
		t.Errorf("correo de subsanación: %+v", correo)
	}
	// Mientras el postulante corrige, el revisor espera.
	for nombre, intento := range map[string]func() []string{
		"dictaminar un documento": func() []string { return rita.rechaza("PUT", rutaDictamen, obj{"dictamen": "valido"}) },
		"dictaminar la postulación": func() []string {
			return rita.rechaza("POST", ruta+"/dictamen", obj{"dictamen": "descartada", "motivo": "No subsanó."})
		},
		"otra subsanación": func() []string {
			return rita.rechaza("POST", ruta+"/subsanaciones", obj{"fecha_limite": limite, "problemas": []obj{{"clase": "campo", "campo": "datos.genero", "problema": "x"}}})
		},
	} {
		if campos := intento(); fmt.Sprint(campos) != "[estado]" {
			t.Errorf("%s con la subsanación en curso: %v", nombre, campos)
		}
	}

	// --- El postulante: sólo lo señalado es editable (§4.2) -------------------
	propia := luis.exige(http.StatusOK, "GET", ruta, nil)
	if propia["editable"] != true || len(lista(propia["subsanacion"].(obj), "problemas")) != 3 || propia["revision"] != nil {
		t.Fatalf("lo que ve el postulante en subsanación: %v", propia["subsanacion"])
	}
	guardados, rechazados := guardar(t, luis, ruta, obj{"datos.telefono": "+52 55 1234 5678", "datos.semblanza": "Otra semblanza.", "concierto.entidad": "OAX"})
	if guardados != "[datos.telefono]" || rechazados != "[concierto.entidad datos.semblanza]" {
		t.Errorf("sólo debe guardarse el campo señalado: %s %s", guardados, rechazados)
	}
	rutaObra := fmt.Sprintf("/api/v1/obras/%d", obraID)
	if g, rz := guardar(t, luis, rutaObra, obj{"obra.duracion_min": 14, "obra.titulo": "Otro título"}); g != "[obra.duracion_min]" || rz != "[obra.titulo]" {
		t.Errorf("campos de la obra en subsanación: %s %s", g, rz)
	}
	for nombre, intento := range map[string]func() []string{
		"añadir una obra":    func() []string { return luis.rechaza("POST", ruta+"/obras", nil) },
		"quitar la obra":     func() []string { return luis.rechaza("DELETE", rutaObra, nil) },
		"cambiar los videos": func() []string { return luis.rechaza("PUT", ruta+"/videos", obj{"urls": []string{"https://a.mx/1"}}) },
		"cambiar de subcategoría": func() []string {
			return luis.rechaza("PUT", ruta+"/subcategoria", obj{"subcategoria": "ensamble", "confirmar": true})
		},
		"quitar un documento no señalado": func() []string {
			return luis.rechaza("DELETE", fmt.Sprintf("/api/v1/documentos/%d", id(documentoDe(t, propia, "fotografia")["id"])), nil)
		},
	} {
		if campos := intento(); len(campos) != 1 {
			t.Errorf("%s durante la subsanación debe rechazarse: %v", nombre, campos)
		}
	}
	if problema := luis.rechazoDeSubida(ruta, [][2]string{{"clave", "fotografia"}}, "retrato.jpg", contenidoJPEG); !strings.Contains(problema, "no está entre los señalados") {
		t.Errorf("subir un documento no señalado: %s", problema)
	}
	// La partitura señalada sí: sube una versión nueva.
	propia = luis.documento(ruta, "partitura", obraID, "partitura-completa.pdf", pdfDePrueba("partitura completa"))
	if nueva := documentoDe(t, propia, "partitura"); id(nueva["version"]) != 2 || nueva["dictamen"] != "pendiente" {
		t.Errorf("partitura subsanada: %v", nueva)
	}
	if propia = luis.exige(http.StatusOK, "GET", ruta, nil); propia["campos"].(obj)["datos.semblanza"] != "Semblanza de Ana." {
		t.Error("lo no señalado debe quedar como se envió")
	}

	// Solicitud de desbloqueo de algo que el revisor no señaló (RN-12d).
	semblanza := obj{"clase": "campo", "campo": "datos.semblanza", "justificacion": "Menciona por error otra convocatoria."}
	if campos := luis.rechaza("POST", ruta+"/desbloqueos", obj{"clase": "campo", "campo": "datos.telefono", "justificacion": "x"}); fmt.Sprint(campos) != "[campo]" {
		t.Errorf("pedir algo ya desbloqueado: %v", campos)
	}
	if estado, _ := ana.api("POST", ruta+"/desbloqueos", semblanza); estado != http.StatusNotFound {
		t.Errorf("solicitud sobre una postulación ajena: %d", estado)
	}
	propia = luis.exige(http.StatusCreated, "POST", ruta+"/desbloqueos", semblanza)
	propia = luis.exige(http.StatusCreated, "POST", ruta+"/desbloqueos", obj{"clase": "documento", "campo": "fotografia", "justificacion": "Tengo una mejor."})
	if campos := luis.rechaza("POST", ruta+"/desbloqueos", semblanza); fmt.Sprint(campos) != "[campo]" {
		t.Errorf("solicitud repetida: %v", campos)
	}
	solicitudes := lista(propia["subsanacion"].(obj), "solicitudes")
	if len(solicitudes) != 2 || solicitudes[0]["concedida"] != nil || solicitudes[0]["etiqueta"] != "Semblanza artística" {
		t.Fatalf("solicitudes de desbloqueo: %v", solicitudes)
	}
	// Pedirlo no desbloquea nada.
	if _, rz := guardar(t, luis, ruta, obj{"datos.semblanza": "Semblanza corregida.", "datos.telefono": "+52 55 1234 5678"}); rz != "[datos.semblanza]" {
		t.Errorf("una solicitud sin resolver no desbloquea: %s", rz)
	}
	rutaSolicitud := func(n int) string { return fmt.Sprintf("%s/desbloqueos/%d", ruta, id(solicitudes[n]["id"])) }
	if estado, _ := luis.api("PUT", rutaSolicitud(0), obj{"concedida": true}); estado != http.StatusForbidden {
		t.Errorf("el postulante no se concede a sí mismo: %d", estado)
	}
	if campos := rita.rechaza("PUT", rutaSolicitud(1), obj{"concedida": false}); fmt.Sprint(campos) != "[motivo]" {
		t.Errorf("negar sin motivo: %v", campos)
	}
	if estado, _ := rita.api("PUT", rutaSolicitud(0), obj{}); estado != http.StatusBadRequest {
		t.Errorf("respuesta sin «concedida»: %d", estado)
	}
	rita.exige(http.StatusOK, "PUT", rutaSolicitud(1), obj{"concedida": false, "motivo": "La fotografía cumple las bases."})
	vista = raul.exige(http.StatusOK, "PUT", rutaSolicitud(0), obj{"concedida": true})
	if campos := rita.rechaza("PUT", rutaSolicitud(0), obj{"concedida": true}); fmt.Sprint(campos) != "[solicitud]" {
		t.Errorf("resolver dos veces: %v", campos)
	}
	if n := len(lista(vista["subsanacion"].(obj), "problemas")); n != 4 {
		t.Errorf("lo concedido se suma a los problemas de la subsanación: %d", n)
	}
	// Concedido, ya puede; negado, sigue sin poder.
	if g, _ := guardar(t, luis, ruta, obj{"datos.semblanza": "Semblanza corregida."}); g != "[datos.semblanza]" {
		t.Errorf("el campo concedido debe poder guardarse: %s", g)
	}
	if problema := luis.rechazoDeSubida(ruta, [][2]string{{"clave", "fotografia"}}, "retrato.jpg", contenidoJPEG); !strings.Contains(problema, "no está entre los señalados") {
		t.Errorf("lo negado sigue bloqueado: %s", problema)
	}

	// --- Reenvío --------------------------------------------------------------
	propia = luis.exige(http.StatusOK, "POST", ruta+"/envio", si)
	if propia["estado"] != "en_revision" || propia["folio"] != "FIM50-INT-0001" || id(propia["envios"]) != 2 || propia["editable"] != false ||
		len(lista(propia, "acuses")) != 2 || propia["subsanacion"] != nil {
		t.Fatalf("tras el reenvío: %v %v %v", propia["estado"], propia["folio"], propia["envios"])
	}
	// El acuse anterior se conserva (RN-13) y el nuevo es otro archivo.
	primero, segundo := luis.bajar(ruta+"/acuses/1"), luis.bajar(ruta+"/acuses/2")
	if primero.Code != http.StatusOK || segundo.Code != http.StatusOK || sumaDe(primero.Body.Bytes()) == sumaDe(segundo.Body.Bytes()) {
		t.Errorf("acuses del envío y del reenvío: %d %d", primero.Code, segundo.Code)
	}
	if acuses := e.correosDe(models.PlantillaAcuseSubsanacion); len(acuses) != 1 || acuses[0].Destinatario != "luis@ejemplo.mx" ||
		len(acuses[0].Adjuntos) != 1 || acuses[0].Adjuntos[0].Nombre != "acuse_FIM50-INT-0001_v2.pdf" {
		t.Errorf("acuse de la subsanación: %+v", acuses)
	}
	// Aviso a los dos revisores de la convocatoria, y sólo a ellos (RF-REV-07).
	avisos := e.correosDe(models.PlantillaAvisoSubsanada)
	if len(avisos) != 2 || avisos[0].Destinatario != "rita@ejemplo.mx" || avisos[1].Destinatario != "raul@ejemplo.mx" || !strings.Contains(avisos[0].Texto, "FIM50-INT-0001") {
		t.Errorf("avisos a los revisores: %+v", avisos)
	}
	if n := len(e.asientos("postulacion.reenviar")); n != 1 || len(e.asientos("postulacion.enviar")) != 2 {
		t.Errorf("asientos del reenvío: %d", n)
	}
	if campos := luis.rechaza("PATCH", ruta, obj{"campos": obj{"datos.telefono": "1"}}); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("tras reenviar vuelve a quedar congelada: %v", campos)
	}

	// La bandeja la marca como subsanada, y la vista trae el diff.
	renglon := lista(rita.exige(http.StatusOK, "GET", bandeja+"?q=0001", nil), "postulaciones")[0]
	if renglon["subsanada"] != true || renglon["estado"] != "en_revision" || id(renglon["envios"]) != 2 || id(renglon["documentos_invalidos"]) != 0 || id(renglon["observaciones"]) != 2 {
		t.Errorf("renglón de la bandeja tras el reenvío: %v", renglon)
	}
	vista = rita.exige(http.StatusOK, "GET", ruta, nil)
	var cambios []string
	for _, c := range lista(vista["revision"].(obj), "cambios") {
		cambios = append(cambios, fmt.Sprintf("%s:%v→%v", c["etiqueta"], c["antes"], c["despues"]))
	}
	diff := strings.Join(cambios, " | ")
	for _, fragmento := range []string{"Teléfono:55 1234 5678→+52 55 1234 5678", "Semblanza artística:Semblanza de Ana.→Semblanza corregida.",
		"Duración en minutos:12→14", "Partitura:"} {
		if !strings.Contains(diff, fragmento) {
			t.Errorf("falta en el diff %q: %s", fragmento, diff)
		}
	}
	if len(cambios) != 4 {
		t.Errorf("el diff debe traer exactamente los cuatro cambios: %s", diff)
	}

	// --- Dictamen ---------------------------------------------------------------
	correosAntes = len(e.correos())
	vista = rita.exige(http.StatusOK, "POST", ruta+"/dictamen", obj{"dictamen": "validada"})
	if vista["estado"] != "validada" || lista(vista["revision"].(obj), "subsanaciones")[0]["cerrada_at"] == nil {
		t.Errorf("validada: %v", vista["estado"])
	}
	// Un dictamen puede corregirse mientras dure la etapa.
	vista = raul.exige(http.StatusOK, "POST", ruta+"/dictamen", obj{"dictamen": "descartada", "motivo": "El programa no es de música nueva."})
	if vista["estado"] != "descartada" || vista["revision"].(obj)["motivo_descarte"] != "El programa no es de música nueva." {
		t.Errorf("descartada: %v %v", vista["estado"], vista["revision"].(obj)["motivo_descarte"])
	}
	// El postulante ve su estado, no el motivo; y no recibe ningún correo.
	if propia = luis.exige(http.StatusOK, "GET", ruta, nil); propia["estado"] != "descartada" || strings.Contains(fmt.Sprint(propia), "música nueva.") {
		t.Error("el postulante ve el estado, pero no el motivo del descarte")
	}
	if len(e.correos()) != correosAntes {
		t.Error("el dictamen no debe enviar ningún correo")
	}
	tablero := rita.exige(http.StatusOK, "GET", bandeja, nil)["tablero"].(obj)
	if id(tablero["total"]) != 2 || fmt.Sprint(tablero["por_estado"]) != "map[descartada:1 enviada:1]" {
		t.Errorf("tablero: %v", tablero)
	}
	if n := len(e.asientos("postulacion.dictaminar")); n != 2 {
		t.Errorf("asientos de dictamen: %d", n)
	}

	// --- Subsanación vencida (RF-REV-09) ---------------------------------------
	rita.exige(http.StatusCreated, "POST", rutaDeAna+"/subsanaciones", obj{"fecha_limite": "2026-12-01T06:00:00Z",
		"problemas": []obj{{"clase": "campo", "campo": "videos", "problema": "La liga no abre."}}})
	// Señalar la lista de videos la desbloquea entera.
	ana.exige(http.StatusOK, "PUT", rutaDeAna+"/videos", obj{"urls": []string{"https://vimeo.com/999"}})
	e.hasta("2026-12-01T06:00:01Z")
	for _, c := range []*cliente{ana, rita} {
		c.ingresar(map[*cliente]string{ana: "ana@ejemplo.mx", rita: "rita@ejemplo.mx"}[c], passwordPrueba)
	}
	if campos := ana.rechaza("PUT", rutaDeAna+"/videos", obj{"urls": []string{"https://vimeo.com/1000"}}); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("editar con la subsanación vencida: %v", campos)
	}
	if campos := ana.rechaza("POST", rutaDeAna+"/envio", si); fmt.Sprint(campos) != "[estado]" {
		t.Errorf("reenviar con la subsanación vencida: %v", campos)
	}
	// No se descarta sola: queda marcada para el revisor, que decide.
	renglon = lista(rita.exige(http.StatusOK, "GET", bandeja+"?estado=en_subsanacion", nil), "postulaciones")[0]
	if renglon["vencida"] != true || renglon["estado"] != "en_subsanacion" {
		t.Errorf("la subsanación vencida debe quedar marcada, no descartada: %v", renglon)
	}
	vista = rita.exige(http.StatusOK, "POST", rutaDeAna+"/dictamen", obj{"dictamen": "descartada", "motivo": "No subsanó en el plazo."})
	if vista["estado"] != "descartada" || vista["subsanacion"] != nil {
		t.Errorf("dictamen tras el vencimiento: %v", vista["estado"])
	}
}
