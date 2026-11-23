import type { ApiError } from "@/api/ApiError";

export const BAD_REQUEST = 400;
export const TOO_MANY_REQUESTS = 429;
export const SERVER_ERROR = 500;
export const SERVICE_UNAVAILABLE = 503;

export const serverMessage = (error: ApiError) => {
  const message = error.message.trim();
  return message && !message.includes("<") ? message : "";
};
