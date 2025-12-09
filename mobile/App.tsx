import React from "react";
import { NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";

import { AuthProvider } from "./src/context/AuthContext";
import { SignalingProvider } from "./src/context/SignalingContext";
import { SettingsProvider } from "./src/context/SettingsContext";
import AppNavigator from "./src/navigation/AppNavigator";

const App = () => {
  return (
    <SafeAreaProvider>
      <AuthProvider>
        <SettingsProvider>
          <SignalingProvider>
            <NavigationContainer>
              <AppNavigator />
              <StatusBar style="auto" />
            </NavigationContainer>
          </SignalingProvider>
        </SettingsProvider>
      </AuthProvider>
    </SafeAreaProvider>
  );
};

export default App;
