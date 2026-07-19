import { describe, expect, it } from "vitest";

import type { MCPInstallation } from "@/api/mcp";
import { canBindInstallationToSkill, canPublishInstallation, installationRevisionLabel, installationSourceLabel, toolApprovalReason, toolRiskLabel } from "@/pages/mcp/mcpFormatters";

const installation = {
  id: 1,
  organization_id: 2,
  owner_user_id: 3,
  scope: "personal",
  display_name: "GitHub",
  source_type: "oci",
  status: "active",
  active_revision_id: 8,
  secrets_configured: true,
  created_at: "2026-07-11T00:00:00Z",
  updated_at: "2026-07-11T00:00:00Z",
  latest_revision: { id: 8, revision: 4, transport: "stdio", image_digest: "sha256:1234567890abcdef1234567890abcdef", scan_status: "passed", created_by: 3, created_at: "2026-07-11T00:00:00Z" },
} satisfies MCPInstallation;

describe("MCP presentation policy", () => {
  it("keeps revision and source provenance visible", () => {
    expect(installationRevisionLabel(installation)).toBe("Revision 4");
    expect(installationSourceLabel(installation)).toContain("sha256:1234567890a");
  });

  it("requires approval for write and unknown tools", () => {
    expect(toolRiskLabel("read")).toBe("只读");
    expect(toolApprovalReason("write")).toContain("需要审批");
    expect(toolApprovalReason("unknown")).toContain("需要审批");
  });

  it("only exposes organization publishing to admins with an active revision", () => {
    expect(canPublishInstallation(installation, "admin")).toBe(true);
    expect(canPublishInstallation(installation, "member")).toBe(false);
    expect(canPublishInstallation({ ...installation, status: "disabled" }, "owner")).toBe(false);
    expect(canPublishInstallation({ ...installation, scope: "organization" }, "owner")).toBe(false);
  });

  it("prevents organization Skills from depending on personal installations", () => {
    expect(canBindInstallationToSkill("organization", "personal")).toBe(false);
    expect(canBindInstallationToSkill("organization", "organization")).toBe(true);
    expect(canBindInstallationToSkill("personal", "organization")).toBe(true);
  });
});
