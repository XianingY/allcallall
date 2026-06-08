import assert from "node:assert/strict";
import test from "node:test";

import type {
  ConversationDetailRecord,
  ConversationRecord,
} from "../api/collaboration";
import {
  applyConversationDetailPatch,
  applyConversationListPatch,
} from "./conversationRealtimeReducer";

const conversation = (overrides: Partial<ConversationRecord> = {}): ConversationRecord => ({
  id: 1,
  organization_id: 10,
  type: "direct",
  title: "Customer issue",
  status: "open",
  priority: "normal",
  unread_count: 0,
  ...overrides,
});

const detail = (record: ConversationRecord = conversation()): ConversationDetailRecord => ({
  conversation: record,
  workspace: {
    status: record.status,
    priority: record.priority,
    assignee_user_id: record.assignee_user_id,
    assignee_label: record.assignee_display_name || record.assignee_email || "未指派",
  },
});

test("applyConversationListPatch updates only the targeted conversation", () => {
  const first = conversation({ id: 1, status: "open" });
  const second = conversation({ id: 2, status: "open" });

  const next = applyConversationListPatch([first, second], {
    conversation_id: 2,
    changes: { status: "pending", priority: "high" },
  });

  assert.equal(next[0], first);
  assert.notEqual(next[1], second);
  assert.equal(next[1].status, "pending");
  assert.equal(next[1].priority, "high");
});

test("applyConversationListPatch preserves array reference when event is irrelevant", () => {
  const items = [conversation({ id: 1 })];

  assert.equal(applyConversationListPatch(items, undefined), items);
  assert.equal(applyConversationListPatch(items, { conversation_id: 9, changes: { status: "pending" } }), items);
});

test("applyConversationDetailPatch updates conversation and workspace summary", () => {
  const previous = detail(conversation({
    assignee_user_id: 1,
    assignee_display_name: "Alice",
  }));

  const next = applyConversationDetailPatch(previous, {
    conversation_id: 1,
    changes: {
      assignee_user_id: 2,
      assignee_display_name: "Bob",
      status: "resolved",
      priority: "urgent",
    },
  });

  assert.notEqual(next, previous);
  assert.equal(next?.conversation.assignee_user_id, 2);
  assert.equal(next?.workspace.assignee_user_id, 2);
  assert.equal(next?.workspace.assignee_label, "Bob");
  assert.equal(next?.workspace.status, "resolved");
  assert.equal(next?.workspace.priority, "urgent");
});

test("applyConversationDetailPatch ignores updates for other conversations", () => {
  const previous = detail();
  assert.equal(
    applyConversationDetailPatch(previous, {
      conversation_id: 99,
      changes: { status: "pending" },
    }),
    previous
  );
});
