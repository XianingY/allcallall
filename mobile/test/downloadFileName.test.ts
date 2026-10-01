import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { resolveSafeDownloadFileName } from "../src/platform/downloadFileName";

describe("resolveSafeDownloadFileName", () => {
  it("keeps an ordinary filename unchanged", () => {
    assert.equal(
      resolveSafeDownloadFileName("meeting-recording.mp3"),
      "meeting-recording.mp3",
    );
  });

  it("confines traversal paths to their final path segment", () => {
    assert.equal(
      resolveSafeDownloadFileName("../../outside.txt"),
      "outside.txt",
    );
    assert.equal(resolveSafeDownloadFileName("/etc/passwd"), "passwd");
  });

  it("removes control characters that are unsafe in filesystem names", () => {
    assert.equal(resolveSafeDownloadFileName("bad\x00name\n.txt"), "badname.txt");
  });

  it("falls back instead of returning an empty filename", () => {
    assert.equal(resolveSafeDownloadFileName("   "), "download");
    assert.equal(resolveSafeDownloadFileName("../"), "download");
  });

  it("caps long names so native filesystem limits are not hit", () => {
    const name = `${"a".repeat(250)}.mp3`;
    const result = resolveSafeDownloadFileName(name);
    assert.ok(result.length <= 180, `expected <=180, got ${result.length}`);
    assert.match(result, /\.mp3$/);
  });
});
