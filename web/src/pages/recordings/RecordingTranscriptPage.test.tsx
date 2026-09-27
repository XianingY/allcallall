import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { getRecordingMock, getTranscriptMock, downloadRecordingMock, retryTranscriptionMock } = vi.hoisted(() => ({
  getRecordingMock: vi.fn<(id: number) => Promise<unknown>>(),
  getTranscriptMock: vi.fn<(id: number, afterId?: number) => Promise<unknown>>(),
  downloadRecordingMock: vi.fn<(recordingId: number, fileId: number) => Promise<unknown>>(),
  retryTranscriptionMock: vi.fn<(id: number) => Promise<unknown>>(),
}));

vi.mock("@/api/meetings", () => ({
  getRecording: getRecordingMock,
  getTranscript: getTranscriptMock,
  downloadRecording: downloadRecordingMock,
  retryTranscription: retryTranscriptionMock,
}));

import { RecordingTranscriptPage } from "@/pages/recordings/RecordingTranscriptPage";

const session = {
  id: 42,
  organization_id: 7,
  room_id: 3,
  started_by: 1,
  status: "stopped",
  started_at: "2026-09-27T00:00:00Z",
  stopped_at: "2026-09-27T00:30:00Z",
  created_at: "2026-09-27T00:00:00Z",
  updated_at: "2026-09-27T00:30:00Z",
};

const recordingFile = {
  id: 7,
  recording_session_id: 42,
  storage_driver: "local",
  object_key: "recordings/42/audio.mp3",
  content_type: "audio/mpeg",
  duration_seconds: 180,
  created_at: "2026-09-27T00:30:00Z",
  download_url: "/api/v1/recordings/42/files/7",
  file_name: "a.mp3",
  file_size_bytes: 1024,
  recording_kind: "audio",
};

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <MemoryRouter initialEntries={["/recordings/42"]}>
        <Routes>
          <Route path="/recordings/:recordingId" element={<RecordingTranscriptPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("RecordingTranscriptPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getTranscriptMock.mockResolvedValue({ segments: [], transcription: null, next_after_id: null });
    downloadRecordingMock.mockResolvedValue({ blob: new Blob(["x"]), fileName: "a.mp3" });
    retryTranscriptionMock.mockResolvedValue({ transcription: null });
  });
  afterEach(cleanup);

  it("renders the header without download buttons when the payload omits files", async () => {
    // The API may omit/null `files`; the page must not crash on `.map` of undefined.
    getRecordingMock.mockResolvedValue({ session, transcription: null });
    renderPage();

    expect(await screen.findByRole("heading", { name: "会议转写 #42" })).toBeInTheDocument();
    expect(screen.queryAllByRole("button")).toEqual([]);
  });

  it("surfaces a failed download in an alert", async () => {
    getRecordingMock.mockResolvedValue({ session, files: [recordingFile], transcription: null });
    downloadRecordingMock.mockRejectedValueOnce(new Error("存储服务不可用"));
    renderPage();

    const button = await screen.findByRole("button", { name: "a.mp3" });
    fireEvent.click(button);

    expect(await screen.findByRole("alert")).toHaveTextContent("下载失败：存储服务不可用");
  });
});
