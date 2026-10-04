import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { AppProviders } from "@/app/AppProviders";

vi.mock("@/api/queryClient", () => ({
  createAuthBridge: () => ({ kind: "auth-bridge" }),
  createQueryClient: () => ({ kind: "query-client" }),
}));
vi.mock("@tanstack/react-query", () => ({
  QueryClientProvider: ({ children }: { children: React.ReactNode }) => <div data-provider="query">{children}</div>,
}));
vi.mock("@/auth/AuthProvider", () => ({
  AuthProvider: ({ children }: { children: React.ReactNode }) => <div data-provider="auth">{children}</div>,
}));
vi.mock("@/organizations/OrganizationProvider", () => ({
  OrganizationProvider: ({ children }: { children: React.ReactNode }) => <div data-provider="organization">{children}</div>,
}));
vi.mock("@/calls/CallProvider", () => ({
  CallProvider: ({ children }: { children: React.ReactNode }) => <div data-provider="call">{children}</div>,
}));
vi.mock("@/realtime/ChatRealtimeProvider", () => ({
  ChatRealtimeProvider: ({ children }: { children: React.ReactNode }) => <div data-provider="realtime">{children}</div>,
}));

it("preserves the application provider nesting", () => {
  render(
    <AppProviders>
      <span data-testid="content" />
    </AppProviders>,
  );

  const providers: string[] = [];
  let current = screen.getByTestId("content").parentElement;
  while (current?.dataset.provider) {
    providers.push(current.dataset.provider);
    current = current.parentElement;
  }

  expect(providers).toEqual(["realtime", "call", "organization", "auth", "query"]);
});
