import {defineStore} from "pinia";
import {ref, watch} from "vue";
import {LogEvent} from "@/types/Logs";

export const useLogsStore = defineStore('logsStore', () => {
  const loadFromLocalStorage = (key: string, defaultValue: any) => {
    const storedValue = localStorage.getItem(key);
    return storedValue ? JSON.parse(storedValue) : defaultValue;
  };

  const logs = ref<LogEvent[]>([]);
  const settings = ref(loadFromLocalStorage('settings', {
    scrollOnNewLog: false,
    indentLogLines:false,
    hideWrappedText: false
  }));
  const searchFilter = ref('');
  const filteredLogs = ref<LogEvent[]>([]);
  const isInvalidRegex = ref(false);

  const addLog = (log: LogEvent) => {
    logs.value.push(log);
  };

  const clearLogs = () => {
    logs.value = [];
  };

  // Watch for changes in settings and update localStorage when they occur
  watch(settings, (newSettings) => {
    localStorage.setItem('settings', JSON.stringify(newSettings));
  }, {
    deep: true
  });


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
