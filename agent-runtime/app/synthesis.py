"""Synthesis functions for generating summaries, action items, and risk flags."""

from __future__ import annotations

from .helpers import (
    WORKFLOW_CONTEXT_QA,
    WORKFLOW_FOLLOW_UP_PLANNER,
    WORKFLOW_MEETING_BRIEF,
    WORKFLOW_REACT_GENERAL,
    WORKFLOW_RISK_REVIEW,
    contains_any,
    unique_strings,
)
from .models import WorkflowRequest


def synthesize_summary(request: WorkflowRequest, snippets: list[str]) -> str:
    """Generate a summary based on the workflow preset and available snippets."""
    if request.preset == WORKFLOW_CONTEXT_QA:
        if snippets:
            return "Context QA: 基于已检索上下文回答：" + snippets[0][:180]
        return "Context QA: 当前上下文不足，无法给出有依据的回答。"
    if request.preset == WORKFLOW_REACT_GENERAL:
        if snippets:
            return "ReAct Agent: 基于会话上下文完成自然语言任务：" + snippets[0][:180]
        return "ReAct Agent: 当前缺少可引用上下文，建议补充会话消息、会议转写或知识库内容。"
    if request.preset == WORKFLOW_RISK_REVIEW:
        if snippets:
            return "Risk Review: 已基于会议转写和会话上下文提取风险：" + snippets[0][:180]
        return "Risk Review: 当前缺少可引用上下文，建议补充会议转写或相关备注。"
    if request.preset == WORKFLOW_FOLLOW_UP_PLANNER:
        if snippets:
            return "Follow-up Plan: 已基于上下文整理后续行动：" + snippets[0][:180]
        return "Follow-up Plan: 当前上下文不足，建议补充会议结论或客户上下文后再创建任务。"
    if snippets:
        return "Meeting Brief: 会议复盘基于会议录音转写和会话上下文生成：" + snippets[0][:180]
    if request.meeting_transcripts:
        return "Meeting Brief: 会议复盘基于会议录音转写生成：" + request.meeting_transcripts[0].text[:180]
    return "Meeting Brief: 会议复盘缺少可引用的会议转写内容，建议先确认转写状态。"


def synthesize_next_step(request: WorkflowRequest) -> str:
    """Generate the next step recommendation based on workflow preset."""
    if request.preset == WORKFLOW_CONTEXT_QA:
        return "If the answer is insufficient, add more meeting transcript or knowledge context before retrying."
    if request.preset == WORKFLOW_REACT_GENERAL:
        return "Review the grounded answer and approve any proposed write-back before applying side effects."
    return "Review the cited evidence, then approve the proposed write-back if it is accurate."


def synthesize_action_items(request: WorkflowRequest) -> list[str]:
    """Extract action items from meeting transcripts or context chunks."""
    text = " ".join([segment.text for segment in request.meeting_transcripts])
    if not text:
        text = " ".join([chunk.snippet for chunk in request.context_chunks])
    candidates: list[str] = []
    for marker in ("跟进", "确认", "发送", "补充", "review", "confirm", "send", "follow"):
        if marker.lower() in text.lower():
            candidates.append(f"Follow up on item containing `{marker}` from the meeting transcript.")
    if not candidates:
        candidates.append("确认会议结论、风险点和下一步 owner。")
    return unique_strings(candidates)[:3]


def infer_risk_flags(request: WorkflowRequest, snippets: list[str]) -> list[str]:
    """Infer risk flags from snippets and context chunks."""
    text = " ".join(snippets + [chunk.snippet for chunk in request.context_chunks]).lower()
    flags: list[str] = []
    if contains_any(text, ("approval", "security", "legal", "privacy", "审批", "安全", "法务")):
        flags.append("approval_sensitive_action")
    if contains_any(text, ("deadline", "budget", "delay", "延期", "预算", "截止")):
        flags.append("budget_or_timeline_risk")
    if contains_any(text, ("risk", "blocker", "unresolved", "风险", "阻塞", "未解决")):
        flags.append("unresolved_meeting_risk")
    return unique_strings(flags)
