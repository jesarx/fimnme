/*
 * FIMNME — JavaScript propio, mínimo.
 * Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
 *
 * Todo lo que hace este archivo es una MEJORA: la plataforma funciona sin él
 * y sin htmx (cada formulario se envía con su botón). Aquí no hay reglas de
 * negocio ni validación: eso vive una sola vez, en el servidor.
 *
 *   1. Marca el documento como «con JavaScript» y activa el menú móvil.
 *   2. Avisa cuando un guardado automático falla, y no deja ir lo capturado;
 *      llena la barra de progreso de las subidas.
 *   3. Muestra las horas en la hora local de quien mira.
 *
 * La CSP no admite código en línea: por eso todo está aquí, y htmx se usa
 * sólo con atributos.
 */
(function () {
  "use strict";

  var raiz = document.documentElement;
  raiz.classList.add("js");

  /* --- 1. Menú en pantallas angostas ----------------------------------- */

  var boton = document.querySelector(".menu-boton");
  var menu = document.getElementById("menu");
  if (boton && menu) {
    boton.hidden = false;
    boton.addEventListener("click", function () {
      var abierto = boton.getAttribute("aria-expanded") === "true";
      boton.setAttribute("aria-expanded", abierto ? "false" : "true");
      menu.classList.toggle("menu--abierto", !abierto);
    });
  }

  /* --- 2. Guardado automático ------------------------------------------ */

  // Formularios con algo escrito que el servidor aún no confirma.
  var pendientes = new Set();

  function formularioDe(elemento) {
    return elemento && elemento.closest ? elemento.closest("form[data-autoguardado]") : null;
  }

  // Un formulario que el servidor ya sustituyó por otro (al añadir un
  // integrante, por ejemplo) ya no cuenta: lo que tenía se envió con esa
  // misma petición, y reenviarlo después pisaría lo nuevo con lo viejo.
  function podar() {
    pendientes.forEach(function (formulario) {
      if (!document.contains(formulario)) pendientes.delete(formulario);
    });
  }

  function avisar(clave) {
    var aviso = document.getElementById("guardado");
    if (!aviso) return;
    aviso.textContent = aviso.getAttribute("data-" + clave) || "";
    aviso.classList.toggle("guardado--error", clave === "fallo");
  }

  document.addEventListener("input", function (evento) {
    var formulario = formularioDe(evento.target);
    if (formulario) {
      pendientes.add(formulario);
      avisar("pendiente");
    }
  });

  // El servidor confirmó: lo de ese formulario ya está guardado. La hora la
  // escribe el propio servidor en el aviso.
  document.addEventListener("htmx:afterRequest", function (evento) {
    var formulario = formularioDe(evento.detail.elt);
    if (formulario && evento.detail.successful) {
      pendientes.delete(formulario);
    } else if (formulario) {
      avisar("fallo");
    }
    podar();
  });

  // Sin red, o el servidor no responde: se dice, y lo escrito sigue ahí.
  document.addEventListener("htmx:sendError", function () { avisar("fallo"); });
  document.addEventListener("htmx:timeout", function () { avisar("fallo"); });

  // Al salir de la página con algo sin confirmar, se manda de todos modos.
  // sendBeacon entrega la petición aunque la página ya no exista.
  window.addEventListener("pagehide", function () {
    podar();
    pendientes.forEach(function (formulario) {
      if (navigator.sendBeacon) {
        navigator.sendBeacon(formulario.action, new URLSearchParams(new FormData(formulario)));
      }
    });
    pendientes.clear();
  });

  /* --- Barra de progreso de las subidas (RF-POST-07) ------------------- */

  // htmx avisa del avance de la petición; aquí sólo se llena la barra del
  // formulario que sube. Sin JavaScript el navegador muestra el suyo.
  document.addEventListener("htmx:xhr:progress", function (evento) {
    var formulario = evento.detail.elt;
    var barra = formulario && formulario.querySelector ? formulario.querySelector(".subida__progreso") : null;
    if (!barra || !evento.detail.lengthComputable) return;
    barra.hidden = false;
    barra.value = Math.round((evento.detail.loaded / evento.detail.total) * 100);
  });

  /* --- 3. Horas en la hora local --------------------------------------- */

  // El servidor escribe la hora de la Ciudad de México, con su zona. Quien
  // postula desde otro huso la ve en el suyo.
  function localizarHoras(dentroDe) {
    var horas = (dentroDe || document).querySelectorAll("time[data-hora-local]");
    Array.prototype.forEach.call(horas, function (hora) {
      var momento = new Date(hora.getAttribute("datetime"));
      if (isNaN(momento.getTime())) return;
      hora.textContent = momento.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    });
  }

  localizarHoras();
  // Lo que htmx acaba de pintar, dentro o fuera de banda, también.
  document.addEventListener("htmx:afterSettle", function () { localizarHoras(); });
  document.addEventListener("htmx:oobAfterSwap", function () { localizarHoras(); });
})();
