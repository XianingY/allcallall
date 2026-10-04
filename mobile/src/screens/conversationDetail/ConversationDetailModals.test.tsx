import React from "react";
import { render } from "@testing-library/react-native";

import ConversationDetailModals from "./ConversationDetailModals";
import {
  CitationPreviewModal,
  EditMessageModal,
  KnowledgePreviewModal,
  MessageActionMenuModal,
  WorkflowDebugModal,
} from "./Modals";

jest.mock("./Modals", () => ({
  __esModule: true,
  KnowledgePreviewModal: jest.fn(() => null),
  CitationPreviewModal: jest.fn(() => null),
  WorkflowDebugModal: jest.fn(() => null),
  MessageActionMenuModal: jest.fn(() => null),
  EditMessageModal: jest.fn(() => null),
}));

const createProps = () => ({
  knowledgePreview: null,
  citationPreview: null,
  workflowDebugVisible: false,
  activeWorkflow: null,
  orderedTasks: [],
  workflowLoading: false,
  token: "token",
  actionMenuMessage: null,
  editingMessage: null,
  editDraft: "",
  savingEdit: false,
  currentUserId: 1,
  onCloseKnowledgePreview: jest.fn(),
  onCloseCitationPreview: jest.fn(),
  onCloseWorkflowDebug: jest.fn(),
  onCloseActionMenu: jest.fn(),
  onStartEditMessage: jest.fn(),
  onRecallMessage: jest.fn(),
  onDeleteMessage: jest.fn(),
  onDraftChange: jest.fn(),
  onCloseEditMessage: jest.fn(),
  onSaveEditMessage: jest.fn(),
  onProcessCurrentWorkflow: jest.fn(),
});

describe("ConversationDetailModals", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("passes parent state and handlers to each modal", () => {
    const props = createProps();
    render(<ConversationDetailModals {...props} />);

    expect((KnowledgePreviewModal as jest.Mock).mock.calls[0][0]).toEqual(
      expect.objectContaining({
        knowledgePreview: null,
        onClose: props.onCloseKnowledgePreview,
      }),
    );
    expect((CitationPreviewModal as jest.Mock).mock.calls[0][0]).toEqual(
      expect.objectContaining({
        citationPreview: null,
        onClose: props.onCloseCitationPreview,
      }),
    );
    expect((WorkflowDebugModal as jest.Mock).mock.calls[0][0]).toEqual(
      expect.objectContaining({
        visible: false,
        activeWorkflow: null,
        orderedTasks: [],
        workflowLoading: false,
        token: "token",
        onClose: props.onCloseWorkflowDebug,
        onProcess: props.onProcessCurrentWorkflow,
      }),
    );
    expect((MessageActionMenuModal as jest.Mock).mock.calls[0][0]).toEqual(
      expect.objectContaining({
        visible: false,
        message: null,
        currentUserId: 1,
        canDeleteAny: false,
        onClose: props.onCloseActionMenu,
        onEdit: props.onStartEditMessage,
        onRecall: props.onRecallMessage,
        onDelete: props.onDeleteMessage,
      }),
    );
    expect((EditMessageModal as jest.Mock).mock.calls[0][0]).toEqual(
      expect.objectContaining({
        visible: false,
        message: null,
        draft: "",
        saving: false,
        onDraftChange: props.onDraftChange,
        onClose: props.onCloseEditMessage,
        onSave: props.onSaveEditMessage,
      }),
    );
  });

  it("skips rendering when a parent re-renders with unchanged props", () => {
    const props = createProps();
    const { rerender } = render(<ConversationDetailModals {...props} />);
    const initialCount = (WorkflowDebugModal as jest.Mock).mock.calls.length;

    rerender(<ConversationDetailModals {...props} />);
    expect((WorkflowDebugModal as jest.Mock).mock.calls.length).toBe(
      initialCount,
    );

    rerender(
      <ConversationDetailModals
        {...props}
        workflowDebugVisible
      />,
    );
    expect((WorkflowDebugModal as jest.Mock).mock.calls.length).toBeGreaterThan(
      initialCount,
    );
  });
});
