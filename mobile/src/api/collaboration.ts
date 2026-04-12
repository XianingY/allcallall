import { createApiClient } from "./client";

export interface OrganizationRecord {
  id: number;
  name: string;
  slug: string;
  description?: string;
  role: string;
}

export interface OrganizationPolicyRecord {
  id: number;
  organization_id: number;
  recording_mode: string;
  recording_storage_days: number;
  recording_export_allowed: boolean;
}

export interface ConversationRecord {
  id: number;
  organization_id: number;
  team_id?: number | null;
  room_id?: number | null;
  type: string;
  title: string;
  topic?: string;
  last_message_at?: string | null;
  last_message_preview?: string;
  last_message_type?: string;
  unread_count: number;
}

export interface MessageRecord {
  id: number;
  organization_id: number;
  conversation_id: number;
  sender_id: number;
  sender_email: string;
  sender_display_name: string;
  type: string;
  body: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface PipelineStageRecord {
  id: number;
  pipeline_id: number;
  name: string;
  position: number;
  is_closed: boolean;
}

export interface PipelineRecord {
  id: number;
  organization_id: number;
  name: string;
  is_default: boolean;
  stages: PipelineStageRecord[];
}

export interface DealRecord {
  id: number;
  organization_id: number;
  pipeline_id: number;
  stage_id?: number | null;
  stage_name?: string;
  owner_id: number;
  title: string;
  description?: string;
  status: string;
  value_cents: number;
  currency: string;
  created_at: string;
  updated_at: string;
}

export interface DealActivityRecord {
  id: number;
  organization_id: number;
  deal_id: number;
  type: string;
  reference_type: string;
  reference_id: string;
  summary: string;
  metadata_json?: string;
  created_by: number;
  created_at: string;
}

export const listOrganizations = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ organizations: OrganizationRecord[] }>("/organizations");
  return response.data.organizations;
};

export const createOrganization = async (token: string, name: string) => {
  const api = createApiClient(token);
  const response = await api.post<{ organization: OrganizationRecord }>("/organizations", { name });
  return response.data.organization;
};

export const switchOrganization = async (token: string, organizationId: number) => {
  const api = createApiClient(token);
  const response = await api.post<{ organization: OrganizationRecord }>(`/organizations/${organizationId}/switch`);
  return response.data.organization;
};

export const fetchOrganizationPolicy = async (token: string, organizationId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ policy: OrganizationPolicyRecord }>(`/organizations/${organizationId}/policy`);
  return response.data.policy;
};

export const updateOrganizationPolicy = async (
  token: string,
  organizationId: number,
  payload: Pick<OrganizationPolicyRecord, "recording_mode" | "recording_storage_days" | "recording_export_allowed">
) => {
  const api = createApiClient(token);
  const response = await api.put<{ policy: OrganizationPolicyRecord }>(`/organizations/${organizationId}/policy`, payload);
  return response.data.policy;
};

export const listConversations = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ conversations: ConversationRecord[] }>("/conversations");
  return response.data.conversations;
};

export interface CreateConversationPayload {
  type: string;
  title?: string;
  topic?: string;
  member_ids?: number[];
  team_id?: number;
}

export const createConversation = async (token: string, payload: CreateConversationPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ conversation: ConversationRecord }>("/conversations", payload);
  return response.data.conversation;
};

export const listMessages = async (token: string, conversationId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ messages: MessageRecord[] }>(`/conversations/${conversationId}/messages`);
  return response.data.messages;
};

export interface CreateMessagePayload {
  type?: string;
  body: string;
  metadata?: Record<string, unknown>;
}

export const createMessage = async (token: string, conversationId: number, payload: CreateMessagePayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ message: MessageRecord }>(`/conversations/${conversationId}/messages`, payload);
  return response.data.message;
};

export const markConversationRead = async (token: string, conversationId: number) => {
  const api = createApiClient(token);
  await api.post(`/conversations/${conversationId}/read`);
};

export const listPipelines = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ pipelines: PipelineRecord[] }>("/pipelines");
  return response.data.pipelines;
};

export const listDeals = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ deals: DealRecord[] }>("/deals");
  return response.data.deals;
};

export interface CreateDealPayload {
  title: string;
  description?: string;
  value_cents?: number;
  currency?: string;
  stage_id?: number;
}

export const createDeal = async (token: string, payload: CreateDealPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ deal: DealRecord }>("/deals", payload);
  return response.data.deal;
};

export const fetchDeal = async (token: string, dealId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ deal: DealRecord }>(`/deals/${dealId}`);
  return response.data.deal;
};

export const updateDeal = async (
  token: string,
  dealId: number,
  payload: Partial<CreateDealPayload> & { status?: string }
) => {
  const api = createApiClient(token);
  const response = await api.patch<{ deal: DealRecord }>(`/deals/${dealId}`, payload);
  return response.data.deal;
};

export const addDealContact = async (token: string, dealId: number, contactId: number) => {
  const api = createApiClient(token);
  await api.post(`/deals/${dealId}/contacts`, { contact_id: contactId });
};

export const listDealActivities = async (token: string, dealId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ activities: DealActivityRecord[] }>(`/deals/${dealId}/activities`);
  return response.data.activities;
};
