<template>
  <v-card :loading="isLoading" variant="flat">
    <v-card-title class="d-flex align-center">
      API Keys
      <v-spacer></v-spacer>
      <AddApiKey />
    </v-card-title>
    <v-card-subtitle class="mb-3"> Manage API Keys.</v-card-subtitle>

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
      <v-card-item v-if="dataTableComputed.length <= 0">
        <h3 class="text-center">No Api keys!</h3>
      </v-card-item>

      <v-table v-if="data && dataTableComputed.length > 0">
        <thead>
          <tr>
            <th class="text-left">Name</th>
            <th class="text-left">Key</th>
          </tr>
        </thead>
        <tbody v-if="data && dataTableComputed.length > 0">
          <tr v-for="(item, index) in dataTableComputed" :key="index">
            <td>{{ item.name }}</td>
            <td>
              <v-text-field
                variant="underlined"
                :model-value="item.key"
                :readonly="true"
                :type="showPassword[index] ? 'text' : 'password'"
              >
                <template #append-inner>
                  <v-icon @click="togglePasswordVisibility(index)" class="mr-2">
                    mdi-eye{{ showPassword[index] ? "-off" : "" }}
                  </v-icon>
                  <v-icon @click="copyToClipboard(item.key)">
                    mdi-content-copy
                  </v-icon>
                </template>
                <template #append>
                  <v-icon @click="showDeleteConfirmation(item.key)"
                    >mdi-file-document-remove
                  </v-icon>
                </template>
              </v-text-field>
            </td>
          </tr>
        </tbody>
      </v-table>
    </div>

    <v-snackbar
      v-model="snackbarVisible"
      :color="snackbarColor"
      :timeout="1500"
      variant="elevated"
    >
      {{ snackbarMessage }}
    </v-snackbar>

    <confirmation-modal
      ref="deleteConfirmationModal"
      title="Delete Api Key"
      message="Are you sure you want to delete this Api key?"
      @confirmed="confirmedDeleteNotification"
      @canceled="canceledDeleteNotification"
    />
  </v-card>
</template>

<script lang="ts" setup>
import { APIClient } from "@/api/APIClient";
import { computed, reactive, Ref, ref } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import ConfirmationModal from "@/components/modals/DeleteConfirmationModal.vue";
import AddApiKey from "@/components/modals/AddApiKey.vue";

const snackbarVisible: Ref<boolean> = ref(false);
const snackbarMessage: Ref<string> = ref("Config updated successfully!");
const snackbarColor: Ref<string> = ref("success");
const deleteConfirmationModal: Ref<any> = ref(null);
const selectedApiKey: Ref<string> = ref("");
const showPassword = reactive({});

// Get QueryClient from context
const queryClient = useQueryClient();

const { isLoading, data } = useQuery({
  queryKey: ["apiKeys"],
  queryFn: () => APIClient.apikeys.getAll(),
  retry: false,
  refetchOnWindowFocus: false,
  onError: (error) => {
    console.log(error);
  },
});

const deleteApiKey = useMutation({
  mutationFn: (name: string) => APIClient.apikeys.delete(name),
  onSuccess: () => {
    snackbarVisible.value = true;
    snackbarMessage.value = "Api Key deleted successfully!";
    snackbarColor.value = "success";
    queryClient.invalidateQueries(["apiKeys"]);
  },
  onError: (error) => {
    console.log(error);
    snackbarVisible.value = true;
    snackbarMessage.value = "Error deleting Api Key!";
    snackbarColor.value = "error";
  },
});

const togglePasswordVisibility = (index: number) => {
  showPassword[index] = !showPassword[index];
};

const showDeleteConfirmation = (key: string) => {
  selectedApiKey.value = key;
  deleteConfirmationModal.value.showModal();
};

const confirmedDeleteNotification = () => {
  deleteApiKey.mutate(selectedApiKey.value);
};

const canceledDeleteNotification = () => {
  selectedApiKey.value = "";
};

const dataTableComputed = computed(() => {
  if (data.value && data.value.length > 0) {
    return data.value.map((item: APIKey) => ({
      name: item.name,
      key: item.key,
    }));
  } else {
    return [];
  }
});

const copyToClipboard = (text: string) => {
  navigator.clipboard.writeText(text).then(
    () => {
      snackbarVisible.value = true;
      snackbarMessage.value = "Copied to clipboard!";
    },
    () => {
      snackbarVisible.value = true;
      snackbarMessage.value = "Error copying to clipboard!";
      snackbarColor.value = "error";
    }
  );
};
</script>

<style scoped></style>
