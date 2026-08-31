// Package compliance implements China-facing compliance helpers: AI-generated
// content watermarking/labeling, ICP license validation, generative-AI service
// filing metadata, and a production-readiness posture assessment.
package compliance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
)

const watermarkVersion = 1

// WatermarkMeta describes an AI-generated-content marker.
type WatermarkMeta struct {
	Version int    `json:"v"`
	Issuer  string `json:"iss"`
	Sig     string `json:"sig"`
}

var markerRE = regexp.MustCompile(`>>>AIGC v(\d+) sig=([A-Za-z0-9_-]+) iss=([^\n<]*)<<<`)

// AILabel returns the regulation-compliant visible label for AI-generated content
// (生成式人工智能服务管理暂行办法 requires clear labeling of AIGC).
func AILabel(kind string) string {
	switch strings.ToLower(kind) {
	case "image", "img":
		return "AI生成图片"
	case "video":
		return "AI生成视频"
	case "audio":
		return "AI生成音频"
	default:
		return "由人工智能生成"
	}
}

func sha256B64(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func sign(visible, issuer string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(sha256B64(visible) + "|" + issuer))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Watermark marks AI-generated text with a visible label and a tamper-evident,
// machine-detectable marker. The signature covers the content hash + issuer, so
// editing the visible text invalidates the marker (Verify returns false).
func Watermark(text, issuer string, key []byte) (string, WatermarkMeta) {
	visible := strings.TrimRight(text, "\n")
	meta := WatermarkMeta{Version: watermarkVersion, Issuer: issuer, Sig: sign(visible, issuer, key)}
	marked := visible + "\n" + AILabel("text") +
		"\n>>>AIGC v" + strconv.Itoa(meta.Version) + " sig=" + meta.Sig + " iss=" + issuer + "<<<"
	return marked, meta
}

// Detect finds a watermark marker (if present) and returns its metadata.
func Detect(text string) (*WatermarkMeta, bool) {
	m := markerRE.FindStringSubmatch(text)
	if m == nil {
		return nil, false
	}
	v, _ := strconv.Atoi(m[1])
	return &WatermarkMeta{Version: v, Issuer: m[3], Sig: m[2]}, true
}

// Verify checks the watermark signature against the content + key. A tampered
// body returns false.
func Verify(text, issuer string, key []byte) bool {
	meta, ok := Detect(text)
	if !ok {
		return false
	}
	visible := strings.TrimRight(text, "\n")
	lines := strings.Split(visible, "\n")
	// Last line is the marker, second-last is the visible label; content is the rest.
	content := ""
	if len(lines) >= 2 {
		content = strings.Join(lines[:len(lines)-2], "\n")
	}
	expected := sign(content, issuer, key)
	return hmac.Equal([]byte(meta.Sig), []byte(expected))
}
