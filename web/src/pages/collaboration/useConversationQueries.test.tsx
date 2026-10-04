import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { listConversations } from "@/api/collaboration";
import { useConversationQueries } from "@/pages/collaboration/useConversationQueries";

const listConversationsMock = vi.mocked(listConversations);

vi.mock("@/api/collaboration", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/collaboration")>()),
  listConversations: vi.fn(async () => ({
    conversations: [
      {
        id: 1,
        organization_id: 7,
        type: "channel",
        title: "Alpha",
        topic: "Alpha topic",
        status: "open",
        priority: "normal",
        unread_count: 0,
        last_message_preview: "Alpha preview",
        last_message_at: "2026-01-01T00:00:00Z",
      },
    ],
    pagination: { total: 1, limit: 50, offset: 0, has_more: false },
  })),
}));

function createWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useConversationQueries", () => {
  it("defers the visible list until after the input render", async () => {
    const wrapper = createWrapper();
    const visibleCounts: number[] = [];

    function Probe({ keyword }: { keyword: string }) {
      const queries = useConversationQueries({
          organizationId: 7,
          conversationId: null,
          filter: { status: "", keyword, unreadOnly: false, messageQuery: "" },
      });
      visibleCounts.push(queries.visibleConversations.length);
      return null;
    }

    const { rerender } = render(<Probe keyword="" />, { wrapper });

    await waitFor(() => expect(visibleCounts).toContain(1));
    expect(listConversationsMock).toHaveBeenCalledTimes(1);

    const beforeKeywordChange = visibleCounts.length;
    rerender(<Probe keyword="does-not-match" />);

    await waitFor(() => expect(visibleCounts.slice(beforeKeywordChange)).toEqual([1, 0]));
  });
});
