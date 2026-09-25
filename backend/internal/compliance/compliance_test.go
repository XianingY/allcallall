package compliance

import (
	"os"
	"strings"
	"testing"
)

func TestWatermarkDetectAndVerify(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	text := "尊敬的客户，这是AI生成的会议总结。"

	marked, _ := Watermark(text, "allcallall", key)
	if !strings.Contains(marked, AILabel("text")) {
		t.Fatal("marked text missing visible label")
	}
	detected, ok := Detect(marked)
	if !ok {
		t.Fatal("Detect failed to find marker")
	}
	if detected.Issuer != "allcallall" || detected.Version != 1 {
		t.Fatalf("unexpected meta: %+v", detected)
	}
	if !Verify(marked, "allcallall", key) {
		t.Fatal("Verify should pass for untampered marked text")
	}
}

func TestWatermarkDetectsTamper(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	marked, _ := Watermark("original content", "iss", key)
	tampered := marked + "\ninjected by attacker"
	if Verify(tampered, "iss", key) {
		t.Fatal("Verify should fail after tampering with body")
	}
}

func TestAILabelKinds(t *testing.T) {
	if AILabel("image") != "AI生成图片" {
		t.Fatal("image label mismatch")
	}
	if AILabel("text") != "由人工智能生成" {
		t.Fatal("default label mismatch")
	}
}

func TestValidateICPFormat(t *testing.T) {
	if !ValidateICPFormat("京ICP备12345678号-1") {
		t.Fatal("valid ICP rejected")
	}
	if ValidateICPFormat("not-a-icp") {
		t.Fatal("invalid ICP accepted")
	}
}

func TestICPRegistry(t *testing.T) {
	r := NewICPRegistry("京ICP备12345678号-1", "沪ICP备87654321号")
	if !r.IsRegistered("京ICP备12345678号-1") {
		t.Fatal("registered number not found")
	}
	if r.IsRegistered("粤ICP备00000000号") {
		t.Fatal("unregistered number wrongly accepted")
	}
}

func TestGenerativeAIFilingRoundtrip(t *testing.T) {
	f := GenerativeAIServiceFiling{
		ServiceName:           "AllCallAll AI",
		Domain:                "https://ai.allcallall.com",
		ICPNumber:             "京ICP备12345678号-1",
		AlgorithmFilingNumber: "网信算备XXXXXXXX号",
		Provider:              "self-hosted",
		ModelList:             []string{"llama-3-70b", "qwen2-72b"},
		Owner:                 "AllCallAll Inc.",
		Status:                "filed",
	}
	js, err := f.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseGenerativeAIFiling(js)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.AlgorithmFilingNumber != f.AlgorithmFilingNumber || len(parsed.ModelList) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", parsed)
	}
}

func TestAssessPostureReflectsEnv(t *testing.T) {
	t.Setenv("MESSAGE_RETENTION_ENABLED", "true")
	t.Setenv("MESSAGE_ENCRYPTION_ENABLED", "true")
	t.Setenv("SECURITY_REQUIRE_TLS", "true")
	t.Setenv("ICP_LICENSE_NUMBER", "京ICP备12345678号-1")

	p := AssessPosture()
	byID := map[string]Control{}
	for _, c := range p.Controls {
		byID[c.ID] = c
	}
	if byID["message_retention"].Status != ControlEnabled {
		t.Fatal("expected message_retention enabled")
	}
	if byID["icp_registered"].Status != ControlEnabled {
		t.Fatal("expected icp_registered enabled")
	}
	if byID["content_moderation"].Status != ControlDisabled {
		t.Fatal("expected content_moderation disabled when env unset")
	}
	if p.ReadinessPct <= 0 || p.ReadinessPct > 100 {
		t.Fatalf("invalid readiness pct: %d", p.ReadinessPct)
	}
}

// TestAssessPostureDoesNotSelfCertify 锁定一个曾经真实存在过的坑：
// .github/workflows/compliance-annual.yml 曾把九个开关硬编码为 "true" 注入，
// 而 AssessPosture 只读环境变量，于是年检恒为达标、fail 分支不可达。
// 开关未配置时，态势必须如实反映为不达标。
func TestAssessPostureDoesNotSelfCertify(t *testing.T) {
	for _, name := range []string{
		"MESSAGE_RETENTION_ENABLED", "MESSAGE_ENCRYPTION_ENABLED", "MESSAGE_RECALL_ENABLED",
		"SEARCH_INDEX_ENABLED", "CONTENT_MODERATION_ENABLED", "MESSAGE_ERASURE_ENABLED",
		"REALNAME_VERIFICATION_ENABLED", "SECURITY_REQUIRE_TLS", "AIGC_LABELING_ENABLED",
		"KMS_PROVIDER", "MESSAGE_ENCRYPTION_MASTER_KEY", "ICP_LICENSE_NUMBER",
	} {
		if prev, existed := os.LookupEnv(name); existed {
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("unset %s failed: %v", name, err)
			}
			t.Cleanup(func() { _ = os.Setenv(name, prev) })
		}
	}

	p := AssessPosture()
	for _, c := range p.Controls {
		if c.Status == ControlEnabled {
			t.Fatalf("control %s must not read as enabled with nothing configured", c.ID)
		}
	}
	if p.ReadinessPct != 0 {
		t.Fatalf("expected 0%% readiness when unconfigured, got %d", p.ReadinessPct)
	}
	if p.GAReady {
		t.Fatal("posture must not self-certify GA readiness when unconfigured")
	}
}
