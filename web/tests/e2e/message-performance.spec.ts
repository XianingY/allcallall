import { expect, test } from "@playwright/test";

test("uses stable native rendering containment for long message streams", async ({ page }) => {
  const messages = Array.from({ length: 160 }, (_, index) => ({
    id: index + 1,
    organization_id: 7,
    conversation_id: 12,
    sender_id: index % 2 === 0 ? 2 : 1,
    sender_email: index % 2 === 0 ? "pm@example.com" : "demo@example.com",
    sender_display_name: index % 2 === 0 ? "产品经理" : "演示用户",
    type: "text",
    body: `长消息 ${index + 1} ${"这是一段用于验证消息流渲染隔离的长文本。".repeat(3 + (index % 4))}`,
    created_at: `2026-06-27T10:${String(index % 60).padStart(2, "0")}:00Z`,
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
        conversations: [
          {
            id: 12,
            organization_id: 7,
            type: "group",
            title: "产品复盘",
            topic: "Beta 反馈",
            status: "open",
            priority: "high",
            unread_count: 2,
            last_message_preview: "请确认行动项",
            last_message_at: "2026-06-27T10:00:00Z",
          },
        ],
        pagination: { total: 1, limit: 50, offset: 0, has_more: false },
      },
    }),
  );
  await page.route("**/api/v1/conversations/12", (route) =>
    route.fulfill({
      json: {
        conversation: {
          conversation: {
            id: 12,
            organization_id: 7,
            type: "group",
            title: "产品复盘",
            topic: "Beta 反馈",
            status: "open",
            priority: "high",
            unread_count: 2,
          },
          workspace: {
            status: "open",
            priority: "high",
            agent_context: {
              transcript_segment_count: 0,
              meeting_transcript_segment_count: 8,
              knowledge_source_count: 2,
              pending_approval_count: 1,
              meeting_transcription_status: "ready",
            },
          },
        },
      },
    }),
  );
  await page.route("**/api/v1/conversations/12/messages**", (route) =>
    route.fulfill({
      json: {
        messages,
        has_more_prev: false,
      },
    }),
  );
  await page.route("**/api/v1/conversations/12/pins", (route) =>
    route.fulfill({
      json: {
        messages: [
          {
            ...messages[39],
            pinned: true,
            body: "这是需要跳转的置顶消息",
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/conversations/12/notes", (route) =>
    route.fulfill({ json: { notes: [] } }),
  );
  await page.route("**/api/v1/conversations/12/read", (route) =>
    route.fulfill({ json: { success: true } }),
  );

  await page.goto("/conversations/12");
  await expect(page.getByRole("heading", { name: "产品复盘" })).toBeVisible();

  const items = page.locator(".message-bubble");
  await expect(items).toHaveCount(160);

  const container = page.locator(".message-stream");
  const initialGeometry = await container.evaluate((element) => ({
    scrollHeight: element.scrollHeight,
    clientHeight: element.clientHeight,
  }));
  expect(initialGeometry.scrollHeight).toBeGreaterThan(initialGeometry.clientHeight * 3);

  const firstMessage = items.first();
  const contentVisibility = await firstMessage.evaluate(
    (element) => getComputedStyle(element).contentVisibility,
  );
  expect(contentVisibility).toBe("auto");
  const intrinsicSize = await firstMessage.evaluate(
    (element) => getComputedStyle(element).containIntrinsicSize,
  );
  expect(intrinsicSize).toBe("auto 168px");

  const farMessage = items.nth(159);
  await farMessage.getByTitle("回复").focus();
  await expect(farMessage).toBeVisible();
  await expect(farMessage.getByTitle("回复")).toBeFocused();

  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
  const scrolledGeometry = await container.evaluate((element) => ({
    scrollTop: element.scrollTop,
    scrollHeight: element.scrollHeight,
  }));
  expect(scrolledGeometry.scrollTop).toBeGreaterThan(0);

  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
  const stableGeometry = await container.evaluate((element) => ({
    scrollTop: element.scrollTop,
    scrollHeight: element.scrollHeight,
  }));
  expect(Math.abs(stableGeometry.scrollHeight - scrolledGeometry.scrollHeight)).toBeLessThanOrEqual(1);

  await page.locator(".pinned-strip button").click();
  const pinnedMessage = page.locator("#message-40");
  await expect(pinnedMessage).toBeVisible();
  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
  const pinnedGeometry = await container.evaluate(
    (element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      viewportCenter:
        element.getBoundingClientRect().top + element.getBoundingClientRect().height / 2,
    }),
  );
  const pinnedBox = await pinnedMessage.boundingBox();
  expect(pinnedBox).toBeTruthy();
  if (pinnedBox) {
    const pinnedCenter = pinnedBox.y + pinnedBox.height / 2;
    expect(Math.abs(pinnedCenter - pinnedGeometry.viewportCenter)).toBeLessThanOrEqual(
      pinnedGeometry.clientHeight / 4,
    );
  }

  const hiddenMessageBody = page.locator("#message-1 p");
  await hiddenMessageBody.scrollIntoViewIfNeeded();
  await expect(hiddenMessageBody).toBeVisible();

  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
  const discoverabilityGeometry = await container.evaluate(
    (element) => ({ scrollHeight: element.scrollHeight }),
  );
  expect(
    Math.abs(discoverabilityGeometry.scrollHeight - pinnedGeometry.scrollHeight),
  ).toBeLessThanOrEqual(1);

  for (const rootFontSize of [20, 24, 28, 32]) {
    await page.evaluate((size) => {
      document.documentElement.style.fontSize = `${size}px`;
    }, rootFontSize);
    await page.evaluate(
      () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
    );

    const scaledRows = await items.evaluateAll((rows) => {
      const container = document.querySelector(".message-stream");
      if (!container) return [];
      const viewport = container.getBoundingClientRect();
      return rows
        .filter((row) => {
          const box = row.getBoundingClientRect();
          return box.bottom > viewport.top && box.top < viewport.bottom;
        })
        .map((row) => {
          const rowBox = row.getBoundingClientRect();
          return {
            boxHeight: rowBox.height,
            scrollHeight: row.scrollHeight,
            hasClippedDescendant: Array.from(row.querySelectorAll("*")).some((descendant) => {
              const descendantBox = descendant.getBoundingClientRect();
              return (
                descendantBox.height > 0 &&
                descendantBox.bottom > rowBox.bottom + 0.5
              );
            }),
          };
        });
    });
    expect(scaledRows.length).toBeGreaterThan(0);
    for (const row of scaledRows) {
      expect(row.hasClippedDescendant).toBe(false);
      expect(row.scrollHeight).toBeLessThanOrEqual(row.boxHeight + 0.5);
    }
  }
});
