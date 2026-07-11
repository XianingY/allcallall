import { useQuery } from "@tanstack/react-query";
import { Activity, Bot, Clock3, PackagePlus, Search, ShieldAlert, ShieldCheck, Wrench } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { getMCPExecution, getMCPInstallation, listMCPInstallationTools } from "@/api/mcp";
import { FormError } from "@/components/AuthLayout";
import { EmptyPanel, StatusBadge } from "@/components/admin/AdminPrimitives";
import { PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationContext";
import { InstallationWizard } from "@/pages/mcp/MCPDialogs";
import { MCPInstallationsPanel } from "@/pages/mcp/MCPInstallationsPanel";
import { MCPSkillsPanel } from "@/pages/mcp/MCPSkillsPanel";
import { formatTimestamp, installationSourceLabel, toolApprovalReason, toolRiskLabel } from "@/pages/mcp/mcpFormatters";

type View = "installations" | "skills" | "executions";

export function MCPPlatformPage() {
  const { activeOrganization } = useOrganization(); const organizationId = activeOrganization?.id; const [view, setView] = useState<View>("installations");
  const [wizardOpen, setWizardOpen] = useState(false); const [installationSelections, setInstallationSelections] = useState<Record<number, number>>({});
  const selectedInstallationId = organizationId ? installationSelections[organizationId] ?? 0 : 0;
  const setSelectedInstallationId = (installationId: number) => { if (organizationId) setInstallationSelections((current) => ({ ...current, [organizationId]: installationId })); };
  return <main className="mcp-page">
    <header className="mcp-page-header"><div><p className="eyebrow">Agent Tool Platform</p><h1>MCP 与 Skills</h1></div><div className="button-row"><Link className="button-secondary" to="/agent-lab"><ShieldCheck size={17} />审批与 Trace</Link>{view === "installations" && <button className="button-primary" onClick={() => setWizardOpen(true)}><PackagePlus size={17} />安装 MCP</button>}</div></header>
    <nav className="mcp-view-tabs" aria-label="MCP 平台视图"><button className={view === "installations" ? "active" : ""} onClick={() => setView("installations")}><Wrench size={16} />安装与工具</button><button className={view === "skills" ? "active" : ""} onClick={() => setView("skills")}><Bot size={16} />Skills</button><button className={view === "executions" ? "active" : ""} onClick={() => setView("executions")}><Activity size={16} />执行追踪</button></nav>
    <div className="mcp-page-content">{!organizationId ? <div className="pane-empty">请选择组织</div> : view === "installations" ? <MCPInstallationsPanel organizationId={organizationId} organizationRole={activeOrganization?.role} selectedId={selectedInstallationId} onSelectedId={setSelectedInstallationId} /> : view === "skills" ? <MCPSkillsPanel organizationId={organizationId} organizationRole={activeOrganization?.role} /> : <MCPExecutionLookup organizationId={organizationId} />}</div>
    <InstallationWizard open={wizardOpen} onOpenChange={setWizardOpen} organizationId={organizationId} canManageOrganization={activeOrganization?.role === "owner" || activeOrganization?.role === "admin"} onCreated={(installation) => setSelectedInstallationId(installation.id)} />
  </main>;
}

function MCPExecutionLookup({ organizationId }: { organizationId: number }) {
  const [input, setInput] = useState(""); const [executionId, setExecutionId] = useState("");
  const execution = useQuery({ queryKey: ["organizations", organizationId, "mcp", "executions", executionId], queryFn: () => getMCPExecution(executionId), enabled: Boolean(executionId), retry: false });
  const installation = useQuery({ queryKey: ["organizations", organizationId, "mcp", "installations", execution.data?.installation_id], queryFn: () => getMCPInstallation(execution.data!.installation_id), enabled: Boolean(execution.data?.installation_id) });
  const tools = useQuery({ queryKey: ["organizations", organizationId, "mcp", "installations", execution.data?.installation_id, "tools"], queryFn: () => listMCPInstallationTools(execution.data!.installation_id), enabled: Boolean(execution.data?.installation_id) });
  const tool = tools.data?.find((item) => item.id === execution.data?.tool_id);
  return <section className="mcp-execution-view">
    <div className="mcp-execution-search"><label><span>Execution ID</span><div className="input-icon"><Search size={16} /><input value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && input.trim()) setExecutionId(input.trim()); }} placeholder="例如：run-42:call-7" /></div></label><button className="button-primary" disabled={!input.trim()} onClick={() => setExecutionId(input.trim())}>查询</button></div>
    <FormError error={execution.error} />
    {execution.isLoading ? <PageLoading /> : !execution.data ? <EmptyPanel>输入 Execution ID 查看工具执行状态</EmptyPanel> : <div className="mcp-execution-detail">
      <header><div><h2>{execution.data.execution_id}</h2><p>Tool call {execution.data.tool_call_id}</p></div><StatusBadge value={execution.data.status} /></header>
      <dl className="mcp-meta-grid"><div><dt>来源</dt><dd>{installation.data ? installationSourceLabel(installation.data) : `Installation #${execution.data.installation_id}`}</dd></div><div><dt>Revision</dt><dd>#{execution.data.revision_id}</dd></div><div><dt>工具</dt><dd>{tool?.name ?? `Tool #${execution.data.tool_id}`}</dd></div><div><dt>风险</dt><dd>{tool ? toolRiskLabel(tool.risk) : "-"}</dd></div><div><dt>尝试次数</dt><dd>{execution.data.attempts}</dd></div><div><dt>创建时间</dt><dd>{formatTimestamp(execution.data.created_at)}</dd></div></dl>
      <div className="mcp-execution-policy"><ShieldAlert size={17} /><div><strong>{tool ? toolApprovalReason(tool.risk) : "工具风险策略由 Go 网关判定"}</strong><span>工具输出是未经信任的外部数据</span></div></div>
      <div className="mcp-execution-timeline"><div><Clock3 size={16} /><span>入队</span><time>{formatTimestamp(execution.data.created_at)}</time></div><div className={execution.data.started_at ? "done" : ""}><Activity size={16} /><span>开始执行</span><time>{formatTimestamp(execution.data.started_at)}</time></div><div className={execution.data.completed_at ? "done" : ""}><Activity size={16} /><span>执行完成</span><time>{formatTimestamp(execution.data.completed_at)}</time></div></div>
      {execution.data.error_message && <div className="status-error">{execution.data.error_message}</div>}
      <div className="mcp-payload-grid"><section><h3>输入</h3><pre>{JSON.stringify(execution.data.input, null, 2)}</pre></section><section><h3>输出 · 不可信</h3><pre>{JSON.stringify(execution.data.output, null, 2)}</pre></section></div>
    </div>}
  </section>;
}
