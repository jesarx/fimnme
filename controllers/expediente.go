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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jesarx/fimnme/models"
	"github.com/jesarx/fimnme/views"
)

// Expediente en PDF (§5.12.4): de una postulación, al momento; de una
// convocatoria, en segundo plano.
//
// El contenido se decide con los permisos de quien lo pide EN EL MOMENTO DE
// GENERARLO (RN-26), porque el archivo sale del sistema: lo que una persona
// no puede ver no llega a imprimirse.

// errSinSupervision indica que alguien sin perfil de supervisión intentó
// llegar a lo que cruza a los jurados. No debería ocurrir nunca: es la
// segunda cerradura.
var errSinSupervision = errors.New("los resultados de la evaluación son sólo de la supervisión")

// matrizDeSupervision es LA puerta de los controladores a la matriz de
// resultados, que es lo único que reúne las calificaciones de los tres
// jurados. La usan la consulta de resultados, los reportes y el expediente.
//
// Comprueba aquí mismo, y no en quien la llama, que la persona sea
// administrador u observador: así ningún camino nuevo puede olvidarlo. Una
// prueba (TestSupervisionSoloTrasComprobar) falla si Supervision() aparece
// en cualquier otra función.
func matrizDeSupervision(ctx context.Context, datos models.Persistencia, visor *models.Sesion, actor models.Actor, convocatoriaID int64) (models.MatrizDeResultados, error) {
	if !models.PuedeSupervisar(visor) {
		return models.MatrizDeResultados{}, errSinSupervision
	}
	return datos.Supervision().Matriz(ctx, actor, convocatoriaID)
}

// avanceDeJurados es la puerta de los controladores al avance de cada jurado
// (cuántas lleva de cuántas, sin puntajes). También cruza a los jurados, y
// también comprueba aquí que quien lo pide supervise.
func avanceDeJurados(ctx context.Context, datos models.Persistencia, visor *models.Sesion, convocatoriaID int64) ([]models.AvanceDeJurado, error) {
	if !models.PuedeSupervisar(visor) {
		return nil, errSinSupervision
	}
	return datos.Supervision().Avance(ctx, convocatoriaID)
}

// tamanoMaxFotografia es lo más que se lee de una fotografía para el
// expediente. El requisito ya la limita al subirla; esto es un segundo tope.
const tamanoMaxFotografia = 25 << 20

// armadorDeExpedientes reúne lo que el expediente de cada postulación
// necesita, para una misma persona. Recuerda lo que ya leyó de cada
// convocatoria: en un expediente de doscientas postulaciones la
// configuración y la matriz se leen una vez, no doscientas.
type armadorDeExpedientes struct {
	datos   models.Persistencia
	almacen models.BlobStore
	// visor es quien pide el expediente: de sus permisos sale el recorte.
	visor *models.Sesion
	actor models.Actor

	configuraciones map[int64]models.ConfiguracionConvocatoria
	matrices        map[int64]models.MatrizDeResultados
}

// nuevoArmador prepara un armador para una persona.
func nuevoArmador(datos models.Persistencia, almacen models.BlobStore, visor *models.Sesion, actor models.Actor) *armadorDeExpedientes {
	return &armadorDeExpedientes{datos: datos, almacen: almacen, visor: visor, actor: actor,
		configuraciones: make(map[int64]models.ConfiguracionConvocatoria), matrices: make(map[int64]models.MatrizDeResultados)}
}

// configuracion devuelve la configuración de una convocatoria.
func (ar *armadorDeExpedientes) configuracion(ctx context.Context, convocatoriaID int64) (models.ConfiguracionConvocatoria, error) {
	if cfg, leida := ar.configuraciones[convocatoriaID]; leida {
		return cfg, nil
	}
	cfg, err := ar.datos.ConfiguracionDeConvocatoria(ctx, convocatoriaID)
	if err == nil {
		ar.configuraciones[convocatoriaID] = cfg
	}
	return cfg, err
}

// armar reúne el expediente de una postulación para quien lo pide. puede es
// false si esa persona no puede ver la postulación. sinFotografia evita leer
// la fotografía del acervo cuando no va a incluirse.
//
// Carga sólo lo que el recorte permite y, al final, RecortarExpediente quita
// lo que sobre: son dos cerraduras, a propósito.
func (ar *armadorDeExpedientes) armar(ctx context.Context, p models.Postulacion, sinFotografia bool) (x models.Expediente, fotografia []byte, puede bool, err error) {
	recorte, puede := models.RecorteDelExpediente(ar.visor, p)
	if !puede {
		return x, nil, false, nil
	}
	cfg, err := ar.configuracion(ctx, p.ConvocatoriaID)
	if err != nil {
		return x, nil, true, err
	}
	titular, err := ar.datos.UsuarioPorID(ctx, p.UsuarioID)
	if err != nil {
		return x, nil, true, err
	}
	x = models.Expediente{Postulacion: p, Configuracion: cfg, Titular: models.TitularParaMostrar(p, titular.Usuario), Recorte: recorte}
	if recorte.Revision {
		if x.Revision, err = ar.datos.RevisionDe(ctx, p.ID); err != nil {
			return x, nil, true, err
		}
	}
	if recorte.EvaluacionPropia {
		// La evaluación de quien lo pide: JuradoRepo no sabe leer otra.
		propias := ar.datos.ParaJurado(ar.visor.Usuario.ID)
		for _, obraID := range models.UnidadesDeEvaluacion(cfg.Convocatoria.Tipo, p) {
			evaluacion, err := propias.Evaluacion(ctx, p.ID, obraID)
			if err != nil {
				return x, nil, true, err
			}
			evaluacion.PostulacionID, evaluacion.ObraID = p.ID, obraID
			x.Propias = append(x.Propias, evaluacion)
		}
	}
	if recorte.Supervision && models.EnEvaluacion(p.Estado) {
		matriz, leida := ar.matrices[p.ConvocatoriaID]
		if !leida {
			if matriz, err = matrizDeSupervision(ctx, ar.datos, ar.visor, ar.actor, p.ConvocatoriaID); err != nil {
				return x, nil, true, err
			}
			ar.matrices[p.ConvocatoriaID] = matriz
		}
		x.Jurados = matriz.Jurados
		for _, fila := range matriz.Filas {
			if fila.Unidad.PostulacionID == p.ID {
				x.Resultados = append(x.Resultados, fila)
			}
		}
	}
	models.RecortarExpediente(&x)

	if sinFotografia {
		return x, nil, true, nil
	}
	for _, documento := range p.Documentos {
		if documento.Clave != "fotografia" {
			continue
		}
		// Si la fotografía no puede leerse el expediente sale sin ella: un
		// archivo dañado no debe impedir consultar todo lo demás.
		if contenido, errAbrir := ar.almacen.Abrir(documento.Archivo.Clave); errAbrir == nil {
			fotografia, err = io.ReadAll(io.LimitReader(contenido, tamanoMaxFotografia))
			if errCerrar := contenido.Close(); err == nil {
				err = errCerrar
			}
			if err != nil {
				fotografia = nil
			}
		}
		break
	}
	return x, fotografia, true, nil
}

// opcionesDeExpediente lee de la petición las opciones del PDF: «idioma» (es
// o en; por omisión, el de quien lo pide), «fotografia=no» y «hoja=si».
func opcionesDeExpediente(r *http.Request) (idioma string, sinFotografia, hoja bool) {
	consulta := r.URL.Query()
	idioma = consulta.Get("idioma")
	if idioma != models.IdiomaES && idioma != models.IdiomaEN {
		idioma = idiomaDe(r)
	}
	return idioma, consulta.Get("fotografia") == "no", consulta.Get("hoja") == "si"
}

// nombreDeSesion es el nombre de la persona en sesión, para el pie del PDF.
func nombreDeSesion(s *models.Sesion) string {
	return strings.TrimSpace(s.Usuario.Nombre + " " + s.Usuario.Apellidos)
}

// GET /api/v1/postulaciones/{id}/expediente   — expediente en PDF de una postulación
//
// Lo pide quien puede ver la postulación: su titular, los revisores y el
// jurado de su convocatoria, y la supervisión. Cada quien recibe lo que le
// corresponde (RF-ARCH-41). Parámetros: idioma, fotografia=no, hoja=si.
// Queda registrado.
func (a *App) apiExpediente(w http.ResponseWriter, r *http.Request) {
	id, ok := a.idDeRuta(w, r)
	if !ok {
		return
	}
	p, ok := a.postulacionVisible(w, r, id)
	if !ok {
		return
	}
	sesion := sesionDe(r)
	idioma, sinFotografia, hoja := opcionesDeExpediente(r)
	armador := nuevoArmador(a.datos, a.almacen, sesion, a.actorDe(r))
	x, fotografia, puede, err := armador.armar(r.Context(), p, sinFotografia)
	if err == nil && !puede {
		err = models.ErrNoEncontrado
	}
	if a.falloDatos(w, r, err) {
		return
	}
	expediente := a.vistas.NuevoExpediente(views.OpcionesDeExpediente{Idioma: idioma, Solicitante: nombreDeSesion(sesion),
		Generado: a.ahora(), SinFotografia: sinFotografia, HojaDeCalificacion: hoja})
	expediente.Agregar(x, fotografia)
	contenido, err := expediente.Bytes("")
	if err != nil {
		a.log.Error("no se pudo componer el expediente", "postulacion", p.ID, "error", err)
		a.errorAPI(w, r, http.StatusInternalServerError, "error_interno")
		return
	}
	if a.falloDatos(w, r, a.datos.RegistrarExpediente(r.Context(), a.actorDe(r), p.ID)) {
		return
	}
	nombre := "expediente_" + models.NombreParaArchivo(p.Folio) + ".pdf"
	if p.Folio == "" {
		nombre = fmt.Sprintf("expediente_borrador_%d.pdf", p.ID)
	}
	cabeceras := w.Header()
	cabeceras.Set("Content-Type", "application/pdf")
	cabeceras.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, nombre))
	cabeceras.Set("Cache-Control", "private, no-store")
	if _, err := w.Write(contenido); err != nil {
		a.log.Warn("no se terminó de enviar el expediente", "postulacion", p.ID, "error", err)
	}
}

// POST /api/v1/expedientes   — pide el expediente de una convocatoria
//
// Un solo PDF con las postulaciones de UNA convocatoria que la persona puede
// ver y que pasan el filtro (los mismos parámetros que el listado del
// archivo; «convocatoria» es obligatorio), con portada, índice y marcadores.
// Opciones: idioma, fotografia=no y hoja=si (hoja de calificación en blanco
// tras cada postulación).
//
// Se genera en segundo plano: responde 202 con la tarea, que se consulta y se
// baja igual que una descarga masiva, en /api/v1/exportaciones/{id}. Al
// terminar se avisa por correo.
func (a *App) apiPedirExpediente(w http.ResponseWriter, r *http.Request) {
	sesion := sesionDe(r)
	if models.AlcanceDe(sesion).Vacio() {
		a.errorAPI(w, r, http.StatusForbidden, "sin_permiso")
		return
	}
	filtro, errores := filtroDeLaPeticion(r)
	if filtro.ConvocatoriaID == 0 {
		errores = append(errores, models.ErrorCampo{Campo: "convocatoria", Clave: "error.convocatoria_requerida"})
	}
	if len(errores) > 0 {
		a.falloDatos(w, r, &models.ErrorValidacion{Errores: errores})
		return
	}
	idioma, sinFotografia, hoja := opcionesDeExpediente(r)
	// El alcance NO se guarda: se calcula al generar, con los permisos que la
	// persona tenga entonces.
	id, err := a.datos.EncolarExportacion(r.Context(), a.actorDe(r), models.TareaExpediente,
		models.ParametrosDeExportacion{Filtro: filtro, Idioma: idioma, SinFotografia: sinFotografia, HojaDeCalificacion: hoja})
	if a.falloDatos(w, r, err) {
		return
	}
	tarea, err := a.datos.ExportacionPorID(r.Context(), id)
	if a.falloDatos(w, r, err) {
		return
	}
	a.responder(w, http.StatusAccepted, views.ExportacionJSON(tarea))
}

// expedienteDeTarea compone el expediente de una tarea en segundo plano y
// devuelve el PDF y cuántas postulaciones lleva.
func (t *TrabajadorExportaciones) expedienteDeTarea(ctx context.Context, tarea models.TareaDeExportacion) ([]byte, int, error) {
	// Los permisos de quien lo pidió, tal como están AHORA.
	visor, err := t.Datos.SesionDeUsuario(ctx, tarea.SolicitadoPor)
	if err != nil {
		return nil, 0, fmt.Errorf("leer los permisos de quien pidió el expediente: %w", err)
	}
	parametros := tarea.Parametros
	filtro := parametros.Filtro
	filtro.Pagina, filtro.Tamano = 0, 0 // todas, sin paginar
	renglones, _, err := t.Datos.ListarPostulaciones(ctx, models.AlcanceDe(visor), filtro)
	if err != nil {
		return nil, 0, err
	}
	actor := models.Actor{UsuarioID: visor.Usuario.ID, Perfil: string(visor.PerfilPrincipal()), Agente: "fimnme-web (expediente en segundo plano)"}
	armador := nuevoArmador(t.Datos, t.Almacen, visor, actor)
	cfg, err := armador.configuracion(ctx, filtro.ConvocatoriaID)
	if err != nil {
		return nil, 0, err
	}
	expediente := t.Vistas.NuevoExpediente(views.OpcionesDeExpediente{Idioma: parametros.Idioma, Solicitante: nombreDeSesion(visor),
		Generado: t.ahora(), SinFotografia: parametros.SinFotografia, HojaDeCalificacion: parametros.HojaDeCalificacion})
	for _, renglon := range renglones {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err() // apagado en curso: la tarea se rehará
		}
		p, err := t.Datos.PostulacionPorID(ctx, renglon.ID)
		if err != nil {
			return nil, 0, err
		}
		x, fotografia, puede, err := armador.armar(ctx, p, parametros.SinFotografia)
		if err != nil {
			return nil, 0, fmt.Errorf("postulación %d: %w", p.ID, err)
		}
		if puede {
			expediente.Agregar(x, fotografia)
		}
	}
	titulo := cfg.Convocatoria.Titulo(parametros.Idioma) + " · " + cfg.Convocatoria.Edicion.Nombre(parametros.Idioma)
	contenido, err := expediente.Bytes(titulo)
	return contenido, expediente.Postulaciones(), err
}
