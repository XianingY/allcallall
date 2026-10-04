import type { PropsWithChildren } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { ErrorBoundary } from "../components/ErrorBoundary";
import { AuthProvider } from "../context/AuthContext";
import { CommercialProvider } from "../context/CommercialContext";
import { FollowUpProvider } from "../context/FollowUpContext";
import { OrganizationProvider } from "../context/OrganizationContext";
import RoomCallProvider from "../context/RoomCallContext";
import { SettingsProvider } from "../context/SettingsContext";
import { SignalingProvider } from "../context/SignalingContext";

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <SafeAreaProvider>
      <ErrorBoundary>
        <AuthProvider>
          <OrganizationProvider>
            <CommercialProvider>
              <FollowUpProvider>
                <SettingsProvider>
                  <RoomCallProvider>
                    <SignalingProvider>{children}</SignalingProvider>
                  </RoomCallProvider>
                </SettingsProvider>
              </FollowUpProvider>
            </CommercialProvider>
          </OrganizationProvider>
        </AuthProvider>
      </ErrorBoundary>
    </SafeAreaProvider>
  );
}
