import type { components } from "@/api/schema";
import { apiRequest } from "@/api/http";

export type KnowledgeSource = components["schemas"]["KnowledgeSource"];
export type KnowledgeSourceDetail = components["schemas"]["KnowledgeSourceDetail"];
export type KnowledgeSourceVersion = components["schemas"]["KnowledgeSourceVersion"];
export type RAGChunk = components["schemas"]["RAGChunk"];
export type SourceGroup = components["schemas"]["SourceGroup"];
export type DuplicateCandidate = components["schemas"]["DuplicateCandidate"];
export type DeadLetter = components["schemas"]["DeadLetter"];

export const listKnowledgeSources = () => apiRequest<{ sources: KnowledgeSource[] }>("/knowledge/sources").then((value) => value.sources ?? []);
export const getKnowledgeSource = (id: number) => apiRequest<KnowledgeSourceDetail>(`/knowledge/sources/${id}`);
export const createTextSource = (title: string, text: string, conversationId?: number) => apiRequest<{ source: KnowledgeSource }>("/knowledge/sources", { method: "POST", body: JSON.stringify({ kind: "manual_text", title, text, conversation_id: conversationId }) }).then((value) => value.source);
export const createURLSource = (title: string, url: string, conversationId?: number) => apiRequest<{ source: KnowledgeSource }>("/knowledge/sources", { method: "POST", body: JSON.stringify({ kind: "url", title, url, conversation_id: conversationId }) }).then((value) => value.source);
export const createFileSource = (title: string, file: File, conversationId?: number) => { const body = new FormData(); body.append("kind", "file"); body.append("title", title); body.append("file", file); if (conversationId) body.append("conversation_id", String(conversationId)); return apiRequest<{ source: KnowledgeSource }>("/knowledge/sources", { method: "POST", body }).then((value) => value.source); };
export const reingestSource = (id: number) => apiRequest<void>(`/knowledge/sources/${id}/reingest`, { method: "POST", body: "{}" });
export const listSourceGroups = () => apiRequest<{ source_groups: SourceGroup[] }>("/knowledge/source-groups").then((value) => value.source_groups ?? []);
export const setCanonicalSource = (groupId: number, sourceId: number) => apiRequest<void>(`/knowledge/source-groups/${groupId}/canonical`, { method: "POST", body: JSON.stringify({ source_id: sourceId }) });
export const listDuplicates = () => apiRequest<{ duplicate_candidates: DuplicateCandidate[] }>("/knowledge/duplicate-candidates").then((value) => value.duplicate_candidates ?? []);
export const decideDuplicate = (id: number, decision: "confirm" | "reject") => apiRequest<void>(`/knowledge/duplicate-candidates/${id}/decision`, { method: "POST", body: JSON.stringify({ decision }) });
export const listDeadLetters = () => apiRequest<{ dead_letters: DeadLetter[] }>("/knowledge/dead-letters").then((value) => value.dead_letters ?? []);
export const retryDeadLetter = (id: number) => apiRequest<void>(`/knowledge/dead-letters/${id}/retry`, { method: "POST", body: "{}" });
