package lineutil

import (
	"fmt"
	"time"
)

// Taipei timezone for consistent time display and scheduling
var taipeiTZ *time.Location

func init() {
	var err error
	taipeiTZ, err = time.LoadLocation("Asia/Taipei")
	if err != nil {
		// Fallback to UTC+8 if timezone data is not available
		taipeiTZ = time.FixedZone("Asia/Taipei", 8*60*60)
	}
}

// GetTaipeiLocation returns the Taiwan (Asia/Taipei) timezone location.
// Use this for scheduling jobs and time calculations that should be in Taiwan time.
func GetTaipeiLocation() *time.Location {
	return taipeiTZ
}

// FormatCacheTime formats a Unix timestamp into a user-friendly time string.
// Uses Asia/Taipei timezone for consistency.
//
// Returns:
//   - "今天 HH:MM" if the timestamp is today
//   - "昨天 HH:MM" if the timestamp is yesterday
//   - "MM/DD HH:MM" for other dates
//
// Example:
//
//	FormatCacheTime(1732780800) -> "今天 14:30" (if today)
//	FormatCacheTime(1732694400) -> "昨天 09:15" (if yesterday)
//	FormatCacheTime(1732608000) -> "11/26 18:00" (older dates)
func FormatCacheTime(cachedAt int64) string {
	if cachedAt == 0 {
		return ""
	}

	t := time.Unix(cachedAt, 0).In(taipeiTZ)
	now := time.Now().In(taipeiTZ)

	// Get start of today and yesterday
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, taipeiTZ)
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	timeStr := t.Format("15:04")

	if t.After(todayStart) || t.Equal(todayStart) {
		return fmt.Sprintf("今天 %s", timeStr)
	}
	if t.After(yesterdayStart) || t.Equal(yesterdayStart) {
		return fmt.Sprintf("昨天 %s", timeStr)
	}

	// For older dates, show MM/DD HH:MM
	return t.Format("01/02 15:04")
}

// NewCacheTimeHint creates a Flex Text component for displaying cache time in Flex Messages.
// Style: xxs size, gray color (ColorGray400), right-aligned, with lg top margin.
// Designed to be unobtrusive while providing useful information.
//
// Returns nil if cachedAt is 0 (no cache time available).
//
// Example usage in Flex Message body:
//
//	body.AddComponent(NewCacheTimeHint(student.CachedAt).FlexText)
func NewCacheTimeHint(cachedAt int64) *FlexText {
	timeStr := FormatCacheTime(cachedAt)
	if timeStr == "" {
		return nil
	}

	return NewFlexText(fmt.Sprintf("🕐 %s 更新", timeStr)).
		WithSize("xxs").
		WithColor(ColorGray400).
		WithAlign("end").
		WithMargin("lg")
}

// FormatCacheTimeFooter formats cache time for plain text message footers.
// Returns a formatted string suitable for appending to text message endings.
//
// Returns empty string if cachedAt is 0.
//
// Example:
//
//	FormatCacheTimeFooter(1732780800) -> "\n\n🕐 資料更新於 今天 14:30"
func FormatCacheTimeFooter(cachedAt int64) string {
	timeStr := FormatCacheTime(cachedAt)
	if timeStr == "" {
		return ""
	}

	return fmt.Sprintf("\n\n🕐 資料更新於 %s", timeStr)
}

// MinCachedAt returns the minimum (oldest) timestamp from a list of timestamps.
// Returns 0 if no timestamps are provided or all are 0.
// This is useful for getting the oldest cache time from a list of records.
//
// Example:
//
//	MinCachedAt(1732780800, 1732694400, 1732608000) -> 1732608000
func MinCachedAt(cachedAts ...int64) int64 {
	if len(cachedAts) == 0 {
		return 0
	}

	var minTime int64
	for _, t := range cachedAts {
		if t == 0 {
			continue
		}
		if minTime == 0 || t < minTime {
			minTime = t
		}
	}

	return minTime
}

// ================================================
// Data Source & Availability Hints
// ================================================

// NewDataRangeHint creates a standardized hint about ID data availability.
// Returns a Flex Text showing data range.
//
// This provides transparency about data limitations.
func NewDataRangeHint() *FlexText {
	return NewFlexText("📊 資料範圍：94-113 學年度").
		WithSize("xxs").
		WithColor(ColorGray400).
		WithAlign("end").
		WithMargin("sm")
}
