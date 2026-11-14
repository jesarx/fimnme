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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Documentos, envío y acuse (§5.4, §4.1 y §5.10).
//
// Los archivos nunca se sirven directamente desde el disco (RF-DOC-03): toda
// descarga pasa por aquí, con el permiso comprobado en cada petición, y queda
// registrada (RN-27).

const (
	// plazoDeSubida es el tiempo que se concede a una subida. El servidor
	// corta a los 60 segundos las peticiones normales; un audio de 100 MB por
	// una conexión doméstica necesita bastante más.
	plazoDeSubida = 30 * time.Minute
	// holguraDeSubida es lo que puede medir una subida además del archivo:
	// los demás campos del formulario y los delimitadores.
	holguraDeSubida = 1 << 20
	// tamanoMaxCampoDeSubida acota los campos de texto que acompañan al archivo.
	tamanoMaxCampoDeSubida = 1 << 10
	// tamanoMaxAcuse es el límite duro del acuse en PDF (RF-PDF-02).
	tamanoMaxAcuse = 300 << 10
	// longitudMaxNombreDeArchivo acota el nombre original que se guarda.
	longitudMaxNombreDeArchivo = 200
)

// subida es un archivo que está llegando en una petición multipart: los
// campos de texto que lo preceden y el archivo mismo, aún sin leer.
type subida struct {
	campos   map[string]string
	nombre   string        // nombre original del archivo
	mime     string        // formato detectado por la firma del contenido
	vacio    bool          // el archivo no trae ni un byte
	cuerpo   *bufio.Reader // el contenido, desde su primer byte
	lectorMP *multipart.Reader
}

// ampliarPlazos da a la petición el tiempo de una subida, en vez de los
// plazos cortos del servidor. Si la conexión no lo permite (ocurre en las
// pruebas) sigue con los plazos que haya.
func (a *App) ampliarPlazos(w http.ResponseWriter) {
	control := http.NewResponseController(w)
	limite := time.Now().Add(plazoDeSubida)
	for _, fijar := range []func(time.Time) error{control.SetReadDeadline, control.SetWriteDeadline} {
		if err := fijar(limite); err != nil && !errors.Is(err, http.ErrNotSupported) {
			a.log.Warn("no se pudo ampliar el plazo de la subida", "error", err)
		}
	}
}

// abrirSubida lee una petición multipart hasta llegar al archivo (el campo
// «archivo») y se detiene ahí, sin leerlo: quien llama decide, con los campos
// ya conocidos, cuánto puede medir y dónde se guarda. Por eso los campos de
// texto deben ir ANTES del archivo; un navegador los envía en el orden del
// formulario.
//
// El contenido se procesa al vuelo: nunca se carga entero en memoria ni se
// escribe en un temporal fuera del almacén.
func (a *App) abrirSubida(w http.ResponseWriter, r *http.Request, tamanoMax int64) (*subida, bool) {
	a.ampliarPlazos(w)
	r.Body = http.MaxBytesReader(w, r.Body, tamanoMax+holguraDeSubida)
	lector, err := r.MultipartReader()
	if err != nil {
		a.errorAPI(w, r, http.StatusBadRequest, "subida_invalida")
		return nil, false
	}
	s := &subida{campos: make(map[string]string), lectorMP: lector}
	for {
		parte, err := lector.NextPart()
		if err != nil {
			// Se acabó la petición sin archivo, o llegó cortada.
			a.errorAPI(w, r, http.StatusBadRequest, "subida_invalida")
			return nil, false
		}
		if parte.FormName() != "archivo" {
			valor, err := io.ReadAll(io.LimitReader(parte, tamanoMaxCampoDeSubida+1))
			if err != nil || len(valor) > tamanoMaxCampoDeSubida || parte.FileName() != "" {
				a.errorAPI(w, r, http.StatusBadRequest, "subida_invalida")
				return nil, false
			}
			s.campos[parte.FormName()] = strings.TrimSpace(string(valor))
			continue
		}
		s.nombre = nombreDeArchivo(parte.FileName())
		s.cuerpo = bufio.NewReaderSize(parte, 2*models.TamanoCabeceraMIME)
		// Peek mira los primeros bytes sin consumirlos: con ellos se reconoce
		// el formato antes de escribir nada en disco.
		cabecera, err := s.cuerpo.Peek(models.TamanoCabeceraMIME)
		if err != nil && !errors.Is(err, io.EOF) {
			a.errorAPI(w, r, http.StatusBadRequest, "subida_invalida")
			return nil, false
		}
		s.mime, s.vacio = models.DetectarMIME(cabecera), len(cabecera) == 0
		return s, true
	}
}

// nombreDeArchivo limpia el nombre con que llegó un archivo: sin caracteres
// de control y sin pasar de una longitud razonable. Sólo se guarda como
// referencia; las descargas usan un nombre generado.
func nombreDeArchivo(nombre string) string {
	limpio := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, strings.TrimSpace(nombre))
	for utf8.RuneCountInString(limpio) > longitudMaxNombreDeArchivo {
		// Se recorta por el principio para conservar la extensión.
		_, tamano := utf8.DecodeRuneInString(limpio)
		limpio = limpio[tamano:]
	}
	return limpio
}

// guardarSubida escribe el archivo de una subida en el almacén. Antes valida
// lo que ya se sabe —formato y extensión— con la función validar, que recibe
// un tamaño provisional: así un archivo de formato equivocado se rechaza sin
// tocar el disco. Devuelve false si ya respondió con un error.
func (a *App) guardarSubida(w http.ResponseWriter, r *http.Request, s *subida, tamanoMax int64,
	validar func(tamano int64) []models.ErrorCampo) (models.Blob, bool) {
	tamanoProvisional := int64(1)
	if s.vacio {
		tamanoProvisional = 0
	}
	if errores := validar(tamanoProvisional); len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return models.Blob{}, false
	}
	blob, err := a.almacen.Guardar(s.cuerpo, tamanoMax)
	var demasiado *http.MaxBytesError
	switch {
	case errors.Is(err, models.ErrArchivoDemasiadoGrande), errors.As(err, &demasiado):
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: []models.ErrorCampo{{Campo: "archivo", Clave: "error.archivo_grande"}}})
		return models.Blob{}, false
	case err != nil:
		// Lo habitual: quien subía cortó la conexión. No queda nada en el almacén.
		a.log.Warn("subida interrumpida", "ruta", r.URL.Path, "error", err)
		a.errorAPI(w, r, http.StatusBadRequest, "subida_invalida")
		return models.Blob{}, false
	}
	return blob, true
}

// POST /api/v1/postulaciones/{id}/documentos   — sube un documento
//
// Petición multipart/form-data con, en este orden: «clave» (el requisito:
// fotografia, partitura…), «obra_id» si el documento es de una obra,
// «reemplaza» si sustituye a un documento concreto, y al final «archivo».
func (a *App) apiSubirDocumento(w http.ResponseWriter, r *http.Request) {
	postulacionID, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	s, ok := a.abrirSubida(w, r, models.TamanoMaxArchivo)
	if !ok {
		return
	}
	obraID, errObra := enteroOpcional(s.campos["obra_id"])
	reemplaza, errReemplaza := enteroOpcional(s.campos["reemplaza"])
	if errObra != nil || errReemplaza != nil {
		a.errorAPI(w, r, http.StatusBadRequest, "subida_invalida")
		return
	}

	// Antes de recibir el archivo se comprueba todo lo que puede saberse sin
	// él: que la postulación sea de la cuenta y siga editable, y qué requisito
	// se quiere cumplir. El repositorio lo vuelve a comprobar al registrar.
	sesion := sesionDe(r)
	p, err := a.datos.PostulacionPorID(r.Context(), postulacionID)
	if err == nil && p.UsuarioID != sesion.Usuario.ID {
		err = models.ErrNoEncontrado
	}
	if a.falloDatos(w, r, err) {
		return
	}
	cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), p.ConvocatoriaID)
	if a.falloDatos(w, r, err) {
		return
	}
	requisito, errores := models.RequisitoParaDocumento(p, cfg, s.campos["clave"], obraID)
	if len(errores) == 0 {
		var permiso models.PermisoDeEdicion
		permiso, errores = models.PermisoDeEdicionDe(p, cfg.Convocatoria, a.ahora())
		if len(errores) == 0 && !permiso.Documento(s.campos["clave"], obraID) {
			// En una subsanación sólo se suben los documentos señalados.
			errores = []models.ErrorCampo{{Campo: "archivo", Clave: "error.elemento_bloqueado"}}
		}
	}
	if len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return
	}

	blob, ok := a.guardarSubida(w, r, s, requisito.TamanoMaxBytes, func(tamano int64) []models.ErrorCampo {
		return models.ValidarArchivo(requisito, s.nombre, s.mime, tamano)
	})
	if !ok {
		return
	}
	_, err = a.datos.RegistrarDocumento(r.Context(), a.actorDe(r), sesion.Usuario.ID, postulacionID, models.NuevoDocumento{
		Clave: s.campos["clave"], ObraID: obraID, ReemplazaA: reemplaza, Blob: blob, MIME: s.mime, NombreOriginal: s.nombre,
	})
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusCreated, postulacionID)
}

// enteroOpcional convierte un campo opcional a entero; vacío es 0.
func enteroOpcional(texto string) (int64, error) {
	if texto == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(texto, 10, 64)
	if err == nil && n < 0 {
		err = errors.New("negativo")
	}
	return n, err
}

// DELETE /api/v1/documentos/{id}   — retira un documento del borrador
func (a *App) apiQuitarDocumento(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	postulacionID, err := a.datos.QuitarDocumento(r.Context(), a.actorDe(r), sesionDe(r).Usuario.ID, id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, postulacionID)
}

// servirArchivo envía un archivo del almacén. Por omisión se muestra en el
// navegador (un PDF o un audio pueden verse sin descargarlos, RF-DOC-08); con
// ?descargar=1 se ofrece como descarga.
//
// Admite peticiones por rangos (Range), que es como un reproductor de audio
// salta a la mitad de una pista.
func (a *App) servirArchivo(w http.ResponseWriter, r *http.Request, archivo models.Archivo, nombre string) {
	contenido, err := a.almacen.Abrir(archivo.Clave)
	if err != nil {
		// La base dice que el archivo existe y el disco no lo tiene: es un
		// problema de integridad del acervo, no un error de quien pide.
		a.log.Error("archivo registrado que no está en el almacén", "archivo", archivo.ID, "sha256", archivo.SHA256, "error", err)
		a.errorAPI(w, r, http.StatusNotFound, "archivo_no_disponible")
		return
	}
	defer contenido.Close()
	disposicion := "inline"
	if r.URL.Query().Get("descargar") != "" {
		disposicion = "attachment"
	}
	cabeceras := w.Header()
	cabeceras.Set("Content-Type", archivo.MIME)
	cabeceras.Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposicion, nombre))
	// Material inédito: ni el navegador ni un intermediario deben guardarlo.
	cabeceras.Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "", time.Time{}, contenido)
}

// esPrimeraLectura indica si la petición pide el archivo desde el principio.
// Un visor de PDF o un reproductor piden el mismo archivo en muchos trozos:
// sólo el primero cuenta como una descarga.
func esPrimeraLectura(r *http.Request) bool {
	rango := r.Header.Get("Range")
	return rango == "" || strings.HasPrefix(rango, "bytes=0-")
}

// postulacionVisible lee una postulación y comprueba que la sesión pueda
// verla. Si no, responde 404 —igual que si no existiera— y devuelve false.
func (a *App) postulacionVisible(w http.ResponseWriter, r *http.Request, id int64) (models.Postulacion, bool) {
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if err == nil && !models.PuedeVerPostulacion(sesionDe(r), p) {
		err = models.ErrNoEncontrado
	}
	return p, !a.falloDatos(w, r, err)
}

// identificadorDe es lo que encabeza el nombre de una descarga: el folio, o
// «borrador-N» mientras la postulación no lo tiene.
func identificadorDe(p models.Postulacion) string {
	if p.Folio != "" {
		return p.Folio
	}
	return "borrador-" + strconv.FormatInt(p.ID, 10)
}

// GET /api/v1/documentos/{id}/archivo   — descarga un documento
//
// Lo descargan el titular de la postulación, el administrador y el
// observador. Cada descarga queda registrada con cuenta, IP y momento.
func (a *App) apiDescargarDocumento(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	documento, err := a.datos.DocumentoPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	p, ok := a.postulacionVisible(w, r, documento.PostulacionID)
	if !ok {
		return
	}
	if esPrimeraLectura(r) {
		if a.falloDatos(w, r, a.datos.RegistrarDescarga(r.Context(), a.actorDe(r), p.ID, documento.ID, documento.Archivo)) {
			return
		}
	}
	ordenDeObra := 0
	for _, obra := range p.Obras {
		if obra.ID == documento.ObraID {
			ordenDeObra = obra.Orden
		}
	}
	a.servirArchivo(w, r, documento.Archivo, models.NombreDeDescarga(identificadorDe(p), documento, ordenDeObra))
}

// GET /api/v1/postulaciones/{id}/acuses/{version}   — descarga un acuse en PDF
func (a *App) apiDescargarAcuse(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	version, err := strconv.Atoi(r.PathValue("version"))
	if !ok {
		return
	}
	if err != nil || version < 1 {
		a.errorAPI(w, r, http.StatusNotFound, "no_encontrado")
		return
	}
	p, ok := a.postulacionVisible(w, r, id)
	if !ok {
		return
	}
	acuse, err := a.datos.AcusePorVersion(r.Context(), p.ID, version)
	if a.falloDatos(w, r, err) {
		return
	}
	if esPrimeraLectura(r) {
		if a.falloDatos(w, r, a.datos.RegistrarDescarga(r.Context(), a.actorDe(r), p.ID, 0, acuse.Archivo)) {
			return
		}
	}
	a.servirArchivo(w, r, acuse.Archivo, fmt.Sprintf("acuse_%s_v%d.pdf", p.Folio, acuse.Version))
}

// POST /api/v1/postulaciones/{id}/envio   — envía la postulación (irreversible)
//
// Cuerpo: {"confirmo": true, "acepto_bases": true}. Si a la postulación le
// falta algo responde 422 con código «incompleta» y la lista de bloqueadores.
func (a *App) apiEnviarPostulacion(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	var entrada struct {
		Confirmo    bool `json:"confirmo"`
		AceptoBases bool `json:"acepto_bases"`
	}
	if !ok || !a.leerJSON(w, r, &entrada) {
		return
	}
	sesion := sesionDe(r)
	p, err := a.datos.PostulacionPorID(r.Context(), id)
	if err == nil && p.UsuarioID != sesion.Usuario.ID {
		err = models.ErrNoEncontrado
	}
	if a.falloDatos(w, r, err) {
		return
	}

	// Las ligas de video se comprueban en red justo antes de enviar (RN-08).
	// Va fuera de la transacción: una consulta a Internet no debe retener la
	// única conexión de escritura de la base.
	reenvio := p.Estado == models.PostulacionEnSubsanacion
	if p.Estado == models.PostulacionBorrador || reenvio {
		a.verificarEnlacesDe(r.Context(), id)
	}

	// El reenvío de una subsanación lleva su propio texto de acuse y avisa a
	// los revisores de que la postulación volvió a la bandeja (RF-REV-07).
	clavePlantilla := models.PlantillaAcuse
	var revisores []models.Usuario
	avisos := make(map[string]models.PlantillaCorreo) // por idioma
	if reenvio {
		clavePlantilla = models.PlantillaAcuseSubsanacion
		cfg, err := a.datos.ConfiguracionDeConvocatoria(r.Context(), p.ConvocatoriaID)
		if a.falloDatos(w, r, err) {
			return
		}
		for _, asignacion := range cfg.Asignaciones {
			if asignacion.Activo && asignacion.Perfil == models.PerfilRevisorDocumental {
				revisores = append(revisores, asignacion.Usuario)
			}
		}
		for _, idioma := range []string{models.IdiomaES, models.IdiomaEN} {
			if avisos[idioma], err = a.datos.PlantillaCorreo(r.Context(), models.PlantillaAvisoSubsanada, idioma); a.falloDatos(w, r, err) {
				return
			}
		}
	}
	plantilla, err := a.datos.PlantillaCorreo(r.Context(), clavePlantilla, sesion.Usuario.Idioma)
	if a.falloDatos(w, r, err) {
		return
	}
	// componer es la parte de la Vista en el envío: recibe el snapshot ya
	// congelado, compone el PDF, lo deja en el almacén y arma el correo.
	componer := func(s models.SnapshotEnvio) (models.AcuseCompuesto, error) {
		contenido, err := a.vistas.AcusePDF(s)
		if err != nil {
			return models.AcuseCompuesto{}, err
		}
		blob, err := a.almacen.Guardar(bytes.NewReader(contenido), tamanoMaxAcuse)
		if err != nil {
			return models.AcuseCompuesto{}, fmt.Errorf("guardar acuse: %w", err)
		}
		titulo, edicion := s.Convocatoria.TituloES, s.Edicion.NombreES
		if s.Idioma == models.IdiomaEN {
			if s.Convocatoria.TituloEN != "" {
				titulo = s.Convocatoria.TituloEN
			}
			if s.Edicion.NombreEN != "" {
				edicion = s.Edicion.NombreEN
			}
		}
		correo := views.RenderCorreo(plantilla, s.Titular.Email, map[string]string{
			"nombre": s.Titular.Nombre, "folio": s.Folio, "convocatoria": titulo, "edicion": edicion,
			"liga": a.urlBase + "/postulaciones/" + s.Folio,
		})
		compuesto := models.AcuseCompuesto{Blob: blob, Correo: correo}
		for _, revisor := range revisores {
			idioma := revisor.Idioma
			if idioma != models.IdiomaEN {
				idioma = models.IdiomaES
			}
			tituloParaRevisor, edicionParaRevisor := s.Convocatoria.TituloES, s.Edicion.NombreES
			if idioma == models.IdiomaEN && s.Convocatoria.TituloEN != "" {
				tituloParaRevisor = s.Convocatoria.TituloEN
			}
			if idioma == models.IdiomaEN && s.Edicion.NombreEN != "" {
				edicionParaRevisor = s.Edicion.NombreEN
			}
			compuesto.Avisos = append(compuesto.Avisos, views.RenderCorreo(avisos[idioma], revisor.Email, map[string]string{
				"nombre": revisor.Nombre, "folio": s.Folio, "convocatoria": tituloParaRevisor, "edicion": edicionParaRevisor,
				"liga": a.urlBase + "/postulaciones/" + s.Folio,
			}))
		}
		return compuesto, nil
	}
	err = a.datos.EnviarPostulacion(r.Context(), a.actorDe(r), sesion.Usuario.ID, id,
		models.ConfirmacionEnvio{Confirmo: entrada.Confirmo, AceptoBases: entrada.AceptoBases}, componer)
	var incompleta *models.ErrorDeEnvio
	if errors.As(err, &incompleta) {
		cuerpo := views.ErrorDeEnvioJSON(a.vistas.T(idiomaDe(r), "api.incompleta"), a.bloqueadoresDeEnvio(r, incompleta.Bloqueadores))
		if err := views.JSON(w, http.StatusUnprocessableEntity, cuerpo); err != nil {
			a.log.Error("no se pudo escribir la respuesta JSON", "error", err)
		}
		return
	}
	if a.falloDatos(w, r, err) {
		return
	}
	a.responderPostulacion(w, r, http.StatusOK, id)
}

// --- Archivos públicos de una convocatoria (RF-CONV-02) -----------------------

// PUT /api/v1/convocatorias/{id}/archivos/{clase}   — carga las bases o la imagen
//
// clase es «bases_pdf_es», «bases_pdf_en» o «imagen». Petición
// multipart/form-data con el campo «archivo».
func (a *App) apiSubirArchivoDeConvocatoria(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	clase := models.ClaseArchivoConvocatoria(r.PathValue("clase"))
	tamanoMax := int64(models.TamanoMaxBasesPDF)
	if clase == models.ArchivoImagen {
		tamanoMax = models.TamanoMaxImagen
	}
	s, ok := a.abrirSubida(w, r, tamanoMax)
	if !ok {
		return
	}
	blob, ok := a.guardarSubida(w, r, s, tamanoMax, func(tamano int64) []models.ErrorCampo {
		return models.ValidarArchivoDeConvocatoria(clase, s.nombre, s.mime, tamano)
	})
	if !ok {
		return
	}
	if a.falloDatos(w, r, a.datos.FijarArchivoDeConvocatoria(r.Context(), a.actorDe(r), id, clase, blob, s.mime, s.nombre)) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusOK, id)
}

// DELETE /api/v1/convocatorias/{id}/archivos/{clase}   — retira las bases o la imagen
func (a *App) apiQuitarArchivoDeConvocatoria(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	clase := models.ClaseArchivoConvocatoria(r.PathValue("clase"))
	if a.falloDatos(w, r, a.datos.FijarArchivoDeConvocatoria(r.Context(), a.actorDe(r), id, clase, models.Blob{}, "", "")) {
		return
	}
	a.responderConfiguracion(w, r, http.StatusOK, id)
}

// archivoPublico sirve un archivo público de una convocatoria visible: las
// bases en PDF o la imagen de cabecera. No exige sesión ni se registra: es
// material de difusión, no del acervo. prefijo encabeza el nombre con que se
// ofrece el archivo.
func (a *App) archivoPublico(w http.ResponseWriter, r *http.Request, clase models.ClaseArchivoConvocatoria, prefijo string) {
	numero, err := strconv.Atoi(r.PathValue("edicion"))
	tipo := models.TipoConvocatoria(r.PathValue("tipo"))
	if err != nil || numero <= 0 || !tipo.Valido() {
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	c, err := a.datos.ConvocatoriaPublica(r.Context(), numero, tipo)
	var archivos map[models.ClaseArchivoConvocatoria]models.Archivo
	if err == nil {
		archivos, err = a.datos.ArchivosDeConvocatoria(r.Context(), c.ID)
	}
	archivo, tiene := archivos[clase]
	switch {
	case errors.Is(err, models.ErrNoEncontrado), err == nil && !tiene:
		a.errorPagina(w, r, http.StatusNotFound)
		return
	case err != nil:
		a.log.Error("no se pudo leer el archivo de la convocatoria", "error", err)
		a.errorPagina(w, r, http.StatusInternalServerError)
		return
	}
	contenido, err := a.almacen.Abrir(archivo.Clave)
	if err != nil {
		a.log.Error("archivo registrado que no está en el almacén", "archivo", archivo.ID, "sha256", archivo.SHA256, "error", err)
		a.errorPagina(w, r, http.StatusNotFound)
		return
	}
	defer contenido.Close()
	cabeceras := w.Header()
	cabeceras.Set("Content-Type", archivo.MIME)
	cabeceras.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s_fimnme%d_%s%s"`,
		prefijo, numero, tipo, models.ExtensionDeMIME(archivo.MIME)))
	// Si el administrador sustituye el archivo cambia su suma, y con ella la
	// etiqueta: el navegador vuelve a pedirlo.
	cabeceras.Set("ETag", `"`+archivo.SHA256+`"`)
	cabeceras.Set("Cache-Control", "public, max-age=300")
	http.ServeContent(w, r, "", time.Time{}, contenido)
}

// GET /convocatorias/{edicion}/{tipo}/bases.pdf   — bases en PDF
//
// Se sirven en el idioma de la visita; con ?idioma=es o ?idioma=en, en ése.
// Si no hay versión en inglés, la página enlaza a la española.
func (a *App) basesPDF(w http.ResponseWriter, r *http.Request) {
	idioma := r.URL.Query().Get("idioma")
	if idioma != models.IdiomaES && idioma != models.IdiomaEN {
		idioma = idiomaDe(r)
	}
	clase := models.ArchivoBasesES
	if idioma == models.IdiomaEN {
		clase = models.ArchivoBasesEN
	}
	a.archivoPublico(w, r, clase, "bases_"+idioma)
}

// GET /convocatorias/{edicion}/{tipo}/imagen   — imagen de cabecera
func (a *App) imagenDeConvocatoria(w http.ResponseWriter, r *http.Request) {
	a.archivoPublico(w, r, models.ArchivoImagen, "imagen")
}
