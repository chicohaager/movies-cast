# Movies-Cast for Android (native player)

Plays the films of IceWhale Movies (ZimaOS) or a Jellyfin server on the Android device itself:
the original file, straight from the server, no conversion on the box and no movies-cast server
in between. Kotlin, Jetpack Compose, AndroidX Media3 (ExoPlayer).

State 2026-11-13: first working build (0.1.0), in git, application id
`io.github.chicohaager.moviescast` (debug builds: `.debug` appended). Runs from Android 6
(`minSdk` 23) on phones, tablets and Android TV (remote control only, own launcher entry and
banner). Texts in the 18 languages of movies-cast, with a language list in the app. Picture-in-picture
from Android 8: Home during playback keeps the film in a small window. The release build (shrunk,
APK and bundle) is built, signed and checked; nothing is published yet. "About" (menu and login page)
shows the version, the licences of every library, the privacy policy's address and "Share diagnostics".

## Build

    cd android
    ffmpeg/build-ffmpeg.sh                      # once: FFmpeg n6.0.1 for 4 ABIs (about a minute)
    ./gradlew :core:test :app:assembleDebug     # APK: app/build/outputs/apk/debug/app-debug.apk
    ../deploy/tools/check-android.sh            # what ../check.sh runs for the app
    ../deploy/tools/check-android.sh --device   # the screen tests on the device adb sees
    ../deploy/tools/check-android.sh --device emulator-5554   # ... on this one, when several are connected
    ./gradlew :app:assembleRelease :app:bundleRelease   # app-release.apk and app-release.aab

The release key is not in the repository. A properties file names it; the build takes the first of:
the file `MOVIESCAST_KEYSTORE_PROPERTIES` points to, `keystore.properties` in this folder (ignored
by git), `~/.local/share/movies-cast/android-keystore.properties` (the folder where the key of the
Android TV app lies as well). Keep the key and its password: an update signed with another key does
not install over the app.

    storeFile=release.jks        # relative to the properties file
    storePassword=…
    keyAlias=…
    keyPassword=…

Without that file the release APK comes out as `app-release-unsigned.apk`.

Needs JDK 17 or newer and the Android SDK (platform `android-37.0`, build-tools 37.0.0);
`local.properties` with `sdk.dir=…` is not checked in. The FFmpeg step needs NDK 26.1.10909125 and
CMake 3.31.6 (`sdkmanager --install "ndk;26.1.10909125" "cmake;3.31.6"`); without it the app builds
and plays with the device's own decoders only.

## Layout

| Module | What |
|---|---|
| `core` | plain Kotlin: the client for the Jellyfin-compatible API (`MoviesClient`, 24 tests against a local fake server, 4 against one with a self-signed certificate) and what the library screen shows (`LibraryView.kt`, 8 tests), the search for servers (`ServerSearch.kt`, 8 tests), the list of logins kept (`SavedLogins.kt`, 12 tests) |
| `app` | the Android app: login, library with search and "continue watching", film page with seasons and "mark as watched", player with a queue, the list of servers and accounts, About; 18 tests (Dolby Vision base layer, shape of the small window, language list, the diagnostics text, the queue, the privacy text) and 17 on a device (`src/androidTest`) |
| `config` | what the licences page needs besides the Gradle dependencies: FFmpeg and the copied Media3 module as libraries, the LGPL's text as FFmpeg ships it |
| `ffmpeg` | Media3's FFmpeg audio decoder (`androidx/media` 1.11.1, `libraries/decoder_ffmpeg`, Apache 2.0), copied because Google publishes it only as source; `build-ffmpeg.sh` builds FFmpeg for it |

What the client keeps from movies-cast's Go client (`internal/movies`): the token rides in the
`X-Emby-Authorization` header and never in a URL, no redirect is followed with it, the film comes
from `/Videos/{id}/stream?static=true` with Range requests. `PlaybackInfo` is called in one case
only, the fallback below, because Movies answers it by starting a transcode.

The player says so when the device has no decoder for a film's sound or picture format, and logs
decoders and formats under the tag `MoviesCast` (`adb logcat -s MoviesCast`), without addresses or tokens.

## About, licences, diagnostics

The licences page lists every library of the build (129 on 2026-11-13) from the file the
AboutLibraries plugin writes at build time (`res/raw/aboutlibraries.json`), as Jellyfin's Android TV
app does. FFmpeg is no Gradle dependency: `config/libraries/ffmpeg.json` names it, and it heads the
list with the LGPL 2.1's text. `check-android.sh` fails when an APK lacks FFmpeg or that text.

"Share diagnostics" shows a text and hands it to Android's share sheet: app, Android and FFmpeg
version, device model, the server's product and version, the player's last 200 log lines (kept in
memory only, `AppLog`), the device's decoders. No address, user name, token or film title is put
in; `LogBook.scrub` replaces URLs and IPv4 addresses in every line all the same.

## Series, the queue, subtitle files, "watched"

A series page lists the episodes by season. "Play" on an episode hands the player that episode and
the ones after it (at most 100) as one Media3 playlist: the next starts by itself, the player's
next/previous buttons step through them. At each change the server gets "stopped" for the old
episode at the place it had reached (its end when it ran out) and a new play session for the next.

Subtitle files lying next to the film (`IsExternal` in the server's stream list; SRT, WebVTT,
ASS/SSA) go to the player as `SubtitleConfiguration`s, from `/Videos/{id}/{source}/Subtitles/{index}/Stream.{format}`,
the address Jellyfin's Android TV app builds for external players. The subtitle button offers them;
none is switched on by itself.

"Mark as watched" / "Mark as unwatched" (`POST` / `DELETE /Users/{user}/PlayedItems/{item}`) on the
page of a film, series or episode; watched posters and episodes carry a ✓.

The privacy policy is a page of the app: the build copies the repository's `PRIVACY.md` into the
APK (`assets/PRIVACY.md`, task `privacyAsset`).

## Library: order, filter, favourites

Two menus above the grid, as on movies-cast's phone page and with its words (10 of the texts are
taken from `internal/server/web/i18n`): the order (name, recently added, year, rating; kept for the
next start) and a filter (all, unwatched, started, favourites, one genre of the library). The app
orders and filters the library it has in memory (`LibraryView.kt` in `core`); the server is not asked
again. "Recently added" goes by the moment, not by the text: Movies writes
`2026-10-27T18:04:21.329122815+01:00`, local time with an offset that changes with summer time.
"Continue watching" stands above the grid only while nothing is searched or filtered.

"Add to favourites" / "Remove from favourites" on the page of a film, series or episode
(`POST` / `DELETE /Users/{user}/FavoriteItems/{item}`). An episode that carries its series' name as
its own is listed as "S1 E2" without the name.

## Finding the server in the home network

"Search the network for servers" on the login page asks two ways at the same time for 4 seconds and
lists what then answers `System/Info/Public` (name, product, version, address); a tap puts the
address into the field. On Android 17 the question for "Local network access" comes first.

- **Jellyfin**: the text `who is JellyfinServer?` to UDP port 7359 of every broadcast address of the
  device, as Jellyfin's own apps do. The answer names the server's address. A Jellyfin in a
  container without host networking names the address inside the container's network, which a phone
  does not reach; so the sender's own address with the same port is tried second.
- **IceWhale Movies** does not answer that call. A ZimaOS box announces itself by mDNS as
  `_zimaos._tcp` (port 80), found with Android's service search (`NsdManager`); Movies is behind
  its gateway at `/movies` if it is installed. Android at times names only an IPv6 link-local
  address for a box at first; the app asks again (up to 6 times, 250 ms apart) until the IPv4
  address comes.

`ServerSearch.kt` in `core` holds the call, the choice of addresses and the check (8 tests, the call
against a fake on the loopback); `ZimaSearch.kt` in `app` is the mDNS part.

## More than one server or account

The app keeps every login (`SavedLogins` in `core`, written by `SessionStore`): a server's address,
the account and its token, encrypted with the keystore's key as before. "Servers and accounts" in
the menu lists them; a tap puts another one in use without its password, "Add a server or account"
opens the login page, "Remove" logs that login out at its server and forgets it. "Log out" does the
same for the login in use; if others are kept, their list is shown and not the login page. A second
account at the same server is a login of its own. Jellyfin's Android TV app keeps servers and their
users the same way (`AuthenticationStore`). The one login that 0.1.0 kept in four keys is taken
over into the list at the first start.

An answer that comes after the change of login does not touch the new one: a request that fails
with 401 logs out only the login it was made with.

## HTTPS with a certificate of the owner's own

A server with a self-signed certificate (or one of an authority of the owner's own) is reached
once that certificate is installed on the device, in Android's settings ("Install a certificate" →
"CA certificate"): `network_security_config.xml` trusts the certificates the user installed besides
the system's, as Jellyfin's Android TV app does. Until then the login page says "This device does
not trust the server's certificate …" (also when the certificate is made out for another name than
the address typed). Decision of the owner (2026-11-13): there is no button "accept the certificate
anyway", and none is to be built.

`deploy/tools/user-ca/measure-user-ca.sh [serial]` repeats that measurement without a hand on the
device: it makes a certificate for the test, starts a fake HTTPS server on this machine
(`fake-https.py`, reached from the device through `adb reverse`), and logs in from the app's login
page three times — before the certificate is in Android's user store, after it was put there as the
settings put it (`/data/misc/user/0/cacerts-added/<subject hash>.0`), after it was taken out again —
and fails unless the app refuses, trusts, refuses. Putting a file into that store needs root, so it
runs on an emulator with an image "Google APIs" (a "Google Play" image gives none); one such:

    sdkmanager "system-images;android-36;google_apis;x86_64"
    avdmanager create avd -n mc-api36-root -k "system-images;android-36;google_apis;x86_64" -d medium_phone

## Tests on a device

`check-android.sh --device` installs the debug build and the test APK and runs `AppUiTest`: login,
library and film page, a wrong password, About with the licences, the diagnostics text, a series
with its seasons, "mark as watched", the privacy page, order and filter, a favourite, the search for
servers (what it brings is set by the test, the network of the device is not asked), a second login
with the change back, removing a login, "Log out" with a second login kept, a certificate the
device does not trust, the take-over of the login 0.1.0 kept, a refusal (401) that arrives after
the change of login — against a
fake server inside the test (MockWebServer on 127.0.0.1). The app runs as `TestApp` there, which
keeps its login in a file of its own (`session-test`), so a real login on the device stays. The
script installs with `adb install -r` and not with Gradle's `connectedDebugAndroidTest`. With more
than one device connected it stops before the build and names their serial numbers; `--device
<serial>` or `ANDROID_SERIAL` names the one (`pick-device.sh`, 10 cases in `test-pick-device.sh`
with a stand-in for adb, each red once). It stops
with "the device is locked" when the lock screen stays: behind it no activity starts and every test
fails with "No compose hierarchies found" (Pixel 7, 2026-11-13).
Espresso is pinned to 3.7.0: the 3.5.0 that Compose's test library brings fails on Android 17
(`NoSuchMethodException: InputManager.getInstance`).

## Android 17: access to the home network

From Android 17 (API 37) an app that targets API 37 needs the runtime permission
`ACCESS_LOCAL_NETWORK` to reach addresses in the home network. Without it a ZimaOS box at
192.168.x.x does not answer the app, while the same request from `adb shell` gets 200
(Pixel 7, Android 17, 2026-11-12). The app declares it and asks at "Log in" and, with a saved login,
before the library is read; a refusal shows its own message with a button to the app settings. Android 16
and earlier have no such permission (emulator API 36: login without a question).

## Picture formats the device cannot decode

Dolby Vision profile 7 (UHD Blu-ray rips): Media3 has no fallback for it, so a device without a
decoder for `video/dolby-vision` showed no picture. `BaseLayerVideoRenderer` hands such a track to
the HEVC decoder as plain HEVC (the HDR10 base layer); the enhancement layer is ignored.

No decoder at all for the picture (AV1 or HEVC on an old device), or the decoder fails: the player
asks the server once to convert (`PlaybackInfo` with a profile that offers H.264/AAC over HLS only)
and plays that stream; the film then starts at the position it had reached. The profile carries the
largest picture the device's H.264 decoders take (width and height conditions, as Jellyfin's Android
TV app sends them) and a bit rate to match (40, 10 or 4 Mbit/s). Jellyfin 10.11.0 scales by it;
Movies 0.4.5 sends the original size whatever the profile says.

On a TV the keyboard opens only after OK on a text field, so the arrow keys keep moving the focus.

## Translations

`app/src/main/res/values-*/strings.xml`, one folder per language. `tools/strings-check.py` (part of
`check-android.sh`) fails when a language lacks a text, has one too many or differs in a placeholder;
`check-android.sh` also fails when a language is missing in the APK, which happens when it is not
listed in `localeFilters` in `app/build.gradle.kts`.

The app's menu (and the login page) has a language list: "System default" and the 18 languages
under their own names. It is AppCompat's per-app language (`AppCompatDelegate.setApplicationLocales`,
kept by `AppLocalesMetadataHolderService` on Android 6 to 12, by the system from Android 13, where
the system's own setting for the app shows the same choice). The list comes from `languages` in
`app/build.gradle.kts`, the same list the build keeps translations by.

English and German are written by the author. The other 16 languages are not yet read by a native
speaker; that reading is planned for the open test. Corrections are welcome as an issue or a pull
request against the language's `strings.xml`.

## Sound formats the device cannot decode

The player uses the device's own decoders first and FFmpeg only for a format the device has no
decoder for (`EXTENSION_RENDERER_MODE_ON`). FFmpeg decoders built in, as in Jellyfin's build:
flac, alac, pcm_mulaw, pcm_alaw, mp3, aac, ac3, eac3, dca (DTS), mlp, truehd.

Licences: FFmpeg n6.0.1 is configured without `--enable-gpl` and `--enable-nonfree`
(`config.h`: `CONFIG_GPL 0`, `FFMPEG_LICENSE "LGPL version 2.1 or later"`) and linked statically
into `libffmpegJNI.so`; the app's source and `build-ffmpeg.sh` let anyone rebuild it against another
FFmpeg, as the LGPL asks. Media3's module stays Apache 2.0 (`ffmpeg/LICENSE-media3.txt`). Jellyfin's
prebuilt `org.jellyfin.media3:media3-ffmpeg-decoder` is not used: its repository is GPL-3.0.

`libffmpegJNI.so` is linked for 16 KB pages (`-Wl,-z,max-page-size=16384`); without it Android
warned on the Pixel 7 that the app is not 16 KB compatible (LOAD alignment 0x1000, now 0x4000;
`zipalign -c -P 16` passes).

## Measured 2026-11-12

Test clips (60 s, own, from a local Jellyfin 10.11.0) on a Pixel 7 with Android 17:

| Sound | Decoder | First picture |
|---|---|---|
| AAC 2.0 | `c2.android.aac.decoder` | 0.6 s |
| AC3 5.1 | `c2.dolby.eac3.decoder.ac3` | 0.4 s |
| E-AC3 5.1 | `c2.dolby.eac3.decoder.eac3` | 0.3 s |
| TrueHD 5.1 | none on the device (before FFmpeg) | picture only |
| DTS 5.1 | none on the device (before FFmpeg) | picture only |

Against Movies 0.4.5 on a ZimaOS 1.8.1-alpha1 box (emulator, no playback): login, library,
"continue watching", posters and film page; Movies logged 200 for every request and 204 for the logout.

With FFmpeg, in the emulator (Android 16, x86_64, no device decoder for any of them): AC3, E-AC3 and
DTS decoded by `ffmpegLavc60.3.100-ac3/-eac3/-dca`; TrueHD's decoder `ffmpegLavc60.3.100-truehd`
started, but the clip stopped because the emulator cannot decode its 10-bit HEVC picture.

Real films from Movies 0.4.5 on the Pixel 7: DTS-HD by `ffmpegLavc60.3.100-dca`, TrueHD 7.1 by
`ffmpegLavc60.3.100-truehd`, a Dolby Vision profile 7 film as HEVC by `c2.exynos.hevc.decoder`.

Emulators: Android 6 (API 23, x86_64) start, login and playback with FFmpeg's AC3; Android TV 11
(x86) login, library and playback with the remote's keys only; tablet size 1600x2560 (Android 16).

Server conversion, Android 6 emulator (no AV1 decoder): an AV1 film from Movies 0.4.5 came as H.264
3840x1606 with AAC, first picture 5.6 s after opening, Movies logged `PlaybackInfo`, `master.m3u8`
and `main.m3u8` with 200 and `transcode codec=h264`. Same case against Jellyfin 10.11.0 (Android 16
emulator, HEVC clip): H.264/AAC, first picture 1.9 s.

Size limit, Android 6 emulator (H.264 decoder up to 1920x1088): a 3840x2160 AV1 clip from Jellyfin
10.11.0 came as H.264 1920x1080 (Jellyfin's FFmpeg line: `-maxrate 9808000`, `scale=…min(1920,1088*a)`);
the AV1 film from Movies 0.4.5 came as 3840x1606 with the same profile.

Picture-in-picture: Android 16 emulator: Home during playback pins the player and the position goes
on (6.9 s, 13.0 s four seconds later), a tap on the window's full-screen button brings the player
back, its X ends the player and Jellyfin logs "Playback stopped". Android TV 11 emulator (the path
for Android 8 to 11): pinned, 10.0 s, 13.0 s three seconds later. Android 6 emulator: Home pauses,
no crash.

Languages, Android 16 emulator, app language set with `cmd locale set-app-locales`: library texts
read in fr, ja, pt-BR, pt-PT, zh-TW, zh-CN, ru and hu (8 of the 16 new ones).

Language list: Android 6 emulator: Japanese chosen in the app, texts Japanese, still Japanese after
the app was stopped and started. Android 16 emulator: French chosen in the app, `cmd locale
get-app-locales` says `[fr]`; "System default" empties it again. Android TV 11 emulator, keys of
the remote only: menu, language, three rows down, OK: texts German.

Release build with a throwaway key, Android 16 emulator: login, DTS clip decoded by
`ffmpegLavc60.3.100-dca`, first picture 0.8 s, Jellyfin logs "Playback stopped"; APK 23.9 MB
(debug 38.6 MB), bundle 15.5 MB.

Release key (RSA 3072, alias `moviescast`, created 2026-11-12): APK and bundle carry the same
certificate (`apksigner verify --print-certs`, `keytool -printcert -jarfile`), the signed APK starts
in the Android 16 emulator.

## Measured 2026-11-13

Pixel 7, Android 17, debug build: About, the licences list (FFmpeg n6.0.1 first, LGPL text shown), an
AV1 film from Movies played (`c2.google.av1.decoder`, 3840x1606, first picture 0.8 s), afterwards
"Share diagnostics" showed 6 log lines and 50 decoders and opened Android's share sheet.
`check-android.sh --device`: OK (4 tests); each was red once with the app broken on purpose (no
`Items/{id}` request, 401 shown as a network error, FFmpeg missing in `config`, an address in the
report with `scrub` switched off), and the real login of the debug app was still there afterwards.

Signed release build on the same Pixel 7: starts, About, licences with the LGPL's text, the
diagnostics text says `release`. With the permission taken away (`pm revoke`) a fresh start shows
the login page and no permission dialog. After a login by the owner a Dolby Vision profile 7 film
from Movies played: `dolby vision dvhe.07.06: no decoder, playing the HEVC base layer`,
`c2.exynos.hevc.decoder` 3840x2160, `ffmpegLavc60.3.100-truehd` 8 channels, first picture 1.7 s.

A release APK built without `res/raw/keep.xml` still carried the licences file (the code names
`R.raw.aboutlibraries`), so no keep file is in the tree.

Android 16 emulator against Jellyfin 10.11.0 (a test series of three 15 s and one 20 s episode):
"Play" on S1E2 logged `queue of 3`; S1E3 and S2E1 followed by themselves (`next item of the queue
(2 of 3)`, `(3 of 3)`), and the server then listed all three as watched. A film with a `.de.srt`
next to it: `queue of 1, subtitle files 1`, `subtitle tracks 1: de`; the subtitle button offered
"Deutsch" and the file's second line showed at 0:11. `check-android.sh --device` there and on the
Pixel 7: OK (7 tests); the three new ones and the four new unit tests were red once with the app
broken on purpose.

Against Movies 0.4.5 on the test NAS (2026-11-13, one library of the kind "movies", 8 films):

- "Mark as watched" and its opposite work: pressed in the app on the Pixel 7, Movies then answered
  `Played=True, PlayCount=1` and afterwards `Played=False, PlayCount=0` for the film.
- Series: two episode files in `Test series/Season 01/` inside that library were not taken in
  (`[scan] library 1: 3 new … 2 skipped`, `3 movies + 0 shows`); the film next to them was. In a
  library of the kind "TV shows" (added in Movies' settings, `CollectionType=tvshows`) the same two
  files came as `Series`, `Season` and two `Episode` items with `SeriesId`, `ParentIndexNumber` and
  `IndexNumber`. The app on the Pixel 7 showed "Season 1" with both episodes, "Play" on the first
  logged `queue of 2`, the second followed by itself (`next item of the queue (2 of 2)`), and Movies
  then listed both as `Played=True`. Both test episodes carried the series' name as their own.
- Subtitle files: Movies does not name a `.de.srt` next to the film in the item
  (`/Users/{user}/Items/{id}?Fields=MediaStreams,MediaSources` lists video and sound only), so the app
  offers none. `GET /Items/{id}/PlaybackInfo` names it (`IsExternal`, index 2, `DeliveryUrl`), and
  `/Videos/{id}/{id}/Subtitles/2/Stream.srt` answered the file byte for byte (SHA-256 equal);
  `Stream.vtt` answered 404. But the same `PlaybackInfo` started `movies-ffmpeg` for an HEVC film
  (`transcode … from seg 0`; the process was gone 41 s later), which is why the app does not call it.
- `PlaybackInfo` also lists more streams than the item does (17 against 2 for one film), so the
  film page shows fewer sound and subtitle tracks with Movies than the file has; the player reads
  the tracks inside the file itself.

The test files were removed again and Movies dropped the film (`removed=1`, 8 films).

The workflow's first run on GitHub (commit 588a860) passed: FFmpeg built, `check-android.sh` green.

Order, filter and favourites (2026-11-13): Movies 0.4.5 on the test NAS answers `ProductionYear`,
`CommunityRating`, `DateCreated`, `Genres` and `UserData.IsFavorite` for its films (one film was a
favourite, set in Movies' own page; two films have no rating, one no year). `check-android.sh
--device` in the Android 16 emulator: OK (9 tests), the two new ones red once with the app broken on
purpose; the 9 new unit tests in `core` likewise. A favourite set and taken back from the app in the
emulator: Jellyfin 10.11.0 answered `IsFavorite=True`, then `False`. The same from the app on the
Pixel 7 against Movies 0.4.5: `UserData.IsFavorite` of the film `True` after the press, `False` after
the second, and the list of all 8 films as before.

Search for servers (2026-11-13, home network with two ZimaOS boxes, Movies on one of them, and a
Jellyfin 10.11.0 on a third machine):
- UDP 7359 from a computer: 0 answers from the boxes; Jellyfin answered, also to the broadcast once
  it ran with host networking. Published as `-p 7359:7359/udp` from a bridge network it answered
  only a packet sent straight to its host, and named its address inside the bridge network.
- mDNS: both boxes answered `_zimaos._tcp.local` (SRV port 80, TXT `os=ZimaOS`);
  `/movies/System/Info/Public` answered 200 on one and 404 on the other.
- The app on the Pixel 7 (Android 17): before the app asked a box again for its address, 5 of 8
  searches lost one box ("1 address, none of them IPv4" in the log); with it 10 of 10 searches found
  both boxes and listed the two servers (Movies, Jellyfin), 5 of them after asking 3 times. A tap on
  the Movies entry and the account: logged in.
- `check-android.sh --device` on the Pixel 7: OK (11 tests); the two new ones red once with the app
  broken on purpose (the tap does nothing, no "nothing found" text, no list), the 8 new tests in
  `core` likewise.

More than one login, HTTPS with a self-signed certificate (2026-11-13):
- `check-android.sh --device` in the Android 16 emulator: OK (17 tests). The 6 new ones were red
  once with the app broken on purpose (the change of login refused; "Remove" does not tell the
  server; "Log out" goes to the login page; the certificate error shown as "no server"; the old
  login not taken over: `Tests run: 16, Failures: 5`; a late 401 of the old login not told apart:
  `Tests run: 17, Failures: 1`, the new login was gone). The 16 new tests in `core` likewise, each
  with a break of its own; two of them stayed green at first (they asked only for the login in use,
  which is none for an index behind the list) and now also ask for the index.
- A self-signed certificate from the app: a fake server on the workstation with a certificate made
  for the test (`subjectAltName` the address under which the emulator reaches the workstation,
  `basicConstraints=CA:TRUE`), the debug app in the
  Android 16 emulator (Play Store image, no root). Before the certificate was installed the login
  page showed the text above and the server saw no request. After installing it in Android's
  settings ("Encryption & credentials" → "Install a certificate" → "CA certificate"; it then stands
  under "Trusted credentials" → "User") the same login showed the library with the server's one
  film; the server saw `AuthenticateByName` and the two list requests. After removing the
  certificate again the app refused the server as before. Not played: the fake server has no film
  file. Not measured on a device other than this emulator.

Tools (2026-11-13): `measure-user-ca.sh` on the emulator `mc-api36-root` (Android 16, image
"Google APIs" revision 7): refused, trusted, refused. With the app built without the user
certificates in `network_security_config.xml` it failed at the second case (`refused` where
`trusted` was expected), as it has to. `check-android.sh --device` with the Pixel 7 and that
emulator connected stops before the build and names both serials; `--device <serial>` then runs on
the one named.
- Pixel 7 (Android 17), the 17 screen tests: OK. The real login of the debug app there, written by
  0.1.0 in four keys, was taken over into the list at the first start (the file then had `logins`
  and none of the four keys). Then a second login from the app at a Jellyfin 10.11.0 with an
  account made for the test (reached through `adb reverse`): its 6 test films. "Servers and
  accounts" listed both; a tap changed to the Movies login and back, the films were each server's,
  and Jellyfin's log had one password login only. "Remove" on the Jellyfin login: Jellyfin logged
  its token out and dropped the device from its list; the Movies login stayed in use.
