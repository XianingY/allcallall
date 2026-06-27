import { Plus } from "lucide-react";
import { useState } from "react";

import { FormError } from "@/components/AuthLayout";
import { PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

export function OrganizationsPage() {
  const { organizations, activeOrganization, loading, select, create } = useOrganization();
  const [name, setName] = useState("");
  const [error, setError] = useState<unknown>();
  const submit = async () => { setError(undefined); try { await create(name); setName(""); } catch (caught) { setError(caught); } };
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Workspace</p><h1>组织</h1><p>选择当前工作空间，所有业务查询会按组织隔离。</p></div></header>
    <div className="settings-grid"><section className="panel panel-body"><h2>我的组织</h2>{loading ? <PageLoading /> : <div className="list-stack">{organizations.map((organization) => <button key={organization.id} className={`select-row ${organization.id === activeOrganization?.id ? "select-row-active" : ""}`} onClick={() => void select(organization.id)}><span><strong>{organization.name}</strong><small>{organization.slug || `ID ${organization.id}`}</small></span><span className="role-badge">{organization.role}</span></button>)}</div>}</section>
      <section className="panel panel-body"><h2>创建组织</h2><p className="section-copy">新组织会自动创建默认 Pipeline 和权限边界。</p><FormError error={error} /><div className="form-stack"><label>组织名称<input className="field" value={name} onChange={(event) => setName(event.target.value)} /></label><button className="button-primary" disabled={!name.trim()} onClick={() => void submit()}><Plus size={17} />创建并切换</button></div></section>
    </div>
  </div>;
}

