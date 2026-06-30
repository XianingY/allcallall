import { Search } from "lucide-react";
import type { ReactNode } from "react";

export function MetricTile({ icon, label, value, hint }: { icon: ReactNode; label: string; value: string | number; hint?: string }) {
  return <article className="metric-tile">{icon}<span>{label}</span><strong>{value}</strong>{hint && <small>{hint}</small>}</article>;
}

export function AdminTableToolbar({ search, onSearch, filter, onFilter, filterLabel = "状态", options, resultCount }: { search: string; onSearch(value: string): void; filter?: string; onFilter?(value: string): void; filterLabel?: string; options?: Array<{ label: string; value: string }>; resultCount?: number }) {
  return <div className="admin-table-toolbar">
    <label className="search-field"><Search size={16} /><input aria-label="搜索" placeholder="搜索" value={search} onChange={(event) => onSearch(event.target.value)} /></label>
    {options?.length && onFilter ? <label className="admin-filter"><span>{filterLabel}</span><select className="field compact-field" value={filter ?? ""} onChange={(event) => onFilter(event.target.value)}>{options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label> : null}
    {typeof resultCount === "number" && <span className="admin-result-count">{resultCount} 条</span>}
  </div>;
}

export function StatusBadge({ value }: { value?: string | null }) {
  const status = value || "pending";
  return <span className={`transcription-badge status-${status}`}>{status}</span>;
}

export function EmptyPanel({ children }: { children: ReactNode }) {
  return <div className="inline-empty">{children}</div>;
}

export function AuditEventRow({ action, target, actor, createdAt }: { action: string; target: string; actor: string; createdAt: string }) {
  return <div className="data-row"><div><strong>{action}</strong><small>{target} · {actor}</small><small>{dateOnly(createdAt)}</small></div></div>;
}

function dateOnly(value?: string | null) {
  return value ? new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)) : "-";
}
