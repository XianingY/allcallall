import { describe, expect, it } from "vitest";

import { buildQuery } from "@/api/query";

describe("buildQuery", () => {
  it("returns an empty string when every value is omitted", () => {
    expect(buildQuery({ limit: undefined, offset: undefined })).toBe("");
  });

  it("keeps offset 0, which the first page of every paginated list relies on", () => {
    expect(buildQuery({ limit: 50, offset: 0 })).toBe("?limit=50&offset=0");
  });

  it("drops empty strings so optional filters do not send blank params", () => {
    expect(buildQuery({ search: "", limit: 10 })).toBe("?limit=10");
  });

  it("preserves declared order of the keys", () => {
    expect(buildQuery({ limit: 20, offset: 40 })).toBe("?limit=20&offset=40");
  });

  it("URL-encodes values that need it", () => {
    expect(buildQuery({ q: "a b&c" })).toBe("?q=a+b%26c");
  });

  it("renders a leading '?' only once when several params are present", () => {
    const query = buildQuery({ a: 1, b: 2, c: 3 });
    expect(query.startsWith("?")).toBe(true);
    expect(query.slice(1).split("&")).toHaveLength(3);
  });
});
