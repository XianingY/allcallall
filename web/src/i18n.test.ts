import { describe, expect, it } from "vitest";

import en from "@/i18n/locales/en.json";
import zh from "@/i18n/locales/zh.json";

const placeholders = (value: string) =>
  (value.match(/\{\{[^}]+\}\}/g) ?? []).sort();

describe("i18n locale files", () => {
  it("zh and en expose exactly the same keys", () => {
    expect(Object.keys(zh).sort()).toEqual(Object.keys(en).sort());
  });

  it("has no empty translations", () => {
    Object.entries({ ...zh, ...en }).forEach(([key, value]) => {
      expect(String(value).trim(), key).not.toBe("");
    });
  });

  it("uses the same interpolation placeholders in both languages", () => {
    Object.keys(zh).forEach((key) => {
      const source = zh[key as keyof typeof zh];
      const target = en[key as keyof typeof en];
      expect(placeholders(target), key).toEqual(placeholders(source));
    });
  });

  it("resolves every placeholder the app actually passes", () => {
    // call.micFallback is rendered with { index }.
    expect(zh["call.micFallback"]).toContain("{{index}}");
    expect(en["call.micFallback"]).toContain("{{index}}");
  });
});
