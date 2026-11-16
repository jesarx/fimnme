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

package models

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Continuidad operativa (§5.17): verificación del acervo de archivos e
// instantáneas firmadas para el servidor espejo.
//
// Aquí no hay SQL —eso vive en repository.go—: sólo el trabajo sobre
// archivos y la firma. Una instantánea es una copia en caliente de la base
// (VACUUM INTO) acompañada de un manifiesto que dice qué archivo es, cuánto
// pesa y cuál es su SHA-256, firmado con HMAC-SHA256. Quien tiene la clave
// puede comprobar que la copia que recibió es la que se produjo y que nadie
// la tocó por el camino. El acervo se replica por contenido: cada archivo se
// llama como su suma, así que copiar es idempotente y verificar es volver a
// sumar.

// VerificacionDeAcervo es el resultado de comparar lo que la base dice que
// existe con lo que hay en disco (RF-CLI-08), en los dos sentidos.
type VerificacionDeAcervo struct {
	Registrados int // archivos que la base registra y cuyo contenido debe estar
	Correctos   int
	// Faltantes están registrados pero no en disco. Es pérdida de datos.
	Faltantes []Archivo
	// Alterados están en disco, pero su contenido no es el registrado.
	Alterados []Archivo
	// PurgadosPresentes se dieron por borrados en una anonimización y siguen
	// en disco: la purga quedó a medias y hay que terminarla.
	PurgadosPresentes []Archivo
	// Huerfanos son archivos en disco que la base no registra. No son
	// pérdida: suele ser una subida cuya transacción no llegó a confirmarse.
	Huerfanos []string
	// Ajenos son cosas en el almacén que no tienen forma de archivo guardado.
	Ajenos []string
}

// Integro indica que no hay pérdida ni alteración, y que ninguna purga quedó
// pendiente. Los huérfanos no rompen la integridad.
func (v VerificacionDeAcervo) Integro() bool {
	return len(v.Faltantes) == 0 && len(v.Alterados) == 0 && len(v.PurgadosPresentes) == 0
}

// VerificarAcervo comprueba que cada archivo registrado exista en el almacén
// y que su contenido coincida con su SHA-256, y busca lo que hay en disco sin
// registro. Con rapido sólo compara existencia y tamaño, sin leer cada
// archivo: sirve para una comprobación diaria; la completa, para la semanal.
func VerificarAcervo(ctx context.Context, archivos []Archivo, almacen *AlmacenEnDisco, rapido bool) (VerificacionDeAcervo, error) {
	var v VerificacionDeAcervo
	registradas := make(map[string]bool, len(archivos))
	for _, archivo := range archivos {
		if err := ctx.Err(); err != nil {
			return v, err
		}
		registradas[archivo.Clave] = true
		tamano, err := almacen.Tamano(archivo.Clave)
		presente := err == nil
		if err != nil && !errors.Is(err, ErrNoEncontrado) {
			return v, err
		}
		if !archivo.PurgadoAt.IsZero() {
			if presente {
				v.PurgadosPresentes = append(v.PurgadosPresentes, archivo)
			}
			continue
		}
		v.Registrados++
		switch {
		case !presente:
			v.Faltantes = append(v.Faltantes, archivo)
			continue
		case tamano != archivo.TamanoBytes:
			v.Alterados = append(v.Alterados, archivo)
			continue
		}
		if !rapido {
			suma, _, err := almacen.Suma(archivo.Clave)
			if err != nil {
				return v, err
			}
			if suma != archivo.SHA256 {
				v.Alterados = append(v.Alterados, archivo)
				continue
			}
		}
		v.Correctos++
	}
	err := almacen.Recorrer(func(clave string) error {
		if !registradas[clave] {
			v.Huerfanos = append(v.Huerfanos, clave)
		}
		return ctx.Err()
	}, func(ruta string) { v.Ajenos = append(v.Ajenos, ruta) })
	return v, err
}

// --- Instantáneas firmadas ----------------------------------------------------

const (
	// formatoDeInstantanea identifica la versión del manifiesto. Entra en la
	// firma: un manifiesto de otro formato no verifica.
	formatoDeInstantanea = "fimnme-espejo-v1"
	// DirectorioDeInstantaneas y DirectorioDeBlobsEspejo son los dos
	// subdirectorios del destino del espejo.
	DirectorioDeInstantaneas = "instantaneas"
	DirectorioDeBlobsEspejo  = "blobs"
	// longitudMinimaDeClave es lo menos que puede medir la clave del espejo.
	longitudMinimaDeClave = 32
)

// ManifiestoDeInstantanea acompaña a cada instantánea de la base: dice qué
// archivo es y lleva la firma que permite comprobarlo (RF-OPS-01 y 06).
type ManifiestoDeInstantanea struct {
	Formato     string `json:"formato"`
	Archivo     string `json:"archivo"` // nombre del archivo de la base, sin directorio
	SHA256      string `json:"sha256"`
	TamanoBytes int64  `json:"tamano_bytes"`
	CreadoAt    string `json:"creado_at"` // UTC, ISO-8601
	Version     string `json:"version"`   // del programa que la produjo
	// Estado del acervo replicado al momento de la instantánea.
	BlobsRegistrados int    `json:"blobs_registrados"`
	BlobsCopiados    int    `json:"blobs_copiados"`  // nuevos en esta instantánea
	BlobsFaltantes   int    `json:"blobs_faltantes"` // registrados que el origen no tenía
	BlobsRetirados   int    `json:"blobs_retirados"` // purgados por ARCO, quitados del espejo
	HMACSHA256       string `json:"hmac_sha256"`
}

// mensajeFirmado es el texto exacto sobre el que se calcula la firma: todos
// los campos del manifiesto menos la propia firma, uno por línea y en orden
// fijo. No se firma el JSON porque su forma puede variar sin que cambie su
// contenido.
func (m ManifiestoDeInstantanea) mensajeFirmado() []byte {
	return []byte(fmt.Sprintf("%s\n%s\n%s\n%d\n%s\n%s\n%d\n%d\n%d\n%d\n", m.Formato, m.Archivo, m.SHA256, m.TamanoBytes,
		m.CreadoAt, m.Version, m.BlobsRegistrados, m.BlobsCopiados, m.BlobsFaltantes, m.BlobsRetirados))
}

// Firmar calcula el HMAC-SHA256 del manifiesto con la clave del espejo.
func (m ManifiestoDeInstantanea) Firmar(clave []byte) string {
	firma := hmac.New(sha256.New, clave)
	firma.Write(m.mensajeFirmado())
	return hex.EncodeToString(firma.Sum(nil))
}

// FirmaValida indica si la firma del manifiesto corresponde a su contenido y
// a la clave. La comparación tarda lo mismo acierte o no.
func (m ManifiestoDeInstantanea) FirmaValida(clave []byte) bool {
	esperada, err := hex.DecodeString(m.Firmar(clave))
	recibida, errRecibida := hex.DecodeString(m.HMACSHA256)
	return err == nil && errRecibida == nil && m.Formato == formatoDeInstantanea && hmac.Equal(esperada, recibida)
}

// NuevaClaveDeEspejo genera una clave aleatoria y la guarda en un archivo
// que sólo su dueño puede leer. Se niega a sobrescribir una existente:
// cambiar la clave invalida la verificación de todas las instantáneas
// anteriores, y eso debe ser un acto deliberado.
func NuevaClaveDeEspejo(ruta string) error {
	aleatorio := make([]byte, longitudMinimaDeClave)
	if _, err := rand.Read(aleatorio); err != nil {
		return fmt.Errorf("generar la clave: %w", err)
	}
	// O_EXCL: falla si el archivo ya existe.
	archivo, err := os.OpenFile(ruta, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o400)
	if err != nil {
		return fmt.Errorf("crear el archivo de la clave: %w", err)
	}
	_, err = archivo.WriteString(hex.EncodeToString(aleatorio) + "\n")
	if errCerrar := archivo.Close(); err == nil {
		err = errCerrar
	}
	if err != nil {
		return fmt.Errorf("escribir la clave: %w", err)
	}
	return nil
}

// LeerClaveDeEspejo lee la clave del espejo de su archivo. La clave nunca se
// pasa como argumento ni como variable de entorno: quedaría en el historial
// de órdenes o a la vista en la lista de procesos. Rechaza un archivo que
// otros usuarios del sistema puedan leer.
func LeerClaveDeEspejo(ruta string) ([]byte, error) {
	info, err := os.Stat(ruta)
	if err != nil {
		return nil, fmt.Errorf("leer la clave del espejo: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("la clave del espejo %s es legible por otros usuarios (permisos %o): déjela en 400", ruta, info.Mode().Perm())
	}
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return nil, fmt.Errorf("leer la clave del espejo: %w", err)
	}
	clave := []byte(strings.TrimSpace(string(contenido)))
	if len(clave) < longitudMinimaDeClave {
		return nil, fmt.Errorf("la clave del espejo es demasiado corta: mide %d caracteres y debe medir al menos %d", len(clave), longitudMinimaDeClave)
	}
	return clave, nil
}

// sumaDeArchivo devuelve el SHA-256 y el tamaño de un archivo cualquiera.
func sumaDeArchivo(ruta string) (string, int64, error) {
	archivo, err := os.Open(ruta)
	if err != nil {
		return "", 0, err
	}
	suma := sha256.New()
	tamano, err := io.Copy(suma, archivo)
	if errCerrar := archivo.Close(); err == nil {
		err = errCerrar
	}
	return hex.EncodeToString(suma.Sum(nil)), tamano, err
}

// NombreDeInstantanea es el nombre del archivo de la base de una instantánea
// tomada en un momento dado: se ordenan solas por fecha.
func NombreDeInstantanea(momento time.Time) string {
	return "fimnme-" + momento.UTC().Format("20060102T150405Z") + ".db"
}

// rutaDeManifiesto es el archivo del manifiesto de una instantánea.
func rutaDeManifiesto(rutaDeLaBase string) string {
	return strings.TrimSuffix(rutaDeLaBase, ".db") + ".json"
}

// SincronizarBlobs replica el acervo en el destino del espejo, por
// contenido:
//
//   - copia los archivos registrados que el destino aún no tiene, sumándolos
//     mientras los copia: si lo leído no es lo registrado, no se da por bueno;
//   - cuenta como faltantes los registrados que el origen no tiene;
//   - retira del destino los purgados por una anonimización (ARCO): un dato
//     que se borró aquí no debe sobrevivir en el espejo.
//
// Nunca borra del destino un archivo que siga registrado.
func SincronizarBlobs(ctx context.Context, origen *AlmacenEnDisco, destino string, archivos []Archivo) (registrados, copiados, faltantes, retirados int, err error) {
	if err := os.MkdirAll(filepath.Join(destino, subdirectorioTemporal), 0o750); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("crear el acervo del espejo: %w", err)
	}
	espejo := &AlmacenEnDisco{raiz: destino}
	for _, archivo := range archivos {
		if err := ctx.Err(); err != nil {
			return registrados, copiados, faltantes, retirados, err
		}
		if !archivo.PurgadoAt.IsZero() {
			if _, err := espejo.Tamano(archivo.Clave); err == nil {
				if err := espejo.Borrar(archivo.Clave); err != nil {
					return registrados, copiados, faltantes, retirados, err
				}
				retirados++
			}
			continue
		}
		registrados++
		if tamano, err := espejo.Tamano(archivo.Clave); err == nil && tamano == archivo.TamanoBytes {
			continue // ya estaba: el nombre es su suma, no hace falta volver a copiarlo
		}
		contenido, err := origen.Abrir(archivo.Clave)
		if errors.Is(err, ErrNoEncontrado) {
			faltantes++
			continue
		}
		if err != nil {
			return registrados, copiados, faltantes, retirados, err
		}
		// Guardar suma lo que escribe y lo deja bajo el nombre de su suma: si
		// el origen estuviera dañado, la copia no quedaría bajo la clave
		// registrada y se cuenta como faltante.
		blob, err := espejo.Guardar(contenido, archivo.TamanoBytes)
		if errCerrar := contenido.Close(); err == nil {
			err = errCerrar
		}
		switch {
		case errors.Is(err, ErrArchivoDemasiadoGrande) || (err == nil && blob.SHA256 != archivo.SHA256):
			faltantes++
			if err == nil {
				if err := espejo.Borrar(blob.Clave); err != nil {
					return registrados, copiados, faltantes, retirados, err
				}
			}
		case err != nil:
			return registrados, copiados, faltantes, retirados, fmt.Errorf("copiar %s al espejo: %w", archivo.SHA256, err)
		default:
			copiados++
		}
	}
	return registrados, copiados, faltantes, retirados, nil
}

// EscribirManifiesto firma el manifiesto y lo guarda junto a su instantánea.
func EscribirManifiesto(rutaDeLaBase string, m ManifiestoDeInstantanea, clave []byte) (ManifiestoDeInstantanea, error) {
	m.Formato = formatoDeInstantanea
	m.Archivo = filepath.Base(rutaDeLaBase)
	m.HMACSHA256 = m.Firmar(clave)
	texto, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	if err := os.WriteFile(rutaDeManifiesto(rutaDeLaBase), append(texto, '\n'), 0o640); err != nil {
		return m, fmt.Errorf("escribir el manifiesto: %w", err)
	}
	return m, nil
}

// SumaDeInstantanea devuelve el SHA-256 y el tamaño del archivo de una
// instantánea, para su manifiesto.
func SumaDeInstantanea(rutaDeLaBase string) (string, int64, error) {
	suma, tamano, err := sumaDeArchivo(rutaDeLaBase)
	if err != nil {
		return "", 0, fmt.Errorf("sumar la instantánea: %w", err)
	}
	return suma, tamano, nil
}

// Instantaneas lista los archivos de base que hay en el destino del espejo,
// de la más antigua a la más reciente.
func Instantaneas(destino string) ([]string, error) {
	entradas, err := os.ReadDir(filepath.Join(destino, DirectorioDeInstantaneas))
	if err != nil {
		return nil, fmt.Errorf("leer las instantáneas: %w", err)
	}
	var nombres []string
	for _, entrada := range entradas {
		if nombre := entrada.Name(); !entrada.IsDir() && strings.HasPrefix(nombre, "fimnme-") && strings.HasSuffix(nombre, ".db") {
			nombres = append(nombres, nombre)
		}
	}
	slices.Sort(nombres)
	return nombres, nil
}

// VerificacionDeInstantanea es el resultado de comprobar una instantánea del
// espejo.
type VerificacionDeInstantanea struct {
	Manifiesto ManifiestoDeInstantanea
	// FirmaValida: el manifiesto es el que se firmó con esta clave.
	FirmaValida bool
	// BaseIntegra: el archivo de la base es el que el manifiesto describe.
	BaseIntegra bool
	// Acervo es la comparación entre los archivos que la base de la
	// instantánea registra y los que hay en el acervo del espejo.
	Acervo VerificacionDeAcervo
}

// Correcta indica que la instantánea puede usarse para restaurar: firma
// válida, base íntegra y acervo completo.
func (v VerificacionDeInstantanea) Correcta() bool {
	return v.FirmaValida && v.BaseIntegra && v.Acervo.Integro()
}

// VerificarInstantanea comprueba una instantánea en el destino del espejo
// (RF-CLI-06): que la firma del manifiesto sea válida, que el archivo de la
// base sea el descrito y que cada archivo que esa base registra esté en el
// acervo del espejo con el contenido correcto. No necesita la base en
// producción: está pensada para ejecutarse en el servidor espejo. leerArchivos
// abre la base de la instantánea y devuelve sus archivos registrados; lo da
// repository.go, que es donde vive el SQL.
func VerificarInstantanea(ctx context.Context, destino, nombre string, clave []byte, rapido bool,
	leerArchivos func(ctx context.Context, rutaDeLaBase string) ([]Archivo, error)) (VerificacionDeInstantanea, error) {
	var v VerificacionDeInstantanea
	rutaDeLaBase := filepath.Join(destino, DirectorioDeInstantaneas, nombre)
	texto, err := os.ReadFile(rutaDeManifiesto(rutaDeLaBase))
	if err != nil {
		return v, fmt.Errorf("leer el manifiesto: %w", err)
	}
	if err := json.Unmarshal(texto, &v.Manifiesto); err != nil {
		return v, fmt.Errorf("el manifiesto no es JSON válido: %w", err)
	}
	// El manifiesto debe hablar de ESTE archivo: un manifiesto válido de otra
	// instantánea no sirve para avalar ésta.
	v.FirmaValida = v.Manifiesto.FirmaValida(clave) && v.Manifiesto.Archivo == nombre
	suma, tamano, err := sumaDeArchivo(rutaDeLaBase)
	if err != nil {
		return v, fmt.Errorf("sumar la instantánea: %w", err)
	}
	v.BaseIntegra = suma == v.Manifiesto.SHA256 && tamano == v.Manifiesto.TamanoBytes
	if !v.FirmaValida || !v.BaseIntegra {
		// Sin firma o sin integridad no se abre la base: no se sabe qué es.
		return v, nil
	}
	archivos, err := leerArchivos(ctx, rutaDeLaBase)
	if err != nil {
		return v, err
	}
	espejo, err := AbrirAlmacenExistente(filepath.Join(destino, DirectorioDeBlobsEspejo))
	if err != nil {
		return v, err
	}
	v.Acervo, err = VerificarAcervo(ctx, archivos, espejo, rapido)
	return v, err
}

// DepurarInstantaneas borra las instantáneas anteriores a un momento dado,
// con sus manifiestos, y devuelve cuántas borró. Conserva siempre la más
// reciente, tenga la fecha que tenga. No toca el acervo.
func DepurarInstantaneas(destino string, anterioresA time.Time) (int, error) {
	nombres, err := Instantaneas(destino)
	if err != nil || len(nombres) <= 1 {
		return 0, err
	}
	limite := NombreDeInstantanea(anterioresA)
	borradas := 0
	for _, nombre := range nombres[:len(nombres)-1] {
		if nombre >= limite {
			break
		}
		ruta := filepath.Join(destino, DirectorioDeInstantaneas, nombre)
		for _, archivo := range []string{ruta, rutaDeManifiesto(ruta)} {
			if err := os.Remove(archivo); err != nil && !errors.Is(err, os.ErrNotExist) {
				return borradas, fmt.Errorf("borrar instantánea vencida: %w", err)
			}
		}
		borradas++
	}
	return borradas, nil
}

// PurgarArchivos borra del almacén los archivos que una anonimización marcó
// como purgados y devuelve cuántos no pudo borrar. La comparten el servidor y
// fimnme-cli. Si alguno falla no es grave: sigue marcado, la verificación del
// acervo lo señala y la purga puede repetirse.
func PurgarArchivos(almacen BlobStore, archivos []Archivo, alFallar func(Archivo, error)) (pendientes int) {
	for _, archivo := range archivos {
		if err := almacen.Borrar(archivo.Clave); err != nil {
			pendientes++
			if alFallar != nil {
				alFallar(archivo, err)
			}
		}
	}
	return pendientes
}
