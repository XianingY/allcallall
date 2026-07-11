import EventSource from "react-native-sse";
import type { components } from "@allcallall/api-types";

import { API_BASE_URL } from "../config";
import { createApiClient, getActiveOrganizationHeader } from "./client";

type APISchemas = components["schemas"];

export type CreateAgentRunRequest = APISchemas["CreateAgentRunRequest"];
export type AgentRunRecord = APISchemas["AgentRun"];
export type AgentStepRecord = APISchemas["AgentStep"];
export type AgentToolCallRecord = APISchemas["AgentToolCall"];
export type AgentTraceEventRecord = APISchemas["AgentTraceEvent"];
export type AgentRunEventRecord = APISchemas["AgentRunEvent"];
export type AgentCitation = APISchemas["AgentCitation"];
export type AgentRunResult = APISchemas["AgentRunResult"];

type AgentRunResultPayload = Pick<AgentRunResult, "run"> &
  Partial<Omit<AgentRunResult, "run">>;

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
    case "meeting_transcript":
      return `Meeting transcript #${sourceId}`;
    default:
      return `${sourceType || "context"} #${sourceId}`;
  }
};

const parseJSON = (raw?: string): Record<string, unknown> | null => {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
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

export const extractAgentCitations = (
  toolCalls: AgentToolCallRecord[],
): AgentCitation[] => {
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

const normalizeAgentRunResult = (
  payload: AgentRunResultPayload,
): AgentRunResult => {
  const toolCalls = payload.tool_calls ?? [];
  return {
    run: payload.run,
    steps: payload.steps ?? [],
    tool_calls: toolCalls,
    trace: payload.trace ?? [],
    citations: payload.citations?.length
      ? payload.citations
      : extractAgentCitations(toolCalls),
  };
};

export type AgentRunResponse = AgentRunResult;

export interface AgentRunEventsResponse {
  run_id: number;
  events: AgentRunEventRecord[];
}

export type CreateWorkflowRequest = APISchemas["CreateWorkflowRequest"];
export type WorkflowRunRecord = APISchemas["WorkflowRun"];
export type WorkflowTaskRecord = APISchemas["WorkflowTask"];
export type WorkflowAgentMessageRecord = APISchemas["WorkflowAgentMessage"];
export type ToolApprovalRecord = APISchemas["ToolApproval"];
export type WorkflowHistoryRecord = APISchemas["WorkflowHistory"];
export type WorkflowSignalRecord = APISchemas["WorkflowSignal"];
export type WorkflowTimerRecord = APISchemas["WorkflowTimer"];
export type WorkflowResult = APISchemas["WorkflowResult"];

export interface WorkflowListResponse {
  workflows: WorkflowResult[];
}

export interface ToolApprovalsResponse {
  approvals: ToolApprovalRecord[];
}

export interface WorkflowListParams {
  conversation_id?: number;
  status?: string;
  limit?: number;
}

export interface ToolApprovalListParams {
  conversation_id?: number;
  status?: string;
}

export const createAgentRun = async (
  token: string,
  data: CreateAgentRunRequest,
): Promise<AgentRunResponse> => {
  const client = createApiClient(token);
  const response = await client.post<AgentRunResultPayload>(
    "/agent/runs",
    data,
  );
  return normalizeAgentRunResult(response.data);
};

export const fetchAgentRun = async (
  token: string,
  runId: number,
): Promise<AgentRunResponse> => {
  const client = createApiClient(token);
  const response = await client.get<AgentRunResultPayload>(
    `/agent/runs/${runId}`,
  );
  return normalizeAgentRunResult(response.data);
};

export const fetchAgentRunEvents = async (
  token: string,
  runId: number,
): Promise<AgentRunEventRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<AgentRunEventsResponse>(
    `/agent/runs/${runId}/events`,
  );
  return response.data.events ?? [];
};

export const createAgentRunEventSource = (
  token: string,
  runId: number,
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
  data: CreateWorkflowRequest,
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.post<WorkflowResult>("/agent/workflows", data);
  return response.data;
};

export const fetchWorkflowRun = async (
  token: string,
  workflowId: number,
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.get<WorkflowResult>(
    `/agent/workflows/${workflowId}`,
  );
  return response.data;
};

export const listWorkflowRuns = async (
  token: string,
  params: number | WorkflowListParams = 25,
): Promise<WorkflowResult[]> => {
  const client = createApiClient(token);
  const queryParams =
    typeof params === "number" ? { limit: params } : { limit: 25, ...params };
  const response = await client.get<WorkflowListResponse>("/agent/workflows", {
    params: queryParams,
  });
  return response.data.workflows ?? [];
};

export const processWorkflowRun = async (
  token: string,
  workflowId: number,
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.post<WorkflowResult>(
    `/agent/workflows/${workflowId}/process`,
    {},
  );
  return response.data;
};

export const listToolApprovals = async (
  token: string,
  params?: string | ToolApprovalListParams,
): Promise<ToolApprovalRecord[]> => {
  const client = createApiClient(token);
  const queryParams =
    typeof params === "string" ? { status: params } : params;
  const response = await client.get<ToolApprovalsResponse>("/agent/approvals", {
    params: queryParams,
  });
  return response.data.approvals ?? [];
};

export const submitToolApprovalDecision = async (
  token: string,
  approvalId: number,
  decision: "approve" | "reject",
): Promise<WorkflowResult> => {
  const client = createApiClient(token);
  const response = await client.post<WorkflowResult>(
    `/agent/approvals/${approvalId}/decision`,
    {
      decision,
    },
  );
  return response.data;
};
