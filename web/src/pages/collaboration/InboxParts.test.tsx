import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { createConversationMock } = vi.hoisted(() => ({
  createConversationMock: vi.fn<
    (input: { type: string; title?: string; topic?: string }) => Promise<unknown>
  >(() => new Promise(() => {})),
}));

vi.mock("@/api/collaboration", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/collaboration")>()),
  createConversation: (input: { type: string; title?: string; topic?: string }) =>
    createConversationMock(input),
}));

vi.mock("@/pages/collaboration/InboxFormat", () => ({
  formatTime: vi.fn(() => "formatted"),
}));

import { formatTime } from "@/pages/collaboration/InboxFormat";
import { MessageBubble, NewConversationDialog } from "@/pages/collaboration/InboxParts";

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
  created_at: "2026-09-27T08:00:00Z",
};

const renderBubble = () => {
  const onReply = vi.fn();
  const onEdit = vi.fn();
  const onAction = vi.fn();
  const view = render(<MessageBubble message={message} currentUserId={1} onReply={onReply} onEdit={onEdit} onAction={onAction} />);
  return { ...view, onReply, onEdit, onAction };
};

const renderDialog = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <NewConversationDialog open onOpenChange={() => {}} orgId={7} onCreated={() => {}} />
    </QueryClientProvider>,
  );

describe("NewConversationDialog double-submit guards", () => {
  afterEach(cleanup);

  it("creates a conversation only once for a burst of clicks", async () => {
    createConversationMock.mockClear();
    renderDialog();

    fireEvent.change(await screen.findByLabelText("标题"), { target: { value: "产品讨论" } });
    const button = screen.getByRole("button", { name: /创建会话/ });
    fireEvent.click(button);
    await waitFor(() => expect(createConversationMock).toHaveBeenCalledTimes(1));
    // RQ flushes isPending in a microtask, so re-click only after that render — real browser clicks are separate tasks.
    fireEvent.click(button);
    expect(createConversationMock).toHaveBeenCalledTimes(1);
    expect(createConversationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: "channel", title: "产品讨论" }),
    );
  });
});

describe("MessageBubble render stability", () => {
  afterEach(cleanup);

  it("does not re-render when the parent updates with the same message", () => {
    const onReply = vi.fn();
    const onEdit = vi.fn();
    const onAction = vi.fn();
    const makeBubble = () => (
      <MessageBubble
        message={message}
        currentUserId={1}
        onReply={onReply}
        onEdit={onEdit}
        onAction={onAction}
      />
    );

    const { rerender } = render(makeBubble());
    expect(screen.getByText("你好，需要帮助")).toBeInTheDocument();
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(1);

    rerender(makeBubble());
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(1);
  });

  it("re-renders when the message data changes", () => {
    const onReply = vi.fn();
    const onEdit = vi.fn();
    const onAction = vi.fn();
    const { rerender } = renderBubble();
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(1);

    const updatedMessage = { ...message, body: "问题已解决" };
    rerender(
      <MessageBubble
        message={updatedMessage}
        currentUserId={1}
        onReply={onReply}
        onEdit={onEdit}
        onAction={onAction}
      />,
    );

    expect(screen.getByText("问题已解决")).toBeInTheDocument();
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(2);
  });
});
