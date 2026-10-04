import type { PropsWithChildren } from "react";
import { QueryClientProvider } from "@tanstack/react-query";

import { createAuthBridge, createQueryClient } from "@/api/queryClient";
import { AuthProvider } from "@/auth/AuthProvider";
import { CallProvider } from "@/calls/CallProvider";
import { OrganizationProvider } from "@/organizations/OrganizationProvider";
import { ChatRealtimeProvider } from "@/realtime/ChatRealtimeProvider";

const authBridge = createAuthBridge();
const queryClient = createQueryClient(authBridge);

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider authBridge={authBridge}>
        <OrganizationProvider>
          <CallProvider>
            <ChatRealtimeProvider>{children}</ChatRealtimeProvider>
          </CallProvider>
        </OrganizationProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}
