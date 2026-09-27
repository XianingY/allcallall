import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useResendCooldown } from "@/hooks/useResendCooldown";

// The hook re-arms its timer from an effect, so each tick has to commit before
// the next one is scheduled - advancing a long window in one call would only
// run the first timer.
const tick = (ms: number) => {
  for (let elapsed = 0; elapsed < ms; elapsed += 1000) {
    act(() => vi.advanceTimersByTime(1000));
  }
};

describe("useResendCooldown", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("counts down to zero after start", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResendCooldown(60));

    expect(result.current.coolingDown).toBe(false);
    expect(result.current.remaining).toBe(0);

    act(() => result.current.start());
    expect(result.current.coolingDown).toBe(true);
    expect(result.current.remaining).toBe(60);

    tick(3000);
    expect(result.current.remaining).toBe(57);

    tick(57000);
    expect(result.current.remaining).toBe(0);
    expect(result.current.coolingDown).toBe(false);
  });

  it("restarts the full window when started again", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResendCooldown(60));

    act(() => result.current.start());
    tick(10000);
    expect(result.current.remaining).toBe(50);

    act(() => result.current.start());
    expect(result.current.remaining).toBe(60);
  });
});
