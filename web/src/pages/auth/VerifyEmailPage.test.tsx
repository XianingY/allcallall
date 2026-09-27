import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const { sendCodeMock, verifyCodeMock } = vi.hoisted(() => ({
  sendCodeMock: vi.fn<(email: string, purpose: string) => Promise<unknown>>(() => Promise.resolve({})),
  verifyCodeMock: vi.fn<(email: string, code: string, purpose: string) => Promise<unknown>>(() => Promise.resolve({})),
}));

vi.mock("@/api/identity", () => ({
  sendVerificationCode: (email: string, purpose: string) => sendCodeMock(email, purpose),
  verifyEmailCode: (email: string, code: string, purpose: string) =>
    verifyCodeMock(email, code, purpose),
}));

import { VerifyEmailPage } from "@/pages/auth/VerifyEmailPage";

const renderPage = () =>
  render(
    <MemoryRouter initialEntries={["/verify-email?email=ada@example.com"]}>
      <VerifyEmailPage />
    </MemoryRouter>,
  );

describe("VerifyEmailPage", () => {
  afterEach(cleanup);

  it("starts from the email query parameter", () => {
    renderPage();
    expect(screen.getByLabelText("邮箱")).toHaveValue("ada@example.com");
  });

  it("throttles repeated verification code sends", async () => {
    sendCodeMock.mockClear();
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: /发送验证码/ }));
    await waitFor(() => expect(sendCodeMock).toHaveBeenCalledTimes(1));

    const button = await screen.findByRole("button", { name: /秒后可重发/ });
    expect(button).toBeDisabled();

    fireEvent.click(button);
    expect(sendCodeMock).toHaveBeenCalledTimes(1);
  });
});
