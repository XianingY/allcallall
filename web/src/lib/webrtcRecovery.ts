/**
 * WebRTC connection recovery shared by the meeting engine and the 1:1 call
 * provider.
 *
 * Both used to only observe connection state: a dropped connection stayed
 * dropped until the user reloaded, and the call provider even showed a
 * "reconnecting" status it never acted on. Recovery lives here so the two
 * call sites cannot drift apart.
 */

export const ICE_RECOVERY_DEFAULTS = {
  /** Matches the mobile client: give up after two restart attempts. */
  maxAttempts: 2,
  /** Grace period before acting on "disconnected" - transient blips recover. */
  reconnectGraceMs: 1_500,
  /** Hard deadline for a restart to produce a connected state. */
  hardTimeoutMs: 10_000,
} as const;

export type RecoveryRole = "caller" | "callee";

export interface IceRecoveryHandlers {
  /** Only the caller may kick off an ICE restart. */
  role: RecoveryRole;
  /**
   * Renegotiate with iceRestart: true and deliver the new offer.
   * Omit where the signalling protocol cannot carry a re-offer (the 1:1 call
   * channel has no sdp.offer message type yet); recovery then only observes
   * state and reports an honest failure instead of hanging in "reconnecting".
   */
  restart?: () => Promise<unknown> | unknown;
  onRecovering?: () => void;
  onRecovered?: () => void;
  onFailed?: () => void;
  maxAttempts?: number;
}

export interface IceRecovery {
  /** Detach listeners and clear pending timers. */
  detach: () => void;
  /** Restart attempts used so far. */
  attempts: () => number;
}

/**
 * Decide whether a connection state warrants a new ICE restart attempt.
 * Exported separately so the decision stays unit-testable without a
 * PeerConnection.
 */
export function shouldRestartIce(params: {
  state: RTCPeerConnectionState | RTCIceConnectionState;
  role: RecoveryRole;
  attempts: number;
  maxAttempts?: number;
}): boolean {
  const { state, role, attempts } = params;
  const maxAttempts = params.maxAttempts ?? ICE_RECOVERY_DEFAULTS.maxAttempts;
  if (role !== "caller") return false;
  if (attempts >= maxAttempts) return false;
  return state === "failed" || state === "disconnected";
}

/**
 * Backoff between restart attempts: 1s, 2s, 4s ... capped at 8s.
 */
export function nextRecoveryDelayMs(attempts: number): number {
  if (attempts <= 0) return 1_000;
  return Math.min(8_000, 1_000 * 2 ** (attempts - 1));
}

export function attachIceRecovery(
  connection: RTCPeerConnection,
  handlers: IceRecoveryHandlers,
): IceRecovery {
  const maxAttempts = handlers.maxAttempts ?? ICE_RECOVERY_DEFAULTS.maxAttempts;
  let attempts = 0;
  let graceTimer: number | null = null;
  let hardTimer: number | null = null;

  const clearTimers = () => {
    if (graceTimer !== null) window.clearTimeout(graceTimer);
    if (hardTimer !== null) window.clearTimeout(hardTimer);
    graceTimer = null;
    hardTimer = null;
  };

  /** Terminal deadline: never leave the UI stuck in "reconnecting". */
  const armHardDeadline = () => {
    if (hardTimer !== null) return;
    hardTimer = window.setTimeout(() => {
      hardTimer = null;
      if (connection.connectionState !== "connected") handlers.onFailed?.();
    }, ICE_RECOVERY_DEFAULTS.hardTimeoutMs);
  };

  const attemptRestart = () => {
    if (attempts >= maxAttempts) {
      handlers.onFailed?.();
      return;
    }
    attempts += 1;
    handlers.onRecovering?.();
    void Promise.resolve(handlers.restart?.()).catch(() => {
      // A failed restart is retried by the next connection state change.
    });
    armHardDeadline();
  };

  const handleState = (state: RTCPeerConnectionState | RTCIceConnectionState) => {
    if (state === "connected") {
      clearTimers();
      attempts = 0;
      handlers.onRecovered?.();
      return;
    }
    if (state === "closed") {
      clearTimers();
      return;
    }
    if (state === "failed") {
      clearTimers();
      if (handlers.restart && attempts < maxAttempts) attemptRestart();
      else handlers.onFailed?.();
      return;
    }
    if (state === "disconnected" && graceTimer === null) {
      graceTimer = window.setTimeout(() => {
        graceTimer = null;
        if (connection.connectionState === "connected") return;
        // Without a re-offer channel there is nothing to send, so give the
        // browser's ICE a window to recover on its own before giving up.
        if (!handlers.restart) {
          armHardDeadline();
          return;
        }
        attemptRestart();
      }, ICE_RECOVERY_DEFAULTS.reconnectGraceMs);
    }
  };

  const onIceState = () => handleState(connection.iceConnectionState);
  const onConnectionState = () => handleState(connection.connectionState);

  connection.addEventListener("iceconnectionstatechange", onIceState);
  connection.addEventListener("connectionstatechange", onConnectionState);

  return {
    detach: () => {
      clearTimers();
      connection.removeEventListener("iceconnectionstatechange", onIceState);
      connection.removeEventListener("connectionstatechange", onConnectionState);
    },
    attempts: () => attempts,
  };
}
