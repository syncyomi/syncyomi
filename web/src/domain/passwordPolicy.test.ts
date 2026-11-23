import { describe, expect, it } from "vitest";
import { passwordBytes, passwordLength } from "./passwordPolicy";

describe("passwordLength", () => {
  it("counts a character outside the BMP once", () => {
    expect(passwordLength("😀")).toBe(1);
    expect(passwordLength("a😀b")).toBe(3);
  });

  it("is zero for an empty password", () => {
    expect(passwordLength("")).toBe(0);
  });
});

describe("passwordBytes", () => {
  it("counts UTF-8 bytes", () => {
    expect(passwordBytes("abc")).toBe(3);
    expect(passwordBytes("é")).toBe(2);
    expect(passwordBytes("😀")).toBe(4);
  });
});
