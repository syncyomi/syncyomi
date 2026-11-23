import { ApiError } from "@/api/ApiError";
import { LOGIN_UNAVAILABLE_MESSAGE, serverMessage } from "./loginError";

export const ONBOARD_FAILED_MESSAGE =
  "Failed to create account! currently only one account is allowed, make sure browser cookies are cleared.";

const BAD_REQUEST = 400;
const SERVICE_UNAVAILABLE = 503;
const SERVER_ERROR = 500;

export function onboardErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return ONBOARD_FAILED_MESSAGE;

  if (error.status === BAD_REQUEST || error.status === SERVICE_UNAVAILABLE) {
    return serverMessage(error) || LOGIN_UNAVAILABLE_MESSAGE;
  }

  return error.status >= SERVER_ERROR
    ? LOGIN_UNAVAILABLE_MESSAGE
    : ONBOARD_FAILED_MESSAGE;
}
