/**
 * Follow-up task status values, shared by the web and mobile clients.
 *
 * These must match the backend's allowed set (models.FollowupTaskStatus*:
 * open, done, snoozed, cancelled). Sending anything else is rejected by
 * normalizeFollowUpTaskStatus and the update call fails.
 *
 * Web used to send "completed" and "pending", neither of which the backend
 * accepts: every complete/reopen press returned an error and the "completed"
 * column stayed empty forever. Mobile already used "done". Keeping the
 * values here stops the two clients from disagreeing again.
 */

export const FOLLOWUP_STATUS = {
  open: "open",
  done: "done",
  snoozed: "snoozed",
  cancelled: "cancelled",
} as const;

export type FollowUpStatus = (typeof FOLLOWUP_STATUS)[keyof typeof FOLLOWUP_STATUS];

/** Statuses that mean the task is still waiting on someone. */
export function isFollowUpPending(status: string): boolean {
  const normalized = status.trim().toLowerCase();
  return normalized !== FOLLOWUP_STATUS.done && normalized !== FOLLOWUP_STATUS.cancelled;
}

/** Statuses that mean the task is finished. */
export function isFollowUpCompleted(status: string): boolean {
  const normalized = status.trim().toLowerCase();
  return normalized === FOLLOWUP_STATUS.done || normalized === FOLLOWUP_STATUS.cancelled;
}
