/**
 * Share-link construction shared by the web and mobile clients.
 *
 * The web app never had a way to share a meeting, and mobile built the URL as
 * /rooms/:id - a route the web app does not serve, so the recipient landed in
 * Inbox instead of the meeting. Keeping one implementation means a path can
 * only be wrong in one place, and both clients are wrong together rather than
 * one silently working.
 *
 * The web route is /meetings/:id; the app deep link stays allcallall://rooms/:id
 * because that is what the mobile app already registers.
 */

export interface ShareLinks {
  /** Deep link that opens the native app. */
  appURL: string;
  /** https URL that opens the same thing in a browser. */
  webURL: string;
}

function normalizeOrigin(origin: string): string {
  return origin.replace(/\/+$/, "");
}

export function buildMeetingShareLinks(roomId: number, webOrigin: string): ShareLinks {
  const origin = normalizeOrigin(webOrigin);
  return {
    appURL: `allcallall://rooms/${roomId}`,
    webURL: `${origin}/meetings/${roomId}`,
  };
}

export function buildConversationShareLinks(conversationId: number, webOrigin: string): ShareLinks {
  const origin = normalizeOrigin(webOrigin);
  return {
    appURL: `allcallall://conversations/${conversationId}`,
    webURL: `${origin}/conversations/${conversationId}`,
  };
}

export function buildInvitationShareLinks(code: string, webOrigin: string): ShareLinks {
  const origin = normalizeOrigin(webOrigin);
  const trimmed = code.trim();
  return {
    appURL: `allcallall://invite/${trimmed}`,
    webURL: `${origin}/invite/${trimmed}`,
  };
}
