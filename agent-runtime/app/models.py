from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator


class ConversationMessage(BaseModel):
    id: int = 0
    sender_id: int = 0
    body: str = ""
    created_at: str | None = None


class ConversationNote(BaseModel):
    id: int = 0
    author_id: int = 0
    body: str = ""
    created_at: str | None = None


class MeetingTranscriptSegment(BaseModel):
    id: int = 0
    recording_session_id: int = 0
    recording_file_id: int = 0
    start_ms: int = 0
    end_ms: int = 0
    text: str = ""
    speaker: str = ""


class ContextChunk(BaseModel):
    chunk_id: str = ""
    source_type: str
    source_id: str
    source_title: str = ""
    title: str = ""
    snippet: str
    score: int = 0
    retrieval_mode: str = ""
    bm25_rank: int = 0
    vector_rank: int = 0
    rrf_score: float = 0
    bm25_score: float = 0
    vector_score: float = 0
    rerank_score: float = 0
    rerank_reason: str = ""
    final_rank: int = 0
    recording_session_id: int | None = None
    recording_file_id: int | None = None
    transcript_segment_id: int | None = None
    start_ms: int | None = None
    end_ms: int | None = None


class ToolPolicy(BaseModel):
    read_tools: list[str] = Field(default_factory=list)
    write_tools: list[str] = Field(default_factory=list)


class AgenticRAGConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    enabled: bool = False
    max_steps: int = 3
    allowed_source_types: list[str] = Field(
        default_factory=lambda: [
            "meeting_transcript",
            "knowledge",
            "conversation",
            "message",
            "note",
            "followup",
            "memory",
            "contact_profile",
        ]
    )
    min_confidence: float = 0.6


class MeetingBriefRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    request_id: str = ""
    organization_id: int
    user_id: int
    conversation_id: int
    agent_run_id: int = 0
    workflow_run_id: int
    preset: str = "meeting_brief"
    goal: str
    messages: list[ConversationMessage] = Field(default_factory=list)
    notes: list[ConversationNote] = Field(default_factory=list)
    meeting_transcripts: list[MeetingTranscriptSegment] = Field(default_factory=list)
    context_chunks: list[ContextChunk] = Field(default_factory=list)
    tool_policy: ToolPolicy = Field(default_factory=ToolPolicy)
    max_iterations: dict[str, int] = Field(default_factory=dict)
    agentic_rag: AgenticRAGConfig = Field(default_factory=AgenticRAGConfig)

    @field_validator(
        "messages",
        "notes",
        "meeting_transcripts",
        "context_chunks",
        mode="before",
    )
    @classmethod
    def none_to_list(cls, value: object) -> object:
        return [] if value is None else value

    @field_validator("max_iterations", mode="before")
    @classmethod
    def none_to_dict(cls, value: object) -> object:
        return {} if value is None else value


class RetrievalPlanStep(BaseModel):
    step: int
    query: str
    source_scope: str = "all"
    tool_name: str = "query_context_chunks"
    rationale: str = ""


class RetrievalPlan(BaseModel):
    enabled: bool = False
    max_steps: int = 3
    min_confidence: float = 0.6
    steps: list[RetrievalPlanStep] = Field(default_factory=list)


class RetrievalAttempt(BaseModel):
    step: int
    query: str
    tool_name: str
    source_scope: str = "all"
    hit_count: int = 0
    source_types: list[str] = Field(default_factory=list)
    selected_chunk_ids: list[str] = Field(default_factory=list)
    observation: str = ""
    refined: bool = False
    confidence: float = 0


class Citation(BaseModel):
    chunk_id: str = ""
    source_type: str
    source_id: str
    source_title: str = ""
    title: str = ""
    snippet: str
    score: int = 0
    retrieval_mode: str = ""
    rerank_score: float = 0
    rerank_reason: str = ""
    final_rank: int = 0
    recording_session_id: int | None = None
    recording_file_id: int | None = None
    transcript_segment_id: int | None = None
    start_ms: int | None = None
    end_ms: int | None = None


class TraceEvent(BaseModel):
    event: str
    node: str
    role: str = ""
    status: str = "completed"
    iteration: int | None = None
    thought: str = ""
    tool_name: str = ""
    tool_input: dict[str, Any] = Field(default_factory=dict)
    observation: str = ""
    metadata: dict[str, Any] = Field(default_factory=dict)


class RoleResult(BaseModel):
    role: str
    summary: str = ""
    action_items: list[str] = Field(default_factory=list)
    next_step: str = ""
    risk_flags: list[str] = Field(default_factory=list)
    citations: list[Citation] = Field(default_factory=list)
    snippets: list[str] = Field(default_factory=list)
    react_trace: list[TraceEvent] = Field(default_factory=list)


class ToolProposal(BaseModel):
    tool_name: str
    arguments: dict[str, Any] = Field(default_factory=dict)
    reason: str = ""
    idempotency_key: str = ""
    approval_required: bool = True


class EvidencePack(BaseModel):
    selected_chunk_ids: list[str] = Field(default_factory=list)
    rejected_count: int = 0
    confidence: float = 0
    source_types: list[str] = Field(default_factory=list)
    snippets: list[str] = Field(default_factory=list)
    citations: list[Citation] = Field(default_factory=list)


class ContextSufficiency(BaseModel):
    sufficient: bool = True
    confidence: float = 1
    reason: str = ""
    missing_info: list[str] = Field(default_factory=list)


class MeetingBriefResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    status: Literal["ready", "requires_action", "failed"] = "requires_action"
    runtime: str = "python_langgraph"
    provider: str = "rules"
    summary: str = ""
    action_items: list[str] = Field(default_factory=list)
    next_step: str = ""
    risk_flags: list[str] = Field(default_factory=list)
    citations: list[Citation] = Field(default_factory=list)
    role_results: list[RoleResult] = Field(default_factory=list)
    trace_events: list[TraceEvent] = Field(default_factory=list)
    proposed_tool_calls: list[ToolProposal] = Field(default_factory=list)
    prompt_version: str = ""
    grounding_check_result: dict[str, Any] = Field(default_factory=dict)
    retrieval_plan: RetrievalPlan = Field(default_factory=RetrievalPlan)
    retrieval_attempts: list[RetrievalAttempt] = Field(default_factory=list)
    evidence_pack: EvidencePack = Field(default_factory=EvidencePack)
    context_sufficiency: ContextSufficiency = Field(default_factory=ContextSufficiency)
    error: str = ""


WorkflowRequest = MeetingBriefRequest
WorkflowResponse = MeetingBriefResponse
AgentRunRequest = MeetingBriefRequest
AgentRunResponse = MeetingBriefResponse


class WorkflowEvalCase(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str
    preset: str = "meeting_brief"
    goal: str
    request: WorkflowRequest
    expected_status: str = "requires_action"
    required_output_substrings: list[str] = Field(default_factory=list)
    required_citation_source_types: list[str] = Field(default_factory=list)
    required_tool_proposals: list[str] = Field(default_factory=list)
    forbidden_tool_proposals: list[str] = Field(default_factory=list)
    requires_unsupported_claim_guard: bool = False


class WorkflowEvalCaseResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str
    preset: str
    passed: bool
    status: str
    task_success: bool
    citation_grounded: bool
    tool_intent_matched: bool
    approval_safe: bool
    unsupported_claim_guarded: bool
    prompt_schema_valid: bool = True
    grounding_check_passed: bool = True
    retrieval_refinement_succeeded: bool = True
    citation_coverage_passed: bool = True
    max_iteration_compliant: bool = True
    unnecessary_tool_calls_avoided: bool = True
    errors: list[str] = Field(default_factory=list)


class WorkflowEvalSummary(BaseModel):
    model_config = ConfigDict(extra="forbid")

    total_cases: int = 0
    passed_cases: int = 0
    task_success_rate: float = 0
    citation_grounding_rate: float = 0
    tool_intent_match_rate: float = 0
    approval_safety_rate: float = 0
    unsupported_claim_guard_rate: float = 0
    prompt_schema_valid_rate: float = 0
    grounding_check_rate: float = 0
    retrieval_refinement_success_rate: float = 0
    citation_coverage_rate: float = 0
    max_iteration_compliance_rate: float = 0
    unnecessary_tool_call_rate: float = 0


class WorkflowEvalReport(BaseModel):
    model_config = ConfigDict(extra="forbid")

    runtime: str = "python_langgraph"
    provider: str = "rules"
    summary: WorkflowEvalSummary = Field(default_factory=WorkflowEvalSummary)
    cases: list[WorkflowEvalCaseResult] = Field(default_factory=list)
