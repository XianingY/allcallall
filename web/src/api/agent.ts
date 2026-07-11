import type { components } from "@allcallall/api-types";
import { apiRequest, getAccessToken, getOrganizationId } from "@/api/http";
import { runtimeConfig } from "@/lib/runtime-config";

export type AgentRun = components["schemas"]["AgentRun"];
export type AgentRunResult = components["schemas"]["AgentRunResult"];
export type AgentStep = components["schemas"]["AgentStep"];
export type AgentToolCall = components["schemas"]["AgentToolCall"];
export type AgentCitation = components["schemas"]["AgentCitation"];
export type WorkflowRun = components["schemas"]["WorkflowRun"];
export type WorkflowTask = components["schemas"]["WorkflowTask"];
export type ToolApproval = components["schemas"]["ToolApproval"];
export type WorkflowResult = components["schemas"]["WorkflowResult"];

const query = (values: Record<string, string | number | undefined>) => { const params = new URLSearchParams(); Object.entries(values).forEach(([key, value]) => { if (value !== undefined && value !== "") params.set(key, String(value)); }); return params.toString() ? `?${params}` : ""; };

export const createAgentRun = (conversationId: number, goal: string) => apiRequest<AgentRunResult>("/agent/runs", { method: "POST", body: JSON.stringify({ conversation_id: conversationId, goal }) });
export const getAgentRun = (id: number) => apiRequest<AgentRunResult>(`/agent/runs/${id}`);
export const getAgentRunEvents = (id: number) => apiRequest<{ events: Array<Record<string, unknown>> }>(`/agent/runs/${id}/events`).then((value) => value.events ?? []);
export const createWorkflow = (conversationId: number, goal: string, preset: string) => apiRequest<WorkflowResult>("/agent/workflows", { method: "POST", body: JSON.stringify({ conversation_id: conversationId, goal, preset }) });
export const listWorkflows = (conversationId?: number) => apiRequest<{ workflows: WorkflowResult[] }>(`/agent/workflows${query({ conversation_id: conversationId, limit: 25 })}`).then((value) => value.workflows ?? []);
export const getWorkflow = (id: number) => apiRequest<WorkflowResult>(`/agent/workflows/${id}`);
export const processWorkflow = (id: number) => apiRequest<WorkflowResult>(`/agent/workflows/${id}/process`, { method: "POST", body: "{}" });
export const listApprovals = (status?: string) => apiRequest<{ approvals: ToolApproval[] }>(`/agent/approvals${query({ status })}`).then((value) => value.approvals ?? []);
export const decideApproval = (id: number, decision: "approve" | "reject") => apiRequest<WorkflowResult>(`/agent/approvals/${id}/decision`, { method: "POST", body: JSON.stringify({ decision }) });

export async function streamAgentRun(id: number, signal: AbortSignal, onEvent: (event: Record<string, unknown>) => void) {
  const headers = new Headers({ Accept: "text/event-stream" });
  const token = getAccessToken(); const organizationId = getOrganizationId();
  if (token) headers.set("Authorization", `Bearer ${token}`); if (organizationId) headers.set("X-Organization-ID", String(organizationId));
  const response = await fetch(`${runtimeConfig.apiBaseUrl}/agent/runs/${id}/events/stream`, { headers, credentials: "include", signal });
  if (!response.ok || !response.body) throw new Error(`Agent stream failed (${response.status})`);
  const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = "";
  while (true) { const { done, value } = await reader.read(); if (done) break; buffer += decoder.decode(value, { stream: true }); const blocks = buffer.split("\n\n"); buffer = blocks.pop() ?? ""; for (const block of blocks) { const data = block.split("\n").filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n"); if (data) { try { onEvent(JSON.parse(data) as Record<string, unknown>); } catch { /* ignore malformed stream events */ } } } }
}
