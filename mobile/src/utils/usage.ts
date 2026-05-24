import type { UsageRecord } from "../api/commercial";

export const TRANSLATION_USAGE_FEATURE = "translation_seconds";

export const findTranslationUsage = (usage: UsageRecord[]) =>
  usage.find((item) => item.feature === TRANSLATION_USAGE_FEATURE);

export const formatTranslationMinutes = (seconds: number | null | undefined) => {
  if (seconds === null || seconds === undefined) {
    return "--";
  }
  return Math.max(0, Math.ceil(seconds / 60)).toString();
};

export const formatTranslationUsageSummary = (record?: UsageRecord | null) => {
  if (!record) {
    return "正在同步翻译配额";
  }
  if (record.unlimited) {
    return "本月翻译不限量";
  }
  const remaining = formatTranslationMinutes(record.remaining_units);
  const limit = formatTranslationMinutes(record.limit_units);
  return `本月翻译剩余 ${remaining} / ${limit} 分钟`;
};
