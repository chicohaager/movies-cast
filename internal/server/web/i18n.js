// Texts of the phone and the TV page. ES5 only: the TV runs Chrome 53.
// The strings come from /strings.js (web/i18n/<lang>.json, served by the
// server), English the default and the fallback for every missing key. The
// language is "?lang=xx" (remembered in localStorage), else the remembered
// one, else the first device language we have, else English.
var I18N = (function () {
  "use strict";
  var ALL = window.I18N_STRINGS || { en: {} };
  // Native names for the language picker, in ZimaOS' set (assets on .143, 2026-10-28).
  var NAMES = {
    en: "English", de: "Deutsch", fr: "Français", es: "Español", it: "Italiano", nl: "Nederlands",
    pl: "Polski", cs: "Čeština", hu: "Magyar", sv: "Svenska", tr: "Türkçe", ru: "Русский",
    ja: "日本語", ko: "한국어", zh_CN: "简体中文", zh_TW: "繁體中文", pt_BR: "Português (Brasil)", pt_PT: "Português (Portugal)"
  };

  function store(v) {
    try {
      if (v === undefined) return window.localStorage.getItem("moviescast.lang") || "";
      window.localStorage.setItem("moviescast.lang", v);
    } catch (e) { /* private mode: the device language decides */ }
    return "";
  }

  // match turns a tag like "pt-BR", "zh-Hant-TW", "de" or "fr_FR" into a code we have, or "".
  function match(tag) {
    if (!tag) return "";
    var t = String(tag).replace(/-/g, "_");
    var low = t.toLowerCase();
    var p = low.split("_");
    if (p[0] === "zh") {
      if (low.indexOf("hant") >= 0 || p.indexOf("tw") > 0 || p.indexOf("hk") > 0 || p.indexOf("mo") > 0) return ALL.zh_TW ? "zh_TW" : "";
      return ALL.zh_CN ? "zh_CN" : "";
    }
    if (p[0] === "pt") {
      if (p[1] === "br") return ALL.pt_BR ? "pt_BR" : "";
      return ALL.pt_PT ? "pt_PT" : (ALL.pt_BR ? "pt_BR" : "");
    }
    if (ALL[t]) return t;
    return ALL[p[0]] ? p[0] : "";
  }

  var q = /[?&]lang=([A-Za-z_-]+)/.exec(location.search);
  var asked = q ? match(q[1]) : "";
  if (asked) store(asked);
  var lang = asked || match(store());
  var tags = navigator.languages || [navigator.language];
  for (var i = 0; !lang && i < tags.length; i++) lang = match(tags[i]);
  if (!lang) lang = "en";
  var dict = ALL[lang] || {};
  var en = ALL.en || {};

  function t(key) {
    var s = dict.hasOwnProperty(key) ? dict[key] : (en.hasOwnProperty(key) ? en[key] : key);
    for (var i = 1; i < arguments.length; i++) s = s.split("{" + (i - 1) + "}").join(String(arguments[i]));
    return s;
  }

  function tArgs(key, args) {
    return t.apply(null, [key].concat(args || []));
  }

  // data-i18n="key" sets the text, data-i18n-attr="placeholder:key;title:key" sets attributes.
  function apply(root) {
    var els = (root || document).querySelectorAll("[data-i18n],[data-i18n-attr]");
    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      if (el.getAttribute("data-i18n")) el.textContent = t(el.getAttribute("data-i18n"));
      var attrs = (el.getAttribute("data-i18n-attr") || "").split(";");
      for (var j = 0; j < attrs.length; j++) {
        var kv = attrs[j].split(":");
        if (kv.length === 2) el.setAttribute(kv[0], t(kv[1]));
      }
    }
    document.documentElement.lang = lang.replace("_", "-");
    document.title = t("title");
  }

  // Server errors carry "key" (+ "args", + "suffix") for every language, and
  // "error" (German) / "errorEn" for anything without a key.
  function serverError(data) {
    if (!data) return "";
    if (data.key && (dict.hasOwnProperty(data.key) || en.hasOwnProperty(data.key))) {
      var s = tArgs(data.key, data.args);
      if (data.suffix && data.suffix.key) s += " " + tArgs(data.suffix.key, data.suffix.args);
      return s;
    }
    return lang === "de" ? data.error : (data.errorEn || data.error);
  }

  // The languages for a picker: [code, native name], English first.
  function languages() {
    var out = [];
    for (var c in NAMES) if (NAMES.hasOwnProperty(c) && ALL[c]) out.push([c, NAMES[c]]);
    return out;
  }

  return { lang: lang, t: t, apply: apply, serverError: serverError, languages: languages, match: match };
})();
