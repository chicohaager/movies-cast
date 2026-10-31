# Contributing

Movies-Cast is tested on few TVs. What helps most:

1. **A TV report** — your TV or box, what played with picture and sound, what did not. Use the issue
   template **"TV report"** and attach the diagnostics file (phone: "Status" → "Download diagnostics"): it lists
   per TV model the formats it played, refused and needed converted sound for — without addresses or titles.
2. **A translation check** — the 18 languages were written by machine; see
   [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md) and the template **"Translation"**.
3. **A bug** — template **"Bug"**: what you did, what you expected, what happened, the version (Status card)
   and the lines of the container log around that time (`docker logs movies-cast`). Please remove addresses
   and names you do not want public.

## Code

- `./check.sh` must pass: gofmt + vet, the Go tests, the browser tests (`e2e/`), the scan for private data.
- Every message the user sees is keyed and exists in all 18 language files (`internal/server/web/i18n/`).
- A new test should fail without the change it tests (break the code once, see it red, restore).
- Statements in docs and comments say what was measured, on which device and when; "not measured" is fine.

License: MIT ([LICENSE](LICENSE)).
