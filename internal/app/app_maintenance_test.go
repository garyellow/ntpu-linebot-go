package app

import (
	"testing"
	"time"
)

func TestMaintenanceCheckInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		refresh  time.Duration
		cleanup  time.Duration
		expected time.Duration
	}{
		{
			name:     "disabled intervals",
			refresh:  0,
			cleanup:  0,
			expected: 0,
		},
		{
			name:     "min interval floor",
			refresh:  30 * time.Second,
			cleanup:  0,
			expected: time.Minute,
		},
		{
			name:     "uses smaller positive interval",
			refresh:  2 * time.Hour,
			cleanup:  20 * time.Minute,
			expected: 2 * time.Minute,
		},
		{
			name:     "caps at max interval",
			refresh:  5 * time.Hour,
			cleanup:  0,
			expected: 15 * time.Minute,
		},
		{
			name:     "cleanup only",
			refresh:  0,
			cleanup:  10 * time.Minute,
			expected: time.Minute,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := maintenanceCheckInterval(tt.refresh, tt.cleanup); got != tt.expected {
				t.Fatalf("maintenanceCheckInterval()=%v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMinPositiveDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		a        time.Duration
		b        time.Duration
		expected time.Duration
	}{
		{
			name:     "both zero",
			a:        0,
			b:        0,
			expected: 0,
		},
		{
			name:     "a zero",
			a:        0,
			b:        5 * time.Minute,
			expected: 5 * time.Minute,
		},
		{
			name:     "b zero",
			a:        3 * time.Minute,
			b:        0,
			expected: 3 * time.Minute,
		},
		{
			name:     "a smaller",
			a:        3 * time.Minute,
			b:        10 * time.Minute,
			expected: 3 * time.Minute,
		},
		{
			name:     "b smaller",
			a:        10 * time.Minute,
			b:        3 * time.Minute,
			expected: 3 * time.Minute,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := minPositiveDuration(tt.a, tt.b); got != tt.expected {
				t.Fatalf("minPositiveDuration()=%v, want %v", got, tt.expected)
			}
		})
	}
}

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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isMaintenanceDue(tt.lastUnix, tt.interval, now); got != tt.expected {
				t.Fatalf("isMaintenanceDue()=%v, want %v", got, tt.expected)
			}
		})
	}
}
