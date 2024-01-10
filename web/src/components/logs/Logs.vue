<template>
  <v-main>
    <v-container>
      <h1 class="text-h3 py-6">Logs</h1>

      <v-card class="mb-12" outlined>
        <v-card-text>
          <v-row>
            <v-col cols="12" sm="6">
              <v-text-field
                v-model="logsStore.searchFilter"
                label="Enter a regex pattern to filter logs by..."
                outlined
                clearable
                @input="logsStore.searchFilter = $event"
              ></v-text-field>
            </v-col>
            <!-- Add LogsDropdown component here if needed -->
          </v-row>

          <v-row>
            <v-col cols="12">
              <div class="overflow-y-auto" style="max-height: 60vh;">
                <div v-for="(entry, idx) in logsStore.filteredLogs" :key="idx" class="my-2">
                  <span :title="formatTime(entry.time)" class="font-mono grey--text mr-2">{{ formatTime(entry.time) }}</span>
                  <span :class="getLogLevelColor(entry.level)" class="font-mono font-weight-bold mr-2">{{ entry.level }}</span>
                  <span>{{ entry.message }}</span>
                </div>
              </div>
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>
    </v-container>
  </v-main>
</template>


<script lang="ts" setup>
import {onBeforeUnmount, onMounted, ref, watch} from "vue";
import {useLogsStore} from "@/store/logStore";
import {LogEvent} from "@/types/Logs";
import {APIClient} from "@/api/APIClient";
import {format} from "date-fns";

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

const messagesEndRef = ref<HTMLDivElement>();

const formatTime = (time: string) => {
  return  format(new Date(time), "HH:mm:ss");
};

watch(() => logsStore.filteredLogs, (newLogs, oldLogs) => {
  if (logsStore.settings.scrollOnNewLog) {
    scrollToBottom();
  }
});

watch(() => logsStore.settings.scrollOnNewLog, (newValue, oldValue) => {
  logsStore.clearLogs();
});

watch([() => logsStore.logs, () => logsStore.searchFilter], () => {
  // Logic to set filteredLogs based on logs and searchFilter
  // Similar to your useEffect logic in React
}, { immediate: true });

const scrollToBottom = () => {
  if (messagesEndRef.value) {
    messagesEndRef.value.scrollTop = messagesEndRef.value.scrollHeight;
  }
};

onMounted(() => {
  const es = APIClient.events.logs();

  es.onmessage = (event) => {
    const newData = JSON.parse(event.data) as LogEvent;
    logsStore.addLog(newData);
  };

  onBeforeUnmount(() => {
    es.close();
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
});
</script>
