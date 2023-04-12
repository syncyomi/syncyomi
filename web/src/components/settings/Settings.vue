<template>
  <v-container>
    <v-card>
      <v-toolbar color="primary">
        <v-toolbar-title>{{ toolbarTitle }}</v-toolbar-title>
      </v-toolbar>
      <div :class="horizontalTabs ? 'd-flex flex-row' : ''">
        <v-tabs
          v-model="tab"
          :direction="horizontalTabs ? 'vertical' : 'horizontal'"
          color="primary"
        >
          <v-tab value="option-1">
            <v-icon start> mdi-application</v-icon>
            Application
          </v-tab>

          <v-tab value="option-2">
            <v-icon start> mdi-file-document-outline</v-icon>
            Logs
          </v-tab>

          <v-tab value="option-3">
            <v-icon start> mdi-bell-outline</v-icon>
            Notifications
          </v-tab>

          <v-tab value="option-4">
            <v-icon start> mdi-key</v-icon>
            Api Keys
          </v-tab>
        </v-tabs>

        <v-window v-model="tab">
          <v-window-item value="option-1">
            <v-container fluid>
              <h1>Application</h1>
            </v-container>
          </v-window-item>

          <v-window-item value="option-2">
            <v-container>
              <h1>Logs</h1>
            </v-container>
          </v-window-item>

          <v-window-item value="option-3">
            <v-container>
              <h1>Notifications</h1>
            </v-container>
          </v-window-item>

          <v-window-item value="option-4">
            <v-container>
              <h1>Api Keys</h1>
            </v-container>
          </v-window-item>
        </v-window>
      </div>
    </v-card>
  </v-container>
</template>

<script lang="ts" setup>
import { computed, ref, watchEffect } from "vue";
import { useDisplay } from "vuetify";

const tab = ref("option-1");
const toolbarTitle = ref("User Profile");
const { width } = useDisplay();

const horizontalTabs = computed(() => {
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
