import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { AppProviders } from "@/app/AppProviders";

const authState = vi.hoisted(() => ({ status: "anonymous" as "loading" | "authenticated" | "anonymous" }));

afterEach(cleanup);

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
vi.mock("@/auth/AuthContext", () => ({
  useAuth: () => authState,
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

it("preserves the authenticated application provider nesting", () => {
  authState.status = "authenticated";
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

it.each(["anonymous", "loading"] as const)("does not mount authenticated runtime providers while %s", (status) => {
  authState.status = status;
  const { container } = render(
    <AppProviders>
      <span data-testid="content" />
    </AppProviders>,
  );

  expect(screen.getByTestId("content")).toBeInTheDocument();
  expect(container.querySelector('[data-provider="organization"]')).toBeNull();
  expect(container.querySelector('[data-provider="call"]')).toBeNull();
  expect(container.querySelector('[data-provider="realtime"]')).toBeNull();
});
