export const parseParticipantIdFromMediaIds = (
  ...mediaIds: Array<string | null | undefined>
): number | undefined => {
  const candidate = mediaIds.find((value) => typeof value === "string" && value.includes("participant-"));
  if (!candidate) {
    return undefined;
  }
  const match = candidate.match(/participant-(\d+)/);
  if (!match) {
    return undefined;
  }
  const parsed = Number(match[1]);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
};

export const buildRemoteStreamKey = (
  streamId: string | null | undefined,
  trackKind: string | null | undefined,
  trackId: string | null | undefined
): string => {
  const trimmedStreamId = typeof streamId === "string" ? streamId.trim() : "";
  if (trimmedStreamId) {
    return trimmedStreamId;
  }
  const kind = typeof trackKind === "string" && trackKind.trim() ? trackKind.trim() : "track";
  const id = typeof trackId === "string" && trackId.trim() ? trackId.trim() : "unknown";
  return `${kind}-${id}`;
};
