import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

beforeEach(() => {
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  });
});

const {
  sendMessageMock,
  uploadAttachmentMock,
  createNoteMock,
  updateConversationMock,
  pinMessageMock,
  createRoomMock,
  sendTypingMock,
  conversation,
  detail,
  message,
  note,
} = vi.hoisted(() => {
  const conversation = {
    id: 5,
    organization_id: 7,
    type: "direct",
    title: "客户支持",
    topic: "退款流程",
    status: "open",
    priority: "normal",
    unread_count: 2,
    last_message_preview: "你好",
    last_message_at: "2026-09-27T08:00:00Z",
  };
  const detail = {
    conversation,
    workspace: {
      agent_context: {
        meeting_transcript_segment_count: 0,
        transcript_segment_count: 0,
        knowledge_source_count: 0,
        pending_approval_count: 0,
      },
    },
  };
  const message = {
    id: 101,
    organization_id: 7,
    conversation_id: 5,
    sender_id: 2,
    sender_email: "bob@example.com",
    sender_display_name: "Bob",
    type: "text",
    body: "你好，需要帮助",
    pinned: false,
    created_at: "2026-09-27T08:01:00Z",
  };
  const note = {
    id: 31,
    body: "客户要求升级处理",
    author_display_name: "Ada",
    created_at: "2099-01-01T00:00:00Z",
  };
  const noop = vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined));
  return {
    sendMessageMock: noop,
    uploadAttachmentMock: vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined)),
    createNoteMock: vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined)),
    updateConversationMock: vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined)),
    pinMessageMock: vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined)),
    createRoomMock: vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined)),
    sendTypingMock: vi.fn<(...args: unknown[]) => Promise<unknown>>(() => Promise.resolve(undefined)),
    conversation,
    detail,
    message,
    note,
  };
});

vi.mock("@/api/collaboration", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/collaboration")>()),
  listConversations: () =>
    Promise.resolve({
      conversations: [conversation],
      pagination: { total: 1, limit: 50, offset: 0, has_more: false },
    }),
  getConversation: () => Promise.resolve(detail),
  listMessages: () => Promise.resolve({ messages: [message], has_more_prev: false }),
  listPinnedMessages: () => Promise.resolve([]),
  listNotes: () => Promise.resolve([note]),
  markConversationRead: () => Promise.resolve(),
  sendTyping: (...args: unknown[]) => sendTypingMock(...args),
  sendMessage: (...args: unknown[]) => sendMessageMock(...args),
  uploadAttachment: (...args: unknown[]) => uploadAttachmentMock(...args),
  createNote: (...args: unknown[]) => createNoteMock(...args),
  updateConversation: (...args: unknown[]) => updateConversationMock(...args),
  pinMessage: (...args: unknown[]) => pinMessageMock(...args),
}));

vi.mock("@/api/meetings", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/meetings")>()),
  createRoom: (...args: unknown[]) => createRoomMock(...args),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({
    status: "authenticated",
    user: { id: 1, email: "ada@example.com", display_name: "Ada" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({
    organizations: [{ id: 7, name: "Demo", slug: "demo", role: "owner" }],
    activeOrganization: { id: 7, name: "Demo", slug: "demo", role: "owner" },
    loading: false,
    error: null,
    retry: vi.fn(),
    select: () => Promise.resolve(),
    create: () => Promise.resolve({}),
  }),
}));

vi.mock("@/pages/collaboration/InboxFormat", () => ({
  formatTime: vi.fn(() => "formatted"),
}));

import { InboxPage } from "@/pages/collaboration/InboxPage";
import { formatTime } from "@/pages/collaboration/InboxFormat";

const renderPage = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <MemoryRouter initialEntries={["/conversations/5"]}>
        <Routes>
          <Route path="/conversations/:conversationId" element={<InboxPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("InboxPage mutation failures", () => {
  afterEach(cleanup);
  beforeEach(() => vi.clearAllMocks());

  it("keeps the draft and surfaces an alert when sending fails", async () => {
    sendMessageMock.mockRejectedValueOnce(new Error("发送失败"));
    const { container } = renderPage();

    fireEvent.change(await screen.findByLabelText("输入消息"), { target: { value: "hello" } });
    fireEvent.submit(container.querySelector("form.message-composer") as HTMLFormElement);

    expect(await screen.findByRole("alert")).toHaveTextContent("发送失败");
    expect(screen.getByLabelText("输入消息")).toHaveValue("hello");
  });

  it("surfaces an alert when uploading an attachment fails", async () => {
    uploadAttachmentMock.mockRejectedValueOnce(new Error("上传失败"));
    const { container } = renderPage();
    await screen.findByLabelText("输入消息");

    const fileInput = container.querySelector("input[type=file]") as HTMLInputElement;
    expect(fileInput).not.toBeNull();
    fireEvent.change(fileInput, {
      target: { files: [new File(["hi"], "a.txt", { type: "text/plain" })] },
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("上传失败");
  });

  it("keeps the note draft and surfaces an alert when saving a note fails", async () => {
    createNoteMock.mockRejectedValueOnce(new Error("备注失败"));
    renderPage();

    fireEvent.change(await screen.findByPlaceholderText("仅团队可见"), { target: { value: "内部信息" } });
    fireEvent.click(screen.getByRole("button", { name: /添加备注/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("备注失败");
    expect(screen.getByPlaceholderText("仅团队可见")).toHaveValue("内部信息");
  });

  it("surfaces an alert when updating the conversation status fails", async () => {
    updateConversationMock.mockRejectedValueOnce(new Error("状态更新失败"));
    renderPage();

    fireEvent.change(await screen.findByRole("combobox", { name: "状态" }), {
      target: { value: "resolved" },
    });

    expect(await screen.findByRole("alert")).toHaveTextContent("状态更新失败");
  });

  it("surfaces an alert when a message action fails", async () => {
    pinMessageMock.mockRejectedValueOnce(new Error("置顶失败"));
    renderPage();

    fireEvent.click(await screen.findByTitle("置顶"));

    expect(await screen.findByRole("alert")).toHaveTextContent("置顶失败");
  });

  it("surfaces an alert when starting a meeting fails", async () => {
    createRoomMock.mockRejectedValueOnce(new Error("开会失败"));
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "开会" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("开会失败");
  });
});

describe("InboxPage typing lifecycle", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });
  beforeEach(() => vi.clearAllMocks());

  it("refreshes typing on an interval and stops after a successful send", async () => {
    const { container } = renderPage();
    const field = await screen.findByLabelText("输入消息");
    vi.useFakeTimers();

    fireEvent.change(field, { target: { value: "h" } });
    act(() => vi.advanceTimersByTime(1000));
    fireEvent.change(field, { target: { value: "he" } });
    act(() => vi.advanceTimersByTime(1000));
    fireEvent.change(field, { target: { value: "hel" } });
    act(() => vi.advanceTimersByTime(500));

    expect(sendTypingMock).toHaveBeenCalledTimes(2);
    expect(sendTypingMock).toHaveBeenNthCalledWith(1, 5, true);
    expect(sendTypingMock).toHaveBeenNthCalledWith(2, 5, true);

    fireEvent.submit(container.querySelector("form.message-composer") as HTMLFormElement);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(sendTypingMock).toHaveBeenCalledTimes(3);
    expect(sendTypingMock).toHaveBeenLastCalledWith(5, false);
    expect(field).toHaveValue("");
  });
});

describe("InboxPage context panel performance", () => {
  afterEach(cleanup);
  beforeEach(() => vi.clearAllMocks());

  it("keeps the context panel from re-rendering while the user types", async () => {
    renderPage();
    await screen.findByText("客户要求升级处理");

    const contextPanelRenders = () =>
      vi.mocked(formatTime).mock.calls.filter(([value]) => value === note.created_at).length;
    const initialRenders = contextPanelRenders();

    const field = await screen.findByLabelText("输入消息");
    fireEvent.change(field, { target: { value: "h" } });
    fireEvent.change(field, { target: { value: "he" } });
    fireEvent.change(field, { target: { value: "hel" } });
    expect(field).toHaveValue("hel");

    expect(contextPanelRenders()).toBe(initialRenders);
  });
});

describe("InboxPage render partition performance", () => {
  afterEach(cleanup);
  beforeEach(() => vi.clearAllMocks());

  it("keeps the sidebar and message bubbles from re-rendering while the user types", async () => {
    renderPage();
    await screen.findByText("你好，需要帮助");

    const renderCount = (timestamp: string) =>
      vi.mocked(formatTime).mock.calls.filter(([value]) => value === timestamp).length;
    const initialSidebarRenders = renderCount(conversation.last_message_at);
    const initialMessageRenders = renderCount(message.created_at);

    const field = screen.getByLabelText("输入消息");
    fireEvent.change(field, { target: { value: "h" } });
    fireEvent.change(field, { target: { value: "he" } });
    fireEvent.change(field, { target: { value: "hel" } });
    expect(field).toHaveValue("hel");

    expect(renderCount(conversation.last_message_at)).toBe(initialSidebarRenders);
    expect(renderCount(message.created_at)).toBe(initialMessageRenders);
  });
});
