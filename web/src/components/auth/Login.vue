<template>
  <v-container class="d-flex fill-height">
    <v-row class="align-center justify-center">
      <v-col md="8" sm="12">
        <h1 class="text-center text-uppercase text-h2">SYNCYOMI</h1>
        <v-sheet
          class="pa-4 mx-auto mt-5"
          elevation="10"
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
          </v-container>
        </v-sheet>

        <v-snackbar
          v-model="snackbar"
          color="red"
          timeout="5000"
          variant="outlined"
        >
          {{ message }}
          <template v-slot:actions>
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

interface LoginFormFields {
  username: string;
  password: string;
}

const router = useRouter();
const valid = ref<boolean>(false);
const username = ref<string>("");
const password = ref<string>("");
const snackbar = ref<boolean>(false);
const message = ref<string>(
  "Login failed. Please check your username and password."
);
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
  onError: () => {
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
});
</script>

<style scoped></style>
