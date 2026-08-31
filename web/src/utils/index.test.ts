import { afterEach, describe, expect, it, vi } from "vitest";
import { baseUrl, relativeDate, simplifyDate } from "./index";

describe("baseUrl", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const stubAppBaseUrl = (value: unknown) => {
    vi.stubGlobal("window", { APP: { baseUrl: value } });
  };

  it("falls back to / when the server placeholder was not substituted", () => {
    stubAppBaseUrl("{{.BaseUrl}}");
    expect(baseUrl()).toBe("/");
  });

  it("returns a real base url unchanged", () => {
    stubAppBaseUrl("/syncyomi/");
    expect(baseUrl()).toBe("/syncyomi/");
  });

  it("returns empty string when no base url is set", () => {
    stubAppBaseUrl("");
    expect(baseUrl()).toBe("");
  });
});

describe("simplifyDate", () => {
  it("returns n/a for an empty string", () => {
    expect(simplifyDate("")).toBe("n/a");
  });

  it("returns n/a for the Go zero date", () => {
    expect(simplifyDate("0001-01-01T00:00:00Z")).toBe("n/a");
  });

  it("returns n/a for null and undefined", () => {
    expect(simplifyDate(null)).toBe("n/a");
    expect(simplifyDate(undefined)).toBe("n/a");
  });

  it("formats a real date", () => {
    // formatISO9075 renders in local time, so assert the shape not an exact instant.
    expect(simplifyDate("2026-07-16T10:20:30Z")).toMatch(
      /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/
    );
  });
});

describe("relativeDate", () => {
  it("is empty for missing dates", () => {
    expect(relativeDate(null)).toBe("");
    expect(relativeDate("")).toBe("");
    expect(relativeDate("0001-01-01T00:00:00Z")).toBe("");
  });

  it("describes a past date relative to now", () => {
    const past = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    expect(relativeDate(past)).toBe("5 minutes ago");
  });
});
