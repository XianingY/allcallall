# Web Experience and Performance Verification Report

**Date:** 2026-10-04  
**Branch:** `feature/web-experience-performance`  
**Workspace:** `.worktrees/web-experience-performance`

This report verifies the completed Inbox-first web experience and performance work against the
plan in `docs/superpowers/plans/2026-10-04-web-experience-performance.md`.

## Verification summary

| Check | Result |
| --- | --- |
| TypeScript | Passed (`tsc -p tsconfig.app.json --noEmit`) |
| ESLint | Passed (`eslint .`) |
| Unit and component tests | Passed: 55 files, 207 tests |
| Coverage gate | Passed |
| Browser E2E | Passed: 33 tests across desktop, tablet, and mobile |
| Production build | Passed |
| Semantic bundle budget | Passed |
| Responsive visual states | Captured at 390, 768, and 1440 px |
| Accessibility behavior | Keyboard, live region, dialog, mobile region, and reduced-motion checks passed |
| Runtime Web Vitals | Recorded values met targets; two login LCP samples did not emit an entry |

The `npm test` run prints an expected uncaught `Error: Load failed` from
`LazyLoad.test.tsx`, which exercises the lazy-loading failure path. Vitest reports all 55 test
files and all 207 tests as passed.

## Bundle graph and budgets

The budget script measures the semantic public preload graph, the Inbox first-use import closure,
and the initial stylesheet with Node's gzip implementation.

| Metric | Measured | Budget | Result |
| --- | ---: | ---: | --- |
| Public entry JavaScript | 100.2 KiB | 180.0 KiB | Pass |
| Inbox first-use JavaScript | 124.6 KiB | 240.0 KiB | Pass |
| Initial CSS | 15.5 KiB | 16.0 KiB | Pass |

### Public entry JavaScript

```text
index-D4FAITE-.js             14.6 KiB
rolldown-runtime-hePW80VL.js   0.4 KiB
vendor-react-D7AwK3gx.js      57.6 KiB
vendor-core-CdBaUX03.js       27.5 KiB
```

No Agent Graph, RevenueCat, or Firebase asset is present in the public preload graph.

### Inbox first-use JavaScript

```text
AuthLayout-7wXE9CVG.js          0.9 KiB
InboxPage-BGAcHPS3.js           8.3 KiB
check-DKr-NVbG.js               0.1 KiB
collaboration-BN-dhttw.js       0.9 KiB
dist-BiFAxcwg.js                2.4 KiB
dist-CDqL9Gdt.js                8.5 KiB
format-PX4uqa90.js              0.4 KiB
index-D4FAITE-.js              14.6 KiB
meetings-B5SYzva0.js            0.4 KiB
pen-line-9V54effa.js            0.2 KiB
plus-Bxcv45lb.js                0.1 KiB
rolldown-runtime-hePW80VL.js   0.4 KiB
search-Ck5bIJKB.js              0.2 KiB
shared-APaMUOU-.js              0.4 KiB
trash-2-D0tBFjyM.js            0.2 KiB
useInfiniteQuery-C3mEAtPZ.js   0.5 KiB
useMutation-DZ5Zzv6q.js         0.9 KiB
vendor-core-CdBaUX03.js       27.5 KiB
vendor-react-D7AwK3gx.js      57.6 KiB
```

### Agent Lab graph loading

`WorkflowGraph` is a second-level dynamic import. Opening Agent Lab loads the run, approval, and
citation surfaces first; selecting the task graph loads the graph chunk only then. The current
production graph chunk is `WorkflowGraph-U9Hmyuoa.js` at 56.95 KiB gzip, and browser E2E asserts
that the task node renders after selecting the graph tab.

### Initial CSS

```text
index-C4NoBdeD.css             15.5 KiB
```

## Runtime Web Vitals

Environment: local Chromium 153.0.8010.12 against the Vite production preview at
`http://127.0.0.1:4173`, with deterministic API and WebSocket mocks.

| Page | Viewport | LCP | CLS | INP |
| --- | ---: | ---: | ---: | ---: |
| Login | 390×844 | 72 ms | 0 | 32 ms |
| Login | 768×1024 | Not emitted | 0 | 32 ms |
| Login | 1440×1000 | Not emitted | 0 | 32 ms |
| Inbox | 1440×1000 | 144 ms | 0.000991 | 40 ms |

All emitted values are below the targets of LCP `< 2.5 s`, INP `< 200 ms`, and CLS `< 0.1`.
The two missing login LCP entries are recorded as environmental measurement variance rather than
as a zero-second result. INP is approximated from `PerformanceObserver` event durations.

Because these measurements are local and use mocked APIs, the deterministic bundle, test,
typecheck, lint, and behavioral gates remain the merge-blocking criteria.

## Responsive and state coverage

Artifacts are stored under `output/web-verification/` and are intentionally not committed.

| Artifact | Viewport | State |
| --- | ---: | --- |
| `login-390.png` | 390×844 | Public login |
| `login-768.png` | 768×1024 | Public login |
| `login-1440.png` | 1440×1000 | Public login |
| `inbox-390-list.png` | 390×844 | Inbox list |
| `inbox-390-conversation.png` | 390×844 | Conversation and composer |
| `inbox-768-list.png` | 768×1024 | Inbox list |
| `inbox-768-conversation.png` | 768×1024 | Conversation and composer |
| `inbox-1440-loading.png` | 1440×1000 | Conversations loading |
| `inbox-1440-empty.png` | 1440×1000 | Empty inbox |
| `inbox-1440-error.png` | 1440×1000 | Conversation load error |
| `inbox-1440-unread.png` | 1440×1000 | Unread conversation |
| `inbox-1440-typing.png` | 1440×1000 | Remote participant typing |
| `inbox-1440-disconnected.png` | 1440×1000 | Realtime reconnecting |
| `inbox-1440-agent-running.png` | 1440×1000 | Agent processing |

Additional accessibility artifacts:

- `a11y-desktop-dialog.png`
- `a11y-mobile-drawer.png`
- `a11y-mobile-context.png`
- `a11y-reduced-motion.png`

## Accessibility verification

- The typing indicator is exposed as `role="status"` with `aria-live="polite"`.
- Keyboard tab order reaches the sidebar search, conversation link, composer, upload control,
  and context panel with visible focus.
- The new-conversation dialog receives focus, shows a visible focus ring, closes with Escape,
  and returns focus to its trigger.
- The closed mobile navigation rail is removed from the tab order; opening it exposes a visible
  drawer control.
- Mobile Inbox uses a single active region; inactive list and conversation regions are both
  hidden and inert.
- Closing the mobile context region restores visible focus to the context toggle.
- Under `prefers-reduced-motion: reduce`, the app rail transition, signal pulse, and conversation
  item transition are all `none`.

The following defects were found and fixed during verification:

- Added a polite live region to the remote typing indicator.
- Removed the closed mobile rail from the tab order with CSS visibility.
- Added visible focus outlines for modal content and the context toggle.
- Added a regression test for the typing live region.

## Automated command results

| Command | Result |
| --- | --- |
| `cd web && npm run typecheck` | Passed |
| `cd web && npm run lint` | Passed |
| `cd web && npm test` | Passed: 55 files, 207 tests |
| `cd web && npm run test:coverage` | Passed |
| `cd web && npm run build` | Passed |
| `cd web && npm run bundle:budget` | Passed |
| `cd web && npx vitest run src/pages/collaboration/ConversationWorkspace.test.tsx` | Passed: 3 tests |

Coverage summary:

| Metric | Coverage |
| --- | ---: |
| Statements | 55.86% |
| Branches | 49.56% |
| Functions | 41.20% |
| Lines | 69.86% |

## Completion audit

| Requirement | Status |
| --- | --- |
| Public routes do not mount authenticated providers | Verified |
| Public JavaScript and CSS are within budget | Verified |
| Inbox first-use JavaScript is within budget | Verified |
| Agent Graph, RevenueCat, and Firebase are absent from public preload | Verified |
| Agent Lab loads the task graph only when its tab is selected | Verified by browser E2E |
| Inbox is split into the planned hooks and components | Verified |
| Realtime events use targeted cache updates | Verified by tests |
| Typing events issue no query invalidation | Verified by tests |
| Typing requests are bounded by the state machine | Verified by tests |
| Shell, Inbox, and auth pages use the approved visual system | Verified |
| Responsive and accessibility states are covered | Verified |
| All required web verification commands pass | Verified |

## Known limitations

- Web Vitals were collected on a local machine with mocked APIs and are not field data.
- Two login LCP samples did not emit a `largest-contentful-paint` entry; they are reported as
  not emitted rather than as zero.
- INP is approximated from PerformanceObserver event durations, not the full Chrome UX report.
- Screenshots and measurement scripts remain under `output/` and are not committed.
