# FIMNME-06 · Formato de intercambio y portal público

**Plataforma de Postulaciones FIMNME (CNMO / INBAL)** · Versión 1.0.0-beta.1

El portal público del Foro es un sitio estático (Hugo) que **no se conecta** a la
plataforma de postulaciones. Se alimenta de un JSON **sanitizado** que la plataforma
genera (perfil de dominio, §3.E). Así, la consulta ciudadana nunca toca la base de
postulaciones, sus datos de contacto, documentos ni calificaciones.

Decisiones: ADR 0014.

## 1. Cómo se obtiene

Hay dos vías, con el mismo contenido.

**Por HTTP, sin sesión.** El generador del sitio lo descarga al construir:

```
GET https://dominio/api/v1/interop/convocatoria.json
GET https://dominio/api/v1/interop/convocatoria.json?edicion=49
```

Se puede guardar en caché 5 minutos, no más: así una nómina retirada deja de servirse
pronto.

**Desde el servidor:**

```bash
sudo runuser -u fimnme -- /opt/fimnme/bin/fimnme-cli --datos /var/lib/fimnme export-json --edicion 49 --salida convocatoria.json
```

La orden imprime la SHA-256 del archivo. Sin `--edicion` exporta todas las ediciones.

## 2. Qué contiene, y qué nunca

El contenido lo definen los tipos `Interop*` de `models/entities.go`, que son la **lista
blanca hecha estructura**: no tienen dónde llevar nada más. Ampliar lo que se publica
exige añadir un campo ahí, y debe pensarse como tal.

| Sale | Nunca sale |
|---|---|
| Edición: número, año y nombre. | Datos de contacto (correos, teléfonos, domicilios). |
| Convocatoria: tipo, estado, títulos y bases en ambos idiomas. | Fechas de nacimiento, género y nacionalidad. |
| Calendario de sus tres etapas. | Documentos, sus rutas o sus sumas. |
| Catálogo de dotaciones activas. | Dictámenes de revisión, observaciones y motivos. |
| **Tras el cierre y por acción explícita del administrador:** la nómina de seleccionados (nombre con que se anuncian, subcategoría, semblanza y títulos de obra). | Calificaciones, evaluaciones y resultados numéricos. |
| | Postulaciones no seleccionadas, borradores y cuentas. |

**Qué convocatorias aparecen:**

- Las visibles al público: publicadas en adelante, nunca un borrador.
- La nómina sólo acompaña a una convocatoria **cerrada o archivada**. Si se reabriera,
  deja de salir.

**Pruebas que lo vigilan:**

- `TestListaBlancaDelPortal`: fija la lista exacta de campos. Falla si alguien añade,
  quita o renombra uno.
- `TestPortalPublico`.
- `TestReglasDeNominaYAnonimizacion`.

## 3. Esquema

```json
{
  "generado_at": "2027-04-20T18:00:00Z",
  "ediciones": [
    {
      "numero": 49,
      "anio": 2027,
      "nombre_es": "Edición 49 · 2027",
      "nombre_en": "49th Edition · 2027",
      "convocatorias": [
        {
          "tipo": "compositores",
          "estado": "cerrada",
          "titulo_es": "Convocatoria de Compositores",
          "titulo_en": "Call for Composers",
          "bases_md_es": "…Markdown…",
          "bases_md_en": "…Markdown…",
          "etapas": [
            {"clave": "postulacion", "inicia_at": "2026-11-23T06:00:00Z", "termina_at": "2027-02-13T05:59:00Z"},
            {"clave": "revision_documental", "inicia_at": "…", "termina_at": "…"},
            {"clave": "evaluacion", "inicia_at": "…", "termina_at": "…"}
          ],
          "dotaciones": [
            {"nombre_es": "Cuarteto de cuerdas", "nombre_en": "String quartet", "descripcion_es": "…", "descripcion_en": "…"}
          ],
          "nomina": {
            "publicada_at": "2027-04-18T17:30:00Z",
            "seleccionados": [
              {"nombre": "Nombre artístico", "semblanza": "…", "obras": [{"titulo": "Título de la obra"}]}
            ]
          }
        }
      ]
    }
  ]
}
```

| Campo | Tipo | Notas |
|---|---|---|
| `generado_at` | texto ISO-8601 UTC | Cuándo se generó. |
| `ediciones[]` | lista | En orden de número. |
| `…convocatorias[].tipo` | texto | `interpretes` o `compositores`. |
| `…convocatorias[].estado` | texto | `publicada`, `postulacion`, `revision_documental`, `evaluacion`, `cerrada` o `archivada`. |
| `…bases_md_es` / `_en` | texto | Markdown; el inglés puede venir vacío (el portal muestra el español). |
| `…etapas[]` | lista | Instantes en UTC; el portal los muestra en hora de la Ciudad de México. |
| `…dotaciones[]` | lista | Vacía en Intérpretes. |
| `…nomina` | objeto o `null` | `null` mientras no se publique, o si se retiró. |
| `…seleccionados[].nombre` | texto | Nombre artístico; si no tiene, el nombre completo o el del ensamble. |
| `…seleccionados[].subcategoria` | texto, opcional | `solista` o `ensamble`; sólo en Intérpretes. |
| `…seleccionados[].obras[].titulo` | texto | Títulos tal como se capturaron. |

Los PDF de las bases y la imagen de cabecera no van en el JSON: son públicos en la
propia plataforma, en `/convocatorias/{edición}/{tipo}/bases.pdf` y `…/imagen`.

## 4. Publicación de la nómina

1. **Cerrar** la convocatoria. La evaluación termina sola en su fecha, o el
   administrador la cierra.
2. Registrar el **dictamen final** de cada postulación en *Supervisión › Resultados*.
3. En *Supervisión › Nómina*:
   - revisar la **vista previa**: lo que saldría, exactamente;
   - **publicar**.

   La publicación queda registrada en `exportacion_interop` con su SHA-256 y quién la
   autorizó. También se puede **retirar**: el JSON vuelve a `null` en esa convocatoria.
4. Reconstruir el portal (§5).

La nómina **nunca** sale sola al cambiar de etapa: la publicación es un acto explícito
del administrador. Las bases y el calendario, en cambio, salen en vivo desde que se
publica la convocatoria.

## 5. El sitio en Hugo

Una forma de alimentar el sitio es dejar el JSON como archivo de datos y construir:

```bash
curl -fsS https://dominio/api/v1/interop/convocatoria.json -o data/fimnme.json
```

```bash
hugo --minify
```

En las plantillas, los datos quedan en `.Site.Data.fimnme`. Por ejemplo, las
convocatorias de la edición más reciente:

```
{{ $ed := index (last 1 .Site.Data.fimnme.ediciones) 0 }}
{{ range $ed.convocatorias }}<h2>{{ .titulo_es }}</h2>{{ .bases_md_es | markdownify }}{{ end }}
```

El buscador del portal (Pagefind o Lunr) indexa el sitio ya generado, en el
navegador: no consulta la plataforma.

Una tarea programada (cron o un temporizador de systemd en el servidor del portal) puede
reconstruirlo cada hora durante la convocatoria, y tras cada publicación de la nómina.
