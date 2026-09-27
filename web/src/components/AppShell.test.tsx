import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { selectMock } = vi.hoisted(() => ({ selectMock: vi.fn<(id: number) => Promise<void>>() }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({
    status: "authenticated",
    user: { id: 1, email: "ada@example.com", display_name: "Ada" },
    logout: vi.fn(),
  }),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({
    organizations: [
      { id: 1, name: "Alpha", slug: "alpha", role: "owner" },
      { id: 2, name: "Beta", slug: "beta", role: "member" },
    ],
    activeOrganization: { id: 1, name: "Alpha", slug: "alpha", role: "owner" },
    loading: false,
    error: null,
    retry: vi.fn(),
    select: (id: number) => selectMock(id),
    create: vi.fn(),
  }),
}));

vi.mock("@/calls/CallOverlay", () => ({ CallOverlay: () => null }));
vi.mock("@/platform/PushNotificationBridge", () => ({ PushNotificationBridge: () => null }));

import { AppShell } from "@/components/AppShell";

describe("AppShell organization switching", () => {
  afterEach(cleanup);
  beforeEach(() => {
    vi.clearAllMocks();
    selectMock.mockResolvedValue(undefined);
  });

  it("surfaces an alert when switching organizations fails", async () => {
    selectMock.mockRejectedValueOnce(new Error("切换服务不可用"));
    render(
      <MemoryRouter>
        <AppShell />
      </MemoryRouter>,
    );

    fireEvent.change(await screen.findByRole("combobox", { name: "当前组织" }), {
      target: { value: "2" },
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("切换组织失败：切换服务不可用");
  });
});
