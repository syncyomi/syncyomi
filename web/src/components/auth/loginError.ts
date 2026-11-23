export const LOGIN_FAILED_MESSAGE =
  "Login failed. Please check your username and password.";

const BLOCKED_PREFIX = "Too many failed attempts";

export function loginErrorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message.trim() : "";
  return message.startsWith(BLOCKED_PREFIX) ? message : LOGIN_FAILED_MESSAGE;
}
