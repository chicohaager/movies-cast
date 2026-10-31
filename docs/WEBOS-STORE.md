# webOS app in the LG Content Store (roadmap 23)

Today the webOS app runs in developer mode only, renewed daily (see
[TECHNICAL.md](TECHNICAL.md#tv-app-webos-stage-2)). This page lists what is ready in the repository for a store
submission and what only the owner can do.

## Ready in the repository

- **A store build:** `STORE=1 deploy/webos/package.sh` packages the app with
  [`deploy/webos/store-index.html`](../deploy/webos/store-index.html) as its start page: one app for everyone,
  it asks for the address of Movies-Cast in the home network (`10.20.30.40` becomes
  `http://10.20.30.40:8097`), probes `/healthz` before it leaves the page (a webOS app has no shell that would
  catch a load error), keeps the address and connects to it on the next start; when the server does not
  answer it says so and tries again every 10 s. ES5 only (webOS 4.x runs Chrome 53). Checked in a browser on
  2026-10-31: a reachable address opens `/tv/`, an unreachable one shows "did not answer (no connection)" and
  keeps the focus in the address field; the stored address connects at once on the next start. Not tried on
  the TV yet (it would replace the developer-mode app of the same id).
- **Icons** in `deploy/webos/app/`: `icon.png` 80×80, `largeIcon.png` 130×130 (measured from the files); the
  512×512 logo in `deploy/webos/logo/`.
- **Screenshots** of the TV page (1280-wide WebP) in `docs/screenshots/` — the store's required sizes are not
  checked against them (below).

## What LG asks for

From LG's page [App Approval Process](https://webostv.developer.lge.com/distribute/app-approval-process)
(read 2026-10-31):

1. The app is submitted in **LG Seller Lounge** with the app package (IPK), images and supporting documents.
2. A **UX scenario** (template in Seller Lounge) helps LG's testers understand the app.
3. The **self-checklist** is mandatory, filled in with real test results; an app without it, or with too little
   in it, is rejected ([App Self Checklist](https://webostv.developer.lge.com/distribute/app-self-checklist)).
4. LG tests in three parts: pretest of the submitted information, function test, content test; a failed app
   comes back with a list of issues.
5. Every later change goes through approval again.

Not on that page, so not stated here: icon and screenshot sizes for the store listing, review time, fees.
Third-party guides name a 400×400 icon in Seller Lounge, a title of up to 95 characters and a review of 8–12
weeks ([EaselTV](https://docs.easeltv.com/docs/lg-tv-submission),
[LG developer forum](https://forum.webostv.developer.lge.com/t/error-related-to-icon-size-at-app-submission/4443))
— to be checked against Seller Lounge's own guide.

## Only the owner can do

- Create the **LG Seller Lounge** account (seller = the owner) and accept its terms.
- Decide the **app id** for the store (`deploy/webos/local.env` `APP_ID`; once published it cannot change).
- Fill in the UX scenario and the self-checklist after testing the store build on a TV.
- **LG's content test needs a server:** Movies-Cast shows nothing without a Movies, Jellyfin or Emby server in
  the tester's network. How LG tests such an app (a demo server reachable from LG, a video of the flow) is
  unknown — to be asked in Seller Lounge before the submission.
- Submit, answer LG's issue lists.
