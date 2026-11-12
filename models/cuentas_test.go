// Este archivo es parte de la Plataforma de Postulaciones FIMNME (CNMO / INBAL).
// Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
// SPDX-License-Identifier: GPL-3.0-only

package models

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Seguridad -----------------------------------------------------------------

func TestResumenDePasswordArgon2id(t *testing.T) {
	resumen, err := HashPassword("una frase con eñe y acentó")
	if err != nil {
		t.Fatal(err)
	}
	// Formato PHC con los parámetros dentro: podrán subirse sin invalidar lo guardado.
	if !strings.HasPrefix(resumen, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("formato inesperado: %s", resumen)
	}
	if !VerificarPassword(resumen, "una frase con eñe y acentó") {
		t.Error("la contraseña correcta no verificó")
	}
	if VerificarPassword(resumen, "una frase con eñe y acento") {
		t.Error("verificó una contraseña distinta")
	}
	// Cada resumen lleva su propia sal: dos de la misma contraseña difieren.
	otro, _ := HashPassword("una frase con eñe y acentó")
	if otro == resumen {
		t.Error("dos resúmenes de la misma contraseña no deben coincidir")
	}
	// Nada malformado verifica, y un resumen manipulado no puede pedir recursos desmedidos.
	for _, malo := range []string{"", PasswordSinDefinir, "texto-plano", "$argon2i$v=19$m=19456,t=2,p=1$c2Fs$Y2xhdmU",
		"$argon2id$v=19$m=99999999,t=2,p=1$c2Fs$Y2xhdmU", "$argon2id$v=19$m=19456,t=2,p=1$%%%$Y2xhdmU"} {
		if VerificarPassword(malo, "una frase con eñe y acentó") || VerificarPassword(malo, malo) {
			t.Errorf("un resumen malformado verificó: %q", malo)
		}
	}
	GastarVerificacion("cualquiera") // no debe fallar ni bloquearse
}

func TestSecretosDeUnSoloUso(t *testing.T) {
	claro, resumen, err := NuevoSecreto()
	if err != nil {
		t.Fatal(err)
	}
	otro, _, _ := NuevoSecreto()
	if len(claro) != 43 || claro == otro { // 256 bits en base64 para URL
		t.Errorf("secreto inesperado: %q / %q", claro, otro)
	}
	if resumen != ResumenDeSecreto(claro) || len(resumen) != 64 || strings.Contains(resumen, claro) {
		t.Errorf("el resumen debe ser el SHA-256 del secreto: %q", resumen)
	}
}

// --- Reglas ---------------------------------------------------------------------

func TestBloqueoDeIngreso(t *testing.T) {
	ultimo := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		fallos int
		espera time.Duration // 0 = sin bloqueo
	}{
		{0, 0}, {4, 0}, {5, time.Minute}, {6, 2 * time.Minute}, {7, 4 * time.Minute},
		{9, 16 * time.Minute}, {10, 30 * time.Minute}, {50, 30 * time.Minute}, {5000, 30 * time.Minute},
	}
	for _, c := range casos {
		hasta := BloqueoDeIngreso(c.fallos, ultimo)
		if c.espera == 0 {
			if !hasta.IsZero() {
				t.Errorf("%d fallos no deben bloquear", c.fallos)
			}
			continue
		}
		if got := hasta.Sub(ultimo); got != c.espera {
			t.Errorf("%d fallos: bloqueo de %v, se esperaba %v", c.fallos, got, c.espera)
		}
	}
}

func TestDuracionDeSesionSegunPerfil(t *testing.T) {
	jurado := []Asignacion{{ConvocatoriaID: 1, Perfil: PerfilJurado}}
	casos := []struct {
		nombre       string
		perfiles     []Perfil
		asignaciones []Asignacion
		duracion     time.Duration
	}{
		{"postulante", []Perfil{PerfilPostulante}, nil, SesionPostulante},
		{"administrador", []Perfil{PerfilAdminSistema}, nil, SesionInterna},
		{"observador", []Perfil{PerfilCoordinacionNacional}, nil, SesionInterna},
		{"postulante que además es jurado", []Perfil{PerfilPostulante}, jurado, SesionInterna},
	}
	for _, c := range casos {
		if got := DuracionSesion(c.perfiles, c.asignaciones); got != c.duracion {
			t.Errorf("%s: %v, se esperaba %v", c.nombre, got, c.duracion)
		}
	}
}

func TestEsperaEntreEnlacesYReintentosDeCorreo(t *testing.T) {
	ahora := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	if !PuedeEmitirseOtroToken(TokenResetPassword, time.Time{}, ahora) {
		t.Error("sin enlaces previos debe poder emitirse uno")
	}
	// Un enlace de recuperación emitido hace 2 minutos vence en 58.
	if PuedeEmitirseOtroToken(TokenResetPassword, ahora.Add(58*time.Minute), ahora) {
		t.Error("a los 2 minutos no debe emitirse otro enlace")
	}
	if !PuedeEmitirseOtroToken(TokenResetPassword, ahora.Add(54*time.Minute), ahora) {
		t.Error("a los 6 minutos sí debe poder emitirse otro enlace")
	}

	// Reintentos: 2, 4, 8… minutos, con tope de 6 horas, y abandono al octavo.
	for intentos, espera := range map[int]time.Duration{1: 2 * time.Minute, 2: 4 * time.Minute, 7: 128 * time.Minute} {
		proximo, reintentar := ReintentoDeCorreo(intentos, ahora)
		if !reintentar || proximo.Sub(ahora) != espera {
			t.Errorf("intento %d: reintento en %v (%v), se esperaba %v", intentos, proximo.Sub(ahora), reintentar, espera)
		}
	}
	if _, reintentar := ReintentoDeCorreo(IntentosMaxCorreo, ahora); reintentar {
		t.Error("agotados los intentos no debe reintentarse")
	}
	if !PlantillaConEnlaceSecreto(PlantillaResetPassword) || PlantillaConEnlaceSecreto(PlantillaCambioEmail) {
		t.Error("clasificación de plantillas con enlace secreto incorrecta")
	}
}

func TestPermisoDeAuditoria(t *testing.T) {
	if PuedeConsultarAuditoria(nil) {
		t.Error("sin sesión no hay acceso a la bitácora")
	}
	casos := map[Perfil]bool{PerfilAdminSistema: true, PerfilCoordinacionNacional: true, PerfilPostulante: false}
	for perfil, esperado := range casos {
		if got := PuedeConsultarAuditoria(&Sesion{Perfiles: []Perfil{perfil}}); got != esperado {
			t.Errorf("%s: acceso=%v, se esperaba %v", perfil, got, esperado)
		}
	}
	// Ser jurado o revisor de una convocatoria no da acceso a la bitácora.
	operativo := &Sesion{Perfiles: []Perfil{PerfilPostulante}, Asignaciones: []Asignacion{{1, PerfilJurado}, {1, PerfilRevisorDocumental}}}
	if PuedeConsultarAuditoria(operativo) {
		t.Error("un perfil operativo no debe consultar la bitácora")
	}
}

// --- Repositorio: cuentas ---------------------------------------------------------

// cuentaDePrueba crea una cuenta de postulante.
func cuentaDePrueba(t *testing.T, r *Repositorio, email string) int64 {
	t.Helper()
	id, err := r.CrearUsuario(context.Background(), actorPrueba, NuevoUsuario{
		Email: email, Nombre: "Ana", Apellidos: "López", Idioma: IdiomaES,
		PasswordHash: PasswordSinDefinir, AvisoVersion: VersionAvisoPrivacidad, Perfil: PerfilPostulante,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAltaDeUsuario(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	id := cuentaDePrueba(t, r, "ana@ejemplo.mx")

	// El correo es único sin distinguir mayúsculas.
	_, err := r.CrearUsuario(ctx, actorPrueba, NuevoUsuario{Email: "ANA@EJEMPLO.MX", Nombre: "x", Apellidos: "y",
		Idioma: IdiomaES, PasswordHash: PasswordSinDefinir, Perfil: PerfilPostulante}, nil)
	if !errors.Is(err, ErrEmailEnUso) {
		t.Fatalf("se esperaba ErrEmailEnUso, se obtuvo %v", err)
	}
	u, resumen, err := r.UsuarioPorEmail(ctx, "Ana@Ejemplo.mx")
	if err != nil || u.ID != id || u.EmailVerificado() || !u.Activo || resumen != PasswordSinDefinir {
		t.Errorf("cuenta leída incorrecta: %+v %q (%v)", u, resumen, err)
	}
	if _, _, err := r.UsuarioPorEmail(ctx, "nadie@ejemplo.mx"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	// Consentimiento con versión y sello de tiempo (LFPDPPP).
	var version, aceptado string
	if err := r.lectura.QueryRow("SELECT aviso_privacidad_version, aviso_privacidad_aceptado_at FROM usuario WHERE id = ?", id).
		Scan(&version, &aceptado); err != nil || version != VersionAvisoPrivacidad || len(aceptado) != 20 {
		t.Errorf("consentimiento mal registrado: %q %q (%v)", version, aceptado, err)
	}
	if n := contar(t, r, "SELECT COUNT(*) FROM usuario_perfil WHERE usuario_id = ? AND perfil = 'postulante'", id); n != 1 {
		t.Error("falta el perfil inicial")
	}
}

// Un enlace es de un solo uso aunque dos peticiones lo presenten a la vez.
func TestEnlaceDeUnSoloUsoBajoConcurrencia(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	id := cuentaDePrueba(t, r, "ana@ejemplo.mx")
	_, resumen, _ := NuevoSecreto()
	if err := r.EmitirToken(ctx, actorPrueba, id, NuevoToken{Resumen: resumen, Proposito: TokenVerificarEmail,
		ExpiraAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	const intentos = 12
	resultados := make(chan error, intentos)
	var grupo sync.WaitGroup
	for i := 0; i < intentos; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			_, err := r.VerificarEmailConToken(ctx, actorPrueba, resumen)
			resultados <- err
		}()
	}
	grupo.Wait()
	close(resultados)
	exitos := 0
	for err := range resultados {
		switch {
		case err == nil:
			exitos++
		case !errors.Is(err, ErrTokenInvalido):
			t.Errorf("error inesperado: %v", err)
		}
	}
	if exitos != 1 {
		t.Errorf("el enlace se consumió %d veces, se esperaba exactamente 1", exitos)
	}
	// En la base sólo está el resumen del enlace; emitir otro anula el anterior.
	_, segundo, _ := NuevoSecreto()
	_, tercero, _ := NuevoSecreto()
	for _, resumen := range []string{segundo, tercero} {
		if err := r.EmitirToken(ctx, actorPrueba, id, NuevoToken{Resumen: resumen, Proposito: TokenResetPassword,
			ExpiraAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.TokenVigente(ctx, segundo, TokenResetPassword); !errors.Is(err, ErrTokenInvalido) {
		t.Error("el enlace anterior debió quedar anulado al emitir otro")
	}
	if u, err := r.TokenVigente(ctx, tercero, TokenResetPassword); err != nil || u.ID != id {
		t.Errorf("el enlace más reciente debe valer: %v", err)
	}
}

// La sesión lleva los perfiles globales y sólo las asignaciones ACTIVAS.
func TestSesionConPerfilesYAsignaciones(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	id := cuentaDePrueba(t, r, "jurado@ejemplo.mx")
	edicion, _ := r.CrearEdicion(ctx, actorPrueba, Edicion{Numero: 49, Anio: 2027, NombreES: "a", NombreEN: "b"})
	interpretes, _ := r.CrearConvocatoria(ctx, actorPrueba, convocatoriaDePrueba(edicion, TipoInterpretes, EstadoEvaluacion))
	compositores, _ := r.CrearConvocatoria(ctx, actorPrueba, convocatoriaDePrueba(edicion, TipoCompositores, EstadoEvaluacion))
	ejecutar(t, r, "INSERT INTO asignacion (convocatoria_id, usuario_id, perfil, activo, creado_at) VALUES (?, ?, 'jurado', 1, 'x')", interpretes, id)
	ejecutar(t, r, "INSERT INTO asignacion (convocatoria_id, usuario_id, perfil, activo, creado_at) VALUES (?, ?, 'jurado', 0, 'x')", compositores, id)

	_, resumen, _ := NuevoSecreto()
	if err := r.AbrirSesion(ctx, Actor{UsuarioID: id, IP: "203.0.113.9", Agente: "navegador"}, id, resumen, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s, err := r.SesionPorID(ctx, resumen)
	if err != nil {
		t.Fatal(err)
	}
	if !s.TienePerfil(PerfilPostulante) || s.TienePerfil(PerfilAdminSistema) || s.PerfilPrincipal() != PerfilPostulante {
		t.Errorf("perfiles globales incorrectos: %v", s.Perfiles)
	}
	if !s.TieneAsignacion(interpretes, PerfilJurado) {
		t.Error("falta la asignación activa")
	}
	// Ser jurado de Intérpretes no da nada en Compositores, y una asignación retirada no cuenta.
	if s.TieneAsignacion(compositores, PerfilJurado) || s.TieneAsignacion(interpretes, PerfilRevisorDocumental) {
		t.Errorf("asignaciones de más: %v", s.Asignaciones)
	}
	if DuracionSesion(s.Perfiles, s.Asignaciones) != SesionInterna {
		t.Error("un jurado debe tener sesión de 8 horas")
	}
	// Una cuenta desactivada pierde sus sesiones de inmediato.
	ejecutar(t, r, "UPDATE usuario SET activo = 0 WHERE id = ?", id)
	if _, err := r.SesionPorID(ctx, resumen); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("la sesión de una cuenta desactivada debe dejar de valer: %v", err)
	}
	if _, err := r.SesionPorID(ctx, "inexistente"); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("se esperaba ErrNoEncontrado: %v", err)
	}
}

// Las plantillas se leen por idioma, con respaldo en español; y el cuerpo de
// un correo con enlace de un solo uso se retira de la base al enviarse.
func TestPlantillasYBandejaDeSalida(t *testing.T) {
	r := repoDePrueba(t)
	ctx := context.Background()
	for _, clave := range []string{PlantillaVerificarEmail, PlantillaResetPassword, PlantillaActivacion, PlantillaCambioEmail} {
		for _, idioma := range []string{IdiomaES, IdiomaEN} {
			p, err := r.PlantillaCorreo(ctx, clave, idioma)
			if err != nil || p.Idioma != idioma || p.Version != 1 || !strings.Contains(p.CuerpoMD, "{{nombre}}") {
				t.Errorf("plantilla %s/%s: %+v (%v)", clave, idioma, p, err)
			}
		}
	}
	// Falta la versión inglesa: se entrega la española. Una versión nueva sustituye a la anterior.
	ejecutar(t, r, "UPDATE plantilla_correo SET activa = 0 WHERE clave = 'activacion' AND idioma = 'en'")
	ejecutar(t, r, `INSERT INTO plantilla_correo (clave, idioma, version, asunto, cuerpo_md, creado_at)
		VALUES ('activacion', 'es', 2, 'Asunto nuevo', 'Cuerpo nuevo', 'x')`)
	if p, err := r.PlantillaCorreo(ctx, PlantillaActivacion, IdiomaEN); err != nil || p.Idioma != IdiomaES || p.Version != 2 || p.Asunto != "Asunto nuevo" {
		t.Errorf("respaldo o versión incorrectos: %+v (%v)", p, err)
	}
	if _, err := r.PlantillaCorreo(ctx, "inexistente", IdiomaES); !errors.Is(err, ErrNoEncontrado) {
		t.Errorf("se esperaba ErrNoEncontrado: %v", err)
	}

	id := cuentaDePrueba(t, r, "ana@ejemplo.mx")
	_, resumen, _ := NuevoSecreto()
	token := NuevoToken{Resumen: resumen, Proposito: TokenVerificarEmail, ExpiraAt: time.Now().Add(time.Hour)}
	secreto := CorreoSaliente{Destinatario: "ana@ejemplo.mx", Idioma: IdiomaES, Asunto: "a",
		HTML: "enlace-secreto", Texto: "enlace-secreto", PlantillaClave: PlantillaVerificarEmail}
	aviso := CorreoSaliente{Destinatario: "ana@ejemplo.mx", Idioma: IdiomaES, Asunto: "b",
		HTML: "sin secretos", Texto: "sin secretos", PlantillaClave: PlantillaCambioEmail}
	if err := r.EmitirToken(ctx, actorPrueba, id, token, secreto, aviso); err != nil {
		t.Fatal(err)
	}
	pendientes, err := r.CorreosPendientes(ctx, 10)
	if err != nil || len(pendientes) != 2 {
		t.Fatalf("pendientes: %d (%v)", len(pendientes), err)
	}
	for _, c := range pendientes {
		if err := r.MarcarCorreoEnviado(ctx, c, "<id@prueba>"); err != nil {
			t.Fatal(err)
		}
	}
	var cuerpos []string
	filas, _ := r.lectura.Query("SELECT cuerpo_texto || '|' || cuerpo_html || '|' || estado || '|' || message_id FROM email_outbox ORDER BY id")
	defer filas.Close()
	for filas.Next() {
		var c string
		filas.Scan(&c)
		cuerpos = append(cuerpos, c)
	}
	if len(cuerpos) != 2 || strings.Contains(cuerpos[0], "enlace-secreto") || !strings.HasSuffix(cuerpos[0], "|enviado|<id@prueba>") {
		t.Errorf("el cuerpo con enlace de un solo uso debió retirarse: %v", cuerpos)
	}
	if !strings.HasPrefix(cuerpos[1], "sin secretos|sin secretos|enviado") {
		t.Errorf("un correo sin enlace conserva su cuerpo: %v", cuerpos)
	}
	// En la tabla de enlaces sólo hay resúmenes: 64 caracteres hexadecimales.
	if n := contar(t, r, "SELECT COUNT(*) FROM token WHERE LENGTH(hash) <> 64"); n != 0 {
		t.Error("la tabla token debe guardar sólo resúmenes SHA-256")
	}
}
