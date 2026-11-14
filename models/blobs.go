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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Almacén de archivos (§7.8). Los documentos de las postulaciones —partituras,
// audio, fotografías— y los acuses NO se guardan en la base de datos: viven en
// disco, direccionados por el SHA-256 de su contenido, y la base sólo guarda
// sus metadatos en la tabla «archivo».
//
// Direccionar por contenido da tres cosas a la vez: deduplicación (el mismo
// archivo subido dos veces ocupa una vez), integridad verificable (el nombre
// del archivo ES su suma) y un respaldo incremental trivial (un archivo jamás
// cambia: sólo aparecen archivos nuevos).

// ErrArchivoDemasiadoGrande indica que el contenido rebasó el límite dado a
// Guardar. No queda nada escrito en el almacén.
var ErrArchivoDemasiadoGrande = errors.New("el archivo rebasa el tamaño máximo")

// Blob identifica un contenido ya guardado en el almacén.
type Blob struct {
	SHA256 string // 64 dígitos hexadecimales
	Clave  string // clave abstracta de almacenamiento; no es una ruta
	Tamano int64  // bytes
}

// BlobStore es el almacén de archivos. La tabla «archivo» guarda la Clave que
// devuelve, nunca una ruta: si algún día el acervo se muda a S3 o MinIO se
// añade otra implementación sin tocar el esquema (§7.8). Hoy sólo existe
// AlmacenEnDisco.
type BlobStore interface {
	// Guardar lee el contenido completo, calcula su SHA-256 y lo deja en el
	// almacén. Si pasa de limite bytes devuelve ErrArchivoDemasiadoGrande.
	Guardar(origen io.Reader, limite int64) (Blob, error)
	// Abrir devuelve el contenido de una clave. Quien llama debe cerrarlo.
	Abrir(clave string) (io.ReadSeekCloser, error)
	// Borrar quita el contenido de una clave. Sólo lo usa la anonimización
	// ARCO, que es la única operación destructiva del sistema (RN-28).
	// Borrar lo que ya no está no es un error.
	Borrar(clave string) error
}

// AlmacenEnDisco guarda cada contenido en raiz/ab/cd/abcd…, donde «abcd…» es
// su SHA-256. Los dos niveles de directorios evitan carpetas con decenas de
// miles de archivos.
type AlmacenEnDisco struct {
	raiz string
}

// subdirectorioTemporal recibe las subidas en curso. Está dentro del almacén
// para que el paso final sea un cambio de nombre en el mismo sistema de
// archivos, que es atómico.
const subdirectorioTemporal = "tmp"

// claveDeBlob es la única forma que puede tener una clave. Comprobarla antes
// de tocar el disco impide que una clave manipulada salga del almacén («../»).
var claveDeBlob = regexp.MustCompile(`^[0-9a-f]{2}/[0-9a-f]{2}/[0-9a-f]{64}$`)

// AbrirAlmacenEnDisco prepara el almacén en el directorio dado, creándolo si
// hace falta, y limpia las subidas que un apagado dejó a medias. Sólo debe
// abrirlo el servidor: al arrancar no hay ninguna subida en curso.
func AbrirAlmacenEnDisco(raiz string) (*AlmacenEnDisco, error) {
	temporal := filepath.Join(raiz, subdirectorioTemporal)
	// 0750: el acervo es material inédito; otros usuarios del sistema no lo leen.
	if err := os.MkdirAll(temporal, 0o750); err != nil {
		return nil, fmt.Errorf("crear el almacén de archivos: %w", err)
	}
	restos, err := os.ReadDir(temporal)
	if err != nil {
		return nil, fmt.Errorf("leer el almacén de archivos: %w", err)
	}
	for _, resto := range restos {
		if err := os.Remove(filepath.Join(temporal, resto.Name())); err != nil {
			return nil, fmt.Errorf("limpiar subidas incompletas: %w", err)
		}
	}
	return &AlmacenEnDisco{raiz: raiz}, nil
}

// Guardar escribe el contenido en un archivo temporal mientras calcula su
// suma, y al terminar lo mueve a su lugar definitivo. Si el contenido ya
// existía no se escribe dos veces. Ante cualquier error —incluido que quien
// sube corte la conexión— el temporal se borra.
func (a *AlmacenEnDisco) Guardar(origen io.Reader, limite int64) (blob Blob, err error) {
	temporal, err := os.CreateTemp(filepath.Join(a.raiz, subdirectorioTemporal), "subida-*")
	if err != nil {
		return Blob{}, fmt.Errorf("crear archivo temporal: %w", err)
	}
	defer func() {
		// Si el temporal ya se movió a su lugar, borrar no encuentra nada.
		if errBorrar := os.Remove(temporal.Name()); errBorrar != nil && !errors.Is(errBorrar, os.ErrNotExist) && err == nil {
			err = fmt.Errorf("borrar archivo temporal: %w", errBorrar)
		}
	}()

	suma := sha256.New()
	// Se lee un byte más que el límite: es la forma de saber que se rebasó.
	escritos, err := io.Copy(io.MultiWriter(temporal, suma), io.LimitReader(origen, limite+1))
	if err == nil && escritos > limite {
		err = ErrArchivoDemasiadoGrande
	}
	if err == nil {
		// Sync: el archivo debe estar en disco antes de que la base diga que existe.
		err = temporal.Sync()
	}
	if errCerrar := temporal.Close(); err == nil {
		err = errCerrar
	}
	if err != nil {
		if errors.Is(err, ErrArchivoDemasiadoGrande) {
			return Blob{}, err
		}
		return Blob{}, fmt.Errorf("escribir archivo: %w", err)
	}

	sumaHex := hex.EncodeToString(suma.Sum(nil))
	blob = Blob{SHA256: sumaHex, Clave: sumaHex[:2] + "/" + sumaHex[2:4] + "/" + sumaHex, Tamano: escritos}
	destino := filepath.Join(a.raiz, filepath.FromSlash(blob.Clave))
	if _, err := os.Stat(destino); err == nil {
		return blob, nil // deduplicación: ese contenido ya estaba
	} else if !errors.Is(err, os.ErrNotExist) {
		return Blob{}, fmt.Errorf("consultar el almacén: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil {
		return Blob{}, fmt.Errorf("crear directorio del almacén: %w", err)
	}
	// 0440: un archivo guardado no se modifica nunca.
	if err := os.Chmod(temporal.Name(), 0o440); err != nil {
		return Blob{}, fmt.Errorf("fijar permisos del archivo: %w", err)
	}
	if err := os.Rename(temporal.Name(), destino); err != nil {
		return Blob{}, fmt.Errorf("mover archivo al almacén: %w", err)
	}
	return blob, nil
}

// Abrir devuelve el archivo de una clave, o ErrNoEncontrado.
func (a *AlmacenEnDisco) Abrir(clave string) (io.ReadSeekCloser, error) {
	if !claveDeBlob.MatchString(clave) {
		return nil, fmt.Errorf("clave de almacenamiento inválida %q", clave)
	}
	archivo, err := os.Open(filepath.Join(a.raiz, filepath.FromSlash(clave)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoEncontrado
	}
	if err != nil {
		return nil, fmt.Errorf("abrir archivo del almacén: %w", err)
	}
	return archivo, nil
}

// AbrirAlmacenExistente abre un almacén que ya existe, sin crearlo y sin
// limpiar las subidas en curso. Es el modo de fimnme-cli, que puede
// ejecutarse mientras el servidor está recibiendo archivos.
func AbrirAlmacenExistente(raiz string) (*AlmacenEnDisco, error) {
	info, err := os.Stat(raiz)
	if err != nil {
		return nil, fmt.Errorf("abrir el almacén de archivos: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("el almacén de archivos %s no es un directorio", raiz)
	}
	return &AlmacenEnDisco{raiz: raiz}, nil
}

// Borrar quita un archivo del almacén. Si ya no estaba, no hace nada.
func (a *AlmacenEnDisco) Borrar(clave string) error {
	if !claveDeBlob.MatchString(clave) {
		return fmt.Errorf("clave de almacenamiento inválida %q", clave)
	}
	if err := os.Remove(filepath.Join(a.raiz, filepath.FromSlash(clave))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("borrar archivo del almacén: %w", err)
	}
	return nil
}

// Suma lee un archivo del almacén y devuelve el SHA-256 de su contenido y su
// tamaño. Si no está devuelve ErrNoEncontrado.
func (a *AlmacenEnDisco) Suma(clave string) (sha string, tamano int64, err error) {
	contenido, err := a.Abrir(clave)
	if err != nil {
		return "", 0, err
	}
	suma := sha256.New()
	tamano, err = io.Copy(suma, contenido)
	if errCerrar := contenido.Close(); err == nil {
		err = errCerrar
	}
	if err != nil {
		return "", 0, fmt.Errorf("leer archivo del almacén: %w", err)
	}
	return hex.EncodeToString(suma.Sum(nil)), tamano, nil
}

// Tamano devuelve el tamaño de un archivo del almacén sin leerlo, o
// ErrNoEncontrado.
func (a *AlmacenEnDisco) Tamano(clave string) (int64, error) {
	if !claveDeBlob.MatchString(clave) {
		return 0, fmt.Errorf("clave de almacenamiento inválida %q", clave)
	}
	info, err := os.Stat(filepath.Join(a.raiz, filepath.FromSlash(clave)))
	if errors.Is(err, os.ErrNotExist) {
		return 0, ErrNoEncontrado
	}
	if err != nil {
		return 0, fmt.Errorf("consultar archivo del almacén: %w", err)
	}
	return info.Size(), nil
}

// Recorrer llama a visitar con la clave de cada archivo guardado. No entra
// al subdirectorio de subidas en curso. Lo que haya en el almacén con un
// nombre que no es una clave se entrega a ajeno, si no es nil: no debería
// haber nada, y si lo hay conviene saberlo.
func (a *AlmacenEnDisco) Recorrer(visitar func(clave string) error, ajeno func(ruta string)) error {
	return filepath.WalkDir(a.raiz, func(ruta string, entrada os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("recorrer el almacén: %w", err)
		}
		relativa, err := filepath.Rel(a.raiz, ruta)
		if err != nil {
			return err
		}
		if entrada.IsDir() {
			if relativa == subdirectorioTemporal {
				return filepath.SkipDir
			}
			return nil
		}
		clave := filepath.ToSlash(relativa)
		if !claveDeBlob.MatchString(clave) || filepath.Base(clave)[:4] != strings.ReplaceAll(filepath.Dir(clave), "/", "") {
			if ajeno != nil {
				ajeno(clave)
			}
			return nil
		}
		return visitar(clave)
	})
}

var _ BlobStore = (*AlmacenEnDisco)(nil)
