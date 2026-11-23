import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { vuetify } from "@/test/setup";
import { ApiError } from "@/api/ApiError";
import { PasskeyPromptError } from "@/utils/passkeyErrors";
import { useAuthStore } from "@/store/auth/authStore";

const mocks = vi.hoisted(() => ({
  canOnboard: vi.fn(),
  capabilities: vi.fn(),
  login: vi.fn(),
  signInWithPasskey: vi.fn(),
  passkeysAvailable: vi.fn(),
  push: vi.fn(),
}));
vi.mock("@/api/APIClient", () => ({
  APIClient: {
    auth: {
      canOnboard: mocks.canOnboard,
      capabilities: mocks.capabilities,
      login: mocks.login,
    },
  },
}));
vi.mock("@/utils/passkeys", () => ({
  passkeysAvailable: mocks.passkeysAvailable,
  signInWithPasskey: mocks.signInWithPasskey,
}));
vi.mock("vue-router", () => ({ useRouter: () => ({ push: mocks.push }) }));

import Login from "./Login.vue";

const VSnackbarStub = {
  name: "VSnackbar",
  props: ["modelValue"],
  template: `<div v-if="modelValue"><slot /></div>`,
};

const mountLogin = async () => {
  const pinia = createPinia();
  setActivePinia(pinia);
  const wrapper = mount(Login, {
    attachTo: document.body,
    global: {
      plugins: [vuetify, pinia, VueQueryPlugin],
      stubs: { VSnackbar: VSnackbarStub },
    },
  });
  await flushPromises();
  return wrapper;
};

const passkeyButton = (wrapper: Awaited<ReturnType<typeof mountLogin>>) =>
  wrapper.findAll("button").find((b) => b.text().includes("passkey"));

describe("Login", () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset());
    mocks.canOnboard.mockRejectedValue(new Error("onboarding unavailable"));
    mocks.capabilities.mockResolvedValue({ passkeys: true });
    mocks.passkeysAvailable.mockReturnValue(true);
    document.body.innerHTML = "";
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  it("offers a passkey button when the server and the browser both support it", async () => {
    const wrapper = await mountLogin();

    expect(passkeyButton(wrapper)).toBeDefined();
  });

  it.each([
    [
      "the server has passkeys off",
      () => mocks.capabilities.mockResolvedValue({ passkeys: false }),
    ],
    [
      "the browser cannot do passkeys here",
      () => mocks.passkeysAvailable.mockReturnValue(false),
    ],
    [
      "the capabilities request fails",
      () => mocks.capabilities.mockRejectedValue(new Error("down")),
    ],
  ])("hides the passkey button when %s", async (_, arrange) => {
    arrange();

    const wrapper = await mountLogin();

    expect(passkeyButton(wrapper)).toBeUndefined();
  });

  it("signs in with the passkey and remembers who signed in", async () => {
    mocks.signInWithPasskey.mockResolvedValue("admin");
    const wrapper = await mountLogin();

    await passkeyButton(wrapper)!.trigger("click");
    await flushPromises();

    expect(useAuthStore().loggedInUser).toBe("admin");
    expect(mocks.push).toHaveBeenCalledWith("/");
  });

  it("shows why a passkey prompt failed and stays on the page", async () => {
    mocks.signInWithPasskey.mockRejectedValue(
      new PasskeyPromptError("The passkey prompt was cancelled or timed out."),
    );
    const wrapper = await mountLogin();

    await passkeyButton(wrapper)!.trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain(
      "The passkey prompt was cancelled or timed out.",
    );
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("shows the server's rate limit message for passkeys too", async () => {
    mocks.signInWithPasskey.mockRejectedValue(
      new ApiError("Too many failed attempts. Try again in 12 minutes.", 429),
    );
    const wrapper = await mountLogin();

    await passkeyButton(wrapper)!.trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain(
      "Too many failed attempts. Try again in 12 minutes.",
    );
  });
});
