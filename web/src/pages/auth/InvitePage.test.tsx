import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const { acceptInviteMock } = vi.hoisted(() => ({
  acceptInviteMock: vi.fn<(code: string) => Promise<unknown>>(() => new Promise(() => {})),
}));

vi.mock("@/api/identity", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/identity")>()),
  acceptOrganizationInvite: (code: string) => acceptInviteMock(code),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({ status: "authenticated" }),
}));

import { InvitePage } from "@/pages/auth/InvitePage";

const renderPage = () =>
  render(
    <MemoryRouter initialEntries={["/invite/abc123"]}>
      <Routes>
        <Route path="/invite/:code" element={<InvitePage />} />
      </Routes>
    </MemoryRouter>,
  );

describe("InvitePage", () => {
  afterEach(cleanup);

  it("accepts the invite only once for a burst of clicks", async () => {
    acceptInviteMock.mockClear();
    renderPage();

    const button = await screen.findByRole("button", { name: /接受邀请/ });
    fireEvent.click(button);
    fireEvent.click(button);

    await waitFor(() => expect(acceptInviteMock).toHaveBeenCalledTimes(1));
    expect(acceptInviteMock).toHaveBeenCalledWith("abc123");
  });
});
