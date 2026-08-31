package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/tenant"
)

// OrgBillingHandler exposes B2B organization billing endpoints: plan/quota
// state, multi-dimensional usage analytics and invoices.
//
// These endpoints are organization-scoped by nature, so they require a resolved
// tenant (unlike account-level endpoints, where a user without an organization
// is still legitimate). The org id always comes from the tenant middleware,
// never from a client header.
type OrgBillingHandler struct {
	logger   zerolog.Logger
	billing  *commerce.OrgBillingService
	usage    *commerce.UsageStatsService
	invoices *commerce.InvoiceService
	quota    *commerce.QuotaService
}

func NewOrgBillingHandler(
	log zerolog.Logger,
	billing *commerce.OrgBillingService,
	usage *commerce.UsageStatsService,
	invoices *commerce.InvoiceService,
	quota *commerce.QuotaService,
) *OrgBillingHandler {
	return &OrgBillingHandler{
		logger:   log.With().Str("component", "org_billing_handler").Logger(),
		billing:  billing,
		usage:    usage,
		invoices: invoices,
		quota:    quota,
	}
}

func (h *OrgBillingHandler) RegisterProtectedRoutes(protected *gin.RouterGroup) {
	protected.GET("/org/plan", h.handleOrgPlan)
	protected.GET("/org/usage", h.handleOrgUsage)
	protected.GET("/org/invoices/:invoiceNo", h.handleGetInvoice)
}

// resolveOrg 取租户中间件解析出的组织 ID。组织级端点必须有归属，
// 未解析到一律 400（鉴权由上游中间件保证，此处只判归属）。
func (h *OrgBillingHandler) resolveOrg(c *gin.Context) (uint64, bool) {
	orgID := tenant.OrgID(c)
	if orgID == 0 {
		JSONError(c, http.StatusBadRequest, "organization context required")
		return 0, false
	}
	return orgID, true
}

// handleOrgPlan 返回当前组织计划与各功能用量快照。
// GET /api/v1/org/plan
func (h *OrgBillingHandler) handleOrgPlan(c *gin.Context) {
	orgID, ok := h.resolveOrg(c)
	if !ok {
		return
	}
	snapshots, err := h.billing.GetOrganizationUsage(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error().Err(err).Uint64("organization_id", orgID).Msg("get organization usage failed")
		JSONError(c, http.StatusInternalServerError, "failed to load organization plan")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"organization_id": orgID, "features": snapshots})
}

// handleOrgUsage 返回按功能拆分、趋势与 Top 用户的多维度用量看板。
// GET /api/v1/org/usage?period=2026-08&feature=translation&top=10
func (h *OrgBillingHandler) handleOrgUsage(c *gin.Context) {
	orgID, ok := h.resolveOrg(c)
	if !ok {
		return
	}
	period := c.DefaultQuery("period", time.Now().UTC().Format("2006-01"))
	feature := c.Query("feature")
	topN, _ := strconv.Atoi(c.DefaultQuery("top", "10"))

	features, memberCount, err := h.usage.OrganizationBreakdown(c.Request.Context(), orgID, period)
	if err != nil {
		h.logger.Error().Err(err).Uint64("organization_id", orgID).Msg("organization breakdown failed")
		JSONError(c, http.StatusInternalServerError, "failed to load usage breakdown")
		return
	}

	payload := gin.H{
		"organization_id": orgID,
		"period":          period,
		"member_count":    memberCount,
		"features":        features,
	}

	if feature != "" {
		top, err := h.usage.TopUsersByFeature(c.Request.Context(), orgID, feature, period, topN)
		if err != nil {
			h.logger.Error().Err(err).Uint64("organization_id", orgID).Str("feature", feature).Msg("top users failed")
			JSONError(c, http.StatusInternalServerError, "failed to load top users")
			return
		}
		payload["top_users"] = top

		trend, err := h.usage.PeriodTrend(c.Request.Context(), orgID, feature, lastNPeriods(period, 6))
		if err != nil {
			h.logger.Error().Err(err).Uint64("organization_id", orgID).Str("feature", feature).Msg("period trend failed")
			JSONError(c, http.StatusInternalServerError, "failed to load usage trend")
			return
		}
		payload["trend"] = trend
	}

	JSONSuccess(c, http.StatusOK, payload)
}

// handleGetInvoice 返回指定发票与其行项目（组织隔离）。
// GET /api/v1/org/invoices/:invoiceNo
func (h *OrgBillingHandler) handleGetInvoice(c *gin.Context) {
	orgID, ok := h.resolveOrg(c)
	if !ok {
		return
	}
	invoiceNo := c.Param("invoiceNo")
	invoice, lines, err := h.invoices.GetInvoice(c.Request.Context(), orgID, invoiceNo)
	if err != nil {
		// 查不到是正常业务结果（也可能是查了别组织的单号），不应报 500。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			JSONError(c, http.StatusNotFound, "invoice not found")
			return
		}
		h.logger.Error().Err(err).Uint64("organization_id", orgID).Str("invoice_no", invoiceNo).Msg("get invoice failed")
		JSONError(c, http.StatusInternalServerError, "failed to load invoice")
		return
	}
	if invoice == nil {
		JSONError(c, http.StatusNotFound, "invoice not found")
		return
	}
	JSONSuccess(c, http.StatusOK, gin.H{"invoice": invoice, "lines": lines})
}

// lastNPeriods 生成以 period 结尾的最近 n 个月度周期键（YYYY-MM），用于趋势图。
func lastNPeriods(period string, n int) []string {
	end, err := time.Parse("2006-01", period)
	if err != nil {
		end = time.Now().UTC()
	}
	if n <= 0 {
		n = 6
	}
	out := make([]string, 0, n)
	for i := n - 1; i >= 0; i-- {
		out = append(out, end.AddDate(0, -i, 0).Format("2006-01"))
	}
	return out
}
