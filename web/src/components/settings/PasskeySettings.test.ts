import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, VueWrapper } from "@vue/test-utils";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { createPinia, setActivePinia } from "pinia";
import { vuetify } from "@/test/setup";
import { ApiError } from "@/api/ApiError";
import { useAuthStore } from "@/store/auth/authStore";
import { PasskeyPromptError } from "@/utils/passkeyErrors";

const mocks = vi.hoisted(() => ({
  capabilities: vi.fn(),
  list: vi.fn(),
  remove: vi.fn(),
  addPasskey: vi.fn(),
  passkeysAvailable: vi.fn(),
}));
vi.mock("@/api/APIClient", () => ({
  APIClient: {
    auth: { capabilities: mocks.capabilities },
    passkeys: { list: mocks.list, delete: mocks.remove },
  },
}));
vi.mock("@/utils/passkeys", () => ({
  addPasskey: mocks.addPasskey,
  passkeysAvailable: mocks.passkeysAvailable,
}));

import PasskeySettings from "./PasskeySettings.vue";

const VDialogStub = {
  name: "VDialog",
  props: ["modelValue"],
  template: `<div v-if="modelValue"><slot /></div>`,
};

const laptop: Passkey = {
  id: 1,
  name: "Laptop",
  created_at: "2026-01-02T03:04:05Z",
  last_used_at: null,
};

const mountSettings = async () => {
  const pinia = createPinia();
  setActivePinia(pinia);
  useAuthStore().login("admin");
  const wrapper = mount(PasskeySettings, {
    attachTo: document.body,
    global: {
      plugins: [vuetify, pinia, VueQueryPlugin],
      stubs: { VDialog: VDialogStub },
    },
  });
  await flushPromises();
  return wrapper;
};

const fieldFor = (wrapper: VueWrapper, autocomplete: string) =>
  wrapper.find(`input[autocomplete="${autocomplete}"]`);

const fillAndAdd = async (
  wrapper: VueWrapper,
  name: string,
  password: string,
) => {
  await wrapper.find('input[name="passkey-name"]').setValue(name);
  await fieldFor(wrapper, "current-password").setValue(password);
  await wrapper.find("form").trigger("submit");
  await flushPromises();
};

describe("PasskeySettings", () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset());
    mocks.capabilities.mockResolvedValue({ passkeys: true });
    mocks.passkeysAvailable.mockReturnValue(true);
    mocks.list.mockResolvedValue([laptop]);
    document.body.innerHTML = "";
  });

  it("lists the passkeys with when they were added and used", async () => {
    mocks.list.mockResolvedValue([
      laptop,
      { ...laptop, id: 2, name: "Phone", last_used_at: "2026-02-03T04:05:06Z" },
    ]);

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("Laptop");
    expect(wrapper.text()).toContain("Never used");
    expect(wrapper.text()).toContain("Phone");
    expect(wrapper.text()).not.toContain("Never used Never used");
  });

  it("says there are none yet", async () => {
    mocks.list.mockResolvedValue([]);

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("No passkeys yet");
  });

  it("explains that passkeys are off on the server and offers no form", async () => {
    mocks.capabilities.mockResolvedValue({ passkeys: false });

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("webauthnOrigin");
    expect(wrapper.find("form").exists()).toBe(false);
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it("explains that the page needs a secure address, but still lists passkeys", async () => {
    mocks.passkeysAvailable.mockReturnValue(false);

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("https");
    expect(wrapper.text()).toContain("Laptop");
    expect(wrapper.find("form").exists()).toBe(false);
  });

  it("asks for a name and the current password before doing anything", async () => {
    const wrapper = await mountSettings();

    await wrapper.find("form").trigger("submit");
    await flushPromises();

    expect(mocks.addPasskey).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("Required");
  });

  it("moves focus to the first field that needs attention", async () => {
    const wrapper = await mountSettings();

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(document.activeElement).toBe(
      wrapper.find('input[name="passkey-name"]').element,
    );

    await wrapper.find('input[name="passkey-name"]').setValue("Phone");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(document.activeElement).toBe(
      fieldFor(wrapper, "current-password").element,
    );
  });

  it("shows the signed in username so password managers can pair the password", async () => {
    const wrapper = await mountSettings();

    const username = wrapper.find<HTMLInputElement>(
      'input[autocomplete="username"]',
    );
    expect(username.element.value).toBe("admin");
    expect(username.attributes("readonly")).toBeDefined();
  });

  it("accepts a long name made of multi-unit characters", async () => {
    mocks.addPasskey.mockResolvedValue({ ...laptop, id: 2 });
    const wrapper = await mountSettings();
    const name = "😀".repeat(40);

    await fillAndAdd(wrapper, `  ${name}  `, "right-password-1");

    expect(mocks.addPasskey).toHaveBeenCalledWith(name, "right-password-1");
  });

  it("does not flag the cleared fields as required while the list refreshes", async () => {
    mocks.addPasskey.mockResolvedValue({ ...laptop, id: 2 });
    const wrapper = await mountSettings();
    mocks.list.mockImplementation(() => new Promise(() => {}));

    await fillAndAdd(wrapper, "Phone", "right-password-1");

    expect(wrapper.text()).not.toContain("Required");
  });

  it("refreshes the list after a failed add, for example at the limit", async () => {
    mocks.addPasskey.mockRejectedValue(
      new ApiError("The passkey limit is reached\n", 409),
    );
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "right-password-1");

    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it("does not print an HTML error page from a proxy", async () => {
    mocks.addPasskey.mockRejectedValue(
      new ApiError("<html><body>502 Bad Gateway</body></html>", 502),
    );
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "right-password-1");

    expect(wrapper.find('.v-alert[role="alert"]').text()).toBe(
      "Could not add the passkey. Try again.",
    );
  });

  it("explains a network failure", async () => {
    mocks.addPasskey.mockRejectedValue(new TypeError("Failed to fetch"));
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "right-password-1");

    expect(wrapper.find('.v-alert[role="alert"]').text()).toContain(
      "Could not reach the server",
    );
  });

  it("says when the sign-in methods cannot be loaded", async () => {
    mocks.capabilities.mockRejectedValue(new TypeError("Failed to fetch"));

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("Could not load your passkeys");
    expect(wrapper.find("form").exists()).toBe(false);
  });

  it("says when the passkey list cannot be loaded", async () => {
    mocks.list.mockRejectedValue(new TypeError("Failed to fetch"));

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("Could not load your passkeys");
    expect(wrapper.find("form").exists()).toBe(false);
  });

  it("refreshes the list and explains a removal that failed", async () => {
    mocks.remove.mockRejectedValue(new ApiError("404 page not found\n", 404));
    const wrapper = await mountSettings();

    await wrapper
      .find('button[aria-label="Remove passkey Laptop"]')
      .trigger("click");
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("Remove"))
      ?.trigger("click");
    await flushPromises();

    expect(mocks.list).toHaveBeenCalledTimes(2);
    expect(wrapper.find('.v-alert[role="alert"]').text()).toContain(
      "404 page not found",
    );
  });

  it("adds a passkey, clears the form and refreshes the list", async () => {
    mocks.addPasskey.mockResolvedValue({ ...laptop, id: 2, name: "Phone" });
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "right-password-1");

    expect(mocks.addPasskey).toHaveBeenCalledWith("Phone", "right-password-1");
    expect(mocks.list).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("Passkey added");
    expect(
      (fieldFor(wrapper, "current-password").element as HTMLInputElement).value,
    ).toBe("");
    expect(
      (wrapper.find('input[name="passkey-name"]').element as HTMLInputElement)
        .value,
    ).toBe("");
  });

  it("shows why adding failed and keeps the name", async () => {
    mocks.addPasskey.mockRejectedValue(
      new PasskeyPromptError("The passkey prompt was cancelled or timed out."),
    );
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "right-password-1");

    expect(wrapper.find('.v-alert[role="alert"]').text()).toContain(
      "The passkey prompt was cancelled or timed out.",
    );
    expect(
      (wrapper.find('input[name="passkey-name"]').element as HTMLInputElement)
        .value,
    ).toBe("Phone");
    expect(
      (fieldFor(wrapper, "current-password").element as HTMLInputElement).value,
    ).toBe("");
  });

  it("does not flag the cleared password as required after a failed attempt", async () => {
    mocks.addPasskey.mockRejectedValue(
      new Error("Current password is incorrect"),
    );
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "wrong-password-1");
    await flushPromises();

    expect(wrapper.text()).not.toContain("Required");
  });

  it("shows the server's message, for example a wrong password", async () => {
    mocks.addPasskey.mockRejectedValue(
      new Error("Current password is incorrect\n"),
    );
    const wrapper = await mountSettings();

    await fillAndAdd(wrapper, "Phone", "wrong-password-1");

    expect(wrapper.find('.v-alert[role="alert"]').text()).toBe(
      "Current password is incorrect",
    );
  });

  it("removes a passkey only after confirmation", async () => {
    mocks.remove.mockResolvedValue(undefined);
    const wrapper = await mountSettings();

    await wrapper
      .find('button[aria-label="Remove passkey Laptop"]')
      .trigger("click");
    await flushPromises();
    expect(mocks.remove).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("Remove passkey");

    const confirm = wrapper
      .findAll("button")
      .find((b) => b.text().includes("Remove"));
    await confirm?.trigger("click");
    await flushPromises();

    expect(mocks.remove).toHaveBeenCalledWith(1);
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it("stops offering to add passkeys at the limit", async () => {
    mocks.list.mockResolvedValue(
      Array.from({ length: 10 }, (_, i) => ({
        ...laptop,
        id: i + 1,
        name: `Key ${i + 1}`,
      })),
    );

    const wrapper = await mountSettings();

    expect(wrapper.text()).toContain("limit");
    expect(wrapper.find("form").exists()).toBe(false);
  });
});
