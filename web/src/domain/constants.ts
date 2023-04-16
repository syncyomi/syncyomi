import { NotificationType } from "@/types/Notification";

const logLevel = ["DEBUG", "INFO", "WARN", "ERROR", "TRACE"] as const;

export const LogLevelOptions = logLevel.map((v) => ({
  value: v,
  title: v,
  key: v,
}));

export interface OptionBasicTyped<T> {
  title: string;
  value: T;
}

export const NotificationTypeOptions: OptionBasicTyped<NotificationType>[] = [
  {
    title: "Discord",
    value: "DISCORD",
  },
  {
    title: "Notifiarr",
    value: "NOTIFIARR",
  },
  {
    title: "Telegram",
    value: "TELEGRAM",
  },
];

export const EventOptions = [
  {
    title: "Server Update Available",
    value: "SERVER_UPDATE_AVAILABLE",
    subtitle: "A server update is available for download",
    enabled: false,
  },
  {
    title: "Sync Started",
    value: "SYNC_STARTED",
    subtitle: "Synchronization process has started",
    enabled: false,
  },
  {
    title: "Sync Success",
    value: "SYNC_SUCCESS",
    subtitle: "Synchronization process completed successfully",
    enabled: false,
  },
  {
    title: "Sync Failed",
    value: "SYNC_FAILED",
    subtitle: "Synchronization process failed to complete",
    enabled: false,
  },
  {
    title: "Sync Error",
    value: "SYNC_ERROR",
    subtitle: "An error occurred during the synchronization process",
    enabled: false,
  },
];
