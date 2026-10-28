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

## Configuration (environment)

| Variable | Default | Meaning |
|---|---|---|
| `LISTEN` | `:8097` | listen address |
| `MOVIES_URL` | `http://127.0.0.1/movies` | Movies' Jellyfin-compatible API |
| `PUBLIC_URL` | derived per TV | URL the TV uses to reach this server |
| `RENDERERS` | — | comma-separated DLNA description URLs, in addition to SSDP |
| `SSDP` | `true` | `false` turns discovery off |

On .143, SSDP finds nothing: ZFW drops the TV's answers (default-deny INPUT; the LG answers from a
random source port, so neither a multicast nor a unicast query gets a reply through — 0 replies
measured on .143, replies on .250). There the TV is set with `RENDERERS=http://10.0.0.252:1795/`.
Whether the LG keeps port 1795 across reboots is not measured.

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
SSHPASS_FILE=/path/to/pw deploy/zimaos/build-on-host.sh <user>@<nas> 0.1.0
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

- **Pairing:** the TV shows a six-digit code; on the phone, "Fernseher-App koppeln" confirms it. The TV
  gets its own session that shares the phone's Movies login (phone logout keeps the TV logged in;
  only the last session logs out at Movies).
- **Library:** a "Weiterschauen" row (Movies' `Items/Resume`, its order) above "Alle Filme".
- **Subtitles:** text tracks from Movies' PlaybackInfo (only there; the item answer lists none), served
  as WebVTT through `/api/tv/subs/<play>/<index>.vtt` (own session's play only). Image subtitles (PGS)
  are left out. Measured on the TV: 1276 cues loaded, cue shown at 0:46. Movies' `DeliveryUrl`
  carries `?api_key=` and is never used. The last choice (a language or "off") is kept in the TV's
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
- **Chrome 53:** no async/await, `?.`, `??`, object spread, padStart, CSS grid, flex `gap`,
  `aspect-ratio`; `TestTVAssetsFitChrome53` fails the build on them.
- **Sessions survive restarts** with `STATE_FILE` (bind `/DATA/AppData/movies-cast/state`, dir owned
  by 65534, file 0600). Without it every update sent the TV back to pairing (seen on the TV).

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
