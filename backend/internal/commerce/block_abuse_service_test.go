package commerce

import (
	"sort"
	"testing"
)

func TestNormalizeReportCategory(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "spam", want: "spam"},
		{in: " Spam ", want: "spam"},
		{in: "HARASSMENT", want: "harassment"},
		{in: "Impersonation", want: "impersonation"},
		{in: "FRAUD", want: "fraud"},
		{in: "sexual_content", want: "sexual_content"},
		{in: "other", want: "other"},
		{in: "bogus", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeReportCategory(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for category %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeReportCategory(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestReportCategoryListConsistency ensures the exported category list, the
// internal allow-list, and the service accessor all agree.
func TestReportCategoryListConsistency(t *testing.T) {
	fromFunc := reportCategoryList()
	if len(fromFunc) != len(allowedReportCategories) {
		t.Fatalf("reportCategoryList length %d != allowedReportCategories length %d", len(fromFunc), len(allowedReportCategories))
	}
	for _, c := range fromFunc {
		if _, ok := allowedReportCategories[c]; !ok {
			t.Fatalf("category %q is in reportCategoryList but not in allowedReportCategories", c)
		}
		// Every listed category must round-trip through normalization.
		if _, err := normalizeReportCategory(c); err != nil {
			t.Fatalf("category %q failed normalization: %v", c, err)
		}
	}

	// ReportCategories() (the service accessor) must return the same set.
	svc := NewBlockAbuseService(nil)
	fromSvc := svc.ReportCategories()
	if len(fromSvc) != len(fromFunc) {
		t.Fatalf("ReportCategories length %d != reportCategoryList length %d", len(fromSvc), len(fromFunc))
	}
	a := append([]string(nil), fromSvc...)
	sort.Strings(a)
	b := append([]string(nil), fromFunc...)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("ReportCategories mismatch at %d: %q vs %q", i, a[i], b[i])
		}
	}
}
