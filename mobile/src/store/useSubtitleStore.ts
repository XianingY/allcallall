import { create } from "zustand";

export type SubtitleSource = "online" | "remote";

export interface SubtitleItem {
  id: string;
  segmentId: string;
  revision: number;
  isFinal: boolean;
  source: SubtitleSource;
  original: string;
  translated: string;
  timestamp: number;
  expiresAt: number;
}

interface SubtitleUpsertInput {
  segmentId: string;
  revision: number;
  isFinal: boolean;
  source: SubtitleSource;
  original: string;
  translated: string;
  timestamp: number;
  expiresAt?: number;
}

interface SubtitleState {
  subtitles: SubtitleItem[];
  /**
   * 兼容旧接口：append 一条 final 字幕。
   * Backward-compatible append API.
   */
  addSubtitle: (subtitle: { id: string; original: string; translated: string; timestamp: number }) => void;
  /**
   * 新接口：按 segmentId + revision 回改。
   * Upsert subtitle by segment and revision.
   */
  upsertSubtitle: (subtitle: SubtitleUpsertInput) => void;
  /**
   * 清理过期字幕。
   * Remove expired subtitle items.
   */
  pruneExpired: (now?: number) => void;
  /**
   * 清空所有字幕。
   * Clear all subtitles.
   */
  clearSubtitles: () => void;
}

const FINAL_TTL_MS = 8_000;
const PARTIAL_TTL_MS = 3_000;
const MAX_SUBTITLES = 20;

export const useSubtitleStore = create<SubtitleState>((set) => ({
  subtitles: [],
  addSubtitle: (subtitle) =>
    set((state) => {
      const segmentId = subtitle.id || `legacy-${subtitle.timestamp}`;
      const item: SubtitleItem = {
        id: segmentId,
        segmentId,
        revision: 1,
        isFinal: true,
        source: "online",
        original: subtitle.original,
        translated: subtitle.translated,
        timestamp: subtitle.timestamp,
        expiresAt: subtitle.timestamp + FINAL_TTL_MS,
      };

      return {
        subtitles: [...state.subtitles, item].slice(-MAX_SUBTITLES),
      };
    }),
  upsertSubtitle: (subtitle) =>
    set((state) => {
      const expiresAt =
        typeof subtitle.expiresAt === "number"
          ? subtitle.expiresAt
          : subtitle.timestamp + (subtitle.isFinal ? FINAL_TTL_MS : PARTIAL_TTL_MS);

      const normalized: SubtitleItem = {
        id: subtitle.segmentId,
        segmentId: subtitle.segmentId,
        revision: subtitle.revision,
        isFinal: subtitle.isFinal,
        source: subtitle.source,
        original: subtitle.original,
        translated: subtitle.translated,
        timestamp: subtitle.timestamp,
        expiresAt,
      };

      const index = state.subtitles.findIndex((item) => item.segmentId === subtitle.segmentId);
      if (index < 0) {
        return {
          subtitles: [...state.subtitles, normalized].slice(-MAX_SUBTITLES),
        };
      }

      const existing = state.subtitles[index];
      if (subtitle.revision < existing.revision) {
        return state;
      }

      const next = [...state.subtitles];
      next[index] = {
        ...existing,
        ...normalized,
        timestamp: Math.max(existing.timestamp, normalized.timestamp),
      };

      return { subtitles: next.slice(-MAX_SUBTITLES) };
    }),
  pruneExpired: (now = Date.now()) =>
    set((state) => ({
      subtitles: state.subtitles.filter((item) => item.expiresAt > now),
    })),
  clearSubtitles: () => set({ subtitles: [] }),
}));
