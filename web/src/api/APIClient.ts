import { baseUrl, sseBaseUrl } from "@/utils";
import { GithubRelease } from "@/types/Update";
import { useAuthStore } from "@/store/auth/authStore";
import router from "@/router";
import { Notification } from "@/types/Notification";
import { ApiError } from "./ApiError";
import type { CreationOptionsJSON, RequestOptionsJSON } from "@/utils/webauthn";

interface ConfigType {
  body?: BodyInit | Record<string, unknown> | unknown;
  headers?: Record<string, string>;
}

type PostBody = BodyInit | Record<string, unknown> | unknown;

export async function HttpClient<T>(
  endpoint: string,
  method: string,
  { body, ...customConfig }: ConfigType = {},
): Promise<T> {
  const config = {
    method: method,
    body: body ? JSON.stringify(body) : undefined,
    headers: {
      "Content-Type": "application/json",
    },
    // NOTE: customConfig can override the above defined settings
    ...customConfig,
  } as RequestInit;

  return window
    .fetch(`${baseUrl()}${endpoint}`, config)
    .then(async (response) => {
      if (!response.ok) {
        // if 401 consider the session expired and force logout
        if (response.status === 401) {
          // Remove auth info from state
          const authStore = useAuthStore();
          authStore.logout();
          // push to log in only if not already there
          if (router.currentRoute.value.path !== "/login") {
            await router.push({ name: "Login" });
          }

          // Show an error toast to notify the user what occurred
          return Promise.reject(new Error("Unauthorized"));
        }

        return Promise.reject(
          new ApiError(await response.text(), response.status),
        );
      }

      // Resolve immediately since 204 contains no data
      if (response.status === 204) return Promise.resolve(response);

      return await response.json();
    });
}

const appClient = {
  Get: <T>(endpoint: string) => HttpClient<T>(endpoint, "GET"),
  Post: <T = void>(endpoint: string, data: PostBody = undefined) =>
    HttpClient<T>(endpoint, "POST", { body: data }),
  Put: (endpoint: string, data: PostBody) =>
    HttpClient<void>(endpoint, "PUT", { body: data }),
  Patch: (endpoint: string, data: PostBody = undefined) =>
    HttpClient<void>(endpoint, "PATCH", { body: data }),
  Delete: (endpoint: string) => HttpClient<void>(endpoint, "DELETE"),
};

export const APIClient = {
  auth: {
    login: (username: string, password: string) =>
      appClient.Post("api/auth/login", {
        username: username,
        password: password,
      }),
    logout: () => appClient.Post("api/auth/logout"),
    validate: () => appClient.Get<void>("api/auth/validate"),
    onboard: (username: string, password: string) =>
      appClient.Post("api/auth/onboard", {
        username: username,
        password: password,
      }),
    canOnboard: () => appClient.Get("api/auth/onboard"),
    capabilities: () =>
      appClient.Get<{ passkeys: boolean }>("api/auth/capabilities"),
    beginPasskeyLogin: () =>
      appClient.Post<RequestOptionsJSON>("api/auth/passkey/login/begin"),
    finishPasskeyLogin: (credential: object) =>
      appClient.Post<{ username: string }>(
        "api/auth/passkey/login/finish",
        credential,
      ),
    changePassword: (
      currentPassword: string,
      newPassword: string,
      revokePasskeys: boolean,
    ) =>
      appClient.Post("api/auth/password", {
        current_password: currentPassword,
        new_password: newPassword,
        revoke_passkeys: revokePasskeys,
      }),
  },
  passkeys: {
    list: () => appClient.Get<Passkey[]>("api/auth/passkeys"),
    beginRegistration: (name: string, currentPassword: string) =>
      appClient.Post<CreationOptionsJSON>("api/auth/passkeys/register/begin", {
        name,
        current_password: currentPassword,
      }),
    finishRegistration: (credential: object) =>
      appClient.Post<Passkey>("api/auth/passkeys/register/finish", credential),
    delete: (id: number) => appClient.Delete(`api/auth/passkeys/${id}`),
  },
  apikeys: {
    getAll: () => appClient.Get<APIKey[]>("api/keys"),
    create: (key: APIKey) => appClient.Post("api/keys", key),
    delete: (key: string) => appClient.Delete(`api/keys/${key}`),
  },
  sync: {
    status: (key: string) =>
      appClient.Get<SyncStatus>(`api/sync/admin/${key}/status`),
    devices: (key: string) =>
      appClient.Get<SyncDevice[]>(`api/sync/admin/${key}/devices`),
    history: (key: string) =>
      appClient.Get<SyncHistoryEntry[]>(`api/sync/admin/${key}/history`),
    restore: (key: string, id: number) =>
      appClient.Post<{ etag: string }>(
        `api/sync/admin/${key}/history/${id}/restore`,
      ),
    deleteDevice: (key: string, id: number) =>
      appClient.Delete(`api/sync/admin/${key}/devices/${id}`),
    downloadHistoryUrl: (key: string, id: number) =>
      `${baseUrl()}api/sync/admin/${key}/history/${id}/download`,
  },
  config: {
    get: () => appClient.Get<Config>("api/config"),
    update: (config: ConfigUpdate) => appClient.Patch("api/config", config),
  },
  logs: {
    files: () => appClient.Get<LogFileResponse>("api/logs/files"),
    getFile: (file: string) => appClient.Get(`api/logs/files/${file}`),
  },
  events: {
    logs: () =>
      new EventSource(`${sseBaseUrl()}api/events?stream=logs`, {
        withCredentials: true,
      }),
  },
  notifications: {
    getAll: () => appClient.Get<Notification[]>("api/notification"),
    create: (notification: Notification) =>
      appClient.Post("api/notification", notification),
    update: (notification: Notification) =>
      appClient.Put(`api/notification/${notification.id}`, notification),
    delete: (id: number) => appClient.Delete(`api/notification/${id}`),
    test: (n: Notification) => appClient.Post("api/notification/test", n),
  },
  updates: {
    check: () => appClient.Get("api/updates/check"),
    getLatestRelease: () =>
      appClient.Get<GithubRelease | undefined>("api/updates/latest"),
  },
};
