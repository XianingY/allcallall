import * as Dialog from "@radix-ui/react-dialog";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Globe2, KeyRound, Package, Plus, Save, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";

import {
  createMCPInstallation,
  putMCPInstallationSecrets,
  updateMCPInstallation,
  type CreateMCPInstallationInput,
  type MCPInstallation,
} from "@/api/mcp";
import { FormError } from "@/components/AuthLayout";

interface InstallationWizardProps {
  open: boolean;
  onOpenChange(open: boolean): void;
  organizationId?: number;
  canManageOrganization: boolean;
  onCreated(installation: MCPInstallation): void;
}

export function InstallationWizard({ open, onOpenChange, organizationId, canManageOrganization, onCreated }: InstallationWizardProps) {
  const queryClient = useQueryClient();
  const [step, setStep] = useState<1 | 2>(1);
  const [displayName, setDisplayName] = useState("");
  const [scope, setScope] = useState<CreateMCPInstallationInput["scope"]>("personal");
  const [sourceType, setSourceType] = useState<CreateMCPInstallationInput["source_type"]>("https");
  const [transport, setTransport] = useState<"stdio" | "http" | "streamable_http" | "sse">("streamable_http");
  const [imageRef, setImageRef] = useState("");
  const [endpointUrl, setEndpointUrl] = useState("");
  const [command, setCommand] = useState("");
  const [args, setArgs] = useState("");
  const [allowlist, setAllowlist] = useState("");

  const reset = () => {
    setStep(1); setDisplayName(""); setScope("personal"); setSourceType("https");
    setTransport("streamable_http"); setImageRef(""); setEndpointUrl(""); setCommand(""); setArgs(""); setAllowlist("");
  };
  const mutation = useMutation({
    mutationFn: () => createMCPInstallation(buildInstallationInput({ displayName, scope, sourceType, transport, imageRef, endpointUrl, command, args, allowlist })),
    onSuccess: (installation) => {
      queryClient.setQueryData<MCPInstallation[]>(["organizations", organizationId, "mcp", "installations"], (current = []) => [installation, ...current.filter((item) => item.id !== installation.id)]);
      void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "mcp", "installations"] });
      onCreated(installation); reset(); onOpenChange(false);
    },
  });

  const connectionError = useMemo(() => validateConnection(sourceType, imageRef, endpointUrl), [sourceType, imageRef, endpointUrl]);
  const close = (next: boolean) => { if (!next && !mutation.isPending) { reset(); mutation.reset(); } onOpenChange(next); };

  return <Dialog.Root open={open} onOpenChange={close}><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content mcp-dialog">
    <div className="dialog-header"><div><Dialog.Title>安装 MCP Server</Dialog.Title><span className="mcp-step-label">步骤 {step} / 2</span></div><Dialog.Close className="icon-button" aria-label="关闭"><X size={18} /></Dialog.Close></div>
    <Dialog.Description>{step === 1 ? "选择安装归属与交付方式。" : "配置固定版本的 MCP 连接。"}</Dialog.Description>
    <FormError error={mutation.error} />
    {step === 1 ? <div className="form-stack">
      <label>显示名称<input autoFocus className="field" value={displayName} maxLength={160} onChange={(event) => setDisplayName(event.target.value)} placeholder="例如：GitHub 工具" /></label>
      <label>作用域<div className="segmented" role="group" aria-label="安装作用域"><button className={scope === "personal" ? "active" : ""} onClick={() => setScope("personal")}>个人</button><button className={scope === "organization" ? "active" : ""} disabled={!canManageOrganization} onClick={() => setScope("organization")}>组织</button></div>{!canManageOrganization && <span className="field-hint">组织安装由 owner 或 admin 创建</span>}</label>
      <label>来源<div className="segmented" role="group" aria-label="MCP 来源"><button className={sourceType === "https" ? "active" : ""} onClick={() => { setSourceType("https"); setTransport("streamable_http"); }}><Globe2 size={15} />HTTPS</button><button className={sourceType === "oci" ? "active" : ""} onClick={() => { setSourceType("oci"); setTransport("stdio"); }}><Package size={15} />OCI</button></div></label>
      <div className="mcp-dialog-actions"><button className="button-primary" disabled={!displayName.trim()} onClick={() => setStep(2)}>下一步<ArrowRight size={16} /></button></div>
    </div> : <div className="form-stack">
      {sourceType === "https" ? <>
        <label>HTTPS Endpoint<input autoFocus className="field" type="url" value={endpointUrl} onChange={(event) => setEndpointUrl(event.target.value)} placeholder="https://mcp.example.com/v1" /></label>
        <label>传输协议<select className="field" value={transport} onChange={(event) => setTransport(event.target.value as typeof transport)}><option value="streamable_http">Streamable HTTP</option><option value="http">HTTP</option><option value="sse">SSE</option></select></label>
        <label>允许访问的域名<input className="field" value={allowlist} onChange={(event) => setAllowlist(event.target.value)} placeholder="api.example.com, files.example.com" /><span className="field-hint">逗号分隔，沙箱出口仅限这些域名</span></label>
      </> : <>
        <label>OCI 镜像<input autoFocus className="field" value={imageRef} onChange={(event) => setImageRef(event.target.value)} placeholder="registry.example.com/mcp/server@sha256:..." /><span className="field-hint">必须固定到 64 位 sha256 digest</span></label>
        <label>启动命令<input className="field" value={command} onChange={(event) => setCommand(event.target.value)} placeholder="python,-m,server" /></label>
        <label>参数<input className="field" value={args} onChange={(event) => setArgs(event.target.value)} placeholder="--stdio,--quiet" /></label>
        <label>允许访问的域名<input className="field" value={allowlist} onChange={(event) => setAllowlist(event.target.value)} placeholder="api.example.com" /></label>
      </>}
      {connectionError && <span className="field-error">{connectionError}</span>}
      <div className="mcp-dialog-actions"><button className="button-secondary" onClick={() => setStep(1)}><ArrowLeft size={16} />上一步</button><button className="button-primary" disabled={Boolean(connectionError) || mutation.isPending} onClick={() => mutation.mutate()}><Plus size={16} />创建安装</button></div>
    </div>}
  </Dialog.Content></Dialog.Portal></Dialog.Root>;
}

function buildInstallationInput(values: { displayName: string; scope: CreateMCPInstallationInput["scope"]; sourceType: CreateMCPInstallationInput["source_type"]; transport: "stdio" | "http" | "streamable_http" | "sse"; imageRef: string; endpointUrl: string; command: string; args: string; allowlist: string }): CreateMCPInstallationInput {
  const networkAllowlist = splitValues(values.allowlist);
  if (values.sourceType === "oci") {
    return { display_name: values.displayName.trim(), scope: values.scope, source_type: "oci", transport: "stdio", image_ref: values.imageRef.trim(), command: splitValues(values.command), args: splitValues(values.args), network_allowlist: networkAllowlist };
  }
  return { display_name: values.displayName.trim(), scope: values.scope, source_type: "https", transport: values.transport === "stdio" ? "streamable_http" : values.transport, endpoint_url: values.endpointUrl.trim(), network_allowlist: networkAllowlist };
}

const splitValues = (value: string) => value.split(",").map((item) => item.trim()).filter(Boolean);

function validateConnection(sourceType: CreateMCPInstallationInput["source_type"], imageRef: string, endpointUrl: string) {
  if (sourceType === "oci") return /^\S+@sha256:[a-fA-F0-9]{64}$/.test(imageRef.trim()) ? "" : "请输入固定到 sha256 digest 的 OCI 镜像";
  try {
    const endpoint = new URL(endpointUrl);
    if (endpoint.protocol === "https:" && !endpoint.username && !endpoint.password) return "";
  } catch { /* handled below */ }
  return "请输入不含凭据的 HTTPS 地址";
}

interface SecretRow { id: number; key: string; value: string }

export function MCPSecretsDialog({ installation, open, onOpenChange, organizationId }: { installation: MCPInstallation | null; open: boolean; onOpenChange(open: boolean): void; organizationId?: number }) {
  const queryClient = useQueryClient();
  const [nextId, setNextId] = useState(2);
  const [rows, setRows] = useState<SecretRow[]>([{ id: 1, key: "", value: "" }]);
  const secrets = Object.fromEntries(rows.filter((row) => row.key.trim() && row.value).map((row) => [row.key.trim(), row.value]));
  const mutation = useMutation({
    mutationFn: () => putMCPInstallationSecrets(installation!.id, secrets),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "mcp"] });
      setRows([{ id: 1, key: "", value: "" }]); onOpenChange(false);
    },
  });
  const updateRow = (id: number, field: "key" | "value", value: string) => setRows((current) => current.map((row) => row.id === id ? { ...row, [field]: value } : row));
  const addRow = () => { setRows((current) => [...current, { id: nextId, key: "", value: "" }]); setNextId((value) => value + 1); };
  const changeOpen = (next: boolean) => { if (!next) { setRows([{ id: 1, key: "", value: "" }]); mutation.reset(); } onOpenChange(next); };

  return <Dialog.Root open={open} onOpenChange={changeOpen}><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content">
    <div className="dialog-header"><div><Dialog.Title>管理 Secret</Dialog.Title><span className="mcp-step-label">{installation?.display_name}</span></div><Dialog.Close className="icon-button" aria-label="关闭"><X size={18} /></Dialog.Close></div>
    <Dialog.Description>现有值不会回显；提交同名字段会覆盖远端 Secret。</Dialog.Description>
    <FormError error={mutation.error} />
    <div className="secret-editor">{rows.map((row) => <div className="secret-row" key={row.id}><input className="field" aria-label="Secret 名称" autoComplete="off" placeholder="API_TOKEN" value={row.key} onChange={(event) => updateRow(row.id, "key", event.target.value)} /><input className="field" aria-label="Secret 值" type="password" autoComplete="new-password" placeholder="Secret value" value={row.value} onChange={(event) => updateRow(row.id, "value", event.target.value)} /><button className="icon-button text-danger" aria-label="移除 Secret 字段" disabled={rows.length === 1} onClick={() => setRows((current) => current.filter((item) => item.id !== row.id))}><Trash2 size={16} /></button></div>)}</div>
    <div className="mcp-dialog-actions"><button className="button-secondary" onClick={addRow}><Plus size={16} />增加字段</button><button className="button-primary" disabled={!Object.keys(secrets).length || mutation.isPending} onClick={() => mutation.mutate()}><KeyRound size={16} />安全存储</button></div>
  </Dialog.Content></Dialog.Portal></Dialog.Root>;
}

export function RenameInstallationDialog({ installation, open, onOpenChange, organizationId }: { installation: MCPInstallation | null; open: boolean; onOpenChange(open: boolean): void; organizationId?: number }) {
  const queryClient = useQueryClient(); const [name, setName] = useState(installation?.display_name ?? "");
  const mutation = useMutation({ mutationFn: () => updateMCPInstallation(installation!.id, { display_name: name.trim() }), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "mcp"] }); onOpenChange(false); } });
  const changeOpen = (next: boolean) => { if (!next) { setName(installation?.display_name ?? ""); mutation.reset(); } onOpenChange(next); };
  return <Dialog.Root open={open} onOpenChange={changeOpen}><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>重命名安装</Dialog.Title><Dialog.Close className="icon-button" aria-label="关闭"><X size={18} /></Dialog.Close></div><Dialog.Description>连接配置和当前 revision 不会改变。</Dialog.Description><FormError error={mutation.error} /><div className="form-stack"><label>显示名称<input autoFocus className="field" value={name} maxLength={160} onChange={(event) => setName(event.target.value)} /></label><button className="button-primary" disabled={!name.trim() || mutation.isPending} onClick={() => mutation.mutate()}><Save size={16} />保存</button></div></Dialog.Content></Dialog.Portal></Dialog.Root>;
}
