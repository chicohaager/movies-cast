# Translations

movies-cast speaks the 18 languages the ZimaOS web UI ships: en, de, fr, es, it, nl, pl, cs, hu,
sv, tr, ru, ja, ko, zh_CN, zh_TW, pt_BR, pt_PT. English is the reference and the fallback.

**No native speaker has read any of them yet.** English and German were written with the code; the
other 16 were translated by AI agents (0.18) and, for the strings added in 0.19, 0.20 and 0.21, by the
author's AI assistant. If you speak one of these languages, a review is the most useful contribution
right now.

## Where the texts are

- `internal/server/web/i18n/<lang>.json` — one file per language, keys sorted.
- `{0}`, `{1}` are placeholders; keep them, in any order the sentence needs.
- `srv:<English text>` keys are messages from the server; the English key is the meaning.
- `go test ./internal/server/ -run Strings` checks that every language has every key, no empty text
  and the same placeholders as English. It does not check the language itself.

## Asked to be checked

From the translator of pl, cs, pt_BR and pt_PT (its own notes, 2026-10-28):

| Key | Language | Question |
|---|---|---|
| `pairtv.s2`, `pair.fromQR`, `play.viaApp` | pl | quotes are „…” (closing U+201D), the usual Polish form — right? |
| `pair.summary` | pl, cs, pt_BR, pt_PT | longer than the English "TV app": "Sparuj aplikację na telewizorze", "Spárovat aplikaci v televizi", "Parear o app da TV", "Emparelhar a app da TV" |
| all | pt_BR / pt_PT / pl / cs | form of address: pt_BR "você" imperative, pt_PT formal imperative (with PT-PT words: telemóvel, ecrã, ficheiro, palavra-passe, utilizador), pl informal imperative, cs formal ("Zadejte") |
| `m.added` | cs | "dotáže se při příštím hledání" reads awkwardly (no subject) — the meaning is "the TV is asked at the next search" |

From the other translators (summary only, their full notes were not kept):

| Key | Language | Question |
|---|---|---|
| bracket and quote style | ja | brackets are not used the same way in every string |
| `osd.hint` | all | names the remote's Back button; whether the word matches the label on an LG remote in that language is unchecked |

## Added after the first translation (0.19, 0.20) — every language

Written by the author's AI assistant, not by the translators above:

| Key | English |
|---|---|
| `tv.off` | `{0} – off` (a TV in standby, in the TV list) |
| `wake.waking` | `Switching on {0} …` |
| `wake.btn` | `Switch on and play` |
| `wake.noAnswer` | the TV did not answer the wake-up packet; names LG's "Mobile TV On" setting |
| `srv:the TV is off or not reachable` | |
| `srv:no MAC address is known for this TV` | |
| `srv:the wake-up packet could not be sent` | |
| `play.convert` | `Play with converted sound` |
| `p.noSound` | `No sound?` (a link in the player) |
| `p.original` | `Original sound` |
| `conv.on` | `The sound is converted for this TV (remembered).` |
| `srv:Movies could not convert the film` | |

"Mobile TV On" is left in English on purpose: it is the name of LG's setting. Whether LG uses that
name in every language's TV menu is unchecked.

## How to send a fix

Edit the JSON file and open a pull request, or open an issue with the key, the language and the
better wording. Please say whether you are a native speaker.
