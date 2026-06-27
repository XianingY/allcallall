import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "@/components/AppShell";
import { PlaceholderPage } from "@/pages/PlaceholderPage";

const pages: Array<[string, string]> = [
  ["inbox", "协作 Inbox"], ["meetings", "会议"], ["agent-lab", "Agent Lab"],
  ["knowledge", "知识库"], ["contacts", "联系人"], ["deals", "商机"],
  ["recordings", "录音转写"], ["follow-ups", "跟进"], ["organizations", "组织"], ["settings", "设置"],
];

export function App() {
  return <Routes><Route element={<AppShell />}>{pages.map(([path, title]) => <Route key={path} path={`/${path}`} element={<PlaceholderPage title={title} />} />)}<Route index element={<Navigate to="/inbox" replace />} /></Route><Route path="*" element={<Navigate to="/inbox" replace />} /></Routes>;
}
