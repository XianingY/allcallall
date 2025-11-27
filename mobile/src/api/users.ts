import { createApiClient } from "./client";

export interface User {
  id: number;
  email: string;
  display_name: string;
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
