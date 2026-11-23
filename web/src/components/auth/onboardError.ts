import { ApiError } from "@/api/ApiError";
import {
  BAD_REQUEST,
  SERVER_ERROR,
  SERVICE_UNAVAILABLE,
  serverMessage,
} from "./apiErrorMessage";

export const ONBOARD_FAILED_MESSAGE =
  "Failed to create account! currently only one account is allowed, make sure browser cookies are cleared.";
export const ONBOARD_UNAVAILABLE_MESSAGE =
  "Could not create the account right now. Try again shortly.";

export function onboardErrorMessage(error: unknown): string {
  if (!(error instanceof ApiError)) return ONBOARD_FAILED_MESSAGE;

  if (error.status === BAD_REQUEST || error.status === SERVICE_UNAVAILABLE) {
    return serverMessage(error) || ONBOARD_UNAVAILABLE_MESSAGE;
  }

  return error.status >= SERVER_ERROR
    ? ONBOARD_UNAVAILABLE_MESSAGE
    : ONBOARD_FAILED_MESSAGE;
}
