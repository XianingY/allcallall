import { expect, test } from "@playwright/test";

test("installs, validates, publishes, and disables an MCP server", async ({ page }) => {
  const revision = { id: 201, revision: 4, transport: "stdio", image_ref: "ghcr.io/acme/github-mcp@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", image_digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", scan_status: "passed", created_by: 1, created_at: "2026-07-11T08:00:00Z" };
  const existing = { id: 101, organization_id: 7, owner_user_id: 1, scope: "personal", display_name: "GitHub Operations", source_type: "oci", status: "active", active_revision_id: 201, secrets_configured: true, latest_revision: revision, created_at: "2026-07-10T08:00:00Z", updated_at: "2026-07-11T08:00:00Z" };
  let installations = [existing];
  let createdBody: Record<string, unknown> | undefined;
  const actions: string[] = [];

  await page.route("**/api/v1/auth/refresh", (route) => route.fulfill({ json: { access_token: "test-token", user: { id: 1, email: "owner@example.com", display_name: "Owner" } } }));
  await page.route("**/api/v1/organizations", (route) => route.fulfill({ json: { organizations: [{ id: 7, name: "Platform Team", slug: "platform", role: "owner" }] } }));
  await page.route("**/api/v1/realtime/tickets", (route) => route.fulfill({ status: 503, json: { code: "REALTIME_UNAVAILABLE" } }));
  await page.route("**/api/v1/conversations?**", (route) => route.fulfill({ json: { conversations: [{ id: 12, organization_id: 7, type: "group", title: "支持升级", status: "open", priority: "high", unread_count: 0 }] } }));
  await page.route("**/api/v1/agent/mcp/installations", async (route) => {
    if (route.request().method() === "GET") return route.fulfill({ json: { installations } });
    createdBody = route.request().postDataJSON() as Record<string, unknown>;
    const created = { ...existing, id: 102, display_name: createdBody.display_name as string, status: "draft", active_revision_id: null, latest_revision: { ...revision, id: 202, revision: 1, image_ref: createdBody.image_ref as string, image_digest: undefined, scan_status: "pending" } };
    installations = [created, ...installations];
    return route.fulfill({ status: 201, json: { installation: created } });
  });
  await page.route("**/api/v1/agent/mcp/installations/**", (route) => {
    const path = route.request().url().split("?")[0];
    if (path.endsWith("/tools")) return route.fulfill({ json: { tools: [{ id: 301, installation_id: 101, revision_id: 201, name: "mcp.101.create_issue", original_name: "create_issue", description: "Create an issue", input_schema: {}, output_schema: {}, risk: "write", status: "active", schema_version: "1" }] } });
    if (path.endsWith("/validate")) { actions.push("validate"); return route.fulfill({ json: { installation: { ...existing, status: "disabled" } } }); }
    if (path.endsWith("/publish")) { actions.push("publish"); return route.fulfill({ json: { installation: { ...existing, scope: "organization" } } }); }
    const id = Number(path.split("/").pop());
    const installation = installations.find((item) => item.id === id) ?? existing;
    if (route.request().method() === "DELETE") { actions.push("disable"); return route.fulfill({ status: 204 }); }
    return route.fulfill({ json: { installation } });
  });

  await page.goto("/agent-tools");
  await expect(page.getByRole("link", { name: "审批与 Trace" })).toHaveAttribute("href", "/agent-lab");
  await expect(page.getByText("mcp.101.create_issue")).toBeVisible();
  await expect(page.getByText("写入操作需要审批")).toBeVisible();
  await expect(page.getByRole("table").getByRole("link", { name: "Agent Lab" })).toHaveAttribute("href", /conversationId=12.*mode=react.*mcp.101.create_issue/);
  await page.getByRole("button", { name: "连接验证" }).click();
  await expect.poll(() => actions).toContain("validate");
  await page.getByRole("button", { name: "发布到组织" }).click();
  await expect.poll(() => actions).toContain("publish");
  const disableButton = page.getByRole("button", { name: "禁用" });
  await expect(disableButton).toBeVisible();
  await disableButton.click({ force: true });

  await page.getByRole("button", { name: "安装 MCP" }).click();
  await page.getByRole("textbox", { name: "显示名称" }).fill("Internal CRM");
  await page.getByRole("button", { name: "OCI" }).click();
  await page.getByRole("button", { name: "下一步" }).click();
  await page.getByRole("textbox", { name: "OCI 镜像" }).fill(`registry.example.com/mcp/server@sha256:${"b".repeat(64)}`);
  await page.getByRole("button", { name: "创建安装" }).click();

  await expect(page.getByRole("heading", { name: "Internal CRM" })).toBeVisible();
  expect(createdBody).toMatchObject({ display_name: "Internal CRM", source_type: "oci", transport: "stdio" });
  expect(String(createdBody?.image_ref)).toMatch(/@sha256:b{64}$/);
  expect(actions).toEqual(["validate", "publish", "disable"]);
});
