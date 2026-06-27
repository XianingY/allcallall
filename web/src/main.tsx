import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";

import { App } from "@/app/App";
import { AuthProvider } from "@/auth/AuthProvider";
import { AppErrorBoundary } from "@/components/AppErrorBoundary";
import { OrganizationProvider } from "@/organizations/OrganizationProvider";
import { CallProvider } from "@/calls/CallProvider";
import { ChatRealtimeProvider } from "@/realtime/ChatRealtimeProvider";
import "@/i18n";
import "@/styles.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 15_000, retry: 1, refetchOnWindowFocus: false },
    mutations: { retry: 0 },
  },
});

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
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
