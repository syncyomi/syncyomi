<template>
  <v-card :loading="isLoading" variant="flat">
    <v-card-title>Logs</v-card-title>
    <v-card-subtitle class="mb-3"> Set Log Level.</v-card-subtitle>

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
        <v-row>
          <v-col cols="12">
            <v-row>
              <v-col class="text-h6 font-weight-bold"> Path:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-chip
                  :color="data?.log_path ? 'primary' : ''"
                  class="mx-1"
                  variant="tonal"
                >
                  <span>{{ data?.log_path || `No Path!` }} </span>
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
              <v-col class="text-h6 font-weight-bold"> Level:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-select
                  :model-value="data?.log_level"
                  :items="LogLevelOptions"
                  @update:modelValue="setLogLevelUpdateMutation.mutate($event)"
                  color="primary"
                  label="Log Level"
                  variant="underlined"
                ></v-select>
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
              <v-col class="text-h6 font-weight-bold"> Max Size:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-chip :color="data?.log_max_size ? '' : ''" variant="tonal">
                  <span>{{ data?.log_max_size }} MB </span>
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
              <v-col class="text-h6 font-weight-bold"> Max Backups:</v-col>
              <v-col class="d-flex justify-end text-h6">
                <v-chip
                  :color="data?.log_max_backups ? '' : ''"
                  variant="tonal"
                >
                  <span>{{ data?.log_max_backups }}</span>
                </v-chip>
              </v-col>
            </v-row>
          </v-col>
        </v-row>
      </v-card-item>
      <v-divider></v-divider>

      <v-table>
        <thead>
          <tr>
            <th class="text-left">Log File</th>
            <th class="text-left">Size</th>
            <th class="text-left">Last Modified At</th>
          </tr>
        </thead>
        <tbody v-if="logFiles?.files">
          <tr v-for="item in logFiles.files" :key="item.filename">
            <td>{{ item.filename }}</td>
            <td>{{ item.size }}</td>
            <td>{{ simplifyDate(item.updated_at) }}</td>
            <td>
              <v-icon @click="downloadLogFile(item.filename)"
                >mdi-download
              </v-icon>
            </td>
          </tr>
        </tbody>
      </v-table>
    </div>

    <v-snackbar
      v-model="snackbarVisible"
      :color="snackbarColor"
      :timeout="800"
      variant="elevated"
    >
      {{ snackbarMessage }}
    </v-snackbar>
  </v-card>
</template>

<script lang="ts" setup>
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import { Ref, ref } from "vue";
import { LogLevelOptions } from "@/domain/constants";
import { baseUrl, simplifyDate } from "@/utils";

const snackbarVisible: Ref<boolean> = ref(false);
const snackbarMessage: Ref<string> = ref("Config updated successfully!");
const snackbarColor: Ref<string> = ref("success");

// Get QueryClient from context
const queryClient = useQueryClient();

const { isLoading, data } = useQuery({
  queryKey: ["config"],
  queryFn: () => APIClient.config.get(),
  retry: false,
  refetchOnWindowFocus: false,
  onError: (error) => {
    console.log(error);
  },
});

const { data: logFiles } = useQuery({
  queryKey: ["log-files"],
  queryFn: () => APIClient.logs.files(),
  retry: false,
  refetchOnWindowFocus: false,
  onError: (error) => {
    console.log(error);
  },
});

const setLogLevelUpdateMutation = useMutation({
  mutationFn: (val: string) =>
    APIClient.config.update({
      log_level: val,
    }),
  onSuccess: () => {
    queryClient.invalidateQueries(["config"]);
    snackbarVisible.value = true;
  },
  onError: () => {
    snackbarVisible.value = true;
    snackbarColor.value = "error";
    snackbarMessage.value = "Error updating config!";
  },
});

const downloadLogFile = async (filename: string) => {
  try {
    const response = await fetch(`${baseUrl()}api/logs/files/${filename}`);
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.click();
    URL.revokeObjectURL(url);
  } catch (error) {
    console.error("Error downloading log file:", error);
    snackbarVisible.value = true;
    snackbarColor.value = "error";
    snackbarMessage.value = "Error downloading log file!";
  }
};
</script>

<style scoped></style>
