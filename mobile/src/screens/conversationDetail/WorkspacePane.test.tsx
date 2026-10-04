import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";

import WorkspacePane from "./WorkspacePane";
import type { WorkspacePaneProps } from "./types";

const renderWorkspacePane = (overrides: Partial<WorkspacePaneProps> = {}) => {
  const onAddNote = overrides.onAddNote ?? jest.fn().mockResolvedValue(true);
  const props = {
    conversation: {
      id: 1,
      organization_id: 1,
      type: "thread",
      title: "Quarterly review",
      status: "active",
      priority: "normal",
      unread_count: 0,
    },
    workspace: undefined,
    latestRoom: null,
    latestFollowup: null,
    assigneeLabel: "Unassigned",
    boundContact: undefined,
    agentContext: undefined,
    contacts: [],
    notes: [],
    latestRecording: null,
    activeWorkflow: null,
    pendingApprovals: [],
    completedTaskCount: 0,
    executedApprovalCount: 0,
    rejectedApprovalCount: 0,
    transcriptStatusText: "No transcript",
    agentStatusLabel: "idle",
    meetingTranscriptReady: false,
    workflowLoading: false,
    navigation: { navigate: jest.fn() },
    onCopyLink: jest.fn(),
    onCreateMeeting: jest.fn(),
    onAssignSelf: jest.fn(),
    onUnassign: jest.fn(),
    onRunMeetingAgent: jest.fn(),
    onOpenWorkflowDebug: jest.fn(),
    onCitationPress: jest.fn(),
    onApprovalDecision: jest.fn(),
    onUpdateStatus: jest.fn(),
    onUpdatePriority: jest.fn(),
    onBindContact: jest.fn(),
    onDownloadRecording: jest.fn(),
    ...overrides,
    onAddNote,
  } as unknown as WorkspacePaneProps;

  return {
    onAddNote,
    ...render(<WorkspacePane {...props} />),
  };
};

describe("WorkspacePane", () => {
  it("keeps the note draft local and clears it after a successful add", async () => {
    const { onAddNote } = renderWorkspacePane();
    const input = screen.getByPlaceholderText(
      "记录交接说明、风险点或下一步动作",
    );

    fireEvent.changeText(input, "handoff note");
    fireEvent.press(screen.getByText("添加内部备注"));

    await waitFor(() => expect(onAddNote).toHaveBeenCalledWith("handoff note"));
    await waitFor(() => expect(input.props.value).toBe(""));
  });

  it("keeps the note draft when adding fails", async () => {
    const { onAddNote } = renderWorkspacePane({
      onAddNote: jest.fn().mockResolvedValue(false),
    });
    const input = screen.getByPlaceholderText(
      "记录交接说明、风险点或下一步动作",
    );

    fireEvent.changeText(input, "retry this note");
    fireEvent.press(screen.getByText("添加内部备注"));

    await waitFor(() =>
      expect(onAddNote).toHaveBeenCalledWith("retry this note"),
    );
    expect(input.props.value).toBe("retry this note");
  });
});
