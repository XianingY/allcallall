import type { components } from "@allcallall/api-types";

import { apiRequest } from "@/api/http";

export type MCPInstallation = components["schemas"]["MCPInstallation"];
export type MCPInstallationDefinition = components["schemas"]["MCPInstallationDefinition"];
export type CreateMCPInstallationInput = components["schemas"]["CreateMCPInstallationRequest"];
export type UpdateMCPInstallationInput = components["schemas"]["UpdateMCPInstallationRequest"];
export type MCPInstallationRevision = components["schemas"]["MCPInstallationRevision"];
export type MCPTool = components["schemas"]["MCPTool"];
export type MCPExecution = components["schemas"]["MCPExecution"];
export type AgentSkill = components["schemas"]["AgentSkill"];
export type CreateAgentSkillInput = components["schemas"]["CreateAgentSkillRequest"];
export type UpdateAgentSkillInput = components["schemas"]["UpdateAgentSkillRequest"];

export const listMCPInstallations = () =>
  apiRequest<{ installations: MCPInstallation[] }>("/agent/mcp/installations")
    .then((response) => response.installations ?? []);

export const createMCPInstallation = (input: CreateMCPInstallationInput) =>
  apiRequest<{ installation: MCPInstallation }>("/agent/mcp/installations", {
    method: "POST",
    body: JSON.stringify(input),
  }).then((response) => response.installation);

export const getMCPInstallation = (installationId: number) =>
  apiRequest<{ installation: MCPInstallation }>(`/agent/mcp/installations/${installationId}`)
    .then((response) => response.installation);

export const updateMCPInstallation = (installationId: number, input: UpdateMCPInstallationInput) =>
  apiRequest<{ installation: MCPInstallation }>(`/agent/mcp/installations/${installationId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }).then((response) => response.installation);

export const disableMCPInstallation = (installationId: number) =>
  apiRequest<void>(`/agent/mcp/installations/${installationId}`, { method: "DELETE" });

const installationAction = (installationId: number, action: "validate" | "activate" | "publish") =>
  apiRequest<{ installation: MCPInstallation }>(`/agent/mcp/installations/${installationId}/${action}`, {
    method: "POST",
  }).then((response) => response.installation);

export const validateMCPInstallation = (installationId: number) => installationAction(installationId, "validate");
export const activateMCPInstallation = (installationId: number) => installationAction(installationId, "activate");
export const publishMCPInstallation = (installationId: number) => installationAction(installationId, "publish");

export const putMCPInstallationSecrets = (installationId: number, secrets: Record<string, string>) =>
  apiRequest<{ secrets_configured: boolean }>(`/agent/mcp/installations/${installationId}/secrets`, {
    method: "POST",
    body: JSON.stringify({ secrets }),
  });

export const listMCPInstallationTools = (installationId: number) =>
  apiRequest<{ tools: MCPTool[] }>(`/agent/mcp/installations/${installationId}/tools`)
    .then((response) => response.tools ?? []);

export const getMCPExecution = (executionId: string) =>
  apiRequest<{ execution: MCPExecution }>(`/agent/mcp/executions/${encodeURIComponent(executionId)}`)
    .then((response) => response.execution);

export const listAgentSkills = () =>
  apiRequest<{ skills: AgentSkill[] }>("/agent/skills")
    .then((response) => response.skills ?? []);

export const createAgentSkill = (input: CreateAgentSkillInput) =>
  apiRequest<{ skill: AgentSkill }>("/agent/skills", {
    method: "POST",
    body: JSON.stringify(input),
  }).then((response) => response.skill);

export const updateAgentSkill = (skillId: number, input: UpdateAgentSkillInput) =>
  apiRequest<{ skill: AgentSkill }>(`/agent/skills/${skillId}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }).then((response) => response.skill);

export const deleteAgentSkill = (skillId: number) =>
  apiRequest<void>(`/agent/skills/${skillId}`, { method: "DELETE" });
