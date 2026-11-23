import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/ApiError";
import { LOGIN_FAILED_MESSAGE, loginErrorMessage } from "./loginError";

describe("loginErrorMessage", () => {
  it("passes the server's rate limit message through, trimmed", () => {
    const error = new ApiError("Slow down, try later.\n", 429);

    expect(loginErrorMessage(error)).toBe("Slow down, try later.");
  });

  it.each([
    new ApiError("Server busy", 503),
    new ApiError("Too many failed attempts", 500),
    new Error("Unauthorized"),
    new TypeError("Failed to fetch"),
    "boom",
    undefined,
  ])("falls back to the generic message for %j", (error) => {
    expect(loginErrorMessage(error)).toBe(LOGIN_FAILED_MESSAGE);
  });
});
