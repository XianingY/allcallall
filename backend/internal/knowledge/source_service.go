package knowledge

import (
	"context"
	"errors"
	"fmt"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
	"math/bits"
	"strings"
	"time"
)


func (s *Service) CreateSource(ctx context.Context, organizationID, userID uint64, in CreateSourceInput) (*models.RAGSource, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	if in.ConversationID != nil {
		if err := s.ensureConversationMember(ctx, organizationID, userID, *in.ConversationID); err != nil {
			return nil, err
		}
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = models.RAGSourceKindManualText
	}
	if kind != models.RAGSourceKindManualText && kind != models.RAGSourceKindURL && kind != models.RAGSourceKindFile {
		return nil, ErrUnsupportedSource
	}

	title := strings.TrimSpace(in.Title)
	rawText := ""
	contentType := strings.TrimSpace(in.ContentType)
	switch kind {
	case models.RAGSourceKindManualText:
		rawText = NormalizeText(in.Text)
		if title == "" {
			title = "Manual knowledge"
		}
	case models.RAGSourceKindURL:
		if _, err := validateFetchURL(in.URL); err != nil {
			return nil, err
		}
		if title == "" {
			title = in.URL
		}
	case models.RAGSourceKindFile:
		extracted, detected, err := ExtractFileText(in.FileName, contentType, in.FileBytes)
		if err != nil {
			return nil, err
		}
		rawText = NormalizeText(extracted)
		contentType = detected
		if title == "" {
			title = strings.TrimSpace(in.FileName)
		}
		if title == "" {
			title = "Uploaded knowledge file"
		}
	}
	if kind != models.RAGSourceKindURL && rawText == "" {
		return nil, errors.New("knowledge source text is empty")
	}

	now := time.Now().UTC()
	source := models.RAGSource{
		OrganizationID: organizationID,
		ConversationID: in.ConversationID,
		CreatedBy:      userID,
		Kind:           kind,
		Title:          title,
		URI:            strings.TrimSpace(in.URL),
		FileName:       strings.TrimSpace(in.FileName),
		ContentType:    contentType,
		AuthorityScore: 0.5,
		AuthorityLabel: "user_provided",
		DedupeStatus:   models.RAGSourceDedupeStatusUnique,
		Status:         models.RAGSourceStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		group := models.RAGSourceGroup{
			OrganizationID:    organizationID,
			CanonicalSourceID: &source.ID,
			Title:             source.Title,
			Status:            models.RAGSourceGroupStatusActive,
			AuthorityScore:    source.AuthorityScore,
			AuthorityLabel:    source.AuthorityLabel,
			CreatedBy:         userID,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
			"source_group_id":     group.ID,
			"canonical_source_id": source.ID,
			"updated_at":          now,
		}).Error; err != nil {
			return err
		}
		source.SourceGroupID = &group.ID
		source.CanonicalSourceID = &source.ID
		if kind != models.RAGSourceKindURL {
			version := models.RAGSourceVersion{
				OrganizationID: organizationID,
				SourceID:       source.ID,
				Version:        1,
				ContentHash:    HashText(rawText),
				NormalizedHash: HashText(rawText),
				SimHash64:      SimHashText(rawText),
				RawText:        rawText,
				Status:         models.RAGSourceVersionStatusPending,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
		}
		if s.outbox != nil {
			_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "rag_source",
				AggregateID:    source.ID,
				Event:          EventSourceIngestRequested,
				IdempotencyKey: fmt.Sprintf("rag.source.ingest:%d:%d", source.ID, now.UnixNano()),
				Payload: map[string]any{
					"source_id":       source.ID,
					"organization_id": organizationID,
				},
			})
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &source, nil
}

func (s *Service) ListSources(ctx context.Context, organizationID, userID uint64, filter ListSourcesFilter) ([]models.RAGSource, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	if filter.ConversationID != nil {
		if err := s.ensureConversationMember(ctx, organizationID, userID, *filter.ConversationID); err != nil {
			return nil, err
		}
	}
	return s.repo.ListSources(ctx, organizationID, filter.ConversationID, strings.TrimSpace(filter.Status))
}

func (s *Service) ListSourceGroups(ctx context.Context, organizationID, userID uint64) ([]models.RAGSourceGroup, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListSourceGroups(ctx, organizationID)
}

func (s *Service) GetSourceGroup(ctx context.Context, organizationID, userID, groupID uint64) (models.RAGSourceGroup, []models.RAGSource, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return models.RAGSourceGroup{}, nil, err
	}
	group, err := s.repo.GetSourceGroupByID(ctx, groupID, organizationID)
	if err != nil {
		return models.RAGSourceGroup{}, nil, err
	}
	sources, err := s.repo.ListSourcesInGroup(ctx, organizationID, group.ID)
	if err != nil {
		return models.RAGSourceGroup{}, nil, err
	}
	return *group, sources, nil
}

func (s *Service) SetSourceGroupCanonical(ctx context.Context, organizationID, userID, groupID, sourceID uint64) error {
	if err := s.ensureOrganizationAdmin(ctx, organizationID, userID); err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.repo.Run(ctx, func(tx *gorm.DB) error {
		source, err := s.repo.GetSourceInGroup(ctx, sourceID, organizationID, groupID)
		if err != nil {
			return err
		}
		if err := s.repo.UpdateSourceGroupFields(ctx, groupID, organizationID, map[string]any{
			"canonical_source_id": sourceID,
			"title":               source.Title,
			"authority_score":     source.AuthorityScore,
			"authority_label":     source.AuthorityLabel,
			"updated_at":          now,
		}); err != nil {
			return err
		}
		return s.repo.UpdateSourceByOrgAndGroup(ctx, organizationID, groupID, map[string]any{
			"canonical_source_id": sourceID,
			"updated_at":          now,
		})
	})
}

func (s *Service) GetSource(ctx context.Context, organizationID, userID, sourceID uint64) (models.RAGSource, []models.RAGSourceVersion, []models.RAGChunk, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return models.RAGSource{}, nil, nil, err
	}
	source, err := s.repo.GetSourceByID(ctx, sourceID, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.RAGSource{}, nil, nil, ErrSourceNotFound
		}
		return models.RAGSource{}, nil, nil, err
	}
	if source.ConversationID != nil {
		if err := s.ensureConversationMember(ctx, organizationID, userID, *source.ConversationID); err != nil {
			return models.RAGSource{}, nil, nil, err
		}
	}
	versions, err := s.repo.ListVersionsBySource(ctx, source.ID)
	if err != nil {
		return models.RAGSource{}, nil, nil, err
	}
	chunks, err := s.repo.ListChunksBySource(ctx, source.ID)
	if err != nil {
		return models.RAGSource{}, nil, nil, err
	}
	return *source, versions, chunks, nil
}

func (s *Service) ReingestSource(ctx context.Context, organizationID, userID, sourceID uint64) error {
	source, _, _, err := s.GetSource(ctx, organizationID, userID, sourceID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return s.repo.Run(ctx, func(tx *gorm.DB) error {
		if source.Kind != models.RAGSourceKindURL {
			active, err := s.repo.GetActiveVersionBySource(ctx, source.ID)
			if err != nil {
				return err
			}
			nextVersion, err := s.repo.NextVersionNumberTx(ctx, tx, source.ID)
			if err != nil {
				return err
			}
			pending := models.RAGSourceVersion{
				OrganizationID: source.OrganizationID,
				SourceID:       source.ID,
				Version:        nextVersion,
				ContentHash:    active.ContentHash,
				NormalizedHash: active.NormalizedHash,
				SimHash64:      active.SimHash64,
				RawText:        active.RawText,
				Status:         models.RAGSourceVersionStatusPending,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(&pending).Error; err != nil {
				return err
			}
		}
		if err := s.repo.UpdateSourceFields(ctx, source.ID, map[string]any{
			"status":     models.RAGSourceStatusPending,
			"last_error": "",
			"updated_at": now,
		}); err != nil {
			return err
		}
		if s.outbox != nil {
			_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
				AggregateType:  "rag_source",
				AggregateID:    source.ID,
				Event:          EventSourceIngestRequested,
				IdempotencyKey: fmt.Sprintf("rag.source.reingest:%d:%d", source.ID, now.UnixNano()),
				Payload: map[string]any{
					"source_id":       source.ID,
					"organization_id": organizationID,
				},
			})
			return err
		}
		return nil
	})
}

func (s *Service) prepareVersionForIngest(ctx context.Context, source models.RAGSource) (models.RAGSourceVersion, string, error) {
	version, err := s.repo.GetPendingVersionBySource(ctx, source.ID)
	if err == nil {
		if strings.TrimSpace(version.RawText) != "" {
			return *version, version.RawText, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.RAGSourceVersion{}, "", err
	}
	if source.Kind == models.RAGSourceKindURL {
		rawText, err := s.fetchURLText(ctx, source.URI)
		if err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		nextVersion, err := s.repo.NextVersionNumber(ctx, source.ID)
		if err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		now := time.Now().UTC()
		newVersion := models.RAGSourceVersion{
			OrganizationID: source.OrganizationID,
			SourceID:       source.ID,
			Version:        nextVersion,
			ContentHash:    HashText(rawText),
			RawText:        rawText,
			Status:         models.RAGSourceVersionStatusPending,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.repo.CreateVersion(ctx, &newVersion); err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		return newVersion, rawText, nil
	}
	if source.ActiveVersionID != nil {
		active, err := s.repo.GetVersionByID(ctx, *source.ActiveVersionID)
		if err != nil {
			return models.RAGSourceVersion{}, "", err
		}
		return *active, active.RawText, nil
	}
	return models.RAGSourceVersion{}, "", errors.New("no pending knowledge source version found")
}

func (s *Service) nextVersionNumber(ctx context.Context, tx *gorm.DB, sourceID uint64) (int, error) {
	var latest models.RAGSourceVersion
	if err := tx.WithContext(ctx).Where("source_id = ?", sourceID).Order("version DESC").Take(&latest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 1, nil
		}
		return 0, err
	}
	return latest.Version + 1, nil
}

func (s *Service) ensureSourceGroupTx(ctx context.Context, tx *gorm.DB, source *models.RAGSource, now time.Time) error {
	if source.SourceGroupID != nil {
		return nil
	}
	group := models.RAGSourceGroup{
		OrganizationID:    source.OrganizationID,
		CanonicalSourceID: &source.ID,
		Title:             source.Title,
		Status:            models.RAGSourceGroupStatusActive,
		AuthorityScore:    source.AuthorityScore,
		AuthorityLabel:    source.AuthorityLabel,
		CreatedBy:         source.CreatedBy,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := tx.WithContext(ctx).Create(&group).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
		"source_group_id":     group.ID,
		"canonical_source_id": source.ID,
		"dedupe_status":       models.RAGSourceDedupeStatusUnique,
		"updated_at":          now,
	}).Error
}

func (s *Service) createDuplicateCandidatesTx(ctx context.Context, tx *gorm.DB, source models.RAGSource, versionID uint64, normalizedHash string, simHash uint64, now time.Time) error {
	var candidates []struct {
		SourceID       uint64
		SourceGroupID  *uint64
		ContentHash    string
		NormalizedHash string
		SimHash64      uint64
	}
	if err := tx.WithContext(ctx).
		Table("rag_source_versions").
		Select("rag_source_versions.source_id, rag_sources.source_group_id, rag_source_versions.content_hash, rag_source_versions.normalized_hash, rag_source_versions.sim_hash64").
		Joins("JOIN rag_sources ON rag_sources.id = rag_source_versions.source_id").
		Where("rag_source_versions.organization_id = ? AND rag_source_versions.status = ? AND rag_source_versions.id <> ?", source.OrganizationID, models.RAGSourceVersionStatusActive, versionID).
		Where("(rag_sources.dedupe_status IS NULL OR rag_sources.dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate).
		Find(&candidates).Error; err != nil {
		return err
	}
	created := false
	for _, candidate := range candidates {
		if candidate.SourceID == source.ID {
			continue
		}
		kind := ""
		similarity := 0.0
		candidateHash := candidate.NormalizedHash
		if candidateHash == "" {
			candidateHash = candidate.ContentHash
		}
		switch {
		case candidateHash != "" && candidateHash == normalizedHash:
			kind = models.RAGSourceDuplicateKindExact
			similarity = 1
		case candidate.SimHash64 != 0 && simHash != 0:
			distance := bits.OnesCount64(candidate.SimHash64 ^ simHash)
			if distance <= nearDuplicateHammingMax {
				kind = models.RAGSourceDuplicateKindNear
				similarity = 1 - float64(distance)/64
			}
		}
		if kind == "" {
			continue
		}
		groupID := source.SourceGroupID
		if groupID == nil {
			groupID = candidate.SourceGroupID
		}
		duplicate := models.RAGSourceDuplicate{
			OrganizationID:    source.OrganizationID,
			SourceGroupID:     groupID,
			SourceID:          source.ID,
			CandidateSourceID: candidate.SourceID,
			DuplicateKind:     kind,
			Similarity:        similarity,
			Status:            models.RAGSourceDuplicateStatusPending,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := tx.WithContext(ctx).Where(
			"organization_id = ? AND source_id = ? AND candidate_source_id = ?",
			duplicate.OrganizationID,
			duplicate.SourceID,
			duplicate.CandidateSourceID,
		).FirstOrCreate(&duplicate).Error; err != nil {
			return err
		}
		created = true
	}
	if created {
		return tx.WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", source.ID).Updates(map[string]any{
			"dedupe_status": models.RAGSourceDedupeStatusDuplicateCandidate,
			"updated_at":    now,
		}).Error
	}
	return nil
}

func (s *Service) loadSourcesByID(ctx context.Context, ids map[uint64]bool) (map[uint64]models.RAGSource, error) {
	out := map[uint64]models.RAGSource{}
	if len(ids) == 0 {
		return out, nil
	}
	var values []uint64
	for id := range ids {
		values = append(values, id)
	}
	var rows []models.RAGSource
	if err := s.repo.DB().WithContext(ctx).Where("id IN ?", values).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (s *Service) loadVersionsByID(ctx context.Context, ids map[uint64]bool) (map[uint64]models.RAGSourceVersion, error) {
	out := map[uint64]models.RAGSourceVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	var values []uint64
	for id := range ids {
		values = append(values, id)
	}
	var rows []models.RAGSourceVersion
	if err := s.repo.DB().WithContext(ctx).Where("id IN ?", values).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (s *Service) markSourceFailed(ctx context.Context, sourceID, versionID uint64, cause error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	_ = s.repo.DB().WithContext(ctx).Model(&models.RAGSource{}).Where("id = ?", sourceID).Updates(map[string]any{
		"status":     models.RAGSourceStatusFailed,
		"last_error": message,
		"updated_at": now,
	}).Error
	if versionID != 0 {
		_ = s.repo.DB().WithContext(ctx).Model(&models.RAGSourceVersion{}).Where("id = ?", versionID).Updates(map[string]any{
			"status":     models.RAGSourceVersionStatusFailed,
			"last_error": message,
			"updated_at": now,
		}).Error
	}
}
