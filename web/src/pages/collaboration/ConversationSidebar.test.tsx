import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Conversation, MessageSearchHit } from "@/api/collaboration";
import { ConversationSidebar } from "@/pages/collaboration/ConversationSidebar";
import { formatTime } from "@/pages/collaboration/InboxFormat";

vi.mock("@/pages/collaboration/InboxFormat", () => ({
  formatTime: vi.fn(() => "formatted"),
}));

const conversation: Conversation = {
  id: 5,
  organization_id: 7,
  type: "channel",
  title: "退款处理",
  topic: "客户支持",
  status: "open",
  priority: "normal",
  unread_count: 2,
  last_message_preview: "需要帮助",
  last_message_at: "2026-09-27T08:00:00Z",
};

const conversationPages = {
  pages: [{ conversations: [conversation], pagination: { total: 1 } }],
};
const visibleConversations = [conversation];
const emptyMessageHits: MessageSearchHit[] = [];
const searchFilter = { status: "", keyword: "退款", unreadOnly: false, messageQuery: "退款" };
const listFilter = { status: "", keyword: "", unreadOnly: false, messageQuery: "" };

const makeQueries = () => ({
  messageHits: { data: emptyMessageHits, isLoading: false, isError: false },
  conversations: {
    data: conversationPages,
    isLoading: false,
    isError: false,
  },
  visibleConversations,
});

const renderSidebar = () => {
  const onFilterChange = vi.fn();
  const queries = {
    messageHits: { data: [], isLoading: false, isError: false },
    conversations: {
      data: { pages: [{ conversations: [conversation], pagination: { total: 1 } }] },
      isLoading: false,
      isError: false,
    },
    visibleConversations: [conversation],
  } as never;

  const queryClient = new QueryClient();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ConversationSidebar
          filter={{ status: "", keyword: "", unreadOnly: false, messageQuery: "" }}
          onFilterChange={onFilterChange}
          queries={queries}
          selectedId={5}
          onCreateConversation={vi.fn()}
          onOpenConversation={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, onFilterChange };
};

describe("ConversationSidebar", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(cleanup);

  it("changes local filters and commits message search on Enter", () => {
    const { rerender, onFilterChange } = renderSidebar();
    const search = screen.getByLabelText("搜索会话，按回车搜索消息内容");

    fireEvent.change(search, { target: { value: "退款" } });
    expect(onFilterChange).toHaveBeenLastCalledWith({ keyword: "退款" });

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ConversationSidebar
            filter={{ status: "", keyword: "退款", unreadOnly: false, messageQuery: "" }}
            onFilterChange={onFilterChange}
            queries={{
              messageHits: { data: [], isLoading: false, isError: false },
              conversations: { isLoading: false, isError: false, data: { pages: [] } },
              visibleConversations: [],
            } as never}
            selectedId={5}
            onCreateConversation={vi.fn()}
            onOpenConversation={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    fireEvent.keyDown(search, { key: "Enter" });
    expect(onFilterChange).toHaveBeenLastCalledWith({ messageQuery: "退款" });
  });

  it("clears message search and filters through explicit actions", () => {
    const onFilterChange = vi.fn();
    const queries = {
      messageHits: { data: [], isLoading: false, isError: false },
      conversations: { isLoading: false, isError: false, data: { pages: [] } },
      visibleConversations: [],
    } as never;
    const queryClient = new QueryClient();
    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ConversationSidebar
            filter={{ status: "my", keyword: "退款", unreadOnly: true, messageQuery: "退款" }}
            onFilterChange={onFilterChange}
            queries={queries}
            selectedId={null}
            onCreateConversation={vi.fn()}
            onOpenConversation={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "退出消息搜索" }));
    expect(onFilterChange).toHaveBeenLastCalledWith({ messageQuery: "" });

    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ConversationSidebar
            filter={{ status: "my", keyword: "退款", unreadOnly: true, messageQuery: "" }}
            onFilterChange={onFilterChange}
            queries={queries}
            selectedId={null}
            onCreateConversation={vi.fn()}
            onOpenConversation={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "清除筛选" }));
    expect(onFilterChange).toHaveBeenLastCalledWith({ keyword: "", unreadOnly: false, status: "" });

    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ConversationSidebar
            filter={{ status: "my", keyword: "", unreadOnly: false, messageQuery: "" }}
            onFilterChange={onFilterChange}
            queries={queries}
            selectedId={null}
            onCreateConversation={vi.fn()}
            onOpenConversation={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(screen.getByRole("button", { name: "我的" })).toHaveClass("active");
  });

  it("skips re-rendering when the parent updates with the same data", () => {
    const onFilterChange = vi.fn();
    const onCreateConversation = vi.fn();
    const onOpenConversation = vi.fn();
    const filter = { status: "", keyword: "", unreadOnly: false, messageQuery: "" };
    const makeSidebar = () => (
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ConversationSidebar
            filter={filter}
            onFilterChange={onFilterChange}
            queries={makeQueries() as never}
            selectedId={5}
            onCreateConversation={onCreateConversation}
            onOpenConversation={onOpenConversation}
          />
        </MemoryRouter>
      </QueryClientProvider>
    );

    const { rerender } = render(makeSidebar());
    expect(screen.getByRole("link", { name: /退款处理/ })).toBeInTheDocument();
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(1);

    rerender(makeSidebar());
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(1);
  });

  it("re-renders when message search results change", () => {
    const onFilterChange = vi.fn();
    const onCreateConversation = vi.fn();
    const onOpenConversation = vi.fn();
    const hit: MessageSearchHit = {
      id: "message-101",
      conversation_id: 5,
      message_id: 101,
      sender_display_name: "Bob",
      sender_email: "bob@example.com",
      body: "找到退款记录",
      created_at: "2026-09-27T08:00:00Z",
    };
    const renderSearchSidebar = (data: MessageSearchHit[]) => {
      const queries = makeQueries();
      queries.messageHits = { ...queries.messageHits, data };
      return (
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ConversationSidebar
              filter={searchFilter}
              onFilterChange={onFilterChange}
              queries={queries as never}
              selectedId={5}
              onCreateConversation={onCreateConversation}
              onOpenConversation={onOpenConversation}
            />
          </MemoryRouter>
        </QueryClientProvider>
      );
    };

    const { rerender } = render(renderSearchSidebar([]));
    expect(screen.getByText("没有匹配的消息")).toBeInTheDocument();

    rerender(renderSearchSidebar([hit]));
    expect(screen.getByText("找到退款记录")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Bob/ })).toHaveAttribute(
      "href",
      "/conversations/5",
    );
  });

  it("re-renders when message search enters loading or error state", () => {
    const onFilterChange = vi.fn();
    const onCreateConversation = vi.fn();
    const onOpenConversation = vi.fn();
    const renderSearchSidebar = (
      state: Partial<{ isLoading: boolean; isError: boolean; error: Error }>,
    ) => {
      const queries = makeQueries();
      queries.messageHits = { ...queries.messageHits, ...state };
      return (
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ConversationSidebar
              filter={searchFilter}
              onFilterChange={onFilterChange}
              queries={queries as never}
              selectedId={5}
              onCreateConversation={onCreateConversation}
              onOpenConversation={onOpenConversation}
            />
          </MemoryRouter>
        </QueryClientProvider>
      );
    };

    const { rerender } = render(renderSearchSidebar({}));
    rerender(renderSearchSidebar({ isLoading: true }));
    expect(screen.getByText("正在搜索消息")).toBeInTheDocument();

    rerender(renderSearchSidebar({ isLoading: false, isError: true, error: new Error("搜索失败") }));
    expect(screen.getByText("搜索失败")).toBeInTheDocument();
  });

  it("re-renders when the conversation list enters loading or error state", () => {
    const onFilterChange = vi.fn();
    const onCreateConversation = vi.fn();
    const onOpenConversation = vi.fn();
    const renderListSidebar = (
      state: Partial<{ isLoading: boolean; isError: boolean; error: Error }>,
    ) => {
      const queries = makeQueries();
      queries.conversations = { ...queries.conversations, ...state };
      return (
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <ConversationSidebar
              filter={listFilter}
              onFilterChange={onFilterChange}
              queries={queries as never}
              selectedId={5}
              onCreateConversation={onCreateConversation}
              onOpenConversation={onOpenConversation}
            />
          </MemoryRouter>
        </QueryClientProvider>
      );
    };

    const { rerender } = render(renderListSidebar({}));
    expect(screen.getByRole("link", { name: /退款处理/ })).toBeInTheDocument();

    rerender(renderListSidebar({ isLoading: true }));
    expect(screen.queryByRole("link", { name: /退款处理/ })).not.toBeInTheDocument();

    rerender(renderListSidebar({ isLoading: false, isError: true, error: new Error("会话加载失败") }));
    expect(screen.getByText("会话加载失败")).toBeInTheDocument();
  });

  it("re-renders when the visible conversations change", () => {
    const updatedConversation = { ...conversation, title: "升级处理", unread_count: 3 };
    const { rerender } = renderSidebar();
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(1);

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ConversationSidebar
            filter={{ status: "", keyword: "", unreadOnly: false, messageQuery: "" }}
            onFilterChange={vi.fn()}
            queries={{
              messageHits: { data: [], isLoading: false, isError: false },
              conversations: {
                data: { pages: [{ conversations: [updatedConversation], pagination: { total: 1 } }] },
                isLoading: false,
                isError: false,
              },
              visibleConversations: [updatedConversation],
            } as never}
            selectedId={5}
            onCreateConversation={vi.fn()}
            onOpenConversation={vi.fn()}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(screen.getByRole("link", { name: /升级处理/ })).toBeInTheDocument();
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(2);
  });

  it("only re-renders the rows whose selection changes", () => {
    const conversations = [
      { ...conversation, id: 1, title: "Alpha" },
      { ...conversation, id: 2, title: "Beta" },
      { ...conversation, id: 3, title: "Gamma" },
    ];
    const onFilterChange = vi.fn();
    const onCreateConversation = vi.fn();
    const onOpenConversation = vi.fn();
    const queries = {
      messageHits: { data: [], isLoading: false, isError: false },
      conversations: {
        data: { pages: [{ conversations, pagination: { total: 3 } }] },
        isLoading: false,
        isError: false,
      },
      visibleConversations: conversations,
    } as never;

    const view = (selectedId: number) => (
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ConversationSidebar
            filter={{ status: "", keyword: "", unreadOnly: false, messageQuery: "" }}
            onFilterChange={onFilterChange}
            queries={queries}
            selectedId={selectedId}
            onCreateConversation={onCreateConversation}
            onOpenConversation={onOpenConversation}
          />
        </MemoryRouter>
      </QueryClientProvider>
    );

    const { rerender } = render(view(1));
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(3);

    rerender(view(2));
    expect(vi.mocked(formatTime)).toHaveBeenCalledTimes(5);
    expect(screen.getByRole("link", { name: /Beta/ })).toHaveClass("conversation-item-active");
    expect(screen.getByRole("link", { name: /Beta/ })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: /Alpha/ })).not.toHaveClass("conversation-item-active");
    expect(screen.getByRole("link", { name: /Alpha/ })).not.toHaveAttribute("aria-current");
  });
});
