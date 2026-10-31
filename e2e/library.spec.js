// Search, sort and filter of the library on the phone (roadmap 13). The dev
// server's fake Movies serves the four films captured from Movies 0.3.0 with
// their real genres and years; it has no series, music or photos, so the
// tabs stay hidden (Movies has films only, measured 2026-10-31).
const { test, expect } = require("@playwright/test");

async function login(page) {
  await page.goto("/");
  await expect(page.locator("#login-view, #logout").locator("visible=true").first()).toBeVisible();
  if (!(await page.locator("#logout").isVisible())) {
    await page.locator('#login-form input[name="user"]').fill("Demo");
    await page.locator('#login-form input[name="password"]').fill("richtig");
    await page.locator('#login-form button[type="submit"]').click();
  }
  await expect(page.locator("#list .tile")).toHaveCount(4);
}

const names = (page) => page.locator("#list .tile .cap").evaluateAll((els) => els.map((e) => e.firstChild.textContent));

test("library: search, filter by state and genre, sort by year", async ({ page }) => {
  await login(page);
  await expect(page.locator("#tabs")).toBeHidden();
  await expect(page.locator("#count")).toHaveText("4 von 4");

  await page.locator("#q").fill("shel");
  await expect(page.locator("#list .tile")).toHaveCount(1);
  expect(await names(page)).toEqual(["Shelter"]);
  await expect(page.locator("#count")).toHaveText("1 von 4");
  await expect(page.locator("#resume-wrap")).toBeHidden(); // a search shows its hits only

  await page.locator("#q").fill("zzz");
  await expect(page.locator("#list .tile")).toHaveCount(0);
  await expect(page.locator("#empty")).toHaveText("Nichts passt zu Suche oder Filter.");
  await page.locator("#q").fill("");

  await page.locator("#filter").selectOption("started");
  expect(await names(page)).toEqual(["Avatar: Fire and Ash"]);
  await page.locator("#filter").selectOption({ label: "Genre: Comedy" });
  expect(await names(page)).toEqual(["Glennkill: Ein Schafskrimi"]);
  await page.locator("#filter").selectOption("");

  await page.locator("#sort").selectOption("year");
  const byYear = await names(page);
  expect(byYear.slice(0, 2)).toEqual(["Glennkill: Ein Schafskrimi", "Shelter"]); // 2026, then by name
  expect(byYear.slice(2)).toEqual(["Avatar: Fire and Ash", "Mission: Impossible - The Final Reckoning"]);
  await page.locator("#sort").selectOption("name");
});
