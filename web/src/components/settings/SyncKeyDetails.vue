<template>
  <div class="pa-4">
    <v-row>
      <v-col cols="12" md="4">
        <v-card variant="outlined" title="Status">
          <v-card-text v-if="statusQuery.isLoading.value">Loading…</v-card-text>
          <v-card-text v-else-if="!status">No sync recorded yet.</v-card-text>
          <v-table v-else density="compact">
            <tbody>
              <tr>
                <th class="text-left text-no-wrap">Last upload</th>
                <td :title="relativeDate(status.last_synced_at)">
                  {{ simplifyDate(status.last_synced_at) }}
                </td>
              </tr>
              <tr>
                <th class="text-left text-no-wrap">Last event</th>
                <td :class="statusTextClass(status.last_status)">
                  {{ status.last_event || "—" }}
                  <span v-if="status.last_device">
                    · {{ status.last_device }}</span
                  >
                </td>
              </tr>
              <tr v-if="status.last_event_at">
                <th class="text-left text-no-wrap">Event time</th>
                <td :title="relativeDate(status.last_event_at)">
                  {{ simplifyDate(status.last_event_at) }}
                </td>
              </tr>
              <tr v-if="status.last_message">
                <th class="text-left text-no-wrap">Message</th>
                <td>{{ status.last_message }}</td>
              </tr>
              <tr>
                <th class="text-left text-no-wrap">Stored payload</th>
                <td :title="relativeDate(status.data_updated_at)">
                  {{ formatBytes(status.data_size) }}
                  <span v-if="status.data_updated_at">
                    · {{ simplifyDate(status.data_updated_at) }}</span
                  >
                </td>
              </tr>
            </tbody>
          </v-table>
        </v-card>
      </v-col>

      <v-col cols="12" md="8">
        <v-card variant="outlined" title="Devices">
          <v-data-table
            :headers="deviceHeaders"
            :items="devices"
            :loading="devicesQuery.isLoading.value"
            :sort-by="[{ key: 'last_seen', order: 'desc' }]"
            :items-per-page="-1"
            item-value="id"
            density="compact"
            hide-default-footer
            no-data-text="No devices seen yet. Devices appear once they report sync events."
          >
            <template #[`item.name`]="{ item }">
              {{ item.device_name || item.device_id }}
            </template>
            <template #[`item.last_status`]="{ item }">
              <v-chip
                v-if="item.last_status"
                :color="statusColor(item.last_status)"
                size="x-small"
              >
                {{ item.last_status }}
              </v-chip>
            </template>
            <template #[`item.protocol`]="{ item }">
              <v-chip
                v-if="item.protocol === 'v1'"
                color="warning"
                size="x-small"
                title="This device uses the legacy sync protocol. Update the app to sync faster and more reliably."
              >
                legacy
              </v-chip>
              <span v-else>{{ item.protocol || "—" }}</span>
            </template>
            <template #[`item.last_event`]="{ item }">
              {{ item.last_event || "—" }}
            </template>
            <template #[`item.last_seen`]="{ item }">
              <span :title="relativeDate(item.last_seen)">{{
                simplifyDate(item.last_seen)
              }}</span>
            </template>
          </v-data-table>
        </v-card>
      </v-col>

      <v-col cols="12">
        <v-card variant="outlined" title="History">
          <v-data-table
            :headers="historyHeaders"
            :items="history"
            :loading="historyQuery.isLoading.value"
            :items-per-page="-1"
            item-value="id"
            density="compact"
            hide-default-footer
            no-data-text="No previous payloads kept."
          >
            <template #[`item.created_at`]="{ item }">
              <span :title="relativeDate(item.created_at)">{{
                simplifyDate(item.created_at)
              }}</span>
            </template>
            <template #[`item.size`]="{ item }">
              {{ formatBytes(item.size) }}
            </template>
            <template #[`item.etag`]="{ item }">
              <code>{{ item.etag }}</code>
            </template>
            <template #[`item.actions`]="{ item, index }">
              <v-chip v-if="index === 0" size="x-small">current</v-chip>
              <v-btn
                v-else
                size="small"
                variant="text"
                :loading="restore.isPending.value && restoringId === item.id"
                @click="askRestore(item.id)"
              >
                Restore
              </v-btn>
            </template>
          </v-data-table>
        </v-card>
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
import { relativeDate, simplifyDate } from "@/utils";

const props = defineProps<{ apiKey: string }>();
const emit = defineEmits<{
  (e: "restored"): void;
  (e: "error", message: string): void;
}>();

const queryClient = useQueryClient();
const restoreConfirmationModal = ref<InstanceType<
  typeof ConfirmationModal
> | null>(null);
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

const deviceHeaders = [
  { title: "Device", key: "name", sortable: false },
  { title: "Status", key: "last_status" },
  { title: "Protocol", key: "protocol" },
  { title: "Last event", key: "last_event" },
  { title: "Last seen", key: "last_seen" },
  { title: "Cursor", key: "cursor", align: "end" as const },
];

const historyHeaders = [
  { title: "Created", key: "created_at", sortable: false },
  { title: "Size", key: "size", sortable: false },
  { title: "ETag", key: "etag", sortable: false },
  { title: "", key: "actions", sortable: false, align: "end" as const },
];

const formatBytes = (bytes: number) => {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
};

const statusTextClass = (s: string) => {
  const color = statusColor(s);
  return color ? `text-${color}` : "";
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
