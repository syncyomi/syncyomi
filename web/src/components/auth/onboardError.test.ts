import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/ApiError";
import {
  ONBOARD_FAILED_MESSAGE,
  ONBOARD_UNAVAILABLE_MESSAGE,
  onboardErrorMessage,
} from "./onboardError";

describe("onboardErrorMessage", () => {
  it.each([
    [
      400,
      "Password must be at least 8 characters\n",
      "Password must be at least 8 characters",
    ],
    [503, "Server busy, try again shortly", "Server busy, try again shortly"],
  ])("passes the server's message through for %i", (status, body, want) => {
    expect(onboardErrorMessage(new ApiError(body, status))).toBe(want);
  });

  it.each([
    [400, ""],
    [503, "<html><body>503 Service Unavailable</body></html>"],
  ])("ignores an empty or HTML body from a proxy for %i", (status, body) => {
    expect(onboardErrorMessage(new ApiError(body, status))).toBe(
      ONBOARD_UNAVAILABLE_MESSAGE,
    );
  });

  it("does not tell the user to clear cookies when the server fails", () => {
    expect(onboardErrorMessage(new ApiError("oops", 500))).toBe(
      ONBOARD_UNAVAILABLE_MESSAGE,
    );
  });

  it("keeps the one-account hint for a refused onboarding", () => {
    expect(onboardErrorMessage(new ApiError("Forbidden", 403))).toBe(
      ONBOARD_FAILED_MESSAGE,
    );
  });

  it.each([new TypeError("Failed to fetch"), "boom", undefined])(
    "falls back to the one-account hint for %j",
    (error) => {
      expect(onboardErrorMessage(error)).toBe(ONBOARD_FAILED_MESSAGE);
    },
  );
});
