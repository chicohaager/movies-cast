"use strict";

const $ = (id) => document.getElementById(id);
const state = { movies: [], tv: localGet("tv"), selected: null, polling: null, status: null, seeking: false };

function localGet(k) { try { return localStorage.getItem(k) || ""; } catch { return ""; } }
function localSet(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } }

function showError(msg) {
  const e = $("error");
  e.textContent = msg;
  e.hidden = !msg;
}

async function api(path, body) {
  const opt = body === undefined ? {} : {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  };
  let res;
  try {
    res = await fetch(path, { credentials: "same-origin", ...opt });
  } catch (err) {
    throw new Error("Server nicht erreichbar");
  }
  let data = null;
  try { data = await res.json(); } catch { /* no body */ }
  if (res.status === 401 && path !== "/api/login") {
    showView("login");
    throw new Error((data && data.error) || "Bitte neu anmelden");
  }
  if (!res.ok) throw new Error((data && data.error) || `Fehler ${res.status}`);
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
  await Promise.all([loadMovies(), loadTVs(false)]);
  startPolling();
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
      sel.append(new Option("Kein Fernseher gefunden", ""));
      showError("Kein Fernseher gefunden. Ist er eingeschaltet und im selben Netz?");
      return;
    }
    for (const tv of tvs) sel.append(new Option(`${tv.name} (${tv.host})`, tv.id));
    if (tvs.some((t) => t.id === state.tv)) sel.value = state.tv;
    state.tv = sel.value;
    localSet("tv", state.tv);
  } catch (err) {
    showError(err.message);
  }
}

$("tv").addEventListener("change", (ev) => {
  state.tv = ev.target.value;
  localSet("tv", state.tv);
  pollOnce();
});
$("refresh").addEventListener("click", () => loadTVs(true));

async function loadMovies() {
  try {
    state.movies = await api("/api/movies");
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
  const list = $("list");
  list.innerHTML = "";
  $("empty").hidden = state.movies.length > 0;
  for (const m of state.movies) {
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
  r.textContent = `Fortsetzen bei ${hms(m.resumeSeconds)}`;
  $("detail").showModal();
}

$("detail").addEventListener("close", async () => {
  const v = $("detail").returnValue;
  if (v !== "resume" && v !== "start") return;
  if (!state.tv) {
    showError("Erst einen Fernseher wählen.");
    return;
  }
  showError("");
  try {
    await api("/api/play", { renderer: state.tv, movie: state.selected.id, resume: v === "resume" });
    pollOnce();
  } catch (err) {
    showError(err.message);
  }
});

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
    out.textContent = "Bitte die 6 Ziffern vom Fernseher eingeben.";
    return;
  }
  try {
    await api("/api/tv/pair/approve", { code });
    out.textContent = "Gekoppelt – der Fernseher zeigt gleich die Filme.";
    $("pair-code").value = "";
    showError("");
  } catch (err) {
    out.textContent = "";
    showError(err.message);
  }
});

const stateNames = {
  PLAYING: "läuft", PAUSED_PLAYBACK: "Pause", STOPPED: "gestoppt",
  TRANSITIONING: "lädt …", NO_MEDIA_PRESENT: "bereit",
};

async function pollOnce() {
  if (!state.tv) return;
  try {
    const st = await api(`/api/status?renderer=${encodeURIComponent(state.tv)}`);
    state.status = st;
    if ($("error").textContent.startsWith("Fernseher:")) showError("");
    const active = st.ours && st.state !== "STOPPED" && st.state !== "NO_MEDIA_PRESENT";
    $("player").hidden = !active;
    if (!active) return;
    $("p-state").textContent = stateNames[st.state] || st.state;
    $("p-title").textContent = st.title || "";
    $("p-toggle").textContent = st.state === "PLAYING" ? "Pause" : "Weiter";
    if (!state.seeking) {
      seek.max = st.durationSeconds || 0;
      seek.value = st.positionSeconds || 0;
      $("p-pos").textContent = hms(st.positionSeconds);
    }
    $("p-dur").textContent = hms(st.durationSeconds);
    $("p-volrow").hidden = typeof st.volume !== "number";
    if (typeof st.volume === "number" && document.activeElement !== $("p-vol")) $("p-vol").value = st.volume;
  } catch (err) {
    // A TV that is switched off answers nothing; show it, keep polling.
    $("player").hidden = true;
    if (!/anmelden/i.test(err.message)) showError(`Fernseher: ${err.message}`);
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
