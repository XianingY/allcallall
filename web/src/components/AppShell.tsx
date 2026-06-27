import { useState } from "react";
import { Bot, Building2, CalendarDays, ContactRound, FileAudio, Inbox, Menu, Settings, Target, X, ListTodo, BookOpen } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import clsx from "clsx";

const nav = [
  ["/inbox", "nav.inbox", Inbox], ["/meetings", "nav.meetings", CalendarDays],
  ["/agent-lab", "nav.agent", Bot], ["/knowledge", "nav.knowledge", BookOpen],
  ["/contacts", "nav.contacts", ContactRound], ["/deals", "nav.deals", Target],
  ["/recordings", "nav.recordings", FileAudio], ["/follow-ups", "nav.followups", ListTodo],
  ["/organizations", "组织", Building2], ["/settings", "nav.settings", Settings],
] as const;

export function AppShell() {
  const [open, setOpen] = useState(false);
  const { t } = useTranslation();
  return (
    <div className="min-h-screen bg-canvas text-ink">
      <header className="fixed inset-x-0 top-0 z-30 flex h-14 items-center border-b border-line bg-panel px-4 lg:hidden">
        <button className="icon-button" aria-label="打开导航" onClick={() => setOpen(true)}><Menu size={20} /></button>
        <span className="ml-3 font-semibold">AllCallAll</span>
      </header>
      {open && <button className="fixed inset-0 z-30 bg-black/30 lg:hidden" aria-label="关闭导航" onClick={() => setOpen(false)} />}
      <aside className={clsx("fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-line bg-panel transition-transform lg:translate-x-0", open ? "translate-x-0" : "-translate-x-full")}>
        <div className="flex h-16 items-center justify-between border-b border-line px-5">
          <div><div className="text-lg font-bold">AllCallAll</div><div className="text-xs text-muted">{t("brand.tagline")}</div></div>
          <button className="icon-button lg:hidden" aria-label="关闭导航" onClick={() => setOpen(false)}><X size={19} /></button>
        </div>
        <nav className="flex-1 space-y-1 overflow-y-auto p-3" aria-label="主导航">
          {nav.map(([to, label, Icon]) => <NavLink key={to} to={to} onClick={() => setOpen(false)} className={({ isActive }) => clsx("nav-link", isActive && "nav-link-active")}><Icon size={18} /><span>{label.startsWith("nav.") ? t(label) : label}</span></NavLink>)}
        </nav>
      </aside>
      <main className="min-h-screen pt-14 lg:ml-60 lg:pt-0"><Outlet /></main>
    </div>
  );
}
