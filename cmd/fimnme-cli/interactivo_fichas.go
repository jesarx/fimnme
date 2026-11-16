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
	"fmt"
	"strconv"
	"strings"

	"github.com/jesarx/fimnme/models"
)

// Modo interactivo, segunda parte: lo que se configura dentro de una
// convocatoria. Cada pantalla lista los registros; al elegir uno se edita (y,
// donde el modelo lo permite, se elimina), y «n» da de alta uno nuevo.

// megabyte es la unidad en que se pregunta el tamaño máximo de un documento.
const megabyte = 1 << 20

// perfilesOperativos son los que se otorgan por convocatoria, con la letra
// con que se eligen y su nombre.
var perfilesOperativos = []struct {
	letra  string
	perfil models.Perfil
	nombre string
}{
	{"j", models.PerfilJurado, "Jurado"},
	{"r", models.PerfilRevisorDocumental, "Revisor documental"},
}

// nombreDePerfil nombra un perfil operativo.
func nombreDePerfil(perfil models.Perfil) string {
	for _, p := range perfilesOperativos {
		if p.perfil == perfil {
			return p.nombre
		}
	}
	return string(perfil)
}

// editarOBorrar pregunta qué hacer con un registro elegido de una lista.
func (c *consola) editarOBorrar() (string, error) {
	respuesta, err := c.leer("e) editar · b) eliminar · Intro) volver:")
	return strings.ToLower(respuesta), err
}

// --- Dotaciones (RF-CONV-06) -----------------------------------------------------

// pantallaDeDotaciones lista el catálogo de dotaciones de la convocatoria.
func (c *consola) pantallaDeDotaciones(convocatoriaID int64) (pantalla, error) {
	cfg, err := c.repositorio.ConfiguracionDeConvocatoria(c.ctx, convocatoriaID)
	if err != nil {
		return pantalla{}, err
	}
	p := pantalla{titulo: "Dotaciones · " + cfg.Convocatoria.TituloES}
	if cfg.Convocatoria.Tipo != models.TipoCompositores {
		p.resumen = []string{"Las dotaciones sólo existen en la convocatoria de Compositores."}
		return p, nil
	}
	p.resumen = []string{"Una dotación no se elimina: se desactiva, y deja de ofrecerse sin afectar a las obras que ya la usan."}
	for i, dotacion := range cfg.Dotaciones {
		estado := ""
		if !dotacion.Activa {
			estado = " [inactiva]"
		}
		p.opciones = append(p.opciones, opcionDeMenu{strconv.Itoa(i + 1),
			fmt.Sprintf("%s%s (orden %d)", dotacion.NombreES, estado, dotacion.Orden),
			func() error { return c.guardarDotacion(dotacion, "Dotación actualizada.") }})
	}
	p.opciones = append(p.opciones, opcionDeMenu{"n", "Nueva dotación", func() error {
		return c.guardarDotacion(models.Dotacion{ConvocatoriaID: convocatoriaID, Activa: true, Orden: len(cfg.Dotaciones) + 1}, "Dotación creada.")
	}})
	return p, nil
}

// guardarDotacion pregunta los datos de una dotación y la crea o actualiza.
func (c *consola) guardarDotacion(d models.Dotacion, exito string) error {
	f := ficha{c: c}
	f.texto("Nombre en español", &d.NombreES)
	f.texto("Nombre en inglés (opcional)", &d.NombreEN)
	f.texto("Descripción en español (opcional)", &d.DescripcionES)
	f.texto("Descripción en inglés (opcional)", &d.DescripcionEN)
	f.entero("Orden en la lista", &d.Orden)
	f.siNo("Activa", &d.Activa)
	if f.err != nil {
		return f.err
	}
	_, _, err := c.repositorio.GuardarDotacion(c.ctx, c.actor, d)
	return c.informar(err, exito)
}

// --- Requisitos documentales (RF-CONV-07) ----------------------------------------

// nombreDeDotacion nombra la dotación de un requisito; 0 es la regla general.
func nombreDeDotacion(dotaciones []models.Dotacion, id int64) string {
	for _, d := range dotaciones {
		if d.ID == id {
			return d.NombreES
		}
	}
	return "regla general"
}

// pantallaDeRequisitos lista los documentos que pide la convocatoria.
func (c *consola) pantallaDeRequisitos(convocatoriaID int64) (pantalla, error) {
	cfg, err := c.repositorio.ConfiguracionDeConvocatoria(c.ctx, convocatoriaID)
	if err != nil {
		return pantalla{}, err
	}
	p := pantalla{titulo: "Requisitos documentales · " + cfg.Convocatoria.TituloES,
		resumen: []string{"Un requisito ligado a una dotación sustituye, para las obras de esa dotación, a la regla general de la misma clave."}}
	for i, requisito := range cfg.Requisitos {
		caracter := "opcional"
		if requisito.Obligatorio {
			caracter = "obligatorio"
		}
		p.opciones = append(p.opciones, opcionDeMenu{strconv.Itoa(i + 1),
			fmt.Sprintf("%s · por %s · %s · %s · %s · hasta %d MB", requisito.Clave, requisito.Ambito,
				nombreDeDotacion(cfg.Dotaciones, requisito.DotacionID), caracter,
				strings.Join(requisito.MimesPermitidos, ","), requisito.TamanoMaxBytes/megabyte),
			func() error {
				respuesta, err := c.editarOBorrar()
				switch {
				case err != nil:
					return err
				case respuesta == "e":
					return c.guardarRequisito(requisito, cfg.Dotaciones, "Requisito actualizado.")
				case respuesta == "b":
					if confirmado, err := c.confirmar("¿Eliminar el requisito «" + requisito.Clave + "»?"); err != nil || !confirmado {
						return err
					}
					return c.informar(c.repositorio.BorrarRequisito(c.ctx, c.actor, requisito.ID), "Requisito eliminado.")
				}
				return nil
			}})
	}
	p.opciones = append(p.opciones, opcionDeMenu{"n", "Nuevo requisito", func() error {
		return c.guardarRequisito(models.RequisitoDocumental{ConvocatoriaID: convocatoriaID, Ambito: models.AmbitoObra,
			MimesPermitidos: []string{"application/pdf"}, TamanoMaxBytes: 30 * megabyte, MaxArchivos: 1,
			Orden: len(cfg.Requisitos) + 1}, cfg.Dotaciones, "Requisito creado.")
	}})
	return p, nil
}

// guardarRequisito pregunta los datos de un requisito y lo crea o actualiza.
func (c *consola) guardarRequisito(rq models.RequisitoDocumental, dotaciones []models.Dotacion, exito string) error {
	f := ficha{c: c}
	f.texto("Clave (fotografia, partitura, audio…)", &rq.Clave)
	f.texto("Etiqueta en español", &rq.EtiquetaES)
	f.texto("Etiqueta en inglés (opcional)", &rq.EtiquetaEN)
	f.texto("Ayuda en español (opcional)", &rq.AyudaES)
	f.texto("Ayuda en inglés (opcional)", &rq.AyudaEN)
	ambito := string(rq.Ambito)
	f.eleccion("Se adjunta por", []string{string(models.AmbitoPostulacion), string(models.AmbitoObra)}, &ambito)
	rq.Ambito = models.AmbitoRequisito(ambito)

	// La dotación se elige por su número en la lista; 0 es la regla general.
	if len(dotaciones) > 0 && f.err == nil {
		c.decir("  Dotación a la que aplica:   0) regla general, para todas")
		numero := 0
		for i, d := range dotaciones {
			c.decir("  %28d) %s", i+1, d.NombreES)
			if d.ID == rq.DotacionID {
				numero = i + 1
			}
		}
		for {
			f.entero("Dotación", &numero)
			if f.err != nil || (numero >= 0 && numero <= len(dotaciones)) {
				break
			}
			c.decir("  Elija un número de la lista.")
			numero = 0
		}
		if rq.DotacionID = 0; numero > 0 && f.err == nil {
			rq.DotacionID = dotaciones[numero-1].ID
		}
	}

	f.siNo("Obligatorio", &rq.Obligatorio)
	formatos := strings.Join(rq.MimesPermitidos, ",")
	f.texto("Formatos admitidos, separados por coma ("+strings.Join(models.MimesAdmitidos, ", ")+")", &formatos)
	rq.MimesPermitidos = strings.FieldsFunc(formatos, func(r rune) bool { return r == ',' || r == ' ' })
	megas := int(rq.TamanoMaxBytes / megabyte)
	f.entero("Tamaño máximo, en MB", &megas)
	rq.TamanoMaxBytes = int64(megas) * megabyte
	f.entero("Máximo de archivos", &rq.MaxArchivos)
	f.entero("Orden en la lista", &rq.Orden)
	if f.err != nil {
		return f.err
	}
	_, _, err := c.repositorio.GuardarRequisito(c.ctx, c.actor, rq)
	return c.informar(err, exito)
}

// --- Criterios de evaluación (RF-EVA-10) -----------------------------------------

// pantallaDeCriterios lista los criterios con que califica el jurado.
func (c *consola) pantallaDeCriterios(convocatoriaID int64) (pantalla, error) {
	cfg, err := c.repositorio.ConfiguracionDeConvocatoria(c.ctx, convocatoriaID)
	if err != nil {
		return pantalla{}, err
	}
	p := pantalla{titulo: "Criterios de evaluación · " + cfg.Convocatoria.TituloES,
		resumen: []string{"Con la evaluación en marcha los criterios ya no se modifican."}}
	for i, criterio := range cfg.Criterios {
		caracter := "opcional"
		if criterio.Obligatorio {
			caracter = "obligatorio"
		}
		p.opciones = append(p.opciones, opcionDeMenu{strconv.Itoa(i + 1),
			fmt.Sprintf("%s · %s · escala %d a %d · %s", criterio.Clave, criterio.EtiquetaES, criterio.EscalaMin, criterio.EscalaMax, caracter),
			func() error {
				respuesta, err := c.editarOBorrar()
				switch {
				case err != nil:
					return err
				case respuesta == "e":
					return c.guardarCriterio(criterio, "Criterio actualizado.")
				case respuesta == "b":
					if confirmado, err := c.confirmar("¿Eliminar el criterio «" + criterio.Clave + "»?"); err != nil || !confirmado {
						return err
					}
					return c.informar(c.repositorio.BorrarCriterio(c.ctx, c.actor, criterio.ID), "Criterio eliminado.")
				}
				return nil
			}})
	}
	p.opciones = append(p.opciones, opcionDeMenu{"n", "Nuevo criterio", func() error {
		return c.guardarCriterio(models.CriterioEvaluacion{ConvocatoriaID: convocatoriaID, EscalaMin: 1, EscalaMax: 5,
			Obligatorio: true, Orden: len(cfg.Criterios) + 1}, "Criterio creado.")
	}})
	return p, nil
}

// guardarCriterio pregunta los datos de un criterio y lo crea o actualiza.
func (c *consola) guardarCriterio(cr models.CriterioEvaluacion, exito string) error {
	f := ficha{c: c}
	f.texto("Clave (realizacion, artistico…)", &cr.Clave)
	f.texto("Etiqueta en español", &cr.EtiquetaES)
	f.texto("Etiqueta en inglés (opcional)", &cr.EtiquetaEN)
	f.texto("Descripción en español (opcional)", &cr.DescripcionES)
	f.texto("Descripción en inglés (opcional)", &cr.DescripcionEN)
	f.entero("Puntaje mínimo de la escala", &cr.EscalaMin)
	f.entero("Puntaje máximo de la escala", &cr.EscalaMax)
	f.siNo("Obligatorio", &cr.Obligatorio)
	f.entero("Orden en la lista", &cr.Orden)
	if f.err != nil {
		return f.err
	}
	_, _, err := c.repositorio.GuardarCriterio(c.ctx, c.actor, cr)
	return c.informar(err, exito)
}

// --- Jurados y revisores (RF-CONV-09) ---------------------------------------------

// pantallaDeAsignaciones lista a los jurados y revisores de la convocatoria.
func (c *consola) pantallaDeAsignaciones(convocatoriaID int64) (pantalla, error) {
	cfg, err := c.repositorio.ConfiguracionDeConvocatoria(c.ctx, convocatoriaID)
	if err != nil {
		return pantalla{}, err
	}
	p := pantalla{titulo: "Jurados y revisores · " + cfg.Convocatoria.TituloES,
		resumen: []string{fmt.Sprintf("Jurados: %d de %d · Revisores: %d (hace falta al menos uno).",
			cfg.Activos(models.PerfilJurado), models.CuotaJurados, cfg.Activos(models.PerfilRevisorDocumental))}}
	numero := 0
	for _, asignacion := range cfg.Asignaciones {
		if !asignacion.Activo {
			continue
		}
		numero++
		cuenta := ""
		if !asignacion.CuentaActivada {
			cuenta = " [cuenta sin activar]"
		}
		p.opciones = append(p.opciones, opcionDeMenu{strconv.Itoa(numero),
			fmt.Sprintf("%s · %s %s <%s>%s", nombreDePerfil(asignacion.Perfil), asignacion.Usuario.Nombre,
				asignacion.Usuario.Apellidos, asignacion.Usuario.Email, cuenta),
			func() error {
				// Retirar no borra lo que la persona ya hizo: sólo le quita el acceso.
				pregunta := fmt.Sprintf("¿Retirar a %s %s como %s?", asignacion.Usuario.Nombre, asignacion.Usuario.Apellidos,
					strings.ToLower(nombreDePerfil(asignacion.Perfil)))
				if confirmado, err := c.confirmar(pregunta); err != nil || !confirmado {
					return err
				}
				_, err := c.repositorio.RetirarAsignacion(c.ctx, c.actor, asignacion.ID)
				return c.informar(err, "Asignación retirada. Lo que ya hizo se conserva.")
			}})
	}
	p.opciones = append(p.opciones,
		opcionDeMenu{"a", "Asignar una cuenta que ya existe", func() error { return c.asignarExistente(convocatoriaID) }},
		opcionDeMenu{"c", "Crear una cuenta nueva y asignarla", func() error { return c.crearYAsignar(convocatoriaID) }})
	return p, nil
}

// elegirPerfil pregunta con qué perfil se asigna. Devuelve "" si se cancela.
func (c *consola) elegirPerfil() (models.Perfil, error) {
	respuesta, err := c.leer("Perfil — j) jurado · r) revisor documental · Intro) cancelar:")
	for _, p := range perfilesOperativos {
		if strings.EqualFold(respuesta, p.letra) {
			return p.perfil, err
		}
	}
	return "", err
}

// asignarExistente busca una cuenta y le otorga un perfil en la convocatoria.
// La cuenta la elige quien opera, de entre las que coinciden con lo que
// escribió: el sistema nunca decide por sí solo que alguien es la misma
// persona (RN-30).
func (c *consola) asignarExistente(convocatoriaID int64) error {
	texto, err := c.leer("Buscar la cuenta por correo, nombre o apellidos:")
	if err != nil || texto == "" {
		return err
	}
	const maximo = 15
	cuentas, total, err := c.repositorio.BuscarUsuarios(c.ctx, texto, 1, maximo)
	if err != nil {
		return err
	}
	if len(cuentas) == 0 {
		c.decir("Ninguna cuenta coincide. Puede crearla con la opción «c».")
		return nil
	}
	for i, cuenta := range cuentas {
		estado := ""
		if !cuenta.Usuario.Activo {
			estado = " [desactivada]"
		}
		c.decir("  %2d) %s %s <%s>%s", i+1, cuenta.Usuario.Nombre, cuenta.Usuario.Apellidos, cuenta.Usuario.Email, estado)
	}
	if total > len(cuentas) {
		c.decir("  … y %d más: afine la búsqueda.", total-len(cuentas))
	}
	respuesta, err := c.leer("Cuenta (Intro para cancelar):")
	if err != nil || respuesta == "" {
		return err
	}
	numero, err := strconv.Atoi(respuesta)
	if err != nil || numero < 1 || numero > len(cuentas) {
		c.decir("No hay ninguna opción «%s».", respuesta)
		return nil
	}
	perfil, err := c.elegirPerfil()
	if err != nil || perfil == "" {
		return err
	}
	_, err = c.repositorio.Asignar(c.ctx, c.actor, convocatoriaID, cuentas[numero-1].Usuario.ID, perfil)
	return c.informar(err, nombreDePerfil(perfil)+" asignado.")
}

// crearYAsignar da de alta una cuenta interna sin contraseña, muestra su
// enlace de activación y la asigna a la convocatoria. Si la asignación se
// rechaza (ya están los tres jurados, por ejemplo), la cuenta queda creada y
// se dice.
func (c *consola) crearYAsignar(convocatoriaID int64) error {
	perfil, err := c.elegirPerfil()
	if err != nil || perfil == "" {
		return err
	}
	nuevo := models.NuevoUsuario{Idioma: models.IdiomaES, PasswordHash: models.PasswordSinDefinir}
	f := ficha{c: c}
	f.texto("Correo", &nuevo.Email)
	f.texto("Nombre(s)", &nuevo.Nombre)
	f.texto("Apellidos", &nuevo.Apellidos)
	f.eleccion("Idioma", []string{models.IdiomaES, models.IdiomaEN}, &nuevo.Idioma)
	if f.err != nil {
		return f.err
	}
	nuevo.Email = models.NormalizarEmail(nuevo.Email)
	if errores := models.ValidarCuentaInterna(nuevo); len(errores) > 0 {
		return c.informar(&models.ErrorValidacion{Errores: errores}, "")
	}
	id, enlace, err := altaConEnlace(c.ctx, c.repositorio, nuevo, c.urlBase)
	if err != nil {
		return c.informar(err, "")
	}
	c.decir("Cuenta creada: %s.\nEnlace de activación, válido %d días y de un solo uso:\n\n    %s\n\n"+
		"Entréguelo a su titular por un canal seguro: quien lo abra elige la contraseña.",
		nuevo.Email, int(models.VigenciaActivacion.Hours()/24), enlace)
	_, err = c.repositorio.Asignar(c.ctx, c.actor, convocatoriaID, id, perfil)
	return c.informar(err, nombreDePerfil(perfil)+" asignado.")
}
