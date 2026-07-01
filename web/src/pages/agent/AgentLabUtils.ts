import type { WorkflowTask } from "@/api/agent";

export type AgenticRAGAttempt = {
  step?: number;
  query?: string;
  tool_name?: string;
  source_scope?: string;
  hit_count?: number;
  observation?: string;
  confidence?: number;
  source_types?: string[];
  strategy?: string;
  expanded_terms?: string[];
  graph_edge_ids?: string[];
};

export type AgenticRAGChunk = {
  chunk_id?: string;
  source_type?: string;
  source_id?: string;
  source_title?: string;
  title?: string;
  snippet?: string;
  retrieval_mode?: string;
  rerank_score?: number;
  rerank_reason?: string;
  final_rank?: number;
};

export type AgentLoopTrace = {
  role?: string;
  stop_reason?: string;
  completed?: boolean;
  spec?: { max_steps?: number; allowed_tools?: string[]; objective?: string };
  budget?: { max_steps?: number; used_steps?: number; read_tool_calls?: number };
};

export type AgenticRAGView = {
  plan?: {
    enabled?: boolean;
    max_steps?: number;
    min_confidence?: number;
    steps?: Array<Record<string, unknown>>;
  };
  attempts: AgenticRAGAttempt[];
  evidence?: {
    confidence?: number;
    source_types?: string[];
    selected_chunk_ids?: string[];
    rejected_count?: number;
    coverage?: number;
  };
  sufficiency?: {
    sufficient?: boolean;
    confidence?: number;
    reason?: string;
    missing_info?: string[];
  };
  harness?: {
    name?: string;
    graph_name?: string;
    prompt_version?: string;
    input_modalities?: string[];
  };
  route?: {
    route?: string;
    intent?: string;
    target_workflow?: string;
    confidence?: number;
    retrieval_strategy?: string;
  };
  critic?: {
    passed?: boolean;
    issues?: string[];
    citation_coverage?: number;
    budget_respected?: boolean;
    write_proposal_safe?: boolean;
    grounding_passed?: boolean;
  };
  loopTraces: AgentLoopTrace[];
  rawHits: AgenticRAGChunk[];
  rerankedHits: AgenticRAGChunk[];
  rejectedChunks: AgenticRAGChunk[];
  stopReason?: string;
  budget?: { max_steps?: number; used_steps?: number; read_tool_calls?: number };
};

export function extractAgenticRAG(tasks: WorkflowTask[]): AgenticRAGView | undefined {
  for (const task of tasks) {
    const payload = parseJSONRecord(task.output_json);
    if (!payload) continue;
    const plan = recordValue(payload.agentic_rag) ?? recordValue(payload.retrieval_plan);
    const attempts = arrayValue(payload.retrieval_attempts)
      .map((item) => recordValue(item))
      .filter((item): item is Record<string, unknown> => Boolean(item))
      .map(parseAttempt);
    const evidence = recordValue(payload.evidence_pack);
    const sufficiency = recordValue(payload.context_sufficiency);
    const harness = recordValue(payload.harness);
    const route = recordValue(payload.route_decision);
    const critic = recordValue(payload.critic_result);
    const loopTraces = arrayValue(payload.loop_traces)
      .map((item) => recordValue(item))
      .filter((item): item is Record<string, unknown> => Boolean(item))
      .map(parseLoopTrace);
    const rawHits = arrayValue(payload.raw_hits).map(parseChunk).filter(isChunk);
    const rerankedHits = arrayValue(payload.reranked_hits).map(parseChunk).filter(isChunk);
    const rejectedChunks = arrayValue(payload.rejected_chunks).map(parseChunk).filter(isChunk);
    if (
      plan ||
      attempts.length ||
      evidence ||
      sufficiency ||
      harness ||
      route ||
      critic ||
      loopTraces.length ||
      rawHits.length ||
      rerankedHits.length ||
      rejectedChunks.length
    ) {
      return {
        plan: plan
          ? {
              enabled: Boolean(plan.enabled),
              max_steps: numberValue(plan.max_steps),
              min_confidence: numberValue(plan.min_confidence),
              steps: arrayValue(plan.steps)
                .map((item) => recordValue(item))
                .filter((item): item is Record<string, unknown> => Boolean(item)),
            }
          : undefined,
        attempts,
        evidence: evidence
          ? {
              confidence: numberValue(evidence.confidence),
              rejected_count: numberValue(evidence.rejected_count),
              coverage: numberValue(evidence.coverage),
              selected_chunk_ids: stringArray(evidence.selected_chunk_ids),
              source_types: stringArray(evidence.source_types),
            }
          : undefined,
        sufficiency: sufficiency
          ? {
              sufficient: Boolean(sufficiency.sufficient),
              confidence: numberValue(sufficiency.confidence),
              reason: stringValue(sufficiency.reason),
              missing_info: stringArray(sufficiency.missing_info),
            }
          : undefined,
        harness: harness
          ? {
              name: stringValue(harness.name),
              graph_name: stringValue(harness.graph_name),
              prompt_version: stringValue(harness.prompt_version),
              input_modalities: stringArray(harness.input_modalities),
            }
          : undefined,
        route: route
          ? {
              route: stringValue(route.route),
              intent: stringValue(route.intent),
              target_workflow: stringValue(route.target_workflow),
              confidence: numberValue(route.confidence),
              retrieval_strategy: stringValue(route.retrieval_strategy),
            }
          : undefined,
        critic: critic
          ? {
              passed: booleanValue(critic.passed),
              issues: stringArray(critic.issues),
              citation_coverage: numberValue(critic.citation_coverage),
              budget_respected: booleanValue(critic.budget_respected),
              write_proposal_safe: booleanValue(critic.write_proposal_safe),
              grounding_passed: booleanValue(critic.grounding_passed),
            }
          : undefined,
        loopTraces,
        rawHits,
        rerankedHits,
        rejectedChunks,
        stopReason: stringValue(payload.stop_reason),
        budget: parseBudget(recordValue(payload.budget)),
      };
    }
  }
  return undefined;
}

export const compactJSON = (raw?: string) => {
  if (!raw) return "";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2).slice(0, 800);
  } catch {
    return raw.slice(0, 800);
  }
};

export const sourceLabel = (source: string) =>
  ({
    meeting_transcript: "会议录音转写",
    knowledge: "知识库",
    message: "会话消息",
    note: "内部备注",
    conversation: "会话上下文",
  })[source] ?? source;

export const workflowRuntimeLabel = (stateJSON?: string) => {
  if (!stateJSON) return "";
  try {
    const state = JSON.parse(stateJSON) as { runtime?: string };
    return state.runtime === "python_langgraph"
      ? "Python LangGraph Runtime"
      : state.runtime === "go"
        ? "Go Runtime"
        : "";
  } catch {
    return "";
  }
};

const parseAttempt = (item: Record<string, unknown>): AgenticRAGAttempt => ({
  step: numberValue(item.step),
  query: stringValue(item.query),
  tool_name: stringValue(item.tool_name),
  source_scope: stringValue(item.source_scope),
  hit_count: numberValue(item.hit_count),
  observation: stringValue(item.observation),
  confidence: numberValue(item.confidence),
  source_types: stringArray(item.source_types),
  strategy: stringValue(item.strategy),
  expanded_terms: stringArray(item.expanded_terms),
  graph_edge_ids: stringArray(item.graph_edge_ids),
});

const parseLoopTrace = (item: Record<string, unknown>): AgentLoopTrace => {
  const spec = recordValue(item.spec);
  return {
    role: stringValue(item.role),
    stop_reason: stringValue(item.stop_reason),
    completed: booleanValue(item.completed),
    spec: spec
      ? {
          max_steps: numberValue(spec.max_steps),
          allowed_tools: stringArray(spec.allowed_tools),
          objective: stringValue(spec.objective),
        }
      : undefined,
    budget: parseBudget(recordValue(item.budget)),
  };
};

const parseChunk = (item: unknown): AgenticRAGChunk | undefined => {
  const record = recordValue(item);
  if (!record) return undefined;
  return {
    chunk_id: stringValue(record.chunk_id),
    source_type: stringValue(record.source_type),
    source_id: stringValue(record.source_id),
    source_title: stringValue(record.source_title),
    title: stringValue(record.title),
    snippet: stringValue(record.snippet),
    retrieval_mode: stringValue(record.retrieval_mode),
    rerank_score: numberValue(record.rerank_score),
    rerank_reason: stringValue(record.rerank_reason),
    final_rank: numberValue(record.final_rank),
  };
};

const parseBudget = (item?: Record<string, unknown>) =>
  item
    ? {
        max_steps: numberValue(item.max_steps),
        used_steps: numberValue(item.used_steps),
        read_tool_calls: numberValue(item.read_tool_calls),
      }
    : undefined;

const parseJSONRecord = (raw?: string) => {
  if (!raw) return undefined;
  try {
    const value = JSON.parse(raw) as unknown;
    return recordValue(value);
  } catch {
    return undefined;
  }
};

const recordValue = (value: unknown): Record<string, unknown> | undefined =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;

const arrayValue = (value: unknown): unknown[] => (Array.isArray(value) ? value : []);
const stringValue = (value: unknown): string => (typeof value === "string" ? value : "");
const numberValue = (value: unknown): number | undefined =>
  typeof value === "number" ? value : undefined;
const booleanValue = (value: unknown): boolean | undefined =>
  typeof value === "boolean" ? value : undefined;
const stringArray = (value: unknown): string[] =>
  arrayValue(value)
    .map((item) => stringValue(item))
    .filter(Boolean);

const isChunk = (value: AgenticRAGChunk | undefined): value is AgenticRAGChunk => Boolean(value);
