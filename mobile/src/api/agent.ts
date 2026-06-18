import EventSource from "react-native-sse";
import { API_BASE_URL } from "../config";
import { createApiClient, getActiveOrganizationHeader } from "./client";

export interface CreateAgentRunRequest {
  conversation_id: number;
  goal?: string;
}

export interface AgentRunRecord {
  id: number;
  organization_id: number;
  user_id: number;
  conversation_id: number;
  idempotency_key?: string;
  request_id?: string;
  source: string;
  status: string;
  goal: string;
  summary: string;
  action_items: string[];
  next_step: string;
  risk_flags: string[];
  error_message?: string;
  attempts: number;
  lease_until?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface AgentStepRecord {
  id: number;
  run_id: number;
  name: string;
  status: string;
  input_json?: string;
  output_json?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentToolCallRecord {
  id: number;
  run_id: number;
  step_id?: number | null;
  tool_name: string;
  status: string;
  input_json?: string;
  output_json?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentTraceEventRecord {
  type: string;
  name: string;
  status: string;
  ref_id?: number;
  at: string;
  metadata?: Record<string, unknown>;
}

export interface AgentRunEventRecord {
  sequence: number;
  event: string;
  status: string;
  ref_type: string;
  ref_id?: number;
  name?: string;
  at: string;
  metadata?: Record<string, unknown>;
}

export interface AgentCitation {
  chunk_id?: string;
  source_type: string;
  source_id: string;
  source_title?: string;
  title: string;
  snippet: string;
  origin_type?: string;
  origin_url?: string;
  conversation_id?: number;
  knowledge_source_id?: number;
  version?: number;
  retrieval_mode?: "vector" | "sql_fallback" | string;
  score: number;
  created_at?: string;
}

export interface AgentRunResult {
  run: AgentRunRecord;
  steps: AgentStepRecord[];
  tool_calls: AgentToolCallRecord[];
  trace: AgentTraceEventRecord[];
  citations: AgentCitation[];
}

interface AgentRunResultPayload {
  run: AgentRunRecord;
  steps?: AgentStepRecord[];
  tool_calls?: AgentToolCallRecord[];
  trace?: AgentTraceEventRecord[];
  citations?: AgentCitation[];
}

const citationTitle = (sourceType: string, sourceId: string) => {
  switch (sourceType) {
    case "message":
      return `Message #${sourceId}`;
    case "note":
      return `Internal note #${sourceId}`;
    case "memory":
      return `Agent memory #${sourceId}`;
    case "followup":
      return `Follow-up #${sourceId}`;
    case "contact_profile":
      return `Contact profile #${sourceId}`;
    case "transcript":
      return `Transcript #${sourceId}`;
    default:
      return `${sourceType || "context"} #${sourceId}`;
  }
};

const parseJSON = (raw?: string): Record<string, unknown> | null => {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : null;
  } catch {
    return null;
  }
};

const toNumber = (value: unknown, fallback = 0) => {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
  }
  return fallback;
};

export const extractAgentCitations = (toolCalls: AgentToolCallRecord[]): AgentCitation[] => {
  const seen = new Set<string>();
  const citations: AgentCitation[] = [];

  for (const call of toolCalls) {
    if (call.tool_name !== "query_context_chunks") {
      continue;
    }
    const output = parseJSON(call.output_json);
    const chunks = Array.isArray(output?.chunks) ? output.chunks : [];
    for (const chunk of chunks) {
      if (!chunk || typeof chunk !== "object") {
        continue;
      }
      const item = chunk as Record<string, unknown>;
      const sourceType = String(item.source_type ?? "context");
      const sourceId = String(item.source_id ?? item.chunk_id ?? "");
      const snippet = String(item.snippet ?? "").trim();
      if (!sourceId || !snippet) {
        continue;
      }
      const key = `${sourceType}:${sourceId}`;
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      citations.push({
        source_type: sourceType,
        source_id: sourceId,
        title: citationTitle(sourceType, sourceId),
        snippet,
        score: toNumber(item.score),
      });
    }
  }

  return citations;
};

const normalizeAgentRunResult = (payload: AgentRunResultPayload): AgentRunResult => {
  const toolCalls = payload.tool_calls ?? [];
  return {
    run: payload.run,
    steps: payload.steps ?? [],
    tool_calls: toolCalls,
    trace: payload.trace ?? [],
    citations: payload.citations?.length ? payload.citations : extractAgentCitations(toolCalls),
  };
};

export type AgentRunResponse = AgentRunResult;

export interface AgentRunEventsResponse {
  run_id: number;
  events: AgentRunEventRecord[];
}

export interface CreateWorkflowRequest {
  conversation_id: number;
  goal?: string;
}

export interface WorkflowRunRecord {
  id: number;
  organization_id: number;
  user_id: number;
  conversation_id: number;
  agent_run_id?: number | null;
  idempotency_key?: string;
  request_id?: string;
  status: string;
  goal: string;
  summary: string;
  action_items: string[];
  next_step: string;
  risk_flags: string[];
  error_message?: string;
  attempts: number;
  lease_until?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTaskRecord {
  id: number;
  workflow_run_id: number;
  organization_id: number;
  name: string;
  role: string;
  status: string;
  depends_on_json?: string;
  input_json?: string;
  output_json?: string;
  error_message?: string;
  attempts: number;
  lease_until?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkflowAgentMessageRecord {
  id: number;
  workflow_run_id: number;
  task_id?: number | null;
  organization_id: number;
  from_role: string;
  to_role: string;
  message_type: string;
  content_json: string;
  correlation_id: string;
  created_at: string;
}

export interface ToolApprovalRecord {
  id: number;
  workflow_run_id: number;
  task_id: number;
  organization_id: number;
  tool_call_id: string;
  tool_name: string;
  status: string;
  input_json?: string;
  output_json?: string;
  error_message?: string;
  requested_by: number;
  decided_by?: number | null;
  decision?: string;
  requested_at: string;
  decided_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkflowResult {
  workflow: WorkflowRunRecord;
  tasks: WorkflowTaskRecord[];
  messages: WorkflowAgentMessageRecord[];
  approvals: ToolApprovalRecord[];
  citations: AgentCitation[];
  actionItems?: string[];
  riskFlags?: string[];
}

export interface WorkflowListResponse {
  workflows: WorkflowResult[];
}

export interface ToolApprovalsResponse {
  approvals: ToolApprovalRecord[];
}

export const createAgentRun = async (
  token: string,
  data: CreateAgentRunRequest
): Promise<AgentRunResponse> => {
  const client = createApiClient(token);
  const response = await client.post<AgentRunResultPayload>("/agent/runs", data);
  return normalizeAgentRunResult(response.data);
};

export const fetchAgentRun = async (
  token: string,
  runId: number
): Promise<AgentRunResponse> => {
  const client = createApiClient(token);
  const response = await client.get<AgentRunResultPayload>(`/agent/runs/${runId}`);
  return normalizeAgentRunResult(response.data);
};

export const fetchAgentRunEvents = async (
  token: string,
  runId: number
): Promise<AgentRunEventRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<AgentRunEventsResponse>(`/agent/runs/${runId}/events`);
  return response.data.events ?? [];
};

export const createAgentRunEventSource = (
  token: string,
  runId: number
): EventSource => {
  const url = `${API_BASE_URL}/agent/runs/${runId}/events/stream`;
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
  };
  const orgId = getActiveOrganizationHeader();
  if (orgId) {
    headers["X-Organization-ID"] = String(orgId);
  }
  return new EventSource(url, {
    headers,
  });
};

export const createWorkflowRun = async (
  token: string,
  data: CreateWorkflowRequest
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.post<WorkflowResult>("/agent/workflows", data);
  return response.data;
};

export const fetchWorkflowRun = async (
  token: string,
  workflowId: number
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.get<WorkflowResult>(`/agent/workflows/${workflowId}`);
  return response.data;
};

export const listWorkflowRuns = async (
  token: string,
  limit = 25
): Promise<WorkflowResult[]> => {
  const client = createApiClient(token);
  const response = await client.get<WorkflowListResponse>("/agent/workflows", {
    params: { limit },
  });
  return response.data.workflows ?? [];
};

export const processWorkflowRun = async (
  token: string,
  workflowId: number
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.post<WorkflowResult>(`/agent/workflows/${workflowId}/process`, {});
  return response.data;
};

export const listToolApprovals = async (
  token: string,
  status?: string
): Promise<ToolApprovalRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<ToolApprovalsResponse>("/agent/approvals", {
    params: status ? { status } : undefined,
  });
  return response.data.approvals ?? [];
};

export const submitToolApprovalDecision = async (
  token: string,
  approvalId: number,
  decision: "approve" | "reject"
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.post<WorkflowResult>(`/agent/approvals/${approvalId}/decision`, {
    decision,
  });
  return response.data;
};
