/**
 * Holds a deep link that arrived before the app could act on it.
 *
 * A meeting or conversation link opened from another app is delivered while
 * the navigator is still starting, and often before the user is logged in.
 * The old handling ran the link through the navigator once and dropped it if
 * the ref was not ready, so tapping a link on a cold start (or while logged
 * out) did nothing, and the link was lost for good once the app was ready.
 *
 * Who does what:
 * - App.tsx stores the intent when it cannot navigate yet.
 * - AppNavigator consumes it once there is a token and the navigator is
 *   attached, because only then do the target routes exist in the stack.
 */

export type PendingIntent =
  | { kind: "room"; roomId: number }
  | { kind: "conversation"; conversationId: number }
  | { kind: "invitation"; code: string };

let pending: PendingIntent | null = null;

export function setPendingIntent(intent: PendingIntent | null): void {
  pending = intent;
}

export function takePendingIntent(): PendingIntent | null {
  const intent = pending;
  pending = null;
  return intent;
}

export function peekPendingIntent(): PendingIntent | null {
  return pending;
}
