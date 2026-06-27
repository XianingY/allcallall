import type { components } from "@/api/schema";
import { apiRequest, setAccessToken } from "@/api/http";

export type User = components["schemas"]["User"];
export type Organization = components["schemas"]["Organization"];
export type AuthResponse = components["schemas"]["AuthResponse"];
export type LegalInfo = components["schemas"]["LegalInfo"];
export type RefreshSession = components["schemas"]["RefreshSession"];
export type UserBlock = components["schemas"]["UserBlock"];

export interface RegisterInput { email: string; password: string; display_name: string; accept_current_legal: boolean }
export interface PasswordResetInput { email: string; code: string; new_password: string; confirm_password: string }

const storeAuth = (payload: AuthResponse) => { setAccessToken(payload.access_token); return payload; };

export const login = async (email: string, password: string) => storeAuth(await apiRequest<AuthResponse>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }, { auth: false }));
export const register = async (input: RegisterInput) => storeAuth(await apiRequest<AuthResponse>("/auth/register", { method: "POST", body: JSON.stringify(input) }, { auth: false }));
export const restoreSession = async () => storeAuth(await apiRequest<AuthResponse>("/auth/refresh", { method: "POST" }, { auth: false }));
export const logout = async () => { await apiRequest<void>("/auth/logout", { method: "POST" }, { auth: false }); setAccessToken(null); };
export const logoutAll = async () => { await apiRequest<void>("/auth/logout-all", { method: "POST" }); setAccessToken(null); };

export const sendVerificationCode = (email: string, purpose: "register" | "password_reset" | "account_deletion") => apiRequest<{ message: string }>("/email/send-verification-code", { method: "POST", body: JSON.stringify({ email, purpose }) }, { auth: false });
export const verifyEmailCode = (email: string, code: string, purpose: "register" | "password_reset" | "account_deletion") => apiRequest<{ message: string }>("/email/verify-code", { method: "POST", body: JSON.stringify({ email, code, purpose }) }, { auth: false });
export const sendPasswordReset = (email: string) => apiRequest<{ message: string }>("/auth/password-reset/send", { method: "POST", body: JSON.stringify({ email }) }, { auth: false });
export const confirmPasswordReset = (input: PasswordResetInput) => apiRequest<{ message: string }>("/auth/password-reset/confirm", { method: "POST", body: JSON.stringify(input) }, { auth: false });

export const getLegal = () => apiRequest<{ legal: LegalInfo }>("/legal/current", {}, { auth: false }).then((value) => value.legal);
export const acceptLegal = () => apiRequest<void>("/legal/accept", { method: "POST" });
export const changePassword = (oldPassword: string, newPassword: string, confirmPassword: string) => apiRequest<void>("/users/change-password", { method: "POST", body: JSON.stringify({ old_password: oldPassword, new_password: newPassword, confirm_password: confirmPassword }) });
export const listSessions = () => apiRequest<{ sessions: RefreshSession[] }>("/auth/sessions").then((value) => value.sessions);
export const revokeSession = (id: number) => apiRequest<void>(`/auth/sessions/${id}`, { method: "DELETE" });
export const listBlocks = () => apiRequest<{ blocks: UserBlock[] }>("/users/blocks").then((value) => value.blocks);
export const unblockUser = (userId: number) => apiRequest<void>(`/users/blocks/${userId}`, { method: "DELETE" });
export const reportUser = (input: { reported_user_id: number; category: string; details: string }) => apiRequest<void>("/users/reports", { method: "POST", body: JSON.stringify(input) });
export const deleteAccount = (input: { password?: string; code?: string }) => apiRequest<{ message: string }>("/users/me/deletion", { method: "POST", body: JSON.stringify(input) });

export const listOrganizations = () => apiRequest<{ organizations: Organization[] }>("/organizations").then((value) => value.organizations);
export const createOrganization = (name: string) => apiRequest<{ organization: Organization }>("/organizations", { method: "POST", body: JSON.stringify({ name }) }).then((value) => value.organization);
export const switchOrganization = (id: number) => apiRequest<{ organization: Organization }>(`/organizations/${id}/switch`, { method: "POST" }).then((value) => value.organization);
export const acceptOrganizationInvite = (code: string) => apiRequest<{ invite: { organization_id: number } }>(`/organizations/invites/${encodeURIComponent(code)}/accept`, { method: "POST" });

