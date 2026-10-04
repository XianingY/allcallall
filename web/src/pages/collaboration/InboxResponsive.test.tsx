import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { conversation, detail, message } = vi.hoisted(() => {
  const conversation = {
    id: 5,
    organization_id: 7,
    type: "channel",
    title: "客户支持",
    topic: "退款流程",
    status: "open",
    priority: "normal",
    unread_count: 1,
    last_message_preview: "需要帮助",
    last_message_at: "2026-09-27T08:00:00Z",
  };
  return {
    conversation,
    detail: { conversation, workspace: { agent_context: {} } },
    message: {
      id: 101,
      organization_id: 7,
      conversation_id: 5,
      sender_id: 2,
      sender_email: "bob@example.com",
      sender_display_name: "Bob",
      type: "text",
      body: "需要帮助",
      created_at: "2026-09-27T08:00:00Z",
    },
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
  listPinnedMessages: () => Promise.resolve({ messages: [] }),
  listNotes: () => Promise.resolve([]),
  markConversationRead: () => Promise.resolve(),
  searchMessages: () => Promise.resolve([]),
  sendTyping: () => Promise.resolve(),
}));

vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => ({
    status: "authenticated",
    user: { id: 1, email: "ada@example.com", display_name: "Ada" },
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

import { InboxPage } from "@/pages/collaboration/InboxPage";

const renderInbox = () => {
  const view = render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter initialEntries={["/inbox"]}>
        <Routes>
          <Route path="/inbox" element={<InboxPage />} />
          <Route path="/conversations/:conversationId" element={<InboxPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, container: view.container };
};

describe("Inbox responsive regions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.matchMedia = vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
  });
  afterEach(cleanup);

  it("exposes one narrow-screen region at a time", async () => {
    const { container } = renderInbox();
    const list = container.querySelector(".inbox-region-list");
    const conversation = container.querySelector(".inbox-region-conversation");
    const context = container.querySelector(".inbox-region-context");

    expect(list).not.toHaveAttribute("hidden");
    expect(list).not.toHaveAttribute("inert");
    expect(conversation).toHaveAttribute("hidden");
    expect(conversation).toHaveAttribute("inert");
    expect(context).toHaveAttribute("hidden");
    expect(context).toHaveAttribute("inert");

    fireEvent.click(await screen.findByRole("link", { name: /客户支持/ }));
    expect(await screen.findByRole("heading", { name: "客户支持" })).toBeInTheDocument();
    expect(within(list as HTMLElement).getByText("客户支持")).toHaveAttribute(
      "title",
      "客户支持",
    );
    expect(within(list as HTMLElement).getByText("普通")).toBeInTheDocument();
    expect(screen.getByTitle("1 条未读")).toBeInTheDocument();
    expect(list).toHaveAttribute("hidden");
    expect(conversation).not.toHaveAttribute("hidden");
    expect(context).toHaveAttribute("hidden");
  });

  it("moves between conversation and context while restoring focus", async () => {
    const { container } = renderInbox();
    fireEvent.click(await screen.findByRole("link", { name: /客户支持/ }));
    const contextButton = await screen.findByRole("button", { name: "业务上下文" });
    fireEvent.click(contextButton);

    expect(await screen.findByRole("heading", { name: "会话状态" })).toBeInTheDocument();
    expect(container.querySelector(".inbox-region-context")).not.toHaveAttribute("hidden");

    fireEvent.click(screen.getByRole("button", { name: "关闭上下文" }));
    expect(container.querySelector(".inbox-region-conversation")).not.toHaveAttribute("hidden");
    await waitFor(() => expect(contextButton).toHaveFocus());

    fireEvent.click(screen.getByRole("button", { name: "返回会话列表" }));
    expect(container.querySelector(".inbox-region-list")).not.toHaveAttribute("hidden");
  });

  it("collapses and restores the desktop context panel", async () => {
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    const { container } = renderInbox();
    const context = container.querySelector(".inbox-region-context");

    fireEvent.click(await screen.findByRole("link", { name: /客户支持/ }));
    expect(context).not.toHaveAttribute("hidden");
    fireEvent.click(await screen.findByRole("button", { name: "业务上下文" }));
    expect(context).toHaveAttribute("hidden");

    fireEvent.click(screen.getByRole("button", { name: "业务上下文" }));
    expect(context).not.toHaveAttribute("hidden");
  });

  it("signals Agent processing through the realtime status region", async () => {
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    detail.workspace.agent_context = { meeting_transcription_status: "processing" };
    try {
      renderInbox();
      fireEvent.click(await screen.findByRole("link", { name: /客户支持/ }));

      const label = await screen.findByText("Agent 正在整理资料");
      expect(label.parentElement).toHaveClass("signal-track");
      expect(label.parentElement).toHaveAttribute("role", "status");
    } finally {
      detail.workspace.agent_context = {};
    }
  });
});
