# movies-cast

Plays the films of IceWhale **Movies** (ZimaOS sysext `zimaos-movies`) on a DLNA TV,
controlled from a phone browser. The TV fetches the original file and decodes it
itself; the NAS does no transcoding.

```
phone ──HTTP──> movies-cast :8097 ──Jellyfin API──> Movies (127.0.0.1/movies, ZimaOS gateway)
                     │  SOAP (AVTransport)                 ▲
                     ▼                                     │ bytes, token in header
                    TV ──GET /s/<play-id>/movie.mkv──> movies-cast
```

## Why it exists (measured 2026-10-28, .143, Movies 0.3.0, LG OLED55B97LA)

- Movies imitates a Jellyfin server under `/movies` (`System/Info/Public` → `"ProductName":"Jellyfin Server"`),
  but serves no `/movies/web/` (404). The official Jellyfin app for LG webOS is a shell that loads
  `<server>/web/…` (`jellyfin-webos/frontend/js/index.js:264,331`), so it cannot use Movies.
- The LG is a DLNA MediaRenderer and plays HEVC + AC3/E-AC3 MKV directly (Movies log: one `GET 206`, no HLS).

## Use

The phone shows the same "Weiterschauen" row above all films; it reloads when a film on the TV ends
or is stopped, so the new resume point appears.

Open `http://<nas>:8097` on the phone — on .143: `http://10.0.0.143:8097` in the home Wi-Fi,
`http://100.64.0.10:8097` (or `http://my-nas:8097`) via Tailscale from outside; .143
advertises no subnet route, so the LAN address does not work over Tailscale. Log in with the **ZimaOS account**, pick a film, pick the TV,
press play. Pause, ±30 s, seek bar, stop and "continue at" work; progress goes back to Movies.

- **Manage TVs** (card on the phone): give a TV a name ("Living room"), hide one (a TV nobody uses),
  or add one by IP when the search cannot see it. Stored in `tvs.json` next to `STATE_FILE` (0600);
  an added IP is searched like a `RENDERERS` host. An empty list says why: no answer at all to the
  search (firewall?), answers but no video device, or every TV hidden.
- **Status** (card on the phone): version, Movies reachable (`/System/Info/Public`, no token), last
  search (time, answers, TVs), search port held, hidden/added TVs, proven formats, active plays.

## Configuration (environment)

| Variable | Default | Meaning |
|---|---|---|
| `LISTEN` | `:8097` | listen address |
| `MOVIES_URL` | `http://127.0.0.1/movies` | Movies' Jellyfin-compatible API |
| `PUBLIC_URL` | derived per TV | URL the TV uses to reach this server |
| `RENDERERS` | — | comma-separated DLNA description URLs **or hosts**, in addition to SSDP; a host gets its current URL by SSDP (multicast and unicast), the last one found stands in when the TV misses a search |
| `SSDP_PORT` | `1901` | local UDP port of the SSDP search, so a firewall can allow the answers (`0` = any port) |
| `SSDP` | `true` | `false` turns discovery off |

On .143, SSDP from a random port finds nothing: ZFW drops the TV's answers (default-deny INPUT; the LG
answers from a random source port, so neither a multicast nor a unicast query gets a reply through —
0 replies measured on .143, replies on .250). The LG does **not** keep its port either: 1795 earlier,
1626 after a later start (measured 2026-10-28), so a fixed URL breaks. Hence the fixed search port,
held open from start-up: ZFW (v1.0.28+) treats it as a new app port, opens it to the LAN and asks
once in its Exposure tab — answer *LAN only*. Measured on .143 with 0.16.0: port held, ZFW logged
"new app port … 1901 udp", a search found both TVs by host with 0 drops. (A socket open only for the
2 s of a search was opened by ZFW and closed again 19 s later; the answers fell in between.) Then
`RENDERERS` can name hosts (`10.0.0.252,10.0.0.86`) instead of URLs. The Samsung UE50MU6179 answers the multicast query
only, not a unicast one (measured), so it needs the multicast answer to come through as well.

## Format hint before sending

movies-cast sends the original file (converting only on request, see below). Before a DLNA send it reads the TV's
format list (`GetProtocolInfo`) and, if the film's container or HEVC is not in it, answers 409 with
the missing names; the phone shows the hint and a "Play anyway" button. It is a hint, not a block:
the LG OLED55B9 lists no HEVC and plays 4K HEVC; the 2012 Samsung UE32EH5300 lists none either; the
2017 Samsung UE50MU6179 lists `video/hevc` (lists captured in `internal/dlna/testdata`). Once a TV
reports PLAYING past 10 s, its TV/container/codec combination counts as proven and the hint is skipped;
with `STATE_FILE` set the proven list is kept in `formats.json` next to it (0600).

## Converted sound (on request)

For a TV that stays silent with a film's audio format, "No sound?" in the player restarts the film at
the same place with the sound converted; the choice is kept per TV and audio format in `convert.json`
next to `STATE_FILE` ("Original sound" undoes it). The same is offered when a DLNA send fails.

Measured on Movies 0.3.0: `/Videos/{id}/stream?static=false&AudioCodec=aac` returns the original file;
only `PlaybackInfo` converts, as HLS — 5 s MPEG-TS segments, the picture copied (HEVC 4K stays HEVC
4K), the audio AAC. movies-cast joins the segments into one MPEG-TS stream for the TV (`video/mp2t`,
or `video/mpeg` when the TV lists only that) with DLNA time seek (`TimeSeekRange.dlna.org`); a seek
from the phone re-sends the film from the segment that holds the new time. After a jump Movies
answers every segment with a 307 to the same path; redirects are followed only on the Movies server,
because the segment URLs carry the token (never logged).

The LG OLED55B9 lists no AC3, E-AC3, TrueHD or DTS but plays all of them with sound (the owner,
2026-10-28), so nothing is converted unless asked for. The Samsung UE50MU6179 lists AC3
(`audio/vnd.dolby.dd-raw`), no E-AC3, TrueHD or DTS.

## Switching a TV on (Wake-on-LAN)

Every TV seen is remembered in `tvs.json` with its MAC address (from the host's ARP table, read in
`/proc/net/arp` — the container runs on the host network) and its description URL. A TV that misses a
search is asked at that URL; if it does not answer, the phone lists it as "off". Sending a film to it
first sends a magic packet (UDP 9, limited and directed broadcast) every 3 s for up to 60 s, until
the TV's description answers. Whether a TV wakes this way depends on the TV (LG: "Mobile TV On").

## LG quirks the code handles (all measured)

- Seek works only when the DIDL carries `DLNA.ORG_OP=01`, `size` and `duration`; without them the TV
  answers 200 and keeps playing.
- Seek while STOPPED gets no answer at all → every SOAP call has a 6 s timeout.
- `GetVolume` answers UPnP 606 when nothing plays; during playback it works.
- After Stop the TV still hands the last URL to anyone asking `GetPositionInfo`.

## Security

- The TV never gets a Movies token. It gets `/s/<128-bit play id>/movie.<ext>`; this server forwards
  the bytes with the token in a header. The id dies on Stop, on a new play on the same TV, on logout,
  when the TV stops or is unreachable for 2 min, and after 24 h — and a running transfer is cut then.
- Tokens live only in memory; a restart logs everyone out.
- POSTs need same-origin (`Origin`, `Sec-Fetch-Site`) and a JSON body. 5 wrong passwords per client
  IP in 10 min → 429. At most 200 sessions.
- SSDP answers are trusted only if the sender is the host in `LOCATION`, the address is private LAN
  and not this machine, and the control URLs stay on that host. No HTTP client follows redirects
  (Go does not strip `X-Emby-Authorization` on a cross-host redirect).
- **Known limit:** cookies are not bound to a port. Every other app on the same NAS address receives
  the `mc_s` cookie. Without TLS there is no `__Host-` protection. A malicious app on the same NAS
  could take over a session.

## Build, test, deploy

```bash
go vet ./... && go test -race ./...
SSHPASS_FILE=/path/to/pw deploy/zimaos/build-on-host.sh <user>@<nas> 0.20.2
# builds only a committed tree; -version then prints 0.20.2+<commit>
# first install:
ZIMA_USER=… ZIMA_PASS=… python3 ~/dev/zimapp/zimapp.py install --host <nas> --ssh-user <user> deploy/zimaos/docker-compose.yml
# update after a rebuild, on the host:
cd /var/lib/casaos/apps/movies-cast && sudo docker compose up -d --force-recreate movies-cast
```

ZimaOS rewrites the compose on install (environment as a map, `TZ: $TZ` left literal). The installed
file on .143 was edited by hand (`RENDERERS`, `TZ: Europe/Berlin`); backups under
`/DATA/AppData/ssh-sudo-backups/movies-cast-compose-*.yml`. A reinstall via ZimaOS overwrites it.

## TV app (webOS, stage 2)

`deploy/webos/app` is a hosted web app (title "Movies"): it checks `<SERVER_URL>/healthz` and then
opens `/tv/` on movies-cast. The UI is one code base
with the server; the TV gets no Movies token.

- **Pairing:** the TV shows a six-digit code and a QR code (`/api/tv/pair/qr?id=<pairing id>`, only
  for an open pairing, drawn server-side with `rsc.io/qr`). The QR code holds
  `http://<host the TV used>/?pair=<code>`: the phone camera opens the page with the code filled in and
  the pairing card open — pairing still takes a tap on "Koppeln", so a code from another TV is never
  approved unasked. Typing the code works as before. The TV
  gets its own session that shares the phone's Movies login (phone logout keeps the TV logged in;
  only the last session logs out at Movies).
- **Library:** a "Weiterschauen" row (Movies' `Items/Resume`, its order) above "Alle Filme".
- **Subtitles:** text tracks from the item answer with `Fields=MediaStreams`, served as WebVTT through
  `/api/tv/subs/<play>/<index>.vtt` (own session's play only). Image subtitles (PGS) are left out.
  Measured on the TV: 1276 cues loaded, cue shown at 0:46. **PlaybackInfo only on request:** Movies answers every
  PlaybackInfo that lists the streams by starting a transcode on the box (log `[streamMedia] movie/1
  transcode codec=h264 codec_decision=client_hevc_not_supported_or_profile_limited`, ffmpeg with VAAPI
  for the first segment), although the TV plays the original file; flags or a device profile in the
  request only gave `NoCompatibleStream` or still a transcode (measured 2026-10-28). Embedded subtitles
  are extracted by Movies on first use (30 s for one track of a 37 GiB MKV).
  **On request:** for 3 of the 4 films on .143 the item answer listed no subtitle and no video width (Shelter: none,
  while PlaybackInfo lists en and fr). Such a play answers `subtitlesOnRequest`; the TV asks `/api/tv/subs-probe`
  only when someone wants subtitles (▼, or a remembered language), and the server then asks PlaybackInfo once per
  film and keeps the answer. Measured on the LG: Shelter starts without PlaybackInfo, ▼ loads en/fr (1276 cues).
  Tracks are attached only once the picture runs: with a remembered language the LG held the start until
  that track had loaded (Avatar: canplay at 30.1 s, Movies needs 30 s per embedded track; 2.1 s since 0.13).
  While the bar at the bottom shows, cues move up above it (line −(4 + rows), measured on the LG: one row −5,
  two rows −6). The last choice (a language or "off") is kept in the TV's
  `localStorage` and preselected for the next film; it survives an app restart (measured). Exactly
  one track shows: preselecting before load left both "showing" on the TV, so the choice is enforced
  again after load and on every track-list change.
- **Audio track:** the last language chosen with ▲ (`moviescast.audioLang` in `localStorage`) is enabled
  at the next film's start; a film without that language keeps its own first track. Measured on the TV:
  Shelter de→en, then Shelter and Avatar start in en, also after an app restart.
- **Remote:** arrows move, OK opens/plays; in the player OK = pause, ◀ −10 s, ▶ +30 s, ▲ audio track, ▼ subtitles (off → each track → off),
  Back (461) or Stop = end and back to the library; Play/Pause/FF/RW keys work too. Progress goes to
  Movies, so "continue at" is the same on phone and TV.
- **Measured on the LG** (Chrome 53 engine, `ares-inspect` + DevTools protocol, 2026-10-28): `<video>`
  plays the original MKV (HEVC 4K, AC3/E-AC3) and seeks in < 1 s; `audioTracks` switches language;
  `canPlayType("video/x-matroska")` answers "" anyway, so the UI never asks it. The full flow
  (pair → library → detail → play → +30 s → audio → pause → Back) ran on the TV.
- **Languages:** the 18 of the ZimaOS UI, English the default (`internal/server/web/i18n/<lang>.json`, served
  as `/strings.js` to both pages). `?lang=xx` (remembered), else the remembered one, else the device language,
  else English; a picker on the phone. Server errors carry a `key` for every language;
  `TestEveryFailMessageHasEnglish` and `TestStringsComplete` keep them complete. Review status:
  [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md) — no native speaker has read them yet.
- **Phone home screen:** `manifest.json` with 192/512 px icons and an `apple-touch-icon` (rendered from the logo by
  `deploy/webos/logo/render.py`; `TestManifestIconsExist`).
- **Chrome 53:** no async/await, `?.`, `??`, object spread, padStart, CSS grid, flex `gap`,
  `aspect-ratio`; `TestTVAssetsFitChrome53` fails the build on them.
- **Sessions survive restarts** with `STATE_FILE` (bind `/DATA/AppData/movies-cast/state`, dir owned
  by 65534, file 0600). Without it every update sent the TV back to pairing (seen on the TV).

**Testing on a real library:** a test playback is a real playback; each stop overwrites the user's resume point on
every device. `deploy/tools/resume-guard.py save|restore <file>` (env `MOVIES_URL`, `MOVIES_USER`,
`MOVIES_PASSWORD_FILE`) saves all positions before and writes them back after a test, exit 1 if Movies does not
answer with the saved state.

Install / update on the TV (developer mode, device `lgtv` in ares):

```bash
deploy/webos/package.sh        # reads deploy/webos/local.env: APP_ID, VENDOR, SERVER_URL
ares-install -d lgtv deploy/webos/out/<APP_ID>_0.1.0_all.ipk
ares-launch -d lgtv <APP_ID>
```

`deploy/webos/app` carries neutral defaults (`io.github.moviescast`, `http://nas.local:8097`); the
values of one installation stay in the git-ignored `local.env`. Keep `APP_ID` once the app is
installed: another id installs a second app with its own storage (pairing, track choices).

**Logo:** `deploy/webos/logo/logo.svg` (a TV whose screen is a film frame, colours of the TV UI) is the
source. `python3 deploy/webos/logo/render.py` (Playwright/Chromium) writes the app icons `icon.png`
(80 px), `largeIcon.png` (130 px) and `logo-512.png`. `internal/server/web/logo.svg` is a copy for the
phone and TV pages (header, pairing screen, favicon); `TestLogoServedIsTheIconSource` fails when the
two differ.

Device setup pitfalls (measured): log in to the Developer Mode app with the **e-mail** ID; after
`ares-novacom --getkey` set `ares-setup-device -m lgtv -i privateKey=lgtv_webos -i passphrase=…`
(ares had stored no key); the TV offers only `ssh-rsa` (OpenSSH: `-o HostKeyAlgorithms=+ssh-rsa
-o PubkeyAcceptedAlgorithms=+ssh-rsa`).

**Developer-mode session:** about 1000 h (Remain Session 999h:58m after a renewal). Apps installed in
developer mode are removed after a reboot past expiry, and an expired session cannot be extended
(LG docs). `deploy/webos/renew-devmode.sh` renews it daily from .143
(`movies-cast-devmode.timer`, data in `/DATA/AppData/movies-cast-devmode`); check with
`journalctl -u movies-cast-devmode.service -n 3` → `renewed: result=success`.
