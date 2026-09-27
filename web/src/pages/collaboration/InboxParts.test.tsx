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

import { NewConversationDialog } from "@/pages/collaboration/InboxParts";

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
