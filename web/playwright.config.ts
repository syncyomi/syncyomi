import { defineConfig } from "@playwright/test";
import path from "node:path";
import { BASE_URL, DATA_DIR } from "./e2e/constants";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["html", { open: "never" }], ["list"]] : "list",
  // Config + fresh DB are provisioned by `node e2e/reset.mjs` in the test:e2e
  // script, which must run before the webServer boots (Playwright starts the
  // webServer before any globalSetup).
  use: {
    baseURL: BASE_URL,
    trace: "on-first-retry",
  },
  webServer: {
    command: `go run . --config "${DATA_DIR}"`,
    cwd: path.resolve(__dirname, ".."),
    url: BASE_URL,
    // Always start fresh: reusing a server would keep a stale config/DB, and the
    // reset script has already wiped the data dir for this run.
    reuseExistingServer: false,
    timeout: 120_000, // first `go run` cold-compiles
    stdout: "pipe",
    stderr: "pipe",
  },
});
