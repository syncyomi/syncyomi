<template>
  <v-app-bar app fixed>
    <v-toolbar-title class="text-capitalize" @click="$router.push('/')">
      TachiyomiSync
    </v-toolbar-title>

    <template v-if="showToolbarItems">
      <v-toolbar-items>
        <v-col>
          <v-btn
            height="100%"
            rounded
            size="large"
            to="/"
            variant="flat"
            width="100%"
          >
            Dashboard
          </v-btn>
        </v-col>

        <v-col>
          <v-btn
            height="100%"
            rounded
            size="large"
            to="logs"
            variant="flat"
            width="100%"
          >
            Logs
          </v-btn>
        </v-col>

        <v-col>
          <v-btn
            height="100%"
            rounded
            size="large"
            to="settings"
            variant="flat"
            width="100%"
          >
            Settings
          </v-btn>
        </v-col>
      </v-toolbar-items>
    </template>

    <v-spacer></v-spacer>
  </v-app-bar>

  <template v-if="!showToolbarItems">
    <v-bottom-navigation grow>
      <v-btn to="/" value="dashboard">
        <v-icon>mdi-view-dashboard</v-icon>
        Dashboard
      </v-btn>

      <v-btn to="logs" value="logs">
        <v-icon>mdi-text-box-multiple-outline</v-icon>
        Logs
      </v-btn>

      <v-btn to="settings" value="settings">
        <v-icon>mdi-cog-outline</v-icon>
        Settings
      </v-btn>
    </v-bottom-navigation>
  </template>
</template>

<script lang="ts" setup>
import { useDisplay } from "vuetify";
import { computed } from "vue";

const { width } = useDisplay();

const showToolbarItems = computed(() => {
  return width.value > 700;
});
</script>
