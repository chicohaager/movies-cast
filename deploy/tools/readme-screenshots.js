// README screenshots of the phone page and the TV page against the dev server with the demo library
// (README "Screenshots"): phone 390x844 @2x (Pixel-7-like), TV 1280x720, English.
// Usage, from the repository root:
//   MC_DEV_LIBRARY=../../docs/screenshots/demo MC_DEV_SERVER=127.0.0.1:18097 \
//     go test -count=1 ./internal/server/ -run '^TestDevServer$' -timeout 0 -v &   # demo library, 6 films
//   NODE_PATH=e2e/node_modules node deploy/tools/readme-screenshots.js <out-dir> [base-url]
//   for n in phone-login phone-library phone-film phone-player phone-status tv-pair tv-library; do
//     magick <out-dir>/$n.png -quality 82 docs/screenshots/$n.webp; done
const { chromium } = require("playwright");
const out = process.argv[2];
const base = process.argv[3] || "http://127.0.0.1:18097";

(async () => {
  const browser = await chromium.launch();
  const phoneCtx = await browser.newContext({
    viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, locale: "en-US",
  });
  const p = await phoneCtx.newPage();
  await p.goto(base + "/?lang=en");
  await p.locator("#login-view, #logout").locator("visible=true").first().waitFor();
  if (await p.locator("#logout").isVisible()) { await p.locator("#logout").click(); await p.goto(base + "/?lang=en"); }
  await p.locator('#login-form input[name="user"]').waitFor();
  await p.waitForTimeout(400);
  await p.screenshot({ path: `${out}/phone-login.png` });

  await p.locator('#login-form input[name="user"]').fill("Demo");
  await p.locator('#login-form input[name="password"]').fill("richtig");
  await p.locator('#login-form button[type="submit"]').click();
  await p.locator("#logout").waitFor({ state: "visible" });
  await p.locator("#list .tile").first().waitFor();
  await p.waitForTimeout(1500); // posters
  await p.screenshot({ path: `${out}/phone-library.png` });

  await p.locator("#list button", { hasText: "A Trip to the Moon" }).first().click();
  await p.locator("#detail").waitFor({ state: "visible" });
  await p.waitForTimeout(600);
  await p.screenshot({ path: `${out}/phone-film.png` });

  await p.locator("#d-start").click();
  await p.locator("#p-state").filter({ hasText: /playing/i }).waitFor({ timeout: 15000 });
  await p.waitForTimeout(2500);
  await p.screenshot({ path: `${out}/phone-player.png` });
  await p.locator("#p-stop, button:has-text('Stop')").first().click().catch(() => {});
  await p.waitForTimeout(800);

  await p.evaluate(() => window.scrollTo(0, 0));
  const sys = p.locator("#sys");
  await sys.evaluate((el) => { el.open = true; el.scrollIntoView({ block: "start" }); });
  await p.evaluate(() => window.scrollBy(0, -80));
  await p.waitForTimeout(600);
  await p.screenshot({ path: `${out}/phone-status.png` });

  // TV: pairing screen, then the library after the phone approves the code.
  const tvCtx = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: "en-US" });
  const tv = await tvCtx.newPage();
  await tv.goto(base + "/tv/?lang=en");
  const code = tv.locator("#pair-code");
  await code.filter({ hasText: /^\d{3} \d{3}$/ }).waitFor();
  await tv.waitForTimeout(600);
  await tv.screenshot({ path: `${out}/tv-pair.png` });
  const shown = (await code.textContent()).trim();
  await p.evaluate(() => window.scrollTo(0, 0));
  await p.locator("details.pair summary").first().click();
  await p.locator("#pair-code").fill(shown);
  await p.locator('#pair-form button[type="submit"]').click();
  await tv.locator("#lib").waitFor({ state: "visible", timeout: 15000 });
  await tv.waitForTimeout(1500);
  await tv.keyboard.press("ArrowRight");
  await tv.waitForTimeout(600);
  await tv.screenshot({ path: `${out}/tv-library.png` });

  await browser.close();
  console.log("shots done");
})().catch((e) => { console.error(e); process.exit(1); });
