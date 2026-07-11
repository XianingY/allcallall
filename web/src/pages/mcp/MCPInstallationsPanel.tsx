import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, CheckCircle2, Edit3, Globe2, KeyRound, Package, Play, Search, ShieldAlert, ShieldCheck, UploadCloud } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  activateMCPInstallation,
  disableMCPInstallation,
  getMCPInstallation,
  listMCPInstallations,
  listMCPInstallationTools,
  publishMCPInstallation,
  validateMCPInstallation,
} from "@/api/mcp";
import { FormError } from "@/components/AuthLayout";
import { EmptyPanel, StatusBadge } from "@/components/admin/AdminPrimitives";
import { PageError, PageLoading } from "@/components/PageState";
import { MCPSecretsDialog, RenameInstallationDialog } from "@/pages/mcp/MCPDialogs";
import { canPublishInstallation, formatTimestamp, installationRevisionLabel, installationSourceLabel, toolApprovalReason, toolRiskLabel } from "@/pages/mcp/mcpFormatters";

interface Props {
  organizationId?: number;
  organizationRole?: string;
  selectedId: number;
  onSelectedId(id: number): void;
}

export function MCPInstallationsPanel({ organizationId, organizationRole, selectedId, onSelectedId }: Props) {
  const queryClient = useQueryClient(); const [search, setSearch] = useState(""); const [scope, setScope] = useState<"all" | "personal" | "organization">("all");
  const [secretsOpen, setSecretsOpen] = useState(false); const [renameOpen, setRenameOpen] = useState(false);
  const installations = useQuery({ queryKey: ["organizations", organizationId, "mcp", "installations"], queryFn: listMCPInstallations, enabled: Boolean(organizationId) });
  const visible = useMemo(() => (installations.data ?? []).filter((item) => {
    const matchesScope = scope === "all" || item.scope === scope;
    return matchesScope && item.display_name.toLowerCase().includes(search.trim().toLowerCase());
  }), [installations.data, scope, search]);
  useEffect(() => {
    if (!selectedId && visible[0]) onSelectedId(visible[0].id);
    if (selectedId && installations.data && !installations.data.some((item) => item.id === selectedId)) onSelectedId(visible[0]?.id ?? 0);
  }, [installations.data, onSelectedId, selectedId, visible]);
  const detail = useQuery({ queryKey: ["organizations", organizationId, "mcp", "installations", selectedId], queryFn: () => getMCPInstallation(selectedId), enabled: Boolean(organizationId && selectedId) });
  const tools = useQuery({ queryKey: ["organizations", organizationId, "mcp", "installations", selectedId, "tools"], queryFn: () => listMCPInstallationTools(selectedId), enabled: Boolean(organizationId && selectedId) });
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "mcp"] });
  };
  const validate = useMutation({ mutationFn: () => validateMCPInstallation(selectedId), onSuccess: refresh });
  const activate = useMutation({ mutationFn: () => activateMCPInstallation(selectedId), onSuccess: refresh });
  const publish = useMutation({ mutationFn: () => publishMCPInstallation(selectedId), onSuccess: refresh });
  const disable = useMutation({ mutationFn: () => disableMCPInstallation(selectedId), onSuccess: refresh });
  const actionError = validate.error || activate.error || publish.error || disable.error;
  const installation = detail.data ?? installations.data?.find((item) => item.id === selectedId) ?? null;
  const canMutate = installation?.scope === "personal" || organizationRole === "owner" || organizationRole === "admin";

  if (installations.isLoading) return <PageLoading />;
  if (installations.isError) return <PageError error={installations.error} retry={() => void installations.refetch()} />;
  return <div className="mcp-installation-layout">
    <aside className="mcp-installation-list" aria-label="MCP 安装列表">
      <label className="search-field"><Search size={16} /><input aria-label="搜索 MCP 安装" placeholder="搜索安装" value={search} onChange={(event) => setSearch(event.target.value)} /></label>
      <div className="filter-tabs" role="group" aria-label="安装作用域"><button className={scope === "all" ? "active" : ""} onClick={() => setScope("all")}>全部</button><button className={scope === "personal" ? "active" : ""} onClick={() => setScope("personal")}>个人</button><button className={scope === "organization" ? "active" : ""} onClick={() => setScope("organization")}>组织</button></div>
      <div className="mcp-installation-items">{visible.map((item) => <button key={item.id} className={item.id === selectedId ? "active" : ""} onClick={() => onSelectedId(item.id)}>
        <span className="mcp-source-icon">{item.source_type === "oci" ? <Package size={17} /> : <Globe2 size={17} />}</span><span className="mcp-list-copy"><strong>{item.display_name}</strong><small>{item.scope === "personal" ? "个人" : "组织"} · {item.source_type.toUpperCase()}</small></span><StatusBadge value={item.status} />
      </button>)}</div>
      {!visible.length && <EmptyPanel>没有匹配的 MCP 安装</EmptyPanel>}
    </aside>
    <section className="mcp-installation-detail">
      {!installation ? <div className="pane-empty"><Package size={28} /><strong>选择一个 MCP 安装</strong></div> : detail.isLoading ? <PageLoading /> : detail.isError ? <PageError error={detail.error} retry={() => void detail.refetch()} /> : <>
        <header className="mcp-detail-header"><div><div className="mcp-title-line"><h2>{installation.display_name}</h2><StatusBadge value={installation.status} /></div><p>{installationSourceLabel(installation)}</p></div>{canMutate && <div className="button-row"><button className="icon-button" title="重命名" aria-label="重命名安装" onClick={() => setRenameOpen(true)}><Edit3 size={17} /></button><button className="button-secondary" onClick={() => setSecretsOpen(true)}><KeyRound size={16} />Secret</button></div>}</header>
        <FormError error={actionError} />
        {installation.last_error && <div className="status-error"><strong>最近一次验证失败</strong><span>{installation.last_error}</span></div>}
        <dl className="mcp-meta-grid">
          <div><dt>来源</dt><dd>{installation.source_type.toUpperCase()}</dd></div><div><dt>作用域</dt><dd>{installation.scope === "personal" ? "个人" : "组织"}</dd></div><div><dt>Revision</dt><dd>{installationRevisionLabel(installation)}</dd></div><div><dt>扫描</dt><dd>{installation.latest_revision?.scan_status ?? "pending"}</dd></div><div><dt>Secret</dt><dd>{installation.secrets_configured ? "已配置" : "未配置"}</dd></div><div><dt>更新时间</dt><dd>{formatTimestamp(installation.updated_at)}</dd></div>
        </dl>
        <div className="mcp-action-band">
          <div><strong>安装生命周期</strong><span>验证结果与工具 schema 固定在当前 revision</span></div>
          {canMutate && <div className="button-row"><button className="button-secondary" disabled={validate.isPending || installation.status === "validating"} onClick={() => validate.mutate()}><ShieldCheck size={16} />连接验证</button>{installation.status === "disabled" && <button className="button-primary" disabled={activate.isPending} onClick={() => activate.mutate()}><Play size={16} />启用</button>}{canPublishInstallation(installation, organizationRole) && <button className="button-secondary" disabled={publish.isPending} onClick={() => publish.mutate()}><UploadCloud size={16} />发布到组织</button>}{installation.status !== "disabled" && <button className="button-secondary text-danger" disabled={disable.isPending} onClick={() => disable.mutate()}><Ban size={16} />禁用</button>}</div>}
        </div>
        <section className="mcp-tools-section"><header><div><h3>工具目录</h3><p>{installationRevisionLabel(installation)} · MCP 返回内容按不可信数据处理</p></div><span>{tools.data?.length ?? 0} tools</span></header>
          {tools.isLoading ? <PageLoading /> : tools.isError ? <PageError error={tools.error} retry={() => void tools.refetch()} /> : tools.data?.length ? <div className="table-wrap"><table><thead><tr><th>工具</th><th>风险</th><th>Revision</th><th>执行策略</th><th>状态</th></tr></thead><tbody>{tools.data.map((tool) => <tr key={tool.id}><td><div className="mcp-tool-name"><code>{tool.name}</code><span>{tool.description || tool.original_name}</span></div></td><td><span className={`mcp-risk risk-${tool.risk}`}>{tool.risk === "read" ? <CheckCircle2 size={13} /> : <ShieldAlert size={13} />}{toolRiskLabel(tool.risk)}</span></td><td>#{tool.revision_id}<small className="mcp-schema-version">schema {tool.schema_version}</small></td><td>{toolApprovalReason(tool.risk)}</td><td><StatusBadge value={tool.status} /></td></tr>)}</tbody></table></div> : <EmptyPanel>验证成功后显示发现的工具</EmptyPanel>}
        </section>
      </>}
    </section>
    <MCPSecretsDialog key={`secrets-${installation?.id ?? 0}`} installation={installation} open={secretsOpen} onOpenChange={setSecretsOpen} organizationId={organizationId} />
    <RenameInstallationDialog key={`rename-${installation?.id ?? 0}`} installation={installation} open={renameOpen} onOpenChange={setRenameOpen} organizationId={organizationId} />
  </div>;
}
