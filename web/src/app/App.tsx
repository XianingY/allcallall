import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { AnonymousRoute, ProtectedRoute } from "@/auth/ProtectedRoute";
import { AppShell } from "@/components/AppShell";
import { PageLoading } from "@/components/PageState";
import { OrganizationsPage } from "@/pages/OrganizationsPage";
import { ForgotPasswordPage } from "@/pages/auth/ForgotPasswordPage";
import { InvitePage } from "@/pages/auth/InvitePage";
import { LoginPage } from "@/pages/auth/LoginPage";
import { RegisterPage } from "@/pages/auth/RegisterPage";
import { VerifyEmailPage } from "@/pages/auth/VerifyEmailPage";
import { SettingsLayout } from "@/pages/settings/SettingsLayout";
import { BlockedSettingsPage, DangerSettingsPage, LegalSettingsPage, NotificationSettingsPage, PasswordSettingsPage, PreferencesSettingsPage, ProfileSettingsPage, SessionsSettingsPage } from "@/pages/settings/SettingsPages";
import { InboxPage } from "@/pages/collaboration/InboxPage";

const AgentLabPage = lazy(() => import("@/pages/agent/AgentLabPage").then((module) => ({ default: module.AgentLabPage })));
const KnowledgePage = lazy(() => import("@/pages/knowledge/KnowledgePage").then((module) => ({ default: module.KnowledgePage })));
const CallHistoryPage = lazy(() => import("@/pages/collaboration/CallHistoryPage").then((module) => ({ default: module.CallHistoryPage })));
const ContactsPage = lazy(() => import("@/pages/collaboration/ContactsPage").then((module) => ({ default: module.ContactsPage })));
const DealDetailPage = lazy(() => import("@/pages/collaboration/DealDetailPage").then((module) => ({ default: module.DealDetailPage })));
const DealsPage = lazy(() => import("@/pages/collaboration/DealsPage").then((module) => ({ default: module.DealsPage })));
const FollowUpsPage = lazy(() => import("@/pages/collaboration/FollowUpsPage").then((module) => ({ default: module.FollowUpsPage })));
const MeetingsPage = lazy(() => import("@/pages/meetings/MeetingsPage").then((module) => ({ default: module.MeetingsPage })));
const MeetingPreflightPage = lazy(() => import("@/pages/meetings/MeetingPreflightPage").then((module) => ({ default: module.MeetingPreflightPage })));
const MeetingRoomPage = lazy(() => import("@/pages/meetings/MeetingRoomPage").then((module) => ({ default: module.MeetingRoomPage })));
const RecordingsPage = lazy(() => import("@/pages/recordings/RecordingsPage").then((module) => ({ default: module.RecordingsPage })));
const RecordingTranscriptPage = lazy(() => import("@/pages/recordings/RecordingTranscriptPage").then((module) => ({ default: module.RecordingTranscriptPage })));
const BillingSettingsPage = lazy(() => import("@/pages/settings/BillingSettingsPage").then((module) => ({ default: module.BillingSettingsPage })));

export function App() {
  return <Suspense fallback={<PageLoading />}>
    <Routes>
      <Route element={<AnonymousRoute />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/verify-email" element={<VerifyEmailPage />} />
        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      </Route>
      <Route path="/invite/:code" element={<InvitePage />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route path="/agent-lab" element={<AgentLabPage />} />
          <Route path="/knowledge" element={<KnowledgePage />} />
          <Route path="/inbox" element={<InboxPage />} />
          <Route path="/conversations/:conversationId" element={<InboxPage />} />
          <Route path="/contacts" element={<ContactsPage />} />
          <Route path="/follow-ups" element={<FollowUpsPage />} />
          <Route path="/calls" element={<CallHistoryPage />} />
          <Route path="/deals" element={<DealsPage />} />
          <Route path="/deals/:dealId" element={<DealDetailPage />} />
          <Route path="/meetings" element={<MeetingsPage />} />
          <Route path="/recordings" element={<RecordingsPage />} />
          <Route path="/recordings/:recordingId" element={<RecordingTranscriptPage />} />
          <Route path="/organizations" element={<OrganizationsPage />} />
          <Route path="/settings" element={<SettingsLayout />}>
            <Route index element={<Navigate to="profile" replace />} />
            <Route path="profile" element={<ProfileSettingsPage />} />
            <Route path="password" element={<PasswordSettingsPage />} />
            <Route path="sessions" element={<SessionsSettingsPage />} />
            <Route path="blocked" element={<BlockedSettingsPage />} />
            <Route path="notifications" element={<NotificationSettingsPage />} />
            <Route path="billing" element={<BillingSettingsPage />} />
            <Route path="preferences" element={<PreferencesSettingsPage />} />
            <Route path="legal" element={<LegalSettingsPage />} />
            <Route path="danger" element={<DangerSettingsPage />} />
          </Route>
          <Route index element={<Navigate to="/inbox" replace />} />
        </Route>
        <Route path="/meetings/:roomId/preflight" element={<MeetingPreflightPage />} />
        <Route path="/meetings/:roomId" element={<MeetingRoomPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/inbox" replace />} />
    </Routes>
  </Suspense>;
}
