import path from "node:path";

// A dedicated port keeps the E2E backend clear of the dev server on 8282.
export const PORT = 8787;
export const BASE_URL = `http://127.0.0.1:${PORT}`;

export const DATA_DIR = path.resolve(__dirname, ".data").replace(/\\/g, "/");
