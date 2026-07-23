package commerce

import "testing"

func TestParseAppUserID(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    uint64
		wantErr bool
	}{
		{name: "prefixed", value: "user:123", want: 123},
		{name: "bare number", value: "123", want: 123},
		{name: "whitespace prefixed", value: " user:456 ", want: 456},
		{name: "zero bare", value: "0", wantErr: true},
		{name: "zero prefixed", value: "user:0", wantErr: true},
		{name: "non numeric", value: "abc", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "malformed prefix", value: "user:abc", wantErr: true},
		{name: "negative", value: "-5", wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAppUserID(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.value, err)
			}
			if got != tc.want {
				t.Fatalf("parseAppUserID(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
