// Browser tests of the phone page. Playwright starts the dev server
// (TestDevServer: the real handler against the fake Movies and fake TV of the
// Go tests) and stops it afterwards. One worker: the tests share the dev
// server's accounts (Demo, Anna), so they run one after the other.
const { defineConfig, devices } = require("@playwright/test");

const port = 18098;

module.exports = defineConfig({
  testDir: ".",
  workers: 1,
  fullyParallel: false,
  retries: 0,
  reporter: [["list"]],
  use: {
    ...devices["Pixel 7"],
    baseURL: `http://127.0.0.1:${port}`,
    locale: "de-DE",
    colorScheme: "light",
    trace: "retain-on-failure",
  },
  webServer: {
    command: `go test ./internal/server/ -run '^TestDevServer$' -count=1 -timeout 0`,
    cwd: "..",
    env: { MC_DEV_SERVER: `127.0.0.1:${port}` },
    url: `http://127.0.0.1:${port}/healthz`,
    reuseExistingServer: false,
    timeout: 120_000,
    gracefulShutdown: { signal: "SIGTERM", timeout: 5_000 },
  },
});
