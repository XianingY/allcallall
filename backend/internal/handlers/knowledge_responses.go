package handlers

import (
	"strings"

	"github.com/allcallall/backend/internal/models"
)

func toKnowledgeSourceResponse(source models.RAGSource) knowledgeSourceResponse {
	return knowledgeSourceResponse{
		ID:                source.ID,
		OrganizationID:    source.OrganizationID,
		ConversationID:    source.ConversationID,
		CreatedBy:         source.CreatedBy,
		SourceGroupID:     source.SourceGroupID,
		CanonicalSourceID: source.CanonicalSourceID,
		Kind:              source.Kind,
		Title:             source.Title,
		URI:               source.URI,
		FileName:          source.FileName,
		ContentType:       source.ContentType,
		AuthorityScore:    source.AuthorityScore,
		AuthorityLabel:    source.AuthorityLabel,
		DedupeStatus:      source.DedupeStatus,
		Status:            source.Status,
		ActiveVersionID:   source.ActiveVersionID,
		LastError:         source.LastError,
		CreatedAt:         source.CreatedAt,
		UpdatedAt:         source.UpdatedAt,
	}
}

func toKnowledgeSourceVersionResponse(version models.RAGSourceVersion) knowledgeSourceVersionResponse {
	return knowledgeSourceVersionResponse{
		ID:             version.ID,
		SourceID:       version.SourceID,
		Version:        version.Version,
		ContentHash:    version.ContentHash,
		NormalizedHash: version.NormalizedHash,
		SimHash64:      version.SimHash64,
		Status:         version.Status,
		ChunkCount:     version.ChunkCount,
		LastError:      version.LastError,
		CreatedAt:      version.CreatedAt,
		UpdatedAt:      version.UpdatedAt,
		ActivatedAt:    version.ActivatedAt,
	}
}

func toSourceGroupResponse(group models.RAGSourceGroup) sourceGroupResponse {
	return sourceGroupResponse{
		ID:                group.ID,
		OrganizationID:    group.OrganizationID,
		CanonicalSourceID: group.CanonicalSourceID,
		Title:             group.Title,
		Status:            group.Status,
		AuthorityScore:    group.AuthorityScore,
		AuthorityLabel:    group.AuthorityLabel,
		CreatedBy:         group.CreatedBy,
		CreatedAt:         group.CreatedAt,
		UpdatedAt:         group.UpdatedAt,
	}
}

func toDuplicateCandidateResponse(row models.RAGSourceDuplicate) duplicateCandidateResponse {
	return duplicateCandidateResponse{
		ID:                row.ID,
		OrganizationID:    row.OrganizationID,
		SourceGroupID:     row.SourceGroupID,
		SourceID:          row.SourceID,
		CandidateSourceID: row.CandidateSourceID,
		DuplicateKind:     row.DuplicateKind,
		Similarity:        row.Similarity,
		Status:            row.Status,
		DecidedBy:         row.DecidedBy,
		Decision:          row.Decision,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		DecidedAt:         row.DecidedAt,
	}
}

func toRAGChunkResponse(chunk models.RAGChunk) ragChunkResponse {
	return ragChunkResponse{
		ID:              chunk.ID,
		SourceID:        chunk.SourceID,
		SourceVersionID: chunk.SourceVersionID,
		ConversationID:  chunk.ConversationID,
		ChunkIndex:      chunk.ChunkIndex,
		StartOffset:     chunk.StartOffset,
		EndOffset:       chunk.EndOffset,
		ContentHash:     chunk.ContentHash,
		Snippet:         compactHandlerSnippet(chunk.Content, 240),
		IndexStatus:     chunk.IndexStatus,
		LastError:       chunk.LastError,
		IndexedAt:       chunk.IndexedAt,
		CreatedAt:       chunk.CreatedAt,
		UpdatedAt:       chunk.UpdatedAt,
	}
}

func toDeadLetterResponse(row models.EventOutbox) deadLetterResponse {
	return deadLetterResponse{
		ID:             row.ID,
		AggregateType:  row.AggregateType,
		AggregateID:    row.AggregateID,
		Event:          row.Event,
		PayloadJSON:    row.PayloadJSON,
		IdempotencyKey: row.IdempotencyKey,
		RequestID:      row.RequestID,
		Status:         row.Status,
		Attempts:       row.Attempts,
		LastError:      row.LastError,
		AvailableAt:    row.AvailableAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func compactHandlerSnippet(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
