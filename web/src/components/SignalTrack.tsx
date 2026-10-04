import clsx from "clsx";

export interface SignalTrackProps {
  state: "connected" | "connecting" | "error";
  activity?: "idle" | "incoming" | "agent" | "meeting";
  label: string;
  compact?: boolean;
}

export function SignalTrack({
  state,
  activity = "idle",
  label,
  compact = false,
}: SignalTrackProps) {
  const active = state === "connected" && activity !== "idle";

  return (
    <div
      className={clsx(
        "signal-track",
        `signal-track-${state}`,
        active && "signal-track-active",
        compact && "signal-track-compact",
      )}
      role="status"
      aria-live="polite"
    >
      <span aria-hidden="true" className="signal-track-dot" />
      <span className="signal-track-label">{label}</span>
      {active ? <span aria-hidden="true" className="signal-track-pulse" /> : null}
    </div>
  );
}
