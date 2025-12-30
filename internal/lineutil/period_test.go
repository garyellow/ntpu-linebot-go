package lineutil

import (
	"testing"
)

func TestFormatCourseTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "morning period range",
			input:    "每週一1~2",
			expected: "每週一 1~2 (08:10-10:00)",
		},
		{
			name:     "afternoon period range",
			input:    "每週一5~6",
			expected: "每週一 5~6 (13:10-15:00)",
		},
		{
			name:     "evening period range",
			input:    "每週三10~11",
			expected: "每週三 10~11 (18:30-20:15)",
		},
		{
			name:     "late evening period range",
			input:    "每週五12~13",
			expected: "每週五 12~13 (20:25-22:10)",
		},
		{
			name:     "single period (3~3)",
			input:    "每週二3~3",
			expected: "每週二 3~3 (10:10-11:00)",
		},
		{
			name:     "long period range",
			input:    "每週四1~4",
			expected: "每週四 1~4 (08:10-12:00)",
		},
		{
			name:     "cross noon period range",
			input:    "每週一4~5",
			expected: "每週一 4~5 (11:10-14:00)",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no period pattern",
			input:    "每週一",
			expected: "每週一",
		},
		{
			name:     "invalid period number",
			input:    "每週一0~1",
			expected: "每週一0~1", // period 0 doesn't exist
		},
		{
			name:     "out of range period",
			input:    "每週一14~15",
			expected: "每週一14~15", // periods 14, 15 don't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FormatCourseTime(tt.input)
			if result != tt.expected {
				t.Errorf("FormatCourseTime(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatCourseTimes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "multiple times",
			input:    []string{"每週一5~6", "每週三10~11"},
			expected: []string{"每週一 5~6 (13:10-15:00)", "每週三 10~11 (18:30-20:15)"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single time",
			input:    []string{"每週二1~2"},
			expected: []string{"每週二 1~2 (08:10-10:00)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := FormatCourseTimes(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("FormatCourseTimes() returned %d items, want %d", len(result), len(tt.expected))
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("FormatCourseTimes()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestGetPeriodTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		period    int
		wantStart string
		wantEnd   string
	}{
		{
			name:      "period 1",
			period:    1,
			wantStart: "08:10",
			wantEnd:   "09:00",
		},
		{
			name:      "period 5",
			period:    5,
			wantStart: "13:10",
			wantEnd:   "14:00",
		},
		{
			name:      "period 10 (evening)",
			period:    10,
			wantStart: "18:30",
			wantEnd:   "19:20",
		},
		{
			name:      "period 13 (last)",
			period:    13,
			wantStart: "21:20",
			wantEnd:   "22:10",
		},
		{
			name:      "invalid period 0",
			period:    0,
			wantStart: "",
			wantEnd:   "",
		},
		{
			name:      "invalid period 14",
			period:    14,
			wantStart: "",
			wantEnd:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			startTime, endTime := GetPeriodTime(tt.period)
			if startTime != tt.wantStart {
				t.Errorf("GetPeriodTime(%d) start = %q, want %q", tt.period, startTime, tt.wantStart)
			}
			if endTime != tt.wantEnd {
				t.Errorf("GetPeriodTime(%d) end = %q, want %q", tt.period, endTime, tt.wantEnd)
			}
		})
	}
}

func TestGetPeriodRangeTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		startPeriod int
		endPeriod   int
		wantStart   string
		wantEnd     string
	}{
		{
			name:        "period 5~6",
			startPeriod: 5,
			endPeriod:   6,
			wantStart:   "13:10",
			wantEnd:     "15:00",
		},
		{
			name:        "period 1~4",
			startPeriod: 1,
			endPeriod:   4,
			wantStart:   "08:10",
			wantEnd:     "12:00",
		},
		{
			name:        "period 10~11",
			startPeriod: 10,
			endPeriod:   11,
			wantStart:   "18:30",
			wantEnd:     "20:15",
		},
		{
			name:        "invalid start period",
			startPeriod: 0,
			endPeriod:   6,
			wantStart:   "",
			wantEnd:     "",
		},
		{
			name:        "invalid end period",
			startPeriod: 5,
			endPeriod:   14,
			wantStart:   "",
			wantEnd:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			startTime, endTime := GetPeriodRangeTime(tt.startPeriod, tt.endPeriod)
			if startTime != tt.wantStart {
				t.Errorf("GetPeriodRangeTime(%d, %d) start = %q, want %q", tt.startPeriod, tt.endPeriod, startTime, tt.wantStart)
			}
			if endTime != tt.wantEnd {
				t.Errorf("GetPeriodRangeTime(%d, %d) end = %q, want %q", tt.startPeriod, tt.endPeriod, endTime, tt.wantEnd)
			}
		})
	}
}
