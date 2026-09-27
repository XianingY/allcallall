import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { room, getRoomMock, startRecordingMock, stopRecordingMock } = vi.hoisted(() => {
  const room = {
    room: {
      id: 42,
      organization_id: 7,
      title: "产品评审",
      status: "live",
      created_by: 1,
      created_at: "2026-09-27T00:00:00Z",
      updated_at: "2026-09-27T00:00:00Z",
    },
    members: [],
    events: [],
    active_recording: null,
    participant_count: 1,
    is_active: true,
    has_recording: false,
    latest_recording_id: null,
  };
  return {
    room,
    getRoomMock: vi.fn<(id: number) => Promise<unknown>>(),
    startRecordingMock: vi.fn<(id: number) => Promise<unknown>>(),
    stopRecordingMock: vi.fn<(id: number) => Promise<unknown>>(),
  };
});

vi.mock("@/api/meetings", () => ({
  getRoom: getRoomMock,
  startRecording: startRecordingMock,
  stopRecording: stopRecordingMock,
}));
vi.mock("@/meetings/useMeetingEngine", () => ({
  useMeetingEngine: () => ({
    localStream: null,
    remoteStreams: new Map<string, MediaStream>(),
    state: "connected",
    error: "",
    audio: true,
    video: false,
    toggleAudio: vi.fn(),
    toggleVideo: vi.fn(),
  }),
}));
vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({ activeOrganization: { id: 7, name: "Alpha", role: "owner" } }),
}));

import { MeetingRoomPage } from "@/pages/meetings/MeetingRoomPage";

function renderRoom() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/meetings/42"]}>
        <Routes>
          <Route path="/meetings/:roomId" element={<MeetingRoomPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("MeetingRoomPage recording", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getRoomMock.mockResolvedValue(room);
    startRecordingMock.mockResolvedValue({ id: 1 });
    stopRecordingMock.mockResolvedValue({ id: 1 });
  });
  afterEach(cleanup);

  it("surfaces a failed recording start in an alert", async () => {
    startRecordingMock.mockRejectedValueOnce(new Error("录制服务暂时不可用"));
    renderRoom();
    const button = await screen.findByRole("button", { name: "开始录制" });
    fireEvent.click(button);
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("录制失败：录制服务暂时不可用");
  });

  it("disables the record button while the mutation is pending", async () => {
    startRecordingMock.mockImplementation(() => new Promise(() => {}));
    renderRoom();
    const button = await screen.findByRole("button", { name: "开始录制" });
    fireEvent.click(button);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "开始录制" })).toBeDisabled();
    });
  });
});
