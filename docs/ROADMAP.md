# movies-cast roadmap

From the walk-through of a new user's path on 2026-10-28 (every step below was a real obstacle that
day). Order approved on 2026-10-28.

## Done

| Version | What | Why (measured) |
|---|---|---|
| 0.14 | format hint before a DLNA send, "Play anyway", proven formats | LG lists no HEVC but plays it; 2012 Samsung lists none |
| 0.15 | SSDP from a fixed port (1901), `RENDERERS` takes hosts, `formats.json` | ZFW dropped the answers; LG moved its port 1795 → 1626 |
| 0.16 | SSDP port held open from start-up | ZFW opens a new host port only while it is listened on |
| 0.17 | manage TVs in the app (name, hide, add by IP), explained empty list, status card, QR code on the pairing screen | `RENDERERS` was edited by hand three times; "search failed" hid a firewall cause; codes were typed off the TV |
| 0.18 | 18 languages (ZimaOS' set, English default, picker on the phone, every server message keyed); failed DLNA formats remembered; Play waits 30 s instead of 6 s; Play time and every stream request logged; "Open in the movies-cast app" offered only when DLNA fails and the app is open | owner: multilingual is a must, DLNA is the top feature |
| 0.19 | Wake-on-LAN: a TV seen once is remembered with its MAC (ARP table) and listed as "off" when it answers no search; sending a film to it wakes it first (packet every 3 s, up to 60 s, until its description URL answers); an unreachable TV no longer counts as a failed format | a TV in standby vanished from the list; the owner's test with the LG in standby is open |
| 0.20 | sound converted on request ("No sound?" in the player, remembered per TV and audio format; offered when a DLNA send fails): Movies' HLS segments joined into one MPEG-TS stream, picture copied, audio AAC; seek re-sends from the segment; known TVs asked at their description URL when a search misses them; Play no longer cut at 6 s | 0.18's 30 s Play limit never applied: the HTTP client's own 6 s timeout cut first (a converted Play on the LG ended at 6.2 s, 2026-10-28); the Samsung answered its URL while a search missed it |
| 0.21 | colour mode on the phone: System, Light or Dark in the footer next to the language, remembered on the device, applied before the first paint; the browser bar colour follows | owner's request 2026-10-28; the phone page followed the system only; the TV page is always dark |
| 0.22 | colour mode stored with the account (themes.json), not per address; "Log out" clears the device's copy; a dev server (`TestDevServer`) runs the real page against the test fakes for browser checks with a login | in 0.21 the LAN and the Tailscale address each had their own choice; status card and player could not be checked in a browser without a real login |
| 0.23 | language stored with the account like the colour mode (langs.json); the dev server's TV plays (TRANSITIONING → PLAYING, position counts), a new URI starts at 0 | in 0.22 the language still lived per address; the player could not be clicked through in the dev server (the fake TV stayed in TRANSITIONING) |
| 0.24 | the movies-cast logo on the ZimaOS tile (data: URI, was Jellyfin's); check.sh (vet, tests, browser tests incl. the TV page, public-data scan) before every build; a package for testers: public image on ghcr.io, compose for ZimaOS "Import", guides de/en with a test list | the owner wants testers; the installer of ZimaOS ≥ 1.7.1 pulls every image itself, so a docker-loaded image would need root over SSH |
| 0.25 | the app is called "Movies-Cast" wherever its name shows: ZimaOS tile (de and en), browser tab, page heading, home-screen icon, TV page, every text and server message naming the app (18 languages); a test holds it | the tile said "Movies auf dem Fernseher" / "Movies Cast", the texts "movies-cast-App"; the owner asked for one name |

With 0.16 on .143 the search found both TVs with `RENDERERS` empty (3 of 3 searches, 0 drops).

DLNA on the LG, measured 2026-10-28 with own 8 s clips from a LAN server: HEVC 1080p/2160p/10-bit,
H.264, AC3, E-AC3, TrueHD, three audio tracks — all PLAYING, Play answered in 1.7–4.0 s (PLAYING is the transport state; whether there was sound was not checked). The real
Avatar (37 GiB, HEVC 4K, 3× E-AC3) through movies-cast 0.18.0: Play answered after 2.26 s, 40 s
PLAYING. The two failures of 19:35 and 17:51 (no answer to Play within 6 s) were not reproduced.

## Next

| # | What | Effort | Note |
|---|---|---|---|
| 4 | one-click install from an app store | 2–3 h | needs the repo public (owner's decision) and tests by the owner and by others before any store entry |
| 11 | one login per person | — | the phone has it since 0.1 (each person logs in with their own ZimaOS account; library and resume points are theirs). The TV app runs with the account of the phone that approved its pairing (`tvPairApprove`), so on a shared TV everyone continues in that account. Open: switching the account on the TV — only if more than one person uses it |

Sound (8): the LG lists no AC3, E-AC3, TrueHD or DTS (measured 2026-10-28) yet plays them with sound
(the owner, 2026-10-28) — a hint from the format list would be wrong for every film here, so 8 became
"No sound?" in 0.20. The Samsung UE50MU6179 lists AC3 only; whether it has sound with E-AC3 or TrueHD
is unknown.

## Measured and parked

- **Login through the ZimaOS session (5):** the ZimaOS UI keeps its token in `localStorage.access_token`
  on its own origin (ZFW `internal/auth/auth.go`); movies-cast on `:8097` is another origin and cannot
  read it. Single sign-on would mean running movies-cast as a ZimaOS module behind the gateway (like
  ZFW). Whether Movies accepts a ZimaOS token instead of a password is unknown — measurable with a real
  ZimaOS token against Movies.
- **Subtitles via DLNA on Samsung (7):** Samsung has its own extension for external subtitles; whether
  the UE50MU6179 supports it is unknown. Not needed by the owner.
