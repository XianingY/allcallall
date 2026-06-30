package commerce

import (
	"context"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

const (
	legalTermsVersion   = "2026-04-11"
	legalPrivacyVersion = "2026-04-11"
)

// LegalDocumentSet contains the current legal document URLs and version info.
type LegalDocumentSet struct {
	TermsVersion       string `json:"terms_version"`
	PrivacyVersion     string `json:"privacy_version"`
	TermsURL           string `json:"terms_url"`
	PrivacyPolicyURL   string `json:"privacy_policy_url"`
	SupportEmail       string `json:"support_email"`
	AccountDeletionURL string `json:"account_deletion_url"`
}

// LegalService handles legal document versioning and user acceptance records.
type LegalService struct {
	repo *Repository
}

// NewLegalService creates a new LegalService.
func NewLegalService(repo *Repository) *LegalService {
	return &LegalService{repo: repo}
}

// CurrentLegal returns the current legal document set with URLs and versions.
func (s *LegalService) CurrentLegal() LegalDocumentSet {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_WEB_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://allcallall.app"
	}
	supportEmail := strings.TrimSpace(os.Getenv("SUPPORT_EMAIL"))
	if supportEmail == "" {
		supportEmail = "support@allcallall.app"
	}
	return LegalDocumentSet{
		TermsVersion:       legalTermsVersion,
		PrivacyVersion:     legalPrivacyVersion,
		TermsURL:           baseURL + "/legal/terms",
		PrivacyPolicyURL:   baseURL + "/legal/privacy",
		SupportEmail:       supportEmail,
		AccountDeletionURL: baseURL + "/legal/delete-account",
	}
}

// AcceptLegal records the user's acceptance of the current legal documents.
func (s *LegalService) AcceptLegal(ctx context.Context, userID uint64) error {
	return s.repo.UpsertLegalAcceptance(ctx, userID, legalTermsVersion, legalPrivacyVersion)
}

// GetLegalAcceptance retrieves the user's legal acceptance record.
func (s *LegalService) GetLegalAcceptance(ctx context.Context, userID uint64) (*models.LegalAcceptance, error) {
	return s.repo.GetLegalAcceptance(ctx, userID)
}
