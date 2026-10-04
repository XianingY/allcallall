# Open-Source Repository Modernization Design

**Status:** Approved for implementation planning

**Date:** 2026-10-04

**Repositories:** `AllCallAll` and sibling `allcallall-agent-runtime`

## Context

AllCallAll is a public realtime collaboration and AI Agent product split across
two repositories:

- `AllCallAll` owns the Go product backend, React Web client, Expo Mobile app,
  Electron shell, infrastructure, product data, permissions, approvals, audit,
  and write execution.
- `allcallall-agent-runtime` owns Python Agent orchestration, LangGraph
  workflows, RAG, reranking, grounding, traces, citations, tool proposals, and
  evaluation.

Both repositories contain substantial working software, but their current
presentation mixes product documentation with interview notes, resume material,
historical plans, generated evaluation reports, and dated engineering audits.
Several composition roots and feature files are also large enough that a new
contributor must understand unrelated responsibilities before making a safe
change.

The modernization makes the product path obvious without changing the product's
runtime behavior or the ownership boundary between the Go and Python systems.

## Goals

- Make both repositories understandable and runnable as independent public
  open-source projects.
- Give first-time contributors a short path from the root README to setup,
  architecture, testing, contribution, security, and support information.
- Establish one maintained documentation hierarchy in each repository.
- Remove interview, resume, generated evidence, and historical plans from the
  primary product navigation while preserving them as clearly labeled archives.
- Reduce the cognitive load of composition roots and oversized source files by
  splitting them along stable responsibilities.
- Preserve public behavior: HTTP routes, JSON and OpenAPI contracts, deep links,
  environment variables, command names, database semantics, and published
  Python imports.
- Add automated checks that prevent documentation and repository structure from
  drifting again.

## Non-goals

- Merging the two Git repositories.
- Moving the main repository into a new `apps/` hierarchy.
- Replacing the Go/Python service boundary or moving product writes into Python.
- Redesigning product features, user flows, network protocols, or schemas.
- Sharing UI code between React DOM and React Native merely to reduce file
  count.
- Rewriting stable modules solely to make directory trees look symmetrical.
- Hand-editing generated protobuf, schema, or evaluation output.

## Design Principles

1. **Public product first.** Product use, architecture, operation, and
   contribution material appears before portfolio or historical material.
2. **One fact, one owner.** Configuration, contracts, and architectural facts
   have a single canonical document; other documents link to it.
3. **Stable outside, clearer inside.** Internal files and packages may move,
   while externally observable behavior remains compatible.
4. **Feature and responsibility boundaries.** Code that changes together lives
   together; generic technical layers are introduced only at true shared seams.
5. **Incremental migration.** Each phase is independently reviewable, testable,
   and revertible.
6. **Evidence before deletion.** Compatibility paths and tracked artifacts are
   removed only after reference searches and tests show they are unused.

## Repository Boundaries

The repositories remain separately buildable, releasable, and mergeable.
Neither repository may require an atomic commit in the other repository.

The authoritative ownership boundary remains:

- Go owns users, organizations, conversations, meetings, transcripts,
  permissions, approvals, audit logs, durable product state, and writes.
- Python owns orchestration, retrieval planning, model/provider integration,
  grounding, citations, tool proposals, and evaluation.
- Cross-repository interaction uses the existing HTTP, Tool Bridge, OpenAPI,
  JSON Schema, and generated-contract surfaces.

The main repository provides the product-level overview and links to the Python
runtime. The Python repository documents its own runtime in full and links back
to the main product integration guide.

## Target Information Architecture

### Main repository

```text
AllCallAll/
├── README.md
├── README.zh-CN.md
├── CONTRIBUTING.md
├── SECURITY.md
├── CODE_OF_CONDUCT.md
├── SUPPORT.md
├── LICENSE
├── backend/
├── web/
├── mobile/
├── desktop/
├── packages/
├── contracts/
├── infra/
├── deploy/
├── scripts/
└── docs/
    ├── README.md
    ├── getting-started/
    ├── architecture/
    ├── guides/
    │   ├── development/
    │   ├── deployment/
    │   └── operations/
    ├── reference/
    │   ├── api/
    │   ├── configuration/
    │   └── security/
    └── archive/
        ├── portfolio/
        ├── reports/
        └── plans/
```

### Python runtime repository

```text
allcallall-agent-runtime/
├── README.md
├── README.zh-CN.md
├── CONTRIBUTING.md
├── SECURITY.md
├── CODE_OF_CONDUCT.md
├── SUPPORT.md
├── LICENSE
├── services/
├── packages/
├── contracts/
├── examples/
├── scripts/
└── docs/
    ├── README.md
    ├── getting-started/
    ├── architecture/
    ├── guides/
    ├── reference/
    ├── evaluation/
    └── archive/
        ├── portfolio/
        ├── reports/
        └── plans/
```

`docs/README.md` is the canonical documentation index in each repository.
Existing root `INDEX.md` files remain as short compatibility pointers during the
migration so previously published links continue to resolve.

## Documentation Content Model

Documentation is classified by purpose rather than by the team or event that
created it:

- **Getting started:** prerequisites, quick start, local topology, and first
  successful request.
- **Architecture:** maintained system boundaries, data flows, component
  responsibilities, security boundaries, and architectural decisions.
- **Guides:** task-oriented development, deployment, operations, demo, and
  troubleshooting procedures.
- **Reference:** APIs, configuration, protocols, schemas, data models, and
  compatibility guarantees.
- **Evaluation:** maintained methodology, reproducible commands, and clearly
  scoped current evidence for the Python runtime.
- **Archive:** portfolio/interview material, dated reports, superseded plans,
  and generated evidence not required to operate or contribute to the product.

Current interview documents that contain maintained technical truth are not
blindly archived. Their product-relevant content is rewritten into architecture,
guide, or reference documents. Interview questions, resume wording, job-fit
material, and portfolio narratives move to `archive/portfolio`. Dated code
reviews, coverage analyses, load-test snapshots, and architecture audits move to
`archive/reports`. Historical implementation plans move to `archive/plans` and
receive a visible historical-status notice.

Generated reports live in an explicitly named generated subdirectory under the
appropriate archive or evaluation area. Generated files are not hand-edited and
do not appear in the primary documentation navigation.

## Language Policy

English is the canonical language for technical and governance documentation.
Each repository provides a concise `README.zh-CN.md` that mirrors the project
overview, quick start, repository relationship, and key links. Full parallel
translations are not maintained because they create two competing sources of
truth. Existing mixed-language documents are rewritten in English; useful
Chinese-only portfolio material may remain in the archive with a language
label.

## Open-Source Governance

The main repository adopts the MIT License to match the Python runtime
repository. The copyright attribution remains consistent with the runtime
repository.

Both repositories provide:

- a concise contributor guide with exact install, test, lint, and commit
  expectations;
- a Contributor Covenant code of conduct;
- a support policy that distinguishes community help from vulnerability
  reporting;
- repository-specific security reporting instructions;
- GitHub issue templates for bugs, features, and documentation problems;
- a pull request template with test, contract, documentation, and security
  checklists.

The root README in each repository follows the same reading order: problem and
scope, capabilities and maturity, architecture, quick start, repository map,
documentation, contributing, security, support, and license. Marketing or
benchmark claims must link to reproducible evidence and state their scope.

## Main Repository Code Structure

### Go backend

`backend/cmd/*` entrypoints become thin process adapters. They parse process
arguments or environment, invoke a bootstrap constructor, run the process, and
translate startup failure into logging and exit status.

A new `backend/internal/bootstrap/` package owns reusable process assembly:

- configuration, logging, tracing, database, cache, and metrics setup;
- shared privacy and compliance policy wiring;
- API server dependency construction;
- embedded and extracted worker construction;
- lifecycle and shutdown coordination.

Domain behavior remains in existing domain-oriented packages such as
`collaboration`, `agent`, `knowledge`, `commerce`, `media`, and `user`.
`bootstrap` may depend on domain packages; domain packages must not depend on
`bootstrap`.

`backend/internal/config/config.go` is split into feature-focused files in the
same `config` package. Existing exported configuration types and `config.Load`
remain stable. Generated protobuf files are excluded from size-based refactoring.

Large handlers and services are split only where a responsibility can be named
and tested independently, such as validation, query construction, persistence,
policy evaluation, or response mapping.

### Web

The Web client converges on a feature-oriented layout:

```text
web/src/
├── app/          # composition, routes, providers, shell
├── features/     # product capabilities grouped by domain
├── shared/       # reusable UI, hooks, utilities, and test support
├── platform/     # browser/Electron runtime integration
└── i18n/
```

The current centralized route component is split into domain route modules
composed by `app`. Feature folders may contain their API adapter, page,
components, hooks, and feature-local types. Shared code must have at least two
real consumers and may not import from a feature.

### Mobile

The Mobile client follows the same conceptual boundaries without forcing file
identity with Web:

```text
mobile/src/
├── app/          # navigation, deep links, providers, startup
├── features/     # screens and feature-local API/state/services
├── shared/       # reusable native components, hooks, and utilities
├── platform/     # native platform adapters
└── i18n/
```

`App.tsx` becomes a thin composition root. Deep-link parsing and deferred
navigation remain behavior-compatible but move behind focused modules. Large
screens and contexts are split into screen composition, view components,
feature hooks, and pure state/service logic. React-Native-dependent tests remain
on Jest; pure logic remains on the explicit Node test runner list required by
the repository guide.

### Desktop

Desktop remains a thin Electron shell. Main-process, preload, and route/security
logic move under explicit `desktop/src/main`, `desktop/src/preload`, and
`desktop/src/shared` boundaries. Package entrypoints and build scripts are
updated together, while the `allcallall://` scheme and route normalization stay
unchanged.

### Shared packages

Shared packages contain stable contracts, generated API types, and clients.
They do not become a dumping ground for feature helpers, and Web/Mobile visual
components remain platform-specific.

## Python Runtime Code Structure

The Agent Runtime moves toward the following internal structure:

```text
allcallall_agent_runtime/
├── api/             # FastAPI routes, auth dependencies, request mapping
├── orchestration/   # harness, graph construction, nodes, workflow control
├── retrieval/       # retrieval planning and RAG runtime integration
├── tools/           # registry, Tool Bridge, proposal and queue abstractions
├── persistence/     # checkpoint and durable runtime adapters
├── providers/       # model/provider implementations
├── evaluation/      # deterministic eval and online-eval support
├── models/          # focused request, response, trace, and workflow models
└── config.py
```

The existing `harness.py`, `models.py`, `checkpoint/mysql.py`, and synthesis
logic are split behind compatibility re-exports. Import moves are incremental:
old published imports continue to work and receive tests until a separately
announced deprecation cycle removes them.

The RAG Runtime separates FastAPI request mapping, retrieval pipeline,
infrastructure adapters, and evaluation. Shared Pydantic contracts and scoring
utilities remain in `packages/shared`; the typed external client remains in
`packages/sdk`.

`services/interview-mcp` is repositioned as a reference/demo MCP service with a
public-purpose name. The old Make target, executable invocation, and integration
behavior remain available through a compatibility entrypoint during this
migration. Sandbox security behavior and trusted-host restrictions are not
weakened by the rename.

## Compatibility Contract

The modernization must not intentionally change:

- public HTTP methods, paths, status codes, or response fields;
- OpenAPI operation identifiers or generated shared types;
- JSON Schema files and golden fixtures unless a separately reviewed contract
  change requires regeneration;
- environment variable names or supported values;
- Go command names and Make targets;
- Mobile and Desktop deep-link schemes and normalized routes;
- migration ordering or database behavior;
- Python SDK method signatures and documented package imports;
- approval, audit, privacy, TLS, encryption, or write-execution boundaries.

Compatibility shims are explicit and tested. High-traffic moved documents retain
short pointer files for the migration period. Internal code re-exports and Make
aliases are removed only in a later, separately reviewed cleanup.

## Automated Documentation Checks

The existing main-repository documentation consistency script is expanded to
cover all maintained Markdown files, not only six root documents. Checks cover:

- relative links and referenced repository paths;
- inclusion of maintained documents in `docs/README.md`;
- exclusion of archive and generated directories from primary navigation;
- one H1 per document and non-skipped heading levels;
- language-tagged fenced code blocks;
- forbidden stale workflow and repository-path references;
- compatibility pointer targets.

The Python repository receives an equivalent check. Both CI workflows run their
local documentation checks. Cross-repository links are checked as declared
integration references without making either repository's normal CI depend on a
sibling checkout.

## Migration Sequence

1. Record baseline tests, contract checks, documentation links, public imports,
   commands, and repository status.
2. Add governance files and rewrite root READMEs without moving source code.
3. Create the target documentation hierarchy, migrate maintained content, and
   add compatibility pointers for important old paths.
4. Add documentation structure and link checks to each repository's CI.
5. Extract Go bootstrap responsibilities and split configuration while
   preserving exported APIs.
6. Split Web routing and move feature code in independently testable batches.
7. Extract Mobile startup/navigation concerns and split large screens, contexts,
   and API modules with the correct test runner for each unit.
8. Organize the Desktop shell under explicit process boundaries.
9. Split Python Agent and RAG modules behind compatibility re-exports.
10. Reposition the reference MCP service with compatible commands and security
    behavior.
11. Run complete verification in both repositories and audit compatibility
    shims; retain shims whose removal is not independently proven safe.

Each phase is committed separately. Cross-repository changes reference one
another in commit messages or documentation, but neither repository is left
unbuildable while waiting for the other commit.

## Verification

Main repository verification includes:

```bash
make verify-full
node scripts/check-docs-consistency.mjs
```

Targeted work also runs the affected backend package tests, Web typecheck and
Vitest suites, Mobile typecheck and both test runners, Desktop checks, and the
OpenAPI contract check before the full gate.

Python runtime verification includes:

```bash
make verify
```

Targeted work first runs the affected service or package's pytest, Ruff, and
mypy checks. Contract work runs generation followed by `contracts-check` and
confirms that checked-in output is deterministic.

Refactoring tests assert compatibility for routes, commands, deep links,
environment parsing, Python imports, security policy assembly, and Tool Bridge
payloads. Structural changes do not rely only on snapshot or compile success.

## Success Criteria

- A new contributor can reach setup, architecture, testing, and contribution
  instructions from the root README within three navigation steps.
- Product navigation contains no interview questions, resume material, job-fit
  content, historical implementation plans, or unlabeled generated evidence.
- Every maintained document is indexed and all checked internal links resolve.
- Both repositories use consistent governance files and the MIT License.
- Composition roots assemble dependencies but contain no domain workflows.
- Large source files are split into units with one named responsibility and
  focused tests; generated files and cohesive test fixtures are exempt.
- Main and Python repository full verification commands pass.
- Existing routes, contracts, commands, deep links, environment variables, and
  published Python imports remain compatible.

## Risks and Mitigations

- **Broken external documentation links:** retain compatibility pointers for
  important paths and test their targets.
- **Import churn:** use compatibility re-exports and migrate consumers in small
  batches.
- **False architectural symmetry:** keep platform-specific Web and Mobile code
  separate and extract shared code only for demonstrated reuse.
- **Cross-repository merge dependency:** keep commits independently buildable
  and treat cross-repository documents as links, not copied truth.
- **Hidden behavior changes during file splits:** write characterization tests
  before moving logic and run targeted tests after each responsibility is
  extracted.
- **Archive becoming a second source of truth:** label archived documents as
  historical and remove them from maintained navigation and consistency claims.
- **Accidental user-file cleanup:** preserve untracked and ignored files,
  including the main repository's existing `patches/` and local output
  directories.

## Approved Decisions

- The project is presented as a public open-source product first.
- Both repositories are included in the modernization.
- The restructuring is incremental and compatibility-preserving.
- English is canonical; concise Chinese root README translations are provided.
- The main repository adopts MIT to match the Python runtime.
- Documentation, governance, code boundaries, and CI drift prevention are all
  in scope.
