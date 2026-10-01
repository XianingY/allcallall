import {
  buildConversationShareLinks as buildConversationShareLinksFromShared,
  buildMeetingShareLinks,
} from "@allcallall/shared";

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

const parsePositiveId = (value: string) => {
  const parsed = Number(value.split(/[?#]/)[0]);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
};

export const parseRoomIdFromURL = (value: string | null | undefined) => {
  if (!value) {
    return null;
  }
  const marker = "/rooms/";
  const index = value.indexOf(marker);
  if (index >= 0) {
    return parsePositiveId(value.slice(index + marker.length));
  }
  if (value.startsWith("allcallall://rooms/")) {
    return parsePositiveId(value.replace("allcallall://rooms/", ""));
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
    return parsePositiveId(value.slice(index + marker.length));
  }
  if (value.startsWith("allcallall://conversations/")) {
    return parsePositiveId(value.replace("allcallall://conversations/", ""));
  }
  return null;
};

// Both of these delegate to @allcallall/shared so web and mobile cannot build
// different URLs for the same thing. The paths themselves (and why /meetings
// rather than /rooms) are documented there.
export const buildRoomShareLinksWithOrigin = (roomId: number, webOrigin: string) =>
  buildMeetingShareLinks(roomId, webOrigin);

export const buildConversationShareLinksWithOrigin = (conversationId: number, webOrigin: string) =>
  buildConversationShareLinksFromShared(conversationId, webOrigin);
