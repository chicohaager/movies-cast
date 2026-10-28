// TV UI for the LG webOS app. Runs on Chrome 53 (measured user agent,
// 2026-10-28): no async/await, no object spread, no ?. / ??, no padStart,
// no Object.entries/values. TestTVAssetsFitChrome53 enforces this list.
"use strict";
var tr = I18N.t;
I18N.apply();

var KEY = {
  LEFT: 37, UP: 38, RIGHT: 39, DOWN: 40, OK: 13,
  BACK: 461, BACKSPACE: 8, ESC: 27,
  PLAY: 415, PAUSE: 19, PLAYPAUSE: 10252, STOP: 413, FF: 417, RW: 412
};

function $(id) { return document.getElementById(id); }

var state = { screen: "", movies: [], resume: [], selected: null, play: null, lastBeat: 0, osdTimer: 0, pairTimer: 0, subIdx: -1 };

function showMsg(text) {
  var m = $("msg");
  m.textContent = text || "";
  m.hidden = !text;
  if (text) {
    clearTimeout(showMsg.t);
    showMsg.t = setTimeout(function () { m.hidden = true; }, 8000);
  }
}

function api(path, body) {
  var opt = { credentials: "same-origin" };
  if (body !== undefined) {
    opt.method = "POST";
    opt.headers = { "Content-Type": "application/json" };
    opt.body = JSON.stringify(body);
  }
  return fetch(path, opt).then(function (res) {
    return res.json().catch(function () { return null; }).then(function (data) {
      if (!res.ok) {
        var err = new Error(I18N.serverError(data) || tr("err.http", res.status));
        err.status = res.status;
        throw err;
      }
      return data;
    });
  });
}

function pad2(n) { return (n < 10 ? "0" : "") + n; }
function hms(sec) {
  sec = Math.max(0, Math.floor(sec || 0));
  return Math.floor(sec / 3600) + ":" + pad2(Math.floor(sec / 60) % 60) + ":" + pad2(sec % 60);
}

// ---- screens and focus -------------------------------------------------

function show(name) {
  ["pair", "lib", "detail", "player"].forEach(function (s) { $(s).hidden = s !== name; });
  state.screen = name;
}

function focusables() {
  var root = $(state.screen);
  return Array.prototype.slice.call(root.querySelectorAll(".focusable")).filter(function (el) {
    return !el.hidden && el.offsetParent !== null;
  });
}

function setFocus(el) {
  var old = document.querySelectorAll(".focus");
  for (var i = 0; i < old.length; i++) old[i].classList.remove("focus");
  if (!el) return;
  el.classList.add("focus");
  if (el.scrollIntoViewIfNeeded) el.scrollIntoViewIfNeeded(false);
  else el.scrollIntoView(false);
}

function current() { return document.querySelector("#" + state.screen + " .focus"); }

// Spatial navigation: nearest focusable in the pressed direction.
function move(dx, dy) {
  var cur = current();
  var all = focusables();
  if (!cur) { setFocus(all[0]); return; }
  var a = cur.getBoundingClientRect();
  var ax = a.left + a.width / 2, ay = a.top + a.height / 2;
  var best = null, bestScore = Infinity;
  all.forEach(function (el) {
    if (el === cur) return;
    var b = el.getBoundingClientRect();
    var bx = b.left + b.width / 2, by = b.top + b.height / 2;
    var along = dx ? (bx - ax) * dx : (by - ay) * dy;
    var across = dx ? Math.abs(by - ay) : Math.abs(bx - ax);
    if (along <= 1) return;
    var score = along + across * 3;
    if (score < bestScore) { bestScore = score; best = el; }
  });
  if (best) setFocus(best);
}

// ---- pairing ------------------------------------------------------------

function startPairing() {
  show("pair");
  $("pair-url").textContent = location.origin;
  clearTimeout(state.pairTimer);
  api("/api/tv/pair/start", {}).then(function (p) {
    $("pair-code").textContent = p.code.slice(0, 3) + " " + p.code.slice(3);
    // The phone camera opens the page with this code filled in (tvPairQR).
    var img = $("pair-qr");
    img.onload = function () { img.hidden = false; $("pair-qrhint").hidden = false; };
    img.onerror = function () { img.hidden = true; $("pair-qrhint").hidden = true; };
    img.src = "/api/tv/pair/qr?id=" + encodeURIComponent(p.id);
    $("pair-state").textContent = tr("pairtv.wait");
    pollPairing(p.id);
  }).catch(function (e) {
    $("pair-state").textContent = tr("pairtv.fail", e.message);
    state.pairTimer = setTimeout(startPairing, 10000);
  });
}

function pollPairing(id) {
  state.pairTimer = setTimeout(function () {
    fetch("/api/tv/pair/poll?id=" + encodeURIComponent(id), { credentials: "same-origin" }).then(function (res) {
      if (res.status === 404) { startPairing(); return null; } // expired: new code
      return res.json();
    }).then(function (d) {
      if (!d) return;
      if (d.pending) { pollPairing(id); return; }
      openLibrary();
    }).catch(function () { pollPairing(id); });
  }, 2000);
}

// ---- library and detail -------------------------------------------------

function posterURL(m) { return "/api/poster/" + encodeURIComponent(m.id); }

function openLibrary(keepId) {
  show("lib");
  api("/api/me").then(function (me) {
    $("lib-user").textContent = me.name;
    return Promise.all([api("/api/movies"), api("/api/resume").catch(function () { return []; })]);
  }).then(function (lists) {
    state.movies = lists[0];
    state.resume = lists[1] || [];
    renderGrid();
    var target = null;
    if (keepId) target = document.querySelector('.tile[data-id="' + keepId + '"]');
    setFocus(target || document.querySelector(".tile"));
  }).catch(function (e) {
    if (e.status === 401) { startPairing(); return; }
    showMsg(e.message);
  });
}

function renderGrid() {
  renderTiles($("resume"), state.resume);
  $("resume-wrap").hidden = state.resume.length === 0;
  $("all-h").hidden = state.resume.length === 0;
  renderTiles($("grid"), state.movies);
  $("lib-empty").hidden = state.movies.length > 0;
}

function renderTiles(grid, list) {
  grid.innerHTML = "";
  list.forEach(function (m) {
    var t = document.createElement("div");
    t.className = "tile focusable";
    t.setAttribute("data-id", m.id);
    var img = document.createElement(m.hasPoster ? "img" : "div");
    img.className = "poster";
    if (m.hasPoster) img.src = posterURL(m);
    t.appendChild(img);
    var bar = document.createElement("div");
    bar.className = "bar2";
    if (m.resumeSeconds > 0 && m.runtimeSeconds > 0) {
      var i = document.createElement("i");
      i.style.width = Math.min(100, 100 * m.resumeSeconds / m.runtimeSeconds) + "%";
      bar.appendChild(i);
    }
    t.appendChild(bar);
    var cap = document.createElement("div");
    cap.className = "cap";
    cap.textContent = m.name;
    t.appendChild(cap);
    grid.appendChild(t);
  });
}

// Detail data comes from the full list; the resume row only points into it.
function findMovie(id) {
  var m = null;
  state.movies.forEach(function (x) { if (x.id === id) m = x; });
  return m;
}

function metaLine(m) {
  var parts = [];
  if (m.year) parts.push(m.year);
  if (m.runtimeSeconds) parts.push(Math.round(m.runtimeSeconds / 60) + " min");
  if (m.width >= 3800) parts.push("4K");
  if (m.videoCodec) parts.push(m.videoCodec.toUpperCase());
  if (m.audio) parts.push(m.audio.toUpperCase());
  return parts.join(" · ");
}

function openDetail(id) {
  var m = findMovie(id);
  if (!m) return;
  state.selected = m;
  show("detail");
  $("d-title").textContent = m.name;
  $("d-meta").textContent = metaLine(m);
  $("d-overview").textContent = m.overview || "";
  $("d-poster").src = m.hasPoster ? posterURL(m) : "";
  var r = $("d-resume");
  r.hidden = !(m.resumeSeconds > 0);
  r.textContent = tr("d.resume", hms(m.resumeSeconds));
  setFocus(r.hidden ? $("d-start") : r);
}

// ---- player -------------------------------------------------------------

var video = $("video");

function startPlay(resume) {
  var m = state.selected;
  show("player");
  $("spinner").hidden = false;
  $("o-title").textContent = m.name;
  $("o-audio").textContent = "";
  $("o-sub").textContent = "";
  api("/api/tv/play", { movie: m.id, resume: resume }).then(function (p) {
    state.play = p;
    state.subsOnRequest = !!p.subtitlesOnRequest;
    // Subtitle tracks come only once the picture runs: with a remembered
    // language the LG held the start until that track had loaded, and Movies
    // takes 30 s to extract an embedded track (measured: canplay at 30.1 s
    // with tracks, 0.86 s without, Avatar, 2026-10-28).
    setTracks([]);
    state.pendingSubs = p.subtitles || [];
    video.src = p.url;
    var started = false;
    video.onloadedmetadata = function () {
      if (!started && p.resumeSeconds > 0) video.currentTime = p.resumeSeconds;
      started = true;
      applyAudioPref();
      showAudio();
      applySub();
    };
    var pr = video.play();
    if (pr && pr.catch) pr.catch(function (e) { showMsg(tr("play.err", e.message)); });
    osd();
  }).catch(function (e) {
    showMsg(e.message);
    openDetail(m.id);
  });
}

function report(extra) {
  if (!state.play) return Promise.resolve();
  var body = { play: state.play.play, position: video.currentTime || 0, paused: video.paused, stopped: false };
  if (extra && extra.stopped) body.stopped = true;
  state.lastBeat = Date.now();
  return api("/api/tv/progress", body).catch(function () { /* next beat retries */ });
}

function stopPlay() {
  if (!state.play) return;
  var id = state.selected && state.selected.id;
  var done = report({ stopped: true });
  state.play = null;
  video.pause();
  video.removeAttribute("src");
  video.load();
  done.then(function () { openLibrary(id); });
}

function osd() {
  var o = $("osd");
  o.classList.remove("off");
  clearTimeout(state.osdTimer);
  state.osdTimer = setTimeout(function () {
    if (!video.paused) o.classList.add("off");
    liftCues();
  }, 4000);
  updateOSD();
}

// Rows a cue takes: its line breaks, plus one for a long line that wraps. Line
// -N places the cue's first row N rows from the bottom; further rows run down
// (two-row cues reached into the bar at -5, seen in a recording).
function cueLines(text) {
  var rows = String(text || "").split("\n"), n = rows.length;
  for (var i = 0; i < rows.length; i++) if (rows[i].length > 42) n++;
  return n;
}

// While the bar at the bottom shows, subtitles move up above it; they lay on
// the key line before (seen in a recording, 2026-10-28). Cues load after the
// track, so the cue count is part of the remembered state.
function liftCues() {
  var up = !$("osd").classList.contains("off");
  var tt = video.textTracks, n = 0, i, j;
  for (i = 0; i < tt.length; i++) n += tt[i].cues ? tt[i].cues.length : 0;
  var key = (up ? "up:" : "down:") + n;
  if (key === state.cueLift) return;
  state.cueLift = key;
  for (i = 0; i < tt.length; i++) {
    var c = tt[i].cues;
    if (!c) continue;
    for (j = 0; j < c.length; j++) {
      try { c[j].line = up ? -(4 + cueLines(c[j].text)) : "auto"; } catch (e) { /* a cue that refuses keeps its place */ }
    }
    // A cue on screen keeps its old place until the track is drawn again.
    if (tt[i].mode === "showing") { tt[i].mode = "hidden"; tt[i].mode = "showing"; }
  }
}

function updateOSD() {
  var d = video.duration || (state.play && state.play.durationSeconds) || 0;
  $("o-pos").textContent = hms(video.currentTime);
  $("o-dur").textContent = hms(d);
  $("o-fill").style.width = d ? (100 * video.currentTime / d) + "%" : "0";
  liftCues();
  $("o-state").textContent = video.paused ? tr("o.paused") : tr("o.playing");
}

function seekBy(sec) {
  var d = video.duration || 0;
  var to = Math.max(0, video.currentTime + sec);
  if (d) to = Math.min(d - 1, to);
  video.currentTime = to;
  osd();
}

function audioTracks() { return video.audioTracks || { length: 0 }; }

function showAudio() {
  var at = audioTracks();
  for (var i = 0; i < at.length; i++) {
    if (at[i].enabled) {
      $("o-audio").textContent = tr("o.audio", i + 1, at.length) + (at[i].language ? " · " + at[i].language : "") + (at[i].label ? " · " + at[i].label : "");
    }
  }
}

function cycleAudio(dir) {
  var at = audioTracks();
  if (at.length < 2) { osd(); return; }
  var cur = 0;
  for (var i = 0; i < at.length; i++) if (at[i].enabled) cur = i;
  var next = (cur + dir + at.length) % at.length;
  for (var j = 0; j < at.length; j++) at[j].enabled = j === next;
  if (at[next].language) savePref("audioLang", at[next].language);
  showAudio();
  osd();
}

// The last chosen audio language carries over to the next film. A film
// without that language keeps its own default track; the pref stays.
function applyAudioPref() {
  var pref = loadPref("audioLang");
  var at = audioTracks();
  if (!pref || at.length < 2) return;
  var want = -1;
  for (var i = 0; i < at.length; i++) {
    if (at[i].language === pref) { want = i; break; }
  }
  if (want < 0 || at[want].enabled) return;
  for (var j = 0; j < at.length; j++) at[j].enabled = j === want;
}

video.textTracks.addEventListener("change", function () {
  if (state.screen === "player") applySub(); // applySub only writes on a difference, so no loop
});
video.addEventListener("waiting", function () { $("spinner").hidden = false; });
video.addEventListener("playing", function () {
  $("spinner").hidden = true;
  attachPendingSubs(true);
  report();
  osd();
});

// attachPendingSubs adds the play's subtitle tracks; probe: also ask for
// subtitles the item did not list when a language is remembered.
function attachPendingSubs(probe) {
  if (!state.pendingSubs) return;
  var subs = state.pendingSubs;
  state.pendingSubs = null;
  setTracks(subs);
  var want = loadPref("subLang");
  if (probe && state.subsOnRequest && want && want !== "off") probeSubs(false);
}
video.addEventListener("pause", function () { report(); osd(); });
video.addEventListener("timeupdate", function () {
  if (state.screen === "player") updateOSD();
  if (Date.now() - state.lastBeat > 10000) report();
});
video.addEventListener("ended", stopPlay);
video.addEventListener("error", function () {
  var e = video.error;
  showMsg(tr("play.fail") + (e ? " (Code " + e.code + ")" : ""));
});

// ---- subtitles ----------------------------------------------------------

function setTracks(subs) {
  var old = video.querySelectorAll("track");
  for (var i = 0; i < old.length; i++) video.removeChild(old[i]);
  subs.forEach(function (st) {
    var t = document.createElement("track");
    t.kind = "subtitles";
    t.label = st.label;
    if (st.lang) t.srclang = st.lang;
    t.src = st.url;
    video.appendChild(t);
  });
  state.subIdx = -1;
  // The last choice ("off" or a language) carries over to the next film.
  var pref = loadPref("subLang");
  for (var j = 0; j < video.textTracks.length; j++) {
    if (pref && pref !== "off" && video.textTracks[j].language === pref) {
      state.subIdx = j;
      break;
    }
  }
  applySub();
}

// Exactly state.subIdx shows. Measured on the TV: with the choice set before
// the film loaded, both tracks ended up "showing"; so the choice is enforced
// again after load and whenever the track list changes.
function applySub() {
  var tt = video.textTracks;
  for (var i = 0; i < tt.length; i++) {
    var want = i === state.subIdx ? "showing" : "disabled";
    if (tt[i].mode !== want) tt[i].mode = want;
  }
  showSub();
}

// localStorage can be missing or throw (private mode, quota); the choice is a
// convenience, so failures fall back to "off".
function loadPref(k) { try { return window.localStorage.getItem("moviescast." + k) || ""; } catch (e) { return ""; } }
function savePref(k, v) { try { window.localStorage.setItem("moviescast." + k, v); } catch (e) { /* ignore */ } }

function showSub() {
  var tt = video.textTracks;
  if (!tt.length) { $("o-sub").textContent = ""; return; }
  $("o-sub").textContent = state.subIdx < 0 ? tr("o.subOff") : tr("o.sub", tt[state.subIdx].label || tt[state.subIdx].language);
}

// Down walks: off, track 1, ..., off.
// probeSubs asks the server for subtitles the item did not list (the server
// then asks Movies' PlaybackInfo once per film). pick: select the first track.
function probeSubs(pick) {
  if (!state.play || state.probing) return;
  state.probing = true;
  var playID = state.play.play;
  $("o-sub").textContent = tr("o.subLoading");
  osd();
  api("/api/tv/subs-probe", { play: playID }).then(function (r) {
    state.probing = false;
    if (!state.play || state.play.play !== playID) return;
    state.subsOnRequest = false;
    setTracks(r.subtitles || []);
    if (!video.textTracks.length) { $("o-sub").textContent = tr("o.subNone"); return; }
    if (pick && state.subIdx < 0) {
      state.subIdx = 0;
      applySub();
      savePref("subLang", video.textTracks[0].language || "off");
    }
  }).catch(function (e) {
    state.probing = false;
    showMsg(e.message);
  });
}

function cycleSub() {
  attachPendingSubs(false);
  var n = video.textTracks.length;
  if (!n) {
    if (state.subsOnRequest) probeSubs(true);
    else osd();
    return;
  }
  state.subIdx = state.subIdx + 1 >= n ? -1 : state.subIdx + 1;
  applySub();
  savePref("subLang", state.subIdx < 0 ? "off" : (video.textTracks[state.subIdx].language || "off"));
  showSub();
  osd();
}

function togglePause() {
  if (video.paused) video.play(); else video.pause();
  osd();
}

// ---- keys ---------------------------------------------------------------

document.addEventListener("keydown", function (ev) {
  var k = ev.keyCode;
  var isBack = k === KEY.BACK || k === KEY.BACKSPACE || k === KEY.ESC;
  if (state.screen === "player") {
    ev.preventDefault();
    if (isBack || k === KEY.STOP) stopPlay();
    else if (k === KEY.OK || k === KEY.PLAYPAUSE) togglePause();
    else if (k === KEY.PLAY) { video.play(); osd(); }
    else if (k === KEY.PAUSE) { video.pause(); osd(); }
    else if (k === KEY.LEFT || k === KEY.RW) seekBy(-10);
    else if (k === KEY.RIGHT || k === KEY.FF) seekBy(30);
    else if (k === KEY.UP) cycleAudio(1);
    else if (k === KEY.DOWN) cycleSub();
    else osd();
    return;
  }
  if (k === KEY.LEFT) { ev.preventDefault(); move(-1, 0); }
  else if (k === KEY.RIGHT) { ev.preventDefault(); move(1, 0); }
  else if (k === KEY.UP) { ev.preventDefault(); move(0, -1); }
  else if (k === KEY.DOWN) { ev.preventDefault(); move(0, 1); }
  else if (k === KEY.OK) {
    ev.preventDefault();
    var c = current();
    if (!c) return;
    if (state.screen === "lib") openDetail(c.getAttribute("data-id"));
    else if (c.id === "d-resume") startPlay(true);
    else if (c.id === "d-start") startPlay(false);
    else if (c.id === "d-back") openLibrary(state.selected && state.selected.id);
  } else if (isBack && state.screen === "detail") {
    ev.preventDefault();
    openLibrary(state.selected && state.selected.id);
  }
  // Back on the library is left to webOS (it offers to close the app).
});

// Mouse/Magic Remote pointer: hover focuses, click activates.
document.addEventListener("mouseover", function (ev) {
  var el = ev.target.closest ? ev.target.closest(".focusable") : null;
  if (el) setFocus(el);
});
document.addEventListener("click", function (ev) {
  if (state.screen === "player") { togglePause(); return; }
  var el = ev.target.closest ? ev.target.closest(".focusable") : null;
  if (!el) return;
  setFocus(el);
  if (state.screen === "lib") openDetail(el.getAttribute("data-id"));
  else if (el.id === "d-resume") startPlay(true);
  else if (el.id === "d-start") startPlay(false);
  else if (el.id === "d-back") openLibrary(state.selected && state.selected.id);
});

// ---- films sent from the phone -----------------------------------------
// While the library or a detail page shows, the app asks every 3 s whether the
// phone chose a film for this TV (server: tvCmd). The phone's choice used to go
// by DLNA, which the LG refused for HEVC MKV ("file not recognised", 2026-10-28).

function pollCommand() {
  if (state.screen !== "lib" && state.screen !== "detail") return;
  api("/api/tv/cmd").then(function (c) {
    if (!c || !c.movie) return;
    var go = function () {
      if (!findMovie(c.movie)) return;
      openDetail(c.movie);
      startPlay(!!c.resume);
    };
    if (findMovie(c.movie)) { go(); return; }
    api("/api/movies").then(function (list) { state.movies = list; go(); });
  }).catch(function () { /* next poll */ });
}
setInterval(pollCommand, 3000);

// ---- start --------------------------------------------------------------

api("/api/me").then(function () { openLibrary(); }).catch(function (e) {
  if (e.status === 401) startPairing();
  else showMsg(tr("err.serverDetail", e.message));
});
