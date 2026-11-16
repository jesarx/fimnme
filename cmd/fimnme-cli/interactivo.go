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

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Modo interactivo (fimnme-cli --tui, RF-CLI-02): administra desde la
// terminal las fichas institucionales —ediciones, convocatorias, calendario,
// dotaciones, requisitos documentales, criterios de evaluación y
// asignaciones de jurados y revisores—.
//
// Es para cuando el panel web no está a la mano: una instalación nueva, una
// sesión por SSH, el servidor detenido por mantenimiento. No tiene reglas
// propias: cada cambio llama a la misma operación del repositorio que usa el
// panel, que valida y escribe en una sola transacción y lo asienta en la
// bitácora, aquí con el usuario del sistema que opera la herramienta
// (RF-CLI-09). Lo que el modelo rechaza se explica con los mismos textos que
// muestra el panel.
//
// Es deliberadamente un diálogo por renglones —un menú, una pregunta, una
// respuesta— y no una pantalla dibujada: no necesita ninguna biblioteca de
// terminal, funciona igual en una consola serie, dentro de «podman exec» o
// con un lector de pantalla, y puede probarse y hasta guionarse con una
// tubería (ADR 0016).
//
// Convenciones de captura, las mismas en todas las fichas:
//
//	Intro    conserva el valor actual, que se muestra entre corchetes
//	-        deja vacío un dato opcional
//	fechas   dd/mm/aaaa hh:mm (24 horas), en hora de la Ciudad de México

// errFinDeEntrada indica que la entrada se acabó (Ctrl+D, o la tubería que
// alimentaba el diálogo terminó). No es una falla: el modo interactivo
// termina sin dejar nada a medias, porque cada cambio es una transacción.
var errFinDeEntrada = errors.New("fin de la entrada")

// formatoDeFecha es como se muestran las fechas del calendario: la
// convención mexicana, la misma que en el panel web. Al leer se acepta
// además d/m/aaaa h:mm y el formato ISO de versiones anteriores
// (formatosDeLectura).
const formatoDeFecha = "02/01/2006 15:04"

// formatosDeLectura son las formas en que puede escribirse una fecha.
var formatosDeLectura = []string{"2/1/2006 15:04", "2006-01-02 15:04"}

// tamanoMaxBases acota el archivo Markdown de las bases que se carga.
const tamanoMaxBases = 1 << 20

// nombresDeEstado nombra cada estado del ciclo de vida de una convocatoria.
var nombresDeEstado = map[models.EstadoConvocatoria]string{
	models.EstadoBorrador:           "borrador",
	models.EstadoPublicada:          "publicada (próxima apertura)",
	models.EstadoPostulacion:        "postulación abierta",
	models.EstadoRevisionDocumental: "revisión documental",
	models.EstadoEvaluacion:         "evaluación",
	models.EstadoCerrada:            "cerrada",
	models.EstadoArchivada:          "archivada",
}

// etapasDelCalendario son las tres etapas con fechas, en su orden.
var etapasDelCalendario = []models.ClaveEtapa{
	models.EtapaPostulacion, models.EtapaRevisionDocumental, models.EtapaEvaluacion,
}

// consola es el diálogo con quien opera: de dónde se leen las respuestas y
// sobre qué base se trabaja.
type consola struct {
	ctx         context.Context
	repositorio *models.Repositorio
	vistas      *views.Motor // sólo para los textos: nombres de etapa y motivos de rechazo
	lector      *bufio.Reader
	actor       models.Actor
	urlBase     string // para armar enlaces de activación
}

// opcionDeMenu es un renglón elegible de una pantalla.
type opcionDeMenu struct {
	tecla    string // lo que se escribe para elegirla: «3», «n»…
	etiqueta string
	accion   func() error
}

// pantalla es lo que se muestra antes de preguntar: un título, unos
// renglones informativos y las opciones.
type pantalla struct {
	titulo   string
	resumen  []string
	opciones []opcionDeMenu
}

// modoInteractivo es la orden «--tui».
func modoInteractivo(ctx context.Context, repositorio *models.Repositorio) error {
	pendientes, err := repositorio.MigracionesPendientes(ctx)
	if err != nil {
		return err
	}
	if len(pendientes) > 0 {
		return errors.New("el esquema no está al día; ejecute primero: fimnme-cli migrate")
	}
	vistas, err := views.Nuevo()
	if err != nil {
		return err
	}
	c := &consola{ctx: ctx, repositorio: repositorio, vistas: vistas, lector: bufio.NewReader(entrada),
		actor: actorCLI(), urlBase: entorno("FIMNME_URL_BASE", "http://127.0.0.1:8080")}
	fmt.Fprint(salida, `FIMNME — administración interactiva
Intro conserva el valor entre corchetes; «-» vacía un dato opcional.
Las fechas se escriben dd/mm/aaaa hh:mm (24 horas), en hora de la Ciudad de México.
Cada cambio se guarda al momento y queda en la bitácora.
`)
	err = c.menu("s", "Salir", c.pantallaDeInicio)
	if errors.Is(err, errFinDeEntrada) {
		fmt.Fprintln(salida, "\nFin de la entrada: se cierra el modo interactivo.")
		return nil
	}
	return err
}

// --- El diálogo ----------------------------------------------------------------

// decir escribe un renglón para quien opera.
func (c *consola) decir(formato string, args ...any) {
	fmt.Fprintf(salida, formato+"\n", args...)
}

// leer hace una pregunta y devuelve la respuesta, sin espacios a los lados.
// Si la entrada se acabó devuelve errFinDeEntrada.
func (c *consola) leer(pregunta string) (string, error) {
	fmt.Fprint(salida, pregunta+" ")
	respuesta, err := c.lector.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && respuesta != "") {
		if errors.Is(err, io.EOF) {
			return "", errFinDeEntrada
		}
		return "", fmt.Errorf("leer la respuesta: %w", err)
	}
	return strings.TrimSpace(respuesta), nil
}

// confirmar pide un «s» explícito. Cualquier otra respuesta es que no.
func (c *consola) confirmar(pregunta string) (bool, error) {
	respuesta, err := c.leer(pregunta + " (s/N):")
	return strings.EqualFold(respuesta, "s"), err
}

// menu muestra una pantalla y atiende lo que se elija, hasta que se pida
// volver. La pantalla se rehace en cada vuelta, porque lo que se acaba de
// cambiar debe verse. Si lo que la pantalla mostraba ya no existe, vuelve.
func (c *consola) menu(teclaDeSalida, etiquetaDeSalida string, construir func() (pantalla, error)) error {
	for {
		p, err := construir()
		if errors.Is(err, models.ErrNoEncontrado) {
			c.decir("Ese registro ya no existe.")
			return nil
		}
		if err != nil {
			return err
		}
		c.decir("\n== %s ==", p.titulo)
		for _, renglon := range p.resumen {
			c.decir("%s", renglon)
		}
		if len(p.resumen) > 0 {
			c.decir("")
		}
		for _, opcion := range p.opciones {
			c.decir("  %2s) %s", opcion.tecla, opcion.etiqueta)
		}
		c.decir("  %2s) %s", teclaDeSalida, etiquetaDeSalida)
		respuesta, err := c.leer("Opción:")
		if err != nil {
			return err
		}
		if strings.EqualFold(respuesta, teclaDeSalida) {
			return nil
		}
		elegida := false
		for _, opcion := range p.opciones {
			if strings.EqualFold(respuesta, opcion.tecla) {
				elegida = true
				if err := opcion.accion(); err != nil {
					return err
				}
				break
			}
		}
		if !elegida && respuesta != "" {
			c.decir("No hay ninguna opción «%s».", respuesta)
		}
	}
}

// informar dice cómo terminó una operación del repositorio. Lo que el modelo
// rechaza no es una falla de la herramienta: se explica y el diálogo sigue.
// Sólo devuelve error lo inesperado (la base no responde), que sí lo termina.
func (c *consola) informar(err error, exito string) error {
	var regla *models.ErrorValidacion
	switch {
	case err == nil:
		c.decir("%s", exito)
	case errors.As(err, &regla):
		c.decir("No se guardó nada:\n  %s", motivosDeRegla(c.vistas, regla))
	case errors.Is(err, models.ErrNoEncontrado):
		c.decir("No se guardó nada: ese registro ya no existe.")
	case errors.Is(err, models.ErrDuplicado):
		c.decir("No se guardó nada: ya existe un registro igual.")
	case errors.Is(err, models.ErrEmailEnUso):
		c.decir("No se guardó nada: ya existe una cuenta con ese correo.")
	default:
		return err
	}
	return nil
}

// ficha reúne las preguntas de un registro. Tras el primer fallo de lectura
// las demás ya no preguntan nada, y el fallo se consulta una sola vez, al
// terminar: así una ficha de diez datos no lleva diez comprobaciones.
type ficha struct {
	c   *consola
	err error
}

// preguntar muestra el dato con su valor actual y devuelve lo escrito, y si
// se escribió algo: una respuesta vacía conserva el valor.
func (f *ficha) preguntar(etiqueta, actual string) (string, bool) {
	if f.err != nil {
		return "", false
	}
	var respuesta string
	respuesta, f.err = f.c.leer(fmt.Sprintf("%s [%s]:", etiqueta, actual))
	return respuesta, f.err == nil && respuesta != ""
}

// texto captura un texto. «-» lo deja vacío.
func (f *ficha) texto(etiqueta string, valor *string) {
	if respuesta, escrito := f.preguntar(etiqueta, *valor); escrito {
		if respuesta == "-" {
			respuesta = ""
		}
		*valor = respuesta
	}
}

// entero captura un número entero; insiste hasta que lo sea.
func (f *ficha) entero(etiqueta string, valor *int) {
	for {
		respuesta, escrito := f.preguntar(etiqueta, strconv.Itoa(*valor))
		if !escrito {
			return
		}
		if numero, err := strconv.Atoi(respuesta); err == nil {
			*valor = numero
			return
		}
		f.c.decir("  Escriba un número entero.")
	}
}

// siNo captura un sí o un no; insiste hasta que sea «s» o «n».
func (f *ficha) siNo(etiqueta string, valor *bool) {
	actual := "n"
	if *valor {
		actual = "s"
	}
	for {
		respuesta, escrito := f.preguntar(etiqueta+" (s/n)", actual)
		if !escrito {
			return
		}
		if strings.EqualFold(respuesta, "s") || strings.EqualFold(respuesta, "n") {
			*valor = strings.EqualFold(respuesta, "s")
			return
		}
		f.c.decir("  Responda s o n.")
	}
}

// eleccion captura uno de varios valores posibles; insiste hasta que sea uno.
func (f *ficha) eleccion(etiqueta string, posibles []string, valor *string) {
	for {
		respuesta, escrito := f.preguntar(fmt.Sprintf("%s (%s)", etiqueta, strings.Join(posibles, " / ")), *valor)
		if !escrito {
			return
		}
		for _, posible := range posibles {
			if strings.EqualFold(respuesta, posible) {
				*valor = posible
				return
			}
		}
		f.c.decir("  Elija uno de: %s.", strings.Join(posibles, ", "))
	}
}

// momento captura una fecha con hora, escrita en hora de la Ciudad de México
// y guardada en UTC (RN-36). «-» la deja sin definir.
func (f *ficha) momento(etiqueta string, valor *time.Time) {
	actual := "sin definir"
	if !valor.IsZero() {
		actual = valor.In(views.ZonaMexico()).Format(formatoDeFecha)
	}
	for {
		respuesta, escrito := f.preguntar(etiqueta, actual)
		if !escrito {
			return
		}
		if respuesta == "-" {
			*valor = time.Time{}
			return
		}
		respuesta = strings.Join(strings.Fields(respuesta), " ")
		for _, formato := range formatosDeLectura {
			if momento, err := time.ParseInLocation(formato, respuesta, views.ZonaMexico()); err == nil {
				*valor = momento.UTC()
				return
			}
		}
		f.c.decir("  Escriba la fecha como dd/mm/aaaa hh:mm, de 24 horas; por ejemplo, 29/01/2027 23:59.")
	}
}

// --- Ediciones -----------------------------------------------------------------

// pantallaDeInicio lista las ediciones.
func (c *consola) pantallaDeInicio() (pantalla, error) {
	ediciones, err := c.repositorio.ListarEdiciones(c.ctx)
	if err != nil {
		return pantalla{}, err
	}
	p := pantalla{titulo: "Ediciones"}
	if len(ediciones) == 0 {
		p.resumen = []string{"Aún no hay ninguna edición."}
	}
	for i, edicion := range ediciones {
		p.opciones = append(p.opciones, opcionDeMenu{strconv.Itoa(i + 1),
			fmt.Sprintf("Edición %d (%d) · %s", edicion.Numero, edicion.Anio, edicion.NombreES),
			func() error {
				return c.menu("v", "Volver", func() (pantalla, error) { return c.pantallaDeEdicion(edicion.ID) })
			}})
	}
	p.opciones = append(p.opciones, opcionDeMenu{"n", "Nueva edición", c.nuevaEdicion})
	return p, nil
}

// capturarEdicion pregunta los datos de una edición.
func (c *consola) capturarEdicion(e *models.Edicion) error {
	f := ficha{c: c}
	f.entero("Número de edición", &e.Numero)
	f.entero("Año", &e.Anio)
	f.texto("Nombre en español", &e.NombreES)
	f.texto("Nombre en inglés (opcional)", &e.NombreEN)
	return f.err
}

// nuevaEdicion da de alta una edición.
func (c *consola) nuevaEdicion() error {
	var e models.Edicion
	if err := c.capturarEdicion(&e); err != nil {
		return err
	}
	_, err := c.repositorio.CrearEdicion(c.ctx, c.actor, e)
	return c.informar(err, "Edición creada.")
}

// pantallaDeEdicion muestra una edición con sus convocatorias.
func (c *consola) pantallaDeEdicion(edicionID int64) (pantalla, error) {
	edicion, err := c.repositorio.EdicionPorID(c.ctx, edicionID)
	if err != nil {
		return pantalla{}, err
	}
	convocatorias, err := c.repositorio.ConvocatoriasDeEdicion(c.ctx, edicionID)
	if err != nil {
		return pantalla{}, err
	}
	p := pantalla{titulo: fmt.Sprintf("Edición %d (%d) · %s", edicion.Numero, edicion.Anio, edicion.NombreES)}
	if len(convocatorias) == 0 {
		p.resumen = []string{"Esta edición aún no tiene convocatorias."}
	}
	for i, convocatoria := range convocatorias {
		p.opciones = append(p.opciones, opcionDeMenu{strconv.Itoa(i + 1),
			fmt.Sprintf("%s — %s", c.vistas.T(models.IdiomaES, "tipo."+string(convocatoria.Tipo)), nombresDeEstado[convocatoria.Estado]),
			func() error {
				return c.menu("v", "Volver", func() (pantalla, error) { return c.pantallaDeConvocatoria(convocatoria.ID) })
			}})
	}
	p.opciones = append(p.opciones,
		opcionDeMenu{"e", "Editar los datos de la edición", func() error {
			if err := c.capturarEdicion(&edicion); err != nil {
				return err
			}
			return c.informar(c.repositorio.ActualizarEdicion(c.ctx, c.actor, edicion), "Edición actualizada.")
		}},
		opcionDeMenu{"n", "Nueva convocatoria, en blanco", func() error { return c.nuevaConvocatoria(edicion) }},
		opcionDeMenu{"d", "Nueva convocatoria, copiando la configuración de otra edición", func() error { return c.duplicarHacia(edicion) }},
	)
	return p, nil
}

// --- Convocatorias ---------------------------------------------------------------

// nuevaConvocatoria crea una convocatoria en borrador, con los requisitos
// documentales y los criterios iniciales de su tipo, como hace el panel.
func (c *consola) nuevaConvocatoria(edicion models.Edicion) error {
	tipo := string(models.TipoInterpretes)
	nueva := models.Convocatoria{Edicion: edicion, Estado: models.EstadoBorrador, MaxVideos: models.MaxVideosPermitidos}
	f := ficha{c: c}
	f.eleccion("Tipo", []string{string(models.TipoInterpretes), string(models.TipoCompositores)}, &tipo)
	f.texto("Título en español", &nueva.TituloES)
	f.texto("Título en inglés (opcional)", &nueva.TituloEN)
	if f.err != nil {
		return f.err
	}
	if nueva.Tipo = models.TipoConvocatoria(tipo); nueva.Tipo == models.TipoCompositores {
		nueva.MaxObras = models.MaxObrasCompositores
	}
	_, err := c.repositorio.CrearConvocatoriaConfigurada(c.ctx, c.actor, nueva,
		models.RequisitosIniciales(nueva.Tipo), models.CriteriosIniciales())
	return c.informar(err, "Convocatoria creada en borrador, con los requisitos y criterios iniciales de su tipo.\n"+
		"Faltan las bases y el calendario antes de publicarla.")
}

// duplicarHacia crea en una edición una convocatoria con la configuración de
// la de otra edición (textos, límites, dotaciones, requisitos y criterios).
// No copia el calendario ni las asignaciones, que son de cada año.
func (c *consola) duplicarHacia(destino models.Edicion) error {
	ediciones, err := c.repositorio.ListarEdiciones(c.ctx)
	if err != nil {
		return err
	}
	var origenes []models.Convocatoria
	for _, edicion := range ediciones {
		if edicion.ID == destino.ID {
			continue
		}
		convocatorias, err := c.repositorio.ConvocatoriasDeEdicion(c.ctx, edicion.ID)
		if err != nil {
			return err
		}
		origenes = append(origenes, convocatorias...)
	}
	if len(origenes) == 0 {
		c.decir("No hay convocatorias de otras ediciones que copiar.")
		return nil
	}
	for i, origen := range origenes {
		c.decir("  %2d) Edición %d · %s", i+1, origen.Edicion.Numero, c.vistas.T(models.IdiomaES, "tipo."+string(origen.Tipo)))
	}
	respuesta, err := c.leer("Convocatoria de origen (Intro para cancelar):")
	if err != nil || respuesta == "" {
		return err
	}
	numero, err := strconv.Atoi(respuesta)
	if err != nil || numero < 1 || numero > len(origenes) {
		c.decir("No hay ninguna opción «%s».", respuesta)
		return nil
	}
	_, err = c.repositorio.DuplicarConvocatoria(c.ctx, c.actor, origenes[numero-1].ID, destino.ID)
	return c.informar(err, "Convocatoria creada en borrador con la configuración copiada.\n"+
		"El calendario y las asignaciones no se copian: son de cada año.")
}

// pantallaDeConvocatoria muestra el estado de una convocatoria y lo que se
// puede administrar de ella.
func (c *consola) pantallaDeConvocatoria(convocatoriaID int64) (pantalla, error) {
	cfg, err := c.repositorio.ConfiguracionDeConvocatoria(c.ctx, convocatoriaID)
	if err != nil {
		return pantalla{}, err
	}
	conv := cfg.Convocatoria
	p := pantalla{titulo: fmt.Sprintf("%s · edición %d", conv.TituloES, conv.Edicion.Numero)}
	p.resumen = append(p.resumen, fmt.Sprintf("Tipo: %s · Estado: %s", c.vistas.T(models.IdiomaES, "tipo."+string(conv.Tipo)), nombresDeEstado[conv.Estado]))
	for _, clave := range etapasDelCalendario {
		rango := "sin definir"
		if etapa, definida := conv.Etapa(clave); definida {
			rango = etapa.IniciaAt.In(views.ZonaMexico()).Format(formatoDeFecha) + " → " + etapa.TerminaAt.In(views.ZonaMexico()).Format(formatoDeFecha)
		}
		p.resumen = append(p.resumen, fmt.Sprintf("  %-22s %s", c.vistas.T(models.IdiomaES, "etapa."+string(clave)), rango))
	}
	bases := "faltan"
	if strings.TrimSpace(conv.BasesMDES) != "" {
		bases = fmt.Sprintf("%d caracteres en español, %d en inglés", utf8.RuneCountInString(conv.BasesMDES), utf8.RuneCountInString(conv.BasesMDEN))
	}
	p.resumen = append(p.resumen, "Bases: "+bases)

	// Las opciones llevan el mismo número en las dos convocatorias, aunque
	// las dotaciones sólo existan en una: quien opera no reaprende el menú.
	dotaciones := "Dotaciones (sólo existen en Compositores)"
	if conv.Tipo == models.TipoCompositores {
		dotaciones = fmt.Sprintf("Dotaciones (activas: %d)", cfg.DotacionesActivas())
	}
	p.opciones = []opcionDeMenu{
		{"1", "Títulos y límites", func() error { return c.editarConvocatoria(conv) }},
		{"2", "Bases (se cargan de un archivo Markdown)", func() error { return c.cargarBases(conv) }},
		{"3", "Calendario", func() error { return c.editarCalendario(conv) }},
		{"4", "Cambiar de estado", func() error { return c.cambiarEstado(conv) }},
		{"5", dotaciones, func() error {
			return c.menu("v", "Volver", func() (pantalla, error) { return c.pantallaDeDotaciones(convocatoriaID) })
		}},
		{"6", fmt.Sprintf("Requisitos documentales (%d)", len(cfg.Requisitos)), func() error {
			return c.menu("v", "Volver", func() (pantalla, error) { return c.pantallaDeRequisitos(convocatoriaID) })
		}},
		{"7", fmt.Sprintf("Criterios de evaluación (%d)", len(cfg.Criterios)), func() error {
			return c.menu("v", "Volver", func() (pantalla, error) { return c.pantallaDeCriterios(convocatoriaID) })
		}},
		{"8", fmt.Sprintf("Jurados y revisores (jurados: %d de %d · revisores: %d)", cfg.Activos(models.PerfilJurado), models.CuotaJurados,
			cfg.Activos(models.PerfilRevisorDocumental)), func() error {
			return c.menu("v", "Volver", func() (pantalla, error) { return c.pantallaDeAsignaciones(convocatoriaID) })
		}},
	}
	return p, nil
}

// editarConvocatoria cambia los títulos y los límites.
func (c *consola) editarConvocatoria(conv models.Convocatoria) error {
	f := ficha{c: c}
	f.texto("Título en español", &conv.TituloES)
	f.texto("Título en inglés (opcional)", &conv.TituloEN)
	f.entero("Máximo de obras por postulación (0 = sin límite)", &conv.MaxObras)
	f.entero("Duración mínima del programa, en minutos (0 = sin mínimo)", &conv.DuracionMinProgramaMin)
	f.entero("Duración máxima del programa, en minutos (0 = sin límite)", &conv.DuracionMaxProgramaMin)
	f.entero("Máximo de ligas de video", &conv.MaxVideos)
	if f.err != nil {
		return f.err
	}
	return c.informar(c.repositorio.ActualizarConvocatoria(c.ctx, c.actor, conv), "Convocatoria actualizada.")
}

// cargarBases sustituye las bases por el contenido de archivos Markdown. Un
// texto de varias páginas no se escribe en una terminal: se prepara en un
// archivo y aquí sólo se indica dónde está.
func (c *consola) cargarBases(conv models.Convocatoria) error {
	cambio := false
	for _, bases := range []struct {
		idioma  string
		destino *string
	}{{"español", &conv.BasesMDES}, {"inglés", &conv.BasesMDEN}} {
		ruta, err := c.leer(fmt.Sprintf("Archivo con las bases en %s (Intro = sin cambio, «-» = quitarlas):", bases.idioma))
		if err != nil {
			return err
		}
		switch ruta {
		case "":
			continue
		case "-":
			*bases.destino, cambio = "", true
			continue
		}
		contenido, err := os.ReadFile(ruta)
		switch {
		case err != nil:
			c.decir("No se pudo leer el archivo: %v", err)
			return nil
		case len(contenido) > tamanoMaxBases:
			c.decir("El archivo pasa de 1 MB: no parece el texto de unas bases.")
			return nil
		case !utf8.Valid(contenido):
			c.decir("El archivo no es texto en UTF-8.")
			return nil
		}
		*bases.destino, cambio = string(contenido), true
	}
	if !cambio {
		return nil
	}
	return c.informar(c.repositorio.ActualizarConvocatoria(c.ctx, c.actor, conv), "Bases actualizadas.")
}

// editarCalendario pregunta el inicio y el fin de cada etapa y sustituye el
// calendario. Es también la forma de extender un plazo. Una etapa que queda
// sin fechas se omite: el calendario puede estar incompleto mientras la
// convocatoria es borrador.
func (c *consola) editarCalendario(conv models.Convocatoria) error {
	var etapas []models.Etapa
	f := ficha{c: c}
	for _, clave := range etapasDelCalendario {
		etapa, _ := conv.Etapa(clave)
		etapa.Clave = clave
		nombre := c.vistas.T(models.IdiomaES, "etapa."+string(clave))
		f.momento(nombre+", inicia", &etapa.IniciaAt)
		f.momento(nombre+", termina", &etapa.TerminaAt)
		if !etapa.IniciaAt.IsZero() || !etapa.TerminaAt.IsZero() {
			etapas = append(etapas, etapa)
		}
	}
	if f.err != nil {
		return f.err
	}
	return c.informar(c.repositorio.DefinirEtapas(c.ctx, c.actor, conv.ID, etapas), "Calendario actualizado.")
}

// cambiarEstado pasa la convocatoria al estado siguiente o al anterior de su
// ciclo de vida. Es el cambio forzado por el administrador: retroceder exige
// un motivo, y todo queda en la bitácora.
func (c *consola) cambiarEstado(conv models.Convocatoria) error {
	c.decir("Estado actual: %s.", nombresDeEstado[conv.Estado])
	siguiente, haySiguiente := models.EstadoSiguiente(conv.Estado)
	anterior, hayAnterior := models.EstadoAnterior(conv.Estado)
	if haySiguiente {
		c.decir("   a) Avanzar a: %s", nombresDeEstado[siguiente])
	}
	if hayAnterior {
		c.decir("   r) Retroceder a: %s", nombresDeEstado[anterior])
	}
	respuesta, err := c.leer("Cambio (Intro para cancelar):")
	if err != nil {
		return err
	}
	var destino models.EstadoConvocatoria
	motivo := ""
	switch {
	case strings.EqualFold(respuesta, "a") && haySiguiente:
		destino = siguiente
	case strings.EqualFold(respuesta, "r") && hayAnterior:
		destino = anterior
		if motivo, err = c.leer("Motivo del retroceso (queda en la bitácora):"); err != nil {
			return err
		}
	default:
		return nil
	}
	if confirmado, err := c.confirmar(fmt.Sprintf("¿Pasar la convocatoria a «%s»?", nombresDeEstado[destino])); err != nil || !confirmado {
		return err
	}
	_, err = c.repositorio.CambiarEstadoConvocatoria(c.ctx, c.actor, conv.ID, destino, motivo, false)
	return c.informar(err, "La convocatoria está ahora en: "+nombresDeEstado[destino]+".")
}
