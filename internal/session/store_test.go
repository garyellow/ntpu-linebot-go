package session

import (
	"testing"
	"time"
)

func TestStoreRecordAndGet(t *testing.T) {
	t.Parallel()
	s := NewStore(3, 5*time.Minute)

	s.Record("user1", Intent{Module: "course", Action: "search", Params: map[string]string{"query": "微積分"}})
	s.Record("user1", Intent{Module: "contact", Action: "search", Params: map[string]string{"query": "王教授"}})

	intents := s.GetRecentIntents("user1")
	if len(intents) != 2 {
		t.Fatalf("expected 2 intents, got %d", len(intents))
	}
	if intents[0].Module != "course" {
		t.Errorf("expected first intent module 'course', got %q", intents[0].Module)
	}
	if intents[1].Module != "contact" {
		t.Errorf("expected second intent module 'contact', got %q", intents[1].Module)
	}
}

func TestStoreMaxIntents(t *testing.T) {
	t.Parallel()
	s := NewStore(2, 5*time.Minute)

	s.Record("user1", Intent{Module: "course", Action: "search"})
	s.Record("user1", Intent{Module: "contact", Action: "search"})
	s.Record("user1", Intent{Module: "id", Action: "search"})

	intents := s.GetRecentIntents("user1")
	if len(intents) != 2 {
		t.Fatalf("expected 2 intents (max), got %d", len(intents))
	}
	// Oldest should be dropped
	if intents[0].Module != "contact" {
		t.Errorf("expected oldest kept intent 'contact', got %q", intents[0].Module)
	}
	if intents[1].Module != "id" {
		t.Errorf("expected newest intent 'id', got %q", intents[1].Module)
	}
}

func TestStoreTTLExpiry(t *testing.T) {
	t.Parallel()
	s := NewStore(3, 100*time.Millisecond)

	s.Record("user1", Intent{Module: "course", Action: "search"})
	time.Sleep(150 * time.Millisecond)

	intents := s.GetRecentIntents("user1")
	if len(intents) != 0 {
		t.Errorf("expected 0 intents after TTL, got %d", len(intents))
	}
}

func TestStoreEmptyUser(t *testing.T) {
	t.Parallel()
	s := NewStore(3, 5*time.Minute)

	intents := s.GetRecentIntents("nonexistent")
	if intents != nil {
		t.Errorf("expected nil for non-existent user, got %v", intents)
	}

	// Empty userID should be no-op
	s.Record("", Intent{Module: "course"})
	intents = s.GetRecentIntents("")
	if intents != nil {
		t.Errorf("expected nil for empty userID, got %v", intents)
	}
}

func TestFormatContext(t *testing.T) {
	t.Parallel()
	s := NewStore(3, 5*time.Minute)

	// Empty
	ctx := s.FormatContext("user1")
	if ctx != "" {
		t.Errorf("expected empty context for new user, got %q", ctx)
	}

	s.Record("user1", Intent{Module: "course", Action: "search", Params: map[string]string{"query": "微積分"}})
	s.Record("user1", Intent{Module: "contact", Action: "search", Params: map[string]string{"query": "王教授"}})

	ctx = s.FormatContext("user1")
	expected := "[前文：課程搜尋(微積分) → 聯絡搜尋(王教授)]"
	if ctx != expected {
		t.Errorf("expected %q, got %q", expected, ctx)
	}
}

func TestFormatContextUnknownModule(t *testing.T) {
	t.Parallel()
	s := NewStore(3, 5*time.Minute)

	s.Record("user1", Intent{Module: "help", Action: ""})

	ctx := s.FormatContext("user1")
	if ctx != "" {
		t.Errorf("expected empty context for unknown module, got %q", ctx)
	}
}

func TestCleanup(t *testing.T) {
	t.Parallel()
	s := NewStore(3, 100*time.Millisecond)

	s.Record("user1", Intent{Module: "course"})
	s.Record("user2", Intent{Module: "contact"})
	time.Sleep(150 * time.Millisecond)

	// user3 added after expiry of user1/user2
	s.Record("user3", Intent{Module: "id"})

	s.Cleanup()

	// user1 and user2 should be cleaned up
	if s.GetRecentIntents("user1") != nil {
		t.Error("user1 should have been cleaned up")
	}
	if s.GetRecentIntents("user2") != nil {
		t.Error("user2 should have been cleaned up")
	}
	// user3 should still exist
	if len(s.GetRecentIntents("user3")) != 1 {
		t.Error("user3 should still have 1 intent")
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	s := NewStore(5, 5*time.Minute)

	done := make(chan struct{})
	for i := range 10 {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			userID := "user1"
			for range 100 {
				s.Record(userID, Intent{Module: "course", Action: "search"})
				s.GetRecentIntents(userID)
				s.FormatContext(userID)
			}
			_ = id
		}(i)
	}
	for range 10 {
		<-done
	}
}
