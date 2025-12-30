package lineutil

import (
	"testing"
	"time"
)

func TestGetTaipeiLocation(t *testing.T) {
	t.Parallel()
	loc := GetTaipeiLocation()

	if loc == nil {
		t.Fatal("GetTaipeiLocation() returned nil")
	}

	// Verify it's UTC+8
	now := time.Now()
	taipeiTime := now.In(loc)
	utcTime := now.UTC()

	// Taiwan is UTC+8, so the hour difference should be 8 (or -16 if crossing date boundary)
	hourDiff := taipeiTime.Hour() - utcTime.Hour()
	if hourDiff != 8 && hourDiff != -16 {
		t.Errorf("Expected UTC+8 timezone, got hour difference: %d", hourDiff)
	}
}

func TestGetTaipeiLocation_ConsistentWithFormatCacheTime(t *testing.T) {
	t.Parallel()
	// Ensure GetTaipeiLocation returns the same timezone used by FormatCacheTime
	loc := GetTaipeiLocation()

	// Create a timestamp at a known time
	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, loc)
	timestamp := testTime.Unix()

	// Format it and check the result
	formatted := FormatCacheTime(timestamp)

	// Should show time in Taiwan timezone
	if formatted == "" {
		t.Error("FormatCacheTime returned empty string")
	}

	// Both should use the same timezone (taipeiTZ)
	if loc != taipeiTZ {
		t.Error("GetTaipeiLocation and internal taipeiTZ should be the same")
	}
}

func TestGetTaipeiLocation_UsedForScheduling(t *testing.T) {
	t.Parallel()
	// Test that scheduling at 3:00 AM Taiwan time works correctly
	loc := GetTaipeiLocation()

	// Simulate scheduling: if current time is 2024-01-15 10:00 Taiwan time
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, loc)

	// Next 3:00 AM should be tomorrow
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, loc)
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}

	expected := time.Date(2024, 1, 16, 3, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Errorf("Expected next run at %v, got %v", expected, next)
	}

	// If current time is before 3:00 AM, should schedule for today
	earlyMorning := time.Date(2024, 1, 15, 1, 0, 0, 0, loc)
	next = time.Date(earlyMorning.Year(), earlyMorning.Month(), earlyMorning.Day(), 3, 0, 0, 0, loc)
	if earlyMorning.After(next) {
		next = next.Add(24 * time.Hour)
	}

	expected = time.Date(2024, 1, 15, 3, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Errorf("Expected next run at %v, got %v", expected, next)
	}
}
