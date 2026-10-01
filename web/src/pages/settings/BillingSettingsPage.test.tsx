import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getEntitlements: vi.fn(),
    getUsage: vi.fn(),
    isBillingConfigured: vi.fn(),
    openRevenueCatCheckout: vi.fn(),
    openRevenueCatPortal: vi.fn(),
  },
}));

vi.mock("@/api/platform", () => ({
  getEntitlements: mocks.getEntitlements,
  getUsage: mocks.getUsage,
}));

vi.mock("@/platform/billing", () => ({
  isBillingConfigured: mocks.isBillingConfigured,
  openRevenueCatCheckout: mocks.openRevenueCatCheckout,
  openRevenueCatPortal: mocks.openRevenueCatPortal,
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({
    user: { id: 1, email: "user@example.com", display_name: "测试用户" },
  }),
}));

import { BillingSettingsPage } from "@/pages/settings/BillingSettingsPage";

const renderPage = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <BillingSettingsPage />
    </QueryClientProvider>,
  );
  return client;
};

const entitlements = (tier: "free" | "premium") => ({
  tier,
  entitlements: tier === "premium"
    ? [{ id: 1, user_id: 1, entitlement: "pro", tier: "premium", status: "active", source: "revenuecat", expires_at: null, created_at: "", updated_at: "" }]
    : [],
});

describe("BillingSettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getUsage.mockResolvedValue([]);
    mocks.isBillingConfigured.mockReturnValue(true);
  });
  afterEach(cleanup);

  it("disables billing actions and explains the missing RevenueCat key", async () => {
    mocks.getEntitlements.mockResolvedValue(entitlements("free"));
    mocks.isBillingConfigured.mockReturnValue(false);
    renderPage();

    expect(await screen.findByText("Free")).toBeTruthy();
    expect(screen.getByRole("button", { name: /升级/ })).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: "管理订阅" })).toHaveProperty("disabled", true);
    expect(screen.getByText(/未配置 RevenueCat public API key/)).toBeTruthy();
  });

  it("shows entitlement and usage details for premium accounts", async () => {
    mocks.getEntitlements.mockResolvedValue(entitlements("premium"));
    mocks.getUsage.mockResolvedValue([{
      feature: "ai_minutes", period_key: "2026-10", used_units: 42, limit_units: 100,
      remaining_units: 58, unlimited: false, unit: "minutes", id: 1, organization_id: 2, created_at: "", updated_at: "",
    }]);
    renderPage();

    expect(await screen.findByText("Premium")).toBeTruthy();
    expect(screen.getByText("pro")).toBeTruthy();
    expect(screen.getByText(/ai_minutes/)).toBeTruthy();
    expect(screen.getByText(/42\/100 minutes/)).toBeTruthy();
    expect(screen.getByText(/剩余 58/)).toBeTruthy();
  });

  it("keeps polling entitlements after checkout until the backend syncs premium", async () => {
    mocks.getEntitlements.mockResolvedValue(entitlements("free"));
    mocks.openRevenueCatCheckout.mockResolvedValue(undefined);
    renderPage();

    await screen.findByText("Free");
    act(() => fireEvent.click(screen.getByRole("button", { name: /升级/ })));

    // Entitlements come from an async webhook; until it syncs, the page keeps
    // showing the syncing state instead of presenting the stale free tier as final.
    expect(await screen.findByRole("status")).toHaveTextContent("购买已提交，正在等待服务端同步权益");
  });
});
