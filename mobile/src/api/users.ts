// TODO(#22): 以下端点未纳入 openapi.yaml（/invitations/*、/users/contacts/*、/users/fcm-token、
// /users/presence、/users/search），故这几个域仍保留手写实现。
// 已迁移：User（基础字段取自生成契约，客户端额外字段在此补齐）。
// 注意：/users/change-password 在 spec 中未声明 requestBody，请求类型暂无法迁移。
// 端点覆盖清单见 docs/api/mobile-endpoint-coverage.md。
import type { components } from "@allcallall/api-types";

import { createApiClient } from "./client";

type APISchemas = components["schemas"];

// #22：基础字段（id / email / display_name）取自生成契约；
// status / deleted_at / profile 是客户端额外使用的字段，spec 未包含，故在此补上。
export type User = APISchemas["User"] & {
  status?: string;
  deleted_at?: string | null;
  profile?: ContactProfile;
};

export interface ContactProfile {
  company?: string;
  role?: string;
  timezone?: string;
  default_source_lang?: string;
  default_target_lang?: string;
  relationship_status?: string;
  preferred_contact_start?: string;
  preferred_contact_end?: string;
  preferred_contact_days?: string;
  last_followup_state?: string;
  note?: string;
}

export interface Invitation {
  code: string;
  inviter_id: number;
  inviter_email: string;
  inviter_display_name: string;
  target_email: string;
  default_source_lang: string;
  default_target_lang: string;
  note: string;
  status: string;
  accepted_user_id?: number | null;
  accepted_at?: string | null;
  expires_at: string;
  created_at: string;
  share_url: string;
  app_url: string;
}

export interface PresenceRecord {
  email: string;
  online: boolean;
  last_seen: string | null;
}

export const fetchMe = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ user: User }>("/users/me");
  return response.data.user;
};

export const searchUsers = async (token: string, query: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ results: User[] }>("/users/search", {
    params: { q: query }
  });
  return response.data.results;
};

export const listContacts = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ contacts: User[] }>("/users/contacts");
  return response.data.contacts;
};

export const addContact = async (token: string, email: string) => {
  const api = createApiClient(token);
  await api.post("/users/contacts", { email });
};

export const removeContact = async (token: string, contactId: number) => {
  const api = createApiClient(token);
  await api.delete(`/users/contacts/${contactId}`);
};

export interface CreateInvitationPayload {
  target_email: string;
  default_source_lang?: string;
  default_target_lang?: string;
  note?: string;
  expires_at?: string;
}

export const createInvitation = async (token: string, payload: CreateInvitationPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ invitation: Invitation }>("/invitations", payload);
  return response.data.invitation;
};

export const fetchInvitation = async (code: string) => {
  const api = createApiClient();
  const response = await api.get<{ invitation: Invitation }>(`/invitations/${code}`);
  return response.data.invitation;
};

export const acceptInvitation = async (token: string, code: string) => {
  const api = createApiClient(token);
  const response = await api.post<{ invitation: Invitation }>(`/invitations/${code}/accept`);
  return response.data.invitation;
};

export const fetchContactProfile = async (token: string, contactId: number) => {
  const api = createApiClient(token);
  const response = await api.get<{ profile: ContactProfile }>(`/users/contacts/${contactId}/profile`);
  return response.data.profile;
};

export const saveContactProfile = async (token: string, contactId: number, profile: ContactProfile) => {
  const api = createApiClient(token);
  const response = await api.put<{ profile: ContactProfile }>(`/users/contacts/${contactId}/profile`, profile);
  return response.data.profile;
};

export const fetchPresence = async (token: string, emails: string[]) => {
  const api = createApiClient(token);
  const response = await api.get<{ presence: PresenceRecord[] }>(
    "/users/presence",
    {
      params: {
        emails: emails.join(",")
      }
    }
  );
  return response.data.presence;
};

export interface ChangePasswordRequest {
  old_password: string;
  new_password: string;
  confirm_password: string;
}

export interface ChangePasswordResponse {
  message: string;
}

export const changePassword = async (token: string, data: ChangePasswordRequest) => {
  const api = createApiClient(token);
  const response = await api.post<ChangePasswordResponse>("/users/change-password", data);
  return response.data;
};

export interface SavePushTokenPayload {
  provider?: string;
  platform?: string;
  device_name?: string;
  app_version?: string;
}

export const saveFCMToken = async (token: string, fcmToken: string, metadata?: SavePushTokenPayload) => {
  const api = createApiClient(token);
  await api.post("/users/fcm-token", { fcm_token: fcmToken, ...metadata });
};
