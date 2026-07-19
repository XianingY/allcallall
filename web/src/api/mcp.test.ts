import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  activateMCPInstallation,
  createAgentSkill,
  createMCPInstallation,
  deleteAgentSkill,
  disableMCPInstallation,
  getMCPExecution,
  listAgentSkills,
  listMCPInstallationTools,
  publishMCPInstallation,
  putMCPInstallationSecrets,
  updateAgentSkill,
  validateMCPInstallation,
} from "@/api/mcp";
import { setAccessToken, setOrganizationId } from "@/api/http";

const jsonResponse = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("MCP API client", () => {
  beforeEach(() => { setAccessToken(null); setOrganizationId(null); vi.restoreAllMocks(); });

  it("sends a generated installation payload and lifecycle requests", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/secrets")) return jsonResponse({ secrets_configured: true });
      if (url.endsWith("/tools")) return jsonResponse({ tools: [{ id: 9, risk: "read" }] });
      if (url.endsWith("/installations") || /\/(validate|activate|publish)$/.test(url)) return jsonResponse({ installation: { id: 42, display_name: "Search", status: "active" } }, url.endsWith("/installations") ? 201 : 200);
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await createMCPInstallation({ scope: "personal", display_name: "Search", source_type: "https", transport: "streamable_http", endpoint_url: "https://mcp.example.com/v1", network_allowlist: ["mcp.example.com"] });
    await validateMCPInstallation(42);
    await activateMCPInstallation(42);
    await publishMCPInstallation(42);
    await putMCPInstallationSecrets(42, { API_TOKEN: "secret-value" });
    const tools = await listMCPInstallationTools(42);
    await disableMCPInstallation(42);

    expect(toRequest(fetchMock, 0)).toMatchObject({ url: "/api/v1/agent/mcp/installations", method: "POST", body: { scope: "personal", source_type: "https", endpoint_url: "https://mcp.example.com/v1" } });
    expect(fetchMock.mock.calls.slice(1, 4).map(([input]) => String(input))).toEqual(["/api/v1/agent/mcp/installations/42/validate", "/api/v1/agent/mcp/installations/42/activate", "/api/v1/agent/mcp/installations/42/publish"]);
    expect(toRequest(fetchMock, 4)).toMatchObject({ url: "/api/v1/agent/mcp/installations/42/secrets", method: "POST", body: { secrets: { API_TOKEN: "secret-value" } } });
    expect(tools[0]?.id).toBe(9);
    expect(toRequest(fetchMock, 6)).toMatchObject({ url: "/api/v1/agent/mcp/installations/42", method: "DELETE" });
  });

  it("maps Skill CRUD and escaped execution ids", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/agent/skills") && (!init?.method || init.method === "GET")) return jsonResponse({ skills: [{ id: 3, name: "CRM" }] });
      if (url.endsWith("/agent/skills")) return jsonResponse({ skill: { id: 3, name: "CRM" } }, 201);
      if (url.endsWith("/agent/skills/3") && init?.method === "PATCH") return jsonResponse({ skill: { id: 3, name: "CRM v2" } });
      if (url.includes("/agent/mcp/executions/")) return jsonResponse({ execution: { execution_id: "run/42 call" } });
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal("fetch", fetchMock);

    expect(await listAgentSkills()).toHaveLength(1);
    await createAgentSkill({ scope: "personal", name: "CRM", instructions: "Read first", tool_ids: [7, 8] });
    await updateAgentSkill(3, { status: "disabled", tool_ids: [8] });
    await deleteAgentSkill(3);
    const execution = await getMCPExecution("run/42 call");

    expect(toRequest(fetchMock, 1).body).toMatchObject({ scope: "personal", tool_ids: [7, 8] });
    expect(toRequest(fetchMock, 2)).toMatchObject({ method: "PATCH", body: { status: "disabled", tool_ids: [8] } });
    expect(toRequest(fetchMock, 3).method).toBe("DELETE");
    expect(String(fetchMock.mock.calls[4]?.[0])).toBe("/api/v1/agent/mcp/executions/run%2F42%20call");
    expect(execution.execution_id).toBe("run/42 call");
  });
});

function toRequest(fetchMock: ReturnType<typeof vi.fn>, index: number) {
  const [input, init] = fetchMock.mock.calls[index] as [string | URL | Request, RequestInit | undefined];
  return { url: String(input), method: init?.method ?? "GET", body: typeof init?.body === "string" ? JSON.parse(init.body) as unknown : undefined };
}
