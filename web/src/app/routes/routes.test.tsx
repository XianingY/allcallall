import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Outlet, useLocation } from "react-router-dom";

import { App } from "@/app/App";
import { legacyRoutes } from "@/app/routes/legacyRoutes";
import { meetingRoutes } from "@/app/routes/meetingRoutes";
import { publicRoutes } from "@/app/routes/publicRoutes";
import { workspaceRoutes } from "@/app/routes/workspaceRoutes";

vi.mock("@/auth/ProtectedRoute", () => ({
  AnonymousRoute: () => <Outlet />,
  ProtectedRoute: () => <Outlet />,
}));
vi.mock("@/components/AppShell", () => ({ AppShell: () => <Outlet /> }));
vi.mock("@/components/ErrorBoundary", () => ({ ErrorBoundary: ({ children }: { children: React.ReactNode }) => children }));
vi.mock("@/pages/settings/SettingsLayout", () => ({ SettingsLayout: () => <Outlet /> }));

vi.mock("@/pages/auth/LoginPage", () => ({ LoginPage: () => <div data-testid="login" /> }));
vi.mock("@/pages/auth/RegisterPage", () => ({ RegisterPage: () => <div data-testid="register" /> }));
vi.mock("@/pages/auth/VerifyEmailPage", () => ({ VerifyEmailPage: () => <div data-testid="verify-email" /> }));
vi.mock("@/pages/auth/ForgotPasswordPage", () => ({ ForgotPasswordPage: () => <div data-testid="forgot-password" /> }));
vi.mock("@/pages/auth/InvitePage", () => ({ InvitePage: () => <div data-testid="invite" /> }));
vi.mock("@/pages/collaboration/InboxPage", () => ({ InboxPage: () => <div data-testid="inbox" /> }));
vi.mock("@/pages/OrganizationsPage", () => ({ OrganizationsPage: () => <div data-testid="organizations" /> }));
vi.mock("@/pages/settings/SettingsPages", () => ({
  BlockedSettingsPage: () => <div data-testid="settings-blocked" />,
  DangerSettingsPage: () => <div data-testid="settings-danger" />,
  LegalSettingsPage: () => <div data-testid="settings-legal" />,
  NotificationSettingsPage: () => <div data-testid="settings-notifications" />,
  PasswordSettingsPage: () => <div data-testid="settings-password" />,
  PreferencesSettingsPage: () => <div data-testid="settings-preferences" />,
  ProfileSettingsPage: () => <div data-testid="settings-profile" />,
  SessionsSettingsPage: () => <div data-testid="settings-sessions" />,
}));
vi.mock("@/pages/meetings/MeetingPreflightPage", () => ({ MeetingPreflightPage: () => <div data-testid="meeting-preflight" /> }));
vi.mock("@/pages/meetings/MeetingRoomPage", () => ({ MeetingRoomPage: () => <div data-testid="meeting-room" /> }));

afterEach(cleanup);

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}</output>;
}

async function expectRoute(path: string, marker: string, expectedPath = path) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
      <LocationProbe />
    </MemoryRouter>,
  );

  expect(await screen.findByTestId(marker)).toBeInTheDocument();
  expect(screen.getByTestId("location")).toHaveTextContent(expectedPath);
}

describe("application route compatibility", () => {
  it("exports the four route groups consumed by App", () => {
    expect([publicRoutes, workspaceRoutes, meetingRoutes, legacyRoutes].every((routes) => typeof routes === "function")).toBe(true);
  });

  it.each([
    ["/login", "login"],
    ["/invite/test-code", "invite"],
    ["/inbox", "inbox"],
    ["/meetings/42/preflight", "meeting-preflight"],
    ["/meetings/42", "meeting-room"],
    ["/settings/profile", "settings-profile"],
  ])("keeps %s mapped to its existing screen", async (path, marker) => {
    await expectRoute(path, marker);
  });

  it("redirects legacy room links to the canonical meeting path", async () => {
    await expectRoute("/rooms/42", "meeting-room", "/meetings/42");
  });

  it("redirects unknown paths to the inbox", async () => {
    await expectRoute("/not-a-route", "inbox", "/inbox");
  });
});
