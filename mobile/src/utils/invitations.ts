import { API_HOST } from "../config";
import {
  buildConversationShareLinksWithOrigin,
  buildRoomShareLinksWithOrigin,
  parseConversationIdFromURL,
  parseInvitationCodeFromURL,
  parseRoomIdFromURL,
} from "./linking";

const readWebOrigin = () => {
  if (typeof window !== "undefined" && typeof window.location?.origin === "string") {
    return window.location.origin;
  }
  if (typeof process.env.EXPO_PUBLIC_WEB_APP_URL === "string" && process.env.EXPO_PUBLIC_WEB_APP_URL.trim()) {
    return process.env.EXPO_PUBLIC_WEB_APP_URL.trim().replace(/\/+$/, "");
  }
  return API_HOST.replace(/\/+$/, "");
};

export { parseConversationIdFromURL, parseInvitationCodeFromURL, parseRoomIdFromURL };

export const buildRoomShareLinks = (roomId: number) => {
  return buildRoomShareLinksWithOrigin(roomId, readWebOrigin());
};

export const buildConversationShareLinks = (conversationId: number) => {
  return buildConversationShareLinksWithOrigin(conversationId, readWebOrigin());
};
