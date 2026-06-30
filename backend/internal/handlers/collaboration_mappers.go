package handlers

import (
	"encoding/json"
	"strings"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
)

func toOrganizationResponse(org models.Organization, role string) organizationResponse {
	return organizationResponse{
		ID:          org.ID,
		Name:        org.Name,
		Slug:        org.Slug,
		Description: org.Description,
		Role:        role,
	}
}

func toOrganizationPolicyResponse(policy models.OrganizationPolicy) organizationPolicyResponse {
	return organizationPolicyResponse{
		ID:                     policy.ID,
		OrganizationID:         policy.OrganizationID,
		RecordingMode:          policy.RecordingMode,
		RecordingStorageDays:   policy.RecordingStorageDays,
		RecordingExportAllowed: policy.RecordingExportAllowed,
	}
}

func toOrganizationAdminSummaryResponse(item collaboration.OrganizationAdminSummary) organizationAdminSummaryResponse {
	meetings := make([]organizationRecentMeetingResponse, 0, len(item.RecentMeetings))
	for _, meeting := range item.RecentMeetings {
		meetings = append(meetings, organizationRecentMeetingResponse{
			RoomID:         meeting.RoomID,
			ConversationID: meeting.ConversationID,
			Title:          meeting.Title,
			Status:         meeting.Status,
			StartedAt:      meeting.StartedAt,
			EndedAt:        meeting.EndedAt,
			UpdatedAt:      meeting.UpdatedAt,
		})
	}
	recordings := make([]organizationRecentRecordingResponse, 0, len(item.RecentRecordings))
	for _, recording := range item.RecentRecordings {
		recordings = append(recordings, organizationRecentRecordingResponse{
			RecordingSessionID:        recording.RecordingSessionID,
			RoomID:                    recording.RoomID,
			ConversationID:            recording.ConversationID,
			RoomTitle:                 recording.RoomTitle,
			RecordingStatus:           recording.RecordingStatus,
			TranscriptionStatus:       recording.TranscriptionStatus,
			TranscriptionProvider:     recording.TranscriptionProvider,
			TranscriptionSegmentCount: recording.TranscriptionSegmentCount,
			TranscriptionError:        recording.TranscriptionError,
			StartedAt:                 recording.StartedAt,
			StoppedAt:                 recording.StoppedAt,
			UpdatedAt:                 recording.UpdatedAt,
		})
	}
	events := make([]organizationAuditEventResponse, 0, len(item.RecentAuditEvents))
	for _, event := range item.RecentAuditEvents {
		events = append(events, toOrganizationAuditEventResponse(event))
	}
	return organizationAdminSummaryResponse{
		Counts: organizationAdminSummaryCountsResponse{
			MemberCount:           item.Counts.MemberCount,
			TeamCount:             item.Counts.TeamCount,
			PendingInviteCount:    item.Counts.PendingInviteCount,
			OpenConversationCount: item.Counts.OpenConversationCount,
			PendingApprovalCount:  item.Counts.PendingApprovalCount,
		},
		RecentMeetings:    meetings,
		RecentRecordings:  recordings,
		RecentAuditEvents: events,
	}
}

func toOrganizationInviteResponse(item models.OrganizationInvite) organizationInviteResponse {
	return organizationInviteResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		TeamID:         item.TeamID,
		Code:           item.Code,
		TargetEmail:    item.TargetEmail,
		Role:           item.Role,
		Status:         item.Status,
		AcceptedUserID: item.AcceptedUserID,
		AcceptedAt:     item.AcceptedAt,
		ExpiresAt:      item.ExpiresAt,
	}
}

func toOrganizationMemberResponse(item collaboration.OrganizationMemberView) organizationMemberResponse {
	return organizationMemberResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		UserID:         item.UserID,
		Email:          item.Email,
		DisplayName:    item.DisplayName,
		Status:         item.Status,
		Role:           item.Role,
		JoinedAt:       item.JoinedAt,
		LastActiveAt:   item.LastActiveAt,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func toTeamResponse(item collaboration.TeamView) teamResponse {
	members := make([]teamMemberResponse, 0, len(item.Members))
	for _, member := range item.Members {
		members = append(members, toTeamMemberResponse(member))
	}
	return teamResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		Name:           item.Name,
		Slug:           item.Slug,
		Description:    item.Description,
		CreatedBy:      item.CreatedBy,
		MemberCount:    item.MemberCount,
		Members:        members,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func toTeamMemberResponse(item collaboration.TeamMemberView) teamMemberResponse {
	return teamMemberResponse{
		ID:          item.ID,
		TeamID:      item.TeamID,
		UserID:      item.UserID,
		Email:       item.Email,
		DisplayName: item.DisplayName,
		Role:        item.Role,
		JoinedAt:    item.JoinedAt,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func toOrganizationAuditEventResponse(item collaboration.OrganizationAuditEventView) organizationAuditEventResponse {
	response := organizationAuditEventResponse{
		ID:               item.ID,
		OrganizationID:   item.OrganizationID,
		ActorUserID:      item.ActorUserID,
		ActorEmail:       item.ActorEmail,
		ActorDisplayName: item.ActorDisplayName,
		Action:           item.Action,
		TargetType:       item.TargetType,
		TargetID:         item.TargetID,
		CreatedAt:        item.CreatedAt,
	}
	if strings.TrimSpace(item.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(item.MetadataJSON), &metadata); err == nil {
			response.Metadata = metadata
		}
	}
	return response
}

func toConversationResponse(item collaboration.ConversationSummary) conversationResponse {
	return conversationResponse{
		ID:                  item.ID,
		OrganizationID:      item.OrganizationID,
		TeamID:              item.TeamID,
		RoomID:              item.RoomID,
		Type:                item.Type,
		Title:               item.Title,
		Topic:               item.Topic,
		Status:              item.Status,
		AssigneeUserID:      item.AssigneeUserID,
		AssigneeEmail:       item.AssigneeEmail,
		AssigneeDisplayName: item.AssigneeDisplayName,
		Priority:            item.Priority,
		ContactID:           item.ContactID,
		LastInternalNoteAt:  item.LastInternalNoteAt,
		LastMessageAt:       item.LastMessageAt,
		LastMessagePreview:  item.LastMessagePreview,
		LastMessageType:     item.LastMessageType,
		UnreadCount:         item.UnreadCount,
		ActiveRoomID:        item.ActiveRoomID,
		ActiveRoomTitle:     item.ActiveRoomTitle,
		LatestRoomID:        item.LatestRoomID,
		LatestRoomTitle:     item.LatestRoomTitle,
		LatestRecordingID:   item.LatestRecordingID,
	}
}

func toConversationDetailResponse(item collaboration.ConversationDetail) conversationDetailResponse {
	response := conversationDetailResponse{
		Conversation: toConversationResponse(item.Conversation),
		Workspace: conversationWorkspaceResponse{
			AssigneeUserID: item.Workspace.AssigneeUserID,
			AssigneeLabel:  item.Workspace.AssigneeLabel,
			Status:         item.Workspace.Status,
			Priority:       item.Workspace.Priority,
			AgentContext: conversationAgentContextResponse{
				LatestCallID:                  item.Workspace.AgentContext.LatestCallID,
				TranscriptSegmentCount:        item.Workspace.AgentContext.TranscriptSegmentCount,
				LatestTranscriptAt:            item.Workspace.AgentContext.LatestTranscriptAt,
				MeetingTranscriptionStatus:    item.Workspace.AgentContext.MeetingTranscriptionStatus,
				MeetingTranscriptionError:     item.Workspace.AgentContext.MeetingTranscriptionError,
				MeetingTranscriptSegmentCount: item.Workspace.AgentContext.MeetingTranscriptSegmentCount,
				LatestMeetingTranscriptAt:     item.Workspace.AgentContext.LatestMeetingTranscriptAt,
				LatestMemoryKeys:              item.Workspace.AgentContext.LatestMemoryKeys,
				LastAgentRunAt:                item.Workspace.AgentContext.LastAgentRunAt,
				LastAgentStatus:               item.Workspace.AgentContext.LastAgentStatus,
				LastWorkflowID:                item.Workspace.AgentContext.LastWorkflowID,
				LastWorkflowPreset:            item.Workspace.AgentContext.LastWorkflowPreset,
				PendingApprovalCount:          item.Workspace.AgentContext.PendingApprovalCount,
				KnowledgeSourceCount:          item.Workspace.AgentContext.KnowledgeSourceCount,
			},
		},
	}
	if item.LatestNote != nil {
		note := toConversationNoteResponse(*item.LatestNote)
		response.LatestNote = &note
	}
	if item.LatestRoom != nil {
		room := toRoomListItemResponse(*item.LatestRoom)
		response.LatestRoom = &room
	}
	if item.LatestFollowup != nil {
		followup := toConversationFollowupResponse(*item.LatestFollowup)
		response.LatestFollowup = &followup
	}
	if item.Workspace.LatestMeeting != nil {
		room := toRoomListItemResponse(*item.Workspace.LatestMeeting)
		response.Workspace.LatestMeeting = &room
	}
	if item.Workspace.LatestRecording != nil {
		recording := toRecordingResponse(*item.Workspace.LatestRecording)
		response.Workspace.LatestRecording = &recording
	}
	if item.Workspace.MeetingSummary != nil {
		response.Workspace.MeetingSummary = &meetingSummaryCardResponse{
			Summary:     item.Workspace.MeetingSummary.Summary,
			ActionItems: item.Workspace.MeetingSummary.ActionItems,
			NextStep:    item.Workspace.MeetingSummary.NextStep,
			Assignee:    item.Workspace.MeetingSummary.Assignee,
		}
	}
	if item.Workspace.LatestNote != nil {
		note := toConversationNoteResponse(*item.Workspace.LatestNote)
		response.Workspace.LatestNote = &note
	}
	return response
}

func toMessageResponse(item collaboration.MessageRecord) messageResponse {
	response := messageResponse{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		ConversationID:    item.ConversationID,
		SenderID:          item.SenderID,
		SenderEmail:       item.SenderEmail,
		SenderDisplayName: item.SenderDisplayName,
		ReplyToMessageID:  item.ReplyToMessageID,
		Type:              item.Type,
		Body:              item.Body,
		Pinned:            item.Pinned,
		EditedAt:          item.EditedAt,
		DeletedAt:         item.DeletedAt,
		DeletedBy:         item.DeletedBy,
		CreatedAt:         item.CreatedAt,
	}
	if item.ReplyTo != nil {
		response.ReplyTo = &messageReplyResponse{
			ID:                item.ReplyTo.ID,
			SenderID:          item.ReplyTo.SenderID,
			SenderEmail:       item.ReplyTo.SenderEmail,
			SenderDisplayName: item.ReplyTo.SenderDisplayName,
			Body:              item.ReplyTo.Body,
			Deleted:           item.ReplyTo.Deleted,
		}
	}
	for _, attachment := range item.Attachments {
		response.Attachments = append(response.Attachments, toAttachmentResponse(attachment))
	}
	for _, reaction := range item.Reactions {
		response.Reactions = append(response.Reactions, messageReactionResponse{
			Emoji:          reaction.Emoji,
			Count:          reaction.Count,
			ReactedUserIDs: reaction.ReactedUserIDs,
			ReactedByMe:    reaction.ReactedByMe,
		})
	}
	if strings.TrimSpace(item.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(item.MetadataJSON), &metadata); err == nil {
			response.Metadata = metadata
		}
	}
	return response
}

func toAttachmentResponse(item collaboration.AttachmentView) attachmentResponse {
	return attachmentResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		ConversationID: item.ConversationID,
		MessageID:      item.MessageID,
		UploaderID:     item.UploaderID,
		FileName:       item.FileName,
		ContentType:    item.ContentType,
		FileSize:       item.FileSize,
		DownloadURL:    item.DownloadURL,
		CreatedAt:      item.CreatedAt,
	}
}

func toRoomStateResponse(state collaboration.RoomState) roomStateResponse {
	return roomStateResponse{
		Room:              state.Room,
		Members:           state.Members,
		Events:            state.Events,
		ActiveRecording:   state.ActiveRecording,
		ConversationID:    state.ConversationID,
		ConversationTitle: state.ConversationTitle,
		ParticipantCount:  state.ParticipantCount,
		IsActive:          state.IsActive,
		HasRecording:      state.HasRecording,
		LatestRecordingID: state.LatestRecordingID,
	}
}

func toRecordingResponse(item collaboration.RecordingView) recordingResponse {
	files := make([]recordingFileResponse, 0, len(item.Files))
	for _, file := range item.Files {
		files = append(files, recordingFileResponse{
			ID:                 file.ID,
			RecordingSessionID: file.RecordingSessionID,
			StorageDriver:      file.StorageDriver,
			StorageBucket:      file.StorageBucket,
			ObjectKey:          file.ObjectKey,
			ETag:               file.ETag,
			ContentType:        file.ContentType,
			RetentionUntil:     file.RetentionUntil,
			DeletedAt:          file.DeletedAt,
			DurationSeconds:    file.DurationSeconds,
			MetadataJSON:       file.MetadataJSON,
			CreatedAt:          file.CreatedAt,
			DownloadURL:        file.DownloadURL,
			FileName:           file.FileName,
			FileSizeBytes:      file.FileSizeBytes,
			RecordingKind:      file.RecordingKind,
		})
	}
	response := recordingResponse{
		Session: item.Session,
		Files:   files,
	}
	if item.Transcription != nil {
		response.Transcription = &recordingTranscriptionStatusResponse{
			ID:           item.Transcription.ID,
			Status:       item.Transcription.Status,
			Provider:     item.Transcription.Provider,
			SegmentCount: item.Transcription.SegmentCount,
			ErrorMessage: item.Transcription.ErrorMessage,
			StartedAt:    item.Transcription.StartedAt,
			CompletedAt:  item.Transcription.CompletedAt,
			CreatedAt:    item.Transcription.CreatedAt,
			UpdatedAt:    item.Transcription.UpdatedAt,
		}
	}
	return response
}

func toRoomListItemResponse(item collaboration.RoomListItem) roomListItemResponse {
	return roomListItemResponse{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		TeamID:            item.TeamID,
		ConversationID:    item.ConversationID,
		ConversationTitle: item.ConversationTitle,
		Title:             item.Title,
		Status:            item.Status,
		CreatedBy:         item.CreatedBy,
		StartedAt:         item.StartedAt,
		EndedAt:           item.EndedAt,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
		ParticipantCount:  item.ParticipantCount,
		IsActive:          item.IsActive,
		HasRecording:      item.HasRecording,
		LatestRecordingID: item.LatestRecordingID,
	}
}

func toConversationNoteResponse(item collaboration.ConversationNoteRecord) conversationNoteResponse {
	return conversationNoteResponse{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		ConversationID:    item.ConversationID,
		AuthorID:          item.AuthorID,
		AuthorEmail:       item.AuthorEmail,
		AuthorDisplayName: item.AuthorDisplayName,
		Body:              item.Body,
		CreatedAt:         item.CreatedAt,
	}
}

func toConversationFollowupResponse(item collaboration.ConversationFollowupSummary) conversationFollowupResponse {
	return conversationFollowupResponse{
		CallID:      item.CallID,
		SummaryCN:   item.SummaryCN,
		SummaryEN:   item.SummaryEN,
		ActionItems: item.ActionItems,
		NextStep:    item.NextStep,
	}
}

func toPipelineResponse(item collaboration.PipelineView) pipelineResponse {
	return pipelineResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		Name:           item.Name,
		IsDefault:      item.IsDefault,
		Stages:         item.Stages,
	}
}

func toDealResponse(item collaboration.DealView) dealResponse {
	return dealResponse{
		ID:             item.ID,
		OrganizationID: item.OrganizationID,
		PipelineID:     item.PipelineID,
		StageID:        item.StageID,
		StageName:      item.StageName,
		OwnerID:        item.OwnerID,
		Title:          item.Title,
		Description:    item.Description,
		Status:         item.Status,
		ValueCents:     item.ValueCents,
		Currency:       item.Currency,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
