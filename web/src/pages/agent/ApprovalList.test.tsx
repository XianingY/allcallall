import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ToolApproval } from "@/api/agent";
import { ApprovalList } from "@/pages/agent/ApprovalList";

vi.mock("@/api/agent", () => ({ listApprovals: vi.fn() }));

const approval: ToolApproval = {
  id: 1,
  workflow_run_id: 10,
  task_id: 20,
  organization_id: 7,
  tool_call_id: "call-1",
  tool_name: "crm.update_deal",
  status: "pending",
  tool_schema_version: "v1",
  approval_request_id: "apr-1",
  approval_checkpoint_version: 1,
  input_json: "{}",
  output_json: "",
  error_message: "",
  requested_by: 1,
  decided_by: null,
  decision: "",
  requested_at: "2026-09-01T10:00:00Z",
  decided_at: null,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
};

describe("ApprovalList", () => {
  afterEach(cleanup);

  it("shows an error when the approvals query fails", () => {
    render(<ApprovalList approvals={[]} loading={false} decide={vi.fn()} error={new Error("审批加载失败")} />);
    expect(screen.getByText("审批加载失败")).toBeInTheDocument();
  });

  it("disables the decide buttons while a decision is pending", () => {
    render(<ApprovalList approvals={[approval]} loading={false} decide={vi.fn()} deciding />);
    expect(screen.getByRole("button", { name: "批准" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "拒绝" })).toBeDisabled();
  });
});
