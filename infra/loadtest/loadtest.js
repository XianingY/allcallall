// AllCallAll 全链路压测脚本（k6）
//
// 覆盖真实用户关键路径：登录 → 发起聊天 → 触发 Agent 运行 → 知识库检索 →
// 查询用量/状态。用于上线前容量评估与回归基准。
//
// 运行：
//   k6 run -e BASE_URL=https://api.allcallall.example.com \
//          -e USER_PREFIX=loadtest -e USER_COUNT=200 \
//          -e AUTH_TOKEN=<valid-test-token> \
//          infra/loadtest/loadtest.js
//
// 或分阶段：
//   k6 run --stage 2m:50,5m:200,2m:50  infra/loadtest/loadtest.js
//
// 退出阈值（thresholds）定义了 SLA 红线，未达标即视为压测失败（CI 可据此阻断）。

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate } from 'k6/metrics';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const AUTH_TOKEN = __ENV.AUTH_TOKEN || '';
const USER_PREFIX = __ENV.USER_PREFIX || 'loadtest';

// 自定义失败率指标
const errorRate = new Rate('app_errors');

export const options = {
  // 渐进式负载：ramp-up →  plateau → ramp-down，避免冷启动干扰基线
  stages: [
    { duration: '2m', target: 50 },   // 预热
    { duration: '5m', target: 200 },  // 目标峰值
    { duration: '2m', target: 200 },  // 持续 plateau 观察稳定性
    { duration: '1m', target: 0 },    // 优雅退出
  ],
  thresholds: {
    http_req_duration: ['p(95)<800', 'p(99)<1500'], // 接口 95% < 800ms
    http_req_failed: ['rate<0.01'],                  // 错误率 < 1%
    app_errors: ['rate<0.02'],                       // 业务错误（非 2xx/3xx） < 2%
  },
  ext: {
    loadimpact: { name: 'AllCallAll full-chain soak' },
  },
};

function authHeaders() {
  return {
    headers: {
      'Authorization': `Bearer ${AUTH_TOKEN}`,
      'Content-Type': 'application/json',
      'User-Agent': 'k6-loadtest',
    },
  };
}

// 每个 VU 复用独立身份，模拟不同租户
function vuUser() {
  return `${USER_PREFIX}-${exec.vu.idInTest}`;
}

export default function () {
  const base = BASE_URL;
  const h = authHeaders();

  group('status-probe', () => {
    const r = http.get(`${base}/api/v1/status`, h);
    const ok = check(r, {
      'status page 200': (r) => r.status === 200,
      'status reports ok/degraded': (r) => {
        try { return ['ok', 'degraded'].includes(r.json().status); }
        catch (e) { return false; }
      },
    });
    errorRate.add(!ok);
    sleep(1);
  });

  let conversationId = null;
  group('chat-flow', () => {
    const payload = JSON.stringify({
      message: `loadtest ping from ${vuUser()}`,
      stream: false,
    });
    const r = http.post(`${base}/api/v1/chat/conversations`, payload, h);
    check(r, { 'chat create 2xx': (r) => r.status < 300 });
    errorRate.add(r.status >= 400);
    if (r.status < 300) {
      try { conversationId = r.json().id; } catch (e) {}
    }
    sleep(2);
  });

  group('agent-run', () => {
    const payload = JSON.stringify({
      prompt: `summarize the latest project status for ${vuUser()}`,
      mode: 'sync',
    });
    const r = http.post(`${base}/api/v1/agent/runs`, payload, h);
    check(r, { 'agent run accepted 2xx': (r) => r.status < 300 });
    errorRate.add(r.status >= 400);
    sleep(3);
  });

  group('knowledge-search', () => {
    const r = http.get(
      `${base}/api/v1/knowledge/search?q=${encodeURIComponent('compliance policy')}`,
      h
    );
    check(r, { 'knowledge search 2xx': (r) => r.status < 300 });
    errorRate.add(r.status >= 400);
    sleep(1);
  });

  group('usage-dashboard', () => {
    const r = http.get(`${base}/api/v1/commerce/usage?period=current`, h);
    check(r, { 'usage 2xx': (r) => r.status < 300 });
    errorRate.add(r.status >= 400);
    sleep(1);
  });

  // 思考时间，模拟真实用户节奏
  sleep(Math.random() * 3 + 1);
}
