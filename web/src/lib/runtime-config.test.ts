import { describe, expect, it } from "vitest";
import { runtimeConfig } from "./runtime-config";

describe("runtimeConfig", () => {
  it("uses same-origin defaults", () => {
    expect(runtimeConfig.apiBaseUrl).toBe("/api/v1");
    expect(runtimeConfig.wsBaseUrl).toContain("/api/v1");
  });
});
