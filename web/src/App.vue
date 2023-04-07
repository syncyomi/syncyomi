<template>
  <v-app>
    <v-main>
      <router-view name="navbar" />
      <router-view />
    </v-main>
  </v-app>
</template>

<script lang="ts" setup>
import { computed, ref, watchEffect } from "vue";
import { useRoute } from "vue-router";
import NavBar from "@/components/NavBar.vue";

const route = useRoute();
const showAppBar = ref(false);

const disableAppBarRoutes: string[] = ["Login", "Onboard"];

watchEffect(() => {
  showAppBar.value = !disableAppBarRoutes.includes(route.name as string);
});
</script>
