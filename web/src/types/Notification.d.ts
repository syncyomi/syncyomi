export type NotificationType = "DISCORD" | "NOTIFIARR" | "TELEGRAM" | "NTFY";
export type NotificationEvent =
  | "SYNC_STARTED"
  | "SYNC_SUCCESS"
  | "SYNC_FAILED"
  | "SYNC_ERROR"
  | "SERVER_UPDATE_AVAILABLE";

interface Notification {
  id: number;
  name: string;
  enabled: boolean;
  type: NotificationType;
  events: NotificationEvent[];
  webhook?: string;
  token?: string;
  api_key?: string;
  channel?: string;
}
