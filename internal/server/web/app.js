"use strict";

const $ = (id) => document.getElementById(id);
const t = I18N.t;
I18N.apply();

// Language picker: every language /strings.js has; the choice is remembered.
(function () {
  const sel = document.getElementById("lang");
  for (const [code, name] of I18N.languages()) sel.append(new Option(name, code));
  sel.value = I18N.lang;
  sel.addEventListener("change", () => {
    const u = new URL(location.href);
    u.searchParams.set("lang", sel.value);
    location.href = u.toString();
  });
})();
const state = { movies: [], resume: [], wasPlaying: false, tv: localGet("tv"), selected: null, polling: null, status: null, seeking: false };

function localGet(k) { try { return localStorage.getItem(k) || ""; } catch { return ""; } }
function localSet(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } }

// showNotice is the neutral line under the header (not an error).
function showNotice(msg) {
  const n = $("notice");
  n.textContent = msg;
  n.hidden = !msg;
  if (msg) window.scrollTo({ top: 0, behavior: "smooth" });
}

function showError(msg) {
  if (msg) showNotice("");
  const e = $("error");
  e.textContent = msg;
  e.hidden = !msg;
  $("force-row").hidden = true;
  // The line sits under the header; a film tapped further down left it out of view.
  if (msg) window.scrollTo({ top: 0, behavior: "smooth" });
}

async function api(path, body) {
  const opt = body === undefined ? {} : {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  };
  let res;
  try {
    res = await fetch(path, { credentials: "same-origin", ...opt });
  } catch (err) {
    throw new Error(t("err.server"));
  }
  let data = null;
  try { data = await res.json(); } catch { /* no body */ }
  if (res.status === 401 && path !== "/api/login") {
    showView("login");
    const e = new Error(I18N.serverError(data) || t("err.relogin"));
    e.status = 401;
    throw e;
  }
  if (!res.ok) {
    const e = new Error(I18N.serverError(data) || t("err.http", res.status));
    e.data = data;
    throw e;
  }
  return data;
}

function hms(sec) {
  sec = Math.max(0, Math.floor(sec || 0));
  const h = Math.floor(sec / 3600), m = Math.floor(sec / 60) % 60, s = sec % 60;
  return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function showView(which) {
  $("login-view").hidden = which !== "login";
  $("app-view").hidden = which !== "app";
  $("logout").hidden = which !== "app";
  if (which !== "app") stopPolling();
}

async function boot() {
  try {
    await api("/api/me");
  } catch {
    showView("login");
    return;
  }
  showView("app");
  takePairCodeFromLink();
  await Promise.all([loadMovies(), loadTVs(false)]);
  startPolling();
}

// The QR code on the TV opens "/?pair=123456": fill the code in and open the
// pairing card. Pairing still takes a tap on "Pair" — a QR code shown by some
// other TV must not hand it this login unasked.
function takePairCodeFromLink() {
  const code = new URLSearchParams(location.search).get("pair") || "";
  if (!/^[0-9]{6}$/.test(code)) return;
  const card = document.querySelector("details.pair");
  card.open = true;
  $("pair-code").value = code;
  $("pair-result").textContent = t("pair.fromQR");
  card.scrollIntoView({ block: "center" });
}

$("login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const f = new FormData(ev.target);
  const btn = ev.target.querySelector("button");
  btn.disabled = true;
  try {
    await api("/api/login", { user: f.get("user"), password: f.get("password") });
    ev.target.reset();
    showError("");
    await boot();
  } catch (err) {
    showError(err.message);
  } finally {
    btn.disabled = false;
  }
});

$("logout").addEventListener("click", async () => {
  try { await api("/api/logout", {}); } catch (err) { showError(err.message); }
  showView("login");
});

async function loadTVs(refresh) {
  const sel = $("tv");
  try {
    const tvs = await api("/api/renderers" + (refresh ? "?refresh=1" : ""));
    sel.innerHTML = "";
    if (!tvs.length) {
      sel.append(new Option(t("tv.none"), ""));
      showError(t("tv.noneHint"));
      return;
    }
    state.tvs = tvs;
    for (const tv of tvs) {
      // "off" before the address: a narrow select cuts the end of the text.
      sel.append(new Option(`${tv.off ? t("tv.off", tv.name) : tv.name} (${tv.host})`, tv.id));
    }
    if (tvs.some((t) => t.id === state.tv)) sel.value = state.tv;
    state.tv = sel.value;
    localSet("tv", state.tv);
  } catch (err) {
    sel.innerHTML = "";
    sel.append(new Option(t("tv.none"), ""));
    showError(err.message);
  }
}

// "Manage TVs": a name per TV, hide it, add one by IP (stored on the server, tvs.json).
async function loadManage() {
  const list = $("m-list"), out = $("m-result");
  try {
    const d = await api("/api/tvs");
    list.textContent = "";
    if (!d.tvs.length) {
      const p = document.createElement("p");
      p.className = "hint";
      p.textContent = t("m.none");
      list.append(p);
    }
    for (const tv of d.tvs) {
      const row = document.createElement("div");
      row.className = "mrow";
      const name = document.createElement("input");
      name.value = tv.label;
      name.placeholder = tv.name;
      name.maxLength = 40;
      name.setAttribute("aria-label", t("m.name", tv.name));
      const host = document.createElement("small");
      host.textContent = tv.host;
      const hide = document.createElement("label");
      hide.className = "chk";
      const cb = document.createElement("input");
      cb.type = "checkbox";
      cb.checked = tv.hidden;
      hide.append(cb, " " + t("m.hide"));
      const save = document.createElement("button");
      save.type = "button";
      save.className = "ghost";
      save.textContent = t("m.save");
      save.addEventListener("click", async () => {
        try {
          await api("/api/tvs", { id: tv.id, label: name.value, hidden: cb.checked });
          out.textContent = t("m.saved");
          loadTVs(false);
        } catch (err) {
          showError(err.message);
        }
      });
      row.append(name, host, hide, save);
      list.append(row);
    }
    const ul = $("m-hosts");
    ul.textContent = "";
    for (const h of d.hosts) {
      const li = document.createElement("li");
      const rm = document.createElement("button");
      rm.type = "button";
      rm.className = "ghost";
      rm.textContent = t("m.remove");
      rm.addEventListener("click", async () => {
        try {
          await api("/api/tvs/hosts", { host: h, remove: true });
          loadManage();
        } catch (err) {
          showError(err.message);
        }
      });
      li.append(h + " ", rm);
      ul.append(li);
    }
  } catch (err) {
    showError(err.message);
  }
}
$("manage").addEventListener("toggle", () => { if ($("manage").open) loadManage(); });
$("m-add").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    await api("/api/tvs/hosts", { host: $("m-host").value.trim() });
    $("m-host").value = "";
    $("m-result").textContent = t("m.added");
    showError("");
    await loadTVs(true);
    loadManage();
  } catch (err) {
    showError(err.message);
  }
});

// "Status": what was measured by hand over ssh before (Movies, search, port).
async function loadStatus() {
  const dl = $("s-list");
  try {
    const d = await api("/api/system");
    const s = d.search || {};
    const at = s.at && new Date(s.at).getFullYear() > 2000 ? new Date(s.at).toLocaleTimeString() : "";
    const rows = [
      [t("s.version"), d.version],
      [t("s.movies"), d.moviesOK ? t("s.ok") : t("s.bad", d.moviesError)],
      [t("s.search"), at ? t("s.found", at, s.answers, s.renderers) : t("s.never")],
      [t("s.port"), s.port ? t(s.portHeld ? "s.portHeld" : "s.portFree", s.port) : t("s.portRandom")],
      [t("s.hidden"), d.hiddenTVs],
      [t("s.added"), d.addedHosts],
      [t("s.formats"), d.provenFormats],
      [t("s.plays"), d.activePlays],
    ];
    if (s.error) rows.push([t("s.searchErr"), s.error]);
    dl.textContent = "";
    for (const [k, v] of rows) {
      const dt = document.createElement("dt");
      dt.textContent = k;
      const dd = document.createElement("dd");
      dd.textContent = String(v);
      dl.append(dt, dd);
    }
  } catch (err) {
    showError(err.message);
  }
}
$("sys").addEventListener("toggle", () => { if ($("sys").open) loadStatus(); });

$("tv").addEventListener("change", (ev) => {
  state.tv = ev.target.value;
  localSet("tv", state.tv);
  pollOnce();
});
$("refresh").addEventListener("click", () => loadTVs(true));

async function loadMovies() {
  try {
    const [movies, resume] = await Promise.all([
      api("/api/movies"),
      api("/api/resume").catch(() => []), // the row is extra; the list must not depend on it
    ]);
    state.movies = movies;
    state.resume = resume || [];
    renderMovies();
  } catch (err) {
    showError(err.message);
  }
}

function posterImg(m) {
  if (!m.hasPoster) {
    const d = document.createElement("div");
    d.className = "noimg";
    return d;
  }
  const img = document.createElement("img");
  img.loading = "lazy";
  img.alt = "";
  img.src = `/api/poster/${encodeURIComponent(m.id)}`;
  return img;
}

function metaLine(m) {
  const parts = [];
  if (m.year) parts.push(m.year);
  if (m.runtimeSeconds) parts.push(`${Math.round(m.runtimeSeconds / 60)} min`);
  if (m.width >= 3800) parts.push("4K");
  if (m.videoCodec) parts.push(m.videoCodec.toUpperCase());
  if (m.audio) parts.push(m.audio.toUpperCase());
  return parts.join(" · ");
}

function renderMovies() {
  $("resume-wrap").hidden = state.resume.length === 0;
  renderTiles($("resume-list"), state.resume);
  renderTiles($("list"), state.movies);
  $("empty").hidden = state.movies.length > 0;
}

function renderTiles(list, movies) {
  list.innerHTML = "";
  for (const m of movies) {
    const b = document.createElement("button");
    b.className = "tile";
    b.append(posterImg(m));
    if (m.resumeSeconds > 0 && m.runtimeSeconds > 0) {
      const bar = document.createElement("div");
      bar.className = "bar";
      const i = document.createElement("i");
      i.style.width = `${Math.min(100, (100 * m.resumeSeconds) / m.runtimeSeconds)}%`;
      bar.append(i);
      b.append(bar);
    }
    const cap = document.createElement("div");
    cap.className = "cap";
    cap.textContent = m.name;
    const small = document.createElement("small");
    small.textContent = m.year || "";
    cap.append(small);
    b.append(cap);
    b.addEventListener("click", () => openDetail(m));
    list.append(b);
  }
}

function openDetail(m) {
  state.selected = m;
  $("d-title").textContent = m.name;
  $("d-meta").textContent = metaLine(m);
  $("d-overview").textContent = m.overview || "";
  const img = $("d-poster");
  img.src = m.hasPoster ? `/api/poster/${encodeURIComponent(m.id)}` : "";
  img.hidden = !m.hasPoster;
  const r = $("d-resume");
  r.hidden = !(m.resumeSeconds > 0);
  r.textContent = t("d.resume", hms(m.resumeSeconds));
  $("detail").showModal();
}

$("detail").addEventListener("close", async () => {
  const v = $("detail").returnValue;
  if (v !== "resume" && v !== "start") return;
  if (!state.tv) {
    showError(t("play.chooseTv"));
    return;
  }
  showError("");
  play({ renderer: state.tv, movie: state.selected.id, resume: v === "resume" });
});

// The server answers 409 + "unlisted" when the TV's format list lacks this
// film's container or HEVC; the TV may still play it (the LG lists no HEVC
// and plays it), so the user decides.
async function play(req) {
  // A TV that answered no search but was seen before is woken first (Wake-on-LAN).
  const tv = (state.tvs || []).find((x) => x.id === req.renderer);
  if (tv && tv.off && !req.woke) {
    if (!(await wake(tv))) return;
    req = { ...req, woke: true };
  }
  try {
    const r = await api("/api/play", req);
    if (r && r.converted) {
      // Remembered for this TV and sound format; "Original sound" in the player undoes it.
      showNotice(t("conv.on"));
      pollOnce();
      return;
    }
    if (r && r.viaApp) {
      // The TV's movies-cast app opens the film itself; the TV's remote controls it.
      showNotice(t("play.viaApp", r.title));
      return;
    }
    showNotice("");
    pollOnce();
  } catch (err) {
    showError(err.message);
    const d = err.data || {};
    const canForce = Array.isArray(d.unlisted) && !req.force && !req.app;
    // DLNA stays the way; the TV's movies-cast app is offered only when it is open there.
    const canApp = d.appAvailable === true && !req.app;
    // Off or in standby: switch it on by Wake-on-LAN and send the film again.
    const canWake = d.wake === true && !req.woke;
    // Any other refusal: Movies can convert the sound (the picture stays as it is).
    const canConvert = !canWake && !req.convert && !req.app && err.status !== 401;
    if (canForce || canApp || canWake || canConvert) {
      $("force-row").hidden = false;
      $("force").hidden = !canForce;
      $("force").onclick = () => { showError(""); play({ ...req, force: true }); };
      $("inapp").hidden = !canApp;
      $("inapp").onclick = () => { showError(""); play({ ...req, app: true }); };
      $("convert").hidden = !canConvert;
      $("convert").onclick = () => { showError(""); play({ ...req, convert: true, remember: true }); };
      $("wake").hidden = !canWake;
      $("wake").onclick = async () => {
        showError("");
        const name = (tv && tv.name) || $("tv").selectedOptions[0]?.textContent || "";
        if (await wake({ id: req.renderer, name })) play({ ...req, woke: true });
      };
    }
  }
}

// wake sends Wake-on-LAN and waits (up to a minute on the server) until the TV answers.
async function wake(tv) {
  showNotice(t("wake.waking", tv.name));
  try {
    await api("/api/tvs/wake", { id: tv.id });
    showNotice("");
    tv.off = false;
    loadTVs(false);
    return true;
  } catch (err) {
    showNotice("");
    showError(err.message);
    return false;
  }
}

async function control(action, extra) {
  try {
    await api("/api/control", { renderer: state.tv, action, ...extra });
    showError("");
    pollOnce();
  } catch (err) {
    showError(err.message);
  }
}

$("p-toggle").addEventListener("click", () => {
  control(state.status && state.status.state === "PLAYING" ? "pause" : "play");
});
$("p-stop").addEventListener("click", () => control("stop"));
// "No sound?" restarts the film here with converted sound; "Original sound"
// goes back. Either choice is remembered for this TV and sound format.
$("p-sound").addEventListener("click", () => {
  if (!state.status || !state.status.movie) return;
  const at = state.status.positionSeconds || 0;
  const req = { renderer: state.tv, movie: state.status.movie, at, remember: true };
  if (state.converted) req.original = true, req.force = true; else req.convert = true;
  showError("");
  play(req);
});
for (const b of document.querySelectorAll("[data-skip]")) {
  b.addEventListener("click", () => {
    if (!state.status) return;
    const to = Math.max(0, state.status.positionSeconds + Number(b.dataset.skip));
    control("seek", { seconds: to });
  });
}
const seek = $("p-seek");
seek.addEventListener("input", () => { state.seeking = true; $("p-pos").textContent = hms(seek.value); });
seek.addEventListener("change", () => { state.seeking = false; control("seek", { seconds: Number(seek.value) }); });
$("p-vol").addEventListener("change", (ev) => control("volume", { volume: Number(ev.target.value) }));

$("pair-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const code = $("pair-code").value.replace(/\s+/g, "");
  const out = $("pair-result");
  if (!/^[0-9]{6}$/.test(code)) {
    out.textContent = t("pair.need6");
    return;
  }
  try {
    await api("/api/tv/pair/approve", { code });
    out.textContent = t("pair.ok");
    $("pair-code").value = "";
    if (location.search.includes("pair=")) history.replaceState(null, "", location.pathname);
    showError("");
  } catch (err) {
    out.textContent = "";
    showError(err.message);
  }
});


async function pollOnce() {
  if (!state.tv) return;
  // An off TV answers nothing; it is listed as off, not reported as an error.
  const sel = (state.tvs || []).find((x) => x.id === state.tv);
  if (sel && sel.off) { $("player").hidden = true; return; }
  try {
    const st = await api(`/api/status?renderer=${encodeURIComponent(state.tv)}`);
    state.status = st;
    if (state.tvError) { state.tvError = false; showError(""); }
    const active = st.ours && st.state !== "STOPPED" && st.state !== "NO_MEDIA_PRESENT";
    $("player").hidden = !active;
    // A film just ended or was stopped: its new resume point belongs in the row.
    if (state.wasPlaying && !active) loadMovies();
    state.wasPlaying = active;
    if (!active) return;
    $("p-state").textContent = t("st." + st.state) === "st." + st.state ? st.state : t("st." + st.state);
    $("p-title").textContent = st.title || "";
    $("p-toggle").textContent = st.state === "PLAYING" ? t("p.pause") : t("p.continue");
    if (!state.seeking) {
      seek.max = st.durationSeconds || 0;
      seek.value = st.positionSeconds || 0;
      $("p-pos").textContent = hms(st.positionSeconds);
    }
    $("p-dur").textContent = hms(st.durationSeconds);
    $("p-volrow").hidden = typeof st.volume !== "number";
    state.converted = st.converted === true;
    $("p-sound").textContent = state.converted ? t("p.original") : t("p.noSound");
    if (typeof st.volume === "number" && document.activeElement !== $("p-vol")) $("p-vol").value = st.volume;
  } catch (err) {
    // A TV that is switched off answers nothing; show it, keep polling.
    $("player").hidden = true;
    if (err.status !== 401) { state.tvError = true; showError(t("tv.err", err.message)); }
  }
}

function startPolling() {
  stopPolling();
  pollOnce();
  state.polling = setInterval(pollOnce, 2000);
}
function stopPolling() {
  if (state.polling) clearInterval(state.polling);
  state.polling = null;
}

document.addEventListener("visibilitychange", () => {
  if (document.hidden) stopPolling();
  else if (!$("app-view").hidden) startPolling();
});

boot();
