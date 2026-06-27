import { expect, test } from "@playwright/test";

test("renders the responsive workspace shell", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveTitle(/AllCallAll/);
  await expect(page.getByRole("heading", { name: /协作 Inbox/ })).toBeVisible();
  await expect(page.locator("body")).not.toHaveCSS("overflow-x", "scroll");
});
