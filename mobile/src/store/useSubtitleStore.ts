import { create } from "zustand";

export interface SubtitleItem {
  id: string;
  original: string;
  translated: string;
  timestamp: number;
}

interface SubtitleState {
  subtitles: SubtitleItem[];
  /**
   * 添加新字幕，保持最多 10 条的滚动窗口
   * Add new subtitle, maintaining a rolling window of max 10 items
   */
  addSubtitle: (subtitle: SubtitleItem) => void;
  /**
   * 清空所有字幕
   * Clear all subtitles
   */
  clearSubtitles: () => void;
}

export const useSubtitleStore = create<SubtitleState>((set) => ({
  subtitles: [],
  addSubtitle: (subtitle) =>
    set((state) => ({
      subtitles: [...state.subtitles.slice(-9), subtitle]
    })),
  clearSubtitles: () => set({ subtitles: [] })
}));
