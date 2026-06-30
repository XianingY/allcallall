import "react-native-get-random-values";
import React from "react";
import { LinkingOptions, NavigationContainer } from "@react-navigation/native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import { Linking } from "react-native";
import './src/i18n';

import { AuthProvider } from "./src/context/AuthContext";
import { CommercialProvider } from "./src/context/CommercialContext";
import { FollowUpProvider } from "./src/context/FollowUpContext";
import { OrganizationProvider } from "./src/context/OrganizationContext";
import RoomCallProvider from "./src/context/RoomCallContext";
import { SignalingProvider } from "./src/context/SignalingContext";
import { SettingsProvider } from "./src/context/SettingsContext";
import AppNavigator from "./src/navigation/AppNavigator";
import type { RootStackParamList } from "./src/navigation/AppNavigator";
import { navigationRef } from "./src/navigation/navigationRef";
import PushNotificationService from "./src/services/PushNotificationService";
import CallOverlay from "./src/components/CallOverlay";
import {
  parseConversationIdFromURL,
  parseInvitationCodeFromURL,
  parseRoomIdFromURL,
} from "./src/utils/invitations";
import { ErrorBoundary } from "./src/components/ErrorBoundary";

const linking: LinkingOptions<RootStackParamList> = {
  prefixes: ["allcallall://"],
  config: {
    screens: {
      AgentDemo: "agent-demo",
      Rooms: "meetings",
      PreJoin: {
        path: "rooms/:roomId",
        parse: {
          roomId: (value: string) => Number(value),
        },
      },
      Conversations: "inbox",
      ConversationDetail: {
        path: "conversations/:conversationId",
        parse: {
          conversationId: (value: string) => Number(value),
        },
      },
      Contacts: "contacts",
      FollowUps: "follow-ups",
      Recordings: "recordings",
      RecordingTranscript: {
        path: "recordings/:recordingId/transcript",
        parse: {
          recordingId: (value: string) => Number(value),
          segmentId: (value: string) => Number(value),
          startMs: (value: string) => Number(value),
        },
      },
      Settings: "settings",
      Sessions: "sessions",
      InvitationAccept: "invite/:code",
    },
  },
};

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
      const conversationId = parseConversationIdFromURL(url);
      if (conversationId) {
        if (navigationRef.isReady()) {
          navigationRef.navigate("ConversationDetail", { conversationId });
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
      <ErrorBoundary>
        <AuthProvider>
          <OrganizationProvider>
            <CommercialProvider>
              <FollowUpProvider>
                <SettingsProvider>
                  <RoomCallProvider>
                    <SignalingProvider>
                      <NavigationContainer ref={navigationRef} linking={linking}>
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
      </ErrorBoundary>
    </SafeAreaProvider>
  );
};

export default App;
