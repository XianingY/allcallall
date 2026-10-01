import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { MCPInstallation } from "@/api/mcp";

const { mocks } = vi.hoisted(() => ({
  mocks: {
    createMCPInstallation: vi.fn(),
    putMCPInstallationSecrets: vi.fn(),
    updateMCPInstallation: vi.fn(),
  },
}));

vi.mock("@/api/mcp", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/mcp")>()),
  createMCPInstallation: mocks.createMCPInstallation,
  putMCPInstallationSecrets: mocks.putMCPInstallationSecrets,
  updateMCPInstallation: mocks.updateMCPInstallation,
}));

import { InstallationWizard, MCPSecretsDialog, RenameInstallationDialog } from "@/pages/mcp/MCPDialogs";

const installation = (overrides: Partial<MCPInstallation> = {}): MCPInstallation => ({
  id: 1,
  organization_id: 2,
  owner_user_id: 3,
  scope: "personal",
  display_name: "GitHub",
  source_type: "https",
  status: "active",
  active_revision_id: 8,
  secrets_configured: false,
  created_at: "2026-07-11T00:00:00Z",
  updated_at: "2026-07-11T00:00:00Z",
  ...overrides,
});

function renderDialog(node: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>);
}

function renderWizard({ canManageOrganization = true } = {}) {
  const onCreated = vi.fn();
  const onOpenChange = vi.fn();
  renderDialog(
    <InstallationWizard
      open
      onOpenChange={onOpenChange}
      organizationId={2}
      canManageOrganization={canManageOrganization}
      onCreated={onCreated}
    />,
  );
  return { onCreated, onOpenChange };
}

describe("InstallationWizard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.createMCPInstallation.mockResolvedValue(installation());
  });
  afterEach(cleanup);

  it("gates the second step on a display name and organization scope on admin rights", async () => {
    renderWizard({ canManageOrganization: false });

    expect(await screen.findByText("步骤 1 / 2")).toBeTruthy();
    expect(screen.getByRole("button", { name: /下一步/ })).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: "组织" })).toHaveProperty("disabled", true);

    act(() => fireEvent.change(screen.getByPlaceholderText("例如：GitHub 工具"), { target: { value: "GitHub 工具" } }));
    expect(screen.getByRole("button", { name: /下一步/ })).toHaveProperty("disabled", false);
  });

  it("rejects HTTP endpoints and accepts clean HTTPS endpoints", async () => {
    renderWizard();
    const nameInput = await screen.findByPlaceholderText("例如：GitHub 工具");
    act(() => fireEvent.change(nameInput, { target: { value: "GitHub 工具" } }));
    act(() => fireEvent.click(screen.getByRole("button", { name: /下一步/ })));

    const endpoint = await screen.findByPlaceholderText("https://mcp.example.com/v1");
    act(() => fireEvent.change(endpoint, { target: { value: "http://mcp.example.com" } }));
    expect(await screen.findByText("请输入不含凭据的 HTTPS 地址")).toBeTruthy();
    expect(screen.getByRole("button", { name: /创建安装/ })).toHaveProperty("disabled", true);

    act(() => fireEvent.change(endpoint, { target: { value: "https://user:pass@mcp.example.com" } }));
    expect(screen.getByText("请输入不含凭据的 HTTPS 地址")).toBeTruthy();

    act(() => fireEvent.change(endpoint, { target: { value: " https://mcp.example.com/v1 " } }));
    act(() => fireEvent.change(screen.getByPlaceholderText("api.example.com, files.example.com"), { target: { value: " api.example.com , files.example.com " } }));
    expect(screen.getByRole("button", { name: /创建安装/ })).toHaveProperty("disabled", false);

    act(() => fireEvent.click(screen.getByRole("button", { name: /创建安装/ })));
    await waitFor(() => expect(mocks.createMCPInstallation).toHaveBeenCalledWith({
      display_name: "GitHub 工具",
      scope: "personal",
      source_type: "https",
      transport: "streamable_http",
      endpoint_url: "https://mcp.example.com/v1",
      network_allowlist: ["api.example.com", "files.example.com"],
    }));
  });

  it("requires OCI images pinned to a sha256 digest", async () => {
    renderWizard();
    const nameInput = await screen.findByPlaceholderText("例如：GitHub 工具");
    act(() => fireEvent.change(nameInput, { target: { value: "内网工具" } }));
    act(() => fireEvent.click(screen.getByRole("button", { name: "OCI" })));
    act(() => fireEvent.click(screen.getByRole("button", { name: /下一步/ })));

    const image = await screen.findByPlaceholderText("registry.example.com/mcp/server@sha256:...");
    act(() => fireEvent.change(image, { target: { value: "registry.example.com/mcp/server:latest" } }));
    expect(await screen.findByText("请输入固定到 sha256 digest 的 OCI 镜像")).toBeTruthy();
    expect(screen.getByRole("button", { name: /创建安装/ })).toHaveProperty("disabled", true);

    act(() => fireEvent.change(image, { target: { value: "registry.example.com/mcp/server@sha256:" + "a".repeat(64) } }));
    act(() => fireEvent.change(screen.getByPlaceholderText("python,-m,server"), { target: { value: " python , -m , server " } }));
    expect(screen.getByRole("button", { name: /创建安装/ })).toHaveProperty("disabled", false);

    act(() => fireEvent.click(screen.getByRole("button", { name: /创建安装/ })));
    await waitFor(() => expect(mocks.createMCPInstallation).toHaveBeenCalledWith({
      display_name: "内网工具",
      scope: "personal",
      source_type: "oci",
      transport: "stdio",
      image_ref: "registry.example.com/mcp/server@sha256:" + "a".repeat(64),
      command: ["python", "-m", "server"],
      args: [],
      network_allowlist: [],
    }));
  });
});

describe("MCPSecretsDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.putMCPInstallationSecrets.mockResolvedValue({ secrets_configured: true });
  });
  afterEach(cleanup);

  it("submits only completed rows and closes on success", async () => {
    const onOpenChange = vi.fn();
    renderDialog(
      <MCPSecretsDialog installation={installation()} open onOpenChange={onOpenChange} organizationId={2} />,
    );

    expect(await screen.findByRole("button", { name: /安全存储/ })).toHaveProperty("disabled", true);

    const nameInputs = screen.getAllByLabelText("Secret 名称");
    const valueInputs = screen.getAllByLabelText("Secret 值");
    act(() => {
      fireEvent.change(nameInputs[0], { target: { value: "API_TOKEN" } });
      fireEvent.change(valueInputs[0], { target: { value: "token-value" } });
      fireEvent.click(screen.getByRole("button", { name: /增加字段/ }));
    });

    const addedName = screen.getAllByLabelText("Secret 名称")[1];
    act(() => fireEvent.change(addedName, { target: { value: "INCOMPLETE" } }));

    act(() => fireEvent.click(screen.getByRole("button", { name: /安全存储/ })));
    await waitFor(() => expect(mocks.putMCPInstallationSecrets).toHaveBeenCalledWith(1, { API_TOKEN: "token-value" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });
});

describe("RenameInstallationDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.updateMCPInstallation.mockResolvedValue(installation({ display_name: "新名称" }));
  });
  afterEach(cleanup);

  it("requires a non-empty name and submits the trimmed value", async () => {
    const onOpenChange = vi.fn();
    renderDialog(
      <RenameInstallationDialog installation={installation()} open onOpenChange={onOpenChange} organizationId={2} />,
    );

    const save = await screen.findByRole("button", { name: /保存/ });
    expect(save).toHaveProperty("disabled", false);

    act(() => fireEvent.change(screen.getByDisplayValue("GitHub"), { target: { value: "  新名称  " } }));
    act(() => fireEvent.click(save));
    await waitFor(() => expect(mocks.updateMCPInstallation).toHaveBeenCalledWith(1, { display_name: "新名称" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });
});
