import {
  collectRemoteTracks,
  createEmptyRemoteTrackState,
  discardStaleRemoteCandidates,
  flushPendingRemoteCandidatesForCurrentEpoch,
  normalizeIceEpoch,
  queueOrApplyRemoteCandidate,
  removeRemoteTrackState,
  upsertRemoteTrackState,
  type IceCandidatePayload,
  type RemoteTrackLike,
} from "./signalingRtcUtils";

interface FakeTrack extends RemoteTrackLike {
  id: string;
}

const track = (kind: "audio" | "video", id: string): FakeTrack => ({
  kind,
  id,
  muted: false,
  readyState: "live",
});

// Lightweight pseudo-test helper for environments without Jest wiring.
export const runSignalingRtcUtilsPseudoTest = async (): Promise<boolean> => {
  if (normalizeIceEpoch({}) !== 0) return false;
  if (normalizeIceEpoch({ iceEpoch: 3 }) !== 3) return false;

  const applied: string[] = [];
  const queued: IceCandidatePayload[] = [];

  const earlyCandidate: IceCandidatePayload = { candidate: "early", iceEpoch: 0 };
  const staleCandidate: IceCandidatePayload = { candidate: "stale", iceEpoch: 0 };
  const futureCandidate: IceCandidatePayload = { candidate: "future", iceEpoch: 2 };

  const firstResult = await queueOrApplyRemoteCandidate({
    candidate: earlyCandidate,
    currentEpoch: 0,
    hasRemoteDescription: false,
    pendingCandidates: queued,
    applyCandidate: async (candidate) => {
      applied.push(candidate.candidate ?? "");
    },
  });
  if (firstResult !== "queued" || queued.length !== 1) return false;

  queued.push(staleCandidate, futureCandidate);
  const pruned = discardStaleRemoteCandidates(queued, 1);
  if (pruned.length !== 1 || pruned[0].candidate !== "future") return false;

  const flushed = await flushPendingRemoteCandidatesForCurrentEpoch({
    currentEpoch: 0,
    pendingCandidates: [earlyCandidate, futureCandidate],
    applyCandidate: async (candidate) => {
      applied.push(candidate.candidate ?? "");
    },
  });
  if (applied.join(",") !== "early") return false;
  if (flushed.length !== 1 || flushed[0].candidate !== "future") return false;

  const audioTrack = track("audio", "a1");
  const videoTrack = track("video", "v1");
  const replacementVideoTrack = track("video", "v2");

  let remoteState = createEmptyRemoteTrackState<FakeTrack>();
  remoteState = upsertRemoteTrackState(remoteState, audioTrack);
  remoteState = upsertRemoteTrackState(remoteState, videoTrack);
  if (remoteState.audio !== audioTrack || remoteState.video !== videoTrack) return false;

  remoteState = upsertRemoteTrackState(remoteState, replacementVideoTrack);
  if (remoteState.audio !== audioTrack || remoteState.video !== replacementVideoTrack) return false;

  remoteState = removeRemoteTrackState(remoteState, replacementVideoTrack);
  if (remoteState.audio !== audioTrack || remoteState.video !== null) return false;

  const collected = collectRemoteTracks<FakeTrack>({
    track: videoTrack,
    streams: [
      {
        getTracks: () => [audioTrack, videoTrack],
      },
    ],
  });
  if (collected.length !== 2) return false;

  return true;
};
