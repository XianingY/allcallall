import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, LogOut, Trash2, Unlock } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { acceptLegal, changePassword, deleteAccount, getLegal, listBlocks, listSessions, revokeSession, unblockUser } from "@/api/identity";
import { useAuth } from "@/auth/AuthProvider";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";

const dateTime = (value?: string | null) => value ? new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "-";

export function ProfileSettingsPage() {
  const { user, logout } = useAuth();
  return <SettingsPanel title="账号信息" description="Access token 仅保存在当前页面内存中，刷新会通过 HttpOnly Cookie 恢复。"><dl className="detail-list"><div><dt>显示名称</dt><dd>{user?.display_name}</dd></div><div><dt>邮箱</dt><dd>{user?.email}</dd></div><div><dt>用户 ID</dt><dd>{user?.id}</dd></div></dl><button className="button-secondary" onClick={() => void logout()}><LogOut size={17} />退出当前设备</button></SettingsPanel>;
}

export function PasswordSettingsPage() {
  const [oldPassword, setOldPassword] = useState(""); const [newPassword, setNewPassword] = useState(""); const [confirm, setConfirm] = useState("");
  const mutation = useMutation({ mutationFn: () => changePassword(oldPassword, newPassword, confirm), onSuccess: () => { setOldPassword(""); setNewPassword(""); setConfirm(""); } });
  return <SettingsPanel title="修改密码" description="更新后其他 refresh session 会被撤销。"><form className="form-stack max-w-lg" onSubmit={(event) => { event.preventDefault(); mutation.mutate(); }}><FormError error={mutation.error} />{mutation.isSuccess && <div className="status-success">密码已更新</div>}<label>当前密码<input className="field" type="password" value={oldPassword} onChange={(event) => setOldPassword(event.target.value)} /></label><label>新密码<input className="field" type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /></label><label>确认新密码<input className="field" type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} /></label><button className="button-primary">更新密码</button></form></SettingsPanel>;
}

export function SessionsSettingsPage() {
  const queryClient = useQueryClient(); const { logout } = useAuth();
  const query = useQuery({ queryKey: ["account", "sessions"], queryFn: listSessions });
  const revoke = useMutation({ mutationFn: revokeSession, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["account", "sessions"] }) });
  return <SettingsPanel title="登录设备" description="移除不再使用或不认识的会话。"><FormError error={revoke.error} />{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} retry={() => void query.refetch()} /> : <div className="list-stack">{query.data?.map((session) => <div className="data-row" key={session.id}><div><strong>{session.current ? "当前设备" : session.user_agent || "未知设备"}</strong><small>{session.ip_address} · 最近使用 {dateTime(session.last_used_at ?? session.created_at)}</small><small>到期 {dateTime(session.expires_at)} · {session.status}</small></div><button className="icon-button" title="撤销会话" onClick={() => session.current ? void logout() : revoke.mutate(session.id)}><Trash2 size={17} /></button></div>)}</div>}</SettingsPanel>;
}

export function BlockedSettingsPage() {
  const queryClient = useQueryClient(); const query = useQuery({ queryKey: ["account", "blocks"], queryFn: listBlocks });
  const unblock = useMutation({ mutationFn: unblockUser, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["account", "blocks"] }) });
  return <SettingsPanel title="黑名单" description="被解除屏蔽的用户可以重新与你协作。">{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} /> : query.data?.length ? <div className="list-stack">{query.data.map((item) => <div className="data-row" key={item.id}><div><strong>{item.blocked_user_display_name || `用户 ${item.blocked_user_id}`}</strong><small>{item.blocked_user_email || "邮箱不可用"}</small></div><button className="button-secondary" onClick={() => unblock.mutate(item.blocked_user_id)}><Unlock size={16} />解除</button></div>)}</div> : <div className="inline-empty">暂无已屏蔽用户</div>}</SettingsPanel>;
}

export function LegalSettingsPage() {
  const legal = useQuery({ queryKey: ["legal"], queryFn: getLegal }); const accept = useMutation({ mutationFn: acceptLegal });
  return <SettingsPanel title="法律信息" description="查看当前条款版本并记录接受状态。">{legal.isLoading ? <PageLoading /> : legal.isError ? <PageError error={legal.error} /> : <div className="form-stack"><dl className="detail-list"><div><dt>服务条款</dt><dd>版本 {legal.data?.terms_version}</dd></div><div><dt>隐私政策</dt><dd>版本 {legal.data?.privacy_version}</dd></div><div><dt>支持邮箱</dt><dd>{legal.data?.support_email}</dd></div></dl><div className="button-row"><a className="button-secondary" href={legal.data?.terms_url} target="_blank" rel="noreferrer">条款 <ExternalLink size={15} /></a><a className="button-secondary" href={legal.data?.privacy_policy_url} target="_blank" rel="noreferrer">隐私政策 <ExternalLink size={15} /></a><button className="button-primary" onClick={() => accept.mutate()}>接受当前版本</button></div>{accept.isSuccess && <div className="status-success">已记录接受状态</div>}</div>}</SettingsPanel>;
}

export function DangerSettingsPage() {
  const { user, logout } = useAuth(); const navigate = useNavigate(); const [confirm, setConfirm] = useState(""); const [password, setPassword] = useState("");
  const mutation = useMutation({ mutationFn: () => deleteAccount({ password }), onSuccess: async () => { await logout(); navigate("/login", { replace: true }); } });
  return <SettingsPanel title="删除账号" description="此操作会去标识化账号并清理可识别数据，无法撤销。"><div className="danger-zone"><p>输入 <strong>{user?.email}</strong> 并提供当前密码以确认。</p><FormError error={mutation.error} /><label>确认邮箱<input className="field" value={confirm} onChange={(event) => setConfirm(event.target.value)} /></label><label>当前密码<input className="field" type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label><button className="button-danger" disabled={confirm !== user?.email || !password} onClick={() => mutation.mutate()}><Trash2 size={17} />永久删除账号</button></div></SettingsPanel>;
}

function SettingsPanel({ title, description, children }: { title: string; description: string; children: React.ReactNode }) { return <div className="panel panel-body settings-panel"><header><h2>{title}</h2><p>{description}</p></header>{children}</div>; }
