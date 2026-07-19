from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from app.eval_runner import run_eval
from app.main import app, run_meeting_brief, run_react_agent, run_workflow
from app.grounding import check_grounding, meaningful_tokens
from app.llamaindex_adapter import run_fixture_retrieval
from app.models import Citation, ContextChunk, MeetingBriefRequest, MeetingTranscriptSegment, WorkflowRequest
from app.prompts import prompt_version_for, structured_prompt_for
from app.providers import ProviderError, create_provider
from app.nodes.mcp import use_mcp_tools
from app.nodes.context import collect_context
from app.helpers import tool_capability_scope
from app.tool_bridge import ToolObservation, UnifiedToolRegistry, untrusted_mcp_chunk
from app.retrieval import rerank_context_chunks


def test_meeting_brief_returns_trace_citations_and_write_proposals() -> None:
    request = MeetingBriefRequest(
        execution_id="workflow:99:initial",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=99,
        goal="请生成会议复盘，关注风险和行动项。",
        meeting_transcripts=[
            MeetingTranscriptSegment(
                id=10,
                recording_session_id=20,
                recording_file_id=30,
                start_ms=1000,
                end_ms=5000,
                text="本次会议确认需要跟进安全审批，预算截止日期存在风险。",
            )
        ],
        context_chunks=[
            ContextChunk(
                chunk_id="10",
                source_type="meeting_transcript",
                source_id="10",
                source_title="Task Eval Meeting",
                title="Task Eval Meeting",
                snippet="本次会议确认需要跟进安全审批，预算截止日期存在风险。",
                score=10,
                retrieval_mode="rules",
                recording_session_id=20,
                recording_file_id=30,
                transcript_segment_id=10,
                start_ms=1000,
                end_ms=5000,
            )
        ],
        max_iterations={"searcher": 3, "risk_analyst": 2},
    )

    response = run_meeting_brief(request)

    assert response.runtime == "python_langgraph"
    assert response.summary
    assert response.citations[0].source_type == "meeting_transcript"
    assert response.citations[0].transcript_segment_id == 10
    assert response.proposed_tool_calls
    assert response.prompt_version == "meeting_brief_v2"
    assert response.grounding_check_result
    assert all(item.approval_required for item in response.proposed_tool_calls)
    assert any(item.node == "approval_gate" for item in response.trace_events)
    search_events = [
        item
        for item in response.trace_events
        if item.event == "react.observe" and item.role == "searcher" and item.iteration
    ]
    risk_events = [
        item
        for item in response.trace_events
        if item.event == "react.observe" and item.role == "risk_analyst" and item.iteration
    ]
    assert len(search_events) <= 3
    assert len(risk_events) <= 2


def test_prompt_registry_and_rules_rerank_metadata() -> None:
    request = WorkflowRequest(
        execution_id="workflow:100:initial",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=100,
        preset="risk_review",
        goal="security approval risk",
        context_chunks=[],
    )
    version, messages = structured_prompt_for(request, ["security approval is blocked"])
    assert version == "risk_review_v1"
    assert prompt_version_for(request) == "risk_review_v1"
    assert messages[0]["role"] == "system"

    output = rerank_context_chunks(
        "security approval risk",
        [
            ContextChunk(source_type="message", source_id="1", snippet="general logistics", score=100),
            ContextChunk(source_type="meeting_transcript", source_id="2", snippet="security approval risk", score=1),
        ],
    )
    assert output.chunks[0].source_type == "meeting_transcript"
    assert output.chunks[0].final_rank == 1
    assert output.chunks[0].rerank_score > 0


def test_grounding_and_llamaindex_adapter_fallback() -> None:
    grounding = check_grounding(
        "security approval risk",
        [
            Citation(source_type="meeting_transcript", source_id="1", snippet="security approval risk", score=1)
        ],
    )
    assert grounding.trace.event == "grounding.check"

    result = run_fixture_retrieval(
        "security approval",
        [{"title": "Policy", "text": "Security approval policy"}, {"title": "Other", "text": "Billing"}],
        top_k=1,
    )
    assert result.hits


def test_grounding_uses_jieba_for_chinese_claims() -> None:
    tokens = meaningful_tokens("供应商审批流程需要安全团队复核")

    assert {"供应商", "审批", "流程", "安全", "团队", "复核"}.issubset(tokens)

    grounded = check_grounding(
        "供应商审批流程需要安全团队复核",
        [Citation(source_type="knowledge", source_id="1", snippet="供应商审批流程要求安全团队复核。")],
    )
    partial_overlap = check_grounding(
        "供应商审批已经通过，财务预算也已批准",
        [Citation(source_type="knowledge", source_id="1", snippet="供应商审批流程仍在安全团队复核中。")],
    )

    assert grounded.grounded is True
    assert partial_overlap.grounded is False


def test_runtime_supports_risk_review_follow_up_and_context_qa() -> None:
    base = WorkflowRequest(
        execution_id="workflow:101:initial",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=101,
        preset="risk_review",
        goal="请识别风险。",
        context_chunks=[
            ContextChunk(
                chunk_id="risk",
                source_type="meeting_transcript",
                source_id="9",
                title="Risk",
                snippet="安全审批存在 blocker，预算截止日期可能延期。",
                score=10,
                retrieval_mode="rules",
            )
        ],
    )

    risk = run_workflow(base)
    assert risk.status == "requires_action"
    assert "Risk Review" in risk.summary
    assert "write_conversation_message" in [item.tool_name for item in risk.proposed_tool_calls]

    follow_up = run_workflow(
        base.model_copy(
            update={
                "workflow_run_id": 10_001,
                "execution_id": "workflow:10001:initial",
                "preset": "follow_up_planner",
                "goal": "请生成跟进任务。",
            }
        )
    )
    assert follow_up.status == "requires_action"
    assert "create_follow_up_task" in [item.tool_name for item in follow_up.proposed_tool_calls]

    qa = run_workflow(
        base.model_copy(
            update={
                "workflow_run_id": 10_002,
                "execution_id": "workflow:10002:initial",
                "preset": "context_qa",
                "goal": "安全审批是什么？",
            }
        )
    )
    assert qa.status == "ready"
    assert not qa.proposed_tool_calls


def test_react_agent_runtime_uses_python_langgraph_schema() -> None:
    response = run_react_agent(
        WorkflowRequest(
            execution_id="agent:103:initial",
            organization_id=1,
            user_id=7,
            conversation_id=42,
            agent_run_id=103,
            workflow_run_id=0,
            preset="react_general",
            goal="请总结当前会话并给出下一步。",
            context_chunks=[
                ContextChunk(
                    chunk_id="msg-1",
                    source_type="message",
                    source_id="1",
                    title="Message",
                    snippet="客户要求跟进安全审批，并确认预算截止日期。",
                    score=10,
                    retrieval_mode="rules",
                )
            ],
        )
    )

    assert response.runtime == "python_langgraph"
    assert response.prompt_version == "react_general_v1"
    assert response.summary.startswith("ReAct Agent")
    assert response.proposed_tool_calls
    assert all(item.idempotency_key.startswith("agent:103:") for item in response.proposed_tool_calls)


def test_context_qa_guard_when_context_is_missing() -> None:
    response = run_workflow(
        WorkflowRequest(
            execution_id="workflow:102:initial",
            organization_id=1,
            user_id=7,
            conversation_id=42,
            workflow_run_id=102,
            preset="context_qa",
            goal="客户最终价格是多少？",
        )
    )

    assert response.status == "ready"
    assert "不足" in response.summary
    assert not response.citations
    assert not response.proposed_tool_calls


def test_metrics_endpoint_exposes_checkpoint_and_resume_counters() -> None:
    response = TestClient(app).get("/metrics")

    assert response.status_code == 200
    assert "agent_runtime_checkpoint_conflict_total" in response.text
    assert "agent_runtime_resume_total" in response.text


def test_mcp_node_executes_verified_reads_and_proposes_unknown_tools(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    request = WorkflowRequest(
        execution_id="workflow:500:initial",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=500,
        preset="react_general",
        goal="Use mcp.1.search to query the customer status",
    )
    catalog = [
        {
            "name": "mcp.1.search",
            "original_name": "search",
            "risk": "read",
            "mcp_installation_id": 1,
            "mcp_revision_id": 11,
            "mcp_tool_id": 111,
            "input_schema": {
                "type": "object",
                "required": ["query"],
                "properties": {"query": {"type": "string"}},
            },
        }
    ]
    seen_capabilities: list[str] = []

    def execute(
        runtime_request: WorkflowRequest,
        name: str,
        arguments: dict[str, object],
        *,
        mcp_installation_id: int,
        mcp_revision_id: int,
        mcp_tool_id: int,
    ) -> ToolObservation:
        seen_capabilities.append(runtime_request.tool_capability)
        assert (mcp_installation_id, mcp_revision_id, mcp_tool_id) == (1, 11, 111)
        output = '{"customer":"active"}'
        return ToolObservation(
            tool_name=name,
            input=arguments,
            output_json=output,
            chunks=(untrusted_mcp_chunk(name, "read-call", output),),
        )

    monkeypatch.setattr(UnifiedToolRegistry.get_instance(), "execute_read_tool", execute)
    with tool_capability_scope("run-capability"):
        read_result = use_mcp_tools({"request": request, "authorized_mcp_tools": catalog})
    assert seen_capabilities == ["run-capability"]
    assert read_result["external_tool_context_chunks"][0].source_type == "mcp_untrusted"
    assert not read_result["mcp_tool_proposals"]

    request = request.model_copy(update={"goal": "Use mcp.1.update with this query"})
    write_result = use_mcp_tools(
        {
            "request": request,
            "authorized_mcp_tools": [
                {
                    **catalog[0],
                    "name": "mcp.1.update",
                    "original_name": "update",
                    "risk": "unknown",
                }
            ],
        }
    )
    proposals = write_result["mcp_tool_proposals"]
    assert len(proposals) == 1
    assert proposals[0].tool_name == "mcp.1.update"
    assert proposals[0].approval_required
    assert (
        proposals[0].mcp_installation_id,
        proposals[0].mcp_revision_id,
        proposals[0].mcp_tool_id,
    ) == (1, 11, 111)


def test_authorized_skills_are_scoped_to_catalog_tools(monkeypatch: pytest.MonkeyPatch) -> None:
    request = WorkflowRequest(
        execution_id="workflow:501:initial",
        organization_id=1,
        user_id=7,
        conversation_id=42,
        workflow_run_id=501,
        preset="react_general",
        goal="Search policy",
    )

    def catalog(_: WorkflowRequest) -> dict[str, list[dict[str, object]]]:
        return {
            "tools": [
                {
                    "id": 111,
                    "installation_id": 1,
                    "revision_id": 11,
                    "name": "mcp.1.search",
                    "risk": "read",
                    "input_schema": {},
                }
            ],
            "skills": [
                {
                    "name": "Policy search",
                    "instructions": "Use policy sources and report uncertainty.",
                    "tool_names": ["mcp.1.search", "mcp.2.not-authorized"],
                }
            ],
        }

    monkeypatch.setattr(UnifiedToolRegistry.get_instance(), "catalog", catalog)
    result = collect_context({"request": request})

    prompt = result["active_skills_prompt"]
    assert "Policy search" in prompt
    assert "mcp.1.search" in prompt
    assert "mcp.2.not-authorized" not in prompt


def test_python_eval_fixture_passes(monkeypatch: pytest.MonkeyPatch) -> None:
    from app.config import AgentRuntimeConfig

    monkeypatch.setattr("app.config.config", AgentRuntimeConfig(provider="rules"))
    report = run_eval()

    assert report.summary.total_cases >= 8
    assert report.summary.passed_cases == report.summary.total_cases
    assert report.summary.approval_safety_rate == 1
    assert report.summary.grounding_check_rate == 1


def test_openai_provider_requires_base_url_and_model(monkeypatch: pytest.MonkeyPatch) -> None:
    from app.config import AgentRuntimeConfig

    monkeypatch.setattr(
        "app.config.config",
        AgentRuntimeConfig(provider="openai_compatible", openai_base_url="", openai_model=""),
    )

    with pytest.raises(ProviderError) as exc:
        create_provider()

    assert exc.value.kind == "configuration"
