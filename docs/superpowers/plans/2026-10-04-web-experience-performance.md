# Web Experience and Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Inbox-first web experience as a realtime collaboration control surface while making startup, realtime data flow, and interaction cost deterministic and measurable.

**Architecture:** Move authenticated runtime providers inside the authenticated route boundary, lazy-load workspace routes, and enforce semantic bundle budgets. Extract Inbox orchestration into focused hooks and components, then replace broad realtime invalidation and per-keystroke typing requests with targeted cache updates and a bounded typing state machine. Finally, apply the approved forest-green signal-track design system to the shell, Inbox, and public authentication pages.

**Tech Stack:** React 18, TypeScript, React Router 7, TanStack Query 5, Zustand, Vite 8/Rolldown, Vitest, Testing Library, CSS imports.

**Spec:** `docs/superpowers/specs/2026-10-04-web-experience-performance-design.md`

## Global Constraints

- Public entry JavaScript must be `<= 180 KB` gzip.
- Inbox first-use JavaScript must be `<= 240 KB` gzip.
- Initial CSS must be `<= 16 KB` gzip.
- Login/public preload graph must contain zero references to Agent Graph, RevenueCat, or Firebase.
- Public routes must not mount `OrganizationProvider`, `CallProvider`, or `ChatRealtimeProvider`.
- Typing must send immediately on the idle-to-typing transition, refresh on a fixed interval, and stop on idle timeout, send, route change, or unmount.
- Realtime typing events must not invalidate server queries.
- Runtime Web Vitals targets are LCP `< 2.5s`, INP `< 200ms`, and CLS `< 0.1`.
- Preserve backend API contracts and backend behavior.
- Do not add a critical-path web font, animation library, or design-system package.
- Use `apply_patch` for file edits.
- Do not commit `.playwright-mcp/` or `output/`.
- Required final commands:

```bash
cd web && npm run typecheck
cd web && npm run lint
cd web && npm test
cd web && npm run test:coverage
cd web && npm run build
cd web && npm run bundle:budget
```

---

## File Structure

| Path | Responsibility |
| --- | --- |
| `web/src/app/AppProviders.tsx` | Root query/auth providers and authenticated runtime boundary |
| `web/src/app/AuthenticatedRuntime.tsx` | Organization, call, and chat realtime provider composition |
| `web/src/app/routes/publicRoutes.tsx` | Public route graph and lazy auth pages |
| `web/src/app/routes/workspaceRoutes.tsx` | Authenticated route graph, lazy workspace pages, Inbox prefetch |
| `web/src/pages/collaboration/conversationQueryKeys.ts` | Query key factory shared by Inbox and realtime cache |
| `web/src/realtime/chatCache.ts` | Chat event-to-cache update rules |
| `web/src/realtime/ChatRealtimeProvider.tsx` | Socket lifecycle, cursor reduction, event dispatch, targeted cache updates |
| `web/src/pages/collaboration/useTypingSignal.ts` | Bounded typing lifecycle |
| `web/src/pages/collaboration/useConversationQueries.ts` | Inbox reads |
| `web/src/pages/collaboration/useConversationMutations.ts` | Inbox mutations and cache reconciliation |
| `web/src/pages/collaboration/ConversationSidebar.tsx` | Conversation search, filters, list, search results |
| `web/src/pages/collaboration/ConversationWorkspace.tsx` | Messages, typing display, composer, message actions |
| `web/src/pages/collaboration/ConversationContextPanel.tsx` | Status, Agent context, meetings, recording, notes |
| `web/src/pages/collaboration/InboxPage.tsx` | Responsive composition and selected-conversation state |
| `web/src/components/AppShell.tsx` | Grouped navigation, organization state, signal track |
| `web/src/components/SignalTrack.tsx` | Reusable realtime status presentation |
| `web/src/styles/*.css` | Foundations, shell, Inbox, auth, and shared component styles |
| `scripts/web/bundle-budget.mjs` | Semantic gzip budgets and preload graph checks |
| `web/vite.config.ts` | Native-config-compatible alias and code-splitting configuration |

### Task 1: Provider Boundary, Route Loading, and Bundle Budget

**Files:**

- Modify: `web/src/app/AppProviders.tsx`
- Create: `web/src/app/AuthenticatedRuntime.tsx`
- Modify: `web/src/app/AppProviders.test.tsx`
- Modify: `web/src/app/routes/publicRoutes.tsx`
- Modify: `web/src/app/routes/workspaceRoutes.tsx`
- Modify: `web/src/app/routes/routes.test.tsx`
- Modify: `web/vite.config.ts`
- Modify: `scripts/web/bundle-budget.mjs`
- Create: `scripts/web/bundle-graph.mjs`
- Test: `web/src/app/AppProviders.test.tsx`, `web/src/app/routes/routes.test.tsx`, `scripts/web/bundle-budget.mjs`

**Interfaces:**

- Produces: `export function AuthenticatedRuntime({ children }: PropsWithChildren): JSX.Element`
- Produces: `export const publicPage = (loader: () => Promise<{ default: ComponentType }>) => LazyExoticComponent<ComponentType>`
- Produces: `export const workspacePage = (loader: () => Promise<{ default: ComponentType }>) => LazyExoticComponent<ComponentType>`
- Produces: `parseBundleGraph(distDir: string): { publicJs: Asset[]; inboxJs: Asset[]; initialCss: Asset[]; forbiddenPublicAssets: Asset[] }`
- Produces: `gzipSize(path: string): Promise<number>`

- [ ] **Step 1: Write failing provider tests**

Append these cases to `web/src/app/AppProviders.test.tsx`; adapt the existing mocks only if their names already differ:

```tsx
it("does not mount authenticated runtime providers while unauthenticated", () => {
  renderAuthenticatedRuntime = false;
  render(<AppProviders>{children}</AppProviders>);

  expect(screen.queryByTestId("organization-provider")).toBeNull();
  expect(screen.queryByTestId("call-provider")).toBeNull();
  expect(screen.queryByTestId("chat-realtime-provider")).toBeNull();
});

it("mounts organization, call, and chat providers only after authentication", () => {
  renderAuthenticatedRuntime = true;
  render(<AppProviders>{children}</AppProviders>);

  expect(screen.getByTestId("organization-provider")).toBeTruthy();
  expect(screen.getByTestId("call-provider")).toBeTruthy();
  expect(screen.getByTestId("chat-realtime-provider")).toBeTruthy();
});
```

The mocked `AuthProvider` must set `renderAuthenticatedRuntime` through a prop such as `onStatusChange`, or the test can use the existing test auth bridge contract.

- [ ] **Step 2: Run the provider test and verify it fails**

```bash
cd web && npx vitest run src/app/AppProviders.test.tsx
```

Expected: fail because `AppProviders` eagerly mounts all providers.

- [ ] **Step 3: Implement the provider boundary**

Create `AuthenticatedRuntime.tsx`:

```tsx
import type { PropsWithChildren } from "react";

import { CallProvider } from "@/calls/CallProvider";
import { OrganizationProvider } from "@/organizations/OrganizationProvider";
import { ChatRealtimeProvider } from "@/realtime/ChatRealtimeProvider";

export function AuthenticatedRuntime({ children }: PropsWithChildren) {
  return (
    <OrganizationProvider>
      <CallProvider>
        <ChatRealtimeProvider>{children}</ChatRealtimeProvider>
      </CallProvider>
    </OrganizationProvider>
  );
}
```

Replace `AppProviders` with a root that renders `QueryClientProvider`, `AuthProvider`, and an `AuthenticatedRuntimeGate`. The gate reads `useAuth()` and renders `AuthenticatedRuntime` only when `status === "authenticated"`. The implementation must not use router location to decide authentication.

- [ ] **Step 4: Verify provider tests pass**

```bash
cd web && npx vitest run src/app/AppProviders.test.tsx
```

Expected: pass.

- [ ] **Step 5: Write failing route tests**

In `routes.test.tsx`, assert that route functions return elements whose lazy import source does not eagerly reference auth pages, Inbox, Organizations, or settings pages. Use the existing marker-based route tests and add:

```tsx
expect(publicRoutes.toString()).not.toContain("LoginPage");
expect(workspaceRoutes.toString()).not.toContain("InboxPage");
expect(workspaceRoutes.toString()).not.toContain("OrganizationsPage");
expect(workspaceRoutes.toString()).not.toContain("SettingsPages");
```

Because route functions close over lazy constants, add an exported `workspaceRouteImports` test map when needed rather than relying on source text.

- [ ] **Step 6: Make public and workspace pages lazy**

In `publicRoutes.tsx`, replace direct page imports with:

```tsx
const LoginPage = lazy(() => import("@/pages/auth/LoginPage"));
const RegisterPage = lazy(() => import("@/pages/auth/RegisterPage"));
const VerifyEmailPage = lazy(() => import("@/pages/auth/VerifyEmailPage"));
const ForgotPasswordPage = lazy(() => import("@/pages/auth/ForgotPasswordPage"));
const InvitePage = lazy(() => import("@/pages/auth/InvitePage"));
```

Wrap each element with the existing `LazyLoad` component and `PageLoading`.

In `workspaceRoutes.tsx`, do the same for Inbox, Organizations, `SettingsLayout`, and each settings page. Create local `lazyPage` helpers if that removes repetition:

```tsx
const InboxPage = lazy(() =>
  import("@/pages/collaboration/InboxPage").then((module) => ({ default: module.InboxPage })),
);
```

After `ProtectedRoute` and `AppShell` mount, add an idle Inbox prefetch using `requestIdleCallback` when available and `setTimeout` otherwise:

```ts
const prefetchInbox = () => void import("@/pages/collaboration/InboxPage");

if ("requestIdleCallback" in window) {
  const id = window.requestIdleCallback(prefetchInbox, { timeout: 2000 });
  return () => window.cancelIdleCallback(id);
}
const id = window.setTimeout(prefetchInbox, 300);
return () => window.clearTimeout(id);
```

Put this in a small `WorkspacePrefetch` component so route definitions remain declarative.

- [ ] **Step 7: Fix Vite native config warning**

In `web/vite.config.ts`, replace:

```ts
path.resolve(__dirname, "src")
```

with:

```ts
path.resolve(import.meta.dirname, "src")
```

Run the build and verify the warning is absent.

- [ ] **Step 8: Replace the size-only budget with semantic budgets**

Create `scripts/web/bundle-graph.mjs` with exports:

```js
export async function gzipSize(filePath) {
  const input = await readFile(filePath);
  return new Promise((resolve, reject) => {
    gzip(input, (error, result) => error ? reject(error) : resolve(result.length));
  });
}

export function parseBundleGraph(distDir) {
  // Read web/dist/index.html, resolve modulepreload and entry assets,
  // identify Inbox route chunks from their static imports, and return the four sets.
}
```

Modify `bundle-budget.mjs` to:

```js
const budgets = {
  publicJsGzipBytes: 180 * 1024,
  inboxJsGzipBytes: 240 * 1024,
  initialCssGzipBytes: 16 * 1024,
};

const forbiddenPublicPatterns = [
  /vendor-agent-graph-[^/]+\.js$/,
  /vendor-agent-graph-[^/]+\.css$/,
  /vendor-revenuecat-[^/]+\.js$/,
  /vendor-firebase-[^/]+\.js$/,
];
```

Print each measured set and fail with an explicit message if a budget or forbidden-asset rule fails. Keep large async SDK chunks outside the public graph.

- [ ] **Step 9: Run the focused checks**

```bash
cd web && npx vitest run src/app/AppProviders.test.tsx src/app/routes/routes.test.tsx
cd web && npm run build
cd web && npm run bundle:budget
```

Expected: tests pass; build has no `__dirname` warning; budget script passes. If Agent Graph remains in the public preload graph, move the `@xyflow` CSS import into the Agent Lab page and correct code-splitting priority before continuing.

- [ ] **Step 10: Commit**

```bash
git add web/src/app web/vite.config.ts scripts/web/bundle-budget.mjs scripts/web/bundle-graph.mjs
git commit -m "perf(web): isolate public runtime and enforce bundle budgets"
```

### Task 2: Query Keys and Targeted Realtime Cache Updates

**Files:**

- Create: `web/src/pages/collaboration/conversationQueryKeys.ts`
- Create: `web/src/pages/collaboration/conversationQueryKeys.test.ts`
- Create: `web/src/realtime/chatCache.ts`
- Create: `web/src/realtime/chatCache.test.ts`
- Modify: `web/src/realtime/ChatRealtimeProvider.tsx`
- Modify: `web/src/realtime/ChatRealtimeProvider.test.tsx`
- Modify: `web/src/pages/collaboration/InboxPage.tsx`

**Interfaces:**

- Produces:

```ts
export const conversationKeys = {
  all: (organizationId?: number) => ["organizations", organizationId, "conversations"] as const,
  list: (organizationId: number | undefined, status: string) =>
    [...conversationKeys.all(organizationId), status] as const,
  detail: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.all(organizationId), conversationId] as const,
  messages: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.detail(organizationId, conversationId), "messages"] as const,
  pins: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.detail(organizationId, conversationId), "pins"] as const,
  notes: (organizationId: number | undefined, conversationId: number | null) =>
    [...conversationKeys.detail(organizationId, conversationId), "notes"] as const,
  messageSearch: (organizationId: number | undefined, query: string) =>
    ["organizations", organizationId, "search", "messages", query] as const,
};
```

- Produces:

```ts
export type ChatCacheAction =
  | { type: "append-message"; conversationId: number; message: Message }
  | { type: "patch-message"; conversationId: number; message: Partial<Message> & { id: number } }
  | { type: "conversation-activity"; conversationId: number }
  | { type: "unknown"; conversationId?: number };

export function planChatCacheAction(event: ChatEvent): ChatCacheAction;
export function applyChatEventToQueryCache(
  queryClient: QueryClient,
  organizationId: number,
  event: ChatEvent,
): void;
```

- [ ] **Step 1: Write query-key tests**

```ts
import { conversationKeys } from "./conversationQueryKeys";

describe("conversationKeys", () => {
  it("creates nested keys without string assembly", () => {
    expect(conversationKeys.messages(7, 5)).toEqual([
      "organizations", 7, "conversations", 5, "messages",
    ]);
  });
});
```

Run:

```bash
cd web && npx vitest run src/pages/collaboration/conversationQueryKeys.test.ts
```

Expected: fail because the module does not exist.

- [ ] **Step 2: Implement query keys and use them in Inbox**

Create the factory and replace all literal conversation query keys in `InboxPage.tsx` with factory calls. Do not change query semantics in this step.

- [ ] **Step 3: Write failing cache-event tests**

In `chatCache.test.ts`, create a `QueryClient`, seed one infinite message page, one detail query, and one conversation list page, then:

```ts
it("appends a message only to the target conversation", () => {
  applyChatEventToQueryCache(client, 7, messageCreatedEvent);
  expect(client.getQueryData(conversationKeys.messages(7, 5))).toMatchObject({
    pages: [{ messages: [existingMessage, newMessage] }],
  });
  expect(listRefetchSpy).not.toHaveBeenCalled();
});

it("does not invalidate queries for typing events", () => {
  const invalidateSpy = vi.spyOn(client, "invalidateQueries");
  applyChatEventToQueryCache(client, 7, typingEvent);
  expect(invalidateSpy).not.toHaveBeenCalled();
});

it("uses a narrow invalidation for unknown event payloads", () => {
  applyChatEventToQueryCache(client, 7, unknownConversationEvent);
  expect(client.isInvalidated(conversationKeys.detail(7, 5))).toBe(true);
  expect(client.isInvalidated(conversationKeys.all(7))).toBe(false);
});
```

Use the real event names emitted by the backend. Before coding, record them from `ChatRealtimeProvider.test.tsx` and backend event publishers; do not invent event strings.

- [ ] **Step 4: Implement `chatCache.ts`**

Implement deterministic action planning:

```ts
const messagePayload = (payload: Record<string, unknown>) =>
  MessageSchema.safeParse(payload);
```

If an existing local `Message` type is available, use a narrow runtime type guard instead of adding Zod. For `message.created`, append when the message is not already present. For message update/delete/pin events, patch the matching item in all loaded pages. For conversation events, patch list fields when the payload is sufficient, otherwise invalidate only the detail query. For unknown events, invalidate only the identified conversation's detail query; use organization-wide invalidation only in a recovery path exposed by an explicit function.

- [ ] **Step 5: Connect the cache helper to realtime provider**

In `ChatRealtimeProvider`, after cursor reduction and typing dispatch:

```ts
if (!event.event.startsWith("typing.")) {
  applyChatEventToQueryCache(queryClient, organizationId, event);
}
```

Keep the existing window custom event for Inbox typing display. Update provider tests to assert exact query invalidation behavior rather than only connection state.

- [ ] **Step 6: Run realtime and Inbox tests**

```bash
cd web && npx vitest run src/pages/collaboration/conversationQueryKeys.test.ts src/realtime/chatCache.test.ts src/realtime/ChatRealtimeProvider.test.tsx src/pages/collaboration/InboxPage.test.tsx
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/collaboration/conversationQueryKeys.ts web/src/pages/collaboration/conversationQueryKeys.test.ts web/src/realtime/chatCache.ts web/src/realtime/chatCache.test.ts web/src/realtime/ChatRealtimeProvider.tsx web/src/realtime/ChatRealtimeProvider.test.tsx web/src/pages/collaboration/InboxPage.tsx
git commit -m "perf(web): target realtime conversation cache updates"
```

### Task 3: Bounded Typing Lifecycle

**Files:**

- Create: `web/src/pages/collaboration/useTypingSignal.ts`
- Create: `web/src/pages/collaboration/useTypingSignal.test.ts`
- Modify: `web/src/pages/collaboration/InboxPage.tsx`
- Modify: `web/src/pages/collaboration/InboxPage.test.tsx`

**Interfaces:**

- Produces:

```ts
export const TYPING_REFRESH_MS = 2500;
export const TYPING_STOP_MS = 1200;

export function useTypingSignal(input: {
  conversationId: number | null;
  enabled?: boolean;
  onBeforeStop?: () => void;
}): {
  signalTyping: (next: string) => void;
  stopTyping: () => void;
};
```

- [ ] **Step 1: Write failing timer tests**

Use `vi.useFakeTimers()` and mock `sendTyping`:

```ts
it("sends one start event and refreshes on an interval, not per keystroke", () => {
  const { result } = renderHook(() => useTypingSignal({ conversationId: 5 }));

  act(() => result.current.signalTyping("h"));
  act(() => result.current.signalTyping("he"));
  act(() => result.current.signalTyping("hel"));
  act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS));
  act(() => result.current.signalTyping("hell"));

  expect(sendTypingMock).toHaveBeenCalledTimes(2);
  expect(sendTypingMock).toHaveBeenNthCalledWith(1, 5, true);
  expect(sendTypingMock).toHaveBeenNthCalledWith(2, 5, true);
});

it("stops after idle and does not stop twice", () => {
  const { result } = renderHook(() => useTypingSignal({ conversationId: 5 }));
  act(() => result.current.signalTyping("hello"));
  act(() => vi.advanceTimersByTime(TYPING_STOP_MS));
  act(() => vi.advanceTimersByTime(TYPING_REFRESH_MS));

  expect(sendTypingMock).toHaveBeenCalledTimes(2);
  expect(sendTypingMock).toHaveBeenLastCalledWith(5, false);
});
```

Add route-change and unmount tests that stop typing exactly once.

- [ ] **Step 2: Implement the hook**

Keep a `lastSentAt` timestamp, a stop timeout, and an interval. Reset state when `conversationId` changes. Swallow network failures but log once per active session. Call `stopTyping` on unmount and before successful send using `onBeforeStop`.

- [ ] **Step 3: Wire Inbox**

Replace the current `onComposerChange` implementation with:

```ts
const { signalTyping } = useTypingSignal({
  conversationId: selectedId,
  onBeforeStop: () => setComposer(""),
});
```

Keep `setComposer` separate from typing state; do not clear user text in the hook itself. The mutation success path calls the hook's stop cleanup and then clears the draft.

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/pages/collaboration/useTypingSignal.test.ts src/pages/collaboration/InboxPage.test.tsx
```

Expected: all pass, including existing mutation-failure parity tests.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/collaboration/useTypingSignal.ts web/src/pages/collaboration/useTypingSignal.test.ts web/src/pages/collaboration/InboxPage.tsx web/src/pages/collaboration/InboxPage.test.tsx
git commit -m "perf(web): bound conversation typing signal traffic"
```

### Task 4: Inbox Extraction with Behavior Parity

**Files:**

- Create: `web/src/pages/collaboration/useConversationQueries.ts`
- Create: `web/src/pages/collaboration/useConversationMutations.ts`
- Create: `web/src/pages/collaboration/ConversationSidebar.tsx`
- Create: `web/src/pages/collaboration/ConversationWorkspace.tsx`
- Create: `web/src/pages/collaboration/ConversationContextPanel.tsx`
- Modify: `web/src/pages/collaboration/InboxPage.tsx`
- Create or modify focused tests beside each component
- Modify: `web/src/pages/collaboration/InboxPage.test.tsx`

**Interfaces:**

```ts
export interface ConversationFilterState {
  status: string;
  keyword: string;
  unreadOnly: boolean;
  messageQuery: string;
}

export function useConversationQueries(input: {
  organizationId: number | undefined;
  conversationId: number | null;
  filter: ConversationFilterState;
});

export function useConversationMutations(input: {
  organizationId: number | undefined;
  conversationId: number | null;
  onMessageSent: () => void;
});

export function ConversationSidebar(props: {
  filter: ConversationFilterState;
  onFilterChange: (next: Partial<ConversationFilterState>) => void;
  conversations: ReturnType<typeof useConversationQueries>["conversations"];
  selectedId: number | null;
  onCreateConversation: () => void;
});

export function ConversationWorkspace(props: {
  selectedId: number | null;
  detail: ConversationDetailQuery;
  messages: MessagesQuery;
  pins: PinsQuery;
  composerState: ComposerState;
  onComposerChange: (value: string) => void;
  onSubmit: () => void;
  onBackToList: () => void;
});

export function ConversationContextPanel(props: {
  selectedId: number | null;
  detail: ConversationDetailQuery;
  notes: NotesQuery;
  onNoteChange: (value: string) => void;
  onUpdateConversation: (input: { status?: string; priority?: string }) => void;
});
```

- [ ] **Step 1: Extract read and mutation hooks first**

Move queries and mutations from `InboxPage` without changing behavior. Keep existing API mocks and page tests passing. Each hook must accept all identifiers as props rather than importing organization state directly.

- [ ] **Step 2: Extract the sidebar**

Move the search box, search scope, filter tabs, message-search result rendering, conversation list, pagination, and empty/error states. Add a component test that changes filters, commits message search on Enter, and clears search.

- [ ] **Step 3: Extract the workspace**

Move the conversation header, pinned strip, message window, typing line, message actions, composer context, attachment upload, and send form. Preserve all labels used by current tests.

- [ ] **Step 4: Extract the context panel**

Move status, priority, Agent context metrics, meeting actions, latest recording, and notes. The panel owns no route navigation logic beyond rendering links.

- [ ] **Step 5: Reduce `InboxPage` to composition and responsive state**

The final page should own selected conversation ID, filter state, composer state, and mobile region state. It should not contain query construction, mutation implementation, or realtime event internals.

- [ ] **Step 6: Run the Inbox suite**

```bash
cd web && npx vitest run src/pages/collaboration
```

Expected: all existing and newly focused tests pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/collaboration
git commit -m "refactor(web): split inbox workspace boundaries"
```

### Task 5: Design Foundations, Signal Track, and Authenticated Shell

**Files:**

- Create: `web/src/styles/foundations.css`
- Create: `web/src/styles/shell.css`
- Create: `web/src/styles/shared.css`
- Modify: `web/src/styles.css`
- Create: `web/src/components/SignalTrack.tsx`
- Create: `web/src/components/SignalTrack.test.tsx`
- Modify: `web/src/components/AppShell.tsx`
- Create or modify: `web/src/components/AppShell.test.tsx`

**Interfaces:**

```tsx
export interface SignalTrackProps {
  state: "connected" | "connecting" | "error";
  activity?: "idle" | "incoming" | "agent" | "meeting";
  label: string;
  compact?: boolean;
}
```

- [ ] **Step 1: Split styles without changing selectors**

Move reset, tokens, focus, shared buttons/fields/dialogs/state styles into the new CSS files. Keep `web/src/styles.css` as the import root:

```css
@import "./styles/foundations.css";
@import "./styles/shared.css";
@import "./styles/shell.css";
```

The visual shell is not styled in this step; this only establishes ownership and prevents accidental selector changes.

- [ ] **Step 2: Add tokens**

Add CSS custom properties:

```css
:root {
  --color-forest: #12372A;
  --color-moss: #2F6B52;
  --color-signal: #E9A83A;
  --color-mist: #F3F7F4;
  --color-ink: #17211C;
  --surface-raised: #FFFFFF;
  --font-mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
}
```

Wire existing Tailwind/config tokens to these values where applicable; do not introduce a second token system.

- [ ] **Step 3: Build `SignalTrack`**

Implement a small status component with text, an accessible state, a pulse only for active collaboration states, and no animation under `prefers-reduced-motion`. Test state labels and `aria-live="polite"`.

- [ ] **Step 4: Group navigation**

Change the flat nav array into:

```ts
const navGroups = [
  { label: "nav.groups.collaboration", items: [...] },
  { label: "nav.groups.intelligence", items: [...] },
  { label: "nav.groups.administration", items: [...] },
] as const;
```

Add i18n labels in the existing translation files. The active organization selector stays at the top of administration, not above collaboration.

- [ ] **Step 5: Restyle shell**

Implement the deep-forest rail, mist canvas, raised panels, grouped navigation, compact organization selector, and signal track. Mobile navigation remains a drawer with visible focus and a close control. Preserve all existing routes and test IDs.

- [ ] **Step 6: Run shell and type checks**

```bash
cd web && npx vitest run src/components/AppShell.test.tsx src/components/SignalTrack.test.tsx
cd web && npm run typecheck
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/styles.css web/src/styles web/src/components/AppShell.tsx web/src/components/AppShell.test.tsx web/src/components/SignalTrack.tsx web/src/components/SignalTrack.test.tsx web/src/i18n
git commit -m "feat(web): build realtime workspace shell"
```

### Task 6: Inbox Visual and Responsive Design

**Files:**

- Create: `web/src/styles/inbox.css`
- Modify: `web/src/styles.css`
- Modify: `web/src/pages/collaboration/ConversationSidebar.tsx`
- Modify: `web/src/pages/collaboration/ConversationWorkspace.tsx`
- Modify: `web/src/pages/collaboration/ConversationContextPanel.tsx`
- Modify: `web/src/pages/collaboration/InboxPage.tsx`
- Create: `web/src/pages/collaboration/InboxResponsive.test.tsx`
- Modify: relevant component tests

**Interfaces:**

- Produces mobile state:

```ts
type InboxMobileRegion = "list" | "conversation" | "context";
```

- Produces responsive markup classes: `inbox-layout`, `inbox-region-list`, `inbox-region-conversation`, `inbox-region-context`.

- [ ] **Step 1: Write responsive state tests**

Test that at narrow widths only one region is exposed and selecting a conversation moves to `conversation`; the context button moves to `context`; back navigation returns to `conversation`, then `list`. Test focus is restored when the context panel closes.

- [ ] **Step 2: Implement responsive region state**

Use a CSS breakpoint rather than JavaScript width polling for layout. JavaScript state controls focus order and the `hidden`/`inert` treatment for narrow screens only.

- [ ] **Step 3: Design the sidebar**

Use compact search, segmented filters, and a readable conversation list. Long titles truncate with an accessible full-title attribute. Unread, priority, and activity use the signal language rather than large colored badges.

- [ ] **Step 4: Design the workspace**

Improve hierarchy around title, participants/topic, status, message grouping, and pinned content. Keep the existing message windowing and action count. Make the composer visually stable and prevent layout shift while typing.

- [ ] **Step 5: Design the context panel**

Make the panel collapsible. Group status, Agent context, meeting, recording, and notes with clear headings. Agent-running state uses the signal track, not ambient decoration.

- [ ] **Step 6: Run focused tests and build**

```bash
cd web && npx vitest run src/pages/collaboration
cd web && npm run typecheck
cd web && npm run build
```

Expected: all pass; Agent Graph and its CSS are absent from the public preload graph.

- [ ] **Step 7: Commit**

```bash
git add web/src/styles.css web/src/styles/inbox.css web/src/pages/collaboration
git commit -m "feat(web): redesign inbox collaboration surface"
```

### Task 7: Public Authentication Experience

**Files:**

- Create: `web/src/styles/auth.css`
- Modify: `web/src/styles.css`
- Modify: `web/src/pages/auth/LoginPage.tsx`
- Modify: `web/src/pages/auth/RegisterPage.tsx`
- Modify: `web/src/pages/auth/ForgotPasswordPage.tsx`
- Modify: `web/src/pages/auth/VerifyEmailPage.tsx`
- Modify: `web/src/pages/auth/InvitePage.tsx`
- Modify: `web/src/components/AuthLayout.tsx`
- Modify: relevant auth tests

**Interfaces:**

- Produces a shared auth surface component when duplication exceeds three pages:

```tsx
export function AuthActivityPanel({
  title,
  activity,
  children,
}: PropsWithChildren<{ title: string; activity: AuthActivityItem[] }>);
```

- [ ] **Step 1: Remove false sequence markers**

Delete or replace `01`/`02` numbering on auth pages. Numbering is only allowed where the interface truly represents an ordered process.

- [ ] **Step 2: Add a compact collaboration example**

Use the approved signal language to show a small, realistic activity sequence: a person asks a question, an Agent summarizes grounded sources, a teammate approves an action, and a meeting note is filed. Keep it static and readable; do not add an animation loop.

- [ ] **Step 3: Restyle forms**

Apply the shared palette, focus treatment, typography, and scoped loading/error states. Forms remain one primary action and clear recovery paths. No authenticated provider dependency may be added.

- [ ] **Step 4: Run auth tests and bundle check**

```bash
cd web && npx vitest run src/pages/auth
cd web && npm run build
cd web && npm run bundle:budget
```

Expected: all pass and public budget remains `<= 180 KB` gzip.

- [ ] **Step 5: Commit**

```bash
git add web/src/styles.css web/src/styles/auth.css web/src/pages/auth web/src/components/AuthLayout.tsx
git commit -m "feat(web): align auth pages with realtime identity"
```

### Task 8: Accessibility, Visual, Performance, and Final Verification

**Files:**

- Modify only files with defects discovered by verification.
- Create: `docs/superpowers/reports/2026-10-04-web-experience-performance.md`

**Interfaces:**

- Produces a verification report containing:
  - public and Inbox gzip measurements;
  - preload graph list;
  - Web Vitals measurements and environment;
  - screenshots list and widths;
  - automated test results;
  - known limitations.

- [ ] **Step 1: Run the complete web verification set**

```bash
cd web && npm run typecheck
cd web && npm run lint
cd web && npm test
cd web && npm run test:coverage
cd web && npm run build
cd web && npm run bundle:budget
```

Fix any failure before proceeding.

- [ ] **Step 2: Inspect required states and widths**

Use production preview and capture login and Inbox at 390 px, 768 px, and 1440 px. Inspect loading, empty, error, unread, typing, disconnected, and Agent-running states. Save artifacts under `output/` and do not commit them.

- [ ] **Step 3: Verify accessibility behavior**

Exercise keyboard navigation through sidebar, messages, composer, context panel, dialogs, and mobile drawers. Verify visible focus, logical tab order, `aria-live` signal updates, and reduced-motion CSS.

- [ ] **Step 4: Measure Web Vitals**

Use the project's available browser performance tooling against production preview. Record LCP, INP, and CLS. If environmental variance makes a target unreliable, record the variance and keep deterministic bundle and behavior gates as the merge blocker.

- [ ] **Step 5: Write the verification report**

Include exact command output summaries, measured values, screenshots, and remaining limitations. Do not claim a check passed unless it was run.

- [ ] **Step 6: Commit**

```bash
git add docs/superpowers/reports/2026-10-04-web-experience-performance.md
git commit -m "docs(web): verify experience and performance overhaul"
```

## Completion Audit

Before declaring the project complete, verify:

1. Public routes do not mount authenticated providers.
2. Public entry JavaScript and CSS are within approved gzip budgets.
3. Inbox first-use JavaScript is within its approved gzip budget.
4. Agent Graph, RevenueCat, and Firebase are absent from the public preload graph.
5. Inbox is split into the specified components and hooks.
6. Realtime events use targeted cache updates, and typing events issue no query invalidation.
7. Typing requests are bounded by the state-machine tests.
8. Shell, Inbox, and auth pages use the approved visual system.
9. Responsive and accessibility inspections cover all required states and widths.
10. All web verification commands pass.

