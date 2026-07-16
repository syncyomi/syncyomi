/// <reference types="vitest/config" />
// Plugins
import vue from "@vitejs/plugin-vue";
import vuetify, { transformAssetUrls } from "vite-plugin-vuetify";

// Utilities
import { defineConfig } from "vite";

import { fileURLToPath, URL } from "node:url";

// https://vitejs.dev/config/
export default defineConfig({
  base: "",
  plugins: [
    vue({
      template: { transformAssetUrls },
    }),
    // https://github.com/vuetifyjs/vuetify-loader/tree/next/packages/vite-plugin
    vuetify({
      autoImport: true,
    }),
  ],
  define: { "process.env": {} },
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
    extensions: [".js", ".json", ".jsx", ".mjs", ".ts", ".tsx", ".vue"],
  },
  server: {
    port: 3000,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8282/",
        changeOrigin: true,
        secure: false,
      },
    },
  },
  build: {
    sourcemap: true,
    manifest: true,
  },
  test: {
    environment: "happy-dom",
    globals: false,
    // Keep vitest to unit/component tests; Playwright owns e2e/**.
    include: ["src/**/*.test.ts"],
    setupFiles: ["src/test/setup.ts"],
    server: {
      // Inline Vuetify so Vite transforms its per-component CSS imports; left
      // externalized, Node tries to load the .css files raw and throws.
      deps: { inline: ["vuetify"] },
    },
  },
});
