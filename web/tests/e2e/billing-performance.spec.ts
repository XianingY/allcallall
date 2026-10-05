import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) =>
    route.fulfill({
      json: {
        access_token: "test-token",
        user: { id: 1, email: "demo@example.com", display_name: "演示用户" },
      },
    }),
  );
  await page.route("**/api/v1/organizations", (route) =>
    route.fulfill({
      json: {
        organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }],
      },
    }),
  );
  await page.route("**/api/v1/entitlements/me", (route) =>
    route.fulfill({
      json: { tier: "free", entitlements: [] },
    }),
  );
  await page.route("**/api/v1/usage/me", (route) => route.fulfill({ json: { usage: [] } }));
});

const collectRevenueCatRequests = (page: Parameters<Parameters<typeof test>[0]>[0]) => {
  const requests: string[] = [];
  page.on("request", (request) => {
    const { pathname } = new URL(request.url());
    if (/\/vendor-revenuecat-[^/]+\.js$/.test(pathname)) {
      requests.push(pathname);
    }
  });
  return requests;
};

test("does not download RevenueCat until the billing action receives pointer intent", async ({ page }) => {
  await page.addInitScript("window.__ALLCALLALL_CONFIG__ = { revenueCatPublicApiKey: 'e2e-key' };");
  const requests = collectRevenueCatRequests(page);

  await page.goto("/settings/billing");
  await expect(page.getByRole("heading", { name: "订阅与用量" })).toBeVisible();
  expect(requests).toHaveLength(0);

  await page.getByRole("button", { name: /升级/ }).hover();
  await expect.poll(() => requests.length).toBe(1);
});

test("preloads RevenueCat when the billing action receives keyboard focus", async ({ page }) => {
  await page.addInitScript("window.__ALLCALLALL_CONFIG__ = { revenueCatPublicApiKey: 'e2e-key' };");
  const requests = collectRevenueCatRequests(page);

  await page.goto("/settings/billing");
  await expect(page.getByRole("heading", { name: "订阅与用量" })).toBeVisible();
  expect(requests).toHaveLength(0);

  await page.getByRole("button", { name: /升级/ }).focus();
  await expect.poll(() => requests.length).toBe(1);
});

test("preloads RevenueCat when the manage action receives intent", async ({ page }) => {
  await page.addInitScript("window.__ALLCALLALL_CONFIG__ = { revenueCatPublicApiKey: 'e2e-key' };");
  const requests = collectRevenueCatRequests(page);

  await page.goto("/settings/billing");
  await expect(page.getByRole("heading", { name: "订阅与用量" })).toBeVisible();
  expect(requests).toHaveLength(0);

  await page.getByRole("button", { name: /管理订阅/ }).hover();
  await expect.poll(() => requests.length).toBe(1);
});

test("does not download RevenueCat when billing is not configured", async ({ page }) => {
  await page.addInitScript("window.__ALLCALLALL_CONFIG__ = {};");
  const requests = collectRevenueCatRequests(page);

  await page.goto("/settings/billing");
  await expect(page.getByRole("heading", { name: "订阅与用量" })).toBeVisible();
  await page.getByRole("button", { name: /升级/ }).hover();
  await page.getByRole("button", { name: /管理订阅/ }).focus();
  await page.waitForTimeout(250);

  expect(requests).toHaveLength(0);
});

test("deduplicates repeated RevenueCat preload intent", async ({ page }) => {
  await page.addInitScript("window.__ALLCALLALL_CONFIG__ = { revenueCatPublicApiKey: 'e2e-key' };");
  const requests = collectRevenueCatRequests(page);

  await page.goto("/settings/billing");
  await expect(page.getByRole("heading", { name: "订阅与用量" })).toBeVisible();

  await page.getByRole("button", { name: /升级/ }).hover();
  await expect.poll(() => requests.length).toBe(1);
  await page.getByRole("button", { name: /管理订阅/ }).focus();
  await page.waitForTimeout(250);

  expect(requests).toHaveLength(1);
});
