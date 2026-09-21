import type { AgentCitation } from "@/api/agent";
import {
  sourceLabel,
  type AgenticRAGChunk,
  type AgenticRAGView,
} from "@/pages/agent/AgentLabUtils";

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
