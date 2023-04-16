<template>
  <v-container>
    <v-row justify="end">
      <v-dialog
        v-model="dialog"
        :fullscreen="!isDesktop"
        :scrim="false"
        transition="dialog-bottom-transition"
      >
        <template v-slot:activator="{ props }">
          <v-btn color="primary" dark v-bind="props"> Add API Key</v-btn>
        </template>
        <v-card>
          <v-form
            v-model="valid"
            ref="form"
            @submit.prevent="handleSubmit(apiKey)"
          >
            <v-toolbar dark color="primary">
              <v-btn icon dark @click="dialog = false">
                <v-icon>mdi-close</v-icon>
              </v-btn>
              <v-toolbar-title>New API Key</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-toolbar-items>
                <v-btn variant="text" type="submit">Create</v-btn>
              </v-toolbar-items>
            </v-toolbar>
            <v-list subheader>
              <v-list-subheader>API Key Setting</v-list-subheader>
              <v-list-item>
                <v-text-field
                  v-model="apiKey.name"
                  :rules="[rules.required]"
                  aria-required="true"
                  dense
                  label="Name"
                  prepend-inner-icon="mdi-text"
                  variant="filled"
                ></v-text-field>
              </v-list-item>
            </v-list>
            <v-divider></v-divider>
          </v-form>
        </v-card>
      </v-dialog>
    </v-row>
  </v-container>
</template>

<script lang="ts" setup>
import { computed, ref, Ref } from "vue";
import { useMutation, useQueryClient } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import { useDisplay } from "vuetify";

const dialog: Ref<boolean> = ref(false);
const valid = ref<boolean>(false);
const form = ref();
const { width } = useDisplay();
const queryClient = useQueryClient();
const apiKey = ref({
  name: "",
  scopes: [],
});

const isDesktop = computed(() => {
  return width.value > 700;
});

const rules = {
  required: (value: string) => !!value || "Required.",
};

// create new api key
const createNewApiKey = useMutation({
  mutationFn: async (apikey: APIKey) => {
    await APIClient.apikeys.create(apikey);
  },
  onSuccess: () => {
    dialog.value = false;
    form.value.reset();
    queryClient.invalidateQueries(["apiKeys"]);
  },
  onError: (error) => {
    console.log("createMutation error", error);
  },
});

const handleSubmit = (data: unknown) => {
  if (form.value.validate()) {
    if (apiKey.value.name === "") {
      return;
    }

    createNewApiKey.mutate(data as APIKey);
  }
};
</script>

<style scoped></style>
