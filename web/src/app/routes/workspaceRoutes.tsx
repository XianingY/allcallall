import { Suspense, lazy, useEffect, type ComponentType } from "react";
import { Navigate, Route } from "react-router-dom";

import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { AppShell } from "@/components/AppShell";
import { LazyLoad } from "@/components/LazyLoad";
import { PageLoading } from "@/components/PageState";

const loading = <PageLoading />;
const InboxPage = lazy(() => import("@/pages/collaboration/InboxPage").then((module) => ({ default: module.InboxPage })));
const OrganizationsPage = lazy(() => import("@/pages/OrganizationsPage").then((module) => ({ default: module.OrganizationsPage })));
const SettingsLayout = lazy(() => import("@/pages/settings/SettingsLayout").then((module) => ({ default: module.SettingsLayout })));
const BlockedSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.BlockedSettingsPage })));
const DangerSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.DangerSettingsPage })));
const LegalSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.LegalSettingsPage })));
const NotificationSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.NotificationSettingsPage })));
const PasswordSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.PasswordSettingsPage })));
const PreferencesSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.PreferencesSettingsPage })));
const ProfileSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.ProfileSettingsPage })));
const SessionsSettingsPage = lazy(() => import("@/pages/settings/SettingsPages").then((module) => ({ default: module.SessionsSettingsPage })));

const lazyPage = (Page: ComponentType) => (
  <Suspense fallback={loading}>
    <Page />
  </Suspense>
);

function WorkspaceShell() {
  useEffect(() => {
    const prefetchInbox = () => void import("@/pages/collaboration/InboxPage");
    const idleWindow = window as Window & {
      requestIdleCallback?: (callback: () => void, options?: { timeout: number }) => number;
      cancelIdleCallback?: (id: number) => void;
    };

    if (typeof idleWindow.requestIdleCallback === "function") {
      const id = idleWindow.requestIdleCallback(prefetchInbox, { timeout: 2000 });
      return () => idleWindow.cancelIdleCallback?.(id);
    }

    const id = window.setTimeout(prefetchInbox, 300);
    return () => window.clearTimeout(id);
  }, []);

  return <AppShell />;
}

export function workspaceRoutes() {
  return (
    <Route element={<ProtectedRoute />}>
      <Route element={<WorkspaceShell />}>
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
        <Route path="/inbox" element={lazyPage(InboxPage)} />
        <Route path="/conversations/:conversationId" element={lazyPage(InboxPage)} />
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
        <Route path="/organizations" element={lazyPage(OrganizationsPage)} />
        <Route path="/settings" element={lazyPage(SettingsLayout)}>
          <Route index element={<Navigate to="profile" replace />} />
          <Route path="profile" element={lazyPage(ProfileSettingsPage)} />
          <Route path="password" element={lazyPage(PasswordSettingsPage)} />
          <Route path="sessions" element={lazyPage(SessionsSettingsPage)} />
          <Route path="blocked" element={lazyPage(BlockedSettingsPage)} />
          <Route path="notifications" element={lazyPage(NotificationSettingsPage)} />
          <Route
            path="billing"
            element={<LazyLoad loader={() => import("@/pages/settings/BillingSettingsPage").then((module) => ({ default: module.BillingSettingsPage }))} fallback={loading} />}
          />
          <Route path="preferences" element={lazyPage(PreferencesSettingsPage)} />
          <Route path="legal" element={lazyPage(LegalSettingsPage)} />
          <Route path="danger" element={lazyPage(DangerSettingsPage)} />
        </Route>
        <Route index element={<Navigate to="/inbox" replace />} />
      </Route>
    </Route>
  );
}
