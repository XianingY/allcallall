#!/usr/bin/env node

/**
 * fake-agent-provider.mjs — Deterministic OpenAI-compatible /chat/completions provider.
 *
 * Exposes a single POST /v1/chat/completions endpoint that derives delay, failure,
 * timeout, and response size deterministically from the request sequence number.
 *
 * Environment variables:
 *   FAKE_PROVIDER_LATENCY_MS     — base delay per request (default: 50)
 *   FAKE_PROVIDER_FAILURE_RATE   — probability of returning an error (0–1, default: 0)
 *   FAKE_PROVIDER_TIMEOUT_RATE   — probability of simulating a timeout (0–1, default: 0)
 *   FAKE_PROVIDER_RESPONSE_BYTES — approximate response body size in bytes (default: 256)
 *   FAKE_PROVIDER_PORT           — port to listen on (default: 0 = ephemeral)
 *
 * Never prints bearer tokens or provider credentials.
 */

import { createServer } from "node:http";

function envInt(name, fallback) {
  const raw = process.env[name];
  if (!raw) return fallback;
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) && n >= 0 ? n : fallback;
}

function envFloat(name, fallback) {
  const raw = process.env[name];
  if (!raw) return fallback;
  const n = Number.parseFloat(raw);
  return Number.isFinite(n) && n >= 0 && n <= 1 ? n : fallback;
}

/** Redact bearer tokens and provider API keys from a string. */
function redact(s) {
  return s
    .replace(/Bearer\s+\S+/gi, "Bearer [REDACTED]")
    .replace(/sk-[A-Za-z0-9_-]+/g, "sk-[REDACTED]");
}

// Deterministic pseudo-random from sequence number (simple LCG)
function seededRandom(seed) {
  let s = (seed * 1664525 + 1013904223) & 0x7fffffff;
  return () => {
    s = (s * 1103515245 + 12345) & 0x7fffffff;
    return s / 0x7fffffff;
  };
}

function main() {
  const latencyMs = envInt("FAKE_PROVIDER_LATENCY_MS", 50);
  const failureRate = envFloat("FAKE_PROVIDER_FAILURE_RATE", 0);
  const timeoutRate = envFloat("FAKE_PROVIDER_TIMEOUT_RATE", 0);
  const responseBytes = envInt("FAKE_PROVIDER_RESPONSE_BYTES", 256);
  const port = envInt("FAKE_PROVIDER_PORT", 0);

  let seq = 0;

  const server = createServer((req, res) => {
    const url = new URL(req.url, `http://${req.headers.host}`);

    // Health check
    if (req.method === "GET" && url.pathname === "/health") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ status: "ok", seq }));
      return;
    }

    // OpenAI-compatible chat completions
    if (req.method === "POST" && url.pathname === "/v1/chat/completions") {
      const currentSeq = seq++;
      const rng = seededRandom(currentSeq);

      let body = "";
      req.on("data", (chunk) => { body += chunk; });
      req.on("end", () => {
        // Log request with redacted credentials
        const authHeader = req.headers["authorization"] || "";
        console.log(redact(`[fake-agent-provider] seq=${currentSeq} method=${req.method} path=${url.pathname} auth=${authHeader}`));

        // Deterministic timeout simulation
        if (rng() < timeoutRate) {
          // Simulate timeout: delay 10x the normal latency, then close
          setTimeout(() => {
            if (!res.writableEnded) {
              res.destroy();
            }
          }, latencyMs * 10);
          return;
        }

        // Deterministic failure simulation
        if (rng() < failureRate) {
          const delay = Math.round(latencyMs * (0.5 + rng()));
          setTimeout(() => {
            res.writeHead(500, { "Content-Type": "application/json" });
            res.end(JSON.stringify({
              error: { message: "fake provider simulated failure", type: "server_error", seq: currentSeq },
            }));
          }, delay);
          return;
        }

        // Normal response with configurable latency and size
        const delay = Math.round(latencyMs * (0.8 + rng() * 0.4));
        // Build a response body approximately responseBytes long
        const contentPadding = "x".repeat(Math.max(0, responseBytes - 200));
        const content = `Benchmark response seq=${currentSeq}. ${contentPadding}`;

        const response = {
          id: `chatcmpl-fake-${currentSeq}`,
          object: "chat.completion",
          created: Math.floor(Date.now() / 1000),
          model: "fake-provider",
          choices: [{
            index: 0,
            message: { role: "assistant", content },
            finish_reason: "stop",
          }],
          usage: { prompt_tokens: 10, completion_tokens: Math.ceil(content.length / 4), total_tokens: 10 + Math.ceil(content.length / 4) },
        };

        setTimeout(() => {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify(response));
        }, delay);
      });
      return;
    }

    // Models list (minimal)
    if (req.method === "GET" && url.pathname === "/v1/models") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({
        object: "list",
        data: [{ id: "fake-provider", object: "model", owned_by: "benchmark" }],
      }));
      return;
    }

    res.writeHead(404, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ error: "not found" }));
  });

  server.listen(port, "127.0.0.1", () => {
    const addr = server.address();
    console.log(`[fake-agent-provider] listening on 127.0.0.1:${addr.port} latency=${latencyMs}ms failure=${failureRate} timeout=${timeoutRate} responseBytes=${responseBytes}`);
  });
}

main();
