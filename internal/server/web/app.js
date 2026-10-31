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
// Colour mode picker: System, Light or Dark. The account's choice (/api/me,
// POST /api/theme) wins; localStorage only lets theme.js paint the first frame
// in it — on its own it was one choice per address (LAN, Tailscale).
const theme = (function () {
  const sel = document.getElementById("theme");
  const meta = document.querySelector('meta[name="theme-color"]');
  let signedIn = false;
  const apply = (mode) => {
    sel.value = mode;
    if (mode) document.documentElement.setAttribute("data-theme", mode);
    else document.documentElement.removeAttribute("data-theme");
    try { if (mode) localStorage.setItem("moviescast.theme", mode); else localStorage.removeItem("moviescast.theme"); } catch { /* private mode */ }
    if (meta) meta.content = getComputedStyle(document.documentElement).getPropertyValue("--bg").trim() || meta.content;
  };
  apply(document.documentElement.getAttribute("data-theme") || "");
  sel.addEventListener("change", () => {
    apply(sel.value);
    if (signedIn) api("/api/theme", { theme: sel.value }).catch((err) => showError(err.message));
  });
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => apply(sel.value));
  return {
    // fromAccount takes the mode stored with the account; an account without
    // one adopts what this device shows, unless that is the system default.
    fromAccount(me) {
      signedIn = true;
      if (typeof me.theme === "string") apply(me.theme);
      else if (sel.value) api("/api/theme", { theme: sel.value }).catch((err) => showError(err.message));
    },
    signedOut() { signedIn = false; },
    // forget clears this device's copy on "Log out": the next person on a
    // shared phone would otherwise hand it to an account without a choice.
    forget() { signedIn = false; apply(""); },
  };
})();
const state = { movies: [], resume: [], wasPlaying: false, tv: localGet("tv"), selected: null, polling: null, pollBusy: false, waking: false, status: null, seeking: false };

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
  if (which !== "app") { stopPolling(); theme.signedOut(); }
}

// Language: the account's choice wins like the colour mode, except over a
// language asked for in the link (the picker reloads with ?lang=), which is
// the newest choice and is stored with the account.
function langFromAccount(me) {
  const store = () => api("/api/lang", { lang: I18N.lang }).catch((err) => showError(err.message));
  if (I18N.asked) {
    if (me.lang !== I18N.lang) store();
  } else if (typeof me.lang === "string") {
    if (me.lang !== I18N.lang) {
      const u = new URL(location.href);
      u.searchParams.set("lang", me.lang);
      location.replace(u.toString());
    }
  } else if (I18N.chosen) {
    store();
  }
}

// The login hint names the account: ZimaOS for Movies, else the Jellyfin
// or Emby server movies-cast reads from (/api/source, roadmap 20).
fetch("/api/source").then((r) => r.json()).then((src) => {
  if (!src.zimaos) $("login-hint").textContent = t("login.hintServer", src.product || "Jellyfin");
}).catch(() => { /* the ZimaOS hint stays */ });

async function boot() {
  try {
    const me = await api("/api/me");
    theme.fromAccount(me);
    langFromAccount(me);
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
  theme.forget();
  // The next person on a shared phone must not hand this language to an
  // account without one: drop the remembered one and ?lang= in the address.
  I18N.forget();
  const u = new URL(location.href);
  if (u.searchParams.has("lang")) {
    u.searchParams.delete("lang");
    history.replaceState(null, "", u.toString());
  }
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
    const want = state.tv || localGet("tv");
    if (tvs.some((t) => t.id === want)) sel.value = want;
    state.tv = sel.value;
    localSet("tv", state.tv);
  } catch (err) {
    // No TV listed (the server answers 502): the one chosen before must not
    // get the next film. The stored choice stays for when the list is back.
    sel.innerHTML = "";
    sel.append(new Option(t("tv.none"), ""));
    state.tv = "";
    state.tvs = [];
    state.status = null;
    $("player").hidden = true;
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

// The library: films (and series) always; series, music and photos as tabs
// when the server has them (/api/views; Movies 0.3.0 has films only).
// Search, sort and filter run on the phone over the whole list (Movies
// ignores IsPlayed and Genres as filters, measured 2026-10-31); at most
// PAGE tiles are drawn at a time, "Show more" adds the next PAGE.
const PAGE = 120;
const lib = { kind: "films", views: {}, lists: {}, shown: PAGE };

async function loadMovies() {
  try {
    const [movies, resume, views] = await Promise.all([
      api("/api/movies"),
      api("/api/resume").catch(() => []), // the row is extra; the list must not depend on it
      api("/api/views").catch(() => ({})),
    ]);
    state.movies = movies;
    state.resume = resume || [];
    lib.views = views || {};
    lib.lists.films = movies.filter((m) => m.type !== "Series");
    lib.lists.series = movies.filter((m) => m.type === "Series");
    setupTabs();
    renderMovies();
  } catch (err) {
    showError(err.message);
  }
}

function setupTabs() {
  const has = { films: true, series: lib.lists.series.length > 0, music: !!lib.views.music, photos: !!lib.views.photos };
  let shown = 0;
  for (const b of document.querySelectorAll("#tabs [data-kind]")) {
    b.hidden = !has[b.dataset.kind];
    if (!b.hidden) shown++;
  }
  $("tabs").hidden = shown < 2;
  if (!has[lib.kind]) lib.kind = "films";
  for (const b of document.querySelectorAll("#tabs [data-kind]")) b.setAttribute("aria-selected", String(b.dataset.kind === lib.kind));
}

for (const b of document.querySelectorAll("#tabs [data-kind]")) {
  b.addEventListener("click", async () => {
    lib.kind = b.dataset.kind;
    lib.shown = PAGE;
    for (const x of document.querySelectorAll("#tabs [data-kind]")) x.setAttribute("aria-selected", String(x === b));
    if ((lib.kind === "music" || lib.kind === "photos") && !lib.lists[lib.kind]) {
      try { lib.lists[lib.kind] = await api("/api/library?kind=" + lib.kind); } catch (err) { showError(err.message); lib.lists[lib.kind] = []; }
    }
    renderMovies();
  });
}
for (const id of ["q", "sort", "filter"]) {
  $(id).addEventListener("input", () => { lib.shown = PAGE; renderMovies(); });
}
$("more").addEventListener("click", () => { lib.shown += PAGE; renderMovies(); });

// genreOptions refills the filter's genres from the current list, keeping the choice.
function genreOptions(list) {
  const sel = $("filter"), keep = sel.value;
  for (const o of [...sel.options]) if (o.value.startsWith("g:")) o.remove();
  const genres = [...new Set(list.flatMap((m) => m.genres || []))].sort((a, b) => a.localeCompare(b));
  for (const g of genres) sel.append(new Option(t("f.genre", g), "g:" + g));
  sel.value = [...sel.options].some((o) => o.value === keep) ? keep : "";
}

function filtered(list) {
  const q = $("q").value.trim().toLowerCase();
  const f = $("filter").value;
  let out = list.filter((m) => {
    if (q && !(`${m.name} ${m.seriesName || ""} ${m.year || ""}`.toLowerCase().includes(q))) return false;
    if (f === "unwatched" && m.played) return false;
    if (f === "started" && !(m.resumeSeconds > 0)) return false;
    if (f.startsWith("g:") && !(m.genres || []).includes(f.slice(2))) return false;
    return true;
  });
  const by = $("sort").value;
  const name = (a, b) => a.name.localeCompare(b.name, I18N.lang.replace("_", "-"), { numeric: true });
  const cmp = {
    name,
    added: (a, b) => (b.added || "").localeCompare(a.added || "") || name(a, b),
    year: (a, b) => (b.year || 0) - (a.year || 0) || name(a, b),
    score: (a, b) => (b.score || 0) - (a.score || 0) || name(a, b),
  }[by] || name;
  return out.slice().sort(cmp);
}

function renderMovies() {
  const all = lib.lists[lib.kind] || [];
  genreOptions(all);
  const list = filtered(all);
  const searching = $("q").value.trim() !== "" || $("filter").value !== "";
  $("resume-wrap").hidden = state.resume.length === 0 || lib.kind !== "films" || searching;
  renderTiles($("resume-list"), state.resume);
  renderTiles($("list"), list.slice(0, lib.shown));
  $("count").textContent = all.length ? t("f.count", list.length, all.length) : "";
  $("more-row").hidden = list.length <= lib.shown;
  $("more").textContent = t("f.more", list.length - lib.shown);
  $("empty").hidden = list.length > 0;
  $("empty").textContent = all.length ? t("f.none") : t("empty");
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

function epLabel(m) {
  const two = (n) => String(n || 0).padStart(2, "0");
  return m.type === "Episode" ? `S${two(m.season)}E${two(m.episode)}` : (m.type === "Audio" && m.episode ? String(m.episode) : "");
}

function metaLine(m) {
  const parts = [];
  if (m.type === "Series") parts.push(t("d.nEpisodes", m.count || 0));
  if (m.type === "MusicAlbum") parts.push(t("d.nTracks", m.count || 0));
  if (m.type === "Episode") parts.push(`${m.seriesName || ""} ${epLabel(m)}`.trim());
  if (m.year) parts.push(m.year);
  if (m.runtimeSeconds && m.type !== "Audio") parts.push(`${Math.round(m.runtimeSeconds / 60)} min`);
  if (m.runtimeSeconds && m.type === "Audio") parts.push(hms(m.runtimeSeconds));
  if (m.rating) parts.push(m.rating);
  if (m.width >= 3800) parts.push("4K");
  if (m.videoCodec) parts.push(m.videoCodec.toUpperCase());
  if (m.audio) parts.push(m.audio.toUpperCase());
  if (m.genres && m.genres.length) parts.push(m.genres.slice(0, 3).join(", "));
  return parts.join(" · ");
}

function renderTiles(list, movies) {
  list.innerHTML = "";
  for (const m of movies) {
    const b = document.createElement("button");
    b.className = "tile";
    b.type = "button";
    b.append(posterImg(m));
    if (m.resumeSeconds > 0 && m.runtimeSeconds > 0) {
      const bar = document.createElement("div");
      bar.className = "bar";
      bar.setAttribute("role", "progressbar");
      bar.setAttribute("aria-label", t("d.watched"));
      const pct = Math.min(100, Math.round((100 * m.resumeSeconds) / m.runtimeSeconds));
      bar.setAttribute("aria-valuenow", String(pct));
      const i = document.createElement("i");
      i.style.width = `${pct}%`;
      bar.append(i);
      b.append(bar);
    }
    const cap = document.createElement("div");
    cap.className = "cap";
    cap.textContent = m.name;
    const small = document.createElement("small");
    small.textContent = m.type === "Series" ? t("d.nEpisodes", m.count || 0) : (m.year || "");
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
  const folder = m.type === "Series" || m.type === "MusicAlbum";
  const r = $("d-resume");
  r.hidden = folder || !(m.resumeSeconds > 0);
  r.textContent = t("d.resume", hms(m.resumeSeconds));
  $("d-start").hidden = folder;
  $("d-start").textContent = m.type === "Photo" ? t("d.show") : t("d.start");
  $("d-queue").hidden = folder || m.type === "Photo";
  $("d-children-wrap").hidden = !folder;
  loadSubtitles(m);
  $("d-children").textContent = "";
  if (folder) loadChildren(m);
  $("detail").showModal();
}

// loadSubtitles fills the subtitle choice of a film or episode (roadmap 16):
// the TV gets the chosen track as SRT. The last language chosen is
// preselected (this device only).
async function loadSubtitles(m) {
  const row = $("d-sub-row"), sel = $("d-sub");
  row.hidden = true;
  for (const o of [...sel.options]) if (o.value !== "") o.remove();
  sel.value = "";
  if (!(m.type === "Movie" || m.type === "Episode" || !m.type)) return;
  try {
    const d = await api(`/api/subtitles/${encodeURIComponent(m.id)}`);
    if (state.selected !== m || !d.subtitles.length) return;
    const want = localGet("subLang");
    for (const st of d.subtitles) {
      sel.append(new Option(st.label, String(st.index)));
      if (want && st.lang === want && sel.value === "") sel.value = String(st.index);
    }
    state.subs = d.subtitles;
    row.hidden = false;
  } catch { /* the film plays without */ }
}
$("d-sub").addEventListener("change", () => {
  const st = (state.subs || []).find((x) => String(x.index) === $("d-sub").value);
  localSet("subLang", st ? st.lang || "" : "");
});

// loadChildren lists a series' episodes or an album's tracks; each one is a
// button that closes the dialog with "ep:<id>". The first unwatched episode
// is marked "Continue with …" and gets the focus.
async function loadChildren(m) {
  $("d-children-h").textContent = m.type === "Series" ? t("d.episodes") : t("d.tracks");
  try {
    const kids = await api(`/api/children/${encodeURIComponent(m.id)}`);
    if (state.selected !== m) return;
    state.children = kids;
    const next = kids.find((k) => k.resumeSeconds > 0) || kids.find((k) => !k.played);
    const box = $("d-children");
    box.textContent = "";
    for (const k of kids) {
      const b = document.createElement("button");
      b.value = "ep:" + k.id;
      b.textContent = `${epLabel(k)} ${k.name}`.trim();
      const small = document.createElement("small");
      const bits = [];
      if (k === next) bits.push(t("d.nextUp"));
      if (k.played) bits.push(t("d.seen"));
      if (k.resumeSeconds > 0) bits.push(t("d.resume", hms(k.resumeSeconds)));
      else if (k.runtimeSeconds) bits.push(hms(k.runtimeSeconds));
      small.textContent = bits.join(" · ");
      b.append(small);
      box.append(b);
      if (k === next) setTimeout(() => b.focus(), 0);
    }
  } catch (err) {
    showError(err.message);
  }
}

$("detail").addEventListener("close", async () => {
  const v = $("detail").returnValue;
  let m = state.selected, resume = v === "resume";
  if (v && v.startsWith("ep:")) {
    m = (state.children || []).find((k) => "ep:" + k.id === v);
    if (!m) return;
    resume = m.resumeSeconds > 0;
  } else if (v !== "resume" && v !== "start" && v !== "queue") {
    return;
  }
  if (!state.tv) {
    showError(t("play.chooseTv"));
    return;
  }
  showError("");
  if (v === "queue") {
    try {
      await api("/api/queue", { renderer: state.tv, movie: m.id, action: "add" });
      showNotice(t("q.added", m.name));
      loadQueue();
    } catch (err) {
      showError(err.message);
    }
    return;
  }
  const req = { renderer: state.tv, movie: m.id, resume };
  if (m === state.selected && $("d-sub").value !== "" && !$("d-sub-row").hidden) req.sub = Number($("d-sub").value);
  play(req);
});

// The queue of the chosen TV, shown in the player card.
async function loadQueue() {
  if (!state.tv) return;
  try {
    const q = await api(`/api/queue?renderer=${encodeURIComponent(state.tv)}`);
    const ol = $("q-list");
    ol.textContent = "";
    for (const it of q.items) {
      const li = document.createElement("li");
      const rm = document.createElement("button");
      rm.type = "button";
      rm.className = "ghost";
      rm.textContent = t("q.remove");
      rm.setAttribute("aria-label", t("q.removeName", it.name));
      rm.addEventListener("click", async () => {
        try { await api("/api/queue", { renderer: state.tv, movie: it.id, action: "remove" }); loadQueue(); } catch (err) { showError(err.message); }
      });
      li.append(it.name, rm);
      ol.append(li);
    }
    $("q-auto").checked = q.autoplay !== false;
    state.queueLen = q.items.length;
    $("q-wrap").hidden = $("player").hidden;
  } catch (err) {
    showError(err.message);
  }
}
$("q-auto").addEventListener("change", async (ev) => {
  try { await api("/api/queue", { renderer: state.tv, action: ev.target.checked ? "auto" : "noauto" }); } catch (err) { showError(err.message); }
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
  state.waking = true;
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
  } finally {
    state.waking = false;
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
  // One request at a time: against a slow TV the server takes up to 12 s,
  // and answers piling up came back out of order.
  if (state.pollBusy) return;
  // An off TV answers nothing; it is listed as off, not reported as an error.
  const sel = (state.tvs || []).find((x) => x.id === state.tv);
  if (sel && sel.off) { $("player").hidden = true; return; }
  const tv = state.tv;
  state.pollBusy = true;
  try {
    const st = await api(`/api/status?renderer=${encodeURIComponent(tv)}`);
    if (tv !== state.tv) return; // another TV was chosen meanwhile
    state.status = st;
    if (state.tvError) { state.tvError = false; showError(""); }
    const active = st.ours && st.state !== "STOPPED" && st.state !== "NO_MEDIA_PRESENT";
    $("player").hidden = !active;
    // A film just ended or was stopped: its new resume point belongs in the row.
    if (state.wasPlaying && !active) loadMovies();
    if (active && (!state.wasPlaying || st.movie !== state.lastMovie)) loadQueue();
    state.lastMovie = st.movie;
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
    $("p-refused").hidden = !st.refusedFrom;
    $("p-refused").textContent = st.refusedFrom ? t("p.refused", st.refusedFrom, st.streamTo) : "";
    state.converted = st.converted === true;
    $("p-sound").textContent = state.converted ? t("p.original") : t("p.noSound");
    if (typeof st.volume === "number" && document.activeElement !== $("p-vol")) $("p-vol").value = st.volume;
  } catch (err) {
    if (tv !== state.tv) return;
    // A TV that is switched off answers nothing; show it, keep polling.
    $("player").hidden = true;
    // Not over a refusal's buttons or the wake notice: they say what to do next.
    if (err.status !== 401 && $("force-row").hidden && !state.waking) {
      state.tvError = true;
      showError(t("tv.err", err.message));
    }
  } finally {
    state.pollBusy = false;
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
