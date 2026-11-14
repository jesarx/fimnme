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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jesarx/fimnme/models"
)

// Órdenes del portal público y de protección de datos:
//
//	export-json           exportación sanitizada para el portal público (§5.16)
//	arco anonimizar       anonimiza una postulación ante una solicitud ARCO (RN-28)

// exportarJSON ejecuta «export-json» (RF-CLI-04): la misma exportación que
// sirve /api/v1/interop/convocatoria.json, a un archivo o a la salida
// estándar. La nómina de seleccionados que lleve es la última que un
// administrador publicó; esta orden no publica nada.
func exportarJSON(dirDatos string, args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas := flag.NewFlagSet("export-json", flag.ContinueOnError)
		edicion := banderas.Int("edicion", 0, "número de edición; por omisión, todas")
		archivo := banderas.String("salida", "", "archivo de salida; por omisión, la salida estándar")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		almacen, err := models.AbrirAlmacenExistente(directorioDeBlobs(dirDatos))
		if err != nil {
			return err
		}
		portal, err := repositorio.PortalPublico(ctx, almacen, *edicion)
		if err != nil {
			return err
		}
		texto, err := json.MarshalIndent(portal, "", "  ")
		if err != nil {
			return err
		}
		texto = append(texto, '\n')
		suma := sha256.Sum256(texto)
		if err := repositorio.RegistrarExportacionInterop(ctx, actorCLI(), hex.EncodeToString(suma[:]), len(portal.Ediciones)); err != nil {
			return err
		}
		if *archivo == "" {
			_, err = salida.Write(texto)
			return err
		}
		// 0644: lo exportado es público por definición.
		if err := os.WriteFile(*archivo, texto, 0o644); err != nil {
			return fmt.Errorf("escribir la exportación: %w", err)
		}
		fmt.Fprintf(salida, "Exportación escrita en %s (%d ediciones, SHA-256 %s).\n", *archivo, len(portal.Ediciones), hex.EncodeToString(suma[:]))
		return nil
	}
}

// palabraDeConfirmacion es lo que hay que escribir, tal cual, para ejecutar
// una anonimización.
const palabraDeConfirmacion = "ANONIMIZAR"

// anonimizar ejecuta «arco anonimizar FOLIO» (RF-CLI-07). Es la única
// operación destructiva del sistema: muestra qué va a borrarse, pide escribir
// de nuevo el folio y después la palabra ANONIMIZAR, y sólo entonces actúa.
// El fundamento queda en la bitácora.
func anonimizar(dirDatos string, args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		// El folio puede ir antes o después de las opciones.
		folio := ""
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			folio, args = args[0], args[1:]
		}
		banderas := flag.NewFlagSet("arco anonimizar", flag.ContinueOnError)
		fundamento := banderas.String("fundamento", "", "referencia de la solicitud ARCO (p. ej. su número de expediente); queda en la bitácora. No escriba aquí datos personales")
		conCuenta := banderas.Bool("con-cuenta", false, "anonimiza también la cuenta del titular, si no tiene otras postulaciones")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		if folio == "" {
			folio = banderas.Arg(0)
		}
		if folio == "" {
			return errors.New("indique el folio: fimnme-cli arco anonimizar FOLIO --fundamento TEXTO")
		}
		id, err := repositorio.PostulacionPorFolio(ctx, folio)
		if errors.Is(err, models.ErrNoEncontrado) {
			return fmt.Errorf("no existe ninguna postulación con el folio %s", folio)
		}
		if err != nil {
			return err
		}
		p, err := repositorio.PostulacionPorID(ctx, id)
		if err != nil {
			return err
		}
		almacen, err := models.AbrirAlmacenExistente(directorioDeBlobs(dirDatos))
		if err != nil {
			return err
		}
		// Antes de pedir ninguna confirmación se comprueba lo que no depende
		// de ella: que la postulación pueda anonimizarse y que haya
		// fundamento. No tiene sentido hacer confirmar algo que no va a ocurrir.
		cfg, err := repositorio.ConfiguracionDeConvocatoria(ctx, p.ConvocatoriaID)
		if err != nil {
			return err
		}
		previa := models.SolicitudDeAnonimizacion{Folio: p.Folio, Fundamento: *fundamento, Confirmo: true}
		if errores := models.ValidarAnonimizacion(p, cfg.Convocatoria, previa); len(errores) > 0 {
			return fmt.Errorf("no puede anonimizarse: %s", motivosDeRechazo(&models.ErrorValidacion{Errores: errores}))
		}

		nombre := p.Campos["datos.nombre_completo"]
		if nombre == "" {
			nombre = p.Campos["datos.nombre_ensamble"]
		}
		fmt.Fprintf(salida, "Postulación %s · %s · estado: %s\n\n", p.Folio, nombre, p.Estado)
		fmt.Fprint(salida, `Se van a BORRAR, sin posibilidad de recuperarlos:
  · nombre, nombre artístico, teléfono, correo de contacto y semblanza;
  · el día y el mes de la fecha de nacimiento (queda el año);
  · los nombres de los integrantes, las ligas de video y las ligas de las obras;
  · la fotografía, las autorizaciones y los acuses, del disco y del espejo;
  · el texto de los correos y de las subsanaciones que se le enviaron.
Se CONSERVAN la obra (títulos, partituras, particellas y audio), el dictamen,
las evaluaciones y los datos estadísticos (género, nacionalidad, país, entidad).
`)
		if *conCuenta {
			fmt.Fprintln(salida, "Además se anonimizará la CUENTA del titular, que ya no podrá ingresar.")
		}
		lector := bufio.NewReader(entrada)
		preguntar := func(pregunta string) string {
			fmt.Fprint(salida, "\n"+pregunta+" ")
			respuesta, _ := lector.ReadString('\n') // al acabarse la entrada, la respuesta es vacía y no confirma
			return strings.TrimSpace(respuesta)
		}
		folioEscrito := preguntar("Para confirmar, escriba de nuevo el folio:")
		palabra := preguntar("Escriba " + palabraDeConfirmacion + " para ejecutar:")
		fmt.Fprintln(salida)

		resultado, err := repositorio.AnonimizarPostulacion(ctx, actorCLI(), id, models.SolicitudDeAnonimizacion{
			Folio: folioEscrito, Fundamento: *fundamento, Confirmo: palabra == palabraDeConfirmacion, ConCuenta: *conCuenta})
		var regla *models.ErrorValidacion
		if errors.As(err, &regla) {
			return fmt.Errorf("no se anonimizó nada: %s", motivosDeRechazo(regla))
		}
		if err != nil {
			return err
		}
		pendientes := models.PurgarArchivos(almacen, resultado.ArchivosPorPurgar, func(archivo models.Archivo, err error) {
			fmt.Fprintf(os.Stderr, "No se pudo borrar el archivo %d: %v\n", archivo.ID, err)
		})
		fmt.Fprintf(salida, "Postulación %s anonimizada.\n", resultado.Folio)
		fmt.Fprintf(salida, "  Archivos borrados del disco: %d · conservados (la obra): %d\n", len(resultado.ArchivosPorPurgar)-pendientes, resultado.ArchivosConservados)
		if resultado.CuentaAnonimizada {
			fmt.Fprintln(salida, "  La cuenta del titular quedó anonimizada y desactivada.")
		}
		if pendientes > 0 {
			return fmt.Errorf("%d archivos quedaron por borrar del disco; termine con: fimnme-cli verify --purgar", pendientes)
		}
		fmt.Fprint(salida, `
Falta, fuera de esta herramienta:
  · tomar una instantánea nueva («mirror snapshot») para retirar los archivos del espejo;
  · las instantáneas y los respaldos anteriores aún contienen los datos: se
    borran al vencer su plazo de conservación. Si la solicitud exige antes,
    bórrelos a mano;
  · revisar las observaciones escritas por revisores y jurados, que son texto
    libre y no se modifican.
`)
		return nil
	}
}

// motivosDeRechazo explica, en palabras, por qué el modelo rechazó una
// anonimización.
func motivosDeRechazo(regla *models.ErrorValidacion) string {
	motivos := map[string]string{
		"error.estado_no_admite":         "la postulación es un borrador sin folio",
		"error.ya_anonimizada":           "la postulación ya estaba anonimizada",
		"error.proceso_en_curso":         "su convocatoria aún no cierra y la postulación sigue en proceso; sólo puede anonimizarse al cierre, o si fue descartada",
		"error.confirmacion_requerida":   "no se escribió la palabra " + palabraDeConfirmacion,
		"error.folio_no_coincide":        "el folio escrito para confirmar no coincide",
		"error.fundamento_requerido":     "falta --fundamento (entre 10 y 500 caracteres)",
		"error.cuenta_con_postulaciones": "la cuenta tiene otras postulaciones sin anonimizar; anonimícelas antes o quite --con-cuenta",
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
