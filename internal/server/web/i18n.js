// Texts of the phone and the TV page in German and English. ES5 only: the TV
// runs Chrome 53. The language is the device's (navigator.language); "?lang=en"
// or "?lang=de" overrides it and is remembered in localStorage.
var I18N = (function () {
  "use strict";
  var DE = {
    "title": "Movies auf dem Fernseher", "sub": "auf dem Fernseher", "logout": "Abmelden",
    "login.h": "Anmelden", "login.hint": "Mit deinem ZimaOS-Konto.", "login.user": "Benutzer", "login.pw": "Passwort",
    "tv.label": "Fernseher", "tv.search": "Suchen", "tv.searchTitle": "Neu suchen",
    "tv.none": "Kein Fernseher gefunden", "tv.noneHint": "Kein Fernseher gefunden. Ist er eingeschaltet und im selben Netz?",
    "tv.err": "Fernseher: {0}", "play.chooseTv": "Erst einen Fernseher wählen.", "play.force": "Trotzdem abspielen",
    "p.pause": "Pause", "p.continue": "Weiter", "p.stop": "Stopp", "p.vol": "Lautstärke",
    "pair.summary": "Fernseher-App koppeln", "pair.placeholder": "Code vom Fernseher", "pair.btn": "Koppeln",
    "pair.note": "Nur für die movies-cast-App auf dem Fernseher. Fernseher mit DLNA brauchen keine Kopplung.",
    "m.summary": "Fernseher verwalten", "m.hostPh": "IP des Fernsehers", "m.add": "Hinzufügen", "m.none": "Noch kein Fernseher gefunden.",
    "m.name": "Name für {0}", "m.hide": "ausblenden", "m.save": "Speichern", "m.saved": "Gespeichert.", "m.added": "Hinzugefügt – wird bei der nächsten Suche gefragt.",
    "m.remove": "Entfernen", "m.hostsH": "Von Hand hinzugefügt:",
    "s.summary": "Status", "s.version": "Version", "s.movies": "Movies", "s.ok": "erreichbar", "s.bad": "nicht erreichbar ({0})",
    "s.search": "Letzte Suche", "s.never": "noch keine", "s.found": "{0} · {1} Antworten · {2} Fernseher", "s.searchErr": "Suchfehler",
    "s.port": "Suchport", "s.portHeld": "UDP {0}, offen", "s.portFree": "UDP {0}, nicht gebunden", "s.portRandom": "zufällig",
    "s.hidden": "Ausgeblendete Fernseher", "s.added": "Von Hand hinzugefügt", "s.formats": "Bewährte Formate", "s.plays": "Laufende Wiedergaben",
    "pair.need6": "Bitte die 6 Ziffern vom Fernseher eingeben.", "pair.ok": "Gekoppelt – der Fernseher zeigt gleich die Filme.",
    "row.resume": "Weiterschauen", "row.all": "Alle Filme", "empty": "Keine Filme in Movies gefunden.",
    "d.resume": "Fortsetzen bei {0}", "d.start": "Von vorn abspielen", "d.close": "Schließen", "d.back": "Zurück",
    "st.PLAYING": "läuft", "st.PAUSED_PLAYBACK": "Pause", "st.STOPPED": "gestoppt", "st.TRANSITIONING": "lädt …",
    "st.NO_MEDIA_PRESENT": "bereit",
    "err.server": "Server nicht erreichbar", "err.serverDetail": "Server nicht erreichbar: {0}",
    "err.relogin": "Bitte neu anmelden", "err.http": "Fehler {0}",
    "pairtv.h": "Fernseher koppeln", "pairtv.s1a": "Am Handy ", "pairtv.s1b": " öffnen und anmelden",
    "pairtv.qr": "Mit der Handykamera scannen – oder:", "pair.fromQR": "Code vom Fernseher übernommen – jetzt „Koppeln“ drücken.",
    "pairtv.s2": "„Fernseher-App koppeln“ wählen", "pairtv.s3": "Diesen Code eingeben",
    "pairtv.wait": "Wartet auf Bestätigung am Handy …", "pairtv.fail": "Kopplung nicht möglich: {0} — neuer Versuch in 10 s",
    "osd.hint": "OK Pause · ◀ −10 s · ▶ +30 s · ▲ Ton · ▼ Untertitel · Zurück beendet", "loading": "lädt …",
    "o.playing": "läuft", "o.paused": "Pause", "o.audio": "Ton {0}/{1}", "o.subOff": "UT aus", "o.sub": "UT {0}", "o.subLoading": "Untertitel werden geladen …", "o.subNone": "Keine Untertitel",
    "play.err": "Wiedergabe: {0}", "play.fail": "Wiedergabe nicht möglich"
  };
  var EN = {
    "title": "Movies on the TV", "sub": "on the TV", "logout": "Log out",
    "login.h": "Log in", "login.hint": "With your ZimaOS account.", "login.user": "User", "login.pw": "Password",
    "tv.label": "TV", "tv.search": "Search", "tv.searchTitle": "Search again",
    "tv.none": "No TV found", "tv.noneHint": "No TV found. Is it switched on and on the same network?",
    "tv.err": "TV: {0}", "play.chooseTv": "Choose a TV first.", "play.force": "Play anyway",
    "p.pause": "Pause", "p.continue": "Resume", "p.stop": "Stop", "p.vol": "Volume",
    "pair.summary": "Pair the TV app", "pair.placeholder": "Code from the TV", "pair.btn": "Pair",
    "pair.note": "Only for the movies-cast app on the TV. TVs with DLNA need no pairing.",
    "m.summary": "Manage TVs", "m.hostPh": "TV IP address", "m.add": "Add", "m.none": "No TV found yet.",
    "m.name": "Name for {0}", "m.hide": "hide", "m.save": "Save", "m.saved": "Saved.", "m.added": "Added – asked at the next search.",
    "m.remove": "Remove", "m.hostsH": "Added by hand:",
    "s.summary": "Status", "s.version": "Version", "s.movies": "Movies", "s.ok": "reachable", "s.bad": "not reachable ({0})",
    "s.search": "Last search", "s.never": "none yet", "s.found": "{0} · {1} answers · {2} TVs", "s.searchErr": "Search error",
    "s.port": "Search port", "s.portHeld": "UDP {0}, open", "s.portFree": "UDP {0}, not bound", "s.portRandom": "random",
    "s.hidden": "Hidden TVs", "s.added": "Added by hand", "s.formats": "Proven formats", "s.plays": "Active plays",
    "pair.need6": "Enter the 6 digits shown on the TV.", "pair.ok": "Paired – the TV shows your films in a moment.",
    "row.resume": "Continue watching", "row.all": "All films", "empty": "No films found in Movies.",
    "d.resume": "Continue at {0}", "d.start": "Play from the start", "d.close": "Close", "d.back": "Back",
    "st.PLAYING": "playing", "st.PAUSED_PLAYBACK": "paused", "st.STOPPED": "stopped", "st.TRANSITIONING": "loading …",
    "st.NO_MEDIA_PRESENT": "ready",
    "err.server": "Server not reachable", "err.serverDetail": "Server not reachable: {0}",
    "err.relogin": "Please log in again", "err.http": "Error {0}",
    "pairtv.h": "Pair this TV", "pairtv.s1a": "On your phone, open ", "pairtv.s1b": " and log in",
    "pairtv.qr": "Scan with the phone camera – or:", "pair.fromQR": "Code taken from the TV – now press “Pair”.",
    "pairtv.s2": "Choose “Pair the TV app”", "pairtv.s3": "Enter this code",
    "pairtv.wait": "Waiting for confirmation on the phone …", "pairtv.fail": "Pairing not possible: {0} — retrying in 10 s",
    "osd.hint": "OK pause · ◀ −10 s · ▶ +30 s · ▲ audio · ▼ subtitles · Back ends", "loading": "loading …",
    "o.playing": "playing", "o.paused": "paused", "o.audio": "Audio {0}/{1}", "o.subOff": "Subtitles off", "o.sub": "Subtitles {0}", "o.subLoading": "Loading subtitles …", "o.subNone": "No subtitles",
    "play.err": "Playback: {0}", "play.fail": "Playback not possible"
  };

  function store(v) {
    try {
      if (v === undefined) return window.localStorage.getItem("moviescast.lang") || "";
      window.localStorage.setItem("moviescast.lang", v);
    } catch (e) { /* private mode: the device language decides */ }
    return "";
  }

  var m = /[?&]lang=(de|en)\b/.exec(location.search);
  if (m) store(m[1]);
  var lang = (m && m[1]) || store() || ((navigator.language || "en").toLowerCase().indexOf("de") === 0 ? "de" : "en");
  var dict = lang === "de" ? DE : EN;

  function t(key) {
    var s = dict.hasOwnProperty(key) ? dict[key] : key;
    for (var i = 1; i < arguments.length; i++) s = s.split("{" + (i - 1) + "}").join(String(arguments[i]));
    return s;
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
    document.documentElement.lang = lang;
    document.title = t("title");
  }

  // Server errors carry "error" (German) and "errorEn".
  function serverError(data) {
    if (!data) return "";
    return lang === "de" ? data.error : (data.errorEn || data.error);
  }

  return { lang: lang, t: t, apply: apply, serverError: serverError, DE: DE, EN: EN };
})();
