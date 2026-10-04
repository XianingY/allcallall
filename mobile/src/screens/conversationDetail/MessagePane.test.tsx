import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";

import PrimaryButton from "../../components/PrimaryButton";
import type { MessageRecord } from "../../api/collaboration";
import MessagePane from "./MessagePane";
import MessageRow from "./MessageRow";
import type { MessagePaneProps } from "./types";

const MockMessageRow = MessageRow as unknown as jest.Mock;

const message: MessageRecord = {
  id: 1,
  organization_id: 1,
  conversation_id: 1,
  sender_id: 2,
  sender_email: "sender@example.com",
  sender_display_name: "Sender",
  type: "user",
  body: "Existing message",
  pinned: false,
  created_at: "2026-01-01T00:00:00Z",
};

jest.mock("../../components/PrimaryButton", () => {
  const React = require("react");
  const { Text } = require("react-native");
  return {
    __esModule: true,
    default: jest.fn(({ title }: { title: string }) => (
      <Text>{title}</Text>
    )),
  };
});

jest.mock("./MessageRow", () => ({
  // Deliberately unmemoized: this amplifies any FlatList re-render so the
  // composer test can catch keystrokes leaking back into message rows.
  __esModule: true,
  default: jest.fn(() => null),
}));

const renderMessagePane = (overrides: Partial<MessagePaneProps> = {}) => {
  const onSend = jest.fn().mockResolvedValue(true);
  const props: MessagePaneProps = {
    messages: [],
    loading: false,
    hasMorePrev: false,
    loadingMorePrev: false,
    sending: false,
    workflowLoading: false,
    currentUserId: 1,
    pendingAttachments: [],
    uploadingAttachment: false,
    onRefresh: jest.fn(),
    onLoadMorePrev: jest.fn(),
    onSend,
    onAskAgent: jest.fn().mockResolvedValue(true),
    onOpenTranscript: jest.fn(),
    onLongPressMessage: jest.fn(),
    onDownloadAttachment: jest.fn(),
    onPickAttachment: jest.fn(),
    onRemovePendingAttachment: jest.fn(),
    ...overrides,
  };

  return {
    onSend,
    ...render(<MessagePane {...props} />),
  };
};

describe("MessagePane", () => {
  it("passes the local draft to send and clears it after success", async () => {
    const { onSend } = renderMessagePane();
    const input = screen.getByPlaceholderText(
      "输入线程消息，或输入自定义 Agent goal",
    );

    fireEvent.changeText(input, "hello");
    fireEvent.press(screen.getByText("发送消息"));

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("hello"));
    await waitFor(() => expect(input.props.value).toBe(""));
  });

  it("passes the local draft to Run Agent and clears it after success", async () => {
    const onAskAgent = jest.fn().mockResolvedValue(true);
    renderMessagePane({ onAskAgent });
    const input = screen.getByPlaceholderText(
      "输入线程消息，或输入自定义 Agent goal",
    );

    fireEvent.changeText(input, "summarize");
    fireEvent.press(screen.getByText("Run Agent"));

    await waitFor(() => expect(onAskAgent).toHaveBeenCalledWith("summarize"));
    await waitFor(() => expect(input.props.value).toBe(""));
  });

  it("skips rendering when a parent re-renders with unchanged props", () => {
    const props: MessagePaneProps = {
      messages: [],
      loading: false,
      hasMorePrev: false,
      loadingMorePrev: false,
      sending: false,
      workflowLoading: false,
      currentUserId: 1,
      pendingAttachments: [],
      uploadingAttachment: false,
      onRefresh: jest.fn(),
      onLoadMorePrev: jest.fn(),
      onSend: jest.fn().mockResolvedValue(true),
      onAskAgent: jest.fn().mockResolvedValue(true),
      onOpenTranscript: jest.fn(),
      onLongPressMessage: jest.fn(),
      onDownloadAttachment: jest.fn(),
      onPickAttachment: jest.fn(),
      onRemovePendingAttachment: jest.fn(),
    };

    const renderPane = () => <MessagePane {...props} />;

    const { rerender } = render(renderPane());
    const initialRenderCount = (PrimaryButton as jest.Mock).mock.calls.length;
    expect(initialRenderCount).toBeGreaterThan(0);

    rerender(renderPane());

    expect((PrimaryButton as jest.Mock).mock.calls.length).toBe(
      initialRenderCount,
    );

    rerender(<MessagePane {...props} sending />);
    expect((PrimaryButton as jest.Mock).mock.calls.length).toBeGreaterThan(
      initialRenderCount,
    );
  });

  it("keeps the message list stable while the composer draft changes", () => {
    const { getByPlaceholderText } = renderMessagePane({
      messages: [message],
    });
    const initialRowRenderCount = MockMessageRow.mock.calls.length;
    expect(initialRowRenderCount).toBeGreaterThan(0);

    const input = getByPlaceholderText(
      "输入线程消息，或输入自定义 Agent goal",
    );
    fireEvent.changeText(input, "typing performance");

    expect(input.props.value).toBe("typing performance");
    expect(MockMessageRow.mock.calls.length).toBe(
      initialRowRenderCount,
    );
  });
});
