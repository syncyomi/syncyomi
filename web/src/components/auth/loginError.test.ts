import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/ApiError";
import { PasskeyPromptError } from "@/utils/passkeyErrors";
import {
  PASSKEY_FAILED_MESSAGE,
  LOGIN_FAILED_MESSAGE,
  LOGIN_UNAVAILABLE_MESSAGE,
  loginErrorMessage,
  passkeyErrorMessage,
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

  it.each([
    [429, ""],
    [503, "<html><body>503 Service Unavailable</body></html>"],
  ])("ignores an empty or HTML body from a proxy for %i", (status, body) => {
    expect(loginErrorMessage(new ApiError(body, status))).toBe(
      LOGIN_UNAVAILABLE_MESSAGE,
    );
  });

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

describe("passkeyErrorMessage", () => {
  it("shows what went wrong in the browser prompt", () => {
    expect(
      passkeyErrorMessage(
        new PasskeyPromptError("The passkey prompt was cancelled."),
      ),
    ).toBe("The passkey prompt was cancelled.");
  });

  it.each([
    [
      400,
      "The passkey request expired, try again\n",
      "The passkey request expired, try again",
    ],
    [
      429,
      "Too many failed attempts. Try again in 12 minutes.",
      "Too many failed attempts. Try again in 12 minutes.",
    ],
    [503, "Server busy, try again shortly", "Server busy, try again shortly"],
  ])("passes the server's message through for %i", (status, body, want) => {
    expect(passkeyErrorMessage(new ApiError(body, status))).toBe(want);
  });

  it("does not suggest a wrong password for a server error", () => {
    expect(passkeyErrorMessage(new ApiError("oops", 500))).toBe(
      LOGIN_UNAVAILABLE_MESSAGE,
    );
  });

  it.each([
    new Error("Unauthorized"),
    new TypeError("Failed to fetch"),
    new ApiError("Forbidden", 403),
    undefined,
  ])("falls back to a generic passkey failure for %j", (error) => {
    expect(passkeyErrorMessage(error)).toBe(PASSKEY_FAILED_MESSAGE);
  });
});
