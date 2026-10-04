# Open-Source Repository Modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Modernize the `AllCallAll` and `allcallall-agent-runtime` repositories so their documentation, governance, and internal code boundaries are clear to public contributors while preserving all existing runtime contracts.

**Architecture:** Keep the two repositories independently buildable and retain their existing product/runtime ownership boundary. Establish product-first documentation and governance first, then extract thin composition roots and split oversized modules behind compatibility exports in reviewable phases.

**Tech Stack:** Go 1.26, Gin, React 18, TypeScript, Vite, Expo/React Native, Electron, Python 3.11+, FastAPI, LangGraph, pytest, Ruff, mypy, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-04-open-source-repository-modernization-design.md`

## Global Constraints

- The project is presented as a public open-source product first.
- English is the canonical technical-documentation language; each repository provides a concise `README.zh-CN.md`.
- The main repository adopts the MIT License used by `allcallall-agent-runtime`.
- The repositories remain independently buildable, releasable, and mergeable.
- Go continues to own product data, permissions, approvals, audit, and write execution.
- Python continues to own orchestration, retrieval, grounding, citations, tool proposals, and evaluation.
- Preserve HTTP routes, status codes, JSON/OpenAPI/JSON-Schema contracts, environment variables, command names, deep links, migration semantics, and published Python imports.
- Do not hand-edit generated protobuf, generated schemas, or generated evaluation reports.
- Preserve untracked and ignored user files, including `AllCallAll/patches/` and local output directories.
- Dependency installation runs from the `AllCallAll` repository root; do not introduce workspace lockfiles under `web/` or `mobile/`.

---

### Task 1: Record Baselines and Add Documentation Characterization Tests

**Files:**
- Modify: `scripts/check-docs-consistency.mjs`
- Create: `scripts/check-docs-consistency.test.mjs`
- Modify: `package.json`
- Create: `../allcallall-agent-runtime/scripts/check_docs.py`
- Create: `../allcallall-agent-runtime/tests/test_docs.py`
- Modify: `../allcallall-agent-runtime/Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `../allcallall-agent-runtime/.github/workflows/ci.yml`

**Interfaces:**
- Produces main command: `npm run docs:check`
- Produces runtime command: `make docs-check`
- Checks maintained Markdown links, document indexing, archive exclusion, one-H1 structure, heading progression, fenced-code language tags, and compatibility-pointer targets.
- Archive and generated directories are allowed to be absent from the maintained-document index.

- [ ] **Step 1: Capture clean baseline results**

Run:

```bash
cd /Users/byzantium/github/AllCallAll
git status --short
node scripts/check-docs-consistency.mjs
cd ../allcallall-agent-runtime
git status --short
make contracts-check
```

Expected: existing checks pass; the main repository reports only the pre-existing untracked `patches/` path.

- [ ] **Step 2: Write failing main documentation-checker tests**

Add table-driven temporary-directory cases to `scripts/check-docs-consistency.test.mjs` for:

````javascript
[
  ["broken relative link", "docs/guide.md", "[missing](missing.md)"],
  ["unindexed maintained doc", "docs/guides/orphan.md", "# Orphan"],
  ["archive excluded from index requirement", "docs/archive/report.md", "# Historical report"],
  ["untyped fence", "docs/guide.md", "# Guide\n\n```\nmake test\n```"],
]
````

- [ ] **Step 3: Run the main tests and verify failure**

Run: `node --test scripts/check-docs-consistency.test.mjs`

Expected: FAIL because the checker does not yet export or implement the full-repository rules.

- [ ] **Step 4: Refactor the main checker into testable functions**

Export these functions from `scripts/check-docs-consistency.mjs` and retain CLI behavior:

```javascript
export function collectMarkdownFiles(root) {}
export function checkMarkdownFile(root, relativePath, source) {}
export function checkDocumentationIndex(root, maintainedFiles, indexSource) {}
export function checkDocumentationTree(root) {}
```

Exclude `docs/archive/**`, `docs/**/generated-*/**`, and compatibility pointer files from index-membership requirements while still checking links inside pointer files.

- [ ] **Step 5: Add the runtime checker and tests**

Implement equivalent Python interfaces in `scripts/check_docs.py`:
`collect_markdown_files(root: Path) -> list[Path]`,
`check_markdown_file(root: Path, path: Path) -> list[str]`,
`check_documentation_index(root: Path, files: list[Path]) -> list[str]`, and
`main() -> int`.

Test broken links, orphan maintained documents, archive exclusion, headings, and code fences in `tests/test_docs.py` using `tmp_path`.

- [ ] **Step 6: Wire commands and CI**

Add to the main root `package.json`:

```json
"docs:check": "node scripts/check-docs-consistency.mjs",
"test:docs": "node --test scripts/check-docs-consistency.test.mjs"
```

Add to the runtime `Makefile`:

```make
docs-check:
	$(PYTHON) scripts/check_docs.py
```

Make each repository's CI run its local documentation checker without requiring the sibling checkout.

- [ ] **Step 7: Verify and commit independently**

Run:

```bash
cd /Users/byzantium/github/AllCallAll
npm run test:docs
npm run docs:check
git add scripts/check-docs-consistency.mjs scripts/check-docs-consistency.test.mjs package.json .github/workflows/ci.yml
git commit -m "build(docs): enforce documentation structure"

cd ../allcallall-agent-runtime
.venv/bin/python -m pytest tests/test_docs.py -q
make docs-check
git add scripts/check_docs.py tests/test_docs.py Makefile .github/workflows/ci.yml
git commit -m "build(docs): enforce runtime documentation structure"
```

### Task 2: Modernize Main Repository Governance and Root Entry Points

**Files:**
- Create: `LICENSE`
- Create: `README.zh-CN.md`
- Create: `CODE_OF_CONDUCT.md`
- Create: `SUPPORT.md`
- Create: `.github/ISSUE_TEMPLATE/bug_report.yml`
- Create: `.github/ISSUE_TEMPLATE/feature_request.yml`
- Create: `.github/ISSUE_TEMPLATE/documentation.yml`
- Create: `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/pull_request_template.md`
- Modify: `README.md`
- Modify: `CONTRIBUTING.md`
- Modify: `SECURITY.md`
- Modify: `INDEX.md`

**Interfaces:**
- `README.md` is the canonical public landing page.
- `README.zh-CN.md` is a concise translated overview and links to canonical English docs.
- `INDEX.md` becomes a compatibility pointer to `docs/README.md`.
- Security reports continue through `SECURITY.md`, never through public issues.

- [ ] **Step 1: Write governance-file assertions**

Extend `scripts/check-docs-consistency.test.mjs` with a repository fixture asserting these files exist and are linked from README:

```javascript
const governanceFiles = [
  "LICENSE",
  "CONTRIBUTING.md",
  "SECURITY.md",
  "CODE_OF_CONDUCT.md",
  "SUPPORT.md",
];
```

- [ ] **Step 2: Verify the new test fails**

Run: `npm run test:docs`

Expected: FAIL because the main repository lacks `LICENSE`, `CODE_OF_CONDUCT.md`, and `SUPPORT.md`.

- [ ] **Step 3: Add MIT license and governance documents**

Use the same MIT text and `Copyright (c) 2026 XianingY` attribution as the runtime repository. Use Contributor Covenant 2.1 language, link enforcement reports to the private security/support channel described by `SECURITY.md`, and state supported help channels without promising response times.

- [ ] **Step 4: Rewrite the English README**

Use exactly this top-level section order:

```markdown
## Why AllCallAll
## What Is Included
## Architecture
## Quick Start
## Repository Map
## Documentation
## Project Status
## Contributing
## Security and Support
## License
```

Describe production Web as `web/`, Expo as the native client, Electron as a thin shell, and the sibling Python repository as the Agent/RAG intelligence layer. Do not present hidden realtime translation UI or deterministic evaluation metrics as general model-quality claims.

- [ ] **Step 5: Add the Chinese overview and compatibility index**

`README.zh-CN.md` mirrors the problem statement, architecture, quick start, and key links, and begins with:

```markdown
> English documentation is canonical. See [README.md](README.md).
```

Replace `INDEX.md` with a short link to `docs/README.md` and the sibling runtime index.

- [ ] **Step 6: Add issue and pull-request templates**

The PR template requires checkboxes for tests, documentation, contract impact, migrations, and security/privacy impact. Disable blank issues and direct vulnerability reports to `SECURITY.md`.

- [ ] **Step 7: Verify and commit**

Run:

```bash
npm run test:docs
npm run docs:check
git diff --check
git add LICENSE README.md README.zh-CN.md CONTRIBUTING.md SECURITY.md CODE_OF_CONDUCT.md SUPPORT.md INDEX.md .github
git commit -m "docs: modernize open-source project governance"
```

### Task 3: Modernize Runtime Repository Governance and Root Entry Points

**Files:**
- Create: `../allcallall-agent-runtime/README.zh-CN.md`
- Create: `../allcallall-agent-runtime/CONTRIBUTING.md`
- Create: `../allcallall-agent-runtime/SECURITY.md`
- Create: `../allcallall-agent-runtime/CODE_OF_CONDUCT.md`
- Create: `../allcallall-agent-runtime/SUPPORT.md`
- Create: `../allcallall-agent-runtime/.github/ISSUE_TEMPLATE/bug_report.yml`
- Create: `../allcallall-agent-runtime/.github/ISSUE_TEMPLATE/feature_request.yml`
- Create: `../allcallall-agent-runtime/.github/ISSUE_TEMPLATE/documentation.yml`
- Create: `../allcallall-agent-runtime/.github/ISSUE_TEMPLATE/config.yml`
- Create: `../allcallall-agent-runtime/.github/pull_request_template.md`
- Modify: `../allcallall-agent-runtime/README.md`
- Modify: `../allcallall-agent-runtime/INDEX.md`

**Interfaces:**
- Runtime README describes only runtime capabilities and clearly links product behavior to the main repository.
- Deterministic eval metrics always link methodology and fixture scope.
- Existing MIT license remains unchanged.

- [ ] **Step 1: Add failing governance assertions to `tests/test_docs.py`**

Assert the runtime repository contains and links `LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, and `SUPPORT.md`.

- [ ] **Step 2: Run the focused failure**

Run: `.venv/bin/python -m pytest tests/test_docs.py -q`

Expected: FAIL on missing runtime governance files.

- [ ] **Step 3: Add governance and GitHub templates**

Use the same policies and template fields as the main repository, adapting commands to `make install-dev`, `make verify`, and runtime-specific security boundaries.

- [ ] **Step 4: Rewrite runtime root documentation**

Use this README order:

```markdown
## Why a Separate Runtime
## Services and Packages
## Safety Boundary
## Quick Start
## Runtime APIs
## Evaluation
## Documentation
## Contributing
## Security and Support
## License
```

Describe metrics as deterministic regression evidence, not open-domain quality claims. Replace `INDEX.md` with a compatibility pointer to `docs/README.md`.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd /Users/byzantium/github/allcallall-agent-runtime
.venv/bin/python -m pytest tests/test_docs.py -q
make docs-check
git diff --check
git add README.md README.zh-CN.md CONTRIBUTING.md SECURITY.md CODE_OF_CONDUCT.md SUPPORT.md INDEX.md .github
git commit -m "docs: modernize runtime project governance"
```

### Task 4: Reorganize Main Repository Documentation

**Files:**
- Create: `docs/architecture/system-overview.md`
- Create: `docs/architecture/backend.md`
- Create: `docs/architecture/agent-platform.md`
- Create: `docs/architecture/workers.md`
- Create: `docs/guides/development/`
- Create: `docs/guides/deployment/`
- Create: `docs/guides/operations/`
- Create: `docs/reference/api/`
- Create: `docs/reference/configuration/`
- Create: `docs/reference/security/`
- Create: `docs/archive/portfolio/`
- Create: `docs/archive/reports/`
- Create: `docs/archive/plans/`
- Modify: `docs/README.md`
- Modify: all moved documents and their inbound links
- Preserve as pointers: important old paths under `docs/interview/`, `docs/api/`, `docs/configuration/`, `docs/deployment/`, and `docs/superpowers/plans/`

**Interfaces:**
- Maintained technical truth appears only in the product hierarchy.
- Old high-traffic paths remain Markdown pointer documents containing one H1, a historical/moved notice, and a relative link to the canonical location.
- Archived documents begin with `> **Archive status:** Historical material; not part of the maintained product documentation.` and are excluded from primary navigation.

- [ ] **Step 1: Add a migration-map test**

Create an exported mapping in `scripts/check-docs-consistency.mjs` and assert every source and target exists:

```javascript
export const DOCUMENT_MOVES = {
  "docs/interview/system-design.md": "docs/architecture/system-overview.md",
  "docs/interview/backend-deep-dive.md": "docs/architecture/backend.md",
  "docs/interview/ai-agent-design.md": "docs/architecture/agent-platform.md",
  "docs/interview/worker-runtime.md": "docs/architecture/workers.md",
};
```

- [ ] **Step 2: Build canonical architecture documents**

Rewrite the four target documents around maintained behavior. Remove interview prompts, resume framing, unsupported future claims, and duplicate configuration tables. Link the sibling runtime for Python-owned behavior.

- [ ] **Step 3: Move task-oriented and reference documents**

Move development, deployment, maintenance, API, database, configuration, privacy, security, and observability material into their corresponding `guides` or `reference` destinations. Update relative links using repository-root-aware checks.

- [ ] **Step 4: Archive non-product material**

Move interview questions, resume/JD-fit content, generated reports, dated audits, coverage snapshots, performance snapshots, optimization roadmaps, service-evolution plans, and old Superpowers plans into the matching archive category. Add archive-status notices without rewriting generated payloads.

- [ ] **Step 5: Replace old high-traffic documents with pointers**

Use this pointer form:

```markdown
# Document Moved

> This compatibility page preserves an older link. The maintained document is
> [System Overview](../architecture/system-overview.md).
```

- [ ] **Step 6: Rewrite `docs/README.md` as the canonical product index**

Order sections as Getting Started, Architecture, Guides, Reference, Client Documentation, Runtime Repository, and Archive. Do not enumerate individual portfolio documents in primary navigation.

- [ ] **Step 7: Verify and commit**

Run:

```bash
npm run test:docs
npm run docs:check
make web-contract-check
git diff --check
git add docs README.md backend/README.md mobile/README.md desktop/README.md contracts/README.md deploy/README.md infra/README.md scripts/README.md
git commit -m "docs: establish product-first documentation architecture"
```

### Task 5: Reorganize Runtime Documentation

**Files:**
- Create: `../allcallall-agent-runtime/docs/README.md`
- Create: `../allcallall-agent-runtime/docs/getting-started/quick-start.md`
- Create: `../allcallall-agent-runtime/docs/architecture/`
- Create: `../allcallall-agent-runtime/docs/guides/`
- Create: `../allcallall-agent-runtime/docs/reference/`
- Create: `../allcallall-agent-runtime/docs/evaluation/`
- Create: `../allcallall-agent-runtime/docs/archive/portfolio/`
- Create: `../allcallall-agent-runtime/docs/archive/reports/`
- Create: `../allcallall-agent-runtime/docs/archive/plans/`
- Modify: all runtime Markdown links and service READMEs

**Interfaces:**
- `docs/README.md` becomes the canonical runtime index.
- Configuration remains canonical in `docs/reference/configuration.md`.
- Tool Bridge remains canonical in `docs/reference/tool-bridge-protocol.md`.
- Eval claims link to `docs/evaluation/methodology.md`.

- [ ] **Step 1: Add expected canonical-path tests**

Assert these paths exist and are indexed:

```python
CANONICAL_DOCS = {
    "docs/architecture/overview.md",
    "docs/architecture/harness.md",
    "docs/reference/configuration.md",
    "docs/reference/tool-bridge-protocol.md",
    "docs/evaluation/methodology.md",
    "docs/evaluation/engineering-harness.md",
}
```

- [ ] **Step 2: Move and normalize maintained runtime docs**

Map architecture, harness, loops, check agents, context compression, skill registry, MCP queue, configuration, integration, protocol, and evaluation documents into their canonical categories. Update service README links.

- [ ] **Step 3: Archive portfolio and dated design material**

Move resume metrics, generated portfolio reports, manual portfolio evidence, and the dated deep-optimization design into archive categories with status labels.

- [ ] **Step 4: Add compatibility pointers and canonical index**

Keep pointer files at externally referenced old paths and make `docs/README.md` the only detailed index.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd /Users/byzantium/github/allcallall-agent-runtime
.venv/bin/python -m pytest tests/test_docs.py -q
make docs-check
make contracts-check
git diff --check
git add docs README.md INDEX.md services/*/README.md contracts/README.md
git commit -m "docs: establish runtime documentation architecture"
```

### Task 6: Extract Go Server Bootstrap and Split Configuration

**Files:**
- Create: `backend/internal/bootstrap/server.go`
- Create: `backend/internal/bootstrap/server_test.go`
- Create: `backend/internal/bootstrap/lifecycle.go`
- Modify: `backend/cmd/server/main.go`
- Create: `backend/internal/config/core.go`
- Create: `backend/internal/config/privacy.go`
- Create: `backend/internal/config/realtime.go`
- Create: `backend/internal/config/workers.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/README.md`
- Modify: `docs/architecture/backend.md`

**Interfaces:**
- `config.Load() (*config.Config, error)` remains unchanged.
- Existing exported config type names and YAML/environment tags remain unchanged.
- `bootstrap.RunServer(ctx context.Context, cfg *config.Config, log zerolog.Logger) error` owns process assembly and lifecycle.
- `cmd/server/main.go` only loads environment/configuration, creates the root signal context, calls `RunServer`, and maps failure to process exit.

- [ ] **Step 1: Write bootstrap characterization tests**

Add a test around an injectable listener and shutdown context:

```go
func TestRunServerStopsWhenContextIsCancelled(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    err := RunServer(ctx, testConfig(), zerolog.Nop())
    require.NoError(t, err)
}
```

Add compile-time assertions in config tests that `Config` fields and key exported types retain their names.

- [ ] **Step 2: Run focused tests and confirm the bootstrap test fails**

Run: `cd backend && go test ./internal/bootstrap ./internal/config`

Expected: FAIL because `internal/bootstrap` and `RunServer` do not exist.

- [ ] **Step 3: Move server assembly without changing behavior**

Move dependency creation, handler registration, worker startup, HTTP serving, drain ordering, and shutdown from `cmd/server/main.go` into `bootstrap.RunServer`. Keep `runtime.ApplyPrivacyPolicies` in the shared startup path and preserve the probe/TLS middleware split.

- [ ] **Step 4: Split config declarations within the same package**

Keep environment expansion, singleton loading, and `postProcess` orchestration in `config.go`. Move structs and feature-local defaults into the new files without changing package name, exported names, tags, or default values.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd backend
gofmt -w cmd/server internal/bootstrap internal/config
go test ./internal/bootstrap ./internal/config ./internal/runtime ./internal/server ./internal/handlers
go build ./cmd/server
cd ..
git add backend/cmd/server backend/internal/bootstrap backend/internal/config backend/README.md docs/architecture/backend.md
git commit -m "refactor(backend): extract server bootstrap boundaries"
```

### Task 7: Split Web Application Routing by Feature

**Files:**
- Create: `web/src/app/routes/publicRoutes.tsx`
- Create: `web/src/app/routes/workspaceRoutes.tsx`
- Create: `web/src/app/routes/meetingRoutes.tsx`
- Create: `web/src/app/routes/legacyRoutes.tsx`
- Create: `web/src/app/routes/routes.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/main.tsx`
- Create: `web/src/app/AppProviders.tsx`
- Modify: `docs/guides/development/web-desktop-workflow.md`

**Interfaces:**
- All existing route paths and redirect behavior remain unchanged.
- `AppProviders` owns the current provider nesting and accepts `children: React.ReactNode`.
- Route modules export React fragments or route arrays consumed only by `App`.

- [ ] **Step 1: Write route compatibility tests**

Add memory-router tests covering:

```typescript
const routes = [
  "/login",
  "/invite/test-code",
  "/inbox",
  "/meetings/42/preflight",
  "/meetings/42",
  "/rooms/42",
  "/settings/profile",
];
```

Assert `/rooms/42` redirects to `/meetings/42` and unknown paths redirect to `/inbox`.

- [ ] **Step 2: Run the focused test before extraction**

Run: `cd web && npx vitest run src/app/routes/routes.test.tsx`

Expected: FAIL because the route modules do not exist.

- [ ] **Step 3: Extract provider composition**

Move `QueryClientProvider`, `AuthProvider`, `OrganizationProvider`, `CallProvider`, and `ChatRealtimeProvider` nesting into `AppProviders`. Keep `BrowserRouter` outside feature providers only if tests confirm no provider consumes router context; otherwise preserve its current relative position.

- [ ] **Step 4: Extract route groups**

Move anonymous/authentication routes, workspace routes, meeting routes, and compatibility redirects into the four route modules. Preserve lazy import targets and fallbacks exactly.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd web
npm run typecheck
npx vitest run src/app
npx vitest run
cd ..
git add web/src/app web/src/main.tsx docs/guides/development/web-desktop-workflow.md
git commit -m "refactor(web): clarify application and route boundaries"
```

### Task 8: Extract Mobile Startup, Providers, and Deep-Link Handling

**Files:**
- Create: `mobile/src/app/AppProviders.tsx`
- Create: `mobile/src/app/linking.ts`
- Create: `mobile/src/app/useDeepLinks.ts`
- Create: `mobile/src/app/__tests__/linking.test.ts`
- Modify: `mobile/App.tsx`
- Modify: `mobile/package.json`
- Modify: `docs/mobile/README.md`

**Interfaces:**
- Export `linking: LinkingOptions<RootStackParamList>`.
- Export `resolveDeepLink(url: string | null | undefined): PendingIntent | null` as pure logic.
- Export `useDeepLinks(): void` for React-Native subscription and deferred navigation.
- `AppProviders` preserves the existing provider order.

- [ ] **Step 1: Write pure deep-link tests**

Cover:

```typescript
assert.deepEqual(resolveDeepLink("allcallall://rooms/42"), { kind: "room", roomId: 42 });
assert.deepEqual(resolveDeepLink("allcallall://conversations/7"), { kind: "conversation", conversationId: 7 });
assert.deepEqual(resolveDeepLink("allcallall://invite/abc"), { kind: "invitation", code: "abc" });
assert.equal(resolveDeepLink("https://example.com"), null);
```

- [ ] **Step 2: Add the pure test to `test:unit` and verify failure**

Run: `cd mobile && npm run test:unit`

Expected: FAIL because `resolveDeepLink` does not exist.

- [ ] **Step 3: Extract linking and subscription behavior**

Move the navigation linking config unchanged. Implement `resolveDeepLink` using the existing parsers, and keep the current readiness check and `setPendingIntent` fallback inside `useDeepLinks`.

- [ ] **Step 4: Extract provider composition**

Move the current provider nesting into `AppProviders` without reordering providers. Leave `NavigationContainer`, `VersionGate`, `AppNavigator`, `CallOverlay`, and `StatusBar` behavior unchanged.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd mobile
npm run typecheck
npm run test:unit
npm run test:jest
cd ..
git add mobile/App.tsx mobile/src/app mobile/package.json docs/mobile/README.md
git commit -m "refactor(mobile): isolate startup and deep-link behavior"
```

### Task 9: Organize the Electron Shell

**Files:**
- Create: `desktop/src/main/index.cjs`
- Create: `desktop/src/preload/index.cjs`
- Create: `desktop/src/shared/route-utils.cjs`
- Modify: `desktop/package.json`
- Modify: `desktop/scripts/deep-link-check.cjs`
- Modify: `desktop/scripts/permission-check.cjs`
- Replace with compatibility loaders: `desktop/main.cjs`, `desktop/preload.cjs`, `desktop/route-utils.cjs`
- Modify: `desktop/README.md`

**Interfaces:**
- Electron package main remains launchable with `electron .`.
- Root compatibility loaders require the exact canonical files: `./src/main/index.cjs`, `./src/preload/index.cjs`, and `./src/shared/route-utils.cjs`.
- `createRouteHelpers` export and all `allcallall://` normalization remain unchanged.

- [ ] **Step 1: Extend script checks to assert compatibility loaders**

Make deep-link and permission checks import both the canonical module and root compatibility module and assert identical exported behavior.

- [ ] **Step 2: Run checks before moving files**

Run: `cd desktop && npm run check`

Expected: current checks pass, establishing the baseline.

- [ ] **Step 3: Move implementation and add root loaders**

Use root files containing only:

```javascript
require("./src/main/index.cjs");
```

for the process entrypoint, with equivalent exports for preload and route helpers. Update package build file inclusion to include `src/**/*` and the compatibility loaders.

- [ ] **Step 4: Verify and commit**

Run:

```bash
cd desktop
npm run check
npm run build
cd ..
git add desktop
git commit -m "refactor(desktop): organize shell process boundaries"
```

### Task 10: Split Agent Runtime API, Models, and Orchestration

**Files:**
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/api/__init__.py`
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/api/app.py`
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/api/routes.py`
- Modify: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/main.py`
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/orchestration/__init__.py`
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py`
- Modify: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/harness.py`
- Replace module with package: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/models/`
- Modify: affected runtime tests and `services/agent-runtime/README.md`

**Interfaces:**
- `allcallall_agent_runtime.main:app` remains the Uvicorn entrypoint.
- `allcallall_agent_runtime.harness.AllCallAllAgentHarness`, `HarnessTimeoutExceeded`, and `get_harness` remain importable.
- Every name currently imported from `allcallall_agent_runtime.models` remains re-exported from `models/__init__.py`.
- `create_app() -> FastAPI` becomes the testable API factory.

- [ ] **Step 1: Write compatibility import and route tests**

Add tests asserting:

```python
from allcallall_agent_runtime.main import app
from allcallall_agent_runtime.harness import AllCallAllAgentHarness, get_harness
from allcallall_agent_runtime.models import AgentRunRequest, WorkflowResponse

assert app.title == "AllCallAll Agent Runtime"
assert callable(get_harness)
assert AllCallAllAgentHarness.name == "allcallall_v1"
```

Use FastAPI TestClient to snapshot route method/path pairs before extraction.

- [ ] **Step 2: Run focused tests**

Run: `cd services/agent-runtime && ../../.venv/bin/python -m pytest tests/test_api.py tests/test_harness_factory.py -q`

Expected: baseline tests pass; newly added `create_app` assertion fails.

- [ ] **Step 3: Extract API factory and routes**

Move endpoint functions and queue-worker lifecycle into `api/routes.py` and `api/app.py`. Keep `main.py` as:

```python
from .api.app import create_app

app = create_app()
```

- [ ] **Step 4: Convert models into focused modules**

Create `models/context.py`, `models/retrieval.py`, `models/tools.py`, `models/trace.py`, and `models/workflows.py`. Re-export all previous names from `models/__init__.py`; update internal imports only after compatibility tests pass.

- [ ] **Step 5: Move harness implementation behind a compatibility module**

Move the class implementation to `orchestration/harness.py`. Keep root `harness.py` as explicit re-exports, not wildcard imports:

```python
from .orchestration.harness import AllCallAllAgentHarness, HarnessTimeoutExceeded, get_harness

__all__ = ["AllCallAllAgentHarness", "HarnessTimeoutExceeded", "get_harness"]
```

- [ ] **Step 6: Verify and commit**

Run:

```bash
cd /Users/byzantium/github/allcallall-agent-runtime
.venv/bin/python -m pytest services/agent-runtime/tests -q
.venv/bin/python -m ruff check services/agent-runtime
cd services/agent-runtime && ../../.venv/bin/python -m mypy .
cd ../..
git add services/agent-runtime docs/architecture
git commit -m "refactor(agent-runtime): clarify API and orchestration boundaries"
```

### Task 11: Split Persistence and RAG Boundaries; Rename Reference MCP Service

**Files:**
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/persistence/__init__.py`
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/persistence/mysql_pool.py`
- Create: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/persistence/mysql_schema.py`
- Modify: `../allcallall-agent-runtime/services/agent-runtime/allcallall_agent_runtime/checkpoint/mysql.py`
- Create: `../allcallall-agent-runtime/services/rag-runtime/allcallall_rag_runtime/api.py`
- Create: `../allcallall-agent-runtime/services/rag-runtime/allcallall_rag_runtime/pipeline.py`
- Modify: `../allcallall-agent-runtime/services/rag-runtime/allcallall_rag_runtime/main.py`
- Move: `../allcallall-agent-runtime/services/interview-mcp/` to `../allcallall-agent-runtime/services/reference-mcp/`
- Create compatibility package: `../allcallall-agent-runtime/services/reference-mcp/allcallall_interview_mcp/`
- Modify: `../allcallall-agent-runtime/pyproject.toml`
- Modify: `../allcallall-agent-runtime/Makefile`
- Modify: sandbox/reference MCP tests and docs

**Interfaces:**
- `checkpoint.mysql.MySQLCheckpointSaver` and exception imports remain unchanged.
- `allcallall_rag_runtime.main:app` remains unchanged.
- RAG route method/path pairs and response models remain unchanged.
- The new distribution is `allcallall-reference-mcp`; `allcallall_interview_mcp` remains as a compatibility import package for one deprecation cycle.
- Existing Make targets remain accepted aliases.

- [ ] **Step 1: Write persistence, RAG-route, and compatibility-import tests**

Assert the old imports and route sets still work:

```python
from allcallall_agent_runtime.checkpoint.mysql import MySQLCheckpointSaver
from allcallall_rag_runtime.main import app
from allcallall_interview_mcp.main import mcp
```

- [ ] **Step 2: Extract MySQL pool and schema responsibilities**

Move connection creation/pooling into `persistence/mysql_pool.py` and idempotent DDL into `persistence/mysql_schema.py`. Keep transaction semantics, limits, exception types, and public saver class in `checkpoint/mysql.py`.

- [ ] **Step 3: Extract RAG API mapping and pipeline selection**

Move FastAPI route construction to `api.py` and the Go Bridge/Qdrant/inline source-selection logic to `pipeline.py`. Keep retrieval algorithms in `retrieval.py`.

- [ ] **Step 4: Rename the MCP service with compatibility**

Rename directory and distribution metadata to reference terminology. Provide `allcallall_interview_mcp` modules that import explicit names from `allcallall_reference_mcp`. Update workspace members, Make targets, Docker/examples, and sandbox tests without weakening exact-host or HTTPS validation.

- [ ] **Step 5: Verify and commit**

Run:

```bash
cd /Users/byzantium/github/allcallall-agent-runtime
.venv/bin/python -m pytest services/agent-runtime/tests services/rag-runtime/tests services/sandbox-runner/tests services/reference-mcp/tests -q
.venv/bin/python -m ruff check services packages scripts
make typecheck
make contracts-check
git add services packages pyproject.toml Makefile docs examples
git commit -m "refactor(runtime): separate persistence retrieval and reference MCP"
```

### Task 12: Full Cross-Repository Verification and Final Documentation Audit

**Files:**
- Modify only files required to fix verification failures introduced by Tasks 1–11.
- Update: `AGENTS.md`, `docs/README.md`, and runtime `docs/README.md` if command or path verification reveals drift.

**Interfaces:**
- Both repositories finish clean except for explicitly pre-existing untracked user files.
- All compatibility contracts in the design spec have executable evidence.

- [ ] **Step 1: Run main repository full verification**

Run:

```bash
cd /Users/byzantium/github/AllCallAll
make verify-full
npm run docs:check
npm run test:docs
```

Expected: PASS.

- [ ] **Step 2: Run runtime full verification**

Run:

```bash
cd /Users/byzantium/github/allcallall-agent-runtime
make verify
make docs-check
```

Expected: PASS.

- [ ] **Step 3: Audit contracts and compatibility names**

Run repository searches for removed routes, commands, environment variables, old Python imports, and moved documentation paths. Confirm compatibility pointers or exports exist for every externally referenced old name.

- [ ] **Step 4: Audit repository state**

Run:

```bash
git -C /Users/byzantium/github/AllCallAll status --short
git -C /Users/byzantium/github/allcallall-agent-runtime status --short
```

Expected: no unintended files; `AllCallAll/patches/` remains untouched and untracked if it was still present.

- [ ] **Step 5: Commit final verification fixes separately in each repository**

Use descriptive commit messages scoped to the actual verification correction. Do not create an empty final commit.
