// #22：/calls/* 与 /follow-ups/* 已纳入 openapi.yaml，相关类型改为引用
// @allcallall/api-types 生成的共享契约。
import type { components } from "@allcallall/api-types";

import { createApiClient } from "./client";

type APISchemas = components["schemas"];

// #22：LegalInfo 与 spec 完全对齐，直接改用生成契约（保留旧导出名）。
export type LegalInfo = APISchemas["LegalInfo"];

export type CallHistoryRecord = APISchemas["CallHistory"];

export type CallFollowupRecord = APISchemas["CallFollowup"];

export type FollowUpTaskRecord = APISchemas["FollowUpTask"];

// #22：spec 的 contact 允许 null，而客户端此前按"可选但非 null"消费，
// 这里收敛成原来的形状，保证下游屏幕无需改动。
export type FollowUpListItem = Omit<APISchemas["FollowUpItem"], "contact"> & {
  contact?: APISchemas["ContactProfile"];
};

// #22：字段取自生成契约；blocker_id 是客户端额外使用的字段，spec 未包含，故在此补上。
export type UserBlockRecord = APISchemas["UserBlock"] & {
  blocker_id: number;
};

export interface AbuseReportPayload {
  reported_user_id: number;
  category: string;
  details?: string;
}

// #22：字段取自生成契约；spec 的 product_id 不声明 null，这里保留客户端原有的可空性。
export type EntitlementRecord = Omit<
  APISchemas["UserEntitlement"],
  "product_id"
> & {
  product_id?: string | null;
};

// #22：UsageSnapshot 与手写结构完全一致。
export type UsageRecord = APISchemas["UsageSnapshot"];

export interface RevenueCatConfig {
  apiKey: string;
  offeringId: string;
  monthlyProductId: string;
  yearlyProductId: string;
  androidPackageName: string;
}

export const fetchCurrentLegal = async () => {
  const api = createApiClient();
  const response = await api.get<{ legal: LegalInfo }>("/legal/current");
  return response.data.legal;
};

export const acceptLegal = async (token: string) => {
  const api = createApiClient(token);
  await api.post("/legal/accept");
};

export const sendPasswordResetCode = async (email: string) => {
  const api = createApiClient();
  await api.post("/auth/password-reset/send", { email });
};

export const confirmPasswordReset = async (
  email: string,
  code: string,
  newPassword: string,
  confirmPassword: string
) => {
  const api = createApiClient();
  await api.post("/auth/password-reset/confirm", {
    email,
    code,
    new_password: newPassword,
    confirm_password: confirmPassword
  });
};

export const fetchCallHistory = async (token: string, days = 30) => {
  const api = createApiClient(token);
  const response = await api.get<{ calls: CallHistoryRecord[] }>("/calls/history", {
    params: { days }
  });
  return response.data.calls;
};

export const fetchCallFollowup = async (token: string, callId: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ followup: CallFollowupRecord; tasks: FollowUpTaskRecord[] }>(`/calls/${callId}/followup`);
  return response.data;
};

export const generateCallFollowup = async (token: string, callId: string, force = false) => {
  const api = createApiClient(token);
  const endpoint = force ? `/calls/${callId}/followup/regenerate` : `/calls/${callId}/followup/generate`;
  const response = await api.post<{ followup: CallFollowupRecord; tasks: FollowUpTaskRecord[] }>(endpoint);
  return response.data;
};

export const fetchFollowUps = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ items: FollowUpListItem[] }>("/follow-ups");
  return response.data.items;
};

export type CreateFollowUpTaskPayload = APISchemas["CreateFollowUpTaskRequest"];

export const createFollowUpTask = async (token: string, payload: CreateFollowUpTaskPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ task: FollowUpTaskRecord }>("/follow-ups", payload);
  return response.data.task;
};

export type UpdateFollowUpTaskPayload = APISchemas["UpdateFollowUpTaskRequest"];

export const updateFollowUpTask = async (token: string, taskId: number, payload: UpdateFollowUpTaskPayload) => {
  const api = createApiClient(token);
  const response = await api.patch<{ task: FollowUpTaskRecord }>(`/follow-ups/${taskId}`, payload);
  return response.data.task;
};

export const createBlock = async (token: string, blockedUserId: number) => {
  const api = createApiClient(token);
  await api.post("/users/blocks", { blocked_user_id: blockedUserId });
};

export const listBlocks = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ blocks: UserBlockRecord[] }>("/users/blocks");
  return response.data.blocks;
};

export const removeBlock = async (token: string, blockedUserId: number) => {
  const api = createApiClient(token);
  await api.delete(`/users/blocks/${blockedUserId}`);
};

export const createAbuseReport = async (token: string, payload: AbuseReportPayload) => {
  const api = createApiClient(token);
  await api.post("/users/reports", payload);
};

export const fetchEntitlements = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ tier: string; entitlements: EntitlementRecord[] }>("/entitlements/me");
  return response.data;
};

export const fetchUsage = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ usage: UsageRecord[] }>("/usage/me");
  return response.data.usage;
};

export const deleteAccount = async (token: string, input: { password?: string; code?: string }) => {
  const api = createApiClient(token);
  const response = await api.post<{ message: string }>("/users/me/deletion", input);
  return response.data;
};

export const getRevenueCatConfig = (): RevenueCatConfig | null => {
  const apiKey = process.env.EXPO_PUBLIC_REVENUECAT_API_KEY?.trim();
  const offeringId = process.env.EXPO_PUBLIC_REVENUECAT_OFFERING_ID?.trim() ?? "default";
  const monthlyProductId = "premium_monthly";
  const yearlyProductId = "premium_yearly";
  const androidPackageName = "com.allcallall.mobile";

  if (!apiKey) {
    return null;
  }

  return {
    apiKey,
    offeringId,
    monthlyProductId,
    yearlyProductId,
    androidPackageName
  };
};
