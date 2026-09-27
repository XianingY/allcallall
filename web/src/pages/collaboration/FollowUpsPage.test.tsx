import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

const { listFollowUpsMock, updateFollowUpMock } = vi.hoisted(() => ({
  listFollowUpsMock: vi.fn(),
  updateFollowUpMock: vi.fn(),
}));

vi.mock("@/api/collaboration", () => ({
  listFollowUps: () => listFollowUpsMock(),
  updateFollowUp: (id: number, input: unknown) => updateFollowUpMock(id, input),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({
    organizations: [],
    activeOrganization: { id: 7, name: "Demo", slug: "demo", role: "owner" },
    loading: false,
    select: () => Promise.resolve(),
    create: () => Promise.resolve({}),
  }),
}));

import { FollowUpsPage } from "@/pages/collaboration/FollowUpsPage";

const item = {
  task: {
    id: 5,
    user_id: 1,
    peer_user_id: 2,
    type: "call",
    status: "pending",
    title: "回访客户",
    description: "确认续约意向",
    due_at: null,
    completed_at: null,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  },
  is_overdue: false,
};

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <FollowUpsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("FollowUpsPage", () => {
  afterEach(cleanup);

  it("shows an error when the follow-up update rejects", async () => {
    listFollowUpsMock.mockResolvedValue([item]);
    updateFollowUpMock.mockRejectedValue(new Error("更新失败"));
    renderPage();

    fireEvent.click(await screen.findByTitle("标记完成"));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("更新失败"));
  });

  it("keeps the complete button disabled while the update is pending", async () => {
    listFollowUpsMock.mockResolvedValue([item]);
    updateFollowUpMock.mockReturnValue(new Promise(() => {}));
    renderPage();

    fireEvent.click(await screen.findByTitle("标记完成"));

    await waitFor(() => expect(screen.getByTitle("标记完成")).toBeDisabled());
  });
});
