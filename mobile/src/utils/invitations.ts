import { API_HOST } from "../config";

const readWebOrigin = () => {
  if (typeof window !== "undefined" && typeof window.location?.origin === "string") {
    return window.location.origin;
  }
  if (typeof process.env.EXPO_PUBLIC_WEB_APP_URL === "string" && process.env.EXPO_PUBLIC_WEB_APP_URL.trim()) {
    return process.env.EXPO_PUBLIC_WEB_APP_URL.trim().replace(/\/+$/, "");
  }
  return API_HOST.replace(/\/+$/, "");
};

export const parseInvitationCodeFromURL = (value: string | null | undefined) => {
  if (!value) {
    return null;
  }
  const marker = "/invite/";
  const index = value.indexOf(marker);
  if (index >= 0) {
    return value.slice(index + marker.length).split(/[?#]/)[0];
  }
  if (value.startsWith("allcallall://invite/")) {
    return value.replace("allcallall://invite/", "").split(/[?#]/)[0];
  }
  return null;
};

export const parseRoomIdFromURL = (value: string | null | undefined) => {
  if (!value) {
    return null;
  }
  const marker = "/rooms/";
  const index = value.indexOf(marker);
  if (index >= 0) {
    const parsed = Number(value.slice(index + marker.length).split(/[?#]/)[0]);
    return Number.isFinite(parsed) ? parsed : null;
  }
  if (value.startsWith("allcallall://rooms/")) {
    const parsed = Number(value.replace("allcallall://rooms/", "").split(/[?#]/)[0]);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
};

export const parseConversationIdFromURL = (value: string | null | undefined) => {
  if (!value) {
    return null;
  }
  const marker = "/conversations/";
  const index = value.indexOf(marker);
  if (index >= 0) {
    const parsed = Number(value.slice(index + marker.length).split(/[?#]/)[0]);
    return Number.isFinite(parsed) ? parsed : null;
  }
  if (value.startsWith("allcallall://conversations/")) {
    const parsed = Number(value.replace("allcallall://conversations/", "").split(/[?#]/)[0]);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
};

export const buildRoomShareLinks = (roomId: number) => {
  const webURL = `${readWebOrigin()}/rooms/${roomId}`;
  return {
    appURL: `allcallall://rooms/${roomId}`,
    webURL,
  };
};

export const buildConversationShareLinks = (conversationId: number) => {
  const webURL = `${readWebOrigin()}/conversations/${conversationId}`;
  return {
    appURL: `allcallall://conversations/${conversationId}`,
    webURL,
  };
};
