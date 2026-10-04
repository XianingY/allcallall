import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Conversation } from "@/api/collaboration";
import { ConversationSidebar } from "@/pages/collaboration/ConversationSidebar";

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
});
