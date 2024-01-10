// Utilities
import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', () => {
  const loadFromLocalStorage = (key: string, defaultValue: any) => {
    const storedValue = localStorage.getItem(key);
    return storedValue ? JSON.parse(storedValue) : defaultValue;
  };

  return {

  }
});
