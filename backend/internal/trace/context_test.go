package trace

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeRequestID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trims", in: " req-1 ", want: "req-1"},
		{name: "empty", in: " ", want: ""},
		{name: "control character", in: "req\n1", want: ""},
		{name: "space inside", in: "req 1", want: ""},
		{name: "too long", in: strings.Repeat("a", MaxRequestIDLength+1), want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeRequestID(tc.in); got != tc.want {
				t.Fatalf("unexpected request id: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestEnsureRequestID(t *testing.T) {
	if got := EnsureRequestID("req-1"); got != "req-1" {
		t.Fatalf("unexpected preserved request id: %q", got)
	}
	generated := EnsureRequestID("req\n1")
	if generated == "" || generated == "req\n1" {
		t.Fatalf("expected generated request id, got %q", generated)
	}
}

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-context")
	if got := RequestID(ctx); got != "req-context" {
		t.Fatalf("unexpected request id: %q", got)
	}
}
