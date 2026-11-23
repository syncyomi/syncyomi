import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/ApiError";
import {
  LOGIN_FAILED_MESSAGE,
  LOGIN_UNAVAILABLE_MESSAGE,
  loginErrorMessage,
} from "./loginError";

describe("loginErrorMessage", () => {
  it.each([
    [429, "Slow down, try later.\n", "Slow down, try later."],
    [503, "Server busy, try again shortly\n", "Server busy, try again shortly"],
  ])(
    "passes the server's message through for %i, trimmed",
    (status, body, want) => {
      expect(loginErrorMessage(new ApiError(body, status))).toBe(want);
    },
  );

  it("does not blame the credentials for a server error", () => {
    expect(loginErrorMessage(new ApiError("Internal server error", 500))).toBe(
      LOGIN_UNAVAILABLE_MESSAGE,
    );
  });

  it.each([
    new ApiError("Forbidden", 403),
    new Error("Unauthorized"),
    new TypeError("Failed to fetch"),
    "boom",
    undefined,
  ])("falls back to the credentials message for %j", (error) => {
    expect(loginErrorMessage(error)).toBe(LOGIN_FAILED_MESSAGE);
  });
});
