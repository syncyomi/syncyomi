<template>
  <v-app-bar app fixed>
    <v-toolbar-title class="text-capitalize" @click="$router.push('/settings')">
      <v-img
        class="d-none d-sm-block mx-1"
        height="40"
        max-height="40"
        max-width="50"
        src="@/assets/logo.png"
      ></v-img>
    </v-toolbar-title>

    <template v-if="showToolbarItems">
      <v-toolbar-items>
<!--        <v-col>-->
<!--          <v-btn-->
<!--            height="100%"-->
<!--            rounded-->
<!--            size="large"-->
<!--            to="/"-->
<!--            variant="flat"-->
<!--            width="100%"-->
<!--          >-->
<!--            Dashboard-->
<!--          </v-btn>-->
<!--        </v-col>-->

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

    <template v-if="showToolbarItems" v-slot:append>
      <v-btn icon="mdi-power" @click="mutation.mutate()"></v-btn>
    </template>
  </v-app-bar>

  <template v-if="!showToolbarItems">
    <v-bottom-navigation grow>
<!--      <v-btn to="/" value="dashboard">-->
<!--        <v-icon>mdi-view-dashboard</v-icon>-->
<!--        Dashboard-->
<!--      </v-btn>-->

      <v-btn to="logs" value="logs">
        <v-icon>mdi-text-box-multiple-outline</v-icon>
        Logs
      </v-btn>

      <v-btn to="settings" value="settings">
        <v-icon>mdi-cog-outline</v-icon>
        Settings
      </v-btn>

      <v-btn value="logout" @click="mutation.mutate()">
        <v-icon>mdi-logout-variant</v-icon>
        Logout
      </v-btn>
    </v-bottom-navigation>
  </template>

  <v-snackbar
    v-model="snackbar"
    :color="snackbarColor"
    timeout="10000"
    variant="tonal"
  >
    {{ snackbarMessage }}
    <template v-slot:actions>
      <v-btn color="orange" variant="text" @click="snackbar = false">
        Close
      </v-btn>
    </template>
  </v-snackbar>
</template>

<script lang="ts" setup>
import { useDisplay } from "vuetify";
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import { useMutation } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import { useAuthStore } from "@/store/auth/authStore";

const { width } = useDisplay();
const router = useRouter();
const authStore = useAuthStore();
const snackbar = ref<boolean>(false);
const snackbarMessage = ref<string>("Logout successful!.");
const snackbarColor = ref<string>("green");

const showToolbarItems = computed(() => {
  return width.value > 700;
});

const mutation = useMutation({
  mutationFn: async () => {
    await APIClient.auth.logout();
  },
  onSuccess: () => {
    authStore.logout();
    router.push({ name: "Login" });
  },
  onError: () => {
    snackbarColor.value = "red";
    snackbarMessage.value = "Logout failed. Please try again.";
    snackbar.value = true;
  },
});
</script>
