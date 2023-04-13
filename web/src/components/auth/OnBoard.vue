<template>
  <v-container class="d-flex fill-height">
    <v-row class="align-center justify-center">
      <v-col md="8" sm="12">
        <h1 class="text-center text-uppercase">SyncYomi</h1>
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
                    prepend-inner-icon="mdi-account-outline"
                    rounded
                    variant="outlined"
                  ></v-text-field>
                </v-col>
                <v-col cols="12">
                  <v-text-field
                    v-model="password"
                    :rules="[rules.required, rules.min]"
                    dense
                    label="Password"
                    prepend-inner-icon="mdi-lock-outline"
                    rounded
                    type="password"
                    variant="outlined"
                  ></v-text-field>
                </v-col>

                <v-col>
                  <v-text-field
                    v-model="passwordConfirm"
                    :rules="[rules.required, rules.min, rules.match]"
                    dense
                    label="Confirm Password"
                    prepend-inner-icon="mdi-lock-outline"
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
                  color="primary"
                  rounded
                  type="submit"
                  variant="flat"
                  width="100%"
                >
                  Create account
                </v-btn>
              </div>
            </v-form>
          </v-container>
        </v-sheet>
        <v-snackbar
          v-model="snackbar"
          color="red"
          multi-line
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
import {ref} from "vue";
import {useMutation} from "@tanstack/vue-query";
import {APIClient} from "@/api/APIClient";
import {useRouter} from "vue-router";
import {useAuthStore} from "@/store/auth/authStore";

interface InputValues {
  username: string;
  password1: string;
  password2: string;
}

const router = useRouter();
const valid = ref<boolean>(false);
const username = ref<string>("");
const password = ref<string>("");
const passwordConfirm = ref<string>("");
const snackbar = ref<boolean>(false);
const message = ref<string>(
  "Failed to create account! currently only one account is allowed, make sure browser cookies are cleared."
);
const authStore = useAuthStore();

const rules = {
  required: (value: string) => !!value || "Required.",
  min: (v: string) => v.length >= 8 || "Min 8 characters",
  match: (v: string) => v === password.value || "Passwords must match",
};

const mutation = useMutation({
  mutationFn: async (values: InputValues) => {
    await APIClient.auth.onboard(values.username, values.password1);
  },
  onSuccess: (_, variables: InputValues) => {
    authStore.login(variables.username);
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
    password1: password.value,
    password2: passwordConfirm.value,
  });
};
</script>

<style scoped></style>
