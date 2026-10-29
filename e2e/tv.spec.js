// The TV page (web/tv/, run by the webOS app) in a desktop Chromium at
// 1920x1080: pairing with a code approved on the phone, the library, and the
// remote's keys (arrows, OK, Back). Playback in the TV's <video> is not
// covered: the fake Movies serves no decodable film. Chromium is not the
// LG's Chrome 53 either — TestTVAssetsFitChrome53 (Go) guards the syntax.
const { test, expect } = require("@playwright/test");

test("TV: pair with a code from the phone, library, remote keys", async ({ browser }) => {
  const tvCtx = await browser.newContext({ viewport: { width: 1920, height: 1080 }, locale: "de-DE" });
  const tv = await tvCtx.newPage();
  await tv.goto("/tv/");
  await expect(tv.locator("#pair")).toBeVisible();
  const code = tv.locator("#pair-code");
  await expect(code).toHaveText(/^\d{3} \d{3}$/);
  const shown = (await code.textContent()).trim();

  const phoneCtx = await browser.newContext(test.info().project.use);
  const phone = await phoneCtx.newPage();
  await phone.goto("/");
  await phone.locator('#login-form input[name="user"]').fill("Demo");
  await phone.locator('#login-form input[name="password"]').fill("richtig");
  await phone.locator('#login-form button[type="submit"]').click();
  await expect(phone.locator("#logout")).toBeVisible();
  await phone.locator("details.pair summary").first().click();
  await phone.locator("#pair-code").fill(shown);
  const approved = phone.waitForResponse((r) => r.url().endsWith("/api/tv/pair/approve") && r.status() === 200);
  await phone.locator('#pair-form button[type="submit"]').click();
  await approved;

  await expect(tv.locator("#lib")).toBeVisible({ timeout: 10_000 });
  await expect(tv.locator("#lib-user")).toContainText("Demo");
  await expect(tv.locator("#grid .focusable")).toHaveCount(4);

  // Remote: the focus moves with the arrows, OK opens the film, Back returns.
  await tv.keyboard.press("ArrowRight");
  const focused = tv.locator("#lib .focus");
  await expect(focused).toHaveCount(1);
  const id = await focused.getAttribute("data-id");
  await tv.keyboard.press("Enter");
  await expect(tv.locator("#detail")).toBeVisible();
  await expect(tv.locator("#d-title")).not.toBeEmpty();
  await expect(tv.locator("#d-start")).toBeVisible();
  await tv.keyboard.press("Escape");
  await expect(tv.locator("#lib")).toBeVisible();
  await expect(tv.locator("#lib .focus")).toHaveAttribute("data-id", id); // back on the same film

  await tvCtx.close();
  await phoneCtx.close();
});
