import { formatBytes as sharedFormatBytes, formatShortDateTime } from "@allcallall/shared";

// Both of these now come from @allcallall/shared. They used to call
// Intl/toLocaleString directly, which meant the same timestamp rendered
// differently on web and mobile, and shifted with the device's locale and
// 12/24-hour setting.
export const formatTime = (value?: string | null) => formatShortDateTime(value);

export const formatBytes = (value?: number) => sharedFormatBytes(value);
