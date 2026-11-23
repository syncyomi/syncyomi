<template>
  <v-container class="fill-height d-flex align-center flex-wrap">
    <v-row class="align-center justify-center">
      <v-col md="8" sm="12">
        <h1 class="text-center text-uppercase text-display-large">SYNCYOMI</h1>
        <v-sheet
          class="pa-4 mx-auto mt-5"
          elevation="5"
          max-width="600"
          rounded="lg"
          width="100%"
        >
          <v-container>
            <v-form v-model="valid" @submit.prevent="submit()">
              <v-row>
                <v-col cols="12">
                  <v-text-field
                    v-model="username"
                    :rules="[rules.required]"
                    dense
                    label="Username"
                    prepend-inner-icon="mdi-account"
                    rounded
                    variant="outlined"
                  ></v-text-field>
                </v-col>
                <v-col cols="12">
                  <v-text-field
                    v-model="password"
                    :rules="[rules.required]"
                    dense
                    label="Password"
                    prepend-inner-icon="mdi-lock"
                    rounded
                    type="password"
                    variant="outlined"
                  ></v-text-field>
                </v-col>
              </v-row>

              <v-divider class="mb-4"></v-divider>

              <div class="text-end">
                <v-btn
                  block
                  class="text-uppercase"
                  color="green"
                  rounded
                  type="submit"
                  variant="flat"
                >
                  Login
                </v-btn>
              </div>
            </v-form>

            <template v-if="passkeysEnabled">
              <div class="d-flex align-center my-4">
                <v-divider></v-divider>
                <span class="mx-3 text-medium-emphasis">or</span>
                <v-divider></v-divider>
              </div>

              <v-btn
                :loading="passkeyMutation.isPending.value"
                block
                class="text-uppercase"
                color="primary"
                prepend-icon="mdi-fingerprint"
                rounded
                variant="outlined"
                @click="passkeyMutation.mutate()"
              >
                Sign in with passkey
              </v-btn>
            </template>
          </v-container>
        </v-sheet>

        <v-snackbar
          v-model="snackbar"
          color="red"
          timeout="5000"
          variant="outlined"
        >
          {{ message }}
          <template #actions>
            <v-btn color="orange" variant="text" @click="snackbar = false">
              Close
            </v-btn>
          </template>
        </v-snackbar>
      </v-col>
    </v-row>
  </v-container>
</template>

<script lang="ts" setup>
import { useRouter } from "vue-router";
import { onMounted, ref } from "vue";
import { useMutation } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import { useAuthStore } from "@/store/auth/authStore";
import { passkeysAvailable, signInWithPasskey } from "@/utils/passkeys";
import {
  LOGIN_FAILED_MESSAGE,
  loginErrorMessage,
  passkeyErrorMessage,
} from "./loginError";

interface LoginFormFields {
  username: string;
  password: string;
}

const router = useRouter();
const valid = ref<boolean>(false);
const username = ref<string>("");
const password = ref<string>("");
const snackbar = ref<boolean>(false);
const message = ref<string>(LOGIN_FAILED_MESSAGE);
const store = useAuthStore();

const rules = {
  required: (value: string) => !!value || "Required.",
};

const mutation = useMutation({
  mutationFn: async (values: LoginFormFields) => {
    await APIClient.auth.login(values.username, values.password);
  },
  onSuccess: (_, variables: LoginFormFields) => {
    store.login(variables.username);
    router.push("/");
  },
  onError: (error) => {
    message.value = loginErrorMessage(error);
    snackbar.value = true;
  },
});

const passkeysEnabled = ref<boolean>(false);

const passkeyMutation = useMutation({
  mutationFn: signInWithPasskey,
  onSuccess: (signedInUser: string) => {
    store.login(signedInUser);
    router.push("/");
  },
  onError: (error) => {
    message.value = passkeyErrorMessage(error);
    snackbar.value = true;
  },
});

const submit = () => {
  if (!valid.value) return;
  mutation.mutate({
    username: username.value,
    password: password.value,
  });
};

onMounted(async () => {
  try {
    const canOnboard = await APIClient.auth.canOnboard();
    if (canOnboard) {
      await router.push({ name: "Onboard" });
    }
  } catch (error) {
    console.error("Error checking onboarding availability:", error);
  }

  try {
    const { passkeys } = await APIClient.auth.capabilities();
    passkeysEnabled.value = passkeys && passkeysAvailable();
  } catch (error) {
    console.error("Error checking sign-in methods:", error);
  }
});
</script>

<style scoped></style>
