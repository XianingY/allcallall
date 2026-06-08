#!/usr/bin/env node

const baseUrl = process.env.WS_URL || 'ws://localhost:8080/api/v1/chat/ws';
const token = process.env.TOKEN || '';
const organizationId = process.env.ORGANIZATION_ID || '';
const clients = Number.parseInt(process.env.CLIENTS || '10', 10);
const durationMs = Number.parseInt(process.env.DURATION_MS || '10000', 10);

if (typeof WebSocket === 'undefined') {
  console.error('[ws-connections] global WebSocket is not available in this Node runtime. Use Node 22+ or install a ws-based runner.');
  process.exit(2);
}

if (!token || !organizationId) {
  console.error('Usage: TOKEN=<jwt> ORGANIZATION_ID=<id> CLIENTS=10 DURATION_MS=10000 node scripts/load/ws-connections.mjs');
  process.exit(2);
}

const url = new URL(baseUrl);
url.searchParams.set('token', token);
url.searchParams.set('organization_id', organizationId);

let opened = 0;
let closed = 0;
let errors = 0;
let messages = 0;
const sockets = [];

for (let i = 0; i < clients; i += 1) {
  const socket = new WebSocket(url.toString());
  sockets.push(socket);
  socket.addEventListener('open', () => {
    opened += 1;
  });
  socket.addEventListener('message', () => {
    messages += 1;
  });
  socket.addEventListener('error', () => {
    errors += 1;
  });
  socket.addEventListener('close', () => {
    closed += 1;
  });
}

setTimeout(() => {
  for (const socket of sockets) {
    try {
      socket.close();
    } catch {
      // Ignore close races in a smoke/load script.
    }
  }
  setTimeout(() => {
    console.log(JSON.stringify({ clients, duration_ms: durationMs, opened, closed, errors, messages }, null, 2));
    process.exit(errors > 0 ? 1 : 0);
  }, 500);
}, durationMs);
