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
    deleteDevice: vi.fn(),
    downloadHistoryUrl: vi.fn(
      (key: string, id: number) => `/api/sync/admin/${key}/history/${id}/download`,
    ),
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
  // the status payload of a server that predates protocol/seq/count fields
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

    // the status predates last_protocol, but a v1 device still triggers the banner
    expect(wrapper.text()).toContain("Legacy sync protocol in use");
    // no library data from an old server: no extra rows
    expect(tables[0].text()).not.toContain("Library");

    const deviceRows = tables[1].findAll("tbody tr");
    expect(deviceRows).toHaveLength(2);
    expect(deviceRows[0].text()).toContain("Phone");
    expect(deviceRows[1].text()).toContain("dev-b");
    expect(deviceRows[1].text()).toContain("legacy");
    // seq unknown: lag cannot be computed
    expect(deviceRows[0].find('[aria-label="Up to date"]').exists()).toBe(false);

    const historyRows = tables[2].findAll("tbody tr");
    expect(historyRows).toHaveLength(2);
    expect(historyRows[0].text()).toContain("Current");
    expect(historyRows[0].text()).toContain("server merge");
    expect(historyRows[0].text()).not.toContain("Restore");
    expect(historyRows[1].text()).toContain("4.8 MB");
    expect(historyRows[1].text()).toContain("Restore");
  });

  it("flags a v1 key and shows lag and library stats", async () => {
    sync.status.mockResolvedValue({
      last_synced_at: "2026-08-30T15:04:27Z",
      last_event_at: null,
      last_event: "",
      last_status: "",
      last_device: "",
      last_message: "",
      last_protocol: "v1",
      data_size: 1024,
      data_updated_at: "2026-08-30T15:04:27Z",
      seq: 12,
      manga_count: 42,
      chapter_count: 800,
      category_count: 3,
      history_limit: 10,
    });
    sync.devices.mockResolvedValue([
      {
        id: 3,
        device_id: "legacy",
        device_name: "",
        last_seen: "2026-08-30T15:04:27Z",
        last_event: "",
        last_status: "",
        last_message: "",
        protocol: "v1",
        cursor: 12,
        created_at: "2026-08-30T15:00:00Z",
      },
      {
        id: 4,
        device_id: "dev-c",
        device_name: "Tablet",
        last_seen: "2026-08-30T15:00:00Z",
        last_event: "",
        last_status: "",
        last_message: "",
        protocol: "v2",
        cursor: 9,
        created_at: "2026-08-30T15:00:00Z",
      },
    ]);
    sync.history.mockResolvedValue([
      { id: 11, etag: "uuid=abc", size: 1024, created_at: "2026-08-30T15:04:27Z" },
    ]);

    const wrapper = await mountDetails();
    expect(wrapper.text()).toContain("Legacy sync protocol in use");

    const tables = wrapper.findAll("table");
    expect(tables[0].text()).toContain("v1 · legacy");
    expect(tables[0].text()).toContain("42 manga");
    expect(tables[0].text()).toContain("800 chapters");
    expect(tables[0].text()).toContain("3 categories");
    expect(wrapper.text()).toContain("Keeps the last 10 payloads.");

    const deviceRows = tables[1].findAll("tbody tr");
    expect(deviceRows[0].text()).toContain("Legacy device");
    expect(deviceRows[0].find('[aria-label="Up to date"]').exists()).toBe(true);
    expect(deviceRows[1].text()).toContain("3 behind");

    const historyRows = tables[2].findAll("tbody tr");
    expect(historyRows[0].text()).toContain("device upload");
    const download = historyRows[0].find("a[href]");
    expect(download.attributes("href")).toContain("/history/11/download");
  });

  it("collapses the legacy aggregate into a tagged device", async () => {
    sync.status.mockResolvedValue({
      last_synced_at: "2026-09-02T01:13:29Z",
      last_event_at: "2026-09-02T01:13:29Z",
      last_event: "SYNC_SUCCESS",
      last_status: "success",
      last_device: "My Phone",
      last_message: "",
      last_protocol: "v1",
      data_size: 1024,
      data_updated_at: "2026-09-02T01:13:29Z",
      seq: 4,
      manga_count: 359,
      chapter_count: 42137,
      category_count: 11,
      history_limit: 10,
    });
    sync.devices.mockResolvedValue([
      {
        id: 1,
        device_id: "My Phone",
        device_name: "My Phone",
        last_seen: "2026-09-02T01:13:29Z",
        last_event: "SYNC_SUCCESS",
        last_status: "success",
        last_message: "",
        protocol: "v1",
        cursor: 0,
        created_at: "2026-09-02T01:00:00Z",
      },
      {
        id: 2,
        device_id: "legacy",
        device_name: "",
        last_seen: "2026-09-02T01:13:29Z",
        last_event: "",
        last_status: "",
        last_message: "",
        protocol: "v1",
        cursor: 4,
        created_at: "2026-09-02T01:00:00Z",
      },
    ]);
    sync.history.mockResolvedValue([]);

    const wrapper = await mountDetails();
    const deviceRows = wrapper.findAll("table")[1].findAll("tbody tr");
    expect(deviceRows).toHaveLength(1);
    expect(deviceRows[0].text()).toContain("My Phone");
    expect(deviceRows[0].text()).not.toContain("Legacy device");
    // the tagged row's cursor is not real; no lag state is invented for it
    expect(deviceRows[0].find('[aria-label="Up to date"]').exists()).toBe(false);
    expect(deviceRows[0].text()).not.toContain("behind");
  });

  it("shows empty states", async () => {
    sync.status.mockRejectedValue(new Error("404"));
    sync.devices.mockResolvedValue([]);
    sync.history.mockResolvedValue([]);

    const wrapper = await mountDetails();
    expect(wrapper.text()).toContain("No sync recorded yet.");
    expect(wrapper.text()).toContain("No devices seen yet.");
    expect(wrapper.text()).toContain("No previous payloads kept.");
    expect(wrapper.text()).not.toContain("Legacy sync protocol in use");
  });
});
