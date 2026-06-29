#!/usr/bin/env node

const requiredEnv = ["BASE_URL", "TOKEN", "ORGANIZATION_ID", "CONVERSATION_ID"];

function requireEnv(name) {
  const value = (process.env[name] || "").trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function intEnv(name, fallback) {
  const raw = (process.env[name] || "").trim();
  if (!raw) return fallback;
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return value;
}

function normalizeScenario(raw) {
  const scenario = (raw || "get_messages").trim();
  const allowed = new Set(["get_messages", "post_message", "post_agent_run"]);
  if (!allowed.has(scenario)) {
    throw new Error(`SCENARIO must be one of: ${Array.from(allowed).join(", ")}`);
  }
  return scenario;
}

function percentile(values, p) {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const idx = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * p) - 1));
  return Math.round(sorted[idx]);
}

function requestSpec({ scenario, baseURL, conversationID, requestID }) {
  if (scenario === "get_messages") {
    return {
      method: "GET",
      url: `${baseURL}/api/v1/conversations/${conversationID}/messages`,
    };
  }
  if (scenario === "post_message") {
    return {
      method: "POST",
      url: `${baseURL}/api/v1/conversations/${conversationID}/messages`,
      body: {
        type: "text",
        body: `api qps bench message ${requestID}`,
        metadata: { source: "api-qps-bench", request_id: requestID },
      },
    };
  }
  return {
    method: "POST",
    url: `${baseURL}/api/v1/agent/runs`,
    idempotencyKey: `api-qps-bench:${Date.now()}:${requestID}`,
    body: {
      conversation_id: Number(conversationID),
      goal: "benchmark agent run creation only; do not use as product quality evidence",
    },
  };
}

async function timedFetch(spec, headers, timeoutMs) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  const started = performance.now();
  try {
    const response = await fetch(spec.url, {
      method: spec.method,
      headers: {
        ...headers,
        ...(spec.idempotencyKey ? { "Idempotency-Key": spec.idempotencyKey } : {}),
      },
      body: spec.body ? JSON.stringify(spec.body) : undefined,
      signal: controller.signal,
    });
    const elapsedMs = performance.now() - started;
    let sample = "";
    if (!response.ok) {
      sample = (await response.text()).slice(0, 240);
    } else {
      await response.arrayBuffer();
    }
    return {
      ok: response.ok,
      status: response.status,
      latencyMs: elapsedMs,
      error: response.ok ? "" : sample,
    };
  } catch (err) {
    return {
      ok: false,
      status: 0,
      latencyMs: performance.now() - started,
      error: err && err.name === "AbortError" ? "request_timeout" : String(err),
    };
  } finally {
    clearTimeout(timeout);
  }
}

async function main() {
  for (const name of requiredEnv) {
    requireEnv(name);
  }
  const baseURL = requireEnv("BASE_URL").replace(/\/+$/, "");
  const token = requireEnv("TOKEN");
  const organizationID = requireEnv("ORGANIZATION_ID");
  const conversationID = requireEnv("CONVERSATION_ID");
  const scenario = normalizeScenario(process.env.SCENARIO);
  const concurrency = intEnv("CONCURRENCY", 10);
  const durationSeconds = intEnv("DURATION_SECONDS", 30);
  const requestTimeoutMs = intEnv("REQUEST_TIMEOUT_MS", 10000);

  const headers = {
    Authorization: `Bearer ${token}`,
    "Content-Type": "application/json",
    "X-Organization-ID": organizationID,
  };
  const deadline = Date.now() + durationSeconds * 1000;
  const latencies = [];
  const statusCounts = {};
  const sampleErrors = [];
  let requests = 0;
  let successes = 0;
  let failures = 0;

  async function worker(workerID) {
    let local = 0;
    while (Date.now() < deadline) {
      const requestID = `${workerID}-${local++}-${requests + 1}`;
      const spec = requestSpec({ scenario, baseURL, conversationID, requestID });
      const result = await timedFetch(spec, headers, requestTimeoutMs);
      requests++;
      latencies.push(result.latencyMs);
      statusCounts[result.status] = (statusCounts[result.status] || 0) + 1;
      if (result.ok) {
        successes++;
      } else {
        failures++;
        if (sampleErrors.length < 5) {
          sampleErrors.push({ status: result.status, error: result.error });
        }
      }
    }
  }

  const startedAt = new Date();
  const started = performance.now();
  await Promise.all(Array.from({ length: concurrency }, (_, idx) => worker(idx + 1)));
  const durationMs = performance.now() - started;
  const durationSec = durationMs / 1000;

  const output = {
    started_at: startedAt.toISOString(),
    scenario,
    base_url: baseURL,
    organization_id: Number(organizationID),
    conversation_id: Number(conversationID),
    concurrency,
    duration_seconds: durationSeconds,
    measured_duration_sec: Number(durationSec.toFixed(3)),
    requests,
    successes,
    failures,
    qps: durationSec > 0 ? Number((requests / durationSec).toFixed(2)) : 0,
    error_rate: requests > 0 ? Number((failures / requests).toFixed(4)) : 0,
    latency_ms: {
      count: latencies.length,
      p50: percentile(latencies, 0.50),
      p95: percentile(latencies, 0.95),
      p99: percentile(latencies, 0.99),
      max: latencies.length ? Math.round(Math.max(...latencies)) : 0,
    },
    status_counts: statusCounts,
    sample_errors: sampleErrors,
  };
  console.log(JSON.stringify(output, null, 2));
}

main().catch((err) => {
  console.error(`[api-qps-bench] ${err.message}`);
  process.exit(2);
});
