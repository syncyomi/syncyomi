import { ApiError } from "@/api/ApiError";
import {
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
