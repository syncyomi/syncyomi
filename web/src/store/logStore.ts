import {defineStore} from "pinia";
import {ref} from "vue";
import {LogEvent} from "@/types/Logs";

export const useLogsStore = defineStore('logsStore', () => {
  const logs = ref<LogEvent[]>([]);
  const scrollOnNewLog = ref(false);
  const indentLogLines = ref(false);
  const hideWrappedText = ref(false);
  const searchFilter = ref('');
  const filteredLogs = ref<LogEvent[]>([]);
  const isInvalidRegex = ref(false);

  const addLog = (log: LogEvent) => {
    logs.value.push(log);
  };

  const clearLogs = () => {
    logs.value = [];
  };


  return {
    logs,
    scrollOnNewLog,
    indentLogLines,
    hideWrappedText,
    searchFilter,
    filteredLogs,
    isInvalidRegex,
    addLog,
    clearLogs
  }
});
