import React from "react";
import { NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import { Linking } from "react-native";

import { AuthProvider } from "./src/context/AuthContext";
import { CommercialProvider } from "./src/context/CommercialContext";
import { FollowUpProvider } from "./src/context/FollowUpContext";
import { OrganizationProvider } from "./src/context/OrganizationContext";
import RoomCallProvider from "./src/context/RoomCallContext";
import { SignalingProvider } from "./src/context/SignalingContext";
import { SettingsProvider } from "./src/context/SettingsContext";
import AppNavigator from "./src/navigation/AppNavigator";
import { navigationRef } from "./src/navigation/navigationRef";
import PushNotificationService from "./src/services/PushNotificationService";
import CallOverlay from "./src/components/CallOverlay";
import { parseInvitationCodeFromURL, parseRoomIdFromURL } from "./src/utils/invitations";

const App = () => {
  React.useEffect(() => {
    const handleURL = (url: string | null | undefined) => {
      const roomId = parseRoomIdFromURL(url);
      if (roomId) {
        if (navigationRef.isReady()) {
          navigationRef.navigate("PreJoin", { roomId });
        }
        return;
      }
      const code = parseInvitationCodeFromURL(url);
      if (!code) {
        return;
      }
      if (navigationRef.isReady()) {
        navigationRef.navigate("InvitationAccept", { code });
      }
    };

    void Linking.getInitialURL().then(handleURL).catch(() => {});
    const subscription = Linking.addEventListener("url", ({ url }) => {
      handleURL(url);
    });
    return () => subscription.remove();
  }, []);

  // 设置推送通知的导航引用
  // Set navigation ref for push notification handling
  React.useEffect(() => {
    if (navigationRef.current) {
      PushNotificationService.setNavigationRef(navigationRef as any);
    }
  }, []);

  return (
    <SafeAreaProvider>
      <AuthProvider>
        <OrganizationProvider>
          <CommercialProvider>
            <FollowUpProvider>
              <SettingsProvider>
                <RoomCallProvider>
                  <SignalingProvider>
                    <NavigationContainer ref={navigationRef}>
                      <AppNavigator />
                      <CallOverlay />
                      <StatusBar style="auto" />
                    </NavigationContainer>
                  </SignalingProvider>
                </RoomCallProvider>
              </SettingsProvider>
            </FollowUpProvider>
          </CommercialProvider>
        </OrganizationProvider>
      </AuthProvider>
    </SafeAreaProvider>
  );
};

export default App;
