/**
 * Pure guards that stop a second tap from firing the same create/unblock action
 * twice while the first request is still in flight.
 */
export const canSubmitInput = (value: string, pending: boolean): boolean =>
  !pending && value.trim().length > 0;

export const isPendingAction = (pendingId: number | null, targetId: number): boolean =>
  pendingId !== null && pendingId === targetId;
