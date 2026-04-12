import { createApiClient } from "./client";

export interface LegalInfo {
  terms_version: string;
  privacy_version: string;
  terms_url: string;
  privacy_policy_url: string;
  support_email: string;
  account_deletion_url: string;
}

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
}

export interface UserBlockRecord {
  id: number;
  blocker_id: number;
  blocked_user_id: number;
  blocked_user_email?: string;
  blocked_user_display_name?: string;
  blocked_user_status?: string;
  blocked_user_deleted_at?: string | null;
  created_at: string;
}

export interface AbuseReportPayload {
  reported_user_id: number;
  category: string;
  details?: string;
}

export interface EntitlementRecord {
  id: number;
  entitlement: string;
  tier: string;
  product_id?: string | null;
  status: string;
  expires_at?: string | null;
  source: string;
}

export interface UsageRecord {
  feature: string;
  period_key: string;
  unit: string;
  used_units: number;
  limit_units: number;
  unlimited: boolean;
  remaining_units: number;
}

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
