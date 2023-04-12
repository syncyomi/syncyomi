import { defineStore } from "pinia";
import { Ref, ref } from "vue";

export const useAuthStore = defineStore("auth", () => {
  const loadFromLocalStorage = (key: string, defaultValue: any) => {
    const storedValue = localStorage.getItem(key);
    return storedValue ? JSON.parse(storedValue) : defaultValue;
  };

  const isAuthenticated: Ref<boolean> = ref(
    loadFromLocalStorage("auth_isAuthenticated", false)
  );
  const loggedInUser: Ref<string> = ref(
    loadFromLocalStorage("auth_loggedInUser", "")
  );

  const login = (username: string): void => {
    isAuthenticated.value = true;
    loggedInUser.value = username;

    // save to local storage
    localStorage.setItem(
      "auth_isAuthenticated",
      JSON.stringify(isAuthenticated.value)
    );
    localStorage.setItem(
      "auth_loggedInUser",
      JSON.stringify(loggedInUser.value)
    );
  };

  const logout = (): void => {
    isAuthenticated.value = false;
    loggedInUser.value = "";

    // remove from local storage
    localStorage.removeItem("auth_isAuthenticated");
    localStorage.removeItem("auth_loggedInUser");
  };

  return {
    isAuthenticated,
    loggedInUser,
    login,
    logout,
  };
});
