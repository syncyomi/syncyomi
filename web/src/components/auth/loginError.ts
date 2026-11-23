import { ApiError } from "@/api/ApiError";
import { PasskeyPromptError } from "@/utils/passkeyErrors";
import {
  BAD_REQUEST,
  SERVER_ERROR,
  SERVICE_UNAVAILABLE,
  TOO_MANY_REQUESTS,
  serverMessage,
} from "./apiErrorMessage";

export const LOGIN_FAILED_MESSAGE =
  "Login failed. Please check your username and password.";
export const LOGIN_UNAVAILABLE_MESSAGE =
  "Could not sign in right now. Try again shortly.";

export function loginErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return LOGIN_FAILED_MESSAGE;

  if (
    error.status === TOO_MANY_REQUESTS ||
    error.status === SERVICE_UNAVAILABLE
  ) {
    return serverMessage(error) || LOGIN_UNAVAILABLE_MESSAGE;
  }

  return error.status >= SERVER_ERROR
    ? LOGIN_UNAVAILABLE_MESSAGE
    : LOGIN_FAILED_MESSAGE;
}

export const PASSKEY_FAILED_MESSAGE =
  "Passkey sign-in failed. Try again or use your password.";

export function passkeyErrorMessage(error: unknown): string {
  if (error instanceof PasskeyPromptError) return error.message;
  if (!(error instanceof ApiError)) return PASSKEY_FAILED_MESSAGE;

  if (
    error.status === BAD_REQUEST ||
    error.status === TOO_MANY_REQUESTS ||
    error.status === SERVICE_UNAVAILABLE
  ) {
    return serverMessage(error) || LOGIN_UNAVAILABLE_MESSAGE;
  }

  return error.status >= SERVER_ERROR
    ? LOGIN_UNAVAILABLE_MESSAGE
    : PASSKEY_FAILED_MESSAGE;
}
