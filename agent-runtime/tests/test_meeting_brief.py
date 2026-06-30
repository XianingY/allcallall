from __future__ import annotations

import pytest

from app.eval_runner import run_eval
from app.graph import run_meeting_brief, run_react_agent, run_workflow
from app.grounding import check_grounding
from app.llamaindex_adapter import run_fixture_retrieval
from app.models import Citation, ContextChunk, MeetingBriefRequest, MeetingTranscriptSegment, WorkflowRequest
from app.prompts import prompt_version_for, structured_prompt_for
from app.providers import ProviderError, create_provider
from app.retrieval import rerank_context_chunks


def test_meeting_brief_returns_trace_citations_and_write_proposals() -> None:
    request = MeetingBriefRequest(
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


def test_runtime_supports_risk_review_follow_up_and_context_qa() -> None:
    base = WorkflowRequest(
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

    follow_up = run_workflow(base.model_copy(update={"preset": "follow_up_planner", "goal": "请生成跟进任务。"}))
    assert follow_up.status == "requires_action"
    assert "create_follow_up_task" in [item.tool_name for item in follow_up.proposed_tool_calls]

    qa = run_workflow(base.model_copy(update={"preset": "context_qa", "goal": "安全审批是什么？"}))
    assert qa.status == "ready"
    assert not qa.proposed_tool_calls


def test_react_agent_runtime_uses_python_langgraph_schema() -> None:
    response = run_react_agent(
        WorkflowRequest(
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


def test_python_eval_fixture_passes(monkeypatch: pytest.MonkeyPatch) -> None:
    from app.config import AgentRuntimeConfig

    monkeypatch.setattr("app.config.config", AgentRuntimeConfig(provider="rules"))
    report = run_eval()

    assert report.summary.total_cases >= 8
    assert report.summary.passed_cases == report.summary.total_cases
    assert report.summary.approval_safety_rate == 1


def test_openai_provider_requires_base_url_and_model(monkeypatch: pytest.MonkeyPatch) -> None:
    from app.config import AgentRuntimeConfig

    monkeypatch.setattr(
        "app.config.config",
        AgentRuntimeConfig(provider="openai_compatible", openai_base_url="", openai_model=""),
    )

    with pytest.raises(ProviderError) as exc:
        create_provider()

    assert exc.value.kind == "configuration"
