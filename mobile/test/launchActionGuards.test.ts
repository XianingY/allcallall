/**
 * Pure-logic guards for the mobile launch blockers.
 *
 * Two behaviours are locked down here:
 *
 * 1. `resolveRoomsLoadView` — when the organization list fails to load,
 *    data screens must render a load-error + retry state instead of the
 *    "当前工作区还没有会议。" empty copy that turns a network failure into
 *    something that looks like an empty workspace.
 * 2. `createSingleFlight` — the 回拨 / 发送 actions must swallow re-entry
 *    while a previous invocation is still in flight, and convert a rejection
 *    into a result the screen can surface via Alert instead of letting it
 *    escape as an unhandled promise rejection.
 *
 * This suite lives in `mobile/test/` rather than next to the module on
 * purpose: jest's testMatch only covers `src/**`, and the repo's
 * scripts/check-test-coverage.mjs guard fails when one file is claimed by
 * both runners. Placing it outside `src/` keeps the file registered in the
 * `test:unit` explicit list (its only runner) without touching the jest
 * ignore list, which this change is not allowed to edit.
 */
import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  createSingleFlight,
  resolveRoomsLoadView,
} from "../src/screens/launchActionGuards";

describe("resolveRoomsLoadView", () => {
  it("surfaces a failed organization load instead of the empty state", () => {
    const view = resolveRoomsLoadView({
      organizationError: new Error("network down"),
      listError: null,
      hasRooms: false,
    });
    if (view.kind !== "organization-error") {
      assert.fail(`expected organization-error, got ${view.kind}`);
    }
    assert.match(view.message, /network down/);
  });

  it("keeps the organization error ahead of the screen's own load error", () => {
    const view = resolveRoomsLoadView({
      organizationError: new Error("org boom"),
      listError: "rooms boom",
      hasRooms: false,
    });
    assert.equal(view.kind, "organization-error");
  });

  it("reports the screen's own load failure when the organization list is fine", () => {
    const view = resolveRoomsLoadView({
      organizationError: null,
      listError: "无法加载会议列表",
      hasRooms: false,
    });
    if (view.kind !== "list-error") {
      assert.fail(`expected list-error, got ${view.kind}`);
    }
    assert.equal(view.message, "无法加载会议列表");
  });

  it("only shows the empty state when both loads succeeded", () => {
    assert.equal(
      resolveRoomsLoadView({
        organizationError: null,
        listError: null,
        hasRooms: false,
      }).kind,
      "empty",
    );
    assert.equal(
      resolveRoomsLoadView({
        organizationError: null,
        listError: null,
        hasRooms: true,
      }).kind,
      "content",
    );
  });
});

describe("createSingleFlight", () => {
  it("returns the action's value when it succeeds", async () => {
    const flight = createSingleFlight();
    const result = await flight.run(async () => "done");
    assert.deepEqual(result, { status: "ok", value: "done" });
    assert.equal(flight.isBusy(), false);
  });

  it("converts a rejection into an error result the caller can show", async () => {
    const flight = createSingleFlight();
    const result = await flight.run(() =>
      Promise.reject(new Error("update failed")),
    );
    if (result.status !== "error") {
      assert.fail(`expected error, got ${result.status}`);
    }
    assert.equal(result.error.message, "update failed");
    assert.equal(flight.isBusy(), false);
  });

  it("ignores a second invocation while the first is still in flight", async () => {
    const flight = createSingleFlight();
    let openGate: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      openGate = resolve;
    });
    const first = flight.run(async () => {
      await gate;
      return "first";
    });
    assert.equal(flight.isBusy(), true);

    let secondRan = false;
    const second = flight.run(async () => {
      secondRan = true;
      return "duplicate";
    });
    const secondResult = await second;
    assert.equal(secondResult.status, "busy");
    assert.equal(secondRan, false);

    openGate();
    const firstResult = await first;
    assert.deepEqual(firstResult, { status: "ok", value: "first" });
    assert.equal(flight.isBusy(), false);
  });

  it("stays usable after a failure", async () => {
    const flight = createSingleFlight();
    const failed = await flight.run(() => Promise.reject(new Error("boom")));
    assert.equal(failed.status, "error");
    const retried = await flight.run(async () => "recovered");
    assert.deepEqual(retried, { status: "ok", value: "recovered" });
  });
});
