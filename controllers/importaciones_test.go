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
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jesarx/fimnme/models"
)

// Prueba de extremo a extremo de la importación asistida (§5.15): analizar no
// escribe nada, nada entra sin que una persona resuelva cada conflicto, y el
// Commit entra entero o no entra.

// analizar sube una hoja y devuelve el estado HTTP y la respuesta.
func (c *cliente) analizar(tipo string, convocatoriaID int64, nombre string, contenido []byte) (int, obj) {
	c.t.Helper()
	return c.subir("POST", "/api/v1/importaciones", [][2]string{{"tipo", tipo}, {"convocatoria_id", fmt.Sprint(convocatoriaID)}}, nombre, contenido)
}

// importacion sube una hoja, exige que el análisis se acepte y devuelve la
// importación y su ruta.
func (c *cliente) importacion(tipo string, convocatoriaID int64, nombre string, contenido []byte) (obj, string) {
	c.t.Helper()
	estado, respuesta := c.analizar(tipo, convocatoriaID, nombre, contenido)
	if estado != http.StatusCreated {
		c.t.Fatalf("análisis de %s: estado %d: %v", nombre, estado, respuesta)
	}
	return respuesta, fmt.Sprintf("/api/v1/importaciones/%d", id(respuesta["id"]))
}

// conflictoDe busca en una importación el conflicto de un renglón.
func conflictoDe(t *testing.T, importacion obj, fila int) obj {
	t.Helper()
	for _, conflicto := range lista(importacion, "conflictos") {
		if conflicto["fila"] == float64(fila) {
			return conflicto
		}
	}
	t.Fatalf("no hay conflicto en la fila %d: %v", fila, importacion["conflictos"])
	return nil
}

// xlsxDeCuentas arma un .xlsx con una hoja de cuentas, con textos en la celda.
func xlsxDeCuentas(t *testing.T, renglones ...[]string) []byte {
	t.Helper()
	var hoja strings.Builder
	for _, renglon := range renglones {
		hoja.WriteString("<row>")
		for _, celda := range renglon {
			hoja.WriteString(`<c t="inlineStr"><is><t>` + celda + `</t></is></c>`)
		}
		hoja.WriteString("</row>")
	}
	var salida bytes.Buffer
	escritor := zip.NewWriter(&salida)
	parte, err := escritor.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(parte, `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>%s</sheetData></worksheet>`, hoja.String())
	if err := escritor.Close(); err != nil {
		t.Fatal(err)
	}
	return salida.Bytes()
}

func TestImportacionDeDotaciones(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, compositores, cuarteto, _ := e.convocatoriasAbiertas()
	observador := e.activarCuentaInterna("observador@ejemplo.mx", models.PerfilCoordinacionNacional)
	// dotaciones devuelve los nombres del catálogo de Compositores.
	dotaciones := func() string {
		var nombres []string
		for _, d := range lista(admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", compositores), nil), "dotaciones") {
			nombres = append(nombres, d["nombre_es"].(string))
		}
		return strings.Join(nombres, " | ")
	}
	catalogoInicial := dotaciones()

	// --- Lo que no puede analizarse, y por qué.
	for nombre, caso := range map[string]struct {
		tipo         string
		convocatoria int64
		contenido    string
		campo        string
	}{
		"tipo que no existe":         {"postulaciones", compositores, "folio\nFIM50-INT-0001\n", "tipo"},
		"dotaciones en Intérpretes":  {"dotaciones", interpretes, "nombre_es\nDúo\n", "tipo"},
		"sin la columna obligatoria": {"dotaciones", compositores, "nombre_en\nDuo\n", "fila 1"},
		"columna desconocida":        {"dotaciones", compositores, "nombre_es,precio\nDúo,3\n", "fila 1"},
		"renglón repetido":           {"dotaciones", compositores, "nombre_es\nDúo\nTrío\nduo\n", "fila 4"},
		"hoja sin datos":             {"dotaciones", compositores, "nombre_es\n", "archivo"},
		"archivo que no es una hoja": {"dotaciones", compositores, "PK\x03\x04 esto no es un libro", "archivo"},
		"CSV que no está en UTF-8":   {"dotaciones", compositores, "nombre_es\nD\xFAo\n", "archivo"},
	} {
		estado, respuesta := admin.analizar(caso.tipo, caso.convocatoria, "hoja.csv", []byte(caso.contenido))
		if estado != http.StatusUnprocessableEntity || !strings.Contains(fmt.Sprint(respuesta), "campo:"+caso.campo) {
			t.Errorf("%s: estado %d, %v", nombre, estado, respuesta)
		}
	}
	if estado, _ := admin.analizar("dotaciones", 999, "hoja.csv", []byte("nombre_es\nDúo\n")); estado != http.StatusNotFound {
		t.Errorf("convocatoria inexistente: %d", estado)
	}
	if estado, _ := observador.analizar("dotaciones", compositores, "hoja.csv", []byte("nombre_es\nDúo\n")); estado != http.StatusForbidden {
		t.Errorf("el observador no importa: %d", estado)
	}

	// --- Fase Analyze: cuatro renglones, uno con correspondencia unívoca.
	hoja := []byte("\xEF\xBB\xBFnombre_es;nombre_en;descripcion_es\r\n" +
		"Cuarteto de cuerdas;String quartet;Dos violines, viola y violonchelo\r\n" + // idéntica a una existente
		"Quinteto de alientos;Wind quintet;\r\n" + // nueva
		"Cuarteto;Quartet;\r\n" + // se parece a una existente
		"Orquesta de cámara;;\r\n") // nueva
	importacion, ruta := admin.importacion("dotaciones", compositores, "dotaciones 2028.csv", hoja)
	efecto := importacion["efecto"].(obj)
	if importacion["estado"] != "esperando_resolucion" || importacion["filas"] != float64(4) || len(lista(importacion, "conflictos")) != 3 ||
		importacion["puede_confirmarse"] != false || importacion["sha256"] != sumaDe(hoja) ||
		fmt.Sprint(efecto["sin_conflicto"], efecto["sin_resolver"], efecto["crean"]) != "1 3 0" {
		t.Fatalf("resultado del análisis: %v", importacion)
	}
	parecido := conflictoDe(t, importacion, 4)
	if candidatos := lista(parecido, "candidatos"); len(candidatos) != 1 || id(candidatos[0]["id"]) != cuarteto || candidatos[0]["motivo"] != "nombre_parecido" ||
		len(lista(conflictoDe(t, importacion, 3), "candidatos")) != 0 {
		t.Errorf("candidatos del análisis: %v", importacion["conflictos"])
	}
	// ANALIZAR NO ESCRIBE NADA EN EL DOMINIO (RF-IMP-02).
	if dotaciones() != catalogoInicial {
		t.Fatalf("el análisis modificó el catálogo: %s", dotaciones())
	}
	// Sin resolver, no se confirma (RF-IMP-04).
	if campos := admin.rechaza("POST", ruta+"/confirmacion", obj{"confirmo": true}); !slices.Contains(campos, "conflictos") {
		t.Errorf("confirmar con conflictos pendientes: %v", campos)
	}

	// --- Resolución, por una persona.
	rutaParecido := fmt.Sprintf("%s/conflictos/%d", ruta, id(parecido["id"]))
	for nombre, cuerpo := range map[string]obj{
		"resolución inventada":                  {"resolucion": "fusionar"},
		"vincular sin elegir":                   {"resolucion": "vincular"},
		"vincular a una dotación que no existe": {"resolucion": "vincular", "entidad_id": 999},
	} {
		if campos := admin.rechaza("PUT", rutaParecido, cuerpo); len(campos) == 0 {
			t.Errorf("%s debió rechazarse", nombre)
		}
	}
	admin.exige(http.StatusNotFound, "PUT", ruta+"/conflictos/999", obj{"resolucion": "omitir"})
	observador.exige(http.StatusForbidden, "PUT", rutaParecido, obj{"resolucion": "omitir"})
	// «Cuarteto» ES el cuarteto de cuerdas: se vincula. La decisión queda
	// con quién la tomó, y puede cambiarse mientras no se confirme.
	admin.exige(http.StatusOK, "PUT", rutaParecido, obj{"resolucion": "omitir"})
	resuelta := admin.exige(http.StatusOK, "PUT", rutaParecido, obj{"resolucion": "vincular", "entidad_id": cuarteto})
	if c := conflictoDe(t, resuelta, 4); c["resolucion"] != "vincular" || id(c["entidad_id"]) != cuarteto || c["resuelto_por"] == nil || c["resuelto_at"] == nil {
		t.Errorf("conflicto resuelto: %v", c)
	}
	// Los dos que no se parecen a nada, de una vez.
	resuelta = admin.exige(http.StatusOK, "POST", ruta+"/resoluciones", obj{"resolucion": "crear_nuevo"})
	efecto = resuelta["efecto"].(obj)
	if resuelta["puede_confirmarse"] != true || fmt.Sprint(efecto["crean"], efecto["vinculan"], efecto["omiten"], efecto["sin_resolver"], efecto["sin_conflicto"]) != "2 1 0 0 1" {
		t.Fatalf("vista previa del Commit: %v", resuelta["efecto"])
	}
	if dotaciones() != catalogoInicial {
		t.Fatal("resolver conflictos no debe escribir en el dominio")
	}

	// --- Fase Commit.
	if campos := admin.rechaza("POST", ruta+"/confirmacion", obj{"confirmo": false}); !slices.Contains(campos, "confirmo") {
		t.Errorf("confirmar sin confirmar: %v", campos)
	}
	confirmada := admin.exige(http.StatusOK, "POST", ruta+"/confirmacion", obj{"confirmo": true})
	if confirmada["estado"] != "confirmada" || confirmada["confirmada_at"] == nil || confirmada["puede_confirmarse"] != false {
		t.Errorf("importación confirmada: %v", confirmada)
	}
	if dotaciones() != "Cuarteto de cuerdas | Obra electroacústica | Quinteto de alientos | Orquesta de cámara" {
		t.Errorf("catálogo tras importar: %s", dotaciones())
	}
	// La de nombre idéntico recibió lo que le faltaba, sin perder lo que tenía.
	for _, d := range lista(admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", compositores), nil), "dotaciones") {
		if id(d["id"]) == cuarteto && (d["nombre_en"] != "String quartet" || d["descripcion_es"] != "Dos violines, viola y violonchelo") {
			t.Errorf("dotación vinculada: %v", d)
		}
	}
	// En la bitácora, con el SHA-256 del archivo original (RF-IMP-09).
	if asientos := e.asientos("importacion.confirmar"); len(asientos) != 1 || !strings.Contains(fmt.Sprint(asientos[0]), sumaDe(hoja)) {
		t.Errorf("bitácora de la importación: %v", asientos)
	}
	// Confirmada, ya no se toca.
	for nombre, intento := range map[string]func() []string{
		"confirmar otra vez": func() []string { return admin.rechaza("POST", ruta+"/confirmacion", obj{"confirmo": true}) },
		"resolver":           func() []string { return admin.rechaza("PUT", rutaParecido, obj{"resolucion": "omitir"}) },
		"cancelar":           func() []string { return admin.rechaza("DELETE", ruta, nil) },
	} {
		if campos := intento(); !slices.Contains(campos, "estado") {
			t.Errorf("%s una importación confirmada: %v", nombre, campos)
		}
	}

	// --- Todo o nada (RN-29). Dos dotaciones nuevas; entre el análisis y el
	// Commit alguien crea la segunda a mano. No entra ninguna.
	_, otra := admin.importacion("dotaciones", compositores, "mas.csv", []byte("nombre_es\nDúo\nTrío\n"))
	admin.exige(http.StatusOK, "POST", otra+"/resoluciones", obj{"resolucion": "crear_nuevo"})
	admin.exige(http.StatusCreated, "POST", fmt.Sprintf("/api/v1/convocatorias/%d/dotaciones", compositores), obj{"nombre_es": "Trío", "orden": 9})
	antes := dotaciones()
	if campos := admin.rechaza("POST", otra+"/confirmacion", obj{"confirmo": true}); !slices.Contains(campos, "fila 3") {
		t.Errorf("el Commit debió fallar señalando el renglón: %v", campos)
	}
	if dotaciones() != antes || strings.Contains(dotaciones(), "Dúo") {
		t.Errorf("un Commit fallido dejó algo escrito: %s", dotaciones())
	}
	// Cancelar: queda cerrada y nada cambia.
	if cancelada := admin.exige(http.StatusOK, "DELETE", otra, nil); cancelada["estado"] != "cancelada" || dotaciones() != antes {
		t.Errorf("importación cancelada: %v", cancelada["estado"])
	}
}

func TestImportacionDeCuentas(t *testing.T) {
	e := nuevoEntorno(t)
	admin, interpretes, _, _, _ := e.convocatoriasAbiertas()
	existente := e.crearInterno(admin, "existente@ejemplo.mx") // «Persona Interna»
	jurados := func() int {
		n := 0
		for _, a := range lista(admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", interpretes), nil), "asignaciones") {
			if a["perfil"] == "jurado" && a["activo"] == true {
				n++
			}
		}
		return n
	}
	cuentas := func(texto string) int {
		return int(admin.exige(http.StatusOK, "GET", "/api/v1/usuarios?q="+texto, nil)["total"].(float64))
	}
	activaciones := len(e.correosDe(models.PlantillaActivacion))

	// Un .xlsx con tres jurados: una persona nueva, una cuenta que ya existe
	// (mismo correo) y alguien con el mismo nombre que ella y otro correo.
	libro := xlsxDeCuentas(t, []string{"email", "nombre", "apellidos", "idioma"},
		[]string{"nora@ejemplo.mx", "Nora", "Paz", "en"},
		[]string{"Existente@Ejemplo.mx", "Persona", "Interna", ""},
		[]string{"homonimo@ejemplo.mx", "persona", "INTERNA", ""})
	importacion, ruta := admin.importacion("usuarios_jurado", interpretes, "jurado 2028.xlsx", libro)
	// TODO renglón es un conflicto: nunca se infiere que una cuenta es la
	// persona del renglón, ni cuando coincide el correo (RN-30).
	if len(lista(importacion, "conflictos")) != 3 || importacion["puede_confirmarse"] != false {
		t.Fatalf("análisis de cuentas: %v", importacion)
	}
	nueva, mismoCorreo, homonimo := conflictoDe(t, importacion, 2), conflictoDe(t, importacion, 3), conflictoDe(t, importacion, 4)
	if len(lista(nueva, "candidatos")) != 0 || lista(mismoCorreo, "candidatos")[0]["motivo"] != "mismo_correo" || id(lista(mismoCorreo, "candidatos")[0]["id"]) != existente ||
		lista(homonimo, "candidatos")[0]["motivo"] != "mismo_nombre" || mismoCorreo["columna"] != "identidad" {
		t.Fatalf("candidatos de cuentas: %v", importacion["conflictos"])
	}
	if jurados() != 0 || cuentas("nora") != 0 {
		t.Fatal("el análisis escribió en el dominio")
	}
	conflicto := func(c obj) string { return fmt.Sprintf("%s/conflictos/%d", ruta, id(c["id"])) }
	// No puede crearse otra cuenta con un correo ya registrado; ni vincular
	// a una cuenta que no sea candidata (la del administrador, por ejemplo).
	if campos := admin.rechaza("PUT", conflicto(mismoCorreo), obj{"resolucion": "crear_nuevo"}); !slices.Contains(campos, "resolucion") {
		t.Errorf("crear con correo repetido: %v", campos)
	}
	if campos := admin.rechaza("PUT", conflicto(homonimo), obj{"resolucion": "vincular", "entidad_id": 1}); !slices.Contains(campos, "entidad_id") {
		t.Errorf("vincular a una cuenta cualquiera: %v", campos)
	}
	// Una persona decide: la del mismo correo ES esa cuenta; el homónimo es
	// OTRA persona; Nora es nueva.
	admin.exige(http.StatusOK, "PUT", conflicto(mismoCorreo), obj{"resolucion": "vincular", "entidad_id": existente})
	admin.exige(http.StatusOK, "PUT", conflicto(homonimo), obj{"resolucion": "crear_nuevo"})
	lista2 := admin.exige(http.StatusOK, "POST", ruta+"/resoluciones", obj{"resolucion": "crear_nuevo"})
	if efecto := lista2["efecto"].(obj); fmt.Sprint(efecto["crean"], efecto["vinculan"], efecto["sin_resolver"]) != "2 1 0" || lista2["puede_confirmarse"] != true {
		t.Fatalf("vista previa: %v", lista2["efecto"])
	}
	admin.exige(http.StatusOK, "POST", ruta+"/confirmacion", obj{"confirmo": true})
	// Tres jurados asignados; dos cuentas nuevas, sin contraseña, con su
	// correo de activación en su idioma.
	if jurados() != 3 || cuentas("nora") != 1 || cuentas("homonimo") != 1 {
		t.Errorf("tras importar: %d jurados, %d y %d cuentas", jurados(), cuentas("nora"), cuentas("homonimo"))
	}
	correos := e.correosDe(models.PlantillaActivacion)
	if len(correos) != activaciones+2 {
		t.Fatalf("correos de activación: %d nuevos", len(correos)-activaciones)
	}
	if nora := correos[len(correos)-2]; nora.Destinatario != "nora@ejemplo.mx" || nora.Idioma != "en" || !strings.Contains(nora.Texto, "/cuenta/activar?token=") {
		t.Errorf("correo de activación de la cuenta importada: %+v", nora)
	}
	// La cuenta nueva puede activarse con su enlace e ingresar como jurado.
	nora := e.cliente()
	redirige(t, nora.post("/cuenta/activar", map[string][]string{"token": {e.tokenDe("nora@ejemplo.mx", models.PlantillaActivacion, "/cuenta/activar")}, "password": {passwordPrueba}}), "/cuenta/ingresar?aviso=activada")
	nora.ingresar("nora@ejemplo.mx", passwordPrueba)
	if sesion := nora.exige(http.StatusOK, "GET", "/api/v1/sesion", nil); !strings.Contains(fmt.Sprint(sesion["asignaciones"]), "jurado") {
		t.Errorf("la cuenta importada no quedó como jurado: %v", sesion)
	}

	// --- Todo o nada: un cuarto jurado no cabe (son exactamente tres). No
	// se crea ni su cuenta.
	_, cuarto := admin.importacion("usuarios_jurado", interpretes, "extra.csv", []byte("email,nombre,apellidos\nquinto@ejemplo.mx,Quinto,Malo\n"))
	admin.exige(http.StatusOK, "POST", cuarto+"/resoluciones", obj{"resolucion": "crear_nuevo"})
	if campos := admin.rechaza("POST", cuarto+"/confirmacion", obj{"confirmo": true}); !slices.Contains(campos, "fila 2") {
		t.Errorf("el cuarto jurado debió rechazarse: %v", campos)
	}
	if jurados() != 3 || cuentas("quinto") != 0 {
		t.Errorf("un Commit fallido dejó la cuenta creada: %d jurados, %d cuentas", jurados(), cuentas("quinto"))
	}
	// Revisores: el mismo motor. Una cuenta que ya era jurado puede además
	// ser revisora; y omitir un renglón no hace nada.
	revisores, rutaRev := admin.importacion("usuarios_revisor", interpretes, "revisores.csv",
		[]byte("email,nombre,apellidos\nexistente@ejemplo.mx,Persona,Interna\nrita@ejemplo.mx,Rita,Luna\n"))
	admin.exige(http.StatusOK, "PUT", fmt.Sprintf("%s/conflictos/%d", rutaRev, id(conflictoDe(t, revisores, 2)["id"])), obj{"resolucion": "vincular", "entidad_id": existente})
	admin.exige(http.StatusOK, "POST", rutaRev+"/resoluciones", obj{"resolucion": "omitir"})
	admin.exige(http.StatusOK, "POST", rutaRev+"/confirmacion", obj{"confirmo": true})
	if cuentas("rita") != 0 || !strings.Contains(fmt.Sprint(admin.exige(http.StatusOK, "GET", fmt.Sprintf("/api/v1/convocatorias/%d", interpretes), nil)["asignaciones"]), "revisor_documental") {
		t.Error("importación de revisores: se esperaba una asignación y ninguna cuenta nueva")
	}

	// NINGUNA IMPORTACIÓN CREA POSTULACIONES (RN-31).
	if total := admin.exige(http.StatusOK, "GET", "/api/v1/archivo", nil)["total"]; total != float64(0) {
		t.Errorf("tras las importaciones hay %v postulaciones", total)
	}
}
