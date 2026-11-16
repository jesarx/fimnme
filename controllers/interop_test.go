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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Pruebas de extremo a extremo del paso 10a: exportación al portal público,
// anonimización ARCO y alerta de espacio en disco.

// portal baja la exportación pública, sin sesión, y la devuelve como texto y
// como estructura.
func (e *entornoPrueba) portal(parametros string) (string, models.InteropPortal) {
	e.t.Helper()
	respuesta := e.cliente().bajar("/api/v1/interop/convocatoria.json" + parametros)
	if respuesta.Code != http.StatusOK {
		e.t.Fatalf("exportación al portal: %d %s", respuesta.Code, respuesta.Body.String())
	}
	var portal models.InteropPortal
	if err := json.Unmarshal(respuesta.Body.Bytes(), &portal); err != nil {
		e.t.Fatal(err)
	}
	return respuesta.Body.String(), portal
}

// nominaDe devuelve la nómina publicada de la convocatoria de un tipo, o nil.
func nominaDe(portal models.InteropPortal, tipo string) *models.InteropNomina {
	for _, edicion := range portal.Ediciones {
		for _, convocatoria := range edicion.Convocatorias {
			if convocatoria.Tipo == tipo {
				return convocatoria.Nomina
			}
		}
	}
	return nil
}

// LA REGLA DURA DEL PORTAL PÚBLICO (RF-PUB-02 a 04, RN-34): sale la lista
// blanca y nada más; la nómina, sólo después del cierre y por acción
// explícita del administrador, nunca por cambiar de etapa.
func TestPortalPublico(t *testing.T) {
	x := evaluada(t)
	e := x.e
	publicacion := fmt.Sprintf("/api/v1/convocatorias/%d/publicacion", x.interpretes)
	x.admin.exige(http.StatusOK, "PUT", x.rutaLuis+"/dictamen-final", obj{"dictamen": "seleccionada"})

	// Lo que jamás debe aparecer en la exportación, pase lo que pase.
	listaNegra := []string{
		"@ejemplo.mx", "55 1234 5678", "1990-05-17", "Ana López Ruiz", "Julián Carrillo", "JAL", "mujer",
		"Nota interna de Rita.", "Opinión del jurado", "No cumple las bases.", "realizacion", "puntaje", "sha256",
		"Cuarteto n.º", "vimeo.com", "Piano de cola", "Persona Interna", "FIM50-",
	}
	comprobar := func(momento string) (string, models.InteropPortal) {
		t.Helper()
		cuerpo, portal := e.portal("")
		for _, prohibido := range listaNegra {
			if strings.Contains(cuerpo, prohibido) {
				t.Errorf("%s: la exportación al portal contiene «%s»", momento, prohibido)
			}
		}
		return cuerpo, portal
	}

	// En vivo: bases, calendario y dotaciones de las convocatorias públicas
	// de cada edición (la 50 de la prueba y la 49 de demostración), la más
	// reciente primero.
	cuerpo, portal := comprobar("durante la evaluación")
	if len(portal.Ediciones) != 2 || portal.Ediciones[0].Numero != 50 || portal.Ediciones[1].Numero != 49 || len(portal.Ediciones[0].Convocatorias) != 2 {
		t.Fatalf("ediciones exportadas: %+v", portal.Ediciones)
	}
	for _, convocatoria := range portal.Ediciones[0].Convocatorias {
		if convocatoria.TituloES == "" || !strings.Contains(convocatoria.BasesMDES, "# Bases") || len(convocatoria.Etapas) != 3 || convocatoria.Nomina != nil {
			t.Errorf("convocatoria exportada: %+v", convocatoria)
		}
		if convocatoria.Tipo == "compositores" && (len(convocatoria.Dotaciones) == 0 || convocatoria.Dotaciones[0].NombreES == "") {
			t.Errorf("dotaciones de Compositores: %+v", convocatoria.Dotaciones)
		}
	}
	if !strings.Contains(cuerpo, `"nomina":null`) {
		t.Error("sin publicación, la nómina va explícitamente vacía")
	}

	// Con la convocatoria en curso no se publica; la vista previa sí se ve.
	if campos := x.admin.rechaza("POST", publicacion, obj{"confirmo": true}); !slices.Contains(campos, "convocatoria") {
		t.Errorf("publicar antes del cierre: %v", campos)
	}
	previa := x.admin.exige(http.StatusOK, "GET", publicacion, nil)
	if previa["puede_publicar"] != false || len(lista(previa["vista_previa"].(obj), "seleccionados")) != 1 || len(lista(previa, "publicaciones")) != 0 {
		t.Errorf("vista previa antes del cierre: %v", previa)
	}

	// CERRAR NO PUBLICA: la nómina sigue sin salir.
	x.admin.estado(x.interpretes, "cerrada", "")
	if _, portal = comprobar("recién cerrada"); nominaDe(portal, "interpretes") != nil {
		t.Fatal("cerrar la convocatoria publicó la nómina: debe exigir una acción explícita")
	}
	if campos := x.admin.rechaza("POST", publicacion, obj{"confirmo": false}); !slices.Contains(campos, "confirmo") {
		t.Errorf("publicar sin confirmar: %v", campos)
	}
	for nombre, c := range map[string]*cliente{"observador": x.observador, "revisora": x.revisora, "jurado": x.juradosInt[0], "postulante": x.luis} {
		c.exige(http.StatusForbidden, "POST", publicacion, obj{"confirmo": true})
		if estado, _ := c.api("GET", publicacion, nil); estado != http.StatusForbidden {
			t.Errorf("%s: estado %d al ver la publicación", nombre, estado)
		}
	}
	if _, portal = comprobar("tras los intentos rechazados"); nominaDe(portal, "interpretes") != nil {
		t.Fatal("un intento rechazado publicó la nómina")
	}

	// El administrador publica: sale el nombre artístico, la semblanza y el
	// título de la obra. Nada más de la postulación.
	publicada := x.admin.exige(http.StatusOK, "POST", publicacion, obj{"confirmo": true})
	if len(lista(publicada, "publicaciones")) != 1 || len(lista(publicada, "publicaciones")[0]["sha256"].(string)) != 64 {
		t.Errorf("registro de la publicación: %v", publicada)
	}
	_, portal = comprobar("publicada")
	nomina := nominaDe(portal, "interpretes")
	if nomina == nil || len(nomina.Seleccionados) != 1 {
		t.Fatalf("nómina publicada: %+v", nomina)
	}
	if s := nomina.Seleccionados[0]; s.Nombre != "Ana L." || s.Semblanza != "Semblanza de Ana." || s.Subcategoria != "solista" ||
		len(s.Obras) != 1 || s.Obras[0].Titulo != "Preludio a Colón" || nomina.PublicadaAt == "" {
		t.Errorf("seleccionado publicado: %+v", s)
	}
	if nominaDe(portal, "compositores") != nil {
		t.Error("la otra convocatoria no publicó nada")
	}

	// Lo publicado queda congelado: cambiar el dictamen no cambia el portal
	// hasta que se vuelva a publicar.
	x.admin.exige(http.StatusOK, "PUT", x.rutaLuis+"/dictamen-final", obj{"dictamen": "no_seleccionada"})
	if _, portal = comprobar("dictamen cambiado, sin republicar"); nominaDe(portal, "interpretes") == nil {
		t.Error("lo publicado no debe cambiar sin una nueva publicación")
	}
	x.admin.exige(http.StatusOK, "POST", publicacion, obj{"confirmo": true})
	if _, portal = comprobar("republicada sin seleccionados"); nominaDe(portal, "interpretes") != nil {
		t.Error("tras republicar sin seleccionados la nómina debe salir vacía")
	}
	// Publicar y retirar.
	x.admin.exige(http.StatusOK, "PUT", x.rutaLuis+"/dictamen-final", obj{"dictamen": "seleccionada"})
	x.admin.exige(http.StatusOK, "POST", publicacion, obj{"confirmo": true})
	if _, portal = comprobar("vuelta a publicar"); nominaDe(portal, "interpretes") == nil {
		t.Error("la nómina debió volver a salir")
	}
	retirada := x.admin.exige(http.StatusOK, "POST", publicacion, obj{"confirmo": true, "retirar": true})
	if _, portal = comprobar("retirada"); nominaDe(portal, "interpretes") != nil || len(lista(retirada, "publicaciones")) != 4 {
		t.Errorf("la nómina retirada sigue saliendo, o falta su registro: %v", retirada["publicaciones"])
	}

	// Una convocatoria en borrador no sale, ni su edición si no tiene otra.
	nueva := x.admin.exige(http.StatusCreated, "POST", "/api/v1/ediciones", obj{"numero": 51, "anio": 2029, "nombre_es": "Edición 51 · 2029"})
	x.admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/ediciones/%d/convocatorias", id(nueva["id"])),
		obj{"tipo": "interpretes", "titulo_es": "Convocatoria en preparación", "bases_md_es": "# Borrador secreto"})
	if cuerpo, portal = comprobar("con una convocatoria en borrador"); len(portal.Ediciones) != 2 || strings.Contains(cuerpo, "Borrador secreto") {
		t.Errorf("una convocatoria en borrador salió al portal: %d ediciones", len(portal.Ediciones))
	}

	// Por edición; y cada publicación queda en la bitácora, sin nombres.
	if _, soloUna := e.portal("?edicion=50"); len(soloUna.Ediciones) != 1 || soloUna.Ediciones[0].Numero != 50 {
		t.Errorf("exportación de la edición 50: %+v", soloUna.Ediciones)
	}
	if _, ninguna := e.portal("?edicion=12"); len(ninguna.Ediciones) != 0 {
		t.Errorf("exportación de una edición que no existe: %+v", ninguna.Ediciones)
	}
	e.cliente().exige(http.StatusBadRequest, "GET", "/api/v1/interop/convocatoria.json?edicion=x", nil)
	if asientos := e.asientos("interop.publicar"); len(asientos) != 4 || strings.Contains(fmt.Sprint(asientos), "Ana") {
		t.Errorf("bitácora de publicaciones: %d asientos", len(asientos))
	}
}

// LA ÚNICA OPERACIÓN DESTRUCTIVA (RN-28): borra los datos personales,
// conserva obra, dictamen y estadísticas, exige confirmación doble y queda
// en la bitácora con su fundamento.
func TestAnonimizacion(t *testing.T) {
	x := evaluada(t)
	e, ctx := x.e, context.Background()
	almacen := e.almacen.(*models.AlmacenEnDisco)
	ana := x.admin.exige(http.StatusOK, "GET", x.rutaAna, nil)
	folioAna, folioLuis := ana["folio"].(string), x.admin.exige(http.StatusOK, "GET", x.rutaLuis, nil)["folio"].(string)
	solicitud := obj{"folio": folioAna, "fundamento": "Solicitud ARCO 2026/014", "confirmo": true}
	edicion := id(x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", x.interpretes), nil)["edicion"].(obj)["id"])
	// agregados reúne lo que la anonimización no debe alterar.
	agregados := func() string {
		estadisticas := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/ediciones/%d/estadisticas", edicion), nil)
		tablero := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/tablero", x.interpretes), nil)
		matriz := x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/resultados", x.interpretes), nil)
		return fmt.Sprint(estadisticas["postulaciones"], estadisticas["por_genero"], estadisticas["por_entidad"], estadisticas["por_pais"],
			estadisticas["por_nacionalidad"], estadisticas["por_edad"], estadisticas["por_estreno"], estadisticas["duracion_media_min"],
			tablero["por_estado"], lista(matriz, "filas")[0]["total"])
	}
	antes := agregados()
	registros, err := e.repo.ArchivosRegistrados(ctx)
	if err != nil {
		t.Fatal(err)
	}
	verificacionAntes, err := models.VerificarAcervo(ctx, registros, almacen, false)
	if err != nil || !verificacionAntes.Integro() {
		t.Fatalf("acervo antes: %+v %v", verificacionAntes, err)
	}
	if hallados := x.admin.exige(http.StatusOK, "GET", "/api/v1/archivo?q=ana%40ejemplo.mx", nil); hallados["total"] != float64(3) {
		t.Fatalf("antes, las tres postulaciones se encuentran por correo: %v", hallados["total"])
	}

	// Luis sigue en proceso: no puede anonimizarse todavía. Ana fue
	// descartada: sí. Pero sólo el administrador, y con todo en regla.
	if campos := x.admin.rechaza("POST", x.rutaLuis+"/anonimizacion", obj{"folio": folioLuis, "fundamento": "Solicitud ARCO 2026/015", "confirmo": true}); !slices.Contains(campos, "convocatoria") {
		t.Errorf("anonimizar en pleno proceso: %v", campos)
	}
	for nombre, c := range map[string]*cliente{"observador": x.observador, "revisora": x.revisora, "jurado": x.juradosInt[0], "la propia titular": x.ana} {
		if estado, _ := c.api("POST", x.rutaAna+"/anonimizacion", solicitud); estado != http.StatusForbidden {
			t.Errorf("%s: estado %d al anonimizar", nombre, estado)
		}
	}
	for campo, mala := range map[string]obj{
		"folio":      {"folio": folioLuis, "fundamento": "Solicitud ARCO 2026/014", "confirmo": true},
		"confirmo":   {"folio": folioAna, "fundamento": "Solicitud ARCO 2026/014"},
		"fundamento": {"folio": folioAna, "fundamento": "ARCO", "confirmo": true},
	} {
		if campos := x.admin.rechaza("POST", x.rutaAna+"/anonimizacion", mala); !slices.Contains(campos, campo) {
			t.Errorf("solicitud sin %s: %v", campo, campos)
		}
	}
	if sigue := x.admin.exige(http.StatusOK, "GET", x.rutaAna, nil); sigue["campos"].(obj)["datos.telefono"] != "55 1234 5678" || sigue["anonimizada_at"] != nil {
		t.Fatal("una solicitud rechazada borró datos")
	}

	// Se anonimiza.
	hecho := x.admin.exige(http.StatusOK, "POST", x.rutaAna+"/anonimizacion", solicitud)
	if hecho["folio"] != folioAna || hecho["archivos_purgados"] != float64(3) || hecho["archivos_conservados"] != float64(1) ||
		hecho["archivos_por_purgar"] != float64(0) || hecho["cuenta_anonimizada"] != false {
		t.Fatalf("resultado de la anonimización: %v", hecho)
	}

	// Lo personal ya no está; lo estadístico y la obra, sí.
	despues := x.admin.exige(http.StatusOK, "GET", x.rutaAna, nil)
	campos := despues["campos"].(obj)
	if campos["datos.nombre_completo"] != models.TextoAnonimizado || campos["datos.fecha_nacimiento"] != "1990-01-01" ||
		campos["datos.genero"] != "mujer" || campos["datos.entidad_residencia"] != "JAL" || campos["datos.nacionalidad"] != "MX" ||
		despues["estado"] != "descartada" || despues["folio"] != folioAna || despues["anonimizada_at"] == nil || len(lista(despues, "videos")) != 0 {
		t.Errorf("postulación anonimizada: %v", campos)
	}
	for _, borrado := range []string{"datos.nombre_artistico", "datos.telefono", "datos.email_contacto", "datos.semblanza"} {
		if valor, hay := campos[borrado]; hay && valor != nil && valor != "" {
			t.Errorf("%s debió borrarse: %v", borrado, valor)
		}
	}
	obra := lista(despues, "obras")[0]["campos"].(obj)
	if obra["obra.titulo"] != "Preludio a Colón" || obra["obra.compositor"] != "Julián Carrillo" || (obra["obra.liga_media"] != nil && obra["obra.liga_media"] != "") {
		t.Errorf("obra de la postulación anonimizada: %v", obra)
	}
	// Archivos: la partitura se conserva y se baja; la fotografía, la
	// autorización y el acuse ya no están en el disco.
	for clave, estado := range map[string]int{"partitura": http.StatusOK, "fotografia": http.StatusNotFound, "autorizacion": http.StatusNotFound} {
		documento := documentoDe(t, despues, clave)
		if respuesta := x.admin.bajar(fmt.Sprintf("/api/v1/documentos/%d/archivo", id(documento["id"]))); respuesta.Code != estado {
			t.Errorf("descarga de %s tras anonimizar: %d", clave, respuesta.Code)
		}
	}
	if respuesta := x.admin.bajar(x.rutaAna + "/acuses/1"); respuesta.Code != http.StatusNotFound {
		t.Errorf("el acuse debió purgarse: %d", respuesta.Code)
	}
	// El acervo sigue íntegro: lo purgado no cuenta como perdido ni queda en disco.
	registros, err = e.repo.ArchivosRegistrados(ctx)
	if err != nil {
		t.Fatal(err)
	}
	verificacion, err := models.VerificarAcervo(ctx, registros, almacen, false)
	if err != nil || !verificacion.Integro() || verificacion.Registrados != verificacionAntes.Registrados-3 || len(verificacion.Huerfanos) != 0 {
		t.Errorf("acervo tras anonimizar: %+v %v", verificacion, err)
	}

	// Ya no se encuentra por su correo ni por la cuenta que la envió, y ni el
	// expediente ni los reportes dicen de quién era.
	if hallados := x.admin.exige(http.StatusOK, "GET", "/api/v1/archivo?q=ana%40ejemplo.mx", nil); hallados["total"] != float64(2) {
		t.Errorf("tras anonimizar se encuentran %v postulaciones por el correo", hallados["total"])
	}
	if hallados := x.admin.exige(http.StatusOK, "GET", "/api/v1/archivo?q="+folioAna, nil); hallados["total"] != float64(1) {
		t.Error("la postulación anonimizada debe seguir encontrándose por su folio")
	}
	contiene(t, "expediente anonimizado", expedienteDe(t, x.admin, x.rutaAna, ""), []string{folioAna, models.TextoAnonimizado, "Preludio a Colón", "Descartada", "No cumple las bases."},
		[]string{"Ana López", "ana@ejemplo.mx", "55 1234 5678", "Semblanza de Ana.", "1990-05-17", "vimeo.com"})
	reporte := csvDe(t, x.admin, fmt.Sprintf("/api/v1/convocatorias/%d/reporte?campos=folio,datos.nombre_completo,usuario.nombre,usuario.email,datos.genero&estado=descartada", x.interpretes))
	if len(reporte) != 2 || reporte[1] != folioAna+","+models.TextoAnonimizado+","+models.TextoAnonimizado+",,Mujer" {
		t.Errorf("reporte de la postulación anonimizada: %v", reporte)
	}
	// Los correos que se le enviaron por esa postulación ya no la nombran.
	for _, correo := range e.correos() {
		if correo.PostulacionID == id(despues["id"]) && (correo.Destinatario != "anonimizado@anonimizado.invalid" || correo.Texto != "" || len(correo.Adjuntos) != 0) {
			t.Errorf("correo sin anonimizar: %+v", correo)
		}
	}

	// LOS AGREGADOS NO CAMBIAN.
	if ahora := agregados(); ahora != antes {
		t.Errorf("la anonimización alteró los agregados.\nantes:   %s\ndespués: %s", antes, ahora)
	}
	// No se repite; su titular conserva la cuenta y ve su postulación.
	if campos := x.admin.rechaza("POST", x.rutaAna+"/anonimizacion", solicitud); !slices.Contains(campos, "estado") {
		t.Errorf("anonimizar dos veces: %v", campos)
	}
	if propia := x.ana.exige(http.StatusOK, "GET", x.rutaAna, nil); propia["anonimizada_at"] == nil {
		t.Error("la titular debe poder ver que su postulación fue anonimizada")
	}
	// En la bitácora, con su fundamento y sin datos de la persona.
	asientos := e.asientos("arco.anonimizar")
	if len(asientos) != 1 || !strings.Contains(fmt.Sprint(asientos[0]), "Solicitud ARCO 2026/014") || strings.Contains(fmt.Sprint(asientos[0]), "López") {
		t.Errorf("bitácora de la anonimización: %v", asientos)
	}

	// Con la convocatoria cerrada puede anonimizarse a Luis, con su cuenta:
	// deja de poder ingresar.
	x.admin.estado(x.interpretes, "cerrada", "")
	conCuenta := x.admin.exige(http.StatusOK, "POST", x.rutaLuis+"/anonimizacion",
		obj{"folio": folioLuis, "fundamento": "Solicitud ARCO 2026/015", "confirmo": true, "con_cuenta": true})
	if conCuenta["cuenta_anonimizada"] != true {
		t.Errorf("anonimización con cuenta: %v", conCuenta)
	}
	x.luis.exige(http.StatusUnauthorized, "GET", "/api/v1/sesion", nil)
	esperar(t, e.cliente().post("/cuenta/ingresar", url.Values{"email": {"luis@ejemplo.mx"}, "password": {passwordPrueba}}), http.StatusUnauthorized)
	if cuentas := fmt.Sprint(x.admin.exige(http.StatusOK, "GET", "/api/v1/usuarios?q=luis", nil)); strings.Contains(cuentas, "luis@ejemplo.mx") {
		t.Errorf("la cuenta anonimizada conserva su correo: %s", cuentas)
	}
	// La evaluación de Luis sigue en la matriz: el resultado no se pierde.
	if filas := lista(x.admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d/resultados", x.interpretes), nil), "filas"); len(filas) != 1 ||
		filas[0]["total"] != float64(27) || filas[0]["nombre"] != models.TextoAnonimizado {
		t.Errorf("matriz tras anonimizar: %v", filas)
	}
}

// La cuenta no se anonimiza mientras tenga otras postulaciones.
func TestAnonimizacionConOtrasPostulaciones(t *testing.T) {
	x := nuevoArchivoDePrueba(t)
	// Luis, que ya postuló en Intérpretes, empieza otra en Compositores.
	x.luis.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/postulaciones", x.compositores), obj{})
	x.admin.estado(x.interpretes, "cerrada", "")
	folio := x.admin.exige(http.StatusOK, "GET", x.rutaLuis, nil)["folio"].(string)
	solicitud := obj{"folio": folio, "fundamento": "Solicitud ARCO 2026/020", "confirmo": true, "con_cuenta": true}
	if campos := x.admin.rechaza("POST", x.rutaLuis+"/anonimizacion", solicitud); !slices.Contains(campos, "con_cuenta") {
		t.Fatalf("anonimizar la cuenta con otra postulación viva: %v", campos)
	}
	// Nada se tocó: ni la postulación ni la cuenta.
	if intacta := x.luis.exige(http.StatusOK, "GET", x.rutaLuis, nil); intacta["anonimizada_at"] != nil || intacta["campos"].(obj)["datos.nombre_completo"] != "Luis Núñez Carrillo" {
		t.Error("una anonimización rechazada modificó la postulación")
	}
	// Sin la cuenta, sí; y Luis sigue pudiendo trabajar en su otra postulación.
	solicitud["con_cuenta"] = false
	x.admin.exige(http.StatusOK, "POST", x.rutaLuis+"/anonimizacion", solicitud)
	if sesion := x.luis.exige(http.StatusOK, "GET", "/api/v1/sesion", nil); sesion["usuario"] == nil {
		t.Error("la cuenta debió conservarse")
	}
	if obtenido := x.archivo(x.admin, "q=nunez"); obtenido != "" {
		t.Errorf("la postulación anonimizada aún se encuentra por nombre: %q", obtenido)
	}
}

// El vigilante de disco alerta por debajo del umbral, una vez al día.
func TestAlertaDeDisco(t *testing.T) {
	e := nuevoEntorno(t)
	e.activarCuentaInterna("admin@ejemplo.mx", models.PerfilAdminSistema)
	e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	libre := uint64(50 << 30)
	vigilante := &TrabajadorDisco{Datos: e.repo, Vistas: e.vistas(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Directorio: "/var/lib/fimnme",
		MinimoLibre: 10 << 30, Ahora: e.reloj.Ahora, EspacioLibre: func(string) (uint64, error) { return libre, nil }}
	alertas := func() []models.CorreoSaliente { return e.correosDe(models.PlantillaAlertaDisco) }

	if vigilante.Procesar(context.Background()) || len(alertas()) != 0 {
		t.Fatal("con espacio de sobra no se alerta")
	}
	// Baja del umbral: se avisa a los administradores, no al observador.
	libre = 8<<30 + 200<<20
	if !vigilante.Procesar(context.Background()) || len(alertas()) != 1 {
		t.Fatalf("por debajo del umbral debe alertar: %d alertas", len(alertas()))
	}
	if alerta := alertas()[0]; alerta.Destinatario != "admin@ejemplo.mx" || !strings.Contains(alerta.Asunto, "8.2 GB") ||
		!strings.Contains(alerta.Texto, "10.0 GB") || !strings.Contains(alerta.Texto, "/var/lib/fimnme") {
		t.Errorf("alerta de disco: %+v", alerta)
	}
	// Mientras siga bajo, una vez al día.
	e.reloj.Avanzar(23 * time.Hour)
	if vigilante.Procesar(context.Background()) || len(alertas()) != 1 {
		t.Error("la alerta no debe repetirse antes de un día")
	}
	e.reloj.Avanzar(2 * time.Hour)
	if !vigilante.Procesar(context.Background()) || len(alertas()) != 2 {
		t.Error("al día siguiente la alerta se repite")
	}
	// Si se recupera y vuelve a bajar, avisa de inmediato.
	libre = 40 << 30
	vigilante.Procesar(context.Background())
	libre = 1 << 30
	if !vigilante.Procesar(context.Background()) || len(alertas()) != 3 {
		t.Error("tras recuperarse, una nueva caída alerta de inmediato")
	}
	// Con umbral 0 no se vigila: Ejecutar vuelve enseguida.
	terminado := make(chan struct{})
	go func() {
		(&TrabajadorDisco{Log: vigilante.Log}).Ejecutar(context.Background())
		close(terminado)
	}()
	select {
	case <-terminado:
	case <-time.After(2 * time.Second):
		t.Error("sin umbral el vigilante no debe quedarse esperando")
	}
	// El espacio real del sistema se puede medir.
	if real, err := models.EspacioLibre(t.TempDir()); err != nil || real == 0 {
		t.Errorf("medición real del espacio libre: %d %v", real, err)
	}
}
