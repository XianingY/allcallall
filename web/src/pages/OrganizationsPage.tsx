import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useState } from "react";

import {
  getOrganizationAdminSummary,
  getOrganizationPolicy,
  listOrganizationAuditEvents,
  listOrganizationInvites,
  listOrganizationMembers,
  listOrganizationTeams,
} from "@/api/identity";
import { useAuth } from "@/auth/AuthContext";
import { FormError } from "@/components/AuthLayout";
import { PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationContext";
import { AuditTab, InvitesTab, MembersTab, Overview, PoliciesTab, TeamsTab, type Tab } from "@/pages/organizations/OrganizationAdminTabs";
import { organizationTabs, tabLabel } from "@/pages/organizations/organizationTabs";

export function OrganizationsPage() {
  const { organizations, activeOrganization, loading, select, create } = useOrganization();
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("overview");
  const [name, setName] = useState("");
  const [error, setError] = useState<unknown>();
  const orgId = activeOrganization?.id;
  const canManage = activeOrganization?.role === "owner" || activeOrganization?.role === "admin";
  const submit = async () => { setError(undefined); try { await create(name); setName(""); } catch (caught) { setError(caught); } };

  const members = useQuery({ queryKey: ["organizations", orgId, "members"], queryFn: () => listOrganizationMembers(orgId!), enabled: Boolean(orgId) });
  const invites = useQuery({ queryKey: ["organizations", orgId, "invites"], queryFn: () => listOrganizationInvites(orgId!), enabled: Boolean(orgId) });
  const teams = useQuery({ queryKey: ["organizations", orgId, "teams"], queryFn: () => listOrganizationTeams(orgId!), enabled: Boolean(orgId) });
  const policy = useQuery({ queryKey: ["organizations", orgId, "policy"], queryFn: () => getOrganizationPolicy(orgId!), enabled: Boolean(orgId) });
  const audit = useQuery({ queryKey: ["organizations", orgId, "audit"], queryFn: () => listOrganizationAuditEvents(orgId!), enabled: Boolean(orgId) });
  const summary = useQuery({ queryKey: ["organizations", orgId, "admin-summary"], queryFn: () => getOrganizationAdminSummary(orgId!), enabled: Boolean(orgId && canManage) });
  const refreshOrgAdmin = () => {
    void queryClient.invalidateQueries({ queryKey: ["organizations", orgId] });
  };

  return <div className="page"><header className="page-header"><div><p className="eyebrow">Workspace</p><h1>组织</h1><p>小团队 Beta 的成员、邀请、团队和录制策略管理。</p></div></header>
    <div className="org-admin-layout">
      <aside className="panel panel-body org-sidebar">
        <h2>我的组织</h2>
        {loading ? <PageLoading /> : <div className="list-stack">{organizations.map((organization) => <button key={organization.id} className={`select-row ${organization.id === activeOrganization?.id ? "select-row-active" : ""}`} onClick={() => void select(organization.id)}><span><strong>{organization.name}</strong><small>{organization.slug || `ID ${organization.id}`}</small></span><span className="role-badge">{organization.role}</span></button>)}</div>}
        <h2 className="mt-4">创建组织</h2>
        <FormError error={error} />
        <div className="form-stack"><label>组织名称<input className="field" value={name} onChange={(event) => setName(event.target.value)} /></label><button className="button-primary" disabled={!name.trim()} onClick={() => void submit()}><Plus size={17} />创建并切换</button></div>
      </aside>
      <main className="panel panel-body org-admin-main">
        <div className="org-tabs">{organizationTabs.map((item) => <button key={item} className={tab === item ? "active" : ""} onClick={() => setTab(item)}>{tabLabel(item)}</button>)}</div>
        {!orgId ? <div className="pane-empty">请选择组织</div> : tab === "overview" ? <Overview active={activeOrganization} canManage={canManage} members={members.data ?? []} teams={teams.data ?? []} currentUserId={user?.id} summary={summary} /> : tab === "members" ? <MembersTab orgId={orgId} canManage={canManage} currentUserId={user?.id} members={members} refresh={refreshOrgAdmin} /> : tab === "invites" ? <InvitesTab orgId={orgId} canManage={canManage} invites={invites} teams={teams.data ?? []} refresh={refreshOrgAdmin} /> : tab === "teams" ? <TeamsTab orgId={orgId} canManage={canManage} members={members.data ?? []} teams={teams} refresh={refreshOrgAdmin} /> : tab === "policies" ? <PoliciesTab orgId={orgId} canManage={canManage} policy={policy} refresh={refreshOrgAdmin} /> : <AuditTab audit={audit} />}
      </main>
    </div>
  </div>;
}
