package contact

import (
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

func TestContactOrgCacheEmptyOnStart(t *testing.T) {
	t.Parallel()

	cache := NewContactOrgCache(time.Minute)
	contacts, ok := cache.GetCached("教務處")
	if ok {
		t.Fatalf("expected cache miss on empty cache, got %d contacts", len(contacts))
	}
}

func TestContactOrgCacheStoresAndReturnsBeforeTTL(t *testing.T) {
	t.Parallel()

	cache := NewContactOrgCache(time.Minute)
	orgName := "教務處"
	members := []storage.Contact{
		{Name: "王小明", Type: "individual", Organization: orgName},
		{Name: "李大華", Type: "individual", Organization: orgName},
	}

	cache.SetCached(orgName, members)

	got, ok := cache.GetCached(orgName)
	if !ok {
		t.Fatal("expected cache hit after SetCached")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(got))
	}
	if got[0].Name != "王小明" || got[1].Name != "李大華" {
		t.Fatalf("unexpected contact names: %v", got)
	}
}

func TestContactOrgCacheSkipsEmptySlice(t *testing.T) {
	t.Parallel()

	cache := NewContactOrgCache(time.Minute)
	cache.SetCached("教務處", []storage.Contact{})
	cache.SetCached("學務處", nil)

	if _, ok := cache.GetCached("教務處"); ok {
		t.Fatal("expected cache miss after setting empty slice")
	}
	if _, ok := cache.GetCached("學務處"); ok {
		t.Fatal("expected cache miss after setting nil slice")
	}
}

func TestContactOrgCacheExpiredAfterTTL(t *testing.T) {
	t.Parallel()

	cache := NewContactOrgCache(20 * time.Millisecond)
	orgName := "教務處"
	cache.SetCached(orgName, []storage.Contact{
		{Name: "王小明", Type: "individual", Organization: orgName},
	})

	// Should be fresh immediately
	if _, ok := cache.GetCached(orgName); !ok {
		t.Fatal("expected cache hit before TTL")
	}

	time.Sleep(40 * time.Millisecond)

	if _, ok := cache.GetCached(orgName); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestContactOrgCacheReturnsCopy(t *testing.T) {
	t.Parallel()

	cache := NewContactOrgCache(time.Minute)
	orgName := "教務處"
	original := []storage.Contact{
		{Name: "王小明", Type: "individual", Organization: orgName},
	}

	cache.SetCached(orgName, original)

	got, ok := cache.GetCached(orgName)
	if !ok {
		t.Fatal("expected cache hit")
	}

	// Mutating the returned slice must not affect the cache
	got[0].Name = "已修改"
	got2, _ := cache.GetCached(orgName)
	if got2[0].Name != "王小明" {
		t.Fatalf("cache returned mutable reference; expected 王小明, got %q", got2[0].Name)
	}
}
