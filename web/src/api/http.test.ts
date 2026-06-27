import { beforeEach, describe, expect, it, vi } from "vitest";

import { apiRequest, setAccessToken } from "@/api/http";

const jsonResponse = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("apiRequest", () => {
  beforeEach(() => { setAccessToken(null); vi.restoreAllMocks(); });

  it("coalesces concurrent refreshes and retries each request once", async () => {
    let refreshCount = 0;
    const attempts = new Map<string, number>();
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/auth/refresh")) { refreshCount += 1; return jsonResponse({ access_token: "new-token" }); }
      const count = attempts.get(url) ?? 0;
      attempts.set(url, count + 1);
      return count === 0 ? jsonResponse({ success: false, error: "expired" }, 401) : jsonResponse({ ok: true });
    }));

    const [first, second] = await Promise.all([apiRequest<{ ok: boolean }>("/first"), apiRequest<{ ok: boolean }>("/second")]);
    expect(first.ok).toBe(true);
    expect(second.ok).toBe(true);
    expect(refreshCount).toBe(1);
  });
});

