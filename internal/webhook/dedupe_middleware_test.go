package webhook

import (
	"testing"
	"time"
)

func TestBeginDuplicateAndCancel(t *testing.T) {
	dedupes = &dedupeStore{entries: make(map[string]time.Time), ttl: time.Minute, maxEntries: 10}

	key := dedupeHash([]byte(`{"a":1}`))
	if !dedupes.begin(key) {
		t.Fatal("首次 dedupes.begin() = false")
	}
	if dedupes.begin(key) {
		t.Fatal("重复 dedupes.begin() = true")
	}

	dedupes.cancel(key)
	if !dedupes.begin(key) {
		t.Fatal("Cancel 后 dedupes.begin() = false")
	}
}

func TestBeginExpiresEntries(t *testing.T) {
	dedupes = &dedupeStore{entries: make(map[string]time.Time), ttl: 10 * time.Millisecond, maxEntries: 10}
	key := dedupeHash([]byte("expired"))

	if !dedupes.begin(key) {
		t.Fatal("首次 dedupes.begin() = false")
	}
	time.Sleep(20 * time.Millisecond)
	if !dedupes.begin(key) {
		t.Fatal("过期后 dedupes.begin() = false")
	}
}

func TestBeginEvictsOldestWhenFull(t *testing.T) {
	now := time.Now()
	dedupes = &dedupeStore{
		entries: map[string]time.Time{
			"old": now.Add(-2 * time.Minute),
			"new": now.Add(-time.Minute),
		},
		ttl:        time.Hour,
		maxEntries: 2,
	}

	if !dedupes.begin("latest") {
		t.Fatal("dedupes.begin() = false")
	}
	if _, ok := dedupes.entries["old"]; ok {
		t.Error("最旧的记录未被淘汰")
	}
	if _, ok := dedupes.entries["new"]; !ok {
		t.Error("较新的记录被误淘汰")
	}
}

func TestInitUsesDefaults(t *testing.T) {
	dedupes = newDedupeStore()

	if dedupes.ttl != dedupeTTL {
		t.Errorf("ttl = %v", dedupes.ttl)
	}
	if dedupes.maxEntries != dedupeMaxEntries {
		t.Errorf("maxEntries = %d", dedupes.maxEntries)
	}
}
