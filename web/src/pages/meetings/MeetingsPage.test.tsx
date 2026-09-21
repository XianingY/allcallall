import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Room } from "@/api/meetings";
import type { Organization } from "@/api/identity";

const { listRoomsMock } = vi.hoisted(() => ({
  listRoomsMock: vi.fn(),
}));

vi.mock("@/api/meetings", () => ({
  listRooms: () => listRoomsMock(),
  createRoom: () => Promise.resolve({ room: { id: 1 } }),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({
    organizations: [{ id: 7, name: "Demo", slug: "demo", role: "owner" } as Organization],
    activeOrganization: { id: 7, name: "Demo", slug: "demo", role: "owner" } as Organization,
    loading: false,
    select: () => Promise.resolve(),
    create: () => Promise.resolve({} as Organization),
  }),
}));

import { MeetingsPage } from "@/pages/meetings/MeetingsPage";

const room = (id: number, title: string, isActive: boolean): Room => ({
  room: {
    id,
    organization_id: 7,
    title,
    status: isActive ? "active" : "ended",
    created_by: 1,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
  },
  members: [],
  events: [],
  participant_count: 3,
  is_active: isActive,
  has_recording: false,
});

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <MeetingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("MeetingsPage", () => {
  afterEach(cleanup);

  it("renders each room returned by the API", async () => {
    listRoomsMock.mockResolvedValue([room(11, "周会", false), room(12, "客户沟通", true)]);
    renderPage();

    await waitFor(() => expect(screen.getByText("周会")).toBeInTheDocument());
    expect(screen.getByText("客户沟通")).toBeInTheDocument();
    // Active rooms expose a join link, inactive ones a view link.
    expect(screen.getByRole("link", { name: "加入" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看" })).toBeInTheDocument();
  });

  it("renders no cards when the organization has no rooms", async () => {
    listRoomsMock.mockResolvedValue([]);
    renderPage();

    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "会议" })).toBeInTheDocument(),
    );
    expect(screen.queryByText("独立会议")).not.toBeInTheDocument();
  });

  it("surfaces the API error instead of an empty grid", async () => {
    listRoomsMock.mockRejectedValue(new Error("加载会议失败"));
    renderPage();

    await waitFor(() =>
      expect(screen.getByText("加载会议失败")).toBeInTheDocument(),
    );
  });
});
