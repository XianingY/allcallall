import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";

import PrimaryButton from "../../components/PrimaryButton";
import MessagePane from "./MessagePane";
import type { MessagePaneProps } from "./types";

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
});
