import type { MCPInstallation, MCPTool } from "@/api/mcp";

export const installationSourceLabel = (installation: Pick<MCPInstallation, "source_type" | "latest_revision">) => {
  if (installation.source_type === "oci") {
    return installation.latest_revision?.image_digest
      ? `OCI · ${installation.latest_revision.image_digest.slice(0, 18)}...`
      : "OCI image";
  }
  return installation.latest_revision?.endpoint_url ?? "HTTPS endpoint";
};

export const installationRevisionLabel = (installation: Pick<MCPInstallation, "latest_revision">) =>
  installation.latest_revision ? `Revision ${installation.latest_revision.revision}` : "Revision pending";

export const toolRiskLabel = (risk: MCPTool["risk"]) => {
  if (risk === "read") return "只读";
  if (risk === "write") return "写入";
  return "未知";
};

export const toolApprovalReason = (risk: MCPTool["risk"]) =>
  risk === "read" ? "验证通过后可直接执行" : risk === "write" ? "写入操作需要审批" : "风险未分类，需要审批";

export const canPublishInstallation = (installation: MCPInstallation, role?: string) =>
  (role === "owner" || role === "admin")
  && installation.scope === "personal"
  && installation.status === "active"
  && Boolean(installation.active_revision_id);

export const canBindInstallationToSkill = (
  skillScope: "personal" | "organization",
  installationScope: "personal" | "organization",
) => skillScope === "personal" || installationScope === "organization";

export const formatTimestamp = (value?: string | null) => value
  ? new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value))
  : "-";
