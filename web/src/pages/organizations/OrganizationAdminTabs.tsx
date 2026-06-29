import { useMutation, type UseQueryResult } from "@tanstack/react-query";
import { Building2, MailPlus, Plus, RefreshCw, Shield, Trash2, Users, X } from "lucide-react";
import { useState } from "react";

import {
  addOrganizationTeamMember,
  createOrganizationInvite,
  createOrganizationTeam,
  deleteOrganizationTeam,
  removeOrganizationMember,
  removeOrganizationTeamMember,
  resendOrganizationInvite,
  revokeOrganizationInvite,
  updateOrganizationMember,
  updateOrganizationPolicy,
  type Organization,
  type OrganizationMember,
  type OrganizationTeam,
} from "@/api/identity";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";

export type Tab = "overview" | "members" | "invites" | "teams" | "policies" | "audit";

export function Overview({ active, members, teams, currentUserId }: { active: Organization | null; members: OrganizationMember[]; teams: OrganizationTeam[]; currentUserId?: number }) {
  const me = members.find((item) => item.user_id === currentUserId);
  return <div className="org-overview-grid">
    <article><Building2 size={18} /><span>当前组织</span><strong>{active?.name}</strong><small>{active?.slug}</small></article>
    <article><Users size={18} /><span>成员</span><strong>{members.length}</strong><small>我的角色：{me?.role || active?.role}</small></article>
    <article><Shield size={18} /><span>团队</span><strong>{teams.length}</strong><small>默认团队随组织创建</small></article>
  </div>;
}

export function MembersTab({ orgId, canManage, currentUserId, members, refresh }: { orgId: number; canManage: boolean; currentUserId?: number; members: UseQueryResult<OrganizationMember[]>; refresh(): void }) {
  const role = useMutation({ mutationFn: ({ userId, value }: { userId: number; value: string }) => updateOrganizationMember(orgId, userId, value), onSuccess: refresh });
  const remove = useMutation({ mutationFn: (userId: number) => removeOrganizationMember(orgId, userId), onSuccess: refresh });
  if (members.isLoading) return <PageLoading />;
  if (members.isError) return <PageError error={members.error} retry={() => void members.refetch()} />;
  return <div className="list-stack"><FormError error={role.error || remove.error} />{members.data?.map((member) => <div className="data-row" key={member.user_id}><div><strong>{member.display_name || member.email}</strong><small>{member.email}</small><small>{member.status} · joined {dateOnly(member.joined_at)}</small></div><div className="button-row"><select className="field compact-field" value={member.role} disabled={!canManage || member.user_id === currentUserId} onChange={(event) => role.mutate({ userId: member.user_id, value: event.target.value })}><option value="owner">owner</option><option value="admin">admin</option><option value="member">member</option></select><button className="icon-button text-danger" disabled={!canManage || member.user_id === currentUserId} onClick={() => remove.mutate(member.user_id)}><Trash2 size={16} /></button></div></div>)}</div>;
}

export function InvitesTab({ orgId, canManage, invites, teams, refresh }: { orgId: number; canManage: boolean; invites: UseQueryResult; teams: OrganizationTeam[]; refresh(): void }) {
  const [email, setEmail] = useState(""); const [role, setRole] = useState("member"); const [teamId, setTeamId] = useState("");
  const create = useMutation({ mutationFn: () => createOrganizationInvite(orgId, { target_email: email, role, team_id: teamId ? Number(teamId) : undefined }), onSuccess: () => { setEmail(""); refresh(); } });
  const resend = useMutation({ mutationFn: (inviteId: number) => resendOrganizationInvite(orgId, inviteId), onSuccess: refresh });
  const revoke = useMutation({ mutationFn: (inviteId: number) => revokeOrganizationInvite(orgId, inviteId), onSuccess: refresh });
  if (invites.isLoading) return <PageLoading />;
  if (invites.isError) return <PageError error={invites.error} retry={() => void invites.refetch()} />;
  return <div className="form-stack"><FormError error={create.error || resend.error || revoke.error} />{canManage && <div className="toolbar-panel org-invite-bar"><input className="field" type="email" placeholder="成员邮箱" value={email} onChange={(event) => setEmail(event.target.value)} /><select className="field" value={role} onChange={(event) => setRole(event.target.value)}><option value="member">member</option><option value="admin">admin</option></select><select className="field" value={teamId} onChange={(event) => setTeamId(event.target.value)}><option value="">不加入团队</option>{teams.map((team) => <option key={team.id} value={team.id}>{team.name}</option>)}</select><button className="button-primary" disabled={!email.trim()} onClick={() => create.mutate()}><MailPlus size={16} />邀请</button></div>}<div className="list-stack">{(invites.data as Array<{ id: number; target_email: string; role: string; status: string; code: string; expires_at: string }> | undefined)?.map((invite) => <div className="data-row" key={invite.id}><div><strong>{invite.target_email}</strong><small>{invite.role} · {invite.status} · expires {dateOnly(invite.expires_at)}</small><small>code: {invite.code}</small></div><div className="button-row"><button className="button-secondary" disabled={!canManage} onClick={() => resend.mutate(invite.id)}><RefreshCw size={15} />重发</button><button className="icon-button text-danger" disabled={!canManage || invite.status === "accepted"} onClick={() => revoke.mutate(invite.id)}><Trash2 size={16} /></button></div></div>)}</div></div>;
}

export function TeamsTab({ orgId, canManage, members, teams, refresh }: { orgId: number; canManage: boolean; members: OrganizationMember[]; teams: UseQueryResult<OrganizationTeam[]>; refresh(): void }) {
  const [name, setName] = useState(""); const [description, setDescription] = useState("");
  const create = useMutation({ mutationFn: () => createOrganizationTeam(orgId, { name, description }), onSuccess: () => { setName(""); setDescription(""); refresh(); } });
  const removeTeam = useMutation({ mutationFn: (teamId: number) => deleteOrganizationTeam(orgId, teamId), onSuccess: refresh });
  const addMember = useMutation({ mutationFn: ({ teamId, userId }: { teamId: number; userId: number }) => addOrganizationTeamMember(orgId, teamId, userId), onSuccess: refresh });
  const removeMember = useMutation({ mutationFn: ({ teamId, userId }: { teamId: number; userId: number }) => removeOrganizationTeamMember(orgId, teamId, userId), onSuccess: refresh });
  if (teams.isLoading) return <PageLoading />;
  if (teams.isError) return <PageError error={teams.error} retry={() => void teams.refetch()} />;
  return <div className="form-stack"><FormError error={create.error || removeTeam.error || addMember.error || removeMember.error} />{canManage && <div className="toolbar-panel org-team-bar"><input className="field" placeholder="团队名称" value={name} onChange={(event) => setName(event.target.value)} /><input className="field" placeholder="描述" value={description} onChange={(event) => setDescription(event.target.value)} /><button className="button-primary" disabled={!name.trim()} onClick={() => create.mutate()}><Plus size={16} />创建团队</button></div>}<div className="org-team-grid">{teams.data?.map((team) => <article className="panel team-card" key={team.id}><header><div><h2>{team.name}</h2><p>{team.description || team.slug}</p></div><button className="icon-button text-danger" disabled={!canManage} onClick={() => removeTeam.mutate(team.id)}><Trash2 size={16} /></button></header><div className="list-stack">{team.members?.map((member) => <div className="team-member-row" key={member.user_id}><span>{member.display_name || member.email}</span><button className="icon-button" disabled={!canManage} onClick={() => removeMember.mutate({ teamId: team.id, userId: member.user_id })}><X size={14} /></button></div>)}</div>{canManage && <select className="field" defaultValue="" onChange={(event) => { const userId = Number(event.target.value); if (userId) addMember.mutate({ teamId: team.id, userId }); event.currentTarget.value = ""; }}><option value="">添加成员</option>{members.map((member) => <option key={member.user_id} value={member.user_id}>{member.display_name || member.email}</option>)}</select>}</article>)}</div></div>;
}

export function PoliciesTab({ orgId, canManage, policy, refresh }: { orgId: number; canManage: boolean; policy: UseQueryResult; refresh(): void }) {
  if (policy.isLoading) return <PageLoading />;
  if (policy.isError) return <PageError error={policy.error} retry={() => void policy.refetch()} />;
  const data = policy.data as { id?: number; recording_mode?: string; recording_storage_days?: number; recording_export_allowed?: boolean } | undefined;
  return <PolicyForm key={`${orgId}-${data?.id ?? "new"}`} orgId={orgId} canManage={canManage} initialMode={data?.recording_mode ?? "off"} initialDays={data?.recording_storage_days ?? 30} initialExportAllowed={Boolean(data?.recording_export_allowed)} refresh={refresh} />;
}

function PolicyForm({ orgId, canManage, initialMode, initialDays, initialExportAllowed, refresh }: { orgId: number; canManage: boolean; initialMode: string; initialDays: number; initialExportAllowed: boolean; refresh(): void }) {
  const [mode, setMode] = useState(initialMode); const [days, setDays] = useState(initialDays); const [exportAllowed, setExportAllowed] = useState(initialExportAllowed);
  const save = useMutation({ mutationFn: () => updateOrganizationPolicy(orgId, { recording_mode: mode, recording_storage_days: days, recording_export_allowed: exportAllowed }), onSuccess: refresh });
  return <div className="form-stack org-policy-form"><FormError error={save.error} /><label>录制策略<select className="field" disabled={!canManage} value={mode} onChange={(event) => setMode(event.target.value)}><option value="off">off</option><option value="admin_opt_in">admin_opt_in</option><option value="forced_for_team_meetings">forced_for_team_meetings</option></select></label><label>保存天数<input className="field" type="number" min="1" disabled={!canManage} value={days} onChange={(event) => setDays(Number(event.target.value))} /></label><label className="checkbox-row"><input type="checkbox" disabled={!canManage} checked={exportAllowed} onChange={(event) => setExportAllowed(event.target.checked)} />允许导出录音</label><button className="button-primary" disabled={!canManage || save.isPending} onClick={() => save.mutate()}>保存策略</button></div>;
}

export function AuditTab({ audit }: { audit: UseQueryResult }) {
  if (audit.isLoading) return <PageLoading />;
  if (audit.isError) return <PageError error={audit.error} retry={() => void audit.refetch()} />;
  return <div className="list-stack">{(audit.data as Array<{ id: number; action: string; target_type: string; target_id: string; actor_display_name: string; actor_email: string; created_at: string; metadata?: Record<string, unknown> }> | undefined)?.map((event) => <div className="data-row" key={event.id}><div><strong>{event.action}</strong><small>{event.target_type} #{event.target_id} · {event.actor_display_name || event.actor_email}</small><small>{dateOnly(event.created_at)}</small></div>{event.metadata && <code>{JSON.stringify(event.metadata)}</code>}</div>)}</div>;
}

function dateOnly(value?: string | null) {
  return value ? new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)) : "-";
}
