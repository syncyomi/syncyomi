interface APIKey {
  name: string;
  key: string;
  scopes: string[];
  created_at: Date;
}

interface SyncStatus {
  last_synced_at: string | null;
  last_event_at: string | null;
  last_event: string;
  last_status: "" | "running" | "success" | "error" | "cancelled";
  last_device: string;
  last_message: string;
  data_size: number;
  data_updated_at: string | null;
}

interface SyncDevice {
  id: number;
  device_id: string;
  device_name: string;
  last_seen: string;
  last_event: string;
  last_status: string;
  last_message: string;
  protocol: "" | "v1" | "v2";
  cursor: number;
  created_at: string;
}

interface SyncHistoryEntry {
  id: number;
  etag: string;
  size: number;
  created_at: string;
}
