<template>
  <div class="pa-4">
    <v-row>
      <v-col cols="12" md="4">
        <h4 class="mb-2">Status</h4>
        <div v-if="statusQuery.isLoading.value">Loading…</div>
        <div v-else-if="!status">No sync recorded yet.</div>
        <v-list v-else density="compact">
          <v-list-item>
            <v-list-item-title>Last upload</v-list-item-title>
            <v-list-item-subtitle>{{ fmt(status.last_synced_at) }}</v-list-item-subtitle>
          </v-list-item>
          <v-list-item>
            <v-list-item-title>Last event</v-list-item-title>
            <v-list-item-subtitle>
              <v-chip
                v-if="status.last_status"
                :color="statusColor(status.last_status)"
                size="x-small"
                class="mr-1"
              >
                {{ status.last_status }}
              </v-chip>
              {{ status.last_event || "—" }}
              <span v-if="status.last_device"> · {{ status.last_device }}</span>
              <span v-if="status.last_event_at"> · {{ fmt(status.last_event_at) }}</span>
            </v-list-item-subtitle>
          </v-list-item>
          <v-list-item v-if="status.last_message">
            <v-list-item-title>Message</v-list-item-title>
            <v-list-item-subtitle>{{ status.last_message }}</v-list-item-subtitle>
          </v-list-item>
          <v-list-item>
            <v-list-item-title>Stored payload</v-list-item-title>
            <v-list-item-subtitle>
              {{ formatBytes(status.data_size) }}
              <span v-if="status.data_updated_at"> · {{ fmt(status.data_updated_at) }}</span>
            </v-list-item-subtitle>
          </v-list-item>
        </v-list>
      </v-col>

      <v-col cols="12" md="4">
        <h4 class="mb-2">Devices</h4>
        <div v-if="devicesQuery.isLoading.value">Loading…</div>
        <div v-else-if="!devices.length">
          No devices seen yet. Devices appear once they report sync events.
        </div>
        <v-list v-else density="compact">
          <v-list-item v-for="device in devices" :key="device.id">
            <v-list-item-title>
              {{ device.device_name || device.device_id }}
              <v-chip
                v-if="device.last_status"
                :color="statusColor(device.last_status)"
                size="x-small"
                class="ml-1"
              >
                {{ device.last_status }}
              </v-chip>
              <v-chip
                v-if="device.protocol === 'v1'"
                color="warning"
                size="x-small"
                class="ml-1"
                title="This device uses the legacy sync protocol. Update the app to sync faster and more reliably."
              >
                legacy
              </v-chip>
            </v-list-item-title>
            <v-list-item-subtitle>
              Last seen {{ fmt(device.last_seen) }}
              <span v-if="device.last_event"> · {{ device.last_event }}</span>
            </v-list-item-subtitle>
          </v-list-item>
        </v-list>
      </v-col>

      <v-col cols="12" md="4">
        <h4 class="mb-2">History</h4>
        <div v-if="historyQuery.isLoading.value">Loading…</div>
        <div v-else-if="!history.length">No previous payloads kept.</div>
        <v-list v-else density="compact">
          <v-list-item v-for="(entry, index) in history" :key="entry.id">
            <v-list-item-title>
              {{ fmt(entry.created_at) }}
              <v-chip v-if="index === 0" size="x-small" class="ml-1">current</v-chip>
            </v-list-item-title>
            <v-list-item-subtitle>{{ formatBytes(entry.size) }}</v-list-item-subtitle>
            <template #append>
              <v-btn
                v-if="index !== 0"
                size="small"
                variant="text"
                :loading="restore.isPending.value && restoringId === entry.id"
                @click="askRestore(entry.id)"
              >
                Restore
              </v-btn>
            </template>
          </v-list-item>
        </v-list>
      </v-col>
    </v-row>

    <confirmation-modal
      ref="restoreConfirmationModal"
      title="Restore sync data"
      message="Replace the current sync data with this earlier version? Devices will download it on their next sync."
      @confirmed="confirmedRestore"
      @canceled="restoringId = null"
    />
  </div>
</template>

<script lang="ts" setup>
import { computed, ref } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import ConfirmationModal from "@/components/modals/DeleteConfirmationModal.vue";

const props = defineProps<{ apiKey: string }>();
const emit = defineEmits<{
  (e: "restored"): void;
  (e: "error", message: string): void;
}>();

const queryClient = useQueryClient();
const restoreConfirmationModal = ref<InstanceType<typeof ConfirmationModal> | null>(
  null,
);
const restoringId = ref<number | null>(null);

const statusQuery = useQuery({
  queryKey: ["syncStatus", props.apiKey],
  queryFn: () => APIClient.sync.status(props.apiKey).catch(() => null),
  retry: false,
  refetchOnWindowFocus: false,
});
const devicesQuery = useQuery({
  queryKey: ["syncDevices", props.apiKey],
  queryFn: () => APIClient.sync.devices(props.apiKey),
  retry: false,
  refetchOnWindowFocus: false,
});
const historyQuery = useQuery({
  queryKey: ["syncHistory", props.apiKey],
  queryFn: () => APIClient.sync.history(props.apiKey),
  retry: false,
  refetchOnWindowFocus: false,
});

const status = computed(() => statusQuery.data.value ?? null);
const devices = computed(() => devicesQuery.data.value ?? []);
const history = computed(() => historyQuery.data.value ?? []);

const restore = useMutation({
  mutationFn: (id: number) => APIClient.sync.restore(props.apiKey, id),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ["syncStatus", props.apiKey] });
    queryClient.invalidateQueries({ queryKey: ["syncHistory", props.apiKey] });
    emit("restored");
  },
  onError: (error: Error) => emit("error", error.message),
  onSettled: () => {
    restoringId.value = null;
  },
});

const askRestore = (id: number) => {
  restoringId.value = id;
  restoreConfirmationModal.value?.showModal();
};

const confirmedRestore = () => {
  if (restoringId.value !== null) {
    restore.mutate(restoringId.value);
  }
};

const fmt = (value: string | null | undefined) =>
  value ? new Date(value).toLocaleString() : "—";

const formatBytes = (bytes: number) => {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
};

const statusColor = (s: string) => {
  switch (s) {
    case "success":
      return "success";
    case "error":
      return "error";
    case "running":
      return "info";
    case "cancelled":
      return "warning";
    default:
      return undefined;
  }
};
</script>

<style scoped></style>
