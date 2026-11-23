import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, VueWrapper } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { vuetify } from "@/test/setup";

const { onboardMock, loginMock, pushMock } = vi.hoisted(() => ({
  onboardMock: vi.fn(),
  loginMock: vi.fn(),
  pushMock: vi.fn(),
}));
vi.mock("@/api/APIClient", () => ({
  APIClient: { auth: { onboard: onboardMock, login: loginMock } },
}));
vi.mock("vue-router", () => ({ useRouter: () => ({ push: pushMock }) }));

import OnBoard from "./OnBoard.vue";

const mountOnboard = () => {
  const pinia = createPinia();
  setActivePinia(pinia);
  return mount(OnBoard, {
    attachTo: document.body,
    global: { plugins: [vuetify, pinia, VueQueryPlugin] },
  });
};

const fillAndSubmit = async (wrapper: VueWrapper, password: string) => {
  const [username, first, second] = wrapper.findAll("input");
  await username.setValue("admin");
  await flushPromises();
  await first.setValue(password);
  await flushPromises();
  await second.setValue(password);
  await flushPromises();
  await wrapper.find("form").trigger("submit");
  await flushPromises();
};

describe("OnBoard", () => {
  beforeEach(() => {
    onboardMock.mockReset().mockResolvedValue(undefined);
    loginMock.mockReset().mockResolvedValue(undefined);
    pushMock.mockReset();
    document.body.innerHTML = "";
  });

  it("rejects a password of 7 characters", async () => {
    const wrapper = mountOnboard();

    await fillAndSubmit(wrapper, "1234567");

    expect(onboardMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("Min 8 characters");
  });

  it("accepts a password of 8 characters", async () => {
    const wrapper = mountOnboard();

    await fillAndSubmit(wrapper, "12345678");

    expect(onboardMock).toHaveBeenCalledWith("admin", "12345678");
  });

  it("rejects a password over 1024 bytes", async () => {
    const wrapper = mountOnboard();

    await fillAndSubmit(wrapper, "a".repeat(1025));

    expect(onboardMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("Max 1024 bytes");
  });
});
