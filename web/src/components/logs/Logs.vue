<template>
    <v-container>
      <h1 class="text-h3 py-6">Logs</h1>

      <v-card class="mb-12" outlined>
        <v-card-text>
          <div class="d-flex flex-row">
            <!-- Text Field with flex-grow for taking up remaining space -->
            <div class="flex-grow-1 mr-2">
              <v-text-field
                v-model="logsStore.searchFilter"
                label="Enter a regex pattern to filter logs by..."
                outlined
                :clearable=true
                @click:clear="logsStore.searchFilter = ''"
              ></v-text-field>
            </div>

            <div>
              <LogsDropdown />
            </div>
          </div>

          <v-row>
            <v-col cols="12">
              <v-virtual-scroll
                ref="virtualScroller"
                :items="logsStore.filteredLogs"
                height="60vh"
                item-height="48"
              >
              <template v-slot="{ item }">
                <div :class="[
              'my-2 flex items-center',
              logsStore.indentLogLines ? 'pl-4' : '',
              logsStore.hideWrappedText ? 'truncate' : 'whitespace-normal'
            ]">
                  <span :title="formatTime(item.time)" class="font-mono grey--text mr-2">{{ formatTime(item.time) }}</span>
                  <span :class="getLogLevelColor(item.level)" class="font-mono font-weight-bold mr-2">{{ item.level }}</span>
                  <span>{{ item.message }}</span>
                </div>
              </template>
              </v-virtual-scroll>
            </v-col>
          </v-row>

        </v-card-text>
      </v-card>
    </v-container>
</template>


<script lang="ts" setup>
import {onBeforeUnmount, onMounted, ref, watch} from "vue";
import {useLogsStore} from "@/store/logStore";
import {APIClient} from "@/api/APIClient";
import {format} from "date-fns";
import LogsDropdown from "@/components/logs/LogsDropdown.vue";
import {VVirtualScroll} from "vuetify/components";
import {LogEvent} from "@/types/Logs";

export type LogLevel = "TRC" | "DBG" | "INF" | "ERR" | "WRN" | "FTL" | "PNC";

const LogColors: Record<LogLevel, string> = {
  "TRC": "text-purple-300",
  "DBG": "text-yellow-500",
  "INF": "text-green-500",
  "ERR": "text-red-500",
  "WRN": "text-yellow-500",
  "FTL": "text-red-500",
  "PNC": "text-red-600"
};

const getLogLevelColor = (level: string) => {
  return LogColors[level as LogLevel] || "default-color-class";
};

const logsStore = useLogsStore();
const virtualScroller = ref<VVirtualScroll>();
let es : EventSource;

const formatTime = (time: string) => {
  return  format(new Date(time), "HH:mm:ss");
};

const scrollToBottom = () => {
  if (logsStore.filteredLogs.length > 0) {
    virtualScroller.value?.scrollToIndex(logsStore.filteredLogs.length - 1);
  }
};

watch(() => logsStore.scrollOnNewLog, () => {
  if (logsStore.scrollOnNewLog) {
    scrollToBottom();
  }
});

watch(() => [logsStore.logs, logsStore.filteredLogs], () => {
  // Check if scroll on new log is enabled
  if (logsStore.scrollOnNewLog) {
    scrollToBottom();
  }
});

// Watch for new logs being added
watch(() => logsStore.logs.length, (newLength, oldLength) => {
  if (newLength > oldLength && logsStore.scrollOnNewLog) {
    scrollToBottom();
  }
});

watch([() => logsStore.logs, () => logsStore.searchFilter], () => {
  if (!logsStore.searchFilter.length) {
    logsStore.filteredLogs = logsStore.logs;
    logsStore.isInvalidRegex = false;
    return;
  }

  try {
    const pattern = new RegExp(logsStore.searchFilter, "i");
    logsStore.filteredLogs = logsStore.logs.filter(log => pattern.test(log.message));
    logsStore.isInvalidRegex = false;
  } catch (error) {
    // Handle regex errors by showing nothing when the regex pattern is invalid
    logsStore.filteredLogs = [];
    logsStore.isInvalidRegex = true;
  }
}, { immediate: true });

onMounted(() => {
  es = APIClient.events.logs();

  es.onmessage = (event) => {
    const newData = JSON.parse(event.data) as LogEvent;
    logsStore.addLog(newData);
  };
});

onBeforeUnmount(() => {
  if (es) {
    es.close();
  }
});

watch(() => logsStore.scrollOnNewLog, (newVal) => {
  if (newVal) {
    scrollToBottom();
  }
});
</script>
