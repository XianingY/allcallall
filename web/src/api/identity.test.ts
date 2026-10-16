import { beforeEach, describe, expect, it, vi } from "vitest";

const apiRequestMock = vi.hoisted(() => vi.fn());
const setAccessTokenMock = vi.hoisted(() => vi.fn());
vi.mock("@/api/http", () => ({ apiRequest: apiRequestMock, setAccessToken: setAccessTokenMock }));

import * as identity from "@/api/identity";

beforeEach(() => {
  apiRequestMock.mockReset();
  setAccessTokenMock.mockReset();
});

describe("identity api", () => {
  it("auth endpoints store the access token", async () => {
    apiRequestMock.mockResolvedValue({ access_token: "tok" });
    await expect(identity.login("a@b.c", "pw")).resolves.toEqual({ access_token: "tok" });
    expect(apiRequestMock).toHaveBeenCalledWith(
      "/auth/login",
      { method: "POST", body: JSON.stringify({ email: "a@b.c", password: "pw" }) },
      { auth: false },
    );
    expect(setAccessTokenMock).toHaveBeenCalledWith("tok");

    setAccessTokenMock.mockClear();
    await identity.register({ email: "a@b.c", password: "pw", display_name: "A", accept_current_legal: true });
    expect(setAccessTokenMock).toHaveBeenCalledWith("tok");

    setAccessTokenMock.mockClear();
    await identity.restoreSession();
    expect(apiRequestMock).toHaveBeenCalledWith("/auth/refresh", { method: "POST" }, { auth: false });
    expect(setAccessTokenMock).toHaveBeenCalledWith("tok");
  });

  it("logout paths clear the token", async () => {
    await identity.logout();
    expect(apiRequestMock).toHaveBeenCalledWith("/auth/logout", { method: "POST" }, { auth: false });
    expect(setAccessTokenMock).toHaveBeenCalledWith(null);

    setAccessTokenMock.mockClear();
    await identity.logoutAll();
    expect(apiRequestMock).toHaveBeenCalledWith("/auth/logout-all", { method: "POST" });
    expect(setAccessTokenMock).toHaveBeenCalledWith(null);
  });

  it("verification, reset, and legal endpoints", async () => {
    await identity.sendVerificationCode("a@b.c", "register");
    expect(apiRequestMock).toHaveBeenCalledWith(
      "/email/send-verification-code",
      { method: "POST", body: JSON.stringify({ email: "a@b.c", purpose: "register" }) },
      { auth: false },
    );

    await identity.verifyEmailCode("a@b.c", "123456", "password_reset");
    expect(apiRequestMock).toHaveBeenCalledWith(
      "/email/verify-code",
      { method: "POST", body: JSON.stringify({ email: "a@b.c", code: "123456", purpose: "password_reset" }) },
      { auth: false },
    );

    await identity.sendPasswordReset("a@b.c");
    expect(apiRequestMock).toHaveBeenCalledWith(
      "/auth/password-reset/send",
      { method: "POST", body: JSON.stringify({ email: "a@b.c" }) },
      { auth: false },
    );

    const reset = { email: "a@b.c", code: "123456", new_password: "n", confirm_password: "n" };
    await identity.confirmPasswordReset(reset);
    expect(apiRequestMock).toHaveBeenCalledWith("/auth/password-reset/confirm", { method: "POST", body: JSON.stringify(reset) }, { auth: false });

    apiRequestMock.mockResolvedValue({ legal: { version: "v1" } });
    await expect(identity.getLegal()).resolves.toEqual({ version: "v1" });
    expect(apiRequestMock).toHaveBeenCalledWith("/legal/current", {}, { auth: false });

    await identity.acceptLegal();
    expect(apiRequestMock).toHaveBeenCalledWith("/legal/accept", { method: "POST" });
  });

  it("account security endpoints", async () => {
    await identity.changePassword("old", "new", "new");
    expect(apiRequestMock).toHaveBeenCalledWith("/users/change-password", {
      method: "POST",
      body: JSON.stringify({ old_password: "old", new_password: "new", confirm_password: "new" }),
    });

    apiRequestMock.mockResolvedValue({ sessions: [{ id: 1 }] });
    await expect(identity.listSessions()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/auth/sessions");

    await identity.revokeSession(1);
    expect(apiRequestMock).toHaveBeenCalledWith("/auth/sessions/1", { method: "DELETE" });

    apiRequestMock.mockResolvedValue({ blocks: [{ id: 2 }] });
    await expect(identity.listBlocks()).resolves.toEqual([{ id: 2 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/users/blocks");

    await identity.unblockUser(2);
    expect(apiRequestMock).toHaveBeenCalledWith("/users/blocks/2", { method: "DELETE" });

    const report = { reported_user_id: 2, category: "spam", details: "x" };
    await identity.reportUser(report);
    expect(apiRequestMock).toHaveBeenCalledWith("/users/reports", { method: "POST", body: JSON.stringify(report) });

    await identity.deleteAccount({ password: "pw" });
    expect(apiRequestMock).toHaveBeenCalledWith("/users/me/deletion", { method: "POST", body: JSON.stringify({ password: "pw" }) });
  });

  it("organization endpoints", async () => {
    apiRequestMock.mockResolvedValue({ organizations: [{ id: 1 }] });
    await expect(identity.listOrganizations()).resolves.toEqual([{ id: 1 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations");

    apiRequestMock.mockResolvedValue({ organization: { id: 1 } });
    await expect(identity.createOrganization("Acme")).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations", { method: "POST", body: JSON.stringify({ name: "Acme" }) });

    await expect(identity.switchOrganization(1)).resolves.toEqual({ id: 1 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/switch", { method: "POST" });

    await identity.acceptOrganizationInvite("inv-code");
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/invites/inv-code/accept", { method: "POST" });

    apiRequestMock.mockResolvedValue({ summary: { total_members: 3 } });
    await expect(identity.getOrganizationAdminSummary(1)).resolves.toEqual({ total_members: 3 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/admin/summary");
  });

  it("organization member, invite, team, policy, and audit endpoints", async () => {
    await identity.listOrganizationMembers(1, { limit: 10, offset: 20 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/members?limit=10&offset=20");

    apiRequestMock.mockResolvedValue({ member: { id: 2 } });
    await expect(identity.updateOrganizationMember(1, 2, "admin")).resolves.toEqual({ id: 2 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/members/2", { method: "PATCH", body: JSON.stringify({ role: "admin" }) });

    await identity.removeOrganizationMember(1, 2);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/members/2", { method: "DELETE" });

    apiRequestMock.mockResolvedValue({ invites: [{ id: 3 }] });
    await expect(identity.listOrganizationInvites(1)).resolves.toEqual([{ id: 3 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/invites");

    apiRequestMock.mockResolvedValue({ invite: { id: 3 } });
    await expect(identity.createOrganizationInvite(1, { target_email: "x@y.z", role: "member" })).resolves.toEqual({ id: 3 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/invites", {
      method: "POST",
      body: JSON.stringify({ target_email: "x@y.z", role: "member" }),
    });

    await expect(identity.resendOrganizationInvite(1, 3)).resolves.toEqual({ id: 3 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/invites/3/resend", { method: "POST" });

    await identity.revokeOrganizationInvite(1, 3);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/invites/3", { method: "DELETE" });

    apiRequestMock.mockResolvedValue({ teams: [{ id: 4 }] });
    await expect(identity.listOrganizationTeams(1)).resolves.toEqual([{ id: 4 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/teams");

    apiRequestMock.mockResolvedValue({ team: { id: 4 } });
    await expect(identity.createOrganizationTeam(1, { name: "Core" })).resolves.toEqual({ id: 4 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/teams", { method: "POST", body: JSON.stringify({ name: "Core" }) });

    await expect(identity.updateOrganizationTeam(1, 4, { name: "Core 2" })).resolves.toEqual({ id: 4 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/teams/4", { method: "PATCH", body: JSON.stringify({ name: "Core 2" }) });

    await identity.deleteOrganizationTeam(1, 4);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/teams/4", { method: "DELETE" });

    await expect(identity.addOrganizationTeamMember(1, 4, 2)).resolves.toEqual({ id: 4 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/teams/4/members", { method: "POST", body: JSON.stringify({ user_id: 2 }) });

    await expect(identity.removeOrganizationTeamMember(1, 4, 2)).resolves.toEqual({ id: 4 });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/teams/4/members/2", { method: "DELETE" });

    apiRequestMock.mockResolvedValue({ policy: { recording_mode: "off" } });
    await expect(identity.getOrganizationPolicy(1)).resolves.toEqual({ recording_mode: "off" });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/policy");

    const policy = { recording_mode: "off", recording_storage_days: 7, recording_export_allowed: false };
    await expect(identity.updateOrganizationPolicy(1, policy)).resolves.toEqual({ recording_mode: "off" });
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/policy", { method: "PUT", body: JSON.stringify(policy) });

    apiRequestMock.mockResolvedValue({ events: [{ id: 5 }] });
    await expect(identity.listOrganizationAuditEvents(1)).resolves.toEqual([{ id: 5 }]);
    expect(apiRequestMock).toHaveBeenCalledWith("/organizations/1/audit-events");
  });
});
