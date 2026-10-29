// Colour mode of the phone page: "light", "dark" or nothing (follow the system),
// chosen in the footer and remembered on this device. Loaded in <head> before
// style.css, so the first paint is already in the chosen mode (the CSP allows
// no inline script). ES5, like i18n.js.
(function () {
  "use strict";
  var mode = "";
  try { mode = window.localStorage.getItem("moviescast.theme") || ""; } catch (e) { /* private mode: system */ }
  if (mode === "light" || mode === "dark") document.documentElement.setAttribute("data-theme", mode);
})();
