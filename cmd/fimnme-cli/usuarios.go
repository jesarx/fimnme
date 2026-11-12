// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
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

		claro, resumen, err := models.NuevoSecreto()
		if err != nil {
			return err
		}
		token := models.NuevoToken{
			Resumen:   resumen,
			Proposito: models.TokenActivacion,
			ExpiraAt:  time.Now().Add(models.VigenciaActivacion),
		}
		_, err = repositorio.CrearUsuario(ctx, actorCLI(), models.NuevoUsuario{
			Email: correo, Nombre: *nombre, Apellidos: *apellidos, Idioma: *idioma,
			PasswordHash: models.PasswordSinDefinir, Perfil: models.Perfil(*perfil),
		}, &token)
		if errors.Is(err, models.ErrEmailEnUso) {
			return fmt.Errorf("ya existe una cuenta con el correo %s", correo)
		}
		if err != nil {
			return err
		}

		fmt.Printf("Cuenta creada: %s (perfil %s).\n\n", correo, *perfil)
		fmt.Printf("Enlace de activación, válido %d días y de un solo uso:\n\n    %s/cuenta/activar?token=%s\n\n",
			int(models.VigenciaActivacion.Hours()/24), strings.TrimRight(*urlBase, "/"), url.QueryEscape(claro))
		fmt.Println("Entréguelo a su titular por un canal seguro: quien lo abra elige la contraseña de la cuenta.")
		return nil
	}
}
