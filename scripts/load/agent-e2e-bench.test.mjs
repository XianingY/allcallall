#!/usr/bin/env node

import { describe, it, before, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { runAgentBenchmark } from "./agent-e2e-bench.mjs";

/**
 * In-process test server that simulates the Agent run lifecycle:
 *   POST /api/v1/agent/runs  → 202 { run: { id, status: "pending", request_id, … } }
 *   GET  /api/v1/agent/runs/:id  → 200 { run: { id, status, … } }
 *
 * After a configurable delay runs transition: pending → running → ready.
 * A fraction of runs are forced to "failed" for coverage.
 */
function createTestServer({ latencyMs = 2, failRate = 0.1 } = {}) {
  const runs = new Map();
  let nextId = 1;
  let seq = 0;

  const server = createServer((req, res) => {
    const url = new URL(req.url, `http://${req.headers.host}`);

    // GET /api/v1/metrics — return empty Prometheus text for metric-delta tests
    if (req.method === "GET" && url.pathname === "/api/v1/metrics") {
      res.writeHead(200, { "Content-Type": "text/plain" });
      res.end("# HELP test_metric A test\n# TYPE test_metric counter\ntest_metric 0\n");
      return;
    }

    // POST /api/v1/agent/runs
    if (req.method === "POST" && url.pathname === "/api/v1/agent/runs") {
      let body = "";
      req.on("data", (chunk) => { body += chunk; });
      req.on("end", () => {
        const parsed = JSON.parse(body || "{}");
        const id = nextId++;
        const requestId = `req-${id}-${Date.now()}`;
        const traceId = `trace-${id}-${Date.now()}`;
        const run = {
          id,
          organization_id: 1,
          user_id: 1,
          conversation_id: parsed.conversation_id || 1,
          idempotency_key: req.headers["idempotency-key"] || "",
          request_id: requestId,
          source: "test",
          runtime_owner: "test",
          status: "pending",
          goal: parsed.goal || "test",
          summary: "",
          action_items: [],
          next_step: "",
          risk_flags: [],
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        };
        runs.set(id, { run, transitions: ["pending"], transitionAt: Date.now() + latencyMs, fail: Math.random() < failRate });
        res.writeHead(202, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ run }));
      });
      return;
    }

    // GET /api/v1/agent/runs/:id
    const getMatch = url.pathname.match(/^\/api\/v1\/agent\/runs\/(\d+)$/);
    if (req.method === "GET" && getMatch) {
      const id = Number(getMatch[1]);
      const entry = runs.get(id);
      if (!entry) {
        res.writeHead(404, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ error: "not found" }));
        return;
      }
      // Advance lifecycle
      const now = Date.now();
      if (now >= entry.transitionAt) {
        if (entry.run.status === "pending") {
          entry.run.status = "running";
          entry.transitions.push("running");
          entry.transitionAt = now + latencyMs;
        } else if (entry.run.status === "running") {
          entry.run.status = entry.fail ? "failed" : "ready";
          entry.transitions.push(entry.run.status);
        }
        entry.run.updated_at = new Date().toISOString();
      }
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ run: entry.run }));
      return;
    }

    res.writeHead(404);
    res.end();
  });

  return server;
}

function listen(server) {
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      resolve(`http://127.0.0.1:${addr.port}`);
    });
  });
}

function close(server) {
  return new Promise((resolve, reject) => {
    server.close((err) => (err ? reject(err) : resolve()));
  });
}

describe("agent-e2e-bench", () => {
  let server;
  let url;

  before(async () => {
    server = createTestServer({ latencyMs: 5, failRate: 0 });
    url = await listen(server);
  });

  after(async () => {
    if (server) await close(server);
  });

  it("reports enqueue-to-terminal phases separately", async () => {
    const report = await runAgentBenchmark({
      baseUrl: url,
      concurrency: 2,
      runs: 4,
      pollIntervalMs: 5,
      terminalTimeoutMs: 500,
    });

    assert.equal(report.ready, 4);
    assert.equal(report.failed, 0);
    assert.ok(report.enqueueLatency.p95 >= 0);
    assert.ok(report.queueLatency.p95 >= 0);
    assert.ok(report.endToEndLatency.p95 >= report.enqueueLatency.p95);
  });

  it("counts accepted, ready, failed, and timedOut runs", async () => {
    const report = await runAgentBenchmark({
      baseUrl: url,
      concurrency: 1,
      runs: 3,
      pollIntervalMs: 5,
      terminalTimeoutMs: 500,
    });

    assert.equal(report.accepted, 3);
    assert.equal(report.ready + report.failed + report.timedOut, 3);
    assert.ok(report.statusCounts);
  });

  it("captures metricDeltas when metricsUrl resolves", async () => {
    const report = await runAgentBenchmark({
      baseUrl: url,
      metricsUrl: `${url}/api/v1/metrics`,
      concurrency: 1,
      runs: 2,
      pollIntervalMs: 5,
      terminalTimeoutMs: 500,
    });

    assert.ok(report.metricDeltas !== undefined);
    // Even with an empty Prometheus endpoint, the structure should exist
    assert.ok(typeof report.metricDeltas === "object");
  });

  it("records repository SHAs and run metadata", async () => {
    const report = await runAgentBenchmark({
      baseUrl: url,
      concurrency: 1,
      runs: 1,
      pollIntervalMs: 5,
      terminalTimeoutMs: 500,
    });

    assert.ok(report.runId);
    assert.ok(report.repositoryShas);
    // SHA fields may be "unknown" if git is unavailable, but must be present
    assert.ok(typeof report.repositoryShas.allcallall === "string");
    assert.ok(typeof report.repositoryShas.agentRuntime === "string");
  });

  it("never exposes bearer tokens in report output", async () => {
    const secret = "super-secret-token-do-not-log";
    const report = await runAgentBenchmark({
      baseUrl: url,
      token: secret,
      concurrency: 1,
      runs: 1,
      pollIntervalMs: 5,
      terminalTimeoutMs: 500,
    });

    const serialized = JSON.stringify(report);
    assert.ok(!serialized.includes(secret), "report must not contain the bearer token");
  });

  it("computes percentiles on latency distributions", async () => {
    const report = await runAgentBenchmark({
      baseUrl: url,
      concurrency: 2,
      runs: 6,
      pollIntervalMs: 5,
      terminalTimeoutMs: 500,
    });

    for (const key of ["enqueueLatency", "queueLatency", "runtimeLatency", "endToEndLatency"]) {
      assert.ok(report[key], `missing ${key}`);
      assert.ok(typeof report[key].p50 === "number", `${key}.p50 must be a number`);
      assert.ok(typeof report[key].p95 === "number", `${key}.p95 must be a number`);
      assert.ok(typeof report[key].p99 === "number", `${key}.p99 must be a number`);
      assert.ok(typeof report[key].max === "number", `${key}.max must be a number`);
      assert.ok(report[key].p50 <= report[key].p95, `${key}: p50 <= p95`);
      assert.ok(report[key].p95 <= report[key].p99, `${key}: p95 <= p99`);
      assert.ok(report[key].p99 <= report[key].max, `${key}: p99 <= max`);
    }
  });
});
