import { Link } from "react-router-dom";

import type { AgentCitation } from "@/api/agent";
import { sourceLabel } from "@/pages/agent/AgentLabUtils";

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
