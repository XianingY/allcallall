import type { UseQueryResult } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Organization, OrganizationAdminSummary, OrganizationMember, OrganizationTeam } from "@/api/identity";
import { Overview } from "@/pages/organizations/OrganizationAdminTabs";

const active: Organization = {
  id: 1,
  name: "Tencent Demo",
  slug: "tencent-demo",
  description: "",
  role: "owner",
};

const members: OrganizationMember[] = [
  {
    id: 1,
    organization_id: 1,
    user_id: 7,
    email: "owner@example.com",
    display_name: "Owner",
    status: "active",
    role: "owner",
    joined_at: "2026-06-30T00:00:00Z",
    created_at: "2026-06-30T00:00:00Z",
    updated_at: "2026-06-30T00:00:00Z",
  },
];

const teams: OrganizationTeam[] = [
  {
    id: 1,
    organization_id: 1,
    name: "General",
    slug: "general",
    description: "",
    created_by: 7,
    member_count: 1,
    created_at: "2026-06-30T00:00:00Z",
    updated_at: "2026-06-30T00:00:00Z",
  },
];

const summary: OrganizationAdminSummary = {
  counts: {
    member_count: 3,
    team_count: 2,
    pending_invite_count: 1,
    open_conversation_count: 4,
    pending_approval_count: 2,
  },
  recent_meetings: [
    {
      room_id: 9,
      conversation_id: 5,
      title: "Daily Sync",
      status: "ended",
      started_at: "2026-06-30T01:00:00Z",
      ended_at: "2026-06-30T01:30:00Z",
      updated_at: "2026-06-30T01:30:00Z",
    },
  ],
  recent_recordings: [
    {
      recording_session_id: 11,
      room_id: 9,
      conversation_id: 5,
      room_title: "Daily Sync",
      recording_status: "stopped",
      transcription_status: "ready",
      transcription_provider: "mock",
      transcription_segment_count: 8,
      transcription_error: "",
      started_at: "2026-06-30T01:00:00Z",
      stopped_at: "2026-06-30T01:30:00Z",
      updated_at: "2026-06-30T01:31:00Z",
    },
  ],
  recent_audit_events: [
    {
      id: 3,
      organization_id: 1,
      actor_user_id: 7,
      actor_email: "owner@example.com",
      actor_display_name: "Owner",
      action: "organization.team.created",
      target_type: "team",
      target_id: "2",
      created_at: "2026-06-30T00:30:00Z",
    },
  ],
};

const query = (data: OrganizationAdminSummary | undefined, state: "success" | "loading" | "error" = "success") => ({
  data,
  error: state === "error" ? new Error("failed") : null,
  isError: state === "error",
  isLoading: state === "loading",
  refetch: vi.fn(),
}) as unknown as UseQueryResult<OrganizationAdminSummary>;

afterEach(() => cleanup());

describe("Organization overview dashboard", () => {
  it("renders admin summary metrics and recent activity", () => {
    render(<Overview active={active} canManage members={members} teams={teams} currentUserId={7} summary={query(summary)} />);

    expect(screen.getByText("待处理邀请")).toBeInTheDocument();
    expect(screen.getByText("开放会话")).toBeInTheDocument();
    expect(screen.getByText("待审批工具")).toBeInTheDocument();
    expect(screen.getAllByText("Daily Sync").length).toBeGreaterThan(0);
    expect(screen.getByText("organization.team.created")).toBeInTheDocument();
  });

  it("renders member-safe overview without calling admin summary content", () => {
    render(<Overview active={{ ...active, role: "member" }} canManage={false} members={members} teams={teams} currentUserId={7} summary={query(undefined)} />);

    expect(screen.getByText("管理员仪表盘仅 owner/admin 可查看")).toBeInTheDocument();
    expect(screen.queryByText("待审批工具")).not.toBeInTheDocument();
  });
});
