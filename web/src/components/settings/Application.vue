<template>
  <v-card :loading="isLoading" variant="flat">
    <v-card-title>Application</v-card-title>
    <v-card-subtitle class="mb-3">
      Change your application settings in config.toml and restart to take
      effect.
    </v-card-subtitle>

    <template v-slot:loader>
      <v-progress-linear
        :active="isLoading"
        color="primary"
        height="4"
        indeterminate
      ></v-progress-linear>
    </template>
    <v-divider></v-divider>

    <div v-if="!isLoading">
      <v-card-item>
        <v-form v-if="data" readonly>
          <v-row>
            <v-col cols="12" lg="4">
              <v-text-field
                v-model="data.host"
                color="primary"
                label="Host"
                variant="underlined"
              ></v-text-field>
            </v-col>

            <v-col cols="12" lg="4">
              <v-text-field
                v-model="data.port"
                color="primary"
                label="Port"
                variant="underlined"
              ></v-text-field>
            </v-col>

            <v-col cols="12" lg="4">
              <v-text-field
                v-model="data.base_url"
                color="primary"
                label="Base Url"
                variant="underlined"
              ></v-text-field>
            </v-col>
          </v-row>
        </v-form>
      </v-card-item>
      <v-divider></v-divider>

      <v-card-item>
        <v-row>
          <v-col cols="12">
            <v-row>
              <v-col class="text-h6 font-weight-bold"> Version:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-chip
                  :color="data?.version == 'dev' ? 'warning' : ''"
                  class="mx-1"
                  variant="tonal"
                >
                  <span class="text-uppercase">{{ data?.version }} </span>
                </v-chip>

                <v-chip
                  v-if="updateData && updateData.html_url"
                  :color="updateData ? 'warning' : 'success'"
                  variant="tonal"
                >
                  <a
                    :href="updateDataUrl"
                    rel="noopener noreferrer"
                    target="_blank"
                  >
                    {{ `${updateData?.name} available!` }}
                  </a>
                </v-chip>
              </v-col>
            </v-row>
          </v-col>
        </v-row>
      </v-card-item>
      <v-divider></v-divider>

      <v-card-item>
        <v-row>
          <v-col cols="12">
            <v-row>
              <v-col class="text-h6 font-weight-bold"> Commit:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-chip
                  :color="data?.version == 'dev' ? 'warning' : 'primary'"
                  variant="tonal"
                >
                  <span>{{ data?.commit || "dev" }} </span>
                </v-chip>
              </v-col>
            </v-row>
          </v-col>
        </v-row>
      </v-card-item>
      <v-divider></v-divider>

      <v-card-item>
        <v-row>
          <v-col cols="12">
            <v-row>
              <v-col class="text-h6 font-weight-bold"> Build date:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-chip
                  :color="data?.version == 'dev' ? 'warning' : 'primary'"
                  variant="tonal"
                >
                  <span class="text-uppercase">{{ data?.date || "dev" }} </span>
                </v-chip>
              </v-col>
            </v-row>
          </v-col>
        </v-row>
      </v-card-item>
      <v-divider></v-divider>

      <v-card-item>
        <v-row>
          <v-col cols="12">
            <v-row class="align-center">
              <v-col>
                <span class="text-h6 font-weight-bold">Updates</span>
                <v-row>
                  <v-col class="text-subtitle-2">
                    Get notified of new updates.
                  </v-col>
                </v-row>
              </v-col>
              <v-col class="d-flex justify-end">
                <div class="mr-1">
                  <v-switch
                    :model-value="data?.check_for_updates"
                    class="text-center"
                    color="success"
                    hide-details
                    @change="
                      toggleCheckUpdateMutation.mutate($event.target.checked)
                    "
                  ></v-switch>
                </div>
              </v-col>
            </v-row>
          </v-col>
        </v-row>
      </v-card-item>
      <v-divider></v-divider>

      <v-card-item>
        <v-row>
          <v-col cols="12">
            <v-row class="align-center">
              <v-col>
                <span class="text-h6 font-weight-bold">Theme</span>
                <v-row>
                  <v-col class="text-subtitle-2">
                    Switch between dark and light theme.
                  </v-col>
                </v-row>
              </v-col>
              <v-col class="d-flex justify-end">
                <div class="mr-1">
                  <v-switch
                    v-model="toggleTheme"
                    class="text-center"
                    color="black"
                    hide-details
                  ></v-switch>
                </div>
              </v-col>
            </v-row>
          </v-col>
        </v-row>
      </v-card-item>
    </div>

    <v-snackbar
      v-model="snackbarVisible"
      :color="snackbarColor"
      :timeout="3000"
      variant="tonal"
    >
      {{ snackbarMessage }}
    </v-snackbar>
  </v-card>
</template>

<script lang="ts" setup>
import {useMutation, useQuery, useQueryClient} from "@tanstack/vue-query";
import {APIClient} from "@/api/APIClient";
import {computed, Ref, ref, watch} from "vue";
import {useTheme} from "vuetify";

const toggleTheme: Ref<boolean> = ref(true);
const snackbarVisible: Ref<boolean> = ref(false);
const snackbarMessage: Ref<string> = ref("Config updated successfully!");
const snackbarColor: Ref<string> = ref("success");
const theme = useTheme();

// Get QueryClient from context
const queryClient = useQueryClient();

const localStoredTheme = localStorage.getItem("theme");
toggleTheme.value = localStoredTheme !== "light";

const updateDataUrl = computed(() => {
  return updateData?.value?.url || "#";
});

watch(toggleTheme, (newValue) => {
  if (newValue) {
    theme.global.name.value = "dark";
    localStorage.removeItem("theme");
  } else {
    theme.global.name.value = "light";
    localStorage.setItem("theme", "light");
  }
});

const { isLoading, data } = useQuery({
  queryKey: ["config"],
  queryFn: () => APIClient.config.get(),
  retry: false,
  refetchOnWindowFocus: false,
  onError: (error) => {
    console.log(error);
  },
});

const toggleCheckUpdateMutation = useMutation({
  mutationFn: (val: boolean) =>
    APIClient.config.update({
      check_for_updates: val,
    }),
  onSuccess: () => {
    queryClient.invalidateQueries(["config"]);
    checkUpdateMutation.mutate();
    snackbarVisible.value = true;
  },
  onError: () => {
    snackbarVisible.value = true;
    snackbarColor.value = "error";
    snackbarMessage.value = "Error updating config!";
  },
});

const { data: updateData } = useQuery({
  queryKey: ["updates"],
  queryFn: () => APIClient.updates.getLatestRelease(),
  retry: false,
  refetchOnWindowFocus: false,
  onSuccess: () => {},
  onError: (error) => {
    console.log(error);
  },
});

const checkUpdateMutation = useMutation({
  mutationFn: () => APIClient.updates.check(),
  onSuccess: () => {
    queryClient.invalidateQueries(["updates"]);
  },
  onError: (error) => {
    console.log(error);
  },
});
</script>

<style scoped></style>
