import type { LinkingOptions } from "@react-navigation/native";

import type { RootStackParamList } from "../navigation/AppNavigator";
import type { PendingIntent } from "../services/pendingIntent";
import {
  parseConversationIdFromURL,
  parseInvitationCodeFromURL,
  parseRoomIdFromURL,
} from "../utils/linking";

export const linking: LinkingOptions<RootStackParamList> = {
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

export function resolveDeepLink(url: string | null | undefined): PendingIntent | null {
  const roomId = parseRoomIdFromURL(url);
  if (roomId) {
    return { kind: "room", roomId };
  }

  const conversationId = parseConversationIdFromURL(url);
  if (conversationId) {
    return { kind: "conversation", conversationId };
  }

  const code = parseInvitationCodeFromURL(url);
  return code ? { kind: "invitation", code } : null;
}
