import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Recording } from "@/api/meetings";
import type { PageRequest } from "@/api/pagination";

const { listRecordingsMock } = vi.hoisted(() => ({
  listRecordingsMock: vi.fn(),
}));

vi.mock("@/api/meetings", () => ({
  listRecordings: (page?: PageRequest) => listRecordingsMock(page),
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

import { RecordingsPage } from "@/pages/recordings/RecordingsPage";

const recording = (id: number): Recording => ({
  session: {
    id,
    organization_id: 7,
    room_id: 3,
    started_by: 1,
    status: "stopped",
    started_at: "2026-09-01T10:00:00Z",
    stopped_at: "2026-09-01T10:30:00Z",
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:30:00Z",
  },
  files: [],
  transcription: null,
});

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <RecordingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("RecordingsPage", () => {
  afterEach(cleanup);

  it("renders the recordings of the first page with the total count", async () => {
    listRecordingsMock.mockImplementation((page?: PageRequest) =>
      Promise.resolve({
        recordings: [recording(101)],
        pagination: { total: 2, limit: 50, offset: page?.offset ?? 0, has_more: true },
      }),
    );
    renderPage();

    await waitFor(() =>
      expect(screen.getByText("会议录音 #101")).toBeInTheDocument(),
    );
    expect(screen.getByText("共 2 个录音存档")).toBeInTheDocument();
  });

  it("asks the API for offset 0 first", async () => {
    listRecordingsMock.mockResolvedValue({
      recordings: [],
      pagination: { total: 0, limit: 50, offset: 0, has_more: false },
    });
    renderPage();

    await waitFor(() =>
      expect(listRecordingsMock).toHaveBeenCalledWith({ limit: 50, offset: 0 }),
    );
  });

  it("shows an empty state when there is nothing to list", async () => {
    listRecordingsMock.mockResolvedValue({
      recordings: [],
      pagination: { total: 0, limit: 50, offset: 0, has_more: false },
    });
    renderPage();

    await waitFor(() =>
      expect(screen.getByText("暂无录音存档")).toBeInTheDocument(),
    );
    expect(screen.queryByText(/共 0 个录音存档/)).not.toBeInTheDocument();
  });

  // This is the regression guard for the offset-based append pagination:
  // "load more" must request the next offset and append, never refetch page 0.
  it("appends the next offset when loading more", async () => {
    listRecordingsMock.mockImplementation((page?: PageRequest) => {
      const offset = page?.offset ?? 0;
      return Promise.resolve({
        recordings: offset === 0 ? [recording(101)] : [recording(102)],
        pagination: { total: 2, limit: 50, offset, has_more: offset === 0 },
      });
    });
    renderPage();

    await waitFor(() =>
      expect(screen.getByText("会议录音 #101")).toBeInTheDocument(),
    );
    const loadMore = screen.getByRole("button", { name: "加载更多" });
    fireEvent.click(loadMore);

    await waitFor(() =>
      expect(listRecordingsMock).toHaveBeenCalledWith({ limit: 50, offset: 50 }),
    );
    await waitFor(() =>
      expect(screen.getByText("会议录音 #102")).toBeInTheDocument(),
    );
    // The first page is still on screen: pages are appended, not replaced.
    expect(screen.getByText("会议录音 #101")).toBeInTheDocument();
    // has_more is false on the last page, so the button disappears.
    expect(screen.queryByRole("button", { name: "加载更多" })).not.toBeInTheDocument();
  });
});
