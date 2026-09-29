// #22：/invitations/*、/users/contacts/*、/users/fcm-token、/users/presence、/users/search
// 已纳入 openapi.yaml，相关类型改为引用 @allcallall/api-types 生成的共享契约。
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

export type ContactProfile = APISchemas["ContactProfile"];

export type Invitation = APISchemas["Invitation"];

export type PresenceRecord = APISchemas["PresenceRecord"];

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

export type CreateInvitationPayload = APISchemas["CreateInvitationRequest"];

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

export interface OrganizationInvite {
  id: number;
  organization_id: number;
  code: string;
  target_email: string;
  role: string;
  status: string;
  expires_at: string;
  share_url?: string;
}

// Organization invites live in their own table with their own endpoints. They
// used to be looked up with the contact invitation endpoints, which read a
// different table, so every organization invite appeared invalid on mobile.
export const fetchOrganizationInvite = async (token: string, code: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ invite: OrganizationInvite }>(`/organizations/invites/${code}`);
  return response.data.invite;
};

export const acceptOrganizationInvite = async (token: string, code: string) => {
  const api = createApiClient(token);
  const response = await api.post<{ invite: OrganizationInvite }>(`/organizations/invites/${code}/accept`);
  return response.data.invite;
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

export type ChangePasswordRequest = APISchemas["ChangePasswordRequest"];

export type ChangePasswordResponse = { message: string };

export const changePassword = async (token: string, data: ChangePasswordRequest) => {
  const api = createApiClient(token);
  const response = await api.post<ChangePasswordResponse>("/users/change-password", data);
  return response.data;
};

export type SavePushTokenPayload = Omit<APISchemas["SavePushTokenRequest"], "fcm_token">;

export const saveFCMToken = async (token: string, fcmToken: string, metadata?: SavePushTokenPayload) => {
  const api = createApiClient(token);
  await api.post("/users/fcm-token", { fcm_token: fcmToken, ...metadata });
};
