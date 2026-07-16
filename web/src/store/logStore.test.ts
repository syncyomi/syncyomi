import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useLogsStore } from "./logStore";
import { LogEvent } from "@/types/Logs";

const logEvent = (message: string): LogEvent =>
  ({ message }) as LogEvent;

describe("logsStore", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("addLog appends to logs", () => {
    const store = useLogsStore();
    store.addLog(logEvent("first"));
    store.addLog(logEvent("second"));

    expect(store.logs).toHaveLength(2);
    expect(store.logs[1].message).toBe("second");
  });

  it("clearLogs empties both logs and filteredLogs", () => {
    const store = useLogsStore();
    store.addLog(logEvent("first"));
    store.filteredLogs = [logEvent("filtered")];

    store.clearLogs();

    expect(store.logs).toHaveLength(0);
    expect(store.filteredLogs).toHaveLength(0);
  });
});
