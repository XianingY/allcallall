package commerce

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// OrgRepository handles data access for organization-level billing.
type OrgRepository struct {
	db *gorm.DB
}

// NewOrgRepository builds an OrgRepository.
func NewOrgRepository(db *gorm.DB) *OrgRepository {
	return &OrgRepository{db: db}
}

// GetOrganizationPlan returns the current plan for an org, if any.
func (r *OrgRepository) GetOrganizationPlan(ctx context.Context, orgID uint64) (*models.OrganizationPlan, error) {
	var plan models.OrganizationPlan
	err := r.db.WithContext(ctx).Where("organization_id = ?", orgID).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// UpsertOrganizationPlan creates or updates the org's current plan.
func (r *OrgRepository) UpsertOrganizationPlan(ctx context.Context, plan *models.OrganizationPlan) error {
	return r.db.WithContext(ctx).
		Where("organization_id = ?", plan.OrganizationID).
		Assign(models.OrganizationPlan{
			PlanID:             plan.PlanID,
			PlanName:           plan.PlanName,
			Status:             plan.Status,
			BillingCycle:       plan.BillingCycle,
			CurrentPeriodStart: plan.CurrentPeriodStart,
			CurrentPeriodEnd:   plan.CurrentPeriodEnd,
			Seats:              plan.Seats,
		}).
		FirstOrCreate(plan).Error
}

// GetOrgUsageLedger returns the org usage ledger for a feature/period.
func (r *OrgRepository) GetOrgUsageLedger(ctx context.Context, orgID uint64, feature, periodKey string) (*models.OrganizationUsageLedger, error) {
	var ledger models.OrganizationUsageLedger
	err := r.db.WithContext(ctx).
		Where("organization_id = ? AND feature = ? AND period_key = ?", orgID, feature, periodKey).
		Take(&ledger).Error
	if err != nil {
		return nil, err
	}
	return &ledger, nil
}

// FirstOrCreateOrgUsageLedger creates the org usage ledger if absent.
func (r *OrgRepository) FirstOrCreateOrgUsageLedger(ctx context.Context, ledger *models.OrganizationUsageLedger) error {
	return r.db.WithContext(ctx).
		Where("organization_id = ? AND feature = ? AND period_key = ?", ledger.OrganizationID, ledger.Feature, ledger.PeriodKey).
		FirstOrCreate(ledger).Error
}

// SaveOrgUsageLedger saves or updates an org usage ledger.
func (r *OrgRepository) SaveOrgUsageLedger(ctx context.Context, ledger *models.OrganizationUsageLedger) error {
	return r.db.WithContext(ctx).Save(ledger).Error
}

// ListOrganizationMemberUserIDs returns the user ids belonging to an org.
func (r *OrgRepository) ListOrganizationMemberUserIDs(ctx context.Context, orgID uint64) ([]uint64, error) {
	var ids []uint64
	if err := r.db.WithContext(ctx).
		Model(&models.OrganizationMember{}).
		Where("organization_id = ?", orgID).
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// SumUserUsageLedger sums a feature's usage across the given users for a period.
func (r *OrgRepository) SumUserUsageLedger(ctx context.Context, userIDs []uint64, feature, periodKey string) (int64, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	var total int64
	err := r.db.WithContext(ctx).
		Model(&models.UsageLedger{}).
		Where("user_id IN ? AND feature = ? AND period_key = ?", userIDs, feature, periodKey).
		Select("COALESCE(SUM(units), 0)").
		Scan(&total).Error
	return total, err
}

// TopUserUsage returns the highest-usage users for a feature/period (for dashboards).
func (r *OrgRepository) TopUserUsage(ctx context.Context, userIDs []uint64, feature, periodKey string, limit int) ([]models.UsageLedger, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var rows []models.UsageLedger
	err := r.db.WithContext(ctx).
		Where("user_id IN ? AND feature = ? AND period_key = ?", userIDs, feature, periodKey).
		Order("units DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// GetQuotaPolicy returns the quota bound to a plan+feature.
func (r *OrgRepository) GetQuotaPolicy(ctx context.Context, planID, feature string) (*models.QuotaPolicy, error) {
	var policy models.QuotaPolicy
	err := r.db.WithContext(ctx).
		Where("plan_id = ? AND feature = ?", planID, feature).
		Take(&policy).Error
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

// UpsertQuotaPolicy creates or updates a plan/feature quota.
func (r *OrgRepository) UpsertQuotaPolicy(ctx context.Context, policy *models.QuotaPolicy) error {
	return r.db.WithContext(ctx).
		Where("plan_id = ? AND feature = ?", policy.PlanID, policy.Feature).
		Assign(models.QuotaPolicy{LimitUnits: policy.LimitUnits, Unlimited: policy.Unlimited}).
		FirstOrCreate(policy).Error
}

// CreateInvoice persists an invoice header.
func (r *OrgRepository) CreateInvoice(ctx context.Context, inv *models.Invoice) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

// AddInvoiceLines persists invoice line items.
func (r *OrgRepository) AddInvoiceLines(ctx context.Context, lines []models.InvoiceLine) error {
	if len(lines) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&lines).Error
}

// GetInvoice returns an invoice with its lines.
func (r *OrgRepository) GetInvoice(ctx context.Context, orgID uint64, invoiceNo string) (*models.Invoice, []models.InvoiceLine, error) {
	var inv models.Invoice
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND invoice_no = ?", orgID, invoiceNo).Take(&inv).Error; err != nil {
		return nil, nil, err
	}
	var lines []models.InvoiceLine
	if err := r.db.WithContext(ctx).Where("invoice_id = ?", inv.ID).Find(&lines).Error; err != nil {
		return nil, nil, err
	}
	return &inv, lines, nil
}

// SaveInvoice persists invoice changes (status transitions).
func (r *OrgRepository) SaveInvoice(ctx context.Context, inv *models.Invoice) error {
	return r.db.WithContext(ctx).Save(inv).Error
}

// RunInTransaction executes fn inside a transaction.
func (r *OrgRepository) RunInTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// ErrQuotaExceeded is returned when org usage surpasses the plan limit.
var ErrQuotaExceeded = errors.New("commerce: organization quota exceeded")
