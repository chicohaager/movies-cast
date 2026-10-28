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

With 0.16 on .143 the search found both TVs with `RENDERERS` empty (3 of 3 searches, 0 drops).

## Next

| # | What | Effort | Note |
|---|---|---|---|
| 4 | one-click install from an app store | 2–3 h | needs the repo public (owner's decision) and a test by the owner before any store entry |
| 6 | transcode only for TVs that cannot play the format | 3–4 h | Movies can transcode; costs CPU while the film runs |
| 8 | audio-format hint like the picture hint | 1 h | Mission: Impossible has TrueHD only; whether the TVs play it via DLNA is unknown — test first |
| 9 | wake the TV (Wake-on-LAN) | 1 h | whether the LG wakes from standby by WoL is unknown — test first |
| 11 | one login per person | — | only if more than one person uses it |

## Measured and parked

- **Login through the ZimaOS session (5):** the ZimaOS UI keeps its token in `localStorage.access_token`
  on its own origin (ZFW `internal/auth/auth.go`); movies-cast on `:8097` is another origin and cannot
  read it. Single sign-on would mean running movies-cast as a ZimaOS module behind the gateway (like
  ZFW). Whether Movies accepts a ZimaOS token instead of a password is unknown — measurable with a real
  ZimaOS token against Movies.
- **Subtitles via DLNA on Samsung (7):** Samsung has its own extension for external subtitles; whether
  the UE50MU6179 supports it is unknown. Not needed by the owner.
