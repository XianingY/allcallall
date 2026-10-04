import { Navigate, Route } from "react-router-dom";

import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { AppShell } from "@/components/AppShell";
import { LazyLoad } from "@/components/LazyLoad";
import { PageLoading } from "@/components/PageState";
import { OrganizationsPage } from "@/pages/OrganizationsPage";
import { InboxPage } from "@/pages/collaboration/InboxPage";
import { SettingsLayout } from "@/pages/settings/SettingsLayout";
import {
  BlockedSettingsPage,
  DangerSettingsPage,
  LegalSettingsPage,
  NotificationSettingsPage,
  PasswordSettingsPage,
  PreferencesSettingsPage,
  ProfileSettingsPage,
  SessionsSettingsPage,
} from "@/pages/settings/SettingsPages";

const loading = <PageLoading />;

export function workspaceRoutes() {
  return (
    <Route element={<ProtectedRoute />}>
      <Route element={<AppShell />}>
        <Route
          path="/agent-lab"
          element={<LazyLoad loader={() => import("@/pages/agent/AgentLabPage").then((module) => ({ default: module.AgentLabPage }))} fallback={loading} />}
        />
        <Route
          path="/agent-tools"
          element={<LazyLoad loader={() => import("@/pages/mcp/MCPPlatformPage").then((module) => ({ default: module.MCPPlatformPage }))} fallback={loading} />}
        />
        <Route
          path="/knowledge"
          element={<LazyLoad loader={() => import("@/pages/knowledge/KnowledgePage").then((module) => ({ default: module.KnowledgePage }))} fallback={loading} />}
        />
        <Route path="/inbox" element={<InboxPage />} />
        <Route path="/conversations/:conversationId" element={<InboxPage />} />
        <Route
          path="/contacts"
          element={<LazyLoad loader={() => import("@/pages/collaboration/ContactsPage").then((module) => ({ default: module.ContactsPage }))} fallback={loading} />}
        />
        <Route
          path="/follow-ups"
          element={<LazyLoad loader={() => import("@/pages/collaboration/FollowUpsPage").then((module) => ({ default: module.FollowUpsPage }))} fallback={loading} />}
        />
        <Route
          path="/calls"
          element={<LazyLoad loader={() => import("@/pages/collaboration/CallHistoryPage").then((module) => ({ default: module.CallHistoryPage }))} fallback={loading} />}
        />
        <Route
          path="/deals"
          element={<LazyLoad loader={() => import("@/pages/collaboration/DealsPage").then((module) => ({ default: module.DealsPage }))} fallback={loading} />}
        />
        <Route
          path="/deals/:dealId"
          element={<LazyLoad loader={() => import("@/pages/collaboration/DealDetailPage").then((module) => ({ default: module.DealDetailPage }))} fallback={loading} />}
        />
        <Route
          path="/meetings"
          element={<LazyLoad loader={() => import("@/pages/meetings/MeetingsPage").then((module) => ({ default: module.MeetingsPage }))} fallback={loading} />}
        />
        <Route
          path="/recordings"
          element={<LazyLoad loader={() => import("@/pages/recordings/RecordingsPage").then((module) => ({ default: module.RecordingsPage }))} fallback={loading} />}
        />
        <Route
          path="/recordings/:recordingId"
          element={<LazyLoad loader={() => import("@/pages/recordings/RecordingTranscriptPage").then((module) => ({ default: module.RecordingTranscriptPage }))} fallback={loading} />}
        />
        <Route path="/organizations" element={<OrganizationsPage />} />
        <Route path="/settings" element={<SettingsLayout />}>
          <Route index element={<Navigate to="profile" replace />} />
          <Route path="profile" element={<ProfileSettingsPage />} />
          <Route path="password" element={<PasswordSettingsPage />} />
          <Route path="sessions" element={<SessionsSettingsPage />} />
          <Route path="blocked" element={<BlockedSettingsPage />} />
          <Route path="notifications" element={<NotificationSettingsPage />} />
          <Route
            path="billing"
            element={<LazyLoad loader={() => import("@/pages/settings/BillingSettingsPage").then((module) => ({ default: module.BillingSettingsPage }))} fallback={loading} />}
          />
          <Route path="preferences" element={<PreferencesSettingsPage />} />
          <Route path="legal" element={<LegalSettingsPage />} />
          <Route path="danger" element={<DangerSettingsPage />} />
        </Route>
        <Route index element={<Navigate to="/inbox" replace />} />
      </Route>
    </Route>
  );
}
