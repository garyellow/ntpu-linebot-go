// Package session provides lightweight in-memory per-user conversation context.
// It tracks recent intents to help NLU disambiguation without persistent storage.
package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Intent represents a single parsed intent record.
type Intent struct {
	Module string            // e.g., "course", "contact", "id", "program"
	Action string            // e.g., "search", "smart", "emergency"
	Params map[string]string // e.g., {"query": "微積分"}
	Time   time.Time
}

// userSession holds recent intents for a single user.
type userSession struct {
	mu      sync.Mutex
	intents []Intent // circular buffer, newest at end
	maxSize int
}

// Store is a concurrent-safe per-user session store.
// Each user's context expires after TTL and is limited to maxIntents entries.
type Store struct {
	sessions   sync.Map // map[string]*userSession
	maxIntents int
	ttl        time.Duration
}

// NewStore creates a new session store.
// maxIntents controls how many recent intents to keep per user (typically 3).
// ttl controls how long intents remain valid (typically 5 minutes).
func NewStore(maxIntents int, ttl time.Duration) *Store {
	return &Store{
		maxIntents: maxIntents,
		ttl:        ttl,
	}
}

// Record adds a new intent to the user's session.
// Old intents beyond maxIntents are dropped. Expired intents are pruned.
func (s *Store) Record(userID string, intent Intent) {
	if userID == "" {
		return
	}
	intent.Time = time.Now()

	val, _ := s.sessions.LoadOrStore(userID, &userSession{
		intents: make([]Intent, 0, s.maxIntents),
		maxSize: s.maxIntents,
	})
	sess, _ := val.(*userSession)

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Prune expired and rebuild slice to allow GC of old Params maps
	cutoff := time.Now().Add(-s.ttl)
	filtered := make([]Intent, 0, sess.maxSize)
	for _, i := range sess.intents {
		if i.Time.After(cutoff) {
			filtered = append(filtered, i)
		}
	}

	// Append and trim to max size
	filtered = append(filtered, intent)
	if len(filtered) > sess.maxSize {
		filtered = filtered[len(filtered)-sess.maxSize:]
	}
	sess.intents = filtered
}

// GetRecentIntents returns non-expired recent intents for a user (oldest first).
func (s *Store) GetRecentIntents(userID string) []Intent {
	if userID == "" {
		return nil
	}
	val, ok := s.sessions.Load(userID)
	if !ok {
		return nil
	}
	sess, _ := val.(*userSession)

	sess.mu.Lock()
	defer sess.mu.Unlock()

	cutoff := time.Now().Add(-s.ttl)
	var result []Intent
	for _, i := range sess.intents {
		if i.Time.After(cutoff) {
			result = append(result, i)
		}
	}
	return result
}

// FormatContext returns a human-readable context string for NLU prompting.
// Returns empty string if no recent context exists.
// Example: "[前文：課程搜尋(微積分) → 聯絡搜尋(王教授)]"
func (s *Store) FormatContext(userID string) string {
	intents := s.GetRecentIntents(userID)
	if len(intents) == 0 {
		return ""
	}

	parts := make([]string, 0, len(intents))
	for _, i := range intents {
		label := formatIntentLabel(i)
		if label != "" {
			parts = append(parts, label)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "[前文：" + strings.Join(parts, " → ") + "]"
}

// Cleanup removes expired sessions. Call periodically to prevent memory growth.
func (s *Store) Cleanup() {
	cutoff := time.Now().Add(-s.ttl)
	s.sessions.Range(func(key, value any) bool {
		sess, _ := value.(*userSession)
		sess.mu.Lock()
		hasValid := false
		for _, i := range sess.intents {
			if i.Time.After(cutoff) {
				hasValid = true
				break
			}
		}
		if !hasValid {
			// Delete while holding the lock to prevent a concurrent Record()
			// from inserting a fresh intent between unlock and delete.
			s.sessions.Delete(key)
		}
		sess.mu.Unlock()
		return true
	})
}

// formatIntentLabel creates a concise Chinese label for an intent.
func formatIntentLabel(i Intent) string {
	moduleLabels := map[string]string{
		"course":  "課程",
		"contact": "聯絡",
		"id":      "學號",
		"program": "學程",
	}

	label, ok := moduleLabels[i.Module]
	if !ok {
		return ""
	}

	// Add action context
	switch i.Action {
	case "search":
		label += "搜尋"
	case "smart":
		label += "智慧搜尋"
	case "emergency":
		label += "緊急"
	case "list":
		label += "列表"
	default:
		label += "查詢"
	}

	// Add query parameter if present
	if q, ok := i.Params["query"]; ok && q != "" {
		label = fmt.Sprintf("%s(%s)", label, q)
	} else if q, ok := i.Params["keyword"]; ok && q != "" {
		label = fmt.Sprintf("%s(%s)", label, q)
	} else if q, ok := i.Params["name"]; ok && q != "" {
		label = fmt.Sprintf("%s(%s)", label, q)
	}

	return label
}
