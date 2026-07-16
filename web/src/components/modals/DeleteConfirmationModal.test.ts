import { describe, expect, it } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { vuetify } from "@/test/setup";
import DeleteConfirmationModal from "./DeleteConfirmationModal.vue";

// v-dialog's overlay/teleport/transition machinery doesn't survive happy-dom, and
// it isn't what we're testing. Stub it to a passthrough that honours the model, so
// the content renders inline in the wrapper and we exercise the component's own
// open/confirm/cancel logic.
const VDialogStub = {
  name: "VDialog",
  props: ["modelValue"],
  template: `<div v-if="modelValue"><slot /></div>`,
};

const mountModal = () =>
  mount(DeleteConfirmationModal, {
    props: { title: "Delete thing", message: "Are you sure?" },
    global: { plugins: [vuetify], stubs: { VDialog: VDialogStub } },
  });

const clickButton = async (
  wrapper: ReturnType<typeof mountModal>,
  label: string
) => {
  const btn = wrapper
    .findAll("button")
    .find((b) => b.text().includes(label));
  await btn?.trigger("click");
  await flushPromises();
};

const open = async (wrapper: ReturnType<typeof mountModal>) => {
  (wrapper.vm as unknown as { showModal: () => void }).showModal();
  await flushPromises();
};

describe("DeleteConfirmationModal", () => {
  it("is closed until showModal is called", async () => {
    const wrapper = mountModal();
    expect(wrapper.text()).not.toContain("Are you sure?");

    await open(wrapper);
    expect(wrapper.text()).toContain("Are you sure?");
  });

  it("emits confirmed when Confirm is clicked", async () => {
    const wrapper = mountModal();
    await open(wrapper);
    await clickButton(wrapper, "Confirm");

    expect(wrapper.emitted("confirmed")).toBeTruthy();
    expect(wrapper.emitted("canceled")).toBeFalsy();
  });

  it("emits canceled when Cancel is clicked", async () => {
    const wrapper = mountModal();
    await open(wrapper);
    await clickButton(wrapper, "Cancel");

    expect(wrapper.emitted("canceled")).toBeTruthy();
    expect(wrapper.emitted("confirmed")).toBeFalsy();
  });
});
