package dedupe

import (
	"testing"
	"time"

	"github.com/rfancn/zentao-mattermost/g"
)

func TestBeginDuplicateAndCancel(t *testing.T) {
	current = &store{entries: make(map[string]time.Time), ttl: time.Minute, maxEntries: 10}

	key := Key([]byte(`{"a":1}`))
	if !Begin(key) {
		t.Fatal("首次 Begin() = false")
	}
	if Begin(key) {
		t.Fatal("重复 Begin() = true")
	}

	Cancel(key)
	if !Begin(key) {
		t.Fatal("Cancel 后 Begin() = false")
	}
}

func TestBeginExpiresEntries(t *testing.T) {
	current = &store{entries: make(map[string]time.Time), ttl: 10 * time.Millisecond, maxEntries: 10}
	key := Key([]byte("expired"))

	if !Begin(key) {
		t.Fatal("首次 Begin() = false")
	}
	time.Sleep(20 * time.Millisecond)
	if !Begin(key) {
		t.Fatal("过期后 Begin() = false")
	}
}

func TestBeginEvictsOldestWhenFull(t *testing.T) {
	now := time.Now()
	current = &store{
		entries: map[string]time.Time{
			"old": now.Add(-2 * time.Minute),
			"new": now.Add(-time.Minute),
		},
		ttl:        time.Hour,
		maxEntries: 2,
	}

	if !Begin("latest") {
		t.Fatal("Begin() = false")
	}
	if _, ok := current.entries["old"]; ok {
		t.Error("最旧的记录未被淘汰")
	}
	if _, ok := current.entries["new"]; !ok {
		t.Error("较新的记录被误淘汰")
	}
}

func TestInitReadsGlobalConfig(t *testing.T) {
	g.Config.Dedupe = g.DedupeConfig{TTLSeconds: 3, MaxEntries: 7}
	Init()

	if current.ttl != 3*time.Second {
		t.Errorf("ttl = %v", current.ttl)
	}
	if current.maxEntries != 7 {
		t.Errorf("maxEntries = %d", current.maxEntries)
	}
}
