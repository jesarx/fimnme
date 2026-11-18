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
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Orden «import»: importación asistida en dos fases (§5.15), con el mismo
// motor que la interfaz web.
//
//	fimnme-cli import --tipo TIPO --convocatoria ID --como CORREO ARCHIVO
//
// Analiza la hoja, muestra cada conflicto y pregunta cómo resolverlo; al
// final muestra lo que va a ocurrir y pide escribir IMPORTAR. Hasta ese
// momento no se ha escrito nada en el dominio, y si se interrumpe, la
// importación se cancela.

// palabraDeImportacion es lo que hay que escribir para aplicar la importación.
const palabraDeImportacion = "IMPORTAR"

// administradorPorCorreo localiza la cuenta de administrador en cuyo nombre
// se hace la importación. Cada resolución queda registrada con la persona
// que la dio (RF-IMP-03): no basta con «alguien desde la terminal».
func administradorPorCorreo(ctx context.Context, repositorio *models.Repositorio, correo string) (models.Usuario, error) {
	if correo == "" {
		return models.Usuario{}, errors.New("indique con --como el correo de la cuenta de administrador que resuelve la importación")
	}
	usuario, _, err := repositorio.UsuarioPorEmail(ctx, models.NormalizarEmail(correo))
	if errors.Is(err, models.ErrNoEncontrado) {
		return models.Usuario{}, fmt.Errorf("no existe ninguna cuenta con el correo %s", correo)
	}
	if err != nil {
		return models.Usuario{}, err
	}
	cuenta, err := repositorio.UsuarioPorID(ctx, usuario.ID)
	if err != nil {
		return models.Usuario{}, err
	}
	if !usuario.Activo || !slices.Contains(cuenta.Perfiles, models.PerfilAdminSistema) {
		return models.Usuario{}, fmt.Errorf("la cuenta %s no es de un administrador activo", correo)
	}
	return usuario, nil
}

// motivosDeImportacion explica, en palabras, por qué se rechazó algo en una
// importación. Cada error trae el renglón («fila 7») en su campo.
func motivosDeImportacion(regla *models.ErrorValidacion) string {
	motivos := map[string]string{
		"error.tipo_de_importacion":        "--tipo debe ser dotaciones, usuarios_jurado o usuarios_revisor",
		"error.hoja_vacia":                 "la hoja no tiene renglones de datos",
		"error.columnas_de_importacion":    "el primer renglón debe traer los nombres exactos de las columnas, todas las obligatorias y ninguna desconocida",
		"error.fila_incompleta":            "falta un dato obligatorio o alguno no es válido",
		"error.fila_repetida":              "repite a otro renglón del archivo",
		"error.demasiadas_filas":           "la hoja tiene más de 2000 renglones",
		"error.texto_largo_campo":          "un texto es demasiado largo",
		"error.dotacion_solo_compositores": "las dotaciones sólo existen en la convocatoria de Compositores",
		"error.convocatoria_cerrada":       "la convocatoria ya está cerrada",
		"error.jurados_completos":          "la convocatoria ya tiene sus tres jurados",
		"error.asignado_postula":           "esa cuenta postula en la convocatoria",
		"error.usuario_inactivo":           "esa cuenta está desactivada",
		"error.fila_ya_existe":             "crearía un registro que ya existe; vuelva a analizar el archivo",
		"error.base_cambio":                "el registro elegido ya no existe; vuelva a analizar el archivo",
		"error.conflictos_pendientes":      "quedan conflictos sin resolver",
		"error.correo_ya_registrado":       "ya existe una cuenta con ese correo: vincúlela u omita el renglón",
		"error.candidato_requerido":        "elija uno de los registros propuestos",
	}
	var lista []string
	for _, e := range regla.Errores {
		motivo, conocido := motivos[e.Clave]
		if !conocido {
			motivo = e.Clave
		}
		lista = append(lista, e.Campo+": "+motivo)
	}
	return strings.Join(lista, "\n  ")
}

// importar ejecuta «import».
func importar(dirDatos string, args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas := flag.NewFlagSet("import", flag.ContinueOnError)
		tipo := banderas.String("tipo", "", "dotaciones, usuarios_jurado o usuarios_revisor")
		convocatoriaID := banderas.Int64("convocatoria", 0, "id de la convocatoria de destino")
		como := banderas.String("como", "", "correo de la cuenta de administrador que resuelve la importación")
		urlBase := banderas.String("url-base", entorno("FIMNME_URL_BASE", "http://127.0.0.1:8080"),
			"dirección pública del sitio, para los enlaces de activación (FIMNME_URL_BASE)")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		if banderas.NArg() != 1 {
			return errors.New("indique el archivo: fimnme-cli import --tipo TIPO --convocatoria ID --como CORREO ARCHIVO")
		}
		administrador, err := administradorPorCorreo(ctx, repositorio, *como)
		if err != nil {
			return err
		}
		actor := actorCLI()
		actor.UsuarioID = administrador.ID
		contenido, err := os.ReadFile(banderas.Arg(0))
		if err != nil {
			return fmt.Errorf("leer el archivo: %w", err)
		}
		// El archivo original se conserva en el acervo (RF-IMP-09).
		almacen, err := abrirAcervo(dirDatos)
		if err != nil {
			return err
		}

		// --- Fase 1: Analyze. No escribe nada en el dominio.
		id, err := models.AnalizarImportacion(ctx, repositorio, almacen, actor, *tipo, *convocatoriaID, filepath.Base(banderas.Arg(0)), contenido)
		var regla *models.ErrorValidacion
		switch {
		case errors.As(err, &regla):
			return fmt.Errorf("el archivo no puede importarse:\n  %s", motivosDeImportacion(regla))
		case errors.Is(err, models.ErrNoEncontrado):
			return fmt.Errorf("no existe la convocatoria %d", *convocatoriaID)
		case err != nil:
			return err
		}
		importacion, err := repositorio.ImportacionPorID(ctx, id)
		if err != nil {
			return err
		}
		// Si no se llega a confirmar —por una respuesta, por un error o
		// porque se cortó la entrada—, la importación se cancela.
		confirmada := false
		defer func() {
			if !confirmada {
				if err := repositorio.CancelarImportacion(context.WithoutCancel(ctx), actor, id); err != nil {
					fmt.Fprintf(os.Stderr, "No se pudo cancelar la importación %d: %v\n", id, err)
				}
			}
		}()
		fmt.Fprintf(salida, "Importación %d: %d renglones, %d por resolver.\n", id, importacion.FilasTotal, len(importacion.Conflictos))

		lector := bufio.NewReader(entrada)
		preguntar := func(pregunta string) string {
			fmt.Fprint(salida, pregunta+" ")
			respuesta, _ := lector.ReadString('\n') // sin respuesta, no hay decisión
			return strings.ToLower(strings.TrimSpace(respuesta))
		}

		// --- Resolución. Primero, de una vez, los renglones que no se
		// parecen a nada de lo que existe; después, de uno en uno, los demás.
		sinCandidatos := 0
		for _, conflicto := range importacion.Conflictos {
			if len(conflicto.Candidatos) == 0 {
				sinCandidatos++
			}
		}
		if sinCandidatos > 0 {
			fmt.Fprintf(salida, "\n%d renglones no se parecen a ningún registro existente:\n", sinCandidatos)
			for _, conflicto := range importacion.Conflictos {
				if len(conflicto.Candidatos) == 0 {
					fmt.Fprintf(salida, "  fila %d: %s\n", conflicto.Fila, conflicto.ValorOrigen)
				}
			}
			switch preguntar("¿Qué hacer con ellos? [c] crear todos · [o] omitir todos · [u] decidir uno por uno:") {
			case "c":
				_, err = repositorio.ResolverSinCandidatos(ctx, actor, id, models.ResolucionCrear)
			case "o":
				_, err = repositorio.ResolverSinCandidatos(ctx, actor, id, models.ResolucionOmitir)
			case "u":
			default:
				return errors.New("importación cancelada: no se indicó qué hacer")
			}
			if err != nil {
				return err
			}
		}
		if importacion, err = repositorio.ImportacionPorID(ctx, id); err != nil {
			return err
		}
		for _, conflicto := range importacion.Conflictos {
			if conflicto.Resolucion != "" {
				continue
			}
			fmt.Fprintf(salida, "\nFila %d: %s\n", conflicto.Fila, conflicto.ValorOrigen)
			for n, candidato := range conflicto.Candidatos {
				fmt.Fprintf(salida, "  [%d] %s  (%s)\n", n+1, candidato.Descripcion, strings.ReplaceAll(candidato.Motivo, "_", " "))
			}
			for {
				respuesta := preguntar("  Número para vincular · [c] crear nuevo · [o] omitir:")
				resolucion, entidadID := "", int64(0)
				switch numero, errNumero := strconv.Atoi(respuesta); {
				case respuesta == "c":
					resolucion = models.ResolucionCrear
				case respuesta == "o":
					resolucion = models.ResolucionOmitir
				case errNumero == nil && numero >= 1 && numero <= len(conflicto.Candidatos):
					resolucion, entidadID = models.ResolucionVincular, conflicto.Candidatos[numero-1].ID
				case respuesta == "":
					return errors.New("importación cancelada: quedó un conflicto sin resolver")
				default:
					fmt.Fprintln(salida, "  No es una opción.")
					continue
				}
				err := repositorio.ResolverConflicto(ctx, actor, id, conflicto.ID, resolucion, entidadID)
				if errors.As(err, &regla) {
					fmt.Fprintf(salida, "  No se puede: %s\n", motivosDeImportacion(regla))
					continue
				}
				if err != nil {
					return err
				}
				break
			}
		}

		// --- Vista previa y fase 2: Commit.
		if importacion, err = repositorio.ImportacionPorID(ctx, id); err != nil {
			return err
		}
		efecto := importacion.Efecto()
		fmt.Fprintf(salida, "\nSe van a crear %d registros, vincular %d y omitir %d; %d renglones ya existían tal cual.\n",
			efecto.Crean, efecto.Vinculan, efecto.Omiten, efecto.SinConflicto)
		if preguntar("Escriba "+palabraDeImportacion+" para aplicar:") != strings.ToLower(palabraDeImportacion) {
			return errors.New("importación cancelada: no se escribió nada")
		}
		filas, err := models.FilasDeLaImportacion(almacen, importacion)
		if err != nil {
			return err
		}
		// Las cuentas nuevas nacen sin contraseña. Desde aquí no se envía
		// correo: los enlaces de activación se muestran al terminar.
		activaciones := make(map[int]models.ActivacionDeCuenta)
		enlaces := make(map[string]string)
		for _, fila := range models.CuentasPorCrear(importacion, filas) {
			claro, resumen, err := models.NuevoSecreto()
			if err != nil {
				return err
			}
			activaciones[fila.Numero] = models.ActivacionDeCuenta{Token: models.NuevoToken{Resumen: resumen, Proposito: models.TokenActivacion,
				ExpiraAt: time.Now().Add(models.VigenciaActivacion)}}
			enlaces[fila.Valores["email"]] = strings.TrimRight(*urlBase, "/") + "/cuenta/activar?token=" + url.QueryEscape(claro)
		}
		if _, err := repositorio.ConfirmarImportacion(ctx, actor, id, filas, activaciones); errors.As(err, &regla) {
			return fmt.Errorf("no se importó NADA:\n  %s", motivosDeImportacion(regla))
		} else if err != nil {
			return err
		}
		confirmada = true
		fmt.Fprintf(salida, "\nImportación %d aplicada.\n", id)
		if len(enlaces) > 0 {
			fmt.Fprintf(salida, "\nEnlaces de activación de las cuentas nuevas, válidos %d días y de un solo uso:\n", int(models.VigenciaActivacion.Hours()/24))
			correos := make([]string, 0, len(enlaces))
			for correo := range enlaces {
				correos = append(correos, correo)
			}
			slices.Sort(correos)
			for _, correo := range correos {
				fmt.Fprintf(salida, "  %s\n    %s\n", correo, enlaces[correo])
			}
			fmt.Fprintln(salida, "Entréguelos a sus titulares por un canal seguro.")
		}
		return nil
	}
}
