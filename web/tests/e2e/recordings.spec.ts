import { expect, test } from "@playwright/test";

test("opens a recording transcript and targets a cited segment", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "demo@example.com", display_name: "演示用户" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
  await page.route("**/api/v1/recordings/9", (route) => route.fulfill({ json: { recording: { session: { id: 9, organization_id: 7, room_id: 3, started_by: 1, status: "stopped", created_at: "2026-06-27T10:00:00Z", updated_at: "2026-06-27T10:30:00Z" }, files: [], transcription: { id: 4, status: "ready", provider: "openai_compatible", segment_count: 1, created_at: "2026-06-27T10:30:00Z", updated_at: "2026-06-27T10:31:00Z" } } } }));
  await page.route("**/api/v1/recordings/9/transcript?**", (route) => route.fulfill({ json: { transcription: { id: 4, status: "ready", provider: "openai_compatible", segment_count: 1, created_at: "2026-06-27T10:30:00Z", updated_at: "2026-06-27T10:31:00Z" }, segments: [{ id: 31, organization_id: 7, conversation_id: 12, room_id: 3, recording_session_id: 9, recording_file_id: 2, source: "recording", provider: "openai_compatible", language: "zh", text: "下周完成 Beta 回归并确认风险项。", start_ms: 62000, end_ms: 67000, confidence: 0.94, created_at: "2026-06-27T10:31:00Z" }] } }));
  await page.goto("/recordings/9?segmentId=31");
  await expect(page.getByRole("heading", { name: "会议转写 #9" })).toBeVisible();
  await expect(page.getByText("下周完成 Beta 回归并确认风险项。")).toBeVisible();
  await expect(page.locator("#segment-31")).toHaveClass(/target/);
});

test("renders the browser meeting preflight", async ({ page }) => {
  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "demo@example.com", display_name: "演示用户" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "演示组织", slug: "demo", role: "owner" }] } }));
  await page.goto("/meetings/3/preflight");
  await expect(page.getByRole("heading", { name: "加入会议前" })).toBeVisible();
  await expect(page.getByRole("button", { name: "进入会议" })).toBeVisible();
});
