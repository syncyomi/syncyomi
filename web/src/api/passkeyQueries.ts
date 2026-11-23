import { queryOptions } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";

export const PASSKEYS_KEY = ["passkeys"] as const;

export const capabilitiesQuery = queryOptions({
  queryKey: ["auth", "capabilities"],
  queryFn: () => APIClient.auth.capabilities(),
  retry: false,
});

export const passkeysQuery = queryOptions({
  queryKey: PASSKEYS_KEY,
  queryFn: () => APIClient.passkeys.list(),
  retry: false,
});
