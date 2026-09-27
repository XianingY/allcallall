import assert from "node:assert/strict";
import test from "node:test";

import { canSubmitInput, isPendingAction } from "./submitGuard";

test("canSubmitInput rejects an empty value", () => {
  assert.equal(canSubmitInput("", false), false);
});

test("canSubmitInput rejects whitespace-only input", () => {
  assert.equal(canSubmitInput("   \n ", false), false);
});

test("canSubmitInput rejects while a submit is pending", () => {
  assert.equal(canSubmitInput("Tokyo Distributor Expansion", true), false);
});

test("canSubmitInput accepts trimmed text while idle", () => {
  assert.equal(canSubmitInput("跨境客服升级处理", false), true);
});

test("isPendingAction matches only the in-flight target", () => {
  assert.equal(isPendingAction(7, 7), true);
  assert.equal(isPendingAction(7, 8), false);
});

test("isPendingAction is false when nothing is pending", () => {
  assert.equal(isPendingAction(null, 7), false);
});
