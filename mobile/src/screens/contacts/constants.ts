import type { ReportCategoryOption } from "./types";

export const REPORT_CATEGORIES: ReportCategoryOption[] = [
  { value: "spam", label: "垃圾信息", description: "广告、反复骚扰或批量消息" },
  { value: "harassment", label: "骚扰辱骂", description: "辱骂、威胁或持续骚扰" },
  { value: "impersonation", label: "冒充身份", description: "假冒他人或伪装官方身份" },
  { value: "fraud", label: "诈骗欺诈", description: "诱导转账、钓鱼或其他诈骗行为" },
  { value: "sexual_content", label: "性相关内容", description: "不当性暗示、露骨内容或骚扰" },
  { value: "other", label: "其他问题", description: "不属于以上分类，但仍需处理" }
];
