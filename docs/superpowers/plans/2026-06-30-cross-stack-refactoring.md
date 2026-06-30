# Cross-Stack Refactoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor all identified God Objects, add missing Repository layers, centralize configuration, eliminate code duplication, and improve maintainability across Python, Go, TypeScript, and infrastructure.

**Architecture:** Three-wave parallel execution:
- Wave 1: Python foundation (shared packages, config, graph.py split) — no Go dependencies
- Wave 2: Go backend (Repository layers, service splits, eval separation) — parallel with Wave 1 where possible
- Wave 3: Cross-cutting (API type sharing, env centralization, infra fixes)

**Tech Stack:** Python (FastAPI, LangGraph, Pydantic), Go (Gin, Gorm), TypeScript (React, Expo), Docker Compose, GitHub Actions

---

## File Structure

### Python Changes

```
agent-runtime/app/
├── __init__.py (unchanged)
├── main.py (unchanged)
├── config.py (NEW — centralized config)
├── dag.py (NEW — DAG definition)
├── nodes/ (NEW — node functions)
│   ├── __init__.py
│   ├── context.py
│   ├── retrieval.py
│   ├── synthesis.py
│   └── approval.py
├── synthesis.py (NEW — summary/risk logic)
├── retrieval.py (existing, enhanced)
├── helpers.py (NEW — env parsers, string utils)
├── models.py (existing, trimmed)
├── grounding.py (unchanged)
├── tool_bridge.py (existing, uses shared utils)
├── rag_runtime_client.py (existing, uses shared utils)
├── eval_runner.py (unchanged)
├── providers/ (unchanged)
├── prompts/ (unchanged)
└── graphs/ (DELETE — remove empty facade)

rag-runtime/app/
├── __init__.py (unchanged)
├── main.py (unchanged)
├── config.py (NEW — centralized config, mirrors agent-runtime)
├── models.py (existing, imports from shared)
├── retrieval.py (existing, uses shared scoring)
├── go_bridge.py (existing, uses shared utils)
├── eval_runner.py (unchanged)
└── metrics.py (unchanged)

shared/ (NEW — common Python package)
├── __init__.py
├── models.py (ContextChunk, EvidencePack, etc.)
├── utils.py (float_or_zero, env_bool, chunk_key)
└── scoring.py (tokenize, rules_score)
```

### Go Changes

```
backend/internal/
├── commerce/
│   ├── service.go (existing, trimmed)
│   ├── repository.go (NEW)
│   ├── entitlement_service.go (NEW)
│   ├── legal_service.go (NEW)
│   ├── billing_webhook.go (NEW)
│   └── support_service.go (NEW)
├── knowledge/
│   ├── service.go (existing, trimmed)
│   ├── repository.go (NEW)
│   ├── source_service.go (NEW)
│   ├── chunk_service.go (NEW)
│   └── ingestion_pipeline.go (NEW)
├── agent/
│   ├── service.go (existing, trimmed)
│   ├── repository.go (NEW)
│   └── (eval files moved to cmd/agent-eval/)
├── cmd/
│   └── agent-eval/ (NEW — eval entry point)
└── shared/
    └── metrics.go (NEW — shared counterRecorder interface)
```

---

## Wave 1: Python Foundation (Parallel Execution)

### Task 1: Create Shared Python Package

**Files:**
- Create: `shared/__init__.py`
- Create: `shared/models.py`
- Create: `shared/utils.py`
- Create: `shared/scoring.py`
- Modify: `agent-runtime/pyproject.toml`
- Modify: `rag-runtime/pyproject.toml`

- [ ] **Step 1: Create shared package structure**

```bash
mkdir -p shared
touch shared/__init__.py
```

- [ ] **Step 2: Create shared models**

```python
# shared/models.py
from __future__ import annotations

from typing import Any
from pydantic import BaseModel, ConfigDict, Field


class ContextChunk(BaseModel):
    model_config = ConfigDict(extra="allow")

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


class RetrievalAttempt(BaseModel):
    step: int
    query: str
    tool_name: str = ""
    source_scope: str = "all"
    hit_count: int = 0
    source_types: list[str] = Field(default_factory=list)
    selected_chunk_ids: list[str] = Field(default_factory=list)
    observation: str = ""
    refined: bool = False
    confidence: float = 0


class EvidencePack(BaseModel):
    selected_chunk_ids: list[str] = Field(default_factory=list)
    rejected_count: int = 0
    confidence: float = 0
    source_types: list[str] = Field(default_factory=list)
    snippets: list[str] = Field(default_factory=list)
    citations: list[Any] = Field(default_factory=list)


class ContextSufficiency(BaseModel):
    sufficient: bool = True
    confidence: float = 1
    reason: str = ""
    missing_info: list[str] = Field(default_factory=list)
```

- [ ] **Step 3: Create shared utilities**

```python
# shared/utils.py
from __future__ import annotations

import os
import re


def env_bool(name: str, fallback: bool = False) -> bool:
    raw = os.getenv(name, "").strip().lower()
    if raw == "":
        return fallback
    return raw in {"1", "true", "yes", "on"}


def env_int(name: str, fallback: int = 0) -> int:
    raw = os.getenv(name, "").strip()
    if raw == "":
        return fallback
    try:
        return int(raw)
    except ValueError:
        return fallback


def env_float(name: str, fallback: float = 0.0) -> float:
    raw = os.getenv(name, "").strip()
    if raw == "":
        return fallback
    try:
        return float(raw)
    except ValueError:
        return fallback


def float_or_zero(value: Any) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def chunk_key(chunk: Any) -> str:
    return chunk.chunk_id or f"{chunk.source_type}:{chunk.source_id}"


def unique_strings(values: list[str]) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        normalized = re.sub(r"\s+", " ", value.strip())
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        out.append(normalized)
    return out


def contains_any(text: str, keywords: tuple[str, ...]) -> bool:
    lowered = text.lower()
    return any(keyword.lower() in lowered for keyword in keywords)


def first_non_empty(values: list[str]) -> str:
    for value in values:
        if value.strip():
            return value
    return ""
```

- [ ] **Step 4: Create shared scoring**

```python
# shared/scoring.py
from __future__ import annotations

import re

from .models import ContextChunk


def tokenize(text: str, remove_stopwords: bool = True) -> list[str]:
    stopwords = {
        "a", "an", "and", "are", "for", "the", "to", "with", "what", "which",
    } if remove_stopwords else set()
    
    out: list[str] = []
    seen: set[str] = set()
    for token in re.split(r"[^0-9A-Za-z\u4e00-\u9fff]+", text.lower()):
        token = token.strip()
        if len(token) < 2 or token in seen or token in stopwords:
            continue
        seen.add(token)
        out.append(token)
    return out


def rules_score(
    chunk: ContextChunk,
    tokens: list[str],
    original_index: int,
    source_boosts: dict[str, float] | None = None,
) -> tuple[float, str]:
    if source_boosts is None:
        source_boosts = {
            "meeting_transcript": 6.0,
            "knowledge": 5.0,
            "message": 2.0,
            "note": 2.0,
            "followup": 2.0,
            "memory": 1.5,
        }
    
    text = f"{chunk.title} {chunk.source_title} {chunk.snippet}".lower()
    overlap = sum(1 for token in tokens if token in text)
    source_boost = source_boosts.get(chunk.source_type, 0.0)
    score = overlap * 10.0 + source_boost + max(chunk.score, 0) / 100.0 - original_index * 0.01
    return score, f"rules overlap={overlap} source={chunk.source_type}"
```

- [ ] **Step 5: Update pyproject.toml files**

```toml
# agent-runtime/pyproject.toml (add to dependencies)
dependencies = [
  "fastapi>=0.111,<1",
  "httpx>=0.27,<1",
  "langchain-core>=0.2,<1",
  "langgraph>=0.2,<1",
  "pydantic>=2.7,<3",
  "uvicorn[standard]>=0.30,<1",
  "allcallall-shared>=0.1.0",  # NEW
]

# rag-runtime/pyproject.toml (add to dependencies)
dependencies = [
  "fastapi>=0.111,<1",
  "httpx>=0.27,<1",
  "langchain-core>=0.2,<1",
  "pydantic>=2.7,<3",
  "uvicorn[standard]>=0.30,<1",
  "allcallall-shared>=0.1.0",  # NEW
]
```

- [ ] **Step 6: Commit**

```bash
git add shared/
git commit -m "feat(python): create shared package with common models, utils, and scoring"
```

---

### Task 2: Centralize Python Configuration

**Files:**
- Create: `agent-runtime/app/config.py`
- Create: `rag-runtime/app/config.py`
- Modify: `agent-runtime/app/graph.py` (remove env reads)
- Modify: `agent-runtime/app/providers/openai_compatible.py` (remove env reads)
- Modify: `agent-runtime/app/tool_bridge.py` (remove env reads)
- Modify: `agent-runtime/app/rag_runtime_client.py` (remove env reads)

- [ ] **Step 1: Create agent-runtime config**

```python
# agent-runtime/app/config.py
from __future__ import annotations

from pydantic_settings import BaseSettings


class AgentRuntimeConfig(BaseSettings):
    # Provider settings
    provider: str = "rules"
    provider_strict: bool = False
    
    # OpenAI settings
    openai_base_url: str = ""
    openai_api_key: str = ""
    openai_model: str = "gpt-4"
    openai_timeout_sec: float = 30.0
    
    # Tool bridge settings
    tool_bridge_base_url: str = ""
    tool_bridge_token: str = ""
    tool_bridge_timeout_sec: float = 10.0
    
    # RAG runtime settings
    rag_runtime_base_url: str = ""
    rag_runtime_timeout_sec: float = 10.0
    
    # Agentic RAG settings
    enable_agentic_rag: bool = False
    rag_max_retrieval_steps: int = 3
    rag_min_confidence: float = 0.6
    
    # Prompt settings
    prompt_version: str = ""
    enable_grounding_check: bool = False
    
    model_config = {"env_prefix": "PY_AGENT_"}


config = AgentRuntimeConfig()
```

- [ ] **Step 2: Create rag-runtime config**

```python
# rag-runtime/app/config.py
from __future__ import annotations

from pydantic_settings import BaseSettings


class RAGRuntimeConfig(BaseSettings):
    # Tool bridge settings
    tool_bridge_base_url: str = ""
    tool_bridge_token: str = ""
    tool_bridge_timeout_sec: float = 10.0
    
    # Rerank settings
    rerank_provider: str = "rules"
    top_k: int = 8
    max_steps: int = 3
    min_confidence: float = 0.6
    
    model_config = {"env_prefix": "PY_RAG_"}


config = RAGRuntimeConfig()
```

- [ ] **Step 3: Update agent-runtime files to use config**

```python
# In agent-runtime/app/graph.py, replace:
# import os
# os.getenv("PY_AGENT_PROVIDER", "rules")
# With:
from .config import config
config.provider
```

- [ ] **Step 4: Update rag-runtime files to use config**

```python
# In rag-runtime/app/go_bridge.py, replace:
# import os
# os.getenv("PY_RAG_TOOL_BRIDGE_BASE_URL", "")
# With:
from .config import config
config.tool_bridge_base_url
```

- [ ] **Step 5: Commit**

```bash
git add agent-runtime/app/config.py rag-runtime/app/config.py
git commit -m "feat(python): centralize configuration with pydantic-settings"
```

---

### Task 3: Split agent-runtime/graph.py

**Files:**
- Create: `agent-runtime/app/dag.py`
- Create: `agent-runtime/app/nodes/__init__.py`
- Create: `agent-runtime/app/nodes/context.py`
- Create: `agent-runtime/app/nodes/retrieval.py`
- Create: `agent-runtime/app/nodes/synthesis.py`
- Create: `agent-runtime/app/nodes/approval.py`
- Create: `agent-runtime/app/synthesis.py`
- Create: `agent-runtime/app/helpers.py`
- Delete: `agent-runtime/app/graphs/workflows.py`
- Delete: `agent-runtime/app/graphs/react_agent.py`
- Delete: `agent-runtime/app/graphs/__init__.py`
- Modify: `agent-runtime/app/main.py` (update imports)

- [ ] **Step 1: Create helpers.py with utility functions**

```python
# agent-runtime/app/helpers.py
from __future__ import annotations

from shared.utils import (
    env_bool,
    env_int,
    env_float,
    unique_strings,
    dedupe_citations,
    first_non_empty,
    contains_any,
    chunk_key,
)
from shared.models import ContextChunk, Citation


def citations_from_chunks(chunks: list[ContextChunk]) -> list[Citation]:
    return dedupe_citations(
        [
            Citation(
                chunk_id=chunk.chunk_id,
                source_type=chunk.source_type,
                source_id=chunk.source_id,
                source_title=chunk.source_title or chunk.title,
                title=chunk.title or chunk.source_title or f"{chunk.source_type} #{chunk.source_id}",
                snippet=chunk.snippet,
                score=chunk.score,
                retrieval_mode=chunk.retrieval_mode,
                rerank_score=chunk.rerank_score,
                rerank_reason=chunk.rerank_reason,
                final_rank=chunk.final_rank,
                recording_session_id=chunk.recording_session_id,
                recording_file_id=chunk.recording_file_id,
                transcript_segment_id=chunk.transcript_segment_id,
                start_ms=chunk.start_ms,
                end_ms=chunk.end_ms,
            )
            for chunk in chunks
            if chunk.source_type and chunk.source_id and chunk.snippet
        ]
    )


def top_snippets(chunks: list[ContextChunk], limit: int) -> list[str]:
    return unique_strings([chunk.snippet for chunk in chunks if chunk.snippet])[:limit]
```

- [ ] **Step 2: Create synthesis.py with summary/risk logic**

```python
# agent-runtime/app/synthesis.py
from __future__ import annotations

from .helpers import contains_any, unique_strings
from .models import WorkflowRequest


def synthesize_summary(request: WorkflowRequest, snippets: list[str]) -> str:
    # Move the 50+ line synthesize_summary function here
    ...


def synthesize_action_items(request: WorkflowRequest) -> list[str]:
    # Move synthesize_action_items here
    ...


def synthesize_next_step(request: WorkflowRequest) -> str:
    # Move synthesize_next_step here
    ...


def infer_risk_flags(request: WorkflowRequest, snippets: list[str]) -> list[str]:
    # Move infer_risk_flags here
    ...
```

- [ ] **Step 3: Create nodes/context.py**

```python
# agent-runtime/app/nodes/context.py
from __future__ import annotations

from ..models import GraphState, TraceEvent
from ..prompts import prompt_version_for


def collect_context(state: GraphState) -> GraphState:
    # Move collect_context function here
    ...


def retrieval_planner(state: GraphState) -> GraphState:
    # Move retrieval_planner function here
    ...
```

- [ ] **Step 4: Create nodes/retrieval.py**

```python
# agent-runtime/app/nodes/retrieval.py
from __future__ import annotations

from ..models import GraphState, TraceEvent


def retrieval_loop(state: GraphState) -> GraphState:
    # Move retrieval_loop function here
    ...


def retrieve_context(state: GraphState) -> GraphState:
    # Move retrieve_context function here
    ...


def rerank_context(state: GraphState) -> GraphState:
    # Move rerank_context function here
    ...
```

- [ ] **Step 5: Create nodes/synthesis.py**

```python
# agent-runtime/app/nodes/synthesis.py
from __future__ import annotations

from ..models import GraphState, TraceEvent
from ..synthesis import synthesize_summary, synthesize_action_items


def decompose(state: GraphState) -> GraphState:
    # Move decompose function here
    ...


def searcher(state: GraphState) -> GraphState:
    # Move searcher function here
    ...


def synthesize(state: GraphState) -> GraphState:
    # Move synthesize function here
    ...
```

- [ ] **Step 6: Create nodes/approval.py**

```python
# agent-runtime/app/nodes/approval.py
from __future__ import annotations

from ..models import GraphState, TraceEvent


def propose_tools(state: GraphState) -> GraphState:
    # Move propose_tools function here
    ...


def approval_gate(state: GraphState) -> GraphState:
    # Move approval_gate function here
    ...


def finalize(state: GraphState) -> GraphState:
    # Move finalize function here
    ...
```

- [ ] **Step 7: Create dag.py with graph definition**

```python
# agent-runtime/app/dag.py
from __future__ import annotations

from langgraph.graph import END, StateGraph

from .models import GraphState
from .nodes.context import collect_context, retrieval_planner
from .nodes.retrieval import retrieval_loop, retrieve_context, rerank_context
from .nodes.synthesis import decompose, searcher, synthesize
from .nodes.approval import propose_tools, approval_gate, finalize


def build_workflow_graph() -> StateGraph:
    graph = StateGraph(GraphState)
    
    # Add all nodes
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
    
    # Set entry point and edges
    graph.set_entry_point("collect_context")
    graph.add_edge("collect_context", "retrieval_planner")
    # ... rest of edges
    
    return graph.compile()
```

- [ ] **Step 8: Update main.py imports**

```python
# agent-runtime/app/main.py
from .dag import build_workflow_graph
from .models import AgentRunRequest, AgentRunResponse, WorkflowRequest, WorkflowResponse
```

- [ ] **Step 9: Delete empty facade files**

```bash
rm -rf agent-runtime/app/graphs/
```

- [ ] **Step 10: Commit**

```bash
git add agent-runtime/app/
git commit -m "feat(python): split graph.py into dag, nodes, synthesis, and helpers"
```

---

### Task 4: Deduplicate rag-runtime Models

**Files:**
- Modify: `rag-runtime/app/models.py` (import from shared)
- Modify: `rag-runtime/app/retrieval.py` (use shared scoring)

- [ ] **Step 1: Update rag-runtime models to use shared**

```python
# rag-runtime/app/models.py
from __future__ import annotations

from typing import Any, Literal
from pydantic import BaseModel, ConfigDict, Field

# Import from shared
from shared.models import (
    ContextChunk,
    RetrievalAttempt,
    EvidencePack,
    ContextSufficiency,
)


class RetrievalQueryRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    query: str
    chunks: list[ContextChunk] = Field(default_factory=list)
    source_types: list[str] = Field(default_factory=list)
    top_k: int = 8
```

- [ ] **Step 2: Update rag-runtime retrieval to use shared scoring**

```python
# rag-runtime/app/retrieval.py
from shared.scoring import tokenize, rules_score
from shared.utils import chunk_key, unique_strings
```

- [ ] **Step 3: Commit**

```bash
git add rag-runtime/
git commit -m "feat(python): deduplicate rag-runtime models using shared package"
```

---

## Wave 2: Go Backend (Parallel with Wave 1)

### Task 5: Add Repository Layer to Commerce

**Files:**
- Create: `backend/internal/commerce/repository.go`
- Modify: `backend/internal/commerce/service.go` (use repository)

- [ ] **Step 1: Create commerce repository**

```go
// backend/internal/commerce/repository.go
package commerce

import (
    "context"
    "github.com/allcallall/backend/internal/models"
    "gorm.io/gorm"
)

type Repository struct {
    db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
    return &Repository{db: db}
}

func (r *Repository) GetEntitlement(ctx context.Context, userID int64) (*models.Entitlement, error) {
    var entitlement models.Entitlement
    err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&entitlement).Error
    return &entitlement, err
}

func (r *Repository) UpsertEntitlement(ctx context.Context, entitlement *models.Entitlement) error {
    return r.db.WithContext(ctx).Save(entitlement).Error
}
```

- [ ] **Step 2: Update commerce service to use repository**

```go
// backend/internal/commerce/service.go
type Service struct {
    repo    *Repository
    // ... other fields
}

func NewService(repo *Repository) *Service {
    return &Service{repo: repo}
}
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/commerce/
git commit -m "feat(go): add repository layer to commerce service"
```

---

### Task 6: Split commerce/service.go

**Files:**
- Create: `backend/internal/commerce/entitlement_service.go`
- Create: `backend/internal/commerce/legal_service.go`
- Create: `backend/internal/commerce/billing_webhook.go`
- Create: `backend/internal/commerce/support_service.go`
- Modify: `backend/internal/commerce/service.go` (trim to coordinator)

- [ ] **Step 1: Create entitlement_service.go**

```go
// backend/internal/commerce/entitlement_service.go
package commerce

type EntitlementService struct {
    repo *Repository
}

func NewEntitlementService(repo *Repository) *EntitlementService {
    return &EntitlementService{repo: repo}
}

func (s *EntitlementService) GetEntitlement(ctx context.Context, userID int64) (*models.Entitlement, error) {
    return s.repo.GetEntitlement(ctx, userID)
}

func (s *EntitlementService) RefreshEntitlement(ctx context.Context, userID int64) error {
    // Move entitlement refresh logic here
}
```

- [ ] **Step 2: Create other service files (similar pattern)**

- [ ] **Step 3: Trim service.go to coordinator**

```go
// backend/internal/commerce/service.go
type Service struct {
    entitlement *EntitlementService
    legal       *LegalService
    billing     *BillingWebhookService
    support     *SupportService
}

func NewService(repo *Repository) *Service {
    return &Service{
        entitlement: NewEntitlementService(repo),
        legal:       NewLegalService(repo),
        billing:     NewBillingWebhookService(repo),
        support:     NewSupportService(repo),
    }
}
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/commerce/
git commit -m "feat(go): split commerce service into entitlement, legal, billing, support"
```

---

### Task 7: Add Repository Layer to Knowledge

**Files:**
- Create: `backend/internal/knowledge/repository.go`
- Modify: `backend/internal/knowledge/service.go` (use repository)

- [ ] **Step 1: Create knowledge repository**

```go
// backend/internal/knowledge/repository.go
package knowledge

import (
    "context"
    "github.com/allcallall/backend/internal/models"
    "gorm.io/gorm"
)

type Repository struct {
    db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
    return &Repository{db: db}
}

func (r *Repository) GetSource(ctx context.Context, id int64) (*models.KnowledgeSource, error) {
    var source models.KnowledgeSource
    err := r.db.WithContext(ctx).First(&source, id).Error
    return &source, err
}

func (r *Repository) ListChunksBySource(ctx context.Context, sourceID int64) ([]models.KnowledgeChunk, error) {
    var chunks []models.KnowledgeChunk
    err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).Find(&chunks).Error
    return chunks, err
}
```

- [ ] **Step 2: Update knowledge service to use repository**

- [ ] **Step 3: Commit**

```bash
git add backend/internal/knowledge/
git commit -m "feat(go): add repository layer to knowledge service"
```

---

### Task 8: Split knowledge/service.go

**Files:**
- Create: `backend/internal/knowledge/source_service.go`
- Create: `backend/internal/knowledge/chunk_service.go`
- Create: `backend/internal/knowledge/ingestion_pipeline.go`
- Modify: `backend/internal/knowledge/service.go` (trim to coordinator)

- [ ] **Step 1: Create source_service.go**

```go
// backend/internal/knowledge/source_service.go
package knowledge

type SourceService struct {
    repo *Repository
}

func NewSourceService(repo *Repository) *SourceService {
    return &SourceService{repo: repo}
}

func (s *SourceService) CreateSource(ctx context.Context, req CreateSourceRequest) (*models.KnowledgeSource, error) {
    // Move source creation logic here
}
```

- [ ] **Step 2: Create chunk_service.go and ingestion_pipeline.go**

- [ ] **Step 3: Trim service.go to coordinator**

- [ ] **Step 4: Commit**

```bash
git add backend/internal/knowledge/
git commit -m "feat(go): split knowledge service into source, chunk, and ingestion"
```

---

### Task 9: Move Eval Code Out of Agent Package

**Files:**
- Create: `backend/cmd/agent-eval/main.go`
- Move: `backend/internal/agent/rag_eval.go` → `backend/cmd/agent-eval/rag_eval.go`
- Move: `backend/internal/agent/task_eval.go` → `backend/cmd/agent-eval/task_eval.go`
- Move: `backend/internal/agent/workflow_eval.go` → `backend/cmd/agent-eval/workflow_eval.go`

- [ ] **Step 1: Create agent-eval entry point**

```go
// backend/cmd/agent-eval/main.go
package main

import (
    "flag"
    "log"
)

func main() {
    outDir := flag.String("out", "evals/reports", "Output directory")
    flag.Parse()
    
    log.Printf("Running agent eval, output to %s", *outDir)
    // ... eval logic
}
```

- [ ] **Step 2: Move eval files**

```bash
mkdir -p backend/cmd/agent-eval
git mv backend/internal/agent/rag_eval.go backend/cmd/agent-eval/
git mv backend/internal/agent/task_eval.go backend/cmd/agent-eval/
git mv backend/internal/agent/workflow_eval.go backend/cmd/agent-eval/
```

- [ ] **Step 3: Update imports and fix any compilation issues**

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/agent-eval/ backend/internal/agent/
git commit -m "feat(go): move eval code from agent package to cmd/agent-eval"
```

---

### Task 10: Create Shared Metrics Interface

**Files:**
- Create: `backend/internal/shared/metrics.go`
- Modify: `backend/internal/agent/service.go` (use shared interface)
- Modify: `backend/internal/collaboration/service.go` (use shared interface)
- Modify: `backend/internal/commerce/service.go` (use shared interface)
- Modify: `backend/internal/events/` (use shared interface)

- [ ] **Step 1: Create shared metrics interface**

```go
// backend/internal/shared/metrics.go
package shared

type CounterRecorder interface {
    IncrementCounter(name string, tags ...string)
    RecordHistogram(name string, value float64, tags ...string)
}
```

- [ ] **Step 2: Update all packages to use shared interface**

- [ ] **Step 3: Commit**

```bash
git add backend/internal/shared/
git commit -m "feat(go): create shared metrics interface to eliminate duplication"
```

---

## Wave 3: Cross-Cutting (After Wave 2)

### Task 11: Web/Mobile API Type Sharing

**Files:**
- Create: `packages/api-types/` (shared TypeScript types)
- Modify: `web/src/api/schema.d.ts` (generate from shared)
- Modify: `mobile/src/api/collaboration.ts` (import from shared)

- [ ] **Step 1: Create shared api-types package**

```typescript
// packages/api-types/src/index.ts
export interface ContextChunk {
  chunk_id: string;
  source_type: string;
  source_id: string;
  // ... all fields
}

export interface MeetingBriefRequest {
  // ... all fields
}

export interface MeetingBriefResponse {
  // ... all fields
}
```

- [ ] **Step 2: Update web to use shared types**

```typescript
// web/src/api/types.ts
export * from '@allcallall/api-types';
```

- [ ] **Step 3: Update mobile to use shared types**

```typescript
// mobile/src/api/types.ts
export * from '@allcallall/api-types';
```

- [ ] **Step 4: Commit**

```bash
git add packages/api-types/
git commit -m "feat(typescript): create shared API types package"
```

---

### Task 12: Environment Variable Centralization

**Files:**
- Create: `.env.template` (single source of truth)
- Modify: `infra/docker-compose.yml` (reference template)
- Modify: `infra/docker-compose.production.yml` (reference template)
- Modify: `backend/.env.example` (reference template)

- [ ] **Step 1: Create .env.template**

```bash
# .env.template — Single source of truth for all env vars
# Copy to .env and fill in values

# Database
DB_DSN=allcallall:allcallallpass@tcp(127.0.0.1:3306)/allcallall?charset=utf8mb4&parseTime=True&loc=Local

# Redis
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=

# Auth
JWT_SECRET=your-jwt-secret-here

# Agent Runtime
AGENT_PROVIDER=rules
PY_AGENT_RUNTIME_BASE_URL=http://127.0.0.1:8090
AGENT_RUNTIME_TOOL_TOKEN=local-agent-runtime-tool-token

# RAG Runtime
PY_RAG_RUNTIME_BASE_URL=http://127.0.0.1:8091
```

- [ ] **Step 2: Update docker-compose files to use template**

- [ ] **Step 3: Commit**

```bash
git add .env.template infra/
git commit -m "feat(infra): centralize env vars with single .env.template"
```

---

### Task 13: Infrastructure Fixes

**Files:**
- Modify: `infra/nginx.conf` (remove hardcoded IP)
- Modify: `infra/start.sh` (remove hardcoded credentials)
- Modify: `scripts/development/start-services.sh` (add health checks)

- [ ] **Step 1: Fix nginx.conf**

```nginx
# Before
server_name 121.40.22.172;

# After
server_name $host;
```

- [ ] **Step 2: Fix start.sh health checks**

```bash
# Before
sleep 30

# After
wait-for-it -h 127.0.0.1 -p 3306 -t 30
```

- [ ] **Step 3: Commit**

```bash
git add infra/
git commit -m "fix(infra): remove hardcoded IPs and credentials, add proper health checks"
```

---

## Wave 4: Verification (After All Waves)

### Task 14: Run Full Test Suite

- [ ] **Step 1: Run Python tests**

```bash
cd agent-runtime && pytest -v
cd rag-runtime && pytest -v
```

- [ ] **Step 2: Run Go tests**

```bash
cd backend && go test ./...
```

- [ ] **Step 3: Run TypeScript checks**

```bash
cd web && npm run typecheck && npm run lint
cd mobile && npx tsc --noEmit
```

- [ ] **Step 4: Run CI checks**

```bash
make test-backend
```

---

## Scenarios (the contract)

| # | Scenario | Pass Condition | Evidence |
|---|----------|----------------|----------|
| S1 | Python shared package works | `from shared.models import ContextChunk` succeeds | pytest output |
| S2 | Python config loads correctly | `config.provider == "rules"` with default env | pytest output |
| S3 | Graph.py split maintains functionality | All existing tests pass | pytest output |
| S4 | Go commerce repository works | `repo.GetEntitlement()` returns data | go test output |
| S5 | Go knowledge repository works | `repo.GetSource()` returns data | go test output |
| S6 | Eval code separated | `go run cmd/agent-eval/main.go` works | CLI output |
| S7 | API types shared | Both web and mobile compile | tsc output |
| S8 | Env vars centralized | `.env.template` has all vars | grep output |
| S9 | No hardcoded IPs | `grep -r "121.40.22.172" infra/` returns empty | grep output |
| S10 | CI passes | GitHub Actions all green | CI output |

---

## Learnings (patterns / pitfalls for next turn)

TBD — updated as work progresses
