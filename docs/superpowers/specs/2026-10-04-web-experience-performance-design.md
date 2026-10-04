# Web Experience and Performance Design

**Date:** 2026-10-04  
**Status:** Approved design; awaiting written specification review  
**Scope:** `web/` application, with Inbox as the first core workflow

## Summary

AllCallAll's web client is functional and already has useful foundations: route-level splitting on several secondary pages, message windowing, memoized context values, React Query, and dynamically loaded billing and push SDKs. Its current presentation, however, still resembles a generic administration interface, while its runtime loads authenticated-only services and an Agent Graph dependency on the public login route. The Inbox page also combines layout, remote data orchestration, mutations, realtime events, and typing behavior in one component.

This project will turn the web client into a distinctive realtime collaboration control surface. The first delivery focuses on the authenticated Inbox and its surrounding shell, while giving public authentication pages the same visual language. Visual work and performance work are one coordinated change: the UI will expose clearer information hierarchy and realtime state, while the architecture will prevent unrelated routes and services from entering the initial load path.

## Goals

1. Make Inbox the clearest expression of the product: conversations, people, Agent activity, meetings, and organizational context in one coherent workspace.
2. Establish a reusable visual system that feels specific to realtime human-and-Agent collaboration.
3. Reduce public-route startup cost and prevent authenticated-only dependencies and connections from starting before authentication.
4. Split Inbox into understandable, independently testable units without changing backend contracts.
5. Replace broad realtime query invalidation and per-keystroke typing requests with targeted cache updates and bounded event traffic.
6. Add deterministic bundle budgets and responsive, accessibility, and behavior regression coverage.

## Non-goals

- Redesigning every product page in the first delivery.
- Replacing React Query, React Router, Zustand, Tailwind, or the existing API layer.
- Changing backend endpoints or authoritative data ownership.
- Introducing a new design-system package or a heavy animation library.
- Loading third-party web fonts on the critical path.
- Rewriting working RevenueCat or Firebase integrations solely to reduce the size of chunks that are already loaded on demand.

## Current Baseline

The production build on 2026-10-04 succeeds and emits the following notable assets:

| Asset | Raw | Gzip | Current loading behavior |
| --- | ---: | ---: | --- |
| Main entry | 100.86 KB | 27.44 KB | Initial |
| React/router | 180.40 KB | 59.67 KB | Initial |
| Core vendor | 255.13 KB | 79.18 KB | Initial |
| Agent Graph | 175.83 KB | 56.87 KB | Incorrectly preloaded initially |
| Main CSS | 72.72 KB | 14.37 KB | Initial |
| Agent Graph CSS | 15.41 KB | 2.56 KB | Incorrectly preloaded initially |
| Firebase | 55.31 KB | 16.60 KB | Async |
| RevenueCat | 719.70 KB | 188.80 KB | Async |

The HTML entry currently preloads about 225 KB gzip of JavaScript before route-specific work. The Agent Graph JavaScript and CSS are included in that initial dependency graph even though Agent Lab is a lazy route. The Vite configuration also emits a warning because it uses `__dirname`, which is incompatible with the planned native configuration loader.

Structural issues observed in the source:

- `AppProviders` mounts organization, call, and chat realtime providers around public and authenticated routes alike.
- Public auth pages, Inbox, Organizations, and most settings pages are eagerly imported.
- `InboxPage.tsx` owns three-pane rendering, queries, mutations, search, message composition, typing, and realtime event handling.
- Chat realtime events broadly invalidate conversation query prefixes.
- Typing `true` is sent on every composer change, followed by a delayed `false` event.
- The primary stylesheet is monolithic, making page ownership and removal of unused styles difficult to understand.
- The existing 820 KB per-chunk warning does not protect the public entry or critical route payload.

## Experience Direction

### Product identity

The product will retain its forest-green foundation and evolve it into a **realtime collaboration control surface**. The interface should feel calm and operational: dense enough for frequent use, but not visually noisy.

The signature element is a **live signal track**. It is a small, consistent visual vocabulary for connection state, incoming activity, Agent execution, meeting progress, synchronization, and recoverable errors. It may use a pulse, moving marker, or short transition only when state is actively changing. It is not a decorative animation layer.

### Palette

| Token | Value | Role |
| --- | --- | --- |
| Deep forest | `#12372A` | Primary navigation and high-emphasis surfaces |
| Moss | `#2F6B52` | Actions, active states, and positive status |
| Signal amber | `#E9A83A` | Live activity, attention, and pending state |
| Mist | `#F3F7F4` | Application canvas and subtle grouping |
| Ink | `#17211C` | Primary text |
| White | `#FFFFFF` | Raised content surfaces |

Semantic error, warning, and information colors must meet WCAG AA contrast and must not rely on color alone.

### Typography

The critical path will use the system UI stack. Hierarchy will come from deliberate size, weight, width, line height, and spacing rather than a new font download. Timestamps, connection details, compact metrics, and technical identifiers use the system monospace stack. Headings remain restrained; the live signal track, not oversized marketing typography, carries the identity.

### Layout

The authenticated shell groups navigation into three truthful domains:

- **Collaboration:** Inbox, meetings, calls, contacts, follow-ups, deals, recordings.
- **Intelligence:** Agent Lab, tools, knowledge.
- **Administration:** organizations and settings.

Inbox uses four conceptual regions, with the fourth collapsible:

```text
+-------------+----------------------+--------------------------------+------------------+
| Navigation  | Conversation list    | Conversation workspace         | Context / Agent  |
| groups      | search + filters     | messages + composer            | collapsible      |
+-------------+----------------------+--------------------------------+------------------+
```

On tablet, navigation and context become overlays or drawers while the conversation list and workspace remain. On narrow mobile screens, only one Inbox region is shown at a time, with explicit back navigation and preserved selected-conversation state.

### Public authentication experience

Authentication pages reuse the palette, spacing, status language, focus treatment, and signal track. The existing visually arbitrary `01/02` claims are removed. The supporting panel communicates real product behavior through a compact collaboration activity example rather than generic promotional claims. Auth forms remain fast, direct, and usable without animation.

### Interaction and accessibility

- All interactive elements have visible keyboard focus.
- Navigation, filters, dialogs, drawers, and the composer are keyboard reachable in logical order.
- Motion is brief and state-driven; `prefers-reduced-motion` removes nonessential transitions and pulses.
- Empty states explain the next useful action.
- Errors identify the failed area and offer retry where safe.
- Loading feedback is scoped to the region doing work instead of blanking the whole page.

## Application Architecture

### Provider boundaries

The root application keeps only providers required by every route:

```text
QueryClientProvider
└── AuthProvider
    ├── Public routes
    └── AuthenticatedRuntime
        ├── OrganizationProvider
        ├── CallProvider
        ├── ChatRealtimeProvider
        └── Authenticated routes
```

`AuthenticatedRuntime` is mounted only after authentication succeeds. Organization-backed queries, signaling, and chat sockets therefore do not start on login, registration, verification, password recovery, or public invite states that do not require the workspace runtime. Call signaling remains available across authenticated pages so incoming calls are not restricted to Inbox.

Provider lifecycle tests will verify that public routes do not mount authenticated services and that changing organization reconnects organization-scoped realtime services exactly once.

### Route loading

- Keep the minimum public authentication shell in the initial route graph.
- Lazy-load Inbox, Organizations, settings sections, and all existing secondary workspace pages.
- After authentication and organization resolution, prefetch Inbox during idle time because it is the default destination.
- Once the user enters Inbox, optionally prefetch only high-probability adjacent destinations based on visible navigation or intent such as pointer focus. Do not eagerly preload all workspace routes.
- Ensure Agent Graph JavaScript and CSS are reachable only from Agent Lab and routes that render graph components.
- Keep RevenueCat behind billing intent and Firebase behind push-notification intent.

### Inbox boundaries

`InboxPage` becomes an orchestration boundary instead of the implementation of every feature:

| Unit | Responsibility |
| --- | --- |
| `ConversationSidebar` | Search input, server search commit, status/unread filters, pagination, selection |
| `ConversationWorkspace` | Header, message window, message actions, typing display, composer |
| `ConversationContextPanel` | Conversation metadata, Agent context, meetings, recordings, internal notes |
| `useConversationQueries` | Query keys and conversation/message/detail reads |
| `useConversationMutations` | Send, edit, delete, reaction, pin, attachment, note, metadata actions |
| `useConversationRealtime` | Event interpretation and targeted cache updates |
| `useTypingSignal` | Bounded typing lifecycle |

The page owns selected conversation ID, high-level responsive panel state, and composition of these units. Each unit exposes a small prop interface and can be tested without mounting the entire application.

### Realtime data flow

Query key factories become the single source of truth for organization, conversation list, detail, messages, pins, notes, and search keys.

For each chat event:

1. Validate organization and conversation identity.
2. Ignore duplicate or stale events through the existing cursor reducer.
3. Update the exact message/detail/list cache when the event contains sufficient data.
4. Invalidate only the smallest affected query when the payload is incomplete.
5. Reserve broad conversation-list invalidation for recovery or unknown event types.
6. Dispatch typing state independently without invalidating server queries.

Optimistic message changes are allowed only where the API supplies stable reconciliation data. Failed optimistic changes must restore the prior cache and expose an actionable error.

### Typing lifecycle

The typing signal follows a bounded state machine:

- Send `typing: true` immediately when the user transitions from idle to typing.
- While input continues, refresh at a fixed interval rather than per keypress.
- Send `typing: false` after the idle timeout, successful send, conversation switch, or component unmount.
- Do not send typing events when there is no selected conversation or the composer is empty after cleanup.

The implementation must make the interval and timeout explicit constants and test them with fake timers.

### Styling organization

The current monolithic stylesheet will be separated by ownership without introducing CSS-in-JS:

- foundations: reset, tokens, typography, focus, shared surface primitives;
- shell: navigation and authenticated frame;
- Inbox: sidebar, workspace, messages, composer, context panel;
- auth: public authentication layout;
- shared components: dialog, field, button, state, and status patterns.

The exact file organization may follow existing build conventions, but page-specific styles must not be imported by the public shell unless that page is active.

## Performance Design

### Deterministic bundle budgets

The bundle checker will validate route roles rather than relying on a single permissive chunk limit:

| Budget | Target |
| --- | ---: |
| Public entry JavaScript, gzip | `<= 180 KB` |
| Inbox first-use JavaScript, gzip | `<= 240 KB` |
| Initial CSS, gzip | `<= 16 KB` |
| Unexpected lazy-only modules in public preload graph | `0` |

The checker must fail when Agent Graph, RevenueCat, or Firebase appears in the login/public preload graph. Large async SDK chunks may retain separate documented ceilings because their raw sizes do not represent initial-page cost.

The Vite `__dirname` use will be replaced with an ESM-native path so the build is compatible with the planned native config loader.

### Runtime targets

Measured against a production build under the project's documented performance profile:

- Largest Contentful Paint: under 2.5 seconds.
- Interaction to Next Paint: under 200 milliseconds.
- Cumulative Layout Shift: under 0.1.

These are experience targets and diagnostic regression thresholds. Deterministic size and behavior checks remain the merge-blocking CI gates unless the performance environment is made stable enough for Web Vitals to be repeatable.

### Rendering and network behavior

- Retain the existing message-windowing behavior and ensure the split does not expand the rendered message count.
- Memoize derived lists only where computation or reference stability matters; do not blanket-memoize components.
- Avoid request waterfalls between authentication, organization resolution, and Inbox route loading where data can be fetched concurrently after prerequisites are known.
- Realtime events must not trigger an organization-wide refetch for typing events.
- Repeated typing during one active interval must not produce one request per keypress.
- No new critical-path font, analytics, or animation dependency is introduced.

## Error Handling and Resilience

- Public-route failures stay independent from authenticated runtime initialization.
- Inbox panes render scoped loading, empty, and retry states.
- Socket disconnection is represented through the signal track without blocking cached content.
- Unknown realtime events preserve cursor safety and use bounded fallback invalidation.
- Mutation errors retain user input where possible and prevent duplicate submission.
- Responsive drawers preserve focus on open, trap focus where appropriate, and restore focus on close.

## Testing and Verification

### Automated tests

Add or update tests for:

- public versus authenticated provider mounting;
- route-level lazy loading and fallback behavior;
- query key factories and event-to-cache update rules;
- typing throttle, idle stop, send cleanup, route change, and unmount cleanup;
- Inbox unit behavior after component extraction;
- responsive navigation state and context-panel collapse;
- keyboard focus and reduced-motion CSS behavior where practical;
- bundle graph and gzip budgets.

Required web verification:

```bash
cd web && npm run typecheck
cd web && npm run lint
cd web && npm test
cd web && npm run test:coverage
cd web && npm run build
cd web && npm run bundle:budget
```

### Visual verification

Capture and inspect public auth and Inbox at minimum widths of 390 px, 768 px, and 1440 px. Verify:

- hierarchy and clipping;
- mobile region navigation;
- long titles and message content;
- empty, loading, error, unread, typing, disconnected, and Agent-running states;
- keyboard focus visibility;
- reduced-motion behavior.

Generated screenshots remain local artifacts and must not be committed under `output/`.

## Delivery Sequence

1. Establish bundle measurement, query keys, and provider/route boundaries.
2. Extract Inbox data and behavior units with parity tests.
3. Implement precise realtime caching and typing lifecycle.
4. Introduce foundations, shell navigation, and the live signal track.
5. Redesign Inbox desktop and responsive layouts.
6. Align public authentication pages with the new visual language.
7. Complete accessibility, visual, performance, and regression verification.

Each step should remain reviewable and preserve a working application. Architecture and behavior changes land before or alongside their visual consumers so the redesign does not conceal existing loading or state-management problems.

## Risks and Mitigations

- **Incoming-call regression after provider movement:** keep `CallProvider` at the authenticated-runtime level and test it across non-Inbox routes.
- **Realtime cache divergence:** use explicit event handlers, retain bounded invalidation fallback, and compare against server responses in tests.
- **Lazy loading creates visible delay:** idle-prefetch Inbox after authentication and use pane-level skeletons.
- **Responsive redesign hides functionality:** define mobile region navigation before styling and cover it in component/E2E tests.
- **Bundle budgets become brittle:** measure gzip output by semantic entry graph, allow small documented tolerances, and avoid filename-specific assertions.
- **Visual identity overwhelms productivity:** spend visual emphasis on the signal track; keep surrounding surfaces restrained and information-led.

## Acceptance Criteria

The design is complete when:

1. Inbox and the authenticated shell implement the approved realtime collaboration visual language across desktop, tablet, and mobile.
2. Public authentication pages share that language without loading authenticated-only runtime services.
3. Inbox responsibilities are split into focused components and hooks with behavior coverage.
4. Typing and chat event processing meet the bounded-network and targeted-cache rules.
5. The production public entry does not preload Agent Graph, RevenueCat, or Firebase.
6. Deterministic bundle budgets pass at the approved thresholds.
7. Typecheck, lint, tests, coverage, build, and bundle checks pass.
8. Visual inspection covers the required widths, states, keyboard navigation, and reduced motion.
