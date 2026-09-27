import assert from "node:assert/strict";
import test from "node:test";

import { resolveLoadView } from "./loadViewState";

test("resolveLoadView reports empty when nothing has been requested yet", () => {
  assert.equal(
    resolveLoadView({ loading: false, error: null, itemCount: 0 }),
    "empty",
  );
});

test("resolveLoadView reports loading for the first fetch", () => {
  assert.equal(
    resolveLoadView({ loading: true, error: null, itemCount: 0 }),
    "loading",
  );
});

test("resolveLoadView prefers error over the empty state", () => {
  assert.equal(
    resolveLoadView({ loading: false, error: "无法读取商机列表。", itemCount: 0 }),
    "error",
  );
});

test("resolveLoadView keeps reporting error while stale items remain", () => {
  assert.equal(
    resolveLoadView({ loading: false, error: "无法读取协作线程。", itemCount: 3 }),
    "error",
  );
});

test("resolveLoadView reports content while refreshing an existing list", () => {
  assert.equal(
    resolveLoadView({ loading: true, error: null, itemCount: 5 }),
    "content",
  );
});

test("resolveLoadView reports error instead of loading when a retry failed again", () => {
  assert.equal(
    resolveLoadView({ loading: true, error: "无法读取登录会话列表。", itemCount: 0 }),
    "error",
  );
});
