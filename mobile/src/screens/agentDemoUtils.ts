import type {
  CreateWorkflowRequest,
  WorkflowTaskRecord,
} from "../api/agent";

export type LabTab = "knowledge" | "run" | "graph" | "approvals" | "eval";
export type WorkflowPreset = NonNullable<CreateWorkflowRequest["preset"]>;
export type ApprovalFilter = "pending" | "all";

export const tabs: Array<{ key: LabTab; label: string }> = [
  { key: "knowledge", label: "Knowledge" },
  { key: "run", label: "Run" },
  { key: "graph", label: "Graph" },
  { key: "approvals", label: "Approvals" },
  { key: "eval", label: "Eval" },
];

export const defaultGoal =
  "请基于会话消息、内部备注和知识库，给出客户当前诉求、风险点、下一步建议，并列出依据。";

export const workflowPresets: Array<{ key: WorkflowPreset; label: string }> = [
  { key: "meeting_brief", label: "Meeting Brief" },
  { key: "follow_up_planner", label: "Follow-up" },
  { key: "risk_review", label: "Risk Review" },
];

export const terminalRunStatuses = new Set([
  "ready",
  "failed",
  "requires_action",
]);

export const parseJSON = (raw?: string): unknown => {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
};

export const compact = (value: string, max = 180) => {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= max) return normalized;
  return `${normalized.slice(0, Math.max(0, max - 3))}...`;
};

export const formatTime = (value?: string | null) => {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString();
};

export const sourceTypeLabel = (sourceType: string) => {
  switch (sourceType) {
    case "meeting_transcript":
      return "Meeting transcript";
    case "transcript":
      return "Call captions";
    case "knowledge":
      return "Knowledge";
    case "message":
      return "Message";
    case "note":
      return "Internal note";
    case "memory":
      return "Agent memory";
    case "followup":
      return "Follow-up";
    case "contact_profile":
      return "Contact";
    default:
      return sourceType || "Context";
  }
};

export const jsonSummary = (raw?: string, max = 360) => {
  const parsed = parseJSON(raw);
  if (parsed === null || parsed === undefined) {
    return raw ? compact(raw, max) : "";
  }
  return compact(JSON.stringify(parsed), max);
};

export const taskOrder = (task: WorkflowTaskRecord) => {
  const index = [
    "collect_context",
    "decompose",
    "searcher",
    "summarizer",
    "risk_analyst",
    "merge",
    "propose_tools",
    "approval",
    "commit_result",
  ].indexOf(task.name);
  return index >= 0 ? index : 99;
};
