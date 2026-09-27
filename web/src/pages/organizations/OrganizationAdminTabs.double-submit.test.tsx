import type { UseQueryResult } from "@tanstack/react-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { OrganizationInvite, OrganizationTeam } from "@/api/identity";

const { createInviteMock, createTeamMock } = vi.hoisted(() => ({
  createInviteMock: vi.fn<
    (orgId: number, input: { target_email: string; role?: string; team_id?: number }) => Promise<unknown>
  >(() => new Promise(() => {})),
  createTeamMock: vi.fn<(orgId: number, input: { name: string; description?: string }) => Promise<unknown>>(
    () => new Promise(() => {}),
  ),
}));

vi.mock("@/api/identity", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/identity")>()),
  createOrganizationInvite: (orgId: number, input: { target_email: string; role?: string; team_id?: number }) =>
    createInviteMock(orgId, input),
  createOrganizationTeam: (orgId: number, input: { name: string; description?: string }) =>
    createTeamMock(orgId, input),
}));

import { InvitesTab, TeamsTab } from "@/pages/organizations/OrganizationAdminTabs";

const inviteQuery = (data: OrganizationInvite[]) =>
  ({
    data,
    error: null,
    isError: false,
    isLoading: false,
    refetch: vi.fn(),
  }) as unknown as UseQueryResult<OrganizationInvite[]>;

const teamQuery = (data: OrganizationTeam[]) =>
  ({
    data,
    error: null,
    isError: false,
    isLoading: false,
    refetch: vi.fn(),
  }) as unknown as UseQueryResult<OrganizationTeam[]>;

function renderWithQueryClient(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe("OrganizationAdminTabs double-submit guards", () => {
  afterEach(cleanup);

  it("creates an invite only once for a burst of clicks", async () => {
    createInviteMock.mockClear();
    renderWithQueryClient(
      <InvitesTab orgId={7} canManage invites={inviteQuery([])} teams={[]} refresh={vi.fn()} />,
    );

    fireEvent.change(screen.getByPlaceholderText("成员邮箱"), {
      target: { value: "ada@example.com" },
    });
    const button = screen.getByRole("button", { name: /邀请/ });
    fireEvent.click(button);
    await waitFor(() => expect(createInviteMock).toHaveBeenCalledTimes(1));
    // RQ flushes isPending in a microtask, so re-click only after that render — real browser clicks are separate tasks.
    fireEvent.click(button);
    expect(createInviteMock).toHaveBeenCalledTimes(1);
    expect(createInviteMock).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ target_email: "ada@example.com" }),
    );
  });

  it("creates a team only once for a burst of clicks", async () => {
    createTeamMock.mockClear();
    renderWithQueryClient(
      <TeamsTab orgId={7} canManage members={[]} teams={teamQuery([])} refresh={vi.fn()} />,
    );

    fireEvent.change(screen.getByPlaceholderText("团队名称"), { target: { value: "QA" } });
    const button = screen.getByRole("button", { name: /创建团队/ });
    fireEvent.click(button);
    await waitFor(() => expect(createTeamMock).toHaveBeenCalledTimes(1));
    // RQ flushes isPending in a microtask, so re-click only after that render — real browser clicks are separate tasks.
    fireEvent.click(button);
    expect(createTeamMock).toHaveBeenCalledTimes(1);
    expect(createTeamMock).toHaveBeenCalledWith(7, expect.objectContaining({ name: "QA" }));
  });
});
