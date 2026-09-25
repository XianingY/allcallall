import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  attachIceRecovery,
  ICE_RECOVERY_DEFAULTS,
  nextRecoveryDelayMs,
  shouldRestartIce,
} from "@/lib/webrtcRecovery";

/** Minimal RTCPeerConnection stand-in: only what the recovery attaches to. */
class FakeConnection {
  connectionState: RTCPeerConnectionState = "new";
  iceConnectionState: RTCIceConnectionState = "new";
  private listeners = new Map<string, Set<() => void>>();

  addEventListener(type: string, fn: () => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(fn);
  }

  removeEventListener(type: string, fn: () => void) {
    this.listeners.get(type)?.delete(fn);
  }

  emit(type: string) {
    for (const fn of this.listeners.get(type) ?? []) fn();
  }

  become(state: RTCPeerConnectionState) {
    this.connectionState = state;
    this.emit("connectionstatechange");
  }

  asPeerConnection(): RTCPeerConnection {
    return this as unknown as RTCPeerConnection;
  }
}

describe("shouldRestartIce", () => {
  it("restarts for the caller on failed or disconnected", () => {
    expect(shouldRestartIce({ state: "failed", role: "caller", attempts: 0 })).toBe(true);
    expect(shouldRestartIce({ state: "disconnected", role: "caller", attempts: 0 })).toBe(true);
  });

  it("never restarts for the callee", () => {
    expect(shouldRestartIce({ state: "failed", role: "callee", attempts: 0 })).toBe(false);
  });

  it("stops after the attempt budget is spent", () => {
    expect(
      shouldRestartIce({ state: "failed", role: "caller", attempts: ICE_RECOVERY_DEFAULTS.maxAttempts }),
    ).toBe(false);
  });

  it("ignores healthy and in-progress states", () => {
    for (const state of ["connected", "connecting", "new"] as RTCPeerConnectionState[]) {
      expect(shouldRestartIce({ state, role: "caller", attempts: 0 })).toBe(false);
    }
  });
});

describe("nextRecoveryDelayMs", () => {
  it("backs off and stays capped", () => {
    expect(nextRecoveryDelayMs(0)).toBe(1_000);
    expect(nextRecoveryDelayMs(1)).toBe(1_000);
    expect(nextRecoveryDelayMs(2)).toBe(2_000);
    expect(nextRecoveryDelayMs(3)).toBe(4_000);
    expect(nextRecoveryDelayMs(4)).toBe(8_000);
    expect(nextRecoveryDelayMs(20)).toBe(8_000);
  });
});

describe("attachIceRecovery", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("restarts when the connection fails", () => {
    const pc = new FakeConnection();
    const restart = vi.fn();
    const onRecovering = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart, onRecovering });

    pc.become("failed");

    expect(restart).toHaveBeenCalledTimes(1);
    expect(onRecovering).toHaveBeenCalledTimes(1);
  });

  it("waits out a transient disconnect before restarting", () => {
    const pc = new FakeConnection();
    const restart = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart });

    pc.become("disconnected");
    expect(restart).not.toHaveBeenCalled();

    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.reconnectGraceMs + 10);
    expect(restart).toHaveBeenCalledTimes(1);
  });

  it("does not restart if the disconnect heals inside the grace period", () => {
    const pc = new FakeConnection();
    const restart = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart });

    pc.become("disconnected");
    pc.become("connected");
    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.reconnectGraceMs + 10);

    expect(restart).not.toHaveBeenCalled();
  });

  it("resets the attempt budget once reconnected", () => {
    const pc = new FakeConnection();
    const restart = vi.fn();
    const recovery = attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart });

    pc.become("failed");
    expect(recovery.attempts()).toBe(1);
    pc.become("connected");
    expect(recovery.attempts()).toBe(0);

    pc.become("failed");
    expect(recovery.attempts()).toBe(1);
  });

  it("gives up after the attempt budget and reports failure", () => {
    const pc = new FakeConnection();
    const restart = vi.fn();
    const onFailed = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart, onFailed, maxAttempts: 2 });

    pc.become("failed");
    pc.become("failed");
    pc.become("failed");

    expect(restart).toHaveBeenCalledTimes(2);
    expect(onFailed).toHaveBeenCalledTimes(1);
  });

  it("reports a terminal failure when a restart never lands", () => {
    const pc = new FakeConnection();
    const onFailed = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart: vi.fn(), onFailed });

    pc.become("failed");
    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.hardTimeoutMs + 10);

    expect(onFailed).toHaveBeenCalledTimes(1);
  });

  it("never leaves the UI hanging when no re-offer channel exists", () => {
    const pc = new FakeConnection();
    const onFailed = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", onFailed });

    // No restart handler: let ICE heal, but report honestly at the deadline.
    pc.become("disconnected");
    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.reconnectGraceMs + 10);
    expect(onFailed).not.toHaveBeenCalled();

    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.hardTimeoutMs + 10);
    expect(onFailed).toHaveBeenCalledTimes(1);
  });

  it("stops observing after detach", () => {
    const pc = new FakeConnection();
    const restart = vi.fn();
    const recovery = attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart });

    recovery.detach();
    pc.become("failed");
    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.hardTimeoutMs + 10);

    expect(restart).not.toHaveBeenCalled();
  });

  it("does not fire the deadline after the connection recovers", () => {
    const pc = new FakeConnection();
    const onFailed = vi.fn();
    attachIceRecovery(pc.asPeerConnection(), { role: "caller", restart: vi.fn(), onFailed });

    pc.become("failed");
    pc.become("connected");
    vi.advanceTimersByTime(ICE_RECOVERY_DEFAULTS.hardTimeoutMs + 10);

    expect(onFailed).not.toHaveBeenCalled();
  });
});
