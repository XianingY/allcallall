import type { PropsWithChildren } from "react";

import { CallProvider } from "@/calls/CallProvider";
import { OrganizationProvider } from "@/organizations/OrganizationProvider";
import { ChatRealtimeProvider } from "@/realtime/ChatRealtimeProvider";

export function AuthenticatedRuntime({ children }: PropsWithChildren) {
  return (
    <OrganizationProvider>
      <CallProvider>
        <ChatRealtimeProvider>{children}</ChatRealtimeProvider>
      </CallProvider>
    </OrganizationProvider>
  );
}
