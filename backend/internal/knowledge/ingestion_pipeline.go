package knowledge

import (
	"context"
	"errors"
	"fmt"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
	"hash/fnv"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)


func (s *Service) ProcessSourceIngest(ctx context.Context, sourceID uint64) error {
	source, err := s.repo.GetSourceByIDOnly(ctx, sourceID)
	if err != nil {
		return err
	}
	version, rawText, err := s.prepareVersionForIngest(ctx, *source)
	if err != nil {
		s.markSourceFailed(ctx, source.ID, 0, err)
		return err
	}
	rawText = NormalizeText(rawText)
	if rawText == "" {
		err := errors.New("knowledge source text is empty")
		s.markSourceFailed(ctx, source.ID, version.ID, err)
		return err
	}
	contentHash := HashText(rawText)
	simHash := SimHashText(rawText)
	if source.ActiveVersionID != nil {
		active, err := s.repo.GetVersionByID(ctx, *source.ActiveVersionID)
		if err == nil && active.ContentHash == contentHash {
			now := time.Now().UTC()
			return s.repo.Run(ctx, func(tx *gorm.DB) error {
				if version.ID != active.ID {
					if err := s.repo.MarkVersionSuperseded(ctx, version.ID, now); err != nil {
						return err
					}
				}
				return s.repo.UpdateSourceFields(ctx, source.ID, map[string]any{
					"status":     models.RAGSourceStatusReady,
					"last_error": "",
					"updated_at": now,
				})
			})
		}
	}

	chunks := ChunkText(rawText, defaultChunkSize, defaultChunkOverlap)
	if len(chunks) == 0 {
		err := errors.New("knowledge source produced no chunks")
		s.markSourceFailed(ctx, source.ID, version.ID, err)
		return err
	}

	now := time.Now().UTC()
	var chunkIDs []uint64
	err = s.repo.Run(ctx, func(tx *gorm.DB) error {
		if err := s.ensureSourceGroupTx(ctx, tx, source, now); err != nil {
			return err
		}
		if err := s.repo.MarkAllActiveVersionsSuperseded(ctx, source.ID, now); err != nil {
			return err
		}
		if err := s.repo.DeleteChunksByVersion(ctx, version.ID); err != nil {
			return err
		}
		for _, spec := range chunks {
			chunk := models.RAGChunk{
				OrganizationID:  source.OrganizationID,
				ConversationID:  source.ConversationID,
				SourceID:        source.ID,
				SourceVersionID: version.ID,
				ChunkIndex:      spec.Index,
				StartOffset:     spec.StartOffset,
				EndOffset:       spec.EndOffset,
				ContentHash:     spec.ContentHash,
				Content:         spec.Content,
				Keywords:        spec.Keywords,
				IndexStatus:     models.RAGChunkIndexStatusPending,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if err := s.repo.CreateChunk(ctx, &chunk); err != nil {
				return err
			}
			chunkIDs = append(chunkIDs, chunk.ID)
		}
		if err := s.repo.ActivateVersion(ctx, version.ID, contentHash, simHash, rawText, len(chunkIDs), now); err != nil {
			return err
		}
		if err := s.repo.UpdateSourceActiveVersion(ctx, source.ID, version.ID, now); err != nil {
			return err
		}
		if s.outbox != nil {
			for _, chunkID := range chunkIDs {
				if _, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
					AggregateType:  "rag_chunk",
					AggregateID:    chunkID,
					Event:          EventChunkIndexRequested,
					IdempotencyKey: fmt.Sprintf("rag.chunk.index:%d:%d", chunkID, now.UnixNano()),
					Payload: map[string]any{
						"chunk_id":        chunkID,
						"source_id":       source.ID,
						"organization_id": source.OrganizationID,
					},
				}); err != nil {
					return err
				}
			}
		}
		if err := s.createDuplicateCandidatesTx(ctx, tx, *source, version.ID, contentHash, simHash, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		s.markSourceFailed(ctx, source.ID, version.ID, err)
		return err
	}
	return nil
}

func (s *Service) fetchURLText(ctx context.Context, rawURL string) (string, error) {
	parsed, err := validateFetchURL(rawURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AllCallAll-RAG-Ingest/1.0")
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	secureClient := *client
	secureClient.CheckRedirect = secureRedirectPolicy(client.CheckRedirect)
	resp, err := secureClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("url fetch failed: status=%d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxURLBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(raw)) > MaxURLBytes {
		return "", fmt.Errorf("url response exceeds %d bytes", MaxURLBytes)
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "html") || strings.HasPrefix(contentType, "text/") || contentType == "" {
		return ExtractHTMLText(string(raw)), nil
	}
	return "", fmt.Errorf("unsupported url content type: %s", contentType)
}

func ExtractFileText(fileName, contentType string, data []byte) (string, string, error) {
	if int64(len(data)) > MaxUploadBytes {
		return "", "", fmt.Errorf("knowledge file exceeds %d bytes", MaxUploadBytes)
	}
	contentType = normalizeContentType(contentType)
	ext := strings.ToLower(filepath.Ext(fileName))
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	switch {
	case contentType == "text/plain" || contentType == "text/markdown" || ext == ".txt" || ext == ".md":
		if contentType == "" {
			contentType = "text/plain"
		}
		return string(data), contentType, nil
	case contentType == "text/html" || ext == ".html" || ext == ".htm":
		return ExtractHTMLText(string(data)), "text/html", nil
	case contentType == "application/pdf" || ext == ".pdf":
		text, err := extractPDFText(data)
		if err != nil {
			return "", "", err
		}
		return text, "application/pdf", nil
	default:
		return "", "", ErrUnsupportedFileType
	}
}

func NormalizeText(input string) string {
	return strings.Join(strings.Fields(input), " ")
}

func SimHashText(input string) uint64 {
	tokens := extractKeywords(NormalizeText(input))
	if len(tokens) == 0 {
		return 0
	}
	var weights [64]int
	for _, token := range tokens {
		h := fnv.New64a()
		_, _ = h.Write([]byte(token))
		value := h.Sum64()
		for bit := 0; bit < 64; bit++ {
			if value&(uint64(1)<<bit) != 0 {
				weights[bit]++
			} else {
				weights[bit]--
			}
		}
	}
	var out uint64
	for bit, weight := range weights {
		if weight >= 0 {
			out |= uint64(1) << bit
		}
	}
	return out
}

func validateFetchURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("url scheme must be http or https")
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil, errors.New("local urls are not allowed")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return nil, errors.New("private or local urls are not allowed")
		}
	}
	return parsed, nil
}
