import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const { registerMock } = vi.hoisted(() => ({
  registerMock: vi.fn(() => Promise.resolve()),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({ register: registerMock }),
}));

vi.mock("@/api/identity", () => ({
  getLegal: () =>
    Promise.resolve({ terms_url: "/legal/terms", privacy_policy_url: "/legal/privacy" }),
  sendVerificationCode: () => Promise.resolve({}),
  verifyEmailCode: () => Promise.resolve({}),
}));

import { RegisterPage } from "@/pages/auth/RegisterPage";

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <RegisterPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const fill = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });

describe("RegisterPage", () => {
  afterEach(cleanup);

  it("renders every registration field", () => {
    renderPage();
    ["显示名称", "邮箱", "验证码", "密码", "确认密码"].forEach((label) =>
      expect(screen.getByLabelText(label)).toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: /验证并注册/ })).toBeInTheDocument();
  });

  it("requires the legal terms checkbox", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /验证并注册/ }));

    await waitFor(() =>
      expect(screen.getByText("请阅读并同意条款")).toBeInTheDocument(),
    );
    expect(registerMock).not.toHaveBeenCalled();
  });

  it("rejects a mismatched password confirmation", async () => {
    renderPage();
    fill("显示名称", "Ada Lovelace");
    fill("邮箱", "ada@example.com");
    fill("验证码", "123456");
    fill("密码", "supersecret1");
    fill("确认密码", "supersecret2");
    fireEvent.click(screen.getByLabelText("我已阅读并同意 服务条款 与 隐私政策"));
    fireEvent.click(screen.getByRole("button", { name: /验证并注册/ }));

    await waitFor(() =>
      expect(screen.getByText("两次密码不一致")).toBeInTheDocument(),
    );
    expect(registerMock).not.toHaveBeenCalled();
  });

  it("requires a six digit verification code", async () => {
    renderPage();
    fill("显示名称", "Ada Lovelace");
    fill("邮箱", "ada@example.com");
    fill("验证码", "123");
    fill("密码", "supersecret1");
    fill("确认密码", "supersecret1");
    fireEvent.click(screen.getByLabelText("我已阅读并同意 服务条款 与 隐私政策"));
    fireEvent.click(screen.getByRole("button", { name: /验证并注册/ }));

    await waitFor(() =>
      expect(screen.getByText("请输入 6 位验证码")).toBeInTheDocument(),
    );
    expect(registerMock).not.toHaveBeenCalled();
  });
});
