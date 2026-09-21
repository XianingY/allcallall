import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { loginMock } = vi.hoisted(() => ({
  loginMock: vi.fn(() => Promise.resolve()),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({ login: loginMock }),
}));

import { LoginPage } from "@/pages/auth/LoginPage";

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const submit = () => fireEvent.click(screen.getByRole("button", { name: /登录/ }));

describe("LoginPage", () => {
  beforeEach(() => loginMock.mockClear());
  afterEach(cleanup);

  it("renders the email and password fields", () => {
    renderPage();
    expect(screen.getByLabelText("邮箱")).toBeInTheDocument();
    expect(screen.getByLabelText("密码")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /登录/ })).toBeInTheDocument();
  });

  it("blocks submission and surfaces validation errors when empty", async () => {
    renderPage();
    submit();

    await waitFor(() =>
      expect(screen.getByText("请输入有效邮箱")).toBeInTheDocument(),
    );
    expect(screen.getByText("请输入密码")).toBeInTheDocument();
    expect(loginMock).not.toHaveBeenCalled();
  });

  it("submits the credentials the user typed", async () => {
    renderPage();
    fireEvent.change(screen.getByLabelText("邮箱"), {
      target: { value: "user@example.com" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "secret123" },
    });
    submit();

    await waitFor(() =>
      expect(loginMock).toHaveBeenCalledWith("user@example.com", "secret123"),
    );
  });

  it("shows the failure reason when login rejects", async () => {
    loginMock.mockRejectedValueOnce(new Error("邮箱或密码不正确"));
    renderPage();
    fireEvent.change(screen.getByLabelText("邮箱"), {
      target: { value: "user@example.com" },
    });
    fireEvent.change(screen.getByLabelText("密码"), {
      target: { value: "wrong" },
    });
    submit();

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("邮箱或密码不正确"),
    );
  });
});
