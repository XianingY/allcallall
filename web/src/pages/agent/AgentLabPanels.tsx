import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import { Check, X } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import {
  getAgentRun,
  getWorkflow,
  listApprovals,
  type AgentCitation,
  type WorkflowTask,
} from "@/api/agent";
import { PageLoading } from "@/components/PageState";
import {
  compactJSON,
  sourceLabel,
  type AgenticRAGChunk,
  type AgenticRAGView,
} from "@/pages/agent/AgentLabUtils";

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

export function ResultView({
  summary,
  risks = [],
  actions = [],
  next,
  status,
  runtimeLabel,
}: {
  summary?: string;
  risks?: string[];
  actions?: string[];
  next?: string;
  status?: string;
  runtimeLabel?: string;
}) {
  return (
    <>
      <header>
        <h2>结果</h2>
        <div className="result-badges">
          {runtimeLabel && (
            <span className="transcription-badge">{runtimeLabel}</span>
          )}
          <span className={`transcription-badge status-${status}`}>
            {status || "idle"}
          </span>
        </div>
      </header>
      <section>
        <h3>摘要</h3>
        <p>{summary || "等待运行结果"}</p>
      </section>
      <section>
        <h3>风险点</h3>
        {risks.map((item) => (
          <p key={item}>· {item}</p>
        ))}
      </section>
      <section>
        <h3>行动项</h3>
        {actions.map((item) => (
          <p key={item}>· {item}</p>
        ))}
      </section>
      {next && (
        <section>
          <h3>下一步</h3>
          <p>{next}</p>
        </section>
      )}
    </>
  );
}

export function WorkflowGraph({ tasks }: { tasks: WorkflowTask[] }) {
  const nodes: Node[] = tasks.map((task, index) => ({
    id: String(task.id),
    position: { x: (index % 3) * 250, y: Math.floor(index / 3) * 130 },
    data: { label: `${task.name}\n${task.role} · ${task.status}` },
    className: `workflow-node status-${task.status}`,
  }));
  const byName = new Map(tasks.map((task) => [task.name, task]));
  const edges: Edge[] = tasks.flatMap((task) => {
    try {
      const deps = JSON.parse(task.depends_on_json || "[]") as string[];
      return deps
        .map((name) => byName.get(name))
        .filter(Boolean)
        .map((dependency) => ({
          id: `${dependency!.id}-${task.id}`,
          source: String(dependency!.id),
          target: String(task.id),
          animated: task.status === "running",
        }));
    } catch {
      return [];
    }
  });
  return (
    <div className="workflow-canvas">
      {nodes.length ? (
        <ReactFlow nodes={nodes} edges={edges} fitView>
          <Background />
          <Controls />
        </ReactFlow>
      ) : (
        <div className="pane-empty">选择一个 Workflow 查看任务图</div>
      )}
    </div>
  );
}

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

export function RerankPanel({
  citations,
  agenticRAG,
}: {
  citations: AgentCitation[];
  agenticRAG?: AgenticRAGView;
}) {
  const ranked = [...citations].sort(
    (left, right) => (left.final_rank || 999) - (right.final_rank || 999),
  );
  const reranked = agenticRAG?.rerankedHits.length
    ? agenticRAG.rerankedHits
    : ranked;
  if (!reranked.length && !agenticRAG)
    return <div className="pane-empty">暂无可展示的 RAG/Rerank 引用</div>;
  return (
    <div className="citation-groups">
      {agenticRAG && <AgenticRAGSection agenticRAG={agenticRAG} />}
      {reranked.length ? (
        <section>
          <h2>
            RAG / Rerank Top-K<span>{reranked.length}</span>
          </h2>
          <div className="approval-list">
            {reranked.map((citation, index) => (
              <RerankChunkCard
                chunk={citation}
                key={`${citation.source_type}-${citation.source_id}-${index}`}
                index={index}
              />
            ))}
          </div>
        </section>
      ) : null}
      {agenticRAG?.rawHits.length ? (
        <ChunkList title="Raw Hits" chunks={agenticRAG.rawHits} />
      ) : null}
      {agenticRAG?.rejectedChunks.length ? (
        <ChunkList title="Rejected Chunks" chunks={agenticRAG.rejectedChunks} />
      ) : null}
    </div>
  );
}

function AgenticRAGSection({ agenticRAG }: { agenticRAG: AgenticRAGView }) {
  return (
    <section>
      <h2>
        Agentic RAG<span>{agenticRAG.attempts.length} steps</span>
      </h2>
      <article className="panel citation-card">
        <header>
          <strong>
            {agenticRAG.harness?.name || "bounded retrieval loop"}
          </strong>
          <span>
            {agenticRAG.sufficiency?.sufficient === false
              ? "insufficient"
              : "grounded"}
          </span>
        </header>
        <footer className="citation-meta">
          {agenticRAG.route?.route && <span>{agenticRAG.route.route}</span>}
          {agenticRAG.route?.retrieval_strategy && (
            <span>{agenticRAG.route.retrieval_strategy}</span>
          )}
          <span>
            max{" "}
            {agenticRAG.plan?.max_steps ?? agenticRAG.budget?.max_steps ?? 0}
          </span>
          <span>
            confidence{" "}
            {(
              agenticRAG.evidence?.confidence ??
              agenticRAG.sufficiency?.confidence ??
              0
            ).toFixed(2)}
          </span>
          {agenticRAG.evidence?.source_types?.map((source) => (
            <span key={source}>{sourceLabel(source)}</span>
          ))}
        </footer>
        {agenticRAG.harness?.prompt_version && (
          <p>prompt: {agenticRAG.harness.prompt_version}</p>
        )}
        {agenticRAG.stopReason && <p>stop: {agenticRAG.stopReason}</p>}
        {agenticRAG.sufficiency?.reason && (
          <p>{agenticRAG.sufficiency.reason}</p>
        )}
        {agenticRAG.sufficiency?.missing_info?.length ? (
          <pre>{agenticRAG.sufficiency.missing_info.join(", ")}</pre>
        ) : null}
        {agenticRAG.critic && (
          <pre>
            {[
              `critic=${agenticRAG.critic.passed ? "passed" : "guarded"}`,
              `coverage=${(agenticRAG.critic.citation_coverage ?? 0).toFixed(2)}`,
              `budget=${agenticRAG.critic.budget_respected ? "ok" : "exceeded"}`,
              `writes=${agenticRAG.critic.write_proposal_safe ? "approval-only" : "unsafe"}`,
              ...(agenticRAG.critic.issues ?? []),
            ].join(" · ")}
          </pre>
        )}
      </article>
      {agenticRAG.loopTraces.length ? (
        <div className="approval-list">
          {agenticRAG.loopTraces.map((loop) => (
            <article
              className="panel citation-card"
              key={`${loop.role}-${loop.stop_reason}`}
            >
              <header>
                <strong>{loop.role || "role loop"}</strong>
                <span>{loop.stop_reason || "completed"}</span>
              </header>
              <footer className="citation-meta">
                <span>
                  {loop.budget?.used_steps ?? 0}/
                  {loop.budget?.max_steps ?? loop.spec?.max_steps ?? 0} steps
                </span>
                <span>{loop.budget?.read_tool_calls ?? 0} reads</span>
                {loop.spec?.allowed_tools?.map((tool) => (
                  <span key={tool}>{tool}</span>
                ))}
              </footer>
              {loop.spec?.objective && <p>{loop.spec.objective}</p>}
            </article>
          ))}
        </div>
      ) : null}
      <div className="approval-list">
        {agenticRAG.attempts.map((attempt, index) => (
          <article
            className="panel citation-card"
            key={`${attempt.step ?? index}-${attempt.tool_name ?? "tool"}`}
          >
            <header>
              <strong>{attempt.tool_name || "retrieval"}</strong>
              <span>#{attempt.step ?? index + 1}</span>
            </header>
            <p>{attempt.query || "scoped retrieval query"}</p>
            <footer className="citation-meta">
              <span>{attempt.source_scope || "all"}</span>
              {attempt.strategy && <span>{attempt.strategy}</span>}
              <span>{attempt.hit_count ?? 0} hits</span>
              <span>confidence {(attempt.confidence ?? 0).toFixed(2)}</span>
              {attempt.source_types?.map((source) => (
                <span key={source}>{sourceLabel(source)}</span>
              ))}
            </footer>
            {attempt.expanded_terms?.length ? (
              <pre>{attempt.expanded_terms.join(", ")}</pre>
            ) : null}
            {attempt.observation && <pre>{attempt.observation}</pre>}
          </article>
        ))}
      </div>
    </section>
  );
}

function ChunkList({
  title,
  chunks,
}: {
  title: string;
  chunks: AgenticRAGChunk[];
}) {
  return (
    <section>
      <h2>
        {title}
        <span>{chunks.length}</span>
      </h2>
      <div className="approval-list">
        {chunks.map((chunk, index) => (
          <RerankChunkCard
            chunk={chunk}
            index={index}
            key={`${title}-${chunk.source_type}-${chunk.source_id}-${index}`}
          />
        ))}
      </div>
    </section>
  );
}

function RerankChunkCard({
  chunk,
  index,
}: {
  chunk: AgenticRAGChunk;
  index: number;
}) {
  return (
    <article className="panel citation-card">
      <header>
        <strong>
          {chunk.source_title ||
            chunk.title ||
            sourceLabel(chunk.source_type || "source")}
        </strong>
        <span>#{chunk.final_rank || index + 1}</span>
      </header>
      <p>{chunk.snippet}</p>
      <footer className="citation-meta">
        <span>{sourceLabel(chunk.source_type || "source")}</span>
        <span>{chunk.retrieval_mode || "retrieval"}</span>
        {chunk.rerank_score ? (
          <span>rerank {chunk.rerank_score.toFixed(2)}</span>
        ) : null}
      </footer>
      {chunk.rerank_reason && <pre>{chunk.rerank_reason}</pre>}
    </article>
  );
}

export function CitationCard({ citation }: { citation: AgentCitation }) {
  let href = citation.origin_url || "";
  if (
    citation.source_type === "meeting_transcript" &&
    citation.recording_session_id
  ) {
    href = `/recordings/${citation.recording_session_id}?segmentId=${citation.transcript_segment_id ?? ""}&startMs=${citation.start_ms ?? ""}`;
  }
  if (citation.source_type === "knowledge" && citation.knowledge_source_id) {
    href = `/knowledge?sourceId=${citation.knowledge_source_id}`;
  }
  const title =
    citation.source_title ||
    citation.title ||
    sourceLabel(citation.source_type);
  const content = (
    <article className="panel citation-card">
      <header>
        <strong>{title}</strong>
        <span>{Math.round(citation.score * 100)}%</span>
      </header>
      <p>{citation.snippet}</p>
    </article>
  );
  return href ? (
    <Link aria-label={`${title} 引用`} to={href}>
      {content}
    </Link>
  ) : (
    content
  );
}
