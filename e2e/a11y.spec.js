// Accessibility (roadmap 25): axe-core checks the phone page in its states —
// login, library with the filter bar, the film dialog with subtitles and
// queue, the player card — and the TV page's library. Every rule axe tags
// WCAG 2.0/2.1 A and AA must pass.
const { test, expect } = require("@playwright/test");
const AxeBuilder = require("@axe-core/playwright").default;

const tags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];

async function check(page, name) {
  const r = await new AxeBuilder({ page }).withTags(tags).analyze();
  const found = r.violations.map((v) => `${v.id} (${v.impact}): ${v.nodes.map((n) => n.target.join(" ")).slice(0, 4).join(", ")}`);
  expect(found, `${name}: ${found.join("\n")}`).toEqual([]);
}

// axe cannot measure text on a gradient (it reports such a button as "incomplete", not as a violation; the
// gradient buttons of the dark mode stayed green with an unreadable text colour, 2026-11-13). Here every visible
// button with a gradient has its text colour measured against each colour stop: at least 4.5 to 1 (WCAG AA).
async function checkGradientButtons(page, name) {
  const bad = await page.evaluate(() => {
    const lum = ([r, g, b]) => [r, g, b].map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); })
      .reduce((s, v, i) => s + v * [0.2126, 0.7152, 0.0722][i], 0);
    const rgb = (t) => t.match(/[\d.]+/g).slice(0, 3).map(Number);
    const out = [];
    let seen = 0;
    for (const b of document.querySelectorAll("button")) {
      const st = getComputedStyle(b);
      if (!st.backgroundImage.includes("gradient") || b.getBoundingClientRect().width === 0) continue;
      seen++;
      const fg = lum(rgb(st.color));
      for (const stop of st.backgroundImage.match(/rgba?\([^)]*\)/g) || []) {
        const bg = lum(rgb(stop));
        const ratio = (Math.max(fg, bg) + 0.05) / (Math.min(fg, bg) + 0.05);
        if (ratio < 4.5) out.push(`${b.textContent.trim()}: ${st.color} on ${stop} = ${ratio.toFixed(2)}`);
      }
    }
    return { seen, out };
  });
  expect(bad.out, `${name}: ${bad.out.join("\n")}`).toEqual([]);
  return bad.seen;
}

test("phone page: login, library, film dialog pass axe (WCAG 2.1 AA)", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("#login-view, #logout").locator("visible=true").first()).toBeVisible();
  if (await page.locator("#logout").isVisible()) {
    await page.locator("#logout").click();
    await page.goto("/");
  }
  await expect(page.locator("#login-form")).toBeVisible();
  await check(page, "login");
  await page.locator('#login-form input[name="user"]').fill("Demo");
  await page.locator('#login-form input[name="password"]').fill("richtig");
  await page.locator('#login-form button[type="submit"]').click();
  await expect(page.locator("#list .tile")).toHaveCount(4);
  await check(page, "library");
  await page.locator("#list .tile").first().click();
  await expect(page.locator("#detail")).toBeVisible();
  await check(page, "film dialog");
  await page.keyboard.press("Escape");
  for (const d of ["#manage", "#sys", "details.pair"]) await page.locator(d).first().evaluate((el) => { el.open = true; });
  await check(page, "cards open");
});

// The page is always dark (Raumklang's near-black and gold, 2026-11-20); the test above sees it with the phone in
// light mode, this one with the phone in dark mode: the login page, the library and the film dialog.
test("a phone in dark mode passes the contrast rules too", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/");
  await expect(page.locator("#login-view, #logout").locator("visible=true").first()).toBeVisible();
  if (await page.locator("#logout").isVisible()) {
    await page.locator("#logout").click();
    await page.goto("/");
  }
  await expect(page.locator("#login-form")).toBeVisible();
  await check(page, "dark login");
  await page.locator('#login-form input[name="user"]').fill("Demo");
  await page.locator('#login-form input[name="password"]').fill("richtig");
  await page.locator('#login-form button[type="submit"]').click();
  await expect(page.locator("#list .tile")).toHaveCount(4);
  await check(page, "dark library");
  await page.locator("#list .tile").first().click();
  await expect(page.locator("#detail")).toBeVisible();
  await check(page, "dark film dialog");
  // the dialog's play buttons carry the gradient: there must be some to measure
  expect(await checkGradientButtons(page, "dark film dialog")).toBeGreaterThan(0);
  // the player card (Stop is the red button there)
  await page.locator("#d-start").click();
  await expect(page.locator("#p-toggle")).toBeVisible({ timeout: 10_000 });
  await check(page, "dark player");
  await page.locator("#p-stop").click();
});

test("TV page (pairing screen) passes axe", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: "de-DE" });
  const page = await ctx.newPage();
  await page.goto("/tv/");
  await expect(page.locator("#pair-code")).toHaveText(/[0-9]{3} [0-9]{3}/);
  await check(page, "tv pairing");
  await ctx.close();
});
