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

// Auditoría automática de accesibilidad (WCAG 2.1 A y AA) del flujo de
// postulación, con axe-core en un navegador real (Manual de Validación,
// caso V-ACC-01; ADR 0031).
//
// NO es parte del sistema: es una herramienta de verificación. No se compila
// ni viaja en los binarios, y sus dos dependencias (Playwright y axe-core) se
// instalan FUERA del repositorio, en un directorio temporal:
//
//     mkdir /tmp/a11y && cd /tmp/a11y && npm init -y && npm i playwright@1.49.1 axe-core@4.10.2
//     npx playwright install chromium
//     NODE_PATH=/tmp/a11y/node_modules node deploy/accesibilidad/auditar.js http://127.0.0.1:8080
//
// Contra una instalación de PRUEBA (crea una cuenta de postulante y un
// borrador por convocatoria abierta), con al menos una convocatoria de cada
// tipo en etapa de postulación. Recorre las páginas públicas, el registro (también con
// errores), «Mi cuenta», «Mis postulaciones» y cada paso del formulario de
// cada convocatoria abierta, en tres combinaciones: español en escritorio,
// inglés en móvil y español con el tema oscuro. Sale con 0 sin violaciones.

const { chromium } = require('playwright');
const fs = require('fs');

const base = (process.argv[2] || 'http://127.0.0.1:8080').replace(/\/$/, '');
const fuenteAxe = fs.readFileSync(require.resolve('axe-core/axe.min.js'), 'utf8');
const resultados = [];

// auditar corre axe-core sobre la página abierta y guarda sus violaciones.
// La CSP del sistema no permite inyectar scripts: el contexto se abre con
// bypassCSP, sólo para esta herramienta.
async function auditar(pagina, nombre) {
  await pagina.evaluate(fuenteAxe);
  const r = await pagina.evaluate(() => axe.run(document, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] },
  }));
  resultados.push({
    nombre,
    violaciones: r.violations.map((v) => ({
      id: v.id, impacto: v.impact, ayuda: v.help,
      nodos: v.nodes.slice(0, 3).map((n) => `${n.target.join(' ')} :: ${n.failureSummary.split('\n').slice(1, 2).join('')}`),
    })),
  });
}

(async () => {
  const navegador = await chromium.launch();
  const combinaciones = [['es-MX', 1280, 'light'], ['en-US', 390, 'light'], ['es-MX', 1280, 'dark']];
  for (const [idioma, ancho, esquema] of combinaciones) {
    const contexto = await navegador.newContext({ locale: idioma, colorScheme: esquema, bypassCSP: true, viewport: { width: ancho, height: 900 } });
    const p = await contexto.newPage();
    const etiqueta = `${idioma} ${ancho}px ${esquema}`;

    // Páginas públicas: la portada y las convocatorias que enlaza.
    await p.goto(base + '/');
    const convocatorias = await p.evaluate(() => [...new Set(Array.from(document.querySelectorAll('a[href^="/convocatorias/"]')).map((a) => a.getAttribute('href')))]);
    for (const ruta of ['/', ...convocatorias, '/aviso-de-privacidad', '/cuenta/registro', '/cuenta/ingresar', '/cuenta/recuperar']) {
      await p.goto(base + ruta);
      await auditar(p, `${etiqueta} ${ruta}`);
    }
    await p.goto(base + '/cuenta/registro');
    await p.click('main button[type=submit]');
    await p.waitForLoadState();
    await auditar(p, `${etiqueta} registro con errores`);

    // Una cuenta nueva y un borrador por cada convocatoria abierta.
    await p.goto(base + '/cuenta/registro');
    await p.fill('#c-email', `auditoria.${Date.now()}@ejemplo.invalid`);
    await p.fill('#c-nombre', 'Auditoría');
    await p.fill('#c-apellidos', 'Accesibilidad');
    await p.fill('#c-password', 'una frase larga para la auditoría');
    await p.check('#c-acepta_aviso');
    await p.click('main button[type=submit]');
    await p.waitForURL(/\/cuenta/);
    await auditar(p, `${etiqueta} /cuenta`);
    for (;;) {
      await p.goto(base + '/postulaciones');
      await auditar(p, `${etiqueta} /postulaciones`);
      const tarjeta = p.locator('.tarjeta--formulario').first();
      if (await tarjeta.count() === 0) break;
      const radio = tarjeta.locator('input[name=subcategoria]').last();
      if (await radio.count() > 0) await radio.check();
      await tarjeta.locator('button[type=submit]').click();
      await p.waitForURL(/\/postulaciones\/\d+\//);
      const pasos = await p.evaluate(() => [...new Set(Array.from(document.querySelectorAll('a[href]')).map((a) => a.getAttribute('href')).filter((h) => /^\/postulaciones\/\d+\/[a-z-]+$/.test(h)))]);
      for (const paso of pasos) {
        await p.goto(base + paso);
        await auditar(p, `${etiqueta} ${paso}`);
      }
    }
    await contexto.close();
  }
  await navegador.close();

  let total = 0;
  for (const r of resultados) {
    total += r.violaciones.length;
    for (const v of r.violaciones) {
      console.log(`${r.nombre}\n  [${v.impacto}] ${v.id}: ${v.ayuda}\n    ${v.nodos.join('\n    ')}`);
    }
  }
  console.log(`Páginas auditadas: ${resultados.length}. Violaciones: ${total}.`);
  process.exit(total === 0 ? 0 : 1);
})().catch((error) => {
  console.error(error);
  process.exit(2);
});
