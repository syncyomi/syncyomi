export type NotificationType =
  | "DISCORD"
  | "NOTIFIARR"
  | "TELEGRAM"
  | "NTFY"
  | "WEBHOOK";
export type NotificationEvent =
  | "SYNC_STARTED"
  | "SYNC_SUCCESS"
  | "SYNC_FAILED"
  | "SYNC_ERROR"
  | "SYNC_CANCELLED"
  | "SERVER_UPDATE_AVAILABLE"
  | "PASSWORD_CHANGED"
  | "LOGIN_BLOCKED"
  | "PASSKEY_ADDED";

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
