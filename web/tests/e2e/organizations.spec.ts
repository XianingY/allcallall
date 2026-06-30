import { expect, test } from "@playwright/test";

test("renders organization admin dashboard and filters members", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "owner@example.com", display_name: "Owner" } } }));
  await page.route("**/api/v1/organizations/7/admin/summary", (route) => route.fulfill({ json: { summary: {
    counts: { member_count: 3, team_count: 2, pending_invite_count: 1, open_conversation_count: 4, pending_approval_count: 2 },
    recent_meetings: [{ room_id: 9, title: "Daily Sync", status: "ended", updated_at: "2026-06-30T01:30:00Z" }],
    recent_recordings: [{ recording_session_id: 11, room_id: 9, room_title: "Daily Sync", recording_status: "stopped", transcription_status: "ready", transcription_provider: "mock", transcription_segment_count: 8, updated_at: "2026-06-30T01:31:00Z" }],
    recent_audit_events: [{ id: 3, organization_id: 7, actor_user_id: 1, actor_email: "owner@example.com", actor_display_name: "Owner", action: "organization.team.created", target_type: "team", target_id: "2", created_at: "2026-06-30T00:30:00Z" }],
  } } }));
  await page.route("**/api/v1/organizations/7/members", (route) => route.fulfill({ json: { members: [
    { id: 1, organization_id: 7, user_id: 1, email: "owner@example.com", display_name: "Owner", status: "active", role: "owner", joined_at: "2026-06-30T00:00:00Z", created_at: "2026-06-30T00:00:00Z", updated_at: "2026-06-30T00:00:00Z" },
    { id: 2, organization_id: 7, user_id: 2, email: "admin@example.com", display_name: "Admin", status: "active", role: "admin", joined_at: "2026-06-30T00:00:00Z", created_at: "2026-06-30T00:00:00Z", updated_at: "2026-06-30T00:00:00Z" },
    { id: 3, organization_id: 7, user_id: 3, email: "member@example.com", display_name: "Member", status: "active", role: "member", joined_at: "2026-06-30T00:00:00Z", created_at: "2026-06-30T00:00:00Z", updated_at: "2026-06-30T00:00:00Z" },
  ] } }));
  await page.route("**/api/v1/organizations/7/invites", (route) => route.fulfill({ json: { invites: [] } }));
  await page.route("**/api/v1/organizations/7/teams", (route) => route.fulfill({ json: { teams: [{ id: 1, organization_id: 7, name: "General", slug: "general", created_by: 1, member_count: 1, members: [], created_at: "2026-06-30T00:00:00Z", updated_at: "2026-06-30T00:00:00Z" }] } }));
  await page.route("**/api/v1/organizations/7/policy", (route) => route.fulfill({ json: { policy: { id: 1, organization_id: 7, recording_mode: "off", recording_storage_days: 30, recording_export_allowed: false } } }));
  await page.route("**/api/v1/organizations/7/audit-events", (route) => route.fulfill({ json: { events: [] } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));

  await page.goto("/organizations");
  await expect(page.getByText("待审批工具")).toBeVisible();
  await expect(page.getByText("Daily Sync").first()).toBeVisible();

  await page.getByRole("button", { name: "成员" }).click();
  await expect(page.locator(".org-admin-main .data-row").filter({ hasText: "owner@example.com" })).toBeVisible();
  await page.getByLabel("搜索").fill("admin");
  await expect(page.locator(".org-admin-main .data-row").filter({ hasText: "admin@example.com" })).toBeVisible();
  await expect(page.locator(".org-admin-main .data-row").filter({ hasText: "member@example.com" })).toHaveCount(0);
});
