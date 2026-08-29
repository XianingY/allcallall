package compliance

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// GenerativeAIServiceFiling captures the metadata required for the 生成式人工智能
// 服务算法备案 (CAC algorithm filing) and ICP alignment. Persist it (signed) for
// audits and regulator requests.
type GenerativeAIServiceFiling struct {
	ServiceName           string   `json:"service_name"`
	Domain                string   `json:"domain"`
	ICPNumber             string   `json:"icp_number"`
	AlgorithmFilingNumber string   `json:"algorithm_filing_number"` // 算法备案号
	Provider              string   `json:"provider"`
	ModelList             []string `json:"model_list"`
	Owner                 string   `json:"owner"`
	Contact               string   `json:"contact"`
	FilingDate            string   `json:"filing_date"`
	Status                string   `json:"status"` // filed | pending | exempt
}

// ToJSON serializes the filing for storage / regulator submission.
func (f GenerativeAIServiceFiling) ToJSON() (string, error) {
	b, err := json.MarshalIndent(f, "", "  ")
	return string(b), err
}

// ParseGenerativeAIFiling parses a filing from JSON.
func ParseGenerativeAIFiling(jsonStr string) (*GenerativeAIServiceFiling, error) {
	var f GenerativeAIServiceFiling
	if err := json.Unmarshal([]byte(jsonStr), &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ControlStatus is a posture control state.
type ControlStatus string

const (
	ControlEnabled  ControlStatus = "enabled"
	ControlDisabled ControlStatus = "disabled"
	ControlPartial  ControlStatus = "partial"
)

// Control is a single compliance posture control.
type Control struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Status ControlStatus `json:"status"`
	Note   string        `json:"note,omitempty"`
}

// CompliancePosture is a point-in-time readiness assessment.
type CompliancePosture struct {
	GeneratedAt  string    `json:"generated_at"`
	Controls     []Control `json:"controls"`
	ReadinessPct int       `json:"readiness_pct"`
	GAReady      bool      `json:"ga_ready"`
}

func envBool(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

// AssessPosture evaluates the production compliance posture from configuration.
// Controls map 1:1 to the privacy 7-piece suite plus transport/labeling/ICP/KMS.
func AssessPosture() CompliancePosture {
	controls := []Control{
		{ID: "message_retention", Name: "服务端留存TTL与物理清理", Status: boolStatus(envBool("MESSAGE_RETENTION_ENABLED")), Note: "MESSAGE_RETENTION_ENABLED"},
		{ID: "message_encryption", Name: "消息正文信封加密", Status: boolStatus(envBool("MESSAGE_ENCRYPTION_ENABLED")), Note: "MESSAGE_ENCRYPTION_ENABLED + KMS主密钥"},
		{ID: "message_recall", Name: "撤回(销毁正文/附件)", Status: boolStatus(envBool("MESSAGE_RECALL_ENABLED")), Note: "MESSAGE_RECALL_ENABLED"},
		{ID: "search_index_min", Name: "搜索索引最小化", Status: boolStatus(envBool("SEARCH_INDEX_ENABLED")), Note: "SEARCH_INDEX_ENABLED"},
		{ID: "content_moderation", Name: "内容审核异步处置", Status: boolStatus(envBool("CONTENT_MODERATION_ENABLED")), Note: "CONTENT_MODERATION_ENABLED"},
		{ID: "message_erasure", Name: "被遗忘权(用户/组织级擦除)", Status: boolStatus(envBool("MESSAGE_ERASURE_ENABLED")), Note: "MESSAGE_ERASURE_ENABLED(新增规范flag)"},
		{ID: "realname_verification", Name: "实名核验与P2", Status: boolStatus(envBool("REALNAME_VERIFICATION_ENABLED")), Note: "REALNAME_VERIFICATION_ENABLED(新增规范flag)"},
		{ID: "tls_required", Name: "传输层强制TLS", Status: boolStatus(envBool("SECURITY_REQUIRE_TLS")), Note: "SECURITY_REQUIRE_TLS"},
		{ID: "aigc_labeling", Name: "AI生成内容水印标识", Status: boolStatus(envBool("AIGC_LABELING_ENABLED")), Note: "AIGC_LABELING_ENABLED"},
		{ID: "kms_master_key", Name: "KMS主密钥管理", Status: kmsStatus(), Note: "MESSAGE_ENCRYPTION_MASTER_KEY 或 KMS_PROVIDER=cloud"},
		{ID: "icp_registered", Name: "ICP经营许可", Status: icpStatus(), Note: "ICP_LICENSE_NUMBER"},
	}

	enabled := 0
	for _, c := range controls {
		if c.Status == ControlEnabled {
			enabled++
		}
	}
	pct := 0
	if len(controls) > 0 {
		pct = enabled * 100 / len(controls)
	}
	return CompliancePosture{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Controls:     controls,
		ReadinessPct: pct,
		GAReady:      pct >= 90,
	}
}

func boolStatus(on bool) ControlStatus {
	if on {
		return ControlEnabled
	}
	return ControlDisabled
}

func kmsStatus() ControlStatus {
	if strings.TrimSpace(os.Getenv("KMS_PROVIDER")) == "cloud" {
		return ControlEnabled
	}
	if strings.TrimSpace(os.Getenv("MESSAGE_ENCRYPTION_MASTER_KEY")) != "" {
		return ControlEnabled
	}
	return ControlDisabled
}

func icpStatus() ControlStatus {
	if strings.TrimSpace(os.Getenv("ICP_LICENSE_NUMBER")) != "" {
		return ControlEnabled
	}
	return ControlDisabled
}
