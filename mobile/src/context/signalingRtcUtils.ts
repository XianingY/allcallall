export type IceCandidatePayload = RTCIceCandidateInit & { iceEpoch?: number };

export interface RemoteTrackLike {
  kind: string;
  id?: string;
  muted?: boolean;
  readyState?: string;
  onmute?: (() => void) | null;
  onunmute?: (() => void) | null;
  onended?: (() => void) | null;
}

export interface RemoteTrackState<TTrack extends RemoteTrackLike = RemoteTrackLike> {
  audio: TTrack | null;
  video: TTrack | null;
}

export interface RemoteTrackEventLike<TTrack extends RemoteTrackLike = RemoteTrackLike> {
  track?: TTrack | null;
  streams?: Array<{
    getTracks?: () => TTrack[];
  }> | null;
}

export type DerivedNetworkQuality = "excellent" | "good" | "poor" | "bad" | "unknown";

export interface NetworkQualitySnapshot {
  currentRtt: number | null;
  connectionState?: string | null;
  iceConnectionState?: string | null;
}

export const createEmptyRemoteTrackState = <
  TTrack extends RemoteTrackLike = RemoteTrackLike,
>(): RemoteTrackState<TTrack> => ({
  audio: null,
  video: null,
});

export const normalizeIceEpoch = (candidate: { iceEpoch?: number } | null | undefined): number => {
  if (!candidate || typeof candidate.iceEpoch !== "number" || Number.isNaN(candidate.iceEpoch)) {
    return 0;
  }
  return candidate.iceEpoch;
};

export const toRTCIceCandidateInit = (candidate: IceCandidatePayload): RTCIceCandidateInit => ({
  candidate: candidate.candidate ?? "",
  sdpMid: candidate.sdpMid ?? null,
  sdpMLineIndex: candidate.sdpMLineIndex ?? null,
  usernameFragment: candidate.usernameFragment,
});

type CandidateApplyResult = "applied" | "queued" | "discarded";

export interface QueueOrApplyRemoteCandidateArgs {
  candidate: IceCandidatePayload;
  currentEpoch: number;
  hasRemoteDescription: boolean;
  pendingCandidates: IceCandidatePayload[];
  applyCandidate: (candidate: IceCandidatePayload) => Promise<void>;
}

export const queueOrApplyRemoteCandidate = async ({
  candidate,
  currentEpoch,
  hasRemoteDescription,
  pendingCandidates,
  applyCandidate,
}: QueueOrApplyRemoteCandidateArgs): Promise<CandidateApplyResult> => {
  const candidateEpoch = normalizeIceEpoch(candidate);
  if (candidateEpoch < currentEpoch) {
    return "discarded";
  }

  if (candidateEpoch > currentEpoch || !hasRemoteDescription) {
    pendingCandidates.push(candidate);
    return "queued";
  }

  await applyCandidate(candidate);
  return "applied";
};

export interface FlushPendingRemoteCandidatesArgs {
  currentEpoch: number;
  pendingCandidates: IceCandidatePayload[];
  applyCandidate: (candidate: IceCandidatePayload) => Promise<void>;
}

export const flushPendingRemoteCandidatesForCurrentEpoch = async ({
  currentEpoch,
  pendingCandidates,
  applyCandidate,
}: FlushPendingRemoteCandidatesArgs): Promise<IceCandidatePayload[]> => {
  const remaining: IceCandidatePayload[] = [];

  for (const candidate of pendingCandidates) {
    const candidateEpoch = normalizeIceEpoch(candidate);
    if (candidateEpoch < currentEpoch) {
      continue;
    }
    if (candidateEpoch > currentEpoch) {
      remaining.push(candidate);
      continue;
    }
    await applyCandidate(candidate);
  }

  return remaining;
};

export const discardStaleRemoteCandidates = (
  pendingCandidates: IceCandidatePayload[],
  currentEpoch: number
): IceCandidatePayload[] =>
  pendingCandidates.filter((candidate) => normalizeIceEpoch(candidate) >= currentEpoch);

export const deriveNetworkQualityUpdate = ({
  currentRtt,
  connectionState,
  iceConnectionState,
}: NetworkQualitySnapshot): DerivedNetworkQuality | null => {
  if (currentRtt !== null) {
    if (currentRtt < 0.1) return "excellent";
    if (currentRtt < 0.3) return "good";
    if (currentRtt < 0.5) return "poor";
    return "bad";
  }

  if (
    connectionState === "connected" ||
    iceConnectionState === "connected" ||
    iceConnectionState === "completed"
  ) {
    return "good";
  }
  if (
    connectionState === "disconnected" ||
    connectionState === "failed" ||
    iceConnectionState === "failed"
  ) {
    return "bad";
  }
  if (connectionState === "connecting" || iceConnectionState === "checking") {
    return "unknown";
  }
  return null;
};

export const collectRemoteTracks = <TTrack extends RemoteTrackLike = RemoteTrackLike>(
  event: RemoteTrackEventLike<TTrack>
): TTrack[] => {
  const tracks: TTrack[] = [];
  const seen = new Set<string>();

  const pushTrack = (track: TTrack | null | undefined) => {
    if (!track) return;
    const key = `${track.kind}:${track.id ?? `anon-${tracks.length}`}`;
    if (seen.has(key)) return;
    seen.add(key);
    tracks.push(track);
  };

  if (Array.isArray(event.streams)) {
    for (const stream of event.streams) {
      const streamTracks = stream?.getTracks?.() ?? [];
      streamTracks.forEach(pushTrack);
    }
  }

  pushTrack(event.track);

  return tracks;
};

export const upsertRemoteTrackState = <TTrack extends RemoteTrackLike = RemoteTrackLike>(
  previous: RemoteTrackState<TTrack>,
  track: TTrack
): RemoteTrackState<TTrack> => {
  if (track.kind === "audio") {
    return { ...previous, audio: track };
  }
  if (track.kind === "video") {
    return { ...previous, video: track };
  }
  return previous;
};

export const removeRemoteTrackState = <TTrack extends RemoteTrackLike = RemoteTrackLike>(
  previous: RemoteTrackState<TTrack>,
  track: TTrack
): RemoteTrackState<TTrack> => {
  const isSameAudioTrack =
    previous.audio === track || (Boolean(previous.audio?.id) && previous.audio?.id === track.id);
  const isSameVideoTrack =
    previous.video === track || (Boolean(previous.video?.id) && previous.video?.id === track.id);

  if (track.kind === "audio" && isSameAudioTrack) {
    return { ...previous, audio: null };
  }
  if (track.kind === "video" && isSameVideoTrack) {
    return { ...previous, video: null };
  }
  return previous;
};
