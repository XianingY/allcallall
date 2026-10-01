import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { MCPInstallation, MCPTool } from "@/api/mcp";

const { mocks } = vi.hoisted(() => ({
  mocks: {
    listMCPInstallations: vi.fn(),
    getMCPInstallation: vi.fn(),
    listMCPInstallationTools: vi.fn(),
    listConversations: vi.fn(),
  },
}));

vi.mock("@/api/mcp", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/mcp")>()),
  listMCPInstallations: mocks.listMCPInstallations,
  getMCPInstallation: mocks.getMCPInstallation,
  listMCPInstallationTools: mocks.listMCPInstallationTools,
}));

vi.mock("@/api/collaboration", () => ({
  listConversations: mocks.listConversations,
}));

vi.mock("@/pages/mcp/MCPDialogs", () => ({
  MCPSecretsDialog: () => null,
  RenameInstallationDialog: () => null,
  InstallationWizard: () => null,
}));

import { MCPInstallationsPanel } from "@/pages/mcp/MCPInstallationsPanel";

const baseInstallation = (overrides: Partial<MCPInstallation>): MCPInstallation => ({
  id: 1,
  organization_id: 2,
  owner_user_id: 3,
  scope: "personal",
  display_name: "GitHub",
  source_type: "oci",
  status: "active",
  active_revision_id: 8,
  secrets_configured: true,
  created_at: "2026-07-11T00:00:00Z",
  updated_at: "2026-07-11T00:00:00Z",
  latest_revision: {
    id: 8,
    revision: 4,
    transport: "stdio",
    image_digest: "sha256:1234567890abcdef1234567890abcdef",
    scan_status: "passed",
    created_by: 3,
    created_at: "2026-07-11T00:00:00Z",
  },
  ...overrides,
});

const installations = [
  baseInstallation({ id: 1, display_name: "GitHub" }),
  baseInstallation({ id: 2, display_name: "Notion", scope: "organization", status: "disabled" }),
];

const tool: MCPTool = {
  id: 11,
  installation_id: 1,
  revision_id: 8,
  name: "search_issues",
  original_name: "search_issues",
  input_schema: {},
  risk: "read",
  status: "ready",
  schema_version: "v1",
};

function renderPanel(props: { organizationRole?: string; selectedId?: number } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  function Harness() {
    const [selectedId, setSelectedId] = useState(props.selectedId ?? 0);
    return (
      <MCPInstallationsPanel
        organizationId={2}
        organizationRole={props.organizationRole ?? "owner"}
        selectedId={selectedId}
        onSelectedId={setSelectedId}
      />
    );
  }
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <Harness />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return view;
}

describe("MCPInstallationsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.listMCPInstallations.mockResolvedValue(installations);
    mocks.listConversations.mockResolvedValue({ conversations: [], total: 0 });
    mocks.getMCPInstallation.mockImplementation(async (id: number) =>
      installations.find((item) => item.id === id) ?? installations[0]);
    mocks.listMCPInstallationTools.mockResolvedValue([tool]);
  });
  afterEach(cleanup);

  it("auto-selects the first installation when none is selected", async () => {
    renderPanel();

    expect(await screen.findByText("GitHub")).toBeTruthy();
    expect(await screen.findByText("工具目录")).toBeTruthy();
    expect(screen.getByText("Notion")).toBeTruthy();
  });

  it("falls back to the first visible installation when the selection is stale", async () => {
    renderPanel({ selectedId: 999 });

    expect(await screen.findByText("工具目录")).toBeTruthy();
  });

  it("filters the list by search text", async () => {
    renderPanel();

    await screen.findByText("Notion");
    const list = screen.getByLabelText("MCP 安装列表");
    act(() => {
      fireEvent.change(screen.getByPlaceholderText("搜索安装"), { target: { value: "notion" } });
    });

    await waitFor(() => expect(list.textContent).not.toContain("GitHub"));
    expect(list.textContent).toContain("Notion");

    act(() => {
      fireEvent.change(screen.getByPlaceholderText("搜索安装"), { target: { value: "missing" } });
    });
    expect(await screen.findByText("没有匹配的 MCP 安装")).toBeTruthy();
  });

  it("shows lifecycle actions to owners and hides them from members for organization installations", async () => {
    renderPanel({ organizationRole: "member", selectedId: 2 });
    await screen.findByText("工具目录");
    expect(screen.queryByRole("button", { name: "连接验证" })).toBeNull();
    expect(screen.queryByRole("button", { name: "启用" })).toBeNull();

    cleanup();
    renderPanel({ organizationRole: "owner", selectedId: 2 });
    expect(await screen.findByRole("button", { name: "连接验证" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "启用" })).toBeTruthy();
  });

  it("shows the discovered tools with risk and approval policy", async () => {
    renderPanel();

    expect(await screen.findAllByText("search_issues")).not.toHaveLength(0);
    expect(screen.getByText("只读")).toBeTruthy();
    expect(screen.getByText(/1 tools/)).toBeTruthy();
  });
});
