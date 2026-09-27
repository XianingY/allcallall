import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Organization } from "@/api/identity";

const { listRoomsMock, createRoomMock } = vi.hoisted(() => ({
  listRoomsMock: vi.fn(() => Promise.resolve([])),
  createRoomMock: vi.fn<(input: { title: string }) => Promise<unknown>>(() => new Promise(() => {})),
}));

vi.mock("@/api/meetings", () => ({
  listRooms: () => listRoomsMock(),
  createRoom: (input: { title: string }) => createRoomMock(input),
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

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <MemoryRouter>
        <MeetingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("MeetingsPage double-submit guards", () => {
  afterEach(cleanup);

  it("creates a room only once for a burst of clicks", async () => {
    listRoomsMock.mockResolvedValue([]);
    createRoomMock.mockClear();
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /创建会议/ }));
    fireEvent.change(await screen.findByLabelText("会议标题"), { target: { value: "周会" } });
    const button = screen.getByRole("button", { name: /创建并预检/ });
    fireEvent.click(button);
    await waitFor(() => expect(createRoomMock).toHaveBeenCalledTimes(1));
    // RQ flushes isPending in a microtask, so re-click only after that render — real browser clicks are separate tasks.
    fireEvent.click(button);
    expect(createRoomMock).toHaveBeenCalledTimes(1);
    expect(createRoomMock).toHaveBeenCalledWith({ title: "周会" });
  });
});
