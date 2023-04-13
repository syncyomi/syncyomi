<template>
  <v-app>
    <v-main>
      <router-view name="navbar" />
      <router-view />
    </v-main>
  </v-app>
</template>

<script lang="ts" setup>
import { useTheme } from "vuetify";

const theme = useTheme();

// Set theme based on local storage value
const themePreference = localStorage.getItem("theme");
theme.global.name.value = themePreference === "light" ? "light" : "dark";
</script>
