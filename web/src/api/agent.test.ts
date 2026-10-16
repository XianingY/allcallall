import { beforeEach, describe, expect, it, vi } from "vitest";
import { ReadableStream } from "node:stream/web";

const apiRequestMock = vi.hoisted(() => vi.fn());
const getAccessTokenMock = vi.hoisted(() => vi.fn(() => "tok"));
const getOrganizationIdMock = vi.hoisted(() => vi.fn(() => 7));
vi.mock("@/api/http", () => ({
  apiRequest: apiRequestMock,
  getAccessToken: getAccessTokenMock,
  getOrganizationId: getOrganizationIdMock,
}));

import * as agent from "@/api/agent";

beforeEach(() => {
  apiRequestMock.mockReset();
  vi.unstubAllGlobals();
});

describe("agent api", () => {
  it("creates runs and workflows with conversation scope", async () => {
    await agent.createAgentRun(3, "summarize");
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/runs", {
      method: "POST",
      body: JSON.stringify({ conversation_id: 3, goal: "summarize" }),
    });

    await agent.createWorkflow(3, "brief", "meeting_brief");
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/workflows", {
      method: "POST",
      body: JSON.stringify({ conversation_id: 3, goal: "brief", preset: "meeting_brief" }),
    });
  });

  it("gets runs, run events, workflows, and approvals", async () => {
    await agent.getAgentRun(5);
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/runs/5");

    apiRequestMock.mockResolvedValue({ events: [{ id: 1 }] });
    await expect(agent.getAgentRunEvents(5)).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/runs/5/events");

    apiRequestMock.mockResolvedValue({ workflows: [{ id: 2 }] });
    await expect(agent.listWorkflows(3)).resolves.toEqual([{ id: 2 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/workflows?conversation_id=3&limit=25");

    await agent.getWorkflow(6);
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/workflows/6");

    apiRequestMock.mockResolvedValue({ approvals: [{ id: 4 }] });
    await expect(agent.listApprovals("pending")).resolves.toEqual([{ id: 4 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/approvals?status=pending");
  });

  it("submits tool outputs, processes workflows, and decides approvals", async () => {
    await agent.submitAgentToolOutputs(5, "call-1", "approve");
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/runs/5/submit-tool-outputs", {
      method: "POST",
      body: JSON.stringify({ outputs: [{ tool_call_id: "call-1", action: "approve" }] }),
    });

    await agent.processWorkflow(6);
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/workflows/6/process", { method: "POST", body: "{}" });

    await agent.decideApproval(4, "reject");
    expect(apiRequestMock).toHaveBeenCalledWith("/agent/approvals/4/decision", {
      method: "POST",
      body: JSON.stringify({ decision: "reject" }),
    });
  });

  it("streams agent run events and skips malformed blocks", async () => {
    const events: unknown[] = [];
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        const encoder = new TextEncoder();
        controller.enqueue(encoder.encode('data: {"id":1}\n\n'));
        controller.enqueue(encoder.encode("data: not-json\n\n"));
        controller.enqueue(encoder.encode('event: ping\ndata: {"id":2}\n\n'));
        controller.close();
      },
    });
    const fetchMock = vi.fn<(url: string | URL | Request, init?: RequestInit) => Promise<Response>>(
      async () =>
      new Response(body as unknown as BodyInit, { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const controller = new AbortController();
    await agent.streamAgentRun(9, controller.signal, (event) => events.push(event));

    expect(events).toEqual([{ id: 1 }, { id: 2 }]);
    const [url, init] = fetchMock.mock.calls[0];
    const requestInit = init as RequestInit;
    expect(String(url)).toContain("/agent/runs/9/events/stream");
    expect(new Headers(requestInit.headers).get("Authorization")).toBe("Bearer tok");
    expect(new Headers(requestInit.headers).get("X-Organization-ID")).toBe("7");
  });

  it("fails the stream on a non-ok response", async () => {
    const fetchMock = vi.fn<(url: string | URL | Request, init?: RequestInit) => Promise<Response>>(
      async () => new Response("nope", { status: 503 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    await expect(agent.streamAgentRun(9, controller.signal, () => {})).rejects.toThrow("Agent stream failed (503)");
  });
});
