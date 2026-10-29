// The phone page when things go wrong or overlap: a dialog closed with Esc,
// a slow TV, a TV list the server cannot give, a refusal with buttons while
// the status poll fails. Against the dev server (playwright.config.js).
const { test, expect } = require("@playwright/test");

async function login(page) {
  await page.goto("/");
  await expect(page.locator("#login-view, #logout").locator("visible=true").first()).toBeVisible();
  if (await page.locator("#logout").isVisible()) {
    await page.locator("#logout").click();
    await page.goto("/");
  }
  await page.locator('#login-form input[name="user"]').fill("Anna");
  await page.locator('#login-form input[name="password"]').fill("richtig");
  await page.locator('#login-form button[type="submit"]').click();
  await expect(page.locator("#logout")).toBeVisible();
  await expect(page.locator("#list button").first()).toBeVisible();
}

async function stopIfPlaying(page) {
  if (await page.locator("#p-stop").isVisible()) {
    await page.locator("#p-stop").click();
    await expect(page.locator("#player")).toBeHidden();
  }
}

// Esc (or Android back) closes the dialog without a button: returnValue kept
// the last button's value, and the next film started on the TV.
test("film dialog: Esc after an earlier Start plays nothing", async ({ page }) => {
  await login(page);
  const films = page.locator("#list button");
  const played = page.waitForResponse((r) => r.url().endsWith("/api/play"));
  await films.nth(0).click();
  await page.locator("#d-start").click();
  await played;
  await stopIfPlaying(page);

  const plays = [];
  page.on("request", (r) => { if (r.url().endsWith("/api/play")) plays.push(r.url()); });
  await films.nth(1).click();
  await expect(page.locator("#detail")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("#detail")).toBeHidden();
  await page.waitForTimeout(1_000);
  expect(plays, "Esc must not play").toEqual([]);
});

// A slow TV: /api/status can take 12 s on the server. Polled every 2 s
// without a guard, requests piled up and answered out of order.
test("status poll: one request at a time", async ({ page }) => {
  await login(page);
  let open = 0, most = 0;
  await page.route("**/api/status?*", async (route) => {
    open++; most = Math.max(most, open);
    await new Promise((res) => setTimeout(res, 5_000));
    open--;
    await route.continue().catch(() => {});
  });
  await page.waitForTimeout(7_000);
  expect(most, "status requests in flight at once").toBe(1);
  await page.unroute("**/api/status?*");
});

// No visible TV: the server answers 502, and the page kept the TV it had,
// so a film still went to it.
test("TV list fails: the old TV is not used any more", async ({ page }) => {
  await login(page);
  await page.route("**/api/renderers*", (route) =>
    route.fulfill({ status: 502, contentType: "application/json", body: JSON.stringify({ error: "kein Fernseher gefunden" }) }));
  const listed = page.waitForResponse((r) => r.url().includes("/api/renderers"));
  await page.locator("#refresh").click();
  await listed;
  const plays = [];
  page.on("request", (r) => { if (r.url().endsWith("/api/play")) plays.push(r.url()); });
  await page.locator("#list button").first().click();
  await page.locator("#d-start").click();
  await page.waitForTimeout(1_000);
  expect(plays, "a film went to a TV that is no longer listed").toEqual([]);
  await expect(page.locator("#error")).toBeVisible();
  await page.unroute("**/api/renderers*");
});

// A refusal shows its buttons (Play anyway, Convert sound …); a failing
// status poll called showError, which hid them within two seconds.
test("refusal buttons stay while the status poll fails", async ({ page }) => {
  await login(page);
  await page.route("**/api/play", (route) =>
    route.fulfill({ status: 409, contentType: "application/json", body: JSON.stringify({ error: "format.unlisted", unlisted: ["hevc"] }) }));
  await page.route("**/api/status?*", (route) =>
    route.fulfill({ status: 502, contentType: "application/json", body: JSON.stringify({ error: "Fernseher antwortet nicht" }) }));
  await page.locator("#list button").first().click();
  await page.locator("#d-start").click();
  await expect(page.locator("#force-row")).toBeVisible();
  await page.waitForTimeout(4_500);
  await expect(page.locator("#force-row")).toBeVisible();
  await page.unroute("**/api/play");
  await page.unroute("**/api/status?*");
});
