import { ApiError } from "@/api/ApiError";

export const LOGIN_FAILED_MESSAGE =
  "Login failed. Please check your username and password.";

const TOO_MANY_REQUESTS = 429;

export function loginErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === TOO_MANY_REQUESTS) {
    return error.message.trim();
  }

  return LOGIN_FAILED_MESSAGE;
}
