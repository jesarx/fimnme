// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

// Package models contiene el Modelo de la arquitectura MVC: entidades de
// dominio (este archivo), reglas de negocio (rules.go) y, más adelante, la
// persistencia (repository.go).
package models

import (
	"errors"
	"time"
)

// ErrNoEncontrado indica que el registro solicitado no existe o no es visible
// para quien lo pide. Los controladores lo traducen a una respuesta 404.
var ErrNoEncontrado = errors.New("registro no encontrado")

// Idiomas de primera clase de la plataforma (RF-I18N-01).
const (
	IdiomaES = "es"
	IdiomaEN = "en"
)

// TipoConvocatoria distingue las dos convocatorias de cada edición.
type TipoConvocatoria string

const (
	TipoInterpretes  TipoConvocatoria = "interpretes"
	TipoCompositores TipoConvocatoria = "compositores"
)

// Valido informa si el tipo es uno de los dos admitidos por el dominio.
func (t TipoConvocatoria) Valido() bool {
	return t == TipoInterpretes || t == TipoCompositores
}

// EstadoConvocatoria es la etapa del ciclo de vida de una convocatoria.
// La etapa es propiedad de la convocatoria, no de la edición: las dos
// convocatorias de un mismo año pueden llevar calendarios distintos.
type EstadoConvocatoria string

const (
	EstadoBorrador           EstadoConvocatoria = "borrador"
	EstadoPublicada          EstadoConvocatoria = "publicada"
	EstadoPostulacion        EstadoConvocatoria = "postulacion"
	EstadoRevisionDocumental EstadoConvocatoria = "revision_documental"
	EstadoEvaluacion         EstadoConvocatoria = "evaluacion"
	EstadoCerrada            EstadoConvocatoria = "cerrada"
	EstadoArchivada          EstadoConvocatoria = "archivada"
)

// ClaveEtapa identifica cada tramo del calendario de una convocatoria.
type ClaveEtapa string

const (
	EtapaPostulacion        ClaveEtapa = "postulacion"
	EtapaRevisionDocumental ClaveEtapa = "revision_documental"
	EtapaEvaluacion         ClaveEtapa = "evaluacion"
)

// Edicion es el año del Foro: contenedor de las convocatorias de ese año.
type Edicion struct {
	ID       int64
	Numero   int // p. ej. 49
	Anio     int // p. ej. 2027
	NombreES string
	NombreEN string
}

// Nombre devuelve el nombre de la edición en el idioma pedido. Si falta la
// versión en inglés se entrega la española (RF-I18N-04).
func (e Edicion) Nombre(idioma string) string {
	if idioma == IdiomaEN && e.NombreEN != "" {
		return e.NombreEN
	}
	return e.NombreES
}

// Etapa es un tramo del calendario. Las marcas de tiempo están SIEMPRE en UTC
// (RN-36); la conversión a America/Mexico_City ocurre en la vista.
type Etapa struct {
	Clave     ClaveEtapa
	IniciaAt  time.Time
	TerminaAt time.Time
}

// Convocatoria es una de las dos llamadas de cada edición, con sus bases,
// fechas y configuración propias.
type Convocatoria struct {
	ID        int64
	Edicion   Edicion
	Tipo      TipoConvocatoria
	TituloES  string
	TituloEN  string
	BasesMDES string // bases en Markdown, español
	BasesMDEN string // bases en Markdown, inglés
	Estado    EstadoConvocatoria
	MaxObras  int // 0 = sin límite (Intérpretes); Compositores: 2
	MaxVideos int
	Etapas    []Etapa
}

// Titulo devuelve el título en el idioma pedido, con respaldo en español.
func (c Convocatoria) Titulo(idioma string) string {
	if idioma == IdiomaEN && c.TituloEN != "" {
		return c.TituloEN
	}
	return c.TituloES
}

// BasesMD devuelve las bases en el idioma pedido. El segundo valor indica que
// se entregó la versión española como respaldo porque falta la inglesa
// (RF-I18N-04), para que la vista pueda advertirlo.
func (c Convocatoria) BasesMD(idioma string) (texto string, respaldo bool) {
	if idioma == IdiomaEN {
		if c.BasesMDEN != "" {
			return c.BasesMDEN, false
		}
		return c.BasesMDES, c.BasesMDES != ""
	}
	return c.BasesMDES, false
}

// Etapa busca un tramo del calendario por su clave.
func (c Convocatoria) Etapa(clave ClaveEtapa) (Etapa, bool) {
	for _, e := range c.Etapas {
		if e.Clave == clave {
			return e, true
		}
	}
	return Etapa{}, false
}

// Actor identifica a quien ejecuta una operación con efectos, para dejarla
// asentada en la bitácora de auditoría (RF-AUD-02).
type Actor struct {
	UsuarioID int64  // 0 = sin cuenta de usuario (p. ej. fimnme-cli)
	Perfil    string // perfil con el que actúa: admin_sistema, jurado, cli…
	IP        string
	Agente    string // user-agent del navegador, o identificación de fimnme-cli
}

// DatosRegistro son los datos del autorregistro de un postulante (RF-AUT-01).
// Sólo se piden correo, nombre, apellidos, contraseña e idioma: los datos
// artísticos pertenecen a la postulación, no a la cuenta.
type DatosRegistro struct {
	Email       string
	Nombre      string
	Apellidos   string
	Password    string
	Idioma      string
	AceptaAviso bool // aceptación del aviso de privacidad (LFPDPPP)
}

// ErrorCampo describe un problema de validación en un campo de formulario.
// Clave es una clave de traducción (views/i18n), nunca un texto visible: así
// ninguna cadena para el usuario queda escrita en el código (RF-I18N-02).
type ErrorCampo struct {
	Campo string
	Clave string
}
