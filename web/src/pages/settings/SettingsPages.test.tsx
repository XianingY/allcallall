import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const {
  acceptLegalMock,
  changePasswordMock,
  deleteAccountMock,
  getLegalMock,
  listBlocksMock,
  listSessionsMock,
  revokeSessionMock,
  unblockUserMock,
} = vi.hoisted(() => ({
  acceptLegalMock: vi.fn(),
  changePasswordMock: vi.fn(),
  deleteAccountMock: vi.fn(),
  getLegalMock: vi.fn(),
  listBlocksMock: vi.fn(),
  listSessionsMock: vi.fn(),
  revokeSessionMock: vi.fn(),
  unblockUserMock: vi.fn(),
}));

vi.mock("@/api/identity", () => ({
  acceptLegal: () => acceptLegalMock(),
  changePassword: (oldPassword: string, newPassword: string, confirm: string) =>
    changePasswordMock(oldPassword, newPassword, confirm),
  deleteAccount: (input: unknown) => deleteAccountMock(input),
  getLegal: () => getLegalMock(),
  listBlocks: () => listBlocksMock(),
  listSessions: () => listSessionsMock(),
  revokeSession: (id: number) => revokeSessionMock(id),
  unblockUser: (id: number) => unblockUserMock(id),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({
    status: "authenticated",
    user: { id: 1, email: "user@example.com", display_name: "测试用户" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
}));

import {
  BlockedSettingsPage,
  DangerSettingsPage,
  LegalSettingsPage,
  PasswordSettingsPage,
} from "@/pages/settings/SettingsPages";

const block = {
  id: 3,
  blocked_user_id: 9,
  blocked_user_email: "blocked@example.com",
  blocked_user_display_name: "已屏蔽用户",
  created_at: "2026-09-01T10:00:00Z",
};

const legal = {
  terms_version: "v3",
  privacy_version: "v3",
  terms_url: "https://example.com/terms",
  privacy_policy_url: "https://example.com/privacy",
  support_email: "support@example.com",
  account_deletion_url: "https://example.com/delete",
};

const renderPanel = (node: React.ReactNode) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );

describe("SettingsPages", () => {
  afterEach(cleanup);

  it("shows an error when unblocking a user fails", async () => {
    listBlocksMock.mockResolvedValue([block]);
    unblockUserMock.mockRejectedValue(new Error("解除失败"));
    renderPanel(<BlockedSettingsPage />);

    fireEvent.click(await screen.findByRole("button", { name: "解除" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("解除失败"));
  });

  it("shows an error when accepting the legal version fails", async () => {
    getLegalMock.mockResolvedValue(legal);
    acceptLegalMock.mockRejectedValue(new Error("接受失败"));
    renderPanel(<LegalSettingsPage />);

    fireEvent.click(await screen.findByRole("button", { name: "接受当前版本" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("接受失败"));
  });

  it("keeps the delete button disabled while deletion is pending", async () => {
    deleteAccountMock.mockReturnValue(new Promise(() => {}));
    renderPanel(<DangerSettingsPage />);

    fireEvent.change(await screen.findByLabelText("确认邮箱"), {
      target: { value: "user@example.com" },
    });
    fireEvent.change(screen.getByLabelText("当前密码"), {
      target: { value: "secret" },
    });
    const button = screen.getByRole("button", { name: "永久删除账号" });
    fireEvent.click(button);

    await waitFor(() => expect(button).toBeDisabled());
  });

  it("keeps the password submit button disabled while the mutation is pending", async () => {
    changePasswordMock.mockReturnValue(new Promise(() => {}));
    renderPanel(<PasswordSettingsPage />);

    fireEvent.change(await screen.findByLabelText("当前密码"), {
      target: { value: "old-secret" },
    });
    fireEvent.change(screen.getByLabelText("新密码"), {
      target: { value: "new-secret" },
    });
    fireEvent.change(screen.getByLabelText("确认新密码"), {
      target: { value: "new-secret" },
    });
    const button = screen.getByRole("button", { name: "更新密码" });
    fireEvent.click(button);

    await waitFor(() => expect(button).toBeDisabled());
  });
});
