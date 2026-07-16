// Provision a fresh config + sqlite dir for the E2E backend.
//
// This runs from the `test:e2e` script BEFORE `playwright test`, not from a
// Playwright globalSetup: Playwright starts `webServer` before globalSetup, so
// the config has to exist on disk before the server boots or it falls back to
// the 8282 defaults. Keep PORT in sync with e2e/constants.ts.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dirname = path.dirname(fileURLToPath(import.meta.url));
const DATA_DIR = path.resolve(dirname, ".data");
const PORT = 8787;

fs.rmSync(DATA_DIR, { recursive: true, force: true });
fs.mkdirSync(DATA_DIR, { recursive: true });

// A fresh dir means a fresh DB with no users, so onboarding is always available.
// The server only writes config.toml when it's missing, so ours pins the port.
fs.writeFileSync(
  path.join(DATA_DIR, "config.toml"),
  [
    `host = "127.0.0.1"`,
    `port = ${PORT}`,
    `sessionSecret = "e2e-secret"`,
    `databaseType = "sqlite"`,
    `checkForUpdates = false`,
    `logLevel = "ERROR"`, // quiet the piped server output; only errors surface
    "",
  ].join("\n")
);

console.log(`e2e: reset ${DATA_DIR} (port ${PORT})`);
