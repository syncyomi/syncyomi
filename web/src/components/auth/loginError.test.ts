import { describe, expect, it } from "vitest";
import { LOGIN_FAILED_MESSAGE, loginErrorMessage } from "./loginError";

describe("loginErrorMessage", () => {
  it("passes the server's rate limit message through, trimmed", () => {
    const error = new Error(
      "Too many failed attempts. Try again in 12 minutes.\n",
    );

    expect(loginErrorMessage(error)).toBe(
      "Too many failed attempts. Try again in 12 minutes.",
    );
  });

  it.each([
    new Error("Unauthorized"),
    new TypeError("Failed to fetch"),
    "boom",
    undefined,
  ])("falls back to the generic message for %j", (error) => {
    expect(loginErrorMessage(error)).toBe(LOGIN_FAILED_MESSAGE);
  });
});
