import "react-native-get-random-values";
import React from "react";
import { NavigationContainer } from "@react-navigation/native";
import { StatusBar } from "expo-status-bar";
import "./src/i18n";

import { AppProviders } from "./src/app/AppProviders";
import { linking } from "./src/app/linking";
import { useDeepLinks } from "./src/app/useDeepLinks";
import CallOverlay from "./src/components/CallOverlay";
import { VersionGate } from "./src/components/VersionGate";
import AppNavigator from "./src/navigation/AppNavigator";
import { navigationRef } from "./src/navigation/navigationRef";
import PushNotificationService from "./src/services/PushNotificationService";

const App = () => {
  useDeepLinks();

  // Set the navigation ref for push-notification handling after mount.
  React.useEffect(() => {
    if (navigationRef.current) {
      PushNotificationService.setNavigationRef(navigationRef as any);
    }
  }, []);

  return (
    <AppProviders>
      <NavigationContainer ref={navigationRef} linking={linking}>
        <VersionGate>
          <AppNavigator />
          <CallOverlay />
        </VersionGate>
        <StatusBar style="auto" />
      </NavigationContainer>
    </AppProviders>
  );
};

export default App;
