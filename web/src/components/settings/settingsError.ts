import { ApiError } from "@/api/ApiError";
import { serverMessage } from "@/components/auth/apiErrorMessage";
import { PasskeyPromptError } from "@/utils/passkeyErrors";

export const NETWORK_MESSAGE =
  "Could not reach the server. Check your connection and try again.";

export function settingsErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof TypeError) return NETWORK_MESSAGE;
  if (error instanceof ApiError) return serverMessage(error) || fallback;
  if (error instanceof PasskeyPromptError) return error.message;
  return (error instanceof Error && error.message.trim()) || fallback;
}
