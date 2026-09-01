<template>
  <div class="pa-4">
    <v-alert
      v-if="hasLegacyDevice"
      type="warning"
      variant="tonal"
      density="compact"
      class="mb-4"
      title="Legacy sync protocol in use"
      text="A device on this key syncs with the deprecated v1 protocol. It keeps working, but update the app once it supports v2 for faster, finer-grained sync."
    />
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
              <tr v-if="status.last_protocol">
                <th class="text-left text-no-wrap">Protocol</th>
                <td>
                  <v-chip
                    v-if="status.last_protocol === 'v1'"
                    color="warning"
                    size="x-small"
                    title="This key was last synced with the deprecated v1 protocol."
                  >
                    v1 · legacy
                  </v-chip>
                  <v-chip v-else size="x-small" variant="tonal">
                    {{ status.last_protocol }}
                  </v-chip>
                </td>
              </tr>
              <tr v-if="hasLibraryStats">
                <th class="text-left text-no-wrap">Library</th>
                <td>
                  {{ status.manga_count ?? 0 }} manga ·
                  {{ status.chapter_count ?? 0 }} chapters ·
                  {{ status.category_count ?? 0 }} categories
                </td>
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
            :items="visibleDevices"
            :loading="devicesQuery.isLoading.value"
            :sort-by="[{ key: 'last_seen', order: 'desc' }]"
            :items-per-page="-1"
            item-value="id"
            density="compact"
            hide-default-footer
            no-data-text="No devices seen yet. Devices appear once they report sync events."
          >
            <template #[`item.name`]="{ item }">
              <span :title="deviceTooltip(item)">{{ deviceName(item) }}</span>
            </template>
            <template #[`item.last_status`]="{ item }">
              <v-chip
                v-if="item.last_status"
                :color="statusColor(item.last_status)"
                size="x-small"
                :title="item.last_message || undefined"
              >
                {{ item.last_status }}
              </v-chip>
            </template>
            <template #[`item.protocol`]="{ item }">
              <div class="d-flex align-center ga-1 text-no-wrap">
                <v-chip
                  v-if="item.protocol === 'v1'"
                  color="warning"
                  size="x-small"
                  title="This device uses the legacy sync protocol. Update the app to sync faster and more reliably."
                >
                  legacy
                </v-chip>
                <span v-else-if="item.protocol">{{ item.protocol }}</span>
                <span v-else>—</span>
                <v-icon
                  v-if="deviceLag(item) === 0"
                  icon="mdi-check-circle-outline"
                  size="x-small"
                  color="success"
                  title="Up to date"
                  aria-label="Up to date"
                />
                <v-chip
                  v-else-if="deviceLag(item) !== null"
                  size="x-small"
                  variant="tonal"
                  color="warning"
                  title="Merges on the server this device has not pulled yet."
                >
                  {{ deviceLag(item) }} behind
                </v-chip>
              </div>
            </template>
            <template #[`item.last_event`]="{ item }">
              {{ item.last_event || "—" }}
            </template>
            <template #[`item.last_seen`]="{ item }">
              <span :title="relativeDate(item.last_seen)">{{
                simplifyDate(item.last_seen)
              }}</span>
            </template>
            <template #[`item.actions`]="{ item }">
              <v-btn
                icon="mdi-delete-outline"
                size="x-small"
                variant="text"
                title="Forget this device. It reappears if it syncs again."
                :loading="forgetDevice.isPending.value && forgettingId === item.id"
                @click="askForget(item.id)"
              />
            </template>
          </v-data-table>
        </v-card>
      </v-col>

      <v-col cols="12">
        <v-card variant="outlined" title="History" :subtitle="historySubtitle">
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
            <template #[`item.origin`]="{ item }">
              <v-chip
                size="x-small"
                variant="tonal"
                :title="
                  historyOrigin(item) === 'device upload'
                    ? 'Uploaded by a device, byte-for-byte.'
                    : 'Assembled by the server from merged changes.'
                "
              >
                {{ historyOrigin(item) }}
              </v-chip>
            </template>
            <template #[`item.actions`]="{ item, index }">
              <v-btn
                icon="mdi-download-outline"
                size="x-small"
                variant="text"
                title="Download this payload as a backup file."
                :href="APIClient.sync.downloadHistoryUrl(props.apiKey, item.id)"
              />
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
    <confirmation-modal
      ref="forgetConfirmationModal"
      title="Forget device"
      message="Remove this device from the list? Its synced data stays; the device reappears if it syncs again."
      @confirmed="confirmedForget"
      @canceled="forgettingId = null"
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
const forgetConfirmationModal = ref<InstanceType<
  typeof ConfirmationModal
> | null>(null);
const forgettingId = ref<number | null>(null);

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
    queryClient.invalidateQueries({ queryKey: ["syncDevices", props.apiKey] });
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

const forgetDevice = useMutation({
  mutationFn: (id: number) => APIClient.sync.deleteDevice(props.apiKey, id),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ["syncDevices", props.apiKey] });
  },
  onError: (error: Error) => emit("error", error.message),
  onSettled: () => {
    forgettingId.value = null;
  },
});

const askForget = (id: number) => {
  forgettingId.value = id;
  forgetConfirmationModal.value?.showModal();
};

const confirmedForget = () => {
  if (forgettingId.value !== null) {
    forgetDevice.mutate(forgettingId.value);
  }
};

// warn when the last write was v1 or any known device still speaks v1
const hasLegacyDevice = computed(
  () =>
    status.value?.last_protocol === "v1" ||
    devices.value.some((d) => d.protocol === "v1"),
);

// "legacy" aggregates anonymous v1 uploads; once a named row is tagged v1 it represents
// that traffic, so showing the aggregate too would list one phone twice
const visibleDevices = computed(() => {
  const named = devices.value.some(
    (d) => d.device_id !== "legacy" && d.protocol === "v1",
  );
  return named
    ? devices.value.filter((d) => d.device_id !== "legacy")
    : devices.value;
});

const hasLibraryStats = computed(() => {
  const s = status.value;
  return !!s && ((s.manga_count ?? 0) > 0 || (s.category_count ?? 0) > 0);
});

const historySubtitle = computed(() => {
  const limit = status.value?.history_limit;
  return limit ? `Keeps the last ${limit} payloads.` : undefined;
});

// v1 uploads carry no device identity; the server records them under "legacy"
const deviceName = (d: SyncDevice) => {
  if (d.device_name) return d.device_name;
  if (d.device_id === "legacy") return "Legacy device";
  return d.device_id;
};

const deviceTooltip = (d: SyncDevice) => {
  const parts = [];
  if (d.device_id === "legacy") {
    parts.push(
      "One or more v1 clients that don't identify themselves share this row.",
    );
  } else if (d.device_name && d.device_id !== d.device_name) {
    parts.push(`ID: ${d.device_id}`);
  }
  if (d.created_at) parts.push(`First seen ${simplifyDate(d.created_at)}`);
  return parts.join(" · ") || undefined;
};

// merges on the server the device has not pulled; null when unknowable. A v1 row with
// cursor 0 was tagged from events only — its real cursor lives on the anonymous upload.
const deviceLag = (d: SyncDevice) => {
  const seq = status.value?.seq;
  if (seq === undefined || !d.protocol) return null;
  if (d.protocol === "v1" && d.cursor === 0 && d.device_id !== "legacy")
    return null;
  return Math.max(0, seq - d.cursor);
};

const historyOrigin = (h: SyncHistoryEntry) =>
  h.etag.startsWith("uuid=") ? "device upload" : "server merge";

const deviceHeaders = [
  { title: "Device", key: "name", sortable: false },
  { title: "Status", key: "last_status" },
  { title: "Sync", key: "protocol", sortable: false },
  { title: "Last event", key: "last_event" },
  { title: "Last seen", key: "last_seen" },
  { title: "", key: "actions", sortable: false, align: "end" as const },
];

const historyHeaders = [
  { title: "Created", key: "created_at", sortable: false },
  { title: "Size", key: "size", sortable: false },
  { title: "Origin", key: "origin", sortable: false },
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
