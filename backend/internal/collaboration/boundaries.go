package collaboration

import (
	"context"

	"github.com/allcallall/backend/internal/pagination"
)

// ChatServiceBoundary defines the chat/thread subset to extract from Service.
type ChatServiceBoundary interface {
	CreateMessage(ctx context.Context, organizationID, userID, conversationID uint64, input MessageInput) (*MessageRecord, error)
	CreateConversationNote(ctx context.Context, organizationID, userID, conversationID uint64, body string) (*ConversationNoteRecord, error)
	ListRealtimeEventsSince(ctx context.Context, organizationID, userID, sinceID uint64, limit int) ([]RealtimeEventRecord, error)
}

// RoomServiceBoundary defines the meeting/room subset to extract from Service.
type RoomServiceBoundary interface {
	CreateRoom(ctx context.Context, organizationID, userID uint64, input CreateRoomInput) (*RoomState, error)
	JoinRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error)
	LeaveRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error)
	GetRoomState(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error)
}

// RecordingServiceBoundary defines the recording subset to extract from Service.
type RecordingServiceBoundary interface {
	StartRecording(ctx context.Context, organizationID, userID, roomID uint64) (*RecordingView, error)
	StopRecording(ctx context.Context, organizationID, userID, roomID uint64) (*RecordingView, error)
	ListRecordings(ctx context.Context, organizationID, userID uint64, page pagination.Page) (pagination.Result[RecordingView], error)
}

// SupportServiceBoundary defines the read-only diagnostics subset to extract from Service.
type SupportServiceBoundary interface {
	GetSupportRoom(ctx context.Context, roomID uint64) (*SupportRoomView, error)
	GetSupportRecording(ctx context.Context, recordingID uint64) (*SupportRecordingView, error)
}

func (s *Service) ChatBoundary() ChatServiceBoundary {
	return s
}

func (s *Service) RoomBoundary() RoomServiceBoundary {
	return s
}

func (s *Service) RecordingBoundary() RecordingServiceBoundary {
	return s
}

func (s *Service) SupportBoundary() SupportServiceBoundary {
	return s
}
