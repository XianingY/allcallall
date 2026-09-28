/**
 * Date and byte formatting shared by the web and mobile clients.
 *
 * Both sides used to call toLocaleString()/Intl directly, so the same message
 * rendered as "9月28日 14:30" on web and "2026/9/28 14:30:00" on a phone, and
 * both shifted with the device's language and 12/24-hour setting. That makes
 * screenshots irreproducible and support reports impossible to match up.
 *
 * These produce a fixed, locale-independent shape so the three clients agree.
 */

const pad = (value: number) => String(value).padStart(2, "0");

function toDate(value?: string | number | Date | null): Date | null {
  if (value === null || value === undefined || value === "") return null;
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** Full timestamp: 2026-09-28 14:30. Empty string when unusable. */
export function formatDateTime(value?: string | number | Date | null): string {
  const date = toDate(value);
  if (!date) return "";
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** Date only: 2026-09-28. */
export function formatDate(value?: string | number | Date | null): string {
  const date = toDate(value);
  if (!date) return "";
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** Clock time only: 14:30. */
export function formatClock(value?: string | number | Date | null): string {
  const date = toDate(value);
  if (!date) return "";
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/**
 * Compact form for lists and message rows: the time when it is today,
 * month/day plus time within the current year, and the full date otherwise.
 * `now` is injectable so it can be tested and so memoised rows do not drift.
 */
export function formatShortDateTime(
  value?: string | number | Date | null,
  now: Date = new Date(),
): string {
  const date = toDate(value);
  if (!date) return "";

  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();

  if (sameDay) return formatClock(date);
  if (date.getFullYear() === now.getFullYear()) {
    return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${formatClock(date)}`;
  }
  return formatDateTime(date);
}

/** Human-readable byte size, for uploads and attachments. */
export function formatBytes(value?: number | null): string {
  if (!value || value <= 0) return "0 B";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}
