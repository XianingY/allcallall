package search

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type MessageDocument struct {
	ID                string    `json:"id"`
	OrganizationID    uint64    `json:"organization_id"`
	ConversationID    uint64    `json:"conversation_id"`
	MessageID         uint64    `json:"message_id"`
	SenderID          uint64    `json:"sender_id"`
	SenderEmail       string    `json:"sender_email"`
	SenderDisplayName string    `json:"sender_display_name"`
	Type              string    `json:"type"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
}

type MessageSearchQuery struct {
	OrganizationID uint64
	UserID         uint64
	Query          string
	Limit          int
}

type MessageSearchResult struct {
	MessageDocument
	Score float64 `json:"score"`
}

type MessageIndexer interface {
	IndexMessage(ctx context.Context, doc MessageDocument) error
	SearchMessages(ctx context.Context, query MessageSearchQuery) ([]MessageSearchResult, error)
}

type Service struct {
	indexer MessageIndexer
}

func NewService(indexer MessageIndexer) *Service {
	return &Service{indexer: indexer}
}

func (s *Service) IndexMessage(ctx context.Context, doc MessageDocument) error {
	if s == nil || s.indexer == nil {
		return errors.New("search indexer is not configured")
	}
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = MessageDocumentID(doc.MessageID)
	}
	return s.indexer.IndexMessage(ctx, doc)
}

func (s *Service) SearchMessages(ctx context.Context, query MessageSearchQuery) ([]MessageSearchResult, error) {
	if s == nil || s.indexer == nil {
		return nil, errors.New("search indexer is not configured")
	}
	if query.OrganizationID == 0 || strings.TrimSpace(query.Query) == "" {
		return []MessageSearchResult{}, nil
	}
	if query.Limit <= 0 || query.Limit > 50 {
		query.Limit = 20
	}
	return s.indexer.SearchMessages(ctx, query)
}

func MessageDocumentID(messageID uint64) string {
	return "message:" + strconvFormatUint(messageID)
}

func strconvFormatUint(value uint64) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}

type MemoryIndexer struct {
	mu   sync.RWMutex
	docs map[string]MessageDocument
}

func NewMemoryIndexer() *MemoryIndexer {
	return &MemoryIndexer{docs: map[string]MessageDocument{}}
}

func (i *MemoryIndexer) IndexMessage(_ context.Context, doc MessageDocument) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = MessageDocumentID(doc.MessageID)
	}
	i.docs[doc.ID] = doc
	return nil
}

func (i *MemoryIndexer) SearchMessages(_ context.Context, query MessageSearchQuery) ([]MessageSearchResult, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	result := make([]MessageSearchResult, 0)
	for _, doc := range i.docs {
		if doc.OrganizationID != query.OrganizationID {
			continue
		}
		body := strings.ToLower(doc.Body)
		if !strings.Contains(body, needle) {
			continue
		}
		result = append(result, MessageSearchResult{
			MessageDocument: doc,
			Score:           score(body, needle),
		})
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Score == result[b].Score {
			return result[a].CreatedAt.After(result[b].CreatedAt)
		}
		return result[a].Score > result[b].Score
	})
	if len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result, nil
}

func score(body string, needle string) float64 {
	if body == needle {
		return 10
	}
	count := strings.Count(body, needle)
	if count == 0 {
		return 0
	}
	return float64(count)
}
