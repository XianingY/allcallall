import type { WorkflowTask } from "@/api/agent";

export type AgenticRAGAttempt = { step?: number; query?: string; tool_name?: string; source_scope?: string; hit_count?: number; observation?: string; confidence?: number; source_types?: string[] };
export type AgenticRAGView = { plan?: { enabled?: boolean; max_steps?: number; min_confidence?: number; steps?: Array<Record<string, unknown>> }; attempts: AgenticRAGAttempt[]; evidence?: { confidence?: number; source_types?: string[]; selected_chunk_ids?: string[]; rejected_count?: number }; sufficiency?: { sufficient?: boolean; confidence?: number; reason?: string; missing_info?: string[] } };

export function extractAgenticRAG(tasks: WorkflowTask[]): AgenticRAGView | undefined {
  for (const task of tasks) {
    const payload = parseJSONRecord(task.output_json);
    if (!payload) continue;
    const plan = recordValue(payload.agentic_rag);
    const attempts = arrayValue(payload.retrieval_attempts).map((item) => recordValue(item)).filter((item): item is Record<string, unknown> => Boolean(item)).map((item) => ({
      step: numberValue(item.step),
      query: stringValue(item.query),
      tool_name: stringValue(item.tool_name),
      source_scope: stringValue(item.source_scope),
      hit_count: numberValue(item.hit_count),
      observation: stringValue(item.observation),
      confidence: numberValue(item.confidence),
      source_types: arrayValue(item.source_types).map((source) => stringValue(source)).filter(Boolean),
    }));
    const evidence = recordValue(payload.evidence_pack);
    const sufficiency = recordValue(payload.context_sufficiency);
    if (plan || attempts.length || evidence || sufficiency) {
      return {
        plan: plan ? { enabled: Boolean(plan.enabled), max_steps: numberValue(plan.max_steps), min_confidence: numberValue(plan.min_confidence), steps: arrayValue(plan.steps).map((item) => recordValue(item)).filter((item): item is Record<string, unknown> => Boolean(item)) } : undefined,
        attempts,
        evidence: evidence ? { confidence: numberValue(evidence.confidence), rejected_count: numberValue(evidence.rejected_count), selected_chunk_ids: arrayValue(evidence.selected_chunk_ids).map((item) => stringValue(item)).filter(Boolean), source_types: arrayValue(evidence.source_types).map((item) => stringValue(item)).filter(Boolean) } : undefined,
        sufficiency: sufficiency ? { sufficient: Boolean(sufficiency.sufficient), confidence: numberValue(sufficiency.confidence), reason: stringValue(sufficiency.reason), missing_info: arrayValue(sufficiency.missing_info).map((item) => stringValue(item)).filter(Boolean) } : undefined,
      };
    }
  }
  return undefined;
}

export const compactJSON = (raw?: string) => {
  if (!raw) return "";
  try { return JSON.stringify(JSON.parse(raw), null, 2).slice(0, 800); } catch { return raw.slice(0, 800); }
};

export const sourceLabel = (source: string) => ({ meeting_transcript: "会议录音转写", knowledge: "知识库", message: "会话消息", note: "内部备注", conversation: "会话上下文" }[source] ?? source);

export const workflowRuntimeLabel = (stateJSON?: string) => {
  if (!stateJSON) return "";
  try {
    const state = JSON.parse(stateJSON) as { runtime?: string };
    return state.runtime === "python_langgraph" ? "Python LangGraph Runtime" : state.runtime === "go" ? "Go Runtime" : "";
  } catch {
    return "";
  }
};

const parseJSONRecord = (raw?: string) => {
  if (!raw) return undefined;
  try {
    const value = JSON.parse(raw) as unknown;
    return recordValue(value);
  } catch {
    return undefined;
  }
};
const recordValue = (value: unknown): Record<string, unknown> | undefined => value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
const arrayValue = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const stringValue = (value: unknown): string => typeof value === "string" ? value : "";
const numberValue = (value: unknown): number | undefined => typeof value === "number" ? value : undefined;
