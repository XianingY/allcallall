import assert from "node:assert/strict";
import { test } from "node:test";

import { useSubtitleStore } from "./useSubtitleStore";

// Regression cover for the subtitle store: a later revision must replace the
// interim result for the same segment, and pruneExpired must drop stale entries.
test("upsertSubtitle replaces the interim result for the same segment", () => {
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

  const subtitles = useSubtitleStore.getState().subtitles;
  assert.equal(subtitles.length, 1, "revisions collapse onto one segment");
  assert.equal(subtitles[0].translated, "hello");
  assert.equal(subtitles[0].isFinal, true);
});

test("pruneExpired drops subtitles older than the retention window", () => {
  useSubtitleStore.getState().clearSubtitles();
  useSubtitleStore.getState().upsertSubtitle({
    segmentId: "seg-2",
    revision: 1,
    isFinal: true,
    source: "online",
    original: "你好",
    translated: "hello",
    timestamp: 1200,
  });

  useSubtitleStore.getState().pruneExpired(10_000_000);
  assert.equal(useSubtitleStore.getState().subtitles.length, 0);
});
