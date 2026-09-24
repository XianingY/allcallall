import axios from "axios";

import type { MCPInstallationDraft } from "../mcpPlatformUtils";

/** 新建安装表单的初始值（#29 拆分时从 MCPPlatformScreen 原样搬移）。 */
export const EMPTY_INSTALLATION: MCPInstallationDraft = {
  displayName: "",
  scope: "personal",
  sourceType: "https",
  transport: "streamable_http",
  imageRef: "",
  endpointURL: "",
  commandLines: "",
  argumentLines: "",
  allowlistLines: ""
};

export const STATUS_COLORS: Record<
  string,
  { background: string; foreground: string }
> = {
  active: { background: "#dcfce7", foreground: "#166534" },
  validating: { background: "#dbeafe", foreground: "#1d4ed8" },
  quarantined: { background: "#ffedd5", foreground: "#9a3412" },
  failed: { background: "#fee2e2", foreground: "#991b1b" },
  disabled: { background: "#f3f4f6", foreground: "#4b5563" },
  draft: { background: "#fef3c7", foreground: "#92400e" },
  succeeded: { background: "#dcfce7", foreground: "#166534" },
  running: { background: "#dbeafe", foreground: "#1d4ed8" },
  timed_out: { background: "#fee2e2", foreground: "#991b1b" }
};

export const errorMessage = (error: unknown): string => {
  if (axios.isAxiosError(error)) {
    const payload = error.response?.data as
      | { error?: string; message?: string }
      | undefined;
    return payload?.message || payload?.error || error.message;
  }
  return error instanceof Error ? error.message : "请求失败，请稍后重试";
};
