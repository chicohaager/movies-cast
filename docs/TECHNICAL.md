# Movies-Cast: how it works and what was measured

The measurement log behind the [README](../README.md). Statements carry the date and device they were
measured on; "not measured" says so.

## Why it exists (measured 2026-10-28, the test NAS, Movies 0.3.0, LG OLED55B97LA)

- Movies imitates a Jellyfin server under `/movies` (`System/Info/Public` → `"ProductName":"Jellyfin Server"`),
  but serves no `/movies/web/` (404). The official Jellyfin app for LG webOS is a shell that loads
  `<server>/web/…` (`jellyfin-webos/frontend/js/index.js:264,331`), so it cannot use Movies.
- The LG is a DLNA MediaRenderer and plays HEVC + AC3/E-AC3 MKV directly (Movies log: one `GET 206`, no HLS).

## Search port and firewall

On the test NAS, SSDP from a random port finds nothing: ZFW drops the TV's answers (default-deny INPUT; the LG
answers from a random source port, so neither a multicast nor a unicast query gets a reply through —
0 replies measured on the test NAS, replies on the development PC). The LG does **not** keep its port either: 1795 earlier,
1626 after a later start (measured 2026-10-28), so a fixed URL breaks. Hence the fixed search port,
held open from start-up: ZFW (v1.0.28+) treats it as a new app port, opens it to the LAN and asks
once in its Exposure tab — answer *LAN only*. Measured on the test NAS with 0.16.0: port held, ZFW logged
"new app port … 1901 udp", a search found both TVs by host with 0 drops. (A socket open only for the
2 s of a search was opened by ZFW and closed again 19 s later; the answers fell in between.) Then
`RENDERERS` can name hosts (`10.0.0.20,10.0.0.30`) instead of URLs. The Samsung UE50MU6179 answers the multicast query
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
  The TV hands that URL to anyone who asks it over UPnP, so the first address that fetches a play
  keeps it; another host with the same URL gets 403, and the player on the phone names both
  addresses (a TV that fetches from two addresses would otherwise just stay black).
- Tokens live only in memory; a restart logs everyone out.
- POSTs need same-origin (`Origin`, `Sec-Fetch-Site`) and a JSON body. 5 wrong passwords per client
  IP in 10 min → 429. At most 200 sessions.
- SSDP answers are trusted only if the sender is the host in `LOCATION`, the address is private LAN
  and not this machine, and the control URLs stay on that host. No HTTP client follows redirects
  (Go does not strip `X-Emby-Authorization` on a cross-host redirect).
- **HTTPS (since 0.27):** with `TLS_LISTEN` the phone page is also served over TLS and the session cookie
  is `__Host-mc_s` with `Secure` — bound to that origin, so no other app on the same NAS address receives
  it. Over plain HTTP the cookie stays `mc_s`, and the old limit stays: cookies are not bound to a port,
  every other app on the same NAS address receives it, and a malicious app there could take over a session.

## Build, test, deploy

`./check.sh` runs everything that must pass: gofmt + vet, the Go tests, the browser tests (`e2e/`) and the
public-data scan; `build-on-host.sh` and the testers' package run it first (`CHECK_ARGS=--no-e2e` without Node).

### Package for testers

```bash
deploy/testers/make-package.sh 0.27.0 --push   # image to ghcr.io/chicohaager/movies-cast:0.27.0 (public)
# -> deploy/testers/out/movies-cast-0.27.0.zip: docker-compose.yml (ZimaOS "Import"), ANLEITUNG.de.html,
#    GUIDE.en.html, GUIDE.fr.html (install, test list of 16, report template, removal), SHA256SUMS
```

Testers import the compose in the ZimaOS dashboard; ZimaOS pulls the image itself, which is why it lives in a
public registry (a `docker load`ed image is not used by the installer since ZimaOS 1.7.1). The tile shows the
movies-cast logo as a `data:` URI in the compose (`TestComposeIconIsTheLogo`). The webOS TV app is not in the package; the Android TV APK is attached to the release.


```bash
go vet ./... && go test -race ./...
# the phone page in a browser with a login, against the fake Movies and fake TV of the tests; the TV
# goes from TRANSITIONING to PLAYING after a second and counts the position up
# (log in as Demo or Anna, password "richtig"; Ctrl-C stops it):
MC_DEV_SERVER=127.0.0.1:18097 go test -count=1 ./internal/server/ -run TestDevServer -timeout 0 -v
# browser tests of the phone page (Playwright starts and stops the dev server on port 18098):
# login, colour mode and language per account across two "devices", the player
cd e2e && npm ci && npx playwright install chromium && npx playwright test
SSHPASS_FILE=/path/to/pw deploy/zimaos/build-on-host.sh <user>@<nas> 0.22.0
# builds only a committed tree; -version then prints 0.22.0+<commit>
# first install:
ZIMA_USER=… ZIMA_PASS=… python3 ~/dev/zimapp/zimapp.py install --host <nas> --ssh-user <user> deploy/zimaos/docker-compose.yml
# test a build on an install from the store (image ghcr.io/…): --switch points the installed compose
# at the local image (backup in /DATA/AppData/movies-cast-build/compose-backups, proof that only the
# image line changed; deploy/zimaos/switch-image.sh), recreates the container and checks -version in it
SSHPASS_FILE=/path/to/pw deploy/zimaos/build-on-host.sh <user>@<nas> 0.27.0 --switch
# update after a rebuild when the compose already names the local image, on the host:
cd /var/lib/casaos/apps/movies-cast && sudo docker compose up -d --force-recreate movies-cast
```

ZimaOS rewrites the compose on install (environment as a map). Since 0.25.1 the compose sets no `TZ`
and binds the host's `/etc/localtime` read-only: ZimaOS fills `$TZ` in on the first import only, and
after an update in "Verwalten" the program ran in UTC (`TestComposeTimeZoneFromHost`). The installed
file on the test NAS was edited by hand (`RENDERERS`, `TZ: Europe/Berlin`); backups under
`/DATA/AppData/ssh-sudo-backups/movies-cast-compose-*.yml`. A reinstall via ZimaOS overwrites it.

## TV app (webOS, stage 2)

`deploy/webos/app` is a hosted web app (title "Movies-Cast"): it checks `<SERVER_URL>/healthz` and then
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
  **On request:** for 3 of the 4 films on the test NAS the item answer listed no subtitle and no video width (Shelter: none,
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
  else English; a picker on the phone. On the phone the choice is stored with the account like the colour mode
  (`langs.json`, `POST /api/lang`, only languages under `web/i18n`): a language from the link or the picker is
  stored, otherwise the account's language is loaded; the device language is never stored, and "Log out"
  forgets the device's copy and `?lang=`. Server errors carry a `key` for every language;
  `TestEveryFailMessageHasEnglish` and `TestStringsComplete` keep them complete. Review status:
  [docs/TRANSLATIONS.md](TRANSLATIONS.md) — no native speaker has read them yet.
- **Colour mode (phone):** System, Light or Dark in the footer, stored with the Movies account in `themes.json`
  next to `STATE_FILE` (`GET /api/me` → `theme`, `POST /api/theme`), so the LAN and the Tailscale address show
  the same. `localStorage` keeps a copy only so `web/theme.js` can set `data-theme` before `style.css` paints; an
  account without a choice takes the one this device shows, and "Log out" clears the copy (a shared phone).
  `TestThemeChosenOnThePhone` keeps both dark blocks equal, `TestThemeFollowsTheAccount` the per-account store.
  The TV page is always dark (`web/tv/tv.css`).
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
(LG docs). `deploy/webos/renew-devmode.sh` renews it daily from the test NAS
(`movies-cast-devmode.timer`, data in `/DATA/AppData/movies-cast-devmode`); check with
`journalctl -u movies-cast-devmode.service -n 3` → `renewed: result=success`.

## Android TV app (Shield, Google TV; test build)

`deploy/androidtv` is the same hosted app for Android TV: a WebView that opens `/tv/` on movies-cast, so
pairing, library, remote and subtitles are the webOS app's code. On first start it asks for the address of
movies-cast (`10.20.30.40` becomes `http://10.20.30.40:8097`) and keeps it; when the page does not load it
shows the address again and retries every 10 s. Back and the media keys go to the page through
`window.moviesCastKey(<webOS key code>)`; Back on the library closes the app (from Android 13 through an
`OnBackInvokedCallback`: consuming the key in `dispatchKeyEvent` still closed the app, measured on Android 16).

```bash
deploy/androidtv/build.sh 0.26.0   # aapt2, javac, d8, apksigner from the SDK, no Gradle
# -> deploy/androidtv/out/movies-cast-tv-0.26.0.apk; the signing key is created once in
#    ~/.local/share/movies-cast/ — keep it, an update signed with another key does not install
python3 deploy/androidtv/render.py # icon and the 320x180 banner of the Android TV home row, from the logo
```

Measured in the emulator (Android 16, WebView 133, 2026-10-30): pairing, library, D-pad, OK (DPAD_CENTER
arrives as Enter), Back on detail and library, login kept across an app restart. Playback of 8 s test files
in the app's WebView: MKV with H.264 or HEVC plays with picture; **AAC sound decodes, AC3 and E-AC3 do not**
(`webkitAudioDecodedByteCount` 0); MPEG-TS does not open at all (`DEMUXER_ERROR_COULD_NOT_OPEN`), so the
converted stream of the DLNA path is no way out here.

Android TV emulator (Android 11, WebView 90.0.4430.91, 2026-10-30): the banner shows in the app list, the
app starts with D-pad and OK, OK on the address field opens the TV keyboard, pairing and library work, Back
(`onBackPressed` below Android 13) returns from the detail page and closes the app on the library. Playback
there: H.264 MKV with picture, AAC sound decodes, AC3 and E-AC3 do not; HEVC gives no frames (HEVC + AC3:
`DEMUXER_ERROR_NO_SUPPORTED_STREAMS`) — the x86 emulator image, not a statement about a device with an HEVC
decoder; MPEG-TS does not open. Audio track choice (▲) cannot work in an Android WebView: `audioTracks` exists only
with the Blink runtime feature `AudioVideoTracks` (`html_media_element.idl`), status "experimental" (off by default)
in Chromium 90.0.4430.91, 133.0.6943.137 and main; MDN's compatibility data says WebView Android "No". Which track
plays is up to the WebView (not measured). On a tester's Shield (2026-10-30) E-AC3 had sound and AAC was silent — the opposite
of both emulators; why is open. Not measured: a real Android TV device (Shield), the media keys during playback.

Without the app, an Android TV device can be a DLNA TV through Kodi ("Enable UPnP support" and "Allow remote
control via UPnP", setting `services.upnprenderer`, off by default in Kodi's `settings.xml`): a tester's Nvidia
Shield TV Pro then appeared in the list and played a film sent from the phone, in Kodi's player (2026-10-30). Before
that the Shield answered no search: nothing on it offered AVTransport.

### Play with Kodi (0.27)

The TV app on Android TV offers **"Play with: Kodi"** when Kodi (`org.xbmc.kodi`) is installed (the manifest
declares it under `<queries>`, which Android 11+ needs to see the package). The choice is kept on the box.
`POST /api/tv/kodi` then:

- sends the film by DLNA to a renderer that answers on the box's own address (Kodi with UPnP on), like the
  phone sends to a TV — resume, progress, next episode and the phone's remote work; the app then brings Kodi
  to the front;
- else answers a stream URL, which the app hands to Kodi with `ACTION_VIEW` (`setPackage(org.xbmc.kodi)`),
  the way Jellyfin, Plex and Emby use Kodi as external player; Kodi then plays from the start and reports
  nothing back.

Measured in the Android TV emulator (Android 11) with Kodi 21.3 (2026-10-31): the intent path plays (Kodi
fetched the stream from Movies-Cast, picture on screen; the emulator has no sound). The app asks the server
first and starts Kodi afterwards: opening Kodi first put the app in the background and Android dropped the
intent ("Abort background activity starts", logcat). The DLNA path to Kodi was measured on a tester's Shield
TV Pro (a film sent from the phone played in Kodi, 2026-10-30); the emulator's network (NAT) does not let an
SSDP search reach it. Kodi's sound on the Shield follows Kodi's audio settings (passthrough to an AV receiver
or decoding); a sound test on a Shield by a tester is the open measurement.

## Library: series, music, photos, queue (0.27)

Measured 2026-10-31. Movies 0.3.0 on the owner's box lists one view, `movies`, and 0 items of the types
Series, Episode, Audio and Photo; it honours `StartIndex`/`Limit` and `SearchTerm`, ignores `IsPlayed` and
`Genres` as filters, and answers `/Genres` with 404 — so search, sort and filter run on the phone over the
whole list, which the server reads in pages of 200 (a page that repeats stops the reading: a server that
ignores `StartIndex` would otherwise list the first page twice). Jellyfin 10.11.0 and Emby 4.10.2.0 (test
servers with generated media) list series (`/Shows/{id}/Episodes` in order), albums (`ParentId`), photos;
`/Videos/{id}/stream?static=true` serves music too (audio/mpeg), a photo comes from `/Items/{id}/Download`,
which answers `HEAD` with 405 (Jellyfin) or 404 (Emby) and `GET` with the file — the LG asks `HEAD` first and
refused the photo with UPnP 716 until `HEAD` was retried as `GET`.

A play that ran to its end (the last 45 s or 5 %) is followed by the queue, then the next episode or track;
Stop on the phone, or a play stopped half-way, starts nothing. On the LG: episode 1 → episode 2 by autoplay,
then the queued film before episode 3; an MP3 PLAYING; a JPEG accepted (PLAYING; the picture was not seen by
me).

## Subtitles over DLNA (0.27)

The phone lists a film's text subtitles (`/api/subtitles/{id}`); the chosen one goes to the TV as SRT,
converted from the WebVTT the server serves (`Stream.vtt`), at `/s/<play>/sub.srt` next to the film: a second
`res` with `text/srt` and Samsung's `sec:CaptionInfoEx` in the DIDL, and the `CaptionInfo.sec` header when the
TV asks with `getcaptionInfo.sec`. Jellyfin's item answer has no `DeliveryMethod` for subtitles; its text
tracks say `IsTextSubtitleStream: true` (measured 10.11.0). The LG fetched `sub.srt` (2 cues) before it
answered Play (2026-10-31); whether it showed them was not seen by me.

## Google Cast (0.27)

Cast v2 without a library: TLS to port 8009 (the device certificate is self-signed and not verified, as Cast
senders do), 4-byte length + protobuf `CastMessage` (coded by hand), JSON in the connection, heartbeat,
receiver and media namespaces; the Default Media Receiver (`CC1AD845`) plays the same `/s/` URL a DLNA TV
gets. Devices come from an mDNS query for `_googlecast._tcp` (name from TXT `fn`). Tested against two
receivers that speak the framing (one in `internal/cast`, one in `internal/cast/casttest` with its own frame
coding), not against a device: no Cast device answered on the owner's LAN (2026-10-31).

AirPlay, measured the same day: AirPlay devices on the LAN were two Sonos Era 100 (audio), an Apple TV 4K
and the LG (AirPlay 2). `GET :7000/info`: the LG lacks feature bit 0 (video); both video devices answer
`POST /play` without pairing with 403. AirPlay would need Apple's pairing with a PIN shown on the TV.

A pairing tool outside this repository (HAP pair-setup, ported from pyatv) then paired with the Apple TV 4K
(tvOS build 23L781): the PIN appeared only after `POST /pair-pin-start`, with AirPlay access set to "anyone on
the same network", and the device answered `470` until the header was sent spelled exactly `X-Apple-HKP`.
Pair-verify worked, and the AirPlay 2 sequence (SETUP, event channel, RECORD, `POST /play`, `/rate`) was
answered with 200 at every step; the TV showed a spinner, `/playback-info` answered 500, and a packet capture
showed no connection from the Apple TV to the media server. pyatv 0.18.0 (`atvremote play_url`, same
credentials) gave the same result. AirPlay video therefore is not in 0.27.

Later that day the same Apple TV (tvOS 26.6.1) played a URL through its remote-control queue instead of
`/play` (tvOS 26 changed this flow; pyatv issues #2821 and #2906, an unmerged rework of pyatv's `play_url`):
after RECORD a second SETUP with one stream (`type` 130, `controlType` 1, `clientTypeUUID`
`A6B27562-B43A-4F2D-B75F-82391E250194`) returns a `streamID`, and `POST /command` (HTTP/1.1, `X-Apple-StreamID`,
the first SETUP's session UUID as `X-Apple-Session-ID`, body `{params: {data: <binary plist>}}`) carries
`insertPlayQueueItem` with the `Content-Location`, two `setProperty` and `setRate`. The Apple TV fetched the MP4
(`AppleCoreMedia/1.0.0.23L781`, one range request over the whole file), and the event channel reported
`loading`, `readyToPlay`, `playing` with position and duration, and after 30.2 s `itemPlayedToEnd` and `stopped`.
Measured with the pairing tool outside this repository; movies-cast itself has no AirPlay code yet.

## Jellyfin and Emby as the source (0.27)

The same client talks to Movies, Jellyfin and Emby: login (`/Users/AuthenticateByName`), lists, streams,
progress. `/api/source` tells the login page whose account to ask for: ZimaOS for a `…/movies` base,
otherwise the product from `/System/Info/Public` ("Jellyfin Server"; Emby 4.10.2.0 sends no `ProductName`,
so a `/emby` base is taken as Emby).

## HTTPS (0.27)

`TLS_LISTEN` adds an HTTPS listener with the same handler; HTTP stays for the TVs (DLNA TVs and Kodi fetch
over HTTP; a self-signed certificate would stop them). Without `TLS_CERT`/`TLS_KEY` a P-256 certificate is
created once (10 years, the host name, `localhost` and the box's addresses), the key 0600; a second start
reuses it, so phones trust it once. Over HTTPS the session cookie is `__Host-mc_s` with `Secure`.
