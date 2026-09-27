/**
 * Pure view-state helper for list/detail screens that used to conflate a load
 * failure with a genuinely empty list ("暂无会话记录" after a network error).
 *
 * `error` always wins so the screen renders the persistent error block with a
 * retry button instead of an empty state; screens that still hold stale items
 * keep rendering them alongside the error banner.
 */
export type LoadView = "loading" | "error" | "content" | "empty";

export interface LoadViewState {
  loading: boolean;
  error: string | null;
  itemCount: number;
}

export const resolveLoadView = ({ loading, error, itemCount }: LoadViewState): LoadView => {
  if (error !== null) {
    return "error";
  }
  if (itemCount > 0) {
    return "content";
  }
  if (loading) {
    return "loading";
  }
  return "empty";
};
