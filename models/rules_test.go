// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"strings"
	"testing"
)

// registroValido devuelve un registro que pasa todas las reglas, para que cada
// prueba altere un solo campo.
func registroValido() DatosRegistro {
	return DatosRegistro{
		Email:       "ana.lopez@ejemplo.mx",
		Nombre:      "Ana",
		Apellidos:   "López Ñáñez",
		Password:    "una-frase-larga",
		Idioma:      IdiomaES,
		AceptaAviso: true,
	}
}

func TestValidarRegistroAceptaDatosCorrectos(t *testing.T) {
	if errores := ValidarRegistro(registroValido()); len(errores) != 0 {
		t.Fatalf("no se esperaban errores, hubo: %v", errores)
	}
}

func TestValidarRegistroSenalaCadaCampo(t *testing.T) {
	casos := []struct {
		nombre  string
		alterar func(*DatosRegistro)
		campo   string
		clave   string
	}{
		{"correo vacío", func(d *DatosRegistro) { d.Email = "" }, "email", "error.email_requerido"},
		{"correo sin dominio", func(d *DatosRegistro) { d.Email = "ana@local" }, "email", "error.email_invalido"},
		{"correo con nombre para mostrar", func(d *DatosRegistro) { d.Email = "Ana <ana@ejemplo.mx>" }, "email", "error.email_invalido"},
		{"nombre en blanco", func(d *DatosRegistro) { d.Nombre = "   " }, "nombre", "error.nombre_requerido"},
		{"nombre excesivo", func(d *DatosRegistro) { d.Nombre = strings.Repeat("a", 101) }, "nombre", "error.texto_largo"},
		{"apellidos vacíos", func(d *DatosRegistro) { d.Apellidos = "" }, "apellidos", "error.apellidos_requeridos"},
		{"contraseña vacía", func(d *DatosRegistro) { d.Password = "" }, "password", "error.password_requerida"},
		{"contraseña corta", func(d *DatosRegistro) { d.Password = "corta" }, "password", "error.password_corta"},
		{"contraseña excesiva", func(d *DatosRegistro) { d.Password = strings.Repeat("x", 129) }, "password", "error.password_larga"},
		{"idioma ajeno", func(d *DatosRegistro) { d.Idioma = "fr" }, "idioma", "error.idioma_invalido"},
		{"aviso sin aceptar", func(d *DatosRegistro) { d.AceptaAviso = false }, "acepta_aviso", "error.aviso_requerido"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			d := registroValido()
			c.alterar(&d)
			errores := ValidarRegistro(d)
			if len(errores) != 1 {
				t.Fatalf("se esperaba exactamente un error, hubo: %v", errores)
			}
			if errores[0].Campo != c.campo || errores[0].Clave != c.clave {
				t.Fatalf("se esperaba %s/%s, se obtuvo %s/%s", c.campo, c.clave, errores[0].Campo, errores[0].Clave)
			}
		})
	}
}

// La longitud de la contraseña se mide en caracteres: diez letras acentuadas
// ocupan veinte bytes pero deben contar como diez.
func TestValidarRegistroMideCaracteresNoBytes(t *testing.T) {
	d := registroValido()
	d.Password = strings.Repeat("ñ", LongitudMinPassword)
	if errores := ValidarRegistro(d); len(errores) != 0 {
		t.Fatalf("diez caracteres deben bastar, hubo: %v", errores)
	}
}

func TestValidarIngresoYRecuperacion(t *testing.T) {
	if errores := ValidarIngreso("ana@ejemplo.mx", "secreto"); len(errores) != 0 {
		t.Fatalf("ingreso válido rechazado: %v", errores)
	}
	if errores := ValidarIngreso("", ""); len(errores) != 2 {
		t.Fatalf("se esperaban dos errores con el formulario vacío, hubo: %v", errores)
	}
	if errores := ValidarRecuperacion("no-es-correo"); len(errores) != 1 {
		t.Fatalf("se esperaba un error, hubo: %v", errores)
	}
	if errores := ValidarRecuperacion("ana@ejemplo.mx"); len(errores) != 0 {
		t.Fatalf("recuperación válida rechazada: %v", errores)
	}
}

func TestVisibilidadDeConvocatoria(t *testing.T) {
	casos := []struct {
		estado           EstadoConvocatoria
		visible, vigente bool
		admite           bool
	}{
		{EstadoBorrador, false, false, false},
		{EstadoPublicada, true, true, false},
		{EstadoPostulacion, true, true, true},
		{EstadoRevisionDocumental, true, true, false},
		{EstadoEvaluacion, true, true, false},
		{EstadoCerrada, true, true, false},
		{EstadoArchivada, true, false, false},
	}
	for _, c := range casos {
		conv := Convocatoria{Estado: c.estado}
		if got := ConvocatoriaVisibleAlPublico(conv); got != c.visible {
			t.Errorf("%s: visible=%v, se esperaba %v", c.estado, got, c.visible)
		}
		if got := ConvocatoriaVigente(conv); got != c.vigente {
			t.Errorf("%s: vigente=%v, se esperaba %v", c.estado, got, c.vigente)
		}
		if got := AdmitePostulaciones(conv); got != c.admite {
			t.Errorf("%s: admite=%v, se esperaba %v", c.estado, got, c.admite)
		}
	}
}

// Si falta el texto en inglés se entrega el español y se avisa (RF-I18N-04).
func TestBasesConRespaldoEnEspanol(t *testing.T) {
	c := Convocatoria{BasesMDES: "bases", TituloES: "Título"}
	texto, respaldo := c.BasesMD(IdiomaEN)
	if texto != "bases" || !respaldo {
		t.Fatalf("se esperaba el respaldo en español, se obtuvo %q respaldo=%v", texto, respaldo)
	}
	if c.Titulo(IdiomaEN) != "Título" {
		t.Fatalf("el título debe respaldarse en español")
	}
}
