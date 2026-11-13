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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Pruebas de extremo a extremo del paso 6: documentos, envío, acuse, ligas de
// video, recordatorios y archivos de la convocatoria.

// redDePrueba hace de Internet para el verificador de ligas: responde a toda
// petición con 200, salvo a las direcciones que contengan una de las claves
// de estados. Anota lo que se le pide.
type redDePrueba struct {
	mu       sync.Mutex
	estados  map[string]int
	pedidas  []string
	fallaCon error // si no es nil, toda petición falla con este error
}

func (r *redDePrueba) RoundTrip(peticion *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	direccion := peticion.URL.String()
	r.pedidas = append(r.pedidas, direccion)
	if r.fallaCon != nil {
		return nil, r.fallaCon
	}
	estado := http.StatusOK
	for fragmento, codigo := range r.estados {
		if strings.Contains(direccion, fragmento) {
			estado = codigo
		}
	}
	return &http.Response{StatusCode: estado, Body: http.NoBody, Request: peticion, Header: http.Header{}}, nil
}

// Contenidos mínimos con la firma de cada formato.
var (
	contenidoJPEG = []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00 fotografía de prueba")
	contenidoPNG  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR imagen de prueba")
	contenidoWAV  = []byte("RIFF\x24\x08\x00\x00WAVEfmt  audio de prueba")
)

// pdfDePrueba devuelve un contenido con firma de PDF; el texto lo distingue
// de los demás, para que cada uno tenga su propia suma.
func pdfDePrueba(texto string) []byte { return []byte("%PDF-1.7\n" + texto) }

func sumaDe(contenido []byte) string {
	suma := sha256.Sum256(contenido)
	return hex.EncodeToString(suma[:])
}

// subir envía un formulario multipart: los campos, en orden, y al final el
// archivo (si nombre no está vacío).
func (c *cliente) subir(metodo, ruta string, campos [][2]string, nombre string, contenido []byte) (int, obj) {
	c.t.Helper()
	var cuerpo bytes.Buffer
	formulario := multipart.NewWriter(&cuerpo)
	for _, campo := range campos {
		if err := formulario.WriteField(campo[0], campo[1]); err != nil {
			c.t.Fatal(err)
		}
	}
	if nombre != "" {
		parte, err := formulario.CreateFormFile("archivo", nombre)
		if err != nil {
			c.t.Fatal(err)
		}
		if _, err := parte.Write(contenido); err != nil {
			c.t.Fatal(err)
		}
	}
	if err := formulario.Close(); err != nil {
		c.t.Fatal(err)
	}
	r := httptest.NewRequest(metodo, ruta, &cuerpo)
	r.Header.Set("Content-Type", formulario.FormDataContentType())
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := c.hacer(r)
	var respuesta obj
	if err := json.Unmarshal(w.Body.Bytes(), &respuesta); err != nil {
		c.t.Fatalf("%s %s no devolvió JSON (%d): %s", metodo, ruta, w.Code, w.Body.String())
	}
	return w.Code, respuesta
}

// documento sube un documento y exige que se acepte; devuelve la postulación.
func (c *cliente) documento(ruta, clave string, obraID int64, nombre string, contenido []byte) obj {
	c.t.Helper()
	campos := [][2]string{{"clave", clave}}
	if obraID != 0 {
		campos = append(campos, [2]string{"obra_id", fmt.Sprint(obraID)})
	}
	estado, respuesta := c.subir("POST", ruta+"/documentos", campos, nombre, contenido)
	if estado != http.StatusCreated {
		c.t.Fatalf("subir %s (%s): estado %d: %v", clave, nombre, estado, respuesta)
	}
	return respuesta
}

// rechazoDeSubida sube un documento que debe rechazarse con 422 y devuelve
// «campo: mensaje» del primer problema.
func (c *cliente) rechazoDeSubida(ruta string, campos [][2]string, nombre string, contenido []byte) string {
	c.t.Helper()
	estado, respuesta := c.subir("POST", ruta+"/documentos", campos, nombre, contenido)
	if estado != http.StatusUnprocessableEntity || codigoDe(respuesta) != "validacion" {
		c.t.Fatalf("la subida de %s debió rechazarse con 422: %d %v", nombre, estado, respuesta)
	}
	problema := respuesta["error"].(obj)["campos"].([]any)[0].(obj)
	return fmt.Sprintf("%s: %s", problema["campo"], problema["mensaje"])
}

// bajar hace una petición GET y devuelve la respuesta completa.
func (c *cliente) bajar(ruta string, cabeceras ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	r := httptest.NewRequest("GET", ruta, nil)
	for i := 0; i+1 < len(cabeceras); i += 2 {
		r.Header.Set(cabeceras[i], cabeceras[i+1])
	}
	return c.hacer(r)
}

// postulanteVerificado registra una cuenta por el formulario público y
// confirma su correo con el enlace que recibe: sólo así puede enviar.
func (e *entornoPrueba) postulanteVerificado(email string) *cliente {
	e.t.Helper()
	c := e.cliente()
	redirige(e.t, c.post("/cuenta/registro", url.Values{
		"email": {email}, "nombre": {"Ana"}, "apellidos": {"López"}, "password": {passwordPrueba}, "idioma": {"es"}, "acepta_aviso": {"1"},
	}), "/cuenta?aviso=registro")
	token := e.tokenDe(email, models.PlantillaVerificarEmail, "/cuenta/verificar")
	redirige(e.t, c.post("/cuenta/verificar", url.Values{"token": {token}}), "/cuenta?aviso=verificado")
	return c
}

// solistaCompleto crea una postulación de solista y captura todo lo que el
// envío exige, documentos incluidos. sello distingue los archivos de cada
// postulante. Devuelve la ruta de la postulación y el id de su obra.
func (e *entornoPrueba) solistaCompleto(c *cliente, convocatoriaID int64, sello string) (ruta string, obraID int64) {
	e.t.Helper()
	p := c.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", convocatoriaID), obj{"subcategoria": "solista"})
	ruta = fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	guardar(e.t, c, ruta, datosDeSolista())
	c.exige(http.StatusOK, "PUT", ruta+"/videos", obj{"urls": []string{"https://vimeo.com/123", "https://ejemplo.mx/video"}})
	p = c.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	obraID = id(lista(p, "obras")[0]["id"])
	guardar(e.t, c, fmt.Sprintf("/api/v1/obras/%d", obraID), obj{
		"obra.titulo": "Preludio a Colón", "obra.compositor": "Julián Carrillo", "obra.duracion_min": 12, "obra.estreno": "no_es_estreno",
		"obra.semblanza_compositor": "Semblanza.", "obra.nota_obra": "Nota.", "obra.liga_media": "https://ejemplo.mx/obra",
	})
	c.documento(ruta, "fotografia", 0, "retrato.jpg", append(contenidoJPEG, sello...))
	c.documento(ruta, "partitura", obraID, "partitura.pdf", pdfDePrueba("partitura "+sello))
	c.documento(ruta, "autorizacion", obraID, "autorizacion.pdf", pdfDePrueba("autorización "+sello))
	return ruta, obraID
}

// Subida de documentos: formato por firma, tamaño, versiones y descargas.
func TestSubidaYDescargaDeDocumentos(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulante("ana@ejemplo.mx")
	p := ana.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "solista"})
	postulacionID := id(p["id"])
	ruta := fmt.Sprintf("/api/v1/postulaciones/%d", postulacionID)

	// La postulación dice qué documentos se piden: a ella, la fotografía; a
	// cada obra, partitura y autorización.
	requisitos := lista(p, "requisitos")
	if len(requisitos) != 1 || requisitos[0]["clave"] != "fotografia" || requisitos[0]["obra_id"] != nil || requisitos[0]["obligatorio"] != true ||
		fmt.Sprint(requisitos[0]["mimes_permitidos"]) != "[image/jpeg image/png]" || len(lista(p, "documentos")) != 0 {
		t.Errorf("requisitos de una postulación sin obras: %v", requisitos)
	}
	p = ana.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	obraID := id(lista(p, "obras")[0]["id"])
	if requisitos = lista(p, "requisitos"); len(requisitos) != 3 || requisitos[1]["clave"] != "partitura" || id(requisitos[1]["obra_id"]) != obraID || requisitos[2]["clave"] != "autorizacion" {
		t.Errorf("requisitos con una obra: %v", requisitos)
	}

	// Peticiones mal formadas.
	if estado, r := ana.subir("POST", ruta+"/documentos", [][2]string{{"clave", "fotografia"}}, "", nil); estado != http.StatusBadRequest {
		t.Errorf("subida sin archivo: %d %v", estado, r)
	}
	if estado, r := ana.api("POST", ruta+"/documentos", obj{"clave": "fotografia"}); estado != http.StatusBadRequest {
		t.Errorf("subida que no es multipart: %d %v", estado, r)
	}
	if estado, r := ana.subir("POST", ruta+"/documentos", [][2]string{{"clave", "partitura"}, {"obra_id", "dos"}}, "p.pdf", pdfDePrueba("x")); estado != http.StatusBadRequest {
		t.Errorf("obra_id que no es número: %d %v", estado, r)
	}
	// Requisito u obra que no corresponden.
	for nombre, caso := range map[string]struct {
		campos   [][2]string
		esperado string
	}{
		"sin clave":                      {nil, "clave: "},
		"requisito inexistente":          {[][2]string{{"clave", "curriculum"}}, "clave: "},
		"la partitura es de una obra":    {[][2]string{{"clave", "partitura"}}, "clave: "},
		"la fotografía no es de la obra": {[][2]string{{"clave", "fotografia"}, {"obra_id", fmt.Sprint(obraID)}}, "clave: "},
		"obra de otra postulación":       {[][2]string{{"clave", "partitura"}, {"obra_id", "9999"}}, "obra_id: "},
	} {
		if problema := ana.rechazoDeSubida(ruta, caso.campos, "archivo.pdf", pdfDePrueba("x")); !strings.HasPrefix(problema, caso.esperado) {
			t.Errorf("%s: %s", nombre, problema)
		}
	}

	// El formato se reconoce por el contenido, no por el nombre (RF-DOC-01).
	foto := [][2]string{{"clave", "fotografia"}}
	grande := append(bytes.Clone(contenidoJPEG), make([]byte, 5<<20)...) // un byte más que el máximo de 5 MB… y otros tantos
	for nombre, caso := range map[string]struct {
		archivo   string
		contenido []byte
		esperado  string
	}{
		"texto con nombre de imagen": {"retrato.jpg", []byte("esto no es una imagen"), "No se reconoce el formato"},
		"ejecutable disfrazado":      {"retrato.png", []byte("MZ\x90\x00\x03"), "No se reconoce el formato"},
		"PDF donde va una imagen":    {"retrato.pdf", pdfDePrueba("x"), "no es de los admitidos"},
		"PNG con extensión de JPEG":  {"retrato.jpg", contenidoPNG, "extensión"},
		"archivo vacío":              {"retrato.jpg", nil, "vacío"},
		"mayor que el máximo":        {"retrato.jpg", grande, "tamaño máximo"},
	} {
		if problema := ana.rechazoDeSubida(ruta, foto, caso.archivo, caso.contenido); !strings.HasPrefix(problema, "archivo: ") || !strings.Contains(problema, caso.esperado) {
			t.Errorf("%s: %s", nombre, problema)
		}
	}
	if p = ana.exige(http.StatusOK, "GET", ruta, nil); len(lista(p, "documentos")) != 0 {
		t.Errorf("una subida rechazada no debe dejar documento: %v", p["documentos"])
	}

	// Primera fotografía.
	p = ana.documento(ruta, "fotografia", 0, "Retrato de Ana.JPG", contenidoJPEG)
	documentos := lista(p, "documentos")
	if len(documentos) != 1 {
		t.Fatalf("documentos tras la primera subida: %v", documentos)
	}
	primera := documentos[0]
	if primera["clave"] != "fotografia" || primera["obra_id"] != nil || id(primera["version"]) != 1 || primera["dictamen"] != "pendiente" ||
		primera["mime"] != "image/jpeg" || id(primera["tamano_bytes"]) != int64(len(contenidoJPEG)) || primera["sha256"] != sumaDe(contenidoJPEG) ||
		primera["nombre"] != fmt.Sprintf("borrador-%d_fotografia_v1.jpg", postulacionID) {
		t.Errorf("documento registrado: %v", primera)
	}
	if strings.Contains(faltantes(t, p), "documento.fotografia") {
		t.Errorf("con la fotografía subida ya no debe faltar: %s", faltantes(t, p))
	}

	// Reemplazo: el requisito admite un archivo, así que el nuevo sustituye al
	// anterior. Nada se sobrescribe (RF-DOC-07): la versión 1 se conserva.
	p = ana.documento(ruta, "fotografia", 0, "retrato.png", contenidoPNG)
	documentos = lista(p, "documentos")
	if len(documentos) != 1 || id(documentos[0]["version"]) != 2 || documentos[0]["mime"] != "image/png" || documentos[0]["sha256"] != sumaDe(contenidoPNG) {
		t.Errorf("tras reemplazar debe quedar vigente sólo la versión 2: %v", documentos)
	}
	segunda := documentos[0]
	anterior := ana.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", id(primera["id"])))
	if anterior.Code != http.StatusOK || !bytes.Equal(anterior.Body.Bytes(), contenidoJPEG) {
		t.Errorf("la versión reemplazada debe seguir descargable: %d", anterior.Code)
	}
	// El mismo contenido otra vez: versión 3, misma suma (deduplicación).
	p = ana.documento(ruta, "fotografia", 0, "otra.png", contenidoPNG)
	if d := lista(p, "documentos")[0]; id(d["version"]) != 3 || d["sha256"] != sumaDe(contenidoPNG) {
		t.Errorf("volver a subir el mismo contenido: %v", d)
	}
	tercera := lista(p, "documentos")[0]

	// Documentos de la obra.
	p = ana.documento(ruta, "partitura", obraID, "partitura.pdf", pdfDePrueba("partitura"))
	p = ana.documento(ruta, "autorizacion", obraID, "autorizacion.pdf", pdfDePrueba("autorización"))
	if documentos = lista(p, "documentos"); len(documentos) != 3 || id(documentos[1]["obra_id"]) != obraID ||
		documentos[1]["nombre"] != fmt.Sprintf("borrador-%d_partitura_obra1_v1.pdf", postulacionID) {
		t.Errorf("documentos de la obra: %v", documentos)
	}
	partituraID := id(documentos[1]["id"])

	// Descarga: con el tipo detectado, nombre predecible y sin caché.
	rutaArchivo := fmt.Sprintf("/api/v1/documentos/%d/archivo", id(tercera["id"]))
	descarga := ana.bajar(rutaArchivo)
	if descarga.Code != http.StatusOK || !bytes.Equal(descarga.Body.Bytes(), contenidoPNG) ||
		descarga.Header().Get("Content-Type") != "image/png" ||
		descarga.Header().Get("Content-Disposition") != fmt.Sprintf(`inline; filename="borrador-%d_fotografia_v3.png"`, postulacionID) ||
		descarga.Header().Get("Cache-Control") != "private, no-store" || descarga.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("descarga inesperada: %d %v", descarga.Code, descarga.Header())
	}
	if d := ana.bajar(rutaArchivo + "?descargar=1"); !strings.HasPrefix(d.Header().Get("Content-Disposition"), "attachment;") {
		t.Errorf("con ?descargar=1 debe ofrecerse como descarga: %v", d.Header())
	}
	// Por rangos, como lo pide un reproductor: trozo intermedio, que no cuenta
	// como otra descarga.
	if d := ana.bajar(rutaArchivo, "Range", "bytes=1-3"); d.Code != http.StatusPartialContent || d.Body.String() != "PNG" {
		t.Errorf("descarga por rango: %d %q", d.Code, d.Body.String())
	}
	// Permisos: lo ajeno no existe; el administrador sí puede descargar.
	luis := e.postulante("luis@ejemplo.mx")
	if d := luis.bajar(rutaArchivo); d.Code != http.StatusNotFound {
		t.Errorf("descarga desde otra cuenta: %d", d.Code)
	}
	if d := e.cliente().bajar(rutaArchivo); d.Code != http.StatusUnauthorized {
		t.Errorf("descarga sin sesión: %d", d.Code)
	}
	if d := admin.bajar(rutaArchivo); d.Code != http.StatusOK || !bytes.Equal(d.Body.Bytes(), contenidoPNG) {
		t.Errorf("descarga del administrador: %d", d.Code)
	}
	if d := ana.bajar("/api/v1/documentos/9999/archivo"); d.Code != http.StatusNotFound {
		t.Errorf("documento inexistente: %d", d.Code)
	}
	// Toda descarga queda registrada (RN-27): tres de Ana y una del administrador.
	descargas := e.asientos("documento.descargar")
	if len(descargas) != 4 {
		t.Errorf("descargas registradas: %d, se esperaban 4", len(descargas))
	}
	// La bitácora de la subida lleva la suma, no el nombre del archivo.
	subidas := e.asientos("documento.subir")
	if len(subidas) != 5 || !strings.Contains(fmt.Sprint(subidas[0].Despues), sumaDe(pdfDePrueba("autorización"))) {
		t.Errorf("asientos de subida: %d", len(subidas))
	}
	for _, asiento := range subidas {
		if strings.Contains(fmt.Sprint(asiento.Despues), "Retrato de Ana") {
			t.Error("la bitácora no debe guardar el nombre original del archivo")
		}
	}

	// Subir o quitar desde otra cuenta: 404.
	if estado, r := luis.subir("POST", ruta+"/documentos", foto, "retrato.jpg", contenidoJPEG); estado != http.StatusNotFound {
		t.Errorf("subida desde otra cuenta: %d %v", estado, r)
	}
	if estado, _ := luis.api("DELETE", fmt.Sprintf("/api/v1/documentos/%d", partituraID), nil); estado != http.StatusNotFound {
		t.Errorf("quitar un documento ajeno: %d", estado)
	}
	// Quitar: deja de estar vigente y vuelve a faltar; el archivo sigue ahí.
	p = ana.exige(http.StatusOK, "DELETE", fmt.Sprintf("/api/v1/documentos/%d", partituraID), nil)
	if len(lista(p, "documentos")) != 2 || !strings.Contains(faltantes(t, p), fmt.Sprintf("documento.partitura#%d", obraID)) {
		t.Errorf("tras quitar la partitura: %v / %s", p["documentos"], faltantes(t, p))
	}
	if estado, _ := ana.api("DELETE", fmt.Sprintf("/api/v1/documentos/%d", partituraID), nil); estado != http.StatusNotFound {
		t.Errorf("quitar dos veces: %d", estado)
	}
	if estado, _ := ana.api("DELETE", fmt.Sprintf("/api/v1/documentos/%d", id(segunda["id"])), nil); estado != http.StatusNotFound {
		t.Errorf("quitar una versión que ya no es vigente: %d", estado)
	}
	if d := ana.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", partituraID)); d.Code != http.StatusOK {
		t.Errorf("un documento retirado sigue descargable por su titular: %d", d.Code)
	}

	// Al quitar la obra se van sus documentos, pero no el registro de sus descargas.
	ana.exige(http.StatusOK, "DELETE", fmt.Sprintf("/api/v1/obras/%d", obraID), nil)
	if d := ana.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", partituraID)); d.Code != http.StatusNotFound {
		t.Errorf("el documento de una obra borrada ya no existe: %d", d.Code)
	}
	if len(e.asientos("documento.descargar")) != 5 {
		t.Error("el registro de descargas debe conservarse al borrar la obra")
	}

	// Fuera de plazo ya no se sube ni se quita.
	e.hasta("2026-12-20T06:00:00Z")
	ana.ingresar("ana@ejemplo.mx", passwordPrueba)
	if problema := ana.rechazoDeSubida(ruta, foto, "retrato.jpg", contenidoJPEG); !strings.HasPrefix(problema, "convocatoria: ") {
		t.Errorf("subida fuera de plazo: %s", problema)
	}
}

// El envío: irreversible, con folio, snapshot, acuse en PDF y correo.
func TestEnvioYAcuse(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulanteVerificado("ana@ejemplo.mx")
	si := obj{"confirmo": true, "acepto_bases": true}

	// A una postulación incompleta le responde con lo que falta, sin enviar.
	p := ana.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "solista"})
	rutaIncompleta := fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	estado, r := ana.api("POST", rutaIncompleta+"/envio", si)
	if estado != http.StatusUnprocessableEntity || codigoDe(r) != "incompleta" || len(r["error"].(obj)["bloqueadores"].([]any)) < 10 {
		t.Fatalf("envío de una postulación vacía: %d %v", estado, r)
	}
	if p = ana.exige(http.StatusOK, "GET", rutaIncompleta, nil); p["estado"] != "borrador" || p["folio"] != nil {
		t.Errorf("un envío rechazado no debe cambiar nada: %v %v", p["estado"], p["folio"])
	}

	// Luis captura todo; Ana no puede enviar la postulación de Luis.
	luis := e.postulanteVerificado("luis@ejemplo.mx")
	ruta, obraID := e.solistaCompleto(luis, interpretes, "luis")
	if p = luis.exige(http.StatusOK, "GET", ruta, nil); p["lista_para_enviar"] != true || len(lista(p, "bloqueadores")) != 0 {
		t.Fatalf("la postulación completa debe estar lista: %s", faltantes(t, p))
	}
	if estado, _ := ana.api("POST", ruta+"/envio", si); estado != http.StatusNotFound {
		t.Errorf("enviar una postulación ajena: %d", estado)
	}
	// Sin las dos confirmaciones no se envía (RF-POST-10).
	if campos := luis.rechaza("POST", ruta+"/envio", obj{}); fmt.Sprint(campos) != "[confirmo acepto_bases]" {
		t.Errorf("envío sin confirmar: %v", campos)
	}
	if campos := luis.rechaza("POST", ruta+"/envio", obj{"confirmo": true}); fmt.Sprint(campos) != "[acepto_bases]" {
		t.Errorf("envío sin aceptar las bases: %v", campos)
	}

	// Una liga que la plataforma declara inexistente bloquea el envío (RN-08)…
	e.red.estados["vimeo.com/api/oembed"] = http.StatusNotFound
	estado, r = luis.api("POST", ruta+"/envio", si)
	bloqueos := fmt.Sprint(r["error"])
	if estado != http.StatusUnprocessableEntity || codigoDe(r) != "incompleta" || !strings.Contains(bloqueos, "campo:videos") {
		t.Fatalf("envío con una liga rota: %d %v", estado, r)
	}
	p = luis.exige(http.StatusOK, "GET", ruta, nil)
	if videos := lista(p, "videos"); videos[0]["estado"] != "inaccesible" || videos[0]["detalle"] != "El recurso no existe o fue retirado." ||
		videos[1]["estado"] != "ok" || videos[1]["estado_texto"] != "Comprobada: responde públicamente" {
		t.Errorf("estado de las ligas tras comprobarlas: %v", videos)
	}
	// …y una que simplemente no pudo comprobarse, no.
	e.red.estados["vimeo.com/api/oembed"] = http.StatusServiceUnavailable

	// Envío.
	e.reloj.Avanzar(time.Hour)
	p = luis.exige(http.StatusOK, "POST", ruta+"/envio", si)
	if p["estado"] != "enviada" || p["folio"] != "FIM50-INT-0001" || id(p["envios"]) != 1 || p["editable"] != false {
		t.Fatalf("postulación enviada: %v %v %v %v", p["estado"], p["folio"], p["envios"], p["editable"])
	}
	if videos := lista(p, "videos"); videos[0]["estado"] != "pendiente" {
		t.Errorf("la liga no comprobable queda pendiente, para el revisor: %v", videos[0])
	}
	acuses := lista(p, "acuses")
	if len(acuses) != 1 || id(acuses[0]["version"]) != 1 || acuses[0]["idioma"] != "es" || acuses[0]["generado_at"] != "2026-11-25T19:00:00Z" {
		t.Fatalf("acuses: %v", acuses)
	}
	// Los documentos toman el folio en su nombre.
	if d := lista(p, "documentos")[1]; d["nombre"] != "FIM50-INT-0001_partitura_obra1_v1.pdf" {
		t.Errorf("nombre de descarga tras el envío: %v", d["nombre"])
	}

	// Correo de acuse, con el PDF adjunto, en la bandeja de salida.
	var correo models.CorreoSaliente
	for _, c := range e.correos() {
		if c.PlantillaClave == models.PlantillaAcuse {
			correo = c
		}
	}
	if correo.Destinatario != "luis@ejemplo.mx" || correo.Asunto != "Acuse de recibo FIM50-INT-0001 · FIMNME" ||
		!strings.Contains(correo.Texto, "**FIM50-INT-0001**") || !strings.Contains(correo.Texto, "Convocatoria de Intérpretes 2028") ||
		!strings.Contains(correo.Texto, urlBasePrueba+"/postulaciones/FIM50-INT-0001") || strings.Contains(correo.Texto, "{{") ||
		len(correo.Adjuntos) != 1 || correo.Adjuntos[0].Nombre != "acuse_FIM50-INT-0001_v1.pdf" || correo.PostulacionID != id(p["id"]) {
		t.Errorf("correo de acuse inesperado: %+v", correo)
	}
	// El transporte de desarrollo nombra el adjunto en el log.
	var registro bytes.Buffer
	if _, err := (MailerRegistro{Log: slog.New(slog.NewTextHandler(&registro, nil))}).Enviar(context.Background(), correo); err != nil ||
		!strings.Contains(registro.String(), "adjunto=acuse_FIM50-INT-0001_v1.pdf") {
		t.Errorf("el log del correo debe nombrar el adjunto: %s", registro.String())
	}

	// El acuse se descarga: es un PDF pequeño y su suma es la registrada.
	acuse := luis.bajar(ruta + "/acuses/1")
	if acuse.Code != http.StatusOK || !bytes.HasPrefix(acuse.Body.Bytes(), []byte("%PDF-1.4")) || acuse.Body.Len() > 100<<10 ||
		acuse.Header().Get("Content-Type") != "application/pdf" ||
		acuse.Header().Get("Content-Disposition") != `inline; filename="acuse_FIM50-INT-0001_v1.pdf"` ||
		sumaDe(acuse.Body.Bytes()) != acuses[0]["sha256"] {
		t.Errorf("descarga del acuse: %d, %d bytes, %v", acuse.Code, acuse.Body.Len(), acuse.Header())
	}
	for nombre, caso := range map[string]struct {
		c      *cliente
		ruta   string
		estado int
	}{
		"acuse de otra cuenta":     {ana, ruta + "/acuses/1", http.StatusNotFound},
		"versión inexistente":      {luis, ruta + "/acuses/2", http.StatusNotFound},
		"versión que no es número": {luis, ruta + "/acuses/uno", http.StatusNotFound},
		"el administrador":         {admin, ruta + "/acuses/1", http.StatusOK},
	} {
		if d := caso.c.bajar(caso.ruta); d.Code != caso.estado {
			t.Errorf("%s: %d", nombre, d.Code)
		}
	}
	if n := len(e.asientos("acuse.descargar")); n != 2 {
		t.Errorf("descargas de acuse registradas: %d", n)
	}

	// Enviada, la postulación queda congelada (RN-04).
	for nombre, intento := range map[string]func() []string{
		"campo": func() []string { return luis.rechaza("PATCH", ruta, obj{"campos": obj{"datos.telefono": "1"}}) },
		"obra":  func() []string { return luis.rechaza("POST", ruta+"/obras", nil) },
		"campo de obra": func() []string {
			return luis.rechaza("PATCH", fmt.Sprintf("/api/v1/obras/%d", obraID), obj{"campos": obj{"obra.titulo": "Otra"}})
		},
		"videos":  func() []string { return luis.rechaza("PUT", ruta+"/videos", obj{"urls": []string{"https://a.mx/1"}}) },
		"reenvío": func() []string { return luis.rechaza("POST", ruta+"/envio", si) },
		"quitar documento": func() []string {
			return luis.rechaza("DELETE", fmt.Sprintf("/api/v1/documentos/%d", id(lista(p, "documentos")[0]["id"])), nil)
		},
	} {
		if campos := intento(); fmt.Sprint(campos) != "[estado]" {
			t.Errorf("%s tras el envío: %v", nombre, campos)
		}
	}
	if problema := luis.rechazoDeSubida(ruta, [][2]string{{"clave", "fotografia"}}, "retrato.jpg", contenidoJPEG); !strings.HasPrefix(problema, "estado: ") {
		t.Errorf("subida tras el envío: %s", problema)
	}

	// Bitácora: un solo envío, con folio y la suma del acuse.
	envios := e.asientos("postulacion.enviar")
	if len(envios) != 1 || !strings.Contains(fmt.Sprint(envios[0].Despues), "FIM50-INT-0001") ||
		!strings.Contains(fmt.Sprint(envios[0].Despues), fmt.Sprint(acuses[0]["sha256"])) {
		t.Errorf("asiento del envío: %+v", envios)
	}

	// El folio es consecutivo por convocatoria: el siguiente envío lleva el 0002.
	e.red.estados = map[string]int{}
	guardar(t, ana, rutaIncompleta, datosDeSolista())
	ana.exige(http.StatusOK, "PUT", rutaIncompleta+"/videos", obj{"urls": []string{"https://youtu.be/abc"}})
	p = ana.exige(http.StatusCreated, "POST", rutaIncompleta+"/obras", nil)
	obraDeAna := id(lista(p, "obras")[0]["id"])
	guardar(t, ana, fmt.Sprintf("/api/v1/obras/%d", obraDeAna), obj{
		"obra.titulo": "Obra", "obra.compositor": "Compositora", "obra.duracion_min": 8, "obra.estreno": "estreno_mexico",
		"obra.semblanza_compositor": "Semblanza.", "obra.nota_obra": "Nota.", "obra.liga_media": "https://ejemplo.mx/o",
	})
	ana.documento(rutaIncompleta, "fotografia", 0, "retrato.jpg", contenidoJPEG)
	// La misma partitura que subió Luis: mismo contenido, un solo archivo en
	// el almacén, y cada quien descarga la suya con su propio nombre.
	ana.documento(rutaIncompleta, "partitura", obraDeAna, "copia.pdf", pdfDePrueba("partitura luis"))
	ana.documento(rutaIncompleta, "autorizacion", obraDeAna, "autorizacion.pdf", pdfDePrueba("autorización ana"))
	p = ana.exige(http.StatusOK, "POST", rutaIncompleta+"/envio", si)
	if p["folio"] != "FIM50-INT-0002" || p["estado"] != "enviada" {
		t.Errorf("segundo envío: %v %v", p["folio"], p["estado"])
	}
	if d := lista(p, "documentos")[1]; d["sha256"] != sumaDe(pdfDePrueba("partitura luis")) || d["nombre"] != "FIM50-INT-0002_partitura_obra1_v1.pdf" {
		t.Errorf("documento con contenido repetido: %v", d)
	}

	// Vencido el plazo no se envía, por completa que esté.
	eva := e.postulanteVerificado("eva@ejemplo.mx")
	rutaDeEva, _ := e.solistaCompleto(eva, interpretes, "eva")
	e.hasta("2026-12-20T06:00:00Z")
	eva.ingresar("eva@ejemplo.mx", passwordPrueba)
	if campos := eva.rechaza("POST", rutaDeEva+"/envio", si); fmt.Sprint(campos) != "[convocatoria]" {
		t.Errorf("envío fuera de plazo: %v", campos)
	}
}

// Una cuenta sin el correo confirmado captura todo, pero no envía.
func TestEnvioExigeCorreoVerificado(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulante("ana@ejemplo.mx")
	ruta, _ := e.solistaCompleto(ana, interpretes, "ana")
	estado, r := ana.api("POST", ruta+"/envio", obj{"confirmo": true, "acepto_bases": true})
	if estado != http.StatusUnprocessableEntity || codigoDe(r) != "incompleta" || !strings.Contains(fmt.Sprint(r["error"]), "campo:cuenta.email") ||
		len(r["error"].(obj)["bloqueadores"].([]any)) != 1 {
		t.Errorf("envío sin correo verificado: %d %v", estado, r)
	}
}

// En Compositores los documentos de cada obra dependen de su dotación.
func TestDocumentosPorDotacion(t *testing.T) {
	e := nuevoEntorno(t)
	_, _, compositores, cuarteto, electro := e.convocatoriasAbiertas()
	eva := e.postulanteVerificado("eva@ejemplo.mx")
	p := eva.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", compositores), obj{})
	ruta := fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	datos := datosDeSolista()
	delete(datos, "concierto.requerimientos_tecnicos")
	delete(datos, "concierto.entidad")
	guardar(t, eva, ruta, datos)
	eva.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	p = eva.exige(http.StatusCreated, "POST", ruta+"/obras", nil)
	primera, segunda := id(lista(p, "obras")[0]["id"]), id(lista(p, "obras")[1]["id"])
	for obraID, dotacion := range map[int64]int64{primera: cuarteto, segunda: electro} {
		guardar(t, eva, fmt.Sprintf("/api/v1/obras/%d", obraID), obj{"obra.titulo": "Obra", "obra.duracion_min": 9, "obra.estreno": "estreno_mundial",
			"obra.dotacion_id": dotacion, "obra.nota_obra": "Nota.", "obra.requerimientos_tecnicos": "Ninguno."})
	}

	// La lista de requisitos de cada obra refleja su dotación.
	p = eva.exige(http.StatusOK, "GET", ruta, nil)
	obligatorio := map[string]any{}
	for _, rq := range lista(p, "requisitos") {
		obra := int64(0)
		if rq["obra_id"] != nil {
			obra = id(rq["obra_id"])
		}
		obligatorio[fmt.Sprintf("%s#%d", rq["clave"], obra)] = rq["obligatorio"]
	}
	esperado := map[string]any{
		"fotografia#0":                       true,
		fmt.Sprintf("partitura#%d", primera): true, fmt.Sprintf("particellas#%d", primera): true, fmt.Sprintf("audio#%d", primera): false,
		fmt.Sprintf("partitura#%d", segunda): true, fmt.Sprintf("particellas#%d", segunda): false, fmt.Sprintf("audio#%d", segunda): true,
	}
	if fmt.Sprint(obligatorio) != fmt.Sprint(esperado) {
		t.Errorf("requisitos por dotación:\n  %v\nse esperaba:\n  %v", obligatorio, esperado)
	}

	eva.documento(ruta, "fotografia", 0, "retrato.png", contenidoPNG)
	eva.documento(ruta, "partitura", primera, "cuarteto.pdf", pdfDePrueba("cuarteto"))
	eva.documento(ruta, "particellas", primera, "partes.pdf", pdfDePrueba("partes"))
	eva.documento(ruta, "partitura", segunda, "electro.pdf", pdfDePrueba("electro"))
	// El audio de la electroacústica sólo admite WAV: así lo configuró el administrador.
	if problema := eva.rechazoDeSubida(ruta, [][2]string{{"clave", "audio"}, {"obra_id", fmt.Sprint(segunda)}}, "pista.mp3", []byte("ID3\x04\x00 audio")); !strings.Contains(problema, "no es de los admitidos") {
		t.Errorf("MP3 donde sólo se admite WAV: %s", problema)
	}
	// Para el cuarteto, en cambio, el audio es opcional y admite MP3.
	eva.documento(ruta, "audio", primera, "maqueta.mp3", []byte("ID3\x04\x00 audio"))
	if p = eva.exige(http.StatusOK, "GET", ruta, nil); faltantes(t, p) != fmt.Sprintf("documentos/documento.audio#%d", segunda) {
		t.Errorf("sólo debe faltar el audio de la electroacústica: %s", faltantes(t, p))
	}
	p = eva.documento(ruta, "audio", segunda, "pista.wav", contenidoWAV)
	if p["lista_para_enviar"] != true {
		t.Fatalf("la postulación debe estar lista: %s", faltantes(t, p))
	}
	p = eva.exige(http.StatusOK, "POST", ruta+"/envio", obj{"confirmo": true, "acepto_bases": true})
	if p["folio"] != "FIM50-COM-0001" || len(lista(p, "documentos")) != 6 {
		t.Errorf("envío de Compositores: %v, %d documentos", p["folio"], len(lista(p, "documentos")))
	}
	// Un audio se sirve por rangos, para el reproductor.
	var audioID int64
	for _, d := range lista(p, "documentos") {
		if d["clave"] == "audio" && id(d["obra_id"]) == segunda {
			audioID = id(d["id"])
		}
	}
	if d := eva.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", audioID), "Range", "bytes=0-3"); d.Code != http.StatusPartialContent ||
		d.Body.String() != "RIFF" || d.Header().Get("Content-Type") != "audio/wav" || d.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("audio por rangos: %d %q %v", d.Code, d.Body.String(), d.Header())
	}
}

// Las ligas se comprueban en segundo plano y se revalidan cada día.
func TestTrabajadorDeEnlaces(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ana := e.postulante("ana@ejemplo.mx")
	p := ana.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "solista"})
	ruta := fmt.Sprintf("/api/v1/postulaciones/%d", id(p["id"]))
	ana.exige(http.StatusOK, "PUT", ruta+"/videos", obj{"urls": []string{
		"https://www.youtube.com/watch?v=privado", "https://ejemplo.mx/retirado", "https://drive.google.com/file/d/1/view"}})

	e.red.estados = map[string]int{"youtube.com/oembed": http.StatusForbidden, "ejemplo.mx/retirado": http.StatusGone}
	trabajador := &TrabajadorEnlaces{Datos: e.repo, Verificador: &VerificadorDeEnlaces{Cliente: &http.Client{Transport: e.red}},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Ahora: e.reloj.Ahora}
	if n := trabajador.Procesar(context.Background()); n != 3 {
		t.Fatalf("ligas comprobadas: %d", n)
	}
	// A YouTube se le pregunta por oEmbed; a los demás, por el recurso.
	if pedidas := strings.Join(e.red.pedidas, " "); !strings.Contains(pedidas, "https://www.youtube.com/oembed?format=json&url=") ||
		!strings.Contains(pedidas, "https://ejemplo.mx/retirado") || !strings.Contains(pedidas, "https://drive.google.com/file/d/1/view") {
		t.Errorf("peticiones del verificador: %v", e.red.pedidas)
	}
	videos := lista(ana.exige(http.StatusOK, "GET", ruta, nil), "videos")
	if videos[0]["estado"] != "no_publico" || videos[1]["estado"] != "inaccesible" || videos[2]["estado"] != "ok" ||
		videos[0]["detalle"] != "El recurso pide iniciar sesión o es privado." {
		t.Errorf("estado de las ligas: %v", videos)
	}
	// Ya comprobadas, no se repiten… hasta que pasa un día.
	if n := trabajador.Procesar(context.Background()); n != 0 {
		t.Errorf("no debe repetirse la comprobación: %d", n)
	}
	// Cambiar una liga conserva el resultado de las que no cambian.
	ana.exige(http.StatusOK, "PUT", ruta+"/videos", obj{"urls": []string{"https://ejemplo.mx/retirado", "https://vimeo.com/nuevo"}})
	videos = lista(ana.exige(http.StatusOK, "GET", ruta, nil), "videos")
	if videos[0]["estado"] != "inaccesible" || videos[1]["estado"] != "pendiente" {
		t.Errorf("al sustituir la lista: %v", videos)
	}
	if n := trabajador.Procesar(context.Background()); n != 1 {
		t.Errorf("sólo la liga nueva está por comprobar: %d", n)
	}
	e.reloj.Avanzar(25 * time.Hour)
	e.red.estados = map[string]int{}
	if n := trabajador.Procesar(context.Background()); n != 2 {
		t.Errorf("al día siguiente se revalidan todas: %d", n)
	}
	ana.ingresar("ana@ejemplo.mx", passwordPrueba)
	if videos = lista(ana.exige(http.StatusOK, "GET", ruta, nil), "videos"); videos[0]["estado"] != "ok" {
		t.Errorf("la liga que volvió a funcionar: %v", videos[0])
	}
	// Si la red falla, la liga no se da por mala: queda pendiente.
	e.reloj.Avanzar(25 * time.Hour)
	e.red.fallaCon = errors.New("sin conexión")
	trabajador.Procesar(context.Background())
	ana.ingresar("ana@ejemplo.mx", passwordPrueba)
	if videos = lista(ana.exige(http.StatusOK, "GET", ruta, nil), "videos"); videos[0]["estado"] != "pendiente" ||
		videos[0]["detalle"] != "No se pudo comprobar automáticamente; el revisor la abrirá." {
		t.Errorf("una liga que no pudo comprobarse: %v", videos[0])
	}
}

// El verificador real no se conecta a direcciones que no sean públicas.
func TestVerificadorRechazaDireccionesInternas(t *testing.T) {
	for direccion, permitida := range map[string]bool{
		"93.184.216.34:443": true, "[2606:2800:220:1:248:1893:25c8:1946]:443": true,
		"93.184.216.34:80": false, "93.184.216.34:8443": false, // sólo el puerto 443
		"127.0.0.1:443": false, "10.0.0.5:443": false, "192.168.1.10:443": false, "169.254.169.254:443": false,
		"[::1]:443": false, "[fd00::1]:443": false, "[::ffff:127.0.0.1]:443": false, "no-es-una-direccion": false,
	} {
		if err := soloDireccionesPublicas("tcp", direccion, nil); (err == nil) != permitida {
			t.Errorf("conexión a %s: permitida=%v, se esperaba %v", direccion, err == nil, permitida)
		}
	}
	// Con un servidor de verdad en esta misma máquina: el cliente se niega a
	// conectar, también cuando llega ahí por una redirección.
	local := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer local.Close()
	verificador := NuevoVerificadorDeEnlaces()
	if _, err := verificador.Cliente.Get(local.URL); !errors.Is(err, errDireccionNoPermitida) {
		t.Errorf("la conexión a %s debió rechazarse: %v", local.URL, err)
	}
	// El filtro de redirecciones: sólo https, pocas, y nunca a una pantalla de ingreso.
	redireccion := func(destino string, anteriores int) error {
		siguiente, _ := http.NewRequest("GET", destino, nil)
		return verificador.Cliente.CheckRedirect(siguiente, make([]*http.Request, anteriores))
	}
	if redireccion("https://ejemplo.mx/b", 1) != nil || !errors.Is(redireccion("http://ejemplo.mx/b", 1), errRedireccion) ||
		!errors.Is(redireccion("https://ejemplo.mx/b", maxRedirecciones), errRedireccion) ||
		!errors.Is(redireccion("https://accounts.google.com/ServiceLogin", 1), errPideIngreso) {
		t.Error("filtro de redirecciones incorrecto")
	}
	// «El dominio no existe» sólo se cree si el servidor tiene DNS: si no lo
	// tiene, diría lo mismo de todas las ligas e impediría todos los envíos.
	sinDominio := &redDePrueba{fallaCon: &net.DNSError{Err: "no such host", Name: "ejemplo.mx", IsNotFound: true}}
	conDNS := &VerificadorDeEnlaces{Cliente: &http.Client{Transport: sinDominio}, HayDNS: func(context.Context) bool { return true }}
	if r := conDNS.Verificar(context.Background(), "https://ejemplo.mx/video"); r.Estado != models.EnlaceInaccesible || r.Detalle != models.DetalleEnlaceDominio {
		t.Errorf("dominio inexistente con DNS en buen estado: %+v", r)
	}
	sinDNS := &VerificadorDeEnlaces{Cliente: &http.Client{Transport: sinDominio}, HayDNS: func(context.Context) bool { return false }}
	if r := sinDNS.Verificar(context.Background(), "https://ejemplo.mx/video"); r.Estado != models.EnlacePendiente || r.Detalle != models.DetalleEnlaceNoComprobable {
		t.Errorf("sin DNS en el servidor, la liga debe quedar pendiente: %+v", r)
	}
	// Una liga mal formada ni siquiera sale a la red.
	if r := verificador.Verificar(context.Background(), "http://ejemplo.mx/sin-tls"); r.Estado != models.EnlacePendiente {
		t.Errorf("liga sin https: %+v", r)
	}
}

// Recordatorios de borrador a 7 días, 2 días y 12 horas del cierre.
func TestRecordatoriosDeBorrador(t *testing.T) {
	e := nuevoEntorno(t)
	_, interpretes, compositores, _, _ := e.convocatoriasAbiertas() // cierre: 20-dic-2026 05:59 UTC
	ana := e.postulante("ana@ejemplo.mx")
	ana.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", interpretes), obj{"subcategoria": "ensamble"})
	ana.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", compositores), obj{})
	// Luis ya envió: a él no se le recuerda nada.
	luis := e.postulanteVerificado("luis@ejemplo.mx")
	ruta, _ := e.solistaCompleto(luis, interpretes, "luis")
	luis.exige(http.StatusOK, "POST", ruta+"/envio", obj{"confirmo": true, "acepto_bases": true})

	vistas := e.vistas()
	trabajador := &TrabajadorRecordatorios{Datos: e.repo, Vistas: vistas, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), URLBase: urlBasePrueba}
	recordatorios := func() []models.CorreoSaliente {
		var lista []models.CorreoSaliente
		for _, c := range e.correos() {
			if c.PlantillaClave == models.PlantillaRecordatorio {
				lista = append(lista, c)
			}
		}
		return lista
	}
	pasos := []struct {
		momento  string
		encola   int
		acumula  int
		etiqueta string
	}{
		{"2026-11-26T12:00:00Z", 0, 0, "faltan más de siete días"},
		{"2026-12-13T05:58:00Z", 0, 0, "un minuto antes de los siete días"},
		{"2026-12-13T06:00:00Z", 2, 2, "a siete días: los dos borradores de Ana"},
		{"2026-12-13T06:10:00Z", 0, 2, "el mismo recordatorio no se repite"},
		{"2026-12-16T12:00:00Z", 0, 2, "a media semana no hay otro"},
		{"2026-12-18T06:00:00Z", 2, 4, "a dos días"},
		{"2026-12-19T18:00:00Z", 2, 6, "a doce horas"},
		{"2026-12-19T23:00:00Z", 0, 6, "y ya no hay más"},
		{"2026-12-20T06:00:00Z", 0, 6, "cerrado el plazo no se recuerda nada"},
	}
	for _, paso := range pasos {
		e.hasta(paso.momento)
		if n := trabajador.Procesar(context.Background()); n != paso.encola || len(recordatorios()) != paso.acumula {
			t.Fatalf("%s (%s): encoló %d y van %d; se esperaban %d y %d", paso.etiqueta, paso.momento, n, len(recordatorios()), paso.encola, paso.acumula)
		}
	}
	correo := recordatorios()[0]
	if correo.Destinatario != "ana@ejemplo.mx" || correo.PostulacionID == 0 || strings.Contains(correo.Texto, "{{") ||
		!strings.Contains(correo.Texto, "19 de diciembre de 2026, 23:59 h (hora de la Ciudad de México)") ||
		!strings.Contains(correo.Texto, urlBasePrueba+"/cuenta") || !strings.Contains(correo.Texto, "2028") {
		t.Errorf("recordatorio inesperado: %+v", correo)
	}
	for _, c := range recordatorios() {
		if c.Destinatario != "ana@ejemplo.mx" {
			t.Errorf("recordatorio a quien no tiene borrador: %s", c.Destinatario)
		}
	}
}

// Bases en PDF e imagen de cabecera de una convocatoria (RF-CONV-02).
func TestArchivosDeConvocatoria(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()
	ruta := fmt.Sprintf("/api/v1/convocatorias/%d/archivos/", interpretes)
	publica := "/convocatorias/50/interpretes"
	anonimo := e.cliente()

	// Sin cargar, no hay nada que servir ni enlace en la página.
	if d := anonimo.bajar(publica + "/bases.pdf"); d.Code != http.StatusNotFound {
		t.Errorf("bases sin cargar: %d", d.Code)
	}
	if pagina := anonimo.get(publica).Body.String(); strings.Contains(pagina, "bases.pdf") || strings.Contains(pagina, "cabecera-imagen") {
		t.Error("la página no debe enlazar archivos que no existen")
	}

	// Sólo el administrador las carga.
	ana := e.postulante("ana@ejemplo.mx")
	if estado, _ := ana.subir("PUT", ruta+"bases_pdf_es", nil, "bases.pdf", pdfDePrueba("bases")); estado != http.StatusForbidden {
		t.Errorf("carga de bases sin permiso: %d", estado)
	}
	for nombre, caso := range map[string]struct {
		clase, archivo string
		contenido      []byte
	}{
		"las bases deben ser PDF":    {"bases_pdf_es", "bases.png", contenidoPNG},
		"la imagen no puede ser PDF": {"imagen", "cabecera.pdf", pdfDePrueba("x")},
		"clase desconocida":          {"logotipo", "logo.png", contenidoPNG},
	} {
		if estado, r := admin.subir("PUT", ruta+caso.clase, nil, caso.archivo, caso.contenido); estado != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d %v", nombre, estado, r)
		}
	}
	basesES, basesEN := pdfDePrueba("bases en español"), pdfDePrueba("terms in English")
	estado, cfg := admin.subir("PUT", ruta+"bases_pdf_es", nil, "Bases 2028.pdf", basesES)
	archivos := cfg["archivos"].(obj)
	if estado != http.StatusOK || archivos["bases_pdf_es"].(obj)["sha256"] != sumaDe(basesES) || archivos["bases_pdf_en"] != nil || archivos["imagen"] != nil {
		t.Fatalf("carga de las bases: %d %v", estado, archivos)
	}

	// Públicas, sin sesión. En inglés, mientras no haya versión inglesa, la
	// página enlaza a la española y lo dice.
	d := anonimo.bajar(publica + "/bases.pdf")
	if d.Code != http.StatusOK || !bytes.Equal(d.Body.Bytes(), basesES) || d.Header().Get("Content-Type") != "application/pdf" ||
		d.Header().Get("Content-Disposition") != `inline; filename="bases_es_fimnme50_interpretes.pdf"` || d.Header().Get("ETag") != `"`+sumaDe(basesES)+`"` {
		t.Errorf("descarga pública de las bases: %d %v", d.Code, d.Header())
	}
	if d := anonimo.bajar(publica+"/bases.pdf", "If-None-Match", `"`+sumaDe(basesES)+`"`); d.Code != http.StatusNotModified {
		t.Errorf("las bases sin cambios deben responder 304: %d", d.Code)
	}
	if pagina := anonimo.get(publica).Body.String(); !strings.Contains(pagina, publica+"/bases.pdf?idioma=es") || !strings.Contains(pagina, "Descargar las bases en PDF") {
		t.Error("la página debe enlazar las bases en PDF")
	}
	ingles := e.cliente()
	ingles.idioma = "en"
	if pagina := ingles.get(publica).Body.String(); !strings.Contains(pagina, "bases.pdf?idioma=es") || !strings.Contains(pagina, "(in Spanish)") {
		t.Error("sin versión inglesa, la página en inglés ofrece la española y lo avisa")
	}
	if d := ingles.bajar(publica + "/bases.pdf"); d.Code != http.StatusNotFound {
		t.Errorf("no hay bases en inglés todavía: %d", d.Code)
	}
	admin.subir("PUT", ruta+"bases_pdf_en", nil, "terms.pdf", basesEN)
	if d := ingles.bajar(publica + "/bases.pdf"); d.Code != http.StatusOK || !bytes.Equal(d.Body.Bytes(), basesEN) {
		t.Errorf("bases en inglés: %d", d.Code)
	}
	if d := ingles.bajar(publica + "/bases.pdf?idioma=es"); !bytes.Equal(d.Body.Bytes(), basesES) {
		t.Error("?idioma=es debe servir las bases en español")
	}
	if pagina := ingles.get(publica).Body.String(); !strings.Contains(pagina, "bases.pdf?idioma=en") || strings.Contains(pagina, "(in Spanish)") {
		t.Error("con versión inglesa, la página en inglés la enlaza")
	}

	// Imagen de cabecera.
	admin.subir("PUT", ruta+"imagen", nil, "cabecera.png", contenidoPNG)
	if d := anonimo.bajar(publica + "/imagen"); d.Code != http.StatusOK || d.Header().Get("Content-Type") != "image/png" {
		t.Errorf("imagen de cabecera: %d", d.Code)
	}
	if !strings.Contains(anonimo.get(publica).Body.String(), `<img class="cabecera-imagen" src="`+publica+`/imagen"`) {
		t.Error("la página debe mostrar la imagen de cabecera")
	}
	// Retirar.
	cfg = admin.exige(http.StatusOK, "DELETE", ruta+"imagen", nil)
	if cfg["archivos"].(obj)["imagen"] != nil || anonimo.bajar(publica+"/imagen").Code != http.StatusNotFound {
		t.Error("la imagen retirada no debe servirse")
	}
	if n := len(e.asientos("convocatoria.archivo")); n != 4 {
		t.Errorf("asientos de archivos de convocatoria: %d", n)
	}
	// Una convocatoria en borrador no es pública: tampoco sus archivos.
	if d := anonimo.bajar("/convocatorias/49/compositores/bases.pdf"); d.Code != http.StatusNotFound {
		t.Errorf("bases de una convocatoria sin archivos: %d", d.Code)
	}
	if d := anonimo.bajar("/convocatorias/99/interpretes/bases.pdf"); d.Code != http.StatusNotFound {
		t.Errorf("bases de una edición inexistente: %d", d.Code)
	}
}

// vistas carga el frontal incrustado, para los trabajadores que arman correos.
func (e *entornoPrueba) vistas() *views.Motor {
	e.t.Helper()
	motor, err := views.Nuevo()
	if err != nil {
		e.t.Fatal(err)
	}
	return motor
}
