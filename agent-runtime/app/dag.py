"""DAG definition and compilation for the agent runtime graph."""

from __future__ import annotations

from typing import Any

from langgraph.graph import END, StateGraph

from .state import GraphState
from .nodes import (
    approval_gate,
    collect_context,
    decompose,
    finalize,
    propose_tools,
    retrieval_planner,
    retrieval_loop,
    retrieve_context,
    rerank_context,
    risk_analyst,
    searcher,
    synthesize,
)
from .nodes.retrieval import build_evidence_pack, grounding_check, merge, sufficiency_gate


def build_workflow_graph() -> Any:
    """Build and compile the LangGraph workflow graph."""
    graph = StateGraph(GraphState)
    graph.add_node("collect_context", collect_context)
    graph.add_node("retrieval_planner", retrieval_planner)
    graph.add_node("retrieval_loop", retrieval_loop)
    graph.add_node("retrieve_context", retrieve_context)
    graph.add_node("rerank_context", rerank_context)
    graph.add_node("evidence_pack", build_evidence_pack)
    graph.add_node("sufficiency_gate", sufficiency_gate)
    graph.add_node("decompose", decompose)
    graph.add_node("searcher", searcher)
    graph.add_node("synthesize", synthesize)
    graph.add_node("risk_analyst", risk_analyst)
    graph.add_node("merge", merge)
    graph.add_node("grounding_check", grounding_check)
    graph.add_node("propose_tools", propose_tools)
    graph.add_node("approval_gate", approval_gate)
    graph.add_node("finalize", finalize)
    graph.set_entry_point("collect_context")
    graph.add_edge("collect_context", "retrieval_planner")
    graph.add_edge("retrieval_planner", "retrieval_loop")
    graph.add_edge("retrieval_loop", "retrieve_context")
    graph.add_edge("retrieve_context", "rerank_context")
    graph.add_edge("rerank_context", "evidence_pack")
    graph.add_edge("evidence_pack", "sufficiency_gate")
    graph.add_edge("sufficiency_gate", "decompose")
    graph.add_edge("decompose", "searcher")
    graph.add_edge("searcher", "synthesize")
    graph.add_edge("synthesize", "risk_analyst")
    graph.add_edge("risk_analyst", "merge")
    graph.add_edge("merge", "grounding_check")
    graph.add_edge("grounding_check", "propose_tools")
    graph.add_edge("propose_tools", "approval_gate")
    graph.add_edge("approval_gate", "finalize")
    graph.add_edge("finalize", END)
    return graph.compile()
