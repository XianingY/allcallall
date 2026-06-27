import { Ban, FileText, KeyRound, MonitorSmartphone, ShieldAlert, UserRound } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import clsx from "clsx";

const links = [
  ["/settings/profile", "账号", UserRound], ["/settings/password", "密码", KeyRound],
  ["/settings/sessions", "登录设备", MonitorSmartphone], ["/settings/blocked", "黑名单", Ban],
  ["/settings/legal", "法律信息", FileText], ["/settings/danger", "账号删除", ShieldAlert],
] as const;

export function SettingsLayout() {
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Account</p><h1>设置</h1><p>管理账号、安全和隐私。</p></div></header><div className="settings-layout"><nav className="settings-nav" aria-label="设置导航">{links.map(([to, label, Icon]) => <NavLink key={to} to={to} className={({ isActive }) => clsx("settings-link", isActive && "settings-link-active")}><Icon size={17} />{label}</NavLink>)}</nav><section className="settings-content"><Outlet /></section></div></div>;
}

