import { beforeEach, describe, expect, it, vi } from "vitest";

import { apiDownload, apiRequest, setAccessToken, setOrganizationId } from "@/api/http";

const jsonResponse = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("apiRequest", () => {
  beforeEach(() => { setAccessToken(null); setOrganizationId(null); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

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

  it("returns parsed JSON body and sends the bearer token on success", async () => {
    let authHeader: string | null = "";
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      authHeader = new Headers(init?.headers).get("Authorization");
      return jsonResponse({ ok: true, value: 42 });
    }));
    setAccessToken("tok-123");

    const data = await apiRequest<{ ok: boolean; value: number }>("/things");
    expect(data.value).toBe(42);
    expect(authHeader).toBe("Bearer tok-123");
  });

  it("sends the organization id header when set", async () => {
    let orgHeader: string | null = "";
    vi.stubGlobal("fetch", vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      orgHeader = new Headers(init?.headers).get("X-Organization-ID");
      return jsonResponse({ ok: true });
    }));
    setOrganizationId(99);

    await apiRequest<{ ok: boolean }>("/things");
    expect(orgHeader).toBe("99");
  });

  it("does not send an Authorization header when auth is disabled", async () => {
    let authHeader: string | null = "unset";
    vi.stubGlobal("fetch", vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      authHeader = new Headers(init?.headers).get("Authorization");
      return jsonResponse({ ok: true });
    }));
    setAccessToken("tok-123");

    await apiRequest<{ ok: boolean }>("/public", {}, { auth: false });
    expect(authHeader).toBeNull();
  });

  it("returns undefined for a 204 No Content response", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    const data = await apiRequest<{ ok: boolean }>("/things");
    expect(data).toBeUndefined();
  });

  it("returns undefined when the response is not JSON", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("plain text", { status: 200, headers: { "content-type": "text/plain" } })));
    const data = await apiRequest<{ ok: boolean }>("/things");
    expect(data).toBeUndefined();
  });

  it("throws an APIError carrying code, message and request id on failure", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({ code: "NOT_FOUND", error: "missing", request_id: "req-9" }, 404)));
    await expect(apiRequest("/things")).rejects.toMatchObject({
      status: 404,
      code: "NOT_FOUND",
      message: "missing",
      requestId: "req-9",
    });
  });

  it("falls back to statusText when the error body is not JSON", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("boom", { status: 500, statusText: "Internal Server Error" })));
    await expect(apiRequest("/things")).rejects.toMatchObject({ status: 500, code: "HTTP_ERROR", message: "Internal Server Error" });
  });

  it("does not refresh when retry401 is disabled and the response is 401", async () => {
    let refreshCalls = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      if (String(input).endsWith("/auth/refresh")) { refreshCalls += 1; return jsonResponse({ access_token: "x" }); }
      return jsonResponse({ code: "UNAUTHORIZED", error: "no" }, 401);
    }));
    setAccessToken("tok");

    await expect(apiRequest("/things", {}, { retry401: false })).rejects.toMatchObject({ status: 401 });
    expect(refreshCalls).toBe(0);
  });

  it("clears the access token when the refresh itself fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      if (String(input).endsWith("/auth/refresh")) return jsonResponse({ code: "DENIED", error: "bad refresh" }, 401);
      return jsonResponse({ code: "UNAUTHORIZED", error: "no" }, 401);
    }));
    setAccessToken("tok");

    await expect(apiRequest("/things")).rejects.toMatchObject({ status: 401, code: "DENIED" });
    expect(setAccessToken).toBeDefined();
    expect((await import("@/api/http")).getAccessToken()).toBeNull();
  });
});

describe("apiDownload", () => {
  beforeEach(() => { setAccessToken(null); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it("extracts the filename from the content-disposition header", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Blob(["x"]), {
      status: 200,
      headers: { "content-disposition": 'attachment; filename="report.pdf"' },
    })));
    const { fileName } = await apiDownload("/files/1");
    expect(fileName).toBe("report.pdf");
  });

  it("returns undefined filename when the header is absent", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Blob(["x"]), { status: 200 })));
    const { fileName, blob } = await apiDownload("/files/1");
    expect(fileName).toBeUndefined();
    expect(blob).toBeDefined();
    expect((blob as { size: number }).size).toBeGreaterThanOrEqual(0);
  });

  it("refreshes once on 401 and retries", async () => {
    let refreshCalls = 0;
    let first = true;
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      if (String(input).endsWith("/auth/refresh")) { refreshCalls += 1; return jsonResponse({ access_token: "new" }); }
      if (first) { first = false; return new Response(null, { status: 401 }); }
      return new Response(new Blob(["ok"]), { status: 200 });
    }));
    const { blob } = await apiDownload("/files/1");
    expect(refreshCalls).toBe(1);
    expect(blob).toBeDefined();
    expect((blob as { size: number }).size).toBeGreaterThanOrEqual(0);
  });
});


