import { ApiError } from "@/api/ApiError";

export const LOGIN_FAILED_MESSAGE =
  "Login failed. Please check your username and password.";
export const LOGIN_UNAVAILABLE_MESSAGE =
  "Could not sign in right now. Try again shortly.";

const TOO_MANY_REQUESTS = 429;
const SERVICE_UNAVAILABLE = 503;
const SERVER_ERROR = 500;

export function loginErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return LOGIN_FAILED_MESSAGE;

  if (
    error.status === TOO_MANY_REQUESTS ||
    error.status === SERVICE_UNAVAILABLE
  ) {
    return error.message.trim();
  }

  return error.status >= SERVER_ERROR
    ? LOGIN_UNAVAILABLE_MESSAGE
    : LOGIN_FAILED_MESSAGE;
}
