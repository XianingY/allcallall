import { expect, test } from "@playwright/test";

test("uses stable native rendering containment for long conversation lists", async ({ page }) => {
  const conversations = Array.from({ length: 120 }, (_, index) => ({
    id: index + 1,
    organization_id: 7,
    type: "group",
    title: `产品复盘 ${index + 1}`,
    topic: "Beta 反馈",
    status: "open",
    priority: "high",
    unread_count: 2,
    last_message_preview: "请确认行动项",
    last_message_at: "2026-06-27T10:00:00Z",
  }));

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
  await page.route("**/api/v1/realtime/tickets", (route) =>
    route.fulfill({ status: 503, json: { code: "REALTIME_UNAVAILABLE" } }),
  );
  await page.route("**/api/v1/conversations?**", (route) =>
    route.fulfill({
      json: {
        conversations,
        pagination: { total: 120, limit: 50, offset: 0, has_more: false },
      },
    }),
  );

  await page.goto("/inbox");
  const items = page.locator(".conversation-item");
  await expect(items).toHaveCount(120);

  const container = page.locator(".conversation-items");
  const initialGeometry = await container.evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
  }));
  expect(initialGeometry.scrollHeight).toBeGreaterThan(initialGeometry.clientHeight * 3);

  const contentVisibility = await items.first().evaluate(
    (element) => getComputedStyle(element).contentVisibility,
  );
  expect(contentVisibility).toBe("auto");

  const farItem = items.nth(119);
  await farItem.focus();
  await expect(farItem).toBeFocused();
  await expect(farItem).toBeVisible();

  const scrolledGeometry = await container.evaluate((element) => ({
    scrollTop: element.scrollTop,
    scrollHeight: element.scrollHeight,
  }));
  expect(scrolledGeometry.scrollTop).toBeGreaterThan(0);
  expect(scrolledGeometry.scrollHeight).toBe(initialGeometry.scrollHeight);

  for (const rootFontSize of [20, 24, 28, 32]) {
    await page.evaluate((size) => {
      document.documentElement.style.fontSize = `${size}px`;
    }, rootFontSize);
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(resolve)));

    const scaledGeometry = await container.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      paddingBottom: Number.parseFloat(getComputedStyle(element).paddingBottom),
    }));
    const scaledRows = await items.evaluateAll((rows) =>
      rows.slice(0, 10).map((row) => ({
        boxHeight: row.getBoundingClientRect().height,
        scrollHeight: row.scrollHeight,
        hasClippedDescendant: Array.from(row.querySelectorAll("*")).some((descendant) => {
          const rowBox = row.getBoundingClientRect();
          const descendantBox = descendant.getBoundingClientRect();
          return (
            descendantBox.height > 0 &&
            descendantBox.bottom > rowBox.bottom + 0.5
          );
        }),
      })),
    );
    expect(scaledRows).toHaveLength(10);
    for (const row of scaledRows) {
      expect(row.hasClippedDescendant).toBe(false);
      expect(row.scrollHeight).toBeLessThanOrEqual(row.boxHeight + 0.5);
    }

    const rowHeight = scaledRows[0].boxHeight;
    const expectedScrollHeight = conversations.length * rowHeight + scaledGeometry.paddingBottom;
    expect(scaledGeometry.scrollHeight).toBeGreaterThan(initialGeometry.scrollHeight);
    expect(Math.abs(scaledGeometry.scrollHeight - expectedScrollHeight)).toBeLessThanOrEqual(1);
  }
});
