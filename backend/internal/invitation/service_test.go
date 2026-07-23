package invitation

import (
	"strings"
	"testing"
)

func TestNormalizeLang(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"en", "en"},
		{"EN", "en"},
		{" Zh-CN ", "zh-cn"},
		{"  Fr ", "fr"},
	}
	for _, tc := range cases {
		if got := normalizeLang(tc.in); got != tc.want {
			t.Fatalf("normalizeLang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRandomCode(t *testing.T) {
	// 18 raw bytes -> base64.RawURLEncoding length = 4*ceil(18/3) = 24.
	const wantLen = 24
	seen := make(map[string]struct{}, 50)
	for i := 0; i < 50; i++ {
		code, err := randomCode()
		if err != nil {
			t.Fatalf("randomCode returned error: %v", err)
		}
		if len(code) != wantLen {
			t.Fatalf("randomCode length = %d, want %d (code=%q)", len(code), wantLen, code)
		}
		if strings.ContainsAny(code, "+/=") {
			t.Fatalf("randomCode is not url-safe: %q", code)
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("randomCode collision detected: %q", code)
		}
		seen[code] = struct{}{}
	}
}
