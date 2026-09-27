import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

// This runtime does not expose localStorage (Node's experimental global shadows
// the jsdom one and resolves to undefined), but @/i18n reads it at module
// scope. Install a tiny in-memory stand-in before that import; vi.hoisted is
// hoisted above all imports by vitest.
vi.hoisted(() => {
  const store = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      clear: () => store.clear(),
      getItem: (key: string) => store.get(key) ?? null,
      key: (index: number) => [...store.keys()][index] ?? null,
      get length() {
        return store.size;
      },
      removeItem: (key: string) => {
        store.delete(key);
      },
      setItem: (key: string, value: string) => {
        store.set(key, String(value));
      },
    },
  });
});

import i18n from "@/i18n";
import { PageError, PageLoading } from "@/components/PageState";

beforeAll(async () => {
  await i18n.changeLanguage("en");
});

describe("PageState", () => {
  afterEach(cleanup);

  it("renders the English loading label by default", () => {
    render(<PageLoading />);
    expect(screen.getByText("Loading")).toBeInTheDocument();
  });

  it("renders an English retry button", () => {
    const retry = vi.fn();
    render(<PageError error={new Error("boom")} retry={retry} />);
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("falls back to the English load-failed message for non-Error values", () => {
    render(<PageError error="plain failure" />);
    expect(screen.getByText("Failed to load")).toBeInTheDocument();
  });

  it("keeps a caller-supplied loading label", () => {
    render(<PageLoading label="Custom label" />);
    expect(screen.getByText("Custom label")).toBeInTheDocument();
  });
});
