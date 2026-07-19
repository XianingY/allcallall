import type { components } from "@allcallall/api-types";

import { createApiClient } from "./client";

type APISchemas = components["schemas"];

export type MCPInstallation = APISchemas["MCPInstallation"];
export type MCPInstallationDefinition = APISchemas["MCPInstallationDefinition"];
export type CreateMCPInstallationRequest =
  APISchemas["CreateMCPInstallationRequest"];
export type UpdateMCPInstallationRequest =
  APISchemas["UpdateMCPInstallationRequest"];
export type MCPTool = APISchemas["MCPTool"];
export type MCPExecution = APISchemas["MCPExecution"];
export type AgentSkill = APISchemas["AgentSkill"];
export type CreateAgentSkillRequest = APISchemas["CreateAgentSkillRequest"];
export type UpdateAgentSkillRequest = APISchemas["UpdateAgentSkillRequest"];

export const listMCPInstallations = async (
  token: string,
): Promise<MCPInstallation[]> => {
  const api = createApiClient(token);
  const response = await api.get<{ installations: MCPInstallation[] }>(
    "/agent/mcp/installations",
  );
  return response.data.installations ?? [];
};

export const createMCPInstallation = async (
  token: string,
  request: CreateMCPInstallationRequest,
): Promise<MCPInstallation> => {
  const api = createApiClient(token);
  const response = await api.post<{ installation: MCPInstallation }>(
    "/agent/mcp/installations",
    request,
  );
  return response.data.installation;
};

export const getMCPInstallation = async (
  token: string,
  installationId: number,
): Promise<MCPInstallation> => {
  const api = createApiClient(token);
  const response = await api.get<{ installation: MCPInstallation }>(
    `/agent/mcp/installations/${installationId}`,
  );
  return response.data.installation;
};

export const updateMCPInstallation = async (
  token: string,
  installationId: number,
  request: UpdateMCPInstallationRequest,
): Promise<MCPInstallation> => {
  const api = createApiClient(token);
  const response = await api.patch<{ installation: MCPInstallation }>(
    `/agent/mcp/installations/${installationId}`,
    request,
  );
  return response.data.installation;
};

export const disableMCPInstallation = async (
  token: string,
  installationId: number,
): Promise<void> => {
  const api = createApiClient(token);
  await api.delete(`/agent/mcp/installations/${installationId}`);
};

const installationAction = async (
  token: string,
  installationId: number,
  action: "validate" | "activate" | "publish",
): Promise<MCPInstallation> => {
  const api = createApiClient(token);
  const response = await api.post<{ installation: MCPInstallation }>(
    `/agent/mcp/installations/${installationId}/${action}`,
  );
  return response.data.installation;
};

export const validateMCPInstallation = (
  token: string,
  installationId: number,
) => installationAction(token, installationId, "validate");

export const activateMCPInstallation = (
  token: string,
  installationId: number,
) => installationAction(token, installationId, "activate");

export const publishMCPInstallation = (
  token: string,
  installationId: number,
) => installationAction(token, installationId, "publish");

export const putMCPInstallationSecrets = async (
  token: string,
  installationId: number,
  secrets: Record<string, string>,
): Promise<boolean> => {
  const api = createApiClient(token);
  const response = await api.post<{ secrets_configured: boolean }>(
    `/agent/mcp/installations/${installationId}/secrets`,
    { secrets },
  );
  return response.data.secrets_configured;
};

export const listMCPInstallationTools = async (
  token: string,
  installationId: number,
): Promise<MCPTool[]> => {
  const api = createApiClient(token);
  const response = await api.get<{ tools: MCPTool[] }>(
    `/agent/mcp/installations/${installationId}/tools`,
  );
  return response.data.tools ?? [];
};

export const getMCPExecution = async (
  token: string,
  executionId: string,
): Promise<MCPExecution> => {
  const api = createApiClient(token);
  const response = await api.get<{ execution: MCPExecution }>(
    `/agent/mcp/executions/${encodeURIComponent(executionId)}`,
  );
  return response.data.execution;
};

export const listAgentSkills = async (token: string): Promise<AgentSkill[]> => {
  const api = createApiClient(token);
  const response = await api.get<{ skills: AgentSkill[] }>("/agent/skills");
  return response.data.skills ?? [];
};

export const createAgentSkill = async (
  token: string,
  request: CreateAgentSkillRequest,
): Promise<AgentSkill> => {
  const api = createApiClient(token);
  const response = await api.post<{ skill: AgentSkill }>(
    "/agent/skills",
    request,
  );
  return response.data.skill;
};

export const updateAgentSkill = async (
  token: string,
  skillId: number,
  request: UpdateAgentSkillRequest,
): Promise<AgentSkill> => {
  const api = createApiClient(token);
  const response = await api.patch<{ skill: AgentSkill }>(
    `/agent/skills/${skillId}`,
    request,
  );
  return response.data.skill;
};

export const deleteAgentSkill = async (
  token: string,
  skillId: number,
): Promise<void> => {
  const api = createApiClient(token);
  await api.delete(`/agent/skills/${skillId}`);
};
