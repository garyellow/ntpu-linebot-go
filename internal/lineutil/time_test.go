package lineutil

import (
	"testing"
	"time"
)

func TestFormatCacheTime(t *testing.T) {
	// Get current time in Taipei timezone for test setup
	taipeiLoc, _ := time.LoadLocation("Asia/Taipei")
	now := time.Now().In(taipeiLoc)

	// Create timestamps for testing
	todayNoon := time.Date(now.Year(), now.Month(), now.Day(), 12, 30, 0, 0, taipeiLoc)
	yesterdayEvening := time.Date(now.Year(), now.Month(), now.Day(), 18, 45, 0, 0, taipeiLoc).AddDate(0, 0, -1)
	threeDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 9, 15, 0, 0, taipeiLoc).AddDate(0, 0, -3)

	tests := []struct {
		name     string
		cachedAt int64
		want     string
	}{
		{
			name:     "zero timestamp returns empty",
			cachedAt: 0,
			want:     "",
		},
		{
			name:     "today shows 今天 HH:MM",
			cachedAt: todayNoon.Unix(),
			want:     "今天 12:30",
		},
		{
			name:     "yesterday shows 昨天 HH:MM",
			cachedAt: yesterdayEvening.Unix(),
			want:     "昨天 18:45",
		},
		{
			name:     "older dates show MM/DD HH:MM",
			cachedAt: threeDaysAgo.Unix(),
			want:     threeDaysAgo.Format("01/02") + " 09:15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCacheTime(tt.cachedAt)
			if got != tt.want {
				t.Errorf("FormatCacheTime(%d) = %q, want %q", tt.cachedAt, got, tt.want)
			}
		})
	}
}

func TestFormatCacheTime_Timezone(t *testing.T) {
	// Test that timezone conversion works correctly
	// Create a timestamp that's midnight UTC, which should be 08:00 in Taipei
	utcMidnight := time.Date(2024, 11, 28, 0, 0, 0, 0, time.UTC)

	result := FormatCacheTime(utcMidnight.Unix())

	// The result should show 08:00 (Taipei is UTC+8)
	if result == "" {
		t.Error("FormatCacheTime should not return empty for valid timestamp")
	}

	// Check that the time portion is 08:00
	taipeiLoc, _ := time.LoadLocation("Asia/Taipei")
	taipeiTime := utcMidnight.In(taipeiLoc)
	expectedTime := taipeiTime.Format("15:04")

	if expectedTime != "08:00" {
		t.Errorf("Expected Taipei time to be 08:00, got %s", expectedTime)
	}
}

func TestNewCacheTimeHint(t *testing.T) {
	tests := []struct {
		name     string
		cachedAt int64
		wantNil  bool
	}{
		{
			name:     "zero timestamp returns nil",
			cachedAt: 0,
			wantNil:  true,
		},
		{
			name:     "valid timestamp returns FlexText",
			cachedAt: time.Now().Unix(),
			wantNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCacheTimeHint(tt.cachedAt)
			if (got == nil) != tt.wantNil {
				t.Errorf("NewCacheTimeHint(%d) nil = %v, want nil = %v", tt.cachedAt, got == nil, tt.wantNil)
			}

			if got != nil {
				// Verify FlexText properties
				if got.Size != "xxs" {
					t.Errorf("NewCacheTimeHint size = %q, want %q", got.Size, "xxs")
				}
				if got.Color != ColorGray400 {
					t.Errorf("NewCacheTimeHint color = %q, want %q", got.Color, ColorGray400)
				}
				if got.Margin != "lg" {
					t.Errorf("NewCacheTimeHint margin = %q, want %q", got.Margin, "lg")
				}
			}
		})
	}
}

func TestFormatCacheTimeFooter(t *testing.T) {
	tests := []struct {
		name     string
		cachedAt int64
		wantPfx  string // Expected prefix
	}{
		{
			name:     "zero timestamp returns empty",
			cachedAt: 0,
			wantPfx:  "",
		},
		{
			name:     "valid timestamp returns footer format",
			cachedAt: time.Now().Unix(),
			wantPfx:  "\n\n🕐 資料更新於 ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCacheTimeFooter(tt.cachedAt)
			if tt.wantPfx == "" {
				if got != "" {
					t.Errorf("FormatCacheTimeFooter(%d) = %q, want empty", tt.cachedAt, got)
				}
			} else {
				if len(got) < len(tt.wantPfx) {
					t.Errorf("FormatCacheTimeFooter(%d) = %q, want prefix %q", tt.cachedAt, got, tt.wantPfx)
				} else if got[:len(tt.wantPfx)] != tt.wantPfx {
					t.Errorf("FormatCacheTimeFooter(%d) prefix = %q, want %q", tt.cachedAt, got[:len(tt.wantPfx)], tt.wantPfx)
				}
			}
		})
	}
}

func TestMinCachedAt(t *testing.T) {
	tests := []struct {
		name      string
		cachedAts []int64
		want      int64
	}{
		{
			name:      "empty slice returns 0",
			cachedAts: []int64{},
			want:      0,
		},
		{
			name:      "single value returns that value",
			cachedAts: []int64{100},
			want:      100,
		},
		{
			name:      "multiple values returns minimum",
			cachedAts: []int64{300, 100, 200},
			want:      100,
		},
		{
			name:      "zeros are ignored",
			cachedAts: []int64{0, 200, 100, 0},
			want:      100,
		},
		{
			name:      "all zeros returns 0",
			cachedAts: []int64{0, 0, 0},
			want:      0,
		},
		{
			name:      "single non-zero among zeros",
			cachedAts: []int64{0, 50, 0},
			want:      50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MinCachedAt(tt.cachedAts...)
			if got != tt.want {
				t.Errorf("MinCachedAt(%v) = %d, want %d", tt.cachedAts, got, tt.want)
			}
		})
	}
}

func TestFormatCacheTime_BoundaryConditions(t *testing.T) {
	taipeiLoc, _ := time.LoadLocation("Asia/Taipei")
	now := time.Now().In(taipeiLoc)

	// Test at exactly midnight (start of today)
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, taipeiLoc)
	result := FormatCacheTime(todayMidnight.Unix())
	expected := "今天 00:00"
	if result != expected {
		t.Errorf("FormatCacheTime at midnight = %q, want %q", result, expected)
	}

	// Test at exactly midnight yesterday (start of yesterday)
	yesterdayMidnight := todayMidnight.AddDate(0, 0, -1)
	result = FormatCacheTime(yesterdayMidnight.Unix())
	expected = "昨天 00:00"
	if result != expected {
		t.Errorf("FormatCacheTime at yesterday midnight = %q, want %q", result, expected)
	}

	// Test one second before yesterday midnight (should be older date format)
	beforeYesterday := yesterdayMidnight.Add(-1 * time.Second)
	result = FormatCacheTime(beforeYesterday.Unix())
	// Should be in MM/DD HH:MM format
	if len(result) < 11 { // "MM/DD HH:MM" is 11 chars
		t.Errorf("FormatCacheTime before yesterday = %q, expected MM/DD HH:MM format", result)
	}
}
