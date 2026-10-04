import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { sendTyping } from "@/api/collaboration";
import { TYPING_REFRESH_MS, TYPING_STOP_MS, useTypingSignal } from "@/pages/collaboration/useTypingSignal";

const sendTypingMock = vi.mocked(sendTyping);

vi.mock("@/api/collaboration", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/collaboration")>()),
  sendTyping: vi.fn(() => Promise.resolve()),
}));

describe("useTypingSignal", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("sends one start event and refreshes on an interval, not per keystroke", () => {
    const { result } = renderHook(() => useTypingSignal({ conversationId: 5 }));

    act(() => result.current.signalTyping("h"));
    act(() => vi.advanceTimersByTime(1000));
    act(() => result.current.signalTyping("he"));
    act(() => vi.advanceTimersByTime(1000));
    act(() => result.current.signalTyping("hel"));
    act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS - 2000));
    act(() => result.current.signalTyping("hell"));

    expect(sendTypingMock).toHaveBeenCalledTimes(2);
    expect(sendTypingMock).toHaveBeenNthCalledWith(1, 5, true);
    expect(sendTypingMock).toHaveBeenNthCalledWith(2, 5, true);
  });

  it("stops after idle and does not stop twice", () => {
    const { result } = renderHook(() => useTypingSignal({ conversationId: 5 }));

    act(() => result.current.signalTyping("hello"));
    act(() => vi.advanceTimersByTime(TYPING_STOP_MS));
    act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS));

    expect(sendTypingMock).toHaveBeenCalledTimes(2);
    expect(sendTypingMock).toHaveBeenLastCalledWith(5, false);
  });

  it("stops the previous conversation when the route changes", () => {
    const { result, rerender } = renderHook(
      ({ conversationId }: { conversationId: number | null }) => useTypingSignal({ conversationId }),
      { initialProps: { conversationId: 5 as number | null } },
    );

    act(() => result.current.signalTyping("hello"));
    rerender({ conversationId: null });
    act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS));

    expect(sendTypingMock).toHaveBeenCalledTimes(2);
    expect(sendTypingMock).toHaveBeenLastCalledWith(5, false);
  });

  it("stops once on unmount and invokes the caller cleanup first", () => {
    const onBeforeStop = vi.fn();
    const { result, unmount } = renderHook(() =>
      useTypingSignal({ conversationId: 5, onBeforeStop }),
    );

    act(() => result.current.signalTyping("hello"));
    unmount();
    act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS));

    expect(onBeforeStop).toHaveBeenCalledTimes(1);
    expect(sendTypingMock).toHaveBeenCalledTimes(2);
    expect(sendTypingMock).toHaveBeenLastCalledWith(5, false);
  });

  it("does not signal typing while disabled", () => {
    const { result } = renderHook(() =>
      useTypingSignal({ conversationId: 5, enabled: false }),
    );

    act(() => result.current.signalTyping("hello"));
    act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS));

    expect(sendTypingMock).not.toHaveBeenCalled();
  });
});
