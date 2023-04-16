<template>
  <v-container>
    <v-card>
      <v-toolbar color="primary">
        <v-toolbar-title>{{ toolbarTitle }}</v-toolbar-title>
      </v-toolbar>
      <div :class="isDesktop ? 'd-flex flex-row' : ''">
        <v-tabs
          v-model="tab"
          :direction="isDesktop ? 'vertical' : 'horizontal'"
          :grow="!isDesktop"
          color="primary"
        >
          <v-tab value="option-1">
            <v-icon start> mdi-application</v-icon>
            <span v-if="isDesktop"> Application </span>
          </v-tab>

          <v-tab value="option-2">
            <v-icon start> mdi-file-document-outline</v-icon>
            <span v-if="isDesktop"> Logs </span>
          </v-tab>

          <v-tab value="option-3">
            <v-icon start> mdi-bell-outline</v-icon>
            <span v-if="isDesktop"> Notifications </span>
          </v-tab>

          <v-tab value="option-4">
            <v-icon start> mdi-key</v-icon>
            <span v-if="isDesktop">Api Keys </span>
          </v-tab>
        </v-tabs>

        <v-container>
          <v-window v-model="tab">
            <v-window-item value="option-1">
              <Application />
            </v-window-item>

            <v-window-item value="option-2">
              <Logs />
            </v-window-item>

            <v-window-item value="option-3">
              <NotificationSettings />
            </v-window-item>

            <v-window-item value="option-4">
              <ApiKeySettings />
            </v-window-item>
          </v-window>
        </v-container>
      </div>
    </v-card>
  </v-container>
</template>

<script lang="ts" setup>
import Application from "@/components/settings/ApplicationSettings.vue";
import Logs from "@/components/settings/LogsSettings.vue";

import { computed, ref, watchEffect } from "vue";
import { useDisplay } from "vuetify";
import NotificationSettings from "@/components/settings/NotificationSettings.vue";
import ApiKeySettings from "@/components/settings/ApiKeySettings.vue";

const tab = ref("option-1");
const toolbarTitle = ref("User Profile");
const { width } = useDisplay();

const isDesktop = computed(() => {
  return width.value > 700;
});

// use watchEffect to switch toolbar title based on tab value.
watchEffect(() => {
  switch (tab.value) {
    case "option-1":
      toolbarTitle.value = "Application";
      break;
    case "option-2":
      toolbarTitle.value = "Logs";
      break;
    case "option-3":
      toolbarTitle.value = "Notifications";
      break;
    case "option-4":
      toolbarTitle.value = "Api Keys";
      break;
  }
});
</script>

<style scoped></style>
