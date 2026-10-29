// The phone page in a browser against the dev server: login, colour mode and
// language per account (0.22, 0.23), the player (0.20). Each "device" is its
// own browser context — its own localStorage, like the LAN and the Tailscale
// address of the NAS. Accounts of the fake Movies: Demo and Anna, password
// "richtig".
const { test, expect } = require("@playwright/test");

// Logs in at "/" by default: the page then shows the device language
// (de-DE) and stores none; ?lang= would be a choice and go into the account.
async function login(page, user, path = "/") {
  await page.goto(path);
  // Someone is logged in already: log out first, as a person would.
  await expect(page.locator("#login-view, #logout").locator("visible=true").first()).toBeVisible();
  if (await page.locator("#logout").isVisible()) {
    await page.locator("#logout").click();
    await page.goto(path);
  }
  await page.locator('#login-form input[name="user"]').fill(user);
  await page.locator('#login-form input[name="password"]').fill("richtig");
  await page.locator('#login-form button[type="submit"]').click();
  await expect(page.locator("#logout")).toBeVisible();
}

// A new device: a fresh context with the project's settings (phone, de-DE).
async function device(browser) {
  const ctx = await browser.newContext(test.info().project.use);
  return { ctx, page: await ctx.newPage() };
}

function seconds(text) {
  const [h, m, s] = text.trim().split(":").map(Number);
  return h * 3600 + m * 60 + s;
}

test("colour mode follows the account, Log out forgets it on the device", async ({ browser }) => {
  const a = await device(browser);
  await login(a.page, "Demo");
  const stored = a.page.waitForResponse((r) => r.url().endsWith("/api/theme") && r.status() === 200);
  await a.page.locator("#theme").selectOption("dark");
  await stored;
  await expect(a.page.locator("html")).toHaveAttribute("data-theme", "dark");

  const b = await device(browser); // another address: an empty localStorage
  await login(b.page, "Demo");
  await expect(b.page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(b.page.locator("body")).toHaveCSS("background-color", "rgb(17, 19, 24)");
  await expect(b.page.locator("#theme")).toHaveValue("dark");

  await b.page.locator("#logout").click();
  await expect(b.page.locator("html")).not.toHaveAttribute("data-theme", /./);
  expect(await b.page.evaluate(() => localStorage.getItem("moviescast.theme"))).toBeNull();

  await login(b.page, "Anna"); // the next person on the same phone
  await expect(b.page.locator("html")).not.toHaveAttribute("data-theme", /./);
  expect(await b.page.evaluate(async () => (await (await fetch("/api/me")).json()).theme)).toBeUndefined();

  // Back to the system mode for Demo, so the other tests start clean.
  const reset = a.page.waitForResponse((r) => r.url().endsWith("/api/theme") && r.status() === 200);
  await a.page.locator("#theme").selectOption("");
  await reset;
  await a.ctx.close();
  await b.ctx.close();
});

test("language follows the account, the device language is never stored", async ({ browser }) => {
  const a = await device(browser);
  await login(a.page, "Anna"); // the device language (de-DE), not asked for
  expect(await a.page.evaluate(async () => (await (await fetch("/api/me")).json()).lang)).toBeUndefined();

  await login(a.page, "Demo");
  const stored = a.page.waitForResponse((r) => r.url().endsWith("/api/lang") && r.status() === 200);
  await a.page.locator("#lang").selectOption("fr");
  await stored;
  await expect(a.page.locator("#logout")).toHaveText("Déconnexion");

  const b = await device(browser);
  await login(b.page, "Demo");
  await expect(b.page).toHaveURL(/[?&]lang=fr\b/);
  await expect(b.page.locator("#logout")).toHaveText("Déconnexion");

  await b.page.locator("#logout").click();
  await expect(b.page).not.toHaveURL(/lang=/);
  expect(await b.page.evaluate(() => localStorage.getItem("moviescast.lang"))).toBeNull();

  // Back to German for Demo (?lang=de is asked for, so it is stored).
  const reset = a.page.waitForResponse((r) => r.url().endsWith("/api/lang") && r.status() === 200);
  await a.page.goto("/?lang=de");
  await reset;
  await a.ctx.close();
  await b.ctx.close();
});

test("player: play, pause, skip, resume, converted sound, stop", async ({ page }) => {
  await login(page, "Anna");
  await page.locator("#list button", { hasText: "Shelter" }).click();
  await page.locator("#d-start").click();

  const state = page.locator("#p-state");
  const toggle = page.locator("#p-toggle");
  const pos = page.locator("#p-pos");
  await expect(state).toHaveText("läuft", { timeout: 10_000 });
  await expect(toggle).toHaveText("Pause");

  await toggle.click();
  await expect(state).toHaveText("Pause");
  await expect(toggle).toHaveText("Weiter");
  const paused = seconds(await pos.textContent());
  await page.waitForTimeout(2_500);
  expect(seconds(await pos.textContent())).toBe(paused);

  await page.locator('button[data-skip="30"]').click();
  await expect.poll(async () => seconds(await pos.textContent())).toBe(paused + 30);

  await toggle.click();
  await expect(state).toHaveText("läuft");
  const before = seconds(await pos.textContent());
  await page.locator('button[data-skip="-30"]').click();
  await expect.poll(async () => seconds(await pos.textContent())).toBeLessThan(before - 20);

  const sound = page.locator("#p-sound");
  await expect(sound).toHaveText("Kein Ton?");
  await sound.click();
  await expect(page.locator("#notice")).toContainText("umgewandelt");
  await expect(sound).toHaveText("Originalton");
  await expect(state).toHaveText("läuft", { timeout: 10_000 });
  await sound.click(); // back to the original sound, so the next run starts clean
  await expect(sound).toHaveText("Kein Ton?");

  await page.locator("#p-stop").click();
  await expect(page.locator("#player")).toBeHidden();
  await expect(page.locator("#error")).toBeHidden();
});

// The film dialog's buttons ("Continue at 0:32:54", "Play from the start",
// "Close") fit on a narrow phone in every language: at 390 px the English
// labels were cut off (147 px of text in a 116 px button, 29.10.2026). Each
// language is asked for with ?lang=, which goes into Demo's account; Demo is set
// back to German at the end, as the language test leaves it.
test("film dialog: every button label fits at 360 px, in all languages", async ({ browser }) => {
  test.setTimeout(240_000); // 18 languages, a login each
  const fs = require("fs");
  const path = require("path");
  const codes = fs.readdirSync(path.join(__dirname, "../internal/server/web/i18n")).filter((f) => f.endsWith(".json")).map((f) => f.slice(0, -5));
  expect(codes.length).toBeGreaterThanOrEqual(18);
  const cut = [];
  for (const code of codes) {
    const ctx = await browser.newContext({ ...test.info().project.use, viewport: { width: 360, height: 780 } });
    const page = await ctx.newPage();
    await login(page, "Demo", `/?lang=${code}`);
    await expect(page.locator("html")).toHaveAttribute("lang", code.replace("_", "-"));
    await page.locator("#resume-list button").first().click();
    await expect(page.locator("#d-resume")).toBeVisible();
    for (const b of await page.locator("#detail .controls button:visible").all()) {
      const m = await b.evaluate((e) => ({ t: e.textContent.trim(), sw: e.scrollWidth, cw: e.clientWidth }));
      if (m.sw > m.cw + 1) cut.push(`${code}: "${m.t}" needs ${m.sw} px, has ${m.cw}`);
    }
    await page.keyboard.press("Escape"); // the dialog is modal: close it before Log out
    await expect(page.locator("#detail")).not.toBeVisible();
    await page.locator("#logout").click();
    await ctx.close();
  }
  const { ctx, page } = await device(browser);
  const reset = page.waitForResponse((r) => r.url().endsWith("/api/lang") && r.status() === 200);
  await login(page, "Demo", "/?lang=de");
  await reset;
  await ctx.close();
  expect(cut).toEqual([]);
});
