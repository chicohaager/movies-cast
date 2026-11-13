# Privacy

Movies-Cast has no account, no advertising, no analytics and no crash reporting. Its author runs no
server that the software talks to and receives nothing from it. This page says what each part
stores and which machines it talks to, so that the statement can be checked against the code.

## Movies-Cast for Android (`android/`, application id `io.github.chicohaager.moviescast`)

**Talks to:** the server whose address is typed on the login page (IceWhale Movies on a ZimaOS
box, or a Jellyfin server), and the other servers you logged in to and kept under "Servers and
accounts" when you change to one of them; only when you press "Search the network for servers", also to what
answers that search in the home network (below). To nothing else. The app carries no library for analytics,
advertising or crash reports (`android/app/build.gradle.kts` lists every dependency), and the list
of dependencies that Google Play can read out of an app is switched off (`dependenciesInfo`).

**"Search the network for servers"** (login page, only when you press it): sends the question
`who is JellyfinServer?` to the broadcast addresses of the networks the device is in and asks by
mDNS for ZimaOS boxes, then reads the public info page (product, name, version) of what answered.
Nothing of it leaves the home network, and no account data is part of it.

**Sends to that server:** user name and password once at the login; afterwards the token the
server answered with, a random installation id made on the first start, the device's model name and
the app's version (the `X-Emby-Authorization` header, as every Jellyfin client sends it); while a
film plays, the film's id and the position, so that "continue watching" is right on other devices;
"Mark as watched", "Add to favourites" and their opposites, when you press them.

**Stores on the device:** the address as typed, the installation id, the order chosen for the
library, and for every login kept under "Servers and accounts" the server's address and name, the
user name, the login token and which certificate the server is to show (the word "system", or the
SHA-256 fingerprint of a certificate you installed in Android's settings). Each token is encrypted with a key of the Android keystore that does not leave the device;
the password is not stored. The app takes no part in Android's backup (`allowBackup="false"`).
"Log out" deletes the token of the login in use on the device and at the server, "Remove" under
"Servers and accounts" does it for any other; uninstalling the app deletes all of it.

**Permissions:** Internet, and from Android 17 "Local network access", without which a server in
the home network cannot be reached. No location, contacts, microphone, camera or storage.

**"Share diagnostics"** (About, in the menu and on the login page): builds a text with the app
version, the Android version, the device model, the server's product name and version, the names of
the device's decoders and the player's last log lines (decoders, formats, error codes). The app
shows the text first; "Share" hands it to Android's share sheet, and nothing leaves the device
unless you pick an app there and send it. The text holds no server address, no user name, no token
and no film title; addresses are removed from every line in addition (`LogBook.scrub`).

## Movies-Cast server (the container, `cmd/movies-cast`)

Runs in your home network and talks to the Movies, Jellyfin or Emby server you configure and to the
TVs and Cast devices it finds there. What it keeps across restarts (the logins of browsers, the
sound formats a TV proved to play, the chosen theme) are files on your box next to `STATE_FILE`
(see [docs/TECHNICAL.md](docs/TECHNICAL.md)).

The code names one address outside your network (`api.github.com`), for the update notice, which
is off unless you switch it on (`UPDATE_CHECK=true`). It then asks GitHub once a day for the latest
release of this repository; GitHub sees your address, as with any visit to GitHub.

## TV apps (LG webOS, Android TV test build)

Both open the pages of your Movies-Cast server and keep its address on the TV.

## Questions

Open an issue at <https://github.com/chicohaager/movies-cast/issues>.
