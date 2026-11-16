# ADR 0029 — Migración verificada de SQLite a MariaDB

- **Estado:** aceptada
- **Fecha:** 2026-11-16

Quinta entrega del paso 14 (§3.C: «esquemas compatibles y script de migración
verificada, con soporte de tipos de datos, llaves foráneas (InnoDB) y cotejo
utf8mb4_unicode_ci, exportando tanto tablas nativas como columnas agregadas por CLI»).
Sin dependencias: no hace falta un controlador de MariaDB en Go, porque la carga y la
lectura de vuelta las hace el cliente `mariadb`.

## Alcance

Esto **muda los datos** a MariaDB con la garantía de que son idénticos. Que la
aplicación **funcione** sobre MariaDB es otro trabajo, aún no hecho: exige el conector
(previsto en el perfil, con su ADR), un SQL de `repository.go` portable en los pocos
puntos que no lo son y el buscador reescrito sobre `FULLTEXT` en lugar de FTS5.

## Decisión

- **`fimnme-cli exportar-mariadb`** lee el esquema **vivo** dentro de una transacción
  de lectura (una foto coherente, con el servicio en marcha): las tablas, sus columnas
  —también las de `fimnme-cli schema`—, llaves primarias, índices y llaves únicas,
  llaves foráneas con su acción y las restricciones `CHECK`, que se extraen del texto
  `CREATE TABLE` porque SQLite no las expone. Escribe un `.sql` y un manifiesto.
- **Tipos**: `INTEGER` → `BIGINT` (`AUTO_INCREMENT` si es la llave, el rowid de
  SQLite); texto → `LONGTEXT`, o `VARCHAR(255)` si es parte de una llave o un índice
  (MariaDB no indexa `LONGTEXT`; un valor más largo detiene la exportación con su
  nombre). Tablas InnoDB, `utf8mb4` y `utf8mb4_unicode_ci` por omisión.
- **Fidelidad del cotejo**: cada columna de texto conserva la comparación exacta de
  SQLite (`utf8mb4_bin`); las `COLLATE NOCASE` (correos) pasan a `utf8mb4_unicode_ci`.
  Con `unicode_ci` en todo, dos valores que SQLite distingue —«Duo» y «Dúo»—
  chocarían en una llave única.
- **Carga en orden de dependencias** con `foreign_key_checks = 1` y modo estricto:
  MariaDB comprueba cada llave foránea y cada `CHECK` al cargar cada fila. Un ciclo de
  llaves foráneas es un error.
- **Triggers** traducidos a mano (`RAISE(ABORT)` → `SIGNAL`): puntaje dentro de la
  escala y bitácora de sólo inserción. Los del buscador no se trasladan; `indice_busqueda`
  llega con un índice `FULLTEXT`.
- **Verificación** (`deploy/verify_and_migrate_mariadb.sh`): crea la base, se niega a
  cargar si ya tiene tablas, carga, y por cada tabla compara el número de filas y una
  SHA-256 de su contenido, calculada del mismo modo a los dos lados: cada fila como la
  imprime `mariadb --batch`, renglones ordenados byte a byte. Exige haber verificado
  todas las tablas del manifiesto y los cuatro triggers.
- **Corrección de lo dicho en el paso 10b**: un campo extendido «no exportable» queda
  fuera de los reportes, pero **la migración lo lleva**. Mudar la base no es exportar, y
  omitirlo perdería sus datos sin aviso. Se corrigieron el manual, la ayuda de
  `schema add` y el comentario del modelo.

## Comprobado (MariaDB 11.8 en contenedor)

- La base de la prueba de carga (44 tablas, 2 778 filas): las 44 idénticas y 4 triggers.
- Texto con barra, tabulador, salto de línea, retorno, comillas, carácter nulo, emoji y
  acentos; y una columna añadida con `schema add`: idénticos.
- MariaDB hace cumplir las protecciones: modificar o borrar la bitácora, un puntaje fuera
  de escala, un `CHECK` (idioma inválido), una llave foránea (borrar una convocatoria
  con postulaciones) y la unicidad del correo sin distinguir mayúsculas: rechazados.
- Sobre una base con tablas, el guion se niega.
- **Un defecto hallado y corregido**: la primera versión del guion «verificaba» una sola
  tabla y se declaraba correcta. El cliente, dentro del bucle, consumía la entrada (el
  manifiesto). Ahora no la lee, y el guion exige el número completo de tablas.
