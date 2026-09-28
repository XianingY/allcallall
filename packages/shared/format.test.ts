import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { formatBytes, formatClock, formatDate, formatDateTime, formatShortDateTime } from "./format";

// Fixed reference "now" so the short form is deterministic regardless of when
// the suite runs.
const now = new Date(2026, 8, 28, 14, 30);

describe("formatDateTime", () => {
  it("renders a fixed, locale-independent shape", () => {
    assert.equal(formatDateTime("2026-09-28T14:30:00"), "2026-09-28 14:30");
  });

  it("zero-pads single digit parts", () => {
    assert.equal(formatDateTime(new Date(2026, 0, 5, 9, 5)), "2026-01-05 09:05");
  });

  it("returns an empty string instead of 'Invalid Date'", () => {
    assert.equal(formatDateTime("not-a-date"), "");
    assert.equal(formatDateTime(null), "");
    assert.equal(formatDateTime(undefined), "");
    assert.equal(formatDateTime(""), "");
  });
});

describe("formatDate / formatClock", () => {
  it("formatDate keeps only the date", () => {
    assert.equal(formatDate("2026-09-28T14:30:00"), "2026-09-28");
  });

  it("formatClock keeps only the time", () => {
    assert.equal(formatClock("2026-09-28T14:30:00"), "14:30");
  });
});

describe("formatShortDateTime", () => {
  it("shows only the clock for something that happened today", () => {
    assert.equal(formatShortDateTime("2026-09-28T09:05:00", now), "09:05");
  });

  it("drops the year within the current year", () => {
    assert.equal(formatShortDateTime("2026-08-01T09:05:00", now), "08-01 09:05");
  });

  it("keeps the full date for an older year", () => {
    assert.equal(formatShortDateTime("2025-12-31T09:05:00", now), "2025-12-31 09:05");
  });

  it("is empty for unusable input", () => {
    assert.equal(formatShortDateTime("nope", now), "");
  });
});

describe("formatBytes", () => {
  it("scales through the units", () => {
    assert.equal(formatBytes(0), "0 B");
    assert.equal(formatBytes(512), "512 B");
    assert.equal(formatBytes(2048), "2.0 KB");
    assert.equal(formatBytes(5 * 1024 * 1024), "5.0 MB");
  });

  it("handles null and undefined", () => {
    assert.equal(formatBytes(null), "0 B");
    assert.equal(formatBytes(undefined), "0 B");
  });
});
