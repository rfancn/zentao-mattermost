// Package dedupe 提供进程内的 webhook 去重能力。
package dedupe

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/rfancn/zentao-mattermost/g"
)

type store struct {
	mu         sync.Mutex
	entries    map[string]time.Time
	ttl        time.Duration
	maxEntries int
}

var current = &store{
	entries:    make(map[string]time.Time),
	ttl:        10 * time.Minute,
	maxEntries: 2048,
}

// Init 根据全局配置初始化去重存储。
func Init() {
	current = &store{
		entries:    make(map[string]time.Time),
		ttl:        g.Config.DedupeTTL(),
		maxEntries: g.Config.Dedupe.MaxEntries,
	}
}

// Key 计算 webhook 请求体的 SHA-256。
func Key(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Begin 尝试登记一个去重键。
// 返回 true 表示可以继续处理；返回 false 表示重复，应跳过。
func Begin(key string) bool {
	return current.begin(key)
}

// Cancel 在通知失败时移除去重键，让禅道可以重试。
func Cancel(key string) {
	current.cancel(key)
}

func (s *store) begin(key string) bool {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, t := range s.entries {
		if now.Sub(t) >= s.ttl {
			delete(s.entries, k)
		}
	}

	if _, exists := s.entries[key]; exists {
		return false
	}

	if len(s.entries) >= s.maxEntries {
		var oldestKey string
		var oldest time.Time
		for k, t := range s.entries {
			if oldestKey == "" || t.Before(oldest) {
				oldestKey, oldest = k, t
			}
		}
		if oldestKey != "" {
			delete(s.entries, oldestKey)
		}
	}

	s.entries[key] = now
	return true
}

func (s *store) cancel(key string) {
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}
