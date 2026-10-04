import { useEffect } from "react";
import { Linking } from "react-native";

import { navigationRef } from "../navigation/navigationRef";
import { setPendingIntent, type PendingIntent } from "../services/pendingIntent";
import { resolveDeepLink } from "./linking";

function navigateOrDefer(intent: PendingIntent) {
  if (!navigationRef.isReady() || !navigationRef.current?.getCurrentRoute()) {
    console.warn("[App] Deep link received before navigation was ready; deferring", intent);
    setPendingIntent(intent);
    return;
  }

  if (intent.kind === "room") {
    navigationRef.navigate("PreJoin", { roomId: intent.roomId });
    return;
  }
  if (intent.kind === "conversation") {
    navigationRef.navigate("ConversationDetail", { conversationId: intent.conversationId });
    return;
  }
  navigationRef.navigate("InvitationAccept", { code: intent.code });
}

export function useDeepLinks(): void {
  useEffect(() => {
    const handleURL = (url: string | null | undefined) => {
      const intent = resolveDeepLink(url);
      if (intent) {
        navigateOrDefer(intent);
      }
    };

    void Linking.getInitialURL().then(handleURL).catch(() => {});
    const subscription = Linking.addEventListener("url", ({ url }) => handleURL(url));
    return () => subscription.remove();
  }, []);
}
