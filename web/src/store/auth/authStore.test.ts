import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useAuthStore } from "./authStore";

describe("authStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
  });

  it("rehydrates state from localStorage on creation", () => {
    localStorage.setItem("auth_isAuthenticated", "true");
    localStorage.setItem("auth_loggedInUser", JSON.stringify("alice"));

    const store = useAuthStore();
    expect(store.isAuthenticated).toBe(true);
    expect(store.loggedInUser).toBe("alice");
  });

  it("falls back to defaults when localStorage is empty", () => {
    const store = useAuthStore();
    expect(store.isAuthenticated).toBe(false);
    expect(store.loggedInUser).toBe("");
  });

  it("login sets state and persists both keys", () => {
    const store = useAuthStore();
    store.login("bob");

    expect(store.isAuthenticated).toBe(true);
    expect(store.loggedInUser).toBe("bob");
    expect(localStorage.getItem("auth_isAuthenticated")).toBe("true");
    expect(localStorage.getItem("auth_loggedInUser")).toBe(JSON.stringify("bob"));
  });

  it("logout clears state and removes both keys", () => {
    const store = useAuthStore();
    store.login("bob");
    store.logout();

    expect(store.isAuthenticated).toBe(false);
    expect(store.loggedInUser).toBe("");
    expect(localStorage.getItem("auth_isAuthenticated")).toBeNull();
    expect(localStorage.getItem("auth_loggedInUser")).toBeNull();
  });
});
