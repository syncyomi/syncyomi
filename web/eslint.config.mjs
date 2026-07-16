import js from "@eslint/js";
import globals from "globals";
import pluginVue from "eslint-plugin-vue";
import configPrettier from "eslint-config-prettier";
import { defineConfigWithVueTs, vueTsConfigs } from "@vue/eslint-config-typescript";

export default defineConfigWithVueTs(
  { ignores: ["dist/**"] },
  js.configs.recommended,
  pluginVue.configs["flat/recommended"],
  vueTsConfigs.recommended,
  {
    // postcss.config.js and friends are CommonJS
    files: ["**/*.{js,cjs}"],
    languageOptions: { globals: globals.node },
  },
  {
    rules: {
      "vue/multi-word-component-names": "off",
    },
  },
  // Last: turns off the stylistic rules so prettier owns formatting.
  configPrettier,
);
