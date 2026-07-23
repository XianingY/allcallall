package commerce

import (
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestNormalizeFollowUpTaskType(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "callback", want: models.FollowupTaskTypeCallback},
		{in: " Callback ", want: models.FollowupTaskTypeCallback},
		{in: "SEND_MESSAGE", want: models.FollowupTaskTypeSendMessage},
		{in: "Schedule_Next_Call", want: models.FollowupTaskTypeScheduleNextCall},
		{in: "bogus", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeFollowUpTaskType(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeFollowUpTaskType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeFollowUpTaskStatus(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "open", want: models.FollowupTaskStatusOpen},
		{in: " DONE ", want: models.FollowupTaskStatusDone},
		{in: "Snoozed", want: models.FollowupTaskStatusSnoozed},
		{in: "CANCELLED", want: models.FollowupTaskStatusCancelled},
		{in: "bogus", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeFollowUpTaskStatus(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeFollowUpTaskStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMustJSON(t *testing.T) {
	if got := mustJSON([]string{"a", "b"}); got != `["a","b"]` {
		t.Fatalf("mustJSON = %q", got)
	}
	if got := mustJSON([]string{}); got != `[]` {
		t.Fatalf("mustJSON(empty) = %q", got)
	}
	// A nil slice marshals to "null".
	if got := mustJSON(nil); got != `null` {
		t.Fatalf("mustJSON(nil) = %q", got)
	}
}

func TestExtractTexts(t *testing.T) {
	segments := []models.CallTranscriptSegment{
		{OriginalText: "orig1", TranslatedText: "tr1"},
		{OriginalText: "orig2", TranslatedText: ""},
		{OriginalText: "   ", TranslatedText: "   "},
		{OriginalText: "", TranslatedText: ""},
	}
	got := extractTexts(segments)
	want := []string{"tr1", "orig2"}
	if len(got) != len(want) {
		t.Fatalf("extractTexts len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extractTexts[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTruncateSentence(t *testing.T) {
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{in: "short", limit: 10, want: "short"},
		{in: "  padded  ", limit: 100, want: "padded"},
		{in: "abcdefghij", limit: 5, want: "abcde..."},
		{in: "abcdefghij", limit: 0, want: "abcdefghij"},
		{in: "hello world", limit: 5, want: "hello..."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			if got := truncateSentence(tc.in, tc.limit); got != tc.want {
				t.Fatalf("truncateSentence(%q,%d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
	}
}

func TestPeerNameOrEmail(t *testing.T) {
	cases := []struct {
		name  string
		email string
		want  string
	}{
		{name: "Alice", email: "a@x.com", want: "Alice"},
		{name: "  Bob  ", email: "b@x.com", want: "Bob"},
		{name: "", email: "c@x.com", want: "c@x.com"},
		{name: "  ", email: " d@x.com ", want: "d@x.com"},
		{name: "", email: "", want: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"/"+tc.email, func(t *testing.T) {
			if got := peerNameOrEmail(tc.name, tc.email); got != tc.want {
				t.Fatalf("peerNameOrEmail(%q,%q) = %q, want %q", tc.name, tc.email, got, tc.want)
			}
		})
	}
}
