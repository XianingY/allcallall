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
});
