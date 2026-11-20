# Movies-Cast for Android: testing 0.3.0

(Deutsch weiter unten.)

A player for Android phones, tablets and Android TV. It plays the films of IceWhale Movies (ZimaOS)
or a Jellyfin server on the device itself, in the original file, without a conversion on the box.
0.3.0 is the second build for testers (the first public one). It is in no store.

## Install

1. You need Android 7 or newer, and a ZimaOS box with Movies or a Jellyfin server in your network.
2. Copy `movies-cast-android-0.3.0.apk` to the device and open it. Android asks once whether the app
   that opens it (Files, the browser) may install apps; allow it.
3. Later versions install over this one, and the logins are kept.

## First start

- The login page takes the server's address (`http://<box>/movies` for Movies,
  `http://<server>:8096` for Jellyfin), or "Search the network for servers" finds it.
- Android 17 asks for "Local network access" when you tap "Log in". Without it the app cannot reach
  a server in your home network.
- A server with a certificate of its own (self-signed HTTPS) is reached only after you install that
  certificate in Android's settings ("Install a certificate" → "CA certificate"). It then counts for
  that server only. There is no "accept anyway".

## Please try

- Log in, browse the library, search, sort and filter, and play films. Try films with sound in DTS,
  TrueHD or AC3 and in 4K or HDR if you have them.
- Series: the next episode should start by itself, and "Mark as watched" should work.
- "Servers and accounts" (menu): add a second login, change between them, "Remove" one.
- On Android TV: everything with the remote control only.
- Home during playback (Android 8 and newer) keeps the film in a small window.
- New in 0.3.0: the home page with rows (continue watching, favourites, recently added, genres, best
  rated) and "My media" tiles per library; "Home screen" in the menu shows, hides and moves the rows.
- New in 0.3.0: a film's page on its backdrop, with running time, year, age rating and badges for
  picture, range and sound. A Dolby Vision film on a TV without Dolby Vision should show a picture.

## Report a problem

- In the app: menu → "About" → "Share diagnostics". The text shows the versions, the device, its
  decoders and the player's last log lines. It contains no address, user name or password, and you
  see it before you share it.
- Write what you did, what you expected and what happened. Send it to the person who gave you the
  app, or open an issue at https://github.com/chicohaager/movies-cast/issues.
- What the app stores and whom it talks to: [PRIVACY.md](../PRIVACY.md), and in the app under
  "About".

---

# Movies-Cast für Android: 0.3.0 testen

Ein Player für Android-Handys, -Tablets und Android TV. Er spielt die Filme von IceWhale Movies
(ZimaOS) oder einem Jellyfin-Server auf dem Gerät selbst ab, als Originaldatei und ohne Umwandlung
auf der Box. 0.3.0 ist die zweite Version für Tester (die erste öffentliche). Sie steht in keinem Store.

## Installieren

1. Du brauchst Android 7 oder neuer und eine ZimaOS-Box mit Movies oder einen Jellyfin-Server im
   Netz.
2. Kopiere `movies-cast-android-0.3.0.apk` aufs Gerät und öffne die Datei. Android fragt einmal, ob
   die öffnende App (Dateien, Browser) Apps installieren darf. Erlaube es.
3. Spätere Versionen installieren sich über diese, und die Anmeldungen bleiben erhalten.

## Erster Start

- Die Anmeldeseite nimmt die Adresse des Servers (`http://<box>/movies` für Movies,
  `http://<server>:8096` für Jellyfin). Sonst findet ihn „Server im Netz suchen“.
- Android 17 fragt beim Tippen auf „Anmelden“ nach „Zugriff auf lokales Netzwerk“. Ohne diese
  Erlaubnis erreicht die App keinen Server im Heimnetz.
- Einen Server mit eigenem Zertifikat (selbstsigniertes HTTPS) erreicht die App erst, wenn das
  Zertifikat in den Android-Einstellungen als CA-Zertifikat installiert ist (in den Einstellungen
  nach „Zertifikat“ suchen, dann „Zertifikat installieren“). Es gilt dann nur für diesen Server. Ein „trotzdem annehmen“ gibt es nicht.

## Bitte ausprobieren

- Anmelden, in der Bibliothek blättern, suchen, sortieren, filtern und Filme abspielen. Wenn
  vorhanden, auch Filme mit Ton in DTS, TrueHD oder AC3 und in 4K oder HDR.
- Serien: Die nächste Folge soll von selbst starten, und „Als gesehen markieren“ soll funktionieren.
- „Server und Konten“ (Menü): eine zweite Anmeldung hinzufügen, zwischen beiden wechseln, eine
  „Entfernen“.
- Auf Android TV: alles nur mit der Fernbedienung.
- Home während des Abspielens (ab Android 8) behält den Film in einem kleinen Fenster.
- Neu in 0.3.0: die Startseite mit Reihen (Weiterschauen, Favoriten, zuletzt hinzugefügt, Genres,
  am besten bewertet) und Kacheln „Meine Medien“ je Mediathek; „Startseite“ im Menü blendet die
  Reihen ein und aus und verschiebt sie.
- Neu in 0.3.0: die Filmseite auf dem Hintergrundbild, mit Laufzeit, Jahr, Altersfreigabe und
  Abzeichen für Bild, Dynamikumfang und Ton. Ein Dolby-Vision-Film auf einem Fernseher ohne Dolby
  Vision soll ein Bild zeigen.

## Fehler melden

- In der App: Menü → „Über“ → „Diagnose teilen“. Der Text nennt die Versionen, das Gerät, seine
  Dekoder und die letzten Protokollzeilen des Players. Er enthält keine Adresse, keinen
  Benutzernamen und kein Passwort, und du siehst ihn vor dem Teilen.
- Schreib dazu, was du getan hast, was du erwartet hast und was passiert ist. Schick es an die
  Person, von der du die App hast, oder eröffne ein Issue unter
  https://github.com/chicohaager/movies-cast/issues.
- Was die App speichert und mit wem sie spricht: [PRIVACY.md](../PRIVACY.md), in der App unter
  „Über“.
