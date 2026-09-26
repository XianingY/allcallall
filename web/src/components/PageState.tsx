import type { ReactNode } from "react";

import { AlertCircle, Inbox, LoaderCircle } from "lucide-react";

export function PageLoading({ label = "正在加载" }: { label?: string }) {
  return <div className="page-state"><LoaderCircle className="animate-spin" size={20} />{label}</div>;
}

/**
 * Empty state. Without this, a list with no rows renders as a bare container
 * and "no data yet" is indistinguishable from "the request failed" or "the
 * page is broken" - which is exactly what a new user sees on their first
 * login, before they have any conversations, meetings or contacts.
 */
export function PageEmpty({ label, hint, action }: { label: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="page-state">
      <Inbox size={20} />
      <span>{label}</span>
      {hint ? <span className="page-state-hint">{hint}</span> : null}
      {action}
    </div>
  );
}

export function PageError({ error, retry }: { error: unknown; retry?: () => void }) {
  return <div className="page-state text-danger"><AlertCircle size={20} /><span>{error instanceof Error ? error.message : "加载失败"}</span>{retry && <button className="button-secondary" onClick={retry}>重试</button>}</div>;
}

