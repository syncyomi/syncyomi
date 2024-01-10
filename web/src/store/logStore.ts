import {defineStore} from "pinia";
import {ref} from "vue";
import {LogEvent} from "@/types/Logs";

export const useLogsStore = defineStore('logsStore', () => {
  const loadFromLocalStorage = (key: string, defaultValue: any) => {
    const storedValue = localStorage.getItem(key);
    return storedValue ? JSON.parse(storedValue) : defaultValue;
  };

  const logs = ref<LogEvent[]>([]);
  const settings = ref(loadFromLocalStorage('settings', { scrollOnNewLog: false }));
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
    settings,
    searchFilter,
    filteredLogs,
    isInvalidRegex,
    addLog,
    clearLogs
  }
});
