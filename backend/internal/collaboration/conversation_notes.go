package collaboration

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ListConversationNotes(ctx context.Context, organizationID, userID, conversationID uint64, limit int) ([]ConversationNoteRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var notes []ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.organization_id = ? AND conversation_notes.conversation_id = ?", organizationID, conversationID).
		Order("conversation_notes.created_at DESC").
		Limit(limit).
		Find(&notes).Error
	return notes, err
}
func (s *Service) CreateConversationNote(ctx context.Context, organizationID, userID, conversationID uint64, body string) (*ConversationNoteRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("note body required")
	}
	note := &models.ConversationNote{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		AuthorID:       userID,
		Body:           body,
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(note).Error; err != nil {
			return err
		}
		return tx.Model(&models.Conversation{}).
			Where("id = ? AND organization_id = ?", conversationID, organizationID).
			Updates(map[string]any{
				"last_internal_note_at": now,
				"updated_at":            now,
			}).Error
	}); err != nil {
		return nil, err
	}
	record, err := s.loadConversationNote(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publishConversationPatchUpdate(ctx, organizationID, conversationID, map[string]any{
		"last_internal_note_at": record.CreatedAt,
	})
	s.publishConversationEvent(ctx, organizationID, conversationID, "conversation.note.created", record)
	return record, nil
}
func (s *Service) latestConversationNote(ctx context.Context, organizationID, conversationID uint64) (*ConversationNoteRecord, error) {
	var note ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.organization_id = ? AND conversation_notes.conversation_id = ?", organizationID, conversationID).
		Order("conversation_notes.created_at DESC").
		Take(&note).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
}
func (s *Service) loadConversationNote(ctx context.Context, noteID uint64) (*ConversationNoteRecord, error) {
	var note ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.id = ?", noteID).
		Take(&note).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
}
