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
| `core` | plain Kotlin: the client for the Jellyfin-compatible API (`MoviesClient`), 20 tests against a local fake server |
| `app` | the Android app: login, library with search and "continue watching", film page, player, About; 14 tests (Dolby Vision base layer, shape of the small window, language list, the diagnostics text) and 4 on a device (`src/androidTest`) |
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

## Tests on a device

`check-android.sh --device` installs the debug build and the test APK and runs `AppUiTest`: login,
library and film page, a wrong password, About with the licences, the diagnostics text — against a
fake server inside the test (MockWebServer on 127.0.0.1). The app runs as `TestApp` there, which
keeps its login in a file of its own (`session-test`), so a real login on the device stays. The
script installs with `adb install -r` and not with Gradle's `connectedDebugAndroidTest`.
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
the login page and no permission dialog. A login and playback with the release build: not done.

A release APK built without `res/raw/keep.xml` still carried the licences file (the code names
`R.raw.aboutlibraries`), so no keep file is in the tree.
