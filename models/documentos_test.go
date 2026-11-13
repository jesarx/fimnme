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

package models

import (
	"bytes"
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Pruebas de las reglas del paso 6: formato de los archivos, lugar de un
// documento nuevo, folio, recordatorios, ligas, y el almacén en disco.

func TestDetectarMIMEPorFirma(t *testing.T) {
	casos := map[string]string{
		"%PDF-1.7\n%âãÏÓ":                     "application/pdf",
		"\xFF\xD8\xFF\xE0\x00\x10JFIF":        "image/jpeg",
		"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR": "image/png",
		"RIFF\x24\x08\x00\x00WAVEfmt ":        "audio/wav",
		"fLaC\x00\x00\x00\x22":                "audio/flac",
		"ID3\x04\x00\x00\x00\x00\x00\x00":     "audio/mpeg",
		"\xFF\xFB\x90\x64\x00":                "audio/mpeg", // MPEG-1 capa III, sin etiqueta
		"\xFF\xF3\x40\xC0\x00":                "audio/mpeg", // MPEG-2 capa III
		// Lo que NO debe reconocerse.
		"":                             "",
		"hola, esto es texto":          "",
		"RIFF\x24\x08\x00\x00AVI LIST": "", // RIFF que no es WAVE
		"PK\x03\x04":                   "", // ZIP (un .docx)
		"<html><script>":               "",
		"\xFF\xFF\xFF\xFF":             "", // sincronía con valores reservados
		"\xFF\xE0\x00\x00":             "", // capa reservada
		"GIF89a":                       "",
		" %PDF-1.7":                    "", // la firma debe estar al inicio
		"MZ\x90\x00":                   "", // ejecutable
		"\x7FELF":                      "",
		"#!/bin/sh\nrm -rf /":          "",
		"<?xml version=\"1.0\"?><svg>": "",
	}
	for cabecera, esperado := range casos {
		if obtenido := DetectarMIME([]byte(cabecera)); obtenido != esperado {
			t.Errorf("DetectarMIME(%q) = %q, se esperaba %q", cabecera, obtenido, esperado)
		}
	}
	// Todo formato admitido tiene extensión, y la detección sólo devuelve
	// formatos admitidos.
	for _, mime := range MimesAdmitidos {
		if ExtensionDeMIME(mime) == "" {
			t.Errorf("el formato %s no tiene extensión", mime)
		}
	}
}

func TestValidarArchivo(t *testing.T) {
	partitura := RequisitoDocumental{Clave: "partitura", MimesPermitidos: []string{"application/pdf"}, TamanoMaxBytes: 1000}
	foto := RequisitoDocumental{Clave: "fotografia", MimesPermitidos: []string{"image/jpeg", "image/png"}, TamanoMaxBytes: 1000}
	casos := []struct {
		rq     RequisitoDocumental
		nombre string
		mime   string
		tamano int64
		error  string
	}{
		{partitura, "obra.pdf", "application/pdf", 1000, "[]"},
		{partitura, "OBRA.PDF", "application/pdf", 1, "[]"}, // la extensión no distingue mayúsculas
		{partitura, "obra.pdf", "application/pdf", 0, "[error.archivo_vacio]"},
		{partitura, "obra.pdf", "application/pdf", 1001, "[error.archivo_grande]"},
		{partitura, "obra.pdf", "", 10, "[error.formato_no_reconocido]"},
		{partitura, "obra.pdf", "image/png", 10, "[error.formato_no_admitido]"},
		{partitura, "obra.docx", "application/pdf", 10, "[error.extension_no_coincide]"},
		{partitura, "obra", "application/pdf", 10, "[error.extension_no_coincide]"},
		{foto, "retrato.jpeg", "image/jpeg", 10, "[]"},
		{foto, "retrato.jpg", "image/jpeg", 10, "[]"},
		{foto, "retrato.jpg", "image/png", 10, "[error.extension_no_coincide]"}, // PNG disfrazado de JPEG
		{foto, "retrato.pdf", "application/pdf", 10, "[error.formato_no_admitido]"},
	}
	for _, c := range casos {
		if obtenido := claves(ValidarArchivo(c.rq, c.nombre, c.mime, c.tamano)); obtenido != c.error {
			t.Errorf("%s (%s, %d bytes) para %s: %s, se esperaba %s", c.nombre, c.mime, c.tamano, c.rq.Clave, obtenido, c.error)
		}
	}
	// Archivos públicos de la convocatoria.
	for _, c := range []struct {
		clase  ClaseArchivoConvocatoria
		nombre string
		mime   string
		tamano int64
		error  string
	}{
		{ArchivoBasesES, "bases.pdf", "application/pdf", TamanoMaxBasesPDF, "[]"},
		{ArchivoBasesEN, "terms.pdf", "application/pdf", TamanoMaxBasesPDF + 1, "[error.archivo_grande]"},
		{ArchivoBasesES, "bases.png", "image/png", 10, "[error.formato_no_admitido]"},
		{ArchivoImagen, "cabecera.png", "image/png", 10, "[]"},
		{ArchivoImagen, "cabecera.jpg", "image/jpeg", TamanoMaxImagen + 1, "[error.archivo_grande]"},
		{ArchivoImagen, "cabecera.pdf", "application/pdf", 10, "[error.formato_no_admitido]"},
		{"otra", "x.pdf", "application/pdf", 10, "[error.clase_de_archivo]"},
	} {
		if obtenido := claves(ValidarArchivoDeConvocatoria(c.clase, c.nombre, c.mime, c.tamano)); obtenido != c.error {
			t.Errorf("%s como %s: %s, se esperaba %s", c.nombre, c.clase, obtenido, c.error)
		}
	}
}

// Qué requisito cumple un documento depende de la obra y de SU dotación.
func TestRequisitoParaDocumento(t *testing.T) {
	cfg := configuracionDeEnvio(TipoCompositores)
	p := postulacionCompleta(TipoCompositores, "") // obra 10: cuarteto; obra 11: electroacústica
	casos := []struct {
		clave       string
		obraID      int64
		obligatorio bool
		error       string
	}{
		{"fotografia", 0, true, "[]"},
		{"partitura", 10, true, "[]"},
		{"particellas", 10, true, "[]"},
		{"particellas", 11, false, "[]"}, // la electroacústica no las exige
		{"audio", 10, false, "[]"},
		{"audio", 11, true, "[]"},                                  // y sí exige el audio
		{"fotografia", 10, false, "[error.requisito_inexistente]"}, // la fotografía no es de una obra
		{"partitura", 0, false, "[error.requisito_inexistente]"},   // ni la partitura de la postulación
		{"curriculum", 0, false, "[error.requisito_inexistente]"},
		{"partitura", 99, false, "[error.obra_ajena]"},
	}
	for _, c := range casos {
		rq, errores := RequisitoParaDocumento(p, cfg, c.clave, c.obraID)
		if claves(errores) != c.error || (c.error == "[]" && (rq.Clave != c.clave || rq.Obligatorio != c.obligatorio)) {
			t.Errorf("%s de la obra %d: %+v %s, se esperaba obligatorio=%v %s", c.clave, c.obraID, rq, claves(errores), c.obligatorio, c.error)
		}
	}
}

func TestLugarDeNuevoDocumento(t *testing.T) {
	uno := RequisitoDocumental{MaxArchivos: 1}
	tres := RequisitoDocumental{MaxArchivos: 3}
	vigente := func(ids ...int64) []Documento {
		var lista []Documento
		for _, id := range ids {
			lista = append(lista, Documento{ID: id, Version: int(id)})
		}
		return lista
	}
	casos := []struct {
		nombre     string
		rq         RequisitoDocumental
		vigentes   []Documento
		reemplazaA int64
		versiones  int
		reemplaza  int64 // id del documento reemplazado; 0 = se añade
		error      string
	}{
		{"el primero se añade", uno, nil, 0, 0, 0, "[]"},
		{"con un solo archivo, el nuevo reemplaza al que hay", uno, vigente(4), 0, 1, 4, "[]"},
		{"con varios, se añade mientras quepa", tres, vigente(1, 2), 0, 2, 0, "[]"},
		{"con varios y lleno, hay que decir cuál", tres, vigente(1, 2, 3), 0, 3, 0, "[error.demasiados_archivos]"},
		{"con varios y lleno, reemplazo explícito", tres, vigente(1, 2, 3), 2, 3, 2, "[]"},
		{"reemplazar uno que no es vigente", tres, vigente(1, 2), 7, 2, 0, "[error.documento_no_vigente]"},
		{"tope de versiones", uno, vigente(4), 0, MaxVersionesDeDocumento, 0, "[error.demasiadas_versiones]"},
	}
	for _, c := range casos {
		anterior, errores := LugarDeNuevoDocumento(c.rq, c.vigentes, c.reemplazaA, c.versiones)
		var reemplazado int64
		if anterior != nil {
			reemplazado = anterior.ID
		}
		if claves(errores) != c.error || reemplazado != c.reemplaza {
			t.Errorf("%s: reemplaza %d %s, se esperaba %d %s", c.nombre, reemplazado, claves(errores), c.reemplaza, c.error)
		}
	}
	d := Documento{Clave: "partitura", Version: 2, Archivo: Archivo{MIME: "application/pdf"}}
	if nombre := NombreDeDescarga("FIM49-COM-0007", d, 2); nombre != "FIM49-COM-0007_partitura_obra2_v2.pdf" {
		t.Errorf("nombre de descarga: %s", nombre)
	}
	d = Documento{Clave: "fotografia", Version: 1, Archivo: Archivo{MIME: "image/jpeg"}}
	if nombre := NombreDeDescarga("borrador-12", d, 0); nombre != "borrador-12_fotografia_v1.jpg" {
		t.Errorf("nombre de descarga: %s", nombre)
	}
}

func TestFolioYConfirmacion(t *testing.T) {
	for esperado, obtenido := range map[string]string{
		"FIM49-INT-0001":  FolioDe(49, TipoInterpretes, 1),
		"FIM49-COM-0137":  FolioDe(49, TipoCompositores, 137),
		"FIM50-INT-12345": FolioDe(50, TipoInterpretes, 12345), // nunca se trunca
	} {
		if obtenido != esperado {
			t.Errorf("folio %s, se esperaba %s", obtenido, esperado)
		}
	}
	if claves(ValidarConfirmacionDeEnvio(ConfirmacionEnvio{})) != "[error.confirmar_envio error.aceptar_bases]" ||
		claves(ValidarConfirmacionDeEnvio(ConfirmacionEnvio{Confirmo: true})) != "[error.aceptar_bases]" ||
		ValidarConfirmacionDeEnvio(ConfirmacionEnvio{Confirmo: true, AceptoBases: true}) != nil {
		t.Error("confirmación de envío incorrecta")
	}
}

// El snapshot congela sólo lo que define el formulario y los documentos vigentes.
func TestSnapshotDeEnvio(t *testing.T) {
	cfg := configuracionDeEnvio(TipoCompositores)
	p := postulacionCompleta(TipoCompositores, "")
	p.Campos["datos.nombre_artistico"] = "   " // vacío: no entra
	p.Campos["campo.inventado"] = "no es del formulario"
	p.Documentos[1].Archivo = Archivo{SHA256: strings.Repeat("ab", 32), TamanoBytes: 2048, MIME: "application/pdf"}
	titular := Usuario{Nombre: "Eva", Apellidos: "Paz", Email: "eva@ejemplo.mx", Idioma: "en"}
	s := SnapshotDeEnvio(p, cfg, titular, "FIM49-COM-0001", 1, time.Date(2026, 12, 1, 18, 30, 45, 999, time.FixedZone("CDMX", -6*3600)))
	if s.Folio != "FIM49-COM-0001" || s.Version != 1 || s.EnviadaAt != "2026-12-02T00:30:45Z" || s.Idioma != "en" ||
		s.Convocatoria.Tipo != "compositores" || s.Titular.Email != "eva@ejemplo.mx" || s.Subcategoria != "" {
		t.Errorf("encabezado del snapshot: %+v", s)
	}
	if _, hay := s.Campos["campo.inventado"]; hay || s.Campos["datos.nombre_completo"] != "Ana López" {
		t.Errorf("campos del snapshot: %v", s.Campos)
	}
	if _, hay := s.Campos["datos.nombre_artistico"]; hay {
		t.Error("un campo vacío no debe entrar al snapshot")
	}
	if len(s.Obras) != 2 || s.Obras[0].DotacionES != "Cuarteto" || s.Obras[1].DotacionES != "Electroacústica" || s.Obras[0].Campos["obra.titulo"] != "Obra" {
		t.Errorf("obras del snapshot: %+v", s.Obras)
	}
	if len(s.Documentos) != 5 || s.Documentos[1].Clave != "partitura" || s.Documentos[1].EtiquetaEN != "Score" ||
		s.Documentos[1].SHA256 != strings.Repeat("ab", 32) || s.Documentos[1].ObraID != 10 {
		t.Errorf("documentos del snapshot: %+v", s.Documentos)
	}
	// Una cuenta sin idioma reconocido recibe el acuse en español.
	if SnapshotDeEnvio(p, cfg, Usuario{Idioma: "fr"}, "F", 1, ahoraDePrueba).Idioma != "es" {
		t.Error("el idioma por omisión del acuse es el español")
	}
}

func TestVentanaDeRecordatorio(t *testing.T) {
	cierre := time.Date(2027, 1, 30, 5, 59, 0, 0, time.UTC)
	casos := []struct {
		antes  time.Duration // cuánto falta para el cierre
		toca   bool
		inicio time.Duration // inicio de la ventana, antes del cierre
	}{
		{30 * 24 * time.Hour, false, 0},
		{7*24*time.Hour + time.Minute, false, 0},
		{7 * 24 * time.Hour, true, 7 * 24 * time.Hour}, // exactamente a 7 días
		{5 * 24 * time.Hour, true, 7 * 24 * time.Hour},
		{48*time.Hour + time.Second, true, 7 * 24 * time.Hour},
		{48 * time.Hour, true, 48 * time.Hour},
		{13 * time.Hour, true, 48 * time.Hour},
		{12 * time.Hour, true, 12 * time.Hour},
		{time.Minute, true, 12 * time.Hour},
		{0, false, 0},          // en el instante del cierre ya no
		{-time.Hour, false, 0}, // ni después
	}
	for _, c := range casos {
		inicio, toca := VentanaDeRecordatorio(cierre, cierre.Add(-c.antes))
		if toca != c.toca || (toca && !inicio.Equal(cierre.Add(-c.inicio))) {
			t.Errorf("a %s del cierre: toca=%v inicio=%s", c.antes, toca, cierre.Sub(inicio))
		}
	}
}

func TestDireccionPublica(t *testing.T) {
	publicas := []string{"93.184.216.34", "8.8.8.8", "2606:2800:220:1:248:1893:25c8:1946", "::ffff:93.184.216.34", "201.144.0.1"}
	privadas := []string{
		"127.0.0.1", "127.255.255.254", "10.0.0.5", "172.16.0.1", "172.31.255.255", "192.168.1.1", // bucle y redes privadas
		"169.254.169.254",                  // metadatos de nubes
		"0.0.0.0", "0.1.2.3", "100.64.0.1", // sin especificar y compartidas de operador
		"192.0.2.1", "198.51.100.7", "203.0.113.9", "198.18.0.1", "240.0.0.1", "255.255.255.255", "224.0.0.1",
		"::1", "::", "fc00::1", "fd12:3456::1", "fe80::1", "ff02::1", "2001:db8::1", "64:ff9b::a00:1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", // IPv4 escrita como IPv6
	}
	for _, ip := range publicas {
		if !DireccionPublica(netip.MustParseAddr(ip)) {
			t.Errorf("%s es pública y se rechazó", ip)
		}
	}
	for _, ip := range privadas {
		if DireccionPublica(netip.MustParseAddr(ip)) {
			t.Errorf("%s NO es pública y se aceptó", ip)
		}
	}
	if DireccionPublica(netip.Addr{}) {
		t.Error("una dirección inválida no es pública")
	}
}

func TestClasificarEnlace(t *testing.T) {
	casos := []struct {
		porOEmbed   bool
		estado      int
		pideIngreso bool
		esperado    EstadoEnlace
		detalle     string
	}{
		{false, 200, false, EnlaceOK, DetalleEnlaceOK},
		{false, 206, false, EnlaceOK, DetalleEnlaceOK},
		{true, 200, false, EnlaceOK, DetalleEnlaceOK},
		{false, 404, false, EnlaceInaccesible, DetalleEnlaceNoEncontrado},
		{false, 410, false, EnlaceInaccesible, DetalleEnlaceNoEncontrado},
		{true, 404, false, EnlaceInaccesible, DetalleEnlaceNoEncontrado},
		{true, 400, false, EnlaceInaccesible, DetalleEnlaceNoEncontrado},
		{false, 401, false, EnlaceNoPublico, DetalleEnlaceRequiereAcceso},
		{true, 401, false, EnlaceNoPublico, DetalleEnlaceRequiereAcceso},
		{true, 403, false, EnlaceNoPublico, DetalleEnlaceRequiereAcceso}, // la plataforma dice que es privado
		{false, 0, true, EnlaceNoPublico, DetalleEnlaceRequiereAcceso},   // redirige a iniciar sesión
		// Lo dudoso no se da por malo: queda pendiente para el revisor.
		{false, 403, false, EnlacePendiente, DetalleEnlaceNoComprobable}, // muchos sitios responden 403 a un programa
		{false, 400, false, EnlacePendiente, DetalleEnlaceNoComprobable},
		{false, 429, false, EnlacePendiente, DetalleEnlaceNoComprobable},
		{false, 500, false, EnlacePendiente, DetalleEnlaceNoComprobable},
		{true, 503, false, EnlacePendiente, DetalleEnlaceNoComprobable},
		{false, 301, false, EnlacePendiente, DetalleEnlaceNoComprobable},
	}
	for _, c := range casos {
		r := ClasificarEnlace(c.porOEmbed, c.estado, c.pideIngreso)
		if r.Estado != c.esperado || r.Detalle != c.detalle || r.HTTPStatus != c.estado {
			t.Errorf("oEmbed=%v HTTP %d ingreso=%v: %+v, se esperaba %s/%s", c.porOEmbed, c.estado, c.pideIngreso, r, c.esperado, c.detalle)
		}
	}
	for liga, prefijo := range map[string]string{
		"https://www.youtube.com/watch?v=abc&t=1": "https://www.youtube.com/oembed?format=json&url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3Dabc%26t%3D1",
		"https://youtu.be/abc":                    "https://www.youtube.com/oembed?format=json&url=https%3A%2F%2Fyoutu.be%2Fabc",
		"https://vimeo.com/123":                   "https://vimeo.com/api/oembed.json?url=https%3A%2F%2Fvimeo.com%2F123",
		"https://soundcloud.com/a/b":              "https://soundcloud.com/oembed?format=json&url=https%3A%2F%2Fsoundcloud.com%2Fa%2Fb",
		"https://artista.bandcamp.com/track/x":    "",
		"https://ejemplo.mx/video":                "",
	} {
		if obtenido := ConsultaOEmbed(liga); obtenido != prefijo {
			t.Errorf("ConsultaOEmbed(%s) = %s", liga, obtenido)
		}
	}
}

func TestAlmacenEnDisco(t *testing.T) {
	raiz := t.TempDir()
	// Una subida que un apagado dejó a medias se limpia al abrir.
	if err := os.MkdirAll(filepath.Join(raiz, "tmp"), 0o750); err != nil {
		t.Fatal(err)
	}
	resto := filepath.Join(raiz, "tmp", "subida-vieja")
	if err := os.WriteFile(resto, []byte("a medias"), 0o600); err != nil {
		t.Fatal(err)
	}
	almacen, err := AbrirAlmacenEnDisco(raiz)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resto); !errors.Is(err, os.ErrNotExist) {
		t.Error("las subidas incompletas deben limpiarse al abrir el almacén")
	}

	contenido := []byte("%PDF-1.7 contenido de prueba")
	blob, err := almacen.Guardar(bytes.NewReader(contenido), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob.SHA256) != 64 || blob.Tamano != int64(len(contenido)) || blob.Clave != blob.SHA256[:2]+"/"+blob.SHA256[2:4]+"/"+blob.SHA256 {
		t.Errorf("blob inesperado: %+v", blob)
	}
	ruta := filepath.Join(raiz, filepath.FromSlash(blob.Clave))
	info, err := os.Stat(ruta)
	if err != nil || info.Mode().Perm() != 0o440 {
		t.Errorf("el archivo debe quedar en su ruta y de sólo lectura: %v %v", info, err)
	}
	// Deduplicación: el mismo contenido da la misma clave y no se reescribe.
	antes := info.ModTime()
	repetido, err := almacen.Guardar(bytes.NewReader(contenido), 1000)
	if err != nil || repetido != blob {
		t.Errorf("el mismo contenido debe dar el mismo blob: %+v %v", repetido, err)
	}
	if info, _ := os.Stat(ruta); !info.ModTime().Equal(antes) {
		t.Error("un contenido que ya existe no debe volver a escribirse")
	}
	// Lectura.
	lector, err := almacen.Abrir(blob.Clave)
	if err != nil {
		t.Fatal(err)
	}
	leido, err := io.ReadAll(lector)
	if errCerrar := lector.Close(); err != nil || errCerrar != nil || !bytes.Equal(leido, contenido) {
		t.Errorf("lo leído no coincide con lo guardado: %q %v %v", leido, err, errCerrar)
	}

	// Límite: exactamente el límite cabe; un byte más, no, y no deja nada.
	if _, err := almacen.Guardar(bytes.NewReader(make([]byte, 500)), 500); err != nil {
		t.Errorf("un archivo del tamaño límite debe caber: %v", err)
	}
	if _, err := almacen.Guardar(bytes.NewReader(make([]byte, 501)), 500); !errors.Is(err, ErrArchivoDemasiadoGrande) {
		t.Errorf("un archivo mayor que el límite debe rechazarse: %v", err)
	}
	// Un lector que falla a media subida (conexión cortada) tampoco deja nada.
	if _, err := almacen.Guardar(io.MultiReader(strings.NewReader("inicio"), lectorRoto{}), 1000); err == nil {
		t.Error("una lectura interrumpida debe dar error")
	}
	if temporales, _ := os.ReadDir(filepath.Join(raiz, "tmp")); len(temporales) != 0 {
		t.Errorf("quedaron %d archivos temporales", len(temporales))
	}

	// Claves que no tienen la forma de una clave no tocan el disco.
	for _, clave := range []string{"", "../../etc/passwd", "ab/cd/../../../secreto", blob.Clave + "/..", "/etc/passwd", strings.ToUpper(blob.Clave)} {
		if _, err := almacen.Abrir(clave); err == nil || errors.Is(err, ErrNoEncontrado) {
			t.Errorf("la clave %q debe rechazarse por inválida: %v", clave, err)
		}
	}
	if _, err := almacen.Abrir("00/00/" + strings.Repeat("0", 64)); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("una clave bien formada que no existe: %v", err)
	}
}

// lectorRoto simula una conexión que se corta.
type lectorRoto struct{}

func (lectorRoto) Read([]byte) (int, error) { return 0, errors.New("conexión cortada") }
