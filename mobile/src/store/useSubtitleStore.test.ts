import { useSubtitleStore } from "./useSubtitleStore";

// Lightweight pseudo-test helper for environments without Jest wiring.
// Execute manually when needed.
export const runUseSubtitleStorePseudoTest = (): boolean => {
  const store = useSubtitleStore.getState();
  store.clearSubtitles();

  store.upsertSubtitle({
    segmentId: "seg-1",
    revision: 1,
    isFinal: false,
    source: "online",
    original: "你好",
    translated: "hel",
    timestamp: 1000,
  });

  store.upsertSubtitle({
    segmentId: "seg-1",
    revision: 2,
    isFinal: true,
    source: "online",
    original: "你好",
    translated: "hello",
    timestamp: 1200,
  });

  const afterUpsert = useSubtitleStore.getState().subtitles;
  if (afterUpsert.length !== 1) return false;
  if (afterUpsert[0].translated !== "hello") return false;
  if (!afterUpsert[0].isFinal) return false;

  useSubtitleStore.getState().pruneExpired(10_000_000);
  const afterPrune = useSubtitleStore.getState().subtitles;
  return afterPrune.length === 0;
};
