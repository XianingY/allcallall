import { expect, test } from "@playwright/test";

test("opens a conversation and renders messages with business context", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "demo@example.com", display_name: "演示用户" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
  await page.route("**/api/v1/conversations", (route) => route.fulfill({ json: { conversations: [{ id: 12, organization_id: 7, type: "group", title: "产品复盘", topic: "Beta 反馈", status: "open", priority: "high", unread_count: 2, last_message_preview: "请确认行动项", last_message_at: "2026-06-27T10:00:00Z" }] } }));
  await page.route("**/api/v1/conversations/12", (route) => route.fulfill({ json: { conversation: { conversation: { id: 12, organization_id: 7, type: "group", title: "产品复盘", topic: "Beta 反馈", status: "open", priority: "high", unread_count: 2 }, workspace: { status: "open", priority: "high", agent_context: { transcript_segment_count: 0, meeting_transcript_segment_count: 8, knowledge_source_count: 2, pending_approval_count: 1, meeting_transcription_status: "ready" } } } } }));
  await page.route("**/api/v1/conversations/12/messages**", (route) => route.fulfill({ json: { messages: [{ id: 1, organization_id: 7, conversation_id: 12, sender_id: 2, sender_email: "pm@example.com", sender_display_name: "产品经理", type: "text", body: "请确认行动项", created_at: "2026-06-27T10:00:00Z" }], has_more_prev: false } }));
  await page.route("**/api/v1/conversations/12/pins", (route) => route.fulfill({ json: { messages: [] } }));
  await page.route("**/api/v1/conversations/12/notes", (route) => route.fulfill({ json: { notes: [] } }));
  await page.route("**/api/v1/conversations/12/read", (route) => route.fulfill({ json: { success: true } }));
  await page.goto("/inbox");
  const link = page.getByRole("link", { name: /产品复盘/ });
  await expect(link).toBeVisible();
  await link.click();
  await expect(page.getByRole("heading", { name: "产品复盘" })).toBeVisible();
  await expect(page.locator(".message-pane .message-bubble").filter({ hasText: "请确认行动项" })).toBeVisible();
});
