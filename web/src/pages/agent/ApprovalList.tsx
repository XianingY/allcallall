import { Check, X } from "lucide-react";
import { useState } from "react";

import { listApprovals } from "@/api/agent";
import { PageLoading } from "@/components/PageState";
import { compactJSON } from "@/pages/agent/AgentLabUtils";

export function ApprovalList({
  approvals,
  loading,
  decide,
}: {
  approvals: Awaited<ReturnType<typeof listApprovals>>;
  loading: boolean;
  decide(id: number, value: "approve" | "reject"): void;
}) {
  const [pendingOnly, setPendingOnly] = useState(true);
  const visible = pendingOnly
    ? approvals.filter((item) => item.status === "pending")
    : approvals;
  if (loading) return <PageLoading />;
  return (
    <div>
      <div className="approval-toolbar">
        <h2>工具审批</h2>
        <div className="segmented">
          <button
            className={pendingOnly ? "active" : ""}
            onClick={() => setPendingOnly(true)}
          >
            待处理
          </button>
          <button
            className={!pendingOnly ? "active" : ""}
            onClick={() => setPendingOnly(false)}
          >
            全部
          </button>
        </div>
      </div>
      <div className="approval-list">
        {visible.map((item) => (
          <article className="panel" key={item.id}>
            <header>
              <div>
                <strong>{item.tool_name}</strong>
                <span>Workflow #{item.workflow_run_id}</span>
                {item.mcp_revision_id && item.mcp_revision_id > 0 ? (
                  <span>
                    {item.mcp_installation_id
                      ? `MCP Installation #${item.mcp_installation_id}`
                      : "MCP"}{" "}
                    · Revision #{item.mcp_revision_id}
                  </span>
                ) : null}
              </div>
              <span className={`transcription-badge status-${item.status}`}>
                {item.status}
              </span>
            </header>
            <p className="approval-audit">
              Schema {item.tool_schema_version || "-"}
              {item.approval_request_id
                ? ` · Approval ${item.approval_request_id}`
                : ""}
              {item.approval_request_id
                ? ` · checkpoint v${item.approval_checkpoint_version}`
                : ""}
            </p>
            <pre>{compactJSON(item.input_json)}</pre>
            {item.error_message && (
              <p className="text-danger">{item.error_message}</p>
            )}
            {item.status === "pending" && (
              <footer>
                <button
                  className="button-secondary"
                  onClick={() => decide(item.id, "reject")}
                >
                  <X size={16} />
                  拒绝
                </button>
                <button
                  className="button-primary"
                  onClick={() => decide(item.id, "approve")}
                >
                  <Check size={16} />
                  批准
                </button>
              </footer>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}
