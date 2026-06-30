package commerce

import (
	"context"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

var allowedReportCategories = map[string]struct{}{
	"spam":           {},
	"harassment":     {},
	"impersonation":  {},
	"fraud":          {},
	"sexual_content": {},
	"other":          {},
}

func reportCategoryList() []string {
	return []string{
		"spam",
		"harassment",
		"impersonation",
		"fraud",
		"sexual_content",
		"other",
	}
}

func normalizeReportCategory(category string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(category))
	if _, ok := allowedReportCategories[normalized]; !ok {
		return "", ErrInvalidReportCategory
	}
	return normalized, nil
}

// BlockAbuseService manages user blocks and abuse reports.
type BlockAbuseService struct {
	repo *Repository
}

// NewBlockAbuseService creates a new BlockAbuseService.
func NewBlockAbuseService(repo *Repository) *BlockAbuseService {
	return &BlockAbuseService{repo: repo}
}

// CreateBlock blocks a user for the blocker.
func (s *BlockAbuseService) CreateBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	block := &models.UserBlock{
		BlockerID:     blockerID,
		BlockedUserID: blockedUserID,
	}
	return s.repo.FirstOrCreateUserBlock(ctx, block)
}

// RemoveBlock removes a block relationship.
func (s *BlockAbuseService) RemoveBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	return s.repo.DeleteUserBlock(ctx, blockerID, blockedUserID)
}

// ListBlocks returns all blocks created by a user.
func (s *BlockAbuseService) ListBlocks(ctx context.Context, blockerID uint64) ([]models.UserBlock, error) {
	return s.repo.ListUserBlocks(ctx, blockerID)
}

// AreUsersBlocked checks if there is a block relationship between two users.
func (s *BlockAbuseService) AreUsersBlocked(ctx context.Context, userA, userB uint64) (bool, error) {
	count, err := s.repo.CountUserBlocksBetweenUsers(ctx, userA, userB)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateReport creates an abuse report with validated category.
func (s *BlockAbuseService) CreateReport(ctx context.Context, reporterID, reportedUserID uint64, category, details string) error {
	normalizedCategory, err := normalizeReportCategory(category)
	if err != nil {
		return err
	}
	report := &models.AbuseReport{
		ReporterID:     reporterID,
		ReportedUserID: reportedUserID,
		Category:       normalizedCategory,
		Details:        strings.TrimSpace(details),
		Status:         "open",
	}
	return s.repo.CreateAbuseReport(ctx, report)
}

// ReportCategories returns the list of valid abuse report categories.
func (s *BlockAbuseService) ReportCategories() []string {
	return reportCategoryList()
}
