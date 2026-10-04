import assert from "node:assert/strict";
import { test } from "node:test";

import { decideVersionGate, type AppVersionCheck } from "../versionGate";

// The gate decides whether a user can open the app at all, so the interesting
// cases are the ones where it must NOT block. A client that cannot reach the
// server, or a build nobody has configured a policy for, has to keep working -
// otherwise a backend problem or a typo becomes an outage for everyone
// installed.

const base: AppVersionCheck = {
  platform: "android",
  current_version: "1.0.0",
  min_supported_version: "1.0.0",
  latest_version: "1.0.0",
  update_required: false,
  update_allowed: true,
  policy_configured: true,
  force_update: false,
  message: "",
};

test("a current build is allowed with no prompt", () => {
  assert.equal(decideVersionGate(base).status, "allowed");
});

test("a supported build with a newer release gets an advisory, not a block", () => {
  const state = decideVersionGate({ ...base, update_required: true, latest_version: "1.1.0", force_update: true });
  // force_update is a hint for the blocking screen; a build the server still
  // allows must not be blocked by it, or a typo in the policy locks everyone out.
  assert.equal(state.status, "optional");
});

test("a withdrawn build is blocked", () => {
  const state = decideVersionGate({ ...base, update_allowed: false, update_required: true });
  assert.equal(state.status, "blocked");
});

test("withdrawal wins over a missing policy flag", () => {
  // The server sends policy_configured=false together with allowed=true when it
  // has no row. Blocked must be decided first regardless of that flag.
  const state = decideVersionGate({ ...base, update_allowed: false, policy_configured: false });
  assert.equal(state.status, "blocked");
});

test("a build with no policy configured is allowed through", () => {
  const state = decideVersionGate({ ...base, policy_configured: false });
  assert.equal(state.status, "allowed");
});
