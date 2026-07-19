import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, Bot, Check, Plus, Save, ShieldAlert, Trash2, Wrench } from "lucide-react";
import { useMemo, useState } from "react";

import { createAgentSkill, deleteAgentSkill, listAgentSkills, listMCPInstallations, listMCPInstallationTools, updateAgentSkill, type AgentSkill, type MCPTool } from "@/api/mcp";
import { FormError } from "@/components/AuthLayout";
import { EmptyPanel, StatusBadge } from "@/components/admin/AdminPrimitives";
import { PageError, PageLoading } from "@/components/PageState";
import { canBindInstallationToSkill, toolApprovalReason, toolRiskLabel } from "@/pages/mcp/mcpFormatters";

export function MCPSkillsPanel({ organizationId, organizationRole }: { organizationId?: number; organizationRole?: string }) {
  const queryClient = useQueryClient(); const canManageOrganization = organizationRole === "owner" || organizationRole === "admin";
  const skills = useQuery({ queryKey: ["organizations", organizationId, "mcp", "skills"], queryFn: listAgentSkills, enabled: Boolean(organizationId) });
  const installations = useQuery({ queryKey: ["organizations", organizationId, "mcp", "installations"], queryFn: listMCPInstallations, enabled: Boolean(organizationId) });
  const toolQueries = useQueries({ queries: (installations.data ?? []).filter((item) => item.status === "active").map((item) => ({ queryKey: ["organizations", organizationId, "mcp", "installations", item.id, "tools"], queryFn: () => listMCPInstallationTools(item.id), enabled: Boolean(organizationId) })) });
  const tools = useMemo(() => toolQueries.flatMap((query) => query.data ?? []), [toolQueries]);
  const installationScopes = useMemo(() => new Map((installations.data ?? []).map((installation) => [installation.id, installation.scope])), [installations.data]);
  const [selectedId, setSelectedId] = useState<number | "new">("new");
  const effectiveSelectedId = selectedId === "new" || skills.data?.some((skill) => skill.id === selectedId) ? selectedId : skills.data?.[0]?.id ?? "new";
  const selected = effectiveSelectedId === "new" ? null : skills.data?.find((skill) => skill.id === effectiveSelectedId) ?? null;
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "mcp", "skills"] });

  if (skills.isLoading || installations.isLoading) return <PageLoading />;
  if (skills.isError) return <PageError error={skills.error} retry={() => void skills.refetch()} />;
  if (installations.isError) return <PageError error={installations.error} retry={() => void installations.refetch()} />;
  return <div className="mcp-skill-layout">
    <aside className="mcp-skill-list"><button className={effectiveSelectedId === "new" ? "active new-skill" : "new-skill"} onClick={() => setSelectedId("new")}><Plus size={17} /><span><strong>新建 Skill</strong><small>组合已验证的 MCP 工具</small></span></button>{(skills.data ?? []).map((skill) => <button key={skill.id} className={skill.id === effectiveSelectedId ? "active" : ""} onClick={() => setSelectedId(skill.id)}><Bot size={17} /><span><strong>{skill.name}</strong><small>{skill.scope === "personal" ? "个人" : "组织"} · v{skill.version}</small></span><StatusBadge value={skill.status} /></button>)}</aside>
    <SkillEditor key={selected?.id ?? "new"} skill={selected} tools={tools} installationScopes={installationScopes} canManageOrganization={canManageOrganization} onCreated={(skill) => { refresh(); setSelectedId(skill.id); }} onUpdated={refresh} onDeleted={() => { refresh(); setSelectedId("new"); }} />
  </div>;
}

function SkillEditor({ skill, tools, installationScopes, canManageOrganization, onCreated, onUpdated, onDeleted }: { skill: AgentSkill | null; tools: MCPTool[]; installationScopes: Map<number, "personal" | "organization">; canManageOrganization: boolean; onCreated(skill: AgentSkill): void; onUpdated(): void; onDeleted(): void }) {
  const [name, setName] = useState(skill?.name ?? ""); const [description, setDescription] = useState(skill?.description ?? ""); const [instructions, setInstructions] = useState(skill?.instructions ?? "");
  const [scope, setScope] = useState<"personal" | "organization">(skill?.scope ?? "personal"); const [toolIds, setToolIds] = useState<number[]>([]); const [replaceTools, setReplaceTools] = useState(!skill);
  const editable = !skill || skill.scope === "personal" || canManageOrganization;
  const create = useMutation({ mutationFn: () => createAgentSkill({ name: name.trim(), description: description.trim(), instructions: instructions.trim(), scope, tool_ids: toolIds }), onSuccess: onCreated });
  const update = useMutation({ mutationFn: () => updateAgentSkill(skill!.id, { name: name.trim(), description: description.trim(), instructions: instructions.trim(), ...(replaceTools ? { tool_ids: toolIds } : {}) }), onSuccess: onUpdated });
  const status = useMutation({ mutationFn: () => updateAgentSkill(skill!.id, { status: skill!.status === "active" ? "disabled" : "active" }), onSuccess: onUpdated });
  const remove = useMutation({ mutationFn: () => deleteAgentSkill(skill!.id), onSuccess: onDeleted });
  const error = create.error || update.error || status.error || remove.error;
  const eligibleTools = tools.filter((tool) => canBindInstallationToSkill(scope, installationScopes.get(tool.installation_id) ?? "personal"));
  const toggleTool = (toolId: number) => setToolIds((current) => current.includes(toolId) ? current.filter((id) => id !== toolId) : [...current, toolId]);
  const changeScope = (nextScope: "personal" | "organization") => {
    setScope(nextScope);
    if (nextScope === "organization") {
      setToolIds((current) => current.filter((toolId) => eligibleTools.some((tool) => tool.id === toolId && installationScopes.get(tool.installation_id) === "organization")));
    }
  };
  const ready = name.trim() && instructions.trim() && (skill || toolIds.length > 0);

  return <section className="mcp-skill-editor"><header><div><h2>{skill ? skill.name : "新建 Agent Skill"}</h2><p>{skill ? `${skill.scope === "personal" ? "个人" : "组织"} · Version ${skill.version}` : "将指令绑定到固定 revision 的 MCP 工具"}</p></div>{skill && <StatusBadge value={skill.status} />}</header>
    <FormError error={error} />
    {!editable && <div className="status-error"><ShieldAlert size={16} />组织 Skill 仅 owner 或 admin 可编辑</div>}
    <div className="mcp-skill-form">
      <label>名称<input className="field" disabled={!editable} maxLength={160} value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：客户资料同步" /></label>
      {!skill && <label>作用域<div className="segmented" role="group" aria-label="Skill 作用域"><button className={scope === "personal" ? "active" : ""} onClick={() => changeScope("personal")}>个人</button><button className={scope === "organization" ? "active" : ""} disabled={!canManageOrganization} onClick={() => changeScope("organization")}>组织</button></div></label>}
      <label className="mcp-span-two">说明<input className="field" disabled={!editable} value={description} onChange={(event) => setDescription(event.target.value)} placeholder="Skill 的用途和边界" /></label>
      <label className="mcp-span-two">指令<textarea className="field mcp-instructions" disabled={!editable} value={instructions} onChange={(event) => setInstructions(event.target.value)} placeholder="描述 Agent 应如何使用这些工具，以及必须遵守的约束。" /></label>
    </div>
    <section className="mcp-skill-tools"><header><div><h3>工具绑定</h3><p>{skill && !replaceTools ? "保留当前绑定；启用替换后提交新的工具集合" : `${toolIds.length} 个工具已选择`}</p></div>{skill && editable && <label className="mcp-replace-tools"><input type="checkbox" checked={replaceTools} onChange={(event) => setReplaceTools(event.target.checked)} />替换绑定</label>}</header>
      {!eligibleTools.length ? <EmptyPanel>{scope === "organization" ? "组织 Skill 只能绑定已发布到组织的工具" : "暂无可绑定的 active MCP 工具"}</EmptyPanel> : <div className="mcp-tool-picker">{eligibleTools.map((tool) => { const checked = toolIds.includes(tool.id); return <label key={tool.id} className={checked ? "selected" : ""}><input type="checkbox" disabled={!editable || !replaceTools} checked={checked} onChange={() => toggleTool(tool.id)} /><span className="mcp-tool-check">{checked && <Check size={13} />}</span><span><strong><Wrench size={14} />{tool.original_name}</strong><small>{tool.name}</small><small>{toolRiskLabel(tool.risk)} · Revision #{tool.revision_id} · {toolApprovalReason(tool.risk)}</small></span></label>; })}</div>}
    </section>
    {editable && <footer className="mcp-editor-actions">{skill && <><button className="button-secondary" disabled={status.isPending} onClick={() => status.mutate()}><Ban size={16} />{skill.status === "active" ? "禁用" : "启用"}</button><button className="icon-button text-danger" title="删除 Skill" aria-label="删除 Skill" disabled={remove.isPending} onClick={() => remove.mutate()}><Trash2 size={17} /></button></>}<button className="button-primary" disabled={!ready || create.isPending || update.isPending} onClick={() => skill ? update.mutate() : create.mutate()}><Save size={16} />{skill ? "保存版本" : "创建 Skill"}</button></footer>}
  </section>;
}
