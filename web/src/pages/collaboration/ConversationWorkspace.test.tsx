import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ConversationWorkspace } from "@/pages/collaboration/ConversationWorkspace";

const detail = {
  conversation: {
    id: 5,
    organization_id: 7,
    title: "退款处理",
    topic: "客户支持",
    status: "open",
    priority: "normal",
  },
  workspace: { agent_context: {} },
};
const message = {
  id: 101,
  organization_id: 7,
  conversation_id: 5,
  sender_id: 2,
  sender_email: "bob@example.com",
  sender_display_name: "Bob",
  type: "text",
  body: "需要帮助",
  created_at: "2026-09-27T08:00:00Z",
};

const renderWorkspace = () => {
  const onComposerChange = vi.fn();
  const onSubmit = vi.fn();
  const onSetReplyTo = vi.fn();
  const onSetEditing = vi.fn();
    const onClearComposerContext = vi.fn();
    const onBackToList = vi.fn();
    const onToggleContext = vi.fn();
  const queries = {
    detail: { data: detail, isLoading: false, isError: false },
    messages: {
      data: { pages: [{ messages: [message], has_more_prev: false }] },
      isLoading: false,
      isError: false,
      hasNextPage: false,
      isFetchingNextPage: false,
    },
    pins: { data: [] },
    messageItems: [message],
    messageWindow: { visible: [message], hiddenCount: 0 },
  } as never;
  const mutations = {
    send: { isPending: false, error: null, mutate: onSubmit },
    upload: { isPending: false, error: null, mutate: vi.fn() },
    messageAction: { error: null, mutate: vi.fn() },
    startMeeting: { isPending: false, error: null, mutate: vi.fn() },
  } as never;

  render(
    <MemoryRouter>
      <ConversationWorkspace
        selectedId={5}
        currentUserId={2}
        queries={queries}
        mutations={mutations}
        draft={{ composer: "hello", replyTo: null, editing: null, attachments: [] }}
        typingUsers={[]}
        onComposerChange={onComposerChange}
        onSubmit={onSubmit}
        onSetReplyTo={onSetReplyTo}
        onSetEditing={onSetEditing}
        onClearComposerContext={onClearComposerContext}
        onBackToList={onBackToList}
        contextOpen
        onToggleContext={onToggleContext}
      />
    </MemoryRouter>,
  );
  return { onComposerChange, onSubmit, onSetReplyTo, onSetEditing };
};

describe("ConversationWorkspace", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(cleanup);

  it("renders the selected conversation and preserves composer behavior", () => {
    const { onComposerChange, onSubmit } = renderWorkspace();

    expect(screen.getByRole("heading", { name: "退款处理" })).toBeInTheDocument();
    expect(screen.getByText("客户支持")).toBeInTheDocument();

    const field = screen.getByLabelText("输入消息");
    fireEvent.change(field, { target: { value: "hello world" } });
    expect(onComposerChange).toHaveBeenLastCalledWith("hello world");

    fireEvent.click(screen.getByRole("button", { name: "发送消息" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("exposes message reply and edit actions", () => {
    const { onSetReplyTo, onSetEditing } = renderWorkspace();

    fireEvent.click(screen.getByTitle("回复"));
    expect(onSetReplyTo).toHaveBeenCalledWith(message);

    fireEvent.click(screen.getByTitle("编辑"));
    expect(onSetEditing).toHaveBeenCalledWith(message);
  });
});
