/**
 * Deal status values shared by the web and mobile clients.
 *
 * These mirror the backend's constants (models.DealStatusOpen/Won/Lost).
 * Sending anything else is rejected, so the two clients must not invent
 * their own spellings the way the follow-up page did with "completed".
 */

export const DEAL_STATUS = {
  open: "open",
  won: "won",
  lost: "lost",
} as const;

export type DealStatus = (typeof DEAL_STATUS)[keyof typeof DEAL_STATUS];

export const DEAL_STATUS_LABELS: Record<DealStatus, string> = {
  open: "进行中 / Open",
  won: "赢单 / Won",
  lost: "输单 / Lost",
};

/** Statuses in the order a deal moves through them. */
export const DEAL_STATUS_ORDER: DealStatus[] = [DEAL_STATUS.open, DEAL_STATUS.won, DEAL_STATUS.lost];

export function isDealStatus(value: string): value is DealStatus {
  const normalized = value.trim().toLowerCase();
  return normalized === DEAL_STATUS.open || normalized === DEAL_STATUS.won || normalized === DEAL_STATUS.lost;
}

/** A closed deal is decided; nothing further should be pushed on it. */
export function isDealClosed(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return normalized === DEAL_STATUS.won || normalized === DEAL_STATUS.lost;
}
