# Changelog

What changed in each version; the "why" with its measurements is in
[docs/ROADMAP.md](docs/ROADMAP.md) and [docs/TECHNICAL.md](docs/TECHNICAL.md).

## Unreleased

- Colours of Raumklang (the owner's Sonos app) on all three surfaces, the layout unchanged: the Android app, the phone page and the TV page are always dark (near-black, warm off-white writing, one soft gold accent; the stars stay, the glows are gold and grey); buttons, focus frames, the wordmark "CAST", progress bars and the "My media" tiles take Raumklang's colours, the logo keeps its orange planet. The colour-mode picker of the phone page (System/Light/Dark) is gone; the app no longer follows the phone's light mode, and on a phone in light mode the system bars keep light icons.
- Android app, first tests on a real Google TV Streamer (Android 14): a Dolby Vision profile 7 film on a TV without Dolby Vision has its picture again (the RPU and enhancement layer are taken out before the HEVC decoder gets it, as the Jellyfin server and Kodi do); the sound track that plays first is chosen as Kodi chooses it (the device's language, the file's default, then quality), so a film starts on its 5.1 track instead of a mono one; Back, ⋮ and every other text button and menu entry is framed in orange in focus, readable from the sofa. The device test of the keyboard on a TV waits until the field stands still before it taps (30 of 30 on the Streamer).
- Android app: "My media" on the home page, as Infuse's shortcut tiles: a tile for the whole library, then one per library of the server ("Filme", "Serien" …, as Jellyfin's apps list them), coloured by kind with a drawn symbol; a library with nothing in it gets no tile. OK shows that library alone, Back leads home. 1 text in 18 languages (Jellyfin's own translations of "My media"). On a TV the text field being typed into now stays above the keyboard (the password field lay behind it in 3 of 4 logins on the Android TV emulator).

- Android app: a film's page shows a line of facts as Infuse does: running time ("1 h 40 min", 18 languages), year, age rating and badges for the picture (4K, 1080p …), its range (HDR, HLG, Dolby Vision) and the sound ("TrueHD 7.1"); at its foot the file with bit rate and frame rate. What the server does not name (Movies names only the codecs of an MKV) is read from the file's own header without playing it (Media3's MetadataRetriever, `media3-inspector` 1.11.1; 12 KB to 6.7 MB and 42 to 249 ms per film on the owner's library; the resume points stay as they were). With a remote ↓ now reaches the overview and the file facts. A row of the home page keeps the focus at its ends and gives it back to the item that had it. Back while the keyboard is open only closes it (it cleared the search being typed).
- Android app: the home page has rows above the library, after Infuse's home screen: continue watching, favourites (the films marked as favourite), recently added, genres as picture tiles, best rated; "Home screen" in the menu shows or hides each row and sets their order (kept). OK on a genre tile shows its films with the focus on the first; Back shows the whole library again (before, Back on a searched or filtered library left the app). 6 texts in 18 languages.
- Android app: a film's page lies on its backdrop with the film's logo as title, as in Infuse (an episode takes its series' pictures; without a backdrop the poster page stays); every button grows, is lifted and gets an orange frame in focus, readable from the sofa; on a TV the film's page no longer pushes the logo out at the top when the main button takes the focus.
- `deploy/tools/android-screen.py type <label> <file>`: types a value (a password too, from a file) into the text field with that label, opens the keyboard on a TV with OK, replaces what the field held and reads it back; tested against a fake adb, and `./check.sh` runs that test and the one of `movies-probe.py`.
- Phone page and TV page in the logo's colours: a deep-sea blue night with stars and an orange glow (dark mode, the TV page), a pale morning sky (light mode), orange gradient buttons, sky-blue progress bars, a see-through header. The red Stop button in dark mode had white text at 2.8 to 1; it has a dark text now (axe now checks the dark library, film dialog and player, and a check of its own measures text on gradient buttons, which axe cannot). README screenshots taken again.
- Android app: needs Android 7 or newer (minSdk 24). On Android 6 0.2.0 died at its start, as the trust manager of the login (ScopedTrust) uses a class Android has from 7 on; `check-android.sh` now fails on an APK that allows less.
- Android app: the privacy policy and a licence's text scroll with a remote's ↑/↓ (on a TV they did not move, as they hold nothing to focus).
- New logo for every part of movies-cast: a play sign as an orange sun-planet with a sky-blue orbit and a sending satellite, on a deep-sea blue night (Android app icon with themed-icon layer, Android TV banner, the TV starter app, the webOS icons, the phone page's icons, the ZimaOS tile). Masters and render scripts in `android/art/`.
- Android app: a new look: a deep-sea blue starry sky with an orange glow behind every page (a TV always gets it, as Android TV's guidelines ask for a dark interface; a phone in light mode gets a pale morning sky), headings in Space Grotesk (SIL OFL 1.1, on the licences page), the logo in the title bar and on a frosted login panel, play and log-in buttons in the planet's orange with a white ring in focus, poster tiles that grow a little and get a glowing frame in focus.
- Android app 0.2.0, the first build for testers ([android/TESTERS.md](android/TESTERS.md)): everything below, signed with the release key; it installs over 0.1.0 and keeps its login.
- Android app: AC3, E-AC3, DTS and TrueHD are decoded in the app by default, as Kodi 21 ships, so a TV that cannot play what a box would pass on still has sound (Fire TV Stick 4K with DTS-HD passed on: no sound; decoded in the app: sound). "Sound" in the menu sets each format to "Pass on over HDMI (automatic)" for a TV or AV receiver that takes it (4 texts in 18 languages). The diagnostics name each audio output.
- Android app: a login kept by a build before 0.2.0 takes no installed certificate by itself; at a server the system does not trust it asks to log in again once (1 text in 18 languages).
- Android app 0.1.0 (`android/`, not released): a native player for phones, tablets and Android TV that plays the original file straight from Movies or Jellyfin — login, library with search and "continue watching", film page, picture-in-picture, 18 languages; FFmpeg (LGPL build) for sound the device cannot decode; Dolby Vision profile 7 as HEVC; one server conversion when the picture cannot be decoded.
- Android app: "About" (menu and login page) with the version, the open-source licences (every library of the build, FFmpeg with the LGPL's text first), links to the privacy policy and the source, and "Share diagnostics" — versions, device, decoders and the player's last log lines, shown before sharing, without addresses.
- Android app: the question for "Local network access" (Android 17) comes with "Log in", no longer at the first start.
- Android app: an episode brings the episodes after it; the next one starts by itself and the player's next/previous buttons step through them; each episode that ran to its end is reported to the server at its end.
- Android app: a series page lists its episodes by season, with a mark on those watched; "Mark as watched" / "Mark as unwatched" on every film, series and episode; a mark on watched posters.
- Android app: subtitle files lying next to the film (SRT, WebVTT, ASS/SSA) are offered by the player's subtitle button.
- Android app: the library can be ordered (name, recently added, year, rating; kept) and filtered (unwatched, started, favourites, a genre), with the words of the phone page; "Add to favourites" / "Remove from favourites"; an episode without a title of its own is listed as "S1 E2".
- Android app: "Search the network for servers" on the login page finds Jellyfin servers (their UDP call on port 7359) and ZimaOS boxes with Movies (mDNS, `_zimaos._tcp`) and lists what answers; a tap fills in the address.
- Android app: "Servers and accounts" in the menu keeps every login (several servers, several accounts at one); a tap changes to another without its password, "Remove" logs it out at its server. "Log out" with other logins kept shows their list.
- Android app: a server with a self-signed certificate is reached once the certificate is installed in Android's settings; until then the login page says that the device does not trust the certificate. There is no "accept anyway". An installed certificate counts only for the server being logged in to, and from then on only the one of the login: another certificate (also an installed one) gives "not the one it had when you logged in"; every other connection takes the system's certificates only.
- Android app: the privacy policy is a page of the app (the build copies PRIVACY.md into the APK), readable on a TV without a browser.
- `deploy/tools/movies-probe.py`: a list on the way of a field name is walked (`MediaStreams.Codec`), and `--digest` prints status, type, length and SHA-256 of an answer that is no JSON.
- PRIVACY.md: what the Android app, the server and the TV apps store and talk to.
- `./check.sh` has a step for the Android app; `deploy/tools/check-android.sh --device` runs eleven tests on a connected device; a GitHub workflow runs the step on pushes that touch the app.

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
