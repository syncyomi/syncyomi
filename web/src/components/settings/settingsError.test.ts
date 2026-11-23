import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/ApiError";
import { PasskeyPromptError } from "@/utils/passkeyErrors";
import { NETWORK_MESSAGE, settingsErrorMessage } from "./settingsError";

const FALLBACK = "Could not do it.";

describe("settingsErrorMessage", () => {
  it("shows the server's reason", () => {
    const error = new ApiError("Current password is incorrect\n", 403);

    expect(settingsErrorMessage(error, FALLBACK)).toBe(
      "Current password is incorrect",
    );
  });

  it("does not print an HTML error page from a proxy", () => {
    const error = new ApiError(
      "<html><body>502 Bad Gateway</body></html>",
      502,
    );

    expect(settingsErrorMessage(error, FALLBACK)).toBe(FALLBACK);
  });

  it("does not print an empty body", () => {
    expect(settingsErrorMessage(new ApiError("", 404), FALLBACK)).toBe(
      FALLBACK,
    );
  });

  it("explains a failed request", () => {
    expect(
      settingsErrorMessage(new TypeError("Failed to fetch"), FALLBACK),
    ).toBe(NETWORK_MESSAGE);
  });

  it("passes the passkey prompt's own message through", () => {
    const error = new PasskeyPromptError("The passkey prompt was cancelled.");

    expect(settingsErrorMessage(error, FALLBACK)).toBe(
      "The passkey prompt was cancelled.",
    );
  });

  it("falls back for anything else", () => {
    expect(settingsErrorMessage("boom", FALLBACK)).toBe(FALLBACK);
  });
});
