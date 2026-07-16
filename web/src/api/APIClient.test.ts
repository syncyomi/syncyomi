import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";

// The 401 path pushes to the Login route; a stub keeps the real router (and its
// route table / history) out of the unit test. vi.hoisted lets the mock factory
// (which is hoisted above imports) reference pushMock safely.
const { pushMock } = vi.hoisted(() => ({ pushMock: vi.fn() }));
vi.mock("@/router", () => ({
  default: {
    push: pushMock,
    currentRoute: { value: { path: "/" } },
  },
}));

import { HttpClient } from "./APIClient";
import { useAuthStore } from "@/store/auth/authStore";

const mockResponse = (init: {
  ok: boolean;
  status: number;
  json?: unknown;
  text?: string;
}) =>
  ({
    ok: init.ok,
    status: init.status,
    json: async () => init.json,
    text: async () => init.text ?? "",
  }) as Response;

describe("HttpClient", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal("window", {
      APP: { baseUrl: "/" },
      fetch: vi.fn(),
    });
    pushMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("resolves parsed JSON on a 200", async () => {
    window.fetch = vi.fn().mockResolvedValue(
      mockResponse({ ok: true, status: 200, json: { hello: "world" } })
    );

    await expect(HttpClient("api/thing", "GET")).resolves.toEqual({
      hello: "world",
    });
  });

  it("resolves the raw response on a 204 without parsing", async () => {
    const res = mockResponse({ ok: true, status: 204 });
    window.fetch = vi.fn().mockResolvedValue(res);

    await expect(HttpClient("api/thing", "POST")).resolves.toBe(res);
  });

  it("logs out and redirects on a 401", async () => {
    window.fetch = vi.fn().mockResolvedValue(
      mockResponse({ ok: false, status: 401 })
    );
    const store = useAuthStore();
    store.login("alice");

    await expect(HttpClient("api/thing", "GET")).rejects.toThrow(
      "Unauthorized"
    );
    expect(store.isAuthenticated).toBe(false);
    expect(pushMock).toHaveBeenCalledWith({ name: "Login" });
  });

  it("rejects with the body text on a non-ok, non-401 response", async () => {
    window.fetch = vi.fn().mockResolvedValue(
      mockResponse({ ok: false, status: 500, text: "boom" })
    );

    await expect(HttpClient("api/thing", "GET")).rejects.toThrow("boom");
  });
});
