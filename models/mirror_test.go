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
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Pruebas de la continuidad operativa (verificación del acervo, instantáneas
// firmadas), de la lista blanca del portal público y de las reglas de la
// anonimización.

// acervoDePrueba crea un almacén con los contenidos dados y devuelve sus
// archivos, como los registraría la base.
func acervoDePrueba(t *testing.T, contenidos ...string) (*AlmacenEnDisco, []Archivo) {
	t.Helper()
	almacen, err := AbrirAlmacenEnDisco(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var archivos []Archivo
	for i, contenido := range contenidos {
		blob, err := almacen.Guardar(strings.NewReader(contenido), 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		archivos = append(archivos, Archivo{ID: int64(i + 1), SHA256: blob.SHA256, Clave: blob.Clave, TamanoBytes: blob.Tamano})
	}
	return almacen, archivos
}

// sobrescribir cambia el contenido de un archivo del almacén, como lo haría
// un disco dañado o una mano ajena.
func sobrescribir(t *testing.T, almacen *AlmacenEnDisco, clave, contenido string) {
	t.Helper()
	ruta := filepath.Join(almacen.raiz, filepath.FromSlash(clave))
	if err := os.Chmod(ruta, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(contenido), 0o640); err != nil {
		t.Fatal(err)
	}
}

// La verificación distingue pérdida, alteración, purga pendiente y huérfanos.
func TestVerificarAcervo(t *testing.T) {
	ctx := context.Background()
	almacen, archivos := acervoDePrueba(t, "partitura", "fotografía", "audio de la obra", "acuse en PDF", "autorización")
	resumen := func(v VerificacionDeAcervo) string {
		return fmt.Sprintf("registrados=%d correctos=%d faltantes=%d alterados=%d purgados=%d huérfanos=%d ajenos=%d íntegro=%v", v.Registrados, v.Correctos,
			len(v.Faltantes), len(v.Alterados), len(v.PurgadosPresentes), len(v.Huerfanos), len(v.Ajenos), v.Integro())
	}
	v, err := VerificarAcervo(ctx, archivos, almacen, false)
	if err != nil || resumen(v) != "registrados=5 correctos=5 faltantes=0 alterados=0 purgados=0 huérfanos=0 ajenos=0 íntegro=true" {
		t.Fatalf("acervo sano: %s %v", resumen(v), err)
	}

	// 1. Un archivo se pierde. 2. Otro cambia de contenido conservando su
	// tamaño. 3. Otro cambia de tamaño. 4. Uno purgado sigue en disco.
	if err := almacen.Borrar(archivos[0].Clave); err != nil {
		t.Fatal(err)
	}
	sobrescribir(t, almacen, archivos[1].Clave, "fotografíA") // mismo número de bytes
	sobrescribir(t, almacen, archivos[2].Clave, "audio")
	archivos[3].PurgadoAt = time.Now()
	// 5. Un archivo sin registro, y algo que no es un archivo guardado.
	huerfano, err := almacen.Guardar(strings.NewReader("subida que nunca se confirmó"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(almacen.raiz, "notas.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Lo que está a medio subir no es un huérfano.
	if err := os.WriteFile(filepath.Join(almacen.raiz, subdirectorioTemporal, "subida-123"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}

	v, err = VerificarAcervo(ctx, archivos, almacen, false)
	if err != nil || resumen(v) != "registrados=4 correctos=1 faltantes=1 alterados=2 purgados=1 huérfanos=1 ajenos=1 íntegro=false" {
		t.Fatalf("acervo dañado: %s %v", resumen(v), err)
	}
	if v.Faltantes[0].ID != 1 || v.PurgadosPresentes[0].ID != 4 || v.Huerfanos[0] != huerfano.Clave || v.Ajenos[0] != "notas.txt" {
		t.Errorf("detalle de la verificación: %+v", v)
	}
	// La comprobación rápida no lee los archivos: ve el que cambió de tamaño,
	// pero no el que conserva el suyo.
	if v, err = VerificarAcervo(ctx, archivos, almacen, true); err != nil || len(v.Alterados) != 1 || v.Alterados[0].ID != 3 {
		t.Errorf("verificación rápida: %s %v", resumen(v), err)
	}
	// Terminada la purga, y sin lo perdido ni lo alterado, vuelve a estar íntegro.
	if pendientes := PurgarArchivos(almacen, v.PurgadosPresentes, nil); pendientes != 0 {
		t.Errorf("purga pendiente: %d", pendientes)
	}
	if v, err = VerificarAcervo(ctx, archivos[3:], almacen, false); err != nil || !v.Integro() || v.Registrados != 1 {
		t.Errorf("tras la purga: %s %v", resumen(v), err)
	}
	// Borrar dos veces no es un error; una clave que no es una clave, sí.
	if err := almacen.Borrar(archivos[3].Clave); err != nil {
		t.Errorf("borrar lo ya borrado: %v", err)
	}
	if err := almacen.Borrar("../fimnme.db"); err == nil {
		t.Error("una clave manipulada no debe aceptarse")
	}
}

// La firma del manifiesto detecta cualquier cambio, y la clave se cuida.
func TestFirmaDeInstantanea(t *testing.T) {
	clave, otra := []byte(strings.Repeat("a", 32)), []byte(strings.Repeat("b", 32))
	m := ManifiestoDeInstantanea{Formato: formatoDeInstantanea, Archivo: "fimnme-20261114T120000Z.db", SHA256: strings.Repeat("ab", 32),
		TamanoBytes: 4096, CreadoAt: "2026-11-14T12:00:00Z", Version: "1.0", BlobsRegistrados: 7, BlobsCopiados: 2}
	m.HMACSHA256 = m.Firmar(clave)
	if !m.FirmaValida(clave) || len(m.HMACSHA256) != 64 {
		t.Fatalf("la firma recién hecha debe ser válida: %s", m.HMACSHA256)
	}
	if m.FirmaValida(otra) {
		t.Error("la firma no debe valer con otra clave")
	}
	// Cambiar cualquier campo invalida la firma.
	for nombre, cambiar := range map[string]func(*ManifiestoDeInstantanea){
		"archivo":        func(x *ManifiestoDeInstantanea) { x.Archivo = "fimnme-20261115T120000Z.db" },
		"suma":           func(x *ManifiestoDeInstantanea) { x.SHA256 = strings.Repeat("cd", 32) },
		"tamaño":         func(x *ManifiestoDeInstantanea) { x.TamanoBytes++ },
		"fecha":          func(x *ManifiestoDeInstantanea) { x.CreadoAt = "2026-11-15T12:00:00Z" },
		"versión":        func(x *ManifiestoDeInstantanea) { x.Version = "2.0" },
		"acervo":         func(x *ManifiestoDeInstantanea) { x.BlobsFaltantes = 0; x.BlobsRegistrados = 8 },
		"formato":        func(x *ManifiestoDeInstantanea) { x.Formato = "otro" },
		"firma":          func(x *ManifiestoDeInstantanea) { x.HMACSHA256 = strings.Repeat("0", 64) },
		"firma ilegible": func(x *ManifiestoDeInstantanea) { x.HMACSHA256 = "no es hexadecimal" },
	} {
		alterado := m
		cambiar(&alterado)
		if alterado.FirmaValida(clave) {
			t.Errorf("cambiar %s no invalidó la firma", nombre)
		}
	}

	// La clave: se genera una sola vez, sólo la lee su dueño, y no es corta.
	ruta := filepath.Join(t.TempDir(), "espejo.clave")
	if err := NuevaClaveDeEspejo(ruta); err != nil {
		t.Fatal(err)
	}
	leida, err := LeerClaveDeEspejo(ruta)
	if info, _ := os.Stat(ruta); err != nil || len(leida) != 64 || info.Mode().Perm() != 0o400 {
		t.Fatalf("clave generada: %d caracteres, %v", len(leida), err)
	}
	if err := NuevaClaveDeEspejo(ruta); err == nil {
		t.Error("generar la clave no debe sobrescribir una existente")
	}
	if err := os.Chmod(ruta, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LeerClaveDeEspejo(ruta); err == nil || !strings.Contains(err.Error(), "legible por otros") {
		t.Errorf("una clave legible por otros debe rechazarse: %v", err)
	}
	corta := filepath.Join(t.TempDir(), "corta.clave")
	if err := os.WriteFile(corta, []byte("secreto\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := LeerClaveDeEspejo(corta); err == nil || !strings.Contains(err.Error(), "demasiado corta") {
		t.Errorf("una clave corta debe rechazarse: %v", err)
	}
}

// El acervo se replica por contenido: copiar dos veces no duplica, lo dañado
// no se da por bueno y lo purgado se retira del espejo.
func TestSincronizarBlobs(t *testing.T) {
	ctx := context.Background()
	origen, archivos := acervoDePrueba(t, "partitura", "fotografía", "audio de la obra")
	destino := t.TempDir()
	cuenta := func(registrados, copiados, faltantes, retirados int, err error) string {
		return fmt.Sprintf("registrados=%d copiados=%d faltantes=%d retirados=%d %v", registrados, copiados, faltantes, retirados, err)
	}
	if obtenido := cuenta(SincronizarBlobs(ctx, origen, destino, archivos)); obtenido != "registrados=3 copiados=3 faltantes=0 retirados=0 <nil>" {
		t.Fatalf("primera réplica: %s", obtenido)
	}
	// Lo copiado es idéntico y verifica contra los mismos registros.
	espejo, err := AbrirAlmacenExistente(destino)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := VerificarAcervo(ctx, archivos, espejo, false); err != nil || !v.Integro() || v.Correctos != 3 || len(v.Huerfanos) != 0 {
		t.Fatalf("el espejo no verifica: %+v %v", v, err)
	}
	if obtenido := cuenta(SincronizarBlobs(ctx, origen, destino, archivos)); obtenido != "registrados=3 copiados=0 faltantes=0 retirados=0 <nil>" {
		t.Errorf("la segunda réplica no debe copiar nada: %s", obtenido)
	}
	// Un archivo se purga por anonimización, otro se daña en el origen y
	// llega uno nuevo que el origen perdió.
	archivos[1].PurgadoAt = time.Now()
	if err := espejo.Borrar(archivos[2].Clave); err != nil { // para que haya que volver a copiarlo
		t.Fatal(err)
	}
	sobrescribir(t, origen, archivos[2].Clave, "audio de la obrA")
	archivos = append(archivos, Archivo{ID: 9, SHA256: strings.Repeat("ef", 32), Clave: "ef/ef/" + strings.Repeat("ef", 32), TamanoBytes: 5})
	if obtenido := cuenta(SincronizarBlobs(ctx, origen, destino, archivos)); obtenido != "registrados=3 copiados=0 faltantes=2 retirados=1 <nil>" {
		t.Errorf("réplica con problemas: %s", obtenido)
	}
	v, err := VerificarAcervo(ctx, archivos, espejo, false)
	if err != nil || len(v.Faltantes) != 2 || len(v.PurgadosPresentes) != 0 || len(v.Huerfanos) != 0 || v.Correctos != 1 {
		t.Errorf("espejo tras la réplica con problemas: %+v %v", v, err)
	}
}

// Las instantáneas vencidas se borran; la más reciente, nunca.
func TestDepurarInstantaneas(t *testing.T) {
	destino := t.TempDir()
	directorio := filepath.Join(destino, DirectorioDeInstantaneas)
	if err := os.MkdirAll(directorio, 0o750); err != nil {
		t.Fatal(err)
	}
	hoy := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, dias := range []int{200, 120, 91, 30, 1} {
		ruta := filepath.Join(directorio, NombreDeInstantanea(hoy.AddDate(0, 0, -dias)))
		for _, archivo := range []string{ruta, rutaDeManifiesto(ruta)} {
			if err := os.WriteFile(archivo, []byte("x"), 0o640); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(directorio, "LEEME.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	borradas, err := DepurarInstantaneas(destino, hoy.AddDate(0, 0, -90))
	quedan, _ := Instantaneas(destino)
	if err != nil || borradas != 3 || len(quedan) != 2 || quedan[0] != "fimnme-20261015T120000Z.db" {
		t.Fatalf("depuración: %d borradas, quedan %v, %v", borradas, quedan, err)
	}
	if entradas, _ := os.ReadDir(directorio); len(entradas) != 5 { // dos instantáneas con su manifiesto, y el archivo ajeno
		t.Errorf("tras depurar quedan %d archivos", len(entradas))
	}
	// Aunque todas hayan vencido, la más reciente se queda.
	if borradas, err = DepurarInstantaneas(destino, hoy.AddDate(1, 0, 0)); err != nil || borradas != 1 {
		t.Errorf("depuración total: %d %v", borradas, err)
	}
	if quedan, _ = Instantaneas(destino); len(quedan) != 1 || quedan[0] != "fimnme-20261113T120000Z.db" {
		t.Errorf("debe quedar la más reciente: %v", quedan)
	}
}

// camposJSON reúne los nombres JSON de todos los campos que un tipo puede
// serializar, bajando por sus estructuras, listas y punteros.
func camposJSON(tipo reflect.Type, prefijo string, vistos map[string]bool) {
	for tipo.Kind() == reflect.Pointer || tipo.Kind() == reflect.Slice {
		tipo = tipo.Elem()
	}
	if tipo.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < tipo.NumField(); i++ {
		campo := tipo.Field(i)
		nombre, _, _ := strings.Cut(campo.Tag.Get("json"), ",")
		if nombre == "" {
			nombre = campo.Name // un campo sin etiqueta saldría con su nombre de Go: que se note
		}
		vistos[prefijo+nombre] = true
		camposJSON(campo.Type, prefijo+nombre+".", vistos)
	}
}

// LA LISTA BLANCA DEL PORTAL PÚBLICO (RF-PUB-02 y 03, RN-34).
//
// Lo que puede salir hacia el portal es exactamente esto. Si esta prueba
// falla es porque alguien añadió, quitó o renombró un campo de la
// exportación: antes de corregir la lista de aquí, hay que preguntarse si ese
// dato puede hacerse público.
func TestListaBlancaDelPortal(t *testing.T) {
	obtenidos := make(map[string]bool)
	camposJSON(reflect.TypeOf(InteropPortal{}), "", obtenidos)
	var lista []string
	for campo := range obtenidos {
		lista = append(lista, campo)
	}
	slices.Sort(lista)
	permitidos := []string{
		"ediciones", "ediciones.anio", "ediciones.convocatorias",
		"ediciones.convocatorias.bases_md_en", "ediciones.convocatorias.bases_md_es",
		"ediciones.convocatorias.dotaciones", "ediciones.convocatorias.dotaciones.descripcion_en",
		"ediciones.convocatorias.dotaciones.descripcion_es", "ediciones.convocatorias.dotaciones.nombre_en",
		"ediciones.convocatorias.dotaciones.nombre_es",
		"ediciones.convocatorias.estado", "ediciones.convocatorias.etapas", "ediciones.convocatorias.etapas.clave",
		"ediciones.convocatorias.etapas.inicia_at", "ediciones.convocatorias.etapas.termina_at",
		"ediciones.convocatorias.nomina", "ediciones.convocatorias.nomina.publicada_at",
		"ediciones.convocatorias.nomina.seleccionados", "ediciones.convocatorias.nomina.seleccionados.nombre",
		"ediciones.convocatorias.nomina.seleccionados.obras", "ediciones.convocatorias.nomina.seleccionados.obras.titulo",
		"ediciones.convocatorias.nomina.seleccionados.semblanza", "ediciones.convocatorias.nomina.seleccionados.subcategoria",
		"ediciones.convocatorias.tipo", "ediciones.convocatorias.titulo_en", "ediciones.convocatorias.titulo_es",
		"ediciones.nombre_en", "ediciones.nombre_es", "ediciones.numero", "generado_at",
	}
	if !slices.Equal(lista, permitidos) {
		t.Errorf("los campos de la exportación al portal cambiaron.\nobtenidos:  %v\npermitidos: %v", lista, permitidos)
	}

	// La consulta de la nómina no toca ninguna columna de la lista negra ni
	// ninguna tabla del proceso interno, y sólo lee seleccionadas.
	archivos := token.NewFileSet()
	fuente, err := parser.ParseFile(archivos, "repository.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var consultas, textos []string
	for _, declaracion := range fuente.Decls {
		funcion, ok := declaracion.(*ast.FuncDecl)
		if !ok || funcion.Name.Name != "seleccionadosDe" {
			continue
		}
		ast.Inspect(funcion, func(nodo ast.Node) bool {
			if literal, ok := nodo.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				// Una consulta puede ir partida en varios literales: se
				// revisan todos los textos de la función.
				if texto, err := strconv.Unquote(literal.Value); err == nil {
					textos = append(textos, texto)
					if strings.Contains(texto, "SELECT") {
						consultas = append(consultas, texto)
					}
				}
			}
			return true
		})
	}
	if len(consultas) != 2 {
		t.Fatalf("se esperaban las dos consultas de seleccionadosDe y hay %d: ¿cambió su forma?", len(consultas))
	}
	sql := strings.Join(textos, "\n")
	for _, prohibido := range []string{"email", "telefono", "fecha_nacimiento", "nacionalidad", "entidad_residencia", "pais_residencia", "genero",
		"usuario", "documento", "archivo", "sha256", "evaluacion", "puntaje", "observacion", "revision", "subsanacion", "motivo", "acuse",
		"representante", "compositor,", "liga_media", "integrante", "enlace_video", "nota_obra"} {
		if strings.Contains(sql, prohibido) {
			t.Errorf("la consulta de la nómina menciona «%s»:\n%s", prohibido, sql)
		}
	}
	if !strings.Contains(sql, "p.dictamen = 'seleccionada'") || !strings.Contains(sql, "p.anonimizada_at IS NULL") {
		t.Errorf("la consulta de la nómina debe limitarse a seleccionadas no anonimizadas:\n%s", sql)
	}
}

func TestReglasDeNominaYAnonimizacion(t *testing.T) {
	// La nómina sólo se publica con la convocatoria cerrada, y con confirmación.
	for estado, clave := range map[EstadoConvocatoria]string{EstadoPublicada: "error.convocatoria_no_cerrada", EstadoPostulacion: "error.convocatoria_no_cerrada",
		EstadoEvaluacion: "error.convocatoria_no_cerrada", EstadoCerrada: "", EstadoArchivada: ""} {
		errores := ValidarPublicacionDeNomina(Convocatoria{Estado: estado}, true)
		if (clave == "") != (len(errores) == 0) || (clave != "" && errores[0].Clave != clave) {
			t.Errorf("publicar con la convocatoria %s: %v", estado, errores)
		}
	}
	if errores := ValidarPublicacionDeNomina(Convocatoria{Estado: EstadoCerrada}, false); len(errores) != 1 || errores[0].Campo != "confirmo" {
		t.Errorf("publicar sin confirmar: %v", errores)
	}

	cerrada, enCurso := Convocatoria{Estado: EstadoCerrada}, Convocatoria{Estado: EstadoEvaluacion}
	p := Postulacion{Folio: "FIM49-INT-0137", Estado: PostulacionEvaluada}
	buena := SolicitudDeAnonimizacion{Folio: "FIM49-INT-0137", Fundamento: "Solicitud ARCO 2031/014", Confirmo: true}
	if errores := ValidarAnonimizacion(p, cerrada, buena); len(errores) != 0 {
		t.Fatalf("anonimización válida: %v", errores)
	}
	descartada := p
	descartada.Estado = PostulacionDescartada
	if errores := ValidarAnonimizacion(descartada, enCurso, buena); len(errores) != 0 {
		t.Errorf("una descartada puede anonimizarse con la convocatoria en curso: %v", errores)
	}
	yaAnonimizada, borrador := p, Postulacion{Estado: PostulacionBorrador}
	yaAnonimizada.AnonimizadaAt = time.Now()
	for nombre, caso := range map[string]struct {
		p         Postulacion
		c         Convocatoria
		solicitud SolicitudDeAnonimizacion
		esperado  string // campo:clave de cada error
	}{
		"proceso en curso":       {p, enCurso, buena, "convocatoria:error.proceso_en_curso"},
		"ya anonimizada":         {yaAnonimizada, cerrada, buena, "estado:error.ya_anonimizada"},
		"borrador sin folio":     {borrador, cerrada, buena, "estado:error.estado_no_admite"},
		"sin confirmar":          {p, cerrada, SolicitudDeAnonimizacion{Folio: p.Folio, Fundamento: buena.Fundamento}, "confirmo:error.confirmacion_requerida"},
		"folio de otra":          {p, cerrada, SolicitudDeAnonimizacion{Folio: "FIM49-INT-0138", Fundamento: buena.Fundamento, Confirmo: true}, "folio:error.folio_no_coincide"},
		"sin fundamento":         {p, cerrada, SolicitudDeAnonimizacion{Folio: p.Folio, Fundamento: "ARCO", Confirmo: true}, "fundamento:error.fundamento_requerido"},
		"fundamento kilométrico": {p, cerrada, SolicitudDeAnonimizacion{Folio: p.Folio, Fundamento: strings.Repeat("a", 501), Confirmo: true}, "fundamento:error.fundamento_requerido"},
		"nada":                   {p, cerrada, SolicitudDeAnonimizacion{}, "confirmo:error.confirmacion_requerida folio:error.folio_no_coincide fundamento:error.fundamento_requerido"},
	} {
		var obtenido []string
		for _, e := range ValidarAnonimizacion(caso.p, caso.c, caso.solicitud) {
			obtenido = append(obtenido, e.Campo+":"+e.Clave)
		}
		if strings.Join(obtenido, " ") != caso.esperado {
			t.Errorf("%s: %v", nombre, obtenido)
		}
	}

	// De la fecha de nacimiento queda el año; la obra se conserva, lo demás no.
	for fecha, esperado := range map[string]string{"1990-05-17": "1990-01-01", "2001-12-31": "2001-01-01", "": "", "17/05/1990": ""} {
		if obtenido := FechaAnonimizada(fecha); obtenido != esperado {
			t.Errorf("fecha anonimizada de %q: %q", fecha, obtenido)
		}
	}
	for clave, seConserva := range map[string]bool{"partitura": true, "particellas": true, "audio": true, "fotografia": false, "autorizacion": false,
		"identificacion": false, "acuse": false, "": false} {
		if DocumentoSeConserva(clave) != seConserva {
			t.Errorf("¿el documento %q se conserva? se esperaba %v", clave, seConserva)
		}
	}
	// El titular de una postulación anonimizada no se muestra.
	titular := Usuario{ID: 7, Nombre: "Ana", Apellidos: "López", Email: "ana@ejemplo.mx", Idioma: "es"}
	if visible := TitularParaMostrar(p, titular); visible != titular {
		t.Errorf("titular de una postulación sin anonimizar: %+v", visible)
	}
	if oculto := TitularParaMostrar(yaAnonimizada, titular); oculto.Nombre != TextoAnonimizado || oculto.Email != "" || oculto.Apellidos != "" || oculto.ID != 7 {
		t.Errorf("titular de una postulación anonimizada: %+v", oculto)
	}
}
