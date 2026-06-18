import axios from "axios";

import { API_BASE_URL } from "../config";
import { createApiClient, getActiveOrganizationHeader } from "./client";

export type KnowledgeSourceKind = "manual_text" | "url" | "file";

export interface KnowledgeSourceRecord {
  id: number;
  organization_id: number;
  conversation_id?: number | null;
  created_by: number;
  kind: KnowledgeSourceKind | string;
  title: string;
  uri?: string;
  file_name?: string;
  content_type?: string;
  status: string;
  active_version_id?: number | null;
  last_error?: string;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeSourceVersionRecord {
  id: number;
  source_id: number;
  version: number;
  content_hash: string;
  status: string;
  chunk_count: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
  activated_at?: string | null;
}

export interface RAGChunkRecord {
  id: number;
  source_id: number;
  source_version_id: number;
  conversation_id?: number | null;
  chunk_index: number;
  start_offset: number;
  end_offset: number;
  content_hash: string;
  snippet: string;
  index_status: string;
  last_error?: string;
  indexed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeSourceDetail {
  source: KnowledgeSourceRecord;
  versions: KnowledgeSourceVersionRecord[];
  chunks: RAGChunkRecord[];
}

export interface DeadLetterRecord {
  id: number;
  aggregate_type: string;
  aggregate_id: number;
  event: string;
  payload_json: string;
  idempotency_key: string;
  request_id?: string;
  status: string;
  attempts: number;
  last_error?: string;
  available_at?: string | null;
  updated_at: string;
}

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

export const listKnowledgeSources = async (token: string): Promise<KnowledgeSourceRecord[]> => {
  const client = createApiClient(token);
  const response = await client.get<SourcesResponse>("/knowledge/sources");
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
