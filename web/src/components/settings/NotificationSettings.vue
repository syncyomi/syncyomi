<template>
  <v-card :loading="isLoading" variant="flat">
    <v-card-title class="d-flex align-center">
      Notifications
      <v-spacer></v-spacer>
      <AddNotification />
    </v-card-title>
    <v-card-subtitle class="mb-3">
      Send notifications on events.
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
      <v-card-item v-if="dataTableComputed.length <= 0">
        <h3 class="text-center">No notifications!</h3>
      </v-card-item>

      <v-table v-if="data && dataTableComputed.length > 0">
        <thead>
          <tr>
            <th class="text-left">Enabled</th>
            <th class="text-left">Name</th>
            <th class="text-left">Type</th>
            <th class="text-left">Events</th>
          </tr>
        </thead>
        <tbody v-if="data && dataTableComputed.length > 0">
          <tr v-for="item in dataTableComputed" :key="item.id">
            <td>
              <v-switch
                readonly
                v-model="item.enabled"
                color="primary"
              ></v-switch>
            </td>
            <td>{{ item.name }}</td>
            <td>{{ item.type }}</td>
            <td>{{ item.events ? item.events.length : 0 }}</td>
            <td>
              <v-icon @click="showDeleteConfirmation(item.id)"
                >mdi-file-document-remove
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

    <confirmation-modal
      ref="deleteConfirmationModal"
      title="Delete Notification"
      message="Are you sure you want to delete this notification?"
      @confirmed="confirmedDeleteNotification"
      @canceled="canceledDeleteNotification"
    />
  </v-card>
</template>

<script lang="ts" setup>
import { APIClient } from "@/api/APIClient";
import { computed, Ref, ref } from "vue";
import AddNotification from "@/components/modals/AddNotification.vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { Notification } from "@/types/Notification";
import ConfirmationModal from "@/components/modals/DeleteConfirmationModal.vue";

const snackbarVisible: Ref<boolean> = ref(false);
const snackbarMessage: Ref<string> = ref("Config updated successfully!");
const snackbarColor: Ref<string> = ref("success");
const deleteConfirmationModal: Ref<any> = ref(null);
const selectedNotificationId: Ref<number> = ref(0);

// Get QueryClient from context
const queryClient = useQueryClient();

const { isLoading, data, refetch } = useQuery({
  queryKey: ["notifications"],
  queryFn: () => APIClient.notifications.getAll(),
  retry: false,
  refetchOnWindowFocus: false,
  onError: (error) => {
    console.log(error);
  },
});

const deleteNotification = useMutation({
  mutationFn: (id: number) => APIClient.notifications.delete(id),
  onSuccess: () => {
    snackbarVisible.value = true;
    snackbarMessage.value = "Notification deleted successfully!";
    snackbarColor.value = "success";
    queryClient.invalidateQueries(["notifications"]);
    refetch();
  },
  onError: (error) => {
    console.log(error);
    snackbarVisible.value = true;
    snackbarMessage.value = "Error deleting notification!";
    snackbarColor.value = "error";
  },
});

const showDeleteConfirmation = (id) => {
  selectedNotificationId.value = id;
  deleteConfirmationModal.value.showModal();
};

const confirmedDeleteNotification = () => {
  deleteNotification.mutate(selectedNotificationId.value);
};

const canceledDeleteNotification = () => {
  selectedNotificationId.value = 0;
};

const dataTableComputed = computed(() => {
  if (data.value && data.value.length > 0) {
    return data.value.map((item: Notification) => ({
      id: item.id,
      enabled: item.enabled,
      name: item.name,
      type: item.type,
      events: item.events,
    }));
  } else {
    return [];
  }
});
</script>

<style scoped></style>
