<template>
  <v-container>
    <h1 class="text-center text-uppercase">Login</h1>
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
  </v-container>
</template>

<script lang="ts" setup>
import { useRouter } from "vue-router";
import { ref } from "vue";
import { useMutation } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";

interface LoginFormFields {
  username: string;
  password: string;
}

const router = useRouter();
const valid = ref<boolean>(false);
const username = ref<string>("");
const password = ref<string>("");

const rules = {
  required: (value: string) => !!value || "Required.",
};

const mutation = useMutation({
  mutationFn: async (values: LoginFormFields) => {
    await APIClient.auth.login(values.username, values.password);
  },
  onSuccess: () => {
    router.push("/");
  },
  onError: (error: any) => {
    console.log(error);
  },
});

const submit = () => {
  if (!valid.value) return;
  mutation.mutate({
    username: username.value,
    password: password.value,
  });
};
</script>

<style scoped></style>
