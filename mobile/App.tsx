import "react-native-get-random-values";
import React, { useRef } from "react";
import { NavigationContainer, NavigationContainerRef } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

import { AuthProvider } from "./src/context/AuthContext";
import { SignalingProvider } from "./src/context/SignalingContext";
import { SettingsProvider } from "./src/context/SettingsContext";
import AppNavigator from "./src/navigation/AppNavigator";
import PushNotificationService from "./src/services/PushNotificationService";
import CallOverlay from "./src/components/CallOverlay";

const App = () => {
  const navigationRef = useRef<NavigationContainerRef<any>>(null);

  // 设置推送通知的导航引用
  // Set navigation ref for push notification handling
  React.useEffect(() => {
    if (navigationRef.current) {
      PushNotificationService.setNavigationRef(navigationRef);
      console.log("[App] Push notification navigation ref set");
    }
  }, []);

  return (
    <SafeAreaProvider>
      <AuthProvider>
        <SettingsProvider>
          <SignalingProvider>
            <NavigationContainer ref={navigationRef}>
              <AppNavigator />
              <CallOverlay />
              <StatusBar style="auto" />
            </NavigationContainer>
          </SignalingProvider>
        </SettingsProvider>
      </AuthProvider>
    </SafeAreaProvider>
  );
};

export default App;
