import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ConversationContextPanel } from "@/pages/collaboration/ConversationContextPanel";

const detail = {
  conversation: {
    id: 5,
    organization_id: 7,
    title: "退款处理",
    topic: "客户支持",
    status: "open",
    priority: "normal",
  },
  workspace: {
    agent_context: {
      meeting_transcript_segment_count: 3,
      knowledge_source_count: 4,
      pending_approval_count: 1,
    },
  },
};

const renderPanel = () => {
  const onNoteChange = vi.fn();
  const onUpdateConversation = vi.fn();
  const onAddNote = vi.fn();
  const onStartMeeting = vi.fn();
  const view = render(
    <MemoryRouter>
      <ConversationContextPanel
        selectedId={5}
        detail={{ data: detail, isLoading: false, isError: false } as never}
        notes={{ data: [] } as never}
        note=""
        onNoteChange={onNoteChange}
        onUpdateConversation={onUpdateConversation}
        onAddNote={onAddNote}
        onStartMeeting={onStartMeeting}
      />
    </MemoryRouter>,
  );
  return { ...view, onNoteChange, onUpdateConversation, onAddNote, onStartMeeting };
};

describe("ConversationContextPanel", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(cleanup);

  it("renders Agent context and updates conversation metadata", () => {
    const { onUpdateConversation } = renderPanel();

    expect(screen.getByText("会议转写")).toBeInTheDocument();
    expect(screen.getByText("知识来源")).toBeInTheDocument();
    expect(screen.getByText("待审批")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("状态"), { target: { value: "resolved" } });
    expect(onUpdateConversation).toHaveBeenLastCalledWith({ status: "resolved" });
  });

  it("supports internal notes and meeting actions", () => {
    const { rerender, onNoteChange, onUpdateConversation, onAddNote, onStartMeeting } = renderPanel();

    const field = screen.getByPlaceholderText("仅团队可见");
    fireEvent.change(field, { target: { value: "内部信息" } });
    expect(onNoteChange).toHaveBeenLastCalledWith("内部信息");

    rerender(
      <MemoryRouter>
        <ConversationContextPanel
          selectedId={5}
          detail={{ data: detail, isLoading: false, isError: false } as never}
          notes={{ data: [] } as never}
          note="内部信息"
          onNoteChange={onNoteChange}
          onUpdateConversation={onUpdateConversation}
          onAddNote={onAddNote}
          onStartMeeting={onStartMeeting}
        />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: /添加备注/ }));
    expect(onAddNote).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /从当前会话开会/ }));
    expect(onStartMeeting).toHaveBeenCalledTimes(1);
  });
});
