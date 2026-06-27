import { Navigate, Route, Routes } from "react-router-dom";
import { AnonymousRoute, ProtectedRoute } from "@/auth/ProtectedRoute";
import { AppShell } from "@/components/AppShell";
import { OrganizationsPage } from "@/pages/OrganizationsPage";
import { PlaceholderPage } from "@/pages/PlaceholderPage";
import { ForgotPasswordPage } from "@/pages/auth/ForgotPasswordPage";
import { InvitePage } from "@/pages/auth/InvitePage";
import { LoginPage } from "@/pages/auth/LoginPage";
import { RegisterPage } from "@/pages/auth/RegisterPage";
import { VerifyEmailPage } from "@/pages/auth/VerifyEmailPage";
import { SettingsLayout } from "@/pages/settings/SettingsLayout";
import { BlockedSettingsPage, DangerSettingsPage, LegalSettingsPage, PasswordSettingsPage, ProfileSettingsPage, SessionsSettingsPage } from "@/pages/settings/SettingsPages";

const pages: Array<[string, string]> = [
  ["inbox", "协作 Inbox"], ["meetings", "会议"], ["agent-lab", "Agent Lab"],
  ["knowledge", "知识库"], ["contacts", "联系人"], ["deals", "商机"],
  ["recordings", "录音转写"], ["follow-ups", "跟进"],
];

export function App() {
  return <Routes>
    <Route element={<AnonymousRoute />}>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/verify-email" element={<VerifyEmailPage />} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
    </Route>
    <Route path="/invite/:code" element={<InvitePage />} />
    <Route element={<ProtectedRoute />}>
      <Route element={<AppShell />}>
        {pages.map(([path, title]) => <Route key={path} path={`/${path}`} element={<PlaceholderPage title={title} />} />)}
        <Route path="/organizations" element={<OrganizationsPage />} />
        <Route path="/settings" element={<SettingsLayout />}>
          <Route index element={<Navigate to="profile" replace />} />
          <Route path="profile" element={<ProfileSettingsPage />} />
          <Route path="password" element={<PasswordSettingsPage />} />
          <Route path="sessions" element={<SessionsSettingsPage />} />
          <Route path="blocked" element={<BlockedSettingsPage />} />
          <Route path="legal" element={<LegalSettingsPage />} />
          <Route path="danger" element={<DangerSettingsPage />} />
        </Route>
        <Route index element={<Navigate to="/inbox" replace />} />
      </Route>
    </Route>
    <Route path="*" element={<Navigate to="/inbox" replace />} />
  </Routes>;
}
