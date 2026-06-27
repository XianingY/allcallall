import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "demo@example.com", display_name: "演示用户" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
});

test("renders billing entitlement and usage settings", async ({ page }) => {
  await page.route("**/api/v1/entitlements/me", (route) => route.fulfill({ json: { tier: "premium", entitlements: [{ id: 3, entitlement: "premium", tier: "premium", status: "active", source: "revenuecat", product_id: "premium_monthly" }] } }));
  await page.route("**/api/v1/usage/me", (route) => route.fulfill({ json: { usage: [{ feature: "translation_minutes", period_key: "2026-06", unit: "minutes", used_units: 12, limit_units: 60, unlimited: false, remaining_units: 48 }] } }));
  await page.goto("/settings/billing");
  await expect(page.getByRole("heading", { name: "订阅与用量" })).toBeVisible();
  await expect(page.locator(".billing-summary").getByText("Premium", { exact: true })).toBeVisible();
  await expect(page.getByText("translation_minutes")).toBeVisible();
  await expect(page.getByText("未配置 RevenueCat public API key")).toBeVisible();
});

test("renders push notification device settings", async ({ page }) => {
  await page.route("**/api/v1/push/devices", (route) => route.fulfill({ json: { devices: [{ id: 4, provider: "fcm", platform: "web", device_name: "Chrome on macOS", app_version: "web-dev", last_registered: "2026-06-27T10:00:00Z", created_at: "2026-06-27T10:00:00Z", updated_at: "2026-06-27T10:00:00Z" }] } }));
  await page.goto("/settings/notifications");
  await expect(page.getByRole("heading", { name: "浏览器通知" })).toBeVisible();
  await expect(page.getByText("Chrome on macOS")).toBeVisible();
  await expect(page.getByText("Firebase Web Push 未配置")).toBeVisible();
});
