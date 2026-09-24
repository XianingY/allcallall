// TODO(#22): /calls/* 与 /follow-ups/* 在 openapi.yaml 中缺失，故这两个域保留手写实现；
// 已覆盖的 legal / entitlements / usage / users.blocks / password-reset 可单独迁移到共享契约。
// 端点覆盖清单见 docs/api/mobile-endpoint-coverage.md。
import type { components } from "@allcallall/api-types";

import { createApiClient } from "./client";

type APISchemas = components["schemas"];

// #22：LegalInfo 与 spec 完全对齐，直接改用生成契约（保留旧导出名）。
export type LegalInfo = APISchemas["LegalInfo"];

export interface CallHistoryRecord {
  id: number;
  call_id: string;
  caller_id: number;
  callee_id: number;
  caller_email: string;
  callee_email: string;
  caller_display_name: string;
  callee_display_name: string;
  status: string;
  end_reason: string;
  started_at: string;
  answered_at?: string | null;
  ended_at?: string | null;
  followup_status?: string;
  next_task_due_at?: string | null;
  is_overdue?: boolean;
}

export interface CallFollowupRecord {
  id: number;
  call_id: string;
  user_id: number;
  peer_user_id: number;
  status: string;
  source: string;
  summary_cn?: string;
  summary_en?: string;
  key_points: string[];
  action_items: string[];
  next_step?: string;
  risk_flags: string[];
  followup_draft_cn?: string;
  followup_draft_en?: string;
  generated_at?: string | null;
  transcript_count: number;
}

export interface FollowUpTaskRecord {
  id: number;
  user_id: number;
  peer_user_id: number;
  call_id?: string;
  type: string;
  status: string;
  title: string;
  description?: string;
  due_at?: string | null;
  completed_at?: string | null;
  reminder_mode?: string;
  created_at: string;
  updated_at: string;
}

export interface FollowUpListItem {
  task: FollowUpTaskRecord;
  call?: CallHistoryRecord;
  followup?: CallFollowupRecord;
  peer?: {
    id: number;
    email: string;
    display_name: string;
    status?: string;
  };
  contact?: {
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
  };
  is_overdue: boolean;
}

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

export interface CreateFollowUpTaskPayload {
  peer_user_id: number;
  call_id?: string;
  type: string;
  title: string;
  description?: string;
  due_at?: string | null;
  reminder_mode?: string;
}

export const createFollowUpTask = async (token: string, payload: CreateFollowUpTaskPayload) => {
  const api = createApiClient(token);
  const response = await api.post<{ task: FollowUpTaskRecord }>("/follow-ups", payload);
  return response.data.task;
};

export interface UpdateFollowUpTaskPayload {
  status?: string;
  description?: string;
  due_at?: string | null;
  reminder_mode?: string;
}

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
