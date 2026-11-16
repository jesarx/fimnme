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
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jesarx/fimnme/models"
)

// Orden «crear-usuario»: da de alta una cuenta interna con un perfil global
// (admin_sistema o coordinacion_nacional) y entrega su enlace de activación.
//
// Es la forma de crear al PRIMER administrador, cuando aún no hay nadie que
// pueda hacerlo desde el panel. La cuenta nace sin contraseña: quien abre el
// enlace la elige. El enlace se imprime aquí en vez de enviarse por correo,
// para no depender de que el correo ya esté configurado.

// crearUsuario interpreta los argumentos de la orden y devuelve la operación.
func crearUsuario(args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas := flag.NewFlagSet("crear-usuario", flag.ContinueOnError)
		perfil := banderas.String("perfil", "", "perfil global: admin_sistema o coordinacion_nacional")
		email := banderas.String("email", "", "correo de la cuenta")
		nombre := banderas.String("nombre", "", "nombre(s)")
		apellidos := banderas.String("apellidos", "", "apellidos")
		idioma := banderas.String("idioma", models.IdiomaES, "idioma preferido: es o en")
		urlBase := banderas.String("url-base", entorno("FIMNME_URL_BASE", "http://127.0.0.1:8080"),
			"dirección pública del sitio, para armar el enlace (FIMNME_URL_BASE)")
		if err := banderas.Parse(args); err != nil {
			return err
		}

		// Las mismas reglas que aplica el servidor web.
		correo := models.NormalizarEmail(*email)
		*nombre, *apellidos = strings.TrimSpace(*nombre), strings.TrimSpace(*apellidos)
		var problemas []string
		if !models.PerfilGlobalAsignable(models.Perfil(*perfil)) {
			problemas = append(problemas, "--perfil debe ser admin_sistema o coordinacion_nacional")
		}
		if !models.EmailValido(correo) {
			problemas = append(problemas, "--email no es un correo válido")
		}
		for _, e := range models.ValidarDatosPersonales(*nombre, *apellidos, *idioma) {
			problemas = append(problemas, "--"+e.Campo+" falta o no es válido")
		}
		if len(problemas) > 0 {
			return errors.New("datos incompletos:\n  " + strings.Join(problemas, "\n  "))
		}

		pendientes, err := repositorio.MigracionesPendientes(ctx)
		if err != nil {
			return err
		}
		if len(pendientes) > 0 {
			return errors.New("el esquema no está al día; ejecute primero: fimnme-cli migrate")
		}

		_, enlace, err := altaConEnlace(ctx, repositorio, models.NuevoUsuario{
			Email: correo, Nombre: *nombre, Apellidos: *apellidos, Idioma: *idioma,
			PasswordHash: models.PasswordSinDefinir, Perfil: models.Perfil(*perfil),
		}, *urlBase)
		if errors.Is(err, models.ErrEmailEnUso) {
			return fmt.Errorf("ya existe una cuenta con el correo %s", correo)
		}
		if err != nil {
			return err
		}

		fmt.Printf("Cuenta creada: %s (perfil %s).\n\n", correo, *perfil)
		fmt.Printf("Enlace de activación, válido %d días y de un solo uso:\n\n    %s\n\n",
			int(models.VigenciaActivacion.Hours()/24), enlace)
		fmt.Println("Entréguelo a su titular por un canal seguro: quien lo abra elige la contraseña de la cuenta.")
		return nil
	}
}

// altaConEnlace crea una cuenta interna sin contraseña y devuelve su id y el
// enlace de activación, de un solo uso. El enlace no se envía por correo: lo
// entrega quien opera la herramienta, para no depender de que el correo ya
// esté configurado. La usan «crear-usuario» y el modo interactivo.
func altaConEnlace(ctx context.Context, repositorio *models.Repositorio, nuevo models.NuevoUsuario, urlBase string) (int64, string, error) {
	claro, resumen, err := models.NuevoSecreto()
	if err != nil {
		return 0, "", err
	}
	token := models.NuevoToken{
		Resumen:   resumen,
		Proposito: models.TokenActivacion,
		ExpiraAt:  time.Now().Add(models.VigenciaActivacion),
	}
	id, err := repositorio.CrearUsuario(ctx, actorCLI(), nuevo, &token)
	if err != nil {
		return 0, "", err
	}
	return id, strings.TrimRight(urlBase, "/") + "/cuenta/activar?token=" + url.QueryEscape(claro), nil
}

// Orden «enlace-activacion»: emite un enlace de activación NUEVO para una
// cuenta interna que aún no elige su contraseña, y lo imprime.
//
// Sirve donde el correo no sale (un servidor de pruebas con el puerto 25
// bloqueado) o cuando el enlace se perdió o venció. Anula los enlaces de
// activación anteriores de esa cuenta, igual que «Reenviar activación» del
// panel, y queda en la bitácora. Una cuenta ya activada no recibe enlace:
// quien la tiene recupera su contraseña desde la página de ingreso.

// enlaceDeActivacion interpreta los argumentos de la orden y devuelve la
// operación.
func enlaceDeActivacion(args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		banderas := flag.NewFlagSet("enlace-activacion", flag.ContinueOnError)
		email := banderas.String("email", "", "correo de la cuenta")
		urlBase := banderas.String("url-base", entorno("FIMNME_URL_BASE", "http://127.0.0.1:8080"),
			"dirección pública del sitio, para armar el enlace (FIMNME_URL_BASE)")
		if err := banderas.Parse(args); err != nil {
			return err
		}
		correo := models.NormalizarEmail(*email)
		if !models.EmailValido(correo) {
			return errors.New("--email no es un correo válido")
		}
		usuario, _, err := repositorio.UsuarioPorEmail(ctx, correo)
		if errors.Is(err, models.ErrNoEncontrado) {
			return fmt.Errorf("no hay ninguna cuenta con el correo %s", correo)
		}
		if err != nil {
			return err
		}
		cuenta, err := repositorio.UsuarioPorID(ctx, usuario.ID)
		if err != nil {
			return err
		}
		if cuenta.Activada {
			return fmt.Errorf("la cuenta %s ya está activada: su titular puede recuperar la contraseña desde «Ingresar»", correo)
		}
		claro, resumen, err := models.NuevoSecreto()
		if err != nil {
			return err
		}
		token := models.NuevoToken{Resumen: resumen, Proposito: models.TokenActivacion, ExpiraAt: time.Now().Add(models.VigenciaActivacion)}
		if err := repositorio.EmitirToken(ctx, actorCLI(), usuario.ID, token); err != nil {
			return err
		}
		enlace := strings.TrimRight(*urlBase, "/") + "/cuenta/activar?token=" + url.QueryEscape(claro)
		fmt.Fprintf(salida, "Enlace de activación de %s, válido %d días y de un solo uso (anula los anteriores):\n\n    %s\n\n",
			usuario.Email, int(models.VigenciaActivacion.Hours()/24), enlace)
		fmt.Fprintln(salida, "Entréguelo a su titular por un canal seguro: quien lo abra elige la contraseña de la cuenta.")
		return nil
	}
}

// Orden «segundo-factor restablecer --email CORREO»: borra la verificación en
// dos pasos de una cuenta que perdió su teléfono y sus códigos de
// recuperación (RF-AUT-09). Cierra sus sesiones; en el siguiente ingreso se
// le pedirá darse de alta otra vez, con una clave nueva.
//
// Sólo existe aquí, no en la web: quitar el segundo paso de un administrador
// exige acceso al servidor. Queda en la bitácora a nombre de la herramienta.

// segundoFactor interpreta los argumentos de la orden y devuelve la operación.
func segundoFactor(args []string) func(context.Context, *models.Repositorio) error {
	return func(ctx context.Context, repositorio *models.Repositorio) error {
		if len(args) == 0 || args[0] != "restablecer" {
			return errors.New("uso: fimnme-cli segundo-factor restablecer --email CORREO")
		}
		banderas := flag.NewFlagSet("segundo-factor restablecer", flag.ContinueOnError)
		email := banderas.String("email", "", "correo de la cuenta")
		if err := banderas.Parse(args[1:]); err != nil {
			return err
		}
		correo := models.NormalizarEmail(*email)
		if !models.EmailValido(correo) {
			return errors.New("--email no es un correo válido")
		}
		usuario, _, err := repositorio.UsuarioPorEmail(ctx, correo)
		if errors.Is(err, models.ErrNoEncontrado) {
			return fmt.Errorf("no hay ninguna cuenta con el correo %s", correo)
		}
		if err != nil {
			return err
		}
		if err := repositorio.RestablecerSegundoFactor(ctx, actorCLI(), usuario.ID); err != nil {
			return err
		}
		fmt.Fprintf(salida, "Verificación en dos pasos de %s restablecida y sesiones cerradas.\n", usuario.Email)
		fmt.Fprintln(salida, "En su próximo ingreso se le pedirá darla de alta otra vez, con una clave nueva.")
		return nil
	}
}
