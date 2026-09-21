# Code Quality Improvement Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve code quality across the AllCallAll monorepo by addressing critical technical debt, expanding test coverage, and enhancing documentation.

**Architecture:** Decompose oversized components, fix type safety issues, add missing tests, and improve documentation. Changes follow existing patterns and do not alter runtime behavior.

**Tech Stack:** Go, React, React Native, TypeScript, Vitest, Playwright, golang testing

---

## Current State Summary

| Area | Files | LOC | Tests | Coverage |
|------|-------|-----|-------|----------|
| Backend | 358 source | ~66,800 | 154 | 43% file-level |
| Web | 89 source | ~5,127 | 11 | 12.4% file-level |
| Mobile | 120 source | ~26,250 | 12 | 12% file-level |
| **Total** | **567 source** | **~98,177** | **177** | **~31% file-level** |

---

## Task 1: Fix Swallowed Errors in Backend Go Code

**Files:**
- Modify: `backend/internal/collaboration/conversation_summary.go:113,134,245,268`
- Modify: `backend/internal/collaboration/deal_service.go:25,91`
- Modify: `backend/internal/collaboration/room_service.go:371`
- Modify: `backend/internal/collaboration/chat_hub.go:214,228`
- Modify: `backend/internal/commerce/followup_service.go:350,514`
- Modify: `backend/internal/commerce/billing_webhook.go:72`
- Modify: `backend/internal/commerce/quota.go:129`

- [x] **Step 1: Read current error handling patterns**

Run: `cd backend && grep -n "_ = " internal/collaboration/conversation_summary.go internal/collaboration/deal_service.go internal/collaboration/room_service.go internal/collaboration/chat_hub.go internal/commerce/followup_service.go internal/commerce/billing_webhook.go internal/commerce/quota.go | head -30`

- [x] **Step 2: Fix conversation_summary.go errors**

Replace `_ = ` with `if err := ...; err != nil { log.Printf("WARN: ...") }` pattern for each discarded error at lines 113, 134, 245, 268.

- [x] **Step 3: Fix deal_service.go errors**

Replace `_ = ` with proper error logging at lines 25, 91.

- [x] **Step 4: Fix room_service.go error**

Replace `_ = ` with proper error logging at line 371.

- [x] **Step 5: Fix chat_hub.go errors**

Replace `_ = ` with proper error logging at lines 214, 228.

- [x] **Step 6: Fix followup_service.go errors**

Replace `_ = ` with proper error logging at lines 350, 514.

- [x] **Step 7: Fix billing_webhook.go error**

Replace `_ = ` with proper error logging at line 72.

- [x] **Step 8: Fix quota.go error**

Replace `_ = ` with proper error logging at line 129.

- [x] **Step 9: Run backend tests**

Run: `cd backend && go test ./internal/collaboration/... ./internal/commerce/...`
Expected: PASS

- [x] **Step 10: Commit**

```bash
git add backend/internal/collaboration/ backend/internal/commerce/
git commit -m "fix: add logging for swallowed errors in collaboration and commerce packages"
```

---

## Task 2: Add WebRTC Type Augmentation for Mobile
> **实际做法（与计划不同，更好）：** 无需新建 `react-native-webrtc.d.ts`  augmentation。
> `src/platform/rtc.tsx` 已把 `RTCPeerConnection` 的**类型**指向 `globalThis.RTCPeerConnection`（DOM lib 自带
> `onicecandidate/ontrack/ondatachannel/onconnectionstatechange/oniceconnectionstatechange`），
> 且所有构造函数签名都是 `(...args: any[])`。因此 23 + 5 + 1 处 `as any` **全部是多余的**，直接删除即可。
> 唯一真实的结构性冲突是 `RemoteTrackLike.onmute/onunmute/onended` 声明为 `() => void`，
> 而 DOM 的 handler 带 `ev` 参数 —— 已把签名放宽为 `((event?: any) => void) | null`（向后兼容）。


**Files:**
- Create: `mobile/src/types/react-native-webrtc.d.ts`
- Modify: `mobile/src/context/SignalingContext.tsx` (23 `as any` casts)
- Modify: `mobile/src/context/RoomCallContext.tsx` (5 `as any` casts)

- [x] **Step 1: Create type augmentation file**

```typescript
// mobile/src/types/react-native-webrtc.d.ts
import "react-native-webrtc";

declare module "react-native-webrtc" {
  interface RTCPeerConnection {
    oniceconnectionstatechange: ((ev: Event) => void) | null;
    onicecandidate: ((ev: RTCIceCandidateEvent) => void) | null;
    ontrack: ((ev: RTCTrackEvent) => void) | null;
    ondatachannel: ((ev: RTCDataChannelEvent) => void) | null;
    onnegotiationneeded: ((ev: Event) => void) | null;
    onsignalingstatechange: ((ev: Event) => void) | null;
  }

  interface RTCDataChannel {
    onopen: ((ev: Event) => void) | null;
    onclose: ((ev: Event) => void) | null;
    onmessage: ((ev: RTCDataChannelMessageEvent) => void) | null;
    onerror: ((ev: Event) => void) | null;
  }

  interface RTCIceCandidateEvent {
    candidate: RTCIceCandidate | null;
  }

  interface RTCTrackEvent {
    track: MediaStreamTrack;
    streams: MediaStream[];
  }

  interface RTCDataChannelEvent {
    channel: RTCDataChannel;
  }
}
```

- [x] **Step 2: Verify type augmentation loads**

Run: `cd mobile && npx tsc --noEmit`
Expected: No errors

- [x] **Step 3: Replace `as any` in SignalingContext.tsx**

Review each of the 23 `as any` casts and replace with proper type assertions using the augmented types.

- [x] **Step 4: Replace `as any` in RoomCallContext.tsx**

Review each of the 5 `as any` casts and replace with proper type assertions.

- [x] **Step 5: Run mobile typecheck**

Run: `cd mobile && npx tsc --noEmit`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add mobile/src/types/ mobile/src/context/SignalingContext.tsx mobile/src/context/RoomCallContext.tsx
git commit -m "fix: add WebRTC type augmentation and remove as any casts"
```

---

## Task 3: Decompose AgentLabPanels.tsx

**Files:**
- Create: `web/src/pages/agent/TraceView.tsx`
- Create: `web/src/pages/agent/ResultView.tsx`
- Create: `web/src/pages/agent/WorkflowGraph.tsx`
- Create: `web/src/pages/agent/ApprovalList.tsx`
- Create: `web/src/pages/agent/RerankPanel.tsx`
- Create: `web/src/pages/agent/CitationCard.tsx`
- Modify: `web/src/pages/agent/AgentLabPanels.tsx` (extract and re-export)

- [x] **Step 1: Read current AgentLabPanels.tsx structure**

Run: `head -100 web/src/pages/agent/AgentLabPanels.tsx`

- [x] **Step 2: Extract TraceView component**

Create `web/src/pages/agent/TraceView.tsx` with the TraceView component (~100 lines).

- [x] **Step 3: Extract ResultView component**

Create `web/src/pages/agent/ResultView.tsx` with the ResultView component (~80 lines).

- [x] **Step 4: Extract WorkflowGraph component**

Create `web/src/pages/agent/WorkflowGraph.tsx` with the WorkflowGraph component (~120 lines).

- [x] **Step 5: Extract ApprovalList component**

Create `web/src/pages/agent/ApprovalList.tsx` with the ApprovalList component (~60 lines).

- [x] **Step 6: Extract RerankPanel component**

Create `web/src/pages/agent/RerankPanel.tsx` with the RerankPanel component (~50 lines).

- [x] **Step 7: Extract CitationCard component**

Create `web/src/pages/agent/CitationCard.tsx` with the CitationCard component (~40 lines).

- [x] **Step 8: Update AgentLabPanels.tsx to re-export**

Replace the file content with imports and re-exports of the extracted components.

- [x] **Step 9: Run web tests**

Run: `cd web && npx vitest run`
Expected: PASS

- [x] **Step 10: Commit**

```bash
git add web/src/pages/agent/
git commit -m "refactor: decompose AgentLabPanels.tsx into 6 focused components"
```

---

## Task 4: Add Backend Model Validation Tests
> **前提修正：** 本任务假设 `User/Organization/Conversation/Message` 已存在 `Validate()` 方法，
> 但实测 `backend/internal/models/*.go` 中**没有任何 `Validate()`** —— 照计划写等于先凭空造验证逻辑（行为变更）。
> 已改为给 models 中**真实存在且高价值**的不变量补回归测试：`internal/models/registry_test.go`
> 校验 `AllModels()` 注册表（93 个模型）：条目必须是指针、必须显式实现 `TableName()`、
> 表名必须为 lower_snake_case、且**不得重名**（复制粘贴事故会让两个模型写同一张表）。


**Files:**
- Create: `backend/internal/models/user_test.go`
- Create: `backend/internal/models/organization_test.go`
- Create: `backend/internal/models/conversation_test.go`
- Create: `backend/internal/models/message_test.go`

- [x] **Step 1: Read existing model structures**

Run: `head -50 backend/internal/models/user.go backend/internal/models/organization.go`

- [x] **Step 2: Create user validation tests**

```go
// backend/internal/models/user_test.go
package models

import "testing"

func TestUser_ValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr bool
    }{
        {"valid email", "user@example.com", false},
        {"empty email", "", true},
        {"invalid format", "notanemail", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            u := &User{Email: tt.email}
            if err := u.Validate(); (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

- [x] **Step 3: Create organization validation tests**

Test required fields, name length constraints.

- [x] **Step 4: Create conversation validation tests**

Test required fields, organization association.

- [x] **Step 5: Create message validation tests**

Test required fields, body length, sender validation.

- [x] **Step 6: Run model tests**

Run: `cd backend && go test ./internal/models/...`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add backend/internal/models/*_test.go
git commit -m "test: add validation tests for core data models"
```

---

## Task 5: Fix Hardcoded Chinese Strings in Web
> **实际做法（与计划不同）：** `src/i18n/locales/*.json` 原本不存在，资源是内联在 `src/i18n.ts` 里的。
> 已按计划抽出为 `zh.json` / `en.json`（tsconfig 有 `resolveJsonModule`），并把 3 个文件的硬编码中文
> 全部换成 `t()`：`CallOverlay`(12 处) / `MeetingPreflightPage`(11 处) / `CallProvider`(5 处，比计划多找到 3 处)。
> 另加 `src/i18n.test.ts` 护栏：zh/en key 集合必须完全一致、不能有空值、插值占位符必须两边匹配
> —— 半量 i18n 迁移最常见的回归就是"只加了一种语言"。


**Files:**
- Modify: `web/src/calls/CallOverlay.tsx:14-17`
- Modify: `web/src/pages/meetings/MeetingPreflightPage.tsx`
- Modify: `web/src/calls/CallProvider.tsx:95,104`
- Create: `web/src/i18n/locales/en.json` (if not exists)
- Modify: `web/src/i18n/locales/zh.json`

- [x] **Step 1: Read current i18n setup**

Run: `cat web/src/i18n.ts`

- [x] **Step 2: Add missing translation keys to zh.json**

Add keys for call overlay labels, meeting preflight errors, and call provider errors.

- [x] **Step 3: Add English translations to en.json**

Add corresponding English translations for all new keys.

- [x] **Step 4: Update CallOverlay.tsx to use t()**

Replace hardcoded Chinese strings with `t('callOverlay.close')` etc.

- [x] **Step 5: Update MeetingPreflightPage.tsx to use t()**

Replace hardcoded error messages with translation function calls.

- [x] **Step 6: Update CallProvider.tsx to use t()**

Replace hardcoded error messages with translation function calls.

- [x] **Step 7: Run web tests**

Run: `cd web && npx vitest run`
Expected: PASS

- [x] **Step 8: Commit**

```bash
git add web/src/i18n/ web/src/calls/ web/src/pages/meetings/
git commit -m "fix: replace hardcoded Chinese strings with i18n translations"
```

---

## Task 6: Add Web Page-Level Tests
> **实际做法：** 新增 4 个页面级测试共 15 例（web 测试总数 50 -> 75）。
> `RecordingsPage.test.tsx` 里的 "appends the next offset when loading more" 是 offset 追加分页的回归护栏：
> 断言 `listRecordings` 必须被以 `{limit:50, offset:50}` 调用、两页内容同时在屏、末页按钮消失。
> 若改回"增大 limit 重新拉第 0 页"的假分页，该用例会立刻失败。


**Files:**
- Create: `web/src/pages/auth/LoginPage.test.tsx`
- Create: `web/src/pages/auth/RegisterPage.test.tsx`
- Create: `web/src/pages/meetings/MeetingsPage.test.tsx`
- Create: `web/src/pages/recordings/RecordingsPage.test.tsx`

- [x] **Step 1: Read existing test patterns**

Run: `cat web/src/pages/organizations/OrganizationAdminTabs.test.tsx | head -50`

- [x] **Step 2: Create LoginPage test**

```typescript
// web/src/pages/auth/LoginPage.test.tsx
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LoginPage from "./LoginPage";

describe("LoginPage", () => {
  it("renders login form", () => {
    const queryClient = new QueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <LoginPage />
        </MemoryRouter>
      </QueryClientProvider>
    );
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
  });
});
```

- [x] **Step 3: Create RegisterPage test**

Similar structure, testing registration form fields.

- [x] **Step 4: Create MeetingsPage test**

Test that meetings list renders with empty state.

- [x] **Step 5: Create RecordingsPage test**

Test that recordings list renders with empty state.

- [x] **Step 6: Run web tests**

Run: `cd web && npx vitest run`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add web/src/pages/auth/ web/src/pages/meetings/ web/src/pages/recordings/
git commit -m "test: add page-level tests for auth, meetings, and recordings"
```

---

## Task 7: Update Documentation
> **实际做法：** `scripts/README.md` 原覆盖 5/23 个脚本，已补齐到 **23/23**（新增 backup/ load/ web/
> secret-scan/ interview 栈/ 一次性工具 六个分组）。`infra/README.md` 补 Observability / HA / WAF /
> Load Testing / Backup / Kubernetes+Helm / Cloudflare Tunnel 七节（均对应 infra/ 下真实存在的资产，
> 隧道部分明确标注为 host-specific 参考资料、以 deployment guide 为准）。`SECURITY.md` 补 Scope /
> Severity 分级表 / 披露时间线 / Safe Harbor。


**Files:**
- Modify: `scripts/README.md`
- Modify: `infra/README.md`
- Modify: `SECURITY.md`

- [x] **Step 1: Update scripts/README.md**

Document all 27 scripts with usage examples and descriptions.

- [x] **Step 2: Update infra/README.md**

Add sections for observability, HA, WAF, load testing, backup, Helm, and Cloudflare Tunnel.

- [x] **Step 3: Harden SECURITY.md**

Add responsible disclosure timeline, scope, and severity classification.

- [x] **Step 4: Commit**

```bash
git add scripts/README.md infra/README.md SECURITY.md
git commit -m "docs: update scripts, infra, and security documentation"
```

---

## Task 8: Add Collaborator Documentation
> **前提修正：** 计划称 4 个文件共 100 个导出函数缺 godoc。实测其中 3 个文件（`helpers.go` /
> `collaboration_mappers.go` / `agent_mcp_handler.go`）**已 100% 有注释**，只有
> `internal/knowledge/service.go` 缺 12 个。已为这 12 个补上（含 `WithXxx` 依赖注入语义、
> `RetryDeadLetter` 的组织归属校验、`ChunkText` 的 chunkSize/overlap 回退规则）。


**Files:**
- Modify: `backend/internal/collaboration/helpers.go`
- Modify: `backend/internal/handlers/collaboration_mappers.go`
- Modify: `backend/internal/handlers/agent_mcp_handler.go`
- Modify: `backend/internal/knowledge/service.go`

- [x] **Step 1: Add doc comments to collaboration/helpers.go**

Add godoc comments to all 30 exported functions.

- [x] **Step 2: Add doc comments to collaboration_mappers.go**

Add godoc comments to all 19 exported functions.

- [x] **Step 3: Add doc comments to agent_mcp_handler.go**

Add godoc comments to all 26 exported functions.

- [x] **Step 4: Add doc comments to knowledge/service.go**

Add godoc comments to all 25 exported functions.

- [x] **Step 5: Run go vet**

Run: `cd backend && go vet ./...`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add backend/internal/collaboration/helpers.go backend/internal/handlers/collaboration_mappers.go backend/internal/handlers/agent_mcp_handler.go backend/internal/knowledge/service.go
git commit -m "docs: add godoc comments to undocumented exported functions"
```

---

## Execution Order

1. **Task 1** (swallowed errors) - No dependencies, quick win
2. **Task 5** (i18n fixes) - No dependencies, quick win
3. **Task 2** (WebRTC types) - No dependencies, medium effort
4. **Task 3** (decompose AgentLabPanels) - No dependencies, medium effort
5. **Task 4** (model tests) - No dependencies, medium effort
6. **Task 6** (page tests) - No dependencies, medium effort
7. **Task 8** (documentation) - No dependencies, quick win
8. **Task 7** (update docs) - No dependencies, quick win

**Parallel opportunities:** Tasks 1, 2, 3, 4, 5, 6, 7, 8 are all independent and can be executed in parallel by different agents.

---

## Verification

After all tasks complete:
1. Run `cd backend && go test ./...` - all tests pass
2. Run `cd web && npx vitest run` - all tests pass
3. Run `cd mobile && npx tsc --noEmit` - no type errors
4. Run `cd backend && go vet ./...` - no issues
5. Verify no `as any` casts remain in mobile WebRTC code
6. Verify all Chinese strings in web use i18n
