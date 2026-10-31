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

test("dark mode passes the contrast rules too", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/");
  await expect(page.locator("#login-view, #logout").locator("visible=true").first()).toBeVisible();
  await check(page, "dark");
});

test("TV page (pairing screen) passes axe", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: "de-DE" });
  const page = await ctx.newPage();
  await page.goto("/tv/");
  await expect(page.locator("#pair-code")).toHaveText(/[0-9]{3} [0-9]{3}/);
  await check(page, "tv pairing");
  await ctx.close();
});
