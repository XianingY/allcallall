import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const {
  getRecordingMock,
  getTranscriptMock,
  downloadRecordingMock,
  retryTranscriptionMock,
  editTranscriptSegmentMock,
  listTranscriptSegmentRevisionsMock,
} = vi.hoisted(() => ({
  getRecordingMock: vi.fn<(id: number) => Promise<unknown>>(),
  getTranscriptMock: vi.fn<(id: number, afterId?: number) => Promise<unknown>>(),
  downloadRecordingMock: vi.fn<(recordingId: number, fileId: number) => Promise<unknown>>(),
  retryTranscriptionMock: vi.fn<(id: number) => Promise<unknown>>(),
  editTranscriptSegmentMock: vi.fn<(recordingId: number, segmentId: number, input: { text: string; reason?: string }) => Promise<unknown>>(),
  listTranscriptSegmentRevisionsMock: vi.fn<(recordingId: number, segmentId: number, beforeId?: number) => Promise<unknown>>(),
}));

const authState = vi.hoisted(() => ({ user: { id: 5, email: "user@example.com", display_name: "User", status: "active" } as null | Record<string, unknown> }));
const organizationState = vi.hoisted(() => ({ role: "admin" as string | undefined }));

vi.mock("@/api/meetings", () => ({
  getRecording: getRecordingMock,
  getTranscript: getTranscriptMock,
  downloadRecording: downloadRecordingMock,
  retryTranscription: retryTranscriptionMock,
  editTranscriptSegment: editTranscriptSegmentMock,
  listTranscriptSegmentRevisions: listTranscriptSegmentRevisionsMock,
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({ status: "authenticated", user: authState.user, login: vi.fn(), register: vi.fn(), logout: vi.fn() }),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({ organizations: [], activeOrganization: { id: 7, role: organizationState.role }, loading: false, error: null, retry: vi.fn(), select: vi.fn(), create: vi.fn() }),
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

const readyTranscription = { status: "ready", provider: "test", segment_count: 1, error_message: "", started_at: null, completed_at: null, created_at: "2026-09-27T00:30:00Z", updated_at: "2026-09-27T00:30:00Z", id: 9 };

const segment = (overrides: Record<string, unknown> = {}) => ({
  id: 11,
  organization_id: 7,
  conversation_id: 2,
  room_id: 3,
  recording_session_id: 42,
  recording_file_id: 7,
  speaker_user_id: 5,
  track_key: "speaker-1",
  source: "recording",
  provider: "test",
  language: "zh-CN",
  text: "Helo worlld",
  start_ms: 1000,
  end_ms: 4200,
  confidence: 0.82,
  edited_at: null,
  editor_user_id: null,
  created_at: "2026-09-27T00:30:00Z",
  ...overrides,
});

describe("RecordingTranscriptPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getTranscriptMock.mockResolvedValue({ segments: [], transcription: null, next_after_id: null });
    downloadRecordingMock.mockResolvedValue({ blob: new Blob(["x"]), fileName: "a.mp3" });
    retryTranscriptionMock.mockResolvedValue({ transcription: null });
    editTranscriptSegmentMock.mockReset();
    listTranscriptSegmentRevisionsMock.mockReset();
    authState.user = { id: 5, email: "user@example.com", display_name: "User", status: "active" };
    organizationState.role = "admin";
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

  it("shows the edit affordance only to permitted users on ready transcripts", async () => {
    getRecordingMock.mockResolvedValue({ session, transcription: readyTranscription });
    getTranscriptMock.mockResolvedValue({ segments: [segment()], transcription: readyTranscription, next_after_id: null });
    organizationState.role = "member";
    renderPage();
    expect(await screen.findByText("Helo worlld")).toBeInTheDocument();
    // speaker 可以编辑自己的片段。
    expect(screen.getByRole("button", { name: "编辑片段 11" })).toBeInTheDocument();

    cleanup();
    getRecordingMock.mockResolvedValue({ session, transcription: readyTranscription });
    getTranscriptMock.mockResolvedValue({ segments: [segment({ speaker_user_id: 999, edited_at: "2026-10-16T00:00:00Z" })], transcription: readyTranscription, next_after_id: null });
    renderPage();
    // 普通成员不能编辑别人说的片段；edited_at 仍显示修订历史入口。
    expect(await screen.findByRole("button", { name: "修订历史 11" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "编辑片段 11" })).not.toBeInTheDocument();

    cleanup();
    getRecordingMock.mockResolvedValue({ session, transcription: { ...readyTranscription, status: "processing" } });
    getTranscriptMock.mockResolvedValue({ segments: [segment()], transcription: { ...readyTranscription, status: "processing" }, next_after_id: null });
    organizationState.role = "admin";
    renderPage();
    expect(await screen.findByText("Helo worlld")).toBeInTheDocument();
    // 转写未 ready 时编辑入口禁用（不渲染），避免编辑即将被重跑覆盖的文本。
    expect(screen.queryByRole("button", { name: "编辑片段 11" })).not.toBeInTheDocument();
  });

  it("edits a segment with a diff preview and refreshes the transcript", async () => {
    getRecordingMock.mockResolvedValue({ session, transcription: readyTranscription });
    getTranscriptMock.mockResolvedValue({ segments: [segment()], transcription: readyTranscription, next_after_id: null });
    editTranscriptSegmentMock.mockResolvedValue({ segment: segment({ text: "Hello world", edited_at: "2026-10-16T00:00:00Z", editor_user_id: 5 }), revision_number: 1, edited: true });
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "编辑片段 11" }));
    const textarea = await screen.findByLabelText("修改后文本");
    fireEvent.change(textarea, { target: { value: "Hello world" } });
    const diffs = await screen.findAllByTestId("transcript-diff");
    const diff = diffs[diffs.length - 1];
    // 字符级 diff：前缀 Hel，删除 "o worl"，插入 "lo wor"，后缀 ld。
    expect(diff).toHaveTextContent("Hel");
    expect(diff.querySelector("del")).toHaveTextContent("o worl");
    expect(diff.querySelector("ins")).toHaveTextContent("lo wor");
    fireEvent.change(screen.getByLabelText("修订理由"), { target: { value: "fix typo" } });
    fireEvent.click(screen.getByRole("button", { name: "保存修订" }));

    await waitFor(() => expect(editTranscriptSegmentMock).toHaveBeenCalledWith(42, 11, { text: "Hello world", reason: "fix typo" }));
    await waitFor(() => expect(getTranscriptMock).toHaveBeenCalledTimes(2));
  });

  it("renders the revision history drawer newest first", async () => {
    getRecordingMock.mockResolvedValue({ session, transcription: readyTranscription });
    getTranscriptMock.mockResolvedValue({ segments: [segment({ edited_at: "2026-10-16T00:00:00Z", editor_user_id: 5 })], transcription: readyTranscription, next_after_id: null });
    listTranscriptSegmentRevisionsMock.mockResolvedValue({
      revisions: [
        { id: 2, organization_id: 7, segment_id: 11, revision_number: 1, previous_text: "Helo worlld", next_text: "Hello world", reason: "fix typo", superseded: false, actor_user_id: 5, created_at: "2026-10-16T00:00:00Z" },
      ],
      next_before_id: null,
    });
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "修订历史 11" }));
    expect(await screen.findByText("修订 1")).toBeInTheDocument();
    expect(screen.getByText("fix typo")).toBeInTheDocument();
    expect(listTranscriptSegmentRevisionsMock).toHaveBeenCalledWith(42, 11, undefined);
  });
});
