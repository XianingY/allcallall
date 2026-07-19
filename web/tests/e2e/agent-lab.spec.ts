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
  const approvalsTab = page.getByRole("tab", { name: /审批/ });
  await expect(approvalsTab).toBeVisible();
  await approvalsTab.click({ force: true });
  await expect(page.getByText("create_follow_up")).toBeVisible();
  await page.getByRole("tab", { name: "引用" }).click({ force: true });
  await expect(page.getByText("下周完成 Beta 回归")).toBeVisible();
  await expect(page.getByRole("link", { name: /会议录音/ })).toHaveAttribute("href", /recordings\/9/);
});

test("approves a checkpoint-owned MCP tool call from React Agent trace", async ({ page }) => {
  const toolCall = { id: 9, run_id: 44, call_id: "tool-ticket", tool_name: "mcp.101.create_support_ticket", status: "pending", tool_schema_version: "mcp-4", approval_request_id: "approval-44", approval_checkpoint_version: 15, mcp_installation_id: 101, mcp_revision_id: 201, mcp_tool_id: 301, input_json: '{"subject":"升级工单"}', created_at: "2026-07-19T10:00:00Z", updated_at: "2026-07-19T10:00:00Z" };
  let submitted: Record<string, unknown> | undefined;
  const result = (status: string, callStatus: string) => ({ run: { id: 44, organization_id: 7, user_id: 1, conversation_id: 12, source: "python_langgraph", runtime_owner: "python_langgraph", status, prompt_version: "react_general_v1", tool_schema_version: "tool_schema_v1", checkpoint_id: "checkpoint-44", checkpoint_version: status === "ready" ? 17 : 15, approval_request_id: status === "requires_action" ? "approval-44" : undefined, goal: "请使用 mcp.101.create_support_ticket", summary: "已准备支持工单", action_items: [], next_step: "等待审批", risk_flags: [], attempts: 1, created_at: "2026-07-19T10:00:00Z", updated_at: "2026-07-19T10:00:01Z" }, steps: [], tool_calls: [{ ...toolCall, status: callStatus }], trace: [], citations: [] });

  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "owner@example.com", display_name: "Owner" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
  await page.route("**/api/v1/conversations?**", (route) => route.fulfill({ json: { conversations: [{ id: 12, organization_id: 7, type: "group", title: "支持升级", status: "open", priority: "high", unread_count: 0 }] } }));
  await page.route("**/api/v1/agent/workflows?**", (route) => route.fulfill({ json: { workflows: [] } }));
  await page.route("**/api/v1/agent/approvals", (route) => route.fulfill({ json: { approvals: [] } }));
  await page.route("**/api/v1/agent/runs/44/events/stream**", (route) => route.fulfill({ contentType: "text/event-stream", body: "event: requires_action\ndata: {\"status\":\"requires_action\"}\n\n" }));
  await page.route("**/api/v1/agent/runs/44/submit-tool-outputs", async (route) => { submitted = route.request().postDataJSON() as Record<string, unknown>; return route.fulfill({ json: result("ready", "success") }); });
  await page.route("**/api/v1/agent/runs/44", (route) => route.fulfill({ json: result("requires_action", "pending") }));
  await page.route("**/api/v1/agent/runs", (route) => route.fulfill({ status: 202, json: result("requires_action", "pending") }));

  await page.goto("/agent-lab?conversationId=12&mode=react&goal=%E8%AF%B7%E4%BD%BF%E7%94%A8%20mcp.101.create_support_ticket");
  await expect(page.getByRole("textbox", { name: "目标" })).toHaveValue("请使用 mcp.101.create_support_ticket");
  await page.getByRole("button", { name: "启动 ReAct" }).click();
  await expect(page.getByText(/checkpoint-44 · v15/)).toBeVisible();
  await expect(page.getByText(/MCP #101 · revision #201 · checkpoint v15/)).toBeVisible();
  await page.getByRole("button", { name: "批准" }).click();

  expect(submitted).toEqual({ outputs: [{ tool_call_id: "tool-ticket", action: "approve" }] });
});
