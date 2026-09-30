import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";

import { App } from "@/app/App";
import { AuthProvider } from "@/auth/AuthProvider";
import { createQueryClient, type AuthBridge } from "@/api/queryClient";
import { AppErrorBoundary } from "@/components/AppErrorBoundary";
import { OrganizationProvider } from "@/organizations/OrganizationProvider";
import { CallProvider } from "@/calls/CallProvider";
import { ChatRealtimeProvider } from "@/realtime/ChatRealtimeProvider";
import "@/i18n";
import "@/styles.css";

const authBridge: AuthBridge = { endSession: () => undefined };
const queryClient = createQueryClient(authBridge);

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <AuthProvider authBridge={authBridge}>
          <OrganizationProvider>
            <CallProvider>
              <ChatRealtimeProvider>
                <BrowserRouter>
                  <App />
                </BrowserRouter>
              </ChatRealtimeProvider>
            </CallProvider>
          </OrganizationProvider>
        </AuthProvider>
      </QueryClientProvider>
    </AppErrorBoundary>
  </React.StrictMode>,
);
