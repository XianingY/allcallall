import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const { sendResetMock, confirmResetMock } = vi.hoisted(() => ({
  sendResetMock: vi.fn<(email: string) => Promise<unknown>>(() => Promise.resolve({})),
  confirmResetMock: vi.fn<(input: unknown) => Promise<unknown>>(() => Promise.resolve({})),
}));

vi.mock("@/api/identity", () => ({
  sendPasswordReset: (email: string) => sendResetMock(email),
  confirmPasswordReset: (input: unknown) => confirmResetMock(input),
}));

import { ForgotPasswordPage } from "@/pages/auth/ForgotPasswordPage";

const renderPage = () =>
  render(
    <MemoryRouter>
      <ForgotPasswordPage />
    </MemoryRouter>,
  );

describe("ForgotPasswordPage", () => {
  afterEach(cleanup);

  it("sends the reset code only once for a burst of clicks", async () => {
    sendResetMock.mockClear();
    renderPage();

    fireEvent.change(screen.getByLabelText("账号邮箱"), {
      target: { value: "ada@example.com" },
    });
    const button = screen.getByRole("button", { name: /发送重置码/ });
    fireEvent.click(button);
    fireEvent.click(button);

    await waitFor(() => expect(sendResetMock).toHaveBeenCalledTimes(1));
    expect(sendResetMock).toHaveBeenCalledWith("ada@example.com");
  });

  it("confirms the password reset only once for a burst of clicks", async () => {
    sendResetMock.mockClear();
    confirmResetMock.mockClear();
    confirmResetMock.mockImplementation(() => new Promise(() => {}));
    renderPage();

    fireEvent.change(screen.getByLabelText("账号邮箱"), {
      target: { value: "ada@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /发送重置码/ }));
    await screen.findByRole("button", { name: /更新密码/ });

    fireEvent.change(screen.getByLabelText("验证码"), { target: { value: "123456" } });
    fireEvent.change(screen.getByLabelText("新密码"), { target: { value: "supersecret1" } });
    fireEvent.change(screen.getByLabelText("确认新密码"), { target: { value: "supersecret1" } });
    const button = screen.getByRole("button", { name: /更新密码/ });
    fireEvent.click(button);
    fireEvent.click(button);

    await waitFor(() => expect(confirmResetMock).toHaveBeenCalledTimes(1));
    expect(confirmResetMock).toHaveBeenCalledWith({
      email: "ada@example.com",
      code: "123456",
      new_password: "supersecret1",
      confirm_password: "supersecret1",
    });
  });
});
