import { describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { QueryClient, VueQueryPlugin } from "@tanstack/vue-query";
import { vuetify } from "@/test/setup";
import SyncKeyDetails from "./SyncKeyDetails.vue";

const { sync } = vi.hoisted(() => ({
  sync: {
    status: vi.fn(),
    devices: vi.fn(),
    history: vi.fn(),
    restore: vi.fn(),
  },
}));

vi.mock("@/api/APIClient", () => ({ APIClient: { sync } }));

const mountDetails = async () => {
  const wrapper = mount(SyncKeyDetails, {
    props: { apiKey: "key1" },
    global: {
      plugins: [
        vuetify,
        [VueQueryPlugin, { queryClient: new QueryClient() }],
      ],
      stubs: { ConfirmationModal: true },
    },
  });
  await flushPromises();
  return wrapper;
};

describe("SyncKeyDetails", () => {
  it("renders devices and history as table rows", async () => {
    sync.status.mockResolvedValue({
      last_synced_at: "2026-08-30T15:04:27Z",
      last_event_at: "2026-08-30T15:04:27Z",
      last_event: "SYNC_SUCCESS",
      last_status: "success",
      last_device: "Phone",
      last_message: "",
      data_size: 5033164,
      data_updated_at: "2026-08-30T15:04:27Z",
    });
    sync.devices.mockResolvedValue([
      {
        id: 1,
        device_id: "dev-a",
        device_name: "Phone",
        last_seen: "2026-08-30T15:04:27Z",
        last_event: "SYNC_SUCCESS",
        last_status: "success",
        last_message: "",
        protocol: "v2",
        cursor: 12,
        created_at: "2026-08-30T15:04:27Z",
      },
      {
        id: 2,
        device_id: "dev-b",
        device_name: "",
        last_seen: "2026-08-30T15:00:00Z",
        last_event: "",
        last_status: "",
        last_message: "",
        protocol: "v1",
        cursor: 0,
        created_at: "2026-08-30T15:00:00Z",
      },
    ]);
    sync.history.mockResolvedValue([
      { id: 10, etag: "seq=12", size: 5033164, created_at: "2026-08-30T15:04:27Z" },
      { id: 9, etag: "seq=11", size: 5033000, created_at: "2026-08-30T15:00:00Z" },
    ]);

    const wrapper = await mountDetails();
    const tables = wrapper.findAll("table");
    expect(tables).toHaveLength(3);

    const deviceRows = tables[1].findAll("tbody tr");
    expect(deviceRows).toHaveLength(2);
    expect(deviceRows[0].text()).toContain("Phone");
    expect(deviceRows[1].text()).toContain("dev-b");
    expect(deviceRows[1].text()).toContain("legacy");

    const historyRows = tables[2].findAll("tbody tr");
    expect(historyRows).toHaveLength(2);
    expect(historyRows[0].text()).toContain("current");
    expect(historyRows[0].find("button").exists()).toBe(false);
    expect(historyRows[1].text()).toContain("4.8 MB");
    expect(historyRows[1].find("button").text()).toContain("Restore");
  });

  it("shows empty states", async () => {
    sync.status.mockRejectedValue(new Error("404"));
    sync.devices.mockResolvedValue([]);
    sync.history.mockResolvedValue([]);

    const wrapper = await mountDetails();
    expect(wrapper.text()).toContain("No sync recorded yet.");
    expect(wrapper.text()).toContain("No devices seen yet.");
    expect(wrapper.text()).toContain("No previous payloads kept.");
  });
});
