import React from "react";
import { NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

import { AuthProvider } from "./src/context/AuthContext";
import { CommercialProvider } from "./src/context/CommercialContext";
import { SignalingProvider } from "./src/context/SignalingContext";
import { SettingsProvider } from "./src/context/SettingsContext";
import AppNavigator from "./src/navigation/AppNavigator";
import { navigationRef } from "./src/navigation/navigationRef";
import PushNotificationService from "./src/services/PushNotificationService";
import CallOverlay from "./src/components/CallOverlay";

const App = () => {
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
        <CommercialProvider>
          <SettingsProvider>
            <SignalingProvider>
              <NavigationContainer ref={navigationRef}>
                <AppNavigator />
                <CallOverlay />
                <StatusBar style="auto" />
              </NavigationContainer>
            </SignalingProvider>
          </SettingsProvider>
        </CommercialProvider>
      </AuthProvider>
    </SafeAreaProvider>
  );
};

export default App;
