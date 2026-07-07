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
import { ErrorBoundary } from "@/components/ErrorBoundary";

import { LazyLoad } from "@/components/LazyLoad";

export function App() {
  return (
    <ErrorBoundary>
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
              <Route path="/agent-lab" element={<LazyLoad loader={() => import("@/pages/agent/AgentLabPage").then(m => ({ default: m.AgentLabPage }))} fallback={<PageLoading />} />} />
              <Route path="/knowledge" element={<LazyLoad loader={() => import("@/pages/knowledge/KnowledgePage").then(m => ({ default: m.KnowledgePage }))} fallback={<PageLoading />} />} />
              <Route path="/inbox" element={<InboxPage />} />
              <Route path="/conversations/:conversationId" element={<InboxPage />} />
              <Route path="/contacts" element={<LazyLoad loader={() => import("@/pages/collaboration/ContactsPage").then(m => ({ default: m.ContactsPage }))} fallback={<PageLoading />} />} />
              <Route path="/follow-ups" element={<LazyLoad loader={() => import("@/pages/collaboration/FollowUpsPage").then(m => ({ default: m.FollowUpsPage }))} fallback={<PageLoading />} />} />
              <Route path="/calls" element={<LazyLoad loader={() => import("@/pages/collaboration/CallHistoryPage").then(m => ({ default: m.CallHistoryPage }))} fallback={<PageLoading />} />} />
              <Route path="/deals" element={<LazyLoad loader={() => import("@/pages/collaboration/DealsPage").then(m => ({ default: m.DealsPage }))} fallback={<PageLoading />} />} />
              <Route path="/deals/:dealId" element={<LazyLoad loader={() => import("@/pages/collaboration/DealDetailPage").then(m => ({ default: m.DealDetailPage }))} fallback={<PageLoading />} />} />
              <Route path="/meetings" element={<LazyLoad loader={() => import("@/pages/meetings/MeetingsPage").then(m => ({ default: m.MeetingsPage }))} fallback={<PageLoading />} />} />
              <Route path="/recordings" element={<LazyLoad loader={() => import("@/pages/recordings/RecordingsPage").then(m => ({ default: m.RecordingsPage }))} fallback={<PageLoading />} />} />
              <Route path="/recordings/:recordingId" element={<LazyLoad loader={() => import("@/pages/recordings/RecordingTranscriptPage").then(m => ({ default: m.RecordingTranscriptPage }))} fallback={<PageLoading />} />} />
              <Route path="/organizations" element={<OrganizationsPage />} />
              <Route path="/settings" element={<SettingsLayout />}>
                <Route index element={<Navigate to="profile" replace />} />
                <Route path="profile" element={<ProfileSettingsPage />} />
                <Route path="password" element={<PasswordSettingsPage />} />
                <Route path="sessions" element={<SessionsSettingsPage />} />
                <Route path="blocked" element={<BlockedSettingsPage />} />
                <Route path="notifications" element={<NotificationSettingsPage />} />
                <Route path="billing" element={<LazyLoad loader={() => import("@/pages/settings/BillingSettingsPage").then(m => ({ default: m.BillingSettingsPage }))} fallback={<PageLoading />} />} />
                <Route path="preferences" element={<PreferencesSettingsPage />} />
                <Route path="legal" element={<LegalSettingsPage />} />
                <Route path="danger" element={<DangerSettingsPage />} />
              </Route>
              <Route index element={<Navigate to="/inbox" replace />} />
            </Route>
            <Route path="/meetings/:roomId/preflight" element={<LazyLoad loader={() => import("@/pages/meetings/MeetingPreflightPage").then(m => ({ default: m.MeetingPreflightPage }))} fallback={<PageLoading />} />} />
            <Route path="/meetings/:roomId" element={<LazyLoad loader={() => import("@/pages/meetings/MeetingRoomPage").then(m => ({ default: m.MeetingRoomPage }))} fallback={<PageLoading />} />} />
          </Route>
          <Route path="*" element={<Navigate to="/inbox" replace />} />
        </Routes>
    </ErrorBoundary>
  );
}
