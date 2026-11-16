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

// Prueba de carga de RNF-01 (paso 14, ADR 0028): 200 usuarios concurrentes y
// 50 subidas simultáneas en el pico del último día; p95 < 500 ms en lecturas.
//
// Es una herramienta del operador, no un binario que se despliegue. Se corre
// en la MISMA máquina que el servidor, contra 127.0.0.1 (no a través del
// proxy) y contra una instalación de PRUEBA en entorno de desarrollo (la
// cookie de sesión de producción exige HTTPS):
//
//	go run ./deploy/carga --datos /ruta/a/datos --url http://127.0.0.1:8080 --convocatoria 1
//
// Crea en la base cuentas de postulante «carga-N@carga.invalid» ya
// verificadas (crear cuentas no es lo que se mide) y, por HTTP, todo lo
// demás: ingreso, postulación, autoguardado, lectura del formulario y
// subida de documentos. Deja esas cuentas y postulaciones en la base: por
// eso, nunca contra producción.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jesarx/fimnme/models"
)

// mediciones guarda la duración y el resultado de cada petición, por tipo.
type mediciones struct {
	mu       sync.Mutex
	tiempos  map[string][]time.Duration
	errores  map[string]int
	ejemplos map[string]string
}

// anotar registra una petición. ok=false cuenta como error.
func (m *mediciones) anotar(tipo string, d time.Duration, ok bool, detalle string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tiempos[tipo] = append(m.tiempos[tipo], d)
	if !ok {
		m.errores[tipo]++
		if _, hay := m.ejemplos[tipo]; !hay {
			m.ejemplos[tipo] = detalle
		}
	}
}

// percentil devuelve el percentil p (0–100) de una lista ordenada.
func percentil(ordenados []time.Duration, p float64) time.Duration {
	if len(ordenados) == 0 {
		return 0
	}
	i := int(float64(len(ordenados)-1) * p / 100)
	return ordenados[i]
}

// postulante es un usuario virtual: su sesión y su postulación.
type postulante struct {
	cliente *http.Client
	ruta    string // /api/v1/postulaciones/{id}
	pagina  string // /postulaciones/{id}/datos
	obra    int64
}

// conIPPropia hace que cada usuario virtual llegue con su propia IP, como 200
// personas desde 200 conexiones distintas. El servidor sólo acepta
// X-Forwarded-For de conexiones locales (las de su proxy inverso): por eso la
// prueba se corre en la misma máquina, contra 127.0.0.1 y no a través del
// proxy. Sin esto, el límite de intentos de ingreso por IP (30 por minuto)
// trataría a todos como uno solo.
type conIPPropia struct {
	ip   string
	base http.RoundTripper
}

func (c conIPPropia) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("X-Forwarded-For", c.ip)
	return c.base.RoundTrip(r)
}

// peticion hace una petición y la mide. esperado es el estado HTTP correcto.
func peticion(m *mediciones, tipo string, c *http.Client, r *http.Request, esperado int) []byte {
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	inicio := time.Now()
	respuesta, err := c.Do(r)
	if err != nil {
		m.anotar(tipo, time.Since(inicio), false, err.Error())
		return nil
	}
	cuerpo, _ := io.ReadAll(respuesta.Body)
	respuesta.Body.Close()
	ok := respuesta.StatusCode == esperado
	detalle := ""
	if !ok {
		detalle = fmt.Sprintf("%d %s", respuesta.StatusCode, strings.TrimSpace(string(cuerpo[:min(len(cuerpo), 160)])))
	}
	m.anotar(tipo, time.Since(inicio), ok, detalle)
	return cuerpo
}

func main() {
	datos := flag.String("datos", "", "directorio de datos del servidor de PRUEBA (para preparar las cuentas)")
	base := flag.String("url", "http://127.0.0.1:8080", "dirección del servidor")
	convocatoria := flag.Int64("convocatoria", 1, "convocatoria de Intérpretes en etapa de postulación")
	usuarios := flag.Int("usuarios", 200, "postulantes concurrentes")
	subidas := flag.Int("subidas", 50, "de ellos, cuántos suben un documento a la vez")
	tamanoMB := flag.Int("tamano-mb", 5, "tamaño de cada documento subido, en MB")
	duracion := flag.Duration("duracion", 60*time.Second, "duración de la fase de trabajo")
	flag.Parse()
	if *datos == "" {
		fmt.Fprintln(os.Stderr, "falta --datos: el directorio de datos de la instalación de PRUEBA")
		os.Exit(2)
	}
	ctx := context.Background()
	const password = "una frase larga para la prueba de carga"

	// 1. Cuentas verificadas, directo en la base (no se mide).
	repo, err := models.AbrirRepositorio(*datos)
	if err != nil {
		fmt.Fprintln(os.Stderr, "abrir la base:", err)
		os.Exit(1)
	}
	resumen, err := models.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sello := time.Now().Unix()
	correos := make([]string, *usuarios)
	for i := range correos {
		correos[i] = fmt.Sprintf("carga-%d-%d@carga.invalid", sello, i)
		claro, resumenToken, err := models.NuevoSecreto()
		if err == nil {
			_, err = repo.CrearUsuario(ctx, models.Actor{Perfil: models.ActorCLI}, models.NuevoUsuario{Email: correos[i], Nombre: "Carga",
				Apellidos: fmt.Sprint(i), Idioma: models.IdiomaES, PasswordHash: resumen, AvisoVersion: "carga", Perfil: models.PerfilPostulante},
				&models.NuevoToken{Resumen: resumenToken, Proposito: models.TokenVerificarEmail, ExpiraAt: time.Now().Add(time.Hour)})
		}
		if err == nil {
			_, err = repo.VerificarEmailConToken(ctx, models.Actor{Perfil: models.ActorCLI}, models.ResumenDeSecreto(claro))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "preparar cuenta:", err)
			os.Exit(1)
		}
	}
	if err := repo.Cerrar(); err != nil {
		fmt.Fprintln(os.Stderr, "cerrar la base:", err)
	}
	fmt.Printf("%d cuentas preparadas.\n", *usuarios)

	m := &mediciones{tiempos: map[string][]time.Duration{}, errores: map[string]int{}, ejemplos: map[string]string{}}
	transporte := &http.Transport{MaxIdleConnsPerHost: *usuarios}
	nuevo := func(i int) *http.Client {
		frasco, _ := cookiejar.New(nil)
		return &http.Client{Jar: frasco, Transport: conIPPropia{ip: fmt.Sprintf("10.77.%d.%d", i/250, i%250+1), base: transporte},
			Timeout:       2 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}

	// 2. Ingreso de todos a la vez, y su postulación con una obra.
	postulantes := make([]*postulante, *usuarios)
	var grupo sync.WaitGroup
	inicioIngreso := time.Now()
	for i := range postulantes {
		grupo.Add(1)
		go func(i int) {
			defer grupo.Done()
			c := nuevo(i)
			formulario := url.Values{"email": {correos[i]}, "password": {password}}
			r, _ := http.NewRequest("POST", *base+"/cuenta/ingresar", strings.NewReader(formulario.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			peticion(m, "ingreso", c, r, http.StatusSeeOther)
			r, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/convocatorias/%d/postulaciones", *base, *convocatoria), strings.NewReader(`{"subcategoria":"solista"}`))
			r.Header.Set("Content-Type", "application/json")
			var creada struct {
				ID int64 `json:"id"`
			}
			if json.Unmarshal(peticion(m, "crear postulación", c, r, http.StatusCreated), &creada) != nil || creada.ID == 0 {
				return
			}
			p := &postulante{cliente: c, ruta: fmt.Sprintf("/api/v1/postulaciones/%d", creada.ID), pagina: fmt.Sprintf("/postulaciones/%d/datos", creada.ID)}
			r, _ = http.NewRequest("POST", *base+p.ruta+"/obras", nil)
			var conObra struct {
				Obras []struct {
					ID int64 `json:"id"`
				} `json:"obras"`
			}
			if json.Unmarshal(peticion(m, "crear obra", c, r, http.StatusCreated), &conObra) == nil && len(conObra.Obras) > 0 {
				p.obra = conObra.Obras[0].ID
			}
			postulantes[i] = p
		}(i)
	}
	grupo.Wait()
	fmt.Printf("Ingreso y alta de %d postulantes en %s.\n", *usuarios, time.Since(inicioIngreso).Round(time.Millisecond))

	// 3. Fase de trabajo: autoguardado, lectura y subidas simultáneas.
	documento := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("0"), *tamanoMB<<20)...)
	fin := time.Now().Add(*duracion)
	for i, p := range postulantes {
		if p == nil {
			continue
		}
		grupo.Add(1)
		go func(i int, p *postulante) {
			defer grupo.Done()
			if i < *subidas && p.obra != 0 {
				// La subida, al principio y todas a la vez: el peor momento.
				var cuerpo bytes.Buffer
				escritor := multipart.NewWriter(&cuerpo)
				escritor.WriteField("clave", "partitura")
				escritor.WriteField("obra_id", fmt.Sprint(p.obra))
				parte, _ := escritor.CreateFormFile("archivo", "partitura.pdf")
				parte.Write(documento)
				escritor.Close()
				r, _ := http.NewRequest("POST", *base+p.ruta+"/documentos", &cuerpo)
				r.Header.Set("Content-Type", escritor.FormDataContentType())
				peticion(m, "subida", p.cliente, r, http.StatusCreated)
			}
			for n := 0; time.Now().Before(fin); n++ {
				var r *http.Request
				switch tipo := n % 10; {
				case tipo < 7:
					cuerpo := fmt.Sprintf(`{"campos":{"datos.semblanza":"Semblanza de prueba %d-%d","datos.telefono":"55 %08d"}}`, i, n, n)
					r, _ = http.NewRequest("PATCH", *base+p.ruta, strings.NewReader(cuerpo))
					r.Header.Set("Content-Type", "application/json")
					peticion(m, "autoguardado", p.cliente, r, http.StatusOK)
				case tipo < 9:
					r, _ = http.NewRequest("GET", *base+p.pagina, nil)
					peticion(m, "lectura: formulario", p.cliente, r, http.StatusOK)
				default:
					r, _ = http.NewRequest("GET", *base+p.ruta, nil)
					peticion(m, "lectura: postulación (API)", p.cliente, r, http.StatusOK)
				}
				// Una persona no captura sin pausa: entre 0.5 y 1.5 s.
				time.Sleep(time.Duration(500+(i*37+n*101)%1000) * time.Millisecond)
			}
		}(i, p)
	}
	grupo.Wait()

	// 4. Resultados.
	fmt.Printf("\n%-28s %8s %7s %9s %9s %9s\n", "operación", "pedidas", "errores", "p50", "p95", "p99")
	var tipos []string
	for tipo := range m.tiempos {
		tipos = append(tipos, tipo)
	}
	slices.Sort(tipos)
	for _, tipo := range tipos {
		lista := slices.Clone(m.tiempos[tipo])
		slices.Sort(lista)
		fmt.Printf("%-28s %8d %7d %9s %9s %9s\n", tipo, len(lista), m.errores[tipo],
			percentil(lista, 50).Round(time.Millisecond), percentil(lista, 95).Round(time.Millisecond), percentil(lista, 99).Round(time.Millisecond))
	}
	for tipo, ejemplo := range m.ejemplos {
		fmt.Printf("\nPrimer error de «%s»: %s\n", tipo, ejemplo)
	}
}
