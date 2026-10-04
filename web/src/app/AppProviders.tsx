import type { PropsWithChildren } from "react";
import { QueryClientProvider } from "@tanstack/react-query";

import { createAuthBridge, createQueryClient } from "@/api/queryClient";
import { AuthenticatedRuntime } from "@/app/AuthenticatedRuntime";
import { AuthProvider } from "@/auth/AuthProvider";
import { useAuth } from "@/auth/AuthContext";

const authBridge = createAuthBridge();
const queryClient = createQueryClient(authBridge);

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider authBridge={authBridge}>
        <AuthenticatedRuntimeGate>{children}</AuthenticatedRuntimeGate>
      </AuthProvider>
    </QueryClientProvider>
  );
}

function AuthenticatedRuntimeGate({ children }: PropsWithChildren) {
  const { status } = useAuth();

  if (status !== "authenticated") return <>{children}</>;
  return <AuthenticatedRuntime>{children}</AuthenticatedRuntime>;
}
