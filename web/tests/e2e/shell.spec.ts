import { expect, test } from "@playwright/test";

test("redirects anonymous users to login", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ success: false, error: "unauthorized" }) }));
  await page.goto("/");
  await expect(page).toHaveTitle(/AllCallAll/);
  await expect(page.getByRole("heading", { name: "登录工作台" })).toBeVisible();
  await expect(page.locator("body")).not.toHaveCSS("overflow-x", "scroll");
});

test("renders the authenticated responsive workspace shell", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "demo@example.com", display_name: "演示用户" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
  await page.goto("/inbox");
  await expect(page.getByRole("heading", { name: "Inbox" })).toBeVisible();
  if (await page.getByLabel("打开导航").isVisible()) await page.getByLabel("打开导航").click();
  await expect(page.getByLabel("当前组织")).toHaveValue("7");
});
