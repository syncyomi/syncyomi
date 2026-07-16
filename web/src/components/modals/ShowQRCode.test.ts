import { describe, expect, it } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { vuetify } from "@/test/setup";
import ShowQRCode from "./ShowQRCode.vue";

// See DeleteConfirmationModal.test.ts: stub v-dialog to a model-honouring
// passthrough so the QR renders inline without the overlay machinery.
const VDialogStub = {
  name: "VDialog",
  props: ["modelValue"],
  template: `<div v-if="modelValue"><slot /></div>`,
};

describe("ShowQRCode", () => {
  it("renders a QR code for the given key once opened", async () => {
    const wrapper = mount(ShowQRCode, {
      global: { plugins: [vuetify], stubs: { VDialog: VDialogStub } },
    });

    // Closed: no QR yet.
    expect(wrapper.find("svg").exists()).toBe(false);

    (wrapper.vm as unknown as { showModal: (k: string) => void }).showModal(
      "api-key-123"
    );
    await flushPromises();

    // qrcode.vue renders the code as an <svg>.
    expect(wrapper.find("svg").exists()).toBe(true);
  });
});
