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
	"flag"
	"fmt"
	"strings"

	"github.com/jesarx/fimnme/models"
)

// Orden «schema»: extensión dinámica de nodos (§5.14.1).
//
//	schema list    muestra el diccionario de campos extendidos
//	schema add     añade un campo a un nodo de la lista blanca (ALTER TABLE)
//	schema drop    quita un campo extendido y BORRA sus valores
//
// Es la única vía para alterar el esquema sin una migración, y sólo existe
// aquí, en la herramienta: el servidor web nunca ejecuta DDL. Cada ALTER
// TABLE queda en la bitácora tal como se ejecutó.

// ordenDeEsquema reparte las órdenes de «schema».
func ordenDeEsquema(args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		if len(args) == 0 {
			return errors.New("falta la orden: schema list | add | drop")
		}
		switch args[0] {
		case "list":
			return listarCampos(ctx, repositorio)
		case "add":
			return agregarCampo(ctx, repositorio, args[1:])
		case "drop":
			return eliminarCampo(ctx, repositorio, args[1:])
		}
		return fmt.Errorf("orden de esquema desconocida: %s (use list, add o drop)", args[0])
	}
}

// listarCampos ejecuta «schema list».
func listarCampos(ctx context.Context, repositorio *models.Repositorio) error {
	campos, err := repositorio.CamposExtendidos(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(salida, "Nodos que admiten campos: %s\n", strings.Join(models.NodosExtensibles, ", "))
	if len(campos) == 0 {
		fmt.Fprintln(salida, "\nNo hay campos extendidos.")
		return nil
	}
	fmt.Fprintln(salida)
	for _, c := range campos {
		visible := "sólo administración y supervisión"
		if len(c.VisibleEn) > 0 {
			var perfiles []string
			for _, perfil := range c.VisibleEn {
				perfiles = append(perfiles, string(perfil))
			}
			visible = "también " + strings.Join(perfiles, ", ")
		}
		exportable := "exportable"
		if !c.Exportable {
			exportable = "no exportable"
		}
		fmt.Fprintf(salida, "%s.%s  (%s, %s)\n    %s / %s\n    Lo ven: %s\n", c.Tabla, c.Columna, c.Tipo, exportable, c.EtiquetaES, c.EtiquetaEN, visible)
	}
	return nil
}

// motivosDeEsquema explica, en palabras, por qué se rechazó un campo.
func motivosDeEsquema(regla *models.ErrorValidacion) string {
	motivos := map[string]string{
		"error.nodo_vedado":                 "esa tabla es material probatorio del proceso y no admite campos (" + strings.Join(models.NodosVedados, ", ") + ")",
		"error.nodo_no_extensible":          "esa tabla no admite campos; las que sí: " + strings.Join(models.NodosExtensibles, ", "),
		"error.nombre_de_campo":             "el nombre debe empezar por letra y llevar sólo minúsculas, dígitos y guion bajo (de 2 a 40 caracteres)",
		"error.etiqueta_requerida":          "falta --etiqueta-es (hasta 120 caracteres)",
		"error.texto_largo_campo":           "alguna etiqueta o ayuda es demasiado larga",
		"error.visibilidad_sin_postulacion": "--visible sólo aplica a los nodos de una postulación; los de configuración los ve sólo la administración",
		"error.demasiados_campos":           "esa tabla ya tiene el máximo de campos extendidos",
		"error.opcion_invalida":             "--tipo debe ser texto, entero o decimal, y --visible una lista de: postulante, revisor_documental, jurado",
	}
	var lista []string
	for _, e := range regla.Errores {
		if motivo, conocido := motivos[e.Clave]; conocido {
			lista = append(lista, motivo)
		} else {
			lista = append(lista, e.Campo+": "+e.Clave)
		}
	}
	return strings.Join(lista, "; ")
}

// agregarCampo ejecuta «schema add» (RF-CLI-20 y 21).
func agregarCampo(ctx context.Context, repositorio *models.Repositorio, args []string) error {
	banderas := flag.NewFlagSet("schema add", flag.ContinueOnError)
	tabla := banderas.String("tabla", "", "nodo al que se añade el campo")
	nombre := banderas.String("nombre", "", "nombre del campo; la columna se llamará x_NOMBRE")
	tipo := banderas.String("tipo", string(models.ExtendidoTexto), "texto, entero o decimal")
	etiquetaES := banderas.String("etiqueta-es", "", "etiqueta en español")
	etiquetaEN := banderas.String("etiqueta-en", "", "etiqueta en inglés")
	ayudaES := banderas.String("ayuda-es", "", "texto de ayuda en español")
	ayudaEN := banderas.String("ayuda-en", "", "texto de ayuda en inglés")
	visible := banderas.String("visible", "", "perfiles que lo ven además de la administración, separados por coma: postulante, revisor_documental, jurado. Con «postulante», lo captura quien postula")
	noExportable := banderas.Bool("no-exportable", false, "lo deja fuera de los reportes (la migración a MariaDB lo lleva siempre: es mudar la base, no exportar)")
	if err := banderas.Parse(args); err != nil {
		return err
	}
	campo := models.CampoExtendido{
		Tabla: strings.TrimSpace(*tabla), Columna: models.ColumnaExtendida(*nombre), Tipo: models.TipoExtendido(*tipo),
		EtiquetaES: *etiquetaES, EtiquetaEN: *etiquetaEN, AyudaES: *ayudaES, AyudaEN: *ayudaEN, Exportable: !*noExportable,
	}
	for _, perfil := range strings.FieldsFunc(*visible, func(r rune) bool { return r == ',' || r == ' ' }) {
		campo.VisibleEn = append(campo.VisibleEn, models.Perfil(perfil))
	}
	creado, err := repositorio.AgregarCampoExtendido(ctx, actorCLI(), campo)
	var regla *models.ErrorValidacion
	switch {
	case errors.As(err, &regla):
		return fmt.Errorf("no se añadió el campo: %s", motivosDeEsquema(regla))
	case errors.Is(err, models.ErrDuplicado):
		return fmt.Errorf("ya existe el campo %s.%s", campo.Tabla, campo.Columna)
	case err != nil:
		return err
	}
	sentencia, _ := models.SentenciaParaAgregar(creado)
	fmt.Fprintf(salida, "Campo añadido: %s.%s\n  Se ejecutó: %s\n  Clave en reportes: %s\n", creado.Tabla, creado.Columna, sentencia, creado.Ruta())
	switch {
	case creado.LoCapturaElPostulante():
		fmt.Fprintln(salida, "  Aparece en el formulario de postulación; es opcional y sólo se captura mientras la postulación es un borrador.")
	case !models.NodoConCaptura(creado.Tabla):
		fmt.Fprintln(salida, "  AVISO: en esta tabla la columna queda creada y registrada, pero aún no hay forma de capturarla.")
	default:
		fmt.Fprintln(salida, "  Lo captura la administración (PUT /api/v1/extendidos/"+creado.Tabla+"/{id}).")
	}
	return nil
}

// eliminarCampo ejecuta «schema drop». Borra la columna y, con ella, todos
// los valores capturados: pide escribir de nuevo el nombre de la columna.
func eliminarCampo(ctx context.Context, repositorio *models.Repositorio, args []string) error {
	banderas := flag.NewFlagSet("schema drop", flag.ContinueOnError)
	tabla := banderas.String("tabla", "", "nodo del que se quita el campo")
	nombre := banderas.String("nombre", "", "nombre del campo (con o sin el prefijo x_)")
	if err := banderas.Parse(args); err != nil {
		return err
	}
	columna := models.ColumnaExtendida(*nombre)
	if *tabla == "" || *nombre == "" {
		return errors.New("indique --tabla y --nombre")
	}
	fmt.Fprintf(salida, "Se va a ELIMINAR el campo %s.%s y TODOS los valores capturados en él. No se puede deshacer.\n", *tabla, columna)
	fmt.Fprint(salida, "Para confirmar, escriba el nombre de la columna: ")
	respuesta, _ := bufio.NewReader(entrada).ReadString('\n') // sin respuesta no hay confirmación
	fmt.Fprintln(salida)
	if strings.TrimSpace(respuesta) != columna {
		return errors.New("no se eliminó nada: el nombre escrito no coincide")
	}
	err := repositorio.EliminarCampoExtendido(ctx, actorCLI(), *tabla, columna)
	if errors.Is(err, models.ErrNoEncontrado) {
		return fmt.Errorf("no existe el campo extendido %s.%s", *tabla, columna)
	}
	if err != nil {
		return err
	}
	sentencia, _ := models.SentenciaParaEliminar(*tabla, columna)
	fmt.Fprintf(salida, "Campo eliminado.\n  Se ejecutó: %s\n", sentencia)
	return nil
}
