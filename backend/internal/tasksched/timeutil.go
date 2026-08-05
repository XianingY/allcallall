package tasksched

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WeekdayMask 将星期几位图返回。0=周日 ... 6=周六，与 time.Weekday 一致。
// WeekdayMask converts a list of weekday indices (0=Sunday..6=Saturday) into a bitmask.
func WeekdayMask(weekdays []int) uint8 {
	var mask uint8
	for _, d := range weekdays {
		if d < 0 || d > 6 {
			continue
		}
		mask |= 1 << uint(d)
	}
	return mask
}

// weekdayMatch 判断某个 time.Weekday 是否落在位图中。
func weekdayMatch(wd time.Weekday, mask uint8) bool {
	if mask == 0 {
		return false
	}
	return mask&(1<<uint(wd)) != 0
}

// parseRunTime 解析 "HH:MM" 为时/分，非法或缺失时回退 09:00。
func parseRunTime(value string) (int, int) {
	value = strings.TrimSpace(value)
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return 9, 0
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 9, 0
	}
	return hh, mm
}

// LoadLocation 解析 IANA 时区，失败时回退 UTC 并上报错误。
func LoadLocation(tz string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	return loc, nil
}

// ComputeNextRun 计算在 after 之后、符合「星期几位图 + 每日触发时刻 + 间隔周数」的
// 下一次运行时间。anchor 为任务锚点日期（通常为创建时间），用于对齐间隔周数；
// 仅当 candidate 距 anchor 的整周数能被 intervalWeeks 整除时才命中。
//
// 返回零值 time.Time 与 false 表示无法计算（如位图为空或间隔非法）。
func ComputeNextRun(after time.Time, loc *time.Location, mask uint8, runTime string, intervalWeeks int, anchor time.Time) (time.Time, bool) {
	if mask == 0 || intervalWeeks < 1 || loc == nil {
		return time.Time{}, false
	}
	hh, mm := parseRunTime(runTime)
	a := after.In(loc)

	// 从「今天 runTime」开始按天扫描，扫描窗口覆盖 intervalWeeks 个周期 + 余量。
	start := time.Date(a.Year(), a.Month(), a.Day(), hh, mm, 0, 0, loc)
	if start.Before(a) {
		// 今天的触发时刻已过，从明天开始扫描。
		start = start.AddDate(0, 0, 1)
	}

	window := intervalWeeks*7 + 14
	for d := 0; d < window; d++ {
		cand := start.AddDate(0, 0, d)
		if !weekdayMatch(cand.Weekday(), mask) {
			continue
		}
		weeksSince := int(cand.Sub(anchor).Hours() / (24 * 7))
		if weeksSince < 0 {
			weeksSince = 0
		}
		if weeksSince%intervalWeeks != 0 {
			continue
		}
		if cand.After(a) {
			return cand, true
		}
	}
	return time.Time{}, false
}
