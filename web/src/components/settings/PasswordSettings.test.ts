import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, VueWrapper } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { vuetify } from "@/test/setup";
import { useAuthStore } from "@/store/auth/authStore";

const { changePasswordMock } = vi.hoisted(() => ({
  changePasswordMock: vi.fn(),
}));
vi.mock("@/api/APIClient", () => ({
  APIClient: { auth: { changePassword: changePasswordMock } },
}));

import PasswordSettings from "./PasswordSettings.vue";

const CURRENT = "current-pass-1";
const NEXT = "next-pass-2345";

const mountForm = () => {
  const pinia = createPinia();
  setActivePinia(pinia);
  useAuthStore().login("admin");
  return mount(PasswordSettings, {
    attachTo: document.body,
    global: { plugins: [vuetify, pinia, VueQueryPlugin] },
  });
};

const field = (wrapper: VueWrapper, autocomplete: string, index = 0) =>
  wrapper.findAll(`input[autocomplete="${autocomplete}"]`)[index];

const fill = async (
  wrapper: VueWrapper,
  values: { current?: string; next?: string; confirm?: string },
) => {
  if (values.current !== undefined)
    await field(wrapper, "current-password").setValue(values.current);
  if (values.next !== undefined)
    await field(wrapper, "new-password", 0).setValue(values.next);
  if (values.confirm !== undefined)
    await field(wrapper, "new-password", 1).setValue(values.confirm);
};

const submit = async (wrapper: VueWrapper) => {
  await wrapper.find("form").trigger("submit");
  await flushPromises();
};

describe("PasswordSettings", () => {
  beforeEach(() => {
    changePasswordMock.mockReset();
    document.body.innerHTML = "";
  });

  it("shows the signed in username so password managers can pair it", () => {
    const wrapper = mountForm();

    const username = wrapper.find<HTMLInputElement>(
      'input[autocomplete="username"]',
    );
    expect(username.element.value).toBe("admin");
    expect(username.attributes("readonly")).toBeDefined();
  });

  it("labels every password field", () => {
    const wrapper = mountForm();

    for (const label of [
      "Current password",
      "New password",
      "Confirm new password",
    ]) {
      expect(wrapper.text()).toContain(label);
    }
  });

  it("does not call the API when the form is empty", async () => {
    const wrapper = mountForm();

    await submit(wrapper);

    expect(changePasswordMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("Required");
  });

  it("rejects a new password shorter than 8 characters", async () => {
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: "short", confirm: "short" });

    await submit(wrapper);

    expect(changePasswordMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("at least 8 characters");
  });

  it("rejects a confirmation that does not match", async () => {
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: `${NEXT}x` });

    await submit(wrapper);

    expect(changePasswordMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("Passwords must match");
  });

  it("rejects a new password equal to the current one", async () => {
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: CURRENT, confirm: CURRENT });

    await submit(wrapper);

    expect(changePasswordMock).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("different from the current password");
  });

  it("sends both passwords, confirms success and clears the form", async () => {
    changePasswordMock.mockResolvedValue(undefined);
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: NEXT });

    await submit(wrapper);

    expect(changePasswordMock).toHaveBeenCalledWith(CURRENT, NEXT);
    expect(wrapper.find('.v-alert[role="alert"]').text()).toContain(
      "Password changed",
    );
    expect(field(wrapper, "current-password").element).toHaveProperty(
      "value",
      "",
    );
    expect(field(wrapper, "new-password", 0).element).toHaveProperty(
      "value",
      "",
    );
  });

  it("does not flag the cleared fields as required after a successful change", async () => {
    changePasswordMock.mockResolvedValue(undefined);
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: NEXT });

    await submit(wrapper);
    await flushPromises();

    expect(wrapper.text()).not.toContain("Required");
  });

  it("keeps showing the username after a successful change", async () => {
    changePasswordMock.mockResolvedValue(undefined);
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: NEXT });

    await submit(wrapper);

    expect(
      wrapper.find<HTMLInputElement>('input[autocomplete="username"]').element
        .value,
    ).toBe("admin");
  });

  it("shows the server's reason and keeps the form when the current password is wrong", async () => {
    changePasswordMock.mockRejectedValue(
      new Error("Current password is incorrect\n"),
    );
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: NEXT });

    await submit(wrapper);

    expect(wrapper.find('.v-alert[role="alert"]').text()).toBe(
      "Current password is incorrect",
    );
    expect(field(wrapper, "new-password", 0).element).toHaveProperty(
      "value",
      NEXT,
    );
  });

  it("explains a network failure instead of showing the raw error", async () => {
    changePasswordMock.mockRejectedValue(new TypeError("Failed to fetch"));
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: NEXT });

    await submit(wrapper);

    expect(wrapper.find('.v-alert[role="alert"]').text()).toContain(
      "Could not reach the server",
    );
  });

  it("ignores a second submit while the first is in flight", async () => {
    changePasswordMock.mockReturnValue(new Promise(() => {}));
    const wrapper = mountForm();
    await fill(wrapper, { current: CURRENT, next: NEXT, confirm: NEXT });

    await submit(wrapper);
    await submit(wrapper);

    expect(changePasswordMock).toHaveBeenCalledTimes(1);
  });

  it("reveals and hides a password with an accessible toggle", async () => {
    const wrapper = mountForm();
    const toggle = () =>
      wrapper
        .findAll("button")
        .find((b) => b.attributes("aria-label")?.includes("new password"))!;

    expect(field(wrapper, "new-password", 0).attributes("type")).toBe(
      "password",
    );
    await toggle().trigger("click");

    expect(field(wrapper, "new-password", 0).attributes("type")).toBe("text");
    expect(toggle().attributes("aria-pressed")).toBe("true");
    expect(toggle().attributes("type")).toBe("button");
  });

  it("shows strength as a bar without visible text and names it for screen readers", async () => {
    const wrapper = mountForm();

    await fill(wrapper, { next: "Tr0ub4dor&3-horse-staple" });

    const bar = wrapper.find('[role="progressbar"][aria-label]');
    expect(bar.attributes("aria-label")).toBe("Password strength: Strong");
    expect(wrapper.text()).not.toContain("Strong");
    expect(wrapper.text()).not.toContain("Strength");
  });

  it("only shows the length hint while the new password is empty", async () => {
    const wrapper = mountForm();
    expect(wrapper.text()).toContain("Use at least 8 characters.");

    await fill(wrapper, { next: "abcdefghijkl" });

    expect(wrapper.text()).not.toContain("Use at least 8 characters.");
  });
});
