package app

import (
	"testing"
	"time"
)

func TestIsMaintenanceDue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC)
	interval := 10 * time.Minute

	tests := []struct {
		name     string
		lastUnix int64
		interval time.Duration
		expected bool
	}{
		{
			name:     "interval disabled",
			lastUnix: now.Unix(),
			interval: 0,
			expected: false,
		},
		{
			name:     "never ran",
			lastUnix: 0,
			interval: interval,
			expected: true,
		},
		{
			name:     "not due",
			lastUnix: now.Add(-5 * time.Minute).Unix(),
			interval: interval,
			expected: false,
		},
		{
			name:     "due",
			lastUnix: now.Add(-15 * time.Minute).Unix(),
			interval: interval,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isMaintenanceDue(tt.lastUnix, tt.interval, now); got != tt.expected {
				t.Fatalf("isMaintenanceDue()=%v, want %v", got, tt.expected)
			}
		})
	}
}
