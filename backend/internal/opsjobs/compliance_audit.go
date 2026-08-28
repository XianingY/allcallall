package opsjobs

import (
	"fmt"
	"os"
	"time"

	"github.com/allcallall/backend/internal/compliance"
)

// ComplianceAuditItem is one line of the annual self-audit checklist.
type ComplianceAuditItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"` // ok | warn | fail
	Detail         string `json:"detail,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
}

// AnnualComplianceAudit is the point-in-time self-audit report produced yearly
// (and reviewable any time). It reuses the runtime posture assessment and adds
// expiry-aware checks for licenses, filings and certificates.
type AnnualComplianceAudit struct {
	GeneratedAt time.Time                    `json:"generated_at"`
	Overall     string                       `json:"overall"` // pass | review | fail
	Posture     compliance.CompliancePosture `json:"posture"`
	Items       []ComplianceAuditItem        `json:"items"`
}

// RunAnnualAudit evaluates the production compliance posture plus expiry checks.
// It is purely environment-driven (no DB), so it can run in CI or the opsaudit
// CLI without standing up the full stack.
func RunAnnualAudit() AnnualComplianceAudit {
	posture := compliance.AssessPosture()
	items := buildAuditItems(posture)
	overall := "pass"
	for _, it := range items {
		switch it.Status {
		case "fail":
			overall = "fail"
		case "warn":
			if overall != "fail" {
				overall = "review"
			}
		}
	}
	return AnnualComplianceAudit{
		GeneratedAt: time.Now().UTC(),
		Overall:     overall,
		Posture:     posture,
		Items:       items,
	}
}

func buildAuditItems(posture compliance.CompliancePosture) []ComplianceAuditItem {
	items := make([]ComplianceAuditItem, 0, 8)

	// 1. Runtime posture readiness
	status := "ok"
	if !posture.GAReady {
		status = "warn"
	}
	items = append(items, ComplianceAuditItem{
		ID:     "runtime_posture",
		Name:   "运行时合规态势（隐私7件套+传输+标识+KMS+ICP）",
		Status: status,
		Detail: fmt.Sprintf("就绪度 %d%%，GA 就绪=%t", posture.ReadinessPct, posture.GAReady),
	})

	// 2. ICP 经营许可证格式与到期
	icp := os.Getenv("ICP_LICENSE_NUMBER")
	if icp == "" {
		items = append(items, ComplianceAuditItem{
			ID:             "icp_license",
			Name:           "ICP 经营许可证",
			Status:         "fail",
			Detail:         "未配置 ICP_LICENSE_NUMBER",
			Recommendation: "完成 ICP 经营性备案并注入 Secret",
		})
	} else {
		valid := compliance.ValidateICPFormat(icp)
		st := "ok"
		if !valid {
			st = "fail"
		}
		items = append(items, ComplianceAuditItem{
			ID:     "icp_license",
			Name:   "ICP 经营许可证",
			Status: st,
			Detail: icp,
		})
	}

	// 3. 生成式 AI 算法备案号
	filing := os.Getenv("AI_ALGORITHM_FILING_NUMBER")
	if filing == "" {
		items = append(items, ComplianceAuditItem{
			ID:             "ai_filing",
			Name:           "生成式 AI 算法备案（CAC）",
			Status:         "warn",
			Detail:         "未配置 AI_ALGORITHM_FILING_NUMBER",
			Recommendation: "向网信办完成算法备案并登记备案号",
		})
	} else {
		items = append(items, ComplianceAuditItem{
			ID:     "ai_filing",
			Name:   "生成式 AI 算法备案（CAC）",
			Status: "ok",
			Detail: filing,
		})
	}

	// 4. TLS 证书到期（可选，配置 TLS_CERT_EXPIRY_DATE=YYYY-MM-DD）
	if exp := os.Getenv("TLS_CERT_EXPIRY_DATE"); exp != "" {
		if t, err := time.Parse("2006-01-02", exp); err == nil {
			days := time.Until(t).Hours() / 24
			st := "ok"
			rec := ""
			if days < 0 {
				st = "fail"
				rec = "证书已过期，立即续签"
			} else if days < 30 {
				st = "warn"
				rec = "证书即将到期，安排续签"
			}
			items = append(items, ComplianceAuditItem{
				ID:             "tls_cert",
				Name:           "TLS 证书有效期",
				Status:         st,
				ExpiresAt:      exp,
				Recommendation: rec,
			})
		}
	}

	// 5. KMS 主密钥轮换（可选，配置 KMS_KEY_ROTATED_AT=YYYY-MM-DD）
	if rotated := os.Getenv("KMS_KEY_ROTATED_AT"); rotated != "" {
		if t, err := time.Parse("2006-01-02", rotated); err == nil {
			days := time.Until(t).Hours() / 24
			st := "ok"
			rec := ""
			if days < -90 {
				st = "warn"
				rec = "主密钥超过 90 天未轮换，建议按年度/季度策略轮换"
			}
			items = append(items, ComplianceAuditItem{
				ID:             "kms_rotation",
				Name:           "KMS 主密钥轮换",
				Status:         st,
				Detail:         rotated,
				Recommendation: rec,
			})
		}
	}

	// 6. AIGC 标识能力
	aigc := envOn("AIGC_LABELING_ENABLED")
	items = append(items, ComplianceAuditItem{
		ID:     "aigc_labeling",
		Name:   "AI 生成内容水印/标识",
		Status: boolToStatus(aigc),
		Detail: "AIGC_LABELING_ENABLED=" + b2s(aigc),
	})

	// 7. 等保测评对接（可选，配置 MLPS_GRADE 与 MLPS_CERT_NO）
	if grade := os.Getenv("MLPS_GRADE"); grade != "" {
		items = append(items, ComplianceAuditItem{
			ID:     "mlps",
			Name:   "等保测评定级（" + grade + "级）",
			Status: "ok",
			Detail: "MLPS_CERT_NO=" + os.Getenv("MLPS_CERT_NO"),
		})
	} else {
		items = append(items, ComplianceAuditItem{
			ID:             "mlps",
			Name:           "等保测评定级",
			Status:         "warn",
			Detail:         "未配置 MLPS_GRADE",
			Recommendation: "完成等保2.0定级备案与测评对接",
		})
	}

	return items
}

func envOn(name string) bool {
	v := os.Getenv(name)
	return v == "1" || v == "true" || v == "yes"
}

func b2s(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func boolToStatus(b bool) string {
	if b {
		return "ok"
	}
	return "fail"
}
