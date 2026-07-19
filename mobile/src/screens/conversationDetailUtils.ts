import type {
  CreateWorkflowRequest,
  ToolApprovalRecord,
  WorkflowResult,
} from "../api/agent";

type WorkflowPreset = NonNullable<CreateWorkflowRequest["preset"]>;

export const STATUS_OPTIONS = ["open", "pending", "resolved"] as const;
export const PRIORITY_OPTIONS = ["low", "normal", "high", "urgent"] as const;
export const MEETING_PRESETS: Array<{ key: WorkflowPreset; label: string }> = [
  { key: "meeting_brief", label: "Meeting Brief" },
  { key: "follow_up_planner", label: "Follow-up" },
  { key: "risk_review", label: "Risk Review" },
];
export const WORKFLOW_TASK_ORDER = [
  "collect_context",
  "decompose",
  "searcher",
  "summarizer",
  "risk_analyst",
  "merge",
  "propose_tools",
  "approval",
  "commit_result",
];

export const WORKFLOW_TERMINAL_STATUSES = new Set([
  "ready",
  "requires_action",
  "failed",
]);

export const parseJSONRecord = (raw?: string): Record<string, unknown> => {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
};

export const toTextList = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];

export const workflowStatusLabel = (
  workflow: WorkflowResult | null,
  pendingApprovalCount: number,
  loading: boolean,
) => {
  if (loading) return "运行中";
  const status = workflow?.workflow.status;
  if (!status) return "未运行";
  if (status === "requires_action" || pendingApprovalCount > 0) {
    return "等待审批";
  }
  if (status === "ready") return "已写回";
  if (status === "failed") return "失败";
  if (status === "running" || status === "pending") return "运行中";
  return status;
};

export const meetingTranscriptStatusLabel = (
  status: string | undefined,
  meetingCount: number,
  directCount: number,
  error?: string,
) => {
  if (meetingCount > 0) {
    return `${meetingCount} recording transcript segments ready`;
  }
  if (directCount > 0) {
    return `${directCount} final transcript segments`;
  }
  switch (status) {
    case "pending":
      return "Recording transcription queued";
    case "processing":
      return "Recording transcription processing";
    case "failed":
      return error
        ? `Recording transcription failed: ${error}`
        : "Recording transcription failed";
    case "skipped":
      return "Recording transcription skipped";
    default:
      return "No transcript yet; using notes and messages";
  }
};

export const citationModeLabel = (mode?: string) => {
  switch (mode) {
    case "hybrid_rrf":
      return "Hybrid";
    case "bm25":
      return "BM25";
    case "vector":
      return "Vector";
    case "sql_fallback":
      return "Fallback";
    default:
      return mode || "Context";
  }
};

export const readableToolName = (toolName: string) => {
  switch (toolName) {
    case "write_conversation_message":
      return "写入会话消息";
    case "create_follow_up_task":
      return "创建跟进任务";
    case "upsert_agent_memory":
      return "更新 Agent 记忆";
    case "delegate_task":
      return "委派子任务";
    default:
      return toolName;
  }
};

export const approvalPreview = (approval: ToolApprovalRecord) => {
  const input = parseJSONRecord(approval.input_json);
  const actionItems = toTextList(input.action_items);
  const riskFlags = toTextList(input.risk_flags);
  const lines: string[] = [];
  if (typeof input.summary === "string" && input.summary.trim()) {
    lines.push(`摘要：${input.summary.trim()}`);
  }
  if (typeof input.next_step === "string" && input.next_step.trim()) {
    lines.push(`下一步：${input.next_step.trim()}`);
  }
  if (typeof input.key === "string" && input.key.trim()) {
    lines.push(`Memory key：${input.key.trim()}`);
  }
  if (actionItems.length > 0) {
    lines.push(`行动项：${actionItems.join(" / ")}`);
  }
  if (riskFlags.length > 0) {
    lines.push(`风险：${riskFlags.join(" / ")}`);
  }
  if (lines.length === 0 && approval.input_json) {
    lines.push(approval.input_json);
  }
  return {
    title: readableToolName(approval.tool_name),
    lines,
  };
};
