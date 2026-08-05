package tasksched

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed.UTC()
}

func TestWeekdayMask(t *testing.T) {
	mask := WeekdayMask([]int{1, 2, 3, 4, 5}) // Mon..Fri
	if mask != 62 {
		t.Fatalf("expected mask 62 for Mon..Fri, got %d", mask)
	}
	if weekdayMatch(time.Sunday, mask) {
		t.Fatalf("Sunday should not be in Mon..Fri mask")
	}
	if !weekdayMatch(time.Wednesday, mask) {
		t.Fatalf("Wednesday should be in Mon..Fri mask")
	}
	// 越界值被忽略
	if WeekdayMask([]int{0, 9, -1}) != 1 {
		t.Fatalf("expected mask 1 (only Sunday) after filtering invalid")
	}
}

func TestComputeNextRun(t *testing.T) {
	loc := time.UTC
	maskMonFri := WeekdayMask([]int{1, 2, 3, 4, 5})
	anchor := mustTime(t, "2026-08-05T00:00:00Z") // 周三

	cases := []struct {
		name     string
		after    string
		expected string // RFC3339 或空串表示无结果
		interval int
	}{
		{"当天未到点->当天09:00", "2026-08-05T08:00:00Z", "2026-08-05T09:00:00Z", 1},
		{"当天已过->次日09:00(周四)", "2026-08-05T10:00:00Z", "2026-08-06T09:00:00Z", 1},
		{"周五过后->下周一09:00", "2026-08-07T10:00:00Z", "2026-08-10T09:00:00Z", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ComputeNextRun(mustTime(t, tc.after), loc, maskMonFri, "09:00", tc.interval, anchor)
			if tc.expected == "" {
				if ok {
					t.Fatalf("expected no next run, got %v", got)
				}
				return
			}
			if !ok {
				t.Fatalf("expected next run %s, got none", tc.expected)
			}
			if !got.Equal(mustTime(t, tc.expected)) {
				t.Fatalf("expected %s, got %s", tc.expected, got.Format(time.RFC3339))
			}
		})
	}
}

func TestComputeNextRunIntervalWeeks(t *testing.T) {
	loc := time.UTC
	maskSunday := WeekdayMask([]int{0}) // 仅周日
	anchor := mustTime(t, "2026-08-02T00:00:00Z") // 周日

	// 每 2 周一次，after 为周三，下一次应为 2 周后的周日 2026-08-16
	got, ok := ComputeNextRun(mustTime(t, "2026-08-05T09:00:00Z"), loc, maskSunday, "09:00", 2, anchor)
	if !ok {
		t.Fatalf("expected a next run")
	}
	want := mustTime(t, "2026-08-16T09:00:00Z")
	if !got.Equal(want) {
		t.Fatalf("expected %s (every 2 weeks), got %s", want.Format(time.RFC3339), got.Format(time.RFC3339))
	}
}

func TestComputeNextRunInvalid(t *testing.T) {
	loc := time.UTC
	anchor := mustTime(t, "2026-08-05T00:00:00Z")
	// 空位图 / 非法间隔 -> 无结果
	if _, ok := ComputeNextRun(time.Now(), loc, 0, "09:00", 1, anchor); ok {
		t.Fatalf("empty mask should yield no run")
	}
	if _, ok := ComputeNextRun(time.Now(), loc, 62, "09:00", 0, anchor); ok {
		t.Fatalf("interval<1 should yield no run")
	}
	if _, ok := ComputeNextRun(time.Now(), nil, 62, "09:00", 1, anchor); ok {
		t.Fatalf("nil location should yield no run")
	}
}

func TestParseRunTime(t *testing.T) {
	if h, m := parseRunTime("09:00"); h != 9 || m != 0 {
		t.Fatalf("expected 9:00, got %d:%d", h, m)
	}
	if h, m := parseRunTime("bad"); h != 9 || m != 0 {
		t.Fatalf("invalid run time should fall back to 9:00, got %d:%d", h, m)
	}
	if h, m := parseRunTime("25:00"); h != 9 || m != 0 {
		t.Fatalf("out-of-range should fall back to 9:00, got %d:%d", h, m)
	}
}
