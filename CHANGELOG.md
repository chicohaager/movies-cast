# Changelog

What changed in each version; the "why" with its measurements is in
[docs/ROADMAP.md](docs/ROADMAP.md) and [docs/TECHNICAL.md](docs/TECHNICAL.md).

## 0.27.0 (2026-10-31)

- Series and episodes, music albums and photos (with Jellyfin or Emby; Movies 0.3.0 has films only); tabs show only what the server has.
- Search, sort (name, recently added, year, rating) and filter (unwatched, started, genre) on the phone; the library is read in pages of 200.
- Queue, and the next episode or track follows by itself when one ran to its end; "Up next" in the episode list.
- The sleep timer (and Stop on the phone) near the end of an episode no longer lets the next episode start: the watcher could see the TV's STOPPED after the play had been ended.
- Subtitles to DLNA TVs as SRT (picked on the phone).
- Google Cast devices as targets.
- "Search" for TVs says "Searching …" while it waits (7 s on a test NAS) and then what it found, also when no TV answered and the list shows known TVs that are off.
- Each TV search writes one log line with answers, TVs, Cast devices and where the time went (SSDP, descriptions, Cast); the Cast search runs next to the DLNA search instead of after it, and TVs seen before are asked during the SSDP wait (a search with two TVs off took 7 s on a test NAS, 5.1 s after the Cast change); a known TV that does not answer within 1.5 s counts as off (the LG answered in 0.3 s, a device that is off took 3.07 s to fail).
- Android TV: "Play with: Kodi" — Kodi plays the film with its own player and sound (by DLNA with progress, or by intent).
- Jellyfin and Emby servers as the source; the login names the server's account.
- HTTPS (`TLS_LISTEN`) with a `__Host-` session cookie.
- Accessibility checked with axe (WCAG 2.1 AA) in the browser tests.
- webOS: a store build that asks for the server address (`STORE=1 deploy/webos/package.sh`).
- README split into a product page and docs/TECHNICAL.md; CONTRIBUTING and issue templates.

## 0.26.0

- Android TV app (test build, `deploy/androidtv`): WebView on `/tv/`, start page for the server address, Back and media keys through `window.moviesCastKey`; testers' guides explain how it works (phone = remote, nothing installed on a DLNA TV)

## 0.25.4

- MIT license; the repository is public; author, committer and tagger use the GitHub noreply address; image built from the rewritten history, so its revision label names a commit that exists

## 0.25.3

- every stopped report to Movies is logged with film, position and outcome, also when it reached Movies; build-on-host.sh builds a worktree of HEAD, like make-package.sh since 0.25.2

## 0.25.2

- code and security review: parallel login guesses throttled, the cookie slides with the session, unique pair codes, state files synced; a TV that stopped answering ages out, no HTTP client follows redirects; SSDP answers, description fetches, known TVs, added hosts and open pairings capped; a stream stays with the address that fetched it first, and the player names a refused fetch; make-package.sh builds a worktree of HEAD; the phone page polls once at a time and keeps refusal buttons; the English check of server messages reads every source file (the host cap message was German only)

## 0.25.1

- the film dialog's buttons wrap instead of being cut off (all 18 languages, browser test at 360 px); the TV app is called Movies-Cast too; the time zone comes from the host's /etc/localtime (read-only), not TZ=$TZ; README screenshots from a public-domain demo library

## 0.25

- the app is called "Movies-Cast" wherever its name shows: ZimaOS tile (de and en), browser tab, page heading, home-screen icon, TV page, every text and server message naming the app (18 languages); a test holds it

## 0.24

- the movies-cast logo on the ZimaOS tile (data: URI, was Jellyfin's); check.sh (vet, tests, browser tests incl. the TV page, public-data scan) before every build; a package for testers: public image on ghcr.io, compose for ZimaOS "Import", guides de/en with a test list

## 0.23

- language stored with the account like the colour mode (langs.json); the dev server's TV plays (TRANSITIONING → PLAYING, position counts), a new URI starts at 0

## 0.22

- colour mode stored with the account (themes.json), not per address; "Log out" clears the device's copy; a dev server (`TestDevServer`) runs the real page against the test fakes for browser checks with a login

## 0.21

- colour mode on the phone: System, Light or Dark in the footer next to the language, remembered on the device, applied before the first paint; the browser bar colour follows

## 0.20

- sound converted on request ("No sound?" in the player, remembered per TV and audio format; offered when a DLNA send fails): Movies' HLS segments joined into one MPEG-TS stream, picture copied, audio AAC; seek re-sends from the segment; known TVs asked at their description URL when a search misses them; Play no longer cut at 6 s

## 0.19

- Wake-on-LAN: a TV seen once is remembered with its MAC (ARP table) and listed as "off" when it answers no search; sending a film to it wakes it first (packet every 3 s, up to 60 s, until its description URL answers); an unreachable TV no longer counts as a failed format

## 0.18

- 18 languages (ZimaOS' set, English default, picker on the phone, every server message keyed); failed DLNA formats remembered; Play waits 30 s instead of 6 s; Play time and every stream request logged; "Open in the movies-cast app" offered only when DLNA fails and the app is open

## 0.17

- manage TVs in the app (name, hide, add by IP), explained empty list, status card, QR code on the pairing screen

## 0.16

- SSDP port held open from start-up

## 0.15

- SSDP from a fixed port (1901), `RENDERERS` takes hosts, `formats.json`

## 0.14

- format hint before a DLNA send, "Play anyway", proven formats
