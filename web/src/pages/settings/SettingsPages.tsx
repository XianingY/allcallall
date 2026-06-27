import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CreditCard, ExternalLink, Globe2, LogOut, Trash2, Unlock } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { acceptLegal, changePassword, deleteAccount, getLegal, listBlocks, listSessions, revokeSession, unblockUser } from "@/api/identity";
import { deletePushDevice, getEntitlements, getUsage, listPushDevices } from "@/api/platform";
import { useAuth } from "@/auth/AuthProvider";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { isBillingConfigured, openRevenueCatCheckout, openRevenueCatPortal } from "@/platform/billing";
import { clearStoredPushDeviceId, deleteBrowserPushToken, getStoredPushDeviceId, isPushConfigured, registerBrowserPush } from "@/platform/push";

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

export function NotificationSettingsPage() {
  const { user } = useAuth(); const queryClient = useQueryClient();
  const devices = useQuery({ queryKey: ["account", "push-devices"], queryFn: listPushDevices });
  const enable = useMutation({ mutationFn: async () => { if (!user) throw new Error("not signed in"); return registerBrowserPush(); }, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["account", "push-devices"] }) });
  const remove = useMutation({ mutationFn: deletePushDevice, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["account", "push-devices"] }) });
  const disableCurrent = useMutation({ mutationFn: async () => { const id = getStoredPushDeviceId(); if (id) await deletePushDevice(id); await deleteBrowserPushToken(); clearStoredPushDeviceId(); }, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["account", "push-devices"] }) });
  const permission = typeof Notification === "undefined" ? "unsupported" : Notification.permission;
  return <SettingsPanel title="浏览器通知" description="用于后台来电提醒和关键协作事件。"><FormError error={enable.error || disableCurrent.error || remove.error} />{enable.isSuccess && <div className="status-success">当前浏览器已注册通知。</div>}<div className="settings-grid"><article className="settings-card"><Bell size={20} /><div><strong>当前浏览器</strong><p>{isPushConfigured() ? `权限状态：${permission}` : "Firebase Web Push 未配置"}</p></div><div className="button-row"><button className="button-primary" disabled={!isPushConfigured() || enable.isPending} onClick={() => enable.mutate()}>启用通知</button><button className="button-secondary" disabled={disableCurrent.isPending} onClick={() => disableCurrent.mutate()}>注销本机</button></div></article></div>{devices.isLoading ? <PageLoading /> : devices.isError ? <PageError error={devices.error} /> : <div className="list-stack mt-4">{devices.data?.length ? devices.data.map((device) => <div className="data-row" key={device.id}><div><strong>{device.platform} · {device.provider}</strong><small>{device.device_name || "未命名设备"}</small><small>最近注册 {dateTime(device.last_registered)}</small></div><button className="icon-button" title="注销设备" aria-label="注销设备" onClick={() => remove.mutate(device.id)}><Trash2 size={17} /></button></div>) : <div className="inline-empty">暂无注册设备</div>}</div>}</SettingsPanel>;
}

export function BillingSettingsPage() {
  const { user } = useAuth(); const queryClient = useQueryClient();
  const entitlements = useQuery({ queryKey: ["billing", "entitlements"], queryFn: getEntitlements });
  const usage = useQuery({ queryKey: ["billing", "usage"], queryFn: getUsage });
  const purchase = useMutation({ mutationFn: async () => { if (!user) throw new Error("not signed in"); return openRevenueCatCheckout(user); }, onSuccess: () => queryClient.invalidateQueries({ queryKey: ["billing"] }) });
  const portal = useMutation({ mutationFn: async () => { if (!user) throw new Error("not signed in"); return openRevenueCatPortal(user); } });
  return <SettingsPanel title="订阅与用量" description="后端 entitlement 是最终权限来源，RevenueCat 只负责 Web Billing 入口。"><FormError error={purchase.error || portal.error} />{entitlements.isLoading ? <PageLoading /> : entitlements.isError ? <PageError error={entitlements.error} /> : <div className="billing-summary"><div><span>当前版本</span><strong>{entitlements.data?.tier === "premium" ? "Premium" : "Free"}</strong></div><div className="button-row"><button className="button-primary" disabled={!isBillingConfigured() || purchase.isPending} onClick={() => purchase.mutate()}><CreditCard size={17} />升级</button><button className="button-secondary" disabled={!isBillingConfigured() || portal.isPending} onClick={() => portal.mutate()}>管理订阅</button></div>{!isBillingConfigured() && <p className="text-muted">未配置 RevenueCat public API key，生产环境通过 runtime config 注入。</p>}</div>}<section className="settings-section"><h3>权益</h3>{entitlements.data?.entitlements.length ? entitlements.data.entitlements.map((item) => <div className="data-row" key={item.id}><div><strong>{item.entitlement}</strong><small>{item.tier} · {item.status} · {item.source}</small>{item.expires_at && <small>到期 {dateTime(item.expires_at)}</small>}</div></div>) : <div className="inline-empty">暂无付费权益</div>}</section><section className="settings-section"><h3>用量</h3>{usage.isLoading ? <PageLoading /> : usage.isError ? <PageError error={usage.error} /> : usage.data?.map((item) => <div className="data-row" key={`${item.feature}-${item.period_key}`}><div><strong>{item.feature}</strong><small>{item.period_key} · {item.used_units}/{item.unlimited ? "无限" : item.limit_units} {item.unit}</small></div><span>{item.unlimited ? "unlimited" : `剩余 ${item.remaining_units}`}</span></div>)}</section></SettingsPanel>;
}

export function PreferencesSettingsPage() {
  const { i18n } = useTranslation(); const [language, setLanguage] = useState(i18n.language.startsWith("en") ? "en" : "zh");
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return <SettingsPanel title="偏好设置" description="本地浏览器偏好，不影响其他设备。"><div className="form-stack max-w-lg"><label><Globe2 size={17} />界面语言<select className="field" value={language} onChange={(event) => { const next = event.target.value; setLanguage(next); localStorage.setItem("allcallall.language", next); void i18n.changeLanguage(next); }}><option value="zh">中文</option><option value="en">English</option></select></label><dl className="detail-list"><div><dt>浏览器时区</dt><dd>{timezone}</dd></div><div><dt>时间显示</dt><dd>{dateTime(new Date().toISOString())}</dd></div></dl></div></SettingsPanel>;
}

export function DangerSettingsPage() {
  const { user, logout } = useAuth(); const navigate = useNavigate(); const [confirm, setConfirm] = useState(""); const [password, setPassword] = useState("");
  const mutation = useMutation({ mutationFn: () => deleteAccount({ password }), onSuccess: async () => { await logout(); navigate("/login", { replace: true }); } });
  return <SettingsPanel title="删除账号" description="此操作会去标识化账号并清理可识别数据，无法撤销。"><div className="danger-zone"><p>输入 <strong>{user?.email}</strong> 并提供当前密码以确认。</p><FormError error={mutation.error} /><label>确认邮箱<input className="field" value={confirm} onChange={(event) => setConfirm(event.target.value)} /></label><label>当前密码<input className="field" type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label><button className="button-danger" disabled={confirm !== user?.email || !password} onClick={() => mutation.mutate()}><Trash2 size={17} />永久删除账号</button></div></SettingsPanel>;
}

function SettingsPanel({ title, description, children }: { title: string; description: string; children: React.ReactNode }) { return <div className="panel panel-body settings-panel"><header><h2>{title}</h2><p>{description}</p></header>{children}</div>; }
