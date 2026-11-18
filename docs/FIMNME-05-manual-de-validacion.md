# FIMNME-05 · Manual de Validación

**Plataforma de Postulaciones del Foro Internacional de Música Nueva Manuel Enríquez
(CNMO / INBAL)**

Este manual dice **cómo se comprueba** que el sistema cumple su especificación
(`docs/FIMNME-01-…`) y **cómo se firma la entrega**. Reúne:

- los casos de prueba, cada uno con su requisito, su procedimiento y su criterio de
  aceptación;
- los resultados de la verificación previa a la entrega;
- lo que todavía condiciona la firma;
- el acta.

Dos casos son **condición de la firma** por mandato de la especificación (§8.4 y D-30):
**V-JUR-01, aislamiento entre jurados,** y **V-CAR-01, carga de escritura concurrente**.
Si alguno falla, no se firma.

---

## 1. Entorno de validación

- **Versión:** un commit concreto del repositorio, anotado en el acta.
- **Lugar:** un servidor de **pruebas** con Debian 13 instalado según el Manual de
  Adopción (FIMNME-04 §3), nunca el de producción. Las pruebas de carga y de
  accesibilidad crean cuentas y borradores.
- **Herramientas:**
  - Go 1.26 y `make` en la computadora de quien valida;
  - Node.js sólo para V-ACC-01, con Playwright y axe-core instalados **fuera** del
    repositorio;
  - dos navegadores;
  - un lector de pantalla (NVDA en Windows u Orca en Linux).
- **Datos:** `fimnme-cli sembrar-demo` crea la edición de demostración, y con ella se
  recorren los casos manuales.
- **Cuentas:**
  - un administrador;
  - un observador;
  - un revisor por convocatoria;
  - **tres jurados** por convocatoria;
  - al menos dos postulantes, uno solista y uno de ensamble, con un tercero en
    Compositores.

## 2. Cómo se lee cada caso

| Columna | Significado |
|---|---|
| **Id** | `V-ÁREA-NN`. Se cita en el acta. |
| **Requisito** | RF, RNF o RN de la especificación. |
| **Tipo** | **A**: prueba automática; nombre de la prueba o de la orden. **M**: procedimiento manual. **A+M**: ambas. |
| **Aceptación** | Qué debe observarse para darlo por bueno. |

Las pruebas automáticas corren todas con:

```bash
make verificar
```

Revisa formato, `go vet` y las **230 pruebas** de la versión entregada. Para correr una
sola: `go test ./controllers -run NombreDeLaPrueba -v`.

---

## 3. Casos de prueba

### 3.1 Entrega y mantenibilidad

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-ENT-01 | RNF-13 | A | `make verificar` | Sin diferencias de formato, sin avisos de `go vet`, todas las pruebas en verde. |
| V-ENT-02 | RNF-13 | A | `make test`: imprime la cobertura total. Para la capa de dominio: `go test -count=1 -coverpkg=./models -coverprofile=c.out ./... && go tool cover -func=c.out \| tail -1` (con `-count=1`: un resultado en caché trae un perfil de una versión anterior y la cifra sale falsa) | Cobertura de `models/` **≥ 70 %**, medida con todas las pruebas que la ejercen. |
| V-ENT-03 | RNF-06 | A+M | `make build`; `file bin/*` | Dos ELF estáticos (`statically linked`) que arrancan en Debian 13 sin instalar nada. |
| V-ENT-04 | RNF-19 | A | `make build`; `sha256sum bin/*`; `go clean -cache`; `make build`; `sha256sum bin/*` | Sumas idénticas en las dos compilaciones. |
| V-ENT-05 | RNF-14 | M | Revisar que cada archivo `.go` empiece con el aviso de licencia GNU GPL v3.0 y que exista `LICENSE` | Ningún archivo sin aviso. |
| V-ENT-06 | Perfil §3.A | M | `go.mod` | Sólo `modernc.org/sqlite` y `golang.org/x/crypto` como dependencias directas; lo demás, indirecto de ellas. |
| V-ENT-07 | CLAUDE.md, §API | A | `TestOpenAPICoincideConLasRutas` | Cada operación de la API está en `views/api/openapi.json`, y al revés. |
| V-ENT-08 | RNF-07 | A | `TestTraduccionesConLasMismasClaves`, `TestClavesDeLasPlantillasExisten`, `TestClavesDinamicasExisten`, `TestClavesDeLosControladoresExisten` | Las mismas claves en `es.json` y `en.json`; ninguna clave usada falta. |
| V-ENT-09 | RNF-14b | M | Un integrante del equipo receptor que no participó compila, despliega en un servidor de pruebas y abre una convocatoria **sólo con el FIMNME-04** | Lo logra sin ayuda del equipo entregante. Lo que haya faltado se anota y se corrige en el manual. |
| V-ENT-10 | RNF-13, perfil §2.B | A | `TestDiccionarioCubreElEsquema`, `TestDiccionarioCubreLaAPI` | `docs/DICCIONARIO_DATOS_Y_ACL.md` describe cada tabla y columna del esquema vigente y cada operación de la API con quién puede ejecutarla. |

### 3.2 Aislamiento entre jurados — **condición de firma**

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| **V-JUR-01** | RN-15, §8.4 | A | `go test ./controllers -run TestAislamientoEntreJurados -v` | Pasa. Cada jurado deja una marca única en sus observaciones; la prueba recorre **todas** las operaciones de la API y las páginas con la sesión de cada jurado, probando cada identificador, y falla si la marca de otro aparece en cualquier respuesta. |
| **V-JUR-02** | §8.4 (1–3) | A | `go test ./models -run TestAislamientoEstructuralDelRepositorio -v` | Pasa. Analiza `repository.go`: toda consulta de `JuradoRepo` sobre las tablas de evaluación lleva `jurado_id = ?`; fuera de `JuradoRepo` y `SupervisionRepo` no hay SQL sobre ellas; `JuradoRepo` no usa `v_resultado`. |
| V-JUR-03 | §8.4 | M | Con el jurado 1 en sesión: abrir la ficha de evaluación de una postulación; cambiar en la barra de direcciones el identificador por el de otra convocatoria y por el de la evaluación de otro jurado; pedir el expediente de la convocatoria; buscar en `/archivo` | Nunca aparece un puntaje, observación ni suma de otro jurado. Lo ajeno responde «no encontrado». |
| V-JUR-04 | §2.2, C | A+M | `TestExpedientePorPerfil`, `TestRecorteDelExpediente`; a mano, comparar el expediente PDF pedido por un jurado con el del observador | El del jurado sólo trae su propia evaluación; el del observador, las tres y la suma. |
| V-JUR-05 | §8.4 (4) | M | Con el observador, abrir la matriz de resultados; luego, en *Administración › Bitácora*, filtrar por la acción `evaluacion.consultar` | La consulta que cruza jurados queda registrada con quién y cuándo. Guardar, enviar y excusarse de cada jurado también quedan (`evaluacion.*`). |
| V-JUR-06 | Perfil §1.A | M | Tras cerrar y archivar la convocatoria, repetir V-JUR-03 | El resultado es el mismo en el histórico. |

### 3.3 Carga de escritura concurrente — **condición de firma**

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| **V-CAR-01** | RNF-01, D-30 | A | En el servidor de pruebas, con una convocatoria de Intérpretes en postulación (README, «Prueba de carga»; ADR 0028): `go run ./deploy/carga --datos /ruta/datos-de-prueba --url http://127.0.0.1:8080 --convocatoria ID` | **0 errores**; p95 de las lecturas **< 500 ms**; 200 postulantes y 50 subidas simultáneas completados. |
| V-CAR-02 | RNF-02 | A | `TestEscriturasConcurrentesSinErrores`, `TestEnlaceDeUnSoloUsoBajoConcurrencia` | Pasan. Ninguna escritura concurrente se pierde ni falla por bloqueo. |
| V-CAR-03 | RNF-02 | M | Capturar un campo y esperar «Guardado a las …»; cerrar el navegador; volver a entrar | El dato está. |

### 3.4 Postulación

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-POS-01 | RF-AUT | A+M | `TestRegistroVerificacionYSesion`; a mano: registrarse, confirmar el correo, salir y entrar, recuperar la contraseña | Todo el ciclo funciona. El consentimiento del aviso queda con su versión y su sello de tiempo. |
| V-POS-02 | §1.A | A+M | `TestReglasDelEnvio`; a mano, en Intérpretes, cambiar la entidad del concierto entre CDMX y otra | Fuera de CDMX se exigen sede, domicilio, fecha y hora; en CDMX no se piden. |
| V-POS-03 | §1.A | A+M | `TestPostulacionDeSolista`; a mano, ensamble con varios integrantes (nombre e instrumento) | Lista nominal de longitud libre. |
| V-POS-04 | §1.A | A+M | `TestDocumentosPorDotacion`, `TestRequisitosAplicablesPorDotacion`; a mano, en Compositores, cambiar la dotación de una obra | Los documentos obligatorios cambian según la dotación, como se configuraron. Más de 2 obras no se admite. |
| V-POS-05 | §1.A | M | Capturar 1 a 3 ligas de video: una válida, una inexistente | La inexistente bloquea el envío con su motivo; la válida no. |
| V-POS-06 | §1.B | A+M | `TestEnvioYAcuse`, `TestFolioYConfirmacion`, `TestAcusePDF`; a mano, enviar | El envío es irreversible: asigna folio (`FIM49-INT-0001`), congela la postulación y emite el acuse en PDF, también por correo. |
| V-POS-07 | §1.B | A | `TestEnvioExigeCorreoVerificado` | Sin el correo confirmado no se envía. |
| V-POS-08 | RF-DOC | A | `TestDetectarMIMEPorFirma`, `TestSubidaYDescargaDeDocumentos` | El tipo de archivo se reconoce por su contenido, no por su nombre; los límites de tamaño se aplican. |
| V-POS-09 | RNF-09 | M | Recorrer el formulario completo en un teléfono | Se puede postular de principio a fin sin desplazamiento horizontal. |

### 3.5 Revisión documental

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-REV-01 | §1.B.2 | A+M | `TestRevisionSubsanacionYDictamen`, `TestReglasDeLaSubsanacion`; a mano, señalar un documento y un campo y pedir subsanación | El postulante puede corregir **exactamente** lo señalado y nada más. |
| V-REV-02 | §1.B.2 | A | `TestSolicitudDeDesbloqueo`, `TestSubsanacionDesdeLaPagina` | Pedir desbloqueo de algo no señalado exige justificación; el revisor concede o niega. |
| V-REV-03 | §1.B.2 | A | `TestDiferenciasDeEnvio` | Tras el reenvío, el revisor ve qué cambió. |
| V-REV-04 | §1.B.3 | M | Pasar la convocatoria a evaluación y revisar el buzón del postulante | Después de la revisión documental el sistema no le escribe. |

### 3.6 Evaluación, etapas y cuotas

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-EVA-01 | §1.A | A | `TestEvaluacionPorJurado`, `TestFichaPorObraYExcusa` | En Intérpretes se califica la postulación; en Compositores, cada obra. Dos criterios obligatorios, escala entera 1–5. |
| V-EVA-02 | RN-14 | A | `TestCuentasInternasAsignacionesYCuotas`, `TestCondicionesDeCadaTransicion` | Sin revisor no se pasa a revisión documental; sin exactamente tres jurados no se pasa a evaluación. |
| V-EVA-03 | §1.B | A | `TestAvanceDeEtapaPorCalendario` | Las etapas avanzan solas por fecha, con las mismas reglas. |
| V-EVA-04 | §1.A | M | Con el observador: abrir la matriz de resultados | Ve la suma de los tres y cada calificación. |
| V-EVA-05 | Perfil §2.B | M | Con el administrador: intentar capturar una evaluación | No puede calificar en nombre de un jurado. |

### 3.7 Archivo, búsqueda, descargas y expediente

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-ARC-01 | §1.C | A+M | `TestBusquedaDeTextoCompleto`, `TestArchivoBusquedaYFiltros`; a mano, buscar un nombre sin acentos y con una errata | Lo encuentra en ambos casos. |
| V-ARC-02 | §1.C | M | Combinar filtros, copiar la URL y abrirla en otra sesión; guardar la vista; exportar CSV | Los filtros viajan en la URL; la vista guardada y el CSV coinciden con la pantalla. |
| V-ARC-03 | §1.C | A+M | `TestDescargaMasiva`, `TestNomenclaturaDelZIP`; a mano, ZIP de una convocatoria eligiendo tipos de documento | Carpetas por folio, nombres predecibles, `manifiesto.csv`; la descarga queda registrada con usuario, IP y momento. |
| V-ARC-04 | RNF-17 | A | `TestEnlacePermanenteParaElPersonal` | `/postulaciones/{folio}` resuelve para cada perfil según su permiso. |
| V-ARC-05 | §1.C | A | `TestExpedientePDF`, `TestExpedientePorConvocatoria` | Expediente individual y por convocatoria, con índice; su contenido se recorta según quien lo pide. |
| V-ARC-06 | §1.C | A | `TestVisibilidadDeConvocatoria` | Jurados y revisores sólo ven las convocatorias en que participaron, también archivadas. |

### 3.8 Portal público

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-PUB-01 | Perfil §3.E | A | `TestListaBlancaDelPortal`, `TestPortalPublico` | El JSON sólo contiene lo de la lista blanca; nunca contacto, fechas de nacimiento, documentos, dictámenes, calificaciones ni borradores. |
| V-PUB-02 | Perfil §3.E | A+M | `TestReglasDeNominaYAnonimizacion`; a mano, cambiar de etapa hasta CERRADA y revisar el JSON | La nómina no sale al cambiar de etapa: sólo con la acción explícita del administrador, después del cierre. |

### 3.9 Protección de datos y bitácora

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-DAT-01 | RNF-11 | A+M | `TestAnonimizacion`, `TestAnonimizacionEnPantalla`; a mano, anonimizar una postulación cerrada | Doble confirmación y fundamento; desaparecen los datos de contacto; obra, dictamen y estadísticas no cambian. |
| V-DAT-02 | §3.B | A | `TestAuditoriaEsDeSoloInsercion` | La bitácora no admite modificación ni borrado, tampoco desde SQL. |
| V-DAT-03 | §3.B | A | `TestCercaDeLaExtensionDinamica`, `TestNadaImportaPostulaciones` | El DDL dinámico no toca las tablas vedadas; ninguna importación crea postulaciones. |
| V-DAT-04 | RNF-11 | M | Revisar el aviso de privacidad publicado | Es el **texto oficial** de la CNMO, con su versión (§5). |

### 3.10 Seguridad

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-SEG-01 | RNF-10 | M | `curl -sI https://dominio/` | `Strict-Transport-Security`, `Content-Security-Policy` sin `unsafe-inline`; HTTP redirige a HTTPS. |
| V-SEG-02 | RNF-10 | A | `TestPostDeOtroOrigenRechazado` | Un POST de otro origen se rechaza (CSRF). |
| V-SEG-03 | RNF-10 | A | `TestIngresoYBloqueoPorIntentos`, `TestLimiteDeIntentosPorIP`, `TestBloqueoDeIngreso` | Bloqueo por cuenta y límite por IP. |
| V-SEG-04 | RNF-10 | A | `TestResumenDePasswordArgon2id` | Contraseñas con Argon2id. |
| V-SEG-06 | RF-AUT-09 | A+M | `TestCodigosTOTP` (vectores del RFC 6238), `TestSegundoFactor`, `TestRestablecerSegundoFactor`, `TestCorreccionReedSolomon`, `TestSimboloConocido`. A mano: crear un administrador, ingresar, escanear el QR con una aplicación real, entrar; salir y volver a entrar; entrar con un código de recuperación; restablecer con `fimnme-cli` | Sin el código no se alcanza ninguna página interna ni la API; el QR se lee; un código no vale dos veces; los fallos bloquean; tras restablecer se pide alta nueva. Postulantes, revisores y jurados no tienen segundo paso. |
| V-SEG-05 | RNF-10 | M | `git grep -n -i -E "password=\|secret\|BEGIN .*PRIVATE"` y revisar los permisos de `/etc/fimnme` | Ningún secreto en el repositorio; `fimnme-web.env` y las claves con permisos 0640/0600. |

### 3.11 Accesibilidad e idiomas

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-ACC-01 | RNF-08 | A | `deploy/accesibilidad/auditar.js` (instrucciones en su encabezado) contra el servidor de pruebas, con una convocatoria de cada tipo en postulación | **0 violaciones** WCAG 2.1 A y AA en las páginas públicas, el registro, «Mi cuenta», «Mis postulaciones» y cada paso del formulario, en español y escritorio, inglés y móvil, y tema oscuro. |
| V-ACC-02 | RNF-08 | M | Recorrer el flujo de postulación **sólo con teclado** | Todo se alcanza y se acciona; el foco siempre es visible; el orden es lógico. |
| V-ACC-03 | RNF-08 | M | Recorrer el flujo con un lector de pantalla (NVDA u Orca) | Cada campo anuncia su etiqueta y su error; el avance del formulario y «Guardado a las …» se anuncian. |
| V-ACC-04 | RNF-07 | M | Cambiar a inglés en cada pantalla del postulante y en un correo | Nada queda en español; el acuse sale en el idioma de la cuenta. |

Una auditoría automática encuentra sólo una parte de los problemas de accesibilidad:
V-ACC-02 y V-ACC-03 no se sustituyen con V-ACC-01.

### 3.12 Operación

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-OPE-01 | RNF-04 | A+M | `TestEspejo`; en el servidor, `systemctl start fimnme-respaldo` y después `fimnme-restauracion` | Instantánea firmada, `SHA256SUMS` y restauración verificada sin errores. |
| V-OPE-02 | RNF-04 | M | Restaurar a mano en otra máquina con el procedimiento del README | El sistema arranca con los datos y los documentos del respaldo. Se anota la duración. |
| V-OPE-03 | §3.D | M | `fimnme-cli verify` | Todo archivo registrado existe y coincide con su SHA-256. |
| V-OPE-04 | RNF-12 | M | `curl https://dominio/healthz` y `/readyz`; `journalctl -u fimnme-web -o cat \| head` | Responden 200; los registros son JSON. |
| V-OPE-05 | §3.D | A | `TestAlertaDeDisco` | Por debajo del umbral se avisa a los administradores. |
| V-OPE-06 | RNF-05 | M | `make imagen`; `podman compose -f deploy/contenedor/compose.yaml up` | La imagen se construye y el servicio responde. |
| V-OPE-07 | RNF-18, §3.C | M | `verify_and_migrate_mariadb.sh` contra un MariaDB de pruebas (ADR 0029) | Todas las tablas idénticas y los cuatro triggers presentes. |

### 3.13 Correo

| Id | Requisito | Tipo | Procedimiento | Aceptación |
|---|---|---|---|---|
| V-COR-01 | §5.9 | A | `TestMensajeMIMEYTransporteAArchivo`, `TestEntregaPorSMTP`, `TestTrabajadorDeCorreo` | Mensaje MIME correcto; los errores se reintentan. |
| V-COR-02 | Perfil §4 | M | `comprobar_correo.sh` en el servidor final (ADR 0030) | Ninguna falla. |
| V-COR-03 | Perfil §4 | M | «Enviar prueba» a una cuenta de Gmail y otra de Outlook | Llegan a la bandeja de entrada con `spf=pass`, `dkim=pass` y `dmarc=pass`. |

### 3.14 Recorrido completo

| Id | Tipo | Procedimiento | Aceptación |
|---|---|---|---|
| V-E2E-01 | M | Con la edición de demostración, una convocatoria de cada tipo de BORRADOR a ARCHIVADA, siguiendo el FIMNME-04 §5: configurar, publicar, postular (3 postulantes), revisar con una subsanación, evaluar con 3 jurados, cerrar, publicar la nómina y archivar | Cada etapa se completa sin intervención del equipo de desarrollo; la matriz coincide con las calificaciones capturadas a mano. |

---

## 4. Resultados de la verificación previa a la entrega

Hechos por el equipo de desarrollo el **16 de noviembre de 2026**, sobre la versión de
este manual. Los casos que exigen el servidor final quedan pendientes (§5).

| Caso | Resultado |
|---|---|
| V-ENT-01 | `make verificar` en verde: **230 pruebas, 0 fallas** (versión 1.0.0-beta.1). |
| V-ENT-02 | Cobertura de `models/` con todas las pruebas: **82.8 %** (mínimo 70 %). |
| V-ENT-03, 04 | ELF estáticos; **dos compilaciones con la caché vacía dieron sumas idénticas**. |
| V-ENT-05 | Los 118 archivos `.go` con su aviso de licencia. |
| V-ENT-06 | Dos dependencias directas, las previstas. |
| **V-JUR-01, 02** | **Pasan.** |
| **V-CAR-01** | **Pasa** (ADR 0028): 12 622 peticiones, **0 errores**; p95 de lecturas de **4 a 12 ms**; subida de 5 MB con 50 simultáneas, p95 608 ms; 320 MB de memoria. |
| V-SEG-06 | Pasa. El QR de la página real, fotografiado en el navegador, lo lee zbar; el codificador se probó en sus diez versiones y ocho máscaras (ADR 0032). |
| V-ACC-01 | Tras los ajustes del ADR 0032 (fechas en texto, botón de tema), 48 páginas con una convocatoria abierta y las páginas del segundo paso en claro y en oscuro: **0 violaciones**. Antes, **63 páginas, 0 violaciones**. La primera corrida halló una: el estado del paso actual del formulario tenía un contraste de 4.48:1 (mínimo 4.5:1), en todas las páginas del formulario. Se corrigió en `fimnme.css` y se repitió. |
| V-OPE-01 | Pasa en local (ADR 0026), con restauración verificada. |
| V-OPE-06 | La imagen se construye (ADR 0027). |
| V-OPE-07 | Pasa contra MariaDB 11.8: 44 tablas idénticas y 4 triggers (ADR 0029). |
| V-COR-01 | Pasa. Postfix y OpenDKIM probados en local: firma verificada, nada sale sin firma (ADR 0030). |
| Despliegue | El sistema corre en un VPS de pruebas con nginx y TLS, con correo en modo `archivo` porque ese proveedor bloquea el puerto 25. |

## 5. Lo que condiciona la firma

**Bloquean la firma** (son de la CNMO, no del código):

1. **Texto oficial del aviso de privacidad** (RNF-11, V-DAT-04). Hoy la versión
   registrada en cada consentimiento es `provisional`. Al recibir el texto, se publica
   y se cambia `VersionAvisoPrivacidad` en `models/rules.go`, para volver a pedir el
   consentimiento.
2. **Bases oficiales** de las dos convocatorias de la edición de arranque, en español e
   inglés.
3. **Servidor final con correo propio**: V-COR-02 y V-COR-03 sólo pueden hacerse ahí.
4. **Puntos por confirmar con la CNMO** (`docs/PLAN_DE_DESARROLLO.md`). Cada decisión
   tomada en ausencia de una respuesta está anotada con la regla que se aplicó.
   Confirmarlos o pedir su cambio antes de firmar.

**No bloquean, pero se anotan en el acta:**

- **requisitos de prioridad S («should») y C («could») no implementados.** Todos los de
  prioridad M están hechos. Faltan:

  | Requisito | Prioridad | Qué falta |
  |---|---|---|
  | RF-CONV-11 | S | Vista previa de la convocatoria, tal como la verá el público, antes de publicarla. |
  | RF-DOC-05 | S | Antivirus (ClamAV) sobre cada archivo subido. La columna `escaneo_av` queda en `pendiente`. |
  | RF-NOT-09, 10 | S | Correos a los jurados: inicio de la evaluación y recordatorio de pendientes. |
  | RF-SMTP-05 | S | Recepción y procesamiento de los informes DMARC: hoy llegan al buzón indicado en el registro DMARC. |
  | RF-ARCH-36 | S | División de un ZIP muy grande en volúmenes. |
  | RF-ARCH-44 | S | Anexar las partituras al expediente PDF; se descargan en el ZIP. |
  | RF-REP-06 | S | Reportes en segundo plano: con el volumen previsto se generan en menos de un segundo. |
  | RF-REP-07, RF-DASH-04, RF-ARCH-05 | C | Exportación a XLSX y comparativas entre ediciones. |


- la **identidad visual del 49º Foro**: hoy se usa la del 48º, y cambiarla es editar
  un archivo;
- la **conciliación de postulantes recurrentes entre ediciones** (RF-IMP-06), que no se
  hizo: espera una definición de qué significa fundir dos cuentas;
- que la **aplicación sobre MariaDB** es un paso posterior: hoy se migran y verifican
  los datos.
- **la bitácora registra la lectura de evaluaciones que cruza jurados**: la matriz y
  los agregados que consultan administrador y observador. No registra la lectura que
  cada jurado hace de **su propia** evaluación. §8.4 (4) pide registrar «toda
  lectura», para detectar después una fuga. La única lectura capaz de revelar lo ajeno
  es la que se registra, y la de un jurado está acotada a lo suyo por construcción
  (V-JUR-02). Si la CNMO quiere también ésta, es un cambio pequeño en `JuradoRepo`.

## 6. Protocolo de firma de entrega

1. **Preparación.** El equipo entregante:
   - etiqueta el commit (`git tag -s entrega-AAAA-MM-DD`);
   - entrega las sumas SHA-256 de los binarios de `make build`;
   - instala esa versión en el servidor de pruebas.
2. **Ejecución.** El equipo receptor:
   - corre **todos** los casos del §3, o una muestra acordada de los manuales;
   - incluye **siempre** V-JUR-01 a 03, V-CAR-01, V-ACC-01 a 03, V-OPE-02, V-ENT-09 y
     V-E2E-01;
   - compila por su cuenta y compara las sumas con las entregadas (V-ENT-04).
3. **Registro.** Cada caso se anota en el acta como *aceptado*, *rechazado* (con lo
   observado) o *no aplica* (con el motivo).
4. **Rechazos.** Un caso rechazado se corrige en un commit nuevo y se repite, junto con
   `make verificar`. Si el cambio toca el aislamiento o la persistencia, también se
   repiten V-JUR-01 y V-CAR-01.
5. **Firma.**
   - Se firma cuando todos los casos están aceptados, o rechazados sólo con
     observaciones que ambas partes declaran no bloqueantes, y el §5 está resuelto.
   - **V-JUR-01 y V-CAR-01 no admiten esa excepción.**
   - Firman el responsable técnico receptor, la Coordinación del Foro y el equipo
     entregante.
6. **Después de firmar.**
   - El acta, con las sumas y el commit, se guarda junto a los manuales.
   - El FIMNME-04 §7 rige la custodia de los secretos.

---

## Anexo A · Acta de validación y entrega

```
ACTA DE VALIDACIÓN Y ENTREGA — PLATAFORMA DE POSTULACIONES FIMNME

Fecha y lugar: ______________________________________________

Versión
  Commit:            ________________________________________
  Etiqueta:          ________________________________________
  SHA-256 fimnme-web: _______________________________________
  SHA-256 fimnme-cli: _______________________________________
  Compilación reproducida por el receptor:  [ ] sí  [ ] no

Servidor de validación: ______________________________________

Resultados (A = aceptado, R = rechazado, N/A = no aplica)
  Id          Resultado   Observaciones
  V-ENT-01    ____        _________________________________
  ...         (un renglón por caso del §3)
  V-JUR-01    ____        (condición de firma)
  V-CAR-01    ____        (condición de firma)
  ...

Condiciones del §5
  Aviso de privacidad oficial publicado:            [ ] sí
  Bases oficiales cargadas:                         [ ] sí
  Correo del servidor final (V-COR-02, 03):         [ ] sí
  Puntos por confirmar revisados:                   [ ] sí

Observaciones no bloqueantes:
  ____________________________________________________________

Secretos entregados según FIMNME-04 §7:              [ ] sí

Firmas
  Responsable técnico receptor:   ______________________________
  Coordinación del Foro:          ______________________________
  Equipo entregante:              ______________________________
```
