import { describe, expect, it } from "vitest";
import { MAX_PASSKEY_NAME_LENGTH, passkeyNameLength } from "./passkeyPolicy";

describe("passkeyNameLength", () => {
  it("counts characters, not UTF-16 units", () => {
    expect(passkeyNameLength("😀".repeat(MAX_PASSKEY_NAME_LENGTH))).toBe(
      MAX_PASSKEY_NAME_LENGTH,
    );
  });

  it("ignores surrounding whitespace like the server does", () => {
    expect(passkeyNameLength("  Laptop \n")).toBe(6);
  });
});
