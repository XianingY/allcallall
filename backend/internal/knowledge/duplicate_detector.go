package knowledge

import (
	"context"
	"errors"
	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
	"strings"
	"time"
)

func (s *Service) ListDuplicateCandidates(ctx context.Context, organizationID, userID uint64) ([]models.RAGSourceDuplicate, error) {
	if err := s.ensureOrganizationMember(ctx, organizationID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListDuplicateCandidates(ctx, organizationID)
}

func (s *Service) DecideDuplicateCandidate(ctx context.Context, organizationID, userID, duplicateID uint64, decision string) error {
	if err := s.ensureOrganizationAdmin(ctx, organizationID, userID); err != nil {
		return err
	}
	decision = strings.TrimSpace(strings.ToLower(decision))
	if decision != "confirm" && decision != "reject" {
		return errors.New("duplicate decision must be confirm or reject")
	}
	now := time.Now().UTC()
	return s.repo.Run(ctx, func(tx *gorm.DB) error {
		duplicate, err := s.repo.GetDuplicateCandidate(ctx, duplicateID, organizationID)
		if err != nil {
			return err
		}
		status := models.RAGSourceDuplicateStatusRejected
		sourceUpdates := map[string]any{
			"updated_at": now,
		}
		if decision == "confirm" {
			status = models.RAGSourceDuplicateStatusConfirmed
			sourceUpdates["dedupe_status"] = models.RAGSourceDedupeStatusConfirmedDuplicate
			sourceUpdates["canonical_source_id"] = duplicate.CandidateSourceID
		} else {
			sourceUpdates["dedupe_status"] = models.RAGSourceDedupeStatusUnique
		}
		if err := s.repo.UpdateDuplicateCandidateStatus(ctx, duplicate.ID, status, decision, userID, now); err != nil {
			return err
		}
		return s.repo.UpdateSourceFields(ctx, duplicate.SourceID, sourceUpdates)
	})
}
