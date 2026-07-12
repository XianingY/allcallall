from __future__ import annotations

import hashlib
import json
import re
from typing import Any, Literal, cast

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator


SAFE_ID_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/-]{0,95}$")


def normalize_required_id(value: object, field_name: str) -> object:
    if not isinstance(value, str):
        return value
    normalized = value.strip()
    if not normalized:
        raise ValueError(f"{field_name} must not be empty")
    if SAFE_ID_PATTERN.fullmatch(normalized) is None:
        raise ValueError(f"{field_name} must be an ASCII-safe identifier")
    return normalized


def normalize_optional_id(value: object, field_name: str) -> object:
    if value == "":
        return value
    return normalize_required_id(value, field_name)


def require_mcp_revision_identity(
    tool_name: str,
    installation_id: object,
    revision_id: object,
    tool_id: object,
) -> tuple[int, int, int]:
    """Validate the immutable MCP catalog identity carried across approval and execution."""
    raw_identity = (installation_id, revision_id, tool_id)
    if any(type(value) is not int for value in raw_identity):
        raise ValueError("MCP revision identity values must be strict integers")
    identity = cast(tuple[int, int, int], raw_identity)
    is_mcp = tool_name.startswith("mcp.")
    if is_mcp and not all(value > 0 for value in identity):
        raise ValueError("MCP tools require positive installation, revision, and tool IDs")
    if not is_mcp and any(value != 0 for value in identity):
        raise ValueError("local tools must not carry MCP revision identity")
    return identity


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
    execution_id: str = Field(min_length=1, max_length=96)
    expected_checkpoint_version: int = 0
    tool_capability: str = ""
    organization_id: int
    user_id: int
    conversation_id: int
    agent_run_id: int = Field(default=0, ge=0)
    workflow_run_id: int = Field(ge=0)
    preset: str = "meeting_brief"
    goal: str
    messages: list[ConversationMessage] = Field(default_factory=list)
    notes: list[ConversationNote] = Field(default_factory=list)
    meeting_transcripts: list[MeetingTranscriptSegment] = Field(default_factory=list)
    context_chunks: list[ContextChunk] = Field(default_factory=list)
    tool_policy: ToolPolicy = Field(default_factory=ToolPolicy)
    max_iterations: dict[str, int] = Field(default_factory=dict)
    agentic_rag: AgenticRAGConfig = Field(default_factory=AgenticRAGConfig)

    @field_validator("request_id", mode="before")
    @classmethod
    def normalize_optional_request_id(cls, value: object, info: Any) -> object:
        return normalize_optional_id(value, info.field_name)

    @field_validator("execution_id", mode="before")
    @classmethod
    def normalize_execution_id(cls, value: object) -> object:
        return normalize_required_id(value, "execution_id")

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

    @model_validator(mode="after")
    def require_exactly_one_run_id(self) -> MeetingBriefRequest:
        if bool(self.workflow_run_id) == bool(self.agent_run_id):
            raise ValueError("exactly one of workflow_run_id or agent_run_id is required")
        return self


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
    tool_call_id: str = Field(default="", max_length=96)
    tool_name: str = Field(min_length=1)
    arguments: dict[str, Any] = Field(default_factory=dict)
    reason: str = ""
    idempotency_key: str = ""
    approval_required: bool = True
    mcp_installation_id: int = Field(default=0, ge=0, strict=True)
    mcp_revision_id: int = Field(default=0, ge=0, strict=True)
    mcp_tool_id: int = Field(default=0, ge=0, strict=True)

    @model_validator(mode="before")
    @classmethod
    def validate_explicit_tool_call_id(cls, value: object) -> object:
        if not isinstance(value, dict) or "tool_call_id" not in value:
            return value
        normalized = dict(value)
        normalized["tool_call_id"] = normalize_required_id(
            normalized["tool_call_id"], "tool_call_id"
        )
        return normalized

    @model_validator(mode="after")
    def populate_tool_call_id(self) -> ToolProposal:
        tool_call_id = self.tool_call_id.strip()
        if not tool_call_id:
            canonical_arguments = json.dumps(
                self.arguments,
                sort_keys=True,
                separators=(",", ":"),
                ensure_ascii=True,
            )
            identity = self.idempotency_key.strip() or f"{self.tool_name}:{canonical_arguments}"
            tool_call_id = f"tool_{hashlib.sha256(identity.encode()).hexdigest()}"
        if len(tool_call_id) > 96:
            raise ValueError("tool_call_id must contain at most 96 characters")
        self.tool_call_id = tool_call_id
        return self

    @model_validator(mode="after")
    def validate_mcp_revision_identity(self) -> ToolProposal:
        require_mcp_revision_identity(
            self.tool_name,
            self.mcp_installation_id,
            self.mcp_revision_id,
            self.mcp_tool_id,
        )
        return self


class ApprovalToolRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    tool_call_id: str = Field(min_length=1, max_length=96)
    tool_name: str = Field(min_length=1)
    arguments: dict[str, Any] = Field(default_factory=dict)
    arguments_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    reason: str = ""
    mcp_installation_id: int = Field(default=0, ge=0, strict=True)
    mcp_revision_id: int = Field(default=0, ge=0, strict=True)
    mcp_tool_id: int = Field(default=0, ge=0, strict=True)

    @field_validator("tool_call_id", mode="before")
    @classmethod
    def normalize_tool_call_id(cls, value: object) -> object:
        return normalize_required_id(value, "tool_call_id")

    @model_validator(mode="after")
    def validate_mcp_revision_identity(self) -> ApprovalToolRequest:
        require_mcp_revision_identity(
            self.tool_name,
            self.mcp_installation_id,
            self.mcp_revision_id,
            self.mcp_tool_id,
        )
        return self


class ApprovalInterrupt(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    type: Literal["tool_approval"] = "tool_approval"
    approval_request_id: str = Field(min_length=1, max_length=96)
    tools: list[ApprovalToolRequest] = Field(min_length=1)

    @field_validator("approval_request_id", mode="before")
    @classmethod
    def normalize_approval_request_id(cls, value: object) -> object:
        return normalize_required_id(value, "approval_request_id")


class ApprovalDecision(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    tool_call_id: str = Field(min_length=1, max_length=96)
    decision: Literal["approve", "reject"]

    @field_validator("tool_call_id", mode="before")
    @classmethod
    def normalize_tool_call_id(cls, value: object) -> object:
        return normalize_required_id(value, "tool_call_id")


class ApprovalResumePayload(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    approval_request_id: str = Field(min_length=1, max_length=96)
    decisions: list[ApprovalDecision] = Field(min_length=1)

    @field_validator("approval_request_id", mode="before")
    @classmethod
    def normalize_approval_request_id(cls, value: object) -> object:
        return normalize_required_id(value, "approval_request_id")

    @model_validator(mode="after")
    def reject_duplicate_tool_call_ids(self) -> ApprovalResumePayload:
        call_ids = [item.tool_call_id for item in self.decisions]
        if len(call_ids) != len(set(call_ids)):
            raise ValueError("approval decisions contain duplicate tool_call_id values")
        return self


class WorkflowResumeRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    request_id: str = ""
    execution_id: str = Field(min_length=1, max_length=96)
    expected_checkpoint_version: int = Field(ge=0)
    tool_capability: str = ""
    organization_id: int = Field(gt=0)
    user_id: int = Field(gt=0)
    conversation_id: int = Field(gt=0)
    agent_run_id: int = Field(default=0, ge=0)
    workflow_run_id: int = Field(default=0, ge=0)
    resume: ApprovalResumePayload

    @field_validator("execution_id", mode="before")
    @classmethod
    def normalize_execution_id(cls, value: object) -> object:
        return normalize_required_id(value, "execution_id")

    @field_validator("request_id", mode="before")
    @classmethod
    def normalize_request_id(cls, value: object) -> object:
        return normalize_optional_id(value, "request_id")

    @model_validator(mode="after")
    def require_run_id(self) -> WorkflowResumeRequest:
        if bool(self.workflow_run_id) == bool(self.agent_run_id):
            raise ValueError("exactly one of workflow_run_id or agent_run_id is required")
        return self


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
    execution_id: str = ""
    checkpoint_id: str = ""
    checkpoint_version: int = 0
    summary: str = ""
    action_items: list[str] = Field(default_factory=list)
    next_step: str = ""
    risk_flags: list[str] = Field(default_factory=list)
    citations: list[Citation] = Field(default_factory=list)
    role_results: list[RoleResult] = Field(default_factory=list)
    trace_events: list[TraceEvent] = Field(default_factory=list)
    proposed_tool_calls: list[ToolProposal] = Field(default_factory=list)
    pending_approval: ApprovalInterrupt | None = None
    approval_decisions: list[ApprovalDecision] = Field(default_factory=list)
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
