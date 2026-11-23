import { describe, expect, it } from "vitest";
import { passwordStrength } from "./passwordStrength";

describe("passwordStrength", () => {
  it.each(["", "a", "Ab1!xyz"])("rates %j as too short", (password) => {
    expect(passwordStrength(password)).toEqual({
      score: 0,
      label: "Too short",
    });
  });

  it("rates a minimum length single-class password as weak", () => {
    expect(passwordStrength("abcdefgh")).toEqual({ score: 1, label: "Weak" });
  });

  it("rates one repeated character as weak however long it is", () => {
    expect(passwordStrength("a".repeat(40)).label).toBe("Weak");
  });

  it("rates a 12 character two-class password as fair", () => {
    expect(passwordStrength("abcdef123456")).toEqual({
      score: 2,
      label: "Fair",
    });
  });

  it("rates a long single-class passphrase as good", () => {
    expect(passwordStrength("correcthorsebattery")).toEqual({
      score: 3,
      label: "Good",
    });
  });

  it("rates a mixed-class password of 12 characters as good", () => {
    expect(passwordStrength("Tr0ub4dor&3x")).toEqual({
      score: 3,
      label: "Good",
    });
  });

  it("rates a long mixed-class password as strong", () => {
    expect(passwordStrength("Tr0ub4dor&3-horse-staple")).toEqual({
      score: 4,
      label: "Strong",
    });
  });

  it("never scores a longer password lower than a shorter one of the same classes", () => {
    const short = passwordStrength("abcdefghij").score;
    const long = passwordStrength("abcdefghijklmnopqrst").score;
    expect(long).toBeGreaterThanOrEqual(short);
  });
});
