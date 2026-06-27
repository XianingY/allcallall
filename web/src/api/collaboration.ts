import type { components } from "@/api/schema";
import { apiRequest } from "@/api/http";

export type Conversation = components["schemas"]["Conversation"];
export type ConversationDetail = components["schemas"]["ConversationDetail"];
export type Message = components["schemas"]["Message"];
export type ConversationNote = components["schemas"]["ConversationNote"];
export type Contact = components["schemas"]["Contact"];
export type ContactProfile = components["schemas"]["ContactProfile"];
export type FollowUpItem = components["schemas"]["FollowUpItem"];
export type FollowUpTask = components["schemas"]["FollowUpTask"];
export type Pipeline = components["schemas"]["Pipeline"];
export type PipelineStage = components["schemas"]["PipelineStage"];
export type Deal = components["schemas"]["Deal"];
export type DealActivity = components["schemas"]["DealActivity"];
export type CallHistory = components["schemas"]["CallHistory"];

const query = (values: Record<string, string | number | undefined>) => {
  const params = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => { if (value !== undefined && value !== "") params.set(key, String(value)); });
  const encoded = params.toString();
  return encoded ? `?${encoded}` : "";
};

export const listConversations = (filter = "") => apiRequest<{ conversations: Conversation[] }>(`/conversations${query({ filter })}`).then((value) => value.conversations);
export const getConversation = (id: number) => apiRequest<{ conversation: ConversationDetail }>(`/conversations/${id}`).then((value) => value.conversation);
export const createConversation = (input: { type: string; title?: string; topic?: string; member_ids?: number[] }) => apiRequest<{ conversation: Conversation }>("/conversations", { method: "POST", body: JSON.stringify(input) }).then((value) => value.conversation);
export const updateConversation = (id: number, input: { status?: string; priority?: string; assignee_user_id?: number | null; contact_id?: number | null }) => apiRequest<{ conversation: Conversation }>(`/conversations/${id}`, { method: "PATCH", body: JSON.stringify(input) }).then((value) => value.conversation);
export const listMessages = (id: number) => apiRequest<{ messages: Message[] }>(`/conversations/${id}/messages`).then((value) => value.messages);
export const sendMessage = (id: number, body: string, type = "text") => apiRequest<{ message: Message }>(`/conversations/${id}/messages`, { method: "POST", body: JSON.stringify({ body, type }) }).then((value) => value.message);
export const markConversationRead = (id: number) => apiRequest<void>(`/conversations/${id}/read`, { method: "POST" });
export const listNotes = (id: number) => apiRequest<{ notes: ConversationNote[] }>(`/conversations/${id}/notes`).then((value) => value.notes);
export const createNote = (id: number, body: string) => apiRequest<{ note: ConversationNote }>(`/conversations/${id}/notes`, { method: "POST", body: JSON.stringify({ body }) }).then((value) => value.note);

export const listContacts = () => apiRequest<{ contacts: Contact[] }>("/users/contacts").then((value) => value.contacts);
export const addContact = (email: string) => apiRequest<void>("/users/contacts", { method: "POST", body: JSON.stringify({ email }) });
export const removeContact = (id: number) => apiRequest<void>(`/users/contacts/${id}`, { method: "DELETE" });
export const getContactProfile = (id: number) => apiRequest<{ profile: ContactProfile }>(`/users/contacts/${id}/profile`).then((value) => value.profile);
export const saveContactProfile = (id: number, profile: ContactProfile) => apiRequest<{ profile: ContactProfile }>(`/users/contacts/${id}/profile`, { method: "PUT", body: JSON.stringify(profile) }).then((value) => value.profile);
export const createInvitation = (input: { target_email: string; note?: string; expires_at?: string }) => apiRequest<{ invitation: { code: string; share_url: string } }>("/invitations", { method: "POST", body: JSON.stringify(input) }).then((value) => value.invitation);

export const listFollowUps = () => apiRequest<{ items: FollowUpItem[] }>("/follow-ups").then((value) => value.items);
export const createFollowUp = (input: { peer_user_id: number; type: string; title: string; description?: string; due_at?: string | null; reminder_mode?: string }) => apiRequest<{ task: FollowUpTask }>("/follow-ups", { method: "POST", body: JSON.stringify(input) }).then((value) => value.task);
export const updateFollowUp = (id: number, input: { status?: string; description?: string; due_at?: string | null; reminder_mode?: string }) => apiRequest<{ task: FollowUpTask }>(`/follow-ups/${id}`, { method: "PATCH", body: JSON.stringify(input) }).then((value) => value.task);
export const listCallHistory = (days = 30) => apiRequest<{ calls: CallHistory[] }>(`/calls/history${query({ days })}`).then((value) => value.calls);

export const listPipelines = () => apiRequest<{ pipelines: Pipeline[] }>("/pipelines").then((value) => value.pipelines);
export const listDeals = () => apiRequest<{ deals: Deal[] }>("/deals").then((value) => value.deals);
export const getDeal = (id: number) => apiRequest<{ deal: Deal }>(`/deals/${id}`).then((value) => value.deal);
export const createDeal = (input: { title: string; description?: string; value_cents?: number; currency?: string; stage_id?: number }) => apiRequest<{ deal: Deal }>("/deals", { method: "POST", body: JSON.stringify(input) }).then((value) => value.deal);
export const updateDeal = (id: number, input: Partial<{ title: string; description: string; value_cents: number; currency: string; stage_id: number; status: string }>) => apiRequest<{ deal: Deal }>(`/deals/${id}`, { method: "PATCH", body: JSON.stringify(input) }).then((value) => value.deal);
export const addDealContact = (id: number, contactId: number) => apiRequest<void>(`/deals/${id}/contacts`, { method: "POST", body: JSON.stringify({ contact_id: contactId }) });
export const listDealActivities = (id: number) => apiRequest<{ activities: DealActivity[] }>(`/deals/${id}/activities`).then((value) => value.activities);
