package collaboration

import (
	"context"

	"github.com/allcallall/backend/internal/search"
)

func (s *Service) BuildMessageSearchDocument(ctx context.Context, messageID uint64) (search.MessageDocument, error) {
	record, err := s.loadMessageRecord(ctx, messageID)
	if err != nil {
		return search.MessageDocument{}, err
	}
	return search.MessageDocument{
		ID:                search.MessageDocumentID(record.ID),
		OrganizationID:    record.OrganizationID,
		ConversationID:    record.ConversationID,
		MessageID:         record.ID,
		SenderID:          record.SenderID,
		SenderEmail:       record.SenderEmail,
		SenderDisplayName: record.SenderDisplayName,
		Type:              record.Type,
		Body:              record.Body,
		CreatedAt:         record.CreatedAt,
	}, nil
}

func (s *Service) FilterSearchResults(ctx context.Context, organizationID, userID uint64, results []search.MessageSearchResult) ([]search.MessageSearchResult, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	filtered := make([]search.MessageSearchResult, 0, len(results))
	for _, result := range results {
		if result.OrganizationID != organizationID {
			continue
		}
		if err := s.ensureConversationMember(ctx, organizationID, userID, result.ConversationID); err != nil {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered, nil
}
