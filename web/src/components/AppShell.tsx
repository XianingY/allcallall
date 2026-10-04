import {
  AlertCircle,
  Blocks,
  Bot,
  Building2,
  CalendarDays,
  ContactRound,
  FileAudio,
  Inbox,
  ListTodo,
  BookOpen,
  LogOut,
  Menu,
  PhoneCall,
  Settings,
  Target,
  X,
} from "lucide-react";
import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import clsx from "clsx";

import { useAuth } from "@/auth/AuthContext";
import { CallOverlay } from "@/calls/CallOverlay";
import { SignalTrack } from "@/components/SignalTrack";
import { useOrganization } from "@/organizations/OrganizationContext";
import { PushNotificationBridge } from "@/platform/PushNotificationBridge";
import { useChatConnected } from "@/realtime/ChatRealtimeContext";

const navGroups = [
  {
    label: "nav.groups.collaboration",
    items: [
      ["/inbox", "nav.inbox", Inbox],
      ["/meetings", "nav.meetings", CalendarDays],
      ["/contacts", "nav.contacts", ContactRound],
      ["/deals", "nav.deals", Target],
      ["/follow-ups", "nav.followups", ListTodo],
      ["/calls", "nav.callHistory", PhoneCall],
    ],
  },
  {
    label: "nav.groups.intelligence",
    items: [
      ["/agent-lab", "nav.agent", Bot],
      ["/agent-tools", "nav.agentTools", Blocks],
      ["/knowledge", "nav.knowledge", BookOpen],
      ["/recordings", "nav.recordings", FileAudio],
    ],
  },
  {
    label: "nav.groups.administration",
    items: [
      ["/organizations", "nav.organizations", Building2],
      ["/settings", "nav.settings", Settings],
    ],
  },
] as const;

export function AppShell() {
  const [open, setOpen] = useState(false);
  const [switchError, setSwitchError] = useState<unknown>();
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const {
    organizations,
    activeOrganization,
    select,
    error: organizationsError,
    retry: retryOrganizations,
  } = useOrganization();
  const chatConnected = useChatConnected();

  const organizationPicker = organizationsError ? (
    <p className="nav-group-label">组织列表加载失败</p>
  ) : organizations.length === 0 ? (
    <p className="nav-group-label">暂无组织</p>
  ) : (
    <>
      <label className="workspace-picker">
        <span>当前组织</span>
        <select
          aria-label="当前组织"
          value={activeOrganization?.id ?? ""}
          onChange={(event) => {
            setSwitchError(undefined);
            void select(Number(event.target.value)).catch((caught) =>
              setSwitchError(caught),
            );
          }}
        >
          {organizations.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </select>
      </label>
      {switchError ? (
        <p className="nav-group-label" role="alert">
          切换组织失败：{switchError instanceof Error ? switchError.message : String(switchError)}
        </p>
      ) : null}
    </>
  );

  return (
    <div className="app-shell">
      <header className="app-shell-header">
        <button className="icon-button" aria-label="打开导航" onClick={() => setOpen(true)}>
          <Menu size={20} />
        </button>
        <strong className="ml-3">AllCallAll</strong>
      </header>

      {open ? (
        <button
          className="nav-drawer-overlay"
          aria-label="关闭导航"
          onClick={() => setOpen(false)}
        />
      ) : null}

      <aside className={clsx("app-rail", open && "app-rail-open")}>
        <div className="app-rail-brand">
          <div className="app-rail-brand-row">
            <div>
              <strong>AllCallAll</strong>
              <small>{t("brand.tagline")}</small>
            </div>
            <SignalTrack
              compact
              state={chatConnected ? "connected" : "connecting"}
              label={chatConnected ? t("signal.connected") : t("signal.connecting")}
            />
          </div>
          <button
            className="icon-button app-rail-close"
            aria-label="关闭导航"
            onClick={() => setOpen(false)}
          >
            <X size={19} />
          </button>
        </div>

        <nav className="app-rail-nav" aria-label="主导航">
          {navGroups.map((group) => (
            <div key={group.label} className="nav-group" role="group" aria-label={t(group.label)}>
              {group.label === "nav.groups.administration" ? organizationPicker : null}
              <p className="nav-group-label">{t(group.label)}</p>
              {group.items.map(([to, label, Icon]) => (
                <NavLink
                  key={to}
                  to={to}
                  onClick={() => setOpen(false)}
                  className={({ isActive }) => clsx("nav-link", isActive && "nav-link-active")}
                >
                  <Icon size={18} />
                  <span>{label.startsWith("nav.") ? t(label) : label}</span>
                </NavLink>
              ))}
            </div>
          ))}
        </nav>

        <div className="sidebar-account">
          <div className="account-avatar">
            {user?.display_name.slice(0, 1).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <strong>{user?.display_name}</strong>
            <span>{user?.email}</span>
          </div>
          <button
            className="icon-button"
            title="退出"
            aria-label="退出"
            onClick={() => void logout()}
          >
            <LogOut size={17} />
          </button>
        </div>
      </aside>

      <main className="app-shell-main">
        {organizationsError ? (
          <div className="page-state text-danger" role="alert">
            <AlertCircle size={18} />
            <span>组织信息加载失败：{organizationsError.message}</span>
            <button className="button-secondary" onClick={retryOrganizations}>
              重试
            </button>
          </div>
        ) : null}
        <Outlet />
      </main>

      <CallOverlay />
      <PushNotificationBridge />
    </div>
  );
}
