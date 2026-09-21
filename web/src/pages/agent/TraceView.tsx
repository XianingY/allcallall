import { Check, X } from "lucide-react";

import {
  getAgentRun,
  getWorkflow,
} from "@/api/agent";
import { compactJSON } from "@/pages/agent/AgentLabUtils";

export function TraceView({
  run,
  workflow,
  events,
  runtimeLabel,
  deciding = false,
  onAgentDecision,
}: {
  run?: Awaited<ReturnType<typeof getAgentRun>>;
  workflow?: Awaited<ReturnType<typeof getWorkflow>>;
  events: Array<Record<string, unknown>>;
  runtimeLabel?: string;
  deciding?: boolean;
  onAgentDecision?(callId: string, value: "approve" | "reject"): void;
}) {
  const rows = run
    ? [
        ...run.steps.map((item) => ({
          id: `s-${item.id}`,
          type: "step",
          name: item.name,
          status: item.status,
          detail: item.output_json,
        })),
        ...run.tool_calls.map((item) => ({
          id: `t-${item.id}`,
          type: "tool",
          name: item.tool_name,
          status: item.status,
          detail: item.input_json,
        })),
      ]
    : (workflow?.tasks.map((item) => ({
        id: `w-${item.id}`,
        type: item.role,
        name: item.name,
        status: item.status,
        detail: item.output_json,
      })) ?? []);

  return (
    <div>
      <header className="trace-header">
        <div>
          <h2>Trace</h2>
          {Boolean(run?.run.checkpoint_id) && (
            <small>
              {run?.run.checkpoint_id} · v{run?.run.checkpoint_version}
            </small>
          )}
        </div>
        <span>
          {runtimeLabel ? `${runtimeLabel} · ` : ""}
          {rows.length + events.length} events
        </span>
      </header>
      <div className="trace-list">
        {rows.map((row) => (
          <article key={row.id}>
            <span className={`trace-dot status-${row.status}`} />
            <div>
              <header>
                <strong>{row.name}</strong>
                <b>{row.type}</b>
                <em>{row.status}</em>
              </header>
              {row.detail && <pre>{compactJSON(row.detail)}</pre>}
              {row.type === "tool" &&
              run?.tool_calls.find(
                (item) =>
                  `t-${item.id}` === row.id && item.status === "pending",
              )
                ? (() => {
                    const call = run.tool_calls.find(
                      (item) => `t-${item.id}` === row.id,
                    )!;
                    return (
                      <div className="trace-approval">
                        <span>
                          MCP #{call.mcp_installation_id || "-"} · revision #
                          {call.mcp_revision_id || "-"} · checkpoint v
                          {call.approval_checkpoint_version}
                        </span>
                        <div>
                          <button
                            className="button-secondary"
                            disabled={deciding}
                            onClick={() =>
                              onAgentDecision?.(call.call_id, "reject")
                            }
                          >
                            <X size={15} />
                            拒绝
                          </button>
                          <button
                            className="button-primary"
                            disabled={deciding}
                            onClick={() =>
                              onAgentDecision?.(call.call_id, "approve")
                            }
                          >
                            <Check size={15} />
                            批准
                          </button>
                        </div>
                      </div>
                    );
                  })()
                : null}
            </div>
          </article>
        ))}
        {events.map((event, index) => (
          <article key={`event-${index}`}>
            <span className="trace-dot" />
            <div>
              <header>
                <strong>{String(event.name || event.event || "event")}</strong>
                <b>stream</b>
                <em>{String(event.status || "")}</em>
              </header>
              <pre>{compactJSON(JSON.stringify(event))}</pre>
            </div>
          </article>
        ))}
      </div>
    </div>
  );
}
