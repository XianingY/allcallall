// #22：/knowledge 域（sources、source-groups、dead-letters、duplicate-candidates）已纳入
// openapi.yaml，记录类型改为引用 @allcallall/api-types 生成的共享契约。
import axios from "axios";

import type { components } from "@allcallall/api-types";
import { API_BASE_URL } from "../config";
import { createApiClient, getActiveOrganizationHeader } from "./client";

type APISchemas = components["schemas"];

export type KnowledgeSourceKind = "manual_text" | "url" | "file";

export type KnowledgeSourceRecord = APISchemas["KnowledgeSource"];
export type KnowledgeSourceVersionRecord = APISchemas["KnowledgeSourceVersion"];
export type RAGChunkRecord = APISchemas["RAGChunk"];
export type KnowledgeSourceDetail = APISchemas["KnowledgeSourceDetail"];
export type SourceGroupRecord = APISchemas["SourceGroup"];
export type SourceGroupDetail = APISchemas["SourceGroupDetail"];
export type DuplicateCandidateRecord = APISchemas["DuplicateCandidate"];
export type DeadLetterRecord = APISchemas["DeadLetter"];

export interface CreateManualKnowledgeSourceInput {
  title: string;
  text: string;
  conversation_id?: number | null;
}

export interface CreateURLKnowledgeSourceInput {
  title: string;
  url: string;
  conversation_id?: number | null;
}

interface SourcesResponse {
  sources: KnowledgeSourceRecord[];
}

interface SourceResponse {
  source: KnowledgeSourceRecord;
}

interface DeadLettersResponse {
  dead_letters: DeadLetterRecord[];
}

interface SourceGroupsResponse {
  source_groups: SourceGroupRecord[];
}

interface DuplicateCandidatesResponse {
  duplicate_candidates: DuplicateCandidateRecord[];
}

export interface KnowledgeSourceListParams {
  conversation_id?: number;
  status?: string;
}

const authHeaders = (token: string): Record<string, string> => {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
    Accept: "application/json",
  };
  const organizationId = getActiveOrganizationHeader();
  if (organizationId) {
    headers["X-Organization-ID"] = String(organizationId);
  }
  return headers;
};

export const listKnowledgeSources = async (
  token: string,
  params?: KnowledgeSourceListParams
): Promise<KnowledgeSourceRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<SourcesResponse>("/knowledge/sources", {
    params,
  });
  return response.data.sources ?? [];
};

export const fetchKnowledgeSource = async (
  token: string,
  sourceId: number
): Promise<KnowledgeSourceDetail> => {
  const client = createApiClient(token);
  const response = await client.get<KnowledgeSourceDetail>(`/knowledge/sources/${sourceId}`);
  return response.data;
};

export const createManualKnowledgeSource = async (
  token: string,
  input: CreateManualKnowledgeSourceInput
): Promise<KnowledgeSourceRecord> => {
  const client = createApiClient(token);
  const response = await client.post<SourceResponse>("/knowledge/sources", {
    kind: "manual_text",
    title: input.title,
    text: input.text,
    conversation_id: input.conversation_id ?? undefined,
  });
  return response.data.source;
};

export const createURLKnowledgeSource = async (
  token: string,
  input: CreateURLKnowledgeSourceInput
): Promise<KnowledgeSourceRecord> => {
  const client = createApiClient(token);
  const response = await client.post<SourceResponse>("/knowledge/sources", {
    kind: "url",
    title: input.title,
    url: input.url,
    conversation_id: input.conversation_id ?? undefined,
  });
  return response.data.source;
};

export const createFileKnowledgeSource = async (
  token: string,
  file: File,
  title: string,
  conversationId?: number | null
): Promise<KnowledgeSourceRecord> => {
  const body = new FormData();
  body.append("kind", "file");
  body.append("title", title);
  body.append("file", file);
  if (conversationId) {
    body.append("conversation_id", String(conversationId));
  }
  const response = await axios.post<SourceResponse>(`${API_BASE_URL}/knowledge/sources`, body, {
    headers: authHeaders(token),
  });
  return response.data.source;
};

export const reingestKnowledgeSource = async (token: string, sourceId: number): Promise<void> => {
  const client = createApiClient(token);
  await client.post(`/knowledge/sources/${sourceId}/reingest`, {});
};

export const listKnowledgeDeadLetters = async (token: string): Promise<DeadLetterRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<DeadLettersResponse>("/knowledge/dead-letters");
  return response.data.dead_letters ?? [];
};

export const retryKnowledgeDeadLetter = async (token: string, deadLetterId: number): Promise<void> => {
  const client = createApiClient(token);
  await client.post(`/knowledge/dead-letters/${deadLetterId}/retry`, {});
};

export const listKnowledgeSourceGroups = async (token: string): Promise<SourceGroupRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<SourceGroupsResponse>("/knowledge/source-groups");
  return response.data.source_groups ?? [];
};

export const fetchKnowledgeSourceGroup = async (
  token: string,
  groupId: number
): Promise<SourceGroupDetail> => {
  const client = createApiClient(token);
  const response = await client.get<SourceGroupDetail>(`/knowledge/source-groups/${groupId}`);
  return response.data;
};

export const setKnowledgeSourceGroupCanonical = async (
  token: string,
  groupId: number,
  sourceId: number
): Promise<void> => {
  const client = createApiClient(token);
  await client.post(`/knowledge/source-groups/${groupId}/canonical`, { source_id: sourceId });
};

export const listKnowledgeDuplicateCandidates = async (token: string): Promise<DuplicateCandidateRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<DuplicateCandidatesResponse>("/knowledge/duplicate-candidates");
  return response.data.duplicate_candidates ?? [];
};

export const decideKnowledgeDuplicateCandidate = async (
  token: string,
  duplicateId: number,
  decision: "confirm" | "reject"
): Promise<void> => {
  const client = createApiClient(token);
  await client.post(`/knowledge/duplicate-candidates/${duplicateId}/decision`, { decision });
};
