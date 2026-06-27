import { expect, test } from "@playwright/test";

test("renders workflow trace, approvals, and transcript citations", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "demo@example.com", display_name: "演示用户" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
  await page.route("**/api/v1/conversations?**", (route) => route.fulfill({ json: { conversations: [{ id: 12, organization_id: 7, type: "group", title: "产品复盘", status: "open", priority: "high", unread_count: 0 }] } }));
  const workflow = { workflow: { id: 22, organization_id: 7, user_id: 1, conversation_id: 12, status: "requires_action", preset: "meeting_brief", goal: "会议复盘", summary: "Beta 评审已完成", action_items: ["完成回归"], next_step: "确认风险", risk_flags: ["时间窗口较短"], attempts: 1, created_at: "2026-06-27T10:00:00Z", updated_at: "2026-06-27T10:01:00Z" }, tasks: [{ id: 1, workflow_run_id: 22, organization_id: 7, name: "collect_context", role: "context_collector", status: "ready", attempts: 1, depends_on_json: "[]", created_at: "2026-06-27T10:00:00Z", updated_at: "2026-06-27T10:00:01Z" }, { id: 2, workflow_run_id: 22, organization_id: 7, name: "risk_analyst", role: "risk_analyst", status: "ready", attempts: 1, depends_on_json: "[\"collect_context\"]", created_at: "2026-06-27T10:00:01Z", updated_at: "2026-06-27T10:00:02Z" }], messages: [], approvals: [], history: [], signals: [], timers: [], citations: [{ source_type: "meeting_transcript", source_id: "31", source_title: "会议录音", title: "会议录音", snippet: "下周完成 Beta 回归", score: 0.91, recording_session_id: 9, transcript_segment_id: 31, start_ms: 62000 }] };
  await page.route("**/api/v1/agent/workflows?**", (route) => route.fulfill({ json: { workflows: [workflow] } }));
  await page.route("**/api/v1/agent/workflows/22", (route) => route.fulfill({ json: workflow }));
  await page.route("**/api/v1/agent/approvals", (route) => route.fulfill({ json: { approvals: [{ id: 5, workflow_run_id: 22, task_id: 2, organization_id: 7, tool_call_id: "tc-1", tool_name: "create_follow_up", status: "pending", input_json: "{\"title\":\"完成回归\"}", requested_by: 1, requested_at: "2026-06-27T10:01:00Z", created_at: "2026-06-27T10:01:00Z", updated_at: "2026-06-27T10:01:00Z" }] } }));
  await page.goto("/agent-lab?conversationId=12");
  await expect(page.getByText("Beta 评审已完成")).toBeVisible();
  await page.getByRole("tab", { name: /审批/ }).click();
  await expect(page.getByText("create_follow_up")).toBeVisible();
  await page.getByRole("tab", { name: "引用" }).click();
  await expect(page.getByText("下周完成 Beta 回归")).toBeVisible();
  await expect(page.getByRole("link", { name: /会议录音/ })).toHaveAttribute("href", /recordings\/9/);
});
