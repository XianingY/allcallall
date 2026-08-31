package commerce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/allcallall/backend/internal/models"
)

// InvoiceLineInput is a caller-supplied invoice line item.
type InvoiceLineInput struct {
	Description string
	Quantity    int64
	UnitMinor   int64 // 单价(分)
}

// ComputeTax returns the tax (minor units) for a subtotal at the given permille
// rate. Example: rate 60 => 6%, 130 => 13% (China VAT for services/goods).
func ComputeTax(subtotalMinor, taxRatePermille int64) int64 {
	if taxRatePermille <= 0 {
		return 0
	}
	return subtotalMinor * taxRatePermille / 1000
}

// InvoiceService issues and manages billing documents.
type InvoiceService struct {
	repo *OrgRepository
}

// NewInvoiceService builds an InvoiceService.
func NewInvoiceService(repo *OrgRepository) *InvoiceService {
	return &InvoiceService{repo: repo}
}

func generateInvoiceNo(now time.Time, seq int) string {
	return fmt.Sprintf("AC%s%05d", now.UTC().Format("20060102"), seq)
}

// CreateInvoice builds and persists an invoice with line items, computing
// subtotal/tax/total. Currency defaults to CNY; amounts are in minor units (分).
func (s *InvoiceService) CreateInvoice(ctx context.Context, orgID uint64, planID string, periodStart, periodEnd time.Time, taxRatePermille int, currency string, lines []InvoiceLineInput) (*models.Invoice, error) {
	if len(lines) == 0 {
		return nil, errors.New("commerce: invoice requires at least one line")
	}
	if currency == "" {
		currency = "CNY"
	}
	var subtotal int64
	persistLines := make([]models.InvoiceLine, 0, len(lines))
	for _, l := range lines {
		qty := l.Quantity
		if qty <= 0 {
			qty = 1
		}
		amount := qty * l.UnitMinor
		subtotal += amount
		persistLines = append(persistLines, models.InvoiceLine{
			Description: l.Description,
			Quantity:    qty,
			UnitMinor:   l.UnitMinor,
			AmountMinor: amount,
		})
	}
	tax := ComputeTax(subtotal, int64(taxRatePermille))
	total := subtotal + tax

	var count int64
	if err := s.repo.db.WithContext(ctx).Model(&models.Invoice{}).Where("organization_id = ?", orgID).Count(&count).Error; err != nil {
		return nil, err
	}
	inv := &models.Invoice{
		OrganizationID:     orgID,
		InvoiceNo:          generateInvoiceNo(time.Now(), int(count)+1),
		PlanID:             planID,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
		Currency:           currency,
		SubtotalMinor:      subtotal,
		TaxMinor:           tax,
		TotalMinor:         total,
		TaxRatePermille:    taxRatePermille,
		Status:             "draft",
	}
	if err := s.repo.CreateInvoice(ctx, inv); err != nil {
		return nil, err
	}
	for i := range persistLines {
		persistLines[i].InvoiceID = inv.ID
	}
	if err := s.repo.AddInvoiceLines(ctx, persistLines); err != nil {
		return nil, err
	}
	return inv, nil
}

// IssueInvoice transitions a draft invoice to issued and stamps the issue time.
func (s *InvoiceService) IssueInvoice(ctx context.Context, orgID uint64, invoiceNo string) (*models.Invoice, error) {
	inv, _, err := s.repo.GetInvoice(ctx, orgID, invoiceNo)
	if err != nil {
		return nil, err
	}
	if inv.Status != "draft" {
		return nil, fmt.Errorf("commerce: invoice %s is not in draft state (current=%s)", invoiceNo, inv.Status)
	}
	now := time.Now().UTC()
	inv.Status = "issued"
	inv.IssuedAt = &now
	if err := s.repo.SaveInvoice(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// GetInvoice returns an invoice together with its line items.
func (s *InvoiceService) GetInvoice(ctx context.Context, orgID uint64, invoiceNo string) (*models.Invoice, []models.InvoiceLine, error) {
	return s.repo.GetInvoice(ctx, orgID, invoiceNo)
}
