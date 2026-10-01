import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AgentSkill, MCPInstallation, MCPTool } from "@/api/mcp";

const { mocks } = vi.hoisted(() => ({
  mocks: {
    listAgentSkills: vi.fn(),
    listMCPInstallations: vi.fn(),
    listMCPInstallationTools: vi.fn(),
    createAgentSkill: vi.fn(),
  },
}));

vi.mock("@/api/mcp", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/mcp")>()),
  listAgentSkills: mocks.listAgentSkills,
  listMCPInstallations: mocks.listMCPInstallations,
  listMCPInstallationTools: mocks.listMCPInstallationTools,
  createAgentSkill: mocks.createAgentSkill,
}));

import { MCPSkillsPanel } from "@/pages/mcp/MCPSkillsPanel";

const installation = (id: number, scope: "personal" | "organization"): MCPInstallation => ({
  id,
  organization_id: 2,
  owner_user_id: 3,
  scope,
  display_name: `Installation ${id}`,
  source_type: "oci",
  status: "active",
  active_revision_id: 8,
  secrets_configured: true,
  created_at: "2026-07-11T00:00:00Z",
  updated_at: "2026-07-11T00:00:00Z",
});

const tool = (id: number, installationId: number, name: string): MCPTool => ({
  id,
  installation_id: installationId,
  revision_id: 8,
  name: `${name}@v8`,
  original_name: name,
  input_schema: {},
  risk: "read",
  status: "ready",
  schema_version: "v1",
});

const skill = (overrides: Partial<AgentSkill> = {}): AgentSkill => ({
  id: 5,
  organization_id: 2,
  owner_user_id: 3,
  scope: "personal",
  name: "客户资料同步",
  description: "同步客户资料",
  instructions: "使用工具同步客户资料。",
  status: "active",
  version: 1,
  created_at: "2026-07-11T00:00:00Z",
  updated_at: "2026-07-11T00:00:00Z",
  ...overrides,
});

function renderPanel({ organizationRole = "owner" } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={client}>
      <MCPSkillsPanel organizationId={2} organizationRole={organizationRole} />
    </QueryClientProvider>,
  );
  return { ...view, client };
}

describe("MCPSkillsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.listAgentSkills.mockResolvedValue([]);
    mocks.listMCPInstallations.mockResolvedValue([installation(1, "personal"), installation(2, "organization")]);
    mocks.listMCPInstallationTools.mockImplementation(async (installationId: number) =>
      installationId === 1 ? [tool(11, 1, "search_issues")] : [tool(21, 2, "sync_contact")]);
  });
  afterEach(cleanup);

  it("requires name, instructions and at least one tool before creating a personal skill", async () => {
    renderPanel();

    const createButton = await screen.findByRole("button", { name: /创建 Skill/ });
    expect(createButton).toHaveProperty("disabled", true);

    act(() => {
      fireEvent.change(screen.getByPlaceholderText("例如：客户资料同步"), { target: { value: "客户资料同步" } });
      fireEvent.change(screen.getByPlaceholderText("Skill 的用途和边界"), { target: { value: "同步客户资料" } });
      fireEvent.change(screen.getByPlaceholderText("描述 Agent 应如何使用这些工具，以及必须遵守的约束。"), { target: { value: "同步客户资料。" } });
    });
    expect(createButton).toHaveProperty("disabled", true);

    const [firstTool] = await screen.findAllByRole("checkbox");
    act(() => fireEvent.click(firstTool));
    expect(createButton).toHaveProperty("disabled", false);

    act(() => fireEvent.click(createButton));
    await waitFor(() => expect(mocks.createAgentSkill).toHaveBeenCalledWith({
      name: "客户资料同步",
      description: "同步客户资料",
      instructions: "同步客户资料。",
      scope: "personal",
      tool_ids: [11],
    }));
  });

  it("only offers organization tools to organization skills and drops personal bindings on scope switch", async () => {
    renderPanel();

    const organizationScopeButton = await screen.findByRole("button", { name: "组织" });
    act(() => fireEvent.click(organizationScopeButton));

    expect(await screen.findByText("sync_contact")).toBeTruthy();
    expect(screen.queryByText("search_issues")).toBeNull();
    expect(screen.getByText("0 个工具已选择")).toBeTruthy();
  });

  it("does not let members create organization skills", async () => {
    renderPanel({ organizationRole: "member" });

    await screen.findByText("新建 Agent Skill");
    expect(screen.getByRole("button", { name: "组织" })).toHaveProperty("disabled", true);
  });

  it("keeps organization skills read-only for members and editable for owners", async () => {
    const organizationSkill = skill({ scope: "organization" });
    mocks.listAgentSkills.mockResolvedValue([organizationSkill]);
    renderPanel({ organizationRole: "member" });

    const memberSkillButton = await screen.findByRole("button", { name: /客户资料同步/ });
    act(() => fireEvent.click(memberSkillButton));
    await screen.findByText("组织 Skill 仅 owner 或 admin 可编辑");
    expect(screen.getByPlaceholderText("例如：客户资料同步")).toHaveProperty("disabled", true);
    expect(screen.queryByRole("button", { name: /保存版本|创建 Skill/ })).toBeNull();

    cleanup();
    mocks.listAgentSkills.mockResolvedValue([organizationSkill]);
    renderPanel({ organizationRole: "owner" });
    const ownerSkillButton = await screen.findByRole("button", { name: /客户资料同步/ });
    act(() => fireEvent.click(ownerSkillButton));
    expect(await screen.findByRole("button", { name: /保存版本/ })).toBeTruthy();
    expect(screen.getByPlaceholderText("例如：客户资料同步")).toHaveProperty("disabled", false);
  });
});
